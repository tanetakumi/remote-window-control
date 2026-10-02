package media_test

import (
	"context"
	"testing"
	"time"

	"share-app-host/internal/media"
)

// refreshSettle is comfortably longer than the one-second refresh after a
// change, so the pipeline has slowed to its heartbeat.
const refreshSettle = 1500 * time.Millisecond

// startStreaming runs a pipeline on read and returns its sink.
func startStreaming(t *testing.T, read readFunc) (*media.Pipeline, *recordingSink) {
	t.Helper()
	requireFFmpeg(t)
	sink := &recordingSink{}
	pipeline := media.NewPipeline(newFakeSource(read).source(), newFakeTarget(), sink, defaultEncoder())
	ctx, cancel := context.WithCancel(context.Background())
	done := runWindow(ctx, pipeline, 1, make(chan struct{}))
	t.Cleanup(func() {
		cancel()
		if err := receive(t, done, settle, "the pipeline to stop"); err != nil {
			t.Error(err)
		}
	})
	return pipeline, sink
}

// idle waits for the refresh to end, returns the number of samples it
// produced, and checks that the stream then only carries a heartbeat: about
// one delta frame per second, never more than a second and a bit apart.
func idle(t *testing.T, sink *recordingSink) int {
	t.Helper()
	eventually(t, 10*time.Second, "the first encoded sample", func() bool { return sink.encoded() > 0 })
	time.Sleep(refreshSettle)
	count := sink.encoded()
	from := time.Now()
	time.Sleep(3200 * time.Millisecond)

	var previous time.Time
	beats := 0
	for _, sample := range sink.snapshot() {
		if len(sample.Data) == 0 || sample.Timestamp.Before(from) {
			if len(sample.Data) > 0 {
				previous = sample.Timestamp
			}
			continue
		}
		beats++
		if keyframe(sample.Data) {
			t.Fatal("a heartbeat was a keyframe")
		}
		if gap := sample.Timestamp.Sub(previous); gap > 1300*time.Millisecond {
			t.Fatalf("heartbeat gap %s", gap)
		}
		previous = sample.Timestamp
	}
	if beats < 2 || beats > 4 {
		t.Fatalf("%d samples in 3.2 s while idle, want a heartbeat about once a second", beats)
	}
	return count
}

func TestStaticWindowSlowsToAHeartbeatAfterARefresh(t *testing.T) {
	_, sink := startStreaming(t, singleFrame(fakeFrameSize))
	// One change is encoded for a second at DefaultFPS, give or take a tick.
	if count := idle(t, sink); count < media.DefaultFPS-2 || count > media.DefaultFPS+2 {
		t.Fatalf("a single change produced %d samples", count)
	}
}

func TestFramesWithoutPixelChangesKeepTheHeartbeat(t *testing.T) {
	// steady keeps delivering new frames, but their pixels never change.
	_, sink := startStreaming(t, steady(fakeFrameSize))
	idle(t, sink)
}

func TestChangingWindowIsEncodedAtTheFullRateWithOneKeyframe(t *testing.T) {
	_, sink := startStreaming(t, changing(fakeFrameSize))
	eventually(t, 10*time.Second, "the first encoded sample", func() bool { return sink.encoded() > 0 })
	time.Sleep(refreshSettle)
	count := sink.encoded()
	eventually(t, 2*time.Second, "encoding to continue", func() bool { return sink.encoded() > count+3 })
	// Keyframes are not periodic: only the encoder's first frame is one.
	eventually(t, 5*time.Second, "25 encoded frames", func() bool { return sink.encoded() >= 25 })
	if keyframes := sink.keyframes(); keyframes != 1 {
		t.Fatalf("%d keyframes from one encoder", keyframes)
	}
}
