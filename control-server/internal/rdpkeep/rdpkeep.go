// Package rdpkeep maintains a loopback RDP connection to the host's own
// session so the session keeps a display output (Windows Graphics Capture
// stops when the last display client, e.g. a human RDP viewer, leaves).
//
// The connection is started and stopped manually via the web UI. After an
// unexpected disconnect — most importantly a human reconnecting and taking
// the session over — the manager stays down; it never reconnects on its own.
package rdpkeep

import (
	"errors"
	"log"
	"sync"
	"time"

	rdp "github.com/bouncyball-git/gopher-rdp"
)

// The keep-alive always connects to the host's own session over loopback,
// requesting this desktop size. The session's display takes this resolution
// while the connection is up.
const (
	host   = "127.0.0.1"
	port   = 3389
	width  = 1920
	height = 1080
)

// connectTimeout bounds the whole connect handshake from the manager side.
const connectTimeout = 30 * time.Second

// State is the lifecycle state of the keep-alive connection.
type State string

const (
	// StateOff means there is no connection and none is being attempted.
	StateOff State = "off"
	// StateConnecting means a connection attempt is in progress.
	StateConnecting State = "connecting"
	// StateConnected means the connection is established and maintained.
	StateConnected State = "connected"
	// StateDisconnected means a connection was attempted and lost; the
	// error field of Status carries the reason.
	StateDisconnected State = "disconnected"
)

// Status is the current state reported to the web UI.
type Status struct {
	State State `json:"state"`
	// Enabled reports whether the connection is up or being attempted, so
	// the web UI toggles by requesting its negation.
	Enabled bool `json:"enabled"`
	// Error is the reason of the last disconnect or failed attempt.
	Error string `json:"error,omitempty"`
}

// Config is the Windows account the keep-alive signs in with.
type Config struct {
	Username string
	Password string
}

// Manager owns the single keep-alive connection.
type Manager struct {
	cfg Config

	mu      sync.Mutex
	enabled bool
	state   State
	errMsg  string
	client  *rdp.Client
	gen     int // incremented when an attempt ends; stale callbacks check it
}

// NewManager returns a Manager in the off state.
func NewManager(cfg Config) *Manager {
	return &Manager{cfg: cfg, state: StateOff}
}

// Start begins a connection attempt. It returns an error when credentials
// are missing, or when a connection is already up or being attempted.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled {
		return errors.New("session keep-alive is already active")
	}
	if m.cfg.Username == "" || m.cfg.Password == "" {
		return errors.New("RDP username or password is not configured")
	}
	m.enabled = true
	m.state = StateConnecting
	m.errMsg = ""
	go m.connect(m.gen)
	return nil
}

// Stop closes any connection or attempt and returns to the off state.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.enabled {
		m.mu.Unlock()
		return
	}
	m.enabled = false
	m.gen++
	m.state = StateOff
	m.errMsg = ""
	client := m.client
	m.client = nil
	m.mu.Unlock()
	if client != nil {
		client.Close()
	}
}

// SetEnabled starts or stops the connection per the requested state.
func (m *Manager) SetEnabled(enabled bool) (Status, error) {
	if enabled {
		if err := m.Start(); err != nil {
			return m.Status(), err
		}
	} else {
		m.Stop()
	}
	return m.Status(), nil
}

// Status returns the current state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{State: m.state, Enabled: m.enabled, Error: m.errMsg}
}

// connect runs in its own goroutine and performs the connection attempt of
// generation gen.
func (m *Manager) connect(gen int) {
	opts := rdp.DefaultOptions()
	opts.Host = host
	opts.Port = port
	opts.Username = m.cfg.Username
	opts.Password = m.cfg.Password
	opts.Width = width
	opts.Height = height
	opts.Clipboard = false
	opts.AutoReconnect = false // a human takeover must not be undone

	client, err := rdp.NewClient(opts)
	if err != nil {
		m.finishConnect(gen, nil, err)
		return
	}
	m.mu.Lock()
	if m.gen != gen {
		m.mu.Unlock()
		return // stopped before dialing
	}
	m.client = client
	m.mu.Unlock()

	client.OnDisconnect(func(e error) {
		m.mu.Lock()
		if m.gen != gen {
			m.mu.Unlock()
			return // stopped, or already resolved by the watchdog
		}
		// Terminal: a human reconnecting takes the session over here.
		// Stay down until the user asks again; never reconnect.
		m.terminateLocked(e.Error())
		m.mu.Unlock()
		log.Printf("session keep-alive disconnected: %v", e)
	})

	watchdog := time.AfterFunc(connectTimeout, func() {
		m.mu.Lock()
		if m.gen != gen || m.state != StateConnecting {
			m.mu.Unlock()
			return
		}
		m.terminateLocked("connect timed out")
		m.mu.Unlock()
		log.Printf("session keep-alive connect timed out")
		client.Close() // unblocks Connect; the attempt stays disconnected
	})

	err = client.Connect()
	watchdog.Stop()
	m.finishConnect(gen, client, err)
}

// terminateLocked marks the current attempt terminal: down with a reason,
// requiring a new user action to try again. m.mu must be held.
func (m *Manager) terminateLocked(reason string) {
	m.enabled = false
	m.gen++
	m.state = StateDisconnected
	m.errMsg = reason
	m.client = nil
}

// finishConnect records the outcome of a connection attempt. Transitions are
// skipped when the attempt was already resolved (stop or watchdog); a
// connection that still came up then is closed (a failed Connect has
// already closed itself).
func (m *Manager) finishConnect(gen int, client *rdp.Client, err error) {
	m.mu.Lock()
	current := m.gen == gen
	if current {
		if err == nil {
			m.state = StateConnected
			m.mu.Unlock()
			log.Printf("session keep-alive connected at %dx%d", width, height)
			return
		}
		m.terminateLocked(err.Error())
	}
	m.mu.Unlock()
	switch {
	case err != nil:
		log.Printf("session keep-alive connect failed: %v", err)
	case !current:
		client.Close()
	}
}
