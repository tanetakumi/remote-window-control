package win32

// VKShift is VK_SHIFT, the only modifier the web client sends.
const VKShift uint16 = 0x10

// domKeyToVirtualKey maps the KeyboardEvent.key values the web client sends
// for non-printable keys to Win32 virtual-key codes. Printable characters are
// delivered as text instead, so they are intentionally absent.
var domKeyToVirtualKey = map[string]uint16{
	"Backspace":  0x08, // VK_BACK
	"Tab":        0x09, // VK_TAB
	"Enter":      0x0D, // VK_RETURN
	"Shift":      VKShift,
	"Escape":     0x1B, // VK_ESCAPE
	" ":          0x20, // VK_SPACE
	"PageUp":     0x21, // VK_PRIOR
	"PageDown":   0x22, // VK_NEXT
	"End":        0x23, // VK_END
	"Home":       0x24, // VK_HOME
	"ArrowLeft":  0x25, // VK_LEFT
	"ArrowUp":    0x26, // VK_UP
	"ArrowRight": 0x27, // VK_RIGHT
	"ArrowDown":  0x28, // VK_DOWN
	"Delete":     0x2E, // VK_DELETE
}

// IsExtendedKey reports whether a key is reported with the extended-key bit
// (the navigation block and arrows, as opposed to the numeric keypad).
func IsExtendedKey(vk uint16) bool {
	switch vk {
	case 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x2E:
		return true
	}
	return false
}

// VirtualKey returns the virtual-key code for a DOM key name, and false for
// keys that are not delivered as key messages.
func VirtualKey(domKey string) (uint16, bool) {
	vk, ok := domKeyToVirtualKey[domKey]
	return vk, ok
}
