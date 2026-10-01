package httpapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"share-app-host/internal/window"
)

var notepad = window.Info{Handle: 100, Title: "Untitled - Notepad", ProcessID: 42, ProcessName: "notepad", ClassName: "Notepad"}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("response is not the expected JSON (%v): %s", err, w.Body.String())
	}
	return v
}

func TestEndpointsRejectCrossOriginRequests(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "client")
	for _, target := range []string{"/api/windows", "/api/target-window", "/api/snapshot", "/host-ui", "/ws", "/"} {
		if w := e.do("GET", target, "https://other-host"); w.Code != http.StatusForbidden {
			t.Errorf("cross-origin GET %s = %d, want 403", target, w.Code)
		}
	}
	if w := e.do("POST", "/api/target-window", "https://other-host"); w.Code != http.StatusForbidden {
		t.Errorf("cross-origin target change = %d, want 403", w.Code)
	}
	if w := e.doBody("POST", "/api/target-window", "https://other-host", []byte(`{"handle":100}`)); w.Code != http.StatusForbidden {
		t.Errorf("cross-origin target change with a valid body = %d, want 403", w.Code)
	}
	if _, ok := e.windows.Current(); ok {
		t.Error("a cross-origin request changed the target")
	}
}

func TestEndpointsAcceptTheSameOrigin(t *testing.T) {
	e := newEnv(t)
	for _, origin := range []string{"", "http://host:8443", "https://host:8443"} { // the last is a TLS proxy
		if w := e.do("GET", "/api/windows", origin); w.Code != http.StatusOK {
			t.Errorf("origin %q: GET /api/windows = %d", origin, w.Code)
		}
	}
}

func TestResponsesCarrySecurityHeaders(t *testing.T) {
	e := newEnv(t)
	e.writeClientFile(t, "index.html", "client")

	for _, target := range []string{"/api/windows", "/", "/host-ui", "/missing"} {
		w := e.do("GET", target, "")
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q", target, got)
		}
		if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s: Referrer-Policy = %q", target, got)
		}
	}
	if got := e.do("GET", "/api/windows", "").Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("API Cache-Control = %q", got)
	}
	// Even a refused request must not be cached or sniffed.
	w := e.do("GET", "/api/windows", "https://other-host")
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers missing on a refused request: %v", w.Header())
	}
}

func TestListWindows(t *testing.T) {
	e := newEnv(t)
	e.windows.windows = []window.Info{notepad, {Handle: 200, Title: "Calc"}}

	w := e.do("GET", "/api/windows", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/windows = %d", w.Code)
	}
	got := decode[[]window.Info](t, w)
	if len(got) != 2 || got[0] != notepad || got[1].Title != "Calc" {
		t.Fatalf("windows = %+v", got)
	}
	// The wire format the web client reads.
	if !strings.Contains(w.Body.String(), `"process_name":"notepad"`) || !strings.Contains(w.Body.String(), `"handle":100`) {
		t.Fatalf("unexpected JSON field names: %s", w.Body.String())
	}
}

func TestListWindowsReportsAHelperFailureAsBadGateway(t *testing.T) {
	e := newEnv(t)
	e.windows.listErr = errors.New("capture probe unavailable")
	w := e.do("GET", "/api/windows", "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "capture probe unavailable") {
		t.Fatalf("GET /api/windows = %d %q", w.Code, w.Body.String())
	}
}

func TestTargetWindowSelection(t *testing.T) {
	e := newEnv(t)
	e.windows.windows = []window.Info{notepad}

	type targetResponse struct {
		Selected *window.Info `json:"selected"`
	}
	if w := e.do("GET", "/api/target-window", ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"selected":null}` {
		t.Fatalf("nothing selected: %d %s", w.Code, w.Body.String())
	}

	w := e.doBody("POST", "/api/target-window", "", []byte(`{"handle":100}`))
	if w.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", w.Code, w.Body.String())
	}
	if got := decode[window.Info](t, w); got != notepad {
		t.Fatalf("POST returned %+v", got)
	}

	got := decode[targetResponse](t, e.do("GET", "/api/target-window", ""))
	if got.Selected == nil || *got.Selected != notepad {
		t.Fatalf("GET after selecting = %+v", got.Selected)
	}
}

func TestTargetWindowSelectionErrors(t *testing.T) {
	e := newEnv(t)
	e.windows.windows = []window.Info{notepad}

	tests := []struct {
		name string
		body string
		want int
	}{
		{"no body", "", http.StatusBadRequest},
		{"malformed JSON", "{", http.StatusBadRequest},
		{"wrong type", `{"handle":"abc"}`, http.StatusBadRequest},
		{"negative handle", `{"handle":-1}`, http.StatusBadRequest},
		{"body over the size limit", `{"handle":100,"pad":"` + strings.Repeat("x", 5000) + `"}`, http.StatusBadRequest},
		{"window that is not listed", `{"handle":999}`, http.StatusConflict},
		{"missing handle selects nothing real", `{}`, http.StatusConflict},
	}
	for _, tt := range tests {
		w := e.doBody("POST", "/api/target-window", "", []byte(tt.body))
		if w.Code != tt.want {
			t.Errorf("%s: POST = %d %q, want %d", tt.name, w.Code, w.Body.String(), tt.want)
		}
	}
	if _, ok := e.windows.Current(); ok {
		t.Error("a failed request changed the target")
	}

	e.windows.selectErr = errors.New("control server is busy")
	w := e.doBody("POST", "/api/target-window", "", []byte(`{"handle":100}`))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "control server is busy") {
		t.Errorf("selection failure: %d %q", w.Code, w.Body.String())
	}
}

func TestTargetWindowRejectsOtherMethods(t *testing.T) {
	e := newEnv(t)
	for _, method := range []string{"PUT", "DELETE", "PATCH"} {
		if w := e.do(method, "/api/target-window", ""); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/target-window = %d, want 405", method, w.Code)
		}
	}
}

func TestUnknownAPIPathsAreNotFound(t *testing.T) {
	e := newEnv(t)
	for _, target := range []string{"/api/session", "/api/", "/api/nope"} {
		if w := e.do("GET", target, ""); w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, w.Code)
		}
	}
}

func TestHostUI(t *testing.T) {
	e := newEnv(t)
	w := e.do("GET", "/host-ui", "")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("GET /host-ui = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "Select a target window") {
		t.Fatal("host UI page not served")
	}
	if w := e.do("POST", "/host-ui", ""); w.Code == http.StatusOK {
		t.Fatal("POST /host-ui accepted")
	}
}

func TestControlWebSocketHandlerIsMountedAtWS(t *testing.T) {
	var reached bool
	e := newEnv(t, withControl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusTeapot)
	})))
	if w := e.do("GET", "/ws", ""); w.Code != http.StatusTeapot || !reached {
		t.Fatalf("GET /ws = %d (handler reached: %v)", w.Code, reached)
	}
}

func TestWithoutAControlHandlerWSIsNotFound(t *testing.T) {
	e := newEnv(t)
	if w := e.do("GET", "/ws", ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET /ws = %d", w.Code)
	}
}
