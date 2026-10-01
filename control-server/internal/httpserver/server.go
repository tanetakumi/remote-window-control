package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"share-app-host/internal/capture"
	"share-app-host/internal/origin"
	"share-app-host/internal/window"
)

type targetSelector interface {
	SelectTarget(context.Context, uint64) (window.Info, error)
}
type Server struct {
	httpServer  *http.Server
	clientDir   string
	snapshotDir string
	snapshotMu  sync.Mutex
	probe       *capture.Probe
	targets     *window.Selection
	selector    targetSelector
}

func New(addr, clientDir string, wsHandler http.Handler, probe *capture.Probe, targets *window.Selection, snapshotDir string) *Server {
	s := &Server{clientDir: clientDir, probe: probe, targets: targets, snapshotDir: snapshotDir}
	s.selector, _ = wsHandler.(targetSelector)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/windows", s.handleListWindows)
	mux.HandleFunc("/api/target-window", s.handleTargetWindow)
	mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /host-ui", s.handleHostUI)
	mux.Handle("/ws", wsHandler)
	mux.Handle("/", s.staticHandler())
	s.httpServer = &http.Server{Addr: addr, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !origin.Same(r) {
			http.Error(w, "origin forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})}
	return s
}
func (s *Server) ListenAndServe() error              { return s.httpServer.ListenAndServe() }
func (s *Server) Shutdown(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	selected, _ := s.targets.Current()
	writeJSON(w, map[string]any{"webrtc": map[string]any{"iceServers": []any{}}, "target_window": selected})
}
func (s *Server) handleListWindows(w http.ResponseWriter, r *http.Request) {
	windows, err := s.targets.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, windows)
}
func (s *Server) handleTargetWindow(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		selected, ok := s.targets.Current()
		var value any
		if ok {
			value = selected
		}
		writeJSON(w, map[string]any{"selected": value})
	case http.MethodPost:
		var payload struct {
			Handle uint64 `json:"handle"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&payload); err != nil {
			http.Error(w, "invalid target request", 400)
			return
		}
		var selected window.Info
		var err error
		if s.selector != nil {
			selected, err = s.selector.SelectTarget(r.Context(), payload.Handle)
		} else {
			selected, err = s.targets.Select(r.Context(), payload.Handle)
		}
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeJSON(w, selected)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

// snapshotResult is the JSON response for a snapshot saved to disk.
type snapshotResult struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func snapshotName(name string) bool {
	if name == "" || len(name) > 100 || !strings.HasSuffix(strings.ToLower(name), ".png") {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	if strings.Contains(name, "..") || strings.HasPrefix(name, ".") {
		return false
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		return false
	}
	return true
}
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("out")
	if name != "" && !snapshotName(name) {
		http.Error(w, "out must be a simple PNG filename", 400)
		return
	}
	if !s.snapshotMu.TryLock() {
		http.Error(w, "snapshot already in progress", 409)
		return
	}
	defer s.snapshotMu.Unlock()
	handle, _ := s.targets.CurrentHandle()
	if raw := r.URL.Query().Get("hwnd"); raw != "" {
		var err error
		handle, err = strconv.ParseUint(raw, 10, 64)
		if err != nil {
			http.Error(w, "invalid hwnd", 400)
			return
		}
	}
	if handle == 0 {
		http.Error(w, "target window not selected", 400)
		return
	}
	data, err := s.probe.CapturePNG(r.Context(), handle)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if name == "" {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
		return
	}
	if err = os.MkdirAll(s.snapshotDir, 0700); err == nil {
		var root *os.Root
		root, err = os.OpenRoot(s.snapshotDir)
		if err == nil {
			defer root.Close()
			var file *os.File
			file, err = root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				_, err = file.Write(data)
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
	}
	if err != nil {
		http.Error(w, "could not save snapshot (filename must be new)", 409)
		return
	}
	dimensions, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		http.Error(w, "capture returned invalid PNG", 502)
		return
	}
	writeJSON(w, snapshotResult{Path: filepath.Join("snapshots", name), Width: dimensions.Width, Height: dimensions.Height})
}
func (s *Server) handleHostUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, hostUIHTML)
}
func (s *Server) staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", 405)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".html", ".css", ".js", ".png", ".svg", ".ico", ".jpg", ".jpeg", ".webp", ".woff", ".woff2":
		default:
			http.NotFound(w, r)
			return
		}
		for _, part := range strings.Split(name, "/") {
			if part == "" || strings.HasPrefix(part, ".") || strings.ContainsAny(part, "\\:") {
				http.NotFound(w, r)
				return
			}
		}
		root, err := os.OpenRoot(s.clientDir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer root.Close()
		file, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		if name == "index.html" || name == "sw.js" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
}
func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
