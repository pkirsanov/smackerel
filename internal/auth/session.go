package auth

import (
	"context"
	"errors"
	"time"
)

// SessionSource identifies how the runtime authenticated the calling
// principal. Spec 044 distinguishes per-user PASETO sessions from the
// legacy single-tenant SMACKEREL_AUTH_TOKEN fallback so that telemetry,
// audit logs, and admin-route gating can apply different policy to each.
type SessionSource string

const (
	// SessionSourcePerUserToken — request authenticated by a per-user
	// PASETO v4.public bearer token validated against the auth_tokens
	// table and the in-process revocation cache.
	SessionSourcePerUserToken SessionSource = "per_user_token"

	// SessionSourceSharedToken — request authenticated by the legacy
	// SMACKEREL_AUTH_TOKEN dev/test ergonomic. Production-mode requires
	// auth.production_shared_token_fallback_enabled to honor this source.
	SessionSourceSharedToken SessionSource = "shared_token"

	// SessionSourceBootstrap — request authenticated by the one-shot
	// auth.bootstrap_token used by `./smackerel.sh auth bootstrap` to
	// enroll the very first user on a fresh production deployment.
	SessionSourceBootstrap SessionSource = "bootstrap"
)

// Session represents the authenticated principal for a single request.
// The hot path reads it from the request context via SessionFromContext.
// Spec 044 design.md §6 — verify-and-parse populates the Session struct
// and pushes it onto the context before the request handler runs.
type Session struct {
	// UserID is the stable per-user identifier from auth_users.user_id.
	// For SessionSourceSharedToken this value is the literal "shared".
	UserID string

	// TokenID is the auth_tokens.token_id PRIMARY KEY value. Empty when
	// Source is SessionSourceSharedToken or SessionSourceBootstrap.
	TokenID string

	// KeyID is the kid claim from the PASETO footer; empty for non-PASETO
	// sessions. Used by telemetry to label which signing key produced
	// each request during a key rotation window.
	KeyID string

	// IssuedAt is the token's iat claim; ZeroTime() for shared/bootstrap.
	IssuedAt time.Time

	// ExpiresAt is the token's exp claim; ZeroTime() for shared/bootstrap.
	ExpiresAt time.Time

	// Source identifies which authentication path produced this session.
	Source SessionSource
}

// IsAdmin reports whether the session is allowed to call the auth admin
// HTTP surface (POST /v1/auth/users, POST /v1/auth/users/{id}/rotate,
// etc). Spec 044 design.md §6.4 — admin scope is implicit for the
// bootstrap session and granted to per-user sessions whose user_id
// appears in an SST allowlist evaluated by the handler. SessionSourceSharedToken
// is treated as admin in dev/test only; the handler MUST refuse the
// shared session as admin in production-mode (defense-in-depth on top
// of the production_shared_token_fallback_enabled gate).
func (s Session) IsAdmin() bool {
	switch s.Source {
	case SessionSourceBootstrap:
		return true
	case SessionSourceSharedToken:
		return true
	case SessionSourcePerUserToken:
		// Per-user admin gating happens at the handler boundary against
		// the SST allowlist; the Session struct alone does not carry
		// allowlist membership because the SST list belongs to the
		// AuthConfig surface, not the per-request session.
		return false
	default:
		return false
	}
}

// sessionContextKey is the unexported context key type for Session
// values. Using a typed key prevents collisions with other context
// values pushed by upstream middleware.
type sessionContextKey struct{}

// WithSession returns a derived context carrying the supplied Session.
// Called by the bearer middleware after a successful verify-and-parse.
// Returns the original context unchanged when sess is the zero value
// to avoid hiding programming errors behind silent no-ops.
func WithSession(ctx context.Context, sess Session) context.Context {
	if sess.Source == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionContextKey{}, sess)
}

// SessionFromContext extracts the Session pushed by WithSession. Returns
// the zero value and false when no session is present.
func SessionFromContext(ctx context.Context) (Session, bool) {
	sess, ok := ctx.Value(sessionContextKey{}).(Session)
	return sess, ok
}

// UserIDFromContext is the convenience accessor for handlers that only
// need the caller's user_id (the common case for MintReveal,
// drive.Connect, and the annotation pipeline). Returns the empty string
// when no session is present OR when the session has an empty UserID
// (e.g. SessionSourceSharedToken under the dev/test legacy ergonomic).
//
// Spec 044 design.md §14.3 — helper deferred from Scope 01 and landed
// in Scope 02 alongside the bearer middleware integration.
func UserIDFromContext(ctx context.Context) string {
	sess, ok := SessionFromContext(ctx)
	if !ok {
		return ""
	}
	return sess.UserID
}

// ErrNoSession is returned by helpers that REQUIRE an authenticated
// session but receive a context that lacks one. Handlers that depend on
// per-user identity should panic on this error in development and
// reject with 500 in production — the bearer middleware is supposed to
// have rejected the request long before it reached the handler.
var ErrNoSession = errors.New("auth: no session present in request context")
