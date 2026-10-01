package httpapi

import "net/http"

// handleSnapshot returns the selected window as a PNG. It is a debugging aid.
// File output and per-request window selection were removed, so those
// parameters are refused rather than silently ignored.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Has("out") || query.Has("hwnd") {
		http.Error(w, "snapshot parameters out and hwnd are not supported", http.StatusBadRequest)
		return
	}
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}
