package tray_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"share-app-host/internal/tray"
)

func TestIconLoadsAtTraySizes(t *testing.T) {
	iconPNG, err := os.ReadFile(filepath.Join("..", "..", "internal", "tray", "icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(iconPNG) == 0 {
		t.Fatal("tray icon is empty")
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	createIcon := user32.NewProc("CreateIconFromResourceEx")
	destroyIcon := user32.NewProc("DestroyIcon")
	for _, size := range []uintptr{16, 20, 24, 32, 48, 64} {
		icon, _, err := createIcon.Call(uintptr(unsafe.Pointer(&iconPNG[0])), uintptr(len(iconPNG)), 1, 0x00030000, size, size, 0)
		if icon == 0 {
			t.Fatalf("load %dx%d icon: %v", size, size, err)
		}
		if ok, _, err := destroyIcon.Call(icon); ok == 0 {
			t.Fatalf("destroy icon: %v", err)
		}
	}
}

func TestTrayStartupAndCleanup(t *testing.T) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	if shell, _, _ := user32.NewProc("GetShellWindow").Call(); shell == 0 {
		t.Skip("notification area requires a running Explorer shell")
	}
	// Repeating startup checks window cleanup and that teardown leaves no
	// WM_QUIT on a reused Windows thread's message queue.
	for range 2 {
		finished := make(chan error, 1)
		var exits atomic.Int32
		go func() {
			stop, err := tray.Start(func() { exits.Add(1) })
			if err == nil {
				stop()
				stop()
			}
			finished <- err
		}()
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
			if got := exits.Load(); got != 1 {
				t.Fatalf("exit callback called %d times, want 1", got)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("tray startup/cleanup did not finish")
		}
	}
}
