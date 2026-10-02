package win32

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
