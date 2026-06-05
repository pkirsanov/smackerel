// Package config — contract test for `config/prompt_contracts/*.yaml`.
//
// 21 prompt contracts drive the LLM-facing surfaces of every Smackerel
// agent / scenario / extractor. None of them had Go-side validation
// today — they were loaded ad-hoc by each consumer and a regression
// that (a) dropped the `version` field, (b) silently truncated
// `system_prompt`, (c) invented a non-canonical `type`, or (d) named
// a `version` that didn't match the filename would slip past
// `./smackerel.sh test unit` and break:
//   - the agent scenario router (selects contracts by version)
//   - the LLM gateway (selects model_preference / temperature)
//   - the spec-061 scenario manifest cross-check
//   - operator runbooks that reference contract version strings
//
// This Go contract test enforces structural invariants across ALL 21
// contracts in one sweep so a regression fails at `go test ./...` time.
// Adversarial sub-tests prove each invariant catches its target
// failure mode.
//
// References:
//   - config/prompt_contracts/*.yaml (21 contracts)
//   - internal/agent/config.go (KnowledgePromptContract* fields)
//   - internal/config/assistant_intent_compiler.go (PromptContractVersion)
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type promptContractDoc struct {
	ID                 string   `yaml:"id"`
	Version            string   `yaml:"version"`
	Type               string   `yaml:"type"`
	Description        string   `yaml:"description"`
	SystemPrompt       string   `yaml:"system_prompt"`
	PrincipleAlignment []string `yaml:"principleAlignment"`
}

// Canonical contract types. New types are allowed, but additions must be
// intentional — if a typo silently introduces "scenarios" or "Scenario"
// (case-mismatch), the agent router will silently fail to match.
var validPromptContractTypes = map[string]bool{
	"scenario":                true,
	"cross-source-connection": true,
	"digest-assembly":         true,
	"drive-classification":    true,
	"drive-folder-context":    true,
	"ingest-synthesis":        true,
	"lint-audit":              true,
	"domain-extraction":       true,
	"query-augment":           true,
}

// Minimum substantive content thresholds. Prevents accidental truncation
// where someone replaces a multi-paragraph system_prompt with an
// empty-string or one-liner.
const (
	minSystemPromptLen = 100 // shortest live system_prompt is ~229 chars
	minDescriptionLen  = 10
)

func promptContractsRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// expectedVersion infers the canonical version string from the filename.
// Most contracts follow the pattern <basename>.yaml -> version: "<basename>"
// (e.g., ingest-synthesis-v1.yaml -> "ingest-synthesis-v1"). One exception:
// open_knowledge.yaml -> "open-knowledge-v1" (underscore -> hyphen, plus
// "-v1" suffix). This helper encodes the convention so the contract test
// catches regressions in either direction.
func expectedVersion(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), ".yaml")
	// open_knowledge.yaml is the one known exception — historically named
	// with underscore (legacy) but version uses hyphen + v1 suffix.
	if base == "open_knowledge" {
		return "open-knowledge-v1"
	}
	return base
}

// assertPromptContract validates a single prompt-contract document.
func assertPromptContract(filename string, yamlBytes []byte) error {
	var doc promptContractDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed for %s: %w", filename, err)
	}
	base := filepath.Base(filename)

	if strings.TrimSpace(doc.Version) == "" {
		return fmt.Errorf("contract violation: %s missing required field 'version'", base)
	}
	wantVersion := expectedVersion(filename)
	if doc.Version != wantVersion {
		return fmt.Errorf("contract violation: %s declares version=%q but filename implies version=%q (the agent router selects contracts by version; mismatched version silently disables this contract)", base, doc.Version, wantVersion)
	}

	if strings.TrimSpace(doc.Type) == "" {
		return fmt.Errorf("contract violation: %s missing required field 'type'", base)
	}
	if !validPromptContractTypes[doc.Type] {
		return fmt.Errorf("contract violation: %s declares type=%q which is not in the canonical set (%v) — a typo or invented type silently breaks the agent router type-dispatch", base, doc.Type, sortedKeys(validPromptContractTypes))
	}

	if len(strings.TrimSpace(doc.Description)) < minDescriptionLen {
		return fmt.Errorf("contract violation: %s description is too short (%d chars; min %d) — likely truncation or empty placeholder", base, len(strings.TrimSpace(doc.Description)), minDescriptionLen)
	}

	if len(strings.TrimSpace(doc.SystemPrompt)) < minSystemPromptLen {
		return fmt.Errorf("contract violation: %s system_prompt is too short (%d chars; min %d) — likely truncation or empty placeholder; the LLM has no behavior without a substantive system prompt", base, len(strings.TrimSpace(doc.SystemPrompt)), minSystemPromptLen)
	}

	if len(doc.PrincipleAlignment) == 0 {
		return fmt.Errorf("contract violation: %s missing principleAlignment[] (every prompt contract must declare which product principles it serves; empty list slips past traceability)", base)
	}

	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Trivial sort — small fixed map.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// TestPromptContracts_LiveFiles validates every live prompt contract.
