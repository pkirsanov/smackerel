//go:build integration

// Spec 074 — Capture/IntentTrace join (planned).
//
// SCN-074-A07 — Counter and IntentTrace carry the capture link.
// Test plan row TP-074-15. Scenario status="planned" in
// specs/074-capture-as-fallback-policy/scenario-manifest.json. Stub
// keeps the traceability guard satisfied until the integration row
// is wired against the live metrics/trace surface.

package assistant_integration

import "testing"

func TestCaptureFallback_IntentTraceJoinCarriesCaptureLink_TP_074_15(t *testing.T) {
	t.Skip("planned: SCN-074-A07 / TP-074-15 — see specs/074-capture-as-fallback-policy/scenario-manifest.json (status=planned)")
}
