//go:build integration

// Spec 083 Card Rewards Companion (Scope 07) — T-07-03.
// Live-PostgreSQL integration tests for monthly recommendation generation:
// one card_recommendations row per tracked category (SCN-083-G06) and the
// ADVERSARIAL starred-override preservation (SCN-083-G07).
//
// No mocks — the Store and DB are real and ephemeral. Each test namespaces its
// catalog ids, category-alias canonical names, and period_label with a per-test
// prefix so repeated/parallel runs never collide; assertions filter to the
// test's own rows.
//
// Run via: ./smackerel.sh test integration --go-run CardRewardsRecommend

package cardrewards

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// seedCatalogWithBase inserts a catalog card carrying base_benefits JSON and
// returns its id.
func seedCatalogWithBase(t *testing.T, ctx context.Context, s *Store, prefix, suffix, baseJSON string) string {
	t.Helper()
	id := prefix + "-" + suffix
	err := s.CreateCatalogCard(ctx, &CatalogCard{
		ID:           id,
		Name:         "Test " + suffix,
		Issuer:       "TestIssuer",
		CardType:     CardTypeFixed,
		BaseBenefits: json.RawMessage(baseJSON),
		Source:       SourceSeed,
	})
	if err != nil {
		t.Fatalf("seed catalog %s: %v", id, err)
	}
	return id
}

// addWalletCard inserts a wallet entry for a catalog card and returns its uuid.
func addWalletCard(t *testing.T, ctx context.Context, s *Store, catalogID, nickname string) string {
	t.Helper()
	id := uuid.NewString()
	if err := s.CreateUserCard(ctx, &UserCard{
		ID: id, CardCatalogID: catalogID, Nickname: strptr(nickname), Active: true,
	}); err != nil {
		t.Fatalf("add wallet card for %s: %v", catalogID, err)
	}
	return id
}

func newTestRecommender(s *Store) *Recommender {
	r := NewRecommender(s)
	r.now = func() time.Time { return dateUTC(2026, 6, 15) }
	return r
}

// SCN-083-G06 — generation writes exactly one recommendation per tracked
// category for the period, each with a rate and an explainable reason.
func TestRecommendLivePG_PerCategoryGeneration_G06(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	period := prefix // unique period_label isolates this run's rows

	canonGroc := prefix + "-Groceries"
	canonDining := prefix + "-Dining"
	mustUpsertAlias(t, ctx, s, canonGroc, nil)
	mustUpsertAlias(t, ctx, s, canonDining, nil)

	catalogID := seedCatalogWithBase(t, ctx, s, prefix, "groc-card",
		`[{"category":"`+canonGroc+`","rate":3,"rate_type":"percent"}]`)
	walletID := addWalletCard(t, ctx, s, catalogID, "Everyday")

	rec := newTestRecommender(s)
	report, err := rec.GenerateRecommendations(ctx, period, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("GenerateRecommendations: %v", err)
	}

	aliases, err := s.ListCategoryAliases(ctx)
	if err != nil {
		t.Fatalf("ListCategoryAliases: %v", err)
	}
	recs, err := s.ListRecommendationsByPeriod(ctx, period)
	if err != nil {
		t.Fatalf("ListRecommendationsByPeriod: %v", err)
	}

	// G06: exactly one row per tracked category (the period is unique to this run).
	if len(recs) != len(aliases) {
		t.Fatalf("recommendations=%d, tracked categories=%d — want one row per tracked category", len(recs), len(aliases))
	}
	seen := map[string]int{}
	for _, r := range recs {
		seen[r.Category]++
		if r.Reason == "" {
			t.Fatalf("recommendation for %q has no reason (Principle 8)", r.Category)
		}
	}
	if seen[canonGroc] != 1 || seen[canonDining] != 1 {
		t.Fatalf("category coverage = %v, want exactly one row each for %q and %q", seen, canonGroc, canonDining)
	}

	groc, err := s.GetRecommendation(ctx, period, canonGroc)
	if err != nil || groc == nil {
		t.Fatalf("GetRecommendation(%s): %v", canonGroc, err)
	}
	if groc.RecommendedUserCardID == nil || *groc.RecommendedUserCardID != walletID {
		t.Fatalf("Groceries recommended card = %v, want %s", groc.RecommendedUserCardID, walletID)
	}
	if groc.Rate != 3 {
		t.Fatalf("Groceries rate = %v, want 3", groc.Rate)
	}
	if report.Generated < 2 {
		t.Fatalf("report.Generated = %d, want >= 2", report.Generated)
	}
}

