package webrtc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	pion "github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
	"share-app-host/internal/nativecapture"
	"share-app-host/internal/processio"
	"share-app-host/internal/targetwindow"
)

const streamFPS = 10

func attachWindowVideoTrack(ctx context.Context, pc *pion.PeerConnection, bridge *nativecapture.Bridge, targets *targetwindow.Manager, workers *sync.WaitGroup, onFailure func(error)) error {
	track, err := pion.NewTrackLocalStaticSample(pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8}, "video", "share-app")
	if err != nil {
		return err
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		return err
	}
	workers.Add(2)
	go func() {
		defer workers.Done()
		buffer := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(buffer); err != nil {
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		for pc.ConnectionState() != pion.PeerConnectionStateConnected {
			if ctx.Err() != nil {
				return
			}
			sleepContext(ctx, 50*time.Millisecond)
		}
		if err := streamSelectedWindow(ctx, bridge, targets, track); err != nil && ctx.Err() == nil {
			onFailure(err)
		}
	}()
	return nil
}
func streamSelectedWindow(ctx context.Context, bridge *nativecapture.Bridge, targets *targetwindow.Manager, track *pion.TrackLocalStaticSample) error {
	for ctx.Err() == nil {
		handle, ok, changed := targets.State()
		if !ok || handle == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
				continue
			}
		}
		err := streamTarget(ctx, bridge, track, handle, changed)
		if err != nil && ctx.Err() == nil {
			select {
			case <-changed:
				continue
			default:
				return fmt.Errorf("window stream failed: %w", err)
			}
		}
	}
	return nil
}

func streamTarget(parent context.Context, bridge *nativecapture.Bridge, track *pion.TrackLocalStaticSample, handle uint64, changed <-chan struct{}) error {
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
	stream, err := bridge.OpenStream(ctx, handle)
	if err != nil {
		return err
	}
	// Three buffers: the reader, the latest queued frame and the encoder.
	buffers := make(chan []byte, 3)
	for i := 0; i < 3; i++ {
		buffers <- nil
	}
	frames := make(chan nativecapture.StreamFrame, 1)
	readDone := make(chan struct{})
	readErr := make(chan error, 1)
	go func() {
		defer close(readDone)
		for {
			var buffer []byte
			select {
			case <-ctx.Done():
				return
			case buffer = <-buffers:
			}
			frame, err := stream.ReadFrameInto(buffer)
			if err != nil {
				if ctx.Err() == nil {
					readErr <- err
					cancel()
				}
				return
			}
			// Replace a stale queued frame; never overwrite a buffer owned by the encoder.
			select {
			case stale := <-frames:
				buffers <- stale.Data
			default:
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	var encoder *vp8EncoderSession
	defer func() { cancel(); closeEncoderSession(encoder); _ = stream.Close(); <-readDone }()
	timer := time.NewTicker(time.Second / streamFPS)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			select {
			case err := <-readErr:
				return err
			default:
				return nil
			}
		case <-timer.C:
			var frame nativecapture.StreamFrame
			select {
			case frame = <-frames:
			default:
				continue
			}
			if encoder == nil || !encoder.matches(frame.Width, frame.Height) {
				closeEncoderSession(encoder)
				encoder, err = newVP8EncoderSession(ctx, track, frame.Width, frame.Height)
				if err != nil {
					return err
				}
			}
			err = encoder.WriteFrame(frame.Data)
			buffers <- frame.Data
			if err != nil {
				return err
			}
		}
	}
}

type vp8EncoderSession struct {
	width, height         int
	command               *exec.Cmd
	stdin                 io.WriteCloser
	stdout                io.ReadCloser
	cancel                context.CancelFunc
	waitDone, consumeDone chan struct{}
	waitErr, consumeErr   error
	stderr                processio.Diagnostics
	closeOnce             sync.Once
	closeErr              error
}

func resolveFFmpeg() string {
	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), "ffmpeg.exe")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "ffmpeg"
}
func newVP8EncoderSession(parent context.Context, track *pion.TrackLocalStaticSample, width, height int) (*vp8EncoderSession, error) {
	ctx, cancel := context.WithCancel(parent)
	command := exec.CommandContext(ctx, resolveFFmpeg(), "-hide_banner", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "bgra", "-video_size", fmt.Sprintf("%dx%d", width, height), "-framerate", fmt.Sprint(streamFPS), "-i", "pipe:0", "-an", "-vf", "format=yuv420p", "-c:v", "libvpx", "-b:v", "6M", "-crf", "10", "-deadline", "realtime", "-cpu-used", "4", "-auto-alt-ref", "0", "-g", fmt.Sprint(streamFPS*2), "-f", "ivf", "pipe:1")
	command.WaitDelay = 2 * time.Second
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, err
	}
	session := &vp8EncoderSession{width: width, height: height, command: command, stdin: stdin, stdout: stdout, cancel: cancel, waitDone: make(chan struct{}), consumeDone: make(chan struct{})}
	command.Stderr = &session.stderr
	if err = command.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	go func() {
		session.consumeErr = consumeIVF(track, stdout)
		close(session.consumeDone)
		// Consume stdout before Wait, as required by exec.Cmd.StdoutPipe.
		if session.consumeErr != nil {
			cancel()
		}
		session.waitErr = command.Wait()
		close(session.waitDone)
	}()
	return session, nil
}
func consumeIVF(track *pion.TrackLocalStaticSample, stream io.Reader) error {
	ivf, header, err := ivfreader.NewWith(bufio.NewReader(stream))
	if err != nil {
		return err
	}
	if header.TimebaseNumerator == 0 || header.TimebaseDenominator == 0 {
		return fmt.Errorf("invalid IVF timebase")
	}
	duration := time.Second * time.Duration(header.TimebaseNumerator) / time.Duration(header.TimebaseDenominator)
	for {
		payload, _, err := ivf.ParseNextFrame()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = track.WriteSample(media.Sample{Data: payload, Duration: duration}); err != nil {
			return err
		}
	}
}
func (s *vp8EncoderSession) matches(width, height int) bool {
	return s != nil && s.width == width && s.height == height
}
func (s *vp8EncoderSession) WriteFrame(frame []byte) error {
	select {
	case <-s.consumeDone:
		return fmt.Errorf("encoder stopped: %v: %s", s.consumeErr, s.stderr.String())
	default:
	}
	n, err := s.stdin.Write(frame)
	if err != nil {
		return fmt.Errorf("%w: %s", err, s.stderr.String())
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}
func (s *vp8EncoderSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-s.waitDone:
		case <-timer.C:
			s.cancel()
			_ = s.stdout.Close()
			<-s.waitDone
		}
		s.cancel()
		if s.consumeErr != nil && !errors.Is(s.consumeErr, io.EOF) {
			s.closeErr = s.consumeErr
		}
	})
	return s.closeErr
}
func closeEncoderSession(encoder *vp8EncoderSession) {
	if encoder != nil {
		if err := encoder.Close(); err != nil {
			log.Printf("encoder close: %v", err)
		}
	}
}
func sleepContext(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
