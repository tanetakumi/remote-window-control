package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"share-app-host/internal/media"
)

// ErrInvalidSettings is wrapped by every Settings validation failure.
var ErrInvalidSettings = errors.New("invalid settings")

// Settings are the values edited in the web client's settings page and kept in
// the user's config.json. The API only reads and writes these settings.
type Settings struct {
	// FPS is the video encode rate while the window changes, minFPS to maxFPS.
	// A new value applies to the next connection.
	FPS int `json:"fps"`
	// CRF is the video quality, minCRF to maxCRF; lower is better and larger.
	// A new value applies to the next connection.
	CRF int `json:"crf"`
	// MaxScale is the most window pixels per CSS pixel of the controlling
	// browser's viewport, minMaxScale to maxMaxScale. The window is sized to the
	// viewport times the browser's device pixel ratio, limited to this, so a
	// lower value gives a smaller window and less video data. A new value
	// applies from the next viewport change.
	MaxScale float64 `json:"maxScale"`
	// ScrollSensitivity multiplies touch scrolling in both gesture modes.
	// A new value applies to the next connection.
	ScrollSensitivity float64 `json:"scrollSensitivity"`
}

// DefaultSettings returns the settings used while config.json does not exist.
func DefaultSettings() Settings {
	return Settings{FPS: media.DefaultFPS, CRF: media.DefaultCRF, MaxScale: DefaultMaxScale, ScrollSensitivity: 1}
}

// StartupSettings are edited in config.json and apply after restarting the host.
type StartupSettings struct {
	ListenAddr   string `json:"listenAddr"`
	CaptureStats string `json:"captureStats"`
}

type storedSettings struct {
	StartupSettings
	Settings
}

func (s StartupSettings) validate() error {
	host, port, err := net.SplitHostPort(s.ListenAddr)
	if err != nil || strings.ContainsAny(host, " \t\r\n") {
		return fmt.Errorf("%w: listenAddr must be a host:port address", ErrInvalidSettings)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("%w: listenAddr port must be from 1 to 65535", ErrInvalidSettings)
	}
	switch s.CaptureStats {
	case CaptureStatsOff, CaptureStatsOn, CaptureStatsVerify:
		return nil
	default:
		return fmt.Errorf("%w: captureStats must be off, on or verify", ErrInvalidSettings)
	}
}

// Validate reports whether the settings are within their allowed ranges.
func (s Settings) Validate() error {
	if s.FPS < minFPS || s.FPS > maxFPS {
		return fmt.Errorf("%w: fps must be an integer from %d to %d", ErrInvalidSettings, minFPS, maxFPS)
	}
	if s.CRF < minCRF || s.CRF > maxCRF {
		return fmt.Errorf("%w: crf must be an integer from %d to %d", ErrInvalidSettings, minCRF, maxCRF)
	}
	if !(s.MaxScale >= minMaxScale && s.MaxScale <= maxMaxScale) {
		return fmt.Errorf("%w: maxScale must be a number from %g to %g", ErrInvalidSettings, minMaxScale, maxMaxScale)
	}
	if !(s.ScrollSensitivity >= minScrollSensitivity && s.ScrollSensitivity <= maxScrollSensitivity) {
		return fmt.Errorf("%w: scrollSensitivity must be a number from %g to %g", ErrInvalidSettings, minScrollSensitivity, maxScrollSensitivity)
	}
	return nil
}

// SettingsStore holds the current Settings and persists changes to a JSON file.
type SettingsStore struct {
	path string

	mu      sync.Mutex
	current storedSettings
}

// OpenSettings loads the settings file at path. A missing file gives the
// defaults and creates the file; an incomplete, malformed or out-of-range file
// is an error.
func OpenSettings(path string) (*SettingsStore, error) {
	var current storedSettings
	data, err := os.ReadFile(path)
	missing := os.IsNotExist(err)
	switch {
	case missing:
		current = storedSettings{
			StartupSettings: StartupSettings{ListenAddr: defaultAddr, CaptureStats: CaptureStatsOff},
			Settings:        DefaultSettings(),
		}
	case err != nil:
		return nil, fmt.Errorf("%s: %w", path, err)
	default:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, name := range []string{"listenAddr", "captureStats", "fps", "crf", "maxScale", "scrollSensitivity"} {
			if value, ok := fields[name]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return nil, fmt.Errorf("%s: settings require %s", path, name)
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&current); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := current.StartupSettings.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := current.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	store := &SettingsStore{path: path, current: current}
	if missing {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := store.save(current); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return store, nil
}

// Get returns the current settings.
func (s *SettingsStore) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current.Settings
}

// Startup returns the configuration read at startup, without exposing it to
// the browser's settings API.
func (s *SettingsStore) Startup() StartupSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current.StartupSettings
}

// Set validates next, writes it to the settings file and makes it current. The
// current settings are unchanged when it fails.
func (s *SettingsStore) Set(next Settings) error {
	if err := next.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := s.current
	updated.Settings = next
	if err := s.save(updated); err != nil {
		return err
	}
	s.current = updated
	return nil
}

func (s *SettingsStore) save(next storedSettings) error {
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, append(data, '\n'))
}

// writeFileAtomic replaces path through a temporary file so a crash never
// leaves a partly written file.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
