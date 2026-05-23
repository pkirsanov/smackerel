package notification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func PayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DeriveSourceEventID(envelope SourceEventEnvelope) (string, error) {
	if strings.TrimSpace(envelope.SourceInstanceID) == "" {
		return "", fmt.Errorf("derive source event id: source instance id is required")
	}
	if !envelope.SourceForm.Valid() {
		return "", fmt.Errorf("derive source event id: valid source form is required")
	}
	if envelope.ObservedAt.IsZero() {
		return "", fmt.Errorf("derive source event id: observed_at is required")
	}
	metadata, err := canonicalJSON(envelope.DeliveryMetadata)
	if err != nil {
		return "", fmt.Errorf("derive source event id metadata: %w", err)
	}
	window := envelope.ObservedAt.UTC().Truncate(time.Minute).Format(time.RFC3339)
	return hashParts(envelope.SourceInstanceID, string(envelope.SourceForm), window, PayloadHash(envelope.RawPayload), metadata), nil
}

func RawEventRecordFromEnvelope(envelope SourceEventEnvelope, rawEventID string, sourceEventIDOrigin string, payloadHash string, createdAt time.Time) RawEventRecord {
	return RawEventRecord{
		ID:                  rawEventID,
		SourceType:          strings.TrimSpace(envelope.SourceType),
		SourceInstanceID:    strings.TrimSpace(envelope.SourceInstanceID),
		SourceForm:          envelope.SourceForm,
		SourceEventID:       strings.TrimSpace(envelope.SourceEventID),
		SourceEventIDOrigin: sourceEventIDOrigin,
		ObservedAt:          envelope.ObservedAt,
		EventTimestamp:      envelope.EventTimestamp,
		PayloadHash:         payloadHash,
		RawPayloadKind:      RawPayloadKind(envelope.RawPayloadKind),
		RawPayload:          append([]byte(nil), envelope.RawPayload...),
		PayloadSizeBytes:    len(envelope.RawPayload),
		DeliveryMetadata:    cloneMap(envelope.DeliveryMetadata),
		SourceSpecific:      cloneMap(envelope.SourceSpecificFields),
		RedactionState:      map[string]any{"status": "unscanned"},
		ValidationStatus:    "accepted",
		CreatedAt:           createdAt,
	}
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func canonicalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func cloneMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyMap(values map[string]any) map[string]any {
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
