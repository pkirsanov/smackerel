package notification

import (
	"testing"
	"time"
)

func TestNormalizerEmitsRequiredFieldsAndPreservesSourceSpecificContext(t *testing.T) {
	observedAt := time.Date(2026, 5, 22, 7, 5, 0, 0, time.UTC)
	envelope := SourceEventEnvelope{
		SourceType:           "webhook_fixture",
		SourceInstanceID:     "webhook-service-a",
		SourceForm:           SourceFormWebhook,
		SourceEventID:        "evt-42",
		ObservedAt:           observedAt,
		RawPayloadKind:       RawPayloadKindJSON,
		RawPayload:           []byte(`{"title":"API latency high","body":"checkout p95 over threshold"}`),
		DeliveryMetadata:     map[string]string{"request_id": "req-42"},
		SourceSpecificFields: map[string]string{"vendor_priority": "urgent", "topic": "checkout"},
		MappingHints:         map[string]string{"title": "API latency high", "body": "checkout p95 over threshold", "severity": "high", "subject": "checkout", "service": "checkout-api", "domain": "ops", "intent": "investigate", "tag": "latency"},
	}
	raw := RawEventRecordFromEnvelope(envelope, "raw-1", "source", PayloadHash(envelope.RawPayload), observedAt)

	normalized, err := NewNormalizer().Normalize(raw, envelope)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized.RawEventID != raw.ID || normalized.SourceInstanceID != envelope.SourceInstanceID || normalized.SourceEventID != envelope.SourceEventID {
		t.Fatalf("normalized source identity mismatch: %+v", normalized)
	}
	if normalized.Title != "API latency high" || normalized.Body != "checkout p95 over threshold" {
		t.Fatalf("normalized title/body mismatch: %+v", normalized)
	}
	if normalized.Severity != SeverityHigh || normalized.Domain != DomainOps || normalized.Intent != IntentInvestigate {
		t.Fatalf("normalized labels mismatch: severity=%s domain=%s intent=%s", normalized.Severity, normalized.Domain, normalized.Intent)
	}
	if normalized.SourceSpecificRef["vendor_priority"] != "urgent" {
		t.Fatalf("source-specific fields were not preserved by reference: %+v", normalized.SourceSpecificRef)
	}
	if normalized.PolicyInputContainsSourceSpecificField("vendor_priority") {
		t.Fatal("normalized policy input includes source-specific raw field")
	}
}
