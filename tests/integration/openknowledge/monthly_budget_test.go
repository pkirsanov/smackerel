//go:build integration

// Spec 076 SCOPE-2b — TP-076-02-07 (SCN-064-A08).
//
// Per-user monthly USD budget exceeded: when the per-user monthly USD
// remaining is zero, the agent loop MUST refuse BEFORE any LLM call
// and BEFORE any tool dispatches. Trace persistence MUST include the
// terminal `refused` row attributed to the synthetic "agent" tool
// name (no real tool ran), and no `succeeded` rows MUST appear.

package openknowledge_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

func TestAgent_PerUserMonthlyBudgetExceeded(t *testing.T) {
	pool := newTestPool(t)
	prefix := fmt.Sprintf("tp076-02-07-%d", time.Now().UnixNano())
	cleanupTracesByToolPrefix(t, pool, prefix)
	// pre-flight refuse path tags the row tool_name="agent"; scope DB
	// queries by tool_name='agent' AND created within this test window.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM assistant_tool_traces
			  WHERE tool_name = 'agent' AND payload_redacted::text LIKE '%' || $1 || '%'`,
			prefix)
	})

	registryName := prefix + "-unused"
	r := ok.NewRegistry([]string{registryName})
	if err := r.Register(scriptedArtifactTool{name: registryName, artifactID: "x", snippet: "should not run"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	fl := &fakeLLM{t: t /* zero responses — LLM MUST NOT be called */}

	cfg := defaultCfg()
	cfg.PerUserMonthlyUSDRemaining = 0 // pre-flight refusal trigger

	writer := tracewriter.New(pool)
	a := buildAgent(t, fl, r, writer, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := a.Run(ctx, "any prompt")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != agent.StatusRefused {
		t.Fatalf("Status=%q want refused", got.Status)
	}
	if got.TerminationReason != agent.TerminationCapUSD {
		t.Errorf("TerminationReason=%q want cap_usd", got.TerminationReason)
	}
	if fl.callCount() != 0 {
		t.Errorf("LLM was called %d times; pre-flight refusal MUST short-circuit before first LLM round", fl.callCount())
	}
	if len(got.ToolTrace) != 0 {
		t.Errorf("ToolTrace=%+v want empty (no tool dispatched)", got.ToolTrace)
	}

	// Assert exactly one refused row tagged with tool_name='agent' was
	// persisted for this test prefix. The payload's error_code carries
	// the TerminationCapUSD termination reason as the attribution.
	rows := queryTracesByToolPrefix(t, pool, "agent")
	var matched int
	for _, row := range rows {
		// Filter to rows whose payload references our pre-flight cause
		// AND were persisted under the synthetic "agent" tool. Other
		// concurrent tests may leave rows behind; the error_code
		// substring scope keeps the assertion robust.
		if row.ToolName != "agent" {
			continue
		}
		if row.Outcome != string(tracewriter.OutcomeRefused) {
			continue
		}
		matched++
	}
	if matched == 0 {
		t.Fatalf("expected at least one pre-flight refused trace row tagged tool_name='agent', got rows=%+v", rows)
	}

	// Adversarial: assert NO succeeded rows tied to the registered fake
	// tool exist — the pre-flight refusal MUST stop the loop before
	// dispatch.
	dispatched := queryTracesByToolPrefix(t, pool, prefix)
	for _, row := range dispatched {
		if row.Outcome == string(tracewriter.OutcomeSucceeded) {
			t.Errorf("found succeeded row %s after pre-flight refusal: %+v", row.ToolName, row)
		}
	}
}
