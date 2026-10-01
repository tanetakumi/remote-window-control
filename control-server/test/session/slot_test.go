package session_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"share-app-host/internal/session"
)

func TestSlotAdmitsOneHolderUnderConcurrentAcquisition(t *testing.T) {
	var slot session.Slot
	var acquired atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if slot.TryAcquire() {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := acquired.Load(); got != 1 {
		t.Fatalf("holders: %d", got)
	}
	if slot.TryAcquire() {
		t.Fatal("second holder admitted")
	}
	slot.Release()
	if !slot.TryAcquire() {
		t.Fatal("slot did not recover after release")
	}
	slot.Release()
}

func TestSlotRefusesAcquisitionAfterClose(t *testing.T) {
	var slot session.Slot
	slot.Close()
	if slot.TryAcquire() {
		t.Fatal("slot admitted a holder after Close")
	}
}

func TestSlotCloseStopsTheHolderAndWaitsForRelease(t *testing.T) {
	var slot session.Slot
	if !slot.TryAcquire() {
		t.Fatal("could not acquire")
	}
	stopped := make(chan struct{})
	slot.OnClose(func() { close(stopped) })

	closed := make(chan struct{})
	go func() {
		slot.Close()
		close(closed)
	}()

	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not stop the holder")
	}
	select {
	case <-closed:
		t.Fatal("Close returned before the holder released the slot")
	case <-time.After(50 * time.Millisecond):
	}

	slot.Release()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return after the release")
	}
}

func TestSlotOnCloseStopsImmediatelyWhenAlreadyClosed(t *testing.T) {
	var slot session.Slot
	// A holder that won the slot just before Close registers late.
	if !slot.TryAcquire() {
		t.Fatal("could not acquire")
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		slot.Release()
	}()
	go slot.Close()
	time.Sleep(5 * time.Millisecond)

	stopped := make(chan struct{})
	slot.OnClose(func() { close(stopped) })
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("late registration was not stopped")
	}
}

func TestSlotOnCloseRegistrationEndsWithRelease(t *testing.T) {
	var slot session.Slot
	slot.TryAcquire()
	var calls atomic.Int32
	slot.OnClose(func() { calls.Add(1) })
	slot.Release()

	slot.Close()
	if got := calls.Load(); got != 0 {
		t.Fatalf("a released holder was stopped %d time(s)", got)
	}
}
