package httpapi_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"share-app-host/internal/config"
	"share-app-host/internal/httpapi"
)

// fakeSettings is an in-memory httpapi.Settings that validates like the real one.
type fakeSettings struct {
	current config.Settings
	saveErr error
}

func (f *fakeSettings) Get() config.Settings { return f.current }

func (f *fakeSettings) Set(next config.Settings) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if f.saveErr != nil {
		return f.saveErr
	}
	f.current = next
	return nil
}

func withSettings(s httpapi.Settings) envOption {
	return func(o *httpapi.Options) { o.Settings = s }
}

func TestSettingsCanBeReadAndSaved(t *testing.T) {
	settings := &fakeSettings{current: config.DefaultSettings()}
	e := newEnv(t, withSettings(settings))

	if got := decode[config.Settings](t, e.do("GET", "/api/settings", "")); got != config.DefaultSettings() {
		t.Fatalf("GET = %+v", got)
	}
	w := e.doBody("PUT", "/api/settings", "", []byte(`{"fps":20,"crf":24,"maxScale":1.5,"scrollSensitivity":2.5}`))
	want := config.Settings{FPS: 20, CRF: 24, MaxScale: 1.5, ScrollSensitivity: 2.5}
	if w.Code != http.StatusOK || decode[config.Settings](t, w) != want {
		t.Fatalf("PUT = %d %s", w.Code, w.Body)
	}
	if settings.current != want {
		t.Fatalf("saved %+v, want %+v", settings.current, want)
	}
	if got := decode[config.Settings](t, e.do("GET", "/api/settings", "")); got != want {
		t.Fatalf("GET after PUT = %+v", got)
	}
}

func TestInvalidSettingsRequestsAreRejected(t *testing.T) {
	settings := &fakeSettings{current: config.DefaultSettings()}
	e := newEnv(t, withSettings(settings))

	for _, body := range []string{
		`{"fps":`,
		`{"crf":31,"maxScale":2,"scrollSensitivity":1}`,
		`{"fps":99,"crf":31,"maxScale":2,"scrollSensitivity":1}`,
		`{"fps":8,"crf":64,"maxScale":2,"scrollSensitivity":1}`,
		`{"fps":8,"crf":31,"scrollSensitivity":1}`,
		`{"fps":8,"crf":31,"maxScale":9,"scrollSensitivity":1}`,
		`{"fps":8,"crf":31,"maxScale":2,"scrollSensitivity":1,"extra":1}`,
		`{"fps":"8","crf":31,"maxScale":2,"scrollSensitivity":1}`,
		`{"fps":8,"crf":31,"maxScale":2}`,
		`{"fps":8,"crf":31,"maxScale":2,"scrollSensitivity":0}`,
		`{"fps":8,"crf":31,"maxScale":2,"scrollSensitivity":5}`,
		`{"fps":8,"crf":31,"maxScale":2,"scrollSensitivity":"2"}`,
	} {
		if w := e.doBody("PUT", "/api/settings", "", []byte(body)); w.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s = %d, want 400", body, w.Code)
		}
	}
	if settings.current != config.DefaultSettings() {
		t.Fatalf("settings changed to %+v", settings.current)
	}
}

func TestSettingsWriteFailureIsAServerError(t *testing.T) {
	e := newEnv(t, withSettings(&fakeSettings{current: config.DefaultSettings(), saveErr: errors.New("disk full")}))
	w := e.doBody("PUT", "/api/settings", "", []byte(`{"fps":8,"crf":31,"maxScale":2,"scrollSensitivity":1}`))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("PUT = %d, want 500", w.Code)
	}
}

func TestSettingsRejectCrossOriginRequests(t *testing.T) {
	e := newEnv(t, withSettings(&fakeSettings{current: config.DefaultSettings()}))
	if w := e.doBody("PUT", "/api/settings", "https://other-host", []byte(`{"fps":8,"crf":31,"maxScale":2,"scrollSensitivity":1}`)); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin PUT = %d, want 403", w.Code)
	}
	if w := e.do("GET", "/api/settings", "https://other-host"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin GET = %d, want 403", w.Code)
	}
}

func TestWithoutSettingsTheirEndpointIsNotFound(t *testing.T) {
	e := newEnv(t)
	if w := e.do("GET", "/api/settings", ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/settings = %d", w.Code)
	}
}

func TestVideoAPISavesPreserveStartupSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"listenAddr":":9000","captureStats":"verify","fps":8,"crf":31,"maxScale":2,"scrollSensitivity":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, withSettings(store))
	if w := e.doBody("PUT", "/api/settings", "", []byte(`{"fps":12,"crf":20,"maxScale":1.5,"scrollSensitivity":2.5}`)); w.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", w.Code, w.Body)
	}
	reopened, err := config.OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Startup(); got.ListenAddr != ":9000" || got.CaptureStats != "verify" {
		t.Fatalf("video save replaced startup settings: %+v", got)
	}
	if reopened.Get() != (config.Settings{FPS: 12, CRF: 20, MaxScale: 1.5, ScrollSensitivity: 2.5}) {
		t.Fatal("video settings were not saved")
	}
	// These properties are edited on disk; no browser API can change them.
	for _, field := range []string{`"listenAddr":":8443"`, `"captureStats":"off"`, `"username":"someone"`, `"password":"secret"`} {
		body := []byte(`{"fps":12,"crf":20,"maxScale":1.5,"scrollSensitivity":1,` + field + `}`)
		if w := e.doBody("PUT", "/api/settings", "", body); w.Code != http.StatusBadRequest {
			t.Fatalf("startup or credential field accepted: %d", w.Code)
		}
	}
}
