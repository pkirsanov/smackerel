package tools

import (
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

func TestRegisterAll_RejectsNilRegistry(t *testing.T) {
	t.Parallel()
	err := RegisterAll(nil, Deps{GraphSearcher: &stubSearcher{}, WebSearchProvider: &fakeWebProvider{}})
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestRegisterAll_RejectsMissingDeps(t *testing.T) {
	t.Parallel()
	reg := ok.NewRegistry(nil)
	cases := map[string]Deps{
		"both-nil":     {},
		"no-searcher":  {WebSearchProvider: &fakeWebProvider{}},
		"no-websearch": {GraphSearcher: &stubSearcher{}},
	}
	for name, deps := range cases {
		deps := deps
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := RegisterAll(reg, deps); err == nil {
				t.Fatal("expected missing-deps error")
			}
		})
	}
}

func TestRegisterAll_RegistersAllFour(t *testing.T) {
	t.Parallel()
	allowlist := []string{"calculator", "internal_retrieval", "unit_convert", "web_search"}
	reg := ok.NewRegistry(allowlist)

	if err := RegisterAll(reg, Deps{
		GraphSearcher:     &stubSearcher{},
		WebSearchProvider: &fakeWebProvider{},
	}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	enabled := reg.Enabled()
	if len(enabled) != 4 {
		t.Fatalf("enabled count: got %d want 4", len(enabled))
	}
	want := []string{"calculator", "internal_retrieval", "unit_convert", "web_search"}
	for i, w := range want {
		if enabled[i].Name() != w {
			t.Fatalf("enabled[%d]: got %q want %q (alphabetic order)", i, enabled[i].Name(), w)
		}
	}
}

func TestRegisterAll_DuplicateCallFails(t *testing.T) {
	t.Parallel()
	reg := ok.NewRegistry([]string{"calculator"})
	deps := Deps{GraphSearcher: &stubSearcher{}, WebSearchProvider: &fakeWebProvider{}}
	if err := RegisterAll(reg, deps); err != nil {
		t.Fatalf("first RegisterAll: %v", err)
	}
	if err := RegisterAll(reg, deps); err == nil {
		t.Fatal("expected duplicate-tool error on second RegisterAll")
	}
}

func TestRegisterAll_AllowlistFiltersEnabled(t *testing.T) {
	t.Parallel()
	// Only allowlist calculator + web_search; verify the other two
	// are Registered but NOT Enabled.
	reg := ok.NewRegistry([]string{"calculator", "web_search"})
	if err := RegisterAll(reg, Deps{
		GraphSearcher:     &stubSearcher{},
		WebSearchProvider: &fakeWebProvider{},
	}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	enabled := reg.Enabled()
	if len(enabled) != 2 {
		t.Fatalf("enabled count: got %d want 2", len(enabled))
	}
	if enabled[0].Name() != "calculator" || enabled[1].Name() != "web_search" {
		t.Fatalf("enabled order: got %s,%s", enabled[0].Name(), enabled[1].Name())
	}
	// Adversarial: a non-allowlisted tool MUST surface ErrToolNotAllowed
	// rather than ErrUnknownTool (the design contract distinguishes
	// "never registered" from "registered but denied").
	if _, err := reg.Lookup("internal_retrieval"); err == nil {
		t.Fatal("expected ErrToolNotAllowed for non-allowlisted tool")
	}
}
