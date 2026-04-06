package auth

import (
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
