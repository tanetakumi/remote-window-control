package nativecapture

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"share-app-host/internal/tailbuf"
	"share-app-host/internal/window"
	"sync"
	"time"
)

var ErrBridgeUnavailable = errors.New("native capture bridge unavailable")

const streamFrameHeaderSize = 24
const maxFrameBytes = 128 * 1024 * 1024

type Bridge struct{ probePath string }
type StreamFrame struct {
	Width, Height, Stride int
	FrameID               int64
	Data                  []byte
}
type StreamSession struct {
	command     *exec.Cmd
	stdout      *bufio.Reader
	pipe        io.ReadCloser
	stderr      tailbuf.Buffer
	done        chan struct{}
	waitErr     error
	cancel      context.CancelFunc
	closeOnce   sync.Once
	lastFrameID int64
	header      [streamFrameHeaderSize]byte
}

func NewBridge(baseDir string) *Bridge { return &Bridge{probePath: resolveProbePath(baseDir)} }
func resolveProbePath(baseDir string) string {
	candidates := []string{filepath.Join(baseDir, "CaptureProbe", "CaptureProbe.exe")}
	for _, configuration := range []string{"Debug", "Release"} {
		candidates = append(candidates, filepath.Join(baseDir, "window-capture", "apps", "CaptureProbe", "bin", configuration, "net10.0-windows10.0.19041.0", "win-x64", "CaptureProbe.exe"))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return candidates[0]
}
func (b *Bridge) output(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := os.Stat(b.probePath); err != nil {
		return nil, ErrBridgeUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, b.probePath, args...)
	command.WaitDelay = 2 * time.Second
	return command.Output()
}
func (b *Bridge) CaptureSnapshot(ctx context.Context, r SnapshotRequest) (SnapshotResult, error) {
	data, err := b.output(ctx, "--hwnd", fmt.Sprint(r.Handle), "--out", r.OutputPath)
	if err != nil {
		return SnapshotResult{}, err
	}
	var result SnapshotResult
	err = json.Unmarshal(data, &result)
	return result, err
}
func (b *Bridge) ListWindows(ctx context.Context) ([]window.Info, error) {
	data, err := b.output(ctx, "--list")
	if err != nil {
		return nil, err
	}
	var result []window.Info
	err = json.Unmarshal(data, &result)
	return result, err
}
func (b *Bridge) CapturePNG(ctx context.Context, handle uint64) ([]byte, error) {
	return b.output(ctx, "--hwnd", fmt.Sprint(handle), "--stdout-png")
}
func (b *Bridge) OpenStream(ctx context.Context, handle uint64) (*StreamSession, error) {
	if _, err := os.Stat(b.probePath); err != nil {
		return nil, ErrBridgeUnavailable
	}
	ctx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(ctx, b.probePath, "--stream", "--hwnd", fmt.Sprint(handle))
	command.WaitDelay = 2 * time.Second
	// Own the read end independently of exec.Cmd.Wait, so process exit cannot
	// truncate unread frame bytes by closing a StdoutPipe.
	stdout, writer, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	defer writer.Close()
	command.Stdout = writer
	session := &StreamSession{command: command, stdout: bufio.NewReader(stdout), pipe: stdout, done: make(chan struct{}), cancel: cancel}
	command.Stderr = &session.stderr
	if err := command.Start(); err != nil {
		cancel()
		_ = stdout.Close()
		return nil, err
	}
	go func() { session.waitErr = command.Wait(); close(session.done) }()
	return session, nil
}

// ReadFrameInto has a single reader. The caller owns Data until returning it to
// its buffer pool, and must not reuse it while the encoder is writing it.
func (s *StreamSession) ReadFrameInto(buffer []byte) (StreamFrame, error) {
	if _, err := io.ReadFull(s.stdout, s.header[:]); err != nil {
		return StreamFrame{}, s.wrapError(err)
	}
	frame, length, err := decodeFrameHeader(s.header[:], s.lastFrameID)
	if err != nil {
		return StreamFrame{}, err
	}
	if cap(buffer) < length {
		buffer = make([]byte, length)
	} else {
		buffer = buffer[:length]
	}
	if _, err := io.ReadFull(s.stdout, buffer); err != nil {
		return StreamFrame{}, s.wrapError(err)
	}
	s.lastFrameID = frame.FrameID
	frame.Data = buffer
	return frame, nil
}
func decodeFrameHeader(header []byte, lastID int64) (StreamFrame, int, error) {
	if len(header) != streamFrameHeaderSize {
		return StreamFrame{}, 0, fmt.Errorf("invalid capture header length")
	}
	length := uint64(binary.LittleEndian.Uint32(header[0:4]))
	frame := StreamFrame{Width: int(int32(binary.LittleEndian.Uint32(header[4:8]))), Height: int(int32(binary.LittleEndian.Uint32(header[8:12]))), Stride: int(int32(binary.LittleEndian.Uint32(header[12:16]))), FrameID: int64(binary.LittleEndian.Uint64(header[16:24]))}
	if frame.Width <= 0 || frame.Height <= 0 || frame.Width > 16384 || frame.Height > 16384 || frame.Stride != frame.Width*4 || length != uint64(frame.Stride)*uint64(frame.Height) || length > maxFrameBytes || frame.FrameID <= lastID {
		return StreamFrame{}, 0, fmt.Errorf("invalid capture frame header")
	}
	return frame, int(length), nil
}
func (s *StreamSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() { s.cancel(); _ = s.pipe.Close(); <-s.done })
	var exitErr *exec.ExitError
	if errors.As(s.waitErr, &exitErr) {
		return nil
	}
	return s.waitErr
}
func (s *StreamSession) wrapError(err error) error {
	if stderr := s.stderr.String(); stderr != "" {
		return fmt.Errorf("%w: %s", err, stderr)
	}
	return err
}
