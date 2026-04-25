//go:build e2e

// Spec 037 Scope 6 — replay PASS live-stack e2e regression.
//
// Records a real invocation against the live test stack (real Postgres
// for trace persistence, real NATS for event mirroring), then invokes
// the `smackerel agent replay <trace_id>` CLI subprocess against the
// SAME scenario YAML used to record. Asserts:
//
//	G1: exit code 0 (PASS — design §6.2 contract).
//	G2: stdout contains "verdict=PASS".
//	G3: --json mode emits a ReplayResult with Pass=true and an empty
//	    diff array (proves the structured output matches the
//	    human-readable verdict; not vacuous because it's parsed).
//
// Skips when DATABASE_URL or NATS_URL is unset.

package agent_e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

func TestReplayCLI_PassWhenScenarioUnchanged(t *testing.T) {
	pool := liveDB(t)
	nc := liveNATS(t)
	registerEchoTool(t)

	scenarioID := "scope6_e2e_pass"
	scenarioDir := writeScenarioDir(t, scenarioID, "you are a scope-6 e2e replay PASS test agent. echo what was asked.")
	sc := loadScenarioFromDir(t, scenarioDir, scenarioID)

	traceID := recordOneTrace(t, pool, nc, sc)

	// G1+G2: human-readable mode.
	exit, out := runReplayCLI(t, scenarioDir, traceID)
	if exit != 0 {
		t.Fatalf("G1: replay exit=%d want 0\noutput:\n%s", exit, out)
	}
	envHas(t, out, "verdict=PASS")
	envHas(t, out, traceID)

	// G3: JSON mode — parse and assert structured shape.
	exit, out = runReplayCLI(t, scenarioDir, "--json", traceID)
	if exit != 0 {
		t.Fatalf("G3: --json replay exit=%d want 0\noutput:\n%s", exit, out)
	}
	// Locate the JSON object in stdout (loader warnings or go-run
	// chatter may precede it).
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("G3: no JSON object in output:\n%s", out)
	}
	dec := json.NewDecoder(strings.NewReader(out[jsonStart:]))
	var res agent.ReplayResult
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("G3: parse ReplayResult JSON: %v\noutput:\n%s", err, out)
	}
	if !res.Pass {
		t.Fatalf("G3: ReplayResult.Pass=false; diff=%+v", res.Diff)
	}
	if len(res.Diff) != 0 {
		t.Fatalf("G3: unexpected diff entries: %+v", res.Diff)
	}
	if res.TraceID != traceID || res.ScenarioID != scenarioID {
		t.Fatalf("G3: identity mismatch: got %+v", res)
	}
}
