package httpapi_test

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSnapshotReturnsThePNGOfTheSelectedWindow(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad

	w := e.do("GET", "/api/snapshot", "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("GET /api/snapshot = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w.Body.String() != string(e.snapshots.data) {
		t.Fatal("response is not the captured image")
	}
	if calls := e.snapshots.calls(); len(calls) != 1 || calls[0] != notepad.Handle {
		t.Fatalf("captured %v, want the selected window", calls)
	}
}

func TestSnapshotNeedsATarget(t *testing.T) {
	e := newEnv(t)
	w := e.do("GET", "/api/snapshot", "")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "target window not selected") {
		t.Fatalf("GET /api/snapshot = %d %q", w.Code, w.Body.String())
	}
	if calls := e.snapshots.calls(); len(calls) != 0 {
		t.Fatalf("capture ran without a target: %v", calls)
	}
}

// Saving to a file and capturing another window used to be supported here. A
// caller still passing those parameters must get an error, not PNG bytes where
// it expects a JSON result.
func TestSnapshotRefusesTheRemovedParameters(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad
	for _, query := range []string{"out=window.png", "hwnd=777", "out=", "hwnd=", "out=a.png&hwnd=1"} {
		w := e.do("GET", "/api/snapshot?"+query, "")
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not supported") {
			t.Errorf("?%s: %d %q, want 400", query, w.Code, w.Body.String())
		}
	}
	if calls := e.snapshots.calls(); len(calls) != 0 {
		t.Fatalf("capture ran for a refused request: %v", calls)
	}
}

func TestSnapshotReportsACaptureFailureAsBadGateway(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad
	e.snapshots.err = errors.New("capture probe unavailable")
	w := e.do("GET", "/api/snapshot", "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "capture probe unavailable") {
		t.Fatalf("GET = %d %q", w.Code, w.Body.String())
	}
}

func TestOnlyOneSnapshotRunsAtATime(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad
	e.snapshots.started = make(chan struct{})
	e.snapshots.release = make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if w := e.do("GET", "/api/snapshot", ""); w.Code != http.StatusOK {
			t.Errorf("first snapshot = %d", w.Code)
		}
	}()
	select {
	case <-e.snapshots.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first snapshot never started")
	}

	if w := e.do("GET", "/api/snapshot", ""); w.Code != http.StatusConflict {
		t.Errorf("concurrent snapshot = %d, want 409", w.Code)
	}
	close(e.snapshots.release)
	wg.Wait()

	// The lock is released afterwards.
	e.snapshots.release = nil
	if w := e.do("GET", "/api/snapshot", ""); w.Code != http.StatusOK {
		t.Errorf("snapshot after the first finished = %d", w.Code)
	}
}
