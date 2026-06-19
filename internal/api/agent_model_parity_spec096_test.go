// Spec 096 SCOPE-07 (SCN-096-G06) — the 088/089 parity guard tests.
//
// These ADVERSARIAL tests pin the 088/089 invariants provider-agnostically: ONE
// validator + ONE store shared by Telegram + HTTP, the gather-vs-synthesis fork,
// per-request > sticky > default precedence, and the non-tool-capable-gather
// fail-loud rejection (R3). They drive the SHARED agenttool singletons + the
// REAL modelswitch validator (built from the combined catalog) — no second
// validator/store is introduced, and a non-tool-capable gather selection is
// NEVER accepted. Unit (pure mechanism); the live multi-provider fork/precedence
// e2e-api leg is the deferred home-lab dispatch.
package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// TestParity_OneValidatorOneStore_ForkPrecedence_ProviderAgnostic_Spec096 —
// ADVERSARIAL. The gather-vs-synthesis fork + per-request > sticky > default
// precedence + one-validator/one-store behave EXACTLY as 088/089 define, only
// the ids are now provider-qualified. Fails if a second validator/store is
// introduced (the bidirectional shared-store round-trip would break).
func TestParity_OneValidatorOneStore_ForkPrecedence_ProviderAgnostic_Spec096(t *testing.T) {
	store, allow, cat, statuses, cleanup := spec096WireBase(t)
	defer cleanup()
	agenttool.SetModelCatalogProvider(&fakeCatalogProvider{cat: cat, statuses: statuses})
	ctx := context.Background()

	// ONE validator + ONE store — the exact singletons BOTH surfaces read.
	if agenttool.SwitchableModels() != allow {
		t.Fatalf("a SECOND validator was introduced — the HTTP surface MUST read the one shared modelswitch singleton")
	}
	if agenttool.ModelPref() != store {
		t.Fatalf("a SECOND store was introduced — the HTTP surface MUST read the one shared modelpref singleton")
	}

	// One store, proven bidirectionally (a fork would break one direction):
	//  (a) HTTP PUT → singleton store read sees it.
	if rec := modelReq(http.MethodPut, `{"model":"anthropic/claude-3-5-sonnet"}`, "subject-A"); rec.Code != http.StatusOK {
		t.Fatalf("PUT hosted MUST succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	if pref, ok, _ := store.Get(ctx, "subject-A"); !ok || pref.SynthesisModel != "anthropic/claude-3-5-sonnet" {
		t.Fatalf("HTTP PUT MUST be visible through the one shared store; got ok=%v pref=%+v", ok, pref)
	}
	//  (b) singleton store write → HTTP GET sees it.
	if err := store.Set(ctx, "subject-C", "ollama/gemma3:4b"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	if env := decodeModelEnv(t, modelReq(http.MethodGet, "", "subject-C")); env["effective_model"] != "ollama/gemma3:4b" {
		t.Fatalf("a store write MUST be visible through the one shared HTTP surface; got %v", env)
	}

	// The gather-vs-synthesis FORK: each turn re-points independently, the ids
	// are provider-qualified, and per-request beats sticky for synthesis.
	eff, rej := allow.ResolveEffective("anthropic/claude-3-5-haiku", "ollama/llama3:8b", "anthropic/claude-3-5-sonnet")
	if rej != nil {
		t.Fatalf("the provider-agnostic fork MUST resolve; got rejection %q", rej.ReasonCode)
	}
	if eff.SynthesisModel != "anthropic/claude-3-5-haiku" || eff.SynthesisSource != modelswitch.SourcePerRequest {
		t.Fatalf("per-request synthesis MUST beat sticky; got model=%q source=%q", eff.SynthesisModel, eff.SynthesisSource)
	}
	if eff.GatherModel != "ollama/llama3:8b" || eff.GatherSource != modelswitch.SourcePerRequest {
		t.Fatalf("the gather turn MUST re-point independently to a provider-qualified id; got model=%q source=%q", eff.GatherModel, eff.GatherSource)
	}

	// Precedence with no per-request override: the sticky wins.
	effSticky, rej := allow.ResolveEffective("", "", "anthropic/claude-3-5-sonnet")
	if rej != nil || effSticky.SynthesisModel != "anthropic/claude-3-5-sonnet" || effSticky.SynthesisSource != modelswitch.SourceSticky {
		t.Fatalf("sticky MUST win absent a per-request override; got %+v rej=%v", effSticky, rej)
	}

	// Still ONE validator after all the requests.
	if agenttool.SwitchableModels() != allow {
		t.Fatalf("the validator singleton changed mid-flight — a second validator MUST NOT be introduced")
	}
}

// spec096NonToolGatherCatalog builds a catalog where the hosted model
// anthropic/claude-3-5-haiku is switchable for SYNTHESIS but NOT tool_capable
// (so it is invalid for the GATHER turn), alongside a tool-capable sonnet.
func spec096NonToolGatherCatalog() catalog.ModelCatalog {
	return catalog.ModelCatalog{
		Default: "anthropic/claude-3-5-sonnet",
		Models: []catalog.ModelDescriptor{
			{ID: "anthropic/claude-3-5-sonnet", ConnectionID: "anthropic-primary", Kind: "anthropic", ToolCapable: true},
			{ID: "anthropic/claude-3-5-haiku", ConnectionID: "anthropic-primary", Kind: "anthropic", ToolCapable: false},
		},
	}
}

// TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096 —
// ADVERSARIAL (R3). A non-tool-capable model is shown-but-disabled for gather:
// it IS in the synthesis switchable set (shown) but NOT in the tool-capable
// gather set (disabled), and a direct gather selection of it is rejected
// fail-loud with the SAME modelswitch.Rejection. Fails if a non-tool-capable
// gather selection is ever accepted.
func TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096(t *testing.T) {
	cat := spec096NonToolGatherCatalog()
	allow, err := cat.Allowlist()
	if err != nil {
		t.Fatalf("cat.Allowlist(): %v", err)
	}
	store := &fakeModelPrefStore{}
	agenttool.SetSwitchableModels(allow)
	agenttool.SetModelPref(store)
	agenttool.SetModelCatalogProvider(&fakeCatalogProvider{cat: cat, statuses: []catalog.ProviderDiscoveryStatus{
		{ConnectionID: "anthropic-primary", Kind: "anthropic", State: catalog.StateOK, ModelCount: 2},
	}})
	defer func() {
		agenttool.SetSwitchableModels(nil)
		agenttool.SetModelPref(nil)
		agenttool.SetModelCatalogProvider(nil)
	}()

	const nonToolGather = "anthropic/claude-3-5-haiku"
	const toolGather = "anthropic/claude-3-5-sonnet"

	// SHOWN: the non-tool-capable model is a valid SYNTHESIS selection (it is in
	// the switchable set the picker renders).
	shown := false
	for _, m := range allow.AllowedModels() {
		if m == nonToolGather {
			shown = true
		}
	}
	if !shown {
		t.Fatalf("a non-tool-capable model MUST be SHOWN in the synthesis switchable set (transparency); allowed=%v", allow.AllowedModels())
	}

	// DISABLED for gather: it is NOT in the tool-capable gather set.
	for _, m := range allow.ToolCapableGatherModels() {
		if m == nonToolGather {
			t.Fatalf("a non-tool-capable model MUST NOT be in the tool-capable gather set; got %v", allow.ToolCapableGatherModels())
		}
	}

	// REJECTED FAIL-LOUD: a direct gather selection of it is the SAME
	// modelswitch.Rejection (ReasonNotToolCapable, gather turn) — never accepted.
	if _, rej := allow.ResolveGather(nonToolGather); rej == nil {
		t.Fatalf("a non-tool-capable gather selection MUST be rejected fail-loud, got acceptance")
	} else if rej.ReasonCode != modelswitch.ReasonNotToolCapable || rej.RejectedTurn != modelswitch.TurnGather {
		t.Fatalf("the gather rejection MUST be the typed (ReasonNotToolCapable, gather); got code=%q turn=%q", rej.ReasonCode, rej.RejectedTurn)
	}

	// Also rejected through the full precedence resolver (per-request gather).
	if _, rej := allow.ResolveEffective("", nonToolGather, ""); rej == nil {
		t.Fatalf("ResolveEffective MUST reject a non-tool-capable per-request gather; got acceptance")
	}

	// CONTROL (non-tautology): a tool-capable gather selection IS accepted, so
	// the rejection above is a real gate, not a blanket refusal.
	if g, rej := allow.ResolveGather(toolGather); rej != nil || g != toolGather {
		t.Fatalf("a tool-capable gather selection MUST be accepted (control); got g=%q rej=%v", g, rej)
	}
}
