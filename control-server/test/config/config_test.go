package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"share-app-host/internal/config"
	"share-app-host/internal/credentials"
)

const dotnetOutput = "net10.0-windows10.0.26100.0/win-x64"

func mkdir(t *testing.T, elem ...string) string {
	t.Helper()
	dir := filepath.Join(elem...)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func touch(t *testing.T, elem ...string) string {
	t.Helper()
	path := filepath.Join(elem...)
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func env(t *testing.T, exeDir, workDir string) config.Env {
	t.Helper()
	return config.Env{ExeDir: exeDir, WorkDir: workDir, DataDir: filepath.Join(t.TempDir(), "ShareApp")}
}

func checkout(t *testing.T) (root, exeDir string) {
	t.Helper()
	root = t.TempDir()
	touch(t, root, "control-server", "go.mod")
	return root, mkdir(t, t.TempDir(), "bin")
}

func TestDevelopmentCheckoutDefaults(t *testing.T) {
	root, exeDir := checkout(t)
	context := env(t, exeDir, mkdir(t, root, "control-server", "internal", "app"))
	cfg, err := config.LoadFrom(context)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		ListenAddr:   "127.0.0.1:8443",
		ClientDir:    filepath.Join(root, "web-ui", "dist"),
		ProbePath:    filepath.Join(root, "CaptureProbe", "CaptureProbe.exe"),
		FFmpegPath:   "ffmpeg",
		CaptureStats: config.CaptureStatsOff,
		Settings:     cfg.Settings,
	}
	if cfg != want {
		t.Fatalf("got %+v; want %+v", cfg, want)
	}
	if cfg.Settings == nil || cfg.Settings.Get() != config.DefaultSettings() {
		t.Fatal("loaded configuration does not contain the default settings store")
	}
	data, err := os.ReadFile(filepath.Join(context.DataDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var initial map[string]any
	if err := json.Unmarshal(data, &initial); err != nil {
		t.Fatal(err)
	}
	if initial["listenAddr"] != "127.0.0.1:8443" || initial["captureStats"] != "off" || initial["fps"] != float64(8) {
		t.Fatalf("initial configuration = %s", data)
	}
	if cfg.RDPUsername != "" || cfg.RDPPassword != "" {
		t.Fatal("RDP enabled without a credential file")
	}
}

func TestReleaseLayoutIsRecognisedBesideTheExecutable(t *testing.T) {
	exeDir := t.TempDir()
	web := mkdir(t, exeDir, "web")
	probe := touch(t, exeDir, "CaptureProbe", "CaptureProbe.exe")
	ffmpeg := touch(t, exeDir, "ffmpeg.exe")
	root, _ := checkout(t)
	cfg, err := config.LoadFrom(env(t, exeDir, root))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientDir != web || cfg.ProbePath != probe || cfg.FFmpegPath != ffmpeg {
		t.Fatalf("release layout not used: %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(exeDir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("startup wrote configuration to the installation directory")
	}
}

func TestExecutableDirectoryIsTheFallbackAssetBase(t *testing.T) {
	exeDir := t.TempDir()
	cfg, err := config.LoadFrom(env(t, exeDir, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProbePath != filepath.Join(exeDir, "CaptureProbe", "CaptureProbe.exe") || cfg.ClientDir != filepath.Join(exeDir, "web-ui", "dist") {
		t.Fatalf("unexpected asset paths: %+v", cfg)
	}
}

func TestStartupSettingsComeFromUserConfig(t *testing.T) {
	context := env(t, t.TempDir(), "")
	mkdir(t, context.DataDir)
	path := filepath.Join(context.DataDir, "config.json")
	if err := os.WriteFile(path, []byte(`{"listenAddr":":9000","captureStats":"verify","fps":12}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(context)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":9000" || cfg.CaptureStats != config.CaptureStatsVerify {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.Settings == nil || cfg.Settings.Get().FPS != 12 {
		t.Fatal("loaded configuration does not contain the file's video settings")
	}
}

func TestClientDirectoryPrefersBuiltWebAssets(t *testing.T) {
	root, exeDir := checkout(t)
	context := env(t, exeDir, root)
	mkdir(t, root, "web")
	cfg, err := config.LoadFrom(context)
	if err != nil || cfg.ClientDir != filepath.Join(root, "web") {
		t.Fatalf("ClientDir = %q, error = %v", cfg.ClientDir, err)
	}
}

func TestCaptureProbeLocationPreference(t *testing.T) {
	root, exeDir := checkout(t)
	context := env(t, exeDir, root)
	probeAt := func(configuration string) string {
		return filepath.Join(root, "window-capture", "apps", "CaptureProbe", "bin", configuration, filepath.FromSlash(dotnetOutput), "CaptureProbe.exe")
	}
	load := func() string {
		t.Helper()
		cfg, err := config.LoadFrom(context)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.ProbePath
	}
	release := filepath.Join(root, "CaptureProbe", "CaptureProbe.exe")
	if got := load(); got != release {
		t.Fatalf("ProbePath = %q", got)
	}
	touch(t, probeAt("Release"))
	if got := load(); got != probeAt("Release") {
		t.Fatalf("ProbePath = %q", got)
	}
	touch(t, probeAt("Debug"))
	if got := load(); got != probeAt("Debug") {
		t.Fatalf("ProbePath = %q", got)
	}
	touch(t, release)
	if got := load(); got != release {
		t.Fatalf("ProbePath = %q", got)
	}
}

func TestInvalidCredentialFileFailsStartup(t *testing.T) {
	context := env(t, t.TempDir(), "")
	mkdir(t, context.DataDir)
	path := touch(t, context.DataDir, credentials.FileName)
	if _, err := config.LoadFrom(context); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("startup error = %v", err)
	}
}

func TestUserDataDirUsesLocalAppDataOnWindows(t *testing.T) {
	base := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", base)
	case "linux":
		t.Setenv("XDG_CACHE_HOME", base)
	default:
		t.Skip("platform directory covered on Windows and Linux")
	}
	got, err := config.UserDataDir()
	if err != nil || got != filepath.Join(base, "ShareApp") {
		t.Fatalf("UserDataDir = %q, %v", got, err)
	}
}

func TestDataDirMustBeAbsolute(t *testing.T) {
	if _, err := config.LoadFrom(config.Env{ExeDir: t.TempDir(), DataDir: "relative"}); err == nil {
		t.Fatal("relative data directory accepted")
	}
}
