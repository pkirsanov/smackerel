//go:build integration

// Spec 074 — Capture-fallback dashboard metrics (planned).
//
// SCN-074-A07 — Counter and IntentTrace carry the capture link.
// Test plan row TP-074-16. Scenario status="planned" in
// specs/074-capture-as-fallback-policy/scenario-manifest.json. Stub
// keeps the traceability guard satisfied until the live dashboard
// integration row is wired.

package monitoring_integration

import "testing"

func TestCaptureFallback_DashboardMetricsCarryCaptureLink_TP_074_16(t *testing.T) {
	t.Skip("planned: SCN-074-A07 / TP-074-16 — see specs/074-capture-as-fallback-policy/scenario-manifest.json (status=planned)")
}
