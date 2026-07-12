// Spec 096 SCOPE-07 (SCN-096-D03 / D05) — HTTP /v1/agent/model multi-provider
// enrichment + parity tests.
//
// They drive the REAL AgentModelHandler against the agenttool singletons (ONE
// modelswitch validator built from the combined catalog + the SAME claim-bound
// modelpref store) with an authenticated request context, proving: the GET
// enrichment is ADDITIVE (allowed_models[] byte-for-byte preserved for 089
// clients — R2); web PUT accepts the same selection Telegram accepts through the
// SAME validator + store; and a sticky hosted selection persists with the 089
// per-request > sticky > default precedence. No request interception — unit. The
// live cross-surface e2e-api legs are the deferred self-hosted dispatch.
package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// fakeCatalogProvider is an injected combined-catalog source (satisfies
// agenttool.CatalogProvider).
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

// spec096Catalog builds the combined catalog (Ollama free + Anthropic paid, all
// reachable) used by the SCOPE-07 api tests. Default = ollama/gemma3:4b.
func spec096Catalog() (catalog.ModelCatalog, []catalog.ProviderDiscoveryStatus) {
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
	return cat, statuses
}

// spec096WireBase installs a provider-qualified allowlist (built FROM the
// combined catalog — the catalog IS the injected admissible set) + the fake
// claim-bound store into the agenttool singletons. It does NOT wire the catalog
// source, so the GET view is the byte-for-byte 089 shape until the caller wires
// it. Returns the store, the allowlist, the catalog/statuses, and a cleanup that
// clears ALL four singletons.
func spec096WireBase(t *testing.T) (*fakeModelPrefStore, *modelswitch.Allowlist, catalog.ModelCatalog, []catalog.ProviderDiscoveryStatus, func()) {
	t.Helper()
	cat, statuses := spec096Catalog()
	allow, err := cat.Allowlist()
	if err != nil {
		t.Fatalf("cat.Allowlist(): %v", err)
	}
	store := &fakeModelPrefStore{}
	agenttool.SetSwitchableModels(allow)
	agenttool.SetModelPref(store)
	cleanup := func() {
		agenttool.SetSwitchableModels(nil)
		agenttool.SetModelPref(nil)
		agenttool.SetModelCatalogProvider(nil)
		agenttool.SetBudgetProvider(nil)
	}
	return store, allow, cat, statuses, cleanup
}

func toStringSlice(t *testing.T, v any) []string {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("expected a JSON array, got %T (%v)", v, v)
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, ok := e.(string)
		if !ok {
			t.Fatalf("expected string array element, got %T (%v)", e, e)
		}
		out = append(out, s)
	}
	return out
}

// TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096 —
// ADVERSARIAL (R2). GET gains additive per-entry capabilities[] + cost_class (+
// optional budget); allowed_models[] is preserved BYTE-FOR-BYTE (ordering +
// provider-qualified strings) for 089 clients. Fails if the additive enrichment
// renames / reorders / removes any allowed_models entry, or leaks the catalog/
// budget fields when no catalog source is wired.
func TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096(t *testing.T) {
	store, _, cat, statuses, cleanup := spec096WireBase(t)
	defer cleanup()
	_ = store

	// --- 089 baseline: NO catalog source wired ---
	base := decodeModelEnv(t, modelReq(http.MethodGet, "", "subject-A"))
	if _, leaked := base["catalog"]; leaked {
		t.Fatalf("with NO catalog source wired the GET view MUST NOT carry a catalog field (089 byte-for-byte); got %v", base)
	}
	if _, leaked := base["budget"]; leaked {
		t.Fatalf("with NO catalog/budget source wired the GET view MUST NOT carry a budget field; got %v", base)
	}
	if _, leaked := base["provider_statuses"]; leaked {
		t.Fatalf("with NO catalog source wired the GET view MUST NOT carry provider_statuses; got %v", base)
	}
	allowedBefore := toStringSlice(t, base["allowed_models"])

	// --- enrich: wire the combined-catalog source + a budget source ---
	agenttool.SetModelCatalogProvider(&fakeCatalogProvider{cat: cat, statuses: statuses})
	agenttool.SetBudgetProvider(&fakeBudgetProvider{perUser: 2.14, global: 7.40})

	enriched := decodeModelEnv(t, modelReq(http.MethodGet, "", "subject-A"))
	allowedAfter := toStringSlice(t, enriched["allowed_models"])

	// allowed_models[] is byte-for-byte identical (same length, order, strings).
	if len(allowedBefore) != len(allowedAfter) {
		t.Fatalf("allowed_models length changed by enrichment: before=%v after=%v", allowedBefore, allowedAfter)
	}
	for i := range allowedBefore {
		if allowedBefore[i] != allowedAfter[i] {
			t.Fatalf("allowed_models[%d] changed by enrichment: before=%q after=%q (R2 byte-for-byte broken)", i, allowedBefore[i], allowedAfter[i])
		}
	}

	// The enrichment is present and additive: catalog[] with capabilities[] +
	// cost_class per entry.
	catEntries, ok := enriched["catalog"].([]any)
	if !ok || len(catEntries) == 0 {
		t.Fatalf("enriched GET MUST carry a non-empty catalog[]; got %v", enriched["catalog"])
	}
	sawPaidToolCapable := false
	for _, e := range catEntries {
		entry, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("catalog entry MUST be an object, got %T", e)
		}
		if _, ok := entry["cost_class"]; !ok {
			t.Fatalf("each catalog entry MUST carry cost_class; got %v", entry)
		}
		if entry["id"] == "anthropic/claude-3-5-sonnet" {
			if entry["cost_class"] != "paid" {
				t.Fatalf("anthropic/claude-3-5-sonnet cost_class = %v, want paid", entry["cost_class"])
			}
			caps := toStringSlice(t, entry["capabilities"])
			hasTool := false
			for _, c := range caps {
				if c == "tool_capable" {
					hasTool = true
				}
			}
			if hasTool {
				sawPaidToolCapable = true
			}
		}
	}
	if !sawPaidToolCapable {
		t.Fatalf("the enriched catalog MUST carry capabilities[] (tool_capable) + cost_class=paid for the hosted model; got %v", catEntries)
	}

	// Budget enrichment present (a paid model is in the catalog).
	budget, ok := enriched["budget"].(map[string]any)
	if !ok {
		t.Fatalf("enriched GET MUST carry a budget object when a paid model is in the catalog; got %v", enriched["budget"])
	}
	if budget["month_to_date_usd"] != 2.14 {
		t.Fatalf("budget.month_to_date_usd = %v, want 2.14", budget["month_to_date_usd"])
	}

	// Provider statuses present (all providers, for the web picker's
	// shown-but-disabled rendering).
	if ps, ok := enriched["provider_statuses"].([]any); !ok || len(ps) == 0 {
		t.Fatalf("enriched GET MUST carry provider_statuses[]; got %v", enriched["provider_statuses"])
	}
}