// Failure names the offending file so a regression is immediately
// actionable.
func TestPromptContracts_LiveFiles(t *testing.T) {
	root := promptContractsRepoRoot(t)
	dir := filepath.Join(root, "config", "prompt_contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := assertPromptContract(path, b); err != nil {
			t.Errorf("live contract violates invariants: %v", err)
		}
		count++
	}
	if count == 0 {
		t.Fatalf("no prompt contracts found in %s — directory deletion or rename would silently break the agent router", dir)
	}
	t.Logf("validated %d prompt contracts", count)
}

// TestPromptContracts_AdversarialMissingVersion proves the contract test
// would FAIL if a contract dropped its 'version' field (the agent router
// selects contracts by version; missing version makes the contract
// unselectable and silently broken).
func TestPromptContracts_AdversarialMissingVersion(t *testing.T) {
	bad := []byte(`type: scenario
description: "A test contract."
system_prompt: |
  You are a test agent. Do the test thing. Return JSON.
  This is enough text to clear the 100-char minimum prompt length.
principleAlignment:
- "Principle 1"
`)
	err := assertPromptContract("test-contract-v1.yaml", bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted contract without version — agent router would silently fail to find this contract")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version-missing rejection, got: %v", err)
	}
}

// TestPromptContracts_AdversarialVersionFilenameMismatch proves the
// contract test would FAIL if a contract's version string disagrees with
// its filename (e.g., a copy/paste of one contract into a new file
// without updating the version field).
func TestPromptContracts_AdversarialVersionFilenameMismatch(t *testing.T) {
	bad := []byte(`version: "different-contract-v1"
type: scenario
description: "Test."
system_prompt: |
  Long enough system prompt to clear the minimum length threshold for
  the contract test which is one hundred characters of substance.
principleAlignment:
- "Principle 1"
`)
	err := assertPromptContract("expected-name-v1.yaml", bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted version that doesn't match filename — copy/paste bug would silently mis-route the contract")
	}
	if !strings.Contains(err.Error(), "filename implies") {
		t.Fatalf("expected filename-mismatch message, got: %v", err)
	}
}

// TestPromptContracts_AdversarialInvalidType proves the contract test
// would FAIL if a contract regressed to a non-canonical type (typo,
// case mismatch, invented type).
func TestPromptContracts_AdversarialInvalidType(t *testing.T) {
	bad := []byte(`version: "bad-type-v1"
type: Scenario
description: "Adversarial test contract."
system_prompt: |
  Long enough system prompt to clear the minimum length threshold for
  the contract test which is one hundred characters of substance.
principleAlignment:
- "Principle 1"
`)
	err := assertPromptContract("bad-type-v1.yaml", bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted type='Scenario' (case mismatch) — would silently break agent router")
	}
	if !strings.Contains(err.Error(), "type=") {
		t.Fatalf("expected type-rejection message, got: %v", err)
	}
}

// TestPromptContracts_AdversarialEmptySystemPrompt proves the contract
// test would FAIL if a contract's system_prompt were truncated to empty
// or near-empty. An LLM with no system prompt has no behavior.
func TestPromptContracts_AdversarialEmptySystemPrompt(t *testing.T) {
	bad := []byte(`version: "empty-prompt-v1"
type: scenario
description: "Adversarial test contract."
system_prompt: "TODO"
principleAlignment:
- "Principle 1"
`)
	err := assertPromptContract("empty-prompt-v1.yaml", bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted near-empty system_prompt — LLM would have no behavior")
	}
	if !strings.Contains(err.Error(), "system_prompt") {
		t.Fatalf("expected system_prompt rejection, got: %v", err)
	}
}

// TestPromptContracts_AdversarialMissingPrincipleAlignment proves the
// contract test would FAIL if principleAlignment[] were empty — every
// contract must declare which product principles it serves so
// traceability holds.
func TestPromptContracts_AdversarialMissingPrincipleAlignment(t *testing.T) {
	bad := []byte(`version: "no-principle-v1"
type: scenario
description: "Adversarial test contract."
system_prompt: |
  Long enough system prompt to clear the minimum length threshold for
  the contract test which is one hundred characters of substance.
principleAlignment: []
`)
	err := assertPromptContract("no-principle-v1.yaml", bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted empty principleAlignment[] — would slip past Smackerel principle-traceability check")
	}
	if !strings.Contains(err.Error(), "principleAlignment") {
		t.Fatalf("expected principleAlignment rejection, got: %v", err)
	}
}
