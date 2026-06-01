//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/knowledge"
)

// TestOpenKnowledgePersistence_WebSnippetCRUD covers the
// insert / get-by-hash / get-by-id path for the new `web_snippets`
// table introduced by migration 045.
func TestOpenKnowledgePersistence_WebSnippetCRUD(t *testing.T) {
	pool := testPool(t)
	store := knowledge.NewKnowledgeStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hash := fmt.Sprintf("hash-%s", testID(t))
	snip := &knowledge.WebSnippet{
		URL:         "https://example.test/page",
		Title:       "Example page",
		Snippet:     "body text",
		ContentHash: hash,
		Provider:    "searxng",
		FetchedAt:   time.Now().UTC(),
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM web_snippets WHERE content_hash = $1", hash)
	})

	if err := store.InsertWebSnippet(ctx, snip); err != nil {
		t.Fatalf("InsertWebSnippet: %v", err)
	}
	if snip.ID == "" {
		t.Fatal("expected snip.ID to be populated")
	}
	if snip.LifecycleState != knowledge.WebSnippetActive {
		t.Errorf("default lifecycle = %s, want active", snip.LifecycleState)
	}

	got, err := store.GetWebSnippetByContentHash(ctx, hash)
	if err != nil || got == nil {
		t.Fatalf("GetWebSnippetByContentHash: %v %v", got, err)
	}
	if got.ID != snip.ID {
		t.Errorf("round-trip id mismatch: got %s want %s", got.ID, snip.ID)
	}
	if got.URL != snip.URL || got.Snippet != snip.Snippet || got.Provider != snip.Provider {
		t.Errorf("round-trip field mismatch: %+v vs %+v", got, snip)
	}
}

// TestOpenKnowledgePersistence_WebSnippetIdempotentByHash covers the
// idempotence contract documented in the migration comment: inserting
// the same content_hash twice returns the existing row's id and
// bumps last_referenced_at instead of inserting a duplicate.
func TestOpenKnowledgePersistence_WebSnippetIdempotentByHash(t *testing.T) {
	pool := testPool(t)
	store := knowledge.NewKnowledgeStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hash := fmt.Sprintf("hash-%s", testID(t))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM web_snippets WHERE content_hash = $1", hash)
	})

	first := &knowledge.WebSnippet{
		URL: "https://example.test/a", Snippet: "a", ContentHash: hash,
		Provider: "searxng", FetchedAt: time.Now().UTC(),
	}
	if err := store.InsertWebSnippet(ctx, first); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	firstID := first.ID

	second := &knowledge.WebSnippet{
		URL: "https://example.test/b", Snippet: "b", ContentHash: hash,
		Provider: "brave", FetchedAt: time.Now().UTC(),
	}
	if err := store.InsertWebSnippet(ctx, second); err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if second.ID != firstID {
		t.Errorf("duplicate hash should reuse id: got %s want %s", second.ID, firstID)
	}

	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM web_snippets WHERE content_hash = $1", hash,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row, got %d", count)
	}
}

