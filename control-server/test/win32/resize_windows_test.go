package win32_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"share-app-host/internal/win32"
)

// A private, nonactivating window avoids resizing or focusing a user's app.
func maximizedTestWindow() (win32.HWND, func(), error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	class, err := windows.UTF16PtrFromString("STATIC")
	if err != nil {
		return 0, nil, err
	}
	const exNoActivate = 0x08000000
	const overlappedMaximized = 0x00CF0000 | 0x01000000
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(exNoActivate, uintptr(unsafe.Pointer(class)), 0,
		overlappedMaximized, 0, 0, 640, 480, 0, 0, 0, 0)
	if hwnd == 0 {
		return 0, nil, fmt.Errorf("create test window: %w", err)
	}
	return win32.HWND(hwnd), func() { user32.NewProc("DestroyWindow").Call(hwnd) }, nil
}

func startTestWindow(t *testing.T, responsive bool) win32.HWND {
	t.Helper()
	type result struct {
		hwnd   win32.HWND
		thread uint32
		err    error
	}
	ready, stop, exited := make(chan result, 1), make(chan struct{}), make(chan struct{})
	user32 := windows.NewLazySystemDLL("user32.dll")
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(exited)
		hwnd, destroy, err := maximizedTestWindow()
		if err == nil {
			defer destroy()
		}
		ready <- result{hwnd, windows.GetCurrentThreadId(), err}
		if err != nil {
			return
		}
		if !responsive {
			<-stop // Deliberately leave window messages unprocessed.
			return
		}
		// MSG, including its Win64 alignment and private field.
		var msg struct {
			hwnd           uintptr
			id             uint32
			wparam, lparam uintptr
			time           uint32
			x, y           int32
			private        uint32
		}
		for {
			ok, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ok) <= 0 {
				return
			}
			user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
	window := <-ready
	t.Cleanup(func() {
		close(stop)
		user32.NewProc("PostThreadMessageW").Call(uintptr(window.thread), 0x0012, 0, 0) // WM_QUIT
		<-exited
	})
	if window.err != nil {
		t.Fatal(window.err)
	}
	return window.hwnd
}

func TestResizeDoesNotMaximizeAgainAfterMinimizing(t *testing.T) {
	hwnd := startTestWindow(t, true)
	if changed, err := win32.LeaveMaximized(hwnd); err != nil || !changed {
		t.Fatalf("leave maximized: %v, %v", changed, err)
	}
	if err := win32.ResizeClient(hwnd, 320, 240); err != nil {
		t.Fatal(err)
	}
	// ResizeClient queues sizing; do not minimize until it has completed.
	deadline := time.Now().Add(time.Second)
	for {
		client, err := win32.ClientScreenRect(hwnd)
		if err == nil && client.Width() == 320 && client.Height() == 240 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resize did not complete: %+v, %v", client, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	user32.NewProc("ShowWindow").Call(uintptr(hwnd), 7) // SW_SHOWMINNOACTIVE
	if restored, err := win32.RestoreMinimized(hwnd); err != nil || !restored {
		t.Fatalf("restore resized window: %v, %v", restored, err)
	}
	client, err := win32.ClientScreenRect(hwnd)
	if err != nil || client.Width() != 320 || client.Height() != 240 {
		t.Fatalf("restored client = %+v, %v; want 320x240", client, err)
	}
	if changed, err := win32.LeaveMaximized(hwnd); err != nil || changed {
		t.Fatalf("resized window retained maximized state: %v, %v", changed, err)
	}
}

func TestLeaveMaximizedDoesNotWaitIndefinitelyForUnresponsiveWindow(t *testing.T) {
	hwnd := startTestWindow(t, false)
	done := make(chan error, 1)
	go func() { _, err := win32.LeaveMaximized(hwnd); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("unresponsive window unexpectedly restored")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("maximized restoration blocked on the target thread")
	}
}
