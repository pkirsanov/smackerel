// Spec 091 SCOPE-02 — GET /register page + static asset serving.
//
// Renders a CSP-compliant HTML form (no inline scripts, no inline event
// handlers) that posts to /v1/web/register. Mirrors the spec-057 /login
// GET page (web_login_page.go HandleLoginPage): embedded html/template,
// the shared loginUIFS embed.FS, the header trio (text/html, no-store,
// nosniff), and sanitizeNext on the ?next= query so a malicious
// `?next=//evil/` cannot escape origin.
//
// Reconciled AC-5 / AC-10 (non-enumeration): registerPageData carries NO
// invite-token field and the handler NEVER reads the gate configuration,
// so GET /register renders the IDENTICAL form whether or not the invite
// token is configured. There is no separate "registration unavailable"
// GET state — the disabled gate is enforced only at POST. This makes the
// gate state unobservable from GET.
//
// The route is registered OUTSIDE bearerAuthMiddleware (see router.go,
// wired in SCOPE-04) because /register is an entry point — by definition
// unauthenticated browser navigations land here.
package api

import (
	"html/template"
	"net/http"
)

// registerTemplate is parsed from the shared loginUIFS embed (extended in
// web_login_page.go to include register.html). Same package, same embed.FS.
var registerTemplate = template.Must(template.ParseFS(loginUIFS, "admin_ui_static/register.html"))

// registerPageData is the template input.
//
// NOTE: there is deliberately NO invite-token field here. The GET page
// must not read or reflect the gate configuration (Reconciled AC-5 /
// AC-10), so the rendered form is byte-identical regardless of whether
// registration is enabled. Username is echoed on a POST re-render (the
// user's own input, auto-escaped by html/template); password,
// confirm-password and invite-token are ALWAYS rendered empty
// (secret-preservation invariant — see web_register.go).
type registerPageData struct {
	Next     string // sanitized ?next, echoed into the hidden field
	Username string // preserved on POST re-render; blank on first GET
	Error    string // banner text on POST re-render; blank on GET
}

// HandleRegisterPage serves GET /register (and HEAD).
//
// Spec 091 AC-1 (CSP-compliant form with username/password/confirm-password/
// invite-token fields), AC-5 (identical form regardless of gate config).
func (d *Dependencies) HandleRegisterPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// The ?next= destination survives registration -> login. Untrusted;
	// sanitised here and again server-side at POST time (defence in depth).
	next := sanitizeNext(r.URL.Query().Get("next"))

	data := registerPageData{Next: next}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if r.Method == http.MethodHead {
		return
	}
	if err := registerTemplate.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
}
