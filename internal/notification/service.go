package notification

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/metrics"
)

const (
	approvalRequestTTL = 24 * time.Hour
	loopGuardWindow    = 15 * time.Minute
	// outputChannelDashboard is the operator-console output channel the
	// decision engine dispatches user-facing notifications to. It maps to the
	// bounded surfacing web_push budget slot (see mapSurfacingChannel).
	outputChannelDashboard = "dashboard"
)

type Service struct {
	Store      *Store
	Normalizer Normalizer
	Classifier Classifier
	Decider    DecisionEngine

	// surfacingController is the shared spec 078 surfacing controller. When
	// non-nil, every user-facing (RequiresOutput) decision is arbitrated
	// through Controller.Propose before a delivery is queued, so notification
	// nudges honor the SAME global interruption budget, cross-channel dedupe,
	// and acknowledgment suppression as the scheduler producers. When nil
	// (legacy SST-free deployments / tests) the engine falls back to direct
	// dispatch — the explicit, documented rollback seam (mirrors
	// scheduler.proposeSurfacing). Spec 054 Scope 9.
	surfacingController surfacingProposer
	// surfacingAck is the shared acknowledgment registry the controller's
	// SuppressionWindow consults. Operator incident acknowledgments are
	// recorded here via AcknowledgeIncident so same-incident follow-up nudges
	// are suppressed across every surface.
	surfacingAck surfacingAcknowledger
}

// surfacingProposer is the minimal seam the notification service consumes from
// the shared spec 078 surfacing controller (*surfacing.Controller satisfies
// it). A consumer-side interface keeps production honest while letting unit
// tests substitute a spy to assert the SurfacingCandidate contract.
type surfacingProposer interface {
	Propose(ctx context.Context, cand surfacing.SurfacingCandidate) (surfacing.SurfacingDecision, error)
}

// surfacingAcknowledger records cross-surface acknowledgments so follow-up
// nudges for the same incident are suppressed. *surfacing.InMemoryAck
// satisfies it.
type surfacingAcknowledger interface {
	Acknowledge(correlationKey string)
}

func NewService(store *Store, decider DecisionEngine) *Service {
	return &Service{Store: store, Normalizer: NewNormalizer(), Classifier: NewClassifier("notification-rules-v1"), Decider: decider}
}

// SetSurfacingController wires the shared spec 078 surfacing controller so
// user-facing decisions route through Controller.Propose before dispatch.
// Mirrors scheduler.SetSurfacingController. Call exactly once during startup.
// A nil controller leaves the legacy direct-dispatch rollback path active.
func (s *Service) SetSurfacingController(c *surfacing.Controller) {
	if c == nil {
		s.surfacingController = nil
		return
	}
	s.surfacingController = c
}

// SetSurfacingAck wires the shared acknowledgment registry the surfacing
// controller consults for suppression. Pass the SAME *surfacing.InMemoryAck
// instance given to the controller so an operator ack on one surface suppresses
// sibling/follow-up nudges on every surface.
func (s *Service) SetSurfacingAck(a *surfacing.InMemoryAck) {
	if a == nil {
		s.surfacingAck = nil
		return
	}
	s.surfacingAck = a
}

// AcknowledgeIncident records an operator acknowledgment of an incident on the
// shared surfacing ack registry, keyed by the incident correlation key
// (incident.IncidentKey) — the SAME ContentKey the dispatch seam proposes. A
// subsequent same-incident candidate within the suppression window is then
// suppressed (acknowledged-by-user). No-op when no ack registry is wired
// (legacy rollback path) or the key is empty.
func (s *Service) AcknowledgeIncident(correlationKey string) {
	if s == nil || s.surfacingAck == nil || correlationKey == "" {
		return
	}
	s.surfacingAck.Acknowledge(correlationKey)
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

	// Spec 054 Scope 9 — arbitrate user-facing output through the shared
	// surfacing controller BEFORE persisting the decision so the arbitration
	// verdict lands on the decision row (risk_assessment JSONB) for audit. A
	// nil controller is the legacy direct-dispatch fallback: permit, no verdict
	// attached, behavior byte-identical to pre-Scope-9.
	surfacingPermit := true
	surfacingWired := s.surfacingController != nil
	var surfacingVerdict surfacing.SurfacingDecision
	if decision.RequiresOutput {
		cand, candErr := surfacingCandidateFor(incident, now)
		if candErr != nil {
			return PipelineResult{RawEvent: raw, Notification: normalized, Classification: classification, Incident: incident, Decision: decisionRecord}, candErr
		}
		surfacingVerdict, surfacingPermit = s.proposeSurfacing(ctx, cand)
		if surfacingWired {
			decisionRecord.RiskAssessment = attachSurfacingArbitration(decisionRecord.RiskAssessment, surfacingVerdict)
		}
	}

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
	if decision.RequiresOutput && surfacingPermit {
		payloadHash := PayloadHash([]byte(decision.Rationale))
		loopKey := LoopOrigin{DecisionID: decisionRecord.ID, OutputChannel: outputChannelDashboard, PayloadHash: payloadHash, EmittedAt: now}.Key()
		redaction := map[string]any{"status": "redacted", "loop_guard_key": loopKey}
		if surfacingWired {
			redaction["arbitration_outcome"] = string(surfacingVerdict.Kind)
		}
		delivery := DeliveryAttempt{DecisionID: decisionRecord.ID, IncidentID: incident.ID, ApprovalRequestID: approvalID, Channel: outputChannelDashboard, DestinationRef: "operator", PayloadHash: payloadHash, RedactionState: redaction, Status: "queued", AttemptedAt: now}
		created, err := s.Store.CreateDeliveryAttempt(ctx, delivery)
		if err != nil {
			return result, err
		}
		result.Delivery = &created
	}
	return result, nil
}

