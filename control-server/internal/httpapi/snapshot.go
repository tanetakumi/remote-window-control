package httpapi

import (
	"bytes"
	"errors"
	"image/png"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxSnapshotNameLength = 100

// snapshotResult is the JSON response for a snapshot saved to disk.
type snapshotResult struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// handleSnapshot returns the target window as a PNG. With ?out=name.png it
// instead saves the image under the snapshot directory, never overwriting an
// existing file, and returns where. ?hwnd= captures a specific window.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("out")
	if name != "" && !validSnapshotName(name) {
		http.Error(w, "out must be a simple PNG filename", http.StatusBadRequest)
		return
	}
	if !s.snapshotMu.TryLock() {
		http.Error(w, "snapshot already in progress", http.StatusConflict)
		return
	}
	defer s.snapshotMu.Unlock()

	handle, err := s.snapshotHandle(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := s.snapshots.CapturePNG(r.Context(), handle)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if name == "" {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
		return
	}

	// Validate before saving, so a corrupt capture is never written to disk.
	size, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		http.Error(w, "capture returned invalid PNG", http.StatusBadGateway)
		return
	}
	if err := saveSnapshot(s.snapshotDir, name, data); err != nil {
		if errors.Is(err, fs.ErrExist) {
			http.Error(w, "could not save snapshot (filename must be new)", http.StatusConflict)
		} else {
			http.Error(w, "could not save snapshot", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, snapshotResult{
		Path:   filepath.Join("snapshots", name),
		Width:  size.Width,
		Height: size.Height,
	})
}

// snapshotHandle returns the window to capture: ?hwnd= if given, otherwise the
// selected target.
func (s *Server) snapshotHandle(r *http.Request) (uint64, error) {
	var handle uint64
	if current, ok := s.windows.Current(); ok {
		handle = current.Handle
	}
	if raw := r.URL.Query().Get("hwnd"); raw != "" {
		var err error
		if handle, err = strconv.ParseUint(raw, 10, 64); err != nil {
			return 0, errors.New("invalid hwnd")
		}
	}
	if handle == 0 {
		return 0, errors.New("target window not selected")
	}
	return handle, nil
}

// saveSnapshot writes data to a new file name inside dir. The directory is
// opened as a root, so name cannot escape it, and an existing file is never
// overwritten (the error satisfies errors.Is(err, fs.ErrExist)).
func saveSnapshot(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove(name) // do not leave a truncated image behind
	}
	return err
}

// validSnapshotName reports whether name is a plain *.png file name: ASCII
// letters, digits, '-', '_' and '.', no leading dot or "..", not too long and
// not a Windows device name such as CON or COM1.
func validSnapshotName(name string) bool {
	if name == "" || len(name) > maxSnapshotNameLength || !strings.HasSuffix(strings.ToLower(name), ".png") {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	if strings.Contains(name, "..") || strings.HasPrefix(name, ".") {
		return false
	}
	stem, _, _ := strings.Cut(name, ".")
	return !isWindowsDeviceName(strings.ToUpper(stem))
}

// isWindowsDeviceName reports whether an upper-cased file stem is reserved by
// Windows (CON, PRN, AUX, NUL, COM1-9, LPT1-9).
func isWindowsDeviceName(stem string) bool {
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(stem) == 4 &&
		(strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) &&
		stem[3] >= '1' && stem[3] <= '9'
}
