package provenance

import (
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func counterValue(t *testing.T, scenario string, cause contracts.ProvenanceCause) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := ViolationsCounter.WithLabelValues(scenario, string(cause)).Write(m); err != nil {
		t.Fatalf("counter Write: %v", err)
	}
	return m.GetCounter().GetValue()
}

// TestEnforce_BS007 is the BS-007 unit proof: a requires-provenance
// scenario whose response has a non-empty body and an empty Sources
// slice MUST be rewritten to the canonical refusal AND increment the
// violations counter.
func TestEnforce_BS007(t *testing.T) {
	// Not t.Parallel: shares the package-level ViolationsCounter.

	const scenario = "retrieval_qa_test_bs007"
	before := counterValue(t, scenario, contracts.ProvenanceCauseMissingArtifact)

	resp := contracts.AssistantResponse{
		Body:    "this answer was synthesized without any citations",
		Status:  contracts.StatusThinking,
		Sources: nil,
	}
	got := Enforce(true, scenario, contracts.ProvenanceCauseMissingArtifact, resp)

	if got.Body != CanonicalRefusalBody {
		t.Fatalf("Body = %q; want canonical refusal %q", got.Body, CanonicalRefusalBody)
	}
	if got.Status != contracts.StatusSavedAsIdea {
		t.Fatalf("Status = %q; want %q", got.Status, contracts.StatusSavedAsIdea)
	}
	if !got.CaptureRoute {
		t.Fatalf("CaptureRoute = false; want true")
	}
	if len(got.Sources) != 0 {
		t.Fatalf("Sources len = %d; want 0", len(got.Sources))
	}

	after := counterValue(t, scenario, contracts.ProvenanceCauseMissingArtifact)
	if after-before != 1 {
		t.Fatalf("ViolationsCounter delta = %v; want 1", after-before)
	}
}

func TestEnforce_PassthroughWithSources(t *testing.T) {
	const scenario = "retrieval_qa_test_passthrough_with_sources"
	before := counterValue(t, scenario, contracts.ProvenanceCauseMissingArtifact)

	resp := contracts.AssistantResponse{
		Body: "real answer",
		Sources: []contracts.Source{
			{ID: "a1", Title: "Note A", Kind: contracts.SourceArtifact},
		},
		Status: contracts.StatusThinking,
	}
	got := Enforce(true, scenario, contracts.ProvenanceCauseMissingArtifact, resp)

	if got.Body != "real answer" {
		t.Fatalf("Body mutated: %q", got.Body)
	}
	if got.Status != contracts.StatusThinking {
		t.Fatalf("Status mutated: %q", got.Status)
	}
	if got.CaptureRoute {
		t.Fatalf("CaptureRoute mutated to true")
	}
	if len(got.Sources) != 1 {
		t.Fatalf("Sources mutated: len=%d", len(got.Sources))
	}

	after := counterValue(t, scenario, contracts.ProvenanceCauseMissingArtifact)
	if after != before {
		t.Fatalf("ViolationsCounter incremented on passthrough: delta=%v", after-before)
	}
}

func TestEnforce_PassthroughWhenNotRequired(t *testing.T) {
	const scenario = "notification_schedule_test_not_required"
	before := counterValue(t, scenario, contracts.ProvenanceCauseFabricatedSource)

	resp := contracts.AssistantResponse{
		Body:    "scheduled reminder confirmed",
		Status:  contracts.StatusReminderConfirmed,
		Sources: nil,
	}
	got := Enforce(false, scenario, contracts.ProvenanceCauseFabricatedSource, resp)

	if got.Body != "scheduled reminder confirmed" {
		t.Fatalf("Body mutated when requiresProvenance=false: %q", got.Body)
	}
	if got.Status != contracts.StatusReminderConfirmed {
		t.Fatalf("Status mutated when requiresProvenance=false")
	}
	if got.CaptureRoute {
		t.Fatalf("CaptureRoute mutated when requiresProvenance=false")
	}

	after := counterValue(t, scenario, contracts.ProvenanceCauseFabricatedSource)
	if after != before {
		t.Fatalf("counter incremented when requiresProvenance=false")
	}
}

func TestEnforce_EmptyBodyEmptySourcesIsNotAViolation(t *testing.T) {
	const scenario = "weather_query_test_empty_empty"
	before := counterValue(t, scenario, contracts.ProvenanceCauseFabricatedSource)

	resp := contracts.AssistantResponse{
		Body:    "",
		Status:  contracts.StatusUnavailable,
		Sources: nil,
	}
	got := Enforce(true, scenario, contracts.ProvenanceCauseFabricatedSource, resp)

	if got.Body != "" {
		t.Fatalf("Body should remain empty: got %q", got.Body)
	}
	if got.Status != contracts.StatusUnavailable {
		t.Fatalf("Status mutated on empty-empty: %q", got.Status)
	}
	if got.CaptureRoute {
		t.Fatalf("CaptureRoute should not be set by gate on empty-empty (facade owns)")
	}

	after := counterValue(t, scenario, contracts.ProvenanceCauseFabricatedSource)
	if after != before {
		t.Fatalf("counter incremented on empty-empty (should be facade-owned, not a gate violation)")
	}
}

