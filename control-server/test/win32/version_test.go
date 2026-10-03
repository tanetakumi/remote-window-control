package win32_test

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"share-app-host/internal/win32"
)

func TestMinimumWindowsBuild(t *testing.T) {
	for _, build := range []uint32{0, 22000, 26099, 26100, 26200} {
		err := win32.CheckBuild(build)
		if build < 26100 {
			if err == nil || !strings.Contains(err.Error(), fmt.Sprint(build)) {
				t.Fatalf("build %d: %v", build, err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildNumberUsesNativeVersionOrReportsUnsupported(t *testing.T) {
	build, err := win32.BuildNumber()
	if runtime.GOOS != "windows" {
		if !errors.Is(err, win32.ErrUnsupported) {
			t.Fatalf("non-Windows version: %d %v", build, err)
		}
		return
	}
	if err != nil || build == 0 {
		t.Fatalf("native build number: %d %v", build, err)
	}
	t.Logf("Windows build=%d", build)
}
