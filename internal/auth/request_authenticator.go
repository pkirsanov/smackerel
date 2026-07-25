// Package auth — unified request authentication seam (BUG-070-001 SCOPE-03,
// AUTH-015). One RequestAuthenticator owns credential extraction, purpose
// (audience) selection, signature/time verification, and revocation exactly
// once, and returns a SINGLE auth.Session that the legacy web, /api, and /v1
// surfaces all consume identically. This removes the trust split that made a
// production password cookie authenticate legacy pages but be rejected by the
// PASETO-validating /api and /v1 middleware.
//
// This file is the reusable seam. It is intentionally standalone: the concrete
// PASETO verify path (internal/auth/verify.go) and the live router wiring
// (internal/api/router.go bearerAuthMiddleware / webAuthMiddleware) are wired
// to it in the SCOPE-03 live-integration rows, which are deferred in the
// unit-verifiable core. The audience/purpose split on the token itself is
// SCOPE-02 issuance work; here the audience is supplied by the injected
// TokenVerifier so the carrier-precedence and session-parity contract is fully
// unit-testable now.
package auth

import (
	"net/http"
	"strings"
)

// Audience is the closed set of PASETO/session purposes. A cookie is validated
// as the browser-session audience; an Authorization header as the API-bearer
// audience. The audiences are deliberately distinct so a token minted for one
// carrier cannot be replayed on the other (design §PASETO Claims / §Unified
// Request Authentication).
type Audience string

const (
	// AudienceBrowserSession is the aud claim of a browser cookie session.
	AudienceBrowserSession Audience = "smackerel-browser-session"
	// AudienceAPIBearer is the aud claim of a machine Authorization-header token.
	AudienceAPIBearer Audience = "smackerel-api-bearer"
)

// Carrier identifies which request channel presented the credential. Recorded
// on the resolved session for bounded telemetry; it does not change the session
// identity or claims.
type Carrier string

const (
	// CarrierHeader — credential presented in the Authorization header.
	CarrierHeader Carrier = "header"
	// CarrierCookie — credential presented in the browser session cookie.
	CarrierCookie Carrier = "cookie"
)

// AuthFailureKind is the closed-set outcome of Authenticate. The empty value
// AuthOK means the request authenticated successfully. Every failure maps to a
// 401-class response at the presentation layer; it is deliberately distinct
// from authorization (403), which is evaluated downstream once a valid session
// exists (design §Authorization Boundaries).
type AuthFailureKind string

const (
	// AuthOK — a valid session was resolved.
	AuthOK AuthFailureKind = ""
	// AuthMissing — no credential carrier was present.
	AuthMissing AuthFailureKind = "missing"
	// AuthMalformedHeader — an Authorization header was present but malformed.
	// Fail-closed: the cookie is NOT consulted as a fallback (design carrier
	// precedence — "if Authorization is present but malformed, do not silently
	// use the cookie").
	AuthMalformedHeader AuthFailureKind = "malformed_header"
	// AuthInvalid — the token failed signature/time/key verification.
	AuthInvalid AuthFailureKind = "invalid"
	// AuthRevoked — the token verified but is revoked.
	AuthRevoked AuthFailureKind = "revoked"
	// AuthWrongAudience — the token verified but carries the wrong audience for
	// its carrier (e.g. an API-bearer token presented directly in the browser
	// cookie). This is exactly the BUG-070-001 rejection contract for a
	// shared/API-shaped cookie.
	AuthWrongAudience AuthFailureKind = "wrong_audience"
)

// VerifiedToken is what a TokenVerifier returns for a raw token string. The
// verifier is responsible for signature/time/key checks and for reporting the
// token's actual audience and revocation state; the RequestAuthenticator then
// enforces carrier/audience matching and revocation fail-closed.
type VerifiedToken struct {
	// Session is the resolved principal/claims for the token.
	Session Session
	// Audience is the token's actual aud claim.
	Audience Audience
	// Revoked reports whether the token id is in the revocation set.
	Revoked bool
}

// TokenVerifier is the injectable seam over the concrete PASETO verify path.
// Verify receives the raw token and the audience the carrier REQUIRES; it
// returns the VerifiedToken (including the token's actual audience) or a
// non-nil error when signature/time/key verification fails. Production wiring
// supplies an adapter over internal/auth/verify.go + the revocation cache;
// unit tests supply a deterministic fake.
type TokenVerifier interface {
	Verify(raw string, required Audience) (VerifiedToken, error)
}

// browserCookieName is the cookie carrying the browser session. Kept as
// "auth_token" for route/test compatibility (design §Cookie).
const browserCookieName = "auth_token"

// RequestAuthenticator resolves one auth.Session for every surface. It performs
// extraction, carrier-precedence selection, audience enforcement, and
// revocation exactly once. Principal-active status is an issuance-time policy;
// account disablement revokes outstanding tokens rather than adding a database
// query to every authenticated request (design §Unified Request Authentication).
type RequestAuthenticator struct {
	verifier   TokenVerifier
	cookieName string
}

