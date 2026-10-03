package testutil

import (
	"fmt"
	"slices"
	"sync"

	"share-app-host/internal/input"
)

// RecordingInjector is an input.Injector that records every call as a short
// string, such as "down:left:0.5,0", "up:Enter" or "text:hello". It is safe for
// concurrent use, since a Dispatcher calls it from server goroutines.
type RecordingInjector struct {
	mu     sync.Mutex
	events []string
	// Fail, when set, is returned by every call after it has been recorded.
	Fail error
}

// Record appends an event as if an injector call had happened. Tests use it to
// mark points in the sequence.
func (r *RecordingInjector) Record(format string, args ...any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, fmt.Sprintf(format, args...))
	return r.Fail
}

// Events returns a copy of the recorded events.
func (r *RecordingInjector) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

// Reset forgets the recorded events.
func (r *RecordingInjector) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

func (r *RecordingInjector) Move(x, y float64) error { return r.Record("move:%v,%v", x, y) }
func (r *RecordingInjector) Tap(b string, x, y float64) error {
	return r.Record("tap:%s:%v,%v", b, x, y)
}
func (r *RecordingInjector) MouseDown(b string, x, y float64) error {
	return r.Record("down:%s:%v,%v", b, x, y)
}
func (r *RecordingInjector) MouseUp(b string, x, y float64) error {
	return r.Record("up:%s:%v,%v", b, x, y)
}
func (r *RecordingInjector) Scroll(dy, x, y float64) error {
	return r.Record("scroll:%v@%v,%v", dy, x, y)
}
func (r *RecordingInjector) ResizeViewport(c input.Command) error {
	return r.Record("resize:%dx%d", c.Width, c.Height)
}
func (r *RecordingInjector) KeyDown(c input.Command) error { return r.Record("down:%s", c.Key) }
func (r *RecordingInjector) KeyUp(c input.Command) error   { return r.Record("up:%s", c.Key) }
func (r *RecordingInjector) Text(s string, enter bool) error {
	if enter {
		return r.Record("text+enter:%s", s)
	}
	return r.Record("text:%s", s)
}

var _ input.Injector = (*RecordingInjector)(nil)
