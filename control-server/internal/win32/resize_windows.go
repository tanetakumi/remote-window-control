package win32

import (
	"errors"
	"time"
	"unsafe"
)

const (
	swpNoZOrder       = 0x0004
	swpNoActivate     = 0x0010
	swpAsyncWindowPos = 0x4000

	swShowMaximized       = 3
	wpfRestoreToMaximized = 0x0002
	wpfAsyncPlacement     = 0x0004
)

var (
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
)

type windowPlacement struct {
	Length         uint32
	Flags          uint32
	ShowCmd        uint32
	MinPosition    point
	MaxPosition    point
	NormalPosition Rect
}

// LeaveMaximized makes a maximized window, or one that Windows would restore
// as maximized from minimized, a normal window, without activating it. It
// reports whether a change was needed. Resizing alone keeps that state, so a
// later restore from minimized, as when PC mode minimizes all windows, would
// maximize the window again. The change is queued and its completion checked
// for up to restoreTimeout, so a hung target cannot block input indefinitely.
func LeaveMaximized(hwnd HWND) (bool, error) {
	wp, err := getWindowPlacement(hwnd)
	if err != nil {
		return false, err
	}
	if wp.ShowCmd != swShowMaximized && wp.Flags&wpfRestoreToMaximized == 0 {
		return false, nil
	}
	wp.Flags &^= wpfRestoreToMaximized
	wp.Flags |= wpfAsyncPlacement
	wp.ShowCmd = swShowNoActivate
	if ok, _, err := procSetWindowPlacement.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wp))); ok == 0 {
		return true, callError(err)
	}
	deadline := time.Now().Add(restoreTimeout)
	for {
		wp, err = getWindowPlacement(hwnd)
		if err != nil {
			return true, err
		}
		if wp.ShowCmd != swShowMaximized && wp.Flags&wpfRestoreToMaximized == 0 {
			return true, nil
		}
		if time.Now().After(deadline) {
			return true, errors.New("the window could not leave its maximized state; restore it on the host PC and try again")
		}
		time.Sleep(restorePoll)
	}
}

func getWindowPlacement(hwnd HWND) (windowPlacement, error) {
	wp := windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{}))}
	if ok, _, err := procGetWindowPlacement.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wp))); ok == 0 {
		return wp, callError(err)
	}
	return wp, nil
}

// ResizeClient resizes the window's client area to wantWidth x wantHeight
// after fitting to the monitor work area (see PlanClientResize). It does not
// activate the window or change its z-order.
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
