//go:build windows

package win32_test

import (
	"testing"

	"golang.org/x/sys/windows"
	"share-app-host/internal/win32"
)

func TestInstallLockStaysVisibleUntilLastHostExits(t *testing.T) {
	name, err := windows.UTF16PtrFromString(`Local\ShareApp.Running.9E7D147B-C72B-4B55-86C9-1D9A64379EE5`)
	if err != nil {
		t.Fatal(err)
	}
	first, err := win32.HoldInstallLock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first() })
	second, err := win32.HoldInstallLock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second() })
	first()
	// Avoid closing a recycled Windows handle from Cleanup.
	first = func() {}
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		t.Fatalf("lock vanished while second host is running: %v", err)
	}
	_ = windows.CloseHandle(handle)
	second()
	second = func() {}
	if handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name); err == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("lock remained after all host handles closed")
	}
}
