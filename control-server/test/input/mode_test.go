package input_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"share-app-host/internal/input"
	"share-app-host/test/testutil"
)

func modeDispatch(t *testing.T, d *input.Dispatcher, c input.Command) error {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return d.Dispatch(raw)
}

func TestModeSwitchReleasesOldInjectorAndWaitsForCapture(t *testing.T) {
	old, pc := &testutil.RecordingInjector{}, &testutil.RecordingInjector{}
	d := input.NewDispatcher(old)
	d.SetPCInjector(pc)
	d.SetModeHandler(func(mode string, changed bool) {
		if mode != input.ModePC || !changed {
			t.Errorf("mode callback %s %v", mode, changed)
		}
		if want := []string{"down:Enter", "down:left:0.5,0.5", "up:Enter", "up:left:0.5,0.5"}; !slices.Equal(old.Events(), want) {
			t.Errorf("old events before callback = %v", old.Events())
		}
	})
	d.Activate()
	for _, c := range []input.Command{{Type: input.TypeKeyDown, Key: "Enter"}, {Type: input.TypeMouseDown, Button: "left", X: .5, Y: .5}, {Type: input.TypeMode, Mode: input.ModePC}} {
		if err := modeDispatch(t, d, c); err != nil {
			t.Fatal(err)
		}
	}
	if err := modeDispatch(t, d, input.Command{Type: input.TypeText, Text: "blocked"}); err == nil {
		t.Fatal("input accepted during switch")
	}
	d.CompleteMode()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeText, Text: "pc"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pc.Events(), []string{"text:pc"}) {
		t.Fatal(pc.Events())
	}
	d.ReleaseAll()
	d.Activate()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeText, Text: "window"}); err != nil {
		t.Fatal(err)
	}
	if old.Events()[len(old.Events())-1] != "text:window" || len(pc.Events()) != 1 {
		t.Fatal("Activate did not reset to Window")
	}
}

func TestSameModeDoesNotRestartAndInvalidModeDoesNotRelease(t *testing.T) {
	old := &testutil.RecordingInjector{}
	d := input.NewDispatcher(old)
	d.Activate()
	d.SetModeHandler(func(mode string, changed bool) {
		if mode != input.ModeWindow || changed {
			t.Fatal("same-mode requested restart")
		}
	})
	if err := modeDispatch(t, d, input.Command{Type: input.TypeKeyDown, Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := modeDispatch(t, d, input.Command{Type: input.TypeMode, Mode: "other"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
	if err := modeDispatch(t, d, input.Command{Type: input.TypeMode, Mode: input.ModeWindow}); err != nil {
		t.Fatal(err)
	}
	if len(old.Events()) != 1 {
		t.Fatal("same/invalid mode released input")
	}
	d.ReleaseAll()
}

func TestReleaseFailureIsFatalAndDoesNotSwitchInjector(t *testing.T) {
	old, pc := &testutil.RecordingInjector{}, &testutil.RecordingInjector{}
	d := input.NewDispatcher(old)
	d.SetPCInjector(pc)
	d.SetModeHandler(func(string, bool) { t.Fatal("failed release restarted capture") })
	d.Activate()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeKeyDown, Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	old.Fail = errors.New("release failed")
	if err := modeDispatch(t, d, input.Command{Type: input.TypeMode, Mode: input.ModePC}); !errors.Is(err, input.ErrModeSwitch) {
		t.Fatalf("mode error = %v", err)
	}
	if len(pc.Events()) != 0 {
		t.Fatal("switched injector after failed release")
	}
	if err := modeDispatch(t, d, input.Command{Type: input.TypeText, Text: "late"}); err == nil {
		t.Fatal("fatal release failure left input enabled")
	}
	old.Fail = nil
	d.ReleaseAll()
	if want := []string{"down:Enter", "up:Enter", "up:Enter"}; !slices.Equal(old.Events(), want) {
		t.Fatalf("disconnect did not retry failed release: %v", old.Events())
	}
}

func TestPCUnheldKeyReleasesDoNotReachHost(t *testing.T) {
	old, pc := &testutil.RecordingInjector{}, &testutil.RecordingInjector{}
	d := input.NewDispatcher(old)
	d.SetPCInjector(pc)
	d.SetModeHandler(func(string, bool) {})
	d.Activate()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeKeyDown, Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := modeDispatch(t, d, input.Command{Type: input.TypeMode, Mode: input.ModePC}); err != nil {
		t.Fatal(err)
	}
	d.CompleteMode()
	pc.Fail = errors.New("foreground denied")
	if err := modeDispatch(t, d, input.Command{Type: input.TypeKeyDown, Key: "Shift"}); err == nil {
		t.Fatal("press failure lost")
	}
	pc.Fail = nil
	pc.Reset()
	for _, key := range []string{"Shift", "Enter"} {
		if err := modeDispatch(t, d, input.Command{Type: input.TypeKeyUp, Key: key}); err != nil {
			t.Fatal(err)
		}
	}
	if len(pc.Events()) != 0 {
		t.Fatalf("released rejected or old-mode keys on host: %v", pc.Events())
	}
}

func TestFailedReleaseDoesNotBlockLaterConnectionsOrTargetChanges(t *testing.T) {
	old := &testutil.RecordingInjector{}
	d := input.NewDispatcher(old)
	d.Activate()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeKeyDown, Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	old.Fail = errors.New("target window closed")
	d.ReleaseAll()
	old.Fail = nil
	changed := false
	if err := d.ChangeTarget(func() error { changed = true; return nil }); err != nil || !changed {
		t.Fatalf("target change blocked: %v", err)
	}
	d.Activate()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeText, Text: "new connection"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"down:Enter", "up:Enter", "text:new connection"}; !slices.Equal(old.Events(), want) {
		t.Fatalf("events = %v, want %v", old.Events(), want)
	}
}
