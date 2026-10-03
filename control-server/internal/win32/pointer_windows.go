package win32

import "unsafe"

const (
	inputMouse = 0

	mouseeventfMove        = 0x0001
	mouseeventfLeftDown    = 0x0002
	mouseeventfRightDown   = 0x0008
	mouseeventfWheel       = 0x0800
	mouseeventfVirtualDesk = 0x4000
	mouseeventfAbsolute    = 0x8000

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
)

var (
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procWindowFromPoint  = user32.NewProc("WindowFromPoint")
)

// mouseInput and pointerInput mirror MOUSEINPUT and INPUT; MOUSEINPUT is the
// largest union member, so this INPUT has the size the OS expects.
type mouseInput struct {
	dx, dy            int32
	data, flags, time uint32
	extraInfo         uintptr
}

type pointerInput struct {
	inputType uint32
	mi        mouseInput
}

func sendPointer(mi mouseInput) error {
	in := pointerInput{inputType: inputMouse, mi: mi}
	if sent, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in)); sent == 0 {
		return callError(err)
	}
	return nil
}

// MovePointer moves the real cursor to absolute coordinates (0..65535) on the
// whole virtual desktop; see MapToDesktop.
func MovePointer(x, y int32) error {
	return sendPointer(mouseInput{dx: x, dy: y, flags: mouseeventfMove | mouseeventfAbsolute | mouseeventfVirtualDesk})
}

// SendPointerButton presses or releases a button at the cursor as real input.
func SendPointerButton(button Button, up bool) error {
	flags := uint32(mouseeventfLeftDown)
	if button == ButtonRight {
		flags = mouseeventfRightDown
	}
	if up {
		flags <<= 1 // each *UP flag follows its *DOWN flag
	}
	return sendPointer(mouseInput{flags: flags})
}

// SendPointerWheel turns the wheel by delta (see WheelDelta) as real input.
func SendPointerWheel(delta int32) error {
	return sendPointer(mouseInput{data: uint32(delta), flags: mouseeventfWheel})
}

// VirtualDesktop returns the bounding rectangle of all monitors in physical
// pixels.
func VirtualDesktop() (Rect, error) {
	metric := func(index uintptr) int32 {
		value, _, _ := procGetSystemMetrics.Call(index)
		return int32(value)
	}
	x, y := metric(smXVirtualScreen), metric(smYVirtualScreen)
	w, h := metric(smCXVirtualScreen), metric(smCYVirtualScreen)
	if w <= 0 || h <= 0 {
		return Rect{}, ErrWindowUnavailable
	}
	return Rect{Left: x, Top: y, Right: x + w, Bottom: y + h}, nil
}

// WindowAt returns the window at a screen point, or 0.
func WindowAt(x, y int32) HWND {
	// POINT is passed by value, packed into one Win64 register.
	point := uint64(uint32(x)) | uint64(uint32(y))<<32
	hwnd, _, _ := procWindowFromPoint.Call(uintptr(point))
	return HWND(hwnd)
}
