package httpapi_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"share-app-host/internal/httpapi"
	"share-app-host/internal/rdpkeep"
)

// fakeKeepalive is an in-memory httpapi.Keepalive.
type fakeKeepalive struct {
	status rdpkeep.Status
	err    error
}

func (f *fakeKeepalive) Status() rdpkeep.Status { return f.status }

func (f *fakeKeepalive) SetEnabled(enabled bool) (rdpkeep.Status, error) {
	if f.err != nil {
		return f.status, f.err
	}
	f.status = rdpkeep.Status{State: rdpkeep.StateOff}
	if enabled {
		f.status = rdpkeep.Status{State: rdpkeep.StateConnecting, Enabled: true}
	}
	return f.status, nil
}

func withKeepalive(k httpapi.Keepalive) envOption {
	return func(o *httpapi.Options) { o.Keepalive = k }
}

func TestSessionKeepaliveToggle(t *testing.T) {
	keeper := &fakeKeepalive{status: rdpkeep.Status{State: rdpkeep.StateOff}}
	e := newEnv(t, withKeepalive(keeper))

	w := e.doBody("POST", "/api/session-keepalive", "", []byte(`{"enabled":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", w.Code, w.Body.String())
	}
	if got := decode[rdpkeep.Status](t, w); got != keeper.status || !got.Enabled {
		t.Fatalf("POST returned %+v, want %+v", got, keeper.status)
	}
	if got := decode[rdpkeep.Status](t, e.do("GET", "/api/session-keepalive", "")); got != keeper.status {
		t.Fatalf("GET returned %+v, want %+v", got, keeper.status)
	}
}

func TestSessionKeepaliveErrors(t *testing.T) {
	keeper := &fakeKeepalive{err: errors.New("RDP username or password is not configured")}
	e := newEnv(t, withKeepalive(keeper))

	if w := e.doBody("POST", "/api/session-keepalive", "", []byte(`{"enabled":`)); w.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", w.Code)
	}
	w := e.doBody("POST", "/api/session-keepalive", "", []byte(`{"enabled":true}`))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), keeper.err.Error()) {
		t.Errorf("refused start = %d %q, want 400 with the reason", w.Code, w.Body.String())
	}
	if w := e.doBody("POST", "/api/session-keepalive", "https://other-host", []byte(`{"enabled":true}`)); w.Code != http.StatusForbidden {
		t.Errorf("cross-origin toggle = %d, want 403", w.Code)
	}
}

func TestWithoutAKeepaliveItsEndpointIsNotFound(t *testing.T) {
	e := newEnv(t)
	if w := e.do("GET", "/api/session-keepalive", ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/session-keepalive = %d", w.Code)
	}
}
