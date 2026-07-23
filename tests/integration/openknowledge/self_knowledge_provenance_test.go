//go:build integration

// self_knowledge_provenance_test.go — spec 104 SCOPE-07 integration test.
//
// Proves the trust perimeter for self-knowledge answers end-to-end over REAL
// pgvector + the REAL cite-back verifier (the same verifier the open-knowledge
// agent loop runs each turn). It exercises the three trust properties without a
// live LLM (the LLM only produces the answer's citations; the verifier's verdict
// is deterministic given a citation set + tool trace):
//
//  1. GROUNDED  — an answer citing a smackerel_self artifact the tool returned
//     passes cite-back (VerifyResult.OK).
//  2. UNGROUNDABLE / HALLUCINATED — a citation absent from the tool trace is
//     REJECTED (ReasonNotInTrace). The facade renders this as an honest
//     StatusUnavailable refusal (BUG-061-009 INV-HB-REFUSAL) — never "saved as
//     an idea", never a hallucinated answer.
//  3. PERSONAL ISOLATION — a personal-graph (user:) artifact is never in the
//     self_knowledge tool's recorded sources, so an answer can never cite it
//     via self_knowledge (cite-back rejects it).

package openknowledge_integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
)

func TestSelfKnowledge_TrustPerimeter(t *testing.T) {
	pool := openSemanticPool(t)
	pfx := "sk-prov-" + time.Now().Format("150405.000000")
	userNS := "user:" + pfx

	query := vec384(1, 0, 0)
	insertEmbeddedArtifact(t, pool, pfx+"-self", "smackerel_self", "capabilities overview", vec384(0.9, 0.1, 0))
	// A personal-graph row identical to the query — the closest overall, yet it
	// MUST never be citeable through self_knowledge.
	insertEmbeddedArtifact(t, pool, pfx+"-user", userNS, "private personal note", vec384(1, 0, 0))

	tool := tools.NewSelfKnowledge(
		tools.NewPgxSemanticSearcher(pool, fixedEmbedder{vec: query}),
		"smackerel_self",
	)
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"what can smackerel do","k":25}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %v", res.Error)
	}

	// The per-turn tool trace the agent loop hands to the verifier.
	trace := citeback.ToolTrace{{ToolName: tools.SelfKnowledgeToolName, RecordedSources: res.Sources}}

	// The self artifact was returned; the personal artifact was NOT (isolation
	// at the source, before cite-back even runs).
	var selfCited bool
	for _, s := range res.Sources {
		if s.Artifact == nil {
			t.Fatalf("self_knowledge returned a non-artifact source: %+v", s)
		}
		if s.Artifact.ID == pfx+"-user" {
			t.Fatalf("personal-graph artifact leaked into self_knowledge sources: %q", s.Artifact.ID)
		}
		if s.Artifact.ID == pfx+"-self" {
			selfCited = true
		}
	}
	if !selfCited {
		t.Fatalf("self artifact %q not returned by the tool", pfx+"-self")
	}

	// 1. GROUNDED — an answer citing the returned smackerel_self artifact passes.
	grounded := citeback.Verify(
		[]citeback.Citation{{Kind: ok.SourceArtifact, ArtifactID: pfx + "-self"}},
		trace,
	)
	if !grounded.OK {
		t.Fatalf("grounded self-knowledge answer must pass cite-back; rejected=%+v", grounded.Rejected)
	}
	if len(grounded.Verified) != 1 {
		t.Fatalf("grounded answer verified %d citations, want 1", len(grounded.Verified))
	}

	// 2. UNGROUNDABLE / HALLUCINATED — a citation not in the trace is refused.
	hallucinated := citeback.Verify(
		[]citeback.Citation{{Kind: ok.SourceArtifact, ArtifactID: pfx + "-does-not-exist"}},
		trace,
	)
	if hallucinated.OK {
		t.Fatal("a fabricated citation must fail cite-back (ungroundable → honest refusal, never saved-as-idea)")
	}
	if len(hallucinated.Rejected) == 0 || !errors.Is(hallucinated.Rejected[0].Reason, citeback.ReasonNotInTrace) {
		t.Fatalf("want ReasonNotInTrace for a fabricated citation, got %+v", hallucinated.Rejected)
	}

	// 3. PERSONAL ISOLATION — citing the personal-graph artifact via
	//    self_knowledge is rejected (it is not in the tool trace).
	leak := citeback.Verify(
		[]citeback.Citation{{Kind: ok.SourceArtifact, ArtifactID: pfx + "-user"}},
		trace,
	)
	if leak.OK {
		t.Fatal("citing a personal-graph artifact via self_knowledge must be rejected (isolation)")
	}
	if len(leak.Rejected) == 0 || !errors.Is(leak.Rejected[0].Reason, citeback.ReasonNotInTrace) {
		t.Fatalf("want ReasonNotInTrace for the personal-graph citation, got %+v", leak.Rejected)
	}
}
