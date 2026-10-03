package input_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"share-app-host/internal/input"
	"share-app-host/test/testutil"
)

func activeDispatcher() (*input.Dispatcher, *testutil.RecordingInjector) {
	injector := &testutil.RecordingInjector{}
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
	if err := d.ChangeTarget(func() error { i.Record("target changed"); return nil }); err != nil {
		t.Fatal(err)
	}
	want := []string{"down:right:0.5,0", "down:Enter", "up:Enter", "up:right:0.5,0", "target changed"}
	if !reflect.DeepEqual(i.Events(), want) {
		t.Fatalf("events = %v, want %v", i.Events(), want)
	}
	d.ReleaseAll()
	if err := d.Dispatch([]byte(`{"type":"input.mouseDown","button":"right"}`)); err == nil {
		t.Fatal("late input accepted")
	}
	d.Activate()
	mustDispatch(t, d, `{"type":"input.keyDown","key":"Enter"}`)
	d.ReleaseAll()
	d.ReleaseAll()
	if got := i.Events()[len(i.Events())-1]; got != "up:Enter" {
		t.Fatal(i.Events())
	}
}

func TestDispatchRejectsInputUntilActivated(t *testing.T) {
	i := &testutil.RecordingInjector{}
	d := input.NewDispatcher(i)
	if err := d.Dispatch([]byte(`{"type":"input.tap"}`)); err == nil {
		t.Fatal("command accepted before Activate")
	}
	if len(i.Events()) != 0 {
		t.Fatalf("injector reached: %v", i.Events())
	}
}

func TestDispatchRoutesCommandsToTheInjector(t *testing.T) {
	d, i := activeDispatcher()
	for _, command := range []string{
		`{"type":"input.tap","button":"left","x":0.25,"y":0.75}`,
		`{"type":"input.mouseMove","x":0.5,"y":0.5}`,
		`{"type":"input.scroll","deltaY":2,"x":0.1,"y":0.2}`,
		`{"type":"viewport.resize","width":390,"height":844}`,
		`{"type":"input.text","text":"hello"}`,
	} {
		mustDispatch(t, d, command)
	}
	want := []string{
		"tap:left:0.25,0.75",
		"move:0.5,0.5",
		"scroll:2@0.1,0.2",
		"resize:390x844",
		"text:hello",
	}
	if !reflect.DeepEqual(i.Events(), want) {
		t.Fatalf("events = %v, want %v", i.Events(), want)
	}
}

func TestHeldButtonsAreReleasedWhereThePointerLastMoved(t *testing.T) {
	d, i := activeDispatcher()
	mustDispatch(t, d, `{"type":"input.mouseDown","button":"left","x":0.1,"y":0.1}`)
	mustDispatch(t, d, `{"type":"input.mouseMove","x":0.8,"y":0.9}`)
	d.ReleaseAll()
	if got := i.Events()[len(i.Events())-1]; got != "up:left:0.8,0.9" {
		t.Fatalf("release = %q", got)
	}
}

func TestDispatchValidation(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"malformed JSON", `{"type":`},
		{"trailing command", `{"type":"input.keyDown","key":"Enter"} {}`},
		{"trailing garbage", `{"type":"input.keyDown","key":"Enter"} broken`},
		{"removed keyboard metadata", `{"type":"input.keyDown","key":"Enter","code":"Enter","ctrlKey":true,"shiftKey":false}`},
		{"unknown field", `{"type":"input.keyDown","key":"Enter","futureField":{"a":1}}`},
		{"unknown command", `{"type":"input.explode"}`},
		{"missing type", `{}`},
		{"invalid mouse button", `{"type":"input.mouseDown","button":"middle"}`},
		{"empty mouse button", `{"type":"input.mouseDown"}`},
		{"invalid tap button", `{"type":"input.tap","button":"middle"}`},
		{"empty tap button", `{"type":"input.tap"}`},
		{"invalid release button", `{"type":"input.mouseUp","button":"middle"}`},
		{"oversized text", `{"type":"input.text","text":"` + strings.Repeat("a", 16*1024+1) + `"}`},
		{"oversized message", `{"type":"input.text","text":"` + strings.Repeat("a", input.MaxMessageBytes) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, i := activeDispatcher()
			if err := d.Dispatch([]byte(tt.command)); err == nil {
				t.Fatal("command accepted")
			}
			if len(i.Events()) != 0 {
				t.Fatalf("injector reached: %v", i.Events())
			}
		})
	}
}

func TestTextAtTheLimitIsAccepted(t *testing.T) {
	d, _ := activeDispatcher()
	mustDispatch(t, d, `{"type":"input.text","text":"`+strings.Repeat("a", 16*1024)+`"}`)
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
	i.Fail = errors.New("target gone")
	if err := d.Dispatch([]byte(`{"type":"input.keyDown","key":"Enter"}`)); err == nil {
		t.Fatal("injector failure was swallowed")
	}
	if err := d.Dispatch([]byte(`{"type":"input.mouseDown","button":"left"}`)); err == nil {
		t.Fatal("injector failure was swallowed")
	}
	i.Reset()
	i.Fail = nil
	d.ReleaseAll()
	if len(i.Events()) != 0 {
		t.Fatalf("released input that was never pressed: %v", i.Events())
	}
}
