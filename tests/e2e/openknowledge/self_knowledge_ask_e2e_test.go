//go:build e2e

// self_knowledge_ask_e2e_test.go — spec 104 SCOPE-08 regression E2E.
//
// Drives the REAL open-knowledge agent loop end-to-end for a product
// meta-question ("what can you do?") against the live test-stack Postgres, with
// the REAL self_knowledge tool over REAL pgvector. The LLM is the deterministic
// fakeLLM the rest of this package uses (a live model is non-deterministic and
// would make the assertion flaky) — everything else (agent loop, tool, semantic
// search, cite-back verifier, trace persistence) is real.
//
//   - GROUNDED: the agent calls self_knowledge, the tool returns a smackerel_self
//     artifact, the answer cites it → StatusSuccess with that citation (a real,
//     cited capability answer — not a refusal, not "saved as an idea").
//   - UNGROUNDABLE: the answer cites a fabricated artifact absent from the tool
//     trace → under enforce mode the agent REFUSES (StatusRefused /
//     TerminationFabricatedSource) with a typed rejected citation — never a
//     hallucinated success, never saved-as-idea (BUG-061-009 honest fallback).

package openknowledge_e2e

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
	"github.com/smackerel/smackerel/internal/db"
)

type fixedEmbedderE2E struct{ vec []float32 }

func (f fixedEmbedderE2E) Embed(context.Context, string) ([]float32, error) { return f.vec, nil }

func vec384E2E(lead ...float32) []float32 {
	v := make([]float32, 384)
	copy(v, lead)
	return v
}

func seedSelfKnowledgeArtifact(t *testing.T, pool *pgxpool.Pool, id, title string, emb []float32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, embedding)
		VALUES ($1, 'capability', $2, $3, 'smackerel_self', $4::vector)
	`, id, title, "h-"+id, db.FormatEmbedding(emb))
	if err != nil {
		t.Fatalf("seed smackerel_self artifact %s: %v", id, err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, derr := pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, id); derr != nil {
			t.Logf("cleanup artifact %s: %v", id, derr)
		}
	})
}

func TestSelfKnowledge_AskMetaQuestion_GroundedCitedAnswer_E2E(t *testing.T) {
	pool := newTestPool(t)
	writer := tracewriter.New(pool)
	pfx := fmt.Sprintf("sk-ask-e2e-%d", time.Now().UnixNano())

	query := vec384E2E(1, 0, 0)
	selfID := pfx + "-caps"
	seedSelfKnowledgeArtifact(t, pool, selfID, "capabilities overview", vec384E2E(0.9, 0.1, 0))

	toolName := tools.SelfKnowledgeToolName
	r := ok.NewRegistry([]string{toolName})
	if err := r.Register(tools.NewSelfKnowledge(
		tools.NewPgxSemanticSearcher(pool, fixedEmbedderE2E{vec: query}),
		"smackerel_self",
	)); err != nil {
		t.Fatalf("register self_knowledge: %v", err)
	}

	final := fmt.Sprintf(
		`Smackerel is a passive second brain that captures, connects, and answers about your knowledge.<CITATIONS>[{"kind":"artifact","artifact_id":%q}]</CITATIONS>`,
		selfID,
	)
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", toolName, `{"query":"what can smackerel do","k":5}`, 5),
		endTurn(final, 10),
	}}
	a := newAgent(t, fl, r, writer, baseCfg(citeback.EnforcementEnforce))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	got, err := a.Run(ctx, "what can you do?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != agent.StatusSuccess {
		t.Fatalf("grounded meta-question should answer (StatusSuccess); got %q reason=%q rejected=%+v",
			got.Status, got.TerminationReason, got.RejectedCitations)
	}
	var cited bool
	for _, s := range got.Sources {
		if s.Kind == ok.SourceArtifact && s.Artifact != nil && s.Artifact.ID == selfID {
			cited = true
		}
	}
	if !cited {
		t.Fatalf("answer must cite the smackerel_self artifact %q; sources=%+v", selfID, got.Sources)
	}
}

func TestSelfKnowledge_AskUngroundable_RefusesHonestly_E2E(t *testing.T) {
	pool := newTestPool(t)
	writer := tracewriter.New(pool)
	pfx := fmt.Sprintf("sk-ask-ung-e2e-%d", time.Now().UnixNano())

	query := vec384E2E(1, 0, 0)
	selfID := pfx + "-caps"
	seedSelfKnowledgeArtifact(t, pool, selfID, "capabilities overview", vec384E2E(0.9, 0.1, 0))

	toolName := tools.SelfKnowledgeToolName
	r := ok.NewRegistry([]string{toolName})
	if err := r.Register(tools.NewSelfKnowledge(
		tools.NewPgxSemanticSearcher(pool, fixedEmbedderE2E{vec: query}),
		"smackerel_self",
	)); err != nil {
		t.Fatalf("register self_knowledge: %v", err)
	}

	// The LLM fabricates a citation to an artifact the tool never returned.
	final := fmt.Sprintf(
		`Smackerel can cure cancer.<CITATIONS>[{"kind":"artifact","artifact_id":"%s-fabricated"}]</CITATIONS>`,
		pfx,
	)
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", toolName, `{"query":"what can smackerel do","k":5}`, 5),
		endTurn(final, 10),
	}}
	a := newAgent(t, fl, r, writer, baseCfg(citeback.EnforcementEnforce))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	got, err := a.Run(ctx, "what can you do?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Honest refusal — NEVER a hallucinated success, NEVER saved-as-idea.
	if got.Status != agent.StatusRefused {
		t.Fatalf("ungroundable/hallucinated answer must REFUSE (StatusRefused); got %q", got.Status)
	}
	if got.TerminationReason != agent.TerminationFabricatedSource {
		t.Fatalf("want TerminationFabricatedSource; got %q", got.TerminationReason)
	}
	if len(got.RejectedCitations) == 0 {
		t.Fatal("RejectedCitations empty — the verifier rejection MUST be surfaced")
	}
	var sawNotInTrace bool
	for _, rc := range got.RejectedCitations {
		if errors.Is(rc.Reason, citeback.ReasonNotInTrace) {
			sawNotInTrace = true
		}
	}
	if !sawNotInTrace {
		t.Fatalf("want a ReasonNotInTrace rejection for the fabricated citation; got %+v", got.RejectedCitations)
	}
}
