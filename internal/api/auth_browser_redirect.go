// Spec 057 Scope 2 — browser-navigation detection for content-
// negotiated 303 redirect in bearerAuthMiddleware.
//
// Four-gate check (design.md §"Content-Negotiation Logic"):
//  1. Method gate: only GET/HEAD (safe idempotent navigations).
//  2. HTMX suppression: HX-Request: true is an in-page fetch, NOT a top-level nav.
//  3. Sec-Fetch-Mode gate: when present, only "navigate" qualifies.
//  4. Accept gate: explicit text/html required. `*/*` (curl) is NOT enough.
//
// All four must pass for the middleware to issue a 303 instead of 401.
// Anything else preserves spec 044's wire contract byte-for-byte.
package api

import (
	"net/http"
	"net/url"
	"strings"
)

func isBrowserNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if strings.EqualFold(r.Header.Get("HX-Request"), "true") {
		return false
	}
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" && mode != "navigate" {
		return false
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return false
	}
	return true
}

// redirectToLogin issues a 303 See Other to /login with the current
// request URI sanitised into the `next` query param.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	next := sanitizeNext(r.URL.RequestURI())
	dest := "/login?next=" + url.QueryEscape(next)
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
