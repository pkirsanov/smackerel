// Spec 057 Scope 1 — GET /login page + static asset serving.
//
// Renders a CSP-compliant HTML form (no inline scripts, no inline
// event handlers) that posts to /v1/web/login. The hidden `next`
// field is sanitised via sanitizeNext before it is embedded so a
// malicious `?next=//evil/` query parameter cannot escape origin.
//
// Routes are registered OUTSIDE bearerAuthMiddleware (see router.go)
// because /login is the entry point — by definition unauthenticated
// browser navigations land here.
package api

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed admin_ui_static/login.html admin_ui_static/login.js admin_ui_static/login.css admin_ui_static/register.html admin_ui_static/register.js
var loginUIFS embed.FS

var loginTemplate = template.Must(template.ParseFS(loginUIFS, "admin_ui_static/login.html"))

// loginPageData is the template input.
type loginPageData struct {
	AuthEnabled bool
	Next        string
	Error       string
	// Registered renders the spec-091 post-registration success flash
	// ("Account created — sign in.") when GET /login carries ?registered=1.
	// Additive: false (the zero value, and the only value when the query is
	// absent) preserves the spec-057/070 /login render byte-for-byte (AC-9).
	Registered bool
}

// HandleLoginPage serves GET /login.
//
// Spec 057 Scenario 3 (form renders), Scenario 9 (ignores ?token=),
// Scenario 12 (renders disabled banner when AuthConfig.Enabled=false).
func (d *Dependencies) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// SCN-9: never honour `?token=` from the query string. We simply
	// do not read it — the form is the only intake path.
	next := sanitizeNext(r.URL.Query().Get("next"))

	data := loginPageData{
		AuthEnabled: d.loginAuthEnabled(),
		Next:        next,
		// Spec 091 — post-registration success flash on the literal ?registered=1.
		Registered: r.URL.Query().Get("registered") == "1",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if r.Method == http.MethodHead {
		return
	}
	if err := loginTemplate.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
}
