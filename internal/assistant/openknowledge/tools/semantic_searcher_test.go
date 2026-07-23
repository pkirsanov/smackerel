package tools

// semantic_searcher_test.go — spec 104 SCOPE-01 unit tests.
//
// Covers the validation + embed-error short-circuits: every one of these
// paths MUST return a typed error BEFORE any database access (proven by the
// queryGuard, which fails the test if pool.Query is reached). Query
// correctness (namespace scoping + cosine ordering) is proven by the
// integration test against real pgvector, mirroring the PgxGraphSearcher
// precedent (the pgx adapter is integration-tested, not unit-mocked).

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// fakeEmbedder returns a fixed (vec, err) for any input.
type fakeEmbedder struct {
	vec []float32
	err error
}

func (f fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return f.vec, f.err
}

// queryGuard is a rowQuerier that fails the test if Query is ever called.
// The validation + embed-error paths MUST short-circuit before any DB access.
type queryGuard struct{ t *testing.T }

func (g queryGuard) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	g.t.Fatalf("pool.Query must not be reached on a validation/embed-error path")
	return nil, nil
}

func TestPgxSemanticSearcher_ValidationAndEmbedShortCircuit(t *testing.T) {
	t.Parallel()
	okEmbedder := fakeEmbedder{vec: []float32{0.1, 0.2, 0.3}}
	cases := []struct {
		name      string
		namespace string
		query     string
		k         int
		embedder  Embedder
		wantErr   error
	}{
		{"empty namespace", "", "q", 5, okEmbedder, ErrSemanticSearchNamespace},
		{"blank namespace", "   ", "q", 5, okEmbedder, ErrSemanticSearchNamespace},
		{"empty query", "smackerel_self", "", 5, okEmbedder, ErrSemanticSearchQuery},
		{"blank query", "smackerel_self", "   ", 5, okEmbedder, ErrSemanticSearchQuery},
		{"k zero", "smackerel_self", "q", 0, okEmbedder, ErrSemanticSearchK},
		{"k negative", "smackerel_self", "q", -1, okEmbedder, ErrSemanticSearchK},
		{"k over max", "smackerel_self", "q", MaxInternalRetrievalK + 1, okEmbedder, ErrSemanticSearchK},
		{"embedder error", "smackerel_self", "q", 5, fakeEmbedder{err: errors.New("sidecar down")}, ErrSemanticSearchEmbed},
		{"nil embedding", "smackerel_self", "q", 5, fakeEmbedder{vec: nil}, ErrSemanticSearchEmptyVec},
		{"empty embedding slice", "smackerel_self", "q", 5, fakeEmbedder{vec: []float32{}}, ErrSemanticSearchEmptyVec},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewPgxSemanticSearcher(queryGuard{t: t}, tc.embedder)
			got, err := s.Search(context.Background(), tc.namespace, tc.query, tc.k)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Search err = %v, want errors.Is %v", err, tc.wantErr)
			}
			if got != nil {
				t.Fatalf("Search returned %v rows on an error path, want nil", got)
			}
		})
	}
}

func TestNewPgxSemanticSearcher_NilArgsPanic(t *testing.T) {
	t.Parallel()
	assertSemanticSearcherPanics(t, "nil pool", func() {
		NewPgxSemanticSearcher(nil, fakeEmbedder{vec: []float32{0.1}})
	})
	assertSemanticSearcherPanics(t, "nil embedder", func() {
		NewPgxSemanticSearcher(queryGuard{t: t}, nil)
	})
}

func assertSemanticSearcherPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: expected panic, got none", name)
		}
	}()
	fn()
}
