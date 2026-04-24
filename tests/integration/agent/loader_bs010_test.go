//go:build integration

package agent_integration

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// Adversarial regression for BS-010: a scenario that allowlists a tool
// not present in the registry MUST be rejected with a structured error
// naming both the missing tool and the scenario id (transitively, via
// the file path). Sibling valid scenarios MUST still register.
func TestLoader_BS010_UnknownToolRejectsScenarioOnly(t *testing.T) {
	registerToolsForLoader(t)

	// Replace the legitimate allowlisted tool with a bogus name.
	bogus := strings.Replace(validScenarioYAML,
		`  - name: "search_expenses"`,
		`  - name: "extract_rainbow"`,
		1)

	dir := writeFiles(t, map[string]string{
		"bad.yaml":  bogus,
		"good.yaml": validScenarioYAML,
	})

	registered, rejected, fatal := agent.DefaultLoader().Load(dir, "")
	if fatal != nil {
		t.Fatalf("unexpected fatal: %v", fatal)
	}
	if len(rejected) != 1 {
		t.Fatalf("expected exactly 1 rejection; got %+v", rejected)
	}
	msg := rejected[0].Message
	if !strings.Contains(msg, "extract_rainbow") {
		t.Errorf("rejection must name the missing tool: %q", msg)
	}
	if !strings.Contains(rejected[0].Path, "bad.yaml") {
		t.Errorf("wrong file rejected: %q", rejected[0].Path)
	}
	if !strings.Contains(msg, "not in the tool registry") {
		t.Errorf("rejection should describe registry miss: %q", msg)
	}
	if len(registered) != 1 || registered[0].ID != "expense_question" {
		t.Errorf("valid sibling failed to register; got %+v", registered)
	}
}
