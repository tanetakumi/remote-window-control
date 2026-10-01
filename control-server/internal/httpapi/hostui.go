package httpapi

import (
	_ "embed"
	"io"
	"net/http"
)

// hostUIHTML is the page served at /host-ui, where the host picks the window to
// share. It is self-contained and talks to the same API as the web client.
//
//go:embed hostui.html
var hostUIHTML string

func (s *Server) handleHostUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, hostUIHTML)
}
