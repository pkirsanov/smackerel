package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateRawEvent(ctx context.Context, envelope SourceEventEnvelope, now time.Time) (RawEventRecord, error) {
	if s == nil || s.pool == nil {
		return RawEventRecord{}, fmt.Errorf("notification raw event store: postgres pool is required")
	}
	if now.IsZero() {
		return RawEventRecord{}, fmt.Errorf("notification raw event store: timestamp is required")
	}
	if err := validateEnvelopeForRawEvent(envelope); err != nil {
		return RawEventRecord{}, err
	}
	sourceEventID := envelope.SourceEventID
	origin := "source"
	if sourceEventID == "" {
		derived, err := DeriveSourceEventID(envelope)
		if err != nil {
			return RawEventRecord{}, err
		}
		sourceEventID = derived
		origin = "handler_derived"
	}
	envelope.SourceEventID = sourceEventID
	payloadHash := PayloadHash(envelope.RawPayload)
	redactedDelivery, redactionState := RedactStringMap(envelope.DeliveryMetadata)
	raw := RawEventRecordFromEnvelope(envelope, "notif_raw_"+uuid.NewString(), origin, payloadHash, now)
	raw.DeliveryMetadata = redactedDelivery
	raw.RedactionState = redactionState
	raw.SourceEventID = sourceEventID
	raw.SourceEventIDOrigin = origin
	deliveryJSON, err := json.Marshal(raw.DeliveryMetadata)
	if err != nil {
		return RawEventRecord{}, fmt.Errorf("marshal raw delivery metadata: %w", err)
	}
	sourceJSON, err := json.Marshal(raw.SourceSpecific)
	if err != nil {
		return RawEventRecord{}, fmt.Errorf("marshal raw source fields: %w", err)
	}
	redactionJSON, err := json.Marshal(raw.RedactionState)
	if err != nil {
		return RawEventRecord{}, fmt.Errorf("marshal raw redaction state: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_raw_events (
    id, source_instance_id, source_type, source_form, source_event_id,
    source_event_id_origin, observed_at, event_timestamp, payload_hash,
    raw_payload_kind, raw_payload_bytes, raw_payload_text, payload_size_bytes,
    source_specific_fields, delivery_metadata, redaction_state,
    validation_status, validation_errors, loop_guard_key, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		raw.ID, raw.SourceInstanceID, raw.SourceType, raw.SourceForm, raw.SourceEventID,
		raw.SourceEventIDOrigin, raw.ObservedAt, raw.EventTimestamp, raw.PayloadHash,
		raw.RawPayloadKind, raw.RawPayload, string(raw.RawPayload), raw.PayloadSizeBytes,
		sourceJSON, deliveryJSON, redactionJSON, raw.ValidationStatus, []byte(`[]`), nullableString(raw.LoopGuardKey), raw.CreatedAt)
	if err != nil {
		return RawEventRecord{}, fmt.Errorf("insert notification raw event: %w", err)
	}
	return raw, nil
}

func (s *Store) CreateNormalizedNotification(ctx context.Context, notification NormalizedNotification) error {
	deliveryJSON, sourceJSON, redactionJSON, tagsJSON, titleJSON, errorsJSON, err := marshalNormalized(notification)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO normalized_notifications (
    id, raw_event_id, source_instance_id, source_type, source_form, source_event_id,
    observed_at, event_timestamp, title, title_derivation, body, body_hash,
    severity, source_severity, tags, subject, service, domain, intent, canonical_key,
    raw_payload_ref, delivery_metadata, source_specific_ref, redaction_state,
    normalization_status, normalization_errors, payload_hash, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)`,
		notification.ID, notification.RawEventID, notification.SourceInstanceID, notification.SourceType, notification.SourceForm, notification.SourceEventID,
		notification.ObservedAt, notification.EventTimestamp, notification.Title, titleJSON, notification.Body, notification.BodyHash,
		notification.Severity, nullableString(notification.SourceSeverity), tagsJSON, notification.Subject, nullableString(notification.Service), notification.Domain, notification.Intent, notification.CanonicalKey,
		notification.RawPayloadRef, deliveryJSON, sourceJSON, redactionJSON, notification.NormalizationState, errorsJSON, notification.PayloadHash, notification.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert normalized notification: %w", err)
	}
	return nil
}

func (s *Store) CreateClassification(ctx context.Context, classification Classification) error {
	signalsJSON, _ := json.Marshal(classification.Signals)
	uncertaintyJSON, _ := json.Marshal(classification.Uncertainty)
	_, err := s.pool.Exec(ctx, `
INSERT INTO notification_classifications (
    id, notification_id, severity, domain, intent, confidence,
    source_severity_policy, signals, rationale, uncertainty, classifier_version, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		classification.ID, classification.NotificationID, classification.Severity, classification.Domain, classification.Intent, classification.Confidence,
		classification.SourceSeverityPolicy, signalsJSON, classification.Rationale, uncertaintyJSON, classification.ClassifierVersion, classification.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert notification classification: %w", err)
	}
	return nil
}

func (s *Store) ListOpenIncidents(ctx context.Context, notification NormalizedNotification) ([]Incident, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, incident_key, status, title, subject, COALESCE(service,''), severity, domain, intent, risk_level,
       first_event_at, last_event_at, persistence_count, source_instance_ids, state_reason, redaction_state, created_at, updated_at, resolved_at
FROM notification_incidents
WHERE resolved_at IS NULL AND (subject = $1 OR service = $2)
ORDER BY last_event_at DESC`, notification.Subject, notification.Service)
	if err != nil {
		return nil, fmt.Errorf("list notification open incidents: %w", err)
	}
	defer rows.Close()
	return scanIncidents(rows)
}

func (s *Store) UpsertIncident(ctx context.Context, incident Incident, link IncidentEventLink) (Incident, error) {
	redactionJSON, _ := json.Marshal(incident.RedactionState)
	_, err := s.pool.Exec(ctx, `
INSERT INTO notification_incidents (
    id, incident_key, status, title, subject, service, severity, domain, intent, risk_level,
    first_event_at, last_event_at, persistence_count, source_instance_ids, state_reason, redaction_state, created_at, updated_at, resolved_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
ON CONFLICT (incident_key) DO UPDATE SET
    status = EXCLUDED.status,
    severity = EXCLUDED.severity,
    last_event_at = EXCLUDED.last_event_at,
    persistence_count = notification_incidents.persistence_count + 1,
    source_instance_ids = EXCLUDED.source_instance_ids,
    state_reason = EXCLUDED.state_reason,
    updated_at = EXCLUDED.updated_at`,
		incident.ID, incident.IncidentKey, incident.State, incident.Title, incident.Subject, nullableString(incident.Service), incident.Severity, incident.Domain, incident.Intent, incident.RiskLevel,
		incident.FirstEventAt, incident.LastEventAt, incident.PersistenceCount, incident.SourceInstanceIDs, incident.StateReason, redactionJSON, incident.CreatedAt, incident.UpdatedAt, incident.ResolvedAt)
	if err != nil {
		return Incident{}, fmt.Errorf("upsert notification incident: %w", err)
	}
	_, _ = s.pool.Exec(ctx, `INSERT INTO notification_incident_events (incident_id, notification_id, correlation_kind, correlation_score, rationale, created_at) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, link.IncidentID, link.NotificationID, link.CorrelationKind, link.CorrelationScore, link.Rationale, link.CreatedAt)
	stored, err := s.GetIncidentByKey(ctx, incident.IncidentKey)
	if err != nil {
		return Incident{}, err
	}
	return stored, nil
}

func (s *Store) GetIncidentByKey(ctx context.Context, key string) (Incident, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, incident_key, status, title, subject, COALESCE(service,''), severity, domain, intent, risk_level,
       first_event_at, last_event_at, persistence_count, source_instance_ids, state_reason, redaction_state, created_at, updated_at, resolved_at
FROM notification_incidents WHERE incident_key = $1`, key)
	if err != nil {
		return Incident{}, err
	}
	defer rows.Close()
	incidents, err := scanIncidents(rows)
	if err != nil {
		return Incident{}, err
	}
	if len(incidents) == 0 {
		return Incident{}, pgx.ErrNoRows
	}
	return incidents[0], nil
}

func (s *Store) FindSuppressions(ctx context.Context, notification NormalizedNotification, incident Incident) ([]Suppression, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, COALESCE(notification_id,''), COALESCE(incident_id,''), COALESCE(source_instance_id,''), suppression_kind, scope, reason, starts_at, expires_at, created_at
FROM notification_suppressions
WHERE (notification_id = $1 OR incident_id = $2) AND (expires_at IS NULL OR expires_at > $3)
ORDER BY created_at DESC`, notification.ID, incident.ID, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("find notification suppressions: %w", err)
	}
	defer rows.Close()
	return scanSuppressions(rows)
}

