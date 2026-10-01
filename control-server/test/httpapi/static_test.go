package httpapi_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticContainment(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "client")
	e.writeClientFile(t, ".env", "hidden-secret")
	writeFile(t, filepath.Join(e.base, "private.key"), "sentinel-secret")
	writeFile(t, filepath.Join(e.base, "private.js"), "sentinel-secret")
	if err := os.Symlink(filepath.Join(e.base, "private.js"), filepath.Join(e.clientDir, "escape.js")); err != nil {
		t.Log("symlink check unavailable:", err)
	}

	for _, path := range []string{
		"/private.key", "/private.js", "/.env", "/escape.js",
		"/../private.key", "/%2e%2e/private.key", "/%2e%2e%5cprivate.key",
		"/C:/private.key", "/missing/",
	} {
		w := e.do("GET", path, "")
		if strings.Contains(w.Body.String(), "secret") || w.Code == http.StatusOK {
			t.Errorf("exposed %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := e.do("GET", "/", ""); w.Code != http.StatusOK || w.Body.String() != "client" {
		t.Fatalf("client unavailable: %d %q", w.Code, w.Body.String())
	}
}

func TestStaticServesAllowedFiles(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "<html>home</html>")
	e.writeClientFile(t, "assets/app.js", "console.log(1)")
	e.writeClientFile(t, "assets/app.css", "body{}")

	tests := []struct {
		path, body, contentType string
	}{
		{"/", "<html>home</html>", "text/html"},
		{"/index.html", "<html>home</html>", "text/html"},
		{"/assets/app.js", "console.log(1)", "javascript"},
		{"/assets/app.css", "body{}", "text/css"},
	}
	for _, tt := range tests {
		w := e.do("GET", tt.path, "")
		if w.Code != http.StatusOK || w.Body.String() != tt.body {
			t.Errorf("GET %s = %d %q", tt.path, w.Code, w.Body.String())
		}
		if got := w.Header().Get("Content-Type"); !strings.Contains(got, tt.contentType) {
			t.Errorf("GET %s Content-Type = %q, want %q", tt.path, got, tt.contentType)
		}
	}
}

func TestStaticEntryPageIsNotCachedButAssetsMayBe(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "x")
	e.writeClientFile(t, "assets/app.js", "x")

	if got := e.do("GET", "/", "").Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("index Cache-Control = %q", got)
	}
	if got := e.do("GET", "/assets/app.js", "").Header().Get("Cache-Control"); got != "" {
		t.Errorf("asset Cache-Control = %q, want none", got)
	}
}

func TestStaticRefusesOtherFileTypesAndDirectories(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "x")
	e.writeClientFile(t, "data.json", "{}")
	e.writeClientFile(t, "page.php", "<?php")
	e.writeClientFile(t, "noextension", "x")
	// Types the client does not ship are not served either.
	for _, name := range []string{"logo.png", "logo.svg", "favicon.ico", "photo.jpg", "photo.webp", "font.woff2"} {
		e.writeClientFile(t, name, "x")
	}
	e.writeClientFile(t, "dir.js/inner.txt", "x") // a directory named like a script

	for _, path := range []string{"/data.json", "/page.php", "/noextension", "/dir.js", "/dir.js/", "/assets/", "/.git/config",
		"/logo.png", "/logo.svg", "/favicon.ico", "/photo.jpg", "/photo.webp", "/font.woff2"} {
		if w := e.do("GET", path, ""); w.Code != http.StatusNotFound && w.Code != http.StatusMovedPermanently {
			t.Errorf("GET %s = %d, want 404", path, w.Code)
		}
	}
}

func TestStaticAllowsOnlyReadMethods(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "page")

	if w := e.do("HEAD", "/", ""); w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Errorf("HEAD / = %d with %d body bytes", w.Code, w.Body.Len())
	}
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if w := e.do(method, "/", ""); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s / = %d, want 405", method, w.Code)
		}
	}
}

func TestStaticWithoutAClientDirectoryIsNotFound(t *testing.T) {
	e := newEnv(t)
	if err := os.RemoveAll(e.clientDir); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/", ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET / = %d", w.Code)
	}
}

func TestStaticPicksUpARebuiltClient(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "old build")
	if w := e.do("GET", "/", ""); w.Body.String() != "old build" {
		t.Fatalf("first build: %q", w.Body.String())
	}
	// Replace the whole directory, as `npm run build` does.
	if err := os.RemoveAll(e.clientDir); err != nil {
		t.Fatal(err)
	}
	e.writeClientFile(t, "index.html", "new build")
	if w := e.do("GET", "/", ""); w.Body.String() != "new build" {
		t.Fatalf("rebuilt client not served: %q", w.Body.String())
	}
}
