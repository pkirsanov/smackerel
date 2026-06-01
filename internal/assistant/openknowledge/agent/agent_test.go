package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
)

// fakeLLM returns a programmed sequence of llm.Result responses. Each
// Chat call pops the next response; an empty queue is a test failure.
type fakeLLM struct {
	responses []llm.Result
	calls     int
	t         *testing.T
}

func (f *fakeLLM) Chat(_ context.Context, _ llm.ChatRequest) (llm.Result, error) {
	if f.calls >= len(f.responses) {
		f.t.Fatalf("fakeLLM: unexpected call #%d (queue exhausted)", f.calls+1)
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

// fakeWebTool returns a fixed web snippet + source.
type fakeWebTool struct {
	url, hash, snippet string
}

func (fakeWebTool) Name() string                  { return "fake_web" }
func (fakeWebTool) Description() string           { return "fake web search for tests" }
func (fakeWebTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f fakeWebTool) Execute(_ context.Context, _ json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{
		Snippets: []ok.Snippet{{Text: f.snippet, ContentHash: f.hash, SourceRef: f.url}},
		Sources: []ok.Source{{
			Kind: ok.SourceWeb,
			Web:  &ok.WebSource{URL: f.url, ContentHash: f.hash, Provider: "fake", Snippet: f.snippet},
		}},
	}, nil
}

// newRegistry returns a registry with the deterministic calculator and
// the fake web tool, both allowlisted.
func newRegistry(t *testing.T, extras ...ok.Tool) *ok.Registry {
	t.Helper()
	names := []string{"calculator", "fake_web"}
	for _, e := range extras {
		names = append(names, e.Name())
	}
	r := ok.NewRegistry(names)
	if err := r.Register(tools.NewCalculator()); err != nil {
		t.Fatalf("register calc: %v", err)
	}
	if err := r.Register(fakeWebTool{url: "https://example.test/x", hash: "deadbeef", snippet: "hello"}); err != nil {
		t.Fatalf("register web: %v", err)
	}
	for _, e := range extras {
		if err := r.Register(e); err != nil {
			t.Fatalf("register extra: %v", err)
		}
	}
	return r
}

func baseCfg(maxIter, tokens int, perQueryUSD, monthlyUSD, perUserUSD, ratio float64, cost CostFn) Config {
	return Config{
		SystemPrompt:               "test-system-prompt",
		Model:                      "test-model",
		MaxIterations:              maxIter,
		PerQueryTokenBudget:        tokens,
		PerQueryUSDBudget:          perQueryUSD,
		MonthlyBudgetUSDRemaining:  monthlyUSD,
		PerUserMonthlyUSDRemaining: perUserUSD,
		CompactionThresholdRatio:   ratio,
		CostFn:                     cost,
	}
}

func textPtr(s string) *string { return &s }

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

// TestAgent_HappyPath_CalculatorThenEndTurn — LLM requests a calculator
// call, sees the result, and ends the turn with a verifiable
// tool_computation citation.
func TestAgent_HappyPath_CalculatorThenEndTurn(t *testing.T) {
	calcArgs := `{"expression":"2+2"}`
	canonInput := `{"expression":"2+2"}`
	canonOutput := `{"result":4}`

	final := fmt.Sprintf(
		"The answer is 4.\n<CITATIONS>[{\"kind\":\"tool_computation\",\"tool\":\"calculator\",\"input\":%s,\"output\":%s}]</CITATIONS>",
		canonInput, canonOutput)

	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", "calculator", calcArgs, 10),
		endTurn(final, 20),
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "what is 2+2")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q rejected=%+v reason=%q", got.Status, got.RejectedCitations, got.RefusalReason)
	}
	if got.TerminationReason != TerminationFinal {
		t.Errorf("TerminationReason=%q want final", got.TerminationReason)
	}
	if got.FinalText != "The answer is 4." {
		t.Errorf("FinalText=%q want %q", got.FinalText, "The answer is 4.")
	}
	if len(got.Sources) != 1 || got.Sources[0].Kind != ok.SourceToolComputation {
		t.Errorf("Sources=%+v", got.Sources)
	}
	if len(got.ToolTrace) != 1 || got.ToolTrace[0].ToolName != "calculator" {
		t.Errorf("ToolTrace=%+v", got.ToolTrace)
	}
	if got.TokensUsed != 30 {
		t.Errorf("TokensUsed=%d want 30", got.TokensUsed)
	}
}

