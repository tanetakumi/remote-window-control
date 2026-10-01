package httpapi_test

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSnapshotRejectsUnrestrictedOutputBeforeCapture(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad

	for _, name := range []string{
		"../secret.png", "d:/secret.png", "folder/file.png", `folder\file.png`, "a.png:stream",
		"CON.png", "con.png", "NUL.png", "PRN.png", "AUX.png", "COM1.png", "LPT9.png", "COM3.tar.png",
		".hidden.png", "a..png", "a.exe", "a.png.exe", "noextension", ".png",
		"spaces not allowed.png", "ünïcode.png", strings.Repeat("a", 97) + ".png",
	} {
		w := e.do("GET", "/api/snapshot?out="+url.QueryEscape(name), "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("out=%q: %d, want 400", name, w.Code)
		}
	}
	if calls := e.snapshots.calls(); len(calls) != 0 {
		t.Fatalf("capture ran for a rejected filename: %v", calls)
	}
	entries, _ := os.ReadDir(e.snapshotDir)
	if len(entries) != 0 {
		t.Fatalf("files written: %v", entries)
	}
}

func TestSnapshotAcceptsSimplePNGNames(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad
	for _, name := range []string{"window-2026.png", "a.png", "UPPER.PNG", "my_shot.v2.png", "COMX.png", strings.Repeat("a", 96) + ".png"} {
		if w := e.do("GET", "/api/snapshot?out="+name, ""); w.Code != http.StatusOK {
			t.Errorf("out=%q: %d %s, want 200", name, w.Code, w.Body.String())
		}
	}
}

func TestSnapshotReturnsPNGBytes(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad

	w := e.do("GET", "/api/snapshot", "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("GET /api/snapshot = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if string(w.Body.Bytes()) != string(e.snapshots.data) {
		t.Fatal("response is not the captured image")
	}
	if calls := e.snapshots.calls(); len(calls) != 1 || calls[0] != notepad.Handle {
		t.Fatalf("captured %v, want the selected window", calls)
	}
	if entries, _ := os.ReadDir(e.snapshotDir); len(entries) != 0 {
		t.Fatalf("a snapshot without out= wrote files: %v", entries)
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

func TestSnapshotHwndOverridesTheSelectedWindow(t *testing.T) {
	e := newEnv(t)

	// Works with nothing selected.
	if w := e.do("GET", "/api/snapshot?hwnd=777", ""); w.Code != http.StatusOK {
		t.Fatalf("hwnd without a target = %d %s", w.Code, w.Body.String())
	}
	e.windows.current = &notepad
	if w := e.do("GET", "/api/snapshot?hwnd=888", ""); w.Code != http.StatusOK {
		t.Fatalf("hwnd with a target = %d", w.Code)
	}
	if calls := e.snapshots.calls(); len(calls) != 2 || calls[0] != 777 || calls[1] != 888 {
		t.Fatalf("captured %v", calls)
	}
}

func TestSnapshotRejectsAnInvalidHwnd(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad
	for _, hwnd := range []string{"abc", "-1", "1.5", "0x10", "99999999999999999999999"} {
		w := e.do("GET", "/api/snapshot?hwnd="+hwnd, "")
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid hwnd") {
			t.Errorf("hwnd=%q: %d %q", hwnd, w.Code, w.Body.String())
		}
	}
	if w := e.do("GET", "/api/snapshot?hwnd=0", ""); w.Code != http.StatusBadRequest {
		t.Errorf("hwnd=0: %d, want 400", w.Code)
	}
}

func TestSnapshotSavesANewFile(t *testing.T) {
	e := newEnv(t) // the snapshot directory does not exist yet
	e.windows.current = &notepad

	w := e.do("GET", "/api/snapshot?out=shot.png", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", w.Code, w.Body.String())
	}
	result := decode[struct {
		Path          string
		Width, Height int
	}](t, w)
	if want := filepath.Join("snapshots", "shot.png"); result.Path != want || result.Width != 3 || result.Height != 2 {
		t.Fatalf("result = %+v, want path %q 3x2", result, want)
	}
	saved, err := os.ReadFile(filepath.Join(e.snapshotDir, "shot.png"))
	if err != nil || string(saved) != string(e.snapshots.data) {
		t.Fatalf("saved file: %v (matches capture: %v)", err, string(saved) == string(e.snapshots.data))
	}
}

func TestSnapshotNeverOverwritesAnExistingFile(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad
	writeFile(t, filepath.Join(e.snapshotDir, "taken.png"), "precious")

	w := e.do("GET", "/api/snapshot?out=taken.png", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("GET = %d %s, want 409", w.Code, w.Body.String())
	}
	if saved, _ := os.ReadFile(filepath.Join(e.snapshotDir, "taken.png")); string(saved) != "precious" {
		t.Fatalf("existing file was overwritten: %q", saved)
	}
}

func TestSnapshotDoesNotSaveACorruptCapture(t *testing.T) {
	e := newEnv(t)
	e.windows.current = &notepad
	e.snapshots.data = []byte("this is not a png")

	if w := e.do("GET", "/api/snapshot?out=bad.png", ""); w.Code != http.StatusBadGateway {
		t.Fatalf("GET = %d, want 502", w.Code)
	}
	if _, err := os.Stat(filepath.Join(e.snapshotDir, "bad.png")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt capture was saved: %v", err)
	}
	// Without out= the bytes are passed through untouched.
	if w := e.do("GET", "/api/snapshot", ""); w.Code != http.StatusOK {
		t.Fatalf("pass-through = %d", w.Code)
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
