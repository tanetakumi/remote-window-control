package httpapi_test

import (
	"errors"
	"net/http"
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
	w := e.doBody("PUT", "/api/settings", "", []byte(`{"fps":20,"crf":24,"newline":"shift-enter"}`))
	want := config.Settings{FPS: 20, CRF: 24, Newline: config.NewlineShiftEnter}
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
		`{"fps":8}`,
		`{"fps":99,"crf":31,"newline":"enter"}`,
		`{"fps":8,"crf":64,"newline":"enter"}`,
		`{"fps":8,"crf":31,"newline":"enter","extra":1}`,
		`{"fps":"8","crf":31,"newline":"enter"}`,
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
	w := e.doBody("PUT", "/api/settings", "", []byte(`{"fps":8,"crf":31,"newline":"enter"}`))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("PUT = %d, want 500", w.Code)
	}
}

func TestSettingsRejectCrossOriginRequests(t *testing.T) {
	e := newEnv(t, withSettings(&fakeSettings{current: config.DefaultSettings()}))
	if w := e.doBody("PUT", "/api/settings", "https://other-host", []byte(`{"fps":8,"crf":31,"newline":"enter"}`)); w.Code != http.StatusForbidden {
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
