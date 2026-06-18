// Spec 096 SCOPE-04 — identifier canonicalization + off-catalog rejection
// tests (SCN-096-D04). UNIT: the provider-qualified catalog is built directly
// and INJECTED as the existing modelswitch validator's admissible set; no live
// agent, no Ollama, no dispatch. The live off-catalog `/ask` selection leg is
// the deferred e2e-api (C7).
package catalog

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// spyStore is a modelpref.Store that records every Set so the off-catalog test
// can prove NO store write happens on a rejection (and a control write happens
// on a valid selection). It is the EXISTING store interface — no second store.
type spyStore struct {
	sets []struct{ user, model string }
}

func (s *spyStore) Get(_ context.Context, _ string) (modelpref.Preference, bool, error) {
	return modelpref.Preference{}, false, nil
}
func (s *spyStore) Set(_ context.Context, userID, synthesisModel string) error {
	s.sets = append(s.sets, struct{ user, model string }{userID, synthesisModel})
	return nil
}
func (s *spyStore) Clear(_ context.Context, _ string) error { return nil }

// TestCanonicalize_SplitOnFirstSlash_RoundTrip_Spec096 proves the grammar
// (design §8): split on the FIRST `/` only, so a backend id that itself
// contains `/` or `:` round-trips. An already-qualified id passes through
// Canonicalize unchanged.
func TestCanonicalize_SplitOnFirstSlash_RoundTrip_Spec096(t *testing.T) {
	cases := []struct {
		id          string
		wantKind    string
		wantBackend string
	}{
		{"anthropic/claude-3-5-sonnet", "anthropic", "claude-3-5-sonnet"},
		{"ollama/gemma3:4b", "ollama", "gemma3:4b"},
		{"ollama/library/llama3:8b", "ollama", "library/llama3:8b"},                                                   // backend contains "/"
		{"bedrock/anthropic.claude-3-5-sonnet-20241022-v2:0", "bedrock", "anthropic.claude-3-5-sonnet-20241022-v2:0"}, // backend contains ":"
		{"azure-foundry/my-deployment", "azure-foundry", "my-deployment"},
	}
	installed := map[string]struct{}{} // no bare ids here — these are all qualified
	for _, tc := range cases {
		kind, backend, ok := SplitQualified(tc.id)
		if !ok {
			t.Fatalf("SplitQualified(%q): ok=false, want a clean split", tc.id)
		}
		if kind != tc.wantKind || backend != tc.wantBackend {
			t.Fatalf("SplitQualified(%q): got (%q,%q), want (%q,%q) — must split on FIRST '/'", tc.id, kind, backend, tc.wantKind, tc.wantBackend)
		}
		// Round-trip: re-joining on the first '/' reproduces the input exactly.
		if rejoined := kind + "/" + backend; rejoined != tc.id {
			t.Fatalf("round-trip mismatch: %q != %q", rejoined, tc.id)
		}
		// An already-qualified id is passed through Canonicalize unchanged.
		if got := Canonicalize(tc.id, installed); got != tc.id {
			t.Fatalf("Canonicalize(%q) qualified passthrough: got %q, want unchanged", tc.id, got)
		}
	}

	// A bare id (no '/') is NOT a clean split.
	if _, _, ok := SplitQualified("gemma3:4b"); ok {
		t.Fatalf("SplitQualified(%q): ok=true, want false for a bare (unqualified) id", "gemma3:4b")
	}
}

// TestCanonicalize_BareOllamaIdNormalized_Spec096 proves a 089-era bare Ollama
// id is normalized to `ollama/<id>` IFF installed, so today's bare selections
// keep working; a bare id that is NOT installed is left unqualified (the
// validator then rejects it).
func TestCanonicalize_BareOllamaIdNormalized_Spec096(t *testing.T) {
	installed := map[string]struct{}{"gemma3:4b": {}, "llama3:8b": {}}

	if got := Canonicalize("gemma3:4b", installed); got != "ollama/gemma3:4b" {
		t.Fatalf("bare installed id: Canonicalize(%q) = %q, want ollama/gemma3:4b", "gemma3:4b", got)
	}
	// Not installed ⇒ left bare/unqualified (no false ollama/ qualification).
	if got := Canonicalize("phi4:latest", installed); got != "phi4:latest" {
		t.Fatalf("bare uninstalled id: Canonicalize(%q) = %q, want unchanged (validator rejects)", "phi4:latest", got)
	}
	// An already-qualified id is never re-qualified, even a hosted one.
	if got := Canonicalize("anthropic/claude-3-5-sonnet", installed); got != "anthropic/claude-3-5-sonnet" {
		t.Fatalf("qualified id must pass through: got %q", got)
	}
	// Whitespace/empty ⇒ baseline sentinel.
	if got := Canonicalize("  ", installed); got != "" {
		t.Fatalf("blank id: Canonicalize = %q, want \"\" (baseline no-override)", got)
	}
}