// SCN-083-G07 — a starred_override recommendation is PRESERVED over the
// optimizer's pick (ADVERSARIAL). The optimizer would pick a different, higher
// card; the test proves the manual override survives regeneration unchanged.
func TestRecommendLivePG_StarredOverridePreserved_G07(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	period := prefix
	canonDining := prefix + "-Dining"
	mustUpsertAlias(t, ctx, s, canonDining, nil)

	// The card the optimizer WOULD pick: 5% on Dining.
	optCatalog := seedCatalogWithBase(t, ctx, s, prefix, "opt-card",
		`[{"category":"`+canonDining+`","rate":5,"rate_type":"percent"}]`)
	optWallet := addWalletCard(t, ctx, s, optCatalog, "Optimizer Pick")

	// The manually pinned card (lower rate) referenced by the override row.
	pinCatalog := seedCatalogWithBase(t, ctx, s, prefix, "pin-card",
		`[{"category":"`+canonDining+`","rate":1,"rate_type":"percent"}]`)
	pinWallet := addWalletCard(t, ctx, s, pinCatalog, "My Pinned Card")

	// Guard: prove the optimizer genuinely prefers the 5% card, so the override
	// test is not a coincidence.
	inputs := []CardInputs{
		{UserCard: UserCard{ID: optWallet, CardCatalogID: optCatalog, CatalogName: "Optimizer Pick", Active: true},
			Catalog: mustGetCatalog(t, ctx, s, optCatalog)},
		{UserCard: UserCard{ID: pinWallet, CardCatalogID: pinCatalog, CatalogName: "My Pinned Card", Active: true},
			Catalog: mustGetCatalog(t, ctx, s, pinCatalog)},
	}
	if got := Optimize(canonDining, inputs, []CategoryAlias{{CanonicalCategory: canonDining}}, dateUTC(2026, 6, 15)); got.RecommendedUserCardID == nil || *got.RecommendedUserCardID != optWallet {
		t.Fatalf("precondition: optimizer pick = %v, want the 5%% card %s", got.RecommendedUserCardID, optWallet)
	}

	// Pre-seed the manual starred override pointing at the pinned (lower) card.
	if err := s.UpsertRecommendation(ctx, &CardRecommendation{
		ID:                    uuid.NewString(),
		PeriodLabel:           period,
		Category:              canonDining,
		RecommendedUserCardID: &pinWallet,
		Rate:                  1,
		Reason:                "manually pinned by user",
		Starred:               true,
		StarredOverride:       true,
		GeneratedAt:           dateUTC(2026, 6, 1),
	}); err != nil {
		t.Fatalf("seed override recommendation: %v", err)
	}

	rec := newTestRecommender(s)
	report, err := rec.GenerateRecommendations(ctx, period, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("GenerateRecommendations: %v", err)
	}

	got, err := s.GetRecommendation(ctx, period, canonDining)
	if err != nil || got == nil {
		t.Fatalf("GetRecommendation(%s): %v", canonDining, err)
	}
	if !got.StarredOverride {
		t.Fatal("starred_override was cleared — the manual override was overwritten (G07 regression)")
	}
	if got.RecommendedUserCardID == nil || *got.RecommendedUserCardID != pinWallet {
		t.Fatalf("recommended card = %v, want the pinned card %s (override must beat the optimizer's 5%% pick)", got.RecommendedUserCardID, pinWallet)
	}
	if got.Rate != 1 {
		t.Fatalf("rate = %v, want 1 (the pinned override rate, NOT the optimizer's 5)", got.Rate)
	}
	if report.PreservedOverride < 1 {
		t.Fatalf("report.PreservedOverride = %d, want >= 1", report.PreservedOverride)
	}
}

func mustUpsertAlias(t *testing.T, ctx context.Context, s *Store, canonical string, equivalents []string) {
	t.Helper()
	if err := s.UpsertCategoryAlias(ctx, &CategoryAlias{
		ID:                uuid.NewString(),
		CanonicalCategory: canonical,
		Equivalents:       equivalents,
	}); err != nil {
		t.Fatalf("upsert alias %s: %v", canonical, err)
	}
}

func mustGetCatalog(t *testing.T, ctx context.Context, s *Store, id string) *CatalogCard {
	t.Helper()
	c, err := s.GetCatalogCard(ctx, id)
	if err != nil || c == nil {
		t.Fatalf("GetCatalogCard(%s): %v", id, err)
	}
	return c
}
