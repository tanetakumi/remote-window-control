package win32_test

import (
	"math"
	"testing"

	"share-app-host/internal/win32"
)

func TestMapToClient(t *testing.T) {
	capture := win32.Rect{Left: 100, Top: 100, Right: 300, Bottom: 200} // 200x100 image
	client := win32.Rect{Left: 110, Top: 130, Right: 290, Bottom: 195}  // client origin (110, 130)

	tests := []struct {
		name         string
		x, y         float64
		wantX, wantY int32
	}{
		{"top-left", 0, 0, -10, -30},
		{"bottom-right is the last pixel", 1, 1, 189, 69},
		{"centre truncates", 0.5, 0.5, 89, 19},
		{"below range clamps to 0", -3, -3, -10, -30},
		{"above range clamps to 1", 7, 7, 189, 69},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, ok := win32.MapToClient(capture, client, tt.x, tt.y)
			if !ok || x != tt.wantX || y != tt.wantY {
				t.Fatalf("MapToClient = (%d, %d, %v), want (%d, %d, true)", x, y, ok, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestMapToClientRejectsAnEmptyCaptureRect(t *testing.T) {
	for _, capture := range []win32.Rect{{}, {Left: 5, Right: 5, Top: 0, Bottom: 10}, {Left: 0, Right: 10, Top: 9, Bottom: 3}} {
		if _, _, ok := win32.MapToClient(capture, win32.Rect{}, 0.5, 0.5); ok {
			t.Errorf("accepted empty capture rect %+v", capture)
		}
	}
}

func TestWheelDelta(t *testing.T) {
	tests := []struct {
		deltaY float64
		want   int32
	}{
		{0, 0},
		{1, -120},       // scrolling down is a negative wheel delta
		{-1, 120},       // scrolling up is positive
		{0.5, -60},      // fractional gestures keep their magnitude
		{1.0 / 120, -1}, // touch gestures accumulate and send whole wheel units
		{-1.0 / 120, 1},
		{31.0 / 120, -31}, // floating-point conversion must not lose a unit
		{100, -12000},     // a typical pixel delta
		{300, -32768},     // would wrap to the opposite direction unclamped
		{-300, 32767},     // likewise
		{1e9, -32768},
		{-1e9, 32767},
	}
	for _, tt := range tests {
		if got := win32.WheelDelta(tt.deltaY); got != tt.want {
			t.Errorf("WheelDelta(%v) = %d, want %d", tt.deltaY, got, tt.want)
		}
	}
}

func TestFitClientSize(t *testing.T) {
	tests := []struct {
		name                     string
		wantW, wantH, maxW, maxH int
		gotW, gotH               int
	}{
		{"fits already", 1000, 500, 5000, 5000, 1000, 500},
		{"never scales up", 800, 600, 4000, 4000, 800, 600},
		{"halves to fit", 3840, 2160, 1920, 1080, 1920, 1080},
		{"limited by the tighter axis", 2000, 1000, 1000, 1000, 1000, 500},
		{"odd sizes round down to even", 1001, 501, 5000, 5000, 1000, 500},
		{"clamps to the largest supported size", 10000, 10000, 100000, 100000, 4096, 4096},
		{"clamps to the smallest supported size", 100, 100, 5000, 5000, 200, 200},
		{"tiny work area still yields the minimum", 800, 600, 50, 50, 200, 200},
		{"non-positive limit does not divide by zero", 800, 600, 0, -5, 200, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := win32.FitClientSize(tt.wantW, tt.wantH, tt.maxW, tt.maxH)
			if w != tt.gotW || h != tt.gotH {
				t.Fatalf("FitClientSize = %dx%d, want %dx%d", w, h, tt.gotW, tt.gotH)
			}
			if w%2 != 0 || h%2 != 0 {
				t.Fatalf("odd result %dx%d", w, h)
			}
		})
	}
}

func TestViewportPixels(t *testing.T) {
	tests := []struct {
		name          string
		cssW, cssH    int
		dpr, maxScale float64
		wantW, wantH  int
		wantOK        bool
	}{
		{"device pixel ratio below the limit", 390, 844, 2, 3, 780, 1688, true},
		{"limit caps the ratio", 390, 844, 3, 2, 780, 1688, true},
		{"fractional limit rounds", 390, 844, 3, 1.5, 585, 1266, true},
		{"limit below one shrinks the window", 400, 800, 1, 0.5, 200, 400, true},
		{"missing ratio counts as one", 390, 844, 0, 2, 390, 844, true},
		{"negative ratio counts as one", 390, 844, -2, 2, 390, 844, true},
		{"landscape", 844, 390, 3, 2, 1688, 780, true},
		{"oversized dimensions stay within the supported range", math.MaxInt, math.MaxInt, 3, 2, 4096, 4096, true},
		{"rounded width has no pixels", 1, 844, 0.1, 2, 0, 0, false},
		{"rounded height has no pixels", 390, 1, 0.1, 2, 0, 0, false},
		{"no width", 0, 844, 3, 2, 0, 0, false},
		{"no height", 390, 0, 3, 2, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, ok := win32.ViewportPixels(tt.cssW, tt.cssH, tt.dpr, tt.maxScale)
			if w != tt.wantW || h != tt.wantH || ok != tt.wantOK {
				t.Fatalf("ViewportPixels = %dx%d %v, want %dx%d %v", w, h, ok, tt.wantW, tt.wantH, tt.wantOK)
			}
		})
	}
}

func TestPlanClientResize(t *testing.T) {
	// 800x600 outer window with a 784x561 client area: 16x39 of frame.
	window := win32.Rect{Left: 100, Top: 100, Right: 900, Bottom: 700}
	client := win32.Rect{Right: 784, Bottom: 561}
	work := win32.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}

	t.Run("gives the client the requested size and adds the frame", func(t *testing.T) {
		got := win32.PlanClientResize(window, client, &work, 1000, 700)
		want := win32.Rect{Left: 100, Top: 100, Right: 100 + 1016, Bottom: 100 + 739}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})
	t.Run("moves the window back inside the work area", func(t *testing.T) {
		right := win32.Rect{Left: 1500, Top: 800, Right: 2300, Bottom: 1400}
		got := win32.PlanClientResize(right, client, &work, 1000, 700)
		want := win32.Rect{Left: 1920 - 1016, Top: 1040 - 739, Right: 1920, Bottom: 1040}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})
	t.Run("shrinks oversized requests to the work area", func(t *testing.T) {
		got := win32.PlanClientResize(window, client, &work, 4000, 3000)
		if got.Width()-16 != 1334 || got.Height()-39 != 1000 {
			t.Fatalf("client size = %dx%d, want 1334x1000", got.Width()-16, got.Height()-39)
		}
		if got.Width() > work.Width() || got.Height() > work.Height() {
			t.Fatalf("result %+v exceeds work area %+v", got, work)
		}
		if got.Left < work.Left || got.Top < work.Top || got.Right > work.Right || got.Bottom > work.Bottom {
			t.Fatalf("result %+v is outside work area %+v", got, work)
		}
		// Aspect ratio of the 4000x3000 request is preserved within rounding.
		clientW, clientH := got.Width()-16, got.Height()-39
		if d := clientW*3 - clientH*4; d < -8 || d > 8 {
			t.Fatalf("aspect ratio changed: client %dx%d", clientW, clientH)
		}
	})
	t.Run("keeps the position when the work area is unknown", func(t *testing.T) {
		got := win32.PlanClientResize(window, client, nil, 1000, 700)
		want := win32.Rect{Left: 100, Top: 100, Right: 100 + 1016, Bottom: 100 + 739}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})
	t.Run("client size is even", func(t *testing.T) {
		got := win32.PlanClientResize(window, client, &work, 1001, 701)
		if (got.Width()-16)%2 != 0 || (got.Height()-39)%2 != 0 {
			t.Fatalf("odd client size in %+v", got)
		}
	})
}

func TestMapToDesktopEndpointsAndNegativeOrigin(t *testing.T) {
	for _, desktop := range []win32.Rect{{Right: 1920, Bottom: 1080}, {Left: -1920, Top: -1080, Right: 1920, Bottom: 1080}} {
		for _, v := range []float64{0, 1} {
			x, y, ax, ay, ok := win32.MapToDesktop(desktop, desktop, v, v)
			if !ok {
				t.Fatal("valid desktop rejected")
			}
			if v == 0 {
				if x != desktop.Left || y != desktop.Top || ax != 0 || ay != 0 {
					t.Fatal(x, y, ax, ay)
				}
			} else if x != desktop.Right-1 || y != desktop.Bottom-1 || ax != 65535 || ay != 65535 {
				t.Fatal(x, y, ax, ay)
			}
		}
	}
	desktop := win32.Rect{Left: -1920, Right: 1920, Bottom: 1080}
	capture := win32.Rect{Left: -960, Top: 100, Right: 0, Bottom: 500}
	x, y, ax, ay, ok := win32.MapToDesktop(capture, desktop, .5, .5)
	if !ok || x != -481 || y != 299 || ax <= 0 || ax >= 65535 || ay <= 0 || ay >= 65535 {
		t.Fatal(x, y, ax, ay, ok)
	}
	if _, _, _, _, ok = win32.MapToDesktop(win32.Rect{}, desktop, 0, 0); ok {
		t.Fatal("empty capture accepted")
	}
}
