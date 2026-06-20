package cardrewards

// Spec 083 Card Rewards Companion (Scope 07) — T-07-01 / T-07-02.
// Unit tests for the PURE optimizer (optimize.go). No database, no mocks —
// every scenario SCN-083-G01..G05 is decided by a function of its inputs.
//
// G03 (expired rotating ignored) and G04 (shared-limit pool not double-counted)
// are ADVERSARIAL: each feeds input that a naive optimizer (use any rotating
// record regardless of lifecycle / sum the offers' limits) would mis-handle,
// and asserts the optimizer does the right thing. They FAIL if that protection
// is removed — they are not tautological.
//
// Reuses dateUTC / ptrInt from reconcile_test.go (the unit-build test helpers).

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// optCard builds a CardInputs for the optimizer from raw benefit JSON. Empty
// JSON strings leave the corresponding jsonb column unset.
func optCard(id, name, base, rotating, selectable string) CardInputs {
	c := &CatalogCard{ID: "cat-" + id, Name: name}
	if base != "" {
		c.BaseBenefits = json.RawMessage(base)
	}
	if rotating != "" {
		c.RotatingBenefits = json.RawMessage(rotating)
	}
	if selectable != "" {
		c.SelectableBenefits = json.RawMessage(selectable)
	}
	return CardInputs{
		UserCard: UserCard{ID: id, CardCatalogID: c.ID, CatalogName: name, Active: true},
		Catalog:  c,
	}
}

func optNow() time.Time { return dateUTC(2026, 6, 15) }

// SCN-083-G01 — base-rate optimization picks the highest fixed rate.
func TestOptimize_BaseRateHighestWins_G01(t *testing.T) {
	a := optCard("card-a", "Card A", `[{"category":"Groceries","rate":3,"rate_type":"percent"}]`, "", "")
	b := optCard("card-b", "Card B", `[{"category":"Groceries","rate":1,"rate_type":"percent"}]`, "", "")

	res := Optimize("Groceries", []CardInputs{b, a}, nil, optNow())

	if res.RecommendedUserCardID == nil || *res.RecommendedUserCardID != "card-a" {
		t.Fatalf("recommended card = %v, want card-a (higher 3%% base)", res.RecommendedUserCardID)
	}
	if res.Rate != 3 {
		t.Fatalf("rate = %v, want 3", res.Rate)
	}
	if res.Source != BenefitSourceBase {
		t.Fatalf("source = %q, want %q", res.Source, BenefitSourceBase)
	}
	if res.Reason == "" {
		t.Fatal("reason is empty — Principle 8 requires an explainable reason")
	}
}

// SCN-083-G02 — an active rotating category beats a fixed base rate.
func TestOptimize_ActiveRotatingBeatsBase_G02(t *testing.T) {
	rot := optCard("card-r", "Rotating Card",
		`[{"category":"Everything","rate":1,"rate_type":"percent"}]`,
		`{"type":"quarterly","activation_required":true,"rate":5,"rate_type":"percent","limit":1500}`, "")
	rot.Rotating = []RotatingCategory{{
		CardCatalogID:  rot.Catalog.ID,
		PeriodLabel:    "Q2_2026",
		Categories:     []string{"Restaurants"},
		LifecycleState: LifecycleActive,
	}}
	base := optCard("card-b", "Base Card", `[{"category":"Restaurants","rate":3,"rate_type":"percent"}]`, "", "")

	res := Optimize("Restaurants", []CardInputs{base, rot}, nil, optNow())

	if res.RecommendedUserCardID == nil || *res.RecommendedUserCardID != "card-r" {
		t.Fatalf("recommended card = %v, want card-r (active rotating 5%%)", res.RecommendedUserCardID)
	}
	if res.Rate != 5 {
		t.Fatalf("rate = %v, want 5", res.Rate)
	}
	if res.Source != BenefitSourceRotating {
		t.Fatalf("source = %q, want %q", res.Source, BenefitSourceRotating)
	}
	if !strings.Contains(res.Reason, "rotating") {
		t.Fatalf("reason %q does not cite the rotating benefit", res.Reason)
	}
}

// SCN-083-G03 — an EXPIRED rotating category is ignored (ADVERSARIAL). If the
// lifecycle filter were removed, the expired 5%% rotating would win; the test
// asserts the 3%% base card wins instead, so it fails on regression.
func TestOptimize_ExpiredRotatingIgnored_G03(t *testing.T) {
	rot := optCard("card-r", "Rotating Card",
		`[{"category":"Everything","rate":1,"rate_type":"percent"}]`,
		`{"type":"quarterly","activation_required":true,"rate":5,"rate_type":"percent"}`, "")
	rot.Rotating = []RotatingCategory{{
		CardCatalogID:  rot.Catalog.ID,
		PeriodLabel:    "Q1_2026",
		Categories:     []string{"Restaurants"},
		LifecycleState: LifecycleExpired, // expired → must not be used
	}}
	base := optCard("card-b", "Base Card", `[{"category":"Restaurants","rate":3,"rate_type":"percent"}]`, "", "")

	res := Optimize("Restaurants", []CardInputs{base, rot}, nil, optNow())

	if res.Source == BenefitSourceRotating {
		t.Fatalf("source = %q — an EXPIRED rotating benefit was used (G03 regression)", res.Source)
	}
	if res.Rate != 3 {
		t.Fatalf("rate = %v, want 3 (the base card; expired 5%% rotating must be ignored)", res.Rate)
	}
	if res.RecommendedUserCardID == nil || *res.RecommendedUserCardID != "card-b" {
		t.Fatalf("recommended card = %v, want card-b", res.RecommendedUserCardID)
	}
}

