// Package origin implements the same-origin policy applied to every HTTP and
// WebSocket request, because the host exposes unauthenticated control APIs.
package origin

import (
	"net/http"
	"net/url"
	"strings"
)

// Same reports whether r may be served. A request without an Origin header is
// allowed unless the browser marks it cross-site, which also permits
// command-line clients that send no Origin. HTTPS may terminate outside this
// process, so only the Host (the public authority) is compared, not the scheme.
func Same(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") &&
		u.User == nil &&
		u.Path == "" && u.RawQuery == "" && u.Fragment == "" &&
		strings.EqualFold(u.Host, r.Host)
}
