package media

import (
	"context"

	"share-app-host/internal/capture"
)

// pumpBuffers is the number of pixel buffers in circulation: one being read
// into, one holding the newest queued frame, and one owned by the encoder.
const pumpBuffers = 3

// framePump reads frames from a stream on its own goroutine, so a slow encoder
// never stalls capture. Only the newest unconsumed frame is kept; an older one
// is dropped and its buffer recycled. Buffers are reused to avoid allocating a
// full frame (about 8 MB at 1080p) every time.
//
// Ownership rule: a buffer is touched by exactly one party at a time. The
// reader owns it until it queues the frame, the consumer owns it from next()
// until recycle(), and the pump never writes into a buffer it has handed out.
type framePump struct {
	frames  chan capture.Frame // newest frame not yet taken (capacity 1)
	buffers chan []byte        // free buffers
	failed  chan error         // first read error, if the context was still live
	done    chan struct{}      // closed when the reader goroutine has exited
}

// startFramePump starts reading stream until ctx is done. A read error that is
// not caused by cancellation is kept for err() and cancels the pipeline via
// cancel, so the consumer loop wakes up.
func startFramePump(ctx context.Context, cancel context.CancelFunc, stream FrameStream) *framePump {
	p := &framePump{
		frames:  make(chan capture.Frame, 1),
		buffers: make(chan []byte, pumpBuffers),
		failed:  make(chan error, 1),
		done:    make(chan struct{}),
	}
	for i := 0; i < pumpBuffers; i++ {
		p.buffers <- nil
	}
	go p.run(ctx, cancel, stream)
	return p
}

func (p *framePump) run(ctx context.Context, cancel context.CancelFunc, stream FrameStream) {
	defer close(p.done)
	for {
		var buffer []byte
		select {
		case <-ctx.Done():
			return
		case buffer = <-p.buffers:
		}

		frame, err := stream.ReadFrameInto(buffer)
		if err != nil {
			if ctx.Err() == nil {
				p.failed <- err
				cancel()
			}
			return
		}

		// Replace a stale queued frame; never overwrite a buffer the encoder owns.
		select {
		case stale := <-p.frames:
			p.buffers <- stale.Data
		default:
		}
		select {
		case p.frames <- frame:
		case <-ctx.Done():
			return
		}
	}
}

// next returns the newest queued frame without blocking.
func (p *framePump) next() (capture.Frame, bool) {
	select {
	case frame := <-p.frames:
		return frame, true
	default:
		return capture.Frame{}, false
	}
}

// recycle returns a buffer obtained through next() once the consumer is done
// with it.
func (p *framePump) recycle(buffer []byte) {
	p.buffers <- buffer
}

// err returns the read error that stopped the pump, or nil.
func (p *framePump) err() error {
	select {
	case err := <-p.failed:
		return err
	default:
		return nil
	}
}

// wait blocks until the reader goroutine has exited. Close the stream first,
// or the reader may be blocked inside ReadFrameInto.
func (p *framePump) wait() {
	<-p.done
}
