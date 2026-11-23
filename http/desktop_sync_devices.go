package fbhttp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/filebrowser/filebrowser/v2/storage"
)

type driveSyncDeviceView struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Root string `json:"root"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func driveSyncPublicDevice(device storage.SyncDevice) driveSyncDeviceView {
	return driveSyncDeviceView{
		ID: device.ID,
		Name: device.Name,
		Root: device.Root,
		CreatedAt: device.CreatedAt,
		ExpiresAt: device.ExpiresAt,
	}
}

// Bind credentials to the current account scope, password hash, and server key.
// Scope/password/key changes invalidate previously issued device credentials.
func driveSyncBinding(d *data) string {
	mac := hmac.New(sha256.New, d.settings.Key)
	_, _ = mac.Write([]byte(d.user.Scope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(d.user.Password))
	return hex.EncodeToString(mac.Sum(nil))
}

func driveSyncDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func driveSyncSame(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var driveSyncDevicesHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	if d.store.SyncDevices == nil {
		return http.StatusServiceUnavailable, nil
	}

	switch r.Method {
	case http.MethodGet:
		devices, err := d.store.SyncDevices.List(d.user.ID)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		out := make([]driveSyncDeviceView, 0, len(devices))
		for _, device := range devices {
			out = append(out, driveSyncPublicDevice(device))
		}
		return renderJSON(w, r, out)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if len(id) != 32 {
			return http.StatusBadRequest, nil
		}
		err := d.store.SyncDevices.Revoke(d.user.ID, id)
		if errors.Is(err, storage.ErrSyncDeviceMissing) {
			return http.StatusNotFound, nil
		}
		if err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusNoContent, nil

	case http.MethodPost:
		if !d.user.Perm.Download {
			return http.StatusForbidden, nil
		}
		if r.Body == nil {
			return http.StatusBadRequest, nil
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		var body struct {
			Name string `json:"name"`
			Root string `json:"root"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			return http.StatusBadRequest, nil
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return http.StatusBadRequest, nil
		}
		body.Name = strings.TrimSpace(body.Name)
		if len(body.Name) == 0 || len(body.Name) > 100 ||
			strings.IndexFunc(body.Name, func(c rune) bool { return c < 32 || c == 127 }) >= 0 {
			return http.StatusBadRequest, nil
		}
		root, err := driveSyncRoot(body.Root)
		if err != nil {
			return http.StatusBadRequest, err
		}

		// Validate the selected root and each ancestor without scanning its tree.
		current := ""
		for _, part := range strings.Split(strings.TrimPrefix(root, "/"), "/") {
			current += "/" + part
			if !d.Check(current) {
				return http.StatusForbidden, nil
			}
			info, err := driveSyncLstat(d.user.Fs, current)
			if err != nil {
				if errors.Is(err, errDriveSyncLink) {
					return http.StatusBadRequest, err
				}
				return errToStatus(err), err
			}
			if !info.IsDir() {
				return http.StatusBadRequest, errDriveSyncPath
			}
		}

		var random [48]byte
		if _, err := rand.Read(random[:]); err != nil {
			return http.StatusInternalServerError, err
		}
		id := hex.EncodeToString(random[:16])
		token := "ds1." + id + "." + hex.EncodeToString(random[16:])
		now := time.Now().UTC()
		device := storage.SyncDevice{
			ID: id,
			UserID: d.user.ID,
			Name: body.Name,
			Root: root,
			Digest: driveSyncDigest(token),
			Binding: driveSyncBinding(d),
			CreatedAt: now,
			ExpiresAt: now.Add(90 * 24 * time.Hour),
		}
		if err := d.store.SyncDevices.Create(device); err != nil {
			if errors.Is(err, storage.ErrSyncDeviceLimit) {
				return http.StatusConflict, err
			}
			return http.StatusInternalServerError, err
		}
		// The plaintext credential is returned only at creation.
		return renderJSON(w, r, map[string]any{
			"device": driveSyncPublicDevice(device),
			"token": token,
		})
	}
	return http.StatusMethodNotAllowed, nil
})

// Device credentials are accepted only on this explicit sync route.
// They are not general account sessions.
var driveSyncDeviceInventoryHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("WWW-Authenticate", "Bearer")
	if d.store.SyncDevices == nil {
		return http.StatusServiceUnavailable, nil
	}
	auth := r.Header.Get("Authorization")
	if len(auth) > 256 || !strings.HasPrefix(auth, "Bearer ") {
		return http.StatusUnauthorized, nil
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "ds1" || len(parts[1]) != 32 || len(parts[2]) != 64 {
		return http.StatusUnauthorized, nil
	}
	if _, err := hex.DecodeString(parts[1] + parts[2]); err != nil {
		return http.StatusUnauthorized, nil
	}
	device, err := d.store.SyncDevices.Get(parts[1])
	if err != nil {
		if errors.Is(err, storage.ErrSyncDeviceMissing) {
			return http.StatusUnauthorized, nil
		}
		return http.StatusInternalServerError, err
	}
	if !time.Now().Before(device.ExpiresAt) || !driveSyncSame(device.Digest, driveSyncDigest(token)) {
		return http.StatusUnauthorized, nil
	}
	d.user, err = d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, device.UserID)
	if err != nil {
		return http.StatusUnauthorized, nil
	}
	if !driveSyncSame(device.Binding, driveSyncBinding(d)) {
		return http.StatusUnauthorized, nil
	}
	if selected := r.URL.Query().Get("path"); selected != "" && selected != device.Root {
		return http.StatusForbidden, nil
	}
	query := r.URL.Query()
	query.Set("path", device.Root)
	r.URL.RawQuery = query.Encode()
	return driveSyncInventoryCore(w, r, d)
}
