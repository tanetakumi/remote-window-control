// Package input turns the web client's input messages into actions on the
// target window. Dispatcher validates commands and tracks held keys and
// buttons; an Injector performs the actual input.
package input

const (
	ModeWindow = "window"
	ModePC     = "pc"
)

// Command types sent by the web client.
const (
	TypeMode           = "input.mode"
	TypeTap            = "input.tap"
	TypeMouseMove      = "input.mouseMove"
	TypeMouseDown      = "input.mouseDown"
	TypeMouseUp        = "input.mouseUp"
	TypeScroll         = "input.scroll"
	TypeKeyDown        = "input.keyDown"
	TypeKeyUp          = "input.keyUp"
	TypeText           = "input.text"
	TypeViewportResize = "viewport.resize"
)

// MaxMessageBytes is the largest control message accepted from a client, on
// any transport.
const MaxMessageBytes = 64 * 1024

// Command is one input message. Pointer coordinates are normalised to the
// captured window image (0..1). Only the fields the web client sends are
// accepted.
type Command struct {
	Type             string  `json:"type"`
	Mode             string  `json:"mode,omitempty"`
	Button           string  `json:"button,omitempty"`
	Key              string  `json:"key,omitempty"`
	Text             string  `json:"text,omitempty"`
	Enter            bool    `json:"enter,omitempty"`
	X                float64 `json:"x,omitempty"`
	Y                float64 `json:"y,omitempty"`
	DeltaY           float64 `json:"deltaY,omitempty"`
	Width            int     `json:"width,omitempty"`
	Height           int     `json:"height,omitempty"`
	DevicePixelRatio float64 `json:"devicePixelRatio,omitempty"`
}
