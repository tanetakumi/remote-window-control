package config_test

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"share-app-host/internal/config"
)

func settingsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.json")
}

func TestLoadedSettingsPersistInTheUserDataDirectory(t *testing.T) {
	exeDir := t.TempDir()
	mkdir(t, exeDir, "web")
	context := env(t, exeDir, "")
	cfg, err := config.LoadFrom(context)
	if err != nil {
		t.Fatal(err)
	}
	saved := config.Settings{FPS: 15, CRF: 20, MaxScale: 1.5}
	if err := cfg.Settings.Set(saved); err != nil {
		t.Fatal(err)
	}
	store, err := config.OpenSettings(filepath.Join(context.DataDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get(); got != saved {
		t.Fatalf("saved settings = %+v, want %+v", got, saved)
	}
}

func TestMissingSettingsFileCreatesTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ShareApp", "config.json")
	store, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := store.Get(), (config.Settings{FPS: 8, CRF: 31, MaxScale: 2}); got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default config was not created: %v", err)
	}
}

func TestSavedSettingsSurviveReopening(t *testing.T) {
	path := settingsPath(t)
	store, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := config.Settings{FPS: 15, CRF: 20, MaxScale: 1.5}
	if err := store.Set(saved); err != nil {
		t.Fatal(err)
	}
	if got := store.Get(); got != saved {
		t.Fatalf("settings = %+v, want %+v", got, saved)
	}
	reopened, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get(); got != saved {
		t.Fatalf("reopened settings = %+v, want %+v", got, saved)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory = %v (err %v), want only config.json", entries, err)
	}
}

func TestInvalidSettingsAreRejectedAndNotSaved(t *testing.T) {
	path := settingsPath(t)
	store, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []config.Settings{
		{FPS: 0, CRF: 31, MaxScale: 2},
		{FPS: 31, CRF: 31, MaxScale: 2},
		{FPS: 8, CRF: -1, MaxScale: 2},
		{FPS: 8, CRF: 64, MaxScale: 2},
		{FPS: 8, CRF: 31, MaxScale: 0},
		{FPS: 8, CRF: 31, MaxScale: 0.25},
		{FPS: 8, CRF: 31, MaxScale: 4.5},
		{FPS: 8, CRF: 31, MaxScale: math.NaN()},
	} {
		if err := store.Set(bad); !errors.Is(err, config.ErrInvalidSettings) {
			t.Fatalf("Set(%+v) = %v, want ErrInvalidSettings", bad, err)
		}
	}
	if got := store.Get(); got != config.DefaultSettings() {
		t.Fatalf("settings changed to %+v", got)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Fatalf("a rejected save changed the file (err %v)", err)
	}
}

func TestFailedSaveKeepsTheCurrentSettings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ShareApp")
	path := filepath.Join(dir, "config.json")
	store, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	err = store.Set(config.Settings{FPS: 12, CRF: 31, MaxScale: 2})
	if err == nil || errors.Is(err, config.ErrInvalidSettings) {
		t.Fatalf("Set = %v, want a write error", err)
	}
	if got := store.Get(); got != config.DefaultSettings() {
		t.Fatalf("settings changed to %+v", got)
	}
}

func TestSettingsFileFieldsDefaultWhenOmitted(t *testing.T) {
	path := settingsPath(t)
	if err := os.WriteFile(path, []byte(`{"fps": 12}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := store.Get(), (config.Settings{FPS: 12, CRF: 31, MaxScale: 2}); got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}

func TestBadSettingsFileFailsAndNamesTheFile(t *testing.T) {
	for name, content := range map[string]string{
		"syntax":               `{"fps": `,
		"unknown field":        `{"fps": 8, "fpz": 9}`,
		"fps range":            `{"fps": 99}`,
		"crf range":            `{"crf": 64}`,
		"scale range":          `{"maxScale": 5}`,
		"null":                 `null`,
		"trailing object":      `{} {}`,
		"trailing garbage":     `{} broken`,
		"address without port": `{"listenAddr":"127.0.0.1"}`,
		"port range":           `{"listenAddr":"127.0.0.1:65536"}`,
		"port zero":            `{"listenAddr":":0"}`,
		"diagnostics mode":     `{"captureStats":"yes"}`,
	} {
		path := settingsPath(t)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := config.OpenSettings(path); err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("%s: error = %v, want one naming %s", name, err, path)
		}
	}
}
