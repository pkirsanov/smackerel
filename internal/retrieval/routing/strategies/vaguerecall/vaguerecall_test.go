// Spec 095 SCOPE-06 — vague_recall strategy tests.
package vaguerecall

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// fakeExecutor returns a fixed sentinel result so the test can assert the
// strategy delegates it unchanged (NFR-3).
type fakeExecutor struct {
	result routing.RetrievalResult
	err    error
	gotQ   string
	calls  int
}

func (f *fakeExecutor) VagueRecall(_ context.Context, query string) (routing.RetrievalResult, error) {
	f.calls++
	f.gotQ = query
	if f.err != nil {
		return routing.RetrievalResult{}, f.err
	}
	return f.result, nil
}

// TestDelegatesToExistingPipeline — SCN-095-A04 / NFR-3: the adapter delegates
// to the existing pipeline and preserves its sources + answer verbatim (only
// stamping the strategy kind).
func TestDelegatesToExistingPipeline(t *testing.T) {
	sentinel := routing.RetrievalResult{
		Answer: "the pricing video you watched last week",
		Sources: []routing.RetrievedSource{
			{Kind: routing.SourceVagueRecallSet, ArtifactID: "art-42", Detail: "vector+graph+rerank top-1"},
		},
	}
	f := &fakeExecutor{result: sentinel}
	s := New(f)

	res, err := s.Execute(context.Background(), routing.RetrievalRequest{Query: "that pricing video"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if f.calls != 1 || f.gotQ != "that pricing video" {
		t.Errorf("strategy should delegate the query once to the existing pipeline, got calls=%d q=%q", f.calls, f.gotQ)
	}
	if res.Strategy != routing.StrategyVagueRecall {
		t.Errorf("strategy stamp = %s, want vague_recall", res.Strategy)
	}
	// Answer + sources MUST be preserved verbatim (NFR-3 zero regression).
	if res.Answer != sentinel.Answer {
		t.Errorf("answer mutated: got %q, want %q", res.Answer, sentinel.Answer)
	}
	if !reflect.DeepEqual(res.Sources, sentinel.Sources) {
		t.Errorf("sources mutated: got %+v, want %+v", res.Sources, sentinel.Sources)
	}
}

// TestExecute_PropagatesError — the adapter surfaces the existing pipeline's
// error rather than swallowing it.
func TestExecute_PropagatesError(t *testing.T) {
	s := New(&fakeExecutor{err: errors.New("boom")})
	if _, err := s.Execute(context.Background(), routing.RetrievalRequest{Query: "x"}); err == nil {
		t.Fatal("Execute should surface the existing pipeline error")
	}
}
