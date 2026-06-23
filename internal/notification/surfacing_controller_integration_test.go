//go:build integration

// Package notification: spec 054 Scope 9 (Surfacing Controller Integration)
// integration coverage. These realize SCN-054-028 and SCN-054-030 by driving
// the real Service.Process pipeline against an ephemeral Postgres stack with
// the real shared spec 078 surfacing controller + ack registry wired in (no
// in-process stub). Run via `./smackerel.sh test integration`.
package notification

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// surfacingIntegrationEnvelope builds a source envelope whose incident identity
// (domain+service+intent) is controlled by the caller so distinct subjects map
// to distinct IncidentKeys. Severity and intent flow from the mapping hints
// (the normalizer reads them directly), making the resulting Priority /
// TimeCritical deterministic.
func surfacingIntegrationEnvelope(cfg SourceInstanceConfig, id, subject, severity, intent string) SourceEventEnvelope {
	return SourceEventEnvelope{
		SourceType:           cfg.SourceType,
		SourceInstanceID:     cfg.SourceInstanceID,
		SourceForm:           cfg.SourceForm,
		SourceEventID:        id,
		ObservedAt:           time.Now().UTC(),
		RawPayloadKind:       RawPayloadKindText,
		RawPayload:           []byte(subject + " " + severity + " " + intent),
		DeliveryMetadata:     map[string]string{"actor": "integration"},
		SourceSpecificFields: map[string]string{"severity": severity},
		MappingHints: map[string]string{
			"title": subject, "body": subject + " surfacing event",
			"severity": severity, "subject": subject, "service": subject,
			"domain": "ops", "intent": intent,
		},
	}
}

// arbitrationKind extracts the persisted controller verdict from a decision
// record's risk_assessment (additive surfacing_arbitration key).
func arbitrationKind(t *testing.T, decision ProcessingDecision) string {
	t.Helper()
	raw, ok := decision.RiskAssessment["surfacing_arbitration"]
	if !ok {
		t.Fatalf("decision record missing surfacing_arbitration outcome: %+v", decision.RiskAssessment)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("surfacing_arbitration not a map: %T (%v)", raw, raw)
	}
	kind, _ := m["kind"].(string)
	return kind
}

// TestNonUrgentNotificationDeferredWhenGlobalBudgetExhausted covers
// SCN-054-028: with the shared global budget pre-exhausted, a non-urgent
// user-facing notification is arbitrated deferred-budget-exhausted, the outcome
// is persisted on the decision record (in memory AND round-tripped through
// Postgres JSONB), and ZERO deliveries are queued. RED before Scope 9: the
// engine ignored the controller and queued a delivery directly.
func TestNonUrgentNotificationDeferredWhenGlobalBudgetExhausted(t *testing.T) {
	store, _ := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	service := notificationIntegrationService(t, store)

	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        1,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, surfacing.NewInMemoryAck(), nil)
	if err != nil {
		t.Fatalf("controller: %v", err)
	}
	service.SetSurfacingController(ctrl)

	ctx := context.Background()
	now := time.Now().UTC()

	// Consume the single global nudge slot with an unrelated high-severity
	// (non-urgent) incident — it permits and queues a delivery.
	filler, err := service.Process(ctx, surfacingIntegrationEnvelope(cfg, prefix+"-filler", prefix+"-filler-svc", "high", "investigate"), now)
	if err != nil {
		t.Fatalf("process filler: %v", err)
	}
	if got := arbitrationKind(t, filler.Decision); got != "permit" {
		t.Fatalf("filler arbitration: want permit, got %q", got)
	}
	if filler.Delivery == nil {
		t.Fatalf("filler should have queued a delivery (budget available)")
	}

	// The global budget is now exhausted. A non-urgent (high+investigate)
	// notification for a DIFFERENT incident must be deferred with zero deliveries.
	target, err := service.Process(ctx, surfacingIntegrationEnvelope(cfg, prefix+"-target", prefix+"-target-svc", "high", "investigate"), now)
	if err != nil {
		t.Fatalf("process target: %v", err)
	}
	if got := arbitrationKind(t, target.Decision); got != "deferred-budget-exhausted" {
		t.Fatalf("target arbitration: want deferred-budget-exhausted, got %q", got)
	}
	if target.Delivery != nil {
		t.Fatalf("deferred non-urgent decision must queue ZERO deliveries, got %+v", target.Delivery)
	}

	// Persistence: the deferral must survive a Postgres JSONB round-trip on the
	// decision record (observable in the decision audit trail).
	persisted, err := store.getLatestDecision(ctx, target.Notification.ID)
	if err != nil {
		t.Fatalf("reload decision: %v", err)
	}
	if got := arbitrationKind(t, persisted); got != "deferred-budget-exhausted" {
		t.Fatalf("persisted arbitration: want deferred-budget-exhausted, got %q", got)
	}
}