// TestAgent_IterationCap — LLM never ends the turn; loop terminates at
// MaxIterations with cap_iterations and empty FinalText (capture is
// the Facade's job, not the agent's).
func TestAgent_IterationCap(t *testing.T) {
	const maxIter = 3
	resp := make([]llm.Result, maxIter)
	for i := range resp {
		resp[i] = toolUse(fmt.Sprintf("c%d", i), "calculator", `{"expression":"1+1"}`, 5)
	}
	fl := &fakeLLM{t: t, responses: resp}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(maxIter, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusRefused {
		t.Errorf("Status=%q want refused", got.Status)
	}
	if got.TerminationReason != TerminationCapIterations {
		t.Errorf("TerminationReason=%q want cap_iterations", got.TerminationReason)
	}
	if got.FinalText != "" {
		t.Errorf("FinalText=%q want empty", got.FinalText)
	}
	if fl.calls != maxIter {
		t.Errorf("LLM calls=%d want %d", fl.calls, maxIter)
	}
}

// TestAgent_TokenCap — single LLM response exceeds PerQueryTokenBudget.
func TestAgent_TokenCap(t *testing.T) {
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("ignored", 100)}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 50 /*tokens*/, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "big")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.TerminationReason != TerminationCapTokens {
		t.Errorf("TerminationReason=%q want cap_tokens", got.TerminationReason)
	}
	if got.Status != StatusRefused || got.FinalText != "" {
		t.Errorf("expected refused+empty, got status=%q text=%q", got.Status, got.FinalText)
	}
}

// TestAgent_USDCap — token-cheap but USD-expensive response.
func TestAgent_USDCap(t *testing.T) {
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("ignored", 1)}}
	r := newRegistry(t)
	cost := func(_ int) float64 { return 5.0 } // exceeds PerQueryUSD=1
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, cost))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "expensive")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.TerminationReason != TerminationCapUSD {
		t.Errorf("TerminationReason=%q want cap_usd", got.TerminationReason)
	}
}

// TestAgent_ToolLookupError_Recoverable — unknown tool name is reported
// back to the planner, which then ends the turn successfully.
func TestAgent_ToolLookupError_Recoverable(t *testing.T) {
	final := "Recovered: 1+1=2.\n<CITATIONS>[{\"kind\":\"tool_computation\",\"tool\":\"calculator\",\"input\":{\"expression\":\"1+1\"},\"output\":{\"result\":2}}]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("u1", "no_such_tool", `{}`, 5),
		toolUse("c1", "calculator", `{"expression":"1+1"}`, 5),
		endTurn(final, 10),
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q rejected=%+v", got.Status, got.RefusalReason, got.RejectedCitations)
	}
	if len(got.ToolTrace) != 2 {
		t.Errorf("ToolTrace len=%d want 2", len(got.ToolTrace))
	}
	if got.ToolTrace[0].Err == nil {
		t.Errorf("first trace entry should record the lookup error")
	}
}

// TestAgent_ToolExecError_Recoverable — calculator divide-by-zero is
// surfaced to the planner, which recovers.
func TestAgent_ToolExecError_Recoverable(t *testing.T) {
	final := "Cannot divide by zero; using fallback 0.\n<CITATIONS>[]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", "calculator", `{"expression":"1/0"}`, 5),
		endTurn(final, 5),
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q", got.Status, got.RefusalReason)
	}
	if len(got.ToolTrace) != 1 {
		t.Fatalf("ToolTrace len=%d want 1", len(got.ToolTrace))
	}
	// Calculator surfaces the error via ToolResult.Error (not execErr).
	if got.ToolTrace[0].Result == nil || got.ToolTrace[0].Result.Error == nil {
		t.Errorf("expected ToolResult.Error from calculator divide-by-zero")
	}
}

