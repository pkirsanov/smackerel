// Package evergreen implements spec 095 Idea 2 — the evergreen-vs-ephemeral
// signal scored near the ingestion tier-assignment front door, plus the
// synthesis/digest pool-eligibility predicate.
//
// SCOPE-07 (this file) — the EvergreenSignal. Following the cooling.go
// precedent (docs §3.6: "domain reasoning is LLM-driven, not encoded as fixed
// thresholds in Go"), the evergreen-vs-ephemeral JUDGMENT is scenario-driven
// (the canonical path) with a deterministic categorical fallback for when the
// scenario judge is unavailable (NFR-2, Principle 9). Go holds ONLY operational
// bounds (confidence floor, per-tick budget, dedup window) as SST fail-loud
// values — never a business cutoff. The signal NEVER blocks ingestion, search,
// or retrieval (R13, Principle 9); it only weights lifecycle decay and pool
// eligibility.
//
// This package is store-free: the scenario judge is an INJECTED interface; the
// production agent-bridge implementation (BridgeEvergreenJudge, bridge.go) is
// constructed and wired in cmd/core. Importing internal/agent brings in the LLM
// bridge transport, NOT a datastore, so the Principle 5 "no parallel store"
// invariant (enforced by the routing-package architecture tests) is unaffected.
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md R10–R13, SCN-095-B01/B05
//   - specs/095-retrieval-strategy-routing/design.md §6
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-07
package evergreen

import (
	"context"
	"fmt"
	"sync/atomic"
)

// Judgment sources (mirror internal/config EvergreenJudgment* constants).
const (
	JudgmentSourceScenario    = "scenario"
	JudgmentSourceTierSignals = "tier_signals"
)

// Provenance values recorded on the signal's Source field.
const (
	provenanceScenario        = "scenario"
	provenanceTierSignalsFb   = "tier_signals_fallback"
	provenanceTierSignalsOnly = "tier_signals"
)

// EvergreenCandidate carries the deterministic signals retrieved at the
// ingestion front door for one artifact. These are pure retrieved data — NO
// business threshold is applied here; the scenario judge (or the deterministic
// fallback) decides evergreen vs ephemeral. The json tags define exactly what
// the scenario judge sees; ArtifactID is a correlation key only (json:"-") so
// it never reaches the LLM (mirrors the cooling CoolingCandidate.PersonID).
type EvergreenCandidate struct {
	ArtifactID  string `json:"-"`
	SourceKind  string `json:"source_kind"` // e.g. "telegram", "gmail", "capture", "notification"
	ContentLen  int    `json:"content_len"`
	UserStarred bool   `json:"user_starred"`
	HasContext  bool   `json:"has_context"`
}

// EvergreenDecision is the validated output of the evergreen judgment scenario.
type EvergreenDecision struct {
	IsEvergreen bool    `json:"is_evergreen"`
	Confidence  float64 `json:"confidence"`
	Rationale   string  `json:"rationale,omitempty"`
}

// EvergreenJudge judges whether a candidate is evergreen. The production
// implementation routes to the `retrieval_evergreen` scenario via the
// agent bridge (wired in cmd/core); tests inject a scripted judge. A nil judge
// (or a judge error) triggers the deterministic TierSignals fallback (NFR-2,
// Principle 9).
type EvergreenJudge interface {
	JudgeEvergreen(ctx context.Context, candidate EvergreenCandidate) (EvergreenDecision, error)
}

// EvergreenConfig bundles the judge with the OPERATIONAL bounds that govern the
// scoring. The bounds are SST-resolved fail-loud operator knobs — throughput
// cap, decision-confidence safety gate, re-judge dedup window — NOT business
// thresholds. The evergreen JUDGMENT itself is the judge's (LLM) responsibility.
type EvergreenConfig struct {
	Judge           EvergreenJudge
	JudgmentSource  string
	ConfidenceFloor float64
	PerTickBudget   int
	DedupWindowDays int
}

// EvergreenSignal is the attached judgment with full provenance (Principle 8):
// the score, the signals it was judged on, the reason, and the judgment source.
type EvergreenSignal struct {
	ArtifactID string
	// Evergreen is the final judgment after the operational confidence floor.
	Evergreen  bool
	Confidence float64
	// Reason is the human-readable rationale (scenario rationale or the
	// deterministic fallback category).
	Reason string
	// Source is the provenance of the judgment: "scenario",
	// "tier_signals_fallback" (scenario unavailable), or "tier_signals"
	// (deterministic source selected by SST).
	Source string
	// Signals are the front-door signals the judgment was made on.
	Signals []string
}

// String renders a compact attributable trace (the §14.A evergreen_scored
// token — trace/audit only, felt not heard).
func (s EvergreenSignal) String() string {
	return fmt.Sprintf("evergreen_scored id=%s evergreen=%t conf=%.2f source=%s reason=%q signals=%v",
		s.ArtifactID, s.Evergreen, s.Confidence, s.Source, s.Reason, s.Signals)
}

// Scorer computes the evergreen signal at the front door.
//
// The judge is held behind an atomic pointer so cmd/core can late-bind the
// production agent-bridge judge AFTER the bridge is constructed, WITHOUT a data
// race against the connector ingestion goroutines that the supervisor starts
// earlier in startup. Score() loads the judge atomically on each call;
// SetJudge() swaps it in once at startup. The operational bounds (cfg) are
// immutable after construction.
type Scorer struct {
	cfg   EvergreenConfig
	judge atomic.Pointer[judgeHolder]
}

