// Package twitter — User-Context OAuth 2.0 token manager (BUG-056-002 Scope C
// Pass 2).
//
// This file owns the runtime lifecycle of the persisted user-context OAuth 2.0
// token: handing the API client a currently-valid access token (refreshing
// proactively inside the pre-expiry skew window) and force-refreshing after a
// 401 on a user-context-tier endpoint. Twitter/X is a confidential client that
// ROTATES the refresh token on every exchange, so a refresh persists BOTH the
// new access token AND the new refresh token. Token values are NEVER logged.
//
// The manager is the single authority over the encrypted token store +
// confidential-client refresh config; api.go consumes only the two function
// hooks it exposes (apiClient.userContextToken ← AccessToken,
// apiClient.refreshUserContext ← Refresh). Fail-loud is absolute
// (smackerel-no-defaults): a missing token, an empty token, a store error, or a
// failed refresh surface an error — NEVER a silent fallback to the App-Only
// bearer (the original BUG-056-002 defect).
package twitter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// refreshSkew is how far before a user-context access token's expiry the
// manager refreshes it proactively. A token that expires within this window is
// rotated BEFORE the in-flight request goes out so a call does not race the
// expiry boundary. 60s mirrors the BUG-056-002 design A.6 pre-expiry skew.
const refreshSkew = 60 * time.Second

// userContextTokenStore is the persistence surface the token manager depends
// on: read the current encrypted user-context pair and persist the rotated pair
// after a refresh. The production *oauthStore satisfies it against Postgres;
// tests use an in-memory fake. Kept deliberately narrow (no PKCE-state methods)
// so the manager's dependency is exactly what it uses.
type userContextTokenStore interface {
	GetTokens(ctx context.Context, owner string) (*auth.Token, error)
	SaveTokens(ctx context.Context, owner string, t *auth.Token) error
}

// userContextRefresher is the token-endpoint refresh surface: a confidential
// client that presents HTTP Basic auth and receives a rotating refresh token.
// *auth.GenericOAuth2 satisfies it via RefreshTokenBasic.
type userContextRefresher interface {
	RefreshTokenBasic(ctx context.Context, refreshToken string) (*auth.Token, error)
}

// Compile-time proof that the production collaborators satisfy the narrow
// surfaces the manager depends on.
var (
	_ userContextTokenStore = (*oauthStore)(nil)
	_ userContextRefresher  = (*auth.GenericOAuth2)(nil)
)

// userContextManager owns the encrypted user-context token store + the OAuth
// confidential-client refresh provider for a single owner. It is the single
// authority that (a) hands the API client a currently-valid access token
// (refreshing proactively within refreshSkew of expiry) and (b) force-refreshes
// after a 401. Token values are NEVER logged or embedded in returned errors.
type userContextManager struct {
	store     userContextTokenStore
	refresher userContextRefresher
	owner     string
	logger    *slog.Logger
	now       func() time.Time
}

// newUserContextManager builds the manager. A nil logger falls back to
// slog.Default(); the component tag scopes its records for ops filtering.
func newUserContextManager(store userContextTokenStore, refresher userContextRefresher, owner string, logger *slog.Logger) *userContextManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &userContextManager{
		store:     store,
		refresher: refresher,
		owner:     owner,
		logger:    logger.With(slog.String("component", "twitter.usercontext")),
		now:       time.Now,
	}
}

// AccessToken returns a currently-valid user-context access token for the
// owner. Fail-loud contract (BUG-056-002, smackerel-no-defaults):
//   - no persisted token row, an empty access token, or a store error ⇒
//     ErrUserContextTokenRequired (NEVER an App-Only fallback).
//   - a token within refreshSkew of a KNOWN expiry is refreshed BEFORE
//     returning (exchange refresh_token → persist the rotated pair → return the
//     new access token). A refresh failure surfaces a wrapped error — never the
//     stale token, never a silent fallback.
//
// A zero/unknown ExpiresAt is NOT proactively refreshed (we cannot prove it is
// near expiry); the reactive refresh-on-401 path in doWithRetry is the backstop
// for that case.
func (m *userContextManager) AccessToken(ctx context.Context) (string, error) {
	tok, err := m.store.GetTokens(ctx, m.owner)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUserContextTokenRequired, err)
	}
	if tok == nil || tok.AccessToken == "" {
		return "", ErrUserContextTokenRequired
	}
	if !tok.ExpiresAt.IsZero() && tok.ExpiresAt.Sub(m.now()) <= refreshSkew {
		rotated, refreshErr := m.refresh(ctx, tok)
		if refreshErr != nil {
			return "", refreshErr
		}
		return rotated.AccessToken, nil
	}
	return tok.AccessToken, nil
}

// Refresh force-refreshes the user-context token: it reads the stored refresh
// token (fail-loud if absent/empty), exchanges it at the token endpoint
// (grant_type=refresh_token, confidential-client HTTP Basic auth via
// RefreshTokenBasic), and persists the ROTATED pair. It is the backstop the
// API client invokes after a 401 on a user-context-tier endpoint. On any
// failure it returns a wrapped error and NEVER falls back to the stale token.
func (m *userContextManager) Refresh(ctx context.Context) error {
	tok, err := m.store.GetTokens(ctx, m.owner)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserContextTokenRequired, err)
	}
	_, err = m.refresh(ctx, tok)
	return err
}

// refresh is the shared core of Refresh and the AccessToken pre-expiry path. It
// exchanges the current refresh token and persists the rotated pair. Twitter
// rotates the refresh token on every exchange, so BOTH the new access and the
// new refresh token are persisted; a defensively-empty rotated refresh token
// preserves the previous one so refresh capability is never silently lost.
// Token values are NEVER logged.
func (m *userContextManager) refresh(ctx context.Context, current *auth.Token) (*auth.Token, error) {
	if current == nil || current.RefreshToken == "" {
		return nil, fmt.Errorf("%w: no refresh token persisted (re-authorize)", ErrUserContextTokenRequired)
	}
	rotated, err := m.refresher.RefreshTokenBasic(ctx, current.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("twitter user-context token refresh failed: %w", err)
	}
	if rotated == nil || rotated.AccessToken == "" {
		return nil, fmt.Errorf("twitter user-context token refresh returned an empty access token")
	}
	if rotated.RefreshToken == "" {
		rotated.RefreshToken = current.RefreshToken
	}
	if err := m.store.SaveTokens(ctx, m.owner, rotated); err != nil {
		return nil, fmt.Errorf("persist rotated user-context token: %w", err)
	}
	m.logger.Info("user-context token refreshed")
	return rotated, nil
}
