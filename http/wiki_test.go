package fbhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

func TestWikiPaths(t *testing.T) {
	if got, want := wikiPagePath("p1", "home"), "/Projects/p1/wiki/home.json"; got != want {
		t.Fatalf("wikiPagePath = %q, want %q", got, want)
	}
	if got, want := wikiHistoryPath("p1", "home", 3), "/Projects/p1/wiki/.history/home/3.json"; got != want {
		t.Fatalf("wikiHistoryPath = %q, want %q", got, want)
	}
}

type wikiHarness struct {
	t      *testing.T
	router http.Handler
	token  string
}

func newWikiHarness(t *testing.T, perm users.Permissions) *wikiHarness {
	t.Helper()
	key := []byte("wiki-test-key")
	st := scopedUserStorage(t, t.TempDir(), perm, key)
	srv := &settings.Server{Root: t.TempDir()}

	r := mux.NewRouter()
	h := func(fn handleFunc) http.Handler { return handle(fn, "", st, srv) }
	r.Handle("/api/projects/{id}", h(projectPutHandler)).Methods("PUT")
	r.Handle("/api/projects/{id}/wiki", h(wikiListHandler)).Methods("GET")
	r.Handle("/api/projects/{id}/wiki/{page}", h(wikiGetHandler)).Methods("GET")
	r.Handle("/api/projects/{id}/wiki/{page}", h(wikiPutHandler)).Methods("PUT")
	r.Handle("/api/projects/{id}/wiki/{page}", h(wikiDeleteHandler)).Methods("DELETE")
	r.Handle("/api/projects/{id}/wiki/{page}/history", h(wikiHistoryHandler)).Methods("GET")
	r.Handle("/api/projects/{id}/wiki/{page}/history/{version}", h(wikiRevisionHandler)).Methods("GET")

	return &wikiHarness{t: t, router: r, token: signToken(t, perm, key)}
}

func (w *wikiHarness) do(method, url, body string) *httptest.ResponseRecorder {
	w.t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("X-Auth", w.token)
	rec := httptest.NewRecorder()
	w.router.ServeHTTP(rec, req)
	return rec
}

func (w *wikiHarness) mustProject(id string) {
	w.t.Helper()
	rec := w.do("PUT", "/api/projects/"+id, `{"id":"`+id+`","name":"P","version":0}`)
	if rec.Code != http.StatusOK {
		w.t.Fatalf("create project: %d %s", rec.Code, rec.Body.String())
	}
}

func wikiDoc(id, title, parent string, version int, body string) string {
	b, _ := json.Marshal(map[string]any{
		"id": id, "title": title, "parentId": parent, "version": version, "body": body,
	})
	return string(b)
}

func TestWikiLifecycle(t *testing.T) {
	w := newWikiHarness(t, users.Permissions{Create: true, Modify: true, Delete: true})
	w.mustProject("proj")

	// A page needs an existing project.
	if rec := w.do("PUT", "/api/projects/missing/wiki/home", wikiDoc("home", "Home", "", 0, "x")); rec.Code != http.StatusNotFound {
		t.Fatalf("page in missing project: got %d, want 404", rec.Code)
	}

	// Create, then fetch the body back.
	if rec := w.do("PUT", "/api/projects/proj/wiki/home", wikiDoc("home", "Home", "", 0, "# Hello")); rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec := w.do("GET", "/api/projects/proj/wiki/home", "")
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["body"] != "# Hello" || got["version"].(float64) != 1 {
		t.Fatalf("unexpected page: %v", got)
	}

	// Creating a page that already exists, or a stale update, is a conflict.
	if rec := w.do("PUT", "/api/projects/proj/wiki/home", wikiDoc("home", "Home", "", 0, "dup")); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create: got %d, want 409", rec.Code)
	}

	// Update bumps the version and keeps the replaced revision.
	if rec := w.do("PUT", "/api/projects/proj/wiki/home", wikiDoc("home", "Home", "", 1, "# Hello v2")); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if rec := w.do("PUT", "/api/projects/proj/wiki/home", wikiDoc("home", "Home", "", 1, "stale")); rec.Code != http.StatusConflict {
		t.Fatalf("stale update: got %d, want 409", rec.Code)
	}
	rec = w.do("GET", "/api/projects/proj/wiki/home/history", "")
	var revs []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &revs)
	if len(revs) != 1 || revs[0]["version"].(float64) != 1 {
		t.Fatalf("history = %v, want one revision at version 1", revs)
	}
	rec = w.do("GET", "/api/projects/proj/wiki/home/history/1", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || got["body"] != "# Hello" {
		t.Fatalf("revision 1: %d %v", rec.Code, got)
	}

	// The list omits bodies.
	rec = w.do("GET", "/api/projects/proj/wiki", "")
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list = %v, want one page", list)
	}
	if _, has := list[0]["body"]; has {
		t.Fatal("list should not include page bodies")
	}
}

