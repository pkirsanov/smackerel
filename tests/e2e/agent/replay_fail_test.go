//go:build e2e

// Spec 037 Scope 6 — replay FAIL live-stack e2e regression.
//
// Records a real invocation, then mutates the on-disk scenario YAML
// (changes system_prompt) — which makes the loader recompute a new
// content_hash. Invoking `smackerel agent replay <trace_id>` against
// the mutated file must:
//
//	G1: exit code 1 (FAIL — design §6.2 contract).
//	G2: stdout contains "verdict=FAIL".
//	G3: --json mode emits a ReplayResult with Pass=false and at least
//	    one DiffEntry of kind "scenario_content_changed", with the
//	    recorded hash differing from the current hash.
//	G4: passing --allow-content-drift suppresses the FAIL (exit 0,
//	    verdict=PASS) — proves the override is not theater AND that
//	    G1/G2/G3 fail for the right reason (content_hash drift).
//
// Skips when DATABASE_URL or NATS_URL is unset.

package agent_e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

func TestReplayCLI_FailsWhenScenarioContentDrifts(t *testing.T) {
	pool := liveDB(t)
	nc := liveNATS(t)
	registerEchoTool(t)

	scenarioID := "scope6_e2e_fail"
	originalPrompt := "you are a scope-6 e2e replay FAIL test agent at recording time."
	scenarioDir := writeScenarioDir(t, scenarioID, originalPrompt)
	sc := loadScenarioFromDir(t, scenarioDir, scenarioID)
	originalHash := sc.ContentHash

	traceID := recordOneTrace(t, pool, nc, sc)

	// Mutate: rewrite the scenario YAML in place with a different
	// system_prompt. This changes content_hash without changing the
	// scenario id or version.
	mutatedPrompt := "you are a SCOPE-6 E2E REPLAY FAIL TEST AGENT WHOSE PROMPT WAS EDITED."
	mutatedYAML := fmt.Sprintf(scenarioYAML, scenarioID, scenarioID, echoToolName, mutatedPrompt)
	if err := os.WriteFile(filepath.Join(scenarioDir, scenarioID+".yaml"), []byte(mutatedYAML), 0o600); err != nil {
		t.Fatalf("rewrite scenario yaml: %v", err)
	}

	// Sanity: the mutated load must yield a different content_hash, or
	// the test would pass trivially below.
	mutated := loadScenarioFromDir(t, scenarioDir, scenarioID)
	if mutated.ContentHash == originalHash {
		t.Fatalf("setup: rewriting system_prompt did not change content_hash (%s) — loader hash function is broken or the test is no longer adversarial", originalHash)
	}

	// G1+G2: replay must FAIL.
	exit, out := runReplayCLI(t, scenarioDir, traceID)
	if exit != 1 {
		t.Fatalf("G1: replay exit=%d want 1 (FAIL)\noutput:\n%s", exit, out)
	}
	envHas(t, out, "verdict=FAIL")

	// G3: JSON mode — parse and assert the diff shape.
	exit, out = runReplayCLI(t, scenarioDir, "--json", traceID)
	if exit != 1 {
		t.Fatalf("G3: --json replay exit=%d want 1\noutput:\n%s", exit, out)
	}
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("G3: no JSON object in output:\n%s", out)
	}
	dec := json.NewDecoder(strings.NewReader(out[jsonStart:]))
	var res agent.ReplayResult
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("G3: parse ReplayResult JSON: %v\noutput:\n%s", err, out)
	}
	if res.Pass {
		t.Fatalf("G3: ReplayResult.Pass=true on mutated scenario; diff=%+v", res.Diff)
	}
	var found *agent.DiffEntry
	for i, d := range res.Diff {
		if d.Kind == agent.DiffScenarioContentChange {
			found = &res.Diff[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("G3: missing scenario_content_changed diff entry; got %+v", res.Diff)
	}
	if found.Recorded != originalHash || found.Current != mutated.ContentHash {
		t.Fatalf("G3: diff endpoints wrong: recorded=%q current=%q want recorded=%q current=%q",
			found.Recorded, found.Current, originalHash, mutated.ContentHash)
	}

	// G4: --allow-content-drift suppresses the FAIL.
	exit, out = runReplayCLI(t, scenarioDir, "--allow-content-drift", traceID)
	if exit != 0 {
		t.Fatalf("G4: --allow-content-drift exit=%d want 0\noutput:\n%s", exit, out)
	}
	envHas(t, out, "verdict=PASS")
}
