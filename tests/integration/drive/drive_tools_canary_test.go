//go:build integration

// Spec 038 Scope 7 Canary — Drive agent tools coexist with the spec 037
// recommendation tools and the broader agent registry.
//
// This canary fails fast if a regression in package init() ordering, in
// the registry locking, or in the schemas drops or shadows any tool. It
// imports both internal/drive/tools and internal/recommendation/tools
// for their side effect (registration) and asserts that the union of
// expected names is present and that no name appears more than once.
package drive

import (
	"testing"

	"github.com/smackerel/smackerel/internal/agent"

	// Side-effect imports register the tools in agent.DefaultRegistry.
	_ "github.com/smackerel/smackerel/internal/drive/tools"
	_ "github.com/smackerel/smackerel/internal/recommendation/tools"
)

func TestDriveToolsCanary_ExistingAgentToolsStillRegisterAndTrace(t *testing.T) {
	// Drive tools (spec 038 Scope 7) — must all be present.
	driveTools := []string{
		"drive_search",
		"drive_get_file",
		"drive_save_file",
		"drive_list_rules",
	}
	// Recommendation tools (spec 039 — sample of the 12 registered) —
	// proves the drive package's init() did not blow them out.
	recommendationTools := []string{
		"recommendation_parse_intent",
		"recommendation_fetch_candidates",
		"recommendation_persist_outcome",
		"recommendation_record_feedback",
	}

	for _, name := range driveTools {
		if !agent.Has(name) {
			t.Fatalf("drive tool %q missing from registry; init() ordering or registry race?", name)
		}
	}
	for _, name := range recommendationTools {
		if !agent.Has(name) {
			t.Fatalf("recommendation tool %q missing from registry; drive registration shadowed it?", name)
		}
	}

	// All registered tools must have unique names.
	seen := map[string]int{}
	for _, tool := range agent.All() {
		seen[tool.Name]++
	}
	for name, count := range seen {
		if count != 1 {
			t.Fatalf("tool %q registered %d times; registry de-dup invariant broken", name, count)
		}
	}

	// Adversarial: the registry MUST surface schemas for every drive
	// tool. A regression that registered a tool with a nil schema
	// would pass agent.Has but fail SchemasFor — this catches it.
	for _, name := range driveTools {
		input, output, ok := agent.SchemasFor(name)
		if !ok {
			t.Fatalf("SchemasFor(%q) ok = false", name)
		}
		if input == nil || output == nil {
			t.Fatalf("SchemasFor(%q) input=%v output=%v; both must be non-nil", name, input, output)
		}
	}
}
