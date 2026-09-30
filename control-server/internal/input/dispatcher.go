package input

import (
	"encoding/json"
	"fmt"
	"sync"
)

type Injector interface {
	Move(x, y float64) error
	Tap(button string, x, y float64) error
	MouseDown(button string, x, y float64) error
	MouseUp(button string, x, y float64) error
	Scroll(deltaX, deltaY, x, y float64) error
	ResizeViewport(command Command) error
	KeyDown(command Command) error
	KeyUp(command Command) error
	Text(text string) error
}
type Dispatcher struct {
	injector Injector
	mu       sync.Mutex
	enabled  bool
	buttons  map[string]Command
	keys     map[string]Command
}

func NewDispatcher(injector Injector) *Dispatcher {
	return &Dispatcher{injector: injector, buttons: make(map[string]Command), keys: make(map[string]Command)}
}
func (d *Dispatcher) Activate() { d.mu.Lock(); defer d.mu.Unlock(); d.enabled = true }
func (d *Dispatcher) Dispatch(raw []byte) error {
	if len(raw) > 64*1024 {
		return fmt.Errorf("input message too large")
	}
	var c Command
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.enabled {
		return fmt.Errorf("control connection is closed")
	}
	switch c.Type {
	case "input.tap":
		return d.injector.Tap(c.Button, c.X, c.Y)
	case "input.mouseMove":
		for button, held := range d.buttons {
			held.X = c.X
			held.Y = c.Y
			d.buttons[button] = held
		}
		return d.injector.Move(c.X, c.Y)
	case "input.mouseDown":
		if c.Button != "left" && c.Button != "right" {
			return fmt.Errorf("invalid mouse button")
		}
		if _, held := d.buttons[c.Button]; !held && len(d.buttons) >= 2 {
			return fmt.Errorf("too many held buttons")
		}
		if err := d.injector.MouseDown(c.Button, c.X, c.Y); err != nil {
			return err
		}
		d.buttons[c.Button] = c
		return nil
	case "input.mouseUp":
		if err := d.injector.MouseUp(c.Button, c.X, c.Y); err != nil {
			return err
		}
		delete(d.buttons, c.Button)
		return nil
	case "input.scroll":
		return d.injector.Scroll(c.DeltaX, c.DeltaY, c.X, c.Y)
	case "viewport.resize":
		return d.injector.ResizeViewport(c)
	case "input.keyDown":
		if len(d.keys) >= 32 {
			return fmt.Errorf("too many held keys")
		}
		if err := d.injector.KeyDown(c); err != nil {
			return err
		}
		d.keys[c.Key] = c
		return nil
	case "input.keyUp":
		if err := d.injector.KeyUp(c); err != nil {
			return err
		}
		delete(d.keys, c.Key)
		return nil
	case "input.text":
		if len(c.Text) > 4096 {
			return fmt.Errorf("text command too large")
		}
		return d.injector.Text(c.Text)
	default:
		return fmt.Errorf("unsupported input command: %s", c.Type)
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
func (d *Dispatcher) ReleaseAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.enabled = false
	d.releaseLocked()
}
func (d *Dispatcher) ChangeTarget(change func() error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.releaseLocked()
	return change()
}
