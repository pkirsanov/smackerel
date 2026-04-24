//go:build integration

package agent_integration

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// Adversarial regression for BS-011: two scenarios sharing the same id
// MUST cause the loader to return a fatal error so the production
// process refuses to start. The error message MUST name both file
// paths so the operator can resolve the conflict without guessing.
func TestLoader_BS011_DuplicateIDIsFatalAndNamesBothFiles(t *testing.T) {
	registerToolsForLoader(t)

	// Two distinct files declaring the same scenario id ("expense_question").
	dir := writeFiles(t, map[string]string{
		"first.yaml":  validScenarioYAML,
		"second.yaml": validScenarioYAML,
	})

	_, _, fatal := agent.DefaultLoader().Load(dir, "")
	if fatal == nil {
		t.Fatal("expected fatal error for duplicate scenario id; got nil")
	}
	msg := fatal.Error()
	if !strings.Contains(msg, "expense_question") {
		t.Errorf("fatal error must name the duplicate id: %q", msg)
	}
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	if !strings.Contains(msg, first) || !strings.Contains(msg, second) {
		t.Errorf("fatal error must name BOTH file paths; got %q (want both %q and %q)",
			msg, first, second)
	}
	if !strings.Contains(msg, "process refuses to start") {
		t.Errorf("fatal error must declare process refusal: %q", msg)
	}
}
