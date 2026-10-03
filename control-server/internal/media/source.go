// Package media streams the selected window to a browser: it captures frames,
// encodes them to VP9 with ffmpeg and delivers them over WebRTC, and carries
// the browser's control messages back over a data channel.
//
// The pipeline depends only on the small interfaces in this file, so it can be
// exercised without CaptureProbe or a network.
package media

import (
	"context"

	pionmedia "github.com/pion/webrtc/v4/pkg/media"

	"share-app-host/internal/capture"
)

// FrameStream is a running capture of one window. *capture.Stream implements it.
type FrameStream interface {
	// ReadFrameInto reads the next frame, reusing buffer when large enough.
	ReadFrameInto(buffer []byte) (capture.Frame, error)
	// Close stops the capture and unblocks a pending ReadFrameInto.
	Close() error
}

// Source starts capturing the window with the given handle. The capture stops
// when ctx is cancelled or the stream is closed.
type Source func(ctx context.Context, handle uint64) (FrameStream, error)

// ProbeSource captures windows with a CaptureProbe, optionally including
// secondary windows in each stream it opens.
func ProbeSource(probe *capture.Probe, includeSecondaryWindows bool) Source {
	return func(ctx context.Context, handle uint64) (FrameStream, error) {
		stream, err := probe.OpenStream(ctx, handle, includeSecondaryWindows)
		if err != nil {
			return nil, err // not stream: that would be a non-nil interface holding nil
		}
		return stream, nil
	}
}

// Target reports which window should be streamed and when that changes.
// *window.Selection implements it.
type Target interface {
	// State returns the selected handle and a channel that is closed when the
	// selection changes, read under one lock so no change can be missed.
	State() (handle uint64, selected bool, changed <-chan struct{})
}

// SampleWriter receives encoded video samples. pion's
// *webrtc.TrackLocalStaticSample implements it.
type SampleWriter interface {
	WriteSample(sample pionmedia.Sample) error
}
