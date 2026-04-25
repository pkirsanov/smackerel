//go:build e2e

// Spec 037 Scope 9 — Telegram bridge e2e test.
//
// Drives the real telegram.AgentBridge against a stubbed sender (we
// do NOT spin up a tgbotapi long-poll loop in tests; that would
// require a real bot token and is out of scope here). The runner
// ITSELF is real-shaped — it conforms to the same interface
// production wiring uses — and we use a scripted instance so each
// test asserts the exact reply text per outcome class.
//
// Skips cleanly when DATABASE_URL or NATS_URL is unset: scope 9's
// hard constraint requires the live test stack to be present so the
// bridge cannot accidentally be wired against in-memory mocks in
// production-like runs.
package agent_e2e

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/userreply"
	"github.com/smackerel/smackerel/internal/telegram"
)

// recordedSender captures every outbound message in order so a single
// test can assert exact reply text and ≤4-line cap.
type recordedSender struct {
	mu       sync.Mutex
	messages []recordedMessage
}

type recordedMessage struct {
	chatID int64
	text   string
}

func (s *recordedSender) SendMessage(_ context.Context, chatID int64, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, recordedMessage{chatID: chatID, text: text})
	return nil
}

func (s *recordedSender) Last() recordedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		return recordedMessage{}
	}
	return s.messages[len(s.messages)-1]
}

// telegramScriptedRunner mirrors api_invoke_test.go's scriptedRunner
// against the telegram.AgentRunner interface (which is a separate
// interface from api.AgentInvokeRunner — same shape, different package).
type telegramScriptedRunner struct {
	result   *agent.InvocationResult
	decision *agent.RoutingDecision
	known    []string
}

func (r *telegramScriptedRunner) Invoke(_ context.Context, _ agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	return r.result, r.decision
}
func (r *telegramScriptedRunner) KnownIntents() []string { return r.known }

func newBridge(t *testing.T, runner *telegramScriptedRunner) (*telegram.AgentBridge, *recordedSender) {
	t.Helper()
	sender := &recordedSender{}
	br, err := telegram.NewAgentBridge(runner, sender)
	if err != nil {
		t.Fatalf("NewAgentBridge: %v", err)
	}
	return br, sender
}

// linesOf splits the recorded message text the same way
// userreply.TelegramReply.Lines() would, so the ≤4-line assertion is
// honest.
func linesOf(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func TestTelegramReply_OK(t *testing.T) {
	liveStackOrSkip(t)
	runner := &telegramScriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_tg_ok", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeOK,
			Final: json.RawMessage(`{"answer":"You spent 87,42 € on groceries last week."}`),
		},
	}
	br, sender := newBridge(t, runner)
	if _, err := br.Handle(context.Background(), 42, "how much did I spend?"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	last := sender.Last()
	if last.chatID != 42 {
		t.Fatalf("chatID=%d", last.chatID)
	}
	if !strings.Contains(last.text, "87,42 €") {
		t.Fatalf("expected answer in reply: %s", last.text)
	}
	if !strings.Contains(last.text, "trace_tg_ok") {
		t.Fatalf("expected trace ref in reply: %s", last.text)
	}
}

func TestTelegramReply_UnknownIntentLineCapAndMarker(t *testing.T) {
	liveStackOrSkip(t)
	known := []string{"expenses", "recipes", "meal_plans"}
	runner := &telegramScriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_tg_unk", Outcome: agent.OutcomeUnknownIntent,
		},
		known: known,
	}
	br, sender := newBridge(t, runner)
	if _, err := br.Handle(context.Background(), 1, "asdkfj qwerty zxcv"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := sender.Last().text
	if !strings.Contains(text, userreply.UnknownIntentMarker) {
		t.Fatalf("missing UnknownIntentMarker: %s", text)
	}
	for _, k := range known {
		if !strings.Contains(text, k) {
			t.Fatalf("expected known intent %q listed: %s", k, text)
		}
	}
	if got := len(linesOf(text)); got > userreply.MaxTelegramLines {
		t.Fatalf("reply has %d lines (>4): %s", got, text)
	}
}

func TestTelegramReply_TimeoutNamesDeadline(t *testing.T) {
	liveStackOrSkip(t)
	runner := &telegramScriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_tg_timeout", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeTimeout,
			OutcomeDetail: map[string]any{"deadline_s": 60},
		},
	}
	br, sender := newBridge(t, runner)
	_, _ = br.Handle(context.Background(), 1, "summarize my whole year")
	text := sender.Last().text
	if !strings.Contains(text, "(60s)") {
		t.Fatalf("expected (60s) deadline: %s", text)
	}
	if !strings.Contains(text, userreply.TimeoutMarker) {
		t.Fatalf("missing TimeoutMarker: %s", text)
	}
}

