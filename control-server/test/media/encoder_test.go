package media_test

import (
	"bytes"
	"context"
	"os/exec"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/pion/webrtc/v4/pkg/media/ivfreader"

	"share-app-host/internal/media"
)

// Pin the VP9 screen-streaming defaults so tuning is deliberate.
func TestEncoderArgsBaseline(t *testing.T) {
	got := media.DefaultEncoderConfig("ffmpeg").Args(1920, 1080)
	want := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "rawvideo", "-pix_fmt", "bgra",
		"-video_size", "1920x1080",
		"-framerate", "8",
		"-i", "pipe:0",
		"-an",
		"-vf", "format=yuv420p",
		"-c:v", "libvpx-vp9",
		"-profile:v", "0",
		"-b:v", "6M",
		"-crf", "31",
		"-deadline", "realtime",
		"-cpu-used", "4",
		"-auto-alt-ref", "0",
		"-lag-in-frames", "0",
		"-row-mt", "1",
		"-tune-content", "screen",
		"-g", "1000000",
		"-f", "ivf", "pipe:1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ffmpeg arguments changed:\n got  %v\n want %v", got, want)
	}
}

func TestEncoderArgsFollowTheConfiguration(t *testing.T) {
	cfg := media.EncoderConfig{FFmpegPath: "ffmpeg", FPS: 30, Bitrate: "2M", CRF: 20}
	args := cfg.Args(640, 480)

	for _, pair := range [][2]string{
		{"-video_size", "640x480"},
		{"-framerate", "30"},
		{"-b:v", "2M"},
		{"-crf", "20"},
		{"-g", "1000000"}, // no periodic keyframes, whatever the rate
	} {
		i := slices.Index(args, pair[0])
		if i < 0 || i+1 >= len(args) || args[i+1] != pair[1] {
			t.Errorf("want %s %s in %v", pair[0], pair[1], args)
		}
	}
}

func TestEncoderProducesVP9IVFAtEightFPS(t *testing.T) {
	requireFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := media.DefaultEncoderConfig("ffmpeg")
	cmd := exec.CommandContext(ctx, cfg.FFmpegPath, cfg.Args(fakeFrameSize, fakeFrameSize)...)
	cmd.Stdin = bytes.NewReader(make([]byte, fakeFrameSize*fakeFrameSize*4*2))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("encode: %v: %s", err, &stderr)
	}
	reader, header, err := ivfreader.NewWith(bytes.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	if header.FourCC != "VP90" || header.TimebaseDenominator != 8 || header.TimebaseNumerator != 1 {
		t.Fatalf("unexpected IVF header: %+v", header)
	}
	first, _, err := reader.ParseNextFrame()
	if err != nil {
		t.Fatal(err)
	}
	decodeKeyframe(t, first, fakeFrameSize)
	second, _, err := reader.ParseNextFrame()
	if err != nil || len(second) == 0 || keyframe(second) {
		t.Fatalf("expected a delta frame: bytes=%d error=%v", len(second), err)
	}
}
