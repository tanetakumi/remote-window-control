package media_test

import (
	"bytes"
	"context"
	"log"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"share-app-host/internal/media"
)

// lockedBuffer is a log destination that tests can read while the pipeline
// writes to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog redirects the standard logger, which NewPipeline uses, for the
// duration of the test.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buffer := &lockedBuffer{}
	writer, flags := log.Writer(), log.Flags()
	log.SetOutput(buffer)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(writer); log.SetFlags(flags) })
	return buffer
}

func field(t *testing.T, line, name string) int {
	t.Helper()
	match := regexp.MustCompile(`\b` + name + `=(\d+)`).FindStringSubmatch(line)
	if match == nil {
		t.Fatalf("%q has no %s", line, name)
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestStatsIntervalLogsEncodedBytesAndKeyframes(t *testing.T) {
	requireFFmpeg(t)
	logs := captureLog(t)
	sink := &recordingSink{}
	pipeline := media.NewPipeline(newFakeSource(steady(fakeFrameSize)).source(), newFakeTarget(), sink, defaultEncoder())
	pipeline.StatsInterval = 300 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{})
	done := runWindow(ctx, pipeline, 7, changed)

	statsLine := regexp.MustCompile(`media stats hwnd=7 .*`)
	var first string
	eventually(t, 10*time.Second, "a media stats line with samples", func() bool {
		for _, line := range statsLine.FindAllString(logs.String(), -1) {
			if regexp.MustCompile(`\bsamples=[1-9]`).MatchString(line) {
				first = line
				return true
			}
		}
		return false
	})
	// The first encoded frame is a keyframe, and keyframes are part of the total.
	if field(t, first, "keyframes") < 1 || field(t, first, "keyframe_bytes") > field(t, first, "bytes") {
		t.Fatalf("stats line %q", first)
	}

	eventually(t, 3*time.Second, "delta frames", func() bool { return sink.encoded() >= 4 })
	close(changed)
	if err := receive(t, done, 4*time.Second, "RunWindow to stop"); err != nil {
		t.Fatal(err)
	}
	stopped := regexp.MustCompile(`capture stopped hwnd=7 .*`).FindString(logs.String())
	if stopped == "" || field(t, stopped, "keyframes") != 1 || field(t, stopped, "encoded") < field(t, stopped, "samples") {
		t.Fatalf("stop line %q", stopped)
	}
	var total, keyframeBytes int
	for _, sample := range sink.snapshot() {
		total += len(sample.Data)
		if keyframe(sample.Data) {
			keyframeBytes += len(sample.Data)
		}
	}
	if field(t, stopped, "bytes") != total || field(t, stopped, "keyframe_bytes") != keyframeBytes {
		t.Fatalf("VP9 byte counters do not match the samples: %q", stopped)
	}
}

func TestStatsAreNotLoggedByDefault(t *testing.T) {
	requireFFmpeg(t)
	logs := captureLog(t)
	sink := &recordingSink{}
	pipeline := media.NewPipeline(newFakeSource(steady(fakeFrameSize)).source(), newFakeTarget(), sink, defaultEncoder())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{})
	done := runWindow(ctx, pipeline, 1, changed)
	eventually(t, 10*time.Second, "encoded samples", func() bool { return sink.count() > 3 })
	close(changed)
	if err := receive(t, done, 4*time.Second, "RunWindow to stop"); err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`media stats`).MatchString(logs.String()) {
		t.Fatalf("stats logged without an interval:\n%s", logs.String())
	}
}
