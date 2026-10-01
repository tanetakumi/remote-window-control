package app_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"share-app-host/internal/app"
	"share-app-host/internal/input"
	"share-app-host/internal/window"
	"share-app-host/test/testutil"
)

// fakeLister lists a fixed set of windows. When block is non-nil, listing
// waits for it to be closed, like a slow window enumeration.
type fakeLister struct {
	windows []window.Info
	err     error
	block   chan struct{}
	started chan struct{}
	once    sync.Once
}

func (f *fakeLister) ListWindows(ctx context.Context) ([]window.Info, error) {
	if f.started != nil {
		f.once.Do(func() { close(f.started) })
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.windows, f.err
}

type fixture struct {
	service    *app.TargetService
	selection  *window.Selection
	dispatcher *input.Dispatcher
	injector   *testutil.RecordingInjector
	lister     *fakeLister
}

func newFixture(windows ...window.Info) *fixture {
	lister := &fakeLister{windows: windows}
	selection := window.NewSelection(lister)
	injector := &testutil.RecordingInjector{}
	dispatcher := input.NewDispatcher(injector)
	dispatcher.Activate()
	return &fixture{
		service:    app.NewTargetService(selection, dispatcher),
		selection:  selection,
		dispatcher: dispatcher,
		injector:   injector,
		lister:     lister,
	}
}

func (f *fixture) hold(t *testing.T) {
	t.Helper()
	for _, command := range []string{
		`{"type":"input.mouseDown","button":"left","x":0.5,"y":0.25}`,
		`{"type":"input.keyDown","key":"Enter"}`,
	} {
		if err := f.dispatcher.Dispatch([]byte(command)); err != nil {
			t.Fatal(err)
		}
	}
	f.injector.Reset()
}

func TestSelectReleasesHeldInputBeforeSwitching(t *testing.T) {
	f := newFixture(window.Info{Handle: 1}, window.Info{Handle: 2})
	if _, err := f.service.Select(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	f.hold(t)

	got, err := f.service.Select(context.Background(), 2)
	if err != nil || got.Handle != 2 {
		t.Fatalf("Select = %+v, %v", got, err)
	}
	// Keys are released before buttons, on the window they were pressed in.
	if want := []string{"up:Enter", "up:left:0.5,0.25"}; !reflect.DeepEqual(f.injector.Events(), want) {
		t.Fatalf("events = %v, want %v", f.injector.Events(), want)
	}
	if current, ok := f.service.Current(); !ok || current.Handle != 2 {
		t.Fatalf("Current = %+v, %v", current, ok)
	}
}

func TestFailedSelectionLeavesTheTargetAndHeldInputAlone(t *testing.T) {
	f := newFixture(window.Info{Handle: 1})
	if _, err := f.service.Select(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	f.hold(t)

	if _, err := f.service.Select(context.Background(), 99); !errors.Is(err, window.ErrNotFound) {
		t.Fatalf("Select(unlisted) error = %v", err)
	}
	f.lister.err = errors.New("probe down")
	if _, err := f.service.Select(context.Background(), 1); !errors.Is(err, f.lister.err) {
		t.Fatalf("Select with a failing lister: %v", err)
	}

	if events := f.injector.Events(); len(events) != 0 {
		t.Fatalf("held input was released although the target did not change: %v", events)
	}
	if current, _ := f.service.Current(); current.Handle != 1 {
		t.Fatalf("target changed to %+v", current)
	}
}

func TestListAndCurrentPassThrough(t *testing.T) {
	f := newFixture(window.Info{Handle: 1, Title: "A"})
	if _, ok := f.service.Current(); ok {
		t.Fatal("window selected initially")
	}
	list, err := f.service.List(context.Background())
	if err != nil || len(list) != 1 || list[0].Title != "A" {
		t.Fatalf("List = %+v, %v", list, err)
	}
}

// Enumerating windows is slow; input to the current target must keep flowing
// while a selection is being resolved.
func TestSlowWindowEnumerationDoesNotBlockInput(t *testing.T) {
	f := newFixture(window.Info{Handle: 1}, window.Info{Handle: 2})
	if _, err := f.service.Select(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	f.lister.block = make(chan struct{})
	f.lister.started = make(chan struct{})

	selected := make(chan error, 1)
	go func() {
		_, err := f.service.Select(context.Background(), 2)
		selected <- err
	}()
	select {
	case <-f.lister.started:
	case <-time.After(3 * time.Second):
		t.Fatal("enumeration never started")
	}

	dispatched := make(chan error, 1)
	go func() { dispatched <- f.dispatcher.Dispatch([]byte(`{"type":"input.text","text":"typing"}`)) }()
	select {
	case err := <-dispatched:
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("input was blocked while windows were being enumerated")
	}

	close(f.lister.block)
	select {
	case err := <-selected:
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Select did not finish")
	}
}