// TestOpenKnowledgePersistence_WebSnippetEmptyHashRejected covers the
// adversarial G021 guard: empty content_hash is refused at the app
// boundary (NOT NULL on the DB column would also reject this, but the
// app-level validation gives a clearer error).
func TestOpenKnowledgePersistence_WebSnippetEmptyHashRejected(t *testing.T) {
	pool := testPool(t)
	store := knowledge.NewKnowledgeStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := store.InsertWebSnippet(ctx, &knowledge.WebSnippet{
		URL: "https://example.test/empty", Snippet: "x", ContentHash: "",
		Provider: "searxng", FetchedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected empty content_hash to be rejected")
	}
}

// TestOpenKnowledgePersistence_WebSnippetLifecycleTransition exercises
// the deterministic 90-day idleness rule for active → cooling.
func TestOpenKnowledgePersistence_WebSnippetLifecycleTransition(t *testing.T) {
	pool := testPool(t)
	store := knowledge.NewKnowledgeStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hash := fmt.Sprintf("hash-%s", testID(t))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM web_snippets WHERE content_hash = $1", hash)
	})

	snip := &knowledge.WebSnippet{
		URL: "https://example.test/lifecycle", Snippet: "x", ContentHash: hash,
		Provider: "searxng", FetchedAt: time.Now().UTC(),
	}
	if err := store.InsertWebSnippet(ctx, snip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Backdate last_referenced_at directly so the transition function
	// observes a 100-day-idle row deterministically.
	hundredDaysAgo := time.Now().UTC().Add(-100 * 24 * time.Hour)
	if _, err := pool.Exec(ctx,
		"UPDATE web_snippets SET last_referenced_at = $1 WHERE id = $2",
		hundredDaysAgo, snip.ID,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	next, err := store.TransitionWebSnippetLifecycle(ctx, snip.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("TransitionWebSnippetLifecycle: %v", err)
	}
	if next != knowledge.WebSnippetCooling {
		t.Errorf("100-day-idle active should transition to cooling, got %s", next)
	}

	got, _ := store.GetWebSnippetByID(ctx, snip.ID)
	if got == nil || got.LifecycleState != knowledge.WebSnippetCooling {
		t.Errorf("persisted state not updated: got %+v", got)
	}
}

// TestOpenKnowledgePersistence_AgentAnswerFullRoundTrip covers the
// transactional insert of AgentAnswer + ToolTrace + cite-back sources
// and reconstructs the full graph via GetAgentAnswerFull.
func TestOpenKnowledgePersistence_AgentAnswerFullRoundTrip(t *testing.T) {
	pool := testPool(t)
	store := knowledge.NewKnowledgeStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Seed prompt artifact (Idea capture) + two web snippets.
	promptID := testID(t) + "-prompt"
	cleanupArtifact(t, pool, promptID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'idea', 'test prompt', 'h-prompt', 'test', NOW(), NOW())`, promptID); err != nil {
		t.Fatalf("seed prompt: %v", err)
	}

	hashA := fmt.Sprintf("hash-a-%s", testID(t))
	hashB := fmt.Sprintf("hash-b-%s", testID(t))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM web_snippets WHERE content_hash IN ($1,$2)", hashA, hashB)
	})

	snipA := &knowledge.WebSnippet{
		URL: "https://example.test/a", Snippet: "a", ContentHash: hashA,
		Provider: "searxng", FetchedAt: time.Now().UTC(),
	}
	snipB := &knowledge.WebSnippet{
		URL: "https://example.test/b", Snippet: "b", ContentHash: hashB,
		Provider: "tavily", FetchedAt: time.Now().UTC(),
	}
	if err := store.InsertWebSnippet(ctx, snipA); err != nil {
		t.Fatalf("snipA: %v", err)
	}
	if err := store.InsertWebSnippet(ctx, snipB); err != nil {
		t.Fatalf("snipB: %v", err)
	}

	traceID := "trace-" + testID(t)
	bundle := &knowledge.AgentAnswerWrite{
		Answer: &knowledge.AgentAnswer{
			PromptArtifactID:  promptID,
			FinalText:         "final answer",
			TerminationReason: "final",
			TokensUsed:        1234,
			USDSpent:          0.0042,
		},
		Traces: []*knowledge.ToolTrace{
			{
				ID:            traceID,
				Sequence:      1,
				ToolName:      "web_search",
				Params:        json.RawMessage(`{"query":"sourdough"}`),
				ResultSummary: json.RawMessage(`{"hits":2}`),
			},
		},
		Sources: []*knowledge.AgentAnswerSource{
			{Kind: knowledge.AgentAnswerSourceWeb, WebSnippetID: snipA.ID},
			{Kind: knowledge.AgentAnswerSourceWeb, WebSnippetID: snipB.ID},
			{Kind: knowledge.AgentAnswerSourceToolTrace, ToolTraceID: traceID},
		},
	}
	if err := store.InsertAgentAnswer(ctx, bundle); err != nil {
		t.Fatalf("InsertAgentAnswer: %v", err)
	}
	if bundle.Answer.ID == "" {
		t.Fatal("bundle id not populated")
	}

	got, err := store.GetAgentAnswerFull(ctx, bundle.Answer.ID)
	if err != nil || got == nil {
		t.Fatalf("GetAgentAnswerFull: %v %v", got, err)
	}
	if got.Answer.FinalText != "final answer" {
		t.Errorf("final_text mismatch: %q", got.Answer.FinalText)
	}
	if got.Answer.LifecycleState != knowledge.AgentAnswerDerived {
		t.Errorf("default lifecycle = %s, want derived", got.Answer.LifecycleState)
	}
	if got.Answer.PriorityWeight != 0.3 {
		t.Errorf("default priority_weight = %f, want 0.3", got.Answer.PriorityWeight)
	}
	if len(got.Traces) != 1 || got.Traces[0].ToolName != "web_search" {
		t.Errorf("traces mismatch: %+v", got.Traces)
	}
	if len(got.Sources) != 3 {
		t.Errorf("sources mismatch: want 3 got %d", len(got.Sources))
	}

	// Cascade: delete the AgentAnswer; traces + sources must vanish,
	// web snippets must remain (they outlive the answer per design).
	if _, err := pool.Exec(ctx, "DELETE FROM agent_answers WHERE id = $1", bundle.Answer.ID); err != nil {
		t.Fatalf("delete answer: %v", err)
	}
	var traceCount, sourceCount, snippetCount int
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM tool_traces WHERE agent_answer_id = $1", bundle.Answer.ID).Scan(&traceCount)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM agent_answer_sources WHERE agent_answer_id = $1", bundle.Answer.ID).Scan(&sourceCount)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM web_snippets WHERE content_hash IN ($1,$2)", hashA, hashB).Scan(&snippetCount)
	if traceCount != 0 {
		t.Errorf("cascade: tool_traces should be 0, got %d", traceCount)
	}
	if sourceCount != 0 {
		t.Errorf("cascade: agent_answer_sources should be 0, got %d", sourceCount)
	}
	if snippetCount != 2 {
		t.Errorf("web_snippets should outlive the answer: got %d, want 2", snippetCount)
	}
}

// TestOpenKnowledgePersistence_AgentAnswerEmptySourcesAllowedByDB
// asserts the documented G021 boundary: storage does NOT enforce
// "every answer must have sources" — that gate lives in the verifier.
// Empty Sources is a valid INSERT.
func TestOpenKnowledgePersistence_AgentAnswerEmptySourcesAllowedByDB(t *testing.T) {
	pool := testPool(t)
	store := knowledge.NewKnowledgeStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	promptID := testID(t) + "-prompt-empty"
	cleanupArtifact(t, pool, promptID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'idea', 'no-source prompt', 'h-prompt-empty', 'test', NOW(), NOW())`, promptID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	bundle := &knowledge.AgentAnswerWrite{
		Answer: &knowledge.AgentAnswer{
			PromptArtifactID:  promptID,
			FinalText:         "refused",
			TerminationReason: "refused",
		},
	}
	if err := store.InsertAgentAnswer(ctx, bundle); err != nil {
		t.Fatalf("InsertAgentAnswer with empty sources should succeed: %v", err)
	}
	got, _ := store.GetAgentAnswerFull(ctx, bundle.Answer.ID)
	if got == nil || len(got.Sources) != 0 {
		t.Errorf("expected zero sources, got %+v", got)
	}
}
