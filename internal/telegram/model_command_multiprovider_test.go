// Spec 096 SCOPE-07 (SCN-096-D02) — the Telegram `/model` COMBINED picker tests.
//
// They drive the pure renderCombinedPicker against an injected fake catalog +
// the REAL modelswitch validator (built FROM that catalog via cat.Allowlist())
// + the real fake modelpref store, so the provider-grouped rendering, the
// only-reachable-numbered rule, and the unreachable-shown-disabled rule are
// verified WITHOUT a live stack. No request interception — correctly classified
// unit (the rendering + numbering is a pure mechanism). The live Telegram
// pick→persist→dispatch leg (TestTelegram_PickHostedModelPersists_Spec096) is
// the deferred home-lab e2e-api dispatch.
package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// fakeCatalogProvider is an injected combined-catalog source for the picker
// tests (satisfies agenttool.CatalogProvider).
type fakeCatalogProvider struct {
	cat      catalog.ModelCatalog
	statuses []catalog.ProviderDiscoveryStatus
}

func (f *fakeCatalogProvider) GetCatalog(_ context.Context) (catalog.ModelCatalog, []catalog.ProviderDiscoveryStatus) {
	return f.cat, f.statuses
}

// fakeBudgetProvider returns a canned month-to-date spend (satisfies
// agenttool.BudgetProvider).
type fakeBudgetProvider struct {
	perUser float64
	global  float64
}

func (f *fakeBudgetProvider) MonthToDateSpend(_ context.Context) (float64, float64, error) {
	return f.perUser, f.global, nil
}

// spec096Catalog builds a combined catalog with an Ollama (local, free) group
// and an Anthropic (paid) group, plus optional extra statuses (e.g. an
// unreachable provider). Default = ollama/gemma3:4b.
func spec096Catalog(extraStatuses ...catalog.ProviderDiscoveryStatus) (catalog.ModelCatalog, []catalog.ProviderDiscoveryStatus) {
	cat := catalog.ModelCatalog{
		Default: "ollama/gemma3:4b",
		Models: []catalog.ModelDescriptor{
			{ID: "ollama/gemma3:4b", ConnectionID: "local-ollama", Kind: "ollama", ToolCapable: false},
			{ID: "ollama/llama3:8b", ConnectionID: "local-ollama", Kind: "ollama", ToolCapable: true, ContextWindow: 8192},
			{ID: "anthropic/claude-3-5-sonnet", ConnectionID: "anthropic-primary", Kind: "anthropic", ToolCapable: true, Vision: true, ContextWindow: 200000},
			{ID: "anthropic/claude-3-5-haiku", ConnectionID: "anthropic-primary", Kind: "anthropic", ToolCapable: true, ContextWindow: 200000},
		},
	}
	statuses := []catalog.ProviderDiscoveryStatus{
		{ConnectionID: "local-ollama", Kind: "ollama", State: catalog.StateOK, ModelCount: 2},
		{ConnectionID: "anthropic-primary", Kind: "anthropic", State: catalog.StateOK, ModelCount: 2},
	}
	statuses = append(statuses, extraStatuses...)
	return cat, statuses
}

func spec096AllowlistFromCatalog(t *testing.T, cat catalog.ModelCatalog) *modelswitch.Allowlist {
	t.Helper()
	allow, err := cat.Allowlist()
	if err != nil {
		t.Fatalf("cat.Allowlist(): %v", err)
	}
	return allow
}

