package origin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"share-app-host/internal/origin"
)

func TestSame(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		headers map[string]string
		want    bool
	}{
		{"no origin, no fetch metadata (CLI client)", "host:8443", nil, true},
		{"no origin, same-origin fetch", "host:8443", map[string]string{"Sec-Fetch-Site": "same-origin"}, true},
		{"no origin, cross-site fetch", "host:8443", map[string]string{"Sec-Fetch-Site": "cross-site"}, false},
		{"http origin with same host", "host:8443", map[string]string{"Origin": "http://host:8443"}, true},
		{"https origin behind a TLS proxy", "host:8443", map[string]string{"Origin": "https://host:8443"}, true},
		{"host comparison ignores case", "Host:8443", map[string]string{"Origin": "https://host:8443"}, true},
		{"different host", "host:8443", map[string]string{"Origin": "https://other-host"}, false},
		{"different port", "host:8443", map[string]string{"Origin": "https://host:9000"}, false},
		{"opaque origin", "host:8443", map[string]string{"Origin": "null"}, false},
		{"non-http scheme", "host:8443", map[string]string{"Origin": "ftp://host:8443"}, false},
		{"origin with path", "host:8443", map[string]string{"Origin": "https://host:8443/x"}, false},
		{"origin with trailing slash", "host:8443", map[string]string{"Origin": "https://host:8443/"}, false},
		{"origin with query", "host:8443", map[string]string{"Origin": "https://host:8443?x=1"}, false},
		{"origin with fragment", "host:8443", map[string]string{"Origin": "https://host:8443#x"}, false},
		{"origin with userinfo", "host:8443", map[string]string{"Origin": "https://user@host:8443"}, false},
		{"unparsable origin", "host:8443", map[string]string{"Origin": "http://[::1"}, false},
		{"cross-site metadata does not override a matching origin", "host:8443",
			map[string]string{"Origin": "https://host:8443", "Sec-Fetch-Site": "cross-site"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/", nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := origin.Same(r); got != tt.want {
				t.Fatalf("Same() = %v, want %v", got, tt.want)
			}
		})
	}
}
