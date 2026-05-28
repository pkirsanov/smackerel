// BUG-020-008 — regression tests for fail-loud int env parsing.
//
// These tests pin the post-fix contract for the 8 SST-required int env
// vars previously routed through parseIntEnv(key, 0). Each MUST cause
// Load() to return a non-nil error naming the key when the env var is
// unset or unparseable. Today (pre-fix) parseIntEnv silently substitutes
// 0 and the connector remains disabled, so Load() returns nil — these
// tests therefore FAIL against main (Red) and PASS after the helper
// migration (Green).
package config

import (
	"strings"
	"testing"
)

// bug020008IntKeys is the full enumerated set of 8 SST-required int env
// vars covered by BUG-020-008. Order is irrelevant.
var bug020008IntKeys = []string{
	"BOOKMARKS_MIN_URL_LENGTH",
	"BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS",
	"BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD",
	"BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY",
	"QF_DECISIONS_PACKET_VERSION",
	"QF_DECISIONS_PAGE_SIZE",
	"HOSPITABLE_INITIAL_LOOKBACK_DAYS",
	"HOSPITABLE_PAGE_SIZE",
}

// TestBUG020008_MissingSingleIntKey_FailsLoud — for each of the 8 keys,
// physically unset it (adversarial: uses os.Unsetenv via t.Setenv("")).
// Pre-fix: Load() returns nil and field silently becomes 0. Post-fix:
// Load() returns an error naming the key.
func TestBUG020008_MissingSingleIntKey_FailsLoud(t *testing.T) {
	for _, key := range bug020008IntKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "")
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for missing %s, got nil (silent default to 0 is the bug)", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error should name %s, got: %v", key, err)
			}
		})
	}
}

// TestBUG020008_MissingAllIntKeys_ConsolidatedError — unsetting all 8
// simultaneously must produce ONE error that names every missing key,
// matching the existing requiredVars() consolidated-error pattern.
func TestBUG020008_MissingAllIntKeys_ConsolidatedError(t *testing.T) {
	setRequiredEnv(t)
	for _, key := range bug020008IntKeys {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected consolidated error for all 8 missing int keys, got nil")
	}
	for _, key := range bug020008IntKeys {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("consolidated error should name %s, got: %v", key, err)
		}
	}
}

// TestBUG020008_UnparseableIntKey_FailsLoud — for each of the 8 keys,
// setting it to a non-numeric value must produce an error naming the key
// AND the offending value. Pre-fix: parseIntEnv silently returns 0 on
// strconv.Atoi failure and Load() succeeds.
func TestBUG020008_UnparseableIntKey_FailsLoud(t *testing.T) {
	for _, key := range bug020008IntKeys {
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

// TestBUG020008_AllIntKeysValid_NoError — sanity check: with valid non-
// zero ints set (via setRequiredEnv), Load() returns nil. Guards against
// the regression where the fail-loud helper would reject a legitimate
// load.
func TestBUG020008_AllIntKeysValid_NoError(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error with all 8 int keys set to valid values, got: %v", err)
	}
	// Spot-check a representative field round-trips.
	if cfg.BookmarksMinURLLength == 0 {
		t.Errorf("BookmarksMinURLLength should be non-zero after Load with valid env, got 0")
	}
}
