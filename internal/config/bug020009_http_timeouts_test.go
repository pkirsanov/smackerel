// BUG-020-009 — regression tests for fail-loud HTTP timeout SST config.
//
// Pins the post-fix contract for the 2 new SST-required int env vars
// (FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS, AUTH_OAUTH_HTTP_TIMEOUT_SECONDS)
// that replace the hardcoded `Timeout: 10 * time.Second` literal at
// internal/connector/markets/markets.go and `Timeout: 15 * time.Second`
// at internal/auth/oauth.go.
//
// Each MUST cause Load()/Validate() to return a non-nil error naming the
// offending key when the env var is unset, unparseable, or non-positive.
// Pre-fix the keys do not exist in Config; Load() returns nil and the
// hardcoded literals govern, so these tests FAIL against `main` (Red).
package config

import (
	"strings"
	"testing"
)

// bug020009HTTPTimeoutKeys enumerates the 2 SST-required int env vars
// covered by BUG-020-009. Order is irrelevant.
var bug020009HTTPTimeoutKeys = []string{
	"FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS",
	"AUTH_OAUTH_HTTP_TIMEOUT_SECONDS",
}

// TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud — for each of the
// 2 keys, physically unset it. Load() must return an error naming the key.
func TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud(t *testing.T) {
	for _, key := range bug020009HTTPTimeoutKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "")
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for missing %s, got nil (hardcoded literal is the bug)", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error should name %s, got: %v", key, err)
			}
		})
	}
}

// TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError — unsetting
// both simultaneously must produce ONE error that names every missing
// key, matching the BUG-020-008 consolidated-error pattern.
func TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError(t *testing.T) {
	setRequiredEnv(t)
	for _, key := range bug020009HTTPTimeoutKeys {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected consolidated error for both missing HTTP timeout keys, got nil")
	}
	for _, key := range bug020009HTTPTimeoutKeys {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("consolidated error should name %s, got: %v", key, err)
		}
	}
}

// TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud — for each of the
// 2 keys, setting it to a non-numeric value must produce an error naming
// both the key and the offending value.
func TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud(t *testing.T) {
	for _, key := range bug020009HTTPTimeoutKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "abc")
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for unparseable %s=abc, got nil", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error should name %s, got: %v", key, err)
			}
			if !strings.Contains(err.Error(), "abc") {
				t.Errorf("error should name the offending value %q, got: %v", "abc", err)
			}
		})
	}
}

// TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud — for each of the
// 2 keys, setting it to "0" or "-1" must produce a range error naming
// the key. A 0-second HTTP timeout is meaningless; a negative value is
// a Go runtime trap. The range guard runs in Validate() after the
// mustParseIntEnv fail-loud loader, so Load() may surface either the
// loader error or the validate error depending on parse success.
func TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud(t *testing.T) {
	for _, key := range bug020009HTTPTimeoutKeys {
		for _, badVal := range []string{"0", "-1"} {
			key := key
			badVal := badVal
			t.Run(key+"="+badVal, func(t *testing.T) {
				setRequiredEnv(t)
				t.Setenv(key, badVal)
				cfg, loadErr := Load()
				if loadErr != nil {
					// Some loaders surface the range error at Load() time.
					if !strings.Contains(loadErr.Error(), key) {
						t.Errorf("Load error should name %s, got: %v", key, loadErr)
					}
					return
				}
				if err := cfg.Validate(); err == nil {
					t.Fatalf("expected range error for %s=%s, got nil", key, badVal)
				} else if !strings.Contains(err.Error(), key) {
					t.Errorf("Validate error should name %s, got: %v", key, err)
				}
			})
		}
	}
}

// TestBUG020009_AllHTTPTimeoutKeysValid_NoError — sanity: with both
// keys set to valid positive ints (via setRequiredEnv), Load + Validate
// returns nil and the Config fields round-trip. Uses non-default values
// (7 and 9 — NOT the pre-fix literals 10 and 15) so a coincidental
// pass cannot mask a reverted migration.
func TestBUG020009_AllHTTPTimeoutKeysValid_NoError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS", "7")
	t.Setenv("AUTH_OAUTH_HTTP_TIMEOUT_SECONDS", "9")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error with both HTTP timeout keys set to valid values, got: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected Validate to pass with valid HTTP timeout values, got: %v", err)
	}
	if cfg.FinancialMarketsHTTPTimeoutSeconds != 7 {
		t.Errorf("FinancialMarketsHTTPTimeoutSeconds: want 7, got %d", cfg.FinancialMarketsHTTPTimeoutSeconds)
	}
	if cfg.AuthOAuthHTTPTimeoutSeconds != 9 {
		t.Errorf("AuthOAuthHTTPTimeoutSeconds: want 9, got %d", cfg.AuthOAuthHTTPTimeoutSeconds)
	}
}
