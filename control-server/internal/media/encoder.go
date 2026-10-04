package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"share-app-host/internal/hostlog"
	"share-app-host/internal/tailbuf"
	"share-app-host/internal/win32"
)

// Fixed defaults for low-latency window streaming; see the README.
const (
	DefaultFPS     = 8
	DefaultBitrate = "6M"
	DefaultCRF     = 31

	// keyframeInterval is the libvpx-vp9 keyframe interval in frames: large
	// enough that keyframes come only from encoder starts, which also answer
	// browser keyframe requests (see Pipeline). A periodic keyframe of a text
	// screen costs hundreds of kilobytes, a static delta frame a few hundred
	// bytes.
	keyframeInterval = 1_000_000

	// encoderStopWait is how long a closing encoder may take to flush and exit
	// after its input ends, before it is killed.
	encoderStopWait = 2 * time.Second
)

// EncoderConfig describes the ffmpeg VP9 encoder.
type EncoderConfig struct {
	// FFmpegPath is the ffmpeg executable; a bare name is looked up on PATH.
	FFmpegPath string
	// FPS is the encode rate while the window changes; see Pipeline.
	FPS int
	// Bitrate is the target bitrate in ffmpeg notation, such as "6M".
	Bitrate string
	// CRF is the constant-rate-factor quality (lower is better).
	CRF int
}

// DefaultEncoderConfig returns the baseline settings using the given ffmpeg.
func DefaultEncoderConfig(ffmpegPath string) EncoderConfig {
	return EncoderConfig{FFmpegPath: ffmpegPath, FPS: DefaultFPS, Bitrate: DefaultBitrate, CRF: DefaultCRF}
}

// Args returns the ffmpeg arguments for raw BGRA frames of the given size read
// from stdin and an IVF/VP9 stream written to stdout.
func (c EncoderConfig) Args(width, height int) []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "rawvideo", "-pix_fmt", "bgra",
		"-video_size", fmt.Sprintf("%dx%d", width, height),
		"-framerate", strconv.Itoa(c.FPS),
		"-i", "pipe:0",
		"-an",
		"-vf", "format=yuv420p",
		"-c:v", "libvpx-vp9",
		"-threads", "8",
		"-profile:v", "0",
		"-b:v", c.Bitrate,
		"-crf", strconv.Itoa(c.CRF),
		"-deadline", "realtime",
		"-cpu-used", "4",
		"-auto-alt-ref", "0",
		"-lag-in-frames", "0",
		"-row-mt", "1",
		"-tune-content", "screen",
		"-g", strconv.Itoa(keyframeInterval),
		"-f", "ivf", "pipe:1",
	}
}

// encoder is one running ffmpeg process for a fixed frame size. Raw frames go
// in through WriteFrame; encoded samples are written to the sink by a
// background goroutine.
type encoder struct {
	width, height int

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cancel context.CancelFunc
	stderr tailbuf.Buffer

	consumeDone chan struct{} // closed when stdout has been fully consumed
	waitDone    chan struct{} // closed when the process has been waited for
	consumeErr  error         // valid after consumeDone is closed
	waitErr     error         // valid after waitDone is closed

	closeOnce sync.Once
	closeErr  error
}

// startEncoder launches ffmpeg for frames of width x height and forwards its
// output to sink. The process stops when ctx is cancelled or Close is called.
func startEncoder(parent context.Context, cfg EncoderConfig, sink SampleWriter, width, height int) (*encoder, error) {
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, cfg.FFmpegPath, cfg.Args(width, height)...)
	win32.HideConsole(cmd)
	cmd.WaitDelay = 2 * time.Second

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, err
	}
	e := &encoder{
		width: width, height: height,
		cmd: cmd, stdin: stdin, stdout: stdout, cancel: cancel,
		consumeDone: make(chan struct{}),
		waitDone:    make(chan struct{}),
	}
	cmd.Stderr = io.MultiWriter(&e.stderr, hostlog.Stderr{Label: "ffmpeg"})
	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := win32.KillWithHost(cmd.Process); err != nil {
		log.Printf("ffmpeg may outlive the host: %v", err)
	}

	go func() {
		e.consumeErr = consumeIVF(sink, stdout)
		close(e.consumeDone)
		if e.consumeErr != nil {
			cancel()
		}
		// Consume stdout before Wait, as exec.Cmd.StdoutPipe requires.
		e.waitErr = cmd.Wait()
		close(e.waitDone)
	}()
	return e, nil
}

// matches reports whether e encodes frames of the given size.
func (e *encoder) matches(width, height int) bool {
	return e != nil && e.width == width && e.height == height
}

// WriteFrame feeds one raw frame to the encoder.
func (e *encoder) WriteFrame(frame []byte) error {
	select {
	case <-e.consumeDone:
		return fmt.Errorf("encoder stopped: %v: %s", e.consumeErr, e.stderr.String())
	default:
	}
	n, err := e.stdin.Write(frame)
	if err != nil {
		return fmt.Errorf("%w: %s", err, e.stderr.String())
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}

// Close ends the input, gives ffmpeg a moment to flush and exit, then kills it.
// It is idempotent and safe on a nil encoder.
func (e *encoder) Close() error {
	if e == nil {
		return nil
	}
	e.closeOnce.Do(func() {
		_ = e.stdin.Close()
		timer := time.NewTimer(encoderStopWait)
		defer timer.Stop()
		select {
		case <-e.waitDone:
		case <-timer.C:
			e.cancel()
			_ = e.stdout.Close()
			<-e.waitDone
		}
		e.cancel()
		if e.consumeErr != nil && !errors.Is(e.consumeErr, io.EOF) {
			e.closeErr = e.consumeErr
		}
	})
	return e.closeErr
}

// closeEncoder closes e and logs, rather than returns, a failure: the caller
// is already tearing the stream down.
func closeEncoder(e *encoder) {
	if err := e.Close(); err != nil {
		log.Printf("encoder close: %v", err)
	}
}
