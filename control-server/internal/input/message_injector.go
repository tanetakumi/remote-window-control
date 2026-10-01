package input

import (
	"errors"
	"log"
	"time"

	"share-app-host/internal/win32"
	"share-app-host/internal/window"
)

// tapHold is how long a tap keeps the button down before releasing it.
const tapHold = 25 * time.Millisecond

// TargetSource reports the handle of the window that should receive input.
type TargetSource interface {
	CurrentHandle() (uint64, bool)
}

// MessageInjector is an Injector that posts window messages straight to the
// target window, so input needs neither focus nor the physical cursor.
type MessageInjector struct {
	target  TargetSource
	buttons win32.Buttons // buttons currently held, reported with pointer moves
}

// NewMessageInjector returns an injector that resolves the target window on
// every call, so it follows target changes.
func NewMessageInjector(target TargetSource) *MessageInjector {
	return &MessageInjector{target: target}
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
// viewport of the controlling browser, in device pixels.
func (m *MessageInjector) ResizeViewport(c Command) error {
	scale := c.DevicePixelRatio
	if scale <= 0 {
		scale = 1
	}
	width := int(float64(c.Width) * scale)
	height := int(float64(c.Height) * scale)
	if width <= 0 || height <= 0 {
		return nil
	}
	hwnd, err := m.handle()
	if err != nil {
		return err
	}
	// A minimized window ignores the new size until it is restored, and its
	// geometry (parked at -32000,-32000) would mislead the resize plan.
	if restored, err := win32.RestoreMinimized(hwnd); err != nil && !errors.Is(err, win32.ErrUnsupported) {
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

// key posts non-printable keys. Printable characters arrive as Text, so any
// other key is ignored.
func (m *MessageInjector) key(c Command, up bool) error {
	vk, ok := win32.VirtualKey(c.Key)
	if !ok {
		return nil
	}
	hwnd, err := m.handle()
	if err != nil {
		return err
	}
	return win32.PostKey(hwnd, vk, up)
}

func (m *MessageInjector) Text(text string) error {
	hwnd, err := m.handle()
	if err != nil {
		return err
	}
	return win32.SendText(hwnd, text)
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

func buttonFromName(name string) win32.Button {
	if name == buttonRight {
		return win32.ButtonRight
	}
	return win32.ButtonLeft
}
