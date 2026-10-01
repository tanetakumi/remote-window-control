package win32

import (
	"errors"
	"time"
	"unicode/utf16"
)

const (
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmChar       = 0x0102
	wmMouseMove  = 0x0200
	wmMouseWheel = 0x020A

	// textDeadline bounds how long typing one string may take in total.
	textDeadline = 2 * time.Second
)

// Mouse button messages in (down, up) order, indexed by Button.
var buttonMessages = [...][2]uint32{
	ButtonLeft:  {0x0201, 0x0202}, // WM_LBUTTONDOWN, WM_LBUTTONUP
	ButtonRight: {0x0204, 0x0205}, // WM_RBUTTONDOWN, WM_RBUTTONUP
}

var errTextTimeout = errors.New("text input timed out")

// PostKey posts a key press or release for a virtual-key code.
func PostKey(hwnd HWND, vk uint16, up bool) error {
	if up {
		return postMessage(hwnd, wmKeyUp, uintptr(vk), 0xC0000001)
	}
	return postMessage(hwnd, wmKeyDown, uintptr(vk), 1)
}

// PostMouseMove posts a pointer move to client coordinates, reporting the
// buttons currently held.
func PostMouseMove(hwnd HWND, clientX, clientY int32, held Buttons) error {
	return postMessage(hwnd, wmMouseMove, uintptr(held), makeLParam(clientX, clientY))
}

// PostMouseButton posts a button press or release at client coordinates.
func PostMouseButton(hwnd HWND, button Button, down bool, clientX, clientY int32) error {
	msgs := buttonMessages[button]
	if down {
		return postMessage(hwnd, msgs[0], uintptr(button.flag()), makeLParam(clientX, clientY))
	}
	return postMessage(hwnd, msgs[1], 0, makeLParam(clientX, clientY))
}

// SendMouseWheel sends a wheel scroll at client coordinates. delta is a Win32
// wheel delta (see WheelDelta). WM_MOUSEWHEEL carries screen coordinates, so the
// client point is converted first.
func SendMouseWheel(hwnd HWND, delta, clientX, clientY int32) error {
	screenX, screenY := clientX, clientY
	if origin, err := ClientScreenRect(hwnd); err == nil {
		screenX += origin.Left
		screenY += origin.Top
	}
	wParam := uintptr(uint32(int16(delta)) << 16)
	return sendMessage(hwnd, wmMouseWheel, wParam, makeLParam(screenX, screenY))
}

// SendText types text into the window one UTF-16 unit at a time.
func SendText(hwnd HWND, text string) error {
	deadline := time.Now().Add(textDeadline)
	for _, unit := range utf16.Encode([]rune(text)) {
		if time.Now().After(deadline) {
			return errTextTimeout
		}
		if err := sendMessage(hwnd, wmChar, uintptr(unit), 0); err != nil {
			return err
		}
	}
	return nil
}
