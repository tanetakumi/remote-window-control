package win32

import "math"

const (
	minClientDimension = 200
	maxClientDimension = 4096

	// wheelNotch is the Win32 WHEEL_DELTA for one scroll notch.
	wheelNotch = 120
)

// MapToClient converts a point normalised to the captured window image
// (0..1 on both axes, clamped) into client-area coordinates of the window.
// capture is the rectangle the video frame covers and client the window's
// client area, both in screen coordinates. It reports false when capture is
// empty.
func MapToClient(capture, client Rect, x, y float64) (clientX, clientY int32, ok bool) {
	width, height := capture.Width(), capture.Height()
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	clientX = capture.Left + int32(float64(width-1)*clampUnit(x)) - client.Left
	clientY = capture.Top + int32(float64(height-1)*clampUnit(y)) - client.Top
	return clientX, clientY, true
}

// WheelDelta converts wheel notches (positive scrolls down) into a Win32
// wheel delta (positive scrolls up). The result is limited to the 16-bit
// range the message carries, so large gestures cannot wrap around and scroll
// the opposite way.
func WheelDelta(deltaY float64) int32 {
	delta := -deltaY * wheelNotch
	switch {
	case delta > math.MaxInt16:
		return math.MaxInt16
	case delta < math.MinInt16:
		return math.MinInt16
	}
	return int32(math.Round(delta))
}

// FitClientSize scales a desired client size down, preserving aspect ratio,
// until it fits within maxWidth x maxHeight. It never scales up. The result is
// limited to the supported range and has even dimensions, which the video
// encoder requires.
func FitClientSize(desiredWidth, desiredHeight, maxWidth, maxHeight int) (int, int) {
	desiredWidth = clampDimension(desiredWidth)
	desiredHeight = clampDimension(desiredHeight)
	maxWidth = max(maxWidth, 1)
	maxHeight = max(maxHeight, 1)

	scale := min(1, float64(maxWidth)/float64(desiredWidth), float64(maxHeight)/float64(desiredHeight))
	width := int(math.Round(float64(desiredWidth) * scale))
	height := int(math.Round(float64(desiredHeight) * scale))
	return evenDimension(width), evenDimension(height)
}

// ViewportPixels converts a viewport of cssWidth x cssHeight CSS pixels into
// the client size to give the window. Each CSS pixel becomes devicePixelRatio
// window pixels, but never more than maxScale, so a high-density screen cannot
// ask for more pixels than the user allows. A ratio that is not positive counts
// as 1. The result is capped at the supported maximum dimension before
// converting to integers. It reports false when either rounded dimension
// has no pixels.
func ViewportPixels(cssWidth, cssHeight int, devicePixelRatio, maxScale float64) (width, height int, ok bool) {
	if cssWidth <= 0 || cssHeight <= 0 {
		return 0, 0, false
	}
	scale := devicePixelRatio
	if scale <= 0 {
		scale = 1
	}
	scale = min(scale, maxScale)
	w := math.Round(float64(cssWidth) * scale)
	h := math.Round(float64(cssHeight) * scale)
	if !(w >= 1 && h >= 1) {
		return 0, 0, false
	}
	return int(min(w, float64(maxClientDimension))), int(min(h, float64(maxClientDimension))), true
}

// PlanClientResize returns the outer window rectangle that gives the window a
// client area of the requested size after fitting to the monitor work area,
// subject to the minimum size and even dimensions. window and client
// are the current outer and client rectangles; the difference between them
// is the frame. The window is moved back inside work when it is known.
// Pass nil when the work area is unknown.
func PlanClientResize(window, client Rect, work *Rect, wantWidth, wantHeight int) Rect {
	frameWidth := int(window.Width() - client.Width())
	frameHeight := int(window.Height() - client.Height())

	if work != nil {
		wantWidth, wantHeight = FitClientSize(wantWidth, wantHeight,
			int(work.Width())-frameWidth, int(work.Height())-frameHeight)
	}
	width := clampDimension(evenDimension(wantWidth) + frameWidth)
	height := clampDimension(evenDimension(wantHeight) + frameHeight)

	left, top := window.Left, window.Top
	if work != nil {
		left = clampInt32(left, work.Left, work.Right-int32(width))
		top = clampInt32(top, work.Top, work.Bottom-int32(height))
	}
	return Rect{Left: left, Top: top, Right: left + int32(width), Bottom: top + int32(height)}
}

func clampDimension(value int) int {
	return min(max(value, minClientDimension), maxClientDimension)
}

// evenDimension limits value to the supported range and rounds odd values down.
func evenDimension(value int) int {
	return clampDimension(value) &^ 1
}

func clampUnit(value float64) float64 {
	return min(max(value, 0), 1)
}

// clampInt32 limits value to [lo, hi]; an inverted range collapses to lo.
func clampInt32(value, lo, hi int32) int32 {
	if hi < lo {
		return lo
	}
	return min(max(value, lo), hi)
}
