package win32

const (
	swpNoZOrder       = 0x0004
	swpNoActivate     = 0x0010
	swpAsyncWindowPos = 0x4000
)

// ResizeClient resizes the window so its client area is wantWidth x wantHeight,
// fitted to the monitor work area (see PlanClientResize). It does not activate
// the window or change its z-order.
func ResizeClient(hwnd HWND, wantWidth, wantHeight int) error {
	window, err := windowRect(hwnd)
	if err != nil {
		return err
	}
	client, err := clientRect(hwnd)
	if err != nil {
		return err
	}
	var work *Rect
	if area, ok := monitorWorkArea(hwnd); ok {
		work = &area
	}

	r := PlanClientResize(window, client, work, wantWidth, wantHeight)
	ok, _, callErr := procSetWindowPos.Call(uintptr(hwnd), 0,
		uintptr(r.Left), uintptr(r.Top), uintptr(r.Width()), uintptr(r.Height()),
		swpAsyncWindowPos|swpNoZOrder|swpNoActivate)
	if ok == 0 {
		return callError(callErr)
	}
	return nil
}
