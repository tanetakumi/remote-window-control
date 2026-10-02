//go:build !windows

package tray

// Start is a no-op for development builds on other platforms.
func Start(onExit func()) (func(), error) { return func() {}, nil }
