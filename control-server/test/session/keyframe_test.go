package session_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp/codecs"

	"share-app-host/internal/capture"
	"share-app-host/internal/input"
	"share-app-host/internal/media"
	"share-app-host/internal/session"
	"share-app-host/test/testutil"
)

// fixedTarget always has window 1 selected.
type fixedTarget struct{}

func (fixedTarget) State() (uint64, bool, <-chan struct{}) { return 1, true, make(chan struct{}) }

// staticStream delivers one frame, then nothing until it is closed, like WGC
// capturing a window that does not change.
type staticStream struct {
	once   sync.Once
	closed chan struct{}
	sent   bool
}

func (s *staticStream) ReadFrameInto([]byte) (capture.Frame, error) {
	if !s.sent {
		s.sent = true
		const size = 16
		return capture.Frame{Width: size, Height: size, Stride: size * 4, ID: 1, Data: make([]byte, size*size*4)}, nil
	}
	<-s.closed
	return capture.Frame{}, errors.New("stream closed")
}

func (s *staticStream) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

// videoPacket is a received RTP packet that starts a VP8 frame.
type videoPacket struct {
	at        time.Time
	timestamp uint32
	keyframe  bool
}

func TestBrowserKeyframeRequestRestoresAnIdleStream(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	hub := session.NewHub(session.Options{
		Dispatcher: input.NewDispatcher(&testutil.RecordingInjector{}),
		Source: func(context.Context, uint64) (media.FrameStream, error) {
			return &staticStream{closed: make(chan struct{})}, nil
		},
		Target:  fixedTarget{},
		Encoder: media.DefaultEncoderConfig("ffmpeg"),
	})
	server := httptest.NewServer(hub)
	t.Cleanup(server.Close)
	t.Cleanup(hub.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	browser := testutil.ConnectBrowser(t, "ws://"+host+"/ws", http.Header{"Origin": {"https://" + host}})

	var ssrc uint32
	frames := make(chan videoPacket, 256)
	select {
	case remote := <-browser.Tracks:
		ssrc = uint32(remote.SSRC())
		go func() {
			for {
				packet, _, err := remote.ReadRTP()
				if err != nil {
					return
				}
				var vp8 codecs.VP8Packet
				payload, err := vp8.Unmarshal(packet.Payload)
				if err != nil || vp8.S != 1 || vp8.PID != 0 || len(payload) == 0 {
					continue
				}
				frames <- videoPacket{at: time.Now(), timestamp: packet.Timestamp, keyframe: payload[0]&1 == 0}
			}
		}()
	case <-time.After(10 * time.Second):
		t.Fatal("no video track")
	}

	// The first frame is a keyframe; then the stream refreshes for a second
	// and slows to a heartbeat, leaving gaps longer than 700 ms.
	var last videoPacket
	received := false
	deadline := time.After(10 * time.Second)
quiet:
	for {
		select {
		case frame := <-frames:
			if !received && !frame.keyframe {
				t.Fatal("the stream did not start with a keyframe")
			}
			received, last = true, frame
		case <-time.After(700 * time.Millisecond):
			if received {
				break quiet
			}
		case <-deadline:
			t.Fatal("the stream did not start and go quiet")
		}
	}

	if err := browser.PC.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: ssrc}}); err != nil {
		t.Fatal(err)
	}
	// Heartbeat delta frames may arrive before the answer.
	answer := time.After(5 * time.Second)
	for {
		select {
		case frame := <-frames:
			// The RTP clock (90 kHz) must follow the wall clock across gaps.
			rtpGap := time.Duration(frame.timestamp-last.timestamp) * time.Second / 90000
			wallGap := frame.at.Sub(last.at)
			if diff := rtpGap - wallGap; diff < -250*time.Millisecond || diff > 250*time.Millisecond {
				t.Fatalf("RTP gap %s for wall gap %s", rtpGap, wallGap)
			}
			if frame.keyframe {
				return
			}
			last = frame
		case <-answer:
			t.Fatal("no keyframe after the browser's request")
		}
	}
}
