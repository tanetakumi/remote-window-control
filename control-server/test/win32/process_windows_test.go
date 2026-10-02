package win32_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"share-app-host/internal/win32"
)

const helperEnv = "SHARE_APP_TEST_CONSOLE_HELPER"

// Reuse this console test binary as a helper so the test exercises actual
// console window visibility and redirected I/O, rather than just inspecting flags.
func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		getConsoleWindow := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
		window, _, _ := getConsoleWindow.Call()
		var visible uintptr
		if window != 0 {
			isWindowVisible := syscall.NewLazyDLL("user32.dll").NewProc("IsWindowVisible")
			visible, _, _ = isWindowVisible.Call(window)
		}
		fmt.Fprintf(os.Stdout, "consoleVisible=%t\n", visible != 0)
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "helper diagnostic")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHelperHasNoVisibleConsoleAndPreservesRedirectedIO(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	win32.HideConsole(cmd)
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	cmd.Stdin = strings.NewReader("frame payload")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper failed: %v: %s", err, &stderr)
	}
	if got := stdout.String(); got != "consoleVisible=false\nframe payload" {
		t.Fatalf("helper stdout = %q; want no visible console window and intact payload", got)
	}
	if got := stderr.String(); got != "helper diagnostic\n" {
		t.Fatalf("helper stderr = %q; diagnostic output was lost", got)
	}
}
