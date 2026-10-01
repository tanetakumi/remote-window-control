package session

import "sync"

// Slot admits one control connection at a time and coordinates shutdown.
//
// The holder acquires the slot before creating any resources and releases it
// only after every worker and held input has been cleaned up, so a new
// connection can never overlap the previous one's teardown.
type Slot struct {
	mu      sync.Mutex
	held    bool
	closed  bool
	stop    func() // stops the current holder; nil when none registered
	holders sync.WaitGroup
}

// TryAcquire takes the slot. It fails if the slot is held or closed.
func (s *Slot) TryAcquire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.held {
		return false
	}
	s.held = true
	s.holders.Add(1)
	return true
}

// OnClose registers how to stop the current holder when the slot is closed.
// If the slot is already closed, stop runs immediately. The registration ends
// with Release.
func (s *Slot) OnClose(stop func()) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		stop()
		return
	}
	s.stop = stop
	s.mu.Unlock()
}

// Release frees the slot. Call it exactly once per successful TryAcquire.
func (s *Slot) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = false
	s.stop = nil
	s.holders.Done()
}

// Close refuses further acquisitions, stops the current holder if any and
// waits until it has released the slot.
func (s *Slot) Close() {
	s.mu.Lock()
	s.closed = true
	if s.stop != nil {
		s.stop()
	}
	s.mu.Unlock()
	s.holders.Wait()
}
