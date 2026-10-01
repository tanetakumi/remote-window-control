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
)

const (
	envAddr      = "SHARE_APP_ADDR"
	envClientDir = "SHARE_APP_CLIENT_DIR"

	defaultAddr = "127.0.0.1:8443"
)

// Config is the resolved host configuration.
type Config struct {
	// ListenAddr is the HTTP listen address; loopback by default.
	ListenAddr string
	// ClientDir is the directory of web client assets that are served.
	ClientDir string
	// SnapshotDir is where saved snapshots are written.
	SnapshotDir string
	// ProbePath is the CaptureProbe executable.
	ProbePath string
	// FFmpegPath is the ffmpeg executable; a bare name is looked up on PATH.
	FFmpegPath string
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

	clientDir := setting(envClientDir, defaultClientDir(base))
	if !filepath.IsAbs(clientDir) {
		clientDir = filepath.Join(base, clientDir)
	}
	return Config{
		ListenAddr:  setting(envAddr, defaultAddr),
		ClientDir:   clientDir,
		SnapshotDir: filepath.Join(base, "snapshots"),
		ProbePath:   findProbe(base),
		FFmpegPath:  findFFmpeg(env.ExeDir),
	}, nil
}
