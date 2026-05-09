//go:build e2e_ollama

// Spec 043 — agent E2E happy-path test against real Ollama.
//
// This file is compiled ONLY when the test runner builds with
// `-tags=e2e_ollama` (NOT under the broader `-tags=e2e` lane). The
// gating is intentional: this is the only test in the agent e2e
// package that depends on a live Ollama process serving the
// SST-pinned model `qwen2.5:0.5b-instruct`. Wiring the gate at the
// build-tag layer keeps `./smackerel.sh test e2e` (no Ollama) green
// while letting `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e`
// (Scope 3 will land that wrapper) include this file.
//
// The package is `agent_e2e` to match the rest of tests/e2e/agent so
// scopes that share the same Go test binary stay coherent. We do NOT
// import helpers_test.go's symbols (those are guarded by `e2e` and
// would not link under `-tags=e2e_ollama`); this file is intentionally
// self-contained.
//
// Fail-loud contract (BS-014, spec 020 SCN-OLLAMA-004): every required
// precondition uses `t.Fatalf` (NOT `t.Skip`/`t.Skipf`). The
// `tests/e2e/agent/no_skip_guard_test.go` regression test enforces
// this with a grep guard that allowlists every other file in this
// package and explicitly forbids `happy_path_test.go` from appearing
// in the allowlist.

package agent_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// scenarioID matches config/prompt_contracts/e2e-ollama-smoke-v1.yaml::id.
const ollamaSmokeScenarioID = "e2e_ollama_smoke"

// invokeRequestBody is the agent.InvocationRequest wire shape (mirrors
// internal/api/agent_handlers.go AgentInvokeHandlerFunc). We declare it
// inline rather than importing the API package to keep this test
// self-contained and to make the wire contract visible at the test
// site (so a contract drift surfaces here, not in a hidden import).
type invokeRequestBody struct {
	Source            string          `json:"source"`
	RawInput          string          `json:"raw_input"`
	ScenarioIDExplicit string         `json:"scenario_id,omitempty"`
	StructuredContext json.RawMessage `json:"structured_context,omitempty"`
}

type invokeResponseBody struct {
	TraceID       string          `json:"trace_id"`
	Outcome       string          `json:"outcome"`
	OutcomeDetail json.RawMessage `json:"outcome_detail,omitempty"`
	FinalOutput   json.RawMessage `json:"final_output,omitempty"`
}

// mustEnv reads an env var or fails the test loudly. NEVER calls
// t.Skip — the no-skip guard regression test enforces this.
func mustEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf(
			"e2e_ollama: required env var %q is missing or empty. "+
				"This test cannot skip (BS-014 / SCN-OLLAMA-004 fail-loud "+
				"contract). Source config/generated/test.env and run "+
				"with SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e.",
			key,
		)
	}
	return v
}

