// Spec 076 SCOPE-2b — TP-076-02-03 (SCN-064-A04).
//
// Unit coverage for the per-turn token budget exhaustion path:
//   - The agent loop terminates with StatusRefused + TerminationCapTokens
//     when the first LLM response blows through PerQueryTokenBudget.
//   - The underlying cap error returned by the BudgetTracker matches
//     BOTH the umbrella ErrBudgetExhausted sentinel AND the specific
//     ErrCapTokens sentinel via errors.Is — this is the contract the
//     spec 076 typed-sentinel rework adds on top of spec 064.
//   - A `call_outcome='refused'` row is emitted through the spec 076
//     SCOPE-2a tracewriter at the moment of refusal so downstream
//     observability and capture-as-fallback can correlate.
//   - FinalText and Sources are empty (no half-answer on cap), and the
//     in-memory ToolTrace is preserved across the cap so the Facade can
//     still apply capture-as-fallback with full context.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

// spyWriter captures every Entry written by the agent so a unit test
// can assert on outcome attribution without standing up Postgres.
type spyWriter struct {
	mu      sync.Mutex
	entries []tracewriter.Entry
}

func (s *spyWriter) Write(_ context.Context, e tracewriter.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return nil
}

func (s *spyWriter) snapshot() []tracewriter.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tracewriter.Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// TestAgent_PerTurnBudgetExhaustionRefusesWithCapture drives the
// agent loop with a tool_use → end_turn script where the end_turn
// LLM response reports a token count past PerQueryTokenBudget. The
// loop MUST refuse, surface TerminationCapTokens, emit a refused
// tracewriter row, and preserve the in-memory tool trace.
func TestAgent_PerTurnBudgetExhaustionRefusesWithCapture(t *testing.T) {
	spy := &spyWriter{}
	calcArgs := `{"expression":"2+2"}`
	overrunFinal := "verified partial answer 4 <CITATIONS>[]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", "calculator", calcArgs, 10),
		endTurn(overrunFinal, 5000 /* blows PerQueryTokenBudget=100 */),
	}}
	r := newRegistry(t)
	cfg := baseCfg(5, 100 /*token cap*/, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 })
	cfg.TraceWriter = spy

	a, err := New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "what is 2+2")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got.Status != StatusRefused {
		t.Fatalf("Status=%q want refused (reason=%q)", got.Status, got.RefusalReason)
	}
	if got.TerminationReason != TerminationCapTokens {
		t.Errorf("TerminationReason=%q want cap_tokens", got.TerminationReason)
	}
	if got.FinalText != "" {
		t.Errorf("FinalText=%q want empty (no partial answer on cap)", got.FinalText)
	}
	if len(got.Sources) != 0 {
		t.Errorf("Sources=%+v want empty", got.Sources)
	}
	if len(got.ToolTrace) != 1 {
		t.Fatalf("ToolTrace=%d want 1 (preserved across cap for capture context)", len(got.ToolTrace))
	}

	entries := spy.snapshot()
	var (
		sawSucceeded bool
		refused      *tracewriter.Entry
	)
	for i := range entries {
		switch entries[i].CallOutcome {
		case tracewriter.OutcomeSucceeded:
			sawSucceeded = true
		case tracewriter.OutcomeRefused:
			e := entries[i]
			refused = &e
		}
	}
	if !sawSucceeded {
		t.Errorf("expected a succeeded trace row for the calculator call, entries=%+v", entries)
	}
	if refused == nil {
		t.Fatalf("expected a refused trace row at budget-exhaustion, entries=%+v", entries)
	}
	if refused.ToolName == "" {
		t.Errorf("refused row missing tool_name attribution")
	}
	if refused.ErrorCode != string(TerminationCapTokens) {
		t.Errorf("refused row ErrorCode=%q want %q", refused.ErrorCode, string(TerminationCapTokens))
	}
}

// TestBudgetTracker_CapErrorsWrapErrBudgetExhausted asserts the spec
// 076 SCOPE-2b sentinel contract: every per-cap error returned by the
// BudgetTracker matches BOTH the umbrella ErrBudgetExhausted AND the
// specific per-cap sentinel via errors.Is. Callers that only care
// "some budget tripped" can match on ErrBudgetExhausted without
// losing the ability to attribute to a specific cap.
func TestBudgetTracker_CapErrorsWrapErrBudgetExhausted(t *testing.T) {
	cases := []struct {
		name   string
		build  func(t *testing.T) error
		target error
	}{
		{
			name: "tokens",
			build: func(t *testing.T) error {
				b, err := ok.NewBudgetTracker(10, 1.0, 1.0, 1.0)
				if err != nil {
					t.Fatalf("NewBudgetTracker: %v", err)
				}
				return b.RecordLLMCall(0, 1000, 0)
			},
			target: ok.ErrCapTokens,
		},
		{
			name: "usd_per_query",
			build: func(t *testing.T) error {
				b, err := ok.NewBudgetTracker(1000, 0.5, 1.0, 1.0)
				if err != nil {
					t.Fatalf("NewBudgetTracker: %v", err)
				}
				return b.RecordLLMCall(0, 1, 1.0)
			},
			target: ok.ErrCapUSDPerQuery,
		},
		{
			name: "usd_monthly",
			build: func(t *testing.T) error {
				b, err := ok.NewBudgetTracker(1000, 10.0, 0.5, 10.0)
				if err != nil {
					t.Fatalf("NewBudgetTracker: %v", err)
				}
				return b.RecordLLMCall(0, 1, 1.0)
			},
			target: ok.ErrCapUSDMonthly,
		},
		{
			name: "usd_per_user_month",
			build: func(t *testing.T) error {
				b, err := ok.NewBudgetTracker(1000, 10.0, 10.0, 0.5)
				if err != nil {
					t.Fatalf("NewBudgetTracker: %v", err)
				}
				return b.RecordLLMCall(0, 1, 1.0)
			},
			target: ok.ErrCapUSDPerUserMonth,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build(t)
			if err == nil {
				t.Fatalf("expected cap error, got nil")
			}
			if !errors.Is(err, ok.ErrBudgetExhausted) {
				t.Errorf("errors.Is(%v, ErrBudgetExhausted)=false", err)
			}
			if !errors.Is(err, tc.target) {
				t.Errorf("errors.Is(%v, %v)=false", err, tc.target)
			}
		})
	}
}

// TestRegistry_TypedSentinelAliases pins the spec 076 SCOPE-2b
// sentinel-alias contract: ErrToolNotRegistered and ErrToolDisabled
// are matched by the same errors.Is chain as ErrUnknownTool /
// ErrToolNotAllowed.
func TestRegistry_TypedSentinelAliases(t *testing.T) {
	r := ok.NewRegistry([]string{"allowed_tool"})
	stub := stubTool{name: "registered_but_disabled"}
	if err := r.Register(stub); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := r.Lookup("never_registered"); err == nil || !errors.Is(err, ok.ErrToolNotRegistered) {
		t.Fatalf("Lookup(unknown): err=%v want match ErrToolNotRegistered", err)
	}
	if _, err := r.Lookup("registered_but_disabled"); err == nil || !errors.Is(err, ok.ErrToolDisabled) {
		t.Fatalf("Lookup(disabled): err=%v want match ErrToolDisabled", err)
	}
}

type stubTool struct{ name string }

func (s stubTool) Name() string                { return s.name }
func (stubTool) Description() string           { return "stub" }
func (stubTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (stubTool) Execute(context.Context, json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{}, nil
}
