package config_test

import (
	"errors"
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

func TestSettingsPathIsBesideTheEnvFile(t *testing.T) {
	exeDir := t.TempDir()
	mkdir(t, exeDir, "web")
	cfg, err := config.LoadFrom(env(exeDir, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(exeDir, "config.json"); cfg.SettingsPath != want {
		t.Fatalf("SettingsPath = %q, want %q", cfg.SettingsPath, want)
	}
}

func TestMissingSettingsFileGivesTheDefaults(t *testing.T) {
	store, err := config.OpenSettings(settingsPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := store.Get(), (config.Settings{FPS: 8, CRF: 31}); got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}

func TestSavedSettingsSurviveReopening(t *testing.T) {
	path := settingsPath(t)
	store, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := config.Settings{FPS: 15, CRF: 20}
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
	for _, bad := range []config.Settings{
		{FPS: 0, CRF: 31},
		{FPS: 31, CRF: 31},
		{FPS: 8, CRF: -1},
		{FPS: 8, CRF: 64},
	} {
		if err := store.Set(bad); !errors.Is(err, config.ErrInvalidSettings) {
			t.Fatalf("Set(%+v) = %v, want ErrInvalidSettings", bad, err)
		}
	}
	if got := store.Get(); got != config.DefaultSettings() {
		t.Fatalf("settings changed to %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a rejected save created the file (err %v)", err)
	}
}

func TestFailedSaveKeepsTheCurrentSettings(t *testing.T) {
	store, err := config.OpenSettings(filepath.Join(t.TempDir(), "missing-dir", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Set(config.Settings{FPS: 12, CRF: 31})
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
	if got, want := store.Get(), (config.Settings{FPS: 12, CRF: 31}); got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}

func TestBadSettingsFileFailsAndNamesTheFile(t *testing.T) {
	for name, content := range map[string]string{
		"syntax":        `{"fps": `,
		"unknown field": `{"fps": 8, "fpz": 9}`,
		"fps range":     `{"fps": 99}`,
		"crf range":     `{"crf": 64}`,
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
