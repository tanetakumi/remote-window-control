package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"share-app-host/internal/httpapi"
	"share-app-host/internal/window"
)

// fakeWindows is an in-memory httpapi.Windows.
type fakeWindows struct {
	mu        sync.Mutex
	windows   []window.Info
	listErr   error
	selectErr error
	current   *window.Info
}

func (f *fakeWindows) List(context.Context) ([]window.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.windows, f.listErr
}

func (f *fakeWindows) Current() (window.Info, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.current == nil {
		return window.Info{}, false
	}
	return *f.current, true
}

func (f *fakeWindows) Select(_ context.Context, handle uint64) (window.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.selectErr != nil {
		return window.Info{}, f.selectErr
	}
	for _, w := range f.windows {
		if w.Handle == handle {
			f.current = &w
			return w, nil
		}
	}
	return window.Info{}, window.ErrNotFound
}

// fakeSnapshotter is an in-memory httpapi.Snapshotter.
type fakeSnapshotter struct {
	mu      sync.Mutex
	data    []byte
	err     error
	handles []uint64
	started chan struct{} // closed on the first call, when non-nil
	release chan struct{} // when non-nil, calls block until it is closed
}

func (f *fakeSnapshotter) CapturePNG(ctx context.Context, handle uint64) ([]byte, error) {
	f.mu.Lock()
	f.handles = append(f.handles, handle)
	first := len(f.handles) == 1
	f.mu.Unlock()
	if first && f.started != nil {
		close(f.started)
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.data, f.err
}

func (f *fakeSnapshotter) calls() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint64(nil), f.handles...)
}

// env is a server wired to fakes, with a client directory and snapshot
// directory inside a temporary base directory.
type env struct {
	server    *httpapi.Server
	windows   *fakeWindows
	snapshots *fakeSnapshotter
	base      string
	clientDir string
}

type envOption func(*httpapi.Options)

func withControl(h http.Handler) envOption {
	return func(o *httpapi.Options) { o.Control = h }
}

func newEnv(t *testing.T, opts ...envOption) *env {
	t.Helper()
	base := t.TempDir()
	e := &env{
		windows:   &fakeWindows{},
		snapshots: &fakeSnapshotter{data: []byte("fake-png-bytes")},
		base:      base,
		clientDir: filepath.Join(base, "web"),
	}
	if err := os.Mkdir(e.clientDir, 0o700); err != nil {
		t.Fatal(err)
	}
	options := httpapi.Options{
		Addr:      "127.0.0.1:8443",
		ClientDir: e.clientDir,
		Windows:   e.windows,
		Snapshots: e.snapshots,
	}
	for _, apply := range opts {
		apply(&options)
	}
	e.server = httpapi.New(options)
	return e
}

func (e *env) writeClientFile(t *testing.T, name, content string) {
	t.Helper()
	writeFile(t, filepath.Join(e.clientDir, filepath.FromSlash(name)), content)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// do sends a request through the server's complete handler. origin is sent as
// the Origin header when not empty.
func (e *env) do(method, target, origin string) *httptest.ResponseRecorder {
	return e.doBody(method, target, origin, nil)
}

func (e *env) doBody(method, target, origin string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://host:8443"+target, bytes.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	e.server.Handler().ServeHTTP(w, r)
	return w
}
