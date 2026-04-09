package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenStore persists OAuth2 tokens in PostgreSQL with AES-256-GCM encryption.
type TokenStore struct {
	pool   *pgxpool.Pool
	encKey []byte // 32 bytes derived from auth token for AES-256
}

// NewTokenStore creates a token store backed by PostgreSQL.
// The encryptionKey is used to encrypt/decrypt tokens at rest via AES-256-GCM.
// If empty, tokens are stored in plaintext (development only).
func NewTokenStore(pool *pgxpool.Pool, encryptionKey string) *TokenStore {
	var key []byte
	if encryptionKey != "" {
		// Derive a 32-byte key from the auth token using SHA-256
		h := sha256.Sum256([]byte(encryptionKey))
		key = h[:]
	}
	return &TokenStore{pool: pool, encKey: key}
}

// encrypt encrypts plaintext using AES-256-GCM. Returns base64-encoded ciphertext.
func (s *TokenStore) encrypt(plaintext string) (string, error) {
	if len(s.encKey) == 0 || plaintext == "" {
		return plaintext, nil // No encryption key configured
	}

	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts base64-encoded AES-256-GCM ciphertext.
func (s *TokenStore) decrypt(encoded string) (string, error) {
	if len(s.encKey) == 0 || encoded == "" {
		return encoded, nil // No encryption key or empty value
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Not base64 — might be a plaintext token from before encryption was added
		slog.Warn("token not base64-encoded, treating as plaintext")
		return encoded, nil
	}

	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		// Too short for encrypted data — probably a plaintext token from before encryption
		slog.Warn("token too short for encrypted data, treating as plaintext")
		return encoded, nil
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Decryption failed — might be plaintext from before encryption was added
		slog.Warn("token decryption failed, treating as plaintext")
		return encoded, nil
	}

	return string(plaintext), nil
}

// Save stores or updates a token for the given provider (encrypted at rest).
func (s *TokenStore) Save(ctx context.Context, provider string, token *Token) error {
	encAccess, err := s.encrypt(token.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}
	encRefresh, err := s.encrypt(token.RefreshToken)
	if err != nil {
		return fmt.Errorf("encrypt refresh token: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO oauth_tokens (provider, access_token, refresh_token, expires_at, token_type, scopes, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (provider) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = COALESCE(EXCLUDED.refresh_token, oauth_tokens.refresh_token),
			expires_at = EXCLUDED.expires_at,
			token_type = EXCLUDED.token_type,
			scopes = EXCLUDED.scopes,
			updated_at = NOW()
	`, provider, encAccess, encRefresh, token.ExpiresAt, token.TokenType, token.Scopes)
	if err != nil {
		return fmt.Errorf("save token for %s: %w", provider, err)
	}
	return nil
}

// Get retrieves the token for a provider (decrypted from DB).
func (s *TokenStore) Get(ctx context.Context, provider string) (*Token, error) {
	var encAccess, encRefresh string
	var t Token
	err := s.pool.QueryRow(ctx, `
		SELECT access_token, refresh_token, expires_at, token_type, scopes
		FROM oauth_tokens WHERE provider = $1
	`, provider).Scan(&encAccess, &encRefresh, &t.ExpiresAt, &t.TokenType, &t.Scopes)
	if err != nil {
		return nil, fmt.Errorf("get token for %s: %w", provider, err)
	}

	t.AccessToken, err = s.decrypt(encAccess)
	if err != nil {
		return nil, fmt.Errorf("decrypt access token for %s: %w", provider, err)
	}
	t.RefreshToken, err = s.decrypt(encRefresh)
	if err != nil {
		return nil, fmt.Errorf("decrypt refresh token for %s: %w", provider, err)
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

// HasToken returns true if a valid (non-expired) token exists for the provider.
func (s *TokenStore) HasToken(ctx context.Context, provider string) bool {
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		"SELECT expires_at FROM oauth_tokens WHERE provider = $1 AND expires_at > NOW()",
		provider,
	).Scan(&expiresAt)
	return err == nil
}
