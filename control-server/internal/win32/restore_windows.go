package win32

import "time"

const (
	swShowNoActivate = 4

	// These bound the wait for asynchronous window restoration, from either
	// minimized or maximized, before capture or sizing can continue.
	restoreTimeout = time.Second
	restorePoll    = 20 * time.Millisecond
)

// isMinimized reports whether the window is minimized (iconic).
func isMinimized(hwnd HWND) bool {
	iconic, _, _ := procIsIconic.Call(uintptr(hwnd))
	return iconic != 0
}

// RestoreMinimized restores a minimized window to its previous size and
// position without activating it, and waits until it is no longer minimized.
// It reports whether a restore was needed. Windows Graphics Capture delivers no
// frames for a minimized window, and resizing one has no visible effect.
func RestoreMinimized(hwnd HWND) (bool, error) {
	if !isMinimized(hwnd) {
		return false, nil
	}
	// ShowWindowAsync does not block on a hung target. A zero result means the
	// request could not be queued; a nonzero result still needs state checking.
	queued, _, _ := procShowWindowAsync.Call(uintptr(hwnd), swShowNoActivate)
	if queued == 0 {
		return true, ErrWindowMinimized
	}
	deadline := time.Now().Add(restoreTimeout)
	for isMinimized(hwnd) {
		if time.Now().After(deadline) {
			return true, ErrWindowMinimized
		}
		time.Sleep(restorePoll)
	}
	return true, nil
}
