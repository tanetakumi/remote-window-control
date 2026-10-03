package app

import (
	"context"
	"errors"
	"log"
	"time"

	"share-app-host/internal/media"
	"share-app-host/internal/win32"
)

// PrepareWindow restores and brings the target to the foreground. Chromium
// applications can stop rendering while covered by other windows, even though
// Windows Graphics Capture is still running.
func PrepareWindow(ctx context.Context, handle uint64) error {
	if err := RestoreWindow(ctx, handle); err != nil {
		return err
	}
	return win32.BringToForeground(win32.HWND(handle))
}

// RestoreWindow is PrepareWindow without the activation, for PC-mode captures:
// activating the main window would close a popup the user opened.
func RestoreWindow(ctx context.Context, handle uint64) error {
	if restored, err := win32.RestoreMinimized(win32.HWND(handle)); err != nil {
		return err
	} else if restored {
		log.Printf("target was minimized; restored hwnd=%d", handle)
	}
	// A target switch may have cancelled capture while restoration was pending.
	return ctx.Err()
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
			log.Printf("target prepared hwnd=%d", handle)
		}
		return source(ctx, handle)
	}
}

// Desktop is the window management PreparePCMode needs; tests substitute it.
type Desktop interface {
	MinimizeAll() error
	IsMinimized(win32.HWND) bool
	RestoreMinimized(win32.HWND) (bool, error)
	BringToForeground(win32.HWND) error
}

type systemDesktop struct{}

func (systemDesktop) MinimizeAll() error                          { return win32.MinimizeAll() }
func (systemDesktop) IsMinimized(h win32.HWND) bool               { return win32.IsMinimized(h) }
func (systemDesktop) RestoreMinimized(h win32.HWND) (bool, error) { return win32.RestoreMinimized(h) }
func (systemDesktop) BringToForeground(h win32.HWND) error        { return win32.BringToForeground(h) }

const (
	// minimizeTimeout bounds the wait for the shell to minimize the target;
	// MinimizeAll only posts the request.
	minimizeTimeout = 1500 * time.Millisecond
	minimizePoll    = 20 * time.Millisecond
)

// PreparePCMode runs when switching to PC mode: it minimizes the other windows
// so they cannot cover the target and take its clicks, then restores the
// target and brings it forward. Menus open at that moment may close; later PC
// captures only restore (RestoreWindow), keeping menus opened in PC mode.
// The steps are best effort and only logged; just cancellation is returned.
func PreparePCMode(ctx context.Context, handle uint64, desktop Desktop) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h := win32.HWND(handle)
	// Restore first, so the wait below sees the shell's minimization rather
	// than an earlier one.
	if desktop.IsMinimized(h) {
		if _, err := desktop.RestoreMinimized(h); err != nil {
			log.Printf("PC mode initial restore failed hwnd=%d: %v", handle, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := desktop.MinimizeAll(); err != nil {
		log.Printf("minimize other windows failed: %v", err)
	} else if err := waitMinimized(ctx, desktop, h); err != nil {
		return err
	}
	if _, err := desktop.RestoreMinimized(h); err != nil {
		log.Printf("PC mode restore failed hwnd=%d: %v", handle, err)
	}
	if err := desktop.BringToForeground(h); err != nil {
		log.Printf("PC mode foreground failed hwnd=%d: %v", handle, err)
	}
	return ctx.Err()
}

// waitMinimized waits up to minimizeTimeout for the target to be minimized.
func waitMinimized(ctx context.Context, desktop Desktop, h win32.HWND) error {
	timeout := time.NewTimer(minimizeTimeout)
	defer timeout.Stop()
	ticker := time.NewTicker(minimizePoll)
	defer ticker.Stop()
	for !desktop.IsMinimized(h) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return ctx.Err()
}
