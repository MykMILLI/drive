package fbhttp

import (
 "archive/zip"
 "bytes"
 "encoding/json"
 "io"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"

 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/users"
)

func TestDesktopPublishedDownload(t *testing.T) {
 dir := t.TempDir()
 t.Setenv("DRIVE_DESKTOP_DIR", dir)
 root := t.TempDir()
 key := []byte("test-signing-key")
 perm := users.Permissions{Download: true}
 st := scopedUserStorage(t, root, perm, key)
 srv := &settings.Server{Root: root}
 session := signToken(t, perm, key)
 do := func(h handleFunc, token string) *httptest.ResponseRecorder {
  req := httptest.NewRequest("GET", "/api/desktop/download", nil)
  if token != "" { req.Header.Set("X-Auth", token) }
  rec := httptest.NewRecorder()
  handle(h, "", st, srv).ServeHTTP(rec, req)
  return rec
 }
 if rec := do(desktopDownloadHandler, ""); rec.Code != 401 { t.Fatalf("anonymous: %d", rec.Code) }
 if rec := do(desktopDownloadHandler, session); rec.Code != 503 { t.Fatalf("unpublished: %d", rec.Code) }
 if err := os.WriteFile(filepath.Join(dir, "DriveDesktop.exe"), []byte("test-executable"), 0600); err != nil { t.Fatal(err) }
 if err := os.WriteFile(filepath.Join(dir, "instance-url.txt"), []byte("https://drive.example.test/base/\n"), 0600); err != nil { t.Fatal(err) }
 rec := do(desktopDownloadHandler, session)
 if rec.Code != 200 { t.Fatalf("download: %d %s", rec.Code, rec.Body.String()) }
 archive, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
 if err != nil { t.Fatal(err) }
 found := false
 for _, file := range archive.File {
  if file.Name != "DriveDesktop/instance.json" { continue }
  reader, err := file.Open()
  if err != nil { t.Fatal(err) }
  raw, err := io.ReadAll(reader)
  reader.Close()
  if err != nil { t.Fatal(err) }
  var config map[string]string
  if err := json.Unmarshal(raw, &config); err != nil { t.Fatal(err) }
  if len(config) != 1 || config["server"] != "https://drive.example.test/base" { t.Fatalf("unexpected bootstrap: %s", raw) }
  found = true
 }
 if !found { t.Fatal("bootstrap missing") }
 if err := os.WriteFile(filepath.Join(dir, "instance-url.txt"), []byte("https://user:secret@bad.example/"), 0600); err != nil { t.Fatal(err) }
 if _, err := desktopPublishedURL(); err == nil { t.Fatal("embedded credentials accepted") }
}
