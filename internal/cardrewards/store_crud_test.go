//go:build integration

// Spec 083 Card Rewards Companion (Scope 10) — live-PostgreSQL integration tests
// for the offer/selection/bonus Get/Update/Delete/List store methods added to
// back the server-rendered web UI (J04 edit, J05 toggle, J06 offer edit/toggle/
// remove, J07 selection edit, bonus progress). No mocks — every assertion is
// against a real ephemeral database. Reuses the harness helpers from
// store_test.go (same package, same build tag).
//
// Run via: ./smackerel.sh test integration --go-run CardRewardsStoreCRUD

package cardrewards

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// seedUserCard seeds a catalog card + a wallet entry and returns the user-card id.
func seedUserCard(t *testing.T, ctx context.Context, s *Store, prefix, suffix string) string {
	t.Helper()
	catalogID := seedCatalogCard(t, ctx, s, prefix, suffix, CardTypeFixed)
	id := uuid.NewString()
	if err := s.CreateUserCard(ctx, &UserCard{ID: id, CardCatalogID: catalogID, Active: true}); err != nil {
		t.Fatalf("seed user card: %v", err)
	}
	return id
}

// SCN-083-J06 store layer — offer Get/Update(round-trip)/List/Delete.
func TestCardRewardsStoreCRUD_OfferLifecycle_J06(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedUserCard(t, ctx, s, prefix, "offer-card")

	offerID := uuid.NewString()
	o := &Offer{
		ID:               offerID,
		UserCardID:       &cardID,
		Title:            "Q1 Groceries",
		Category:         "groceries",
		Rate:             5,
		RateType:         RateTypePercent,
		SharedLimitGroup: strptr(prefix + "-grp"),
		LimitCents:       intptr(150000),
	}
	if err := s.CreateOffer(ctx, o); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	got, err := s.GetOffer(ctx, offerID)
	if err != nil || got == nil {
		t.Fatalf("GetOffer = (%v, %v), want a row", got, err)
	}
	if got.SharedLimitGroup == nil || *got.SharedLimitGroup != prefix+"-grp" {
		t.Fatalf("shared_limit_group = %v, want %q", got.SharedLimitGroup, prefix+"-grp")
	}

	// Edit must round-trip (J06).
	got.Title = "Q1 Groceries (edited)"
	got.Category = "wholesale_clubs"
	got.Rate = 6
	got.Activated = true
	ok, err := s.UpdateOffer(ctx, got)
	if err != nil || !ok {
		t.Fatalf("UpdateOffer = (%v, %v), want (true, nil)", ok, err)
	}
	reread, err := s.GetOffer(ctx, offerID)
	if err != nil || reread == nil {
		t.Fatalf("GetOffer after update = (%v, %v)", reread, err)
	}
	if reread.Title != "Q1 Groceries (edited)" || reread.Category != "wholesale_clubs" || reread.Rate != 6 || !reread.Activated {
		t.Fatalf("edit did not round-trip: %+v", reread)
	}

	// ListOffers must include it.
	all, err := s.ListOffers(ctx)
	if err != nil {
		t.Fatalf("ListOffers: %v", err)
	}
	found := false
	for _, x := range all {
		if x.ID == offerID {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListOffers did not include %s", offerID)
	}

	// Delete then confirm absence.
	ok, err = s.DeleteOffer(ctx, offerID)
	if err != nil || !ok {
		t.Fatalf("DeleteOffer = (%v, %v), want (true, nil)", ok, err)
	}
	gone, err := s.GetOffer(ctx, offerID)
	if err != nil {
		t.Fatalf("GetOffer after delete: %v", err)
	}
	if gone != nil {
		t.Fatalf("offer still present after delete: %+v", gone)
	}
	// Deleting a missing row reports false (not an error).
	ok, err = s.DeleteOffer(ctx, offerID)
	if err != nil || ok {
		t.Fatalf("DeleteOffer(missing) = (%v, %v), want (false, nil)", ok, err)
	}
}

// SCN-083-J07 store layer — selection Get/Update/List incl. tier change.
func TestCardRewardsStoreCRUD_SelectionLifecycle_J07(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedUserCard(t, ctx, s, prefix, "sel-card")

	selID := uuid.NewString()
	sel := &Selection{
		ID:          selID,
		UserCardID:  cardID,
		Category:    "dining",
		Tier:        intptr(1),
		PeriodLabel: "2026-Q1",
		Enrolled:    true,
	}
	if err := s.CreateSelection(ctx, sel); err != nil {
		t.Fatalf("CreateSelection: %v", err)
	}

	got, err := s.GetSelection(ctx, selID)
	if err != nil || got == nil {
		t.Fatalf("GetSelection = (%v, %v), want a row", got, err)
	}
	if got.Tier == nil || *got.Tier != 1 {
		t.Fatalf("tier = %v, want 1", got.Tier)
	}

	got.Category = "groceries"
	got.Tier = intptr(2)
	ok, err := s.UpdateSelection(ctx, got)
	if err != nil || !ok {
		t.Fatalf("UpdateSelection = (%v, %v), want (true, nil)", ok, err)
	}
	reread, err := s.GetSelection(ctx, selID)
	if err != nil || reread == nil {
		t.Fatalf("GetSelection after update = (%v, %v)", reread, err)
	}
	if reread.Category != "groceries" || reread.Tier == nil || *reread.Tier != 2 {
		t.Fatalf("selection edit did not round-trip: %+v", reread)
	}

	all, err := s.ListSelections(ctx)
	if err != nil {
		t.Fatalf("ListSelections: %v", err)
	}
	found := false
	for _, x := range all {
		if x.ID == selID {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListSelections did not include %s", selID)
	}
}

// Bonus store layer — Get/Update(progress)/List.
func TestCardRewardsStoreCRUD_BonusLifecycle(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedUserCard(t, ctx, s, prefix, "bonus-card")

	bonusID := uuid.NewString()
	b := &SignupBonus{
		ID:                 bonusID,
		UserCardID:         cardID,
		BonusType:          BonusTypeSpend,
		Description:        "Spend $3000 in 90 days",
		SpendRequiredCents: intptr(300000),
		SpendProgressCents: 50000,
	}
	if err := s.CreateSignupBonus(ctx, b); err != nil {
		t.Fatalf("CreateSignupBonus: %v", err)
	}

	got, err := s.GetSignupBonus(ctx, bonusID)
	if err != nil || got == nil {
		t.Fatalf("GetSignupBonus = (%v, %v), want a row", got, err)
	}
	if got.SpendProgressCents != 50000 {
		t.Fatalf("progress = %d, want 50000", got.SpendProgressCents)
	}

	// Record progress that meets the requirement.
	got.SpendProgressCents = 300000
	got.Met = true
	ok, err := s.UpdateSignupBonus(ctx, got)
	if err != nil || !ok {
		t.Fatalf("UpdateSignupBonus = (%v, %v), want (true, nil)", ok, err)
	}
	reread, err := s.GetSignupBonus(ctx, bonusID)
	if err != nil || reread == nil {
		t.Fatalf("GetSignupBonus after update = (%v, %v)", reread, err)
	}
	if reread.SpendProgressCents != 300000 || !reread.Met {
		t.Fatalf("bonus progress did not round-trip: %+v", reread)
	}

	all, err := s.ListBonuses(ctx)
	if err != nil {
		t.Fatalf("ListBonuses: %v", err)
	}
	found := false
	for _, x := range all {
		if x.ID == bonusID {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListBonuses did not include %s", bonusID)
	}
}
