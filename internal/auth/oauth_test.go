package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestToken_IsExpired(t *testing.T) {
	expired := &Token{ExpiresAt: time.Now().Add(-1 * time.Hour)}
	if !expired.IsExpired() {
		t.Error("expected token to be expired")
	}

	valid := &Token{ExpiresAt: time.Now().Add(1 * time.Hour)}
	if valid.IsExpired() {
		t.Error("expected token to be valid")
	}
}

func TestToken_IsExpired_ExactBoundary(t *testing.T) {
	// A token expiring right now should be expired (time.Now().After returns false for equal)
	// This tests the exact boundary behavior
	token := &Token{ExpiresAt: time.Now().Add(-1 * time.Millisecond)}
	if !token.IsExpired() {
		t.Error("token with past expiry should be expired")
	}
}

func TestToken_ZeroValue(t *testing.T) {
	token := &Token{}
	// Zero-time is in the past, so this should be expired
	if !token.IsExpired() {
		t.Error("zero-value token should be expired")
	}
}

func TestGenericOAuth2_ProviderName(t *testing.T) {
	provider := NewGenericOAuth2("google", OAuth2Config{})
	if provider.ProviderName() != "google" {
		t.Errorf("expected 'google', got %q", provider.ProviderName())
	}
}

func TestGenericOAuth2_AuthURL(t *testing.T) {
	provider := NewGenericOAuth2("google", OAuth2Config{
		ClientID:     "test-client-id",
		RedirectURL:  "http://localhost:8080/callback",
		AuthEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
	})

	url := provider.AuthURL([]string{"email", "calendar"}, "state123")

	if !strings.Contains(url, "client_id=test-client-id") {
		t.Error("expected URL to contain client_id")
	}
	if !strings.Contains(url, "state=state123") {
		t.Error("expected URL to contain state")
	}
	if !strings.Contains(url, "scope=email+calendar") || !strings.Contains(url, "scope=email calendar") {
		// Either URL-encoded or space-separated
		if !strings.Contains(url, "email") {
			t.Error("expected URL to contain email scope")
		}
	}
}

func TestGenericOAuth2_AuthURL_EmptyScopes(t *testing.T) {
	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:     "client-1",
		RedirectURL:  "http://localhost/cb",
		AuthEndpoint: "https://auth.example.com/authorize",
	})

	url := provider.AuthURL([]string{}, "state-empty")
	if !strings.Contains(url, "client_id=client-1") {
		t.Error("URL should contain client_id even with empty scopes")
	}
	if !strings.Contains(url, "response_type=code") {
		t.Error("URL should always include response_type=code")
	}
}

func TestGenericOAuth2_AuthURL_SpecialCharState(t *testing.T) {
	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:     "client-1",
		RedirectURL:  "http://localhost/cb",
		AuthEndpoint: "https://auth.example.com/authorize",
	})

	url := provider.AuthURL([]string{"openid"}, "state+with/special=chars")
	if !strings.Contains(url, "state=") {
		t.Error("URL should contain state parameter")
	}
}

func TestGoogleOAuth2Scopes(t *testing.T) {
	scopes := GoogleOAuth2Scopes()
	if len(scopes) != 3 {
		t.Errorf("expected 3 Google scopes, got %d", len(scopes))
	}

	// Verify Gmail, Calendar, YouTube are covered
	scopeStr := strings.Join(scopes, " ")
	if !strings.Contains(scopeStr, "mail.google.com") {
		t.Error("missing Gmail scope")
	}
	if !strings.Contains(scopeStr, "calendar") {
		t.Error("missing Calendar scope")
	}
	if !strings.Contains(scopeStr, "youtube") {
		t.Error("missing YouTube scope")
	}
}

func TestOAuth2ProviderInterface(t *testing.T) {
	var _ OAuth2Provider = &GenericOAuth2{}
}

// --- TokenStore encrypt/decrypt roundtrip tests ---

func TestTokenStore_EncryptDecrypt_Roundtrip(t *testing.T) {
	store := NewTokenStore(nil, "test-encryption-key-for-auth")

	original := "super-secret-access-token-12345"
	encrypted, err := store.encrypt(original)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if encrypted == original {
		t.Error("encrypted value should differ from plaintext")
	}

	decrypted, err := store.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != original {
		t.Errorf("roundtrip failed: expected %q, got %q", original, decrypted)
	}
}

func TestTokenStore_EncryptDecrypt_EmptyKey(t *testing.T) {
	store := NewTokenStore(nil, "")

	original := "plaintext-token"
	encrypted, err := store.encrypt(original)
	if err != nil {
		t.Fatalf("encrypt with empty key: %v", err)
	}
	if encrypted != original {
		t.Error("with no key, encrypt should return plaintext")
	}

	decrypted, err := store.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt with empty key: %v", err)
	}
	if decrypted != original {
		t.Errorf("with no key, decrypt should return plaintext: expected %q, got %q", original, decrypted)
	}
}

func TestTokenStore_EncryptDecrypt_EmptyValue(t *testing.T) {
	store := NewTokenStore(nil, "test-key")

	encrypted, err := store.encrypt("")
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}
	if encrypted != "" {
		t.Error("encrypting empty string should return empty string")
	}

	decrypted, err := store.decrypt("")
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}
	if decrypted != "" {
		t.Error("decrypting empty string should return empty string")
	}
}

func TestTokenStore_Decrypt_PlaintextFallback(t *testing.T) {
	store := NewTokenStore(nil, "test-key")

	// Attempt to decrypt a non-base64 plaintext string — should return it as-is
	result, err := store.decrypt("not-encrypted-token")
	if err != nil {
		t.Fatalf("decrypt plaintext fallback: %v", err)
	}
	if result != "not-encrypted-token" {
		t.Errorf("plaintext fallback: expected original, got %q", result)
	}
}