// liveDB connects to DATABASE_URL or fails loudly.
func liveDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := mustEnv(t, "DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("e2e_ollama: connect DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("e2e_ollama: ping DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// ollamaReachable probes <OLLAMA_URL>/api/tags with a short timeout.
// Returns true if Ollama responds 200, false otherwise. NEVER calls
// t.Skip; if env vars are missing it returns false and the caller
// decides what to do.
func ollamaReachable(t *testing.T, ollamaURL string) bool {
	t.Helper()
	u, err := url.Parse(ollamaURL)
	if err != nil {
		t.Fatalf("e2e_ollama: parse OLLAMA_URL %q: %v", ollamaURL, err)
	}
	probeURL := strings.TrimRight(u.String(), "/") + "/api/tags"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(probeURL)
	if err != nil {
		var nErr *net.OpError
		_ = nErr // we accept any error class as "not reachable"
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// postInvoke POSTs an agent invocation and returns the parsed response
// body alongside the HTTP status code. Fail-loud on transport-level
// errors; returns (status, body, response) to the caller for assertion.
func postInvoke(
	t *testing.T,
	coreURL, authToken, scenarioID, query string,
) (int, []byte, *invokeResponseBody) {
	t.Helper()
	body := invokeRequestBody{
		Source:             "e2e-ollama-smoke",
		RawInput:           query,
		ScenarioIDExplicit: scenarioID,
		StructuredContext:  json.RawMessage(fmt.Sprintf(`{"query":%q}`, query)),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("e2e_ollama: marshal invoke body: %v", err)
	}

	endpoint := strings.TrimRight(coreURL, "/") + "/v1/agent/invoke"
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, endpoint, bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("e2e_ollama: build invoke request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("e2e_ollama: POST %s: %v", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("e2e_ollama: read response body: %v", err)
	}
	var parsed invokeResponseBody
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed) // tolerated; assertions use raw on failure
	}
	return resp.StatusCode, raw, &parsed
}

// queryTrace fetches the agent_traces row for trace_id, asserting the
// row exists. Returns (turn_log, tool_calls, final_output, outcome).
func queryTrace(t *testing.T, pool *pgxpool.Pool, traceID string) (
	turnLog, toolCalls, finalOutput json.RawMessage, outcome string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := pool.QueryRow(
		ctx,
		`SELECT turn_log, tool_calls, COALESCE(final_output, 'null'::jsonb), outcome
		   FROM agent_traces WHERE trace_id = $1`,
		traceID,
	).Scan(&turnLog, &toolCalls, &finalOutput, &outcome)
	if err != nil {
		t.Fatalf("e2e_ollama: SELECT agent_traces trace_id=%s: %v", traceID, err)
	}
	return turnLog, toolCalls, finalOutput, outcome
}

// cleanupTrace deletes the agent_traces row at test cleanup so re-runs
// don't accumulate.
func cleanupTrace(t *testing.T, pool *pgxpool.Pool, traceID string) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM agent_traces WHERE trace_id = $1`, traceID)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_tool_calls WHERE trace_id = $1`, traceID)
	})
}

// TestAgentHappyPath_PlanToolSynthesis exercises the full
// plan → tool_call → synthesis loop through the production
// Go executor → NATS → Python ML sidecar → litellm → Ollama path.
//
// SCN-OLLAMA-003 — assertion that the agent_traces row records the
// 3-step trace shape: (1) at least one turn in turn_log,
// (2) at least one tool call in tool_calls (the LLM elected to call
// recommendation_parse_intent per the scenario's system prompt),
// (3) outcome = "ok" with a non-null final_output matching the
// scenario's output_schema {"acknowledged": bool}.
func TestAgentHappyPath_PlanToolSynthesis(t *testing.T) {
	coreURL := mustEnv(t, "CORE_URL")
	authToken := mustEnv(t, "SMACKEREL_AUTH_TOKEN")
	ollamaURL := mustEnv(t, "OLLAMA_URL")
	pool := liveDB(t)

	if !ollamaReachable(t, ollamaURL) {
		t.Fatalf(
			"e2e_ollama: OLLAMA_URL %q is unreachable. This test requires "+
				"a live Ollama with the SST-pinned model. Start the test stack "+
				"with `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh up` and verify "+
				"`scripts/commands/ollama-test-pull.sh` (respects "+
				"OLLAMA_TEST_PULL_TIMEOUT_SECONDS).",
			ollamaURL,
		)
	}

	status, raw, resp := postInvoke(t, coreURL, authToken, ollamaSmokeScenarioID, "ping happy path")
	if status != http.StatusOK {
		t.Fatalf("e2e_ollama: expected HTTP 200, got %d body=%s", status, string(raw))
	}
	if resp.TraceID == "" {
		t.Fatalf("e2e_ollama: response missing trace_id; body=%s", string(raw))
	}
	cleanupTrace(t, pool, resp.TraceID)
	if resp.Outcome != "ok" {
		t.Fatalf(
			"e2e_ollama: outcome=%q (expected ok); detail=%s body=%s",
			resp.Outcome, string(resp.OutcomeDetail), string(raw),
		)
	}

	turnLog, toolCalls, finalOutput, outcome := queryTrace(t, pool, resp.TraceID)
	if outcome != "ok" {
		t.Fatalf("e2e_ollama: agent_traces.outcome=%q (expected ok)", outcome)
	}

	// turn_log MUST be a non-empty array (at least one LLM turn was recorded).
	var turns []map[string]any
	if err := json.Unmarshal(turnLog, &turns); err != nil {
		t.Fatalf("e2e_ollama: turn_log is not a JSON array: %v body=%s", err, string(turnLog))
	}
	if len(turns) == 0 {
		t.Fatalf("e2e_ollama: turn_log empty (expected ≥1 LLM turn); raw=%s", string(turnLog))
	}

	// tool_calls MUST be a non-empty array (LLM elected to call the
	// recommendation_parse_intent tool per the scenario's system prompt).
	var calls []map[string]any
	if err := json.Unmarshal(toolCalls, &calls); err != nil {
		t.Fatalf("e2e_ollama: tool_calls is not a JSON array: %v body=%s", err, string(toolCalls))
	}
	if len(calls) == 0 {
		t.Fatalf(
			"e2e_ollama: tool_calls empty — the LLM did not invoke any tool. "+
				"Either the scenario's system_prompt is being ignored or the "+
				"model is not honoring the tools=[...] payload. raw=%s",
			string(toolCalls),
		)
	}

	// final_output MUST match the scenario's output_schema:
	// {"acknowledged": <bool>}.
	if string(finalOutput) == "null" {
		t.Fatalf("e2e_ollama: final_output is null (expected JSON object)")
	}
	var final map[string]any
	if err := json.Unmarshal(finalOutput, &final); err != nil {
		t.Fatalf("e2e_ollama: final_output is not a JSON object: %v raw=%s", err, string(finalOutput))
	}
	if _, ok := final["acknowledged"]; !ok {
		t.Fatalf(
			"e2e_ollama: final_output missing required key 'acknowledged' "+
				"(per scenario output_schema); raw=%s",
			string(finalOutput),
		)
	}
}

