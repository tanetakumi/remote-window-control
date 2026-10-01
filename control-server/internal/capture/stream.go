package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"share-app-host/internal/tailbuf"
)

// Stream is a running CaptureProbe process that emits frames of one window.
type Stream struct {
	reader *FrameReader
	pipe   io.ReadCloser
	stderr tailbuf.Buffer

	cancel    context.CancelFunc
	done      chan struct{} // closed once the process has been waited for
	waitErr   error         // valid after done is closed
	closeOnce sync.Once
}

// OpenStream starts a helper streaming the window with the given handle. The
// helper stops when ctx is cancelled or Close is called.
func (p *Probe) OpenStream(ctx context.Context, handle uint64) (*Stream, error) {
	if err := p.check(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(ctx, p.path, "--stream", "--hwnd", fmt.Sprint(handle))
	command.WaitDelay = killDelay

	// Own the read end independently of exec.Cmd.Wait, so process exit cannot
	// truncate unread frame bytes by closing a StdoutPipe.
	pipe, writer, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	defer writer.Close()
	command.Stdout = writer

	s := &Stream{
		reader: NewFrameReader(pipe),
		pipe:   pipe,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	command.Stderr = &s.stderr
	if err := command.Start(); err != nil {
		cancel()
		_ = pipe.Close()
		return nil, err
	}
	go func() {
		s.waitErr = command.Wait()
		close(s.done)
	}()
	return s, nil
}

// ReadFrameInto reads the next frame into buffer (see FrameReader.ReadInto).
// It has a single reader. Errors include the helper's recent stderr output.
func (s *Stream) ReadFrameInto(buffer []byte) (Frame, error) {
	frame, err := s.reader.ReadInto(buffer)
	if err != nil {
		return Frame{}, s.withDiagnostics(err)
	}
	return frame, nil
}

// Close stops the helper and waits for it to exit. It is safe to call from
// several goroutines and unblocks a pending ReadFrameInto. A non-zero exit
// status after being stopped is not reported as an error.
func (s *Stream) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.pipe.Close()
		<-s.done
	})
	var exitErr *exec.ExitError
	if errors.As(s.waitErr, &exitErr) {
		return nil
	}
	return s.waitErr
}

// diagnosticsWait is how long an error report waits for the helper to exit, so
// the stderr it wrote just before dying has been collected.
const diagnosticsWait = 500 * time.Millisecond

func (s *Stream) withDiagnostics(err error) error {
	select {
	case <-s.done:
	case <-time.After(diagnosticsWait):
	}
	if tail := s.stderr.String(); tail != "" {
		return fmt.Errorf("%w: %s", err, tail)
	}
	return err
}
