package win32

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dwmapi = syscall.NewLazyDLL("dwmapi.dll")

	procGetWindowRect             = user32.NewProc("GetWindowRect")
	procGetClientRect             = user32.NewProc("GetClientRect")
	procClientToScreen            = user32.NewProc("ClientToScreen")
	procMonitorFromWindow         = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW           = user32.NewProc("GetMonitorInfoW")
	procSendMessageTimeoutW       = user32.NewProc("SendMessageTimeoutW")
	procPostMessageW              = user32.NewProc("PostMessageW")
	procSetWindowPos              = user32.NewProc("SetWindowPos")
	procIsIconic                  = user32.NewProc("IsIconic")
	procShowWindowAsync           = user32.NewProc("ShowWindowAsync")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procDwmGetWindowAttribute     = dwmapi.NewProc("DwmGetWindowAttribute")
)

var errCallFailed = errors.New("win32 call failed or timed out")

// callError converts the result of a failed call into an error. A Win32 BOOL
// result decides success; GetLastError may hold a stale value, so a zero errno
// still yields a generic failure.
func callError(err error) error {
	if err != nil && err != syscall.Errno(0) {
		return err
	}
	return errCallFailed
}

func postMessage(hwnd HWND, msg uint32, wParam, lParam uintptr) error {
	if ok, _, err := procPostMessageW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam); ok == 0 {
		return callError(err)
	}
	return nil
}

const (
	smtoAbortIfHung = 0x0002
	smtoErrorOnExit = 0x0020

	// sendTimeoutMs bounds how long a hung target can block a send.
	sendTimeoutMs = 100
)

// sendMessage delivers a message synchronously, giving up if the target hangs.
func sendMessage(hwnd HWND, msg uint32, wParam, lParam uintptr) error {
	var result uintptr
	ok, _, err := procSendMessageTimeoutW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam,
		smtoAbortIfHung|smtoErrorOnExit, sendTimeoutMs, uintptr(unsafe.Pointer(&result)))
	if ok == 0 {
		return callError(err)
	}
	return nil
}

// makeLParam packs two 16-bit coordinates, as mouse messages expect.
func makeLParam(x, y int32) uintptr {
	return uintptr(uint32(uint16(x)) | uint32(uint16(y))<<16)
}
