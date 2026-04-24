package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_BS007_SchemaRetryExhaustion is the adversarial regression
// for BS-007. With schema_retry_budget=2, three consecutive
// schema-violating final answers MUST produce outcome=schema-failure
// with attempts=2 and a populated last_error.
//
// Failure modes this test catches:
//
//   - Executor accepts the malformed final and emits OK.
//   - Executor terminates after the first violation (no retry).
//   - Executor never increments the schema-retry counter and loops
//     forever (would hit loop-limit, not schema-failure).
//   - Executor reports schema-failure with attempts=0 or omits
//     last_error.
//   - Executor consumes the iteration budget for retries instead of
//     the dedicated schema-retry budget.
func TestExecutor_BS007_SchemaRetryExhaustion(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()

	// Build a scenario with schema_retry_budget=2 and a generous loop
	// budget so loop-limit cannot pre-empt schema-failure.
	limits := defaultLimits()
	limits.SchemaRetryBudget = 2
	limits.MaxLoopIterations = 32
	sc := makeExecutorScenario(t, nil, limits)

	// Three consecutive malformed finals (missing "answer" field).
	// Budget=2 ⇒ first two are recoverable, third terminates.
	bad := json.RawMessage(`{"wrong":"shape"}`)
	driver := newScriptedDriver(
		turnReplyOrError{resp: TurnResponse{Final: bad}},
		turnReplyOrError{resp: TurnResponse{Final: bad}},
		turnReplyOrError{resp: TurnResponse{Final: bad}},
		// Sentry: if the executor incorrectly continues, this would
		// silently rescue the test by producing a valid final.
		turnReplyOrError{resp: TurnResponse{Final: json.RawMessage(`{"answer":"sentinel"}`)}},
	)

	exe := newTestExecutor(t, driver)
	res := exe.Run(context.Background(), sc, envFromInput(validInput()))

	// Gate 1 — outcome must be schema-failure.
	if res.Outcome != OutcomeSchemaFailure {
		t.Fatalf("Gate 1: outcome = %s, want %s; detail=%v", res.Outcome, OutcomeSchemaFailure, res.OutcomeDetail)
	}

	// Gate 2 — outcome_detail.attempts must equal SchemaRetryBudget (2).
	attempts, ok := res.OutcomeDetail["attempts"].(int)
	if !ok {
		t.Fatalf("Gate 2: outcome_detail.attempts missing or wrong type: %+v", res.OutcomeDetail)
	}
	if attempts != 2 {
		t.Fatalf("Gate 2: attempts = %d, want 2 (the budget)", attempts)
	}

	// Gate 3 — outcome_detail.last_error must be populated.
	lastErr, ok := res.OutcomeDetail["last_error"].(string)
	if !ok || lastErr == "" {
		t.Fatalf("Gate 3: last_error missing or empty: %+v", res.OutcomeDetail)
	}

	// Gate 4 — driver MUST have been called exactly 3 times (one
	// initial + two retries). A 4th call means the executor failed to
	// terminate; a 2nd-only call means it bailed too early.
	if driver.Calls() != 3 {
		t.Fatalf("Gate 4 (retry budget honoured): driver called %d times, want 3", driver.Calls())
	}

	// Gate 5 — the SchemaRetries counter on the result must equal
	// budget+1 (the attempt that exceeded the budget).
	if res.SchemaRetries != 3 {
		t.Fatalf("Gate 5: SchemaRetries = %d, want 3", res.SchemaRetries)
	}

	// Gate 6 — schema retries MUST NOT consume the iteration budget
	// independently. The iteration counter equals the number of LLM
	// turns (3).
	if res.Iterations != 3 {
		t.Fatalf("Gate 6: iterations = %d, want 3 (one per LLM turn)", res.Iterations)
	}
}
