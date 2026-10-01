package app

import (
	"context"
	"errors"
	"log"

	"share-app-host/internal/media"
	"share-app-host/internal/win32"
)

// Restorer restores a minimized window. It reports whether a restore was
// needed, and returns win32.ErrWindowMinimized when the window stays minimized.
// win32.RestoreMinimized implements it for real windows.
type Restorer func(handle uint64) (restored bool, err error)

// RestoreWindow restores a real window through win32.RestoreMinimized.
func RestoreWindow(handle uint64) (bool, error) {
	return win32.RestoreMinimized(win32.HWND(handle))
}

// RestoringSource restores a minimized window before source captures it:
// Windows Graphics Capture delivers no frames for a minimized window, so the
// browser would otherwise wait for video that never comes. A window that stays
// minimized fails the capture with a message saying so; any other restore
// failure is logged and the capture is attempted anyway.
func RestoringSource(source media.Source, restore Restorer) media.Source {
	return func(ctx context.Context, handle uint64) (media.FrameStream, error) {
		restored, err := restore(handle)
		switch {
		case errors.Is(err, win32.ErrWindowMinimized):
			log.Printf("target minimized and restore failed hwnd=%d: %v", handle, err)
			return nil, err
		case err != nil:
			log.Printf("target restore check failed hwnd=%d: %v", handle, err)
		case restored:
			log.Printf("target was minimized; restored without activation hwnd=%d", handle)
		}
		return source(ctx, handle)
	}
}
