//go:build e2e

package e2e

import (
	"testing"
	"time"
)

func TestEquivalentNormalizedEventsClassifySameAcrossDifferentSources(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	shared := map[string]any{"title": "checkout-api outage", "body": "checkout-api outage failed", "severity": "high", "subject": "checkout-api", "service": "checkout-api", "domain": "ops", "intent": "outage", "delivery_metadata": map[string]string{"actor": "e2e"}}

	firstPayload := cloneNotificationPayload(shared)
	firstPayload["source_type"] = "manual_fixture"
	firstPayload["source_instance_id"] = prefix + "-manual-source"
	first := notificationManualIngest(t, cfg, firstPayload)

	secondPayload := cloneNotificationPayload(shared)
	secondPayload["source_type"] = "webhook_fixture"
	secondPayload["source_instance_id"] = prefix + "-webhook-source"
	second := notificationManualIngest(t, cfg, secondPayload)

	firstDetail := notificationEventDetail(t, cfg, first.NotificationID)
	secondDetail := notificationEventDetail(t, cfg, second.NotificationID)
	if firstDetail.Classification == nil || secondDetail.Classification == nil {
		t.Fatalf("classification missing for source-agnostic comparison: first=%+v second=%+v", firstDetail.Classification, secondDetail.Classification)
	}
	if firstDetail.Classification.Severity != secondDetail.Classification.Severity || firstDetail.Classification.Domain != secondDetail.Classification.Domain || firstDetail.Classification.Intent != secondDetail.Classification.Intent {
		t.Fatalf("equivalent normalized events classified differently across sources: first=%+v second=%+v", firstDetail.Classification, secondDetail.Classification)
	}
	if firstDetail.Classification.Confidence == 0 || secondDetail.Classification.Confidence == 0 {
		t.Fatalf("classification confidence was not computed by the live pipeline: first=%+v second=%+v", firstDetail.Classification, secondDetail.Classification)
	}
}

func cloneNotificationPayload(values map[string]any) map[string]any {
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
