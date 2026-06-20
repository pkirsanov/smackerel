package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_MalformedJSONArgumentsRejectedBeforeDispatch is an
// adversarial regression for the argument-validation security boundary
// (spec 037 BS-004 / design §5.1 step 3c). It covers a distinct code
// path from TestExecutor_BS004_ArgumentSchemaRejectionThenRecover:
//
//   - BS-004 proposes WELL-FORMED JSON that fails the *schema* type
//     check (`{"q":42}`); that exercises CompiledSchema.Validate.
//   - This test proposes SYNTACTICALLY MALFORMED JSON (`{"q":` —
//     truncated, not parseable); that exercises the *json-parse*
//     rejection inside CompiledSchema.ValidateBytes
//     (jsonschema.UnmarshalJSON error), a different branch entirely.
//
// More importantly, this test proves the ORDERING invariant that BS-004
// does not assert: a malformed tool call MUST be rejected BEFORE the
// tool handler is dispatched. The handler records every argument set it
// ever receives; the test asserts the malformed call NEVER reaches it.
//
// Failure modes this test would catch if the protection regressed:
//
//   - Executor dispatches the handler before validating arguments
//     (the malformed bytes would reach the handler).
//   - ValidateBytes is weakened to accept un-parseable JSON (the
//     malformed call would dispatch and the outcome would not be
//     argument_schema_violation).
//   - Executor bails out on the malformed call instead of recording a
//     per-call rejection and continuing (no recovery, driver called
//     only once).
//
// The test is constructed so each protection MUST hold for it to pass —
// there is no early-return / bailout that makes it pass vacuously.
func TestExecutor_MalformedJSONArgumentsRejectedBeforeDispatch(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()

	// A read-only tool whose handler records every argument payload it
	// is ever invoked with. If the executor dispatches the malformed
	// call, the malformed bytes (or two invocations) show up here.
	var handlerArgs []string
	RegisterTool(Tool{
		Name:            "echo",
		Description:     "echo q back; records every invocation",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: SideEffectRead,
		OwningPackage:   "executor_test",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			handlerArgs = append(handlerArgs, string(args))
			return args, nil
		},
	})

	// Syntactically invalid JSON — a truncated object. This is NOT a
	// schema violation (which would still be valid JSON); it cannot be
	// parsed at all.
	const malformed = `{"q":`

	driver := newScriptedDriver(
		// Turn 1 — propose echo with un-parseable arguments.
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: json.RawMessage(malformed)}},
		}},
		// Turn 2 — recover with valid arguments.
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: json.RawMessage(`{"q":"recovered"}`)}},
		}},
		// Turn 3 — finalise.
		turnReplyOrError{resp: TurnResponse{Final: json.RawMessage(`{"answer":"recovered after malformed args"}`)}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{{Name: "echo", SideEffectClass: SideEffectRead}}, defaultLimits())
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))

	// Gate 1 (handler bypass — the core security property): the handler
	// must have been invoked EXACTLY ONCE, and that one call must carry
	// the VALID recovery arguments. If the malformed call had been
	// dispatched, handlerArgs would contain the malformed bytes and/or
	// have length 2.
	if len(handlerArgs) != 1 {
		t.Fatalf("Gate 1 (handler bypass): handler invoked %d time(s) (%v), want exactly 1 — malformed args must never reach the handler", len(handlerArgs), handlerArgs)
	}
	if handlerArgs[0] != `{"q":"recovered"}` {
		t.Fatalf("Gate 1 (handler bypass): handler received %q, want the valid recovery args only", handlerArgs[0])
	}

	// Gate 2 (rejection classification): exactly two tool-call records —
	// the malformed rejection and the recovery success.
	if len(res.ToolCalls) != 2 {
		t.Fatalf("Gate 2 (record count): want 2 tool-call records, got %d (%+v)", len(res.ToolCalls), res.ToolCalls)
	}
	rej := res.ToolCalls[0]
	if rej.Name != "echo" {
		t.Fatalf("Gate 2 (record identity): first record name = %q, want echo", rej.Name)
	}
	if rej.Outcome != OutcomeToolError {
		t.Fatalf("Gate 2 (outcome class): first record outcome = %s, want %s", rej.Outcome, OutcomeToolError)
	}
	if rej.RejectionReason != "argument_schema_violation" {
		t.Fatalf("Gate 2 (rejection reason): %q, want argument_schema_violation", rej.RejectionReason)
	}
	// The trace MUST preserve the exact bytes that were attempted, so an
	// operator can audit what the LLM proposed.
	if string(rej.Arguments) != malformed {
		t.Fatalf("Gate 2 (trace fidelity): recorded args = %q, want the malformed bytes %q", string(rej.Arguments), malformed)
	}
	if rej.Error == "" {
		t.Fatal("Gate 2 (error surfaced): malformed-args rejection recorded an empty error string")
	}

	// Gate 3 (loop continuation): overall outcome ok and the recovery
	// call succeeded. If the executor had bailed on the malformed call,
	// this would not be ok.
	if res.Outcome != OutcomeOK {
		t.Fatalf("Gate 3 (recovery): outcome = %s, want ok; detail=%v", res.Outcome, res.OutcomeDetail)
	}
	if res.ToolCalls[1].Outcome != OutcomeOK || res.ToolCalls[1].Name != "echo" {
		t.Fatalf("Gate 3 (loop continuation): recovery record wrong: %+v", res.ToolCalls[1])
	}

	// Gate 4 (no bailout): the driver must have been asked for THREE
	// turns (malformed proposal + recovery proposal + final). A bailout
	// on the malformed call would have stopped at one.
	if driver.Calls() != 3 {
		t.Fatalf("Gate 4 (no bailout): driver called %d times, want 3", driver.Calls())
	}
}
