package win32

import "unsafe"

const (
	inputKeyboard        = 1
	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	vkLShift             = 0xA0
	mapVKToVSC           = 0
)

var (
	procSendInput      = user32.NewProc("SendInput")
	procMapVirtualKeyW = user32.NewProc("MapVirtualKeyW")
)

// keybdInput and input mirror KEYBDINPUT and INPUT; the padding makes the
// union as large as MOUSEINPUT, as the OS expects.
type keybdInput struct {
	vk, scan  uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type input struct {
	inputType uint32
	ki        keybdInput
	_         [8]byte
}

func scanCode(vk uint16) uint16 {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), mapVKToVSC)
	return uint16(scan)
}

// IsForeground reports whether hwnd is the active window, the only one
// SendKey's input reaches.
func IsForeground(hwnd HWND) bool {
	foreground, _, _ := procGetForegroundWindow.Call()
	return HWND(foreground) == hwnd
}

// SendKey injects a key press or release into the system input stream, as a
// keyboard would. Unlike PostKey it also changes the key state that
// GetKeyState reports, which applications read for Shift, but it goes to
// whichever window is in the foreground.
func SendKey(vk uint16, up bool) error {
	if vk == VKShift {
		vk = vkLShift
	}
	ki := keybdInput{vk: vk, scan: scanCode(vk)}
	if IsExtendedKey(vk) {
		ki.flags |= keyeventfExtendedKey
	}
	if up {
		ki.flags |= keyeventfKeyUp
	}
	in := input{inputType: inputKeyboard, ki: ki}
	if sent, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in)); sent == 0 {
		return callError(err)
	}
	return nil
}
