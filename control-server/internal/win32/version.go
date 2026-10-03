package win32

import "fmt"

// MinimumBuild is Windows 11 24H2, the first build with the
// IncludeSecondaryWindows capture option that PC mode needs.
const MinimumBuild uint32 = 26100

// CheckBuild rejects Windows builds older than MinimumBuild.
func CheckBuild(build uint32) error {
	if build < MinimumBuild {
		return fmt.Errorf("this PC runs Windows build %d; Share App requires Windows 11 24H2 (build %d) or later", build, MinimumBuild)
	}
	return nil
}
