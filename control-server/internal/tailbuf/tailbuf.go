// Package tailbuf provides a bounded, concurrency-safe buffer that keeps only
// the most recent bytes written to it. It is used to capture the tail of a
// subprocess's stderr so diagnostics stay available without growing forever.
package tailbuf

import "sync"

// DefaultLimit is the number of bytes kept by a zero-value Buffer.
const DefaultLimit = 16 * 1024

// Buffer keeps the last N bytes written. The zero value keeps DefaultLimit
// bytes and is ready to use as an io.Writer.
type Buffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

// New returns a Buffer that keeps the last limit bytes. A non-positive limit
// selects DefaultLimit.
func New(limit int) *Buffer {
	return &Buffer{limit: limit}
}

// Write appends p, discarding the oldest bytes beyond the limit. It never fails.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	limit := b.capacity()
	n := len(p)
	if len(p) >= limit {
		b.data = append(b.data[:0], p[len(p)-limit:]...)
		return n, nil
	}
	if excess := len(b.data) + len(p) - limit; excess > 0 {
		copy(b.data, b.data[excess:])
		b.data = b.data[:len(b.data)-excess]
	}
	b.data = append(b.data, p...)
	return n, nil
}

// String returns a copy of the retained tail.
func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

func (b *Buffer) capacity() int {
	if b.limit <= 0 {
		return DefaultLimit
	}
	return b.limit
}
