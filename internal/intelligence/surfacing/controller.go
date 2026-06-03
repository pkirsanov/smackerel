package surfacing

import (
	"context"
	"fmt"
)

// MetricsSink is the minimal Prometheus surface the controller needs.
// internal/metrics provides the concrete adapter; keeping the interface
// in this package avoids an import cycle with internal/metrics and
// keeps unit tests self-contained.
type MetricsSink interface {
	IncDelivered(producer Producer, channel Channel)
	IncDeduped(producer Producer)
	IncSuppressed(reason string)
	IncBudgetOverride(reason string)
	IncDeferredBudgetExhausted(producer Producer)
	SetBudgetRemaining(remaining int)
}

// noopMetrics is the default sink — useful in tests and in any wiring
// path where metrics registration has not yet happened.
type noopMetrics struct{}

func (noopMetrics) IncDelivered(Producer, Channel)      {}
func (noopMetrics) IncDeduped(Producer)                 {}
func (noopMetrics) IncSuppressed(string)                {}
func (noopMetrics) IncBudgetOverride(string)            {}
func (noopMetrics) IncDeferredBudgetExhausted(Producer) {}
func (noopMetrics) SetBudgetRemaining(int)              {}

// Config carries the SST-resolved knobs the controller depends on. All
// fields are required; the constructor returns an error if any is zero.
type Config struct {
	DailyNudgeBudget        int
	SuppressionWindowHours  int
	DedupeWindowHours       int
	UrgentEscalationEnabled bool
}

// Controller is the single decision point producers consult before
// dispatching. Construct exactly one per process; share across all
// producers so the budget, dedupe, and suppression state is unified.
type Controller struct {
	budget      *BudgetTracker
	dedupe      *DedupeIndex
	suppression *SuppressionWindow
	urgent      bool
	metrics     MetricsSink
}

// NewController constructs a controller from a validated Config and an
// AckLookup. ackLookup MAY be nil — suppression then short-circuits to
// false, which is the correct behavior when no acknowledgement signal
// is wired (e.g., tests, or pre-spec-027 deployments).
func NewController(cfg Config, ackLookup AckLookup, metrics MetricsSink) (*Controller, error) {
	if cfg.DailyNudgeBudget <= 0 {
		return nil, fmt.Errorf("surfacing controller: daily_nudge_budget must be > 0 (SST surfacing.daily_nudge_budget)")
	}
	if cfg.SuppressionWindowHours <= 0 {
		return nil, fmt.Errorf("surfacing controller: suppression_window_hours must be > 0 (SST surfacing.suppression_window_hours)")
	}
	if cfg.DedupeWindowHours <= 0 {
		return nil, fmt.Errorf("surfacing controller: dedupe_window_hours must be > 0 (SST surfacing.dedupe_window_hours)")
	}
	if metrics == nil {
		metrics = noopMetrics{}
	}
	c := &Controller{
		budget:      NewBudgetTracker(cfg.DailyNudgeBudget),
		dedupe:      NewDedupeIndex(cfg.DedupeWindowHours),
		suppression: NewSuppressionWindow(cfg.SuppressionWindowHours, ackLookup),
		urgent:      cfg.UrgentEscalationEnabled,
		metrics:     metrics,
	}
	c.metrics.SetBudgetRemaining(c.budget.Remaining())
	return c, nil
}

// Propose runs the full pipeline: normalize → dedupe → suppress → budget
// → escalate. On Permit / Escalated, the controller records the
// delivery in the dedupe index so subsequent candidates with the same
// ContentKey collapse.
func (c *Controller) Propose(ctx context.Context, cand SurfacingCandidate) (SurfacingDecision, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return SurfacingDecision{}, err
		}
	}

	// 1. Dedupe — same ContentKey delivered within window collapses.
	if c.dedupe.IsDuplicate(cand.ContentKey) {
		c.metrics.IncDeduped(cand.Producer)
		return SurfacingDecision{Kind: DecisionDeduped, Reason: "duplicate_content_key"}, nil
	}

	// 2. Suppress — acknowledged item suppresses follow-up nudges.
	if c.suppression.IsSuppressed(cand.ContentKey) {
		c.metrics.IncSuppressed("acknowledged-by-user")
		return SurfacingDecision{Kind: DecisionSuppressed, Reason: "acknowledged-by-user"}, nil
	}

	// 3. Budget — non-urgent items hold when daily budget is exhausted.
	if c.budget.TryConsume() {
		c.dedupe.Record(cand.ContentKey)
		c.metrics.IncDelivered(cand.Producer, cand.Channel)
		c.metrics.SetBudgetRemaining(c.budget.Remaining())
		return SurfacingDecision{Kind: DecisionPermit, Reason: "within_budget"}, nil
	}

	// 4. Escalate — priority 1 + timeCritical may bypass exhausted budget
	//    when SST has urgent_escalation_enabled=true. Per-channel safety
	//    nets are enforced UPSTREAM (e.g., alerts.GetPendingAlerts caps
	//    Telegram at 2/day), so the controller only owns the global
	//    cross-channel ceiling here.
	if c.urgent && cand.Priority == 1 && cand.TimeCritical {
		c.budget.RecordOverride()
		c.dedupe.Record(cand.ContentKey)
		c.metrics.IncBudgetOverride("urgent_escalation")
		c.metrics.IncDelivered(cand.Producer, cand.Channel)
		c.metrics.SetBudgetRemaining(c.budget.Remaining())
		return SurfacingDecision{Kind: DecisionEscalated, Reason: "urgent_escalation"}, nil
	}

	c.metrics.IncDeferredBudgetExhausted(cand.Producer)
	c.metrics.SetBudgetRemaining(c.budget.Remaining())
	return SurfacingDecision{Kind: DecisionDeferredBudgetExhausted, Reason: "daily_budget_exhausted"}, nil
}
