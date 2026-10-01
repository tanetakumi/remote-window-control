package win32

import "unsafe"

const (
	monitorDefaultToNearest = 0x00000002
	dwmaExtendedFrameBounds = 9
)

type point struct{ X, Y int32 }

type monitorInfo struct {
	Size    uint32
	Monitor Rect
	Work    Rect
	Flags   uint32
}

// windowRect returns the outer rectangle of the window in screen coordinates.
func windowRect(hwnd HWND) (Rect, error) {
	var r Rect
	if ok, _, _ := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r))); ok == 0 {
		return Rect{}, ErrWindowUnavailable
	}
	return r, nil
}

// clientRect returns the client area size; its origin is always (0, 0).
func clientRect(hwnd HWND) (Rect, error) {
	var r Rect
	if ok, _, _ := procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r))); ok == 0 {
		return Rect{}, ErrWindowUnavailable
	}
	return r, nil
}

// ClientScreenRect returns the window's client area in screen coordinates.
func ClientScreenRect(hwnd HWND) (Rect, error) {
	size, err := clientRect(hwnd)
	if err != nil {
		return Rect{}, err
	}
	var origin point
	if ok, _, _ := procClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&origin))); ok == 0 {
		return Rect{}, ErrWindowUnavailable
	}
	return Rect{
		Left:   origin.X,
		Top:    origin.Y,
		Right:  origin.X + size.Width(),
		Bottom: origin.Y + size.Height(),
	}, nil
}

// CaptureRect returns the screen rectangle covered by the captured image: the
// DWM extended frame bounds, which exclude the invisible resize border, falling
// back to the plain window rectangle.
func CaptureRect(hwnd HWND) (Rect, error) {
	var r Rect
	hr, _, _ := procDwmGetWindowAttribute.Call(uintptr(hwnd), dwmaExtendedFrameBounds,
		uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r))
	if int32(hr) == 0 && r.Width() > 0 && r.Height() > 0 {
		return r, nil
	}
	return windowRect(hwnd)
}

// monitorWorkArea returns the work area of the monitor nearest to the window.
func monitorWorkArea(hwnd HWND) (Rect, bool) {
	monitor, _, _ := procMonitorFromWindow.Call(uintptr(hwnd), monitorDefaultToNearest)
	if monitor == 0 {
		return Rect{}, false
	}
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return Rect{}, false
	}
	return info.Work, true
}
