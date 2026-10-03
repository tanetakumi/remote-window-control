package win32

const gaRootOwner = 3

var procGetAncestor = user32.NewProc("GetAncestor")

// BringToForeground activates the target and brings it in front of ordinary
// windows. Windows can deny activation; this does not set the topmost style or
// retry to take focus back from the host user.
func BringToForeground(hwnd HWND) error {
	foreground, _, _ := procGetForegroundWindow.Call()
	if HWND(foreground) == hwnd {
		return nil
	}
	if ok, _, _ := procSetForegroundWindow.Call(uintptr(hwnd)); ok == 0 {
		// SetForegroundWindow does not provide a reliable GetLastError value.
		return ErrForegroundDenied
	}
	// Activation across input queues is asynchronous. A bounded WM_NULL send
	// lets the target process the activation before we check its foreground
	// state, reusing the existing timeout rather than polling or attaching queues.
	if err := sendMessage(hwnd, 0, 0, 0); err != nil {
		return err
	}
	foreground, _, _ = procGetForegroundWindow.Call()
	if HWND(foreground) != hwnd {
		return ErrForegroundDenied
	}
	return nil
}

// ForegroundWindow returns the active window, or 0.
func ForegroundWindow() HWND {
	foreground, _, _ := procGetForegroundWindow.Call()
	return HWND(foreground)
}

// IsOwnedBy reports whether hwnd belongs to target's window family: the same
// root owner, which covers child windows and owned popups such as menus, but no
// other top-level window of the same process.
func IsOwnedBy(hwnd, target HWND) bool {
	if hwnd == 0 || target == 0 {
		return false
	}
	root, _, _ := procGetAncestor.Call(uintptr(hwnd), gaRootOwner)
	targetRoot, _, _ := procGetAncestor.Call(uintptr(target), gaRootOwner)
	return root != 0 && root == targetRoot
}
