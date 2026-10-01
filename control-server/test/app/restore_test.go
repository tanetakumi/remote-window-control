package app_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"share-app-host/internal/app"
	"share-app-host/internal/capture"
	"share-app-host/internal/media"
	"share-app-host/internal/win32"
)

type nopStream struct{}

func (nopStream) ReadFrameInto([]byte) (capture.Frame, error) { return capture.Frame{}, io.EOF }
func (nopStream) Close() error                                { return nil }

// recordingSource records the order of restore and capture calls.
type recordingSource struct {
	calls []string
}

func (r *recordingSource) source(_ context.Context, handle uint64) (media.FrameStream, error) {
	r.calls = append(r.calls, "open")
	return nopStream{}, nil
}

func (r *recordingSource) restorer(restored bool, err error) app.Restorer {
	return func(uint64) (bool, error) {
		r.calls = append(r.calls, "restore")
		return restored, err
	}
}

func TestRestoringSourceRestoresBeforeCapturing(t *testing.T) {
	for _, restored := range []bool{false, true} {
		rec := &recordingSource{}
		source := app.RestoringSource(rec.source, rec.restorer(restored, nil))
		stream, err := source(context.Background(), 42)
		if err != nil || stream == nil {
			t.Fatalf("restored=%v: source = %v, %v; want a stream", restored, stream, err)
		}
		if got := len(rec.calls); got != 2 || rec.calls[0] != "restore" || rec.calls[1] != "open" {
			t.Errorf("restored=%v: calls = %v, want [restore open]", restored, rec.calls)
		}
	}
}

func TestRestoringSourceFailsWhenWindowStaysMinimized(t *testing.T) {
	rec := &recordingSource{}
	source := app.RestoringSource(rec.source, rec.restorer(false, win32.ErrWindowMinimized))
	stream, err := source(context.Background(), 42)
	if !errors.Is(err, win32.ErrWindowMinimized) {
		t.Fatalf("error = %v, want win32.ErrWindowMinimized", err)
	}
	if stream != nil {
		t.Errorf("stream = %v, want nil", stream)
	}
	if len(rec.calls) != 1 {
		t.Errorf("calls = %v, want only the restore attempt", rec.calls)
	}
}

func TestRestoringSourceCapturesDespiteOtherRestoreErrors(t *testing.T) {
	rec := &recordingSource{}
	source := app.RestoringSource(rec.source, rec.restorer(false, win32.ErrUnsupported))
	if _, err := source(context.Background(), 42); err != nil {
		t.Fatalf("error = %v, want the capture to be attempted", err)
	}
	if len(rec.calls) != 2 {
		t.Errorf("calls = %v, want [restore open]", rec.calls)
	}
}
