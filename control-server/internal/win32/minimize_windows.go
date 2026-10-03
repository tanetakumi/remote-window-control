package win32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmCommand = 0x0111
	// trayMinimizeAll is the taskbar command behind Win+M ("minimize all").
	trayMinimizeAll = 419
)

var procFindWindowW = user32.NewProc("FindWindowW")

// MinimizeAll asks the taskbar to minimize all windows, as Win+M does, which
// leaves the choice of windows to the shell. It returns once the request is
// posted; the windows minimize afterwards.
func MinimizeAll() error {
	name, err := windows.UTF16PtrFromString("Shell_TrayWnd")
	if err != nil {
		return err
	}
	tray, _, err := procFindWindowW.Call(uintptr(unsafe.Pointer(name)), 0)
	if tray == 0 {
		return callError(err)
	}
	return postMessage(HWND(tray), wmCommand, trayMinimizeAll, 0)
}
