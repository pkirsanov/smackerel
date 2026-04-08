package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenStore persists OAuth2 tokens in PostgreSQL.
type TokenStore struct {
	pool *pgxpool.Pool
}

// NewTokenStore creates a token store backed by PostgreSQL.
func NewTokenStore(pool *pgxpool.Pool) *TokenStore {
	return &TokenStore{pool: pool}
}

// Save stores or updates a token for the given provider.
func (s *TokenStore) Save(ctx context.Context, provider string, token *Token) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_tokens (provider, access_token, refresh_token, expires_at, token_type, scopes, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (provider) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = COALESCE(EXCLUDED.refresh_token, oauth_tokens.refresh_token),
			expires_at = EXCLUDED.expires_at,
			token_type = EXCLUDED.token_type,
			scopes = EXCLUDED.scopes,
			updated_at = NOW()
	`, provider, token.AccessToken, token.RefreshToken, token.ExpiresAt, token.TokenType, token.Scopes)
	if err != nil {
		return fmt.Errorf("save token for %s: %w", provider, err)
	}
	return nil
}

// Get retrieves the token for a provider. Returns nil if not found.
func (s *TokenStore) Get(ctx context.Context, provider string) (*Token, error) {
	var t Token
	err := s.pool.QueryRow(ctx, `
		SELECT access_token, refresh_token, expires_at, token_type, scopes
		FROM oauth_tokens WHERE provider = $1
	`, provider).Scan(&t.AccessToken, &t.RefreshToken, &t.ExpiresAt, &t.TokenType, &t.Scopes)
	if err != nil {
		return nil, fmt.Errorf("get token for %s: %w", provider, err)
	}
	return &t, nil
}

// GetValid returns a valid (non-expired) token, refreshing if needed.
func (s *TokenStore) GetValid(ctx context.Context, provider string, oauth OAuth2Provider) (*Token, error) {
	token, err := s.Get(ctx, provider)
	if err != nil {
		return nil, err
	}

	if !token.IsExpired() {
		return token, nil
	}

	// Token expired — try to refresh
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("token expired for %s and no refresh token available", provider)
	}

	newToken, err := oauth.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh token for %s: %w", provider, err)
	}

	// Preserve refresh token if the provider didn't issue a new one
	if newToken.RefreshToken == "" {
		newToken.RefreshToken = token.RefreshToken
	}

	if err := s.Save(ctx, provider, newToken); err != nil {
		return nil, fmt.Errorf("save refreshed token for %s: %w", provider, err)
	}

	return newToken, nil
}

// Delete removes a stored token for a provider.
func (s *TokenStore) Delete(ctx context.Context, provider string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM oauth_tokens WHERE provider = $1", provider)
	return err
}

// HasToken returns true if a valid token exists for the provider.
func (s *TokenStore) HasToken(ctx context.Context, provider string) bool {
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, "SELECT expires_at FROM oauth_tokens WHERE provider = $1", provider).Scan(&expiresAt)
	return err == nil
}
