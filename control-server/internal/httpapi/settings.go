package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"share-app-host/internal/config"
)

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.settings.Get())
}

func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var request config.Settings
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTargetRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid settings request", http.StatusBadRequest)
		return
	}
	if err := s.settings.Set(request); err != nil {
		if errors.Is(err, config.ErrInvalidSettings) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("saving settings failed: %v", err)
		http.Error(w, "could not save the settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, request)
}
