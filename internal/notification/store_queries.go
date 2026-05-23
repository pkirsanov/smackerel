package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type EventDetail struct {
	Notification   NormalizedNotification
	RawEvent       RawEventRecord
	Classification *Classification
	Decision       *ProcessingDecision
	Incident       *Incident
}

type StatusSummary struct {
	SourceCount       int `json:"source_count"`
	OpenIncidentCount int `json:"open_incident_count"`
	PendingApprovals  int `json:"pending_approvals"`
	QueuedDeliveries  int `json:"queued_deliveries"`
}

func (s *Store) ListNotifications(ctx context.Context, limit int) ([]NormalizedNotification, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list notifications: positive limit is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, raw_event_id, source_instance_id, source_type, source_form, source_event_id,
       observed_at, event_timestamp, title, title_derivation, body, body_hash, severity,
       COALESCE(source_severity,''), tags, subject, COALESCE(service,''), domain, intent,
       canonical_key, raw_payload_ref, delivery_metadata, source_specific_ref, redaction_state,
       normalization_status, normalization_errors, payload_hash, created_at
FROM normalized_notifications
ORDER BY observed_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func (s *Store) GetEventDetail(ctx context.Context, id string) (EventDetail, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, raw_event_id, source_instance_id, source_type, source_form, source_event_id,
       observed_at, event_timestamp, title, title_derivation, body, body_hash, severity,
       COALESCE(source_severity,''), tags, subject, COALESCE(service,''), domain, intent,
       canonical_key, raw_payload_ref, delivery_metadata, source_specific_ref, redaction_state,
       normalization_status, normalization_errors, payload_hash, created_at
FROM normalized_notifications
WHERE id = $1 OR raw_event_id = $1
LIMIT 1`, id)
	if err != nil {
		return EventDetail{}, fmt.Errorf("get notification detail: %w", err)
	}
	notifications, err := scanNotifications(rows)
	rows.Close()
	if err != nil {
		return EventDetail{}, err
	}
	if len(notifications) == 0 {
		return EventDetail{}, pgx.ErrNoRows
	}
	detail := EventDetail{Notification: notifications[0]}
	if raw, err := s.getRawEvent(ctx, detail.Notification.RawEventID); err == nil {
		detail.RawEvent = raw
	}
	if classification, err := s.getLatestClassification(ctx, detail.Notification.ID); err == nil {
		detail.Classification = &classification
	}
	if decision, err := s.getLatestDecision(ctx, detail.Notification.ID); err == nil {
		detail.Decision = &decision
		if decision.IncidentID != "" {
			if incident, ierr := s.GetIncident(ctx, decision.IncidentID); ierr == nil {
				detail.Incident = &incident
			}
		}
	}
	return detail, nil
}

func (s *Store) ListIncidents(ctx context.Context, limit int) ([]Incident, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list incidents: positive limit is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, incident_key, status, title, subject, COALESCE(service,''), severity, domain, intent, risk_level,
       first_event_at, last_event_at, persistence_count, source_instance_ids, state_reason, redaction_state, created_at, updated_at, resolved_at
FROM notification_incidents
ORDER BY last_event_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()
	return scanIncidents(rows)
}

func (s *Store) GetIncident(ctx context.Context, id string) (Incident, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, incident_key, status, title, subject, COALESCE(service,''), severity, domain, intent, risk_level,
       first_event_at, last_event_at, persistence_count, source_instance_ids, state_reason, redaction_state, created_at, updated_at, resolved_at
FROM notification_incidents
WHERE id = $1
LIMIT 1`, id)
	if err != nil {
		return Incident{}, fmt.Errorf("get incident: %w", err)
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

func (s *Store) ListSuppressions(ctx context.Context, limit int) ([]Suppression, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, COALESCE(notification_id,''), COALESCE(incident_id,''), COALESCE(source_instance_id,''), suppression_kind, scope, reason, starts_at, expires_at, created_at
FROM notification_suppressions
ORDER BY created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list suppressions: %w", err)
	}
	defer rows.Close()
	return scanSuppressions(rows)
}

func (s *Store) ListDeliveries(ctx context.Context, limit int) ([]DeliveryAttempt, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, decision_id, COALESCE(incident_id,''), COALESCE(approval_request_id,''), channel, destination_ref,
       payload_hash, redaction_state, status, COALESCE(error_kind,''), COALESCE(error_redacted,''), attempted_at, completed_at
FROM notification_delivery_attempts
ORDER BY attempted_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification deliveries: %w", err)
	}
	defer rows.Close()
	deliveries := []DeliveryAttempt{}
	for rows.Next() {
		var delivery DeliveryAttempt
		var redactionJSON []byte
		if err := rows.Scan(&delivery.ID, &delivery.DecisionID, &delivery.IncidentID, &delivery.ApprovalRequestID, &delivery.Channel, &delivery.DestinationRef, &delivery.PayloadHash, &redactionJSON, &delivery.Status, &delivery.ErrorKind, &delivery.ErrorRedacted, &delivery.AttemptedAt, &delivery.CompletedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(redactionJSON, &delivery.RedactionState)
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) StatusSummary(ctx context.Context) (StatusSummary, error) {
	var summary StatusSummary
	queries := []struct {
		dest  *int
		query string
	}{
		{&summary.SourceCount, "SELECT COUNT(*) FROM notification_source_instances"},
		{&summary.OpenIncidentCount, "SELECT COUNT(*) FROM notification_incidents WHERE resolved_at IS NULL"},
		{&summary.PendingApprovals, "SELECT COUNT(*) FROM notification_approval_requests WHERE status = 'pending'"},
		{&summary.QueuedDeliveries, "SELECT COUNT(*) FROM notification_delivery_attempts WHERE status = 'queued'"},
	}
	for _, item := range queries {
		if err := s.pool.QueryRow(ctx, item.query).Scan(item.dest); err != nil {
			return StatusSummary{}, fmt.Errorf("notification status summary: %w", err)
		}
	}
	return summary, nil
}

func (s *Store) getRawEvent(ctx context.Context, id string) (RawEventRecord, error) {
	var raw RawEventRecord
	var form string
	var kind string
	var loopGuardKey *string
	var sourceJSON, deliveryJSON, redactionJSON []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, source_instance_id, source_type, source_form, source_event_id, source_event_id_origin,
       observed_at, event_timestamp, payload_hash, raw_payload_kind, raw_payload_bytes, payload_size_bytes,
       source_specific_fields, delivery_metadata, redaction_state, validation_status, loop_guard_key, created_at
FROM notification_raw_events WHERE id = $1`, id).Scan(&raw.ID, &raw.SourceInstanceID, &raw.SourceType, &form, &raw.SourceEventID, &raw.SourceEventIDOrigin, &raw.ObservedAt, &raw.EventTimestamp, &raw.PayloadHash, &kind, &raw.RawPayload, &raw.PayloadSizeBytes, &sourceJSON, &deliveryJSON, &redactionJSON, &raw.ValidationStatus, &loopGuardKey, &raw.CreatedAt)
	if err != nil {
		return RawEventRecord{}, err
	}
	raw.SourceForm = SourceForm(form)
	raw.RawPayloadKind = RawPayloadKind(kind)
	if loopGuardKey != nil {
		raw.LoopGuardKey = *loopGuardKey
	}
	_ = json.Unmarshal(sourceJSON, &raw.SourceSpecific)
	_ = json.Unmarshal(deliveryJSON, &raw.DeliveryMetadata)
	_ = json.Unmarshal(redactionJSON, &raw.RedactionState)
	return raw, nil
}

func (s *Store) getLatestClassification(ctx context.Context, notificationID string) (Classification, error) {
	var classification Classification
	var severity, domain, intent string
	var signalsJSON, uncertaintyJSON []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, notification_id, severity, domain, intent, confidence, source_severity_policy, signals, rationale, uncertainty, classifier_version, created_at
FROM notification_classifications WHERE notification_id = $1 ORDER BY created_at DESC LIMIT 1`, notificationID).Scan(&classification.ID, &classification.NotificationID, &severity, &domain, &intent, &classification.Confidence, &classification.SourceSeverityPolicy, &signalsJSON, &classification.Rationale, &uncertaintyJSON, &classification.ClassifierVersion, &classification.CreatedAt)
	if err != nil {
		return Classification{}, err
	}
	classification.Severity = Severity(severity)
	classification.Domain = Domain(domain)
	classification.Intent = Intent(intent)
	_ = json.Unmarshal(signalsJSON, &classification.Signals)
	_ = json.Unmarshal(uncertaintyJSON, &classification.Uncertainty)
	return classification, nil
}

func (s *Store) getLatestDecision(ctx context.Context, notificationID string) (ProcessingDecision, error) {
	var decision ProcessingDecision
	var decisionType string
	var thresholdJSON, riskJSON []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, COALESCE(notification_id,''), COALESCE(incident_id,''), decision_type, reason_codes, threshold_inputs, risk_assessment, rationale, created_at
FROM notification_processing_decisions WHERE notification_id = $1 ORDER BY created_at DESC LIMIT 1`, notificationID).Scan(&decision.ID, &decision.NotificationID, &decision.IncidentID, &decisionType, &decision.ReasonCodes, &thresholdJSON, &riskJSON, &decision.Rationale, &decision.CreatedAt)
	if err != nil {
		return ProcessingDecision{}, err
	}
	decision.DecisionType = DecisionType(decisionType)
	_ = json.Unmarshal(thresholdJSON, &decision.ThresholdInputs)
	_ = json.Unmarshal(riskJSON, &decision.RiskAssessment)
	return decision, nil
}

func scanNotifications(rows pgx.Rows) ([]NormalizedNotification, error) {
	notifications := []NormalizedNotification{}
	for rows.Next() {
		var n NormalizedNotification
		var form, severity, domain, intent string
		var titleJSON, tagsJSON, deliveryJSON, sourceJSON, redactionJSON, errorsJSON []byte
		if err := rows.Scan(&n.ID, &n.RawEventID, &n.SourceInstanceID, &n.SourceType, &form, &n.SourceEventID, &n.ObservedAt, &n.EventTimestamp, &n.Title, &titleJSON, &n.Body, &n.BodyHash, &severity, &n.SourceSeverity, &tagsJSON, &n.Subject, &n.Service, &domain, &intent, &n.CanonicalKey, &n.RawPayloadRef, &deliveryJSON, &sourceJSON, &redactionJSON, &n.NormalizationState, &errorsJSON, &n.PayloadHash, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.SourceForm = SourceForm(form)
		n.Severity = Severity(severity)
		n.Domain = Domain(domain)
		n.Intent = Intent(intent)
		_ = json.Unmarshal(titleJSON, &n.TitleDerivation)
		_ = json.Unmarshal(tagsJSON, &n.Tags)
		_ = json.Unmarshal(deliveryJSON, &n.DeliveryMetadata)
		_ = json.Unmarshal(sourceJSON, &n.SourceSpecificRef)
		_ = json.Unmarshal(redactionJSON, &n.RedactionState)
		_ = json.Unmarshal(errorsJSON, &n.NormalizationErrors)
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func DefaultDecisionPolicy() DecisionPolicy {
	return DecisionPolicy{PersistenceThreshold: 2, EscalationSeverity: SeverityHigh, LowConfidenceThreshold: 0.55, OutputChannels: []string{"dashboard"}, MaxRetries: 2}
}

func NewDefaultService(store *Store) (*Service, error) {
	engine, err := NewDecisionEngine(DefaultDecisionPolicy())
	if err != nil {
		return nil, err
	}
	return NewService(store, engine), nil
}

func (s *Store) EnsureSourceInstance(ctx context.Context, cfg SourceInstanceConfig, now time.Time) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("notification source store: postgres pool is required")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return fmt.Errorf("notification source store: timestamp is required")
	}
	metadataJSON, err := json.Marshal(cfg.RedactedMetadata)
	if err != nil {
		return fmt.Errorf("marshal source redacted metadata: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_source_instances (
    source_instance_id, source_type, source_form, enabled, config_hash,
    secret_ref_names, redacted_metadata, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (source_instance_id) DO NOTHING`,
		cfg.SourceInstanceID,
		cfg.SourceType,
		cfg.SourceForm,
		*cfg.Enabled,
		cfg.ConfigHash,
		cfg.SecretRefNames,
		metadataJSON,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("ensure notification source instance %q: %w", cfg.SourceInstanceID, err)
	}
	return nil
}

func FixedNow() time.Time {
	return time.Now().UTC()
}
