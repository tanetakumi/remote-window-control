// Package window models the application window being shared and which window
// is currently selected as the target.
package window

import "errors"

var (
	// ErrNotSelected is returned when an operation needs a target window but
	// none has been selected.
	ErrNotSelected = errors.New("target window not selected")
	// ErrNotFound is returned when the requested window is no longer listed.
	ErrNotFound = errors.New("target window not found")
)

// Info describes a top-level window. The JSON tags are the wire format shared
// with CaptureProbe and the web client.
type Info struct {
	Handle      uint64 `json:"handle"`
	Title       string `json:"title"`
	ProcessID   uint32 `json:"process_id"`
	ProcessName string `json:"process_name"`
	ClassName   string `json:"class_name"`
}
