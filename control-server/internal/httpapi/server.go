// Package httpapi is the host's HTTP surface: the JSON API used by the web
// client and the host page, the web client's static files, and the mount point
// of the control WebSocket.
//
// The host has no application-level authentication; access control is done in
// front of it. Every request is therefore restricted to the same origin (see
// guard), and only explicitly served client files are exposed.
package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"share-app-host/internal/window"
)

const (
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
)

// Windows lists the shareable windows and manages the shared target.
type Windows interface {
	List(ctx context.Context) ([]window.Info, error)
	Current() (window.Info, bool)
	Select(ctx context.Context, handle uint64) (window.Info, error)
}

// Snapshotter captures one window as a PNG image.
type Snapshotter interface {
	CapturePNG(ctx context.Context, handle uint64) ([]byte, error)
}

// Options configure a Server.
type Options struct {
	// Addr is the listen address.
	Addr string
	// ClientDir is the directory of web client files to serve.
	ClientDir string
	// SnapshotDir is where snapshots requested with ?out= are saved.
	SnapshotDir string

	Windows   Windows
	Snapshots Snapshotter
	// Control serves the control WebSocket at /ws. It may be nil.
	Control http.Handler
}

// Server is the host's HTTP server.
type Server struct {
	httpServer *http.Server
	handler    http.Handler

	clientDir   string
	snapshotDir string
	windows     Windows
	snapshots   Snapshotter

	snapshotMu sync.Mutex // one snapshot helper at a time
}

// New returns a Server for opts.
func New(opts Options) *Server {
	s := &Server{
		clientDir:   opts.ClientDir,
		snapshotDir: opts.SnapshotDir,
		windows:     opts.Windows,
		snapshots:   opts.Snapshots,
	}
	control := opts.Control
	if control == nil {
		control = http.NotFoundHandler()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/windows", s.handleListWindows)
	mux.HandleFunc("GET /api/target-window", s.handleGetTarget)
	mux.HandleFunc("POST /api/target-window", s.handleSelectTarget)
	mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /host-ui", s.handleHostUI)
	mux.Handle("/ws", control)
	mux.HandleFunc("/", s.handleStatic)

	s.handler = guard(mux)
	s.httpServer = &http.Server{
		Addr:              opts.Addr,
		Handler:           s.handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}
	return s
}

// Handler returns the complete request handler, including the origin guard.
func (s *Server) Handler() http.Handler { return s.handler }

// ListenAndServe serves requests until Shutdown is called.
func (s *Server) ListenAndServe() error { return s.httpServer.ListenAndServe() }

// Shutdown stops the server gracefully.
func (s *Server) Shutdown(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
