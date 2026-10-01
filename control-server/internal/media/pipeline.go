package media

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Pipeline captures the selected window, encodes it and writes the samples to
// a sink. It follows the target: when another window is selected it stops the
// old capture immediately, without waiting for another frame, and starts the
// new one.
type Pipeline struct {
	source  Source
	target  Target
	sink    SampleWriter
	encoder EncoderConfig
}

// NewPipeline returns a Pipeline that streams the window chosen by target.
func NewPipeline(source Source, target Target, sink SampleWriter, encoder EncoderConfig) *Pipeline {
	if encoder.FPS <= 0 {
		encoder.FPS = DefaultFPS
	}
	return &Pipeline{source: source, target: target, sink: sink, encoder: encoder}
}

// Run streams until ctx is cancelled, waiting while no window is selected. It
// returns nil on cancellation and an error when a stream fails for a reason
// other than the target changing.
func (p *Pipeline) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		handle, selected, changed := p.target.State()
		if !selected || handle == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
				continue
			}
		}

		err := p.RunWindow(ctx, handle, changed)
		if err == nil || ctx.Err() != nil {
			continue
		}
		select {
		case <-changed:
			// The failure came from the target moving on; follow it.
		default:
			return fmt.Errorf("window stream failed: %w", err)
		}
	}
	return nil
}

// RunWindow streams one window until ctx is cancelled or changed is closed. It
// returns nil in both cases, and an error if capture or encoding fails.
//
// It owns one capture stream and one long-lived encoder per frame size; the
// encoder is restarted only when the window's size changes.
func (p *Pipeline) RunWindow(parent context.Context, handle uint64, changed <-chan struct{}) error {
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

	stream, err := p.source(ctx, handle)
	if err != nil {
		return err
	}
	pump := startFramePump(ctx, cancel, stream)

	var enc *encoder
	// Stop in dependency order: the encoder, then the stream (which unblocks
	// the pump's read), then wait for the pump goroutine.
	defer func() {
		cancel()
		closeEncoder(enc)
		_ = stream.Close()
		pump.wait()
	}()

	ticker := time.NewTicker(time.Second / time.Duration(p.encoder.FPS))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return pump.err()
		case <-ticker.C:
		}

		frame, ok := pump.next()
		if !ok {
			continue
		}
		if !enc.matches(frame.Width, frame.Height) {
			closeEncoder(enc)
			enc = nil
			if enc, err = startEncoder(ctx, p.encoder, p.sink, frame.Width, frame.Height); err != nil {
				return err
			}
		}
		err = enc.WriteFrame(frame.Data)
		pump.recycle(frame.Data)
		if err != nil {
			return err
		}
	}
}