func (s *Store) CreateDecision(ctx context.Context, decision ProcessingDecision) error {
	thresholdJSON, _ := json.Marshal(decision.ThresholdInputs)
	riskJSON, _ := json.Marshal(decision.RiskAssessment)
	_, err := s.pool.Exec(ctx, `INSERT INTO notification_processing_decisions (id, notification_id, incident_id, decision_type, reason_codes, threshold_inputs, risk_assessment, rationale, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, decision.ID, nullableString(decision.NotificationID), nullableString(decision.IncidentID), decision.DecisionType, decision.ReasonCodes, thresholdJSON, riskJSON, decision.Rationale, decision.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert notification decision: %w", err)
	}
	return nil
}

func (s *Store) CreateDeliveryAttempt(ctx context.Context, delivery DeliveryAttempt) (DeliveryAttempt, error) {
	if delivery.ID == "" {
		delivery.ID = "notif_delivery_" + uuid.NewString()
	}
	redactionJSON, _ := json.Marshal(delivery.RedactionState)
	_, err := s.pool.Exec(ctx, `INSERT INTO notification_delivery_attempts (id, decision_id, incident_id, approval_request_id, channel, destination_ref, payload_hash, redaction_state, status, error_kind, error_redacted, attempted_at, completed_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, delivery.ID, delivery.DecisionID, nullableString(delivery.IncidentID), nullableString(delivery.ApprovalRequestID), delivery.Channel, delivery.DestinationRef, delivery.PayloadHash, redactionJSON, delivery.Status, nullableString(delivery.ErrorKind), nullableString(delivery.ErrorRedacted), delivery.AttemptedAt, delivery.CompletedAt)
	if err != nil {
		return DeliveryAttempt{}, fmt.Errorf("insert notification delivery attempt: %w", err)
	}
	return delivery, nil
}

func validateEnvelopeForRawEvent(envelope SourceEventEnvelope) error {
	if envelope.SourceType == "" || envelope.SourceInstanceID == "" {
		return fmt.Errorf("notification source event: source identity is required")
	}
	if !envelope.SourceForm.Valid() {
		return fmt.Errorf("notification source event: source form is invalid")
	}
	if envelope.ObservedAt.IsZero() {
		return fmt.Errorf("notification source event: observed_at is required")
	}
	if envelope.RawPayloadKind == "" || len(envelope.RawPayload) == 0 {
		return fmt.Errorf("notification source event: raw payload is required")
	}
	if envelope.DeliveryMetadata == nil {
		return fmt.Errorf("notification source event: delivery metadata is required")
	}
	return nil
}

func marshalNormalized(notification NormalizedNotification) ([]byte, []byte, []byte, []byte, []byte, []byte, error) {
	deliveryJSON, err := json.Marshal(notification.DeliveryMetadata)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	sourceJSON, err := json.Marshal(notification.SourceSpecificRef)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	redactionJSON, err := json.Marshal(notification.RedactionState)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	tagsJSON, err := json.Marshal(notification.Tags)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	titleJSON, err := json.Marshal(notification.TitleDerivation)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	errorsJSON, err := json.Marshal(notification.NormalizationErrors)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	return deliveryJSON, sourceJSON, redactionJSON, tagsJSON, titleJSON, errorsJSON, nil
}

func scanIncidents(rows pgx.Rows) ([]Incident, error) {
	incidents := []Incident{}
	for rows.Next() {
		var incident Incident
		var state, severity, domain, intent, risk string
		var redactionJSON []byte
		if err := rows.Scan(&incident.ID, &incident.IncidentKey, &state, &incident.Title, &incident.Subject, &incident.Service, &severity, &domain, &intent, &risk, &incident.FirstEventAt, &incident.LastEventAt, &incident.PersistenceCount, &incident.SourceInstanceIDs, &incident.StateReason, &redactionJSON, &incident.CreatedAt, &incident.UpdatedAt, &incident.ResolvedAt); err != nil {
			return nil, err
		}
		incident.State = IncidentState(state)
		incident.Severity = Severity(severity)
		incident.Domain = Domain(domain)
		incident.Intent = Intent(intent)
		incident.RiskLevel = RiskLevel(risk)
		_ = json.Unmarshal(redactionJSON, &incident.RedactionState)
		incidents = append(incidents, incident)
	}
	return incidents, rows.Err()
}

func scanSuppressions(rows pgx.Rows) ([]Suppression, error) {
	suppressions := []Suppression{}
	for rows.Next() {
		var suppression Suppression
		var scopeJSON []byte
		if err := rows.Scan(&suppression.ID, &suppression.NotificationID, &suppression.IncidentID, &suppression.SourceInstanceID, &suppression.Kind, &scopeJSON, &suppression.Reason, &suppression.StartsAt, &suppression.ExpiresAt, &suppression.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scopeJSON, &suppression.Scope)
		suppressions = append(suppressions, suppression)
	}
	return suppressions, rows.Err()
}
