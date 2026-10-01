package window_test

import (
	"context"
	"errors"
	"testing"

	"share-app-host/internal/window"
)

type fakeLister struct {
	windows []window.Info
	err     error
}

func (f *fakeLister) ListWindows(context.Context) ([]window.Info, error) { return f.windows, f.err }

func newSelection(windows ...window.Info) (*window.Selection, *fakeLister) {
	lister := &fakeLister{windows: windows}
	return window.NewSelection(lister), lister
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestNothingSelectedInitially(t *testing.T) {
	s, _ := newSelection(window.Info{Handle: 1})
	if _, ok := s.Current(); ok {
		t.Fatal("unexpected current window")
	}
	if h, ok := s.CurrentHandle(); ok || h != 0 {
		t.Fatalf("CurrentHandle() = %d, %v", h, ok)
	}
	if h, ok, ch := s.State(); ok || h != 0 || ch == nil {
		t.Fatalf("State() = %d, %v, %v", h, ok, ch)
	}
}

func TestSelectRequiresAListedWindow(t *testing.T) {
	s, _ := newSelection(window.Info{Handle: 1}, window.Info{Handle: 2})
	if _, err := s.Select(context.Background(), 99); !errors.Is(err, window.ErrNotFound) {
		t.Fatalf("Select(unlisted) error = %v", err)
	}
	if _, ok := s.Current(); ok {
		t.Fatal("failed selection changed the target")
	}
	got, err := s.Select(context.Background(), 2)
	if err != nil || got.Handle != 2 {
		t.Fatalf("Select(2) = %+v, %v", got, err)
	}
	if cur, ok := s.Current(); !ok || cur.Handle != 2 {
		t.Fatalf("Current() = %+v, %v", cur, ok)
	}
	if h, ok := s.CurrentHandle(); !ok || h != 2 {
		t.Fatalf("CurrentHandle() = %d, %v", h, ok)
	}
}

func TestListerFailureLeavesSelectionUntouched(t *testing.T) {
	s, lister := newSelection(window.Info{Handle: 1})
	if _, err := s.Select(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	lister.err = errors.New("probe down")
	if _, err := s.Select(context.Background(), 1); !errors.Is(err, lister.err) {
		t.Fatalf("Select error = %v", err)
	}
	if _, err := s.List(context.Background()); !errors.Is(err, lister.err) {
		t.Fatalf("List error = %v", err)
	}
	if h, ok := s.CurrentHandle(); !ok || h != 1 {
		t.Fatalf("selection lost: %d, %v", h, ok)
	}
}

func TestChangeNotificationFiresOnlyWhenHandleChanges(t *testing.T) {
	s, lister := newSelection(window.Info{Handle: 1, Title: "old"}, window.Info{Handle: 2})
	_, _, first := s.State()

	if _, err := s.Select(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !isClosed(first) {
		t.Fatal("first selection did not notify watchers")
	}

	_, _, second := s.State()
	lister.windows[0].Title = "renamed"
	if _, err := s.Select(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if isClosed(second) {
		t.Fatal("re-selecting the same handle must not notify watchers")
	}
	if cur, _ := s.Current(); cur.Title != "renamed" {
		t.Fatalf("window metadata not refreshed: %+v", cur)
	}

	if _, err := s.Select(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if !isClosed(second) {
		t.Fatal("switching target did not notify watchers")
	}
	if _, _, third := s.State(); isClosed(third) {
		t.Fatal("new notification channel must start open")
	}
}
