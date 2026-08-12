// Regression guard for BUG-069-004 — turn identity on short-circuit paths.
//
// The defect: `internal/assistant/httpadapter` derived both wire trace ids
// from `resp.Invocation`, which is legitimately nil on every short-circuit
// path (contracts/response.go). The deterministic `/weather` fast-path
// (spec 061 SCOPE-03) returns StatusAnswered with no Invocation, so every
// weather turn shipped `assistant_turn_id: ""` and `agent_trace_id: ""` —
// an answered, correct turn that could not be traced from /metrics to the
// log line to the conversation row.
//
// The fix moved turn identity to a chokepoint in Facade.Handle so identity
// belongs to the TURN rather than to whether an LLM invocation happened.
// These tests pin that: a short-circuit response MUST still carry a
// populated, correctly-paired turn identity.
//
// Non-vacuity: each test asserts `Invocation == nil` BEFORE asserting the
// ids. Without that assertion a future change that routed `/weather`
// through the executor would keep these tests green while silently
// abandoning the short-circuit case they exist to protect — the test would
// still pass, but it would no longer be testing anything.

package assistant

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestFacadeShortCircuitTurnCarriesIdentity_BUG069004 — the /weather
// fast-path bypasses the executor (nil Invocation) and MUST still emit a
// non-empty assistant turn id and a paired agent trace id.
func TestFacadeShortCircuitTurnCarriesIdentity_BUG069004(t *testing.T) {
	t.Parallel()

	weatherJSON := []byte(`{"forecast_line":"Beverly Hills, CA: clear, 22°C","provider_name":"open-meteo","retrieved_at":"2026-07-22T15:00:00Z"}`)
	f, executor, _ := weatherShortcutFacade(t, func(_ context.Context, _ string) (json.RawMessage, error) {
		return json.RawMessage(weatherJSON), nil
	})

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-turnid-1", Transport: "web", Text: "/weather 90210", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err: %v", err)
	}

	// --- Non-vacuity guard -------------------------------------------------
	// These two assertions prove we are genuinely exercising the
	// short-circuit path. If either regresses, the identity assertions
	// below would be testing the ordinary executor path instead and would
	// no longer guard this bug.
	if executor.invocations != 0 {
		t.Fatalf("executor invoked %d times; this test only guards the SHORT-CIRCUIT path, so it must bypass the executor (want 0)", executor.invocations)
	}
	if resp.Invocation != nil {
		t.Fatalf("resp.Invocation is non-nil; the /weather fast-path must not produce an Invocation, and without a nil Invocation this test no longer reproduces BUG-069-004")
	}

	// --- The actual regression --------------------------------------------
	if resp.Status != contracts.StatusAnswered {
		t.Fatalf("resp.Status = %q; want %q (a traceable turn must first be an answered turn)", resp.Status, contracts.StatusAnswered)
	}
	if resp.AssistantTurnID == "" {
		t.Error("resp.AssistantTurnID is empty on a short-circuit turn — this is BUG-069-004: the turn answered correctly but cannot be traced from /metrics → log line → conversation row")
	}
	if resp.AgentTraceID == "" {
		t.Error("resp.AgentTraceID is empty on a short-circuit turn — dashboards cannot join this turn to its spec 037 trace")
	}

	// The two ids must be PAIRED, not independently minted. A trace id that
	// does not derive from the turn id would look correct on the wire while
	// failing to join in any dashboard.
	if want := agentTraceID(resp.AssistantTurnID); resp.AgentTraceID != want {
		t.Errorf("resp.AgentTraceID = %q; want %q — the agent trace id must be derived from the assistant turn id so the two join", resp.AgentTraceID, want)
	}
}

// TestFacadeShortCircuitTurnIdentityIsDeterministic_BUG069004 — the id on a
// short-circuit turn MUST be the deterministic `asst-<unix-nano>` value
// derived from the turn's emittedAt, NOT an independently minted token.
//
// This is the assertion that stops the tempting wrong fix. Minting an id at
// the transport (or anywhere off the emittedAt chokepoint) would satisfy
// "non-empty" while producing a value that disagrees with the facade log
// line and the audit row — worse than an absent id, because it looks
// correct and leads a reader to a row that does not exist. Deriving it here
// from the audit row's own EmittedAt proves the response id and the
// recorded turn share one origin.
func TestFacadeShortCircuitTurnIdentityIsDeterministic_BUG069004(t *testing.T) {
	t.Parallel()

	weatherJSON := []byte(`{"forecast_line":"Beverly Hills, CA: clear, 22°C","provider_name":"open-meteo","retrieved_at":"2026-07-22T15:00:00Z"}`)
	f, executor, audit := weatherShortcutFacade(t, func(_ context.Context, _ string) (json.RawMessage, error) {
		return json.RawMessage(weatherJSON), nil
	})

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-turnid-2", Transport: "web", Text: "/weather 90210", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if executor.invocations != 0 {
		t.Fatalf("executor invoked %d times; want 0 (short-circuit path only)", executor.invocations)
	}
	if resp.Invocation != nil {
		t.Fatal("resp.Invocation is non-nil; this test must exercise the short-circuit path")
	}
	if resp.AssistantTurnID == "" {
		t.Fatal("resp.AssistantTurnID is empty — cannot compare an absent id against the audit row")
	}

	if len(audit.turns) == 0 {
		t.Fatalf("audit recorded no turn for an answered /weather turn; nothing to correlate the response id against")
	}
	got := audit.turns[len(audit.turns)-1]
	if want := facadeTurnIDFromTime(got.EmittedAt); resp.AssistantTurnID != want {
		t.Errorf("resp.AssistantTurnID = %q; want %q derived from the audit row's EmittedAt (%s). The turn id MUST come from the emittedAt chokepoint so the wire id, the facade log line and the conversation row all name the same turn; an independently minted id looks correct but points at a row that does not exist", resp.AssistantTurnID, want, got.EmittedAt.Format("2006-01-02T15:04:05.000000000Z07:00"))
	}
	if got.Response.AssistantTurnID != resp.AssistantTurnID {
		t.Errorf("audit row Response.AssistantTurnID = %q but returned response = %q; the recorded turn and the wire response must not diverge", got.Response.AssistantTurnID, resp.AssistantTurnID)
	}
}
