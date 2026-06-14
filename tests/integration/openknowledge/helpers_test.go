//go:build integration

// Spec 076 SCOPE-2b integration helpers.
//
// Shared fakes for the four agent-loop integration tests
// (TP-076-02-02, 02-04, 02-06, 02-07). The agent loop is driven
// against a fake LLM script + the live `assistant_tool_traces` table
// so each refusal/hybrid path proves it persists the contract-shaped
// trace rows the spec 076 SCOPE-2a writer exposes.

package openknowledge_integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

type fakeLLM struct {
	t         *testing.T
	mu        sync.Mutex
	responses []llm.Result
	calls     int
}

func (f *fakeLLM) Chat(_ context.Context, _ llm.ChatRequest) (llm.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls >= len(f.responses) {
		f.t.Fatalf("fakeLLM: unexpected call #%d (queue exhausted)", f.calls+1)
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

func (f *fakeLLM) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func endTurn(text string, tokens int) llm.Result {
	return llm.Result{StopReason: llm.StopEndTurn, FinalText: text, TokensUsed: tokens}
}

func toolUse(id, name, args string, tokens int) llm.Result {
	return llm.Result{
		StopReason: llm.StopToolUse,
		ToolCalls:  []llm.ToolCall{{ID: id, Name: name, Arguments: json.RawMessage(args)}},
		TokensUsed: tokens,
	}
}

type scriptedWebTool struct {
	name, url, hash, snippet string
}

func (s scriptedWebTool) Name() string                { return s.name }
func (scriptedWebTool) Description() string           { return "fake web search for spec 076 SCOPE-2b" }
func (scriptedWebTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s scriptedWebTool) Execute(context.Context, json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{
		Snippets: []ok.Snippet{{Text: s.snippet, ContentHash: s.hash, SourceRef: s.url}},
		Sources: []ok.Source{{
			Kind: ok.SourceWeb,
			Web:  &ok.WebSource{URL: s.url, ContentHash: s.hash, Provider: "fake", Snippet: s.snippet},
		}},
	}, nil
}

type scriptedArtifactTool struct {
	name, artifactID, snippet string
}

func (a scriptedArtifactTool) Name() string { return a.name }
func (scriptedArtifactTool) Description() string {
	return "fake internal_retrieval for spec 076 SCOPE-2b"
}
func (scriptedArtifactTool) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (a scriptedArtifactTool) Execute(context.Context, json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{
		Snippets: []ok.Snippet{{Text: a.snippet, ContentHash: "art-" + a.artifactID, SourceRef: a.artifactID}},
		Sources: []ok.Source{{
			Kind:     ok.SourceArtifact,
			Artifact: &ok.ArtifactRef{ID: a.artifactID, Kind: "note", Title: a.snippet},
		}},
	}, nil
}

type circuitOpenTool struct{ name string }

func (c circuitOpenTool) Name() string                { return c.name }
func (circuitOpenTool) Description() string           { return "fake circuit-open tool" }
func (circuitOpenTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (circuitOpenTool) Execute(context.Context, json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{Error: &ok.ToolError{
		Code:    agent.ToolErrorCodeCircuitOpen,
		Message: "circuit breaker open",
	}}, nil
}

func defaultCfg() agent.Config {
	return agent.Config{
		SystemPrompt:               "spec-076-2b-integration-prompt",
		Model:                      "spec-076-2b-fake-model",
		SynthesisModel:             "spec-076-2b-fake-model",
		MaxIterations:              4,
		SourcesMax:                 5,
		PerQueryTokenBudget:        2000,
		PerQueryUSDBudget:          1.0,
		MonthlyBudgetUSDRemaining:  10.0,
		PerUserMonthlyUSDRemaining: 10.0,
		CompactionThresholdRatio:   0.8,
		CostFn:                     func(int) float64 { return 0 },
		EnforcementMode:            string(citeback.EnforcementEnforce),
	}
}

func buildAgent(t *testing.T, fl *fakeLLM, r *ok.Registry, writer tracewriter.Writer, cfg agent.Config) *agent.Agent {
	t.Helper()
	cfg.TraceWriter = writer
	a, err := agent.New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	return a
}

type persistedTrace struct {
	ToolName    string
	Outcome     string
	Lifecycle   string
	PayloadJSON string
}

func queryTracesByToolPrefix(t *testing.T, pool *pgxpool.Pool, prefix string) []persistedTrace {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rows, err := pool.Query(ctx,
		`SELECT tool_name, call_outcome, lifecycle_state, payload_redacted::text
		   FROM assistant_tool_traces
		  WHERE tool_name LIKE $1
		  ORDER BY id`, prefix+"%")
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}
	defer rows.Close()
	var out []persistedTrace
	for rows.Next() {
		var pt persistedTrace
		if err := rows.Scan(&pt.ToolName, &pt.Outcome, &pt.Lifecycle, &pt.PayloadJSON); err != nil {
			t.Fatalf("scan trace: %v", err)
		}
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

func cleanupTracesByToolPrefix(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM assistant_tool_traces WHERE tool_name LIKE $1", prefix+"%")
	})
}
