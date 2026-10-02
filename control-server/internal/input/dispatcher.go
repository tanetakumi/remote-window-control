package input

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

const (
	buttonLeft  = "left"
	buttonRight = "right"

	maxHeldButtons = 2
	maxHeldKeys    = 32
	maxTextBytes   = 16 * 1024
)

var (
	errMessageTooLarge = errors.New("input message too large")
	errTextTooLarge    = errors.New("text command too large")
	errClosed          = errors.New("control connection is closed")
	errInvalidButton   = errors.New("invalid mouse button")
	errTooManyButtons  = errors.New("too many held buttons")
	errTooManyKeys     = errors.New("too many held keys")
)

// Dispatcher validates input commands and forwards them to an Injector. It
// remembers which keys and buttons are held so they can be released when the
// controlling connection ends or the target window changes, and it rejects
// commands outside an Activate/ReleaseAll window.
type Dispatcher struct {
	injector Injector

	mu      sync.Mutex
	enabled bool
	buttons map[string]Command
	keys    map[string]Command
}

// NewDispatcher returns a disabled Dispatcher; call Activate to accept input.
func NewDispatcher(injector Injector) *Dispatcher {
	return &Dispatcher{
		injector: injector,
		buttons:  make(map[string]Command),
		keys:     make(map[string]Command),
	}
}

// Activate starts accepting commands.
func (d *Dispatcher) Activate() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.enabled = true
}

// ReleaseAll releases everything still held and stops accepting commands, so a
// late message from a closed connection cannot act on the target window.
func (d *Dispatcher) ReleaseAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.enabled = false
	d.releaseLocked()
}

// ChangeTarget releases held input on the old target, then runs change, which
// is expected to switch the target. Input stays blocked while it runs.
func (d *Dispatcher) ChangeTarget(change func() error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.releaseLocked()
	return change()
}

// Dispatch decodes one JSON command and applies it.
func (d *Dispatcher) Dispatch(raw []byte) error {
	if len(raw) > MaxMessageBytes {
		return errMessageTooLarge
	}
	var c Command
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.enabled {
		return errClosed
	}

	// Every button command is checked here, so none falls back to a default.
	switch c.Type {
	case TypeTap, TypeMouseDown, TypeMouseUp:
		if c.Button != buttonLeft && c.Button != buttonRight {
			return errInvalidButton
		}
	}

	switch c.Type {
	case TypeTap:
		return d.injector.Tap(c.Button, c.X, c.Y)
	case TypeMouseMove:
		d.moveHeldButtons(c.X, c.Y)
		return d.injector.Move(c.X, c.Y)
	case TypeMouseDown:
		return d.mouseDown(c)
	case TypeMouseUp:
		if err := d.injector.MouseUp(c.Button, c.X, c.Y); err != nil {
			return err
		}
		delete(d.buttons, c.Button)
		return nil
	case TypeScroll:
		return d.injector.Scroll(c.DeltaY, c.X, c.Y)
	case TypeViewportResize:
		return d.injector.ResizeViewport(c)
	case TypeKeyDown:
		return d.keyDown(c)
	case TypeKeyUp:
		if err := d.injector.KeyUp(c); err != nil {
			return err
		}
		delete(d.keys, c.Key)
		return nil
	case TypeText:
		if len(c.Text) > maxTextBytes {
			return errTextTooLarge
		}
		return d.injector.Text(c.Text)
	default:
		return fmt.Errorf("unsupported input command: %s", c.Type)
	}
}

func (d *Dispatcher) mouseDown(c Command) error {
	if _, held := d.buttons[c.Button]; !held && len(d.buttons) >= maxHeldButtons {
		return errTooManyButtons
	}
	if err := d.injector.MouseDown(c.Button, c.X, c.Y); err != nil {
		return err
	}
	d.buttons[c.Button] = c
	return nil
}

func (d *Dispatcher) keyDown(c Command) error {
	if len(d.keys) >= maxHeldKeys {
		return errTooManyKeys
	}
	if err := d.injector.KeyDown(c); err != nil {
		return err
	}
	d.keys[c.Key] = c
	return nil
}

// moveHeldButtons records the latest pointer position for held buttons, so a
// forced release happens where the pointer actually is.
func (d *Dispatcher) moveHeldButtons(x, y float64) {
	for button, held := range d.buttons {
		held.X, held.Y = x, y
		d.buttons[button] = held
	}
}

func (d *Dispatcher) releaseLocked() {
	for key, c := range d.keys {
		_ = d.injector.KeyUp(c)
		delete(d.keys, key)
	}
	for button, c := range d.buttons {
		_ = d.injector.MouseUp(button, c.X, c.Y)
		delete(d.buttons, button)
	}
}
