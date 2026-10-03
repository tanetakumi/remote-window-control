package media_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"share-app-host/internal/media"
)

const settle = 3 * time.Second

// runWindow starts RunWindow in the background and returns its result channel.
func runWindow(ctx context.Context, p *media.Pipeline, handle uint64, changed <-chan struct{}) <-chan error {
	done := make(chan error, 1)
	go func() { done <- p.RunWindow(ctx, handle, changed) }()
	return done
}

func TestTargetChangeCancelsStalledCapture(t *testing.T) {
	source := newFakeSource(stalled())
	pipeline := media.NewPipeline(source.source(), newFakeTarget(), &recordingSink{}, defaultEncoder())

	changed := make(chan struct{})
	done := runWindow(context.Background(), pipeline, 1, changed)
	eventually(t, settle, "the capture to open", func() bool { return len(source.openedHandles()) == 1 })

	close(changed)
	if err := receive(t, done, settle, "RunWindow to stop after a target change"); err != nil {
		t.Fatalf("RunWindow = %v, want nil", err)
	}
	if !source.stream(0).isClosed() {
		t.Fatal("capture stream was not closed")
	}
}

func TestContextCancellationStopsTheStream(t *testing.T) {
	source := newFakeSource(stalled())
	pipeline := media.NewPipeline(source.source(), newFakeTarget(), &recordingSink{}, defaultEncoder())

	ctx, cancel := context.WithCancel(context.Background())
	done := runWindow(ctx, pipeline, 1, make(chan struct{}))
	eventually(t, settle, "the capture to open", func() bool { return len(source.openedHandles()) == 1 })

	cancel()
	if err := receive(t, done, settle, "RunWindow to stop after cancellation"); err != nil {
		t.Fatalf("RunWindow = %v, want nil", err)
	}
	if !source.stream(0).isClosed() {
		t.Fatal("capture stream was not closed")
	}
}

func TestCaptureFailureIsReported(t *testing.T) {
	source := newFakeSource(failing(errCapture))
	pipeline := media.NewPipeline(source.source(), newFakeTarget(), &recordingSink{}, defaultEncoder())

	done := runWindow(context.Background(), pipeline, 1, make(chan struct{}))
	err := receive(t, done, settle, "RunWindow to report the failure")
	if !errors.Is(err, errCapture) {
		t.Fatalf("RunWindow = %v, want %v", err, errCapture)
	}
	if !source.stream(0).isClosed() {
		t.Fatal("capture stream was not closed after failing")
	}
}

func TestOpenFailureIsReported(t *testing.T) {
	source := newFakeSource(stalled())
	source.openErr = errors.New("probe missing")
	pipeline := media.NewPipeline(source.source(), newFakeTarget(), &recordingSink{}, defaultEncoder())

	err := receive(t, runWindow(context.Background(), pipeline, 1, make(chan struct{})), settle, "RunWindow")
	if !errors.Is(err, source.openErr) {
		t.Fatalf("RunWindow = %v, want %v", err, source.openErr)
	}
}

func TestRunWaitsForASelectionAndFollowsTargetChanges(t *testing.T) {
	source := newFakeSource(stalled())
	target := newFakeTarget()
	pipeline := media.NewPipeline(source.source(), target, &recordingSink{}, defaultEncoder())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- pipeline.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	if opened := source.openedHandles(); len(opened) != 0 {
		t.Fatalf("captured %v before any window was selected", opened)
	}

	target.Select(10)
	eventually(t, settle, "window 10 to be captured", func() bool { return len(source.openedHandles()) == 1 })

	target.Select(20)
	eventually(t, settle, "window 20 to be captured", func() bool { return len(source.openedHandles()) == 2 })
	eventually(t, settle, "window 10's capture to be closed", func() bool { return source.stream(0).isClosed() })
	if source.stream(1).isClosed() {
		t.Fatal("the current capture was closed")
	}

	// Selecting the same window again is not a change.
	target.Select(20)
	time.Sleep(100 * time.Millisecond)
	if opened := source.openedHandles(); !reflect.DeepEqual(opened, []uint64{10, 20}) {
		t.Fatalf("opened %v", opened)
	}

	cancel()
	if err := receive(t, done, settle, "Run to stop"); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if !source.stream(1).isClosed() {
		t.Fatal("capture of window 20 was not closed on shutdown")
	}
}

