//go:build e2e

// Spec 074 — Per-user fallback-capture dedup (planned).
//
// SCN-074-A03 — Same-user same-text within dedup window dedupes.
// Test plan row TP-074-11. Scenario status="planned" in
// specs/074-capture-as-fallback-policy/scenario-manifest.json. Stub
// keeps the traceability guard satisfied until the live e2e is wired.

package assistant_e2e

import "testing"

func TestAssistantHTTPE2E_CaptureFallbackDedupWithinWindow_TP_074_11(t *testing.T) {
	t.Skip("planned: SCN-074-A03 / TP-074-11 — see specs/074-capture-as-fallback-policy/scenario-manifest.json (status=planned)")
}
