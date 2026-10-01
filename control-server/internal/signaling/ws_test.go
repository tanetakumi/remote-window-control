package signaling

import (
	"context"
	"errors"
	"share-app-host/internal/input"
	"share-app-host/internal/nativecapture"
	"share-app-host/internal/targetwindow"
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
			if h.acquire() {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()
	if acquired.Load() != 1 {
		t.Fatalf("active slots: %d", acquired.Load())
	}
	if h.acquire() {
		t.Fatal("second device accepted")
	}
	h.release()
	if !h.acquire() {
		t.Fatal("slot did not recover")
	}
	h.release()
	h.Close()
	if h.acquire() {
		t.Fatal("connection accepted after shutdown")
	}
}

func TestActiveConnectionAllowsTargetSelection(t *testing.T) {
	bridge := nativecapture.NewBridge(t.TempDir())
	h := NewHub(input.NewDispatcher(nil), bridge, targetwindow.NewManager(bridge))
	if !h.acquire() {
		t.Fatal("could not acquire control slot")
	}
	defer h.Close()
	defer h.release()
	// Reaching the missing capture helper confirms selection is allowed while
	// a connection is active, without any identity or ownership token.
	if _, err := h.SelectTarget(context.Background(), 1); !errors.Is(err, nativecapture.ErrBridgeUnavailable) {
		t.Fatalf("active target selection: %v", err)
	}
}
