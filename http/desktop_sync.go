package fbhttp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	driveSyncMaxEntries = 10000
	driveSyncScanTimeout = 3 * time.Second
)

var (
	errDriveSyncPath = errors.New("select a non-root absolute folder without traversal")
	errDriveSyncDenied = errors.New("folder is not permitted")
	errDriveSyncLink = errors.New("sync inventory requires verifiable non-symlink paths")
	errDriveSyncBudget = errors.New("folder exceeds inventory scan budget; select a smaller folder")
)

type driveSyncEntry struct {
	Path string `json:"path"`
	IsDir bool `json:"isDir"`
	Size int64 `json:"size"`
	Modified time.Time `json:"modified"`
}

type driveSyncInventory struct {
	Protocol int `json:"protocol"`
	Root string `json:"root"`
	Entries []driveSyncEntry `json:"entries"`
	Complete bool `json:"complete"`
	DeletionSafe bool `json:"deletionSafe"`
}

// Inventory is a bounded observation, not an atomic snapshot or deletion log.
var driveSyncInventoryHandler = withUser(driveSyncInventoryCore)

func driveSyncInventoryCore(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "no-store")

	if !d.user.Perm.Download {
		return http.StatusForbidden, nil
	}

	root, err := driveSyncRoot(r.URL.Query().Get("path"))
	if err != nil {
		return http.StatusBadRequest, err
	}

	ctx, cancel := context.WithTimeout(r.Context(), driveSyncScanTimeout)
	defer cancel()

	entries, err := driveSyncScan(ctx, d.user.Fs, root, d.Check, driveSyncMaxEntries)
	if err != nil {
		switch {
		case errors.Is(err, errDriveSyncDenied),
			errors.Is(err, os.ErrPermission):
			return http.StatusForbidden, nil
		case errors.Is(err, os.ErrNotExist):
			return http.StatusNotFound, nil
		case errors.Is(err, errDriveSyncLink),
			errors.Is(err, errDriveSyncPath):
			return http.StatusBadRequest, err
		case errors.Is(err, errDriveSyncBudget):
			return http.StatusUnprocessableEntity, err
		case errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, context.Canceled):
			return http.StatusServiceUnavailable, nil
		default:
			return http.StatusInternalServerError, err
		}
	}

	return renderJSON(w, r, driveSyncInventory{
		Protocol: 1,
		Root: root,
		Entries: entries,
		Complete: true,
		DeletionSafe: false,
	})
}

func driveSyncRoot(raw string) (string, error) {
	if raw == "" || len(raw) > 4096 || !strings.HasPrefix(raw, "/") ||
		strings.ContainsAny(raw, "\\\x00") {
		return "", errDriveSyncPath
	}
	for _, component := range strings.Split(raw, "/") {
		if component == "." || component == ".." {
			return "", errDriveSyncPath
		}
	}
	root := path.Clean(raw)
	if root == "/" {
		return "", errDriveSyncPath
	}
	return root, nil
}

func driveSyncLstat(fs afero.Fs, name string) (os.FileInfo, error) {
	lstater, ok := fs.(afero.Lstater)
	if !ok {
		return nil, errDriveSyncLink
	}
	info, supported, err := lstater.LstatIfPossible(name)
	if err != nil {
		return nil, err
	}
	if !supported || info.Mode()&os.ModeSymlink != 0 {
		return nil, errDriveSyncLink
	}
	return info, nil
}

func driveSyncScan(
	ctx context.Context,
	fs afero.Fs,
	root string,
	allowed func(string) bool,
	limit int,
) ([]driveSyncEntry, error) {
	var err error
	root, err = driveSyncRoot(root)
	if err != nil {
		return nil, err
	}

	// Check every selected-root component, not only its final directory.
	current := "/"
	for _, part := range strings.Split(strings.TrimPrefix(root, "/"), "/") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current = path.Join(current, part)
		if !allowed(current) {
			return nil, errDriveSyncDenied
		}
		info, err := driveSyncLstat(fs, current)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errDriveSyncPath
		}
	}

	entries := make([]driveSyncEntry, 0)
	visited := 0

	err = afero.Walk(fs, root, func(name string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Never convert unreadable or missing entries into an empty snapshot.
		if walkErr != nil {
			return walkErr
		}
		visited++
		if visited > limit {
			return errDriveSyncBudget
		}

		virtual := filepath.ToSlash(name)
		if !allowed(virtual) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		checked, err := driveSyncLstat(fs, name)
		if err != nil {
			return err
		}
		if !checked.IsDir() && !checked.Mode().IsRegular() {
			return errDriveSyncLink
		}
		if virtual == root {
			return nil
		}

		relative, err := filepath.Rel(filepath.FromSlash(root), name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".." || strings.HasPrefix(relative, "../") {
			return errDriveSyncPath
		}

		entries = append(entries, driveSyncEntry{
			Path: relative,
			IsDir: checked.IsDir(),
			Size: checked.Size(),
			Modified: checked.ModTime().UTC(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
