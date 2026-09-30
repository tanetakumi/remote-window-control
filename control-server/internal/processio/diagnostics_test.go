package processio

import (
	"strings"
	"sync"
	"testing"
)

func TestDiagnosticsAreBoundedAndSafeDuringConcurrentReads(t *testing.T) {
	d := &Diagnostics{}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				_, _ = d.Write([]byte(strings.Repeat("x", 1024)))
				_ = d.String()
			}
		}()
	}
	wg.Wait()
	if got := len(d.String()); got != limit {
		t.Fatalf("log length: %d", got)
	}
	_, _ = d.Write([]byte("last error"))
	if !strings.HasSuffix(d.String(), "last error") {
		t.Fatal("lost diagnostic tail")
	}
}
