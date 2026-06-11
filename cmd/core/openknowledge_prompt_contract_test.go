// Spec 084 — prompt-content guard for the question-AGNOSTIC reasoning prompt.
//
// These tests read the REAL config/prompt_contracts/open_knowledge.yaml
// agent_system_prompt (via loadOpenKnowledgeAgentPrompt) and assert the
// spec-084 reasoning contract is in force while the spec-064 trust contract is
// preserved verbatim. Non-tautological: the "question-agnostic" test FAILS on
// the pre-spec-084 prompt (which carried the anti-drill bias and the
// BUG-064-002 question-type enumeration, and lacked the decompose/reconcile
// markers) and PASSES only after the rewrite.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootForPromptTest walks up from the test's working directory until it
// finds the module go.mod, so the prompt path resolves regardless of where the
// test binary is run from (host or containerized runner).
func repoRootForPromptTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (go.mod) walking up from test cwd")
		}
		dir = parent
	}
}

func loadRealOpenKnowledgePrompt(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRootForPromptTest(t), "config", "prompt_contracts", "open_knowledge.yaml")
	prompt, err := loadOpenKnowledgeAgentPrompt(path)
	if err != nil {
		t.Fatalf("load real open_knowledge agent prompt: %v", err)
	}
	if strings.TrimSpace(prompt) == "" {
		t.Fatalf("agent_system_prompt is empty")
	}
	return prompt
}

// TestOpenKnowledgeAgentPrompt_IsQuestionAgnostic_Spec084 — ADVERSARIAL.
// Asserts the anti-drill bias and the BUG-064-002 question-type enumeration
// are REMOVED and the decompose/gather-all-sides/reconcile/answer reasoning
// contract is PRESENT. RED on the pre-spec-084 prompt.
func TestOpenKnowledgeAgentPrompt_IsQuestionAgnostic_Spec084(t *testing.T) {
	prompt := loadRealOpenKnowledgePrompt(t)

	// (1) anti-drill bias REMOVED.
	for _, banned := range []string{
		"write the final answer in the NEXT turn",
		"Do NOT keep calling the same tool",
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("SCN-084-A01: anti-drill bias must be removed; prompt still contains %q", banned)
		}
	}

	// (2) BUG-064-002 question-type enumeration REMOVED (and not replaced with
	// another fixed question-type list).
	for _, banned := range []string{
		"times, prices, temperatures",
		"highs/lows, a schedule, a table",
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("SCN-084-A01: question-type enumeration must be removed; prompt still contains %q", banned)
		}
	}

	// (3) question-agnostic reasoning contract PRESENT.
	for _, required := range []string{
		"DECOMPOSE",
		"GATHER",
		"RECONCILE",
		"ANSWER THE ACTUAL QUESTION",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("SCN-084-A01: reasoning contract missing the %q step", required)
		}
	}
	// The contract must name reconciling contradictions and covering all sides
	// of a comparison, in general terms (not a closed question-type list).
	for _, required := range []string{
		"contradict",
		"both",
	} {
		if !strings.Contains(strings.ToLower(prompt), required) {
			t.Fatalf("SCN-084-A01: reasoning contract should reference %q (reconcile / cover-all-sides)", required)
		}
	}
}

// TestOpenKnowledgeAgentPrompt_PreservesCitationContract_Spec084 — GUARD.
// Asserts the spec-064 trust contract (R1-R4 hard rules, the <CITATIONS>
// block, the three citation shapes, the honest-refusal shape) is preserved
// verbatim by the spec-084 rewrite (FR-4).
func TestOpenKnowledgeAgentPrompt_PreservesCitationContract_Spec084(t *testing.T) {
	prompt := loadRealOpenKnowledgePrompt(t)
	for _, required := range []string{
		"R1.", "R2.", "R3.", "R4.",
		"<CITATIONS>",
		"</CITATIONS>",
		"<CITATIONS>[]</CITATIONS>", // honest-refusal shape
		`"kind": "artifact"`,
		`"kind": "web"`,
		`"kind": "tool_computation"`,
		"Do not invent URLs",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("SCN-084-A01 (FR-4): the preserved trust contract is missing %q", required)
		}
	}
}
