// Spec 091 SCOPE-03 — POST /v1/web/register handler.
//
// HandleWebRegister is the invite-token-gated account-intake handler. It
// mirrors the form branch of HandleWebLogin (web_login.go) but performs
// intake only: on success it creates one web_user_credentials row (argon2id
// via the existing webcreds.Repo) and 303-redirects to /login?registered=1
// with NO Set-Cookie. The auth_token session cookie stays minted ONLY by
// POST /v1/web/login, keeping the "create account" and "establish session"
// trust boundaries clean (design.md → No cookie on register).
//
// SECURITY-CRITICAL CONTROL FLOW (design.md → POST Handler Control Flow).
// The invite-token gate is evaluated FIRST — before any username/password
// logic — so a request without a valid token can never produce a
// username-existence or field-specific signal (AC-10 / UC-2 non-enumeration).
// The wrong-token, missing-token, empty-configured, and nil-store responses
// are BYTE-IDENTICAL (same 401, same shared banner, same blank-secrets
// re-render). The invite-token value is never logged, echoed, or placed in
// the redirect/template (AC-10, value-safe).
//
// The route is registered OUTSIDE bearerAuthMiddleware and INSIDE the
// existing httprate.LimitByIP(20, 1*time.Minute) group (router.go, SCOPE-04)
// — it is an entry point by definition.
package api

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/smackerel/smackerel/internal/auth/webcreds"
)

// Exact user-facing banner strings — bound verbatim by spec.md / design.md
// (the ux Error-Message Catalog). Do NOT reword these: the regression and
// non-enumeration tests assert them byte-for-byte.
const (
	// registerGateBanner is the SHARED, non-enumerating banner returned for a
	// wrong token, a missing token, an empty-configured token, OR a nil store.
	// All four produce a byte-identical 401 so the response never reveals
	// whether the gate is configured (AC-5 / AC-10).
	registerGateBanner         = "Registration is not available or the invite is invalid."
	registerMissingFieldMsg    = "All fields are required."
	registerPasswordMismatch   = "Passwords do not match."
	registerPasswordTooShort   = "Password must be at least 12 characters."
	registerInvalidUsernameMsg = "Username must be 64 characters or fewer and contain no control characters."
	registerDuplicateUserMsg   = "That username is taken."
	registerServerErrorMsg     = "Something went wrong. Please try again."
)

// registerRedirectPath is the post-registration landing (no cookie set here;
// the operator authenticates through the unchanged /login flow). The success
// flash on /login is driven by the literal ?registered=1 query (SCOPE-04).
const registerRedirectPath = "/login?registered=1"

