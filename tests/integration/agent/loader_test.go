//go:build integration

// Package agent_integration exercises the scenario loader against on-disk
// fixture directories. The loader has unit-test coverage in
// internal/agent/loader_*_test.go; these integration tests assert the
// directory-scanning behavior end-to-end with mixed valid + adversarial
// fixtures.
package agent_integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// validScenarioYAML is a minimal scenario fixture used across the
// integration-level loader tests. It mirrors the shape declared in the
// design's §2.1 example.
const validScenarioYAML = `version: "expense_question-v1"
type: "scenario"
id: "expense_question"
description: "Answer expense questions"
intent_examples:
  - "how much did I spend on groceries"
system_prompt: |
  You are the expense agent.
allowed_tools:
  - name: "search_expenses"
    side_effect_class: "read"
input_schema:
  type: object
  required: [user_tz]
  properties:
    user_tz: { type: string }
output_schema:
  type: object
  required: [answer]
  properties:
    answer: { type: string }
limits:
  max_loop_iterations: 8
  timeout_ms: 30000
  schema_retry_budget: 2
  per_tool_timeout_ms: 5000
token_budget: 4000
temperature: 0.2
model_preference: "default"
side_effect_class: "read"
`

// recipeScenarioYAML is a second valid scenario with a distinct id so
// the duplicate-id test (BS-011) can place duplicates in a directory
// without colliding with this fixture.
const recipeScenarioYAML = `version: "recipe_scaler-v1"
type: "scenario"
id: "recipe_scaler"
description: "Scale a recipe"
intent_examples:
  - "scale this recipe"
system_prompt: |
  You scale recipes.
allowed_tools:
  - name: "search_expenses"
    side_effect_class: "read"
input_schema:
  type: object
  required: [user_tz]
  properties:
    user_tz: { type: string }
output_schema:
  type: object
  required: [answer]
  properties:
    answer: { type: string }
limits:
  max_loop_iterations: 4
  timeout_ms: 15000
  schema_retry_budget: 1
  per_tool_timeout_ms: 3000
side_effect_class: "read"
`

// registerToolsForLoader registers the "search_expenses" fake tool used
// by the fixture scenarios. The registry survives across tests in the
// same process; resetForTest is unexported in `agent`, so we register
// once per package and accept that subsequent re-registration would
// panic — which is exactly what the design intends for production code.
func registerToolsForLoader(t *testing.T) {
	t.Helper()
	if agent.Has("search_expenses") {
		return
	}
	agent.RegisterTool(agent.Tool{
		Name:            "search_expenses",
		Description:     "Search expense records",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "agent_integration",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	})
}

// writeFiles is a small helper that writes a map of filename → body
// into a fresh temporary directory and returns the directory path.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// TestLoader_MixedDirectory_IsolatesFailures registers the fake tool,
// then loads a directory containing one valid scenario, one bad-field
// scenario, and an unrelated prompt-contract YAML. The loader must
// register the valid scenario, reject only the bad one, and ignore the
// unrelated file.
func TestLoader_MixedDirectory_IsolatesFailures(t *testing.T) {
	registerToolsForLoader(t)
	dir := writeFiles(t, map[string]string{
		"valid.yaml":   validScenarioYAML,
		"bad.yaml":     `type: scenario` + "\n", // missing every other required field
		"contract.yml": "type: domain-extraction\nversion: foo\n",
	})

	registered, rejected, fatal := agent.DefaultLoader().Load(dir, "")
	if fatal != nil {
		t.Fatalf("unexpected fatal: %v", fatal)
	}
	if len(registered) != 1 || registered[0].ID != "expense_question" {
		t.Fatalf("expected only the valid scenario to register; got %+v", registered)
	}
	if len(rejected) != 1 {
		t.Fatalf("expected exactly 1 rejection; got %+v", rejected)
	}
	if filepath.Base(rejected[0].Path) != "bad.yaml" {
		t.Errorf("wrong file rejected: %s", rejected[0].Path)
	}
}