// SCN-083-G04 — two offers in one shared_limit_group are treated as ONE pool;
// the combined limit is NOT double-counted (ADVERSARIAL vs a summing impl).
func TestOptimize_SharedLimitPoolNotDoubleCounted_G04(t *testing.T) {
	card := optCard("card-c", "Pooled Card", `[{"category":"Everything","rate":1,"rate_type":"percent"}]`, "", "")
	group := "dining-pool"
	card.Offers = []Offer{
		{ID: "o1", Title: "5% Dining", Category: "Dining", Rate: 5, RateType: RateTypePercent,
			LimitCents: ptrInt(100000), SharedLimitGroup: &group},
		{ID: "o2", Title: "5% Restaurants", Category: "Restaurants", Rate: 5, RateType: RateTypePercent,
			LimitCents: ptrInt(100000), SharedLimitGroup: &group},
	}

	// Direct unit proof of the pool helper.
	if got := poolLimitCents(card.Offers, group); got == nil || *got != 100000 {
		t.Fatalf("poolLimitCents = %v, want 100000 (one combined pool, not 200000)", got)
	}

	res := Optimize("Dining", []CardInputs{card}, nil, optNow())

	if res.SharedLimitGroup == nil || *res.SharedLimitGroup != group {
		t.Fatalf("shared_limit_group = %v, want %q", res.SharedLimitGroup, group)
	}
	if res.EffectiveLimitCents == nil || *res.EffectiveLimitCents != 100000 {
		t.Fatalf("effective limit = %v, want 100000 (combined pool counted once, not 200000)", res.EffectiveLimitCents)
	}
	if res.Source != BenefitSourceOffer || res.Rate != 5 {
		t.Fatalf("got source=%q rate=%v, want offer/5", res.Source, res.Rate)
	}
}

// SCN-083-G05 — category equivalents normalize the query before matching: a
// query of "eating out" matches a "Dining" benefit via category_aliases. If
// normalization were removed, no card would match and the rate would be 0.
func TestOptimize_EquivalentsNormalizeBeforeMatching_G05(t *testing.T) {
	card := optCard("card-d", "Dining Card", `[{"category":"Dining","rate":4,"rate_type":"percent"}]`, "", "")
	aliases := []CategoryAlias{{
		CanonicalCategory: "Dining",
		Equivalents:       []string{"eating out", "restaurants"},
	}}

	res := Optimize("eating out", []CardInputs{card}, aliases, optNow())

	if res.Category != "Dining" {
		t.Fatalf("category = %q, want canonical \"Dining\"", res.Category)
	}
	if res.Rate != 4 {
		t.Fatalf("rate = %v, want 4 (matched Dining via the \"eating out\" equivalent)", res.Rate)
	}
	if res.RecommendedUserCardID == nil || *res.RecommendedUserCardID != "card-d" {
		t.Fatalf("recommended card = %v, want card-d", res.RecommendedUserCardID)
	}
}

// No owned card has a benefit for the category → explicit none result.
func TestOptimize_NoBenefit_ReturnsNone(t *testing.T) {
	card := optCard("card-e", "Store Card", `[{"category":"Home Depot","rate":0,"rate_type":"percent"}]`, "", "")

	res := Optimize("Dining", []CardInputs{card}, nil, optNow())

	if res.RecommendedUserCardID != nil {
		t.Fatalf("recommended card = %v, want nil (no benefit)", res.RecommendedUserCardID)
	}
	if res.Source != BenefitSourceNone || res.Rate != 0 {
		t.Fatalf("got source=%q rate=%v, want none/0", res.Source, res.Rate)
	}
	if res.Reason == "" {
		t.Fatal("reason is empty — even a no-benefit result must explain itself")
	}
}

// inactive cards are excluded from optimization.
func TestOptimize_InactiveCardExcluded(t *testing.T) {
	card := optCard("card-f", "Inactive Card", `[{"category":"Groceries","rate":6,"rate_type":"percent"}]`, "", "")
	card.UserCard.Active = false

	res := Optimize("Groceries", []CardInputs{card}, nil, optNow())
	if res.Source != BenefitSourceNone {
		t.Fatalf("source = %q, want none (the only card is inactive)", res.Source)
	}
}

