// Live-gated Twitter API v2 tests (spec 056 scope 05).
//
// This file is the single source of truth for tests that hit api.twitter.com
// against a real bearer token. EVERY test in this file MUST short-circuit
// when the opt-in environment variable is not set, so default `go test ./...`
// runs (CI, pre-push hook, contributors' local sandboxes) never pay network
// cost and never burn quota against the operator's Twitter dev account.
//
// Opt-in contract (also documented in docs/Connector_Development.md):
//
//	SMACKEREL_TWITTER_LIVE_TESTS=1
//	  Required to run any live test. Default unset → tests skip.
//
//	SMACKEREL_TWITTER_LIVE_TESTS_TOKEN=<bearer-token>
//	  Required when SMACKEREL_TWITTER_LIVE_TESTS=1. Must be a real Twitter
//	  API v2 bearer token. The token is read into the apiClient and never
//	  logged (the bearer-token-never-in-logs assertion in api_test.go
//	  covers the production code path).
//
// Run locally:
//
//	SMACKEREL_TWITTER_LIVE_TESTS=1 \
//	SMACKEREL_TWITTER_LIVE_TESTS_TOKEN="$(pass twitter/dev-bearer)" \
//	  go test ./internal/connector/twitter/ -run TestTwitterAPILive -v
//
// CI MUST NOT set either variable. The default-unset skip is the contract
// that keeps the test suite hermetic; the regression test below catches any
// attempt to set the env var inside a CI-detected environment.
package twitter

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

const (
	// envLiveTests is the master opt-in switch for every test in this file.
	envLiveTests = "SMACKEREL_TWITTER_LIVE_TESTS"
	// envLiveToken carries the real bearer token used by the opt-in arm.
	envLiveToken = "SMACKEREL_TWITTER_LIVE_TESTS_TOKEN"
)

// requireLiveOptIn is the gate every live test runs through first. Returns
// the bearer token when the gate permits the test to run; calls t.Skip
// (and returns "") when the gate forbids it. CALLERS MUST NOT proceed past
// this function on an empty return value.
func requireLiveOptIn(t *testing.T) string {
	t.Helper()
	if os.Getenv(envLiveTests) == "" {
		t.Skipf("live Twitter API tests are opt-in; set %s=1 + %s=<bearer> to run", envLiveTests, envLiveToken)
		return ""
	}
	token := os.Getenv(envLiveToken)
	if token == "" {
		t.Fatalf("%s=1 set but %s is empty; opt-in requires both", envLiveTests, envLiveToken)
		return ""
	}
	return token
}

// TestTwitterAPI_LiveTestSkipsWhenEnvVarUnset — SCN-056-006.
//
// Given the environment variable SMACKEREL_TWITTER_LIVE_TESTS is unset
// When go test runs this file
// Then this test reports SKIP and does not contact api.twitter.com
//
// This test is the contract anchor: it ASSERTS the skip behavior so a
// future change that loses the gate would break a real test rather than
// silently start making live API calls in CI.
//
// Implementation: temporarily unset the env vars for the duration of this
// test, then invoke requireLiveOptIn and assert it skipped (Skipped()).
func TestTwitterAPI_LiveTestSkipsWhenEnvVarUnset(t *testing.T) {
	t.Setenv(envLiveTests, "")
	t.Setenv(envLiveToken, "")

	// Subtest with the gate inside, so we can observe the skip without
	// short-circuiting this parent test.
	t.Run("gate", func(sub *testing.T) {
		_ = requireLiveOptIn(sub)
		// requireLiveOptIn must call Skip; if it returns, the subtest is
		// expected to report Skipped() == true after this function returns.
	})

	// Inspect the parent test's child results: we expected the subtest to
	// skip. There is no direct API for that in standard testing, but a
	// passing subtest with no errors and no failure is acceptable because
	// Skip terminates the goroutine. The real assertion below is the
	// inverse: with env vars CLEARLY unset, no goroutine should reach
	// past requireLiveOptIn. We assert that by counting via a flag.
	reachedPast := false
	t.Run("flag-after-gate", func(sub *testing.T) {
		_ = requireLiveOptIn(sub)
		reachedPast = true
	})
	if reachedPast {
		t.Fatalf("requireLiveOptIn must Skip when %s is unset; code reached past the gate", envLiveTests)
	}
}

