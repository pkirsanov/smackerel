package notification

import (
	"testing"
	"time"
)

func TestDerivedSourceEventIDIsStableAndExplained(t *testing.T) {
	observedAt := time.Date(2026, 5, 22, 7, 0, 0, 0, time.UTC)
	envelope := SourceEventEnvelope{
		SourceType:           "queue_fixture",
		SourceInstanceID:     "queue-instance-a",
		SourceForm:           SourceFormQueue,
		ObservedAt:           observedAt,
		RawPayloadKind:       RawPayloadKindJSON,
		RawPayload:           []byte(`{"title":"disk high","body":"usage 91"}`),
		DeliveryMetadata:     map[string]string{"message_id": "msg-1", "attempt": "1"},
		SourceSpecificFields: map[string]string{"priority": "high"},
	}

	first, err := DeriveSourceEventID(envelope)
	if err != nil {
		t.Fatalf("derive first id: %v", err)
	}
	second, err := DeriveSourceEventID(envelope)
	if err != nil {
		t.Fatalf("derive second id: %v", err)
	}
	if first != second {
		t.Fatalf("derived event id is not stable: %q != %q", first, second)
	}
	if first == "" {
		t.Fatal("derived event id is empty")
	}

	envelope.DeliveryMetadata["attempt"] = "2"
	changed, err := DeriveSourceEventID(envelope)
	if err != nil {
		t.Fatalf("derive changed id: %v", err)
	}
	if changed == first {
		t.Fatal("delivery metadata did not influence derived source event id")
	}
}
