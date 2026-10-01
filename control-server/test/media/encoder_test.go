package media_test

import (
	"reflect"
	"slices"
	"testing"

	"share-app-host/internal/media"
)

// The baseline settings are pinned on purpose: the README requires keeping the
// 10 fps / quality baseline until interactive Windows measurements justify
// tuning, so changing it should be a deliberate edit to this test.
func TestEncoderArgsBaseline(t *testing.T) {
	got := media.DefaultEncoderConfig("ffmpeg").Args(1920, 1080)
	want := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "rawvideo", "-pix_fmt", "bgra",
		"-video_size", "1920x1080",
		"-framerate", "10",
		"-i", "pipe:0",
		"-an",
		"-vf", "format=yuv420p",
		"-c:v", "libvpx",
		"-b:v", "6M",
		"-crf", "10",
		"-deadline", "realtime",
		"-cpu-used", "4",
		"-auto-alt-ref", "0",
		"-g", "20",
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
		{"-g", "60"}, // a keyframe every two seconds
	} {
		i := slices.Index(args, pair[0])
		if i < 0 || i+1 >= len(args) || args[i+1] != pair[1] {
			t.Errorf("want %s %s in %v", pair[0], pair[1], args)
		}
	}
}
