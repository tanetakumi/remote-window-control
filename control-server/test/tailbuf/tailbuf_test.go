package tailbuf_test

import (
	"strings"
	"sync"
	"testing"

	"share-app-host/internal/tailbuf"
)

func TestBufferKeepsOnlyTheMostRecentBytes(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{"under limit", 8, []string{"abc"}, "abc"},
		{"exactly limit", 4, []string{"abcd"}, "abcd"},
		{"accumulated overflow drops oldest", 4, []string{"abc", "def"}, "cdef"},
		{"single write larger than limit", 4, []string{"abcdefgh"}, "efgh"},
		{"large write replaces earlier data", 4, []string{"ab", "wxyz123"}, "z123"},
		{"many small writes", 3, []string{"a", "b", "c", "d", "e"}, "cde"},
		{"non-positive limit uses default", 0, []string{"hello"}, "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tailbuf.New(tt.limit)
			for _, w := range tt.writes {
				n, err := b.Write([]byte(w))
				if err != nil || n != len(w) {
					t.Fatalf("Write(%q) = %d, %v", w, n, err)
				}
			}
			if got := b.String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZeroValueKeepsDefaultLimit(t *testing.T) {
	var b tailbuf.Buffer
	_, _ = b.Write([]byte(strings.Repeat("x", tailbuf.DefaultLimit+100)))
	if got := len(b.String()); got != tailbuf.DefaultLimit {
		t.Fatalf("length %d, want %d", got, tailbuf.DefaultLimit)
	}
}

func TestBufferIsBoundedAndSafeDuringConcurrentReads(t *testing.T) {
	b := &tailbuf.Buffer{}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				_, _ = b.Write([]byte(strings.Repeat("x", 1024)))
				_ = b.String()
			}
		}()
	}
	wg.Wait()
	if got := len(b.String()); got != tailbuf.DefaultLimit {
		t.Fatalf("log length: %d", got)
	}
	_, _ = b.Write([]byte("last error"))
	if !strings.HasSuffix(b.String(), "last error") {
		t.Fatal("lost diagnostic tail")
	}
}
