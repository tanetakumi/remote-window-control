package media_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"os/exec"
	"testing"
	"time"

	"share-app-host/internal/media"
)

// VP8 frame type is bit zero of the uncompressed header (RFC 6386, 9.1).
func keyframe(data []byte) bool { return len(data) >= 10 && data[0]&1 == 0 }

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

func TestStaticWindowRecoversWhenTheFirstKeyframeIsLost(t *testing.T) {
	requireFFmpeg(t)
	source := newFakeSource(singleFrame(fakeFrameSize))
	sink := &recordingSink{}
	pipeline := media.NewPipeline(source.source(), newFakeTarget(), sink, defaultEncoder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runWindow(ctx, pipeline, 1, make(chan struct{}))
	eventually(t, 5*time.Second, "a second keyframe without another capture frame", func() bool { return sink.keyframes() >= 2 })
	cancel()
	if err := receive(t, done, settle, "static stream shutdown"); err != nil {
		t.Fatal(err)
	}
	if len(source.openedHandles()) != 1 || !source.stream(0).isClosed() {
		t.Fatal("static capture was restarted or not closed")
	}
	// Discard the first encoded keyframe as if it were lost in transit. The
	// next keyframe must be complete and independently decodable by ffmpeg.
	sink.mu.Lock()
	var recovered []byte
	for _, sample := range sink.samples[1:] {
		if keyframe(sample.Data) {
			recovered = sample.Data
			break
		}
	}
	sink.mu.Unlock()
	decodeKeyframe(t, recovered, fakeFrameSize)
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
	copy(header[8:], "VP80")
	binary.LittleEndian.PutUint16(header[12:], uint16(size))
	binary.LittleEndian.PutUint16(header[14:], uint16(size))
	binary.LittleEndian.PutUint32(header[16:], 10)
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
