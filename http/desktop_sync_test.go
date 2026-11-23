package fbhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

func TestDriveSyncRoot(t *testing.T) {
	for _, input := range []string{"", "/", "../docs", "/docs/../private", "/docs/./a", "/docs\\a", "/docs\x00a"} {
		if _, err := driveSyncRoot(input); err == nil {
			t.Errorf("accepted unsafe root %q", input)
		}
	}
	got, err := driveSyncRoot("/docs/work/")
	if err != nil || got != "/docs/work" {
		t.Fatalf("root=%q err=%v", got, err)
	}
}

func TestDriveSyncInventoryScan(t *testing.T) {
	fs := afero.NewBasePathFs(afero.NewOsFs(), t.TempDir())
	if err := fs.MkdirAll("/docs/sub", 0755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/docs/sub/hello.txt", []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	allow := func(string) bool { return true }

	entries, err := driveSyncScan(context.Background(), fs, "/docs", allow, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected directory and file, got %#v", entries)
	}
	if entries[1].Path != "sub/hello.txt" || entries[1].Size != 5 {
		t.Fatalf("unexpected entry: %#v", entries[1])
	}

	if entries, err := driveSyncScan(context.Background(), fs, "/missing", allow, 100); err == nil || entries != nil {
		t.Fatal("missing folder must fail without returning a snapshot")
	}
	if entries, err := driveSyncScan(context.Background(), fs, "/docs", allow, 1); !errors.Is(err, errDriveSyncBudget) || entries != nil {
		t.Fatalf("budget should fail closed: entries=%v err=%v", entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := driveSyncScan(ctx, fs, "/docs", allow, 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled scan, got %v", err)
	}
}

func TestDriveSyncDeniedAncestor(t *testing.T) {
	fs := afero.NewBasePathFs(afero.NewOsFs(), t.TempDir())
	if err := fs.MkdirAll("/private/docs", 0755); err != nil {
		t.Fatal(err)
	}
	allowed := func(p string) bool { return p != "/private" }
	if _, err := driveSyncScan(context.Background(), fs, "/private/docs", allowed, 100); !errors.Is(err, errDriveSyncDenied) {
		t.Fatalf("denied ancestor was not rejected: %v", err)
	}
}

func TestDriveSyncRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "docs", "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	fs := afero.NewBasePathFs(afero.NewOsFs(), root)
	if _, err := driveSyncScan(context.Background(), fs, "/docs", func(string) bool { return true }, 100); !errors.Is(err, errDriveSyncLink) {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestDriveSyncAuthentication(t *testing.T) {
	root := t.TempDir()
	scope := filepath.Join(root, "user")
	if err := os.MkdirAll(filepath.Join(scope, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	key := []byte("test-signing-key")
	perm := users.Permissions{Download: true}
	st := scopedUserStorage(t, scope, perm, key)
	srv := &settings.Server{Root: root}

	for _, tc := range []struct {
		name string
		token string
		want int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"invalid", "not-a-token", http.StatusUnauthorized},
		{"authenticated", signToken(t, perm, key), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sync/inventory?path=/docs", nil)
			if tc.token != "" {
				req.Header.Set("X-Auth", tc.token)
			}
			rec := httptest.NewRecorder()
			handle(driveSyncInventoryHandler, "", st, srv).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
