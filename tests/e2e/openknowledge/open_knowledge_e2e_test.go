//go:build e2e

// Spec 076 SCOPE-2c — TP-076-02-08 (SCN-064-A02..A08 regression E2E).
//
// Regression E2E for the open-knowledge agent surface. Drives the
// real agent loop end-to-end against the live test-stack Postgres so
// every fixed/added behavior from scopes 2a–2c remains observable in
// `assistant_tool_traces` on the disposable stack:
//
//   - A02 tool-trace persistence on a successful tool call.
//   - A03 hybrid internal + web answer with two cited sources.
//   - A04 unknown-tool refusal (ErrToolNotRegistered) — surfaced via
//     the agent loop and persisted as a failed trace row.
//   - A05 tool failure (circuit-open) → refusal-with-capture.
//   - A06 fabricated source → refusal under enforce mode, success
//     (with mismatch logged) under shadow mode — SCOPE-2c.
//   - A07 operator-disabled tool fallback to internal_retrieval.
//   - A08 per-user monthly USD budget pre-flight refusal.
//
// Each subtest persists at least one row to `assistant_tool_traces`
// scoped by a per-subtest tool-name prefix so the assertions never
// touch unrelated rows. Test cleanup deletes those rows on exit.
package openknowledge_e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

// ----- shared fakes -----

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

type webTool struct{ name, url, hash, snippet string }

