package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"share-app-host/internal/auth"
	"share-app-host/internal/nativecapture"
	"share-app-host/internal/targetwindow"
	"strings"
	"testing"
)

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	base := t.TempDir()
	web := filepath.Join(base, "web")
	if err := os.Mkdir(web, 0700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{filepath.Join(web, "index.html"): "client", filepath.Join(base, "private.key"): "sentinel-secret", filepath.Join(base, "private.js"): "sentinel-secret", filepath.Join(web, ".env"): "hidden-secret"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := auth.NewStore("secret")
	session, err := store.Exchange("secret")
	if err != nil {
		t.Fatal(err)
	}
	bridge := nativecapture.NewBridge(base)
	return New(":8443", web, store, http.NotFoundHandler(), bridge, targetwindow.NewManager(bridge), base), session.Token
}
func request(s *Server, method, path, token, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://host:8443"+path, nil)
	r.RemoteAddr = "127.0.0.1:1234"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(w, r)
	return w
}
func TestStaticContainment(t *testing.T) {
	s, _ := testServer(t)
	if err := os.Symlink(filepath.Join(filepath.Dir(s.clientDir), "private.js"), filepath.Join(s.clientDir, "escape.js")); err != nil {
		t.Log("symlink check unavailable:", err)
	}
	for _, path := range []string{"/private.key", "/private.js", "/.env", "/escape.js", "/../private.key", "/%2e%2e/private.key", "/%2e%2e%5cprivate.key", "/C:/private.key", "/missing/"} {
		w := request(s, "GET", path, "", "")
		if strings.Contains(w.Body.String(), "secret") || w.Code == 200 {
			t.Errorf("exposed %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := request(s, "GET", "/", "", ""); w.Code != 200 || w.Body.String() != "client" {
		t.Fatalf("client unavailable: %d", w.Code)
	}
}
func TestAPIsRequireAuthEvenOnLoopback(t *testing.T) {
	s, token := testServer(t)
	for _, path := range []string{"/api/config", "/api/windows", "/api/target-window", "/api/snapshot"} {
		if w := request(s, "GET", path, "", ""); w.Code != 401 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	if w := request(s, "GET", "/api/config", token, ""); w.Code != 200 {
		t.Fatalf("authorized config: %d", w.Code)
	}
	if w := request(s, "GET", "/api/config", token, "https://other-host"); w.Code != 403 {
		t.Fatalf("cross-origin config: %d", w.Code)
	}
	if w := request(s, "GET", "/api/config", token, "https://host:8443"); w.Code != 200 {
		t.Fatalf("HTTPS proxy origin: %d", w.Code)
	}
	if w := request(s, "GET", "/host-ui", "", ""); w.Code != 200 {
		t.Fatalf("login shell: %d", w.Code)
	}
}
func TestSnapshotRejectsUnrestrictedOutputBeforeCapture(t *testing.T) {
	s, token := testServer(t)
	for _, name := range []string{"../secret.png", "d:/secret.png", "CON.png", "NUL.png", "COM1.png", "LPT9.png", ".hidden.png", "a.exe", "a.png:stream", "folder/file.png"} {
		if snapshotName(name) {
			t.Errorf("accepted %q", name)
		}
	}
	if w := request(s, "GET", "/api/snapshot?hwnd=1&out=../secret.png", token, ""); w.Code != 400 {
		t.Fatalf("snapshot path: %d", w.Code)
	}
	if !snapshotName("window-2026.png") {
		t.Fatal("valid filename rejected")
	}
}
