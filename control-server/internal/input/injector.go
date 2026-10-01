package input

// Injector performs input on the target window. The Dispatcher serialises all
// calls, so implementations need no locking of their own.
type Injector interface {
	Move(x, y float64) error
	Tap(button string, x, y float64) error
	MouseDown(button string, x, y float64) error
	MouseUp(button string, x, y float64) error
	Scroll(deltaY, x, y float64) error
	ResizeViewport(command Command) error
	KeyDown(command Command) error
	KeyUp(command Command) error
	Text(text string) error
}