func (w webTool) Name() string                { return w.name }
func (webTool) Description() string           { return "fake web search (spec 076 SCOPE-2c E2E)" }
func (webTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (w webTool) Execute(context.Context, json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{
		Snippets: []ok.Snippet{{Text: w.snippet, ContentHash: w.hash, SourceRef: w.url}},
		Sources: []ok.Source{{
			Kind: ok.SourceWeb,
			Web:  &ok.WebSource{URL: w.url, ContentHash: w.hash, Provider: "fake", Snippet: w.snippet},
		}},
	}, nil
}

type artifactTool struct{ name, artifactID, snippet string }

func (a artifactTool) Name() string                { return a.name }
func (artifactTool) Description() string           { return "fake internal_retrieval (spec 076 SCOPE-2c E2E)" }
func (artifactTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (a artifactTool) Execute(context.Context, json.RawMessage) (*ok.ToolResult, error) {
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

// ----- helpers -----

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func cleanupTracesByToolPrefix(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM assistant_tool_traces WHERE tool_name LIKE $1", prefix+"%")
	})
}

type persistedTrace struct {
	ToolName string
	Outcome  string
}

func queryTracesByToolPrefix(t *testing.T, pool *pgxpool.Pool, prefix string) []persistedTrace {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rows, err := pool.Query(ctx,
		`SELECT tool_name, call_outcome
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
		if err := rows.Scan(&pt.ToolName, &pt.Outcome); err != nil {
			t.Fatalf("scan trace: %v", err)
		}
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

func baseCfg(mode citeback.EnforcementMode) agent.Config {
	return agent.Config{
		SystemPrompt:               "spec-076-2c-e2e-prompt",
		Model:                      "spec-076-2c-fake-model",
		SynthesisModel:             "spec-076-2c-fake-model",
		MaxIterations:              4,
		PerQueryTokenBudget:        2000,
		PerQueryUSDBudget:          1.0,
		MonthlyBudgetUSDRemaining:  10.0,
		PerUserMonthlyUSDRemaining: 10.0,
		CompactionThresholdRatio:   0.8,
		CostFn:                     func(int) float64 { return 0 },
		EnforcementMode:            string(mode),
	}
}

func newAgent(t *testing.T, fl *fakeLLM, r *ok.Registry, writer tracewriter.Writer, cfg agent.Config) *agent.Agent {
	t.Helper()
	cfg.TraceWriter = writer
	a, err := agent.New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	return a
}

// ----- regression sweep -----

func TestOpenKnowledgeAgent_FullScenarioMatrix(t *testing.T) {
	pool := newTestPool(t)
	writer := tracewriter.New(pool)

	t.Run("SCN-064-A02_unit_convert_persists_succeeded_trace", func(t *testing.T) {
		prefix := fmt.Sprintf("tp076-02-08-a02-%d", time.Now().UnixNano())
		cleanupTracesByToolPrefix(t, pool, prefix)

		toolName := prefix + "-unit_convert"
		r := ok.NewRegistry([]string{toolName})
		if err := r.Register(artifactTool{name: toolName, artifactID: prefix + "-art", snippet: "12 inches = 30.48 cm"}); err != nil {
			t.Fatalf("register: %v", err)
		}
		final := fmt.Sprintf(
			`12 inches is 30.48 cm.<CITATIONS>[{"kind":"artifact","artifact_id":%q}]</CITATIONS>`,
			prefix+"-art",
		)
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("c1", toolName, `{"value":12,"from":"in","to":"cm"}`, 5),
			endTurn(final, 10),
		}}
		a := newAgent(t, fl, r, writer, baseCfg(citeback.EnforcementEnforce))
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		got, err := a.Run(ctx, "convert 12 inches to cm")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != agent.StatusSuccess {
			t.Fatalf("Status=%q reason=%q", got.Status, got.TerminationReason)
		}
		rows := queryTracesByToolPrefix(t, pool, prefix)
		if len(rows) != 1 || rows[0].Outcome != string(tracewriter.OutcomeSucceeded) {
			t.Fatalf("expected 1 succeeded row, got %+v", rows)
		}
	})

	t.Run("SCN-064-A03_hybrid_internal_and_web", func(t *testing.T) {
		prefix := fmt.Sprintf("tp076-02-08-a03-%d", time.Now().UnixNano())
		cleanupTracesByToolPrefix(t, pool, prefix)

		internalName := prefix + "-internal_retrieval"
		webName := prefix + "-web_search"
		artifactID := prefix + "-art"
		webURL := "https://example.test/" + prefix
		webHash := "deadbeef" + prefix
		r := ok.NewRegistry([]string{internalName, webName})
		if err := r.Register(artifactTool{name: internalName, artifactID: artifactID, snippet: "internal"}); err != nil {
			t.Fatalf("register internal: %v", err)
		}
		if err := r.Register(webTool{name: webName, url: webURL, hash: webHash, snippet: "web"}); err != nil {
			t.Fatalf("register web: %v", err)
		}
		final := fmt.Sprintf(
			`Combined.<CITATIONS>[{"kind":"artifact","artifact_id":%q},{"kind":"web","url":%q,"content_hash":%q}]</CITATIONS>`,
			artifactID, webURL, webHash,
		)
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("c1", internalName, `{"q":"x"}`, 5),
			toolUse("c2", webName, `{"q":"x"}`, 5),
			endTurn(final, 10),
		}}
		a := newAgent(t, fl, r, writer, baseCfg(citeback.EnforcementEnforce))
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		got, err := a.Run(ctx, "hybrid")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != agent.StatusSuccess || len(got.Sources) != 2 {
			t.Fatalf("status=%q sources=%+v", got.Status, got.Sources)
		}
		rows := queryTracesByToolPrefix(t, pool, prefix)
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %+v", rows)
		}
	})

	t.Run("SCN-064-A04_unknown_tool_persists_failed_trace", func(t *testing.T) {
		prefix := fmt.Sprintf("tp076-02-08-a04-%d", time.Now().UnixNano())
		cleanupTracesByToolPrefix(t, pool, prefix)

		unknownName := prefix + "-not_registered"
		// Registry knows about a different tool only; lookup of the
		// unknown name MUST yield ErrToolNotRegistered.
		registeredName := prefix + "-real"
		r := ok.NewRegistry([]string{registeredName})
		if err := r.Register(artifactTool{name: registeredName, artifactID: "x", snippet: "y"}); err != nil {
			t.Fatalf("register: %v", err)
		}
		final := `Nothing to do.<CITATIONS>[]</CITATIONS>`
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("c1", unknownName, `{}`, 5),
			endTurn(final, 5),
		}}
		a := newAgent(t, fl, r, writer, baseCfg(citeback.EnforcementEnforce))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := a.Run(ctx, "ask"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		rows := queryTracesByToolPrefix(t, pool, prefix)
		var sawFailed bool
		for _, row := range rows {
			if row.ToolName == unknownName && row.Outcome == string(tracewriter.OutcomeFailed) {
				sawFailed = true
			}
		}
		if !sawFailed {
			t.Fatalf("expected failed trace for unknown tool, got %+v", rows)
		}
	})

	t.Run("SCN-064-A05_tool_failure_refuses_with_capture", func(t *testing.T) {
		prefix := fmt.Sprintf("tp076-02-08-a05-%d", time.Now().UnixNano())
		cleanupTracesByToolPrefix(t, pool, prefix)

		toolName := prefix + "-circuit"
		r := ok.NewRegistry([]string{toolName})
		if err := r.Register(circuitOpenTool{name: toolName}); err != nil {
			t.Fatalf("register: %v", err)
		}
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("c1", toolName, `{}`, 5),
		}}
		a := newAgent(t, fl, r, writer, baseCfg(citeback.EnforcementEnforce))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		got, err := a.Run(ctx, "ask")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != agent.StatusRefused || got.TerminationReason != agent.TerminationToolUnavailable {
			t.Fatalf("status=%q reason=%q", got.Status, got.TerminationReason)
		}
		rows := queryTracesByToolPrefix(t, pool, prefix)
		var sawRefused bool
		for _, row := range rows {
			if row.Outcome == string(tracewriter.OutcomeRefused) {
				sawRefused = true
			}
		}
		if !sawRefused {
			t.Fatalf("expected at least one refused row, got %+v", rows)
		}
	})

	t.Run("SCN-064-A06_fabricated_source_enforce_vs_shadow", func(t *testing.T) {
		// Build a fabricated-source script once; run it twice — first
		// under enforce, then under shadow — and assert the agent's
		// terminal state flips appropriately.
		runFabricated := func(t *testing.T, mode citeback.EnforcementMode) agent.TurnResult {
			t.Helper()
			prefix := fmt.Sprintf("tp076-02-08-a06-%s-%d", mode, time.Now().UnixNano())
			cleanupTracesByToolPrefix(t, pool, prefix)

			webName := prefix + "-web_search"
			realURL := "https://example.test/real/" + prefix
			r := ok.NewRegistry([]string{webName})
			if err := r.Register(webTool{name: webName, url: realURL, hash: "real-" + prefix, snippet: "real snippet"}); err != nil {
				t.Fatalf("register: %v", err)
			}
			// LLM cites a different URL than the tool returned — fabricated.
			final := fmt.Sprintf(
				`Answer.<CITATIONS>[{"kind":"web","url":"https://example.test/fake/%s","content_hash":"deadbeef"}]</CITATIONS>`,
				prefix,
			)
			fl := &fakeLLM{t: t, responses: []llm.Result{
				toolUse("c1", webName, `{"q":"q"}`, 5),
				endTurn(final, 10),
			}}
			a := newAgent(t, fl, r, writer, baseCfg(mode))
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			got, err := a.Run(ctx, "ask")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			return got
		}

		enforceResult := runFabricated(t, citeback.EnforcementEnforce)
		if enforceResult.Status != agent.StatusRefused {
			t.Fatalf("enforce: Status=%q want refused for fabricated source", enforceResult.Status)
		}
		if enforceResult.TerminationReason != agent.TerminationFabricatedSource {
			t.Fatalf("enforce: TerminationReason=%q want fabricated_source", enforceResult.TerminationReason)
		}
		if len(enforceResult.RejectedCitations) == 0 {
			t.Fatalf("enforce: RejectedCitations empty — verifier rejection MUST be surfaced")
		}

		shadowResult := runFabricated(t, citeback.EnforcementShadow)
		if shadowResult.Status != agent.StatusSuccess {
			t.Fatalf("shadow: Status=%q want success (shadow MUST NOT flip to refusal)", shadowResult.Status)
		}
		if shadowResult.TerminationReason == agent.TerminationFabricatedSource {
			t.Fatalf("shadow: TerminationReason=%q must NOT be fabricated_source", shadowResult.TerminationReason)
		}
		if len(shadowResult.RejectedCitations) == 0 {
			t.Fatalf("shadow: RejectedCitations empty — mismatch MUST remain observable for promotion review")
		}
		// Adversarial: any rejection reason recorded must be a typed
		// citeback sentinel so operators can grep promotion candidates.
		var sawSentinel bool
		for _, rc := range shadowResult.RejectedCitations {
			if errors.Is(rc.Reason, citeback.ReasonNotInTrace) ||
				errors.Is(rc.Reason, citeback.ReasonHashMismatch) ||
				errors.Is(rc.Reason, citeback.ReasonKindMismatch) ||
				errors.Is(rc.Reason, citeback.ReasonMalformedCitation) {
				sawSentinel = true
			}
		}
		if !sawSentinel {
			t.Fatalf("shadow: rejection reasons not typed citeback sentinels, got %+v", shadowResult.RejectedCitations)
		}
	})

	t.Run("SCN-064-A07_web_disabled_falls_back_to_internal", func(t *testing.T) {
		prefix := fmt.Sprintf("tp076-02-08-a07-%d", time.Now().UnixNano())
		cleanupTracesByToolPrefix(t, pool, prefix)

		internalName := prefix + "-internal_retrieval"
		webName := prefix + "-web_search"
		artifactID := prefix + "-art"
		r := ok.NewRegistry([]string{internalName})
		if err := r.Register(artifactTool{name: internalName, artifactID: artifactID, snippet: "internal"}); err != nil {
			t.Fatalf("register internal: %v", err)
		}
		if err := r.Register(webTool{name: webName, url: "https://disabled.test/" + prefix, hash: "h", snippet: "web"}); err != nil {
			t.Fatalf("register web: %v", err)
		}
		final := fmt.Sprintf(
			`From notes.<CITATIONS>[{"kind":"artifact","artifact_id":%q}]</CITATIONS>`,
			artifactID,
		)
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("c1", webName, `{}`, 5),
			toolUse("c2", internalName, `{}`, 5),
			endTurn(final, 10),
		}}
		a := newAgent(t, fl, r, writer, baseCfg(citeback.EnforcementEnforce))
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		got, err := a.Run(ctx, "ask")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != agent.StatusSuccess || len(got.Sources) != 1 || got.Sources[0].Kind != ok.SourceArtifact {
			t.Fatalf("status=%q sources=%+v", got.Status, got.Sources)
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
		if !sawDisabledFailed || !sawInternalSucceeded {
			t.Fatalf("expected disabled-fail + internal-success trace rows, got %+v", rows)
		}
	})

	t.Run("SCN-064-A08_per_user_monthly_budget_preflight_refusal", func(t *testing.T) {
		prefix := fmt.Sprintf("tp076-02-08-a08-%d", time.Now().UnixNano())
		cleanupTracesByToolPrefix(t, pool, prefix)

		// Snapshot the count of 'agent'-tagged refused rows BEFORE
		// dispatch so we can assert the pre-flight refusal added at
		// least one new row regardless of payload shape.
		ctxPre, cancelPre := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelPre()
		var before int
		if err := pool.QueryRow(ctxPre,
			`SELECT COUNT(*) FROM assistant_tool_traces
			   WHERE tool_name = 'agent' AND call_outcome = $1`,
			string(tracewriter.OutcomeRefused),
		).Scan(&before); err != nil {
			t.Fatalf("count before: %v", err)
		}

		toolName := prefix + "-unused"
		r := ok.NewRegistry([]string{toolName})
		if err := r.Register(artifactTool{name: toolName, artifactID: "x", snippet: "y"}); err != nil {
			t.Fatalf("register: %v", err)
		}
		fl := &fakeLLM{t: t /* zero responses */}
		cfg := baseCfg(citeback.EnforcementEnforce)
		cfg.PerUserMonthlyUSDRemaining = 0
		a := newAgent(t, fl, r, writer, cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		got, err := a.Run(ctx, prefix+"-prompt")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != agent.StatusRefused || got.TerminationReason != agent.TerminationCapUSD {
			t.Fatalf("status=%q reason=%q", got.Status, got.TerminationReason)
		}
		if fl.callCount() != 0 {
			t.Fatalf("LLM called %d times; pre-flight refusal MUST short-circuit", fl.callCount())
		}
		// Adversarial: confirm NO succeeded rows for the registered tool.
		rows := queryTracesByToolPrefix(t, pool, prefix)
		for _, row := range rows {
			if row.Outcome == string(tracewriter.OutcomeSucceeded) {
				t.Fatalf("found succeeded row %s after pre-flight refusal: %+v", row.ToolName, row)
			}
		}
		// Confirm the pre-flight refusal added at least one 'agent'
		// refused row to `assistant_tool_traces`. The synthetic 'agent'
		// tool name is shared across pre-flight refusals; a count
		// delta is the safe scope under parallel test runs.
		ctxPost, cancelPost := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelPost()
		var after int
		if err := pool.QueryRow(ctxPost,
			`SELECT COUNT(*) FROM assistant_tool_traces
			   WHERE tool_name = 'agent' AND call_outcome = $1`,
			string(tracewriter.OutcomeRefused),
		).Scan(&after); err != nil {
			t.Fatalf("count after: %v", err)
		}
		if after <= before {
			t.Fatalf("expected at least one new pre-flight refused 'agent' row (before=%d, after=%d)", before, after)
		}
		_ = strings.TrimSpace // keep strings import used regardless of future edits
	})
}