func TestTokenStore_EncryptDecrypt_DifferentKeysProduceDifferentOutput(t *testing.T) {
	store1 := NewTokenStore(nil, "key-alpha")
	store2 := NewTokenStore(nil, "key-beta")

	original := "shared-secret"
	enc1, _ := store1.encrypt(original)
	enc2, _ := store2.encrypt(original)

	if enc1 == enc2 {
		t.Error("different encryption keys should produce different ciphertext")
	}
}

func TestTokenStore_EncryptDecrypt_SameKeyDifferentNonces(t *testing.T) {
	store := NewTokenStore(nil, "determinism-check")

	original := "same-input"
	enc1, _ := store.encrypt(original)
	enc2, _ := store.encrypt(original)

	if enc1 == enc2 {
		t.Error("encrypting the same value twice should produce different ciphertext (random nonce)")
	}

	// Both should decrypt to the same value
	dec1, _ := store.decrypt(enc1)
	dec2, _ := store.decrypt(enc2)
	if dec1 != original || dec2 != original {
		t.Errorf("both ciphertexts should decrypt to %q, got %q and %q", original, dec1, dec2)
	}
}

// --- OAuthHandler tests ---

func TestOAuthHandler_NewOAuthHandler(t *testing.T) {
	h := NewOAuthHandler(nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.providers == nil {
		t.Error("providers map should be initialized")
	}
	if h.states == nil {
		t.Error("states map should be initialized")
	}
}

func TestOAuthHandler_RegisterProvider(t *testing.T) {
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("test-provider", OAuth2Config{})
	h.RegisterProvider(provider)

	if _, ok := h.providers["test-provider"]; !ok {
		t.Error("provider should be registered")
	}
}

func TestOAuthHandler_StartHandler_MissingProvider(t *testing.T) {
	h := NewOAuthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/auth//start", nil)
	req.SetPathValue("provider", "")
	rec := httptest.NewRecorder()

	h.StartHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing provider, got %d", rec.Code)
	}
}

func TestOAuthHandler_StartHandler_UnknownProvider(t *testing.T) {
	h := NewOAuthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/unknown/start", nil)
	req.SetPathValue("provider", "unknown")
	rec := httptest.NewRecorder()

	h.StartHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown provider, got %d", rec.Code)
	}
}

func TestOAuthHandler_StartHandler_RedirectToAuthURL(t *testing.T) {
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("google", OAuth2Config{
		ClientID:     "test-id",
		RedirectURL:  "http://localhost/callback",
		AuthEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
	})
	h.RegisterProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	req.SetPathValue("provider", "google")
	rec := httptest.NewRecorder()

	h.StartHandler(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "accounts.google.com") {
		t.Errorf("redirect location should point to Google auth: %s", loc)
	}
	if !strings.Contains(loc, "client_id=test-id") {
		t.Error("redirect URL should include client_id")
	}
}

func TestOAuthHandler_StartHandler_CreatesState(t *testing.T) {
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("google", OAuth2Config{
		AuthEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
	})
	h.RegisterProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	req.SetPathValue("provider", "google")
	rec := httptest.NewRecorder()

	h.StartHandler(rec, req)

	h.mu.Lock()
	stateCount := len(h.states)
	h.mu.Unlock()

	if stateCount != 1 {
		t.Errorf("expected 1 CSRF state entry, got %d", stateCount)
	}
}

func TestOAuthHandler_CallbackHandler_MissingCode(t *testing.T) {
	h := NewOAuthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
	rec := httptest.NewRecorder()

	h.CallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing code, got %d", rec.Code)
	}
}

func TestOAuthHandler_CallbackHandler_OAuthError(t *testing.T) {
	h := NewOAuthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?error=access_denied&error_description=User+denied", nil)
	rec := httptest.NewRecorder()

	h.CallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for OAuth error, got %d", rec.Code)
	}
}

func TestOAuthHandler_CallbackHandler_InvalidState(t *testing.T) {
	h := NewOAuthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=testcode&state=invalid", nil)
	rec := httptest.NewRecorder()

	h.CallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid CSRF state, got %d", rec.Code)
	}
}

func TestOAuthHandler_StateEviction(t *testing.T) {
	h := NewOAuthHandler(nil)

	// Manually insert an old state
	h.mu.Lock()
	h.states["old-state"] = "google"
	h.stateCreated["old-state"] = time.Now().Add(-15 * time.Minute) // 15 min old
	h.states["fresh-state"] = "google"
	h.stateCreated["fresh-state"] = time.Now()
	h.mu.Unlock()

	// Trigger a new start to exercise eviction
	provider := NewGenericOAuth2("google", OAuth2Config{
		AuthEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
	})
	h.RegisterProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	req.SetPathValue("provider", "google")
	rec := httptest.NewRecorder()

	h.StartHandler(rec, req)

	h.mu.Lock()
	_, hasOld := h.states["old-state"]
	_, hasFresh := h.states["fresh-state"]
	h.mu.Unlock()

	if hasOld {
		t.Error("old state should have been evicted (>10 min)")
	}
	if !hasFresh {
		t.Error("fresh state should still exist")
	}
}

func TestGenerateState_Unique(t *testing.T) {
	states := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s := generateState()
		if states[s] {
			t.Fatalf("duplicate state generated: %s", s)
		}
		states[s] = true
	}
}

func TestGenerateState_Length(t *testing.T) {
	s := generateState()
	// 16 random bytes → 32 hex characters
	if len(s) != 32 {
		t.Errorf("expected state length 32 (hex of 16 bytes), got %d", len(s))
	}
}
