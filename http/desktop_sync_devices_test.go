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

func TestDriveSyncDeviceLifecycle(t *testing.T) {
	root := t.TempDir()
	scope := filepath.Join(root, "user")
	if err := os.MkdirAll(filepath.Join(scope, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	key := []byte("test-signing-key")
	perm := users.Permissions{Download: true}
	st := scopedUserStorage(t, scope, perm, key)
	srv := &settings.Server{Root: root}
	session := signToken(t, perm, key)

	do := func(h handleFunc, method, target, body, account, device string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if account != "" {
			req.Header.Set("X-Auth", account)
		}
		if device != "" {
			req.Header.Set("Authorization", "Bearer "+device)
		}
		rec := httptest.NewRecorder()
		handle(h, "", st, srv).ServeHTTP(rec, req)
		return rec
	}

	body := `{"name":"Test desktop","root":"/docs"}`
	if rec := do(driveSyncDevicesHandler, "POST", "/api/sync/devices", body, "", ""); rec.Code != 401 {
		t.Fatalf("anonymous registration: %d", rec.Code)
	}
	rec := do(driveSyncDevicesHandler, "POST", "/api/sync/devices", body, session, "")
	if rec.Code != 200 {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Device driveSyncDeviceView `json:"device"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.Device.ID == "" {
		t.Fatal("missing device credentials")
	}

	record, err := st.SyncDevices.Get(created.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Digest == created.Token || record.Digest != driveSyncDigest(created.Token) {
		t.Fatal("incorrect credential storage")
	}

	rec = do(driveSyncDevicesHandler, "GET", "/api/sync/devices", "", session, "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), created.Token) ||
		strings.Contains(strings.ToLower(rec.Body.String()), "digest") ||
		strings.Contains(strings.ToLower(rec.Body.String()), "binding") {
		t.Fatalf("unsafe device listing: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(driveSyncDeviceInventoryHandler, "GET", "/api/sync/device/inventory", "", "", created.Token)
	if rec.Code != 200 {
		t.Fatalf("device inventory: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(driveSyncDeviceInventoryHandler, "GET", "/api/sync/device/inventory?path=/other", "", "", created.Token)
	if rec.Code != 403 {
		t.Fatalf("device escaped selected folder: %d", rec.Code)
	}
	rec = do(driveSyncDevicesHandler, "POST", "/api/sync/devices", body, "", created.Token)
	if rec.Code != 401 {
		t.Fatalf("device credential allowed registration: %d", rec.Code)
	}

	badToken := created.Token[:len(created.Token)-1]
	if strings.HasSuffix(created.Token, "0") {
		badToken += "1"
	} else {
		badToken += "0"
	}
	rec = do(driveSyncDeviceInventoryHandler, "GET", "/api/sync/device/inventory", "", "", badToken)
	if rec.Code != 401 {
		t.Fatalf("tampered token accepted: %d", rec.Code)
	}

	if err := st.SyncDevices.Revoke(record.UserID+1, record.ID); err == nil {
		t.Fatal("another owner revoked this device")
	}

	rec = do(driveSyncDevicesHandler, "DELETE", "/api/sync/devices?id="+created.Device.ID, "", session, "")
	if rec.Code != 204 {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(driveSyncDeviceInventoryHandler, "GET", "/api/sync/device/inventory", "", "", created.Token)
	if rec.Code != 401 {
		t.Fatalf("revoked token accepted: %d", rec.Code)
	}
}

func TestDriveSyncDeviceRequiresDownload(t *testing.T) {
	root := t.TempDir()
	scope := filepath.Join(root, "user")
	if err := os.MkdirAll(filepath.Join(scope, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	key := []byte("test-signing-key")
	perm := users.Permissions{}
	st := scopedUserStorage(t, scope, perm, key)
	req := httptest.NewRequest("POST", "/api/sync/devices",
		strings.NewReader(`{"name":"desktop","root":"/docs"}`))
	req.Header.Set("X-Auth", signToken(t, perm, key))
	rec := httptest.NewRecorder()
	handle(driveSyncDevicesHandler, "", st, &settings.Server{Root: root}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d %s", rec.Code, rec.Body.String())
	}
}
