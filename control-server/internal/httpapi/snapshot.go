package httpapi

import (
	"log"
	"net/http"
)

// handleSnapshot returns the selected window as a PNG. It is a debugging aid.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.snapshotMu.TryLock() {
		http.Error(w, "snapshot already in progress", http.StatusConflict)
		return
	}
	defer s.snapshotMu.Unlock()

	current, ok := s.windows.Current()
	if !ok || current.Handle == 0 {
		http.Error(w, "target window not selected", http.StatusBadRequest)
		return
	}
	data, err := s.snapshots.CapturePNG(r.Context(), current.Handle)
	if err != nil {
		log.Printf("snapshot failed hwnd=%d: %v", current.Handle, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}