// TestAgent_FabricatedSource — LLM cites a URL the trace never saw.
// G021 adversarial: this MUST be rejected as fabricated.
func TestAgent_FabricatedSource(t *testing.T) {
	final := `Per the source, the sky is green.<CITATIONS>[{"kind":"web","url":"https://made-up.test/never","content_hash":"00ff"}]</CITATIONS>`
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn(final, 10)}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusRefused {
		t.Fatalf("Status=%q want refused", got.Status)
	}
	if got.TerminationReason != TerminationFabricatedSource {
		t.Errorf("TerminationReason=%q want fabricated_source", got.TerminationReason)
	}
	if got.FinalText != "" || len(got.Sources) != 0 {
		t.Errorf("expected empty FinalText and Sources, got text=%q sources=%+v", got.FinalText, got.Sources)
	}
	if len(got.RejectedCitations) != 1 || !errors.Is(got.RejectedCitations[0].Reason, citeback.ReasonNotInTrace) {
		t.Errorf("RejectedCitations=%+v", got.RejectedCitations)
	}
}

// TestAgent_ZeroToolAnswerWithCitation_AdversarialG021 — LLM ends the
// turn with a citation but never called a tool. MUST be rejected:
// citations cannot exist without a tool trace.
func TestAgent_ZeroToolAnswerWithCitation_AdversarialG021(t *testing.T) {
	final := `According to internal records, X is true.<CITATIONS>[{"kind":"artifact","artifact_id":"art-1"}]</CITATIONS>`
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn(final, 5)}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusRefused || got.TerminationReason != TerminationFabricatedSource {
		t.Fatalf("expected refused+fabricated_source, got status=%q reason=%q", got.Status, got.TerminationReason)
	}
	if len(got.ToolTrace) != 0 {
		t.Errorf("expected empty trace, got %+v", got.ToolTrace)
	}
}

// TestAgent_BudgetExhaustedMidLoop_NoPartialAnswer_AdversarialG021 —
// after a successful tool call AND with a verifiable partial answer
// theoretically producible, the LLM's next response exhausts the
// budget. Per SCOPE-09 user spec (overrides design §loop "partial
// answer" branch): the loop MUST NOT return a partial answer; it
// returns TerminationCap* with empty FinalText so the Facade can apply
// capture-as-fallback.
func TestAgent_BudgetExhaustedMidLoop_NoPartialAnswer_AdversarialG021(t *testing.T) {
	calcArgs := `{"expression":"2+2"}`
	// The "would-be partial" answer the agent must NOT return:
	wouldBePartial := fmt.Sprintf(
		"Verified partial: 4. <CITATIONS>[{\"kind\":\"tool_computation\",\"tool\":\"calculator\",\"input\":%s,\"output\":%s}]</CITATIONS>",
		calcArgs, `{"result":4}`)
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", "calculator", calcArgs, 10),
		endTurn(wouldBePartial, 9999), // huge token count blows the cap
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 100 /*token cap*/, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.TerminationReason != TerminationCapTokens {
		t.Errorf("TerminationReason=%q want cap_tokens", got.TerminationReason)
	}
	if got.Status != StatusRefused {
		t.Errorf("Status=%q want refused", got.Status)
	}
	if got.FinalText != "" {
		t.Errorf("FinalText=%q want empty (no half-answer on cap)", got.FinalText)
	}
	if len(got.Sources) != 0 {
		t.Errorf("Sources=%+v want empty", got.Sources)
	}
	// The successful tool trace IS preserved for observability + Facade
	// capture-as-fallback context.
	if len(got.ToolTrace) != 1 {
		t.Errorf("ToolTrace=%d want 1 (preserved across cap)", len(got.ToolTrace))
	}
}

// TestAgent_CompactionSignalAtThreshold — once accumulated tokens cross
// the threshold ratio of PerQueryTokenBudget, CompactionSignaled=true.
func TestAgent_CompactionSignalAtThreshold(t *testing.T) {
	final := "Done.\n<CITATIONS>[]</CITATIONS>"
	// PerQueryTokenBudget=100, ratio=0.5 → threshold=50. Response uses 60.
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn(final, 60)}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 100, 1.0, 10.0, 10.0, 0.5, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q", got.Status, got.RefusalReason)
	}
	if !got.CompactionSignaled {
		t.Errorf("CompactionSignaled=false; tokens=%d threshold=%d", got.TokensUsed, 50)
	}
}

