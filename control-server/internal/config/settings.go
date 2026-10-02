package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"share-app-host/internal/media"
)

// ErrInvalidSettings is wrapped by every Settings validation failure.
var ErrInvalidSettings = errors.New("invalid settings")

// Settings are the values edited in the web client's settings page and kept in
// config.json beside the .env file. Unlike .env they change while the host runs.
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
}

// DefaultSettings returns the settings used while config.json does not exist.
func DefaultSettings() Settings {
	return Settings{FPS: media.DefaultFPS, CRF: media.DefaultCRF, MaxScale: DefaultMaxScale}
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
	return nil
}

// SettingsStore holds the current Settings and persists changes to a JSON file.
type SettingsStore struct {
	path string

	mu      sync.Mutex
	current Settings
}

// OpenSettings loads the settings file at path. A missing file gives the
// defaults; a malformed or out-of-range file is an error.
func OpenSettings(path string) (*SettingsStore, error) {
	current := DefaultSettings()
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, err
	default:
		// Fields missing from the file keep their defaults.
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&current); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := current.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return &SettingsStore{path: path, current: current}, nil
}

// Get returns the current settings.
func (s *SettingsStore) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Set validates next, writes it to the settings file and makes it current. The
// current settings are unchanged when it fails.
func (s *SettingsStore) Set(next Settings) error {
	if err := next.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeFileAtomic(s.path, append(data, '\n')); err != nil {
		return err
	}
	s.current = next
	return nil
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
