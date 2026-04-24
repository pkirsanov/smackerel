//go:build integration

package agent_integration

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// Adversarial regression for BS-009 (malformed scenario rejection).
// Every required top-level field MUST, when omitted, trigger a structured
// rejection that names the missing field — and a sibling valid scenario
// in the same directory MUST still register cleanly. This guarantees
// that one malformed file cannot wipe an entire scenario directory.
func TestLoader_BS009_MalformedScenarioRejectionsAreIsolated(t *testing.T) {
	registerToolsForLoader(t)

	requiredFields := []struct {
		name   string
		marker string // distinct line(s) to remove from the YAML body
	}{
		{name: "id", marker: `id: "expense_question"` + "\n"},
		{name: "system_prompt", marker: "system_prompt: |\n  You are the expense agent.\n"},
		{name: "allowed_tools", marker: "allowed_tools:\n  - name: \"search_expenses\"\n    side_effect_class: \"read\"\n"},
		{name: "input_schema", marker: "input_schema:\n  type: object\n  required: [user_tz]\n  properties:\n    user_tz: { type: string }\n"},
		{name: "output_schema", marker: "output_schema:\n  type: object\n  required: [answer]\n  properties:\n    answer: { type: string }\n"},
		{name: "limits", marker: "limits:\n  max_loop_iterations: 8\n  timeout_ms: 30000\n  schema_retry_budget: 2\n  per_tool_timeout_ms: 5000\n"},
		{name: "side_effect_class", marker: "\nside_effect_class: \"read\"\n"},
	}

	for _, rf := range requiredFields {
		t.Run("missing_"+rf.name, func(t *testing.T) {
			body := strings.Replace(validScenarioYAML, rf.marker, "", 1)
			if body == validScenarioYAML {
				t.Fatalf("test setup error: marker for %s did not match the fixture body", rf.name)
			}
			dir := writeFiles(t, map[string]string{
				"bad.yaml":  body,
				"good.yaml": validScenarioYAML,
			})

			registered, rejected, fatal := agent.DefaultLoader().Load(dir, "")
			if fatal != nil {
				t.Fatalf("unexpected fatal: %v", fatal)
			}
			if len(rejected) != 1 {
				t.Fatalf("expected exactly 1 rejection for missing %q; got %+v", rf.name, rejected)
			}
			msg := rejected[0].Message
			if !strings.Contains(msg, rf.name) {
				t.Errorf("rejection for missing %q did not name the field: %q", rf.name, msg)
			}
			// Sibling MUST still register — BS-009 isolation.
			if len(registered) != 1 || registered[0].ID != "expense_question" {
				t.Errorf("good sibling did not register cleanly when %q was missing; got %+v", rf.name, registered)
			}
		})
	}
}
