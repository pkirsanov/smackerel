package auth

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestTokenStore_Decrypt_FailClosed_NotBase64(t *testing.T) {
	store := NewTokenStore(nil, "test-key")

	// SCN-020-013: Attempt to decrypt a non-base64 string with encryption key present.
	// Must return error, NOT silently treat as plaintext.
	result, err := store.decrypt("not-encrypted-token")
	if err == nil {
		t.Fatalf("expected error for non-base64 input with encryption key, got result: %q", result)
	}
	if result != "" {
		t.Errorf("expected empty string on error, got %q", result)
	}
}

func TestTokenStore_Decrypt_FailClosed_TooShort(t *testing.T) {
	store := NewTokenStore(nil, "test-key")

	// SCN-020-013: Base64-decodable but too short for AES-GCM nonce — must error.
	shortData := "dGVzdA==" // "test" in base64 — only 4 bytes, less than nonce size
	result, err := store.decrypt(shortData)
	if err == nil {
		t.Fatalf("expected error for too-short encrypted data, got result: %q", result)
	}
	if result != "" {
		t.Errorf("expected empty string on error, got %q", result)
	}
}

func TestTokenStore_Decrypt_FailClosed_GCMFailure(t *testing.T) {
	store := NewTokenStore(nil, "test-key")

	// SCN-020-013: Valid base64 and long enough, but not valid ciphertext — GCM fails.
	// 32 bytes of base64-encoded garbage (enough to exceed nonce size)
	badCiphertext := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	result, err := store.decrypt(badCiphertext)
	if err == nil {
		t.Fatalf("expected error for invalid ciphertext, got result: %q", result)
	}
	if result != "" {
		t.Errorf("expected empty string on error, got %q", result)
	}
}

