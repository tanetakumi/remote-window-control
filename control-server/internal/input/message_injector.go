package input

import (
	"errors"
	"log"
	"strings"
	"time"

	"share-app-host/internal/win32"
	"share-app-host/internal/window"
)

const (
	// tapHold is how long a tap keeps the button down before releasing it.
	tapHold = 25 * time.Millisecond
	// inputDrain is how long the injector waits after the last key sent as
	// real input. A window receives posted messages ahead of input from the
	// system, so messages posted straight afterwards could overtake those keys.
	inputDrain = 30 * time.Millisecond
)

// TargetSource reports the handle of the window that should receive input.
type TargetSource interface {
	CurrentHandle() (uint64, bool)
}

// Keys delivers key presses and the clipboard to a window. Posted keys reach it
// without focus; sent keys go through the system input stream to the foreground
// window.
type Keys interface {
	// Post posts a key message to hwnd.
	Post(hwnd win32.HWND, vk uint16, up bool) error
	// Send injects a key into the system input stream.
	Send(vk uint16, up bool) error
	// Foreground reports whether hwnd is the active window.
	Foreground(hwnd win32.HWND) bool
	// Activate brings hwnd to the foreground; it does nothing if it already is.
	Activate(hwnd win32.HWND) error
	// SetClipboard replaces the clipboard text.
	SetClipboard(text string) error
}

type win32Keys struct{}

func (win32Keys) Post(hwnd win32.HWND, vk uint16, up bool) error { return win32.PostKey(hwnd, vk, up) }
func (win32Keys) Send(vk uint16, up bool) error                  { return win32.SendKey(vk, up) }
func (win32Keys) Foreground(hwnd win32.HWND) bool                { return win32.IsForeground(hwnd) }
func (win32Keys) Activate(hwnd win32.HWND) error                 { return win32.BringToForeground(hwnd) }
func (win32Keys) SetClipboard(text string) error                 { return win32.SetClipboardText(text) }

// MessageInjector is an Injector that posts window messages straight to the
// target window, so input needs neither focus nor the physical cursor. The
// exceptions are Shift and pasted text: applications read Shift and Ctrl from
// the system key state, which posted messages cannot change, so a Shift press
// on a foreground target is sent as real input, and every key until its
// release follows it there to stay in order. Text always needs the foreground.
type MessageInjector struct {
	target   TargetSource
	maxScale func() float64
	keys     Keys
	buttons  win32.Buttons // buttons currently held, reported with pointer moves
	realKeys bool          // keys are being sent as real input until Shift is released
}

// NewMessageInjector returns an injector that resolves the target window on
// every call, so it follows target changes. maxScale is read on every viewport
// resize and gives the largest number of window pixels per CSS pixel.
func NewMessageInjector(target TargetSource, maxScale func() float64) *MessageInjector {
	return NewMessageInjectorWithKeys(target, maxScale, win32Keys{})
}

// NewMessageInjectorWithKeys is NewMessageInjector with a custom key delivery.
func NewMessageInjectorWithKeys(target TargetSource, maxScale func() float64, keys Keys) *MessageInjector {
	return &MessageInjector{target: target, maxScale: maxScale, keys: keys}
}

func (m *MessageInjector) Move(x, y float64) error {
	hwnd, cx, cy, err := m.locate(x, y)
	if err != nil {
		return err
	}
	return win32.PostMouseMove(hwnd, cx, cy, m.buttons)
}

func (m *MessageInjector) Tap(button string, x, y float64) error {
	hwnd, cx, cy, err := m.locate(x, y)
	if err != nil {
		return err
	}
	b := buttonFromName(button)
	_ = win32.PostMouseMove(hwnd, cx, cy, m.buttons)
	if err := win32.PostMouseButton(hwnd, b, true, cx, cy); err != nil {
		return err
	}
	time.Sleep(tapHold)
	m.buttons = m.buttons.Without(b)
	return win32.PostMouseButton(hwnd, b, false, cx, cy)
}

func (m *MessageInjector) MouseDown(button string, x, y float64) error {
	hwnd, cx, cy, err := m.locate(x, y)
	if err != nil {
		return err
	}
	b := buttonFromName(button)
	_ = win32.PostMouseMove(hwnd, cx, cy, m.buttons)
	if err := win32.PostMouseButton(hwnd, b, true, cx, cy); err != nil {
		return err
	}
	m.buttons = m.buttons.With(b)
	return nil
}

func (m *MessageInjector) MouseUp(button string, x, y float64) error {
	hwnd, cx, cy, err := m.locate(x, y)
	if err != nil {
		return err
	}
	b := buttonFromName(button)
	m.buttons = m.buttons.Without(b)
	return win32.PostMouseButton(hwnd, b, false, cx, cy)
}