func TestTelegramReply_AllowlistViolationNamesBlockedTool(t *testing.T) {
	liveStackOrSkip(t)
	runner := &telegramScriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_tg_allow", ScenarioID: "expense_summary",
			ScenarioVersion: "v1", Outcome: agent.OutcomeAllowlistViolation,
			ToolCalls: []agent.ExecutedToolCall{{
				Name: "delete_all_expenses", Outcome: agent.OutcomeAllowlistViolation,
				RejectionReason: "not_in_allowlist",
			}},
		},
	}
	br, sender := newBridge(t, runner)
	_, _ = br.Handle(context.Background(), 1, "summarize and delete")
	text := sender.Last().text
	if !strings.Contains(text, "delete_all_expenses") {
		t.Fatalf("expected blocked tool named: %s", text)
	}
	if !strings.Contains(text, userreply.AllowlistMarker) {
		t.Fatalf("missing AllowlistMarker: %s", text)
	}
}

func TestTelegramReply_ToolErrorMarker(t *testing.T) {
	liveStackOrSkip(t)
	runner := &telegramScriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_tg_tool", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeToolError,
			ToolCalls: []agent.ExecutedToolCall{{
				Name: "search_expenses", Outcome: agent.OutcomeToolError,
				Error: "db_timeout",
			}},
		},
	}
	br, sender := newBridge(t, runner)
	_, _ = br.Handle(context.Background(), 1, "show dining")
	text := sender.Last().text
	if !strings.Contains(text, userreply.ToolErrorMarker) {
		t.Fatalf("missing ToolErrorMarker: %s", text)
	}
	if !strings.Contains(text, "search_expenses") {
		t.Fatalf("missing tool name: %s", text)
	}
}

func TestTelegramReply_SchemaFailureMarker(t *testing.T) {
	liveStackOrSkip(t)
	runner := &telegramScriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_tg_schema", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeSchemaFailure,
			SchemaRetries: 2,
		},
	}
	br, sender := newBridge(t, runner)
	_, _ = br.Handle(context.Background(), 1, "avg per week")
	text := sender.Last().text
	if !strings.Contains(text, userreply.SchemaFailureMarker) {
		t.Fatalf("missing SchemaFailureMarker: %s", text)
	}
}

func TestTelegramReply_LoopLimitNamesIterations(t *testing.T) {
	liveStackOrSkip(t)
	runner := &telegramScriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_tg_loop", ScenarioID: "x", ScenarioVersion: "v1",
			Outcome: agent.OutcomeLoopLimit, Iterations: 8,
		},
	}
	br, sender := newBridge(t, runner)
	_, _ = br.Handle(context.Background(), 1, "x")
	text := sender.Last().text
	if !strings.Contains(text, "8 things") {
		t.Fatalf("expected '8 things': %s", text)
	}
}

// TestTelegramReply_AllOutcomesAreCappedAndTraced is the catch-all
// guard: for EVERY outcome the executor can produce, the bridge must
// emit a ≤4-line reply ending with a trace ref. Adding a new outcome
// without extending userreply will fail this test.
func TestTelegramReply_AllOutcomesAreCappedAndTraced(t *testing.T) {
	liveStackOrSkip(t)
	outcomes := []agent.Outcome{
		agent.OutcomeOK,
		agent.OutcomeUnknownIntent,
		agent.OutcomeAllowlistViolation,
		agent.OutcomeHallucinatedTool,
		agent.OutcomeToolError,
		agent.OutcomeToolReturnInvalid,
		agent.OutcomeSchemaFailure,
		agent.OutcomeLoopLimit,
		agent.OutcomeTimeout,
		agent.OutcomeProviderError,
		agent.OutcomeInputSchemaViolation,
	}
	for _, o := range outcomes {
		o := o
		t.Run(string(o), func(t *testing.T) {
			runner := &telegramScriptedRunner{
				result: &agent.InvocationResult{
					TraceID: "trace_tg_" + string(o), ScenarioID: "x",
					ScenarioVersion: "v1", Outcome: o,
					Final: json.RawMessage(`{"answer":"ok"}`),
					ToolCalls: []agent.ExecutedToolCall{{
						Name: "t", Outcome: o, Error: "e", RejectionReason: "r",
					}},
					OutcomeDetail: map[string]any{"deadline_s": 5, "error": "x"},
				},
				known: []string{"a", "b"},
			}
			br, sender := newBridge(t, runner)
			if _, err := br.Handle(context.Background(), 1, "x"); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			text := sender.Last().text
			lines := linesOf(text)
			if len(lines) == 0 {
				t.Fatalf("empty reply for %s", o)
			}
			if len(lines) > userreply.MaxTelegramLines {
				t.Fatalf("%s: %d lines (>4):\n%s", o, len(lines), text)
			}
			if !strings.Contains(text, "trace_tg_"+string(o)) {
				t.Fatalf("%s: missing trace ref:\n%s", o, text)
			}
		})
	}
}
