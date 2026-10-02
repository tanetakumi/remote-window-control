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
// console allocation and redirected I/O, rather than just inspecting flags.
func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		getConsoleCP := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleCP")
		codePage, _, _ := getConsoleCP.Call()
		fmt.Fprintf(os.Stdout, "console=%d\n", codePage)
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "helper diagnostic")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHelperHasNoConsoleAndPreservesRedirectedIO(t *testing.T) {
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
	if got := stdout.String(); got != "console=0\nframe payload" {
		t.Fatalf("helper stdout = %q; want no attached console and intact payload", got)
	}
	if got := stderr.String(); got != "helper diagnostic\n" {
		t.Fatalf("helper stderr = %q; diagnostic output was lost", got)
	}
}
