package fbhttp

import (
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/filebrowser/filebrowser/v2/settings"
    "github.com/filebrowser/filebrowser/v2/users"
)

func TestDesktopPairLifecycle(t *testing.T) {
    root := t.TempDir()
    scope := filepath.Join(root, "user")
    if err := os.MkdirAll(filepath.Join(scope, "docs"), 0755); err != nil { t.Fatal(err) }
    key := []byte("test-signing-key")
    perm := users.Permissions{Download: true}
    st := scopedUserStorage(t, scope, perm, key)
    srv := &settings.Server{Root: root}
    session := signToken(t, perm, key)
    do := func(h handleFunc, method, target string, body any, account string) *httptest.ResponseRecorder {
        raw, err := json.Marshal(body)
        if err != nil { t.Fatal(err) }
        req := httptest.NewRequest(method, target, strings.NewReader(string(raw)))
        if account != "" { req.Header.Set("X-Auth", account) }
        rec := httptest.NewRecorder()
        handle(h, "", st, srv).ServeHTTP(rec, req)
        return rec
    }
    verifier := strings.Repeat("a", 64)
    sum := sha256.Sum256([]byte(verifier))
    challenge := base64.RawURLEncoding.EncodeToString(sum[:])
    rec := do(desktopPairStartHandler, "POST", "/api/sync/pair/start", map[string]string{"name": "Test computer", "challenge": challenge}, "")
    if rec.Code != 200 { t.Fatalf("start: %d %s", rec.Code, rec.Body.String()) }
    var started struct { ID string `json:"requestId"` }
    if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil { t.Fatal(err) }
    if len(started.ID) != 32 { t.Fatal("missing request ID") }
    t.Cleanup(func() { desktopPairs.Lock(); delete(desktopPairs.Requests, started.ID); desktopPairs.Unlock() })
    approve := map[string]string{"id": started.ID, "root": "/docs"}
    rec = do(desktopPairApproveHandler, "POST", "/api/sync/pair/approve", approve, "")
    if rec.Code != 401 { t.Fatalf("anonymous approval: %d", rec.Code) }
    rec = do(desktopPairInfoHandler, "GET", "/api/sync/pair/request?id="+started.ID, nil, session)
    if rec.Code != 200 || strings.Contains(rec.Body.String(), "ds1.") { t.Fatalf("request info: %d %s", rec.Code, rec.Body.String()) }
    rec = do(desktopPairApproveHandler, "POST", "/api/sync/pair/approve", approve, session)
    if rec.Code != 200 || strings.Contains(rec.Body.String(), "ds1.") { t.Fatalf("approval: %d %s", rec.Code, rec.Body.String()) }
    rec = do(desktopPairClaimHandler, "POST", "/api/sync/pair/claim", map[string]string{"id": started.ID, "verifier": strings.Repeat("b", 64)}, "")
    if rec.Code != 401 { t.Fatalf("wrong proof: %d", rec.Code) }
    rec = do(desktopPairClaimHandler, "POST", "/api/sync/pair/claim", map[string]string{"id": started.ID, "verifier": verifier}, "")
    if rec.Code != 200 { t.Fatalf("claim: %d %s", rec.Code, rec.Body.String()) }
    var claimed struct { Status string `json:"status"`; Token string `json:"token"` }
    if err := json.Unmarshal(rec.Body.Bytes(), &claimed); err != nil { t.Fatal(err) }
    if claimed.Status != "approved" || !strings.HasPrefix(claimed.Token, "ds1.") { t.Fatal("missing approved credential") }
    replay := do(desktopPairClaimHandler, "POST", "/api/sync/pair/claim", map[string]string{"id": started.ID, "verifier": verifier}, "")
    if replay.Code != 401 { t.Fatalf("claim replay: %d", replay.Code) }
    req := httptest.NewRequest("GET", "/api/sync/device/inventory", nil)
    req.Header.Set("Authorization", "Bearer "+claimed.Token)
    inventory := httptest.NewRecorder()
    handle(driveSyncDeviceInventoryHandler, "", st, srv).ServeHTTP(inventory, req)
    if inventory.Code != 200 { t.Fatalf("paired credential failed: %d %s", inventory.Code, inventory.Body.String()) }
}

func TestDesktopPairExpiredProof(t *testing.T) {
    desktopPairs.Lock()
    desktopPairs.Requests["expired-test"] = &desktopPairRequest{Expires: time.Now().Add(-time.Minute)}
    err := desktopPairCleanup(time.Now())
    _, remains := desktopPairs.Requests["expired-test"]
    desktopPairs.Unlock()
    if err != nil || remains { t.Fatalf("expired request not removed: %v", err) }
}
