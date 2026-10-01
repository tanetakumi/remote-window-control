package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

// maxTargetRequestBytes bounds the body of a target selection request.
const maxTargetRequestBytes = 4096

type selectRequest struct {
	Handle uint64 `json:"handle"`
}

func (s *Server) handleListWindows(w http.ResponseWriter, r *http.Request) {
	windows, err := s.windows.List(r.Context())
	if err != nil {
		log.Printf("window enumeration failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, windows)
}

func (s *Server) handleSelectTarget(w http.ResponseWriter, r *http.Request) {
	var request selectRequest
	body := http.MaxBytesReader(w, r.Body, maxTargetRequestBytes)
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		http.Error(w, "invalid target request", http.StatusBadRequest)
		return
	}
	selected, err := s.windows.Select(r.Context(), request.Handle)
	if err != nil {
		log.Printf("target selection failed hwnd=%d: %v", request.Handle, err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	log.Printf("target selected hwnd=%d process=%q pid=%d title=%q class=%q", selected.Handle, selected.ProcessName, selected.ProcessID, selected.Title, selected.ClassName)
	writeJSON(w, selected)
}
