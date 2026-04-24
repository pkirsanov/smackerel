package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scenarioFixture builds a minimal valid scenario YAML body. Tests
// override individual lines via the `mutators` callback to exercise
// rejection paths.
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

// registerLoaderTestTool registers a single fake tool that the YAML
// fixtures reference via allowed_tools.
func registerLoaderTestTool(t *testing.T) {
	t.Helper()
	resetRegistryForTest()
	RegisterTool(Tool{
		Name:            "search_expenses",
		Description:     "Search expense records",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		SideEffectClass: SideEffectRead,
		OwningPackage:   "agent_test",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	})
}

// writeScenarioDir creates a temp dir and writes the supplied named YAML
// bodies into it.
func writeScenarioDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// loadOK loads `dir`, asserts no fatal error, and returns the registered
// + rejected lists.
func loadOK(t *testing.T, dir string) ([]*Scenario, []LoadError) {
	t.Helper()
	registered, rejected, err := DefaultLoader().Load(dir, "")
	if err != nil {
		t.Fatalf("Load returned fatal error: %v", err)
	}
	return registered, rejected
}

func TestLoader_HappyPath(t *testing.T) {
	registerLoaderTestTool(t)
	defer resetRegistryForTest()
	dir := writeScenarioDir(t, map[string]string{"expense.yaml": validScenarioYAML})

	registered, rejected := loadOK(t, dir)
	if len(rejected) != 0 {
		t.Fatalf("unexpected rejections: %+v", rejected)
	}
	if len(registered) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(registered))
	}
	scn := registered[0]
	if scn.ID != "expense_question" || scn.Version != "expense_question-v1" {
		t.Errorf("id/version mismatch: got %q / %q", scn.ID, scn.Version)
	}
	if scn.SideEffectClass != SideEffectRead {
		t.Errorf("class mismatch: %q", scn.SideEffectClass)
	}
	if scn.ContentHash == "" || len(scn.ContentHash) != 64 {
		t.Errorf("content hash invalid: %q", scn.ContentHash)
	}
	if scn.Limits.MaxLoopIterations != 8 || scn.Limits.TimeoutMs != 30000 {
		t.Errorf("limits not parsed: %+v", scn.Limits)
	}
	if scn.CompiledInputSchema() == nil || scn.CompiledOutputSchema() == nil {
		t.Error("compiled schemas missing")
	}
}

func TestLoader_NonScenarioFilesAreSkippedSilently(t *testing.T) {
	registerLoaderTestTool(t)
	defer resetRegistryForTest()
	dir := writeScenarioDir(t, map[string]string{
		"contract.yaml": "version: foo\ntype: domain-extraction\n",
		"empty.yaml":    "",
		"expense.yaml":  validScenarioYAML,
	})
	registered, rejected := loadOK(t, dir)
	if len(rejected) != 0 {
		t.Fatalf("non-scenario files must not appear in rejected: %+v", rejected)
	}
	if len(registered) != 1 {
		t.Fatalf("expected 1 registered scenario, got %d", len(registered))
	}
}

// rejectionTable drives one rejection rule per row. When `mutateRegistry`
// is true, the rule mutates the global tool registry to provoke the
// failure (e.g., the scenario-class-vs-tool-class rule), in which case
// the test does NOT include a sibling "good.yaml" — the registry change
// would also break the sibling and obscure the test intent.
type rejectionTable struct {
	name           string
	mutate         func(string) string
	contains       string
	mutateRegistry bool
}

