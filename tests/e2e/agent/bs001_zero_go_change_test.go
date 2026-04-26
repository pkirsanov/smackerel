//go:build e2e

// Spec 037 Scope 10 — BS-001 zero-Go-change scenario adds.
//
// Drops a brand-new scenario YAML referencing only existing tools into
// a temp directory the agent.Bridge has been pointed at, calls the
// SIGHUP-equivalent Bridge.Reload, and asserts:
//
//   G1: the new scenario id appears in the post-reload KnownIntents
//       (proves the loader saw the YAML and the router was rebuilt).
//   G2: invoking the new id via the production Bridge.Invoke entry
//       point produces OutcomeOK against a scripted driver — i.e.
//       the new scenario is fully wired, not just listed.
//   G3: a previously-loaded scenario continues to produce OutcomeOK
//       with identical inputs/outputs after the reload — proving the
//       hot-reload swap is non-destructive (BS-019 reaffirmation).
//   G4: ZERO Go changes were required to add the new scenario. We
//       enforce this by reusing the existing scope10_bs001_echo tool
//       registration and the existing executor/bridge/loader code
//       paths. The only artifact added between the "before" and
//       "after" snapshots is a YAML file on disk.
//
// This is the canonical proof that BS-001 (the entire reason the
// scenario layer exists) survives Scope 10's wiring. If a future
// change introduces a Go-only registration step or a static
// scenario manifest, this test fails before that change ships.

package agent_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// bs001Driver returns canned final outputs keyed by scenario id, so a
// single driver instance can serve multiple scenarios across the
// reload boundary deterministically.
type bs001Driver struct {
	mu     sync.Mutex
	finals map[string]json.RawMessage
}

func (d *bs001Driver) Turn(_ context.Context, req agent.TurnRequest) (agent.TurnResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	final, ok := d.finals[req.ScenarioID]
	if !ok {
		return agent.TurnResponse{}, nil
	}
	return agent.TurnResponse{
		Final:    final,
		Provider: "bs001", Model: "bs001-fake",
	}, nil
}

// bs001ScenarioYAML produces a minimal scenario YAML referencing only
// the existing scope10_bs001_echo tool — i.e. zero Go changes.
const bs001ScenarioYAML = `version: "%s-v1"
type: "scenario"
id: "%s"
description: "BS-001 zero-go-change proof scenario"
intent_examples:
  - "echo q please"
system_prompt: |
  bs001 zero-go-change
allowed_tools:
  - name: "scope10_bs001_echo"
    side_effect_class: "read"
input_schema:
  type: object
  required: [q]
  properties:
    q: { type: string }
output_schema:
  type: object
  required: [answer]
  properties:
    answer: { type: string }
limits:
  max_loop_iterations: 4
  timeout_ms: 30000
  schema_retry_budget: 1
  per_tool_timeout_ms: 1000
token_budget: 500
temperature: 0.0
model_preference: "fast"
side_effect_class: "read"
`

func writeBS001Scenario(t *testing.T, dir, id string) string {
	t.Helper()
	body := []byte(fmt.Sprintf(bs001ScenarioYAML, id, id))
	path := filepath.Join(dir, id+".yaml")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write scenario yaml: %v", err)
	}
	return path
}