// Deterministic tie-breaking when two owned cards offer the SAME effective rate
// for a category. design §6: "Ties broken by no spend-limit > higher limit >
// issuer preference"; design §13 names "ties" as a required optimizer unit-test
// target (Scope 07 Implementation Plan + DoD #1 both list "ties"). The G01..G05
// scenarios never construct two equal-rate cards, so before this test the whole
// deterministic tie-break ladder was unexercised.
//
// ADVERSARIAL BY CONSTRUCTION: each sub-case (1) feeds the cards in the OPPOSITE
// order to the expected winner — so the optimizer must actually reorder, not
// echo input order — and (2) gives the intended WINNER the lexicographically
// LARGER user_card_id. The final tie-break key is the SMALLER id, so if the
// named higher-priority tie-break level were removed the optimizer would fall
// through to that id key (or to input order) and pick the LOSER. The assertions
// therefore fail on any regression of the level they target — they are not
// tautological.
func TestOptimize_TieBreak_DeterministicOrdering(t *testing.T) {
	// Tie-break 1 — a benefit with NO spend limit beats an equal-rate capped one.
	// Winner (no-limit) has the larger id, so only tie-break 1 can elect it.
	t.Run("no-limit beats equal-rate capped", func(t *testing.T) {
		capped := optCard("tie1-a-capped", "Capped Card",
			`[{"category":"Groceries","rate":3,"rate_type":"percent","limit_cents":50000}]`, "", "")
		noLimit := optCard("tie1-z-nolimit", "Unlimited Card",
			`[{"category":"Groceries","rate":3,"rate_type":"percent"}]`, "", "")

		res := Optimize("Groceries", []CardInputs{capped, noLimit}, nil, optNow())

		if res.RecommendedUserCardID == nil || *res.RecommendedUserCardID != "tie1-z-nolimit" {
			t.Fatalf("recommended = %v, want tie1-z-nolimit (no-limit beats equal-rate capped)", res.RecommendedUserCardID)
		}
		if res.EffectiveLimitCents != nil {
			t.Fatalf("effective limit = %v, want nil (the unlimited card won)", res.EffectiveLimitCents)
		}
		if res.Rate != 3 {
			t.Fatalf("rate = %v, want 3", res.Rate)
		}
	})

	// Tie-break 2 — between two equal-rate CAPPED benefits the higher cap wins.
	// Winner (higher cap) has the larger id, so only tie-break 2 can elect it.
	t.Run("higher cap beats equal-rate lower cap", func(t *testing.T) {
		lowCap := optCard("tie2-a-lowcap", "Low Cap Card",
			`[{"category":"Dining","rate":3,"rate_type":"percent","limit_cents":50000}]`, "", "")
		highCap := optCard("tie2-z-highcap", "High Cap Card",
			`[{"category":"Dining","rate":3,"rate_type":"percent","limit_cents":100000}]`, "", "")

		res := Optimize("Dining", []CardInputs{lowCap, highCap}, nil, optNow())

		if res.RecommendedUserCardID == nil || *res.RecommendedUserCardID != "tie2-z-highcap" {
			t.Fatalf("recommended = %v, want tie2-z-highcap (higher cap beats equal-rate lower cap)", res.RecommendedUserCardID)
		}
		if res.EffectiveLimitCents == nil || *res.EffectiveLimitCents != 100000 {
			t.Fatalf("effective limit = %v, want 100000 (the higher-cap card won)", res.EffectiveLimitCents)
		}
	})

	// Tie-break 3 — fully tied (same rate, both uncapped): a STABLE, reproducible
	// key (lexicographically smaller user_card_id) decides so recommendations
	// never flap. Fed larger-id-first, and asserted order-independent.
	t.Run("stable smaller-id key on a full tie", func(t *testing.T) {
		larger := optCard("tie3-bbb", "Card BBB",
			`[{"category":"Travel","rate":2,"rate_type":"percent"}]`, "", "")
		smaller := optCard("tie3-aaa", "Card AAA",
			`[{"category":"Travel","rate":2,"rate_type":"percent"}]`, "", "")

		res := Optimize("Travel", []CardInputs{larger, smaller}, nil, optNow())
		if res.RecommendedUserCardID == nil || *res.RecommendedUserCardID != "tie3-aaa" {
			t.Fatalf("recommended = %v, want tie3-aaa (stable smaller-id key on a full tie)", res.RecommendedUserCardID)
		}

		// Determinism: reversing the input order must not change the verdict.
		res2 := Optimize("Travel", []CardInputs{smaller, larger}, nil, optNow())
		if res2.RecommendedUserCardID == nil || *res2.RecommendedUserCardID != "tie3-aaa" {
			t.Fatalf("recommended (reversed input) = %v, want tie3-aaa (must be input-order-independent)", res2.RecommendedUserCardID)
		}
	})
}
