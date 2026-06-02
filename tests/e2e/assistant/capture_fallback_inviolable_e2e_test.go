//go:build e2e

// Spec 074 — Capture inviolable e2e (planned).
//
// SCN-074-A09 — Capture is inviolable.
// Test plan row TP-074-04. Scenario status="planned" in
// specs/074-capture-as-fallback-policy/scenario-manifest.json. Stub
// keeps the traceability guard satisfied until the live e2e is wired.

package assistant_e2e

import "testing"

func TestAssistantHTTPE2E_CaptureFallbackIsInviolable_TP_074_04(t *testing.T) {
	t.Skip("planned: SCN-074-A09 / TP-074-04 — see specs/074-capture-as-fallback-policy/scenario-manifest.json (status=planned)")
}
