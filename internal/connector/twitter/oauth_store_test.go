package twitter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// TestTwitterOAuth_EncryptedStoreRoundTrip proves the AES-256-GCM at-rest
// crypto: the persisted ciphertext is NOT the plaintext, two encryptions of the
// same value differ (random nonce), and decrypt restores the exact original
// access+refresh pair (SCN-BUG-056-002-003). The crypto layer is exercised
// directly so the test is CI-runnable without a database; the SaveTokens →
// GetTokens DB round-trip is covered by the integration pass.
func TestTwitterOAuth_EncryptedStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := newOAuthStore(nil, "a-secure-test-at-rest-key")
	if err != nil {
		t.Fatalf("newOAuthStore with a non-empty key must succeed, got: %v", err)
	}

	tok := &auth.Token{
		AccessToken:  "user-context-access-token-SUPER-SECRET",
		RefreshToken: "user-context-refresh-token-ALSO-SECRET",
		TokenType:    "bearer",
		Scopes:       []string{"offline.access", "bookmark.read"},
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}

	encAccess, err := store.encrypt(tok.AccessToken)
	if err != nil {
		t.Fatalf("encrypt access: %v", err)
	}
	encRefresh, err := store.encrypt(tok.RefreshToken)
	if err != nil {
		t.Fatalf("encrypt refresh: %v", err)
	}

	// Ciphertext must not equal — or contain — the plaintext.
	if encAccess == tok.AccessToken || strings.Contains(encAccess, tok.AccessToken) {
		t.Fatalf("access ciphertext leaks plaintext: %q", encAccess)
	}
	if encRefresh == tok.RefreshToken || strings.Contains(encRefresh, tok.RefreshToken) {
		t.Fatalf("refresh ciphertext leaks plaintext: %q", encRefresh)
	}

	// Random nonce: encrypting the same plaintext twice yields different output.
	encAccess2, err := store.encrypt(tok.AccessToken)
	if err != nil {
		t.Fatalf("encrypt access (2nd): %v", err)
	}
	if encAccess == encAccess2 {
		t.Fatalf("two encryptions of the same plaintext collided (nonce not random): %q", encAccess)
	}

	// Decrypt restores the exact originals.
	gotAccess, err := store.decrypt(encAccess)
	if err != nil {
		t.Fatalf("decrypt access: %v", err)
	}
	gotRefresh, err := store.decrypt(encRefresh)
	if err != nil {
		t.Fatalf("decrypt refresh: %v", err)
	}
	if gotAccess != tok.AccessToken {
		t.Fatalf("access round-trip mismatch: want %q got %q", tok.AccessToken, gotAccess)
	}
	if gotRefresh != tok.RefreshToken {
		t.Fatalf("refresh round-trip mismatch: want %q got %q", tok.RefreshToken, gotRefresh)
	}
}

// TestTwitterOAuth_EmptyKeyFailsLoud proves the deliberate divergence from
// auth.TokenStore: an empty at-rest key makes the constructor fail loud rather
// than silently storing the long-lived refresh token in plaintext
// (SCN-BUG-056-002-004, smackerel-no-defaults).
func TestTwitterOAuth_EmptyKeyFailsLoud(t *testing.T) {
	t.Parallel()

	store, err := newOAuthStore(nil, "")
	if err == nil {
		t.Fatalf("expected an error for an empty at-rest key, got nil (store=%v)", store)
	}
	if store != nil {
		t.Fatalf("expected a nil store when the key is empty, got %v", store)
	}
	if !errors.Is(err, ErrOAuthAtRestKeyRequired) {
		t.Fatalf("error must be the ErrOAuthAtRestKeyRequired sentinel, got %T: %v", err, err)
	}
	// The error must name the at-rest key env so an operator can fix it.
	if !strings.Contains(err.Error(), "SMACKEREL_AUTH_TOKEN") {
		t.Fatalf("fail-loud error must name SMACKEREL_AUTH_TOKEN, got: %v", err)
	}
}
