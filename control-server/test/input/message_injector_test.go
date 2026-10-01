package input_test

import (
	"errors"
	"testing"

	"share-app-host/internal/input"
	"share-app-host/internal/window"
)

type fakeTarget struct {
	handle uint64
	ok     bool
}

func (f fakeTarget) CurrentHandle() (uint64, bool) { return f.handle, f.ok }

func TestMessageInjectorRequiresATargetWindow(t *testing.T) {
	targets := map[string]input.TargetSource{
		"nil target":         nil,
		"nothing chosen":     fakeTarget{},
		"null handle chosen": fakeTarget{handle: 0, ok: true},
	}
	for name, target := range targets {
		t.Run(name, func(t *testing.T) {
			m := input.NewMessageInjector(target)
			calls := map[string]func() error{
				"Move":                func() error { return m.Move(0.5, 0.5) },
				"Tap":                 func() error { return m.Tap("left", 0.5, 0.5) },
				"MouseDown":           func() error { return m.MouseDown("left", 0.5, 0.5) },
				"MouseUp":             func() error { return m.MouseUp("left", 0.5, 0.5) },
				"Scroll":              func() error { return m.Scroll(1, 0.5, 0.5) },
				"Text":                func() error { return m.Text("hello") },
				"KeyDown special key": func() error { return m.KeyDown(input.Command{Key: "Enter"}) },
				"KeyUp special key":   func() error { return m.KeyUp(input.Command{Key: "Enter"}) },
				"ResizeViewport":      func() error { return m.ResizeViewport(input.Command{Width: 390, Height: 844}) },
			}
			for call, do := range calls {
				if err := do(); !errors.Is(err, window.ErrNotSelected) {
					t.Errorf("%s: error = %v, want window.ErrNotSelected", call, err)
				}
			}
		})
	}
}

func TestMessageInjectorIgnoresInputItCannotDeliver(t *testing.T) {
	m := input.NewMessageInjector(fakeTarget{})
	// Printable characters arrive as text, so a bare key press is dropped
	// rather than reported as an error.
	if err := m.KeyDown(input.Command{Key: "a"}); err != nil {
		t.Errorf("KeyDown(printable) = %v", err)
	}
	if err := m.KeyUp(input.Command{Key: "a"}); err != nil {
		t.Errorf("KeyUp(printable) = %v", err)
	}
	// A viewport with no area has nothing to resize to.
	for _, c := range []input.Command{{}, {Width: 100}, {Height: 100}} {
		if err := m.ResizeViewport(c); err != nil {
			t.Errorf("ResizeViewport(%+v) = %v", c, err)
		}
	}
}
