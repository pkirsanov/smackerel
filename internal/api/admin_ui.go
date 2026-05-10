// Spec 044 Scope 03 — Admin UI handler.
//
// Serves the static admin token-management page at
// /admin/auth/tokens. The page is registered behind
// bearerAuthMiddleware in router.go so cookie- or header-bearing
// callers must already hold a valid session before they can render
// the UI. The page itself contains no secrets — the JavaScript
// inside calls the existing Scope 02 admin REST endpoints under
// /v1/auth/* and those endpoints independently enforce the
// callerIsAdmin gate.
//
// The HTML is embedded at compile time so there is no run-time file
// I/O and no static-file root to misconfigure (NFR-AUTH-014 spirit:
// no environment-specific surface in the binary).
package api

import (
	"embed"
	"net/http"
)

//go:embed admin_ui_static/tokens.html
var adminUIFS embed.FS

// HandleAdminTokensUI serves the per-user PASETO admin UI HTML.
//
// Caching: no-store so a freshly-rotated UI is always picked up;
// the page is small and uncached overhead is negligible.
//
// Method: GET only. Anything else returns 405 (which the chi router
// already enforces for the registered route, but we double-check
// here as defense-in-depth in case the registration is reused under
// a chi.Mux that mounts the handler differently).
func (d *Dependencies) HandleAdminTokensUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := adminUIFS.ReadFile("admin_ui_static/tokens.html")
	if err != nil {
		// embed.FS read can only fail on programmer error (the file
		// is compiled into the binary). Return 500 so a regression
		// shows up in observability instead of leaking a confusing
		// blank page.
		http.Error(w, "admin ui asset missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// CSP: deny everything except inline styles + inline scripts the
	// page uses (the page does not load any external resources). The
	// page only calls same-origin /v1/auth/* endpoints.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; "+
			"style-src 'unsafe-inline'; "+
			"script-src 'unsafe-inline'; "+
			"connect-src 'self'; "+
			"base-uri 'none'; "+
			"form-action 'none'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