// TestAgentModel_TelegramWebParity_SameValidatorSameStore_Spec096 — web PUT
// accepts the same selection Telegram accepts and both resolve through the SAME
// modelswitch validator + modelpref store (one-validator/one-store). The
// singleton the HTTP handler reads is the EXACT singleton the Telegram surface
// reads.
func TestAgentModel_TelegramWebParity_SameValidatorSameStore_Spec096(t *testing.T) {
	store, allow, cat, statuses, cleanup := spec096WireBase(t)
	defer cleanup()
	agenttool.SetModelCatalogProvider(&fakeCatalogProvider{cat: cat, statuses: statuses})
	ctx := context.Background()

	// The SAME singletons both surfaces read.
	if agenttool.SwitchableModels() != allow {
		t.Fatalf("the HTTP surface MUST read the SAME validator singleton (one validator)")
	}
	if agenttool.ModelPref() != store {
		t.Fatalf("the HTTP surface MUST read the SAME store singleton (one store)")
	}

	// Web PUT accepts a hosted selection the Telegram /model surface accepts:
	// the validator that gates Telegram (allow.Resolve) admits it, and the PUT
	// succeeds + writes the SAME store.
	if _, rej := allow.Resolve("anthropic/claude-3-5-sonnet"); rej != nil {
		t.Fatalf("the shared validator MUST admit the hosted selection; got rejection %q", rej.ReasonCode)
	}
	if rec := modelReq(http.MethodPut, `{"model":"anthropic/claude-3-5-sonnet"}`, "subject-A"); rec.Code != http.StatusOK {
		t.Fatalf("web PUT of a hosted selection MUST succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	// The HTTP write is visible through the SAME store the Telegram surface
	// reads (a forked store would not reflect it).
	if pref, ok, _ := store.Get(ctx, "subject-A"); !ok || pref.SynthesisModel != "anthropic/claude-3-5-sonnet" {
		t.Fatalf("the HTTP PUT MUST write the shared store the Telegram surface reads; got ok=%v pref=%+v", ok, pref)
	}

	// Bidirectional: a write through the singleton store (the Telegram path) is
	// visible to the HTTP GET — proving ONE store, not a fork.
	if err := store.Set(ctx, "subject-B", "ollama/llama3:8b"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	env := decodeModelEnv(t, modelReq(http.MethodGet, "", "subject-B"))
	if env["effective_model"] != "ollama/llama3:8b" || env["source"] != "sticky" {
		t.Fatalf("HTTP GET MUST reflect a Telegram-path store write (one store); got %v", env)
	}
}

// TestAgentModel_StickyHostedPersistsPrecedence_Spec096 — a sticky hosted
// selection persists across turns and a per-request override beats the sticky
// for one invocation only; the precedence + store reads are byte-for-byte the
// 089 contract with provider-qualified ids only.
func TestAgentModel_StickyHostedPersistsPrecedence_Spec096(t *testing.T) {
	store, allow, _, _, cleanup := spec096WireBase(t)
	defer cleanup()
	ctx := context.Background()

	// Set a sticky HOSTED selection via the web surface; it persists.
	if rec := modelReq(http.MethodPut, `{"model":"anthropic/claude-3-5-sonnet"}`, "subject-A"); rec.Code != http.StatusOK {
		t.Fatalf("PUT sticky hosted MUST succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	getEnv := decodeModelEnv(t, modelReq(http.MethodGet, "", "subject-A"))
	if getEnv["effective_model"] != "anthropic/claude-3-5-sonnet" || getEnv["source"] != "sticky" {
		t.Fatalf("a sticky hosted selection MUST persist across turns; got %v", getEnv)
	}

	// per-request > sticky: a per-request synthesis override beats the sticky
	// for ONE invocation; the validator is provider-agnostic (hosted + ollama).
	sticky, _, _ := store.Get(ctx, "subject-A")
	eff, rej := allow.ResolveEffective("ollama/llama3:8b", "", sticky.SynthesisModel)
	if rej != nil {
		t.Fatalf("ResolveEffective MUST admit the provider-qualified per-request override; got %q", rej.ReasonCode)
	}
	if eff.SynthesisModel != "ollama/llama3:8b" || eff.SynthesisSource != modelswitch.SourcePerRequest {
		t.Fatalf("per-request MUST beat sticky for one invocation; got model=%q source=%q", eff.SynthesisModel, eff.SynthesisSource)
	}

	// Without the per-request override, the sticky wins (precedence preserved).
	effSticky, rej := allow.ResolveEffective("", "", sticky.SynthesisModel)
	if rej != nil {
		t.Fatalf("ResolveEffective(sticky) MUST succeed; got %q", rej.ReasonCode)
	}
	if effSticky.SynthesisModel != "anthropic/claude-3-5-sonnet" || effSticky.SynthesisSource != modelswitch.SourceSticky {
		t.Fatalf("sticky MUST win absent a per-request override; got model=%q source=%q", effSticky.SynthesisModel, effSticky.SynthesisSource)
	}

	// The per-request override did NOT mutate the persisted sticky (one
	// invocation only) — the store read is byte-for-byte the 089 contract.
	after, ok, _ := store.Get(ctx, "subject-A")
	if !ok || after.SynthesisModel != "anthropic/claude-3-5-sonnet" {
		t.Fatalf("a per-request override MUST NOT mutate the persisted sticky; got ok=%v pref=%+v", ok, after)
	}
}
