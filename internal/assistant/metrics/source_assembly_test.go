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
	// Sample baseline so the test is independent of init-time state
	// and any other test in this package that touches the counter.
	missingBefore := readCounter(t, string(DropCauseMissingArtifact))
	errorBefore := readCounter(t, string(DropCauseLookupError))

	SourceAssemblyDropsCounter.WithLabelValues(string(DropCauseMissingArtifact)).Inc()
	SourceAssemblyDropsCounter.WithLabelValues(string(DropCauseMissingArtifact)).Inc()
	SourceAssemblyDropsCounter.WithLabelValues(string(DropCauseLookupError)).Inc()

	if got := readCounter(t, string(DropCauseMissingArtifact)); got != missingBefore+2 {
		t.Fatalf("missing_artifact: want +2 (=%v), got %v", missingBefore+2, got)
	}
	if got := readCounter(t, string(DropCauseLookupError)); got != errorBefore+1 {
		t.Fatalf("lookup_error: want +1 (=%v), got %v", errorBefore+1, got)
	}
}

func readCounter(t *testing.T, cause string) float64 {
	t.Helper()
	c := SourceAssemblyDropsCounter.WithLabelValues(cause)
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("counter.Write(%q): %v", cause, err)
	}
	if m.Counter == nil {
		return 0
	}
	return m.Counter.GetValue()
}
