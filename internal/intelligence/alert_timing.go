// Spec 021 R-021-002/003/004 / BUG-021-006 — LLM-driven alert-timing judgment.
//
// Replaces the hardcoded alert-timing windows in the bill, trip-prep, and
// return-window alert producers (`> 3 days`, `INTERVAL '5 days'`) with a
// per-situation LLM decision, in line with docs/smackerel.md §3.6: "when should
// I alert the user?" is domain reasoning, not a fixed threshold. The Go core
// retrieves upcoming events within a generous OPERATIONAL lookahead horizon and
// their signals; the `alert_timing_evaluate` scenario decides whether NOW is
// the right time to surface each reminder.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/metrics"
)

// AlertKind enumerates the timing-judged alert producers.
type AlertKind string

const (
	AlertKindBill         AlertKind = "bill"
	AlertKindTripPrep     AlertKind = "trip_prep"
	AlertKindReturnWindow AlertKind = "return_window"
)

// AlertTimingCandidate carries the deterministic signals for one upcoming
// event. These are pure retrieved data — NO business threshold is applied in
// Go; the LLM judges whether NOW is a good time to alert.
type AlertTimingCandidate struct {
	// ArtifactID is the row id the alert attaches to. Internal-only — it MUST
	// NOT leak into the LLM prompt (json:"-").
	ArtifactID string `json:"-"`
	// AlertType is the intelligence AlertType to create when surfaced.
	AlertType AlertType `json:"-"`
	// Priority is the alert priority to create when surfaced.
	Priority int `json:"-"`

	AlertKind      AlertKind `json:"alert_kind"`
	Subject        string    `json:"subject"`
	DaysUntilEvent int       `json:"days_until_event"`
	Detail         string    `json:"detail"`
}

// AlertTimingDecision is the validated output_schema of the
// `alert_timing_evaluate` scenario.
type AlertTimingDecision struct {
	ShouldAlert bool    `json:"should_alert"`
	Confidence  float64 `json:"confidence"`
	Rationale   string  `json:"rationale,omitempty"`
}

// AlertTimingEvaluator judges whether NOW is a good time to surface a reminder
// for an upcoming event. The production implementation routes to the LLM via
// the agent bridge; tests inject a scripted evaluator.
type AlertTimingEvaluator interface {
	EvaluateAlertTiming(ctx context.Context, candidate AlertTimingCandidate) (AlertTimingDecision, error)
}

// AlertTimingConfig bundles the LLM evaluator with the OPERATIONAL bounds that
// govern the timing-judged producers. The bounds are SST-resolved (fail-loud)
// operator knobs — candidate lookahead horizon, throughput cap, and the
// decision-confidence safety gate — NOT business thresholds. The "alert now?"
// JUDGMENT itself is the evaluator's (LLM) responsibility.
type AlertTimingConfig struct {
	Evaluator       AlertTimingEvaluator
	LookaheadDays   int
	MaxCandidates   int
	ConfidenceFloor float64
}

// alertTimingShouldSurface applies the OPERATIONAL confidence-floor safety gate
// to the LLM's judgment: surface only when the model says alert-now AND is at
// least `floor` confident (Product Principle 6 — invisible by default).
func alertTimingShouldSurface(decision AlertTimingDecision, floor float64) bool {
	return decision.ShouldAlert && decision.Confidence >= floor
}

// evaluateAndCreateTimedAlert runs the LLM timing judgment for one candidate
// and creates the alert when the model says alert-now with sufficient
// confidence. Returns true when an alert was created. The title is composed by
// the caller (per-kind wording); the LLM rationale becomes the body when
// present. Shared by the bill / trip-prep / return-window producers.
func (e *Engine) evaluateAndCreateTimedAlert(ctx context.Context, c AlertTimingCandidate, title, fallbackBody string) bool {
	decision, err := e.alertTiming.Evaluator.EvaluateAlertTiming(ctx, c)
	if err != nil {
		slog.Warn("alert timing evaluation failed", "kind", c.AlertKind, "subject", c.Subject, "error", err)
		metrics.AlertProducerFailures.WithLabelValues(string(c.AlertType)).Inc()
		return false
	}
	if !alertTimingShouldSurface(decision, e.alertTiming.ConfidenceFloor) {
		return false
	}
	body := decision.Rationale
	if body == "" {
		body = fallbackBody
	}
	if err := e.CreateAlert(ctx, &Alert{
		AlertType:  c.AlertType,
		Title:      title,
		Body:       body,
		Priority:   c.Priority,
		ArtifactID: c.ArtifactID,
	}); err != nil {
		slog.Warn("failed to create timed alert", "kind", c.AlertKind, "subject", c.Subject, "error", err)
		metrics.AlertProducerFailures.WithLabelValues(string(c.AlertType)).Inc()
		return false
	}
	metrics.AlertsProduced.WithLabelValues(string(c.AlertType)).Inc()
	return true
}

// BridgeAlertTimingEvaluator is the production AlertTimingEvaluator, backed by
// the `alert_timing_evaluate` scenario via the agent bridge. The
// marshal/invoke/validate/decode transport is the shared agent.InvokeJudgment
// primitive (BUG-021-010).
type BridgeAlertTimingEvaluator struct {
	Runner agent.JudgmentRunner
}

// EvaluateAlertTiming invokes the `alert_timing_evaluate` scenario for one
// candidate and returns the LLM's structured judgment.
func (b *BridgeAlertTimingEvaluator) EvaluateAlertTiming(ctx context.Context, candidate AlertTimingCandidate) (AlertTimingDecision, error) {
	if b == nil {
		return AlertTimingDecision{}, agent.ErrJudgmentUnavailable
	}
	return agent.InvokeJudgment[AlertTimingDecision](ctx, b.Runner, "scheduler", "alert_timing_evaluate", candidate)
}

func init() {
	// The agent loader (spec 037) requires every scenario to declare at least
	// one registered allowed_tools entry. The alert_timing_evaluate scenario is
	// a pure single-turn judgment with no real tool; this no-op satisfies the
	// contract. The system prompt forbids the LLM from calling it. This init()
	// runs because cmd/core imports the intelligence package.
	agent.RegisterTool(agent.Tool{
		Name:        "noop_alert_timing",
		Description: "Spec 021 BUG-021-006 — no-op tool registered solely to satisfy the agent loader's allowed_tools contract for the alert_timing_evaluate scenario. MUST NOT be invoked by the LLM; the scenario system prompt forbids it.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "properties": {},
            "description": "noop_alert_timing takes no arguments and must never be invoked."
        }`),
		OutputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "required": ["rejected"],
            "properties": {
                "rejected": { "type": "boolean", "const": true }
            }
        }`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "internal/intelligence",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("noop_alert_timing must not be invoked; the alert_timing_evaluate scenario judges in a single LLM turn")
		},
	})
}
