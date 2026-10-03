// Package config resolves settings in the user's data directory and locates
// the host's bundled programs and web client.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"share-app-host/internal/credentials"
)

const (
	defaultAddr          = "127.0.0.1:8443"
	settingsFile         = "config.json"
	minFPS               = 1
	maxFPS               = 30
	minCRF               = 0
	maxCRF               = 63
	minMaxScale          = 0.5
	maxMaxScale          = 4.0
	minScrollSensitivity = 0.5
	maxScrollSensitivity = 4.0
)

// DefaultMaxScale is Settings.MaxScale until it is changed.
const DefaultMaxScale = 2

// Capture measurement modes stored in config.json.
const (
	CaptureStatsOff = "off"
	CaptureStatsOn  = "on"
	// CaptureStatsVerify adds CPU-intensive pixel checks to diagnostics.
	CaptureStatsVerify = "verify"
)

// Config is the resolved host configuration. Credentials are only used by the
// RDP client; they are never included in the settings API.
type Config struct {
	ListenAddr   string
	ClientDir    string
	ProbePath    string
	FFmpegPath   string
	CaptureStats string
	// Settings is opened once at startup and shared by the host components.
	Settings    *SettingsStore
	RDPUsername string
	RDPPassword string
}

// Env separates installed assets from writable per-user data for tests.
type Env struct {
	ExeDir  string
	WorkDir string
	DataDir string
}

// UserDataDir returns %LOCALAPPDATA%\ShareApp on Windows. The platform's
// equivalent directory is used when running development checks elsewhere.
func UserDataDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate local application data: %w", err)
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("local application data directory must be absolute")
	}
	return filepath.Join(base, "ShareApp"), nil
}

// Load resolves assets and reads config.json and optional RDP credentials.
func Load(dataDir string) (Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return Config{}, fmt.Errorf("locate host executable: %w", err)
	}
	workDir, _ := os.Getwd()
	return LoadFrom(Env{ExeDir: filepath.Dir(exe), WorkDir: workDir, DataDir: dataDir})
}

// LoadFrom creates default settings on first run. Installation directories and
// environment variables do not supply application settings.
func LoadFrom(env Env) (Config, error) {
	if !filepath.IsAbs(env.DataDir) {
		return Config{}, fmt.Errorf("user data directory must be absolute")
	}
	base := resolveBaseDir(env)
	settingsPath := filepath.Join(env.DataDir, settingsFile)
	store, err := OpenSettings(settingsPath)
	if err != nil {
		return Config{}, err
	}
	startup := store.Startup()
	rdp, err := credentials.Load(filepath.Join(env.DataDir, credentials.FileName))
	if err != nil {
		return Config{}, err
	}
	return Config{
		ListenAddr:   startup.ListenAddr,
		ClientDir:    defaultClientDir(base),
		ProbePath:    findProbe(base),
		FFmpegPath:   findFFmpeg(env.ExeDir),
		CaptureStats: startup.CaptureStats,
		Settings:     store,
		RDPUsername:  rdp.Username,
		RDPPassword:  rdp.Password,
	}, nil
}
