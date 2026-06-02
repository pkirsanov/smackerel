//go:build e2e

// Spec 074 — Capture-as-Fallback Provenance (planned).
//
// SCN-074-A02 — Explicit capture is provenance-distinct.
// Test plan row TP-074-07. The scenario is marked status="planned" in
// specs/074-capture-as-fallback-policy/scenario-manifest.json; this
// stub exists so the traceability guard can map the planned linkedTests
// entry to an on-disk file. The real assertions land when the planned
// e2e path is implemented.

package assistant_e2e

import "testing"

func TestAssistantHTTPE2E_CaptureProvenanceIsDistinct_TP_074_07(t *testing.T) {
	t.Skip("planned: SCN-074-A02 / TP-074-07 — see specs/074-capture-as-fallback-policy/scenario-manifest.json (status=planned)")
}