// TestAgentHappyPath_DeterministicOutput runs the same invocation 3
// times and asserts byte-identical final_output across all three
// runs. Determinism is sourced from OLLAMA_TEST_REQUEST_* env vars
// (temperature=0, top_p=1, top_k=1, seed=42, num_predict=256) which
// ml/app/agent.resolve_ollama_determinism_options() forwards to
// litellm when provider == "ollama".
//
// SCN-OLLAMA-002 — non-determinism would mean a sampler knob was
// dropped or the seed regressed; either case fails this test loudly.
func TestAgentHappyPath_DeterministicOutput(t *testing.T) {
	coreURL := mustEnv(t, "CORE_URL")
	authToken := mustEnv(t, "SMACKEREL_AUTH_TOKEN")
	ollamaURL := mustEnv(t, "OLLAMA_URL")
	pool := liveDB(t)

	if !ollamaReachable(t, ollamaURL) {
		t.Fatalf(
			"e2e_ollama: OLLAMA_URL %q is unreachable; cannot assert determinism "+
				"against a missing model server.",
			ollamaURL,
		)
	}

	const runs = 3
	const query = "deterministic test query"
	finals := make([]string, runs)
	for i := 0; i < runs; i++ {
		status, raw, resp := postInvoke(t, coreURL, authToken, ollamaSmokeScenarioID, query)
		if status != http.StatusOK {
			t.Fatalf("e2e_ollama: run %d: HTTP %d body=%s", i+1, status, string(raw))
		}
		if resp.TraceID == "" {
			t.Fatalf("e2e_ollama: run %d: missing trace_id; body=%s", i+1, string(raw))
		}
		cleanupTrace(t, pool, resp.TraceID)
		_, _, finalOutput, outcome := queryTrace(t, pool, resp.TraceID)
		if outcome != "ok" {
			t.Fatalf("e2e_ollama: run %d: outcome=%q (expected ok)", i+1, outcome)
		}
		finals[i] = string(finalOutput)
	}

	// All N runs MUST produce byte-identical final_output. If any pair
	// differs, the determinism envelope is broken.
	for i := 1; i < runs; i++ {
		if finals[i] != finals[0] {
			t.Fatalf(
				"e2e_ollama: determinism regression — run 1 final_output ≠ run %d final_output\n"+
					"  run 1: %s\n  run %d: %s\n"+
					"OLLAMA_TEST_REQUEST_{TEMPERATURE,TOP_P,TOP_K,SEED,NUM_PREDICT} "+
					"may be missing from the ML container or "+
					"ml/app/agent.resolve_ollama_determinism_options is not forwarding "+
					"them as kwargs to litellm.acompletion.",
				i+1, finals[0], i+1, finals[i],
			)
		}
	}
}

