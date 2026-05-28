// Spec 061 SCOPE-09 — capability-layer Prometheus metrics inventory.
//
// This file declares 8 of the 10 metric series listed in design.md §8.1.
// The remaining 2 series live in their own owning packages because
// the gate-style counters belong with the gate code:
//
//   - smackerel_assistant_provenance_violations_total
//     (internal/assistant/provenance/gate.go)
//   - smackerel_assistant_source_assembly_drops_total
//     (internal/assistant/metrics/source_assembly.go)
//
// Together those make 10 series with bounded cardinality on every
// label. The labels are CLOSED vocabularies enforced by labels.go
// (sibling). A drift past those constants is a test failure.
//
// Source of truth for the label set and metric type:
// specs/061-conversational-assistant/design.md §8.1.
//
// Cardinality safety: every label below resolves to a finite set
// (transport ∈ {telegram, fake, …}, outcome ∈ closed enum, etc.).
// We never label by user_id, scenario_id is bounded by the
// manifest (~10 entries in v1), and there is no per-message tag.
//
// Naming convention: every series name is prefixed
// `smackerel_assistant_` so a single Prometheus relabel can isolate
// the whole capability layer from one scrape target.
package assistantmetrics

import "github.com/prometheus/client_golang/prometheus"

// ----- Closed-vocabulary label values -----
//
// These constants are the ONLY accepted label values for the
// counters/histograms below. The labels_test.go file proves the
// vocabularies are bounded and the WithLabelValues sites in the
// facade/confirm/disambig dispatch paths use these constants.

// Transport label values. Cardinality bound: # transports ever wired.
const (
	TransportTelegram = "telegram"
	TransportFake     = "fake" // tests / eval harness
)

// Facade-level outcome vocabulary (smackerel_assistant_facade_turns_total).
const (
	OutcomeAnswered  = "answered"  // BandHigh dispatch produced a user-visible answer
	OutcomeCaptured  = "captured"  // CaptureRoute=true (BandLow / unresolvable reference / error)
	OutcomeProposed  = "proposed"  // confirm-card emitted (awaiting user input)
	OutcomeConfirmed = "confirmed" // confirm-card was confirmed
	OutcomeDiscarded = "discarded" // confirm-card was discarded (user or timeout) OR /reset
	OutcomeError     = "error"     // facade returned a non-nil error to the adapter
)

// Router band label values (smackerel_assistant_router_band_total).
const (
	BandHigh       = "high"
	BandBorderline = "borderline"
	BandLow        = "low"
)

// Skill invocation outcome vocabulary (smackerel_assistant_skill_invocations_total).
// Mirrors the spec 037 Outcome enum so cross-layer dashboards align.
const (
	SkillOutcomeOK                   = "ok"
	SkillOutcomeTimeout              = "timeout"
	SkillOutcomeProviderError        = "provider_error"
	SkillOutcomeSchemaFailure        = "schema_failure"
	SkillOutcomeToolReturnInvalid    = "tool_return_invalid"
	SkillOutcomeInputSchemaViolation = "input_schema_violation"
	SkillOutcomeLoopLimit            = "loop_limit"
	SkillOutcomeUnknownIntent        = "unknown_intent"
)

// Capture-fallback cause vocabulary (smackerel_assistant_capture_fallback_total).
// Per design.md §8.1 row 5.
const (
	CauseLowConfidence         = "low_confidence"
	CauseBorderlineTimeout     = "borderline_timeout"
	CauseConfirmDiscarded      = "confirm_discarded"
	CauseConfirmTimeout        = "confirm_timeout"
	CauseErrorOfferedCapture   = "error_offered_capture"
	CauseUnresolvableReference = "unresolvable_reference"
)

// Confirm-card outcome vocabulary (smackerel_assistant_confirm_card_outcomes_total).
const (
	ConfirmOutcomeConfirmed        = "confirmed"
	ConfirmOutcomeDiscardedUser    = "discarded_user"
	ConfirmOutcomeDiscardedTimeout = "discarded_timeout"
)

// Disambiguation outcome vocabulary (smackerel_assistant_disambiguation_outcomes_total).
const (
	DisambigOutcomeResolvedUser             = "resolved_user"
	DisambigOutcomeResolvedTimeoutCapture   = "resolved_timeout_capture"
	DisambigOutcomeResolvedNonMatchingReply = "resolved_non_matching_reply_capture"
)

// ----- Metric series -----

// FacadeTurnsTotal counts facade-level turn outcomes.
// Labels: transport ∈ closed transport vocabulary; outcome ∈ closed
// Outcome* vocabulary above.
var FacadeTurnsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_facade_turns_total",
		Help: "Facade-level turn count. outcome ∈ {answered,captured,proposed,confirmed,discarded,error} (spec 061 design §8.1).",
	},
	[]string{"transport", "outcome"},
)

