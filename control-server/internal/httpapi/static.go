package httpapi

import (
	"net/http"
	"os"
	"path"
	"strings"
)

// staticExtensions are the only file types served from the client directory.
var staticExtensions = map[string]bool{
	".html": true, ".css": true, ".js": true,
	".png": true, ".svg": true, ".ico": true,
	".jpg": true, ".jpeg": true, ".webp": true,
	".woff": true, ".woff2": true,
}

// handleStatic serves the web client. Access is limited to regular files of an
// allowed type below ClientDir: dot-files, backslashes, drive prefixes and
// path escapes are refused, and the directory is opened as a root, so symlinks
// cannot lead out of it either. The directory is opened per request so a
// rebuilt client is picked up without restarting.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if !staticAllowed(name) {
		http.NotFound(w, r)
		return
	}

	root, err := os.OpenRoot(s.clientDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}

	if name == "index.html" {
		// Asset names are fingerprinted by the build; the entry page is not.
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

// staticAllowed reports whether a slash-separated request path may be served.
func staticAllowed(name string) bool {
	if !staticExtensions[strings.ToLower(path.Ext(name))] {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.ContainsAny(part, `\:`) {
			return false
		}
	}
	return true
}
