package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	errSwitching       = errors.New("input mode is switching")
	errNoModeHandler   = errors.New("input mode switching is not configured")

	// ErrModeSwitch is fatal to the connection: held input could not be
	// released before switching injectors.
	ErrModeSwitch = errors.New("input mode switch failed")
)

// Dispatcher validates input commands and forwards them to an Injector. It
// remembers which keys and buttons are held so they can be released when the
// controlling connection ends or the target window changes, and it rejects
// commands outside an Activate/ReleaseAll window. In PC mode it forwards to a
// second Injector; each connection starts in Window mode.
type Dispatcher struct {
	windowInjector Injector

	mu         sync.Mutex
	enabled    bool
	injector   Injector // windowInjector or pcInjector
	pcInjector Injector
	mode       string
	switching  bool // input waits until CompleteMode
	onMode     func(mode string, changed bool)
	buttons    map[string]Command
	keys       map[string]Command
}

// NewDispatcher returns a disabled Dispatcher; call Activate to accept input.
func NewDispatcher(injector Injector) *Dispatcher {
	return &Dispatcher{
		injector:       injector,
		windowInjector: injector,
		mode:           ModeWindow,
		buttons:        make(map[string]Command),
		keys:           make(map[string]Command),
	}
}

// Activate starts accepting commands in Window mode.
func (d *Dispatcher) Activate() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.injector = d.windowInjector
	d.mode = ModeWindow
	d.switching = false
	d.enabled = true
}

// ReleaseAll releases everything still held and stops accepting commands, so a
// late message from a closed connection cannot act on the target window.
func (d *Dispatcher) ReleaseAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.enabled = false
	_ = d.releaseLocked()
	clear(d.keys)
	clear(d.buttons)
}

// ChangeTarget releases held input on the old target, then runs change, which
// is expected to switch the target. Input stays blocked while it runs.
func (d *Dispatcher) ChangeTarget(change func() error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_ = d.releaseLocked()
	clear(d.keys)
	clear(d.buttons)
	return change()
}

// Dispatch decodes one JSON command and applies it.
func (d *Dispatcher) Dispatch(raw []byte) error {
	if len(raw) > MaxMessageBytes {
		return errMessageTooLarge
	}
	var c Command
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected data after input command")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.enabled {
		return errClosed
	}

	if d.switching {
		return errSwitching
	}

	// Every button command is checked here, so none falls back to a default.
	switch c.Type {
	case TypeTap, TypeMouseDown, TypeMouseUp:
		if c.Button != buttonLeft && c.Button != buttonRight {
			return errInvalidButton
		}
	}

	switch c.Type {
	case TypeMode:
		if c.Mode != ModeWindow && c.Mode != ModePC {
			return fmt.Errorf("invalid input mode: %s", c.Mode)
		}
		if d.onMode == nil || (c.Mode == ModePC && d.pcInjector == nil) {
			return errNoModeHandler
		}
		changed := c.Mode != d.mode
		if changed {
			if err := d.releaseLocked(); err != nil {
				d.enabled = false
				return fmt.Errorf("%w: release held input: %v", ErrModeSwitch, err)
			}
			d.injector = d.windowInjector
			if c.Mode == ModePC {
				d.injector = d.pcInjector
			}
			d.mode = c.Mode
			d.switching = true
		}
		d.onMode(c.Mode, changed)
		return nil
	case TypeTap:
		err := d.injector.Tap(c.Button, c.X, c.Y)
		if err != nil {
			// A tap may fail after pressing, while releasing its button.
			// Keep it for a later release; PC input ignores an unowned Up.
			d.buttons[c.Button] = c
		} else {
			delete(d.buttons, c.Button)
		}
		return err
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
		if _, held := d.keys[c.Key]; !held {
			return nil
		}
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

// releaseLocked retains failed releases so a fatal mode switch can retry them
// during connection cleanup. ReleaseAll and ChangeTarget then forget them, so
// an unavailable window cannot block later connections or target changes.
func (d *Dispatcher) releaseLocked() error {
	var err error
	for key, c := range d.keys {
		if releaseErr := d.injector.KeyUp(c); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		} else {
			delete(d.keys, key)
		}
	}
	for button, c := range d.buttons {
		if releaseErr := d.injector.MouseUp(button, c.X, c.Y); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		} else {
			delete(d.buttons, button)
		}
	}
	return err
}

// SetPCInjector sets the Injector used in PC mode.
func (d *Dispatcher) SetPCInjector(injector Injector) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pcInjector = injector
}

// SetModeHandler registers a nonblocking connection-scoped callback. changed
// is false for requests for the current mode; those need no capture restart.
func (d *Dispatcher) SetModeHandler(handler func(string, bool)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onMode = handler
}

// CompleteMode accepts input again after a mode switch.
func (d *Dispatcher) CompleteMode() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.switching = false
}
