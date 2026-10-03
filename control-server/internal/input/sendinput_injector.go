package input

import (
	"errors"

	"share-app-host/internal/win32"
)

// ErrObscured rejects a PC-mode press that another window would receive.
var ErrObscured = errors.New("another window covers the target; bring the target to the front on the host PC")

// DesktopInput is the OS access of SendInputInjector: real pointer input and
// the window family checks guarding it, in addition to Keys.
type DesktopInput interface {
	Keys
	CaptureRect(win32.HWND) (win32.Rect, error)
	VirtualDesktop() (win32.Rect, error)
	WindowAt(x, y int32) win32.HWND
	ForegroundWindow() win32.HWND
	IsOwnedBy(hwnd, target win32.HWND) bool
	MovePointer(x, y int32) error
	MouseButton(button win32.Button, up bool) error
	Wheel(delta int32) error
}

type win32Desktop struct{ win32Keys }

func (win32Desktop) CaptureRect(h win32.HWND) (win32.Rect, error) { return win32.CaptureRect(h) }
func (win32Desktop) VirtualDesktop() (win32.Rect, error)          { return win32.VirtualDesktop() }
func (win32Desktop) WindowAt(x, y int32) win32.HWND               { return win32.WindowAt(x, y) }
func (win32Desktop) ForegroundWindow() win32.HWND                 { return win32.ForegroundWindow() }
func (win32Desktop) IsOwnedBy(h, target win32.HWND) bool          { return win32.IsOwnedBy(h, target) }
func (win32Desktop) MovePointer(x, y int32) error                 { return win32.MovePointer(x, y) }
func (win32Desktop) MouseButton(b win32.Button, up bool) error    { return win32.SendPointerButton(b, up) }
func (win32Desktop) Wheel(delta int32) error                      { return win32.SendPointerWheel(delta) }

// familyKeys treats any window of the target's family as the foreground, so a
// focused popup such as a menu is not closed by reactivating the main window.
type familyKeys struct{ DesktopInput }

func (k familyKeys) Foreground(hwnd win32.HWND) bool {
	return k.IsOwnedBy(k.ForegroundWindow(), hwnd)
}

func (k familyKeys) Activate(hwnd win32.HWND) error {
	if k.Foreground(hwnd) {
		return nil
	}
	return k.DesktopInput.Activate(hwnd)
}

// SendInputInjector is the PC-mode Injector. Pointer and key input go through
// the system input stream, sharing the host's cursor and focus, so popups owned
// by the target receive it too. A new press is sent only where the target's
// family would receive it; releases are always sent. Text and viewport sizing
// are MessageInjector's, with family-aware activation.
type SendInputInjector struct {
	*MessageInjector
	desktop DesktopInput
}

// NewSendInputInjector returns a SendInputInjector that resolves the target
// window on every call, like NewMessageInjector.
func NewSendInputInjector(target TargetSource, maxScale func() float64) *SendInputInjector {
	return NewSendInputInjectorWithDesktop(target, maxScale, win32Desktop{})
}

// NewSendInputInjectorWithDesktop is NewSendInputInjector with custom OS access.
func NewSendInputInjectorWithDesktop(target TargetSource, maxScale func() float64, desktop DesktopInput) *SendInputInjector {
	return &SendInputInjector{
		MessageInjector: NewMessageInjectorWithKeys(target, maxScale, familyKeys{desktop}),
		desktop:         desktop,
	}
}

func (s *SendInputInjector) Move(x, y float64) error { return s.point(x, y, false) }

func (s *SendInputInjector) Tap(button string, x, y float64) error {
	if err := s.MouseDown(button, x, y); err != nil {
		return err
	}
	return s.MouseUp(button, x, y)
}

func (s *SendInputInjector) MouseDown(button string, x, y float64) error {
	if err := s.point(x, y, true); err != nil {
		return err
	}
	b := buttonFromName(button)
	if err := s.desktop.MouseButton(b, false); err != nil {
		return err
	}
	s.buttons = s.buttons.With(b)
	return nil
}

// MouseUp releases only buttons this injector pressed: the Dispatcher also
// forwards the release of a rejected press, which must not reach another
// application. The pointer is not moved, so a release works without a target.
func (s *SendInputInjector) MouseUp(button string, x, y float64) error {
	b := buttonFromName(button)
	if s.buttons.Without(b) == s.buttons {
		return nil
	}
	if err := s.desktop.MouseButton(b, true); err != nil {
		return err
	}
	s.buttons = s.buttons.Without(b)
	return nil
}

// Scroll needs the target family in the foreground as well as under the
// pointer, since Windows can route the wheel to the focused window.
func (s *SendInputInjector) Scroll(deltaY, x, y float64) error {
	if err := s.activate(); err != nil {
		return err
	}
	if err := s.point(x, y, true); err != nil {
		return err
	}
	return s.desktop.Wheel(win32.WheelDelta(deltaY))
}

func (s *SendInputInjector) KeyDown(c Command) error {
	vk, ok := win32.VirtualKey(c.Key)
	if !ok {
		return nil
	}
	if err := s.activate(); err != nil {
		return err
	}
	return s.keys.Send(vk, false)
}

// KeyUp is real input like KeyDown, so it needs no target.
func (s *SendInputInjector) KeyUp(c Command) error {
	vk, ok := win32.VirtualKey(c.Key)
	if !ok {
		return nil
	}
	return s.keys.Send(vk, true)
}

// point moves the real cursor to a normalised point of the target. With check,
// it refuses when the window at that point is not of the target's family.
func (s *SendInputInjector) point(x, y float64, check bool) error {
	hwnd, err := s.handle()
	if err != nil {
		return err
	}
	capture, err := s.desktop.CaptureRect(hwnd)
	if err != nil {
		return err
	}
	desktop, err := s.desktop.VirtualDesktop()
	if err != nil {
		return err
	}
	screenX, screenY, absX, absY, ok := win32.MapToDesktop(capture, desktop, x, y)
	if !ok {
		return win32.ErrWindowUnavailable
	}
	if check && !s.desktop.IsOwnedBy(s.desktop.WindowAt(screenX, screenY), hwnd) {
		return ErrObscured
	}
	return s.desktop.MovePointer(absX, absY)
}

// activate brings the target forward unless its family already has focus.
func (s *SendInputInjector) activate() error {
	hwnd, err := s.handle()
	if err != nil {
		return err
	}
	return s.keys.Activate(hwnd)
}
