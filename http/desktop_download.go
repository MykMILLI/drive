package fbhttp

import (
 "archive/zip"
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "strings"
)

func desktopPublishDir() string {
 if dir := os.Getenv("DRIVE_DESKTOP_DIR"); dir != "" { return dir }
 return "/config/desktop"
}

func desktopPublishedURL() (string, error) {
 raw, err := os.ReadFile(filepath.Join(desktopPublishDir(), "instance-url.txt"))
 if err != nil { return "", err }
 if len(raw) > 4096 { return "", errors.New("instance URL is too long") }
 value := strings.TrimSpace(string(raw))
 parsed, err := url.Parse(value)
 if err != nil || parsed == nil || parsed.Hostname() == "" ||
  (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil ||
  parsed.RawQuery != "" || parsed.Fragment != "" {
  return "", errors.New("invalid configured instance URL")
 }
 return strings.TrimRight(value, "/"), nil
}

var desktopDownloadStatusHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
 w.Header().Set("Cache-Control", "no-store")
 publicURL, urlErr := desktopPublishedURL()
 info, fileErr := os.Stat(filepath.Join(desktopPublishDir(), "DriveDesktop.exe"))
 ready := urlErr == nil && fileErr == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= 64<<20
 return renderJSON(w, r, map[string]any{"available": ready, "server": publicURL, "version": "0.4-two-way-new-item-preview"})
})

var desktopDownloadHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
 w.Header().Set("Cache-Control", "no-store")
 publicURL, err := desktopPublishedURL()
 if err != nil { return http.StatusServiceUnavailable, nil }
 exe, err := os.Open(filepath.Join(desktopPublishDir(), "DriveDesktop.exe"))
 if err != nil { return http.StatusServiceUnavailable, nil }
 defer exe.Close()
 info, err := exe.Stat()
 if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 { return http.StatusServiceUnavailable, nil }
 // Build fully before sending headers, avoiding a successful partial ZIP on error.
 var buffer bytes.Buffer
 writer := zip.NewWriter(&buffer)
 executable, err := writer.Create("DriveDesktop/DriveDesktop.exe")
 if err != nil { return http.StatusInternalServerError, err }
 if _, err := io.CopyN(executable, exe, info.Size()); err != nil { return http.StatusInternalServerError, err }
 config, err := writer.Create("DriveDesktop/instance.json")
 if err != nil { return http.StatusInternalServerError, err }
 if err := json.NewEncoder(config).Encode(map[string]string{"server": publicURL}); err != nil { return http.StatusInternalServerError, err }
 readme, err := writer.Create("DriveDesktop/START-HERE.txt")
 if err != nil { return http.StatusInternalServerError, err }
 if _, err := io.WriteString(readme, "Extract the ENTIRE folder to a permanent location. Run DriveDesktop.exe. Click Connect this computer and approve it in your browser. No server URL or token entry is needed. This unsigned preview creates files and folders recursively in both directions after explicit browser approval. Existing-file edits and deletions are NOT synchronized. Use disposable test folders. Close hides to tray; exit using the tray menu. If your organization blocks unsigned software, use its approved process.\r\n"); err != nil { return http.StatusInternalServerError, err }
 if err := writer.Close(); err != nil { return http.StatusInternalServerError, err }
 w.Header().Set("Content-Type", "application/zip")
 w.Header().Set("Content-Disposition", `attachment; filename="DriveDesktop-Windows.zip"`)
 w.Header().Set("X-Content-Type-Options", "nosniff")
 _, err = w.Write(buffer.Bytes())
 return 0, err
})