// catalogFixture is the provider-qualified catalog used by the rejection test:
// one installed Ollama model + one curated hosted model, default = the Ollama
// model.
func catalogFixture() ModelCatalog {
	return ModelCatalog{
		Default: "ollama/gemma3:4b",
		Models: []ModelDescriptor{
			{ID: "ollama/gemma3:4b", ConnectionID: "local-ollama", Kind: "ollama", ToolCapable: true},
			{ID: "anthropic/claude-3-5-sonnet", ConnectionID: "anthropic-main", Kind: "anthropic", ToolCapable: true, Vision: true, ContextWindow: 200000},
		},
	}
}

// TestValidate_OffCatalogRefused_TypedRejection_Spec096 is the ADVERSARIAL
// proof: an off-catalog id is refused with the SAME modelswitch.Rejection shape
// (the catalog IS the injected admissible set), with NO modelpref store write
// and NO dispatch. It FAILS if an off-catalog id is ever accepted or written.
func TestValidate_OffCatalogRefused_TypedRejection_Spec096(t *testing.T) {
	cat := catalogFixture()
	resolver, err := NewCatalogResolverFromCatalog(cat)
	if err != nil {
		t.Fatalf("NewCatalogResolverFromCatalog: %v", err)
	}

	// --- Controls: in-catalog ids validate (so the rejection is not a blanket reject). ---
	if canon, rej := resolver.Validate("anthropic/claude-3-5-sonnet"); rej != nil || canon != "anthropic/claude-3-5-sonnet" {
		t.Fatalf("control qualified in-catalog: got (%q, %v), want (anthropic/claude-3-5-sonnet, nil)", canon, rej)
	}
	if canon, rej := resolver.Validate("gemma3:4b"); rej != nil || canon != "ollama/gemma3:4b" {
		t.Fatalf("control bare-Ollama in-catalog: got (%q, %v), want (ollama/gemma3:4b, nil) — 089 bare selection must keep working", canon, rej)
	}

	// --- Adversarial: an off-catalog id is refused with the modelswitch.Rejection shape. ---
	const offCatalog = "anthropic/claude-OPUS-does-not-exist"
	canon, rej := resolver.Validate(offCatalog)
	if rej == nil {
		t.Fatalf("off-catalog id %q was ACCEPTED (canon=%q) — must be refused", offCatalog, canon)
	}
	if canon != "" {
		t.Fatalf("off-catalog id: canon=%q, want empty", canon)
	}
	if rej.RejectedModel != offCatalog {
		t.Fatalf("rejection.RejectedModel = %q, want %q", rej.RejectedModel, offCatalog)
	}
	if rej.ReasonCode != modelswitch.ReasonNotAllowlisted {
		t.Fatalf("rejection.ReasonCode = %q, want %q (same modelswitch shape)", rej.ReasonCode, modelswitch.ReasonNotAllowlisted)
	}
	// The injected admissible set IS the provider-qualified catalog.
	wantAllowed := map[string]bool{"ollama/gemma3:4b": true, "anthropic/claude-3-5-sonnet": true}
	if len(rej.AllowedModels) != len(wantAllowed) {
		t.Fatalf("rejection.AllowedModels = %v, want the provider-qualified catalog %v", rej.AllowedModels, wantAllowed)
	}
	for _, m := range rej.AllowedModels {
		if !wantAllowed[m] {
			t.Fatalf("rejection.AllowedModels contains %q, not a catalog id", m)
		}
	}
	if rej.Message == "" {
		t.Fatalf("rejection.Message is empty — must carry the canonical modelswitch UX sentence")
	}

	// --- NO store write on rejection; a store write on a valid selection. ---
	store := &spyStore{}
	gotCanon, gotRej, serr := resolver.Select(context.Background(), store, "user-1", offCatalog)
	if serr != nil {
		t.Fatalf("Select(off-catalog): unexpected error %v", serr)
	}
	if gotRej == nil || gotCanon != "" {
		t.Fatalf("Select(off-catalog): got (%q, %v), want (\"\", rejection)", gotCanon, gotRej)
	}
	if len(store.sets) != 0 {
		t.Fatalf("off-catalog selection wrote to the store %d time(s) — must be 0 (NO store write, NO dispatch)", len(store.sets))
	}
	// Control: a valid selection DOES persist (so the no-write above is not vacuous).
	if _, rej, serr := resolver.Select(context.Background(), store, "user-1", "anthropic/claude-3-5-sonnet"); rej != nil || serr != nil {
		t.Fatalf("Select(valid): got rej=%v err=%v, want a clean persist", rej, serr)
	}
	if len(store.sets) != 1 || store.sets[0].model != "anthropic/claude-3-5-sonnet" {
		t.Fatalf("valid selection store writes = %v, want exactly one anthropic/claude-3-5-sonnet", store.sets)
	}
}
