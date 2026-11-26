package fbhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"
)

// Projects are stored as one JSON document per project inside the signed-in
// user's own scope:
//
//	/Projects/<id>/.project.json
//
// Attachments live next to it in the same folder, so they are ordinary Drive
// files (uploadable, previewable, shareable with personal share links). The
// document is treated as opaque JSON apart from id/name/version/updatedAt.

const (
	projectsDir       = "/Projects"
	projectFileName   = ".project.json"
	maxProjectBytes   = 2 << 20 // 2 MiB per project document
	maxProjectsListed = 500
)

var (
	projectSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	// projectsMu serialises writes so the version check and the rename are
	// atomic with respect to other requests in this process.
	projectsMu sync.Mutex
)

type projectEnvelope struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

func projectPath(id string) string {
	return path.Join(projectsDir, id, projectFileName)
}

func readProject(d *data, id string) ([]byte, *projectEnvelope, error) {
	p := projectPath(id)
	if !d.CheckRules(p) {
		return nil, nil, os.ErrPermission
	}
	raw, err := afero.ReadFile(d.user.Fs, p)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > maxProjectBytes {
		return nil, nil, errors.New("project document too large")
	}
	var env projectEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, nil, err
	}
	return raw, &env, nil
}

// projectsListHandler returns every project document in the user's scope.
var projectsListHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	out := make([]json.RawMessage, 0)

	entries, err := afero.ReadDir(d.user.Fs, projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return renderJSON(w, r, out)
		}
		return errToStatus(err), err
	}

	for _, e := range entries {
		if !e.IsDir() || !projectSlugRe.MatchString(e.Name()) {
			continue
		}
		if len(out) >= maxProjectsListed {
			break
		}
		raw, _, err := readProject(d, e.Name())
		if err != nil {
			// Skip folders without a readable project document.
			continue
		}
		out = append(out, json.RawMessage(raw))
	}

	return renderJSON(w, r, out)
})

// projectPutHandler creates or updates a project. The client sends the
// version it last saw; a mismatch returns 409 so two tabs cannot silently
// overwrite each other. A new project must be sent with version 0.
var projectPutHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	id := mux.Vars(r)["id"]
	if !projectSlugRe.MatchString(id) {
		return http.StatusBadRequest, errors.New("invalid project id")
	}
	if r.Body == nil {
		return http.StatusBadRequest, nil
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProjectBytes))
	if err != nil {
		return http.StatusRequestEntityTooLarge, err
	}

	var env projectEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return http.StatusBadRequest, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return http.StatusBadRequest, err
	}
	if env.ID != id || strings.TrimSpace(env.Name) == "" || len(env.Name) > 200 {
		return http.StatusBadRequest, errors.New("id/name mismatch or missing")
	}

	p := projectPath(id)
	if !d.CheckRules(p) {
		return http.StatusForbidden, nil
	}

	projectsMu.Lock()
	defer projectsMu.Unlock()

	current := -1
	if _, existing, err := readProject(d, id); err == nil {
		current = existing.Version
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

// projectDeleteHandler removes the project document only. The folder and any
// attachments in it are left alone; delete those from the Files view.
var projectDeleteHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	id := mux.Vars(r)["id"]
	if !projectSlugRe.MatchString(id) {
		return http.StatusBadRequest, errors.New("invalid project id")
	}
	if !d.user.Perm.Delete {
		return http.StatusForbidden, nil
	}
	p := projectPath(id)
	if !d.CheckRules(p) {
		return http.StatusForbidden, nil
	}

	projectsMu.Lock()
	defer projectsMu.Unlock()

	if err := d.user.Fs.Remove(p); err != nil {
		return errToStatus(err), err
	}
	return 0, nil
})
