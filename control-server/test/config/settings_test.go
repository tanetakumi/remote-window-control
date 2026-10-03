package config_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"share-app-host/internal/config"
)

const completeSettingsJSON = `{"listenAddr":":9000","captureStats":"on","fps":12,"crf":24,"maxScale":1.5,"scrollSensitivity":1}`

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
	saved := config.Settings{FPS: 15, CRF: 20, MaxScale: 1.5, ScrollSensitivity: 2.5}
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
	if got, want := store.Get(), (config.Settings{FPS: 8, CRF: 31, MaxScale: 2, ScrollSensitivity: 1}); got != want {
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
	// Zero is a valid explicit CRF, but an omitted CRF must be rejected.
	saved := config.Settings{FPS: 15, CRF: 0, MaxScale: 1.5, ScrollSensitivity: 2.5}
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
		{FPS: 0, CRF: 31, MaxScale: 2, ScrollSensitivity: 1},
		{FPS: 31, CRF: 31, MaxScale: 2, ScrollSensitivity: 1},
		{FPS: 8, CRF: -1, MaxScale: 2, ScrollSensitivity: 1},
		{FPS: 8, CRF: 64, MaxScale: 2, ScrollSensitivity: 1},
		{FPS: 8, CRF: 31, MaxScale: 0, ScrollSensitivity: 1},
		{FPS: 8, CRF: 31, MaxScale: 0.25, ScrollSensitivity: 1},
		{FPS: 8, CRF: 31, MaxScale: 4.5, ScrollSensitivity: 1},
		{FPS: 8, CRF: 31, MaxScale: math.NaN(), ScrollSensitivity: 1},
		{FPS: 8, CRF: 31, MaxScale: 2, ScrollSensitivity: 0},
		{FPS: 8, CRF: 31, MaxScale: 2, ScrollSensitivity: 0.25},
		{FPS: 8, CRF: 31, MaxScale: 2, ScrollSensitivity: 4.5},
		{FPS: 8, CRF: 31, MaxScale: 2, ScrollSensitivity: math.NaN()},
		{FPS: 8, CRF: 31, MaxScale: 2, ScrollSensitivity: math.Inf(1)},
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
	err = store.Set(config.Settings{FPS: 12, CRF: 31, MaxScale: 2, ScrollSensitivity: 1})
	if err == nil || errors.Is(err, config.ErrInvalidSettings) {
		t.Fatalf("Set = %v, want a write error", err)
	}
	if got := store.Get(); got != config.DefaultSettings() {
		t.Fatalf("settings changed to %+v", got)
	}
}

func TestIncompleteSettingsFileIsRejectedWithoutRewriting(t *testing.T) {
	for _, field := range []string{"listenAddr", "captureStats", "fps", "crf", "maxScale", "scrollSensitivity"} {
		for _, null := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/null=%t", field, null), func(t *testing.T) {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal([]byte(completeSettingsJSON), &fields); err != nil {
					t.Fatal(err)
				}
				if null {
					fields[field] = json.RawMessage(`null`)
				} else {
					delete(fields, field)
				}
				data, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				path := settingsPath(t)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := config.OpenSettings(path); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), field) {
					t.Fatalf("error = %v, want one naming %s and %s", err, path, field)
				}
				if after, err := os.ReadFile(path); err != nil || string(after) != string(data) {
					t.Fatalf("a rejected load changed the file (err %v)", err)
				}
			})
		}
	}
}

func TestBadSettingsFileFailsAndNamesTheFile(t *testing.T) {
	for name, content := range map[string]string{
		"syntax":               `{"fps": `,
		"unknown field":        strings.Replace(completeSettingsJSON, `}`, `,"fpz":9}`, 1),
		"fps range":            strings.Replace(completeSettingsJSON, `"fps":12`, `"fps":99`, 1),
		"crf range":            strings.Replace(completeSettingsJSON, `"crf":24`, `"crf":64`, 1),
		"scale range":          strings.Replace(completeSettingsJSON, `"maxScale":1.5`, `"maxScale":5`, 1),
		"scroll zero":          strings.Replace(completeSettingsJSON, `"scrollSensitivity":1`, `"scrollSensitivity":0`, 1),
		"scroll range":         strings.Replace(completeSettingsJSON, `"scrollSensitivity":1`, `"scrollSensitivity":5`, 1),
		"null":                 `null`,
		"trailing object":      `{} {}`,
		"trailing garbage":     `{} broken`,
		"address without port": strings.Replace(completeSettingsJSON, `":9000"`, `"127.0.0.1"`, 1),
		"port range":           strings.Replace(completeSettingsJSON, `":9000"`, `"127.0.0.1:65536"`, 1),
		"port zero":            strings.Replace(completeSettingsJSON, `":9000"`, `":0"`, 1),
		"diagnostics mode":     strings.Replace(completeSettingsJSON, `"captureStats":"on"`, `"captureStats":"yes"`, 1),
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

func TestScrollSensitivityRangeIncludesBothEndpoints(t *testing.T) {
	for _, sensitivity := range []float64{0.5, 4} {
		settings := config.DefaultSettings()
		settings.ScrollSensitivity = sensitivity
		if err := settings.Validate(); err != nil {
			t.Fatalf("sensitivity %g: %v", sensitivity, err)
		}
	}
}
