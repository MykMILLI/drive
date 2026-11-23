package fbhttp

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strings"
    "sync"
    "time"

    "github.com/filebrowser/filebrowser/v2/storage"
)

// Pending requests are deliberately ephemeral. A server restart requires a new
// connection attempt; issued device credentials remain in the existing DB.
type desktopPairRequest struct {
    ID string
    Name string
    Challenge string
    Expires time.Time
    LastPoll time.Time
    Token string
    Device storage.SyncDevice
    Store *storage.Storage
}

var desktopPairs = struct {
    sync.Mutex
    Requests map[string]*desktopPairRequest
    Window time.Time
    Starts int
}{Requests: make(map[string]*desktopPairRequest)}

func desktopPairJSON(w http.ResponseWriter, r *http.Request, body any) error {
    if r.Body == nil { return errors.New("missing JSON body") }
    r.Body = http.MaxBytesReader(w, r.Body, 8192)
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(body); err != nil { return err }
    var extra any
    if err := dec.Decode(&extra); err != io.EOF { return errors.New("invalid trailing JSON") }
    return nil
}

// Caller holds desktopPairs. Expired, unclaimed credentials are revoked before
// removing their in-memory secret. A cleanup error fails the request closed.
func desktopPairCleanup(now time.Time) error {
    for id, pending := range desktopPairs.Requests {
        if now.Before(pending.Expires) { continue }
        if pending.Token != "" {
            err := pending.Store.SyncDevices.Revoke(pending.Device.UserID, pending.Device.ID)
            if err != nil && !errors.Is(err, storage.ErrSyncDeviceMissing) { return err }
        }
        delete(desktopPairs.Requests, id)
    }
    return nil
}

var desktopPairStartHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
    w.Header().Set("Cache-Control", "no-store")
    if d.store.SyncDevices == nil { return http.StatusServiceUnavailable, nil }
    var body struct {
        Name string `json:"name"`
        Challenge string `json:"challenge"`
    }
    if err := desktopPairJSON(w, r, &body); err != nil { return http.StatusBadRequest, nil }
    body.Name = strings.TrimSpace(body.Name)
    challenge, err := base64.RawURLEncoding.DecodeString(body.Challenge)
    if err != nil || len(challenge) != sha256.Size || len(body.Challenge) != 43 ||
        len(body.Name) == 0 || len(body.Name) > 100 ||
        strings.IndexFunc(body.Name, func(c rune) bool { return c < 32 || c == 127 }) >= 0 {
        return http.StatusBadRequest, nil
    }
    desktopPairs.Lock()
    defer desktopPairs.Unlock()
    now := time.Now().UTC()
    if err := desktopPairCleanup(now); err != nil { return http.StatusInternalServerError, err }
    // Global limits are deliberate for this single-instance prototype. No
    // forwarded-client-IP header is trusted. Add edge limits for public use.
    if now.Sub(desktopPairs.Window) >= time.Minute {
        desktopPairs.Window = now
        desktopPairs.Starts = 0
    }
    if desktopPairs.Starts >= 20 || len(desktopPairs.Requests) >= 128 {
        w.Header().Set("Retry-After", "60")
        return http.StatusTooManyRequests, nil
    }
    var random [16]byte
    if _, err := rand.Read(random[:]); err != nil { return http.StatusInternalServerError, err }
    id := hex.EncodeToString(random[:])
    desktopPairs.Requests[id] = &desktopPairRequest{
        ID: id, Name: body.Name, Challenge: body.Challenge,
        Expires: now.Add(5 * time.Minute), Store: d.store,
    }
    desktopPairs.Starts++
    return renderJSON(w, r, map[string]any{
        "requestId": id,
        "verificationPath": "settings/connect-desktop?request=" + id,
        "expiresIn": 300,
        "interval": 3,
    })
}

var desktopPairInfoHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
    w.Header().Set("Cache-Control", "no-store")
    desktopPairs.Lock()
    defer desktopPairs.Unlock()
    if err := desktopPairCleanup(time.Now()); err != nil { return http.StatusInternalServerError, err }
    pending := desktopPairs.Requests[r.URL.Query().Get("id")]
    if pending == nil || pending.Store != d.store { return http.StatusNotFound, nil }
    return renderJSON(w, r, map[string]any{
        "name": pending.Name, "expiresAt": pending.Expires,
        "approved": pending.Token != "",
    })
})

var desktopPairApproveHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
    w.Header().Set("Cache-Control", "no-store")
    if !d.user.Perm.Download { return http.StatusForbidden, nil }
    if d.store.SyncDevices == nil { return http.StatusServiceUnavailable, nil }
    var body struct {
        ID string `json:"id"`
        Root string `json:"root"`
        AllowUploads bool `json:"allowUploads"`
    }
    if err := desktopPairJSON(w, r, &body); err != nil { return http.StatusBadRequest, nil }
    if body.AllowUploads && !d.user.Perm.Create { return http.StatusForbidden, nil }
    root, err := driveSyncRoot(body.Root)
    if err != nil { return http.StatusBadRequest, err }
    // Reuse the inventory access checks, including ancestor permissions and
    // symlink rejection. Limitations match the current inventory prototype.
    ctx, cancel := context.WithTimeout(r.Context(), driveSyncScanTimeout)
    defer cancel()
    if _, err := driveSyncScan(ctx, d.user.Fs, root, d.Check, driveSyncMaxEntries); err != nil {
        if errors.Is(err, errDriveSyncDenied) { return http.StatusForbidden, nil }
        if errors.Is(err, errDriveSyncPath) || errors.Is(err, errDriveSyncLink) { return http.StatusBadRequest, err }
        if errors.Is(err, errDriveSyncBudget) { return http.StatusUnprocessableEntity, err }
        if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) { return http.StatusServiceUnavailable, nil }
        return errToStatus(err), err
    }
    desktopPairs.Lock()
    defer desktopPairs.Unlock()
    if err := desktopPairCleanup(time.Now()); err != nil { return http.StatusInternalServerError, err }
    pending := desktopPairs.Requests[body.ID]
    if pending == nil || pending.Store != d.store { return http.StatusNotFound, nil }
    if pending.Token != "" { return http.StatusConflict, nil }
    var random [48]byte
    if _, err := rand.Read(random[:]); err != nil { return http.StatusInternalServerError, err }
    id := hex.EncodeToString(random[:16])
    token := "ds1." + id + "." + hex.EncodeToString(random[16:])
    now := time.Now().UTC()
    device := storage.SyncDevice{
        ID: id, UserID: d.user.ID, Name: pending.Name, Root: root,
        Digest: driveSyncDigest(token), Binding: driveSyncBinding(d),
        CreatedAt: now, ExpiresAt: now.Add(90 * 24 * time.Hour), UploadGranted: body.AllowUploads, TreeGranted: body.AllowUploads, ReceiveGranted: body.AllowUploads,
    }
    if err := d.store.SyncDevices.Create(device); err != nil {
        if errors.Is(err, storage.ErrSyncDeviceLimit) { return http.StatusConflict, err }
        return http.StatusInternalServerError, err
    }
    pending.Device = device
    pending.Token = token
    // Browser never receives the credential.
    return renderJSON(w, r, map[string]any{"approved": true, "root": root})
})

var desktopPairClaimHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
    w.Header().Set("Cache-Control", "no-store")
    var body struct {
        ID string `json:"id"`
        Verifier string `json:"verifier"`
    }
    if err := desktopPairJSON(w, r, &body); err != nil { return http.StatusBadRequest, nil }
    if len(body.ID) != 32 || len(body.Verifier) < 43 || len(body.Verifier) > 128 {
        return http.StatusBadRequest, nil
    }
    for _, c := range body.Verifier {
        if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
            (c >= '0' && c <= '9') || strings.ContainsRune("-._~", c)) {
            return http.StatusBadRequest, nil
        }
    }
    sum := sha256.Sum256([]byte(body.Verifier))
    challenge := base64.RawURLEncoding.EncodeToString(sum[:])
    desktopPairs.Lock()
    defer desktopPairs.Unlock()
    now := time.Now()
    if err := desktopPairCleanup(now); err != nil { return http.StatusInternalServerError, err }
    pending := desktopPairs.Requests[body.ID]
    if pending == nil || pending.Store != d.store || !driveSyncSame(challenge, pending.Challenge) {
        return http.StatusUnauthorized, nil
    }
    if !pending.LastPoll.IsZero() && now.Sub(pending.LastPoll) < 2 * time.Second {
        w.Header().Set("Retry-After", "3")
        return http.StatusTooManyRequests, nil
    }
    pending.LastPoll = now
    if pending.Token == "" {
        w.Header().Set("Retry-After", "3")
        return renderJSON(w, r, map[string]any{"status": "pending"})
    }
    // Re-check account scope/password/key binding and permission before issue.
    user, err := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, pending.Device.UserID)
    if err != nil { return http.StatusUnauthorized, nil }
    d.user = user
    if !d.user.Perm.Download || !driveSyncSame(pending.Device.Binding, driveSyncBinding(d)) {
        return http.StatusForbidden, nil
    }
    response := map[string]any{
        "status": "approved", "token": pending.Token,
        "device": driveSyncPublicDevice(pending.Device),
    }
    // One-use exchange. A lost response requires reconnecting; users can revoke
    // the orphan device from device management. Never log this response body.
    delete(desktopPairs.Requests, body.ID)
    return renderJSON(w, r, response)
}