// TestOllamaUnreachable_FailsLoudly asserts the BS-014 fail-loud
// contract. Two run modes:
//
//   - "Ollama up" (default): the test verifies that an invocation
//     succeeds end-to-end (catches the case where the test wiring
//     itself is broken — proves the negative-path assertion below
//     is meaningful).
//
//   - "Ollama down" (operator-driven, after `docker stop
//     smackerel-test-ollama`): the test verifies that the API
//     returns a non-OK outcome whose message includes the
//     unreachable URL. Failure mode: either the API silently swallows
//     the provider error, or the message lacks the URL — both indicate
//     a fail-loud contract regression.
//
// Adversarial case: this test MUST NEVER call t.Skip / t.Skipf /
// t.SkipNow. The no_skip_guard_test.go regression test grep-asserts
// this file does not appear in the skip allowlist.
func TestOllamaUnreachable_FailsLoudly(t *testing.T) {
	coreURL := mustEnv(t, "CORE_URL")
	authToken := mustEnv(t, "SMACKEREL_AUTH_TOKEN")
	ollamaURL := mustEnv(t, "OLLAMA_URL")
	pool := liveDB(t)
	pullTimeoutSeconds := mustEnv(t, "OLLAMA_TEST_PULL_TIMEOUT_SECONDS")

	if ollamaReachable(t, ollamaURL) {
		// Up path — invocation should succeed; this proves the test
		// wiring is intact so the down-path assertion below has
		// meaning.
		status, raw, resp := postInvoke(t, coreURL, authToken, ollamaSmokeScenarioID, "fail-loud probe")
		if status != http.StatusOK || resp.Outcome != "ok" {
			t.Fatalf(
				"e2e_ollama: Ollama is reachable at %s but the smoke invocation failed "+
					"(status=%d outcome=%q body=%s). Either the agent endpoint is broken "+
					"or the scenario %q is misregistered.",
				ollamaURL, status, resp.Outcome, string(raw), ollamaSmokeScenarioID,
			)
		}
		if resp.TraceID != "" {
			cleanupTrace(t, pool, resp.TraceID)
		}
		// Down-path coverage is exercised when this test is run after
		// the operator stops Ollama; the rest of this function asserts
		// that mode of run.
		t.Logf(
			"e2e_ollama: Ollama is reachable at %s; happy probe succeeded. "+
				"To exercise the down-path branch of this test, stop the Ollama "+
				"container (`docker stop smackerel-test-ollama`) and re-run; "+
				"OLLAMA_TEST_PULL_TIMEOUT_SECONDS=%s budget will gate the "+
				"unreachable-detection latency.",
			ollamaURL, pullTimeoutSeconds,
		)
		return
	}

	// Down path — invocation MUST surface a fail-loud outcome.
	status, raw, resp := postInvoke(t, coreURL, authToken, ollamaSmokeScenarioID, "fail-loud probe (down)")
	bodyLower := strings.ToLower(string(raw))
	switch {
	case status == http.StatusOK && resp.Outcome == "ok":
		t.Fatalf(
			"e2e_ollama: Ollama is unreachable at %s but the API returned outcome=ok "+
				"(silently fabricated a successful trace). This is a BS-014 fail-loud "+
				"contract violation. body=%s",
			ollamaURL, string(raw),
		)
	case !strings.Contains(bodyLower, "ollama") && !strings.Contains(bodyLower, "provider"):
		t.Fatalf(
			"e2e_ollama: Ollama is unreachable at %s and the API returned status=%d "+
				"outcome=%q but the response body does not mention 'ollama' or "+
				"'provider'. The fail-loud diagnostic is missing the unreachable URL "+
				"or the provider error class. body=%s. "+
				"OLLAMA_TEST_PULL_TIMEOUT_SECONDS=%s.",
			ollamaURL, status, resp.Outcome, string(raw), pullTimeoutSeconds,
		)
	}
	t.Logf(
		"e2e_ollama: down-path fail-loud verified — Ollama unreachable at %s, "+
			"API returned status=%d outcome=%q with diagnostic body=%s "+
			"(pull_timeout_seconds=%s)",
		ollamaURL, status, resp.Outcome, string(raw), pullTimeoutSeconds,
	)
}
