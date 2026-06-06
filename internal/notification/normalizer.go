package notification

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/metrics"
)

type Normalizer struct{}

func NewNormalizer() Normalizer {
	return Normalizer{}
}

func (n Normalizer) Normalize(raw RawEventRecord, envelope SourceEventEnvelope) (result NormalizedNotification, err error) {
	defer func() {
		if err != nil {
			metrics.NotificationNormalizationErrors.WithLabelValues(raw.SourceType, normalizationErrorKind(err)).Inc()
		}
	}()
	if raw.ID == "" {
		return NormalizedNotification{}, fmt.Errorf("normalize notification: raw event id is required")
	}
	if strings.TrimSpace(raw.SourceType) == "" || strings.TrimSpace(raw.SourceInstanceID) == "" {
		return NormalizedNotification{}, fmt.Errorf("normalize notification: source identity is required")
	}
	if raw.ObservedAt.IsZero() {
		return NormalizedNotification{}, fmt.Errorf("normalize notification: observed_at is required")
	}
	title, titleDerivation := normalizedTitle(envelope)
	body := normalizedBody(envelope)
	redactedBody, bodyRedaction := RedactText(body)
	severity := ParseSeverity(firstNonEmpty(envelope.MappingHints["severity"], envelope.SourceSpecificFields["severity"]))
	subject := firstNonEmpty(envelope.MappingHints["subject"], envelope.MappingHints["service"], "notification")
	service := envelope.MappingHints["service"]
	domain := ParseDomain(envelope.MappingHints["domain"])
	intent := ParseIntent(envelope.MappingHints["intent"])
	if raw.SourceEventID == "" {
		derived, err := DeriveSourceEventID(envelope)
		if err != nil {
			return NormalizedNotification{}, err
		}
		raw.SourceEventID = derived
		raw.SourceEventIDOrigin = "handler_derived"
	}
	if raw.SourceEventIDOrigin == "" {
		raw.SourceEventIDOrigin = "source"
	}
	if raw.PayloadHash == "" {
		raw.PayloadHash = PayloadHash(envelope.RawPayload)
	}
	tags := map[string][]string{"source": {}, "handler": {"notification"}}
	if tag := strings.TrimSpace(envelope.MappingHints["tag"]); tag != "" {
		tags["source"] = append(tags["source"], tag)
	}
	now := raw.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return NormalizedNotification{
		ID:                  "notif_" + uuid.NewString(),
		RawEventID:          raw.ID,
		SourceType:          raw.SourceType,
		SourceInstanceID:    raw.SourceInstanceID,
		SourceForm:          raw.SourceForm,
		SourceEventID:       raw.SourceEventID,
		ObservedAt:          raw.ObservedAt,
		EventTimestamp:      raw.EventTimestamp,
		Title:               title,
		TitleDerivation:     titleDerivation,
		Body:                redactedBody,
		BodyHash:            hashParts(redactedBody),
		Severity:            severity,
		SourceSeverity:      envelope.SourceSpecificFields["severity"],
		Tags:                tags,
		Subject:             subject,
		Service:             service,
		Domain:              domain,
		Intent:              intent,
		CanonicalKey:        canonicalNotificationKey(domain, subject, service, intent),
		RawPayloadRef:       raw.ID,
		DeliveryMetadata:    cloneMap(raw.DeliveryMetadata),
		SourceSpecificRef:   mapStringToAny(raw.SourceSpecific),
		RedactionState:      RedactionStateMap(bodyRedaction),
		NormalizationState:  "normalized",
		NormalizationErrors: []string{},
		PayloadHash:         raw.PayloadHash,
		CreatedAt:           now,
	}, nil
}

// normalizationErrorKind maps a normalization error to a BOUNDED error_kind
// label value so metrics.NotificationNormalizationErrors never embeds a raw
// (potentially free-text) error string as label cardinality.
func normalizationErrorKind(err error) string {
	if err == nil {
		return "none"
	}
	switch msg := err.Error(); {
	case strings.Contains(msg, "raw event id"):
		return "missing_raw_event_id"
	case strings.Contains(msg, "source identity"):
		return "missing_source_identity"
	case strings.Contains(msg, "observed_at"):
		return "missing_observed_at"
	case strings.Contains(msg, "source event id") || strings.Contains(msg, "source_event_id"):
		return "source_event_id_derivation"
	default:
		return "other"
	}
}

func (n NormalizedNotification) PolicyInputContainsSourceSpecificField(field string) bool {
	if _, ok := n.SourceSpecificRef[field]; !ok {
		return false
	}
	return false
}

func normalizedTitle(envelope SourceEventEnvelope) (string, map[string]any) {
	if title := strings.TrimSpace(envelope.MappingHints["title"]); title != "" {
		return title, map[string]any{"source": "mapping_hint"}
	}
	if title := strings.TrimSpace(envelope.SourceSpecificFields["title"]); title != "" {
		return title, map[string]any{"source": "source_field_mapped"}
	}
	return "Notification for " + firstNonEmpty(envelope.MappingHints["subject"], envelope.SourceInstanceID), map[string]any{"source": "handler_derived"}
}

func normalizedBody(envelope SourceEventEnvelope) string {
	if body := strings.TrimSpace(envelope.MappingHints["body"]); body != "" {
		return body
	}
	if body := strings.TrimSpace(envelope.SourceSpecificFields["body"]); body != "" {
		return body
	}
	return strings.TrimSpace(string(envelope.RawPayload))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mapStringToAny(values map[string]string) map[string]any {
	mapped := make(map[string]any, len(values))
	for key, value := range values {
		mapped[key] = value
	}
	return mapped
}

func canonicalNotificationKey(domain Domain, subject string, service string, intent Intent) string {
	return strings.Join([]string{string(domain), firstNonEmpty(service, subject), string(intent)}, ":")
}
