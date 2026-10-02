// Package config resolves the host's settings and the locations of the files
// it depends on.
//
// Settings come from environment variables, then an optional .env file, then
// defaults. The .env file is plain data (KEY=VALUE), never evaluated as shell.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"share-app-host/internal/media"
)

const (
	envAddr         = "SHARE_APP_ADDR"
	envCaptureStats = "SHARE_APP_CAPTURE_STATS"
	envFPS          = "SHARE_APP_FPS"

	envRDPUsername = "SHARE_APP_RDP_USERNAME"
	envRDPPassword = "SHARE_APP_RDP_PASSWORD"

	defaultAddr = "127.0.0.1:8443"

	minFPS = 1
	maxFPS = 30
)

// Capture measurement modes (SHARE_APP_CAPTURE_STATS).
const (
	// CaptureStatsOff writes no periodic measurements.
	CaptureStatsOff = "off"
	// CaptureStatsOn logs dirty-region and encoded-video statistics.
	CaptureStatsOn = "on"
	// CaptureStatsVerify also checks dirty regions against a pixel comparison
	// of consecutive frames, at a noticeable CPU cost.
	CaptureStatsVerify = "verify"
)

// Config is the resolved host configuration.
type Config struct {
	// ListenAddr is the HTTP listen address; loopback by default.
	ListenAddr string
	// ClientDir is the directory of web client assets that are served: web/
	// beside the executable in a release, web-ui/dist/ in a checkout.
	ClientDir string
	// ProbePath is the CaptureProbe executable.
	ProbePath string
	// FFmpegPath is the ffmpeg executable; a bare name is looked up on PATH.
	FFmpegPath string
	// CaptureStats is one of the CaptureStats* modes; off by default.
	CaptureStats string
	// FPS is the video encode rate while the window changes, minFPS to maxFPS.
	FPS int
	// RDPUsername and RDPPassword are the Windows account (the user running
	// share-host) the loopback RDP keep-alive signs in with. Both must be
	// set to enable it.
	RDPUsername string
	RDPPassword string
}

// Env is the process context a configuration is resolved from. It exists so
// tests can resolve configuration without touching the real environment.
type Env struct {
	// Getenv looks up an environment variable.
	Getenv func(string) string
	// ExeDir is the directory of the running executable.
	ExeDir string
	// WorkDir is the current directory, used to find a source checkout. It may
	// be empty.
	WorkDir string
}

// CurrentEnv returns the environment of the running process.
func CurrentEnv() Env {
	exe, _ := os.Executable()
	workDir, _ := os.Getwd()
	return Env{Getenv: os.Getenv, ExeDir: filepath.Dir(exe), WorkDir: workDir}
}

// Load resolves the configuration of the running process.
func Load() (Config, error) {
	return LoadFrom(CurrentEnv())
}

// LoadFrom resolves the configuration for env.
func LoadFrom(env Env) (Config, error) {
	base := resolveBaseDir(env)

	envFile := filepath.Join(base, ".env")
	fileValues, err := readDotenvFile(envFile)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", envFile, err)
	}
	setting := func(key, fallback string) string {
		if v := env.Getenv(key); v != "" {
			return v
		}
		if v := fileValues[key]; v != "" {
			return v
		}
		return fallback
	}

	stats := setting(envCaptureStats, CaptureStatsOff)
	switch stats {
	case CaptureStatsOff, CaptureStatsOn, CaptureStatsVerify:
	default:
		return Config{}, fmt.Errorf("%s must be %s, %s or %s", envCaptureStats, CaptureStatsOff, CaptureStatsOn, CaptureStatsVerify)
	}

	fps, err := strconv.Atoi(setting(envFPS, strconv.Itoa(media.DefaultFPS)))
	if err != nil || fps < minFPS || fps > maxFPS {
		return Config{}, fmt.Errorf("%s must be an integer from %d to %d", envFPS, minFPS, maxFPS)
	}

	return Config{
		ListenAddr:   setting(envAddr, defaultAddr),
		ClientDir:    defaultClientDir(base),
		ProbePath:    findProbe(base),
		FFmpegPath:   findFFmpeg(env.ExeDir),
		CaptureStats: stats,
		FPS:          fps,
		RDPUsername:  setting(envRDPUsername, ""),
		RDPPassword:  setting(envRDPPassword, ""),
	}, nil
}