// judgeHolder boxes the EvergreenJudge interface so it can live in an
// atomic.Pointer (which needs a concrete element type).
type judgeHolder struct{ judge EvergreenJudge }

// NewScorer constructs a Scorer from the SST evergreen config. When cfg.Judge
// is non-nil it seeds the atomic judge; cmd/core later upgrades it via SetJudge
// once the agent bridge is available.
func NewScorer(cfg EvergreenConfig) *Scorer {
	s := &Scorer{cfg: cfg}
	if cfg.Judge != nil {
		s.judge.Store(&judgeHolder{judge: cfg.Judge})
	}
	return s
}

// SetJudge late-binds the production evergreen judge (race-free) after the
// agent bridge is constructed in cmd/core. A nil scorer is a no-op (the
// ingestion front door simply persists no score — graceful degrade, NFR-3).
func (s *Scorer) SetJudge(j EvergreenJudge) {
	if s == nil {
		return
	}
	s.judge.Store(&judgeHolder{judge: j})
}

// currentJudge atomically loads the wired judge, or nil when none is bound.
func (s *Scorer) currentJudge() EvergreenJudge {
	if h := s.judge.Load(); h != nil {
		return h.judge
	}
	return nil
}

// Score computes the EvergreenSignal for a candidate. When judgment_source is
// "scenario" and a judge is wired, the scenario decides; the operational
// confidence floor then gates whether a low-confidence "ephemeral" call is
// trusted (a low-confidence ephemeral judgment is treated conservatively as
// evergreen — Principle 9, no wrongful exclusion). When the scenario is
// unavailable (nil judge or judge error) or SST selects "tier_signals", the
// deterministic categorical fallback is used and recorded. Score NEVER blocks
// ingestion/search (R13).
func (s *Scorer) Score(ctx context.Context, c EvergreenCandidate) EvergreenSignal {
	signals := collectSignals(c)
	judge := s.currentJudge()

	if s.cfg.JudgmentSource == JudgmentSourceScenario && judge != nil {
		decision, err := judge.JudgeEvergreen(ctx, c)
		if err == nil {
			// Operational confidence floor (NOT a business cutoff): only trust
			// an "ephemeral" call when the judge is confident enough; otherwise
			// keep it evergreen (conservative — Principle 9, no punishment).
			ephemeral := !decision.IsEvergreen && decision.Confidence >= s.cfg.ConfidenceFloor
			reason := decision.Rationale
			if reason == "" {
				reason = "scenario_judged"
			}
			return EvergreenSignal{
				ArtifactID: c.ArtifactID,
				Evergreen:  !ephemeral,
				Confidence: decision.Confidence,
				Reason:     reason,
				Source:     provenanceScenario,
				Signals:    signals,
			}
		}
		// Scenario judge errored — degrade gracefully to the deterministic
		// fallback so ingestion never blocks (NFR-2).
		sig := deterministicFallback(c, signals)
		sig.Source = provenanceTierSignalsFb
		return sig
	}

	// SST selected the deterministic tier_signals source (or no judge wired).
	sig := deterministicFallback(c, signals)
	if s.cfg.JudgmentSource == JudgmentSourceTierSignals {
		sig.Source = provenanceTierSignalsOnly
	} else {
		sig.Source = provenanceTierSignalsFb
	}
	return sig
}

// transientSourceKinds is the documented set of inherently transient ingestion
// sources the deterministic fallback treats as ephemeral. This is the fallback
// path's categorical signal (NOT a numeric business cutoff in the canonical
// scenario judgment).
var transientSourceKinds = map[string]struct{}{
	"notification": {},
	"chat_noise":   {},
	"transient":    {},
	"presence":     {},
}

// deterministicFallback is the conservative categorical judgment used when the
// scenario judge is unavailable or SST selects tier_signals. It uses
// categorical signals (transient source, starred, user context) — NOT a magic
// numeric churn cutoff — and leans evergreen when uncertain (Principle 9).
func deterministicFallback(c EvergreenCandidate, signals []string) EvergreenSignal {
	switch {
	case isTransientSource(c.SourceKind):
		return EvergreenSignal{ArtifactID: c.ArtifactID, Evergreen: false, Confidence: 0.7, Reason: "transient_source", Signals: signals}
	case c.UserStarred:
		return EvergreenSignal{ArtifactID: c.ArtifactID, Evergreen: true, Confidence: 0.8, Reason: "user_starred", Signals: signals}
	case c.HasContext:
		return EvergreenSignal{ArtifactID: c.ArtifactID, Evergreen: true, Confidence: 0.6, Reason: "user_context", Signals: signals}
	default:
		// Lean evergreen when uncertain — never wrongly exclude (Principle 9).
		return EvergreenSignal{ArtifactID: c.ArtifactID, Evergreen: true, Confidence: 0.5, Reason: "default_conservative", Signals: signals}
	}
}

// isTransientSource reports whether the source kind is inherently transient.
func isTransientSource(kind string) bool {
	_, ok := transientSourceKinds[kind]
	return ok
}

// collectSignals records the front-door signals the judgment was made on
// (Principle 8 provenance).
func collectSignals(c EvergreenCandidate) []string {
	out := []string{"source=" + c.SourceKind}
	if c.UserStarred {
		out = append(out, "user_starred")
	}
	if c.HasContext {
		out = append(out, "has_context")
	}
	out = append(out, fmt.Sprintf("content_len=%d", c.ContentLen))
	return out
}
