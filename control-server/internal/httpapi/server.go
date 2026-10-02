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

	"share-app-host/internal/config"
	"share-app-host/internal/rdpkeep"
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

// Keepalive reports and toggles the RDP session keep-alive connection.
type Keepalive interface {
	Status() rdpkeep.Status
	SetEnabled(enabled bool) (rdpkeep.Status, error)
}

// Settings reads and persists the host settings edited in the web client.
type Settings interface {
	Get() config.Settings
	Set(config.Settings) error
}

// Options configure a Server.
type Options struct {
	// Addr is the listen address.
	Addr string
	// ClientDir is the directory of web client files to serve.
	ClientDir string

	Windows   Windows
	Snapshots Snapshotter
	// Control serves the control WebSocket at /ws. It may be nil.
	Control http.Handler
	// Keepalive serves the session keep-alive endpoints. It may be nil.
	Keepalive Keepalive
	// Settings serves the settings endpoints. It may be nil.
	Settings Settings
}

// Server is the host's HTTP server.
type Server struct {
	httpServer *http.Server

	clientDir string
	windows   Windows
	snapshots Snapshotter
	keepalive Keepalive
	settings  Settings

	snapshotMu sync.Mutex // one snapshot helper at a time
}

// New returns a Server for opts.
func New(opts Options) *Server {
	s := &Server{
		clientDir: opts.ClientDir,
		windows:   opts.Windows,
		snapshots: opts.Snapshots,
		keepalive: opts.Keepalive,
		settings:  opts.Settings,
	}
	control := opts.Control
	if control == nil {
		control = http.NotFoundHandler()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/windows", s.handleListWindows)
	mux.HandleFunc("POST /api/target-window", s.handleSelectTarget)
	mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	if s.keepalive != nil {
		mux.HandleFunc("GET /api/session-keepalive", s.handleKeepaliveStatus)
		mux.HandleFunc("POST /api/session-keepalive", s.handleKeepaliveSet)
	}
	if s.settings != nil {
		mux.HandleFunc("GET /api/settings", s.handleSettingsGet)
		mux.HandleFunc("PUT /api/settings", s.handleSettingsPut)
	}
	mux.Handle("/ws", control)
	mux.HandleFunc("/", s.handleStatic)

	s.httpServer = &http.Server{
		Addr:              opts.Addr,
		Handler:           guard(mux),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}
	return s
}

// Handler returns the complete request handler, including the origin guard.
func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

// ListenAndServe serves requests until Shutdown is called.
func (s *Server) ListenAndServe() error { return s.httpServer.ListenAndServe() }

// Shutdown stops the server gracefully.
func (s *Server) Shutdown(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
