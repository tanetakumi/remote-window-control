//go:build windows

package win32

import (
	"errors"

	"golang.org/x/sys/windows"
)

// HoldInstallLock tells Setup and Uninstall that the host (and its child
// processes) must finish before files can be replaced. Keep the handle alive
// throughout app.Run. This is a presence marker, not a single-instance lock.
func HoldInstallLock() (func(), error) {
	name, err := windows.UTF16PtrFromString(`Local\ShareApp.Running.9E7D147B-C72B-4B55-86C9-1D9A64379EE5`)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	return func() { _ = windows.CloseHandle(handle) }, nil
}
