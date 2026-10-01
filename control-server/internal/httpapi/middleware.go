package httpapi

import (
	"net/http"
	"strings"

	"share-app-host/internal/origin"
)

// guard adds the common response headers and rejects cross-origin requests
// before they reach any handler.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			header.Set("Cache-Control", "no-store")
		}
		if !origin.Same(r) {
			http.Error(w, "origin forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
