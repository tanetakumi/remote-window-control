package session_test

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"share-app-host/internal/capture"
	"share-app-host/internal/media"
	"share-app-host/internal/session"
)

type selectedModeTarget struct{}

func (selectedModeTarget) State() (uint64, bool, <-chan struct{}) {
	return 42, true, make(chan struct{})
}

type modeStream struct {
	stop    chan struct{}
	once    sync.Once
	id      int64
	stalled bool
}

func (s *modeStream) ReadFrameInto(buffer []byte) (capture.Frame, error) {
	if s.stalled {
		<-s.stop
		return capture.Frame{}, errors.New("closed")
	}
	select {
	case <-s.stop:
		return capture.Frame{}, errors.New("closed")
	case <-time.After(20 * time.Millisecond):
	}
	s.id++
	if cap(buffer) < 64*64*4 {
		buffer = make([]byte, 64*64*4)
	}
	return capture.Frame{Width: 64, Height: 64, Stride: 256, ID: s.id, Data: buffer[:64*64*4]}, nil
}
func (s *modeStream) Close() error { s.once.Do(func() { close(s.stop) }); return nil }
func modeSource(count *atomic.Int32, stalled bool) media.Source {
	return func(context.Context, uint64) (media.FrameStream, error) {
		count.Add(1)
		return &modeStream{stop: make(chan struct{}), stalled: stalled}, nil
	}
}

func TestInputModeRoundTripConfirmsNewCaptureAndSameModeDoesNotRestart(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	var windowOpens, pcOpens, prepared atomic.Int32
	f := newFixtureWith(t, session.Options{Source: modeSource(&windowOpens, false), PCSource: modeSource(&pcOpens, false), Target: selectedModeTarget{}, PreparePC: func(context.Context) error { prepared.Add(1); return nil }})
	b := f.browser(t)
	for _, mode := range []string{"pc", "pc", "window"} {
		b.SendInput(t, map[string]string{"type": "input.mode", "mode": mode})
		if reply := b.NextReply(t); reply.Type != "input.mode" || reply.Mode != mode {
			t.Fatalf("reply=%+v", reply)
		}
	}
	if pcOpens.Load() != 1 || windowOpens.Load() != 2 || prepared.Load() != 1 {
		t.Fatalf("window=%d pc=%d prepare=%d", windowOpens.Load(), pcOpens.Load(), prepared.Load())
	}
	b.SendInput(t, map[string]string{"type": "input.text", "text": "window again"})
	f.waitForEvents(t, 1)
}
func TestDisconnectCancelsPCPreparationAndJoinsWorker(t *testing.T) {
	var windowOpens, pcOpens atomic.Int32
	started, stopped := make(chan struct{}), make(chan struct{})
	f := newFixtureWith(t, session.Options{Source: modeSource(&windowOpens, true), PCSource: modeSource(&pcOpens, true), Target: selectedModeTarget{}, PreparePC: func(ctx context.Context) error { close(started); <-ctx.Done(); close(stopped); return ctx.Err() }})
	b := f.browser(t)
	b.SendInput(t, map[string]string{"type": "input.mode", "mode": "pc"})
	select {
	case <-started:
	case <-time.After(wait):
		t.Fatal("mode worker not started")
	}
	_ = b.WS.Close()
	f.hub.Close()
	select {
	case <-stopped:
	default:
		t.Fatal("hub did not join mode worker")
	}
	if pcOpens.Load() != 0 {
		t.Fatal("PC capture restarted after disconnect")
	}
}

func TestPCPreparationFinishesBeforeCaptureRestarts(t *testing.T) {
	var windowOpens, pcOpens atomic.Int32
	started, resume := make(chan struct{}), make(chan struct{})
	f := newFixtureWith(t, session.Options{
		Source: modeSource(&windowOpens, true), PCSource: modeSource(&pcOpens, true), Target: selectedModeTarget{},
		PreparePC: func(ctx context.Context) error {
			close(started)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-resume:
				return nil
			}
		},
	})
	b := f.browser(t)
	b.SendInput(t, map[string]string{"type": "input.mode", "mode": "pc"})
	select {
	case <-started:
	case <-time.After(wait):
		t.Fatal("preparation not started")
	}
	if pcOpens.Load() != 0 {
		t.Fatal("PC capture started before preparation finished")
	}
	close(resume)
	deadline := time.After(wait)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for pcOpens.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("PC capture did not start after preparation")
		case <-ticker.C:
		}
	}
}
func TestPCCaptureFailureEndsConnection(t *testing.T) {
	var windowOpens atomic.Int32
	f := newFixtureWith(t, session.Options{Source: modeSource(&windowOpens, true), PCSource: func(context.Context, uint64) (media.FrameStream, error) { return nil, errors.New("PC capture failed") }, Target: selectedModeTarget{}})
	b := f.browser(t)
	b.SendInput(t, map[string]string{"type": "input.mode", "mode": "pc"})
	if reply := b.NextReply(t); reply.Type != "error" {
		t.Fatalf("capture failure did not end connection: %+v", reply)
	}
}
