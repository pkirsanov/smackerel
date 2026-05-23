//go:build e2e

package e2e

import (
	"testing"
	"time"
)

func TestMissingEnrichmentDoesNotFabricateHighConfidenceDecision(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-missing-context", "title": "unmapped threshold failed", "body": "failed threshold without known service metadata", "severity": "high", "subject": "unmapped-component", "domain": "unknown", "intent": "investigate", "delivery_metadata": map[string]string{"actor": "e2e"}})
	detail := notificationEventDetail(t, cfg, created.NotificationID)
	if detail.Classification == nil || detail.Decision == nil {
		t.Fatalf("classification or decision missing for missing-enrichment case: %+v", detail)
	}
	if detail.Classification.Confidence >= 0.55 {
		t.Fatalf("missing enrichment fabricated high confidence: %+v", detail.Classification)
	}
	if got := detail.Classification.Uncertainty["service_context"]; got != "context_unavailable" {
		t.Fatalf("missing service context was not recorded as uncertainty: %+v", detail.Classification.Uncertainty)
	}
	if detail.Decision.DecisionType == "user_escalation" || detail.Decision.DecisionType == "approval_request" || created.ApprovalID != "" {
		t.Fatalf("missing enrichment produced high-confidence action/escalation: decision=%+v approval=%s", detail.Decision, created.ApprovalID)
	}
}