// fakeCircuitOpenTool returns a ToolResult with the well-known
// circuit-open error code so the agent's SCOPE-16 handling can fire
// without standing up a real WebSearchProvider + breaker stack.
type fakeCircuitOpenTool struct{}

func (fakeCircuitOpenTool) Name() string        { return "circuit_open_tool" }
func (fakeCircuitOpenTool) Description() string { return "test stub" }
func (fakeCircuitOpenTool) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (fakeCircuitOpenTool) Execute(_ context.Context, _ json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{Error: &ok.ToolError{
		Code:    ToolErrorCodeCircuitOpen,
		Message: "web search provider circuit breaker is open",
	}}, nil
}

// TestAgent_CircuitOpen_TerminatesToolUnavailable — when a tool
// surfaces ToolError{Code: ToolErrorCodeCircuitOpen}, the agent
// terminates the turn with TerminationToolUnavailable and an empty
// FinalText so the Telegram surface can apply capture-as-fallback.
// SCOPE-16 + G021: this is the resilience path that protects the
// LLM from spinning on a known-down provider.
func TestAgent_CircuitOpen_TerminatesToolUnavailable(t *testing.T) {
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", "circuit_open_tool", `{}`, 5),
	}}
	r := newRegistry(t, fakeCircuitOpenTool{})
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusRefused {
		t.Fatalf("Status=%q want refused", got.Status)
	}
	if got.TerminationReason != TerminationToolUnavailable {
		t.Errorf("TerminationReason=%q want %q", got.TerminationReason, TerminationToolUnavailable)
	}
	if got.FinalText != "" {
		t.Errorf("FinalText=%q want empty (capture-as-fallback contract)", got.FinalText)
	}
	if cause := terminationToRefusalCause(got.TerminationReason); cause != "tool_unavailable" {
		t.Errorf("terminationToRefusalCause=%q want tool_unavailable", cause)
	}
	if len(got.ToolTrace) != 1 {
		t.Fatalf("ToolTrace len=%d want 1", len(got.ToolTrace))
	}
}

// TestAgent_CircuitOpen_DoesNotLeakUnrelatedErrorCodes_AdversarialG021 —
// the agent MUST only terminate on the exact circuit-open code, not
// on any other tool error (which is recoverable and lets the planner
// retry / pivot). A regression that broadened the check would
// short-circuit normal recoverable errors.
func TestAgent_CircuitOpen_DoesNotLeakUnrelatedErrorCodes_AdversarialG021(t *testing.T) {
	final := "Cannot divide by zero; using fallback 0.\n<CITATIONS>[]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", "calculator", `{"expression":"1/0"}`, 5),
		endTurn(final, 5),
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q — divide-by-zero must remain recoverable", got.Status, got.TerminationReason)
	}
	if got.TerminationReason == TerminationToolUnavailable {
		t.Errorf("TerminationReason=%q must NOT be TerminationToolUnavailable for a non-circuit error", got.TerminationReason)
	}
}

