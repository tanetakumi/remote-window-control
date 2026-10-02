package app

import (
	"context"
	"errors"
	"log"

	"share-app-host/internal/media"
	"share-app-host/internal/win32"
)

// PrepareWindow restores and brings the target to the foreground. Chromium
// applications can stop rendering while covered by other windows, even though
// Windows Graphics Capture is still running.
func PrepareWindow(ctx context.Context, handle uint64) error {
	if restored, err := win32.RestoreMinimized(win32.HWND(handle)); err != nil {
		return err
	} else if restored {
		log.Printf("target was minimized; restored hwnd=%d", handle)
	}
	// A target switch may have cancelled capture while restoration was pending.
	if err := ctx.Err(); err != nil {
		return err
	}
	return win32.BringToForeground(win32.HWND(handle))
}

// PreparingSource prepares the target once before each capture starts, covering
// selection, target switches and reconnection. A window that stays minimized
// cannot be captured; other failures (including foreground restrictions) are
// logged and capture is attempted anyway.
func PreparingSource(source media.Source, prepare func(context.Context, uint64) error) media.Source {
	return func(ctx context.Context, handle uint64) (media.FrameStream, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := prepare(ctx, handle)
		if cancelled := ctx.Err(); cancelled != nil {
			return nil, cancelled
		}
		switch {
		case errors.Is(err, win32.ErrWindowMinimized):
			log.Printf("target minimized and restore failed hwnd=%d: %v", handle, err)
			return nil, err
		case err != nil:
			log.Printf("target preparation failed hwnd=%d: %v", handle, err)
		default:
			log.Printf("target brought to foreground hwnd=%d", handle)
		}
		return source(ctx, handle)
	}
}
