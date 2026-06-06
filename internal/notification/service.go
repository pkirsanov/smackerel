package notification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
)

const (
	approvalRequestTTL = 24 * time.Hour
	loopGuardWindow    = 15 * time.Minute
)

type Service struct {
	Store      *Store
	Normalizer Normalizer
	Classifier Classifier
	Decider    DecisionEngine
}

func NewService(store *Store, decider DecisionEngine) *Service {
	return &Service{Store: store, Normalizer: NewNormalizer(), Classifier: NewClassifier("notification-rules-v1"), Decider: decider}
}

func (s *Service) SubmitSourceEvent(ctx context.Context, envelope SourceEventEnvelope) (IngestReceipt, error) {
	if s == nil || s.Store == nil {
		return IngestReceipt{}, fmt.Errorf("notification service: store is required")
	}
	result, err := s.Process(ctx, envelope, time.Now().UTC())
	return result.Receipt, err
}

func (s *Service) ReportSourceHealth(ctx context.Context, report SourceHealthReport) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("notification service: store is required")
	}
	return s.Store.RecordSourceHealth(ctx, report)
}

// notificationStageDurationMs returns the elapsed time since start in
// fractional milliseconds, used to observe per-stage pipeline latency into
// metrics.NotificationProcessingDuration. Microsecond resolution keeps fast
// in-memory stages from collapsing to a zero observation.
func notificationStageDurationMs(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func (s *Service) Process(ctx context.Context, envelope SourceEventEnvelope, now time.Time) (PipelineResult, error) {
	if now.IsZero() {
		return PipelineResult{}, fmt.Errorf("notification service: timestamp is required")
	}
	pipelineStart := time.Now()
	defer func() {
		metrics.NotificationProcessingDuration.WithLabelValues("total").Observe(notificationStageDurationMs(pipelineStart))
	}()
	ingestStart := time.Now()
	raw, err := s.Store.CreateRawEvent(ctx, envelope, now)
	metrics.NotificationProcessingDuration.WithLabelValues("ingest").Observe(notificationStageDurationMs(ingestStart))
	if err != nil {
		metrics.NotificationIngestTotal.WithLabelValues(envelope.SourceType, string(envelope.SourceForm), "rejected").Inc()
		return PipelineResult{}, err
	}
	metrics.NotificationIngestTotal.WithLabelValues(envelope.SourceType, string(envelope.SourceForm), "accepted").Inc()
	normalizeStart := time.Now()
	normalized, err := s.Normalizer.Normalize(raw, envelope)
	metrics.NotificationProcessingDuration.WithLabelValues("normalize").Observe(notificationStageDurationMs(normalizeStart))
	if err != nil {
		return PipelineResult{RawEvent: raw, Receipt: IngestReceipt{SourceType: raw.SourceType, SourceInstanceID: raw.SourceInstanceID, SourceForm: raw.SourceForm, RawEventID: raw.ID, Accepted: false, Status: "normalization_failed"}}, err
	}
	if err := s.Store.CreateNormalizedNotification(ctx, normalized); err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized}, err
	}
	classification, err := s.Classifier.Classify(normalized, ClassificationContext{KnownServices: []string{normalized.Service}})
	if err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized}, err
	}
	if err := s.Store.CreateClassification(ctx, classification); err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification}, err
	}
	active, err := s.Store.ListOpenIncidents(ctx, normalized)
	if err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification}, err
	}
	correlated := NewCorrelator().Correlate(normalized, classification, active, now)
	incidentLink := IncidentEventLink{IncidentID: correlated.Incident.ID, NotificationID: normalized.ID, CorrelationKind: correlated.Correlation.Kind, CorrelationScore: correlated.Correlation.Score, Rationale: correlated.Correlation.Rationale, CreatedAt: now}
	incident, err := s.Store.UpsertIncident(ctx, correlated.Incident, incidentLink)
	if err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification, Incident: correlated.Incident}, err
	}
	suppressions, err := s.Store.FindSuppressions(ctx, normalized, incident)
	if err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification, Incident: incident}, err
	}
	for _, suppression := range suppressions {
		metrics.NotificationDedupeTotal.WithLabelValues(normalized.SourceType, suppression.Kind).Inc()
	}
	loopOrigins, err := s.Store.ListLoopOrigins(ctx, now.Add(-loopGuardWindow), 100)
	if err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification, Incident: incident}, err
	}
	if loopSuppression := NewLoopGuard(loopGuardWindow).Evaluate(envelope, loopOrigins); loopSuppression != nil {
		loopSuppression.NotificationID = normalized.ID
		loopSuppression.IncidentID = incident.ID
		createdSuppression, err := s.Store.CreateSuppression(ctx, *loopSuppression)
		if err != nil {
			return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification, Incident: incident}, err
		}
		suppressions = append(suppressions, createdSuppression)
	}
	decideStart := time.Now()
	decision := s.Decider.Decide(normalized, classification, incident, nil, suppressions)
	metrics.NotificationProcessingDuration.WithLabelValues("decide").Observe(notificationStageDurationMs(decideStart))
	decisionRecord := decision.Record()
	decisionRecord.CreatedAt = now
	if err := s.Store.CreateDecision(ctx, decisionRecord); err != nil {
		return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification, Incident: incident, Decision: decisionRecord}, err
	}
	result := PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification, Incident: incident, Decision: decisionRecord, Suppressions: suppressions, Receipt: IngestReceipt{SourceType: raw.SourceType, SourceInstanceID: raw.SourceInstanceID, SourceForm: raw.SourceForm, RawEventID: raw.ID, Accepted: true, Status: "accepted"}}
	var approvalID string
	if decision.RequiresApproval {
		approval := ApprovalRequest{ID: "notif_approval_" + strings.TrimPrefix(hashParts("approval", decisionRecord.ID, incident.ID), "sha256:"), IncidentID: incident.ID, DecisionID: decisionRecord.ID, ActionKey: "operator_approved_mitigation", TargetRef: incident.ID, RiskExplanation: "high-blast-radius notification handling requires explicit operator approval", ExpectedEffect: "operator-approved non-destructive mitigation may proceed after approval", VerificationPlan: map[string]any{"incident_id": incident.ID, "decision_id": decisionRecord.ID, "requires_operator_review": true}, ExpiresAt: now.Add(approvalRequestTTL), Status: ApprovalStatusPending, CreatedAt: now}
		createdApproval, err := s.Store.CreateApprovalRequest(ctx, approval)
		if err != nil {
			return result, err
		}
		result.Approval = &createdApproval
		approvalID = createdApproval.ID
	}
	if decision.RequiresOutput {
		payloadHash := PayloadHash([]byte(decision.Rationale))
		loopKey := LoopOrigin{DecisionID: decisionRecord.ID, OutputChannel: "dashboard", PayloadHash: payloadHash, EmittedAt: now}.Key()
		delivery := DeliveryAttempt{DecisionID: decisionRecord.ID, IncidentID: incident.ID, ApprovalRequestID: approvalID, Channel: "dashboard", DestinationRef: "operator", PayloadHash: payloadHash, RedactionState: map[string]any{"status": "redacted", "loop_guard_key": loopKey}, Status: "queued", AttemptedAt: now}
		created, err := s.Store.CreateDeliveryAttempt(ctx, delivery)
		if err != nil {
			return result, err
		}
		result.Delivery = &created
	}
	return result, nil
}
