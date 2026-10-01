package window

import (
	"context"
	"sync"
)

// Lister enumerates the windows that can currently be selected.
type Lister interface {
	ListWindows(ctx context.Context) ([]Info, error)
}

// Selection holds the single shared target window and notifies watchers when
// the selected window changes.
type Selection struct {
	lister Lister

	mu      sync.RWMutex
	current Info
	has     bool
	changed chan struct{}
}

// NewSelection returns an empty Selection that validates choices against lister.
func NewSelection(lister Lister) *Selection {
	return &Selection{lister: lister, changed: make(chan struct{})}
}

// List returns the windows that can be selected.
func (s *Selection) List(ctx context.Context) ([]Info, error) {
	return s.lister.ListWindows(ctx)
}

// Select makes the listed window with the given handle the target. Watchers
// are notified only when the handle differs from the previous selection.
func (s *Selection) Select(ctx context.Context, handle uint64) (Info, error) {
	windows, err := s.lister.ListWindows(ctx)
	if err != nil {
		return Info{}, err
	}
	for _, w := range windows {
		if w.Handle != handle {
			continue
		}
		s.mu.Lock()
		if !s.has || s.current.Handle != w.Handle {
			close(s.changed)
			s.changed = make(chan struct{})
		}
		s.current = w
		s.has = true
		s.mu.Unlock()
		return w, nil
	}
	return Info{}, ErrNotFound
}

// Current returns the selected window, if any.
func (s *Selection) Current() (Info, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current, s.has
}

// CurrentHandle returns the selected window handle. It reports false when
// nothing usable is selected, so callers need no separate zero check.
func (s *Selection) CurrentHandle() (uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current.Handle, s.has && s.current.Handle != 0
}

// State returns the handle and its change notification under one lock, so a
// watcher cannot miss a change between reading the handle and waiting.
func (s *Selection) State() (handle uint64, ok bool, changed <-chan struct{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current.Handle, s.has, s.changed
}