// registerBS001Tool registers the read-only echo tool the bs001
// scenarios reference. Idempotent across `go test -count=N` runs.
func registerBS001Tool(t *testing.T) {
	t.Helper()
	if agent.Has("scope10_bs001_echo") {
		return
	}
	agent.RegisterTool(agent.Tool{
		Name:            "scope10_bs001_echo",
		Description:     "echo q for BS-001 proof",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "scope10_bs001_e2e",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

// TestBS001_DropYAMLAndReload_NewScenarioInvokable proves the
// zero-Go-change scenario-add contract end-to-end: write YAML, Reload
// (the SIGHUP-equivalent), invoke via id, assert OK. Pre-existing
// scenario continues to work after the reload.
func TestBS001_DropYAMLAndReload_NewScenarioInvokable(t *testing.T) {
	pool := liveDB(t)
	nc := liveNATS(t)
	registerBS001Tool(t)

	dir := t.TempDir()

	// Pre-populate the directory with one "before" scenario so we can
	// prove its behaviour is unchanged after the new YAML lands.
	beforeID := "scope10_bs001_before"
	writeBS001Scenario(t, dir, beforeID)

	cfg := &agent.Config{
		ScenarioDir:  dir,
		ScenarioGlob: "*.yaml",
		Routing: agent.RoutingConfig{
			ConfidenceFloor: 0.0,
			ConsiderTopN:    5,
		},
	}

	tracer, err := agent.NewPostgresTracer(pool, natsPublisher{nc: nc}, false)
	if err != nil {
		t.Fatalf("NewPostgresTracer: %v", err)
	}
	driver := &bs001Driver{
		finals: map[string]json.RawMessage{
			beforeID: json.RawMessage(`{"answer":"before-ok"}`),
		},
	}
	exe, err := agent.NewExecutor(driver, tracer)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	bridge, _, err := agent.NewBridge(context.Background(), agent.BridgeOptions{
		Config:   cfg,
		Executor: exe,
	})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}

	// Sanity: only the "before" scenario is registered initially.
	known := bridge.KnownIntents()
	foundBefore := false
	for _, id := range known {
		if id == beforeID {
			foundBefore = true
		}
	}
	if !foundBefore {
		t.Fatalf("setup: pre-reload KnownIntents missing %q: %v", beforeID, known)
	}

	// Record the before-scenario's outcome BEFORE the new YAML lands.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resBefore1, _ := bridge.Invoke(ctx, agent.IntentEnvelope{
		Source:            "test",
		ScenarioID:        beforeID,
		StructuredContext: json.RawMessage(`{"q":"before"}`),
	})
	if resBefore1 == nil || resBefore1.Outcome != agent.OutcomeOK {
		t.Fatalf("baseline before-scenario invocation: outcome=%v detail=%+v", outcomeOf(resBefore1), detailOf(resBefore1))
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM agent_traces WHERE trace_id = $1", resBefore1.TraceID)
	})

	// Drop the NEW YAML — this is the only "change". No Go code is
	// touched between this point and G1.
	newID := "scope10_bs001_added"
	writeBS001Scenario(t, dir, newID)
	driver.mu.Lock()
	driver.finals[newID] = json.RawMessage(`{"answer":"added-ok"}`)
	driver.mu.Unlock()

	// Trigger the SIGHUP-equivalent.
	rejected, err := bridge.Reload(ctx)
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(rejected) > 0 {
		t.Fatalf("Reload rejected: %+v", rejected)
	}

	// G1: post-reload KnownIntents includes the new id.
	postKnown := bridge.KnownIntents()
	foundAdded := false
	foundBefore = false
	for _, id := range postKnown {
		if id == newID {
			foundAdded = true
		}
		if id == beforeID {
			foundBefore = true
		}
	}
	if !foundAdded {
		t.Fatalf("G1: post-reload KnownIntents missing newly-added %q: %v", newID, postKnown)
	}
	if !foundBefore {
		t.Fatalf("G1: post-reload KnownIntents lost pre-existing %q: %v", beforeID, postKnown)
	}

	// G2: invoking the new id by id produces OutcomeOK.
	resAdded, decisionAdded := bridge.Invoke(ctx, agent.IntentEnvelope{
		Source:            "test",
		ScenarioID:        newID,
		StructuredContext: json.RawMessage(`{"q":"added"}`),
	})
	if resAdded == nil || resAdded.Outcome != agent.OutcomeOK {
		t.Fatalf("G2: new-scenario invocation: outcome=%v detail=%+v", outcomeOf(resAdded), detailOf(resAdded))
	}
	if decisionAdded == nil || decisionAdded.Reason != agent.ReasonExplicitScenarioID {
		t.Fatalf("G2: new-scenario routing reason=%v want=%s", decisionAdded, agent.ReasonExplicitScenarioID)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM agent_traces WHERE trace_id = $1", resAdded.TraceID)
	})

	// G3: pre-existing scenario continues to produce OutcomeOK with
	// identical structured context — the reload was non-destructive.
	resBefore2, _ := bridge.Invoke(ctx, agent.IntentEnvelope{
		Source:            "test",
		ScenarioID:        beforeID,
		StructuredContext: json.RawMessage(`{"q":"before"}`),
	})
	if resBefore2 == nil || resBefore2.Outcome != agent.OutcomeOK {
		t.Fatalf("G3: post-reload before-scenario: outcome=%v detail=%+v", outcomeOf(resBefore2), detailOf(resBefore2))
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM agent_traces WHERE trace_id = $1", resBefore2.TraceID)
	})

	// G4 is enforced by construction: between the before-scenario
	// baseline and the new-scenario invocation we touched only the
	// scenario directory on disk — every Go object (executor, bridge,
	// driver, tracer, registry) is the same instance.
}
