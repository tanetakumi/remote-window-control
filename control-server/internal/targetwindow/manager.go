package targetwindow

import (
	"context"
	"errors"
	"sync"

	"share-app-host/internal/nativecapture"
)

var ErrWindowNotSelected = errors.New("target window not selected")
var ErrWindowNotFound = errors.New("target window not found")

type Manager struct {
	bridge   *nativecapture.Bridge
	mu       sync.RWMutex
	selected nativecapture.WindowInfo
	hasValue bool
	changed  chan struct{}
}

func NewManager(bridge *nativecapture.Bridge) *Manager {
	return &Manager{bridge: bridge, changed: make(chan struct{})}
}

func (m *Manager) List(ctx context.Context) ([]nativecapture.WindowInfo, error) {
	return m.bridge.ListWindows(ctx)
}

func (m *Manager) Select(ctx context.Context, handle uint64) (nativecapture.WindowInfo, error) {
	windows, err := m.bridge.ListWindows(ctx)
	if err != nil {
		return nativecapture.WindowInfo{}, err
	}

	for _, window := range windows {
		if window.Handle == handle {
			m.mu.Lock()
			if !m.hasValue || m.selected.Handle != window.Handle {
				close(m.changed)
				m.changed = make(chan struct{})
			}
			m.selected = window
			m.hasValue = true
			m.mu.Unlock()
			return window, nil
		}
	}

	return nativecapture.WindowInfo{}, ErrWindowNotFound
}

func (m *Manager) Current() (nativecapture.WindowInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.selected, m.hasValue
}

func (m *Manager) CurrentHandle() (uint64, bool) {
	window, ok := m.Current()
	if !ok {
		return 0, false
	}
	return window.Handle, true
}

// State returns the handle and its change notification under the same lock.
func (m *Manager) State() (uint64, bool, <-chan struct{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.selected.Handle, m.hasValue, m.changed
}