func TestRunStopsWhileWaitingForASelection(t *testing.T) {
	pipeline := media.NewPipeline(newFakeSource(stalled()).source(), newFakeTarget(), &recordingSink{}, defaultEncoder())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- pipeline.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := receive(t, done, settle, "Run to stop"); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

func TestRunReportsAStreamFailure(t *testing.T) {
	target := newFakeTarget()
	target.Select(1)
	pipeline := media.NewPipeline(newFakeSource(failing(errCapture)).source(), target, &recordingSink{}, defaultEncoder())

	done := make(chan error, 1)
	go func() { done <- pipeline.Run(context.Background()) }()
	err := receive(t, done, settle, "Run to report the failure")
	if !errors.Is(err, errCapture) || !strings.Contains(err.Error(), "window stream failed") {
		t.Fatalf("Run = %v", err)
	}
}

func TestStreamingEncodesFramesAndShutsDownOnTargetChange(t *testing.T) {
	requireFFmpeg(t)
	for iteration := 0; iteration < 3; iteration++ {
		source := newFakeSource(steady(fakeFrameSize))
		sink := &recordingSink{}
		pipeline := media.NewPipeline(source.source(), newFakeTarget(), sink, defaultEncoder())

		ctx, cancel := context.WithCancel(context.Background())
		changed := make(chan struct{})
		done := runWindow(ctx, pipeline, 1, changed)

		eventually(t, 10*time.Second, "the first encoded sample", func() bool { return sink.encoded() > 0 })

		close(changed)
		if err := receive(t, done, 4*time.Second, "streaming workers to stop after a target change"); err != nil {
			cancel()
			t.Fatalf("iteration %d: RunWindow = %v", iteration, err)
		}
		cancel()
		if !source.stream(0).isClosed() {
			t.Fatalf("iteration %d: capture stream was not closed", iteration)
		}
	}
}

func TestEncoderFailureIsReported(t *testing.T) {
	source := newFakeSource(steady(fakeFrameSize))
	cfg := media.DefaultEncoderConfig("this-ffmpeg-does-not-exist")
	pipeline := media.NewPipeline(source.source(), newFakeTarget(), &recordingSink{}, cfg)

	done := runWindow(context.Background(), pipeline, 1, make(chan struct{}))
	if err := receive(t, done, settle, "RunWindow to report the encoder failure"); err == nil {
		t.Fatal("a missing ffmpeg was not reported")
	}
	if !source.stream(0).isClosed() {
		t.Fatal("capture stream was not closed after the encoder failed")
	}
}

func TestRestartChangesSourceOnSameTargetAndConfirmsFirstSampleOnce(t *testing.T) {
	requireFFmpeg(t)
	old, next := newFakeSource(stalled()), newFakeSource(steady(fakeFrameSize))
	target := newFakeTarget()
	target.Select(42)
	sink := &recordingSink{}
	pipeline := media.NewPipeline(old.source(), target, sink, defaultEncoder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- pipeline.Run(ctx) }()
	eventually(t, settle, "old source", func() bool { return len(old.openedHandles()) == 1 })
	confirmed := make(chan int, 10)
	pipeline.Restart(next.source(), func() { confirmed <- sink.encoded() })
	count := receive(t, confirmed, 10*time.Second, "first sample confirmation")
	if count == 0 {
		t.Fatal("confirmed before writing sample")
	}
	if got := next.openedHandles(); !reflect.DeepEqual(got, []uint64{42}) {
		t.Fatal(got)
	}
	if !old.stream(0).isClosed() {
		t.Fatal("old capture not closed")
	}
	eventually(t, 3*time.Second, "more samples", func() bool { return sink.encoded() > count+1 })
	select {
	case <-confirmed:
		t.Fatal("first sample confirmed twice")
	default:
	}
	pipeline.Restart(old.source(), nil)
	eventually(t, settle, "original source restarted", func() bool { return len(old.openedHandles()) == 2 })
	cancel()
	if err := receive(t, done, settle, "shutdown"); err != nil {
		t.Fatal(err)
	}
}
