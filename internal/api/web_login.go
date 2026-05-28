// Spec 044 Scope 03 — PWA per-user session foundation.
//
// POST /v1/web/login is the entry-point handler that converts a per-user
// PASETO bearer token (or, in dev/test, the shared SMACKEREL_AUTH_TOKEN)
// into an `auth_token` HTTP-only cookie suitable for the PWA. After
// login, subsequent same-origin requests carry the cookie automatically;
// bearerAuthMiddleware accepts the cookie as a fallback for the
// Authorization header so the PWA does not have to attach `Authorization:
// Bearer ...` to every fetch.
//
// Discharges design.md §10.4 (cookie session model: HttpOnly + Secure +
// SameSite=Lax + Path=/) and unblocks the PWA discharge for spec 044
// FINALIZE-PREREQ-044-V7-001.
//
// Design intent:
//   - Production (AuthConfig.Enabled = true): client POSTs {token: "<paseto>"}
//     and the handler runs auth.VerifyAndParse + revocation.Cache lookup,
//     then sets the cookie to the wire token. Subsequent requests carry
//     the cookie.
//   - Dev/test (AuthConfig.Enabled = false): client POSTs {token: "<shared>"}
//     and the handler does a constant-time compare against
//     d.AuthToken, then sets the cookie. This preserves the existing
//     dev workflow (one shared token).
//   - Empty AuthToken in dev (no auth at all): the handler refuses
//     (400 unsupported_no_auth_token) — dev-mode bypass already lets all
//     traffic through, so a login flow is meaningless.
//
// SCN-AUTH-002 [PWA path] evidence: tests/e2e/auth/pwa_per_user_test.go
// exercises the full cookie roundtrip against a production-mode router
// + real PostgreSQL + real PASETO mint.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/smackerel/smackerel/internal/auth"
)

// webLoginRequest is the POST body shape accepted by /v1/web/login.
//
// In production the `token` field MUST be a PASETO v4.public wire token
// produced by the auth.IssueToken bootstrap or rotation path.
// In dev/test the `token` field MUST equal d.AuthToken (the shared token).
//
// The body is intentionally minimal: user_id is NEVER trusted from the
// request body — in production it comes from PASETO claims; in dev/test
// it is irrelevant because there is only one synthetic user.
type webLoginRequest struct {
	Token string `json:"token"`
}