// NewRequestAuthenticator constructs the authenticator. It panics on a nil
// verifier because a nil verifier is a wiring bug that would silently accept or
// reject every request.
func NewRequestAuthenticator(v TokenVerifier) *RequestAuthenticator {
	if v == nil {
		panic("auth: NewRequestAuthenticator requires a non-nil TokenVerifier")
	}
	return &RequestAuthenticator{verifier: v, cookieName: browserCookieName}
}

// Authenticate resolves the request's session. Carrier precedence is
// fail-closed: a present-but-malformed Authorization header rejects the request
// (the cookie is never used as a fallback); a valid header is verified as the
// API-bearer audience; otherwise the browser cookie is verified as the
// browser-session audience. Returns AuthMissing when no carrier is present.
func (a *RequestAuthenticator) Authenticate(r *http.Request) (Session, AuthFailureKind) {
	if header := r.Header.Get("Authorization"); header != "" {
		raw, ok := parseBearer(header)
		if !ok {
			// Fail-closed: do not silently fall back to the cookie.
			return Session{}, AuthMalformedHeader
		}
		return a.resolve(raw, AudienceAPIBearer)
	}
	if cookie, err := r.Cookie(a.cookieName); err == nil && cookie.Value != "" {
		return a.resolve(cookie.Value, AudienceBrowserSession)
	}
	return Session{}, AuthMissing
}

// resolve verifies a raw token for the required audience and enforces
// audience-matching and revocation fail-closed.
func (a *RequestAuthenticator) resolve(raw string, required Audience) (Session, AuthFailureKind) {
	vt, err := a.verifier.Verify(raw, required)
	if err != nil {
		return Session{}, AuthInvalid
	}
	if vt.Audience != required {
		// A token minted for a different carrier's audience is rejected — the
		// core BUG-070-001 fix: an API/shared-shaped value in the browser
		// cookie is wrong-purpose and never authenticates.
		return Session{}, AuthWrongAudience
	}
	if vt.Revoked {
		return Session{}, AuthRevoked
	}
	return vt.Session, AuthOK
}

// parseBearer extracts the token from a "Bearer <token>" Authorization header.
// Returns ok=false for any other shape (Basic, empty token, missing scheme) so
// the caller can fail closed.
func parseBearer(header string) (string, bool) {
	const scheme = "bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	token := strings.TrimSpace(header[len(scheme):])
	if token == "" {
		return "", false
	}
	return token, true
}

// failurePresenter renders an authentication failure for a surface. The three
// product surfaces differ ONLY in presentation (design §Unified Request
// Authentication) — legacy top-level HTML navigation gets a 303 to /login,
// while /api and /v1 (and HTMX/XHR on legacy) get a 401. Identity resolution is
// identical across all three.
type failurePresenter func(w http.ResponseWriter, r *http.Request, kind AuthFailureKind)

// guard returns a chi-compatible middleware that authenticates via the shared
// RequestAuthenticator, attaches the resolved session, and delegates failure
// rendering to the supplied presenter.
func (a *RequestAuthenticator) guard(present failurePresenter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, kind := a.Authenticate(r)
			if kind != AuthOK {
				present(w, r, kind)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
		})
	}
}

// APIAuthMiddleware is the /api surface authenticator: it delegates to the
// shared RequestAuthenticator and renders failures as a generic 401 with no
// token material in the body.
func (a *RequestAuthenticator) APIAuthMiddleware(next http.Handler) http.Handler {
	return a.guard(apiPresenter)(next)
}

// V1AuthMiddleware is the /v1 surface authenticator. It uses the identical
// shared RequestAuthenticator and the identical 401 presentation as /api,
// proving one session is accepted by both modern surfaces.
func (a *RequestAuthenticator) V1AuthMiddleware(next http.Handler) http.Handler {
	return a.guard(apiPresenter)(next)
}

// LegacyWebAuthMiddleware is the legacy server-rendered surface authenticator.
// It uses the identical shared RequestAuthenticator; only the failure
// presentation differs — a top-level browser navigation is redirected to
// /login (303) while HTMX/XHR/API-like requests receive a 401.
func (a *RequestAuthenticator) LegacyWebAuthMiddleware(next http.Handler) http.Handler {
	return a.guard(legacyPresenter)(next)
}

// apiPresenter writes the generic 401 used by /api, /v1, and legacy non-browser
// requests. NFR-AUTH-007: the body never names which validation step failed.
func apiPresenter(w http.ResponseWriter, _ *http.Request, _ AuthFailureKind) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"UNAUTHORIZED"}`))
}

// legacyPresenter redirects a top-level browser navigation to /login (303) and
// otherwise falls back to the generic 401 (identical content-negotiation to the
// live web middleware).
func legacyPresenter(w http.ResponseWriter, r *http.Request, kind AuthFailureKind) {
	if isTopLevelBrowserNav(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	apiPresenter(w, r, kind)
}

// isTopLevelBrowserNav reports whether the request is a top-level HTML page
// navigation (GET/HEAD, not an HTMX/XHR fetch, Accept prefers text/html). This
// mirrors the live web middleware's browser-navigation detection so the seam's
// presentation matches the production contract; the live wiring uses the
// internal/api implementation.
func isTopLevelBrowserNav(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if r.Header.Get("HX-Request") != "" {
		return false
	}
	if strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}
