// Package win32 is a thin layer over the Win32 calls the host needs to configure
// processes and drive another application's window: posting input messages,
// reading geometry, restoring and activating windows and resizing the client area.
//
// Everything that touches the operating system lives in *_windows.go files.
// On other platforms unsupported.go provides stubs, so dependants build and
// their logic can be tested anywhere.
// The platform-independent rules (key mapping, coordinate mapping, sizing)
// live in untagged files and are unit-tested directly.
package win32

import "errors"

var (
	// ErrUnsupported is returned by operations requiring Windows on other platforms.
	ErrUnsupported = errors.New("win32 API is only available on Windows")
	// ErrWindowUnavailable is returned when a window's geometry cannot be read,
	// for example because the window was closed after being selected.
	ErrWindowUnavailable = errors.New("target window is unavailable")
	// ErrWindowMinimized is returned when a minimized window could not be
	// restored, so it cannot be captured.
	ErrWindowMinimized = errors.New("the window is minimized and could not be restored; restore it on the host PC and connect again")
	// ErrForegroundDenied is returned when the target did not become foreground.
	ErrForegroundDenied = errors.New("the window could not be brought to the foreground; activate it on the host PC if rendering stops")
)

// HWND is a native window handle.
type HWND uintptr

// Rect mirrors the Win32 RECT layout, so it can be passed to the OS directly.
type Rect struct {
	Left, Top, Right, Bottom int32
}

// Width returns the horizontal extent of r.
func (r Rect) Width() int32 { return r.Right - r.Left }

// Height returns the vertical extent of r.
func (r Rect) Height() int32 { return r.Bottom - r.Top }

// Button is a mouse button the host can press on the target window.
type Button uint8

const (
	ButtonLeft Button = iota
	ButtonRight
)

// Buttons is the MK_* bit set reported with mouse messages for held buttons.
type Buttons uintptr

const (
	mkLButton Buttons = 0x0001
	mkRButton Buttons = 0x0002
)

func (b Button) flag() Buttons {
	if b == ButtonRight {
		return mkRButton
	}
	return mkLButton
}

// With returns the set with b marked as held.
func (s Buttons) With(b Button) Buttons { return s | b.flag() }

// Without returns the set with b marked as released.
func (s Buttons) Without(b Button) Buttons { return s &^ b.flag() }
