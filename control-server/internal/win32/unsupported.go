//go:build !windows

package win32

import "os/exec"

// Stubs for platforms without the Win32 API. They keep the dependants
// buildable and testable; Windows command configuration is a no-op.

func HideConsole(*exec.Cmd) {}

func PostKey(HWND, uint16, bool) error                { return ErrUnsupported }
func PostMouseMove(HWND, int32, int32, Buttons) error { return ErrUnsupported }
func PostMouseButton(HWND, Button, bool, int32, int32) error {
	return ErrUnsupported
}
func SendMouseWheel(HWND, int32, int32, int32) error { return ErrUnsupported }
func SetClipboardText(string) error                  { return ErrUnsupported }
func SendKey(uint16, bool) error                     { return ErrUnsupported }
func IsForeground(HWND) bool                         { return false }
func ClientScreenRect(HWND) (Rect, error)            { return Rect{}, ErrUnsupported }
func CaptureRect(HWND) (Rect, error)                 { return Rect{}, ErrUnsupported }
func ResizeClient(HWND, int, int) error              { return ErrUnsupported }
func RestoreMinimized(HWND) (bool, error)            { return false, ErrUnsupported }
func LeaveMaximized(HWND) (bool, error)              { return false, ErrUnsupported }
func BringToForeground(HWND) error                   { return ErrUnsupported }
func EnableDPIAwareness() error                      { return ErrUnsupported }
func BuildNumber() (uint32, error)                   { return 0, ErrUnsupported }
func MovePointer(int32, int32) error                 { return ErrUnsupported }
func SendPointerButton(Button, bool) error           { return ErrUnsupported }
func SendPointerWheel(int32) error                   { return ErrUnsupported }
func VirtualDesktop() (Rect, error)                  { return Rect{}, ErrUnsupported }
func WindowAt(int32, int32) HWND                     { return 0 }
func ForegroundWindow() HWND                         { return 0 }
func IsOwnedBy(HWND, HWND) bool                      { return false }
func IsMinimized(HWND) bool                          { return false }
func MinimizeAll() error                             { return ErrUnsupported }
