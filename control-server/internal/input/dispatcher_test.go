package input

import (
	"reflect"
	"testing"
)

type recordingInjector struct{ events []string }

func (i *recordingInjector) Move(float64, float64) error        { return nil }
func (i *recordingInjector) Tap(string, float64, float64) error { return nil }
func (i *recordingInjector) MouseDown(b string, x, y float64) error {
	i.events = append(i.events, "down:"+b)
	return nil
}
func (i *recordingInjector) MouseUp(b string, x, y float64) error {
	i.events = append(i.events, "up:"+b)
	return nil
}
func (i *recordingInjector) Scroll(float64, float64, float64, float64) error { return nil }
func (i *recordingInjector) ResizeViewport(Command) error                    { return nil }
func (i *recordingInjector) KeyDown(c Command) error {
	i.events = append(i.events, "down:"+c.Key)
	return nil
}
func (i *recordingInjector) KeyUp(c Command) error {
	i.events = append(i.events, "up:"+c.Key)
	return nil
}
func (i *recordingInjector) Text(string) error { return nil }
func TestTargetChangeReleasesOldInputAndDisconnectRejectsLateCommands(t *testing.T) {
	i := &recordingInjector{}
	d := NewDispatcher(i)
	d.Activate()
	for _, command := range []string{`{"type":"input.mouseDown","button":"right","x":0.5}`, `{"type":"input.keyDown","key":"Enter"}`} {
		if err := d.Dispatch([]byte(command)); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.ChangeTarget(func() error { i.events = append(i.events, "target changed"); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(i.events, []string{"down:right", "down:Enter", "up:Enter", "up:right", "target changed"}) {
		t.Fatal(i.events)
	}
	d.ReleaseAll()
	if err := d.Dispatch([]byte(`{"type":"input.mouseDown","button":"right"}`)); err == nil {
		t.Fatal("late input accepted")
	}
	d.Activate()
	if err := d.Dispatch([]byte(`{"type":"input.keyDown","key":"Enter"}`)); err != nil {
		t.Fatal(err)
	}
	d.ReleaseAll()
	d.ReleaseAll()
	if got := i.events[len(i.events)-1]; got != "up:Enter" {
		t.Fatal(i.events)
	}
}