// TestEnforce_UnknownScenarioLabelIsBounded proves the gate uses a
// stable label for unknown scenarios so cardinality is bounded.
func TestEnforce_UnknownScenarioLabelIsBounded(t *testing.T) {
	before := counterValue(t, "unknown", contracts.ProvenanceCauseFabricatedSource)

	resp := contracts.AssistantResponse{Body: "x"}
	_ = Enforce(true, "", "", resp)

	after := counterValue(t, "unknown", contracts.ProvenanceCauseFabricatedSource)
	if after-before != 1 {
		t.Fatalf("unknown-label counter delta = %v; want 1", after-before)
	}
}

// TestEnforce_AdversarialBypass — adversarial regression: if a future
// refactor "optimizes" Enforce by short-circuiting on a non-nil
// Sources slice header (regardless of length), the gate would let an
// empty-but-non-nil Sources slip through. This test fails if that
// regression ships.
func TestEnforce_AdversarialBypass(t *testing.T) {
	const scenario = "retrieval_qa_test_adversarial"
	before := counterValue(t, scenario, contracts.ProvenanceCauseMissingArtifact)

	// Non-nil but empty Sources slice.
	resp := contracts.AssistantResponse{
		Body:    "answer with allocated-but-empty Sources",
		Sources: []contracts.Source{},
	}
	got := Enforce(true, scenario, contracts.ProvenanceCauseMissingArtifact, resp)

	if got.Body != CanonicalRefusalBody {
		t.Fatalf("BYPASS DETECTED: Enforce treated empty-but-allocated Sources as sourced. Body=%q", got.Body)
	}
	if !strings.Contains(got.Body, "sourced answer") {
		t.Fatalf("expected canonical refusal text: %q", got.Body)
	}
	after := counterValue(t, scenario, contracts.ProvenanceCauseMissingArtifact)
	if after-before != 1 {
		t.Fatalf("counter not incremented on empty-but-allocated Sources")
	}
}

// TestEnforce_CauseLabelDifferentiatesIncrements proves the gate
// records each cause as a separate counter series so dashboards can
// distinguish graph-drift (missing_artifact / lookup_error) from
// fabrication (fabricated_source) and SST misconfiguration
// (dropped_for_quota). Adversarial: if a future refactor collapsed
// the cause label, scenario-wide totals would still increment but
// per-cause counters would all stay at baseline (or move in lockstep).
func TestEnforce_CauseLabelDifferentiatesIncrements(t *testing.T) {
	const scenario = "retrieval_qa_test_cause_differentiation"
	causes := []contracts.ProvenanceCause{
		contracts.ProvenanceCauseMissingArtifact,
		contracts.ProvenanceCauseLookupError,
		contracts.ProvenanceCauseFabricatedSource,
		contracts.ProvenanceCauseDroppedForQuota,
	}
	befores := make(map[contracts.ProvenanceCause]float64, len(causes))
	for _, c := range causes {
		befores[c] = counterValue(t, scenario, c)
	}

	// Fire missing_artifact twice, the rest once each.
	for _, c := range []contracts.ProvenanceCause{
		contracts.ProvenanceCauseMissingArtifact,
		contracts.ProvenanceCauseMissingArtifact,
		contracts.ProvenanceCauseLookupError,
		contracts.ProvenanceCauseFabricatedSource,
		contracts.ProvenanceCauseDroppedForQuota,
	} {
		resp := contracts.AssistantResponse{Body: "unsourced body for cause " + string(c)}
		_ = Enforce(true, scenario, c, resp)
	}

	wantDeltas := map[contracts.ProvenanceCause]float64{
		contracts.ProvenanceCauseMissingArtifact:  2,
		contracts.ProvenanceCauseLookupError:      1,
		contracts.ProvenanceCauseFabricatedSource: 1,
		contracts.ProvenanceCauseDroppedForQuota:  1,
	}
	for c, want := range wantDeltas {
		got := counterValue(t, scenario, c) - befores[c]
		if got != want {
			t.Fatalf("cause=%q: delta=%v; want %v (per-cause label drift?)", c, got, want)
		}
	}
}

// TestEnforce_EmptyCauseDefaultsToFabricatedSource proves the
// fallback path: when the upstream assembler did not classify, the
// gate attributes the rewrite to fabricated_source so the counter
// always has a non-empty cause label (closed-vocabulary contract).
func TestEnforce_EmptyCauseDefaultsToFabricatedSource(t *testing.T) {
	const scenario = "retrieval_qa_test_empty_cause_default"
	before := counterValue(t, scenario, contracts.ProvenanceCauseFabricatedSource)

	resp := contracts.AssistantResponse{Body: "unsourced body with no upstream classification"}
	_ = Enforce(true, scenario, "", resp)

	after := counterValue(t, scenario, contracts.ProvenanceCauseFabricatedSource)
	if after-before != 1 {
		t.Fatalf("empty-cause default did not route to fabricated_source: delta=%v", after-before)
	}
}
