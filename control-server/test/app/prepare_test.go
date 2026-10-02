package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"

	"share-app-host/internal/app"
	"share-app-host/internal/capture"
	"share-app-host/internal/media"
	"share-app-host/internal/win32"
)

type nopStream struct{}

func (nopStream) ReadFrameInto([]byte) (capture.Frame, error) { return capture.Frame{}, io.EOF }
func (nopStream) Close() error                                { return nil }

// recordingSource records which target is prepared before each capture opens.
type recordingSource struct {
	calls []string
}

func (r *recordingSource) source(_ context.Context, handle uint64) (media.FrameStream, error) {
	r.calls = append(r.calls, fmt.Sprintf("open:%d", handle))
	return nopStream{}, nil
}

func (r *recordingSource) preparer(err error) func(context.Context, uint64) error {
	return func(_ context.Context, handle uint64) error {
		r.calls = append(r.calls, fmt.Sprintf("prepare:%d", handle))
		return err
	}
}

func TestPreparingSourcePreparesEachTargetAndReconnection(t *testing.T) {
	rec := &recordingSource{}
	source := app.PreparingSource(rec.source, rec.preparer(nil))
	for _, handle := range []uint64{42, 43, 42} {
		stream, err := source(context.Background(), handle)
		if err != nil || stream == nil {
			t.Fatalf("handle=%d: source = %v, %v; want a stream", handle, stream, err)
		}
	}
	if want := []string{"prepare:42", "open:42", "prepare:43", "open:43", "prepare:42", "open:42"}; !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

func TestPreparingSourceFailsWhenWindowStaysMinimized(t *testing.T) {
	rec := &recordingSource{}
	source := app.PreparingSource(rec.source, rec.preparer(win32.ErrWindowMinimized))
	stream, err := source(context.Background(), 42)
	if !errors.Is(err, win32.ErrWindowMinimized) {
		t.Fatalf("error = %v, want win32.ErrWindowMinimized", err)
	}
	if stream != nil {
		t.Errorf("stream = %v, want nil", stream)
	}
	if len(rec.calls) != 1 {
		t.Errorf("calls = %v, want only the preparation attempt", rec.calls)
	}
}

func TestPreparingSourceCapturesDespiteForegroundAndOtherErrors(t *testing.T) {
	for _, prepareErr := range []error{win32.ErrForegroundDenied, win32.ErrUnsupported} {
		t.Run(prepareErr.Error(), func(t *testing.T) {
			rec := &recordingSource{}
			source := app.PreparingSource(rec.source, rec.preparer(prepareErr))
			if stream, err := source(context.Background(), 42); err != nil || stream == nil {
				t.Fatalf("source = %v, %v, want the capture to be attempted", stream, err)
			}
			if want := []string{"prepare:42", "open:42"}; !reflect.DeepEqual(rec.calls, want) {
				t.Errorf("calls = %v, want %v", rec.calls, want)
			}
		})
	}
}

func TestPreparingSourceSkipsCancelledCapture(t *testing.T) {
	for _, cancelBefore := range []bool{true, false} {
		t.Run(fmt.Sprintf("cancelBefore=%v", cancelBefore), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelBefore {
				cancel()
			}
			prepared := false
			source := app.PreparingSource(func(context.Context, uint64) (media.FrameStream, error) {
				t.Fatal("cancelled capture must not open")
				return nil, nil
			}, func(prepareCtx context.Context, _ uint64) error {
				prepared = true
				if prepareCtx != ctx {
					t.Fatal("preparation must receive the capture context")
				}
				cancel()
				return nil
			})
			stream, err := source(ctx, 42)
			if stream != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("source = %v, %v, want nil and context.Canceled", stream, err)
			}
			if prepared == cancelBefore {
				t.Fatalf("prepared = %v, cancelBefore = %v", prepared, cancelBefore)
			}
		})
	}
}