// FacadeLatencySeconds observes facade enter → response emit latency.
// Buckets cover the spec 037/061 G026 manifest budget (<= 5s p95) plus
// slow-path / error-path tail (<= 30s).
var FacadeLatencySeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_assistant_facade_latency_seconds",
		Help:    "Facade enter → response emit latency in seconds (spec 061 design §8.1).",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	},
	[]string{"transport", "outcome"},
)

// RouterBandTotal counts the three-band post-processor classifications.
// Labels: band ∈ {high,borderline,low}; transport ∈ closed vocabulary.
var RouterBandTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_router_band_total",
		Help: "Three-band routing post-processor decision count (spec 061 design §3.2, §8.1).",
	},
	[]string{"band", "transport"},
)

// SkillInvocationsTotal counts per-scenario executor outcomes.
// scenario_id is bounded by the manifest (~10 entries v1).
var SkillInvocationsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_skill_invocations_total",
		Help: "Per-scenario executor outcome counts (maps to spec 037 InvocationResult.Outcome) (spec 061 design §8.1).",
	},
	[]string{"scenario_id", "outcome", "transport"},
)

// CaptureFallbackTotal counts capture-as-fallback events by cause.
var CaptureFallbackTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_capture_fallback_total",
		Help: "Capture-as-fallback event count by cause (spec 061 design §8.1).",
	},
	[]string{"cause", "transport"},
)

// ConfirmCardOutcomesTotal counts terminal confirm-card outcomes.
var ConfirmCardOutcomesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_confirm_card_outcomes_total",
		Help: "Confirm-card terminal outcome count. outcome ∈ {confirmed,discarded_user,discarded_timeout} (spec 061 design §5.4, §8.1).",
	},
	[]string{"scenario_id", "outcome", "transport"},
)

// DisambiguationOutcomesTotal counts terminal disambiguation outcomes.
var DisambiguationOutcomesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_disambiguation_outcomes_total",
		Help: "Disambiguation prompt terminal outcome count. outcome ∈ {resolved_user,resolved_timeout_capture,resolved_non_matching_reply_capture} (spec 061 design §3.2, §8.1).",
	},
	[]string{"outcome", "transport"},
)

// ActiveThreadsGauge tracks active assistant_conversations rows per
// transport. Designed to be Set() by a periodic refresh (see context
// store). Cardinality bound: # transports.
var ActiveThreadsGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "smackerel_assistant_active_threads",
		Help: "Active assistant_conversations rows per transport (spec 061 design §8.1).",
	},
	[]string{"transport"},
)

func init() {
	prometheus.MustRegister(
		FacadeTurnsTotal,
		FacadeLatencySeconds,
		RouterBandTotal,
		SkillInvocationsTotal,
		CaptureFallbackTotal,
		ConfirmCardOutcomesTotal,
		DisambiguationOutcomesTotal,
		ActiveThreadsGauge,
	)
}

// AllTransports is the complete closed vocabulary for the `transport`
// label across every assistant metric. labels_test.go asserts no
// emission site uses a value outside this set.
var AllTransports = []string{TransportTelegram, TransportFake}

// AllFacadeOutcomes is the complete closed vocabulary for the
// `outcome` label on FacadeTurnsTotal + FacadeLatencySeconds.
var AllFacadeOutcomes = []string{
	OutcomeAnswered,
	OutcomeCaptured,
	OutcomeProposed,
	OutcomeConfirmed,
	OutcomeDiscarded,
	OutcomeError,
}

// AllBands is the complete closed vocabulary for the `band` label on
// RouterBandTotal.
var AllBands = []string{BandHigh, BandBorderline, BandLow}

// AllSkillOutcomes is the complete closed vocabulary for the
// `outcome` label on SkillInvocationsTotal.
var AllSkillOutcomes = []string{
	SkillOutcomeOK,
	SkillOutcomeTimeout,
	SkillOutcomeProviderError,
	SkillOutcomeSchemaFailure,
	SkillOutcomeToolReturnInvalid,
	SkillOutcomeInputSchemaViolation,
	SkillOutcomeLoopLimit,
	SkillOutcomeUnknownIntent,
}

// AllCaptureFallbackCauses is the complete closed vocabulary for the
// `cause` label on CaptureFallbackTotal.
var AllCaptureFallbackCauses = []string{
	CauseLowConfidence,
	CauseBorderlineTimeout,
	CauseConfirmDiscarded,
	CauseConfirmTimeout,
	CauseErrorOfferedCapture,
	CauseUnresolvableReference,
}

// AllConfirmCardOutcomes is the complete closed vocabulary for the
// `outcome` label on ConfirmCardOutcomesTotal.
var AllConfirmCardOutcomes = []string{
	ConfirmOutcomeConfirmed,
	ConfirmOutcomeDiscardedUser,
	ConfirmOutcomeDiscardedTimeout,
}

// AllDisambigOutcomes is the complete closed vocabulary for the
// `outcome` label on DisambiguationOutcomesTotal.
var AllDisambigOutcomes = []string{
	DisambigOutcomeResolvedUser,
	DisambigOutcomeResolvedTimeoutCapture,
	DisambigOutcomeResolvedNonMatchingReply,
}
