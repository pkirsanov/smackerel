// Adversarial regression test for spec 061 SCOPE-03: asserts that every
// spec-061 v1 assistant scenario in config/prompt_contracts/ loads against
// the live spec 037 tool registry without rejection AND that each
// required tool name is in fact registered.
//
// This test fails loudly if:
//   - any of the four spec-061 tools (retrieval_search, weather_lookup,
//     notification_propose, notification_execute) is removed from
//     registration (e.g. blank import dropped or init() deleted);
//   - any spec-061 scenario YAML references a tool name that is not in
//     the registry;
//   - any spec-061 scenario YAML otherwise fails Spec 037 loader
//     validation (schema drift, allowlist drift, side-effect class
//     escalation, etc.).
//
// The check is performed against the LIVE config/prompt_contracts/
// directory so a stale staging copy cannot mask a regression in the
// shipping artifacts.

package main

import (
	"path/filepath"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

func TestSpec061_AssistantToolsRegistered(t *testing.T) {
	required := []string{
		"retrieval_search",
		"weather_lookup",
		"notification_propose",
		"notification_execute",
	}
	for _, name := range required {
		if !agent.Has(name) {
			t.Errorf("spec-061 required tool %q is NOT in agent tool registry — registration regression (blank import dropped or init() removed)", name)
		}
	}
}

func TestSpec061_LivePromptContractsLoadCleanly(t *testing.T) {
	// Resolve repo-root config/prompt_contracts/ relative to this test's
	// package directory (cmd/scenario-lint/). Climb two levels.
	dir, err := filepath.Abs("../../config/prompt_contracts")
	if err != nil {
		t.Fatalf("resolve dir: %v", err)
	}
	registered, rejected, fatal := agent.DefaultLoader().Load(dir, "")
	if fatal != nil {
		t.Fatalf("loader fatal: %v", fatal)
	}
	if len(rejected) != 0 {
		for _, r := range rejected {
			t.Errorf("scenario rejected: %s -- %s", r.Path, r.Message)
		}
		t.Fatalf("expected 0 rejected scenarios in live config/prompt_contracts/, got %d (regression: a scenario references an unregistered tool or a spec-061 scenario YAML drifted)", len(rejected))
	}
	// Sanity floor: 5 pre-existing + 3 spec-061 v1 = 8.
	if len(registered) < 8 {
		t.Fatalf("expected at least 8 registered scenarios (5 pre-existing + 3 spec-061 v1), got %d", len(registered))
	}

	wantIDs := map[string]bool{
		"retrieval_qa":          false,
		"weather_query":         false,
		"notification_schedule": false,
	}
	for _, s := range registered {
		if _, ok := wantIDs[s.ID]; ok {
			wantIDs[s.ID] = true
		}
	}
	for id, seen := range wantIDs {
		if !seen {
			t.Errorf("spec-061 v1 scenario %q missing from live config/prompt_contracts/ (or failed loader validation)", id)
		}
	}
}
