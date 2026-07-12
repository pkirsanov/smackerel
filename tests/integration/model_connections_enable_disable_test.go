//go:build integration

// Spec 096 SCOPE-06 (SCN-096-W03) — live-stack enable/disable catalog membership.
//
// DEFERRED LIVE LEG. This test drives the REAL operator admin surface
// (POST /v1/admin/model-connections/{id}/enable | …/disable) against the
// disposable ephemeral test stack and asserts that the effective-enabled
// predicate (registry-declared AND DB enabled AND last_test_outcome=ok AND
// credential present) is the SINGLE gate the discovery aggregator consults:
// enabling a credentialed+tested slot adds its models to the combined catalog,
// and disabling removes them. It hits the REAL aggregator + DB — NO request
// interception. All credentials are SYNTHETIC (test-isolation + env-pollution
// policy), never a real provider secret.
//
// The unit halves already prove the pieces in isolation:
//   - the 409 enable-guard (internal/api/model_connections_admin_test.go::
//     TestAdminModelConnections_EnableUntested_Blocked409_Spec096), and
//   - the effective-enabled predicate (internal/assistant/openknowledge/
//     connstore/store_test.go::TestEffectiveEnabled_SingleGate_Spec096).
//
// This leg proves the LIVE wiring binds end-to-end. As of the SCOPE-06 backend
// dispatch the connstore.Store CredentialSource + effective-enabled predicate
// are wired, but the live catalog-aggregator path (SCOPE-04 discovery consuming
// connstore.Store.DiscoveryConnections) is the deferred SCOPE-07 / self-hosted
// bubbles.devops dispatch, so this test t.Skip's with an explicit message
// rather than failing until those preconditions are seeded.
//
// To enable once the live aggregator wiring + seeded slots ship:
//
//	SPEC096_ADMIN_LIVE_CORE_URL=http://localhost:<port> \
//	SPEC096_ADMIN_LIVE_OPERATOR_TOKEN=<operator-bearer> \
//	    ./smackerel.sh test integration --go-run TestEnableDisable_CatalogMembershipFollows_Spec096
package integration

import (
	"os"
	"testing"
)

func TestEnableDisable_CatalogMembershipFollows_Spec096(t *testing.T) {
	coreURL := os.Getenv("SPEC096_ADMIN_LIVE_CORE_URL")
	operatorToken := os.Getenv("SPEC096_ADMIN_LIVE_OPERATOR_TOKEN")
	if coreURL == "" || operatorToken == "" {
		t.Skip("DEFERRED (SCOPE-06 C7): set SPEC096_ADMIN_LIVE_CORE_URL + " +
			"SPEC096_ADMIN_LIVE_OPERATOR_TOKEN to run the live enable/disable " +
			"catalog-membership leg against the ephemeral stack; the live " +
			"catalog-aggregator wiring is the deferred SCOPE-07 / self-hosted dispatch")
	}
	// Live assertion (run only when the env above is seeded): wire + test +
	// enable a db-mode slot via /v1/admin/model-connections/{id}/{credential,
	// test,enable}, GET the combined catalog and assert the slot's models are
	// present; POST …/disable and assert they are removed; re-enable and assert
	// they return — proving the effective-enabled predicate is the single gate.
	t.Fatal("SPEC096_ADMIN_LIVE_* set but the live enable/disable catalog-membership " +
		"assertion is not yet implemented; it lands with the SCOPE-07 / self-hosted " +
		"aggregator wiring dispatch (do not green-paint this leg)")
}
