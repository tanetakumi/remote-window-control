package media

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	pionmedia "github.com/pion/webrtc/v4/pkg/media"

	"share-app-host/internal/capture"
)

// Pipeline captures the selected window, encodes it and writes the samples to
// a sink. It follows the target: when another window is selected it stops the
// old capture immediately, without waiting for another frame, and starts the
// new one.
type Pipeline struct {
	// StatsInterval, when positive, logs capture and encoded-video statistics
	// at this interval while a window streams.
	StatsInterval time.Duration

	source  Source
	target  Target
	sink    *wallClockSink
	encoder EncoderConfig
	logf    func(string, ...any)

	// keyframeRequest is the time of the oldest unanswered browser keyframe
	// request in Unix nanoseconds, or zero.
	keyframeRequest atomic.Int64
	// keyframeRequests counts every browser request, answered or not.
	keyframeRequests atomic.Uint64
}

const (
	// keyframeRequestInterval limits how often a browser request restarts the
	// encoder. Browsers repeat a request until a keyframe arrives.
	keyframeRequestInterval = time.Second

	// idleHeartbeat is how often an unchanged window is still encoded.
	// This receive-side timeout applies to VP9 as well as other codecs.
	// libwebrtc requests a keyframe when no decodable frame arrives for 3 s
	// while packets came within 5 s (VideoReceiveStream2::
	// OnDecodableFrameTimeout), so a fully silent stream would turn into a
	// keyframe every few seconds. A static delta frame costs a few hundred
	// bytes.
	idleHeartbeat = time.Second
)

// NewPipeline returns a Pipeline that streams the window chosen by target.
func NewPipeline(source Source, target Target, sink SampleWriter, encoder EncoderConfig) *Pipeline {
	if encoder.FPS <= 0 {
		encoder.FPS = DefaultFPS
	}
	return &Pipeline{source: source, target: target, sink: &wallClockSink{SampleWriter: sink}, encoder: encoder, logf: log.Printf}
}

// RequestKeyframe asks for a keyframe, as the browser does with RTCP PLI or FIR
// after losing video. A keyframe written after the request answers it;
// otherwise the encoder is restarted, which always begins with a keyframe.
func (p *Pipeline) RequestKeyframe() {
	p.keyframeRequests.Add(1)
	p.keyframeRequest.CompareAndSwap(0, time.Now().UnixNano())
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
// encoder is restarted when the window's size changes or to answer a keyframe
// request. The newest frame is kept and encoded at the configured rate for a
// second after each change, then once per idleHeartbeat while the window stays
// the same. Keyframes come only from encoder starts, so the stream relies on
// NACK retransmission and keyframe requests to recover from loss.
func (p *Pipeline) RunWindow(parent context.Context, handle uint64, changed <-chan struct{}) (result error) {
	started := time.Now()
	var frames, encoded, forced uint64
	sink := &sampleCounter{SampleWriter: p.sink, logf: p.logf, handle: handle, started: started}
	p.logf("capture starting hwnd=%d", handle)
	defer func() {
		total := sink.snapshot()
		p.logf("capture stopped hwnd=%d elapsed=%s frames=%d encoded=%d forced_keyframes=%d samples=%d bytes=%d keyframes=%d keyframe_bytes=%d error=%v",
			handle, time.Since(started).Round(time.Millisecond), frames, encoded, forced, total.samples, total.bytes, total.keyframes, total.keyframeBytes, result)
	}()
	stats := statsLogger{interval: p.StatsInterval, logf: p.logf, handle: handle, since: started,
		requests: p.keyframeRequests.Load()}
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
	var current capture.Frame
	stallLogged := false
	// refresh counts the ticks that still encode the newest frame at the full
	// rate after it changed, which lets the encoder refine its quality.
	refresh := 0
	var encoderStarted, lastEncoded time.Time
	for {
		select {
		case <-ctx.Done():
			return pump.err()
		case <-ticker.C:
		}
		if ctx.Err() != nil {
			return pump.err()
		}
		stats.logIfDue(frames, encoded, forced, p.keyframeRequests.Load(), sink.snapshot())

		frame, ok := pump.next()
		if ok {
			// WGC also delivers frames whose pixels did not change; those do
			// not count as changes.
			if current.Data == nil || frame.Width != current.Width ||
				frame.Height != current.Height || !bytes.Equal(frame.Data, current.Data) {
				refresh = p.encoder.FPS
			}
			if current.Data != nil {
				pump.recycle(current.Data)
			}
			current = frame
			if frames == 0 {
				p.logf("first capture frame hwnd=%d size=%dx%d elapsed=%s", handle, frame.Width, frame.Height, time.Since(started).Round(time.Millisecond))
			}
			frames++
			lastFrame = time.Now()
			stallLogged = false
		} else if !stallLogged && time.Since(lastFrame) >= 5*time.Second {
			p.logf("capture waiting hwnd=%d no new frame for %s frames=%d", handle, time.Since(lastFrame).Round(time.Millisecond), frames)
			stallLogged = true
		}
		if current.Data == nil {
			continue
		}
		// A request is answered by any keyframe written after it; otherwise
		// restart the encoder, at most once per interval.
		restart := false
		if requested := p.keyframeRequest.Load(); requested != 0 {
			if sink.lastKeyframe.Load() >= requested {
				p.keyframeRequest.CompareAndSwap(requested, 0)
			} else if enc != nil && time.Since(encoderStarted) >= keyframeRequestInterval {
				p.keyframeRequest.CompareAndSwap(requested, 0)
				restart = true
				forced++
				p.logf("keyframe forced hwnd=%d requested_ms_ago=%d", handle, time.Since(time.Unix(0, requested)).Milliseconds())
			}
		}
		if refresh == 0 && !restart && enc.matches(current.Width, current.Height) &&
			time.Since(lastEncoded) < idleHeartbeat {
			continue
		}
		if refresh > 0 {
			refresh--
		}
		// current stays owned by this loop until a newer frame replaces it.
		if restart || !enc.matches(current.Width, current.Height) {
			closeEncoder(enc)
			enc = nil
			if enc, err = startEncoder(ctx, p.encoder, sink, current.Width, current.Height); err != nil {
				return fmt.Errorf("start encoder hwnd=%d: %w", handle, err)
			}
			encoderStarted = time.Now()
		}
		err = enc.WriteFrame(current.Data)
		if err != nil {
			return fmt.Errorf("encode frame hwnd=%d: %w", handle, err)
		}
		encoded++
		lastEncoded = time.Now()
	}
}

// wallClockSink keeps RTP timestamps in step with the wall clock. Encoded
// samples carry a nominal 1/FPS duration, which the RTP packetizer adds after
// each sample. When the next sample comes later than that, as while an
// unchanged window is only encoded once per idleHeartbeat, after an encoder
// restart or after a target change, it first writes an empty
// sample: pion advances the timestamp for it without sending a packet.
//
// It lives as long as the Pipeline, because all windows share one track.
type wallClockSink struct {
	SampleWriter

	mu      sync.Mutex
	started time.Time     // wall time of the first sample
	written time.Duration // media time accounted for so far
}

func (s *wallClockSink) WriteSample(sample pionmedia.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.started.IsZero() {
		s.started = now
	}
	if behind := now.Sub(s.started) - s.written; behind > 0 {
		if err := s.SampleWriter.WriteSample(pionmedia.Sample{Duration: behind}); err != nil {
			return err
		}
		s.written += behind
	}
	if err := s.SampleWriter.WriteSample(sample); err != nil {
		return err
	}
	s.written += sample.Duration
	return nil
}
