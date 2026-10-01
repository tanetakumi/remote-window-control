package media

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	pionmedia "github.com/pion/webrtc/v4/pkg/media"
)

// Pipeline captures the selected window, encodes it and writes the samples to
// a sink. It follows the target: when another window is selected it stops the
// old capture immediately, without waiting for another frame, and starts the
// new one.
type Pipeline struct {
	source  Source
	target  Target
	sink    SampleWriter
	encoder EncoderConfig
	logf    func(string, ...any)
}

// NewPipeline returns a Pipeline that streams the window chosen by target.
func NewPipeline(source Source, target Target, sink SampleWriter, encoder EncoderConfig) *Pipeline {
	if encoder.FPS <= 0 {
		encoder.FPS = DefaultFPS
	}
	return &Pipeline{source: source, target: target, sink: sink, encoder: encoder, logf: log.Printf}
}

// Run streams until ctx is cancelled, waiting while no window is selected. It
// returns nil on cancellation and an error when a stream fails for a reason
// other than the target changing.
func (p *Pipeline) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		handle, selected, changed := p.target.State()
		if !selected || handle == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
				continue
			}
		}

		err := p.RunWindow(ctx, handle, changed)
		if err == nil || ctx.Err() != nil {
			continue
		}
		select {
		case <-changed:
			// The failure came from the target moving on; follow it.
		default:
			return fmt.Errorf("window stream failed: %w", err)
		}
	}
	return nil
}

// RunWindow streams one window until ctx is cancelled or changed is closed. It
// returns nil in both cases, and an error if capture or encoding fails.
//
// It owns one capture stream and one long-lived encoder per frame size; the
// encoder is restarted only when the window's size changes.
func (p *Pipeline) RunWindow(parent context.Context, handle uint64, changed <-chan struct{}) (result error) {
	started := time.Now()
	var frames uint64
	sink := &firstSampleLogger{SampleWriter: p.sink, logf: p.logf, handle: handle, started: started}
	p.logf("capture starting hwnd=%d", handle)
	defer func() {
		p.logf("capture stopped hwnd=%d elapsed=%s frames=%d samples=%d error=%v", handle, time.Since(started).Round(time.Millisecond), frames, sink.samples.Load(), result)
	}()
	ctx, cancel := context.WithCancel(parent)
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		select {
		case <-ctx.Done():
		case <-changed:
			cancel()
		}
	}()
	defer func() { cancel(); watcher.Wait() }()

	stream, err := p.source(ctx, handle)
	if err != nil {
		return fmt.Errorf("open capture hwnd=%d: %w", handle, err)
	}
	p.logf("capture opened hwnd=%d", handle)
	pump := startFramePump(ctx, cancel, stream)

	var enc *encoder
	// Stop in dependency order: the encoder, then the stream (which unblocks
	// the pump's read), then wait for the pump goroutine.
	defer func() {
		cancel()
		closeEncoder(enc)
		_ = stream.Close()
		pump.wait()
	}()

	ticker := time.NewTicker(time.Second / time.Duration(p.encoder.FPS))
	defer ticker.Stop()
	lastFrame := time.Now()
	stallLogged := false
	for {
		select {
		case <-ctx.Done():
			return pump.err()
		case <-ticker.C:
		}

		frame, ok := pump.next()
		if !ok {
			if !stallLogged && time.Since(lastFrame) >= 5*time.Second {
				p.logf("capture waiting hwnd=%d no new frame for %s frames=%d", handle, time.Since(lastFrame).Round(time.Millisecond), frames)
				stallLogged = true
			}
			continue
		}
		if frames == 0 {
			p.logf("first capture frame hwnd=%d size=%dx%d elapsed=%s", handle, frame.Width, frame.Height, time.Since(started).Round(time.Millisecond))
		}
		frames++
		lastFrame = time.Now()
		stallLogged = false
		if !enc.matches(frame.Width, frame.Height) {
			closeEncoder(enc)
			enc = nil
			if enc, err = startEncoder(ctx, p.encoder, sink, frame.Width, frame.Height); err != nil {
				return fmt.Errorf("start encoder hwnd=%d: %w", handle, err)
			}
		}
		err = enc.WriteFrame(frame.Data)
		pump.recycle(frame.Data)
		if err != nil {
			return fmt.Errorf("encode frame hwnd=%d: %w", handle, err)
		}
	}
}

// firstSampleLogger counts samples written to the track and logs the first, so
// a log shows whether video ever left the host.
type firstSampleLogger struct {
	SampleWriter
	logf    func(string, ...any)
	handle  uint64
	started time.Time
	samples atomic.Uint64
}

func (s *firstSampleLogger) WriteSample(sample pionmedia.Sample) error {
	if err := s.SampleWriter.WriteSample(sample); err != nil {
		return err
	}
	if s.samples.Add(1) == 1 {
		s.logf("first video sample written hwnd=%d bytes=%d elapsed=%s", s.handle, len(sample.Data), time.Since(s.started).Round(time.Millisecond))
	}
	return nil
}
