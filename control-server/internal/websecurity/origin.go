package websecurity

import (
	"net/http"
	"net/url"
	"strings"
)

// SameOrigin also permits command-line clients, which do not send Origin.
// HTTPS may terminate outside this process; Host remains the public authority.
func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && strings.EqualFold(u.Host, r.Host)
}
