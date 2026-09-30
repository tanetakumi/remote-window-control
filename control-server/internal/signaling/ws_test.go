package signaling

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestSingleSlotConcurrentAcquisitionAndShutdown(t *testing.T) {
	h := &Hub{}
	var acquired atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if h.acquire("same-token") {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()
	if acquired.Load() != 1 {
		t.Fatalf("active slots: %d", acquired.Load())
	}
	if h.acquire("different-token") {
		t.Fatal("second device accepted")
	}
	h.release()
	if !h.acquire("reconnect") {
		t.Fatal("slot did not recover")
	}
	h.release()
	h.Close()
	if h.acquire("after shutdown") {
		t.Fatal("connection accepted after shutdown")
	}
}
