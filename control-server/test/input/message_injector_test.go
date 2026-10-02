package input_test

import (
	"errors"
	"slices"
	"testing"

	"share-app-host/internal/input"
	"share-app-host/internal/win32"
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

// recordingKeys is an input.Keys that records how each key was delivered.
type recordingKeys struct {
	foreground bool
	calls      []string
}

func (r *recordingKeys) Post(_ win32.HWND, vk uint16, up bool) error {
	r.calls = append(r.calls, "post "+keyEvent(vk, up))
	return nil
}

func (r *recordingKeys) Send(vk uint16, up bool) error {
	r.calls = append(r.calls, "send "+keyEvent(vk, up))
	return nil
}

func (r *recordingKeys) Foreground(win32.HWND) bool { return r.foreground }

func keyEvent(vk uint16, up bool) string {
	name := map[uint16]string{win32.VKShift: "shift", 0x0D: "enter"}[vk]
	if up {
		return name + " up"
	}
	return name + " down"
}

func pressShiftEnter(t *testing.T, m *input.MessageInjector) {
	t.Helper()
	for _, c := range []struct {
		key string
		up  bool
	}{{"Shift", false}, {"Enter", false}, {"Enter", true}, {"Shift", true}} {
		command := input.Command{Key: c.key}
		var err error
		if c.up {
			err = m.KeyUp(command)
		} else {
			err = m.KeyDown(command)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Applications read Shift from the system key state, which posted messages
// cannot change, so Shift+Enter on a foreground target is real input from the
// Shift press to its release.
func TestShiftEnterIsSentAsRealInputToAForegroundTarget(t *testing.T) {
	keys := &recordingKeys{foreground: true}
	pressShiftEnter(t, input.NewMessageInjectorWithKeys(fakeTarget{handle: 7, ok: true}, keys))
	want := []string{"send shift down", "send enter down", "send enter up", "send shift up"}
	if !slices.Equal(keys.calls, want) {
		t.Fatalf("calls = %v, want %v", keys.calls, want)
	}
}

func TestKeysArePostedWhenTheTargetIsNotInTheForeground(t *testing.T) {
	keys := &recordingKeys{}
	pressShiftEnter(t, input.NewMessageInjectorWithKeys(fakeTarget{handle: 7, ok: true}, keys))
	want := []string{"post shift down", "post enter down", "post enter up", "post shift up"}
	if !slices.Equal(keys.calls, want) {
		t.Fatalf("calls = %v, want %v", keys.calls, want)
	}
}

func TestKeysAreOnlyRealInputWhileShiftIsHeld(t *testing.T) {
	keys := &recordingKeys{foreground: true}
	m := input.NewMessageInjectorWithKeys(fakeTarget{handle: 7, ok: true}, keys)
	if err := m.KeyDown(input.Command{Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	pressShiftEnter(t, m)
	if err := m.KeyUp(input.Command{Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"post enter down",
		"send shift down", "send enter down", "send enter up", "send shift up",
		"post enter up",
	}
	if !slices.Equal(keys.calls, want) {
		t.Fatalf("calls = %v, want %v", keys.calls, want)
	}
}

// switchableTarget is a TargetSource whose selection a test can clear.
type switchableTarget struct{ handle uint64 }

func (s *switchableTarget) CurrentHandle() (uint64, bool) { return s.handle, s.handle != 0 }

func TestRealShiftIsReleasedAfterTheTargetIsGone(t *testing.T) {
	keys := &recordingKeys{foreground: true}
	target := &switchableTarget{handle: 7}
	m := input.NewMessageInjectorWithKeys(target, keys)
	if err := m.KeyDown(input.Command{Key: "Shift"}); err != nil {
		t.Fatal(err)
	}
	target.handle = 0
	if err := m.KeyUp(input.Command{Key: "Shift"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"send shift down", "send shift up"}
	if !slices.Equal(keys.calls, want) {
		t.Fatalf("calls = %v, want %v", keys.calls, want)
	}
}