// TestAcknowledgmentSuppressesSiblingAndFollowUpNotifications covers
// SCN-054-030: same-ContentKey siblings collapse to one delivery via deduped,
// an operator acknowledgment recorded on the shared registry flips a subsequent
// same-incident candidate from deferred to suppressed (acknowledged-by-user),
// and no duplicate/follow-up output is delivered. Drives the production
// Service + shared controller + shared ack registry. RED before Scope 9: there
// was no shared ack feed, so the follow-up would have re-dispatched.
func TestAcknowledgmentSuppressesSiblingAndFollowUpNotifications(t *testing.T) {
	store, _ := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	service := notificationIntegrationService(t, store)

	sharedAck := surfacing.NewInMemoryAck()
	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        1,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, sharedAck, nil)
	if err != nil {
		t.Fatalf("controller: %v", err)
	}
	service.SetSurfacingController(ctrl)
	service.SetSurfacingAck(sharedAck)

	ctx := context.Background()
	now := time.Now().UTC()

	// Part A — sibling dedupe. Incident A is permitted and recorded in the
	// dedupe index; a sibling carrying the same incident ContentKey within the
	// window collapses to deduped with no second delivery.
	a1, err := service.Process(ctx, surfacingIntegrationEnvelope(cfg, prefix+"-A-1", prefix+"-A-svc", "high", "investigate"), now)
	if err != nil {
		t.Fatalf("process A1: %v", err)
	}
	if got := arbitrationKind(t, a1.Decision); got != "permit" {
		t.Fatalf("A1 arbitration: want permit, got %q", got)
	}
	if a1.Delivery == nil {
		t.Fatalf("A1 should have queued the first delivery")
	}
	a2, err := service.Process(ctx, surfacingIntegrationEnvelope(cfg, prefix+"-A-2", prefix+"-A-svc", "high", "investigate"), now)
	if err != nil {
		t.Fatalf("process A2 sibling: %v", err)
	}
	if got := arbitrationKind(t, a2.Decision); got != "deduped" {
		t.Fatalf("A2 sibling arbitration: want deduped, got %q", got)
	}
	if a2.Delivery != nil {
		t.Fatalf("deduped sibling must NOT queue a second delivery, got %+v", a2.Delivery)
	}
	if a1.Incident.IncidentKey != a2.Incident.IncidentKey {
		t.Fatalf("sibling must share the incident ContentKey: %q vs %q", a1.Incident.IncidentKey, a2.Incident.IncidentKey)
	}

	// Part B — acknowledgment suppresses follow-up. Incident B is deferred
	// (global budget exhausted by A), so no dedupe entry is recorded. Without an
	// ack it would simply defer again; once the operator acknowledges B on any
	// surface, the same-incident follow-up is SUPPRESSED (acknowledged-by-user).
	b1, err := service.Process(ctx, surfacingIntegrationEnvelope(cfg, prefix+"-B-1", prefix+"-B-svc", "high", "investigate"), now)
	if err != nil {
		t.Fatalf("process B1: %v", err)
	}
	if got := arbitrationKind(t, b1.Decision); got != "deferred-budget-exhausted" {
		t.Fatalf("B1 arbitration (pre-ack): want deferred-budget-exhausted, got %q", got)
	}
	if b1.Delivery != nil {
		t.Fatalf("budget-exhausted B1 must queue zero deliveries, got %+v", b1.Delivery)
	}
	keyB := b1.Incident.IncidentKey

	// Operator acknowledges incident B (the production feed records on the
	// shared registry keyed by the incident correlation key).
	service.AcknowledgeIncident(keyB)

	b2, err := service.Process(ctx, surfacingIntegrationEnvelope(cfg, prefix+"-B-2", prefix+"-B-svc", "high", "investigate"), now)
	if err != nil {
		t.Fatalf("process B2 follow-up: %v", err)
	}
	if got := arbitrationKind(t, b2.Decision); got != "suppressed" {
		t.Fatalf("B2 follow-up arbitration (post-ack): want suppressed, got %q (ack feed not honored)", got)
	}
	if b2.Delivery != nil {
		t.Fatalf("acknowledged follow-up must queue zero deliveries, got %+v", b2.Delivery)
	}
	if b1.Incident.IncidentKey != b2.Incident.IncidentKey {
		t.Fatalf("follow-up must target the same acknowledged incident: %q vs %q", b1.Incident.IncidentKey, b2.Incident.IncidentKey)
	}
}
