package input_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"share-app-host/internal/input"
)

// recordingInjector records the calls that reach the injector.
type recordingInjector struct {
	events []string
	fail   error
}

func (i *recordingInjector) record(format string, args ...any) error {
	i.events = append(i.events, fmt.Sprintf(format, args...))
	return i.fail
}

func (i *recordingInjector) Move(x, y float64) error { return i.record("move:%v,%v", x, y) }
func (i *recordingInjector) Tap(b string, x, y float64) error {
	return i.record("tap:%s:%v,%v", b, x, y)
}
func (i *recordingInjector) MouseDown(b string, x, y float64) error {
	return i.record("down:%s:%v,%v", b, x, y)
}
func (i *recordingInjector) MouseUp(b string, x, y float64) error {
	return i.record("up:%s:%v,%v", b, x, y)
}
func (i *recordingInjector) Scroll(dx, dy, x, y float64) error {
	return i.record("scroll:%v,%v@%v,%v", dx, dy, x, y)
}
func (i *recordingInjector) ResizeViewport(c input.Command) error {
	return i.record("resize:%dx%d", c.Width, c.Height)
}
func (i *recordingInjector) KeyDown(c input.Command) error { return i.record("down:%s", c.Key) }
func (i *recordingInjector) KeyUp(c input.Command) error   { return i.record("up:%s", c.Key) }
func (i *recordingInjector) Text(s string) error           { return i.record("text:%s", s) }

func activeDispatcher() (*input.Dispatcher, *recordingInjector) {
	injector := &recordingInjector{}
	d := input.NewDispatcher(injector)
	d.Activate()
	return d, injector
}

func mustDispatch(t *testing.T, d *input.Dispatcher, command string) {
	t.Helper()
	if err := d.Dispatch([]byte(command)); err != nil {
		t.Fatalf("Dispatch(%s): %v", command, err)
	}
}

func TestTargetChangeReleasesOldInputAndDisconnectRejectsLateCommands(t *testing.T) {
	d, i := activeDispatcher()
	for _, command := range []string{`{"type":"input.mouseDown","button":"right","x":0.5}`, `{"type":"input.keyDown","key":"Enter"}`} {
		mustDispatch(t, d, command)
	}
	if err := d.ChangeTarget(func() error { i.events = append(i.events, "target changed"); return nil }); err != nil {
		t.Fatal(err)
	}
	want := []string{"down:right:0.5,0", "down:Enter", "up:Enter", "up:right:0.5,0", "target changed"}
	if !reflect.DeepEqual(i.events, want) {
		t.Fatalf("events = %v, want %v", i.events, want)
	}
	d.ReleaseAll()
	if err := d.Dispatch([]byte(`{"type":"input.mouseDown","button":"right"}`)); err == nil {
		t.Fatal("late input accepted")
	}
	d.Activate()
	mustDispatch(t, d, `{"type":"input.keyDown","key":"Enter"}`)
	d.ReleaseAll()
	d.ReleaseAll()
	if got := i.events[len(i.events)-1]; got != "up:Enter" {
		t.Fatal(i.events)
	}
}

func TestDispatchRejectsInputUntilActivated(t *testing.T) {
	i := &recordingInjector{}
	d := input.NewDispatcher(i)
	if err := d.Dispatch([]byte(`{"type":"input.tap"}`)); err == nil {
		t.Fatal("command accepted before Activate")
	}
	if len(i.events) != 0 {
		t.Fatalf("injector reached: %v", i.events)
	}
}

