// Spec 095 SCOPE-05 — structured_aggregate strategy tests.
package structuredaggregate

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// fakeAggregator computes the true max-spend month from a dataset where the
// max month is DELIBERATELY different from the most-textually-similar month —
// so the aggregate answer is genuinely different from a most-similar-chunk
// answer (no tautology).
type fakeAggregator struct {
	// monthly maps period bucket -> amount.
	monthly map[string]float64
	table   string
	gotQ    routing.AggregateQuery
}

func (f *fakeAggregator) SuperlativeSpend(_ context.Context, q routing.AggregateQuery) (routing.AggregateResult, error) {
	f.gotQ = q
	var bucket string
	var best float64
	first := true
	for b, amt := range f.monthly {
		if first || (q.Extremum == routing.ExtremumMin && amt < best) || (q.Extremum != routing.ExtremumMin && amt > best) {
			best, bucket, first = amt, b, false
		}
	}
	return routing.AggregateResult{Bucket: bucket, Amount: best, Table: f.table}, nil
}

// TestSuperlativeSpend — SCN-095-A03: the aggregate returns the exact extremum
// from the existing structured table, beating the most-similar-chunk answer.
func TestSuperlativeSpend(t *testing.T) {
	// 2026-03 has the highest spend ($450). The most-similar chunk (the one
	// mentioning "subscription") is from 2026-01 ($90) — the legacy path would
	// return that WRONG month.
	const mostSimilarChunkMonth = "2026-01"
	agg := &fakeAggregator{
		table:   "subscriptions",
		monthly: map[string]float64{"2026-01": 90, "2026-02": 120, "2026-03": 450, "2026-04": 75},
	}
	s := New(agg)

	res, err := s.Execute(context.Background(), routing.RetrievalRequest{
		ArtifactType: "subscription",
		Aggregate:    routing.AggregateQuery{Category: "subscriptions", Period: "month", Extremum: routing.ExtremumMax},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Strategy != routing.StrategyStructuredAggregate {
		t.Errorf("strategy = %s, want structured_aggregate", res.Strategy)
	}
	if !strings.Contains(res.Answer, "2026-03") {
		t.Errorf("answer should name the true max month 2026-03, got: %q", res.Answer)
	}
	// No-tautology guard: the aggregate must beat the most-similar-chunk month.
	if strings.Contains(res.Answer, mostSimilarChunkMonth) {
		t.Errorf("answer must NOT be the most-similar-chunk month %s — that is the wrong legacy answer", mostSimilarChunkMonth)
	}
	if len(res.Sources) != 1 || res.Sources[0].Kind != routing.SourceStructuredAggregate {
		t.Errorf("result should cite the structured aggregate, got %+v", res.Sources)
	}
	if !strings.Contains(res.Sources[0].Detail, "subscriptions") {
		t.Errorf("provenance should name the existing subscriptions table, got %q", res.Sources[0].Detail)
	}
}

// TestExecute_Min — the min extremum returns the lowest bucket.
func TestExecute_Min(t *testing.T) {
	agg := &fakeAggregator{table: "expenses", monthly: map[string]float64{"2026-01": 90, "2026-03": 450, "2026-04": 75}}
	res, err := New(agg).Execute(context.Background(), routing.RetrievalRequest{
		Aggregate: routing.AggregateQuery{Category: "expenses", Period: "month", Extremum: routing.ExtremumMin},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Answer, "2026-04") || !strings.Contains(res.Answer, "lowest") {
		t.Errorf("min should return the lowest month 2026-04, got: %q", res.Answer)
	}
}

// TestFinancialDescriptiveOnly — Principle 10: financial aggregates carry the
// existing non-advice framing and never emit advice copy.
func TestFinancialDescriptiveOnly(t *testing.T) {
	agg := &fakeAggregator{table: "expenses", monthly: map[string]float64{"2026-03": 450}}
	res, err := New(agg).Execute(context.Background(), routing.RetrievalRequest{
		Aggregate: routing.AggregateQuery{Category: "market positions", Period: "month", Extremum: routing.ExtremumMax, Financial: true},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Answer), "no financial advice") {
		t.Errorf("financial answer must carry the non-advice framing, got: %q", res.Answer)
	}
	for _, advice := range []string{"you should", "buy ", "sell ", "i recommend"} {
		if strings.Contains(strings.ToLower(res.Answer), advice) {
			t.Errorf("financial answer must not contain advice verb %q, got: %q", advice, res.Answer)
		}
	}
}

// TestExecute_DefaultsToMax — an unset extremum defaults to max (highest).
func TestExecute_DefaultsToMax(t *testing.T) {
	agg := &fakeAggregator{table: "subscriptions", monthly: map[string]float64{"2026-01": 10, "2026-02": 99}}
	res, err := New(agg).Execute(context.Background(), routing.RetrievalRequest{
		Aggregate: routing.AggregateQuery{Category: "subscriptions", Period: "month"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if agg.gotQ.Extremum != routing.ExtremumMax {
		t.Errorf("unset extremum should default to max, aggregator saw %q", agg.gotQ.Extremum)
	}
	if !strings.Contains(res.Answer, "2026-02") {
		t.Errorf("default max should return 2026-02, got: %q", res.Answer)
	}
}
