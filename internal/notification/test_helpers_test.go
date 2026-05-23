package notification

import "time"

func testNormalizedNotification(id string, severity Severity, domain Domain, intent Intent) NormalizedNotification {
	observedAt := time.Date(2026, 5, 22, 7, 0, 0, 0, time.UTC)
	return NormalizedNotification{
		ID:                id,
		RawEventID:        "raw-" + id,
		SourceType:        "webhook_fixture",
		SourceInstanceID:  "source-a",
		SourceForm:        SourceFormWebhook,
		SourceEventID:     "event-" + id,
		ObservedAt:        observedAt,
		Title:             "checkout event",
		Body:              "checkout-api needs investigation",
		Severity:          severity,
		Tags:              map[string][]string{"source": {"checkout"}, "handler": {"notification"}},
		Subject:           "checkout-api",
		Service:           "checkout-api",
		Domain:            domain,
		Intent:            intent,
		PayloadHash:       "payload-" + id,
		CanonicalKey:      "checkout-api:investigate",
		RawPayloadRef:     "raw-" + id,
		DeliveryMetadata:  map[string]string{"request_id": "req-" + id},
		SourceSpecificRef: map[string]any{"vendor_field": "preserved"},
		RedactionState:    map[string]any{"status": "redacted"},
	}
}
