package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_BS006_HallucinatedToolRejectedBeforeLookup is the
// adversarial regression for BS-006 mandated by Scope 5's DoD.
//
// Failure modes this test would catch if the BS-006 handling were
// removed:
//
//   - Executor dispatches a tool whose name is not registered.
//   - Executor performs a registry lookup with side effects (e.g.,
//     auto-registers a stub) instead of treating the unknown name as a
//     hallucination.
//   - Executor terminates with an undefined / non-structured outcome
//     instead of recording the per-call rejection and continuing.
//   - Executor short-circuits the test by bailing out on first
//     unknown-name (no recovery, no structured error to LLM).
//
// The test is constructed so each protection MUST be present for it to
// pass — there is no early-return / bailout that would make it pass
// vacuously.
func TestExecutor_BS006_HallucinatedToolRejectedBeforeLookup(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerEchoTool(t, "echo")
	// Snapshot the registry size BEFORE the run; if a hallucinated
	// lookup auto-registered anything, this would diverge.
	beforeNames := snapshotRegistryNames()

	hallucinated := "find_random_recipe" // looks plausible; intentionally not registered
	driver := newScriptedDriver(
		// Turn 1 — propose the hallucinated tool name.
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: hallucinated, Arguments: json.RawMessage(`{"q":"anything"}`)}},
		}},
		// Turn 2 — recover with the real allowed tool. Asserts the
		// rejection envelope was readable and the loop did not
		// terminate prematurely.
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: jsonObj(t, map[string]string{"q": "real"})}},
		}},
		// Turn 3 — finalise.
		turnReplyOrError{resp: TurnResponse{Final: json.RawMessage(`{"answer":"recovered after hallucination"}`)}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{{Name: "echo", SideEffectClass: SideEffectRead}}, defaultLimits())
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))

	// Gate 1 — overall outcome must be ok (the LLM recovered). If the
	// executor bailed out on first hallucination, this would be
	// provider-error or similar.
	if res.Outcome != OutcomeOK {
		t.Fatalf("Gate 1 (recovery): outcome = %s, want ok; detail=%v", res.Outcome, res.OutcomeDetail)
	}

	// Gate 2 — exactly one rejection plus one ok call. If the executor
	// dispatched the hallucinated name, we'd see three records (or a
	// dispatch panic).
	if len(res.ToolCalls) != 2 {
		t.Fatalf("Gate 2 (record count): want 2 tool-call records, got %d (%+v)", len(res.ToolCalls), res.ToolCalls)
	}
	rec := res.ToolCalls[0]
	if rec.Name != hallucinated {
		t.Fatalf("Gate 2 (record identity): first record name = %q, want %q", rec.Name, hallucinated)
	}
	if rec.Outcome != OutcomeHallucinatedTool {
		t.Fatalf("Gate 3 (outcome class): first record outcome = %s, want %s", rec.Outcome, OutcomeHallucinatedTool)
	}
	if rec.RejectionReason != "unknown_tool" {
		t.Fatalf("Gate 4 (rejection reason): %q, want unknown_tool", rec.RejectionReason)
	}

	// Gate 5 — the registry MUST NOT have grown. Auto-registration on
	// hallucinated names would be a side effect; this gate would flip
	// the moment such code shipped.
	afterNames := snapshotRegistryNames()
	if !sameStringSet(beforeNames, afterNames) {
		t.Fatalf("Gate 5 (no side-effect lookup): registry mutated; before=%v after=%v", beforeNames, afterNames)
	}

	// Gate 6 — the recovery call must show ok. Asserts the loop
	// continued past the rejection (no bailout return).
	if res.ToolCalls[1].Outcome != OutcomeOK || res.ToolCalls[1].Name != "echo" {
		t.Fatalf("Gate 6 (loop continuation): recovery call wrong: %+v", res.ToolCalls[1])
	}

	// Gate 7 — the driver must have been called THREE times (two
	// proposals + one final). Bailout would call exactly once.
	if driver.Calls() != 3 {
		t.Fatalf("Gate 7 (no bailout): driver called %d times, want 3", driver.Calls())
	}

	// Gate 8 — the rejection envelope sent back to the LLM on turn 2
	// must include the available tool list. We verify by inspecting
	// the second TurnRequest's last tool message.
	reqs := driver.Requests()
	if len(reqs) < 2 {
		t.Fatal("Gate 8 (LLM error envelope): driver only saw < 2 requests")
	}
	turn2 := reqs[1]
	var sawAvailable bool
	for _, m := range turn2.TurnMessages {
		if m.Role == RoleTool && m.ToolName == hallucinated {
			var body map[string]any
			_ = json.Unmarshal(m.Content, &body)
			if av, ok := body["available"].([]any); ok {
				for _, name := range av {
					if name == "echo" {
						sawAvailable = true
					}
				}
			}
		}
	}
	if !sawAvailable {
		t.Fatal("Gate 8 (LLM error envelope): turn 2 did NOT include the available tool list 'echo' for the hallucinated name")
	}
}

func snapshotRegistryNames() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}