// surfacingCandidateFor builds the SurfacingCandidate for a user-facing
// notification decision. The incident correlation key is the cross-channel
// dedupe + ack-suppression identity (ContentKey); Priority and TimeCritical are
// derived from the incident severity/intent so urgent incidents may escalate
// past an exhausted global budget. Returns an error (no silent default) when
// the output channel has no bounded surfacing.Channel mapping.
func surfacingCandidateFor(incident Incident, now time.Time) (surfacing.SurfacingCandidate, error) {
	channel, err := mapSurfacingChannel(outputChannelDashboard)
	if err != nil {
		return surfacing.SurfacingCandidate{}, err
	}
	return surfacing.SurfacingCandidate{
		Producer:     surfacing.ProducerNotification,
		Channel:      channel,
		ContentKey:   incident.IncidentKey,
		Priority:     surfacingPriority(incident.Severity),
		TimeCritical: surfacingTimeCritical(incident.Severity, incident.Intent),
		ProposedAt:   now,
	}, nil
}

// mapSurfacingChannel maps a notification output channel to the bounded
// surfacing.Channel enum. The notification decision engine surfaces only to the
// operator console ("dashboard"), a web-delivered nudge surface, so it consumes
// the web_push global budget slot — operator notifications honor the SAME
// interruption budget as the other surfacing producers (the GAP-06 Principle-6
// cohesion fix). Other bounded channels belong to other producers and are NOT
// emitted by this engine. Fail-loud (no default) for any channel without an
// explicit mapping, so adding a new notification output channel is a deliberate
// edit, never a silent fallback.
func mapSurfacingChannel(channel string) (surfacing.Channel, error) {
	switch channel {
	case outputChannelDashboard:
		return surfacing.ChannelWebPush, nil
	default:
		return "", fmt.Errorf("notification surfacing: output channel %q has no bounded surfacing.Channel mapping (no default)", channel)
	}
}

// surfacingPriority maps notification severity to the controller's 1=high /
// 2=medium / 3=low priority scale (mirrors intelligence.Alert).
func surfacingPriority(sev Severity) int {
	switch sev {
	case SeverityCritical, SeverityHigh:
		return 1
	case SeverityMedium:
		return 2
	default:
		return 3
	}
}

// surfacingTimeCritical reports whether an incident is urgent enough that a
// Priority-1 candidate may escalate past an exhausted global budget. Critical
// severity, or a high-severity active-outage / mitigation incident, qualifies;
// routine high-severity escalations do not.
func surfacingTimeCritical(sev Severity, intent Intent) bool {
	if sev == SeverityCritical {
		return true
	}
	if sev == SeverityHigh && (intent == IntentOutage || intent == IntentMitigation) {
		return true
	}
	return false
}

// proposeSurfacing routes a candidate through the shared surfacing controller.
// It mirrors scheduler.proposeSurfacing: a nil controller permits (legacy
// direct dispatch); a wired controller permits only on Permit/Escalated and
// holds on every other verdict. The returned SurfacingDecision is the
// controller's verdict (zero value when no controller is wired) so the caller
// can persist the arbitration outcome for audit.
func (s *Service) proposeSurfacing(ctx context.Context, cand surfacing.SurfacingCandidate) (surfacing.SurfacingDecision, bool) {
	if s.surfacingController == nil {
		return surfacing.SurfacingDecision{}, true
	}
	dec, err := s.surfacingController.Propose(ctx, cand)
	if err != nil {
		slog.Warn("notification surfacing controller error; holding user-facing output",
			"producer", cand.Producer, "channel", cand.Channel, "error", err)
		return surfacing.SurfacingDecision{Reason: "controller_error"}, false
	}
	switch dec.Kind {
	case surfacing.DecisionPermit, surfacing.DecisionEscalated:
		return dec, true
	default:
		return dec, false
	}
}

// attachSurfacingArbitration records the controller verdict on the decision
// record's risk_assessment (existing JSONB column — additive key, no migration)
// so the arbitration outcome is part of the decision audit trail even when no
// delivery is queued (deferred / suppressed / deduped).
func attachSurfacingArbitration(risk map[string]any, verdict surfacing.SurfacingDecision) map[string]any {
	if risk == nil {
		risk = map[string]any{}
	}
	risk["surfacing_arbitration"] = map[string]any{
		"kind":   string(verdict.Kind),
		"reason": verdict.Reason,
	}
	return risk
}
