//go:build integration

// Spec 076 SCOPE-2b — TP-076-02-06 (SCN-064-A07).
//
// Operator-disabled web_search fallback: when the planner asks for a
// tool that is registered but excluded from the allowlist, the
// registry returns ErrToolDisabled. The agent loop MUST surface the
// disablement back to the LLM, allow the planner to fall back to
// internal_retrieval, and complete the turn with the internal source.
//
// Adversarial assertion: the trace MUST include a failed row for the
// disabled web_search attempt (proving the registry sentinel fired)
// AND a succeeded row for the internal_retrieval fallback (proving the
// agent did not bail on the disablement).

package openknowledge_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

func TestAgent_WebSearchDisabledFallsBack(t *testing.T) {
	pool := newTestPool(t)
	prefix := fmt.Sprintf("tp076-02-06-%d", time.Now().UnixNano())
	cleanupTracesByToolPrefix(t, pool, prefix)

	internalName := prefix + "-internal_retrieval"
	webName := prefix + "-web_search"
	artifactID := prefix + "-art-fallback"

	// internal_retrieval is registered AND allowlisted; web_search is
	// registered but NOT allowlisted — Lookup must return ErrToolDisabled.
	r := ok.NewRegistry([]string{internalName})
	if err := r.Register(scriptedArtifactTool{name: internalName, artifactID: artifactID, snippet: "internal hit"}); err != nil {
		t.Fatalf("register internal: %v", err)
	}
	if err := r.Register(scriptedWebTool{name: webName, url: "https://disabled.test/" + prefix, hash: "h", snippet: "would have been web"}); err != nil {
		t.Fatalf("register web: %v", err)
	}

	final := fmt.Sprintf(
		`Answered from your saved notes.<CITATIONS>[{"kind":"artifact","artifact_id":%q}]</CITATIONS>`,
		artifactID,
	)
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", webName, `{"query":"q"}`, 5),
		toolUse("c2", internalName, `{"query":"q"}`, 5),
		endTurn(final, 10),
	}}

	writer := tracewriter.New(pool)
	a := buildAgent(t, fl, r, writer, defaultCfg())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got, err := a.Run(ctx, "ask")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != agent.StatusSuccess {
		t.Fatalf("Status=%q reason=%q rejected=%+v", got.Status, got.RefusalReason, got.RejectedCitations)
	}
	if len(got.Sources) != 1 || got.Sources[0].Kind != ok.SourceArtifact {
		t.Errorf("expected single artifact source from fallback, got=%+v", got.Sources)
	}

	rows := queryTracesByToolPrefix(t, pool, prefix)
	var sawDisabledFailed, sawInternalSucceeded bool
	for _, row := range rows {
		if row.ToolName == webName && row.Outcome == string(tracewriter.OutcomeFailed) {
			sawDisabledFailed = true
		}
		if row.ToolName == internalName && row.Outcome == string(tracewriter.OutcomeSucceeded) {
			sawInternalSucceeded = true
		}
	}
	if !sawDisabledFailed {
		t.Errorf("expected failed trace row for disabled web_search attempt, rows=%+v", rows)
	}
	if !sawInternalSucceeded {
		t.Errorf("expected succeeded trace row for internal fallback, rows=%+v", rows)
	}
}
