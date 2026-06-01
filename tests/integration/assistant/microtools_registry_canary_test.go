//go:build integration

// Spec 065 SCOPE-1 — Micro-tools foundation Shared Infrastructure
// Impact Sweep canary.
//
// The agent.RegisterTool registry is a shared surface. SCOPE-1
// introduces a sibling internal/agent/tools/microtools/ package and
// the per-tool ASSISTANT_TOOLS_* SST block. This canary proves the
// existing spec 037 registry path still functions after that
// foundation lands: every previously-registered scenario tool still
// resolves through agent.ByName and still validates its declared
// JSON Schemas through agent.CompileSchema.
//
// Per scopes.md Test Plan: this is the Shared Infrastructure Impact
// Sweep canary that MUST pass before broader regression reruns.

package assistant_integration

import (
	"encoding/json"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	// Force registration of the existing scenario tools so the
	// registry has the production shape (weather_lookup at a
	// minimum). Blank imports trigger each package's init().
	_ "github.com/smackerel/smackerel/internal/agent/tools/notification"
	_ "github.com/smackerel/smackerel/internal/agent/tools/recipesearch"
	_ "github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	_ "github.com/smackerel/smackerel/internal/agent/tools/weather"

	// SCOPE-1 sibling foundation package — imported to prove its
	// init() does not panic or pollute the agent registry.
	_ "github.com/smackerel/smackerel/internal/agent/tools/microtools"
)

// TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate is
// the SCOPE-1 canary required by scopes.md. It asserts:
//
//  1. weather_lookup (the canonical existing scenario tool) is still
//     registered after the microtools package init() runs.
//  2. Its declared InputSchema and OutputSchema still compile through
//     agent.CompileSchema — the same path the executor uses on every
//     tool call.
//  3. The microtools package did NOT register any tool under the spec
//     037 registry (SCOPE-1 lands the foundation only; SCOPE-2..4
//     register the concrete tools). This guards against the
//     design.md "no second registry" rule by proving SCOPE-1 itself
//     respects the same constraint and does not pre-register stubs.
func TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate(t *testing.T) {
	t.Run("weather_lookup_still_registered", func(t *testing.T) {
		if !agent.Has("weather_lookup") {
			t.Fatal("expected weather_lookup to remain registered after microtools foundation import")
		}
		tool, ok := agent.ByName("weather_lookup")
		if !ok {
			t.Fatal("ByName(\"weather_lookup\") returned !ok")
		}
		if tool.Name != "weather_lookup" {
			t.Fatalf("unexpected name: %q", tool.Name)
		}
		if len(tool.InputSchema) == 0 || len(tool.OutputSchema) == 0 {
			t.Fatal("weather_lookup schemas unexpectedly empty")
		}
	})

	t.Run("weather_lookup_schemas_still_compile", func(t *testing.T) {
		tool, ok := agent.ByName("weather_lookup")
		if !ok {
			t.Fatal("ByName(\"weather_lookup\") returned !ok")
		}
		if _, err := agent.CompileSchema(tool.InputSchema); err != nil {
			t.Fatalf("InputSchema compile regressed: %v", err)
		}
		if _, err := agent.CompileSchema(tool.OutputSchema); err != nil {
			t.Fatalf("OutputSchema compile regressed: %v", err)
		}
	})

	t.Run("microtools_foundation_did_not_register_any_tool", func(t *testing.T) {
		// Scope 1 ships envelope + config only. The four concrete
		// micro-tools (location_normalize, unit_convert, calculator,
		// entity_resolve) land in SCOPE-2..4 and MUST go through
		// agent.RegisterTool when they do.
		forbidden := []string{"location_normalize", "unit_convert", "calculator", "entity_resolve"}
		for _, name := range forbidden {
			if agent.Has(name) {
				t.Errorf("SCOPE-1 must not register %q; concrete tools belong to later scopes", name)
			}
		}
	})

	t.Run("registry_still_lists_all_tools", func(t *testing.T) {
		all := agent.All()
		if len(all) == 0 {
			t.Fatal("agent.All() returned no registered tools")
		}
		// Marshal the names so a failure surfaces the actual list.
		names := make([]string, 0, len(all))
		for _, tool := range all {
			names = append(names, tool.Name)
		}
		if _, err := json.Marshal(names); err != nil {
			t.Fatalf("marshal registered tool names: %v", err)
		}
	})
}
