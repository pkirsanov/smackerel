//go:build e2e

// Spec 074 — Capture acknowledgement parity across transports (planned).
//
// SCN-074-A11 — Acknowledgement shape is identical across transports.
// Test plan row TP-074-17. Scenario status="planned" in
// specs/074-capture-as-fallback-policy/scenario-manifest.json. Stub
// keeps the traceability guard satisfied until the cross-transport
// e2e is wired (Telegram, HTTP, WhatsApp, web, iOS, Android).

package assistant_e2e

import "testing"

func TestAssistantE2E_CaptureAcknowledgementIsCrossTransportIdentical_TP_074_17(t *testing.T) {
	t.Skip("planned: SCN-074-A11 / TP-074-17 — see specs/074-capture-as-fallback-policy/scenario-manifest.json (status=planned)")
}
