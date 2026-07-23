//go:build integration

// self_knowledge_tool_test.go — spec 104 SCOPE-04 integration test.
//
// Proves the self_knowledge tool end-to-end over REAL pgvector: it returns
// cited Source{Kind:SourceArtifact} entries drawn ONLY from the smackerel_self
// namespace (never a personal-graph namespace), ordered by cosine similarity.
// Deterministic — artifacts carry explicit embeddings and the query is embedded
// by a fixed fake embedder (no ML sidecar needed). Reuses the SCOPE-01 harness
// helpers (openSemanticPool, vec384, insertEmbeddedArtifact, fixedEmbedder).

package openknowledge_integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
)

func TestSelfKnowledgeTool_CitesOnlySmackerelSelf(t *testing.T) {
	pool := openSemanticPool(t)
	pfx := "sk-tool-" + time.Now().Format("150405.000000")
	userNS := "user:" + pfx

	query := vec384(1, 0, 0)
	insertEmbeddedArtifact(t, pool, pfx+"-A", "smackerel_self", "capabilities overview", vec384(0.9, 0.1, 0))
	insertEmbeddedArtifact(t, pool, pfx+"-B", "smackerel_self", "slash commands", vec384(0.2, 0.4, 0))
	// A personal-graph row that is the closest overall — MUST NOT be cited.
	insertEmbeddedArtifact(t, pool, pfx+"-Cuser", userNS, "private personal note", vec384(1, 0, 0))

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

	// Every returned source is an artifact citation (never a web/computation).
	for i, s := range res.Sources {
		if s.Kind != ok.SourceArtifact || s.Artifact == nil {
			t.Fatalf("source %d Kind=%q artifact=%v, want SourceArtifact + non-nil", i, s.Kind, s.Artifact)
		}
		if s.Artifact.ID == pfx+"-Cuser" {
			t.Fatalf("personal-graph artifact %q leaked into a self_knowledge answer (isolation breach)", s.Artifact.ID)
		}
	}

	// This run's smackerel_self rows are cited, cosine-ordered (A before B).
	var inRun []string
	for _, s := range res.Sources {
		id := s.Artifact.ID
		if len(id) >= len(pfx) && id[:len(pfx)] == pfx {
			inRun = append(inRun, id)
		}
	}
	if len(inRun) != 2 {
		t.Fatalf("got %d in-run cited self rows, want 2 (ids=%v)", len(inRun), inRun)
	}
	if inRun[0] != pfx+"-A" {
		t.Fatalf("first cited row = %q, want %q-A (cosine ordering)", inRun[0], pfx)
	}
	// Snippets are 1:1 with sources (each citation carries its evidence text).
	if len(res.Snippets) != len(res.Sources) {
		t.Fatalf("snippets=%d sources=%d, want equal", len(res.Snippets), len(res.Sources))
	}
}
