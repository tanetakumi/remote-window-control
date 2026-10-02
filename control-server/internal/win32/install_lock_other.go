//go:build !windows

package win32

func HoldInstallLock() (func(), error) { return func() {}, nil }
