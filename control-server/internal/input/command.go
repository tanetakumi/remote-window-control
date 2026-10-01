// Package input turns the web client's input messages into actions on the
// target window. Dispatcher validates commands and tracks held keys and
// buttons; an Injector performs the actual input.
package input

// Command types sent by the web client.
const (
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
// declared; anything else in the JSON is ignored.
type Command struct {
	Type             string  `json:"type"`
	Button           string  `json:"button,omitempty"`
	Key              string  `json:"key,omitempty"`
	Text             string  `json:"text,omitempty"`
	X                float64 `json:"x,omitempty"`
	Y                float64 `json:"y,omitempty"`
	DeltaX           float64 `json:"deltaX,omitempty"`
	DeltaY           float64 `json:"deltaY,omitempty"`
	Width            int     `json:"width,omitempty"`
	Height           int     `json:"height,omitempty"`
	DevicePixelRatio float64 `json:"devicePixelRatio,omitempty"`
}
