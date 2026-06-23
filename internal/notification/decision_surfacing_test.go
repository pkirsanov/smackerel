// Package notification: spec 054 Scope 9 (Surfacing Controller Integration)
// unit coverage. These realize SCN-054-027 and SCN-054-029 against the shared
// spec 078 surfacing controller seam (internal/intelligence/surfacing). They
// exercise the production candidate-building + arbitration helpers directly so
// they need no database (category: unit).
package notification

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// spySurfacingController records the last candidate it was asked to arbitrate
// and returns a scripted verdict, so the unit test can assert the exact
// SurfacingCandidate contract the decision engine proposes. It satisfies the
// notification.surfacingProposer seam.
type spySurfacingController struct {
	calls   int
	last    surfacing.SurfacingCandidate
	verdict surfacing.SurfacingDecision
	err     error
}

func (s *spySurfacingController) Propose(_ context.Context, cand surfacing.SurfacingCandidate) (surfacing.SurfacingDecision, error) {
	s.calls++
	s.last = cand
	return s.verdict, s.err
}

// TestDecisionEngineRoutesThroughSurfacingControllerInsteadOfDirectDispatch
// covers SCN-054-027: when a controller is wired the decision engine builds a
// SurfacingCandidate (producer=notification, mapped Channel, ContentKey =
// incident.IncidentKey, severity-derived Priority, urgency TimeCritical), calls
// Controller.Propose, treats the verdict as authoritative, and permits dispatch
// only on permit/escalated. RED before Scope 9: surfacingCandidateFor /
// proposeSurfacing / ProducerNotification do not exist, and a deferred verdict
// would not have gated direct dispatch.
func TestDecisionEngineRoutesThroughSurfacingControllerInsteadOfDirectDispatch(t *testing.T) {
	spy := &spySurfacingController{verdict: surfacing.SurfacingDecision{Kind: surfacing.DecisionPermit, Reason: "within_budget"}}
	svc := &Service{}
	svc.surfacingController = spy

	incident := Incident{ID: "inc-027", IncidentKey: "incident-key-027", Severity: SeverityHigh, Intent: IntentInvestigate}
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)

	cand, err := surfacingCandidateFor(incident, now)
	if err != nil {
		t.Fatalf("surfacingCandidateFor: unexpected error: %v", err)
	}
	verdict, permit := svc.proposeSurfacing(context.Background(), cand)

	// The candidate the controller saw must carry the full contract.
	if spy.calls != 1 {
		t.Fatalf("controller.Propose call count: want 1, got %d (engine bypassed the controller)", spy.calls)
	}
	if spy.last.ContentKey != incident.IncidentKey {
		t.Errorf("candidate ContentKey: want %q (incident.IncidentKey), got %q", incident.IncidentKey, spy.last.ContentKey)
	}
	if spy.last.Producer != surfacing.ProducerNotification {
		t.Errorf("candidate Producer: want %q, got %q", surfacing.ProducerNotification, spy.last.Producer)
	}
	if spy.last.Channel != surfacing.ChannelWebPush {
		t.Errorf("candidate Channel: want %q (dashboard->web_push), got %q", surfacing.ChannelWebPush, spy.last.Channel)
	}
	if spy.last.Priority != 1 {
		t.Errorf("candidate Priority: want 1 (high), got %d", spy.last.Priority)
	}
	if spy.last.TimeCritical {
		t.Errorf("candidate TimeCritical: want false (high+investigate is not urgent), got true")
	}
	if !permit || verdict.Kind != surfacing.DecisionPermit {
		t.Fatalf("permit verdict must allow dispatch: permit=%v verdict=%+v", permit, verdict)
	}

	// Adversarial gate: a non-permit verdict (budget deferral) MUST withhold
	// dispatch. If the engine ignored the verdict and dispatched directly, this
	// would still report permit=true.
	spy.verdict = surfacing.SurfacingDecision{Kind: surfacing.DecisionDeferredBudgetExhausted, Reason: "daily_budget_exhausted"}
	deferredVerdict, deferredPermit := svc.proposeSurfacing(context.Background(), cand)
	if deferredPermit {
		t.Errorf("deferred-budget-exhausted verdict must NOT permit dispatch, got permit=true")
	}
	if deferredVerdict.Kind != surfacing.DecisionDeferredBudgetExhausted {
		t.Errorf("verdict must be authoritative: want deferred-budget-exhausted, got %q", deferredVerdict.Kind)
	}

	// Rollback seam: with NO controller wired the engine permits (legacy
	// direct dispatch) so SST-free deployments/tests keep working.
	legacy := &Service{}
	if _, legacyPermit := legacy.proposeSurfacing(context.Background(), cand); !legacyPermit {
		t.Errorf("nil controller must permit (legacy direct-dispatch fallback), got permit=false")
	}
}

// TestUrgentNotificationEscalatesPastExhaustedGlobalBudget covers SCN-054-029:
// a critical, time-critical incident yields a Priority-1 + TimeCritical
// candidate that the real controller escalates past an exhausted budget,
// permitting exactly one delivery. Uses the real (in-memory) spec 078
// controller — no DB. RED before Scope 9: the urgency signals were never
// propagated, so an exhausted budget would have wrongly deferred the urgent
// nudge.
func TestUrgentNotificationEscalatesPastExhaustedGlobalBudget(t *testing.T) {
	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        1,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, surfacing.NewInMemoryAck(), nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	// Exhaust the single budget slot with an unrelated non-urgent candidate.
	if d, err := ctrl.Propose(context.Background(), surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerDigest,
		Channel:    surfacing.ChannelDigest,
		ContentKey: "budget-filler",
		Priority:   3,
	}); err != nil || d.Kind != surfacing.DecisionPermit {
		t.Fatalf("budget warmup: want permit, got %+v err=%v", d, err)
	}

	svc := &Service{}
	svc.SetSurfacingController(ctrl)

	urgent := Incident{ID: "inc-029", IncidentKey: "ik-029a", Severity: SeverityCritical, Intent: IntentOutage}
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)

	cand, err := surfacingCandidateFor(urgent, now)
	if err != nil {
		t.Fatalf("surfacingCandidateFor: %v", err)
	}
	if cand.Priority != 1 {
		t.Fatalf("urgent candidate Priority: want 1, got %d", cand.Priority)
	}
	if !cand.TimeCritical {
		t.Fatalf("urgent candidate TimeCritical: want true (critical+outage), got false")
	}

	verdict, permit := svc.proposeSurfacing(context.Background(), cand)
	if verdict.Kind != surfacing.DecisionEscalated {
		t.Fatalf("urgent verdict: want escalated past exhausted budget, got %q (%s)", verdict.Kind, verdict.Reason)
	}
	if verdict.Reason != "urgent_escalation" {
		t.Errorf("escalation reason: want urgent_escalation, got %q", verdict.Reason)
	}
	if !permit {
		t.Errorf("escalated verdict must permit exactly one delivery, got permit=false")
	}

	// Adversarial control: a non-urgent (Priority 2) candidate against the same
	// exhausted budget MUST defer, proving the escalation is driven by the
	// urgency signals and not an unconditional bypass.
	nonUrgent := Incident{ID: "inc-029b", IncidentKey: "ik-029b", Severity: SeverityMedium, Intent: IntentInvestigate}
	calmCand, err := surfacingCandidateFor(nonUrgent, now)
	if err != nil {
		t.Fatalf("surfacingCandidateFor calm: %v", err)
	}
	if _, calmPermit := svc.proposeSurfacing(context.Background(), calmCand); calmPermit {
		t.Errorf("non-urgent candidate must defer against exhausted budget, got permit=true")
	}
}