// TestTwitterAPI_LiveTestGateAlsoBlocksTokenOnly verifies the adversarial
// inverse: if a future refactor accidentally read only the token env var
// and forgot the master switch, requireLiveOptIn would silently start
// hitting the live API. This test asserts that setting ONLY the token
// (without the master switch) does NOT bypass the skip.
func TestTwitterAPI_LiveTestGateAlsoBlocksTokenOnly(t *testing.T) {
	t.Setenv(envLiveTests, "")
	t.Setenv(envLiveToken, "would-be-real-but-master-switch-is-off")

	reachedPast := false
	t.Run("inner", func(sub *testing.T) {
		_ = requireLiveOptIn(sub)
		reachedPast = true
	})
	if reachedPast {
		t.Fatalf("setting token without master switch must still Skip; gate bypass detected")
	}
}

// TestTwitterAPI_LiveTestNeverRunsInCI guards against the live tests being
// turned on inside a CI-detected environment. Recognized CI sentinels are
// the GitHub Actions defaults (CI=true, GITHUB_ACTIONS=true). If any of
// these are set AND the master live switch is also set, the test fails
// loud — that combination is forbidden by the opt-in contract.
//
// In normal CI runs this test is a no-op: the master switch is unset, the
// gate skips, the test passes silently. In local opt-in runs the test
// passes because the CI sentinels are unset. Only the unsafe combination
// of CI + live switch trips the failure.
func TestTwitterAPI_LiveTestNeverRunsInCI(t *testing.T) {
	if os.Getenv(envLiveTests) == "" {
		t.Skipf("master switch %s unset; nothing to check", envLiveTests)
		return
	}
	ciSentinels := []string{"CI", "GITHUB_ACTIONS"}
	var tripped []string
	for _, s := range ciSentinels {
		if v := os.Getenv(s); v != "" && strings.ToLower(v) != "false" && v != "0" {
			tripped = append(tripped, s+"="+v)
		}
	}
	if len(tripped) > 0 {
		t.Fatalf("live Twitter tests are forbidden in CI; %s set alongside %s: %v",
			envLiveTests, strings.Join(ciSentinels, "/"), tripped)
	}
}

// TestTwitterAPILive_UsersMe is the opt-in arm. Hits the real Twitter API
// v2 GET /2/users/me and asserts the response shape (data.id non-empty,
// data.username non-empty). Bounded to a 15-second context so a slow or
// hung API does not stall the entire test run.
//
// This test is the smallest possible live exercise of the apiClient. If it
// passes, the foundation, request builder, retry wrapper, and JSON decoder
// all work end-to-end against the real API.
//
// Quota cost: 1 request against the bearer token's `users.read` allowance.
func TestTwitterAPILive_UsersMe(t *testing.T) {
	token := requireLiveOptIn(t)
	if token == "" {
		return // requireLiveOptIn already called t.Skip
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := New("twitter")
	if err := c.Connect(ctx, connectorConfigForLive(token)); err != nil {
		t.Fatalf("Connect (live): %v", err)
	}
	if c.apiClient == nil {
		t.Fatalf("Connect (live) did not construct apiClient")
	}

	user, err := c.apiClient.fetchUsersMe(ctx)
	if err != nil {
		t.Fatalf("live /users/me: %v", err)
	}
	if user == nil || user.Data.ID == "" {
		t.Fatalf("live /users/me returned empty data: %+v", user)
	}
	if user.Data.Username == "" {
		t.Fatalf("live /users/me returned empty username: %+v", user.Data)
	}
	t.Logf("live /users/me OK: id=%s username=%s", user.Data.ID, user.Data.Username)
}

// connectorConfigForLive builds the minimal ConnectorConfig the live test
// needs: api-only sync mode, real bearer token. Kept as a helper so future
// live tests reuse the exact same wiring as the production path.
func connectorConfigForLive(token string) connector.ConnectorConfig {
	return connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "api"},
		Credentials:  map[string]string{"bearer_token": token},
	}
}
