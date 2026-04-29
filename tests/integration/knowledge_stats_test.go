//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/knowledge"
)

func TestKnowledgeStats_EmptyStoreReturnsZeroValues(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetKnowledgeStatsTables(t, ctx, pool)

	store := knowledge.NewKnowledgeStore(pool)
	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats on empty knowledge store: %v", err)
	}

	if stats.ConceptCount != 0 {
		t.Errorf("ConceptCount = %d, want 0", stats.ConceptCount)
	}
	if stats.EntityCount != 0 {
		t.Errorf("EntityCount = %d, want 0", stats.EntityCount)
	}
	if stats.EdgeCount != 0 {
		t.Errorf("EdgeCount = %d, want 0", stats.EdgeCount)
	}
	if stats.SynthesisCompleted != 0 {
		t.Errorf("SynthesisCompleted = %d, want 0", stats.SynthesisCompleted)
	}
	if stats.SynthesisPending != 0 {
		t.Errorf("SynthesisPending = %d, want 0", stats.SynthesisPending)
	}
	if stats.SynthesisFailed != 0 {
		t.Errorf("SynthesisFailed = %d, want 0", stats.SynthesisFailed)
	}
	if stats.LastSynthesisAt != nil {
		t.Errorf("LastSynthesisAt = %v, want nil", stats.LastSynthesisAt)
	}
	if stats.LintFindingsTotal != 0 {
		t.Errorf("LintFindingsTotal = %d, want 0", stats.LintFindingsTotal)
	}
	if stats.LintFindingsHigh != 0 {
		t.Errorf("LintFindingsHigh = %d, want 0", stats.LintFindingsHigh)
	}
	if stats.PromptContractVersion != "" {
		t.Errorf("PromptContractVersion = %q, want empty string", stats.PromptContractVersion)
	}
}

func resetKnowledgeStatsTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		TRUNCATE TABLE
			knowledge_lint_reports,
			knowledge_entities,
			knowledge_concepts,
			edges,
			artifacts
		CASCADE`)
	if err != nil {
		t.Fatalf("reset knowledge stats tables: %v", err)
	}
}
