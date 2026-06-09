package config

import "testing"

// TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault pins the SST contract
// for the three user-context OAuth credential fields (SCN-BUG-056-002-005):
// they are read straight from the environment with NO fallback literal, exactly
// like TWITTER_BEARER_TOKEN. When the env is unset the fields resolve to the
// empty string (fail-loud is enforced downstream where the value is required,
// not via a silent default); when set, the loaded config carries the exact
// values.
func TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault(t *testing.T) {
	// Set → exact passthrough.
	setRequiredEnv(t)
	t.Setenv("TWITTER_OAUTH_CLIENT_ID", "client-abc-123")
	t.Setenv("TWITTER_OAUTH_CLIENT_SECRET", "secret-xyz-789")
	t.Setenv("TWITTER_OAUTH_REDIRECT_URL", "http://127.0.0.1/callback")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with OAuth env set must succeed: %v", err)
	}
	if cfg.TwitterOAuthClientID != "client-abc-123" {
		t.Errorf("TwitterOAuthClientID = %q, want %q", cfg.TwitterOAuthClientID, "client-abc-123")
	}
	if cfg.TwitterOAuthClientSecret != "secret-xyz-789" {
		t.Errorf("TwitterOAuthClientSecret = %q, want %q", cfg.TwitterOAuthClientSecret, "secret-xyz-789")
	}
	if cfg.TwitterOAuthRedirectURL != "http://127.0.0.1/callback" {
		t.Errorf("TwitterOAuthRedirectURL = %q, want %q", cfg.TwitterOAuthRedirectURL, "http://127.0.0.1/callback")
	}

	// Unset → empty string, NO fallback literal substituted (smackerel-no-defaults).
	t.Setenv("TWITTER_OAUTH_CLIENT_ID", "")
	t.Setenv("TWITTER_OAUTH_CLIENT_SECRET", "")
	t.Setenv("TWITTER_OAUTH_REDIRECT_URL", "")

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load with OAuth env empty must still succeed (validation is downstream): %v", err)
	}
	if cfg2.TwitterOAuthClientID != "" {
		t.Errorf("a hidden default was substituted for TWITTER_OAUTH_CLIENT_ID: %q", cfg2.TwitterOAuthClientID)
	}
	if cfg2.TwitterOAuthClientSecret != "" {
		t.Errorf("a hidden default was substituted for TWITTER_OAUTH_CLIENT_SECRET: %q", cfg2.TwitterOAuthClientSecret)
	}
	if cfg2.TwitterOAuthRedirectURL != "" {
		t.Errorf("a hidden default was substituted for TWITTER_OAUTH_REDIRECT_URL: %q", cfg2.TwitterOAuthRedirectURL)
	}
}
