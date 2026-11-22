package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

// fakeGoTrue emulates the subset of the Supabase Auth API we call.
func fakeGoTrue(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "anon" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/auth/v1/token" && r.URL.Query().Get("grant_type") == "password":
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["password"] != "correct-horse" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "tok",
				"user":         map[string]any{"id": "u1", "email": b["email"], "app_metadata": map[string]any{}},
			})
		case r.URL.Path == "/auth/v1/user":
			switch r.Header.Get("Authorization") {
			case "Bearer good":
				_, _ = w.Write([]byte(`{"id":"u2","email":"Oauth@Example.com","app_metadata":{}}`))
			case "Bearer admin":
				_, _ = w.Write([]byte(`{"id":"u3","email":"boss@example.com","app_metadata":{"drive_admin":true}}`))
			default:
				w.WriteHeader(http.StatusUnauthorized)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func supabaseReq(body string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	return r
}

func newSupabaseFixture(t *testing.T) (*SupabaseAuth, *mockUserStore, *settings.Settings, *settings.Server) {
	srvGo := fakeGoTrue(t)
	t.Cleanup(srvGo.Close)
	a := &SupabaseAuth{URL: srvGo.URL + "/", AnonKey: "anon", Admins: []string{"admin@example.com"}}
	store := &mockUserStore{users: map[string]*users.User{}}
	stg := &settings.Settings{
		MinimumPasswordLength: settings.DefaultMinimumPasswordLength,
		Defaults: settings.UserDefaults{
			Scope: ".",
			Perm:  users.Permissions{Execute: true, Create: true, Share: true, Download: true},
		},
	}
	return a, store, stg, &settings.Server{Root: t.TempDir()}
}

func TestSupabasePasswordLoginProvisionsUser(t *testing.T) {
	a, store, stg, srv := newSupabaseFixture(t)

	u, err := a.Auth(supabaseReq(`{"username":"Alice@Example.com","password":"correct-horse"}`), store, stg, srv)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if u.Username != "alice@example.com" {
		t.Fatalf("expected lowercased email username, got %q", u.Username)
	}
	if u.Perm.Admin || u.Perm.Execute || !u.LockPassword {
		t.Fatalf("provisioned user has unsafe defaults: %+v", u.Perm)
	}
	if !u.Perm.Share {
		t.Fatalf("expected defaults to apply")
	}
}

func TestSupabaseWrongPasswordIsPermissionError(t *testing.T) {
	a, store, stg, srv := newSupabaseFixture(t)
	_, err := a.Auth(supabaseReq(`{"username":"alice@example.com","password":"nope"}`), store, stg, srv)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected ErrPermission, got %v", err)
	}
}

func TestSupabaseAccessTokenAndAdmin(t *testing.T) {
	a, store, stg, srv := newSupabaseFixture(t)

	u, err := a.Auth(supabaseReq(`{"accessToken":"good"}`), store, stg, srv)
	if err != nil || u.Username != "oauth@example.com" || u.Perm.Admin {
		t.Fatalf("unexpected result %+v %v", u, err)
	}

	u, err = a.Auth(supabaseReq(`{"accessToken":"admin"}`), store, stg, srv)
	if err != nil || !u.Perm.Admin {
		t.Fatalf("expected app_metadata.drive_admin to grant admin, got %+v %v", u, err)
	}

	if _, err := a.Auth(supabaseReq(`{"accessToken":"forged"}`), store, stg, srv); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected forged token to be rejected, got %v", err)
	}
}

func TestSupabaseAdminList(t *testing.T) {
	a, store, stg, srv := newSupabaseFixture(t)
	u, err := a.Auth(supabaseReq(`{"username":"ADMIN@example.com","password":"correct-horse"}`), store, stg, srv)
	if err != nil || !u.Perm.Admin {
		t.Fatalf("expected admin from list, got %+v %v", u, err)
	}
}

func TestSupabaseAllowedDomains(t *testing.T) {
	a, store, stg, srv := newSupabaseFixture(t)
	a.AllowedDomains = []string{"corp.com"}
	if _, err := a.Auth(supabaseReq(`{"username":"alice@example.com","password":"correct-horse"}`), store, stg, srv); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected domain restriction, got %v", err)
	}
}

func TestSupabaseLocalFallback(t *testing.T) {
	a, store, stg, srv := newSupabaseFixture(t)
	hash, _ := users.ValidateAndHashPwd("localpassword1", 12)
	store.users["admin"] = &users.User{Username: "admin", Password: hash}

	if _, err := a.Auth(supabaseReq(`{"username":"admin","password":"localpassword1"}`), store, stg, srv); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("local login must be refused unless AllowLocal, got %v", err)
	}
	a.AllowLocal = true
	u, err := a.Auth(supabaseReq(`{"username":"admin","password":"localpassword1"}`), store, stg, srv)
	if err != nil || u.Username != "admin" {
		t.Fatalf("expected local fallback, got %+v %v", u, err)
	}
	if _, err := a.Auth(supabaseReq(`{"username":"admin","password":"wrong"}`), store, stg, srv); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected wrong local password to fail, got %v", err)
	}
}
