package fbhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"
)

// Wiki pages belong to a project and live inside the project's own folder:
//
//	/Projects/<project>/wiki/<page>.json
//	/Projects/<project>/wiki/.history/<page>/<version>.json
//
// Like the project document, a page is opaque JSON apart from the envelope
// fields (id, title, parentId, version, updatedAt). The Markdown source sits
// in the "body" field. Because pages are ordinary Drive files they are covered
// by Drive sync, backups, and share links with no extra plumbing, and the
// "kind" field leaves room for other page types (for example a service desk
// knowledge-base article) to reuse the same storage and API.
//
// Every save keeps the version it replaces under .history so pages have a
// Confluence-style revision trail.

const (
	wikiDirName    = "wiki"
	wikiHistoryDir = ".history"
	maxWikiBytes   = 1 << 20 // 1 MiB per page
	maxWikiPages   = 500
	maxWikiHistory = 50
	maxWikiDepth   = 32
)

// wikiMu serialises wiki writes so the version check, the history copy and
// the rename are atomic with respect to other requests in this process.
var wikiMu sync.Mutex

type wikiEnvelope struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	ParentID  string `json:"parentId"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

func wikiDirPath(project string) string {
	return path.Join(projectsDir, project, wikiDirName)
}

func wikiPagePath(project, page string) string {
	return path.Join(wikiDirPath(project), page+".json")
}

func wikiHistoryPath(project, page string, version int) string {
	return path.Join(wikiDirPath(project), wikiHistoryDir, page, strconv.Itoa(version)+".json")
}

// wikiIDs validates the {id} and {page} route variables.
func wikiIDs(r *http.Request, needPage bool) (project, page string, err error) {
	vars := mux.Vars(r)
	project = vars["id"]
	if !projectSlugRe.MatchString(project) {
		return "", "", errors.New("invalid project id")
	}
	if needPage {
		page = vars["page"]
		if !projectSlugRe.MatchString(page) {
			return "", "", errors.New("invalid page id")
		}
	}
	return project, page, nil
}

func readWikiFile(d *data, p string) ([]byte, *wikiEnvelope, error) {
	if !d.CheckRules(p) {
		return nil, nil, os.ErrPermission
	}
	raw, err := afero.ReadFile(d.user.Fs, p)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > maxWikiBytes+(64<<10) {
		return nil, nil, errors.New("wiki page too large")
	}
	var env wikiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, nil, err
	}
	return raw, &env, nil
}

func readWikiPage(d *data, project, page string) ([]byte, *wikiEnvelope, error) {
	return readWikiFile(d, wikiPagePath(project, page))
}

// listWikiPages returns the envelope of every page in a project, in id order.
func listWikiPages(d *data, project string) ([]*wikiEnvelope, [][]byte, error) {
	entries, err := afero.ReadDir(d.user.Fs, wikiDirPath(project))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var envs []*wikiEnvelope
	var raws [][]byte
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !projectSlugRe.MatchString(id) {
			continue
		}
		if len(envs) >= maxWikiPages {
			break
		}
		raw, env, err := readWikiPage(d, project, id)
		if err != nil {
			continue
		}
		envs = append(envs, env)
		raws = append(raws, raw)
	}
	return envs, raws, nil
}

// wikiParentError checks that parent exists and that making it the parent of
// page cannot create a cycle.
func wikiParentError(d *data, project, page, parent string) error {
	if parent == "" {
		return nil
	}
	if !projectSlugRe.MatchString(parent) {
		return errors.New("invalid parent id")
	}
	cur := parent
	for i := 0; i < maxWikiDepth; i++ {
		if cur == page {
			return errors.New("a page cannot be nested under itself")
		}
		_, env, err := readWikiPage(d, project, cur)
		if err != nil {
			if os.IsNotExist(err) {
				return errors.New("parent page not found")
			}
			return err
		}
		if env.ParentID == "" {
			return nil
		}
		cur = env.ParentID
	}
	return errors.New("page tree is nested too deeply")
}

// pruneWikiHistory keeps only the newest maxWikiHistory revisions of a page.
func pruneWikiHistory(d *data, project, page string) {
	dir := path.Dir(wikiHistoryPath(project, page, 1))
	entries, err := afero.ReadDir(d.user.Fs, dir)
	if err != nil {
		return
	}
	var versions []int
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || n < 1 {
			continue
		}
		versions = append(versions, n)
	}
	if len(versions) <= maxWikiHistory {
		return
	}
	sort.Ints(versions)
	for _, v := range versions[:len(versions)-maxWikiHistory] {
		_ = d.user.Fs.Remove(wikiHistoryPath(project, page, v))
	}
}

// wikiListHandler returns every page of a project without the page bodies, so
// the page tree stays cheap to load. Fetch a single page for its body.
var wikiListHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	project, _, err := wikiIDs(r, false)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if _, _, err := readProject(d, project); err != nil {
		return errToStatus(err), err
	}

	out := make([]json.RawMessage, 0)
	_, raws, err := listWikiPages(d, project)
	if err != nil {
		return errToStatus(err), err
	}
	for _, raw := range raws {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue
		}
		delete(doc, "body")
		light, err := json.Marshal(doc)
		if err != nil {
			continue
		}
		out = append(out, light)
	}
	return renderJSON(w, r, out)
})

var wikiGetHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	project, page, err := wikiIDs(r, true)
	if err != nil {
		return http.StatusBadRequest, err
	}
	raw, _, err := readWikiPage(d, project, page)
	if err != nil {
		return errToStatus(err), err
	}
	return renderJSON(w, r, json.RawMessage(raw))
})

// wikiPutHandler creates or updates a page. As with projects, the client sends
// the version it last saw and a mismatch returns 409; a new page is version 0.
var wikiPutHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	project, page, err := wikiIDs(r, true)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if r.Body == nil {
		return http.StatusBadRequest, nil
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWikiBytes+(64<<10)))
	if err != nil {
		return http.StatusRequestEntityTooLarge, err
	}

	var env wikiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return http.StatusBadRequest, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return http.StatusBadRequest, err
	}
	if env.ID != page || strings.TrimSpace(env.Title) == "" || len(env.Title) > 200 {
		return http.StatusBadRequest, errors.New("id/title mismatch or missing")
	}
	var pageBody string
	if rawBody, ok := doc["body"]; ok {
		if err := json.Unmarshal(rawBody, &pageBody); err != nil {
			return http.StatusBadRequest, errors.New("body must be a string")
		}
	}
	if len(pageBody) > maxWikiBytes {
		return http.StatusRequestEntityTooLarge, errors.New("page body too large")
	}

	p := wikiPagePath(project, page)
	if !d.CheckRules(p) {
		return http.StatusForbidden, nil
	}

	wikiMu.Lock()
	defer wikiMu.Unlock()

	// A page can only be added to a project that exists.
	if _, _, err := readProject(d, project); err != nil {
		return errToStatus(err), err
	}
	if err := wikiParentError(d, project, page, env.ParentID); err != nil {
		return http.StatusBadRequest, err
	}

	current := -1
	var previous []byte
	if raw, existing, err := readWikiPage(d, project, page); err == nil {
		current = existing.Version
		previous = raw
	} else if !os.IsNotExist(err) {
		return errToStatus(err), err
	}

	if current < 0 {
		if !d.user.Perm.Create {
			return http.StatusForbidden, nil
		}
		if env.Version != 0 {
			return http.StatusConflict, nil
		}
		if envs, _, err := listWikiPages(d, project); err == nil && len(envs) >= maxWikiPages {
			return http.StatusRequestEntityTooLarge, errors.New("too many wiki pages in this project")
		}
	} else {
		if !d.user.Perm.Modify {
			return http.StatusForbidden, nil
		}
		if env.Version != current {
			return http.StatusConflict, nil
		}
	}

	doc["version"], _ = json.Marshal(env.Version + 1)
	doc["updatedAt"], _ = json.Marshal(time.Now().UTC().Format(time.RFC3339))
	out, err := json.Marshal(doc)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	if err := d.user.Fs.MkdirAll(path.Dir(p), 0o750); err != nil {
		return errToStatus(err), err
	}

	// Keep the revision being replaced before it is overwritten.
	if previous != nil {
		hp := wikiHistoryPath(project, page, current)
		if err := d.user.Fs.MkdirAll(path.Dir(hp), 0o750); err != nil {
			return errToStatus(err), err
		}
		if err := afero.WriteFile(d.user.Fs, hp, previous, 0o640); err != nil {
			return errToStatus(err), err
		}
		pruneWikiHistory(d, project, page)
	}

	tmp := p + ".tmp"
	if err := afero.WriteFile(d.user.Fs, tmp, out, 0o640); err != nil {
		return errToStatus(err), err
	}
	if err := d.user.Fs.Rename(tmp, p); err != nil {
		_ = d.user.Fs.Remove(tmp)
		return errToStatus(err), err
	}

	return renderJSON(w, r, json.RawMessage(out))
})

// wikiDeleteHandler removes a page and its history. A page that still has
// child pages is refused so a whole branch is never deleted by accident.
var wikiDeleteHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	project, page, err := wikiIDs(r, true)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if !d.user.Perm.Delete {
		return http.StatusForbidden, nil
	}
	p := wikiPagePath(project, page)
	if !d.CheckRules(p) {
		return http.StatusForbidden, nil
	}

	wikiMu.Lock()
	defer wikiMu.Unlock()

	envs, _, err := listWikiPages(d, project)
	if err != nil {
		return errToStatus(err), err
	}
	for _, e := range envs {
		if e.ParentID == page {
			return http.StatusConflict, errors.New("page has child pages; move or delete them first")
		}
	}

	if err := d.user.Fs.Remove(p); err != nil {
		return errToStatus(err), err
	}
	_ = d.user.Fs.RemoveAll(path.Dir(wikiHistoryPath(project, page, 1)))
	return 0, nil
})

// wikiHistoryHandler lists the saved revisions of a page, newest first.
var wikiHistoryHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	project, page, err := wikiIDs(r, true)
	if err != nil {
		return http.StatusBadRequest, err
	}

	type rev struct {
		Version   int    `json:"version"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updatedAt"`
	}
	out := make([]rev, 0)

	dir := path.Dir(wikiHistoryPath(project, page, 1))
	entries, err := afero.ReadDir(d.user.Fs, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return renderJSON(w, r, out)
		}
		return errToStatus(err), err
	}
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || n < 1 {
			continue
		}
		_, env, err := readWikiFile(d, path.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, rev{Version: n, Title: env.Title, UpdatedAt: env.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return renderJSON(w, r, out)
})

// wikiRevisionHandler returns one saved revision of a page, body included.
var wikiRevisionHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	project, page, err := wikiIDs(r, true)
	if err != nil {
		return http.StatusBadRequest, err
	}
	version, err := strconv.Atoi(mux.Vars(r)["version"])
	if err != nil || version < 1 {
		return http.StatusBadRequest, errors.New("invalid version")
	}
	raw, _, err := readWikiFile(d, wikiHistoryPath(project, page, version))
	if err != nil {
		return errToStatus(err), err
	}
	return renderJSON(w, r, json.RawMessage(raw))
})
