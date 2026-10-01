package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"share-app-host/internal/config"
)

const dotnetOutput = "net10.0-windows10.0.19041.0/win-x64"

func mkdir(t *testing.T, elem ...string) string {
	t.Helper()
	dir := filepath.Join(elem...)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func touch(t *testing.T, elem ...string) string {
	t.Helper()
	path := filepath.Join(elem...)
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeEnvFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func env(exeDir, workDir string, vars map[string]string) config.Env {
	return config.Env{
		Getenv:  func(key string) string { return vars[key] },
		ExeDir:  exeDir,
		WorkDir: workDir,
	}
}

// checkout creates a source checkout and returns its root and an executable
// directory that is not part of it.
func checkout(t *testing.T) (root, exeDir string) {
	t.Helper()
	root = t.TempDir()
	touch(t, root, "control-server", "go.mod")
	return root, mkdir(t, t.TempDir(), "bin")
}

func TestDevelopmentCheckoutDefaults(t *testing.T) {
	root, exeDir := checkout(t)
	// A nested working directory still finds the checkout.
	workDir := mkdir(t, root, "control-server", "internal", "app")

	cfg, err := config.LoadFrom(env(exeDir, workDir, nil))
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		ListenAddr: "127.0.0.1:8443",
		ClientDir:  filepath.Join(root, "web-ui", "dist"),
		ProbePath:  filepath.Join(root, "CaptureProbe", "CaptureProbe.exe"),
		FFmpegPath: "ffmpeg",
	}
	if cfg != want {
		t.Fatalf("got  %+v\nwant %+v", cfg, want)
	}
}

func TestReleaseLayoutIsRecognisedBesideTheExecutable(t *testing.T) {
	exeDir := t.TempDir()
	web := mkdir(t, exeDir, "web")
	probe := touch(t, exeDir, "CaptureProbe", "CaptureProbe.exe")
	ffmpeg := touch(t, exeDir, "ffmpeg.exe")

	// A checkout above the working directory must not take over.
	root, _ := checkout(t)

	cfg, err := config.LoadFrom(env(exeDir, root, nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientDir != web || cfg.ProbePath != probe || cfg.FFmpegPath != ffmpeg {
		t.Fatalf("release layout not used: %+v", cfg)
	}
}

func TestExecutableDirectoryIsTheFallbackBase(t *testing.T) {
	exeDir := t.TempDir()
	cfg, err := config.LoadFrom(env(exeDir, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	// With no release layout and no checkout, everything is looked up beside
	// the executable.
	if want := filepath.Join(exeDir, "CaptureProbe", "CaptureProbe.exe"); cfg.ProbePath != want {
		t.Fatalf("ProbePath = %q, want %q", cfg.ProbePath, want)
	}
	if want := filepath.Join(exeDir, "web-ui", "dist"); cfg.ClientDir != want {
		t.Fatalf("ClientDir = %q, want %q", cfg.ClientDir, want)
	}
}

func TestSettingsPrecedence(t *testing.T) {
	root, exeDir := checkout(t)
	writeEnvFile(t, root, "SHARE_APP_ADDR=127.0.0.1:9000\nSHARE_APP_CLIENT_DIR=from-file\n")

	t.Run("file overrides defaults", func(t *testing.T) {
		cfg, err := config.LoadFrom(env(exeDir, root, nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ListenAddr != "127.0.0.1:9000" || cfg.ClientDir != filepath.Join(root, "from-file") {
			t.Fatalf("got %+v", cfg)
		}
	})
	t.Run("environment overrides file", func(t *testing.T) {
		vars := map[string]string{"SHARE_APP_ADDR": "0.0.0.0:1", "SHARE_APP_CLIENT_DIR": "from-env"}
		cfg, err := config.LoadFrom(env(exeDir, root, vars))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ListenAddr != "0.0.0.0:1" || cfg.ClientDir != filepath.Join(root, "from-env") {
			t.Fatalf("got %+v", cfg)
		}
	})
	t.Run("an empty environment variable does not override", func(t *testing.T) {
		vars := map[string]string{"SHARE_APP_ADDR": ""}
		cfg, err := config.LoadFrom(env(exeDir, root, vars))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ListenAddr != "127.0.0.1:9000" {
			t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
		}
	})
}

func TestClientDirectoryResolution(t *testing.T) {
	root, exeDir := checkout(t)

	absolute := filepath.Join(t.TempDir(), "assets")
	cfg, err := config.LoadFrom(env(exeDir, root, map[string]string{"SHARE_APP_CLIENT_DIR": absolute}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientDir != absolute {
		t.Fatalf("absolute path changed: %q", cfg.ClientDir)
	}

	// A web/ directory in the checkout wins over the web-ui build output.
	mkdir(t, root, "web")
	cfg, err = config.LoadFrom(env(exeDir, root, nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientDir != filepath.Join(root, "web") {
		t.Fatalf("ClientDir = %q", cfg.ClientDir)
	}
}

func TestEnvFileValuesAreLiteral(t *testing.T) {
	root, exeDir := checkout(t)
	writeEnvFile(t, root, "# test\nSHARE_APP_CLIENT_DIR='literal-$(command)'\n")

	cfg, err := config.LoadFrom(env(exeDir, root, nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientDir != filepath.Join(root, "literal-$(command)") {
		t.Fatalf("configuration was evaluated: %q", cfg.ClientDir)
	}
}

func TestInvalidEnvFileFailsAndNamesTheFile(t *testing.T) {
	root, exeDir := checkout(t)
	writeEnvFile(t, root, "SHARE_APP_ADDR=ok\nNOT_A_SETTING=1\n")

	_, err := config.LoadFrom(env(exeDir, root, nil))
	if err == nil {
		t.Fatal("invalid .env accepted")
	}
	if !strings.Contains(err.Error(), filepath.Join(root, ".env")) || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error does not say where the problem is: %v", err)
	}
}

func TestCaptureProbeLocationPreference(t *testing.T) {
	root, exeDir := checkout(t)
	probeAt := func(configuration string) string {
		return filepath.Join(root, "window-capture", "apps", "CaptureProbe", "bin", configuration,
			filepath.FromSlash(dotnetOutput), "CaptureProbe.exe")
	}
	load := func() string {
		t.Helper()
		cfg, err := config.LoadFrom(env(exeDir, root, nil))
		if err != nil {
			t.Fatal(err)
		}
		return cfg.ProbePath
	}

	release := filepath.Join(root, "CaptureProbe", "CaptureProbe.exe")
	if got := load(); got != release {
		t.Fatalf("with nothing built, ProbePath = %q, want the release location %q", got, release)
	}

	touch(t, probeAt("Release"))
	if got := load(); got != probeAt("Release") {
		t.Fatalf("ProbePath = %q, want the Release build", got)
	}

	touch(t, probeAt("Debug"))
	if got := load(); got != probeAt("Debug") {
		t.Fatalf("ProbePath = %q, want Debug ahead of Release", got)
	}

	touch(t, release)
	if got := load(); got != release {
		t.Fatalf("ProbePath = %q, want the release location ahead of builds", got)
	}
}