func (m *MessageInjector) Scroll(deltaY, x, y float64) error {
	hwnd, cx, cy, err := m.locate(x, y)
	if err != nil {
		return err
	}
	return win32.SendMouseWheel(hwnd, win32.WheelDelta(deltaY), cx, cy)
}

// ResizeViewport resizes the target window so its client area matches the
// viewport of the controlling browser in device pixels, at most maxScale
// pixels per CSS pixel.
func (m *MessageInjector) ResizeViewport(c Command) error {
	width, height, ok := win32.ViewportPixels(c.Width, c.Height, c.DevicePixelRatio, m.maxScale())
	if !ok {
		return nil
	}
	hwnd, err := m.handle()
	if err != nil {
		return err
	}
	// A minimized window ignores the new size until it is restored, and its
	// geometry (parked at -32000,-32000) would mislead the resize plan.
	if restored, err := win32.RestoreMinimized(hwnd); err != nil {
		log.Printf("viewport resize restore failed hwnd=%d: %v", hwnd, err)
		return err
	} else if restored {
		log.Printf("viewport resize restored minimized window hwnd=%d", hwnd)
	}
	if err := win32.ResizeClient(hwnd, width, height); err != nil {
		log.Printf("viewport resize failed hwnd=%d: %v", hwnd, err)
		return err
	}
	return nil
}

func (m *MessageInjector) KeyDown(c Command) error { return m.key(c, false) }

func (m *MessageInjector) KeyUp(c Command) error { return m.key(c, true) }

// key delivers non-printable keys, posted unless Shift made them real input.
// Printable characters arrive as Text, so any other key is ignored.
func (m *MessageInjector) key(c Command, up bool) error {
	vk, ok := win32.VirtualKey(c.Key)
	if !ok {
		return nil
	}
	// Real input needs no target, so a held Shift is released even after the
	// target window is gone.
	if !m.realKeys {
		hwnd, err := m.handle()
		if err != nil {
			return err
		}
		if vk != win32.VKShift || up || !m.keys.Foreground(hwnd) {
			return m.keys.Post(hwnd, vk, up)
		}
	}
	err := m.keys.Send(vk, up)
	if vk == win32.VKShift {
		m.realKeys = !up && err == nil
		if up {
			time.Sleep(inputDrain)
		}
	}
	return err
}

// Text types text by putting it on the host clipboard and pressing Ctrl+V as
// real input, so the text arrives in one piece, in order with other real input
// and without passing through an input method editor. Applications read Ctrl
// from the system key state, so the target must be in the foreground; it is
// brought there first. The host clipboard is left holding the text.
func (m *MessageInjector) Text(text string) error {
	hwnd, err := m.handle()
	if err != nil {
		return err
	}
	if err := m.keys.Activate(hwnd); err != nil {
		return err
	}
	crlf := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	if err := m.keys.SetClipboard(crlf); err != nil {
		return err
	}
	if err := m.keys.Send(win32.VKControl, false); err != nil {
		return err
	}
	err = m.keys.Send(win32.VKV, false)
	if err == nil {
		err = m.keys.Send(win32.VKV, true)
	}
	err = errors.Join(err, m.keys.Send(win32.VKControl, true))
	time.Sleep(inputDrain)
	return err
}

// handle returns the selected window, or window.ErrNotSelected.
func (m *MessageInjector) handle() (win32.HWND, error) {
	if m.target == nil {
		return 0, window.ErrNotSelected
	}
	handle, ok := m.target.CurrentHandle()
	if !ok || handle == 0 {
		return 0, window.ErrNotSelected
	}
	return win32.HWND(handle), nil
}

// locate resolves the target window and maps a normalised point to its client
// coordinates.
func (m *MessageInjector) locate(x, y float64) (hwnd win32.HWND, clientX, clientY int32, err error) {
	hwnd, err = m.handle()
	if err != nil {
		return 0, 0, 0, err
	}
	capture, err := win32.CaptureRect(hwnd)
	if err != nil {
		return 0, 0, 0, err
	}
	client, err := win32.ClientScreenRect(hwnd)
	if err != nil {
		return 0, 0, 0, err
	}
	clientX, clientY, ok := win32.MapToClient(capture, client, x, y)
	if !ok {
		return 0, 0, 0, win32.ErrWindowUnavailable
	}
	return hwnd, clientX, clientY, nil
}

// buttonFromName maps a button name the Dispatcher has already validated.
func buttonFromName(name string) win32.Button {
	if name == buttonRight {
		return win32.ButtonRight
	}
	return win32.ButtonLeft
}
