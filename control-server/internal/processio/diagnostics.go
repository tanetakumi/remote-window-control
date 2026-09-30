package processio

import "sync"

// Diagnostics keeps a synchronized tail, so subprocess logs cannot grow forever.
type Diagnostics struct {
	mu   sync.Mutex
	data []byte
}

const limit = 16 * 1024

func (d *Diagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(p)
	if len(p) >= limit {
		d.data = append(d.data[:0], p[len(p)-limit:]...)
	} else {
		if excess := len(d.data) + len(p) - limit; excess > 0 {
			copy(d.data, d.data[excess:])
			d.data = d.data[:len(d.data)-excess]
		}
		d.data = append(d.data, p...)
	}
	return n, nil
}
func (d *Diagnostics) String() string { d.mu.Lock(); defer d.mu.Unlock(); return string(d.data) }
