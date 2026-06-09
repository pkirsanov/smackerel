package twitter

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/auth"
)

// twitterConnectorID is the fixed connector_id discriminator persisted in the
// twitter_oauth_* tables. The composite (owner_user_id, connector_id) shape
// leaves room for additional connectors/accounts later without DDL churn.
const twitterConnectorID = "twitter"

// ErrOAuthAtRestKeyRequired is returned by newOAuthStore when the at-rest
// encryption key is empty. Unlike auth.TokenStore (which falls back to
// plaintext storage in development when the key is empty), the Twitter
// user-context refresh token is a long-lived credential, so smackerel-no-defaults
// forbids the silent plaintext path: the store fails loud instead.
var ErrOAuthAtRestKeyRequired = errors.New(
	"twitter oauth store: at-rest encryption key (SMACKEREL_AUTH_TOKEN) is empty; " +
		"refusing to store user-context tokens in plaintext")

// pkceState is the server-side binding persisted by authorize-begin into
// twitter_oauth_states and consumed (TTL-checked + deleted) by
// authorize-finalize. CodeVerifier is the single-use RFC 7636 PKCE verifier.
type pkceState struct {
	StateToken   string
	OwnerUserID  string
	ConnectorID  string
	CodeVerifier string
	Scopes       []string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// oauthStore persists Twitter user-context OAuth tokens (AES-256-GCM encrypted
// at rest) and the short-lived PKCE flow state. It reuses the exact
// AES-256-GCM technique from internal/auth/store.go (key = SHA-256 of the
// at-rest key, nonce-prepended, base64) but diverges deliberately on the
// empty-key path: it fails loud rather than storing plaintext.
type oauthStore struct {
	pool *pgxpool.Pool
	gcm  cipher.AEAD
}

// newOAuthStore constructs the encrypted store. The atRestKey is the resolved
// SMACKEREL_AUTH_TOKEN; an empty key returns ErrOAuthAtRestKeyRequired (no
// plaintext fallback). pool may be nil only for crypto-level unit tests that
// never touch the database.
func newOAuthStore(pool *pgxpool.Pool, atRestKey string) (*oauthStore, error) {
	if atRestKey == "" {
		return nil, ErrOAuthAtRestKeyRequired
	}
	h := sha256.Sum256([]byte(atRestKey))
	block, err := aes.NewCipher(h[:])
	if err != nil {
		return nil, fmt.Errorf("twitter oauth store: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("twitter oauth store: create gcm: %w", err)
	}
	return &oauthStore{pool: pool, gcm: gcm}, nil
}

// encrypt encrypts plaintext using AES-256-GCM and returns base64 ciphertext
// (nonce prepended). The empty string maps to the empty string so an absent
// token column round-trips cleanly.
func (s *oauthStore) encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt reverses encrypt.
func (s *oauthStore) decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("token is not valid base64: %w", err)
	}
	nonceSize := s.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("encrypted token too short (got %d bytes, need at least %d)", len(data), nonceSize)
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("token decryption failed: %w", err)
	}
	return string(plaintext), nil
}

