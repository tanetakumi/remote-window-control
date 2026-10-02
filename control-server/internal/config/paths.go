package config

import (
	"os"
	"path/filepath"
)

// Two layouts are supported:
//
//   - release: web/ and CaptureProbe/ sit beside the executable;
//   - development: the executable runs from a source checkout, found by
//     walking up from the working directory to the directory that contains
//     control-server/go.mod.

// probeBuildDir is where `dotnet build` places CaptureProbe in a checkout.
const probeBuildDir = "net10.0-windows10.0.26100.0/win-x64"

// resolveBaseDir returns the directory that holds the client and CaptureProbe.
func resolveBaseDir(env Env) string {
	for _, marker := range []string{"web", "CaptureProbe"} {
		if isDir(filepath.Join(env.ExeDir, marker)) {
			return env.ExeDir
		}
	}
	if env.WorkDir != "" {
		for dir := env.WorkDir; ; dir = filepath.Dir(dir) {
			if exists(filepath.Join(dir, "control-server", "go.mod")) {
				return dir
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return env.ExeDir
}

// defaultClientDir prefers a release web/ directory over the web-ui build.
func defaultClientDir(base string) string {
	if web := filepath.Join(base, "web"); isDir(web) {
		return web
	}
	return filepath.Join(base, "web-ui", "dist")
}

// findProbe returns the first existing CaptureProbe executable, preferring the
// release location. When none exists it returns the release location, so the
// resulting "unavailable" error points at the expected install path.
func findProbe(base string) string {
	candidates := []string{filepath.Join(base, "CaptureProbe", "CaptureProbe.exe")}
	for _, configuration := range []string{"Debug", "Release"} {
		candidates = append(candidates, filepath.Join(base,
			"window-capture", "apps", "CaptureProbe", "bin", configuration,
			filepath.FromSlash(probeBuildDir), "CaptureProbe.exe"))
	}
	for _, path := range candidates {
		if exists(path) {
			return path
		}
	}
	return candidates[0]
}

// findFFmpeg prefers an ffmpeg.exe beside the executable, otherwise it relies
// on PATH.
func findFFmpeg(exeDir string) string {
	if path := filepath.Join(exeDir, "ffmpeg.exe"); exists(path) {
		return path
	}
	return "ffmpeg"
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
