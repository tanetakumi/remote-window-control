// Package capture is the host's client for CaptureProbe, the native helper
// that lists windows and captures them with Windows Graphics Capture.
//
// One-shot commands (window list, PNG snapshot) run the helper to completion
// under a timeout. Streaming keeps one long-lived helper per target window.
package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"share-app-host/internal/window"
)

// ErrUnavailable is returned when the CaptureProbe executable does not exist.
var ErrUnavailable = errors.New("capture probe unavailable")

const (
	// commandTimeout bounds one-shot helper invocations.
	commandTimeout = 10 * time.Second
	// killDelay is how long a cancelled helper gets to exit before its
	// pipes are forcibly closed.
	killDelay = 2 * time.Second
)

// Probe runs the CaptureProbe executable.
type Probe struct {
	path string
}

// NewProbe returns a Probe for the CaptureProbe executable at path. The file
// is only checked when a command runs, so it may be installed after startup.
func NewProbe(path string) *Probe {
	return &Probe{path: path}
}

// ListWindows returns the windows that can be captured.
func (p *Probe) ListWindows(ctx context.Context) ([]window.Info, error) {
	data, err := p.output(ctx, "--list")
	if err != nil {
		return nil, err
	}
	var windows []window.Info
	if err := json.Unmarshal(data, &windows); err != nil {
		return nil, fmt.Errorf("decode window list: %w", err)
	}
	if windows == nil {
		windows = []window.Info{}
	}
	return windows, nil
}

// CapturePNG captures one frame of the window and returns it PNG-encoded.
func (p *Probe) CapturePNG(ctx context.Context, handle uint64) ([]byte, error) {
	return p.output(ctx, "--hwnd", fmt.Sprint(handle), "--stdout-png")
}

// output runs the helper to completion and returns its stdout.
func (p *Probe) output(ctx context.Context, args ...string) ([]byte, error) {
	if err := p.check(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, p.path, args...)
	command.WaitDelay = killDelay
	return command.Output()
}

func (p *Probe) check() error {
	if _, err := os.Stat(p.path); err != nil {
		return ErrUnavailable
	}
	return nil
}
