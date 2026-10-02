package media_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"share-app-host/internal/media"
)

// A profile-0 VP9 keyframe starts with the frame marker, no show-existing
// flag, frame_type=0, and the keyframe sync code (spec sections 6.2 and 6.2.1).
func keyframe(data []byte) bool {
	return len(data) >= 4 && data[0]&0xfc == 0x80 && bytes.Equal(data[1:4], []byte{0x49, 0x83, 0x42})
}

func (s *recordingSink) keyframes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, sample := range s.samples {
		if keyframe(sample.Data) {
			count++
		}
	}
	return count
}

func TestKeyframeRequestWhileIdleSendsADecodableKeyframe(t *testing.T) {
	pipeline, sink := startStreaming(t, singleFrame(fakeFrameSize))
	idle(t, sink)
	keyframes := sink.keyframes()

	requested := time.Now()
	pipeline.RequestKeyframe()
	eventually(t, 5*time.Second, "a keyframe after the request", func() bool { return sink.keyframes() > keyframes })

	samples := sink.snapshot()
	last := samples[len(samples)-1]
	decodeKeyframe(t, last.Data, fakeFrameSize)

	// The RTP clock (the sum of durations before a sample) must follow the
	// wall clock across the idle gap, not advance one nominal frame.
	var mediaTime time.Duration
	for _, sample := range samples[:len(samples)-1] {
		mediaTime += sample.Duration
	}
	wall := last.Timestamp.Sub(samples[0].Timestamp)
	if diff := mediaTime - wall; diff < -50*time.Millisecond || diff > 250*time.Millisecond {
		t.Fatalf("media time %s for wall time %s", mediaTime, wall)
	}
	if last.Timestamp.Sub(requested) > 2*time.Second {
		t.Fatalf("keyframe took %s", last.Timestamp.Sub(requested))
	}
}

func TestKeyframeRequestAnsweredByTheNextKeyframeDoesNotRestart(t *testing.T) {
	requireFFmpeg(t)
	logs := captureLog(t)
	sink := &recordingSink{}
	pipeline := media.NewPipeline(newFakeSource(singleFrame(fakeFrameSize)).source(), newFakeTarget(), sink, defaultEncoder())
	// The encoder's first frame is a keyframe written after this request.
	pipeline.RequestKeyframe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{})
	done := runWindow(ctx, pipeline, 1, changed)
	eventually(t, 10*time.Second, "the first keyframe", func() bool { return sink.keyframes() > 0 })
	time.Sleep(1500 * time.Millisecond)
	close(changed)
	if err := receive(t, done, 4*time.Second, "RunWindow to stop"); err != nil {
		t.Fatal(err)
	}
	stopped := regexp.MustCompile(`capture stopped hwnd=1 .*`).FindString(logs.String())
	if stopped == "" || field(t, stopped, "forced_keyframes") != 0 {
		t.Fatalf("stop line %q", stopped)
	}
}

func decodeKeyframe(t *testing.T, payload []byte, size int) {
	t.Helper()
	if !keyframe(payload) {
		t.Fatal("no recovery keyframe")
	}
	var stream bytes.Buffer
	header := make([]byte, 32)
	copy(header, "DKIF")
	binary.LittleEndian.PutUint16(header[6:], 32)
	copy(header[8:], "VP90")
	binary.LittleEndian.PutUint16(header[12:], uint16(size))
	binary.LittleEndian.PutUint16(header[14:], uint16(size))
	binary.LittleEndian.PutUint32(header[16:], media.DefaultFPS)
	binary.LittleEndian.PutUint32(header[20:], 1)
	binary.LittleEndian.PutUint32(header[24:], 1)
	stream.Write(header)
	frameHeader := make([]byte, 12)
	binary.LittleEndian.PutUint32(frameHeader, uint32(len(payload)))
	stream.Write(frameHeader)
	stream.Write(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "ivf", "-i", "pipe:0", "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "bgra", "pipe:1")
	command.Stdin = &stream
	var stderr bytes.Buffer
	command.Stderr = &stderr
	pixels, err := command.Output()
	if err != nil || len(pixels) != size*size*4 {
		t.Fatalf("recovery keyframe decode: pixels=%d error=%v stderr=%s", len(pixels), err, stderr.String())
	}
}