// TestAgent_New_RejectsInvalidConfig — every required field is
// validated fail-loud (G028 NO-DEFAULTS).
func TestAgent_New_RejectsInvalidConfig(t *testing.T) {
	good := baseCfg(5, 100, 1, 10, 10, 0.8, func(int) float64 { return 0 })
	mut := func(f func(c *Config)) Config { c := good; f(&c); return c }
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"no system prompt", mut(func(c *Config) { c.SystemPrompt = "" }), "SystemPrompt is required"},
		{"whitespace system prompt", mut(func(c *Config) { c.SystemPrompt = "   \t\n" }), "SystemPrompt is required"},
		{"no model", mut(func(c *Config) { c.Model = "" }), "Model is required"},
		{"max iter", mut(func(c *Config) { c.MaxIterations = 0 }), "MaxIterations"},
		{"token budget", mut(func(c *Config) { c.PerQueryTokenBudget = 0 }), "PerQueryTokenBudget"},
		{"per-query usd neg", mut(func(c *Config) { c.PerQueryUSDBudget = -1 }), "PerQueryUSDBudget"},
		{"threshold zero", mut(func(c *Config) { c.CompactionThresholdRatio = 0 }), "CompactionThresholdRatio"},
		{"threshold >1", mut(func(c *Config) { c.CompactionThresholdRatio = 1.5 }), "CompactionThresholdRatio"},
		{"no costfn", mut(func(c *Config) { c.CostFn = nil }), "CostFn"},
	}
	r := ok.NewRegistry(nil)
	fl := &fakeLLM{t: t}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(fl, r, citeback.Verify, tc.cfg)
			if err == nil || !errors.Is(err, ErrAgentInvalid) {
				t.Fatalf("err=%v want ErrAgentInvalid", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err=%q want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// ── BudgetTracker tests ──────────────────────────────────────────────

func TestBudgetTracker_New_Validation(t *testing.T) {
	cases := []struct {
		name    string
		tokens  int
		usd     float64
		monthly float64
		perUser float64
	}{
		{"zero tokens", 0, 1, 1, 1},
		{"neg usd", 1, -1, 1, 1},
		{"neg monthly", 1, 1, -1, 1},
		{"neg per-user", 1, 1, 1, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ok.NewBudgetTracker(tc.tokens, tc.usd, tc.monthly, tc.perUser)
			if !errors.Is(err, ok.ErrBudgetInvalid) {
				t.Fatalf("err=%v want ErrBudgetInvalid", err)
			}
		})
	}
}

func TestBudgetTracker_CapsFireInOrder(t *testing.T) {
	b, err := ok.NewBudgetTracker(100, 1.0, 10.0, 5.0)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// First call under all caps.
	if err := b.RecordLLMCall(40, 40, 0.5); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if b.TokensUsed() != 80 || b.USDSpent() != 0.5 {
		t.Errorf("accumulators wrong: tokens=%d usd=%v", b.TokensUsed(), b.USDSpent())
	}
	// Second call breaks token cap (80+30=110 > 100).
	if err := b.RecordLLMCall(15, 15, 0.1); !errors.Is(err, ok.ErrCapTokens) {
		t.Errorf("err=%v want ErrCapTokens", err)
	}
}

func TestBudgetTracker_PerUserMonthlyCapFires(t *testing.T) {
	b, err := ok.NewBudgetTracker(1000, 100.0, 100.0, 0.5)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := b.RecordLLMCall(1, 1, 1.0); !errors.Is(err, ok.ErrCapUSDPerUserMonth) {
		t.Errorf("err=%v want ErrCapUSDPerUserMonth", err)
	}
}

func TestBudgetTracker_Remaining(t *testing.T) {
	b, _ := ok.NewBudgetTracker(100, 2.0, 5.0, 3.0)
	_ = b.RecordLLMCall(10, 20, 1.0)
	if b.RemainingTokens() != 70 {
		t.Errorf("RemainingTokens=%d want 70", b.RemainingTokens())
	}
	if b.RemainingUSDPerQuery() != 1.0 {
		t.Errorf("RemainingUSDPerQuery=%v want 1.0", b.RemainingUSDPerQuery())
	}
	if b.PerQueryTokenBudget() != 100 {
		t.Errorf("PerQueryTokenBudget=%d", b.PerQueryTokenBudget())
	}
}

// ── citation parser unit tests ───────────────────────────────────────

func TestParseCitations_MissingBlock(t *testing.T) {
	_, _, err := parseCitations("just a plain answer")
	if err == nil {
		t.Fatalf("expected error for missing CITATIONS block")
	}
}

func TestParseCitations_Empty(t *testing.T) {
	text, cites, err := parseCitations("answer body\n<CITATIONS>[]</CITATIONS>")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if text != "answer body" {
		t.Errorf("text=%q", text)
	}
	if len(cites) != 0 {
		t.Errorf("cites=%+v", cites)
	}
}

func TestParseCitations_UnknownKind(t *testing.T) {
	_, _, err := parseCitations(`x<CITATIONS>[{"kind":"weird"}]</CITATIONS>`)
	if err == nil {
		t.Fatalf("expected error for unknown citation kind")
	}
}

// ensure textPtr / llm.Result helpers compile in import-clean way
var _ = textPtr
var _ json.RawMessage