func TestWikiTreeRules(t *testing.T) {
	w := newWikiHarness(t, users.Permissions{Create: true, Modify: true, Delete: true})
	w.mustProject("proj")

	for _, p := range []struct{ id, title, parent string }{
		{"a", "A", ""},
		{"b", "B", "a"},
		{"c", "C", "b"},
	} {
		rec := w.do("PUT", "/api/projects/proj/wiki/"+p.id, wikiDoc(p.id, p.title, p.parent, 0, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("create %s: %d %s", p.id, rec.Code, rec.Body.String())
		}
	}

	// Missing parent, self parent and cycles are rejected.
	if rec := w.do("PUT", "/api/projects/proj/wiki/d", wikiDoc("d", "D", "ghost", 0, "")); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing parent: got %d, want 400", rec.Code)
	}
	if rec := w.do("PUT", "/api/projects/proj/wiki/a", wikiDoc("a", "A", "a", 1, "")); rec.Code != http.StatusBadRequest {
		t.Fatalf("self parent: got %d, want 400", rec.Code)
	}
	if rec := w.do("PUT", "/api/projects/proj/wiki/a", wikiDoc("a", "A", "c", 1, "")); rec.Code != http.StatusBadRequest {
		t.Fatalf("cycle a->c->b->a: got %d, want 400", rec.Code)
	}

	// A page with children cannot be deleted; a leaf can.
	if rec := w.do("DELETE", "/api/projects/proj/wiki/a", ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete with children: got %d, want 409", rec.Code)
	}
	if rec := w.do("DELETE", "/api/projects/proj/wiki/c", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete leaf: got %d, want 200", rec.Code)
	}
	if rec := w.do("GET", "/api/projects/proj/wiki/c", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted page: got %d, want 404", rec.Code)
	}
}

func TestWikiValidationAndPermissions(t *testing.T) {
	w := newWikiHarness(t, users.Permissions{Create: true, Modify: false, Delete: false})
	w.mustProject("proj")

	if rec := w.do("PUT", "/api/projects/proj/wiki/Bad_ID", wikiDoc("Bad_ID", "T", "", 0, "")); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad page id: got %d, want 400", rec.Code)
	}
	if rec := w.do("PUT", "/api/projects/proj/wiki/a", wikiDoc("other", "T", "", 0, "")); rec.Code != http.StatusBadRequest {
		t.Fatalf("id mismatch: got %d, want 400", rec.Code)
	}
	if rec := w.do("PUT", "/api/projects/proj/wiki/a", wikiDoc("a", "  ", "", 0, "")); rec.Code != http.StatusBadRequest {
		t.Fatalf("blank title: got %d, want 400", rec.Code)
	}

	if rec := w.do("PUT", "/api/projects/proj/wiki/a", wikiDoc("a", "A", "", 0, "")); rec.Code != http.StatusOK {
		t.Fatalf("create: %d", rec.Code)
	}
	if rec := w.do("PUT", "/api/projects/proj/wiki/a", wikiDoc("a", "A", "", 1, "edit")); rec.Code != http.StatusForbidden {
		t.Fatalf("update without modify: got %d, want 403", rec.Code)
	}
	if rec := w.do("DELETE", "/api/projects/proj/wiki/a", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("delete without permission: got %d, want 403", rec.Code)
	}
}

func TestWikiHistoryIsPruned(t *testing.T) {
	w := newWikiHarness(t, users.Permissions{Create: true, Modify: true})
	w.mustProject("proj")

	version := 0
	for i := 0; i < maxWikiHistory+5; i++ {
		rec := w.do("PUT", "/api/projects/proj/wiki/a", wikiDoc("a", "A", "", version, strings.Repeat("x", i)))
		if rec.Code != http.StatusOK {
			t.Fatalf("save %d: %d %s", i, rec.Code, rec.Body.String())
		}
		version++
	}
	rec := w.do("GET", "/api/projects/proj/wiki/a/history", "")
	var revs []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &revs)
	if len(revs) != maxWikiHistory {
		t.Fatalf("history length = %d, want %d", len(revs), maxWikiHistory)
	}
	// Newest first, and the oldest revisions are the ones dropped.
	if revs[0]["version"].(float64) != float64(maxWikiHistory+4) {
		t.Fatalf("newest revision = %v", revs[0]["version"])
	}
}
