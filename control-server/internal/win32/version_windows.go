package win32

import "golang.org/x/sys/windows"

// BuildNumber returns the Windows build number. RtlGetVersion reports the
// real version, unlike GetVersionEx without an application manifest.
func BuildNumber() (uint32, error) { return windows.RtlGetVersion().BuildNumber, nil }
