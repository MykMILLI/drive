package fbhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

func TestStarredAndRecentHandlers(t *testing.T) {
	root := t.TempDir()
	userScope := filepath.Join(root, "user")
	if err := os.MkdirAll(filepath.Join(userScope, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", "docs/b.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(userScope, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	key := []byte("test-signing-key")
	perm := users.Permissions{}
	st := scopedUserStorage(t, userScope, perm, key)
	signed := signToken(t, perm, key)
	srv := &settings.Server{Root: root}

	do := func(h handleFunc, method, target, body string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("X-Auth", signed)
		rec := httptest.NewRecorder()
		handle(h, "", st, srv).ServeHTTP(rec, req)
		return rec
	}

	if rec := do(starredPutHandler, http.MethodPut, "/api/starred", `{"path":"docs/b.txt","starred":true}`); rec.Code != http.StatusOK {
		t.Fatalf("star: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(starredPutHandler, http.MethodPut, "/api/starred", `{"path":"/missing","starred":true}`); rec.Code == http.StatusOK {
		t.Fatalf("starring a missing path should fail")
	}

	rec := do(starredGetHandler, http.MethodGet, "/api/starred", "")
	var starred []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &starred)
	if len(starred) != 1 || starred[0]["path"] != "/docs/b.txt" {
		t.Fatalf("unexpected starred list: %s", rec.Body.String())
	}

	if rec := do(starredPutHandler, http.MethodPut, "/api/starred", `{"path":"/docs/b.txt","starred":false}`); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("unstar: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(recentHandler, http.MethodGet, "/api/recent?limit=10", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("recent: %d %s", rec.Code, rec.Body.String())
	}
	var recent struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &recent)
	if len(recent.Items) != 2 {
		t.Fatalf("expected 2 recent files (dotfiles skipped), got %s", rec.Body.String())
	}
}
