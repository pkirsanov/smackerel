//go:build integration

// Spec 074 SCOPE-2 — TP-074-06 / SCN-074-A02.
//
// Provenance-query proof that explicit and fallback captures of the
// SAME normalized text for the SAME user remain provenance-distinct
// and are independently queryable via CountByProvenance. Split from
// capture_fallback_policy_test.go per scopes.md test plan.
//
// The body lives in capture_fallback_policy_test.go (single source of
// truth for the helper insertTestArtifact); this file holds the
// scenario entrypoint so the test ID maps to its scoped file as
// declared in the plan.

package assistant_integration

import "testing"

func TestCaptureProvenanceQuery_TP_074_06_ExplicitAndFallbackDistinctByProvenance(t *testing.T) {
	TestCaptureFallbackPolicy_TP_074_06_ExplicitAndFallbackDistinguishableByProvenanceQuery(t)
}