func TestLoader_Rule_RejectsBadInputs(t *testing.T) {
	registerLoaderTestTool(t)
	defer resetRegistryForTest()

	cases := []rejectionTable{
		{
			name:     "missing required field id",
			mutate:   func(y string) string { return strings.Replace(y, `id: "expense_question"`, "", 1) },
			contains: `missing required field "id"`,
		},
		{
			name:     "missing required field system_prompt",
			mutate:   func(y string) string { return removeBlock(y, "system_prompt:", "allowed_tools:") },
			contains: `missing required field "system_prompt"`,
		},
		{
			name:     "missing required field output_schema",
			mutate:   func(y string) string { return removeBlock(y, "output_schema:", "limits:") },
			contains: `missing required field "output_schema"`,
		},
		{
			name: "id must be snake case",
			mutate: func(y string) string {
				return strings.Replace(y, `id: "expense_question"`, `id: "Expense-Question"`, 1)
			},
			contains: `does not match`,
		},
		{
			name: "version slug must equal id",
			mutate: func(y string) string {
				return strings.Replace(y, `version: "expense_question-v1"`, `version: "recipe_scaler-v1"`, 1)
			},
			contains: `must equal id`,
		},
		{
			name: "version must end with -vN",
			mutate: func(y string) string {
				return strings.Replace(y, `version: "expense_question-v1"`, `version: "expense_question"`, 1)
			},
			contains: `does not match`,
		},
		{
			name: "scenario class below tool class",
			mutate: func(y string) string {
				// Re-register the tool as 'write' but leave scenario class as 'read'.
				resetRegistryForTest()
				RegisterTool(Tool{
					Name:            "search_expenses",
					Description:     "x",
					InputSchema:     json.RawMessage(`{"type":"object"}`),
					OutputSchema:    json.RawMessage(`{"type":"object"}`),
					SideEffectClass: SideEffectWrite,
					OwningPackage:   "agent_test",
					Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
						return json.RawMessage(`{}`), nil
					},
				})
				return strings.Replace(y, `    side_effect_class: "read"`, `    side_effect_class: "write"`, 1)
			},
			contains:       `is below the highest allowed_tools class`,
			mutateRegistry: true,
		},
		{
			name: "tool side_effect_class mismatch with registry",
			mutate: func(y string) string {
				return strings.Replace(y,
					`    side_effect_class: "read"`,
					`    side_effect_class: "write"`,
					1)
			},
			contains: `but the registry has`,
		},
		{
			name: "limits.max_loop_iterations out of range",
			mutate: func(y string) string {
				return strings.Replace(y, `max_loop_iterations: 8`, `max_loop_iterations: 99`, 1)
			},
			contains: `max_loop_iterations must be an integer in [1, 32]`,
		},
		{
			name: "limits.timeout_ms below floor",
			mutate: func(y string) string {
				return strings.Replace(y, `timeout_ms: 30000`, `timeout_ms: 500`, 1)
			},
			contains: `timeout_ms must be an integer in [1000, 120000]`,
		},
		{
			name: "limits.schema_retry_budget over ceiling",
			mutate: func(y string) string {
				return strings.Replace(y, `schema_retry_budget: 2`, `schema_retry_budget: 99`, 1)
			},
			contains: `schema_retry_budget must be an integer in [0, 5]`,
		},
		{
			name: "limits.per_tool_timeout_ms above timeout_ms",
			mutate: func(y string) string {
				return strings.Replace(y, `per_tool_timeout_ms: 5000`, `per_tool_timeout_ms: 999999`, 1)
			},
			contains: `per_tool_timeout_ms must be an integer`,
		},
		{
			name: "x-redact on required output field",
			mutate: func(y string) string {
				return strings.Replace(y,
					"    answer: { type: string }",
					"    answer: { type: string, x-redact: true }",
					1)
			},
			contains: `x-redact: true`,
		},
		{
			name: "input_schema not valid JSON Schema",
			mutate: func(y string) string {
				// Make input_schema a string instead of a mapping.
				return strings.Replace(y,
					"input_schema:\n  type: object\n  required: [user_tz]\n  properties:\n    user_tz: { type: string }",
					`input_schema: "not a schema"`,
					1)
			},
			contains: `input_schema`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerLoaderTestTool(t)
			defer resetRegistryForTest()
			body := tc.mutate(validScenarioYAML)
			files := map[string]string{
				"bad.yaml":  body,
				"other.yml": "type: domain-extraction\nversion: x\n",
			}
			// Only include a "good sibling" when the rule does NOT mutate
			// the registry — otherwise the registry change would also
			// invalidate the sibling and confuse the assertion.
			if !tc.mutateRegistry {
				files["good.yaml"] = validScenarioYAML
			}
			dir := writeScenarioDir(t, files)
			registered, rejected := loadOK(t, dir)

			// The bad file MUST be rejected.
			if len(rejected) != 1 {
				t.Fatalf("expected exactly 1 rejection, got %d: %+v", len(rejected), rejected)
			}
			if !strings.HasSuffix(rejected[0].Path, "bad.yaml") {
				t.Errorf("rejection path mismatch: %q", rejected[0].Path)
			}
			if !strings.Contains(rejected[0].Message, tc.contains) {
				t.Errorf("rejection message %q does not contain %q", rejected[0].Message, tc.contains)
			}

			// The good sibling MUST still register (BS-009: per-file isolation).
			// Skip when the test mutated the global registry — see comment above.
			if tc.mutateRegistry {
				return
			}
			if len(registered) != 1 || registered[0].ID != "expense_question" {
				t.Errorf("good sibling did not register cleanly; got %+v", registered)
			}
		})
	}
}

// removeBlock deletes the lines from `startMarker` (inclusive) up to but
// not including `endMarker` from the YAML body. Used to strip required
// blocks.
func removeBlock(body, startMarker, endMarker string) string {
	startIdx := strings.Index(body, startMarker)
	endIdx := strings.Index(body, endMarker)
	if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
		return body
	}
	return body[:startIdx] + body[endIdx:]
}

func TestLoader_DuplicateID_FatalRefusesToStart(t *testing.T) {
	registerLoaderTestTool(t)
	defer resetRegistryForTest()
	dir := writeScenarioDir(t, map[string]string{
		"a.yaml": validScenarioYAML,
		"b.yaml": validScenarioYAML,
	})
	registered, _, err := DefaultLoader().Load(dir, "")
	if err == nil {
		t.Fatal("expected fatal error for duplicate scenario id")
	}
	if !strings.Contains(err.Error(), "duplicate scenario id") {
		t.Errorf("fatal error must mention duplicate scenario id: %v", err)
	}
	// Both file paths must appear in the message so the operator knows
	// which two files conflict.
	if !strings.Contains(err.Error(), "a.yaml") || !strings.Contains(err.Error(), "b.yaml") {
		t.Errorf("fatal error must name both file paths: %v", err)
	}
	// First-seen still registers so the linter can keep working.
	if len(registered) != 1 {
		t.Errorf("expected first scenario to remain registered; got %d", len(registered))
	}
}