// webLoginResponse is the JSON body returned on successful login. The
// cookie is the load-bearing artifact; the JSON body is informational.
type webLoginResponse struct {
	UserID    string `json:"user_id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// HandleWebLogin implements POST /v1/web/login.
//
// The route is registered OUTSIDE bearerAuthMiddleware in router.go
// because the login handler is the entry point — clients hit it
// without an existing session.
func (d *Dependencies) HandleWebLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}

	// Spec 057 Scope 3 — accept application/x-www-form-urlencoded in
	// addition to JSON. The JSON wire contract for existing spec 044
	// callers remains byte-for-byte identical. Branching is driven by
	// Content-Type; form-encoded requests get a 303 + cookie response,
	// JSON requests get the existing JSON body response.
	isForm := strings.HasPrefix(
		strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0])),
		"application/x-www-form-urlencoded",
	)

	// Reject requests when no auth token is configured at all (pure
	// dev-bypass mode). The login flow has nothing to validate against
	// in that case — the bearer middleware lets every request through.
	if d.AuthToken == "" && !d.AuthConfig.Enabled {
		if isForm {
			d.renderLoginError(w, r, "/", "Login is not available on this deployment.")
			return
		}
		writeError(w, http.StatusBadRequest, "unsupported_no_auth_token",
			"web login is not available when no auth token is configured (dev-bypass mode)")
		return
	}

	var (
		token   string
		nextRaw string
	)

	if isForm {
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()
		if err := r.ParseForm(); err != nil {
			d.renderLoginError(w, r, "/", "Could not read login form.")
			return
		}
		token = strings.TrimSpace(r.PostForm.Get("token"))
		nextRaw = r.PostForm.Get("next")
		if token == "" {
			d.renderLoginError(w, r, sanitizeNext(nextRaw), "Token is required.")
			return
		}
	} else {
		// Limit body size — the request payload is a single short token.
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		defer r.Body.Close()

		var req webLoginRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json",
				"request body must be a JSON object with a single \"token\" field")
			return
		}
		req.Token = strings.TrimSpace(req.Token)
		if req.Token == "" {
			writeError(w, http.StatusBadRequest, "missing_token",
				"\"token\" field is required")
			return
		}
		token = req.Token
	}

	var (
		userID    string
		expiresAt string
	)

	if d.AuthConfig.Enabled {
		// Production / per-user PASETO path.
		parsed, err := auth.VerifyAndParse(token, d.AuthVerifyOptions)
		if err != nil {
			if isForm {
				d.renderLoginError(w, r, sanitizeNext(nextRaw), "Invalid or expired token.")
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid_token",
				"token failed validation")
			return
		}
		if d.RevocationCache != nil && d.RevocationCache.IsRevoked(parsed.TokenID) {
			if isForm {
				d.renderLoginError(w, r, sanitizeNext(nextRaw), "Invalid or expired token.")
				return
			}
			writeError(w, http.StatusUnauthorized, "revoked_token",
				"token has been revoked")
			return
		}
		userID = parsed.UserID
		expiresAt = parsed.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
	} else {
		// Dev/test shared-token path (constant-time compare).
		if subtle.ConstantTimeCompare([]byte(token), []byte(d.AuthToken)) != 1 {
			if isForm {
				d.renderLoginError(w, r, sanitizeNext(nextRaw), "Invalid or expired token.")
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid_token",
				"token does not match shared dev token")
			return
		}
	}

	// Set the auth_token cookie. design.md §10.4 cookie attributes:
	// HttpOnly (no JS access), SameSite=Lax (cross-site form posts
	// blocked, top-level GETs allowed), Path=/ (covers all API + UI).
	// Secure is set in production deployments (TLS) and dropped in
	// dev/test where the test stack runs over plain HTTP on
	// 127.0.0.1.
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.EqualFold(d.Environment, "production"),
	}
	http.SetCookie(w, cookie)

	if isForm {
		// Spec 057 Scope 3 — server-side re-sanitisation of the hidden
		// `next` field. Client-supplied value is untrusted; defence in
		// depth on top of the GET-time sanitisation.
		dest := sanitizeNext(nextRaw)
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	writeJSON(w, http.StatusOK, webLoginResponse{
		UserID:    userID,
		ExpiresAt: expiresAt,
	})
}

// HandleWebLogout implements POST /v1/web/logout. It clears the
// auth_token cookie so the PWA can drop its session without keeping
// the token around. Logout is best-effort and idempotent — it always
// returns 200 to avoid leaking information about session presence.
func (d *Dependencies) HandleWebLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.EqualFold(d.Environment, "production"),
		MaxAge:   -1,
	})
	// Spec 057 Scope 3 — form POSTs from the logout button redirect to
	// /login so the user lands on a usable page. JSON callers (spec 044
	// PWA contract) keep the existing JSON body response.
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]))
	if ct == "application/x-www-form-urlencoded" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "logged_out"})
}

// renderLoginError re-renders the /login page with a non-revealing
// error banner and the supplied (already-sanitised) next value. Used
// by the form POST failure paths so the user can correct their token
// without losing the requested destination. The error message must
// NEVER include the offending token value (privacy / log-injection).
func (d *Dependencies) renderLoginError(w http.ResponseWriter, _ *http.Request, next, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusUnauthorized)
	data := loginPageData{
		AuthEnabled: d.AuthConfig.Enabled || d.AuthToken != "",
		Next:        next,
		Error:       msg,
	}
	_ = loginTemplate.Execute(w, data)
}
