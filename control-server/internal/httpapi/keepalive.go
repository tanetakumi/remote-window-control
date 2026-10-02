package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

type keepaliveRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleKeepaliveStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.keepalive.Status())
}

func (s *Server) handleKeepaliveSet(w http.ResponseWriter, r *http.Request) {
	var request keepaliveRequest
	body := http.MaxBytesReader(w, r.Body, maxTargetRequestBytes)
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		http.Error(w, "invalid keep-alive request", http.StatusBadRequest)
		return
	}
	status, err := s.keepalive.SetEnabled(request.Enabled)
	if err != nil {
		log.Printf("session keep-alive request failed enabled=%v: %v", request.Enabled, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, status)
}