// HandleWebRegister implements POST /v1/web/register.
func (d *Dependencies) HandleWebRegister(w http.ResponseWriter, r *http.Request) {
	// Step 1 — method guard.
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}

	// Step 2 — content-type + parse. A non-form content-type or a parse
	// failure is a structural (non-enumeration) error → generic server error.
	if !isFormContentType(r) {
		d.logRegisterReject(r, "server")
		renderRegisterError(w, r, sanitizeNextDefault, "", registerServerErrorMsg, http.StatusInternalServerError)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	defer r.Body.Close()
	if err := r.ParseForm(); err != nil {
		d.logRegisterReject(r, "server")
		renderRegisterError(w, r, sanitizeNextDefault, "", registerServerErrorMsg, http.StatusInternalServerError)
		return
	}
	nextRaw := r.PostForm.Get("next")
	invite := r.PostForm.Get("invite-token")
	configured := d.WebRegistrationInviteToken

	// Step 3 — invite-token gate FIRST (constant-time, value-safe).
	//
	// Disabled / unavailable: a nil store OR an empty configured token means
	// registration is OFF. This is a plain comparison of a server-side
	// constant (no secret material), so it needs no constant-time treatment;
	// AC-4's constant-time requirement is about comparing the configured
	// SECRET value, done below. A naive single ConstantTimeCompare would be
	// WRONG here: when configured == "" and invite == "" it returns a match,
	// which would be OPEN SIGNUP. The explicit empty-configured guard prevents
	// that trap. On this path the preserved username is BLANK (the request
	// never advanced past the gate), keeping wrong-token and disabled
	// responses byte-identical.
	if d.WebCredentials == nil || configured == "" {
		d.logRegisterReject(r, "gate")
		renderRegisterError(w, r, sanitizeNext(nextRaw), "", registerGateBanner, http.StatusUnauthorized)
		return
	}
	if subtle.ConstantTimeCompare([]byte(invite), []byte(configured)) != 1 {
		d.logRegisterReject(r, "gate")
		renderRegisterError(w, r, sanitizeNext(nextRaw), "", registerGateBanner, http.StatusUnauthorized)
		return
	}

	// Steps 4-7 are reached ONLY after a valid token, so their distinct,
	// helpful messages are safe — seen exclusively by a holder of the
	// operator's trusted secret (already in the full-admin band).
	username := strings.TrimSpace(r.PostForm.Get("username"))
	password := r.PostForm.Get("password")
	confirm := r.PostForm.Get("confirm-password")

	// Step 4 — field presence.
	if username == "" || password == "" || confirm == "" {
		d.logRegisterReject(r, "field")
		renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerMissingFieldMsg, http.StatusBadRequest)
		return
	}

	// Step 5 — password rules.
	if password != confirm {
		d.logRegisterReject(r, "field")
		renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerPasswordMismatch, http.StatusBadRequest)
		return
	}
	if len(password) < webcreds.MinPasswordLength {
		d.logRegisterReject(r, "field")
		renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerPasswordTooShort, http.StatusBadRequest)
		return
	}

	// Step 6 — username validity.
	if err := webcreds.ValidateUsername(username); err != nil {
		d.logRegisterReject(r, "field")
		renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerInvalidUsernameMsg, http.StatusBadRequest)
		return
	}

	// Step 7 — create (create=true ⇒ ErrUserExists guarantees NO overwrite).
	if err := d.WebCredentials.UpsertPassword(r.Context(), username, password, true); err != nil {
		if errors.Is(err, webcreds.ErrUserExists) {
			d.logRegisterReject(r, "duplicate")
			renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerDuplicateUserMsg, http.StatusConflict)
			return
		}
		d.logRegisterReject(r, "server")
		renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerServerErrorMsg, http.StatusInternalServerError)
		return
	}

	// Success — 303 to /login?registered=1 (carry the sanitised next when a
	// non-default destination was supplied). NO Set-Cookie: registration is
	// pure intake; the session cookie is minted only by /v1/web/login.
	dest := registerRedirectPath
	if sn := sanitizeNext(nextRaw); sn != sanitizeNextDefault {
		dest += "&next=" + url.QueryEscape(sn)
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// renderRegisterError re-renders the /register page with a banner + the given
// status. The username is echoed (the user's own input, auto-escaped by
// html/template); password, confirm-password and invite-token are ALWAYS
// rendered empty (secret-preservation invariant). On the shared-gate reject
// the caller passes an empty username so the response is byte-identical
// across wrong-token / missing-token / empty-configured / nil-store.
func renderRegisterError(w http.ResponseWriter, _ *http.Request, next, username, msg string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	data := registerPageData{
		Next:     next,
		Username: username,
		Error:    msg,
	}
	_ = registerTemplate.Execute(w, data)
}

// logRegisterReject emits one value-safe structured log line on a failed
// registration (mirrors the spec-070 web_login_credential_fail pattern). It
// records ONLY: the remote address, the username LENGTH (never the value),
// and a coarse reason enum (gate | field | duplicate | server). The invite
// token value, the username value, and any password value are NEVER logged.
// The shared "gate" reason covers wrong/missing/disabled so the log itself is
// non-enumerating about gate state.
func (d *Dependencies) logRegisterReject(r *http.Request, reason string) {
	slog.Info("web registration rejected",
		"kind", "web_register_fail",
		"remote_addr", r.RemoteAddr,
		"username_len", len(strings.TrimSpace(r.PostForm.Get("username"))),
		"reason", reason,
	)
}
