package common

import "net/url"

// IsTrustedOrigin reports whether a state-changing request is safe from CSRF
// based on its Origin/Referer headers.
//
// Browsers always attach an Origin (or at least a Referer) to cross-site unsafe
// requests and cannot forge or strip the Origin header, so a present-but-
// mismatched value signals a cross-site attack. When both headers are absent
// the caller is a non-browser client (curl, scripts) which is not a CSRF
// vector, so it is allowed. This is the OWASP "verify origin via standard
// headers" defense, complementing the SameSite=Lax session cookie.
func IsTrustedOrigin(origin, referer, host string) bool {
	source := origin
	if source == "" {
		source = referer
	}
	if source == "" {
		return true
	}
	u, err := url.Parse(source)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Host == host
}
