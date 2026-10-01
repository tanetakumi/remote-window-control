//go:build !windows

package win32

// Stubs for platforms without the Win32 API. They keep the dependants
// buildable and testable; every call fails with ErrUnsupported.

func PostKey(HWND, uint16, bool) error                { return ErrUnsupported }
func PostMouseMove(HWND, int32, int32, Buttons) error { return ErrUnsupported }
func PostMouseButton(HWND, Button, bool, int32, int32) error {
	return ErrUnsupported
}
func SendMouseWheel(HWND, int32, int32, int32) error { return ErrUnsupported }
func SendText(HWND, string) error                    { return ErrUnsupported }
func ClientScreenRect(HWND) (Rect, error)            { return Rect{}, ErrUnsupported }
func CaptureRect(HWND) (Rect, error)                 { return Rect{}, ErrUnsupported }
func ResizeClient(HWND, int, int) error              { return ErrUnsupported }
func EnableDPIAwareness() error                      { return ErrUnsupported }
