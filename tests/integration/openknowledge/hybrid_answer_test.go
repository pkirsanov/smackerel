//go:build integration

// Spec 076 SCOPE-2b — TP-076-02-02 (SCN-064-A03).
//
// Hybrid answer path: the agent calls internal_retrieval first, then
// web_search, and synthesizes a final answer citing both source
// kinds. Trace persistence MUST record two succeeded rows (one per
// tool invocation) in `assistant_tool_traces` against the live test
// stack.

package openknowledge_integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

func TestOpenKnowledge_HybridInternalAndWeb(t *testing.T) {
	pool := newTestPool(t)
	prefix := fmt.Sprintf("tp076-02-02-%d", time.Now().UnixNano())
	cleanupTracesByToolPrefix(t, pool, prefix)

	internalName := prefix + "-internal_retrieval"
	webName := prefix + "-web_search"
	artifactID := prefix + "-art-1"
	webURL := "https://example.test/" + prefix
	webHash := "deadbeef" + prefix

	internalTool := scriptedArtifactTool{name: internalName, artifactID: artifactID, snippet: "from your saved notes"}
	webTool := scriptedWebTool{name: webName, url: webURL, hash: webHash, snippet: "from the web"}

	r := ok.NewRegistry([]string{internalName, webName})
	if err := r.Register(internalTool); err != nil {
		t.Fatalf("register internal: %v", err)
	}
	if err := r.Register(webTool); err != nil {
		t.Fatalf("register web: %v", err)
	}

	final := fmt.Sprintf(
		`Combined answer.<CITATIONS>[{"kind":"artifact","artifact_id":%q},{"kind":"web","url":%q,"content_hash":%q}]</CITATIONS>`,
		artifactID, webURL, webHash,
	)
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", internalName, `{"query":"q"}`, 5),
		toolUse("c2", webName, `{"query":"q"}`, 5),
		endTurn(final, 10),
	}}

	writer := tracewriter.New(pool)
	a := buildAgent(t, fl, r, writer, defaultCfg())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got, err := a.Run(ctx, "hybrid question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != agent.StatusSuccess {
		t.Fatalf("Status=%q reason=%q rejected=%+v", got.Status, got.RefusalReason, got.RejectedCitations)
	}
	if got.TerminationReason != agent.TerminationFinal {
		t.Errorf("TerminationReason=%q want final", got.TerminationReason)
	}
	if !strings.Contains(got.FinalText, "Combined answer.") {
		t.Errorf("FinalText=%q missing synthesized prefix", got.FinalText)
	}
	if len(got.Sources) != 2 {
		t.Fatalf("Sources len=%d want 2 (1 graph + 1 web), got=%+v", len(got.Sources), got.Sources)
	}
	var sawArtifact, sawWeb bool
	for _, s := range got.Sources {
		switch s.Kind {
		case ok.SourceArtifact:
			sawArtifact = true
		case ok.SourceWeb:
			sawWeb = true
		}
	}
	if !sawArtifact || !sawWeb {
		t.Errorf("expected one artifact + one web source, got=%+v", got.Sources)
	}

	rows := queryTracesByToolPrefix(t, pool, prefix)
	if len(rows) != 2 {
		t.Fatalf("persisted trace rows=%d want 2: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.Outcome != string(tracewriter.OutcomeSucceeded) {
			t.Errorf("row %s outcome=%q want succeeded", r.ToolName, r.Outcome)
		}
		if r.Lifecycle != tracewriter.LifecycleActive {
			t.Errorf("row %s lifecycle=%q want active", r.ToolName, r.Lifecycle)
		}
	}
}
