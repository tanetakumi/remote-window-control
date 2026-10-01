package app

import (
	"context"

	"share-app-host/internal/input"
	"share-app-host/internal/window"
)

// TargetService manages the shared target window. It is where window selection
// and input meet: switching the target first releases every key and button held
// on the old window, so nothing is left pressed in a window the user can no
// longer see.
type TargetService struct {
	selection  *window.Selection
	dispatcher *input.Dispatcher
}

// NewTargetService returns a service over the given selection and dispatcher.
func NewTargetService(selection *window.Selection, dispatcher *input.Dispatcher) *TargetService {
	return &TargetService{selection: selection, dispatcher: dispatcher}
}

// List returns the windows that can be shared.
func (s *TargetService) List(ctx context.Context) ([]window.Info, error) {
	return s.selection.List(ctx)
}

// Current returns the selected window, if any.
func (s *TargetService) Current() (window.Info, bool) {
	return s.selection.Current()
}

// Select makes the window with the given handle the target. The window is
// looked up first, without holding anything up: enumerating windows is slow and
// input to the current target must keep flowing meanwhile. Only the switch
// itself, together with releasing held input, is done under the dispatcher.
func (s *TargetService) Select(ctx context.Context, handle uint64) (window.Info, error) {
	selected, err := s.selection.Resolve(ctx, handle)
	if err != nil {
		return window.Info{}, err
	}
	err = s.dispatcher.ChangeTarget(func() error {
		s.selection.Set(selected)
		return nil
	})
	return selected, err
}
