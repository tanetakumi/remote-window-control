package win32

import "unicode/utf16"

const (
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmChar       = 0x0102
	wmMouseMove  = 0x0200
	wmMouseWheel = 0x020A
)

// Mouse button messages in (down, up) order, indexed by Button.
var buttonMessages = [...][2]uint32{
	ButtonLeft:  {0x0201, 0x0202}, // WM_LBUTTONDOWN, WM_LBUTTONUP
	ButtonRight: {0x0204, 0x0205}, // WM_RBUTTONDOWN, WM_RBUTTONUP
}

// PostKey posts a key press or release for a virtual-key code, with the scan
// code and extended-key bit a physical key press would carry.
func PostKey(hwnd HWND, vk uint16, up bool) error {
	lParam := uintptr(1) | uintptr(scanCode(vk))<<16
	if IsExtendedKey(vk) {
		lParam |= 1 << 24
	}
	if up {
		return postMessage(hwnd, wmKeyUp, uintptr(vk), lParam|0xC0000000)
	}
	return postMessage(hwnd, wmKeyDown, uintptr(vk), lParam)
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

// PostText types text into the window one UTF-16 unit at a time. The units are
// posted, like PostKey's messages, so text and keys reach the window in the
// order they were sent.
func PostText(hwnd HWND, text string) error {
	for _, unit := range utf16.Encode([]rune(text)) {
		if err := postMessage(hwnd, wmChar, uintptr(unit), 0); err != nil {
			return err
		}
	}
	return nil
}
