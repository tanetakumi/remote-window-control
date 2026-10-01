package win32_test

import (
	"testing"

	"share-app-host/internal/win32"
)

func TestVirtualKey(t *testing.T) {
	tests := map[string]uint16{
		"Backspace":  0x08,
		"Tab":        0x09,
		"Enter":      0x0D,
		"Escape":     0x1B,
		" ":          0x20,
		"PageUp":     0x21,
		"PageDown":   0x22,
		"End":        0x23,
		"Home":       0x24,
		"ArrowLeft":  0x25,
		"ArrowUp":    0x26,
		"ArrowRight": 0x27,
		"ArrowDown":  0x28,
		"Delete":     0x2E,
	}
	for key, want := range tests {
		if got, ok := win32.VirtualKey(key); !ok || got != want {
			t.Errorf("VirtualKey(%q) = %#x, %v; want %#x, true", key, got, ok, want)
		}
	}
}

func TestVirtualKeyIgnoresPrintableAndUnknownKeys(t *testing.T) {
	for _, key := range []string{"a", "A", "1", "", "Shift", "F5", "enter"} {
		if vk, ok := win32.VirtualKey(key); ok {
			t.Errorf("VirtualKey(%q) = %#x, want no mapping", key, vk)
		}
	}
}

func TestButtons(t *testing.T) {
	var held win32.Buttons
	held = held.With(win32.ButtonLeft)
	held = held.With(win32.ButtonRight)
	if held != 0x0003 {
		t.Fatalf("both held = %#x", uint(held))
	}
	held = held.Without(win32.ButtonLeft)
	if held != 0x0002 {
		t.Fatalf("right only = %#x", uint(held))
	}
	if held.Without(win32.ButtonLeft) != held {
		t.Fatal("releasing a button that is not held changed the set")
	}
	if held.Without(win32.ButtonRight) != 0 {
		t.Fatal("not empty after releasing everything")
	}
}
