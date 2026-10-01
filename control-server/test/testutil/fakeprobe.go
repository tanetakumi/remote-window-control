package testutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Environment variables that steer a fake CaptureProbe process.
const (
	envFakeProbe = "RWC_FAKE_PROBE"
	// EnvStall makes the fake stream emit nothing and never exit by itself.
	EnvStall = "RWC_FAKE_PROBE_STALL"
	// EnvStreamFail makes the fake stream write to stderr and exit non-zero.
	EnvStreamFail = "RWC_FAKE_PROBE_STREAM_FAIL"
	// EnvCommandFail makes the fake one-shot commands write to stderr and exit non-zero.
	EnvCommandFail = "RWC_FAKE_PROBE_COMMAND_FAIL"
	// EnvArgsFile names a file the fake writes its arguments to, one per line.
	EnvArgsFile = "RWC_FAKE_PROBE_ARGS_FILE"
)

// Fake probe output, so tests can assert on it.
const (
	FakeIconPNG    = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	FakeWindowJSON = `[{"handle":1,"title":"Notepad","process_id":42,"process_name":"notepad","class_name":"Notepad","icon_png":"` + FakeIconPNG + `"}]`
	FakePNG        = "PNG-BYTES"
	FakeStderr     = "capture exploded"

	// FakeFrameSize is the width and height of streamed fake frames.
	FakeFrameSize = 16
)

// RunFakeProbeIfRequested lets the test binary double as CaptureProbe. Call it
// first in TestMain: when the binary was started as a fake probe (see
// InstallFakeProbe) it serves the request and exits; otherwise it returns.
func RunFakeProbeIfRequested() {
	if os.Getenv(envFakeProbe) != "1" {
		return
	}
	os.Exit(runFakeProbe(os.Args[1:]))
}

// InstallFakeProbe copies the running test binary to CaptureProbe.exe in a
// temporary directory and returns its path. Processes started from it behave
// like a minimal CaptureProbe (--list, --stdout-png, --stream); the package's
// TestMain must call RunFakeProbeIfRequested. The environment is changed for
// the duration of the test, so such tests cannot run in parallel.
func InstallFakeProbe(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "CaptureProbe.exe")

	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dst, src)
	if err := dst.Close(); copyErr == nil {
		copyErr = err
	}
	if copyErr != nil {
		t.Fatal(copyErr)
	}

	t.Setenv(envFakeProbe, "1")
	return path
}

func runFakeProbe(args []string) int {
	if path := os.Getenv(EnvArgsFile); path != "" {
		if err := os.WriteFile(path, []byte(strings.Join(args, "\n")), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "fake probe:", err)
			return 2
		}
	}
	if os.Getenv(EnvCommandFail) == "1" && !slices.Contains(args, "--stream") {
		fmt.Fprintln(os.Stderr, FakeStderr)
		return 2
	}
	switch {
	case slices.Contains(args, "--list"):
		fmt.Print(FakeWindowJSON)
		return 0
	case slices.Contains(args, "--stdout-png"):
		fmt.Print(FakePNG)
		return 0
	case slices.Contains(args, "--stream"):
		return streamFakeFrames()
	}
	fmt.Fprintln(os.Stderr, "fake probe: unexpected arguments:", args)
	return 2
}

func streamFakeFrames() int {
	if os.Getenv(EnvStreamFail) == "1" {
		fmt.Fprintln(os.Stderr, FakeStderr)
		return 3
	}
	if os.Getenv(EnvStall) == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	for id := int64(1); ; id++ {
		if _, err := os.Stdout.Write(FramePacket(id, FakeFrameSize, FakeFrameSize)); err != nil {
			return 0 // the reader went away
		}
		time.Sleep(time.Second / 60)
	}
}
