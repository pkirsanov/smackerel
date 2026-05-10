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

	// Reject requests when no auth token is configured at all (pure
	// dev-bypass mode). The login flow has nothing to validate against
	// in that case — the bearer middleware lets every request through.
	if d.AuthToken == "" && !d.AuthConfig.Enabled {
		writeError(w, http.StatusBadRequest, "unsupported_no_auth_token",
			"web login is not available when no auth token is configured (dev-bypass mode)")
		return
	}

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

	var (
		userID    string
		expiresAt string
	)

	if d.AuthConfig.Enabled {
		// Production / per-user PASETO path.
		parsed, err := auth.VerifyAndParse(req.Token, d.AuthVerifyOptions)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_token",
				"token failed validation")
			return
		}
		if d.RevocationCache != nil && d.RevocationCache.IsRevoked(parsed.TokenID) {
			writeError(w, http.StatusUnauthorized, "revoked_token",
				"token has been revoked")
			return
		}
		userID = parsed.UserID
		expiresAt = parsed.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
	} else {
		// Dev/test shared-token path (constant-time compare).
		if subtle.ConstantTimeCompare([]byte(req.Token), []byte(d.AuthToken)) != 1 {
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
		Value:    req.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.EqualFold(d.Environment, "production"),
	}
	http.SetCookie(w, cookie)

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
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "logged_out"})
}
