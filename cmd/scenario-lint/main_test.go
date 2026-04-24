package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The real config/prompt_contracts/ directory contains existing prompt
// contracts (type != "scenario"). Those are silently skipped by the
// loader, so the linter must exit 0.
func TestScenarioLint_RealPromptContractsDir_ExitsZero(t *testing.T) {
	// Walk up from the test file to repo root, then to config/prompt_contracts/.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Dir(filepath.Dir(wd)) // cmd/scenario-lint -> repo root
	target := filepath.Join(repoRoot, "config", "prompt_contracts")
	if _, err := os.Stat(target); err != nil {
		t.Skipf("config/prompt_contracts not found at %s: %v", target, err)
	}
	code := run([]string{target}, os.Stdout, os.Stderr)
	if code != 0 {
		t.Fatalf("scenario-lint against %s exited %d; want 0", target, code)
	}
}

// Sanity: usage error path returns 2 when no directory argument is supplied.
func TestScenarioLint_MissingArg_ReturnsUsageError(t *testing.T) {
	if code := run(nil, os.Stdout, os.Stderr); code != 2 {
		t.Fatalf("expected usage exit 2, got %d", code)
	}
}
