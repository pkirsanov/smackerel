//go:build e2e

// Spec 096 SCOPE-06 (SCN-096-W01/W02/W04) — live hosted-provider operator
// admin e2e legs.
//
// DEFERRED LIVE LEGS (C7 terminal posture). These drive the REAL operator
// admin surface (/v1/admin/model-connections*) against REAL hosted-provider
// endpoints with REAL operator credentials, on the home-lab bubbles.devops
// dispatch — NOT in-repo and NOT from dev. They are NOT marked passing here.
//
// The unit halves already prove the handler + gate logic in isolation
// (internal/api/model_connections_admin_test.go +
// internal/api/model_connections_operator_gate_test.go); these legs prove the
// live wire → truthful test → enable flow against real provider reachability +
// real credentials, which requires real secrets and is therefore a home-lab
// concern (test-isolation + env-pollution policy forbid real provider secrets
// in dev/CI).
//
// To enable on the home-lab dispatch, seed the live core URL, the operator
// bearer, and the real provider credentials:
//
//	SPEC096_ADMIN_LIVE_CORE_URL=http://localhost:<port> \
//	SPEC096_ADMIN_LIVE_OPERATOR_TOKEN=<operator-bearer> \
//	SPEC096_ADMIN_LIVE_ANTHROPIC_KEY=<real> ... \
//	    ./smackerel.sh test e2e --go-run 'TestAdmin_.*_Spec096'
package admin

import (
	"os"
	"testing"
)

// liveAdminPrereqs skips honestly when the live core URL / operator token are
// absent — the in-repo / dev default. Returns the seeded values when present.
func liveAdminPrereqs(t *testing.T) (coreURL, operatorToken string) {
	t.Helper()
	coreURL = os.Getenv("SPEC096_ADMIN_LIVE_CORE_URL")
	operatorToken = os.Getenv("SPEC096_ADMIN_LIVE_OPERATOR_TOKEN")
	if coreURL == "" || operatorToken == "" {
		t.Skip("DEFERRED (SCOPE-06 C7, home-lab bubbles.devops dispatch): set " +
			"SPEC096_ADMIN_LIVE_CORE_URL + SPEC096_ADMIN_LIVE_OPERATOR_TOKEN (and the " +
			"real provider credentials) to run the live operator admin legs; real " +
			"provider secrets are forbidden in dev/CI")
	}
	return coreURL, operatorToken
}

// TestAdmin_WireTestEnableAnthropic_Spec096 — the live operator wire → truthful
// test → enable flow against a REAL Anthropic connection (home-lab).
func TestAdmin_WireTestEnableAnthropic_Spec096(t *testing.T) {
	_, _ = liveAdminPrereqs(t)
	t.Fatal("SPEC096_ADMIN_LIVE_* set but the live Anthropic wire/test/enable " +
		"assertion is implemented on the home-lab bubbles.devops dispatch (real " +
		"reachability + real credential); do not green-paint this leg from dev")
}

// TestAdmin_AddFourHostedProviders_Independent_Spec096 — adding OpenAI /
// Azure-Foundry / Google / Bedrock as four independent live connections,
// each independently testable/enable-able/disable-able (home-lab).
func TestAdmin_AddFourHostedProviders_Independent_Spec096(t *testing.T) {
	_, _ = liveAdminPrereqs(t)
	t.Fatal("SPEC096_ADMIN_LIVE_* set but the live four-hosted-providers " +
		"assertion is implemented on the home-lab bubbles.devops dispatch (real " +
		"credentials); do not green-paint this leg from dev")
}

// TestAdmin_BadCredential_FailsTruthfully_Spec096 — a bad credential against a
// REAL provider endpoint fails truthfully (outcome:failed, typed detail),
// never a false success and never an Ollama substitute (home-lab).
func TestAdmin_BadCredential_FailsTruthfully_Spec096(t *testing.T) {
	_, _ = liveAdminPrereqs(t)
	t.Fatal("SPEC096_ADMIN_LIVE_* set but the live bad-credential truthful-failure " +
		"assertion is implemented on the home-lab bubbles.devops dispatch (real " +
		"endpoint reachability); do not green-paint this leg from dev")
}
