package quality

import (
	"testing"
)

// TestGroupNearDuplicates_CapsSameChain proves BS-027 / SCN-039-043: when
// multiple same-chain candidates appear, only one keeps its top-K slot and
// the rest are recorded as variants under the parent.
func TestGroupNearDuplicates_CapsSameChain(t *testing.T) {
	input := []CandidateForDiversity{
		{LocalID: "cand_starbucks_mission", CanonicalKey: "place:starbucks-mission", Title: "Starbucks Mission", ChainKey: "starbucks"},
		{LocalID: "cand_blue_bottle", CanonicalKey: "place:blue-bottle", Title: "Blue Bottle", ChainKey: "blue"},
		{LocalID: "cand_starbucks_castro", CanonicalKey: "place:starbucks-castro", Title: "Starbucks Castro", ChainKey: "starbucks"},
		{LocalID: "cand_fogline", CanonicalKey: "place:fogline-coffee", Title: "Fogline Coffee", ChainKey: "fogline"},
		{LocalID: "cand_starbucks_soma", CanonicalKey: "place:starbucks-soma", Title: "Starbucks SoMa", ChainKey: "starbucks"},
	}
	result := GroupNearDuplicates(input)

	wantKept := []string{"cand_starbucks_mission", "cand_blue_bottle", "cand_fogline"}
	if got := result.KeptOrder; !equalStrings(got, wantKept) {
		t.Fatalf("KeptOrder = %v, want %v", got, wantKept)
	}
	variants, ok := result.VariantsByParent["cand_starbucks_mission"]
	if !ok {
		t.Fatalf("expected starbucks parent to carry variants, got %#v", result.VariantsByParent)
	}
	if len(variants) != 2 {
		t.Fatalf("variant count = %d, want 2", len(variants))
	}
	gotVariantIDs := []string{variants[0].CandidateLocalID, variants[1].CandidateLocalID}
	wantVariantIDs := []string{"cand_starbucks_castro", "cand_starbucks_soma"}
	if !equalStrings(gotVariantIDs, wantVariantIDs) {
		t.Fatalf("variant order = %v, want %v", gotVariantIDs, wantVariantIDs)
	}
	for _, variantID := range wantVariantIDs {
		if parent := result.ParentByVariant[variantID]; parent != "cand_starbucks_mission" {
			t.Fatalf("ParentByVariant[%s] = %q, want %q", variantID, parent, "cand_starbucks_mission")
		}
	}
}

// TestGroupNearDuplicates_EmptyChainKeyNeverGrouped guards that a candidate
// without an inferable chain identifier cannot be silently grouped with
// other empty-chain candidates.
func TestGroupNearDuplicates_EmptyChainKeyNeverGrouped(t *testing.T) {
	input := []CandidateForDiversity{
		{LocalID: "cand_a", CanonicalKey: "place:a", Title: "A"},
		{LocalID: "cand_b", CanonicalKey: "place:b", Title: "B"},
		{LocalID: "cand_c", CanonicalKey: "place:c", Title: "C"},
	}
	result := GroupNearDuplicates(input)
	if got := result.KeptOrder; len(got) != 3 {
		t.Fatalf("KeptOrder = %v, want 3 entries", got)
	}
	if len(result.VariantsByParent) != 0 {
		t.Fatalf("VariantsByParent should be empty for unique candidates, got %#v", result.VariantsByParent)
	}
}

// TestChainKeyOf_PrefersExplicitChainID guards that an explicit chain_id beats
// chain_name and beats the title prefix fallback.
func TestChainKeyOf_PrefersExplicitChainID(t *testing.T) {
	if got := ChainKeyOf(map[string]any{"chain_id": "starbucks", "chain_name": "Starbucks"}, "Starbucks Mission"); got != "starbucks" {
		t.Fatalf("ChainKeyOf(chain_id) = %q, want starbucks", got)
	}
	if got := ChainKeyOf(map[string]any{"chain_name": "Blue Bottle"}, "Blue Bottle Mission"); got != "blue-bottle" {
		t.Fatalf("ChainKeyOf(chain_name) = %q, want blue-bottle", got)
	}
	if got := ChainKeyOf(nil, "Fogline Coffee"); got != "fogline" {
		t.Fatalf("ChainKeyOf(title fallback) = %q, want fogline", got)
	}
	if got := ChainKeyOf(nil, ""); got != "" {
		t.Fatalf("ChainKeyOf(empty) = %q, want empty string", got)
	}
}

// TestVariantsDecision_StableShape locks the persisted decision shape so
// renderers and audit tooling can rely on the keys.
func TestVariantsDecision_StableShape(t *testing.T) {
	decision := VariantsDecision([]Variant{
		{CandidateLocalID: "cand_x", CanonicalKey: "place:x", Title: "X"},
		{CandidateLocalID: "cand_y", CanonicalKey: "place:y", Title: "Y"},
	})
	if decision["kind"] != "diversity" || decision["outcome"] != "variants_grouped" || decision["reason"] != "same-chain" {
		t.Fatalf("decision header mismatch: %#v", decision)
	}
	if got := decision["variant_count"]; got != 2 {
		t.Fatalf("variant_count = %v, want 2", got)
	}
	keys, ok := decision["variant_keys"].([]string)
	if !ok || len(keys) != 2 || keys[0] != "place:x" || keys[1] != "place:y" {
		t.Fatalf("variant_keys = %#v, want [place:x place:y]", decision["variant_keys"])
	}
}

// TestEvaluateTotalCost_DisclosesUnknowns ensures unknown shipping/return are
// surfaced and a "cheapest" claim without supporting facts is blocked.
func TestEvaluateTotalCost_DisclosesUnknowns(t *testing.T) {
	facts := TotalCostFactsFromMap(map[string]any{
		"headline_price":   12.99,
		"cheapest_claimed": true,
	})
	decisions := EvaluateTotalCost(facts)
	if len(decisions) == 0 {
		t.Fatalf("expected disclosures for unknown shipping/return + cheapest block, got 0")
	}
	mustHave := map[string]bool{
		"shipping-cost-unknown":    false,
		"return-policy-unknown":    false,
		"taxes-not-included":       false,
		"total-cost-not-supported": false,
	}
	for _, decision := range decisions {
		reason, _ := decision["reason"].(string)
		if _, ok := mustHave[reason]; ok {
			mustHave[reason] = true
		}
	}
	for reason, found := range mustHave {
		if !found {
			t.Fatalf("missing disclosure reason %q in decisions=%#v", reason, decisions)
		}
	}
}

// TestEvaluateTotalCost_AllKnownAndCheapest ensures the guard does NOT block
// "cheapest" when the total-cost facts back the claim.
func TestEvaluateTotalCost_AllKnownAndCheapest(t *testing.T) {
	facts := TotalCostFactsFromMap(map[string]any{
		"headline_price":   10.00,
		"shipping_cost":    0.00,
		"return_policy":    "free 30 day",
		"taxes_included":   true,
		"total_cost":       10.00,
		"cheapest_claimed": true,
	})
	decisions := EvaluateTotalCost(facts)
	for _, decision := range decisions {
		if decision["outcome"] == "block_label_cheapest" {
			t.Fatalf("cheapest claim must not be blocked when facts support it: %#v", decision)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
