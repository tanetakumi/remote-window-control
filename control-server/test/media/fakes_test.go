package media_test

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	pionmedia "github.com/pion/webrtc/v4/pkg/media"

	"share-app-host/internal/capture"
	"share-app-host/internal/media"
	"share-app-host/test/testutil"
)

const fakeFrameSize = testutil.FakeFrameSize

var errCapture = errors.New("capture crashed")

// ---- streams ----------------------------------------------------------------

type readFunc func(s *fakeStream, buffer []byte) (capture.Frame, error)

// fakeStream is an in-process capture stream. Its read function decides what
// each ReadFrameInto does; like a real stream it must return once closed.
type fakeStream struct {
	read   readFunc
	closed chan struct{}
	once   sync.Once
	nextID int64
}

func newFakeStream(read readFunc) *fakeStream {
	return &fakeStream{read: read, closed: make(chan struct{})}
}

func (s *fakeStream) ReadFrameInto(buffer []byte) (capture.Frame, error) { return s.read(s, buffer) }

func (s *fakeStream) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeStream) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

// stalled blocks until the stream is closed, like a capture that stopped
// producing frames.
func stalled() readFunc {
	return func(s *fakeStream, _ []byte) (capture.Frame, error) {
		<-s.closed
		return capture.Frame{}, errors.New("stream closed")
	}
}

// failing reports a capture error on the first read.
func failing(err error) readFunc {
	return func(*fakeStream, []byte) (capture.Frame, error) { return capture.Frame{}, err }
}

// steady produces size x size frames at about 60 per second.
func steady(size int) readFunc {
	return func(s *fakeStream, buffer []byte) (capture.Frame, error) {
		select {
		case <-s.closed:
			return capture.Frame{}, errors.New("stream closed")
		case <-time.After(time.Second / 60):
		}
		length := size * size * 4
		if cap(buffer) < length {
			buffer = make([]byte, length)
		}
		s.nextID++
		return capture.Frame{Width: size, Height: size, Stride: size * 4, ID: s.nextID, Data: buffer[:length]}, nil
	}
}

// singleFrame models a static window: WGC gives one image, then no updates.
func singleFrame(size int) readFunc {
	return func(s *fakeStream, buffer []byte) (capture.Frame, error) {
		if s.nextID > 0 {
			return stalled()(s, buffer)
		}
		s.nextID = 1
		buffer = make([]byte, size*size*4)
		return capture.Frame{Width: size, Height: size, Stride: size * 4, ID: 1, Data: buffer}, nil
	}
}

// ---- source -----------------------------------------------------------------

// fakeSource opens fakeStreams and records which windows were requested.
type fakeSource struct {
	mu      sync.Mutex
	read    readFunc
	openErr error
	opened  []uint64
	streams []*fakeStream
}

func newFakeSource(read readFunc) *fakeSource { return &fakeSource{read: read} }

func (f *fakeSource) source() media.Source {
	return func(_ context.Context, handle uint64) (media.FrameStream, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.openErr != nil {
			return nil, f.openErr
		}
		stream := newFakeStream(f.read)
		f.opened = append(f.opened, handle)
		f.streams = append(f.streams, stream)
		return stream, nil
	}
}

func (f *fakeSource) openedHandles() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.opened)
}

func (f *fakeSource) stream(i int) *fakeStream {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.streams[i]
}

// ---- target -----------------------------------------------------------------

// fakeTarget is a controllable window selection.
type fakeTarget struct {
	mu       sync.Mutex
	handle   uint64
	selected bool
	changed  chan struct{}
}

func newFakeTarget() *fakeTarget { return &fakeTarget{changed: make(chan struct{})} }

func (t *fakeTarget) State() (uint64, bool, <-chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.handle, t.selected, t.changed
}

// Select chooses a window and notifies watchers if it differs from before.
func (t *fakeTarget) Select(handle uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.selected || t.handle != handle {
		close(t.changed)
		t.changed = make(chan struct{})
	}
	t.handle, t.selected = handle, true
}

// ---- sink -------------------------------------------------------------------

// recordingSink collects the samples written by the encoder.
type recordingSink struct {
	mu      sync.Mutex
	samples []pionmedia.Sample
}

func (s *recordingSink) WriteSample(sample pionmedia.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, sample)
	return nil
}

func (s *recordingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.samples)
}

// ---- helpers ----------------------------------------------------------------

// eventually polls cond until it holds or the timeout expires.
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// receive waits for a result from ch, failing the test after the timeout.
func receive[T any](t *testing.T, ch <-chan T, timeout time.Duration, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is required for encoder integration")
	}
}

func defaultEncoder() media.EncoderConfig { return media.DefaultEncoderConfig("ffmpeg") }
