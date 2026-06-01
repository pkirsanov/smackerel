package openknowledge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// fakeTool is a minimal Tool implementation used by the registry tests.
// Execute is never called from these tests; the table cases only
// exercise registration, allowlist gating, and ordering.
type fakeTool struct{ name string }

func (f *fakeTool) Name() string                  { return f.name }
func (f *fakeTool) Description() string           { return "fake tool " + f.name }
func (f *fakeTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
	return &ToolResult{}, nil
}

func TestRegistryRegisterSuccess(t *testing.T) {
	r := NewRegistry([]string{"calculator"})
	if err := r.Register(&fakeTool{name: "calculator"}); err != nil {
		t.Fatalf("Register returned unexpected error: %v", err)
	}
}

func TestRegistryRegisterRejectsDuplicate(t *testing.T) {
	r := NewRegistry([]string{"calculator"})
	if err := r.Register(&fakeTool{name: "calculator"}); err != nil {
		t.Fatalf("first Register returned unexpected error: %v", err)
	}
	err := r.Register(&fakeTool{name: "calculator"})
	if !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool, got %v", err)
	}
}

func TestRegistryRegisterRejectsNilAndEmptyName(t *testing.T) {
	r := NewRegistry([]string{"x"})
	if err := r.Register(nil); !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool for nil tool, got %v", err)
	}
	if err := r.Register(&fakeTool{name: ""}); !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool for empty name, got %v", err)
	}
}

func TestRegistryLookupAllowed(t *testing.T) {
	r := NewRegistry([]string{"calculator"})
	tool := &fakeTool{name: "calculator"}
	if err := r.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Lookup("calculator")
	if err != nil {
		t.Fatalf("Lookup returned unexpected error: %v", err)
	}
	if got != tool {
		t.Fatalf("Lookup returned %v; want %v", got, tool)
	}
}

func TestRegistryLookupDeniedByAllowlist(t *testing.T) {
	r := NewRegistry([]string{"calculator"}) // web_search registered but not allowed
	if err := r.Register(&fakeTool{name: "web_search"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err := r.Lookup("web_search")
	if !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("expected ErrToolNotAllowed, got %v", err)
	}
}

func TestRegistryLookupUnknownTool(t *testing.T) {
	r := NewRegistry([]string{"calculator"})
	_, err := r.Lookup("does_not_exist")
	if !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("expected ErrUnknownTool, got %v", err)
	}
}

func TestRegistryEnabledDeterministicOrdering(t *testing.T) {
	allowlist := []string{"calculator", "internal_retrieval", "unit_convert", "web_search"}
	// Register in three different permutations and confirm Enabled()
	// returns the same sorted slice every time.
	permutations := [][]string{
		{"web_search", "calculator", "internal_retrieval", "unit_convert"},
		{"unit_convert", "internal_retrieval", "web_search", "calculator"},
		{"calculator", "web_search", "unit_convert", "internal_retrieval"},
	}
	var first []string
	for i, perm := range permutations {
		r := NewRegistry(allowlist)
		for _, name := range perm {
			if err := r.Register(&fakeTool{name: name}); err != nil {
				t.Fatalf("perm %d Register %q: %v", i, name, err)
			}
		}
		enabled := r.Enabled()
		names := make([]string, 0, len(enabled))
		for _, tool := range enabled {
			names = append(names, tool.Name())
		}
		if first == nil {
			first = names
			continue
		}
		if len(names) != len(first) {
			t.Fatalf("perm %d length %d != first %d", i, len(names), len(first))
		}
		for j := range names {
			if names[j] != first[j] {
				t.Fatalf("perm %d ordering diverged at %d: got %q want %q", i, j, names[j], first[j])
			}
		}
	}
	// Confirm the deterministic order is the alphabetical order we
	// promised callers in the package docs.
	want := []string{"calculator", "internal_retrieval", "unit_convert", "web_search"}
	for i := range want {
		if first[i] != want[i] {
			t.Fatalf("Enabled()[%d] = %q; want %q", i, first[i], want[i])
		}
	}
}

func TestRegistryNilAllowlistDeniesAll(t *testing.T) {
	r := NewRegistry(nil)
	if err := r.Register(&fakeTool{name: "calculator"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := r.Lookup("calculator"); !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("expected ErrToolNotAllowed under nil allowlist, got %v", err)
	}
	if got := r.Enabled(); len(got) != 0 {
		t.Fatalf("Enabled() under nil allowlist = %d tools; want 0", len(got))
	}
	// Explicit empty allowlist must behave identically to nil.
	r2 := NewRegistry([]string{})
	if err := r2.Register(&fakeTool{name: "calculator"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := r2.Lookup("calculator"); !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("empty allowlist: expected ErrToolNotAllowed, got %v", err)
	}
}
