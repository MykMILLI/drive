package fbhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"

	fbAuth "github.com/filebrowser/filebrowser/v2/auth"
	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

// ---------------------------------------------------------------------------
// Supabase magic links
// ---------------------------------------------------------------------------

type magicLinkBody struct {
	Email string `json:"email"`
}

// magicLinkHandler asks Supabase to send a passwordless sign-in email. It
// always answers 200 for well-formed requests so it can't be used to discover
// which addresses have accounts.
var magicLinkHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if d.settings.AuthMethod != fbAuth.MethodSupabaseAuth {
		return http.StatusNotFound, nil
	}
	raw, err := d.store.Auth.Get(d.settings.AuthMethod)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	sb, ok := raw.(*fbAuth.SupabaseAuth)
	if !ok || !sb.MagicLink {
		return http.StatusNotFound, nil
	}

	if r.Body == nil {
		return http.StatusBadRequest, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodySize)
	var body magicLinkBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}
	email := strings.TrimSpace(body.Email)
	if email == "" || !strings.Contains(email, "@") {
		return http.StatusBadRequest, nil
	}

	if err := sb.SendMagicLink(email, loginRedirectURL(r, d)); err != nil {
		if errors.Is(err, fberrors.ErrPermissionDenied) {
			return http.StatusTooManyRequests, nil
		}
		return http.StatusBadGateway, err
	}
	w.WriteHeader(http.StatusOK)
	return 0, nil
}

// loginRedirectURL builds the absolute URL of the login page, which is where
// Supabase sends users back with their session in the URL fragment.
func loginRedirectURL(r *http.Request, d *data) string {
	origin := r.Header.Get("Origin")
	if u, err := url.Parse(origin); origin == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		origin = scheme + "://" + r.Host
	}
	return strings.TrimRight(origin, "/") + path.Join("/", d.server.BaseURL, "login")
}

// ---------------------------------------------------------------------------
// Starred files
// ---------------------------------------------------------------------------

type starredEntry struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	IsDir    bool      `json:"isDir"`
}

func cleanStarPath(p string) string {
	p = path.Clean("/" + strings.TrimSpace(p))
	return p
}

var starredGetHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	out := make([]starredEntry, 0, len(d.user.Starred))
	for _, p := range d.user.Starred {
		if !d.Check(p) {
			continue
		}
		info, err := d.user.Fs.Stat(p)
		if err != nil {
			// Skip entries that were moved or deleted; they are pruned on the
			// next update.
			continue
		}
		out = append(out, starredEntry{
			Path:     p,
			Name:     info.Name(),
			Size:     info.Size(),
			Modified: info.ModTime(),
			IsDir:    info.IsDir(),
		})
	}
	return renderJSON(w, r, out)
})

type starredPutBody struct {
	Path    string `json:"path"`
	Starred bool   `json:"starred"`
}

const maxStarred = 1000

var starredPutHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if r.Body == nil {
		return http.StatusBadRequest, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var body starredPutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}
	p := cleanStarPath(body.Path)
	if p == "/" {
		return http.StatusBadRequest, nil
	}

	next := make([]string, 0, len(d.user.Starred)+1)
	for _, s := range d.user.Starred {
		if s == p {
			continue
		}
		// Prune stale entries while we are here.
		if _, err := d.user.Fs.Stat(s); err != nil {
			continue
		}
		next = append(next, s)
	}

	if body.Starred {
		if !d.Check(p) {
			return http.StatusForbidden, nil
		}
		if _, err := d.user.Fs.Stat(p); err != nil {
			return errToStatus(err), err
		}
		if len(next) >= maxStarred {
			return http.StatusBadRequest, errors.New("too many starred items")
		}
		next = append([]string{p}, next...)
	}

	d.user.Starred = next
	if err := d.store.Users.Update(d.user, "Starred"); err != nil {
		return http.StatusInternalServerError, err
	}
	return renderJSON(w, r, d.user.Starred)
})

// ---------------------------------------------------------------------------
// Recent files
// ---------------------------------------------------------------------------

const (
	recentDefaultLimit = 50
	recentMaxLimit     = 200
	recentMaxVisited   = 100_000
	recentMaxDuration  = 4 * time.Second
)

type recentResponse struct {
	Items []RecursiveEntry `json:"items"`
	// Partial is true when the scan stopped early because the tree is large.
	Partial bool `json:"partial"`
}

var errRecentBudget = errors.New("recent scan budget exhausted")

// recentHandler returns the most recently modified files in the user's
// scope. It walks the tree with a time and entry budget so very large trees
// still answer quickly.
var recentHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	limit := recentDefaultLimit
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > recentMaxLimit {
		limit = recentMaxLimit
	}

	deadline := time.Now().Add(recentMaxDuration)
	visited := 0
	partial := false
	entries := make([]RecursiveEntry, 0, 256)

	err := afero.Walk(d.user.Fs, "/", func(fPath string, info os.FileInfo, err error) error {
		if ctxErr := r.Context().Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		visited++
		if visited > recentMaxVisited || time.Now().After(deadline) {
			partial = true
			return errRecentBudget
		}
		if fPath == "/" {
			return nil
		}
		if !d.Check(fPath) || strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		entries = append(entries, RecursiveEntry{
			Path:    filepath.ToSlash(fPath),
			Name:    info.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   false,
		})
		// Keep memory bounded on huge trees.
		if len(entries) > limit*20 {
			sort.Slice(entries, func(i, j int) bool { return entries[i].ModTime.After(entries[j].ModTime) })
			entries = entries[:limit]
		}
		return nil
	})
	if err != nil && !errors.Is(err, errRecentBudget) {
		if r.Context().Err() != nil {
			return 0, err
		}
		return http.StatusInternalServerError, err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].ModTime.After(entries[j].ModTime) })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return renderJSON(w, r, recentResponse{Items: entries, Partial: partial})
})