// SaveTokens upserts the encrypted user-context access+refresh pair for an
// owner. Twitter rotates refresh tokens, so the full pair is re-written on
// every call.
func (s *oauthStore) SaveTokens(ctx context.Context, owner string, t *auth.Token) error {
	encAccess, err := s.encrypt(t.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}
	encRefresh, err := s.encrypt(t.RefreshToken)
	if err != nil {
		return fmt.Errorf("encrypt refresh token: %w", err)
	}
	scopesJSON, err := json.Marshal(scopesOrEmpty(t.Scopes))
	if err != nil {
		return fmt.Errorf("marshal scopes: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO twitter_oauth_tokens
			(owner_user_id, connector_id, access_token, refresh_token, token_type, scopes, expires_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (owner_user_id, connector_id) DO UPDATE SET
			access_token  = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			token_type    = EXCLUDED.token_type,
			scopes        = EXCLUDED.scopes,
			expires_at    = EXCLUDED.expires_at,
			updated_at    = now()
	`, owner, twitterConnectorID, encAccess, encRefresh, t.TokenType, scopesJSON, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("save twitter user-context tokens: %w", err)
	}
	return nil
}

// GetTokens loads and decrypts the user-context token pair for an owner.
func (s *oauthStore) GetTokens(ctx context.Context, owner string) (*auth.Token, error) {
	var encAccess, encRefresh string
	var scopesJSON []byte
	var t auth.Token
	err := s.pool.QueryRow(ctx, `
		SELECT access_token, refresh_token, token_type, scopes, expires_at
		FROM twitter_oauth_tokens
		WHERE owner_user_id = $1 AND connector_id = $2
	`, owner, twitterConnectorID).Scan(&encAccess, &encRefresh, &t.TokenType, &scopesJSON, &t.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("get twitter user-context tokens: %w", err)
	}
	if t.AccessToken, err = s.decrypt(encAccess); err != nil {
		return nil, fmt.Errorf("decrypt access token: %w", err)
	}
	if t.RefreshToken, err = s.decrypt(encRefresh); err != nil {
		return nil, fmt.Errorf("decrypt refresh token: %w", err)
	}
	if len(scopesJSON) > 0 {
		if err := json.Unmarshal(scopesJSON, &t.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}
	}
	return &t, nil
}

// HasValidUserContext reports whether a persisted user-context token row exists
// for the owner.
func (s *oauthStore) HasValidUserContext(ctx context.Context, owner string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM twitter_oauth_tokens
			WHERE owner_user_id = $1 AND connector_id = $2
		)
	`, owner, twitterConnectorID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check twitter user-context token: %w", err)
	}
	return exists, nil
}

// SaveState persists a PKCE flow binding (authorize-begin).
func (s *oauthStore) SaveState(ctx context.Context, st pkceState) error {
	scopesJSON, err := json.Marshal(scopesOrEmpty(st.Scopes))
	if err != nil {
		return fmt.Errorf("marshal state scopes: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO twitter_oauth_states
			(state_token, owner_user_id, connector_id, code_verifier, scope, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, now(), $6)
	`, st.StateToken, st.OwnerUserID, twitterConnectorID, st.CodeVerifier, scopesJSON, st.ExpiresAt)
	if err != nil {
		return fmt.Errorf("save twitter oauth state: %w", err)
	}
	return nil
}

// ConsumeState atomically deletes and returns the state row for stateToken
// (delete-on-consume), then enforces the TTL. A missing row or an expired row
// both surface an error; the row is removed in every case so a stale binding
// cannot be replayed.
func (s *oauthStore) ConsumeState(ctx context.Context, stateToken string) (pkceState, error) {
	var st pkceState
	var scopesJSON []byte
	err := s.pool.QueryRow(ctx, `
		DELETE FROM twitter_oauth_states
		WHERE state_token = $1
		RETURNING state_token, owner_user_id, connector_id, code_verifier, scope, created_at, expires_at
	`, stateToken).Scan(
		&st.StateToken, &st.OwnerUserID, &st.ConnectorID, &st.CodeVerifier,
		&scopesJSON, &st.CreatedAt, &st.ExpiresAt,
	)
	if err != nil {
		return pkceState{}, fmt.Errorf("consume twitter oauth state: %w", err)
	}
	if len(scopesJSON) > 0 {
		if err := json.Unmarshal(scopesJSON, &st.Scopes); err != nil {
			return pkceState{}, fmt.Errorf("unmarshal state scopes: %w", err)
		}
	}
	if time.Now().After(st.ExpiresAt) {
		return pkceState{}, fmt.Errorf("twitter oauth state %q expired at %s", stateToken, st.ExpiresAt.Format(time.RFC3339))
	}
	return st, nil
}

// scopesOrEmpty normalizes a nil scope slice to a non-nil empty slice so the
// JSONB column stores '[]' rather than 'null'.
func scopesOrEmpty(scopes []string) []string {
	if scopes == nil {
		return []string{}
	}
	return scopes
}