func TestDispatchRoutesCommandsToTheInjector(t *testing.T) {
	d, i := activeDispatcher()
	for _, command := range []string{
		`{"type":"input.tap","button":"left","x":0.25,"y":0.75}`,
		`{"type":"input.mouseMove","x":0.5,"y":0.5}`,
		`{"type":"input.scroll","deltaX":1,"deltaY":2,"x":0.1,"y":0.2}`,
		`{"type":"viewport.resize","width":390,"height":844}`,
		`{"type":"input.text","text":"hello"}`,
	} {
		mustDispatch(t, d, command)
	}
	want := []string{
		"tap:left:0.25,0.75",
		"move:0.5,0.5",
		"scroll:1,2@0.1,0.2",
		"resize:390x844",
		"text:hello",
	}
	if !reflect.DeepEqual(i.events, want) {
		t.Fatalf("events = %v, want %v", i.events, want)
	}
}

func TestHeldButtonsAreReleasedWhereThePointerLastMoved(t *testing.T) {
	d, i := activeDispatcher()
	mustDispatch(t, d, `{"type":"input.mouseDown","button":"left","x":0.1,"y":0.1}`)
	mustDispatch(t, d, `{"type":"input.mouseMove","x":0.8,"y":0.9}`)
	d.ReleaseAll()
	if got := i.events[len(i.events)-1]; got != "up:left:0.8,0.9" {
		t.Fatalf("release = %q", got)
	}
}

func TestDispatchValidation(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"malformed JSON", `{"type":`},
		{"unknown command", `{"type":"input.explode"}`},
		{"missing type", `{}`},
		{"invalid mouse button", `{"type":"input.mouseDown","button":"middle"}`},
		{"empty mouse button", `{"type":"input.mouseDown"}`},
		{"oversized text", `{"type":"input.text","text":"` + strings.Repeat("a", 4097) + `"}`},
		{"oversized message", `{"type":"input.text","text":"` + strings.Repeat("a", input.MaxMessageBytes) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, i := activeDispatcher()
			if err := d.Dispatch([]byte(tt.command)); err == nil {
				t.Fatal("command accepted")
			}
			if len(i.events) != 0 {
				t.Fatalf("injector reached: %v", i.events)
			}
		})
	}
}

func TestTextAtTheLimitIsAccepted(t *testing.T) {
	d, _ := activeDispatcher()
	mustDispatch(t, d, `{"type":"input.text","text":"`+strings.Repeat("a", 4096)+`"}`)
}

func TestHeldInputIsBounded(t *testing.T) {
	t.Run("buttons", func(t *testing.T) {
		d, _ := activeDispatcher()
		mustDispatch(t, d, `{"type":"input.mouseDown","button":"left"}`)
		mustDispatch(t, d, `{"type":"input.mouseDown","button":"right"}`)
		// Pressing an already-held button again is not a new hold.
		mustDispatch(t, d, `{"type":"input.mouseDown","button":"left"}`)
	})
	t.Run("keys", func(t *testing.T) {
		d, _ := activeDispatcher()
		for n := 0; n < 32; n++ {
			mustDispatch(t, d, fmt.Sprintf(`{"type":"input.keyDown","key":"k%d"}`, n))
		}
		if err := d.Dispatch([]byte(`{"type":"input.keyDown","key":"overflow"}`)); err == nil {
			t.Fatal("33rd held key accepted")
		}
		mustDispatch(t, d, `{"type":"input.keyUp","key":"k0"}`)
		mustDispatch(t, d, `{"type":"input.keyDown","key":"overflow"}`)
	})
}

func TestFailedInjectionIsNotRecordedAsHeld(t *testing.T) {
	d, i := activeDispatcher()
	i.fail = errors.New("target gone")
	if err := d.Dispatch([]byte(`{"type":"input.keyDown","key":"Enter"}`)); err == nil {
		t.Fatal("injector failure was swallowed")
	}
	if err := d.Dispatch([]byte(`{"type":"input.mouseDown","button":"left"}`)); err == nil {
		t.Fatal("injector failure was swallowed")
	}
	i.events = nil
	i.fail = nil
	d.ReleaseAll()
	if len(i.events) != 0 {
		t.Fatalf("released input that was never pressed: %v", i.events)
	}
}
