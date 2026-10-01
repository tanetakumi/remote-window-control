package capture_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"share-app-host/internal/capture"
	"share-app-host/internal/window"
	"share-app-host/test/testutil"
)

func TestMissingProbeIsReportedAsUnavailable(t *testing.T) {
	probe := capture.NewProbe(filepath.Join(t.TempDir(), "CaptureProbe.exe"))
	ctx := context.Background()

	if _, err := probe.ListWindows(ctx); !errors.Is(err, capture.ErrUnavailable) {
		t.Errorf("ListWindows error = %v", err)
	}
	if _, err := probe.CapturePNG(ctx, 1); !errors.Is(err, capture.ErrUnavailable) {
		t.Errorf("CapturePNG error = %v", err)
	}
	if _, err := probe.OpenStream(ctx, 1); !errors.Is(err, capture.ErrUnavailable) {
		t.Errorf("OpenStream error = %v", err)
	}
}

func TestListWindowsDecodesTheProbeOutput(t *testing.T) {
	probe := capture.NewProbe(testutil.InstallFakeProbe(t))
	got, err := probe.ListWindows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []window.Info{{Handle: 1, Title: "Notepad", ProcessID: 42, ProcessName: "notepad", ClassName: "Notepad"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("windows = %+v, want %+v", got, want)
	}
}

func TestCapturePNGReturnsTheProbeOutput(t *testing.T) {
	probe := capture.NewProbe(testutil.InstallFakeProbe(t))
	data, err := probe.CapturePNG(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != testutil.FakePNG {
		t.Fatalf("data = %q", data)
	}
}

func TestOneShotCommandErrorsIncludeTheHelpersStderr(t *testing.T) {
	t.Setenv(testutil.EnvCommandFail, "1")
	probe := capture.NewProbe(testutil.InstallFakeProbe(t))

	_, err := probe.ListWindows(context.Background())
	if err == nil || !strings.Contains(err.Error(), testutil.FakeStderr) {
		t.Fatalf("ListWindows error = %v, want one carrying %q", err, testutil.FakeStderr)
	}
	_, err = probe.CapturePNG(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), testutil.FakeStderr) {
		t.Fatalf("CapturePNG error = %v, want one carrying %q", err, testutil.FakeStderr)
	}
}

func TestStreamDeliversFramesAndClosesCleanly(t *testing.T) {
	probe := capture.NewProbe(testutil.InstallFakeProbe(t))
	stream, err := probe.OpenStream(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	var buffer []byte
	var lastID int64
	for i := 0; i < 3; i++ {
		frame, err := stream.ReadFrameInto(buffer)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Width != testutil.FakeFrameSize || frame.Height != testutil.FakeFrameSize {
			t.Fatalf("frame size %dx%d", frame.Width, frame.Height)
		}
		if frame.ID <= lastID {
			t.Fatalf("frame id %d after %d", frame.ID, lastID)
		}
		lastID, buffer = frame.ID, frame.Data
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestCloseUnblocksAStalledReadAndIsSafeToCallConcurrently(t *testing.T) {
	t.Setenv(testutil.EnvStall, "1")
	probe := capture.NewProbe(testutil.InstallFakeProbe(t))
	stream, err := probe.OpenStream(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	readDone := make(chan error, 1)
	go func() {
		_, err := stream.ReadFrameInto(nil)
		readDone <- err
	}()

	var closers sync.WaitGroup
	for i := 0; i < 3; i++ {
		closers.Add(1)
		go func() {
			defer closers.Done()
			_ = stream.Close()
		}()
	}
	closers.Wait()

	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("stalled read returned without error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stalled reader survived Close")
	}
}

func TestCancellingTheContextStopsTheHelper(t *testing.T) {
	t.Setenv(testutil.EnvStall, "1")
	probe := capture.NewProbe(testutil.InstallFakeProbe(t))
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := probe.OpenStream(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	readDone := make(chan error, 1)
	go func() {
		_, err := stream.ReadFrameInto(nil)
		readDone <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("read succeeded after cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop the helper")
	}
}

func TestStreamErrorsIncludeTheHelpersStderr(t *testing.T) {
	t.Setenv(testutil.EnvStreamFail, "1")
	probe := capture.NewProbe(testutil.InstallFakeProbe(t))
	stream, err := probe.OpenStream(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	_, err = stream.ReadFrameInto(nil)
	if err == nil {
		t.Fatal("read from a failed helper succeeded")
	}
	if !strings.Contains(err.Error(), testutil.FakeStderr) {
		t.Fatalf("error %q does not carry the helper's stderr", err)
	}
}