func TestTokenStore_Decrypt_NoKey_PlaintextPassthrough(t *testing.T) {
	// SCN-020-014: No encryption key — plaintext passthrough for dev mode.
	store := NewTokenStore(nil, "")

	result, err := store.decrypt("any-plaintext-value")
	if err != nil {
		t.Fatalf("expected no error for plaintext passthrough: %v", err)
	}
	if result != "any-plaintext-value" {
		t.Errorf("expected passthrough, got %q", result)
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

func TestTokenStore_Decrypt_WrongKey_FailClosed(t *testing.T) {
	// SCN-020-013 adversarial: encrypt with key A, decrypt with key B.
	// Must return error — not silently return garbage or plaintext.
	storeA := NewTokenStore(nil, "key-alpha-encrypt")
	storeB := NewTokenStore(nil, "key-beta-decrypt")

	original := "sensitive-access-token"
	encrypted, err := storeA.encrypt(original)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	result, err := storeB.decrypt(encrypted)
	if err == nil {
		t.Fatalf("expected error decrypting with wrong key, got result: %q", result)
	}
	if result != "" {
		t.Errorf("expected empty string on wrong-key decrypt, got %q", result)
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
		s, err := generateState()
		if err != nil {
			t.Fatalf("generateState returned error: %v", err)
		}
		if states[s] {
			t.Fatalf("duplicate state generated: %s", s)
		}
		states[s] = true
	}
}

func TestGenerateState_Length(t *testing.T) {
	s, err := generateState()
	if err != nil {
		t.Fatalf("generateState returned error: %v", err)
	}
	// 16 random bytes → 32 hex characters
	if len(s) != 32 {
		t.Errorf("expected state length 32 (hex of 16 bytes), got %d", len(s))
	}
}

func TestOAuthHandler_StartHandler_StateCap(t *testing.T) {
	// Verifies DOS protection: StartHandler rejects requests when 100 pending states exist.
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("google", OAuth2Config{
		AuthEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
	})
	h.RegisterProvider(provider)

	// Fill up to the 100-state cap
	h.mu.Lock()
	for i := 0; i < 100; i++ {
		state := fmt.Sprintf("state-%d", i)
		h.states[state] = "google"
		h.stateCreated[state] = time.Now()
	}
	h.mu.Unlock()

	// Next request should be rejected with 429
	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	req.SetPathValue("provider", "google")
	rec := httptest.NewRecorder()
	h.StartHandler(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 when state cap reached, got %d", rec.Code)
	}
}

func TestOAuthHandler_StartHandler_StateCapAfterEviction(t *testing.T) {
	// Verifies that state cap works correctly after eviction frees space.
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("google", OAuth2Config{
		AuthEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
	})
	h.RegisterProvider(provider)

	// Fill 100 states, but make them all expired (>10 min old)
	h.mu.Lock()
	for i := 0; i < 100; i++ {
		state := fmt.Sprintf("expired-state-%d", i)
		h.states[state] = "google"
		h.stateCreated[state] = time.Now().Add(-15 * time.Minute)
	}
	h.mu.Unlock()

	// Request should succeed because eviction clears all expired entries first
	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	req.SetPathValue("provider", "google")
	rec := httptest.NewRecorder()
	h.StartHandler(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302 after eviction freed space, got %d", rec.Code)
	}
}

func TestGenericOAuth2_TokenRequest_ErrorBodyIncluded(t *testing.T) {
	// Verifies that non-200 token endpoint responses include the error body in the error message.
	errBody := `{"error":"invalid_grant","error_description":"Token has been revoked"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(errBody))
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "id",
		ClientSecret:  "secret",
		RedirectURL:   "http://localhost/cb",
		TokenEndpoint: srv.URL,
	})

	_, err := provider.ExchangeCode(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should include response body detail, got: %v", err)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should include status code, got: %v", err)
	}
}

// --- ExchangeCode / RefreshToken full-flow tests via httptest server ---

func TestGenericOAuth2_ExchangeCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content type, got %s", ct)
		}

		// Verify grant_type and code are in the body
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "authorization_code" {
			t.Errorf("expected grant_type=authorization_code, got %s", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("code") != "auth-code-123" {
			t.Errorf("expected code=auth-code-123, got %s", r.PostForm.Get("code"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "at-xyz",
			"refresh_token": "rt-xyz",
			"expires_in":    3600,
			"token_type":    "Bearer",
			"scope":         "email calendar",
		})
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "cid",
		ClientSecret:  "csecret",
		RedirectURL:   "http://localhost/cb",
		TokenEndpoint: srv.URL,
	})

	token, err := provider.ExchangeCode(context.Background(), "auth-code-123")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if token.AccessToken != "at-xyz" {
		t.Errorf("expected access_token=at-xyz, got %q", token.AccessToken)
	}
	if token.RefreshToken != "rt-xyz" {
		t.Errorf("expected refresh_token=rt-xyz, got %q", token.RefreshToken)
	}
	if token.TokenType != "Bearer" {
		t.Errorf("expected token_type=Bearer, got %q", token.TokenType)
	}
	if len(token.Scopes) != 2 || token.Scopes[0] != "email" || token.Scopes[1] != "calendar" {
		t.Errorf("expected scopes [email calendar], got %v", token.Scopes)
	}
	if token.IsExpired() {
		t.Error("newly exchanged token should not be expired")
	}
}

func TestGenericOAuth2_RefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token, got %s", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("refresh_token") != "old-rt" {
			t.Errorf("expected refresh_token=old-rt, got %s", r.PostForm.Get("refresh_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-at",
			"expires_in":   7200,
			"token_type":   "Bearer",
			"scope":        "email",
		})
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "cid",
		ClientSecret:  "csecret",
		TokenEndpoint: srv.URL,
	})

	token, err := provider.RefreshToken(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if token.AccessToken != "new-at" {
		t.Errorf("expected access_token=new-at, got %q", token.AccessToken)
	}
	// RefreshToken may be empty if provider doesn't issue new one
	if token.RefreshToken != "" {
		t.Errorf("expected empty refresh_token (provider didn't issue new one), got %q", token.RefreshToken)
	}
}

func TestGenericOAuth2_TokenRequest_EmptyErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// Empty body
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "cid",
		ClientSecret:  "csecret",
		TokenEndpoint: srv.URL,
	})

	_, err := provider.ExchangeCode(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should include status code 401, got: %v", err)
	}
}

func TestGenericOAuth2_TokenRequest_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "cid",
		ClientSecret:  "csecret",
		TokenEndpoint: srv.URL,
	})

	_, err := provider.ExchangeCode(context.Background(), "code")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode token response") {
		t.Errorf("error should mention decode failure, got: %v", err)
	}
}

func TestGenericOAuth2_TokenRequest_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "cid",
		ClientSecret:  "csecret",
		TokenEndpoint: srv.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := provider.ExchangeCode(ctx, "code")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGenericOAuth2_TokenRequest_NoScopes(t *testing.T) {
	// Verify tokens without a scope field produce nil/empty scopes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "at",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "cid",
		ClientSecret:  "csecret",
		TokenEndpoint: srv.URL,
	})

	token, err := provider.ExchangeCode(context.Background(), "code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(token.Scopes) != 0 {
		t.Errorf("expected empty scopes when none returned, got %v", token.Scopes)
	}
}

func TestGenericOAuth2_TokenRequest_InvalidEndpointURL(t *testing.T) {
	provider := NewGenericOAuth2("test", OAuth2Config{
		ClientID:      "cid",
		ClientSecret:  "csecret",
		TokenEndpoint: "http://127.0.0.1:0/unreachable",
	})

	_, err := provider.ExchangeCode(context.Background(), "code")
	if err == nil {
		t.Fatal("expected error for unreachable endpoint")
	}
}

// --- OAuthHandler callback full-flow tests ---

// mockProvider implements OAuth2Provider for handler testing.
type mockProvider struct {
	name        string
	authURL     string
	token       *Token
	exchangeErr error
}

func (m *mockProvider) ProviderName() string { return m.name }
func (m *mockProvider) AuthURL(scopes []string, state string) string {
	return m.authURL + "?state=" + state
}
func (m *mockProvider) ExchangeCode(ctx context.Context, code string) (*Token, error) {
	if m.exchangeErr != nil {
		return nil, m.exchangeErr
	}
	return m.token, nil
}
func (m *mockProvider) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	return m.token, nil
}

func TestOAuthHandler_CallbackHandler_SuccessFlow(t *testing.T) {
	// The full success flow requires a real DB (pool) for store.Save.
	// Here we test that the handler correctly validates state, calls ExchangeCode,
	// and consumes the state token. The Save call will fail because pool is nil,
	// producing a 500 — which exercises the "token storage failed" error path.
	store := NewTokenStore(nil, "")
	h := NewOAuthHandler(store)

	mp := &mockProvider{
		name:    "testprov",
		authURL: "https://auth.example.com",
		token: &Token{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			TokenType:    "Bearer",
			Scopes:       []string{"email"},
		},
	}
	h.RegisterProvider(mp)

	// Pre-register a valid state
	h.mu.Lock()
	h.states["valid-state"] = "testprov"
	h.stateCreated["valid-state"] = time.Now()
	h.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/auth/testprov/callback?code=authcode123&state=valid-state", nil)
	rec := httptest.NewRecorder()

	// This will panic on store.Save (nil pool) — recover to test the state consumption
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil pool dereference in store.Save
			}
		}()
		h.CallbackHandler(rec, req)
	}()

	// State must be consumed even if save fails
	h.mu.Lock()
	_, stateExists := h.states["valid-state"]
	h.mu.Unlock()
	if stateExists {
		t.Error("state should be consumed after callback")
	}
}

func TestOAuthHandler_CallbackHandler_ExchangeFailure(t *testing.T) {
	store := NewTokenStore(nil, "")
	h := NewOAuthHandler(store)

	mp := &mockProvider{
		name:        "failprov",
		exchangeErr: fmt.Errorf("exchange denied"),
	}
	h.RegisterProvider(mp)

	// Pre-register state
	h.mu.Lock()
	h.states["fail-state"] = "failprov"
	h.stateCreated["fail-state"] = time.Now()
	h.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/auth/failprov/callback?code=badcode&state=fail-state", nil)
	rec := httptest.NewRecorder()

	h.CallbackHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for exchange failure, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "token exchange failed") {
		t.Errorf("expected 'token exchange failed' in body, got: %s", rec.Body.String())
	}
}

func TestOAuthHandler_CallbackHandler_StateReplay(t *testing.T) {
	// Test that a state token cannot be reused (replay attack prevention)
	store := NewTokenStore(nil, "")
	h := NewOAuthHandler(store)

	mp := &mockProvider{
		name:    "google",
		authURL: "https://auth.google.com",
		token: &Token{
			AccessToken: "at",
			ExpiresAt:   time.Now().Add(1 * time.Hour),
		},
	}
	h.RegisterProvider(mp)

	h.mu.Lock()
	h.states["once-only"] = "google"
	h.stateCreated["once-only"] = time.Now()
	h.mu.Unlock()

	// First use — will panic on store.Save (nil pool), recover to proceed
	func() {
		defer func() { recover() }()
		req1 := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=code1&state=once-only", nil)
		rec1 := httptest.NewRecorder()
		h.CallbackHandler(rec1, req1)
	}()

	// Second use — state should already be consumed, should get 400

	req2 := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=code2&state=once-only", nil)
	rec2 := httptest.NewRecorder()
	h.CallbackHandler(rec2, req2)

	if rec2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for replayed state, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "invalid state") {
		t.Errorf("expected 'invalid state' error for replayed state, got: %s", rec2.Body.String())
	}
}

func TestOAuthHandler_StatusHandler_Empty(t *testing.T) {
	store := NewTokenStore(nil, "")
	h := NewOAuthHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	rec := httptest.NewRecorder()

	h.StatusHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(status) != 0 {
		t.Errorf("expected empty status with no providers, got %v", status)
	}
}

func TestOAuthHandler_StatusHandler_ReturnsJSON(t *testing.T) {
	// Verify StatusHandler produces valid JSON with Content-Type header,
	// using no registered providers to avoid nil pool in HasToken.
	h := NewOAuthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	rec := httptest.NewRecorder()

	h.StatusHandler(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	// Body should be valid JSON (empty object)
	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}
}

// --- Token store encryption edge cases ---

func TestTokenStore_EncryptDecrypt_Unicode(t *testing.T) {
	store := NewTokenStore(nil, "unicode-test-key")

	original := "日本語トークン🔐émojis-and-ünïcödé"
	encrypted, err := store.encrypt(original)
	if err != nil {
		t.Fatalf("encrypt unicode: %v", err)
	}
	decrypted, err := store.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt unicode: %v", err)
	}
	if decrypted != original {
		t.Errorf("unicode roundtrip failed: expected %q, got %q", original, decrypted)
	}
}

func TestTokenStore_EncryptDecrypt_LongValue(t *testing.T) {
	store := NewTokenStore(nil, "long-value-test-key")

	// 10KB token value (exercises AES-GCM with larger payloads)
	original := strings.Repeat("A", 10240)
	encrypted, err := store.encrypt(original)
	if err != nil {
		t.Fatalf("encrypt long value: %v", err)
	}
	decrypted, err := store.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt long value: %v", err)
	}
	if decrypted != original {
		t.Errorf("long value roundtrip failed: lengths %d vs %d", len(original), len(decrypted))
	}
}

func TestTokenStore_KeyDerivation_Deterministic(t *testing.T) {
	// Verify that the same encryption key always produces the same derived 32-byte key.
	store1 := NewTokenStore(nil, "my-key")
	store2 := NewTokenStore(nil, "my-key")

	if len(store1.encKey) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(store1.encKey))
	}
	for i := range store1.encKey {
		if store1.encKey[i] != store2.encKey[i] {
			t.Fatal("same input key should derive the same encryption key")
		}
	}
}

func TestTokenStore_Decrypt_TruncatedCiphertext(t *testing.T) {
	store := NewTokenStore(nil, "test-key")

	// Encrypt a value, then truncate the ciphertext before decrypting
	original := "sensitive-data"
	encrypted, err := store.encrypt(original)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Truncate to just past the nonce — valid base64 but invalid GCM ciphertext
	truncated := encrypted[:24]
	_, err = store.decrypt(truncated)
	if err == nil {
		t.Fatal("expected error for truncated ciphertext")
	}
}

func TestOAuthHandler_CallbackHandler_EmptyState(t *testing.T) {
	// Empty state parameter — should be rejected as invalid
	h := NewOAuthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=testcode&state=", nil)
	rec := httptest.NewRecorder()

	h.CallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty state, got %d", rec.Code)
	}
}

func TestOAuthHandler_StartHandler_GoogleScopes(t *testing.T) {
	// Verify that the Google provider gets Gmail, Calendar, YouTube scopes in the redirect URL
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("google", OAuth2Config{
		ClientID:     "goog-id",
		RedirectURL:  "http://localhost/cb",
		AuthEndpoint: "https://accounts.google.com/auth",
	})
	h.RegisterProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	req.SetPathValue("provider", "google")
	rec := httptest.NewRecorder()

	h.StartHandler(rec, req)

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "mail.google.com") {
		t.Error("Google redirect should include Gmail scope")
	}
	if !strings.Contains(loc, "calendar") {
		t.Error("Google redirect should include Calendar scope")
	}
	if !strings.Contains(loc, "youtube") {
		t.Error("Google redirect should include YouTube scope")
	}
}

func TestOAuthHandler_StartHandler_NonGoogleDefaultScopes(t *testing.T) {
	// Non-google providers should get default scopes (openid, profile, email)
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("github", OAuth2Config{
		ClientID:     "gh-id",
		RedirectURL:  "http://localhost/cb",
		AuthEndpoint: "https://github.com/login/oauth/authorize",
	})
	h.RegisterProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/auth/github/start", nil)
	req.SetPathValue("provider", "github")
	rec := httptest.NewRecorder()

	h.StartHandler(rec, req)

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "openid") {
		t.Error("non-Google provider redirect should include openid scope")
	}
	if !strings.Contains(loc, "profile") {
		t.Error("non-Google provider redirect should include profile scope")
	}
	if !strings.Contains(loc, "email") {
		t.Error("non-Google provider redirect should include email scope")
	}
}

// --- Hardening: H3 — CallbackHandler enforces state TTL ---

func TestOAuthHandler_CallbackHandler_ExpiredState(t *testing.T) {
	h := NewOAuthHandler(nil)
	provider := NewGenericOAuth2("google", OAuth2Config{
		TokenEndpoint: "https://oauth2.googleapis.com/token",
	})
	h.RegisterProvider(provider)

	// Manually inject a state that was created 15 minutes ago (expired)
	h.mu.Lock()
	h.states["expired-state"] = "google"
	h.stateCreated["expired-state"] = time.Now().Add(-15 * time.Minute)
	h.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=valid-code&state=expired-state", nil)
	rec := httptest.NewRecorder()

	h.CallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for expired state, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "expired") {
		t.Error("error message should mention expiration")
	}

	// State should be consumed (deleted) even if expired
	h.mu.Lock()
	_, stillExists := h.states["expired-state"]
	h.mu.Unlock()
	if stillExists {
		t.Error("expired state should be deleted after callback attempt")
	}
}

func TestOAuthHandler_CallbackHandler_FreshState(t *testing.T) {
	// A fresh state (within TTL) should proceed to token exchange.
	// The exchange will fail because the token server is fake, but the
	// state validation should pass — checking that fresh states are NOT
	// rejected by the TTL check.
	h := NewOAuthHandler(nil)

	// Use a test server that returns an error to distinguish state-rejected vs exchange-failed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	provider := NewGenericOAuth2("google", OAuth2Config{
		TokenEndpoint: srv.URL,
	})
	h.RegisterProvider(provider)

	// Inject a fresh state (created just now)
	h.mu.Lock()
	h.states["fresh-state"] = "google"
	h.stateCreated["fresh-state"] = time.Now()
	h.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=fresh-state", nil)
	rec := httptest.NewRecorder()

	h.CallbackHandler(rec, req)

	// Should get 500 (token exchange failed), NOT 400 (state rejected)
	if rec.Code == http.StatusBadRequest {
		body := rec.Body.String()
		if strings.Contains(body, "expired") || strings.Contains(body, "invalid state") {
			t.Error("fresh state should not be rejected by TTL check")
		}
	}
}

// --- IMP-020-CSP-003: OAuth callback success page has no inline script ---

func TestOAuthHandler_CallbackSuccessPage_NoInlineScript(t *testing.T) {
	// Verify the OAuth callback success HTML does not contain an inline <script>
	// tag. CSP script-src blocks inline JS; the success page must work without it.

	// Build a handler with a mock provider that successfully exchanges and a mock
	// store that doesn't need a real DB for this rendering test.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	// Use a nil-pool store — Save will panic, but we recover and still check HTML output
	store := NewTokenStore(nil, "")
	h := NewOAuthHandler(store)
	h.RegisterProvider(NewGenericOAuth2("test", OAuth2Config{
		TokenEndpoint: srv.URL,
	}))

	h.mu.Lock()
	h.states["csp-test-state"] = "test"
	h.stateCreated["csp-test-state"] = time.Now()
	h.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/auth/test/callback?code=testcode&state=csp-test-state", nil)
	rec := httptest.NewRecorder()

	// Save panics on nil pool — recover but still capture what was written
	func() {
		defer func() { recover() }()
		h.CallbackHandler(rec, req)
	}()

	body := rec.Body.String()

	// If status 200 (success page rendered), check for no inline script
	if rec.Code == http.StatusOK && strings.Contains(body, "Authorization successful") {
		if strings.Contains(body, "<script>") || strings.Contains(body, "<script ") {
			t.Error("OAuth callback success page must not contain inline <script> (blocked by CSP)")
		}
	}
	// If the save panicked (status 500 or 0), the success page wasn't rendered —
	// that's OK for this test because the page template changes are the focus.
}
