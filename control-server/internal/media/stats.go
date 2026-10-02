package media

import (
	"sync/atomic"
	"time"

	pionmedia "github.com/pion/webrtc/v4/pkg/media"
)

// sampleCounter counts the samples and bytes written to the track, separating
// VP9 keyframes, and logs the first sample, so a log shows whether video ever
// left the host. The counts cover encoded payload only, not RTP or transport
// overhead.
type sampleCounter struct {
	SampleWriter
	logf    func(string, ...any)
	handle  uint64
	started time.Time

	samples, bytes, keyframes, keyframeBytes atomic.Uint64
	// lastKeyframe is when the newest keyframe was written, in Unix
	// nanoseconds; zero before the first.
	lastKeyframe atomic.Int64
}

type sampleCounts struct {
	samples, bytes, keyframes, keyframeBytes uint64
}

func (s *sampleCounter) WriteSample(sample pionmedia.Sample) error {
	if err := s.SampleWriter.WriteSample(sample); err != nil {
		return err
	}
	size := uint64(len(sample.Data))
	s.bytes.Add(size)
	// VP9 uncompressed header (spec section 6.2), for our profile-0 output:
	// frame_marker=2, profile=0, show_existing_frame=0, frame_type=0.
	// A show-existing frame only displays a reference and cannot answer a PLI.
	if size > 0 && sample.Data[0]&0xfc == 0x80 {
		s.keyframes.Add(1)
		s.keyframeBytes.Add(size)
		s.lastKeyframe.Store(time.Now().UnixNano())
	}
	if s.samples.Add(1) == 1 {
		s.logf("first video sample written hwnd=%d bytes=%d elapsed=%s", s.handle, len(sample.Data), time.Since(s.started).Round(time.Millisecond))
	}
	return nil
}

func (s *sampleCounter) snapshot() sampleCounts {
	return sampleCounts{
		samples:       s.samples.Load(),
		bytes:         s.bytes.Load(),
		keyframes:     s.keyframes.Load(),
		keyframeBytes: s.keyframeBytes.Load(),
	}
}

// statsLogger logs per-interval differences of the stream counters. It is
// used only by the RunWindow loop. A zero interval disables it.
type statsLogger struct {
	interval time.Duration
	logf     func(string, ...any)
	handle   uint64
	since    time.Time

	frames, encoded, forced uint64
	// requests counts browser keyframe requests (PLI or FIR).
	requests uint64
	samples  sampleCounts
}

func (l *statsLogger) logIfDue(frames, encoded, forced, requests uint64, samples sampleCounts) {
	if l.interval <= 0 {
		return
	}
	elapsed := time.Since(l.since)
	if elapsed < l.interval {
		return
	}
	bytes := samples.bytes - l.samples.bytes
	l.logf("media stats hwnd=%d interval_ms=%d frames=%d encoded=%d keyframe_requests=%d forced_keyframes=%d samples=%d bytes=%d kbps=%.1f keyframes=%d keyframe_bytes=%d",
		l.handle, elapsed.Milliseconds(), frames-l.frames, encoded-l.encoded, requests-l.requests, forced-l.forced,
		samples.samples-l.samples.samples, bytes, float64(bytes)*8/elapsed.Seconds()/1000,
		samples.keyframes-l.samples.keyframes, samples.keyframeBytes-l.samples.keyframeBytes)
	l.since = time.Now()
	l.frames, l.encoded, l.forced, l.requests, l.samples = frames, encoded, forced, requests, samples
}