// TestModelPicker_TelegramCombinedProviderGroupedList_Spec096 — the `/model`
// picker renders ONE provider-grouped, provider-qualified, cost-hinted list
// (Ollama/local group FIRST) over the combined catalog, preserving the 089
// current / system-default tags and the numbered-reply mechanic.
func TestModelPicker_TelegramCombinedProviderGroupedList_Spec096(t *testing.T) {
	ctx := context.Background()
	cat, statuses := spec096Catalog()
	allow := spec096AllowlistFromCatalog(t, cat)

	t.Run("provider_grouped_qualified_cost_hinted_ollama_first", func(t *testing.T) {
		store := &fakeModelPrefStore{}
		text, numbered := renderCombinedPicker(ctx, cat, statuses, nil, allow, store, "user-A")

		// Provider-grouped headers with cost hints.
		for _, want := range []string{
			"— Ollama · local · free —",
			"— Anthropic · paid —",
			"ollama/gemma3:4b (system default)",
			"ollama/llama3:8b",
			"anthropic/claude-3-5-sonnet",
			"anthropic/claude-3-5-haiku",
			"🔧 tools",
			"Reply with 1-4",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("combined picker MUST contain %q, got:\n%s", want, text)
			}
		}

		// Ollama/local group FIRST (its header precedes the Anthropic header).
		if strings.Index(text, "— Ollama") > strings.Index(text, "— Anthropic") {
			t.Fatalf("Ollama/local group MUST render first; got:\n%s", text)
		}

		// The numbered list is the provider-qualified reachable set in display
		// order (Ollama first) — the EXACT armed pending-selection list.
		wantNumbered := []string{"ollama/gemma3:4b", "ollama/llama3:8b", "anthropic/claude-3-5-sonnet", "anthropic/claude-3-5-haiku"}
		if len(numbered) != len(wantNumbered) {
			t.Fatalf("numbered list = %v, want %v", numbered, wantNumbered)
		}
		for i, id := range wantNumbered {
			if numbered[i] != id {
				t.Fatalf("numbered[%d] = %q, want %q (order MUST match display)", i, numbered[i], id)
			}
		}

		// A numbered reply ALWAYS maps to a dispatchable model (validator admits
		// every numbered id — the catalog IS the injected admissible set).
		for _, id := range numbered {
			if _, rej := allow.Resolve(id); rej != nil {
				t.Fatalf("numbered id %q MUST validate against the injected catalog allowlist, got rejection %q", id, rej.ReasonCode)
			}
		}
	})

	t.Run("sticky_hosted_marked_current", func(t *testing.T) {
		store := &fakeModelPrefStore{}
		if err := store.Set(ctx, "user-A", "anthropic/claude-3-5-sonnet"); err != nil {
			t.Fatalf("store.Set: %v", err)
		}
		text, _ := renderCombinedPicker(ctx, cat, statuses, nil, allow, store, "user-A")
		if !strings.Contains(text, "anthropic/claude-3-5-sonnet (current)") {
			t.Fatalf("a sticky hosted selection MUST be tagged (current), got:\n%s", text)
		}
		if !strings.Contains(text, "Your /ask model: anthropic/claude-3-5-sonnet (your default)") {
			t.Fatalf("header MUST show the sticky hosted selection as 'your default', got:\n%s", text)
		}
	})

	t.Run("budget_line_only_when_paid_and_budget_wired", func(t *testing.T) {
		store := &fakeModelPrefStore{}
		budget := &fakeBudgetProvider{perUser: 2.14, global: 7.40}
		text, _ := renderCombinedPicker(ctx, cat, statuses, budget, allow, store, "user-A")
		if !strings.Contains(text, "Budget: $2.14 used this month") {
			t.Fatalf("a catalog with a paid model + a wired budget MUST show the budget line, got:\n%s", text)
		}
		// Pull-not-push: a free/local-only catalog shows NO budget line.
		ollamaOnly := catalog.ModelCatalog{
			Default: "ollama/gemma3:4b",
			Models:  []catalog.ModelDescriptor{{ID: "ollama/gemma3:4b", ConnectionID: "local-ollama", Kind: "ollama"}},
		}
		ollamaAllow := spec096AllowlistFromCatalog(t, ollamaOnly)
		ollamaStatuses := []catalog.ProviderDiscoveryStatus{{ConnectionID: "local-ollama", Kind: "ollama", State: catalog.StateOK, ModelCount: 1}}
		freeText, _ := renderCombinedPicker(ctx, ollamaOnly, ollamaStatuses, budget, ollamaAllow, store, "user-A")
		if strings.Contains(freeText, "Budget:") {
			t.Fatalf("a free/local-only catalog MUST NOT show a budget line (Principle 6), got:\n%s", freeText)
		}
	})
}

// TestModelPicker_UnreachableShownDisabledOnlyReachableNumbered_Spec096 —
// ADVERSARIAL. An unreachable provider is SHOWN-BUT-DISABLED with its typed
// status and only reachable models are numbered (a numbered reply always maps
// to a dispatchable model). Fails if a down provider is silently dropped OR a
// non-reachable entry is numbered.
func TestModelPicker_UnreachableShownDisabledOnlyReachableNumbered_Spec096(t *testing.T) {
	ctx := context.Background()
	// OpenAI is effective-enabled but unreachable (auth_failed): its models are
	// ABSENT from the catalog, but it ALWAYS emits a typed status.
	cat, statuses := spec096Catalog(catalog.ProviderDiscoveryStatus{
		ConnectionID: "openai-primary", Kind: "openai", State: catalog.StateAuthFailed, ModelCount: 0,
	})
	allow := spec096AllowlistFromCatalog(t, cat)
	store := &fakeModelPrefStore{}

	text, numbered := renderCombinedPicker(ctx, cat, statuses, nil, allow, store, "user-A")

	// SHOWN, never silently dropped: the OpenAI provider header + its typed
	// reachability status appear.
	if !strings.Contains(text, "OpenAI") {
		t.Fatalf("an unreachable provider MUST be SHOWN (never silently dropped); OpenAI absent from:\n%s", text)
	}
	if !strings.Contains(text, "auth failed") {
		t.Fatalf("an unreachable provider MUST carry its TYPED status (auth failed); got:\n%s", text)
	}
	if !strings.Contains(text, "not silently dropped") {
		t.Fatalf("the unreachable group MUST state its models are hidden, not dropped; got:\n%s", text)
	}

	// DISABLED for selection: only the reachable (ollama + anthropic) models are
	// numbered — 4 total. A numbered reply never maps to the down provider.
	if len(numbered) != 4 {
		t.Fatalf("only the 4 reachable models MUST be numbered, got %d: %v", len(numbered), numbered)
	}
	for _, id := range numbered {
		if strings.HasPrefix(id, "openai/") {
			t.Fatalf("an unreachable provider's model MUST NOT be numbered; openai id %q is in %v", id, numbered)
		}
		// Every numbered id is dispatchable (validates against the catalog).
		if _, rej := allow.Resolve(id); rej != nil {
			t.Fatalf("numbered id %q MUST be dispatchable, got rejection %q", id, rej.ReasonCode)
		}
	}

	// The footer numbers exactly the reachable set.
	if !strings.Contains(text, "Reply with 1-4") {
		t.Fatalf("footer MUST number exactly the 4 reachable models, got:\n%s", text)
	}

	// Adversarial guard: there is NO numbered line that names an openai model
	// (proving the down provider's entries are shown-but-not-numbered).
	if strings.Contains(text, ". openai/") {
		t.Fatalf("an unreachable provider's models MUST NOT appear as a numbered line; got:\n%s", text)
	}
}
