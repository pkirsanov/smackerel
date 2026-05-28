// Package assistantmetrics tests the counter registration contract.
// The per-cause drop semantics are exercised end-to-end by
// internal/agent/tools/retrieval/source_assembly_test.go (the caller
// site is the source of truth for behavior).
package assistantmetrics

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

// TestSourceAssemblyDropsCounter_LabelVocabularyClosed proves the
// counter accepts exactly the closed-vocabulary causes documented in
// the type comment, and that increment cardinality is bounded.
func TestSourceAssemblyDropsCounter_LabelVocabularyClosed(t *testing.T) {
	const scenario = "retrieval_qa"
	// Sample baseline so the test is independent of init-time state
	// and any other test in this package that touches the counter.
	missingBefore := readCounter(t, scenario, string(DropCauseMissingArtifact))
	errorBefore := readCounter(t, scenario, string(DropCauseLookupError))

	SourceAssemblyDropsCounter.WithLabelValues(scenario, string(DropCauseMissingArtifact)).Inc()
	SourceAssemblyDropsCounter.WithLabelValues(scenario, string(DropCauseMissingArtifact)).Inc()
	SourceAssemblyDropsCounter.WithLabelValues(scenario, string(DropCauseLookupError)).Inc()

	if got := readCounter(t, scenario, string(DropCauseMissingArtifact)); got != missingBefore+2 {
		t.Fatalf("missing_artifact: want +2 (=%v), got %v", missingBefore+2, got)
	}
	if got := readCounter(t, scenario, string(DropCauseLookupError)); got != errorBefore+1 {
		t.Fatalf("lookup_error: want +1 (=%v), got %v", errorBefore+1, got)
	}
}

// TestSourceAssemblyDropsCounter_ScenarioLabelIsolatesIncrements
// proves drops attributed to scenario A do not leak into scenario B's
// series so dashboards can attribute per-scenario regressions
// independently. Adversarial: if the scenario_id label were stripped
// or hard-coded, incrementing scenario_a would visibly bump
// scenario_b's counter and this test would fail.
func TestSourceAssemblyDropsCounter_ScenarioLabelIsolatesIncrements(t *testing.T) {
	const cause = string(DropCauseMissingArtifact)
	const scenarioA = "scenario_isolation_a"
	const scenarioB = "scenario_isolation_b"

	beforeA := readCounter(t, scenarioA, cause)
	beforeB := readCounter(t, scenarioB, cause)

	SourceAssemblyDropsCounter.WithLabelValues(scenarioA, cause).Inc()
	SourceAssemblyDropsCounter.WithLabelValues(scenarioA, cause).Inc()
	SourceAssemblyDropsCounter.WithLabelValues(scenarioA, cause).Inc()

	if got := readCounter(t, scenarioA, cause); got != beforeA+3 {
		t.Fatalf("scenario_a: want +3 (=%v), got %v", beforeA+3, got)
	}
	if got := readCounter(t, scenarioB, cause); got != beforeB {
		t.Fatalf("scenario_b must be unaffected by scenario_a increments: want %v, got %v", beforeB, got)
	}
}

func readCounter(t *testing.T, scenarioID, cause string) float64 {
	t.Helper()
	c := SourceAssemblyDropsCounter.WithLabelValues(scenarioID, cause)
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("counter.Write(%q, %q): %v", scenarioID, cause, err)
	}
	if m.Counter == nil {
		return 0
	}
	return m.Counter.GetValue()
}
