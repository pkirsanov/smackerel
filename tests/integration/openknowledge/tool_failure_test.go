//go:build integration

// Spec 076 SCOPE-2b — TP-076-02-04 (SCN-064-A05).
//
// Tool-failure refusal path: a tool returns the circuit-open ToolError
// code (the canonical "provider unavailable" signal). The agent loop
// MUST terminate with TerminationToolUnavailable, emit an empty
// FinalText/Sources for capture-as-fallback, persist the tool's
// `failed` row via the invokeTool path, AND persist a terminal
// `refused` row attributing the refusal to the failing tool.

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

func TestAgent_ToolFailureRefusesWithCapture(t *testing.T) {
	pool := newTestPool(t)
	prefix := fmt.Sprintf("tp076-02-04-%d", time.Now().UnixNano())
	cleanupTracesByToolPrefix(t, pool, prefix)

	toolName := prefix + "-circuit_tool"
	r := ok.NewRegistry([]string{toolName})
	if err := r.Register(circuitOpenTool{name: toolName}); err != nil {
		t.Fatalf("register: %v", err)
	}

	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", toolName, `{"q":"x"}`, 5),
	}}

	writer := tracewriter.New(pool)
	a := buildAgent(t, fl, r, writer, defaultCfg())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got, err := a.Run(ctx, "ask")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != agent.StatusRefused {
		t.Fatalf("Status=%q want refused", got.Status)
	}
	if got.TerminationReason != agent.TerminationToolUnavailable {
		t.Errorf("TerminationReason=%q want %q", got.TerminationReason, agent.TerminationToolUnavailable)
	}
	if got.FinalText != "" {
		t.Errorf("FinalText=%q want empty (capture-as-fallback contract)", got.FinalText)
	}

	rows := queryTracesByToolPrefix(t, pool, prefix)
	if len(rows) < 2 {
		t.Fatalf("persisted rows=%d want >=2 (failed + refused): %+v", len(rows), rows)
	}
	var sawFailed, sawRefused bool
	for _, row := range rows {
		switch row.Outcome {
		case string(tracewriter.OutcomeFailed):
			sawFailed = true
		case string(tracewriter.OutcomeRefused):
			sawRefused = true
		}
	}
	if !sawFailed {
		t.Errorf("missing failed trace row for circuit tool: %+v", rows)
	}
	if !sawRefused {
		t.Errorf("missing refused trace row at terminal refusal: %+v", rows)
	}
}
