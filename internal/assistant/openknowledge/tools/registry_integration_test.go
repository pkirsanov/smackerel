package tools

import (
	"context"
	"encoding/json"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

// TestRegistryIntegration_ExecuteThroughInterface registers both
// concrete tools, looks each up via the registry (gated by allowlist),
// invokes Execute through the openknowledge.Tool interface (not the
// concrete type), and asserts the ToolResult envelope.
func TestRegistryIntegration_ExecuteThroughInterface(t *testing.T) {
	t.Parallel()

	allowlist := []string{"unit_convert", "calculator"}
	reg := ok.NewRegistry(allowlist)

	if err := reg.Register(NewUnitConvert()); err != nil {
		t.Fatalf("register unit_convert: %v", err)
	}
	if err := reg.Register(NewCalculator()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}

	cases := []struct {
		toolName string
		params   map[string]any
	}{
		{"unit_convert", map[string]any{"value": 10.0, "from_unit": "F", "to_unit": "C"}},
		{"calculator", map[string]any{"expression": "3 + 4 * 2"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.toolName, func(t *testing.T) {
			t.Parallel()
			var tool ok.Tool
			tool, err := reg.Lookup(tc.toolName)
			if err != nil {
				t.Fatalf("lookup %s: %v", tc.toolName, err)
			}
			if tool.Name() != tc.toolName {
				t.Fatalf("name: got %q want %q", tool.Name(), tc.toolName)
			}
			if len(tool.ParamsSchema()) == 0 {
				t.Fatalf("empty params schema for %s", tc.toolName)
			}
			body, err := json.Marshal(tc.params)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			res, err := tool.Execute(context.Background(), body)
			if err != nil {
				t.Fatalf("hard error: %v", err)
			}
			if res == nil {
				t.Fatal("nil result")
			}
			if res.Error != nil {
				t.Fatalf("tool error: %v", res.Error)
			}
			if res.Computation == nil || res.Computation.Tool != tc.toolName {
				t.Fatalf("computation envelope: %+v", res.Computation)
			}
			if len(res.Sources) != 1 || res.Sources[0].Kind != ok.SourceToolComputation {
				t.Fatalf("expected single tool-computation source, got %+v", res.Sources)
			}
			if res.Sources[0].Computation == nil || res.Sources[0].Computation.Tool != tc.toolName {
				t.Fatalf("computation source: %+v", res.Sources[0])
			}
		})
	}

	// Enabled() must list both, in deterministic order.
	enabled := reg.Enabled()
	if len(enabled) != 2 {
		t.Fatalf("enabled count: got %d want 2", len(enabled))
	}
	if enabled[0].Name() != "calculator" || enabled[1].Name() != "unit_convert" {
		t.Fatalf("enabled order: got %s,%s", enabled[0].Name(), enabled[1].Name())
	}
}
