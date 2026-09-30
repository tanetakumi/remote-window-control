package webrtc

import (
	"context"
	"encoding/binary"
	pion "github.com/pion/webrtc/v4"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"share-app-host/internal/nativecapture"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("RWC_MEDIA_CAPTURE_HELPER") == "1" {
		for os.Getenv("RWC_MEDIA_HELPER_STALL") == "1" {
			time.Sleep(time.Hour)
		}
		packet := make([]byte, 24+16*16*4)
		binary.LittleEndian.PutUint32(packet, 16*16*4)
		binary.LittleEndian.PutUint32(packet[4:], 16)
		binary.LittleEndian.PutUint32(packet[8:], 16)
		binary.LittleEndian.PutUint32(packet[12:], 64)
		for id := uint64(1); ; id++ {
			binary.LittleEndian.PutUint64(packet[16:], id)
			if _, err := os.Stdout.Write(packet); err != nil {
				os.Exit(0)
			}
			time.Sleep(time.Second / 60)
		}
	}
	os.Exit(m.Run())
}
func helperBridge(t *testing.T) *nativecapture.Bridge {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "CaptureProbe")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	from, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer from.Close()
	to, err := os.OpenFile(filepath.Join(dir, "CaptureProbe.exe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(to, from)
	closeErr := to.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Setenv("RWC_MEDIA_CAPTURE_HELPER", "1")
	return nativecapture.NewBridge(base)
}
func TestTargetChangeCancelsStalledCapture(t *testing.T) {
	bridge := helperBridge(t)
	t.Setenv("RWC_MEDIA_HELPER_STALL", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- streamTarget(ctx, bridge, nil, 1, changed) }()
	time.Sleep(100 * time.Millisecond)
	close(changed)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("target change blocked on stalled frame read")
	}
}
func TestStreamingEncoderAndCaptureShutdown(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is required for encoder integration")
	}
	bridge := helperBridge(t)
	track, err := pion.NewTrackLocalStaticSample(pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8}, "video", "test")
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 3; iteration++ {
		ctx, cancel := context.WithCancel(context.Background())
		changed := make(chan struct{})
		done := make(chan error, 1)
		go func() { done <- streamTarget(ctx, bridge, track, 1, changed) }()
		time.Sleep(350 * time.Millisecond)
		close(changed)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(4 * time.Second):
			cancel()
			t.Fatal("streaming workers survived target change")
		}
		cancel()
	}
}
