package win32

import (
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002

	// Another process can hold the clipboard open for a moment.
	clipboardAttempts = 10
	clipboardRetry    = 10 * time.Millisecond
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
)

// SetClipboardText replaces the clipboard contents with text. Line breaks are
// stored as given; Windows applications expect CR LF.
func SetClipboardText(text string) error {
	units := utf16.Encode([]rune(text + "\x00"))
	size := uintptr(len(units)) * unsafe.Sizeof(units[0])
	mem, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if mem == 0 {
		return callError(err)
	}
	ptr, _, err := procGlobalLock.Call(mem)
	if ptr == 0 {
		procGlobalFree.Call(mem)
		return callError(err)
	}
	procRtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&units[0])), size)
	procGlobalUnlock.Call(mem)

	if err := openClipboard(); err != nil {
		procGlobalFree.Call(mem)
		return err
	}
	defer procCloseClipboard.Call()
	if ok, _, err := procEmptyClipboard.Call(); ok == 0 {
		procGlobalFree.Call(mem)
		return callError(err)
	}
	// On success the clipboard owns the memory.
	if ok, _, err := procSetClipboardData.Call(cfUnicodeText, mem); ok == 0 {
		procGlobalFree.Call(mem)
		return callError(err)
	}
	return nil
}

func openClipboard() error {
	var err error
	for attempt := 0; attempt < clipboardAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(clipboardRetry)
		}
		var ok uintptr
		if ok, _, err = procOpenClipboard.Call(0); ok != 0 {
			return nil
		}
	}
	return callError(err)
}
