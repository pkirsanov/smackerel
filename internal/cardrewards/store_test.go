//go:build integration

// Spec 083 Card Rewards Companion (Scope 02) — T-02-02 / T-02-03.
// Live-PostgreSQL integration tests for the card-rewards Store: user-card CRUD
// (SCN-083-B01), custom (non-catalog) card creation (SCN-083-B04), shared-limit
// offers (SCN-083-B05), tiered selections (SCN-083-B06), and ON DELETE CASCADE
// of dependent rows (SCN-083-B07).
//
// Run via: ./smackerel.sh test integration --go-run CardRewardsStore
// The runner sets DATABASE_URL to the disposable test Postgres and adds
// ./internal/cardrewards/... to the integration package list. No mocks — every
// assertion is against a real ephemeral database. Each test namespaces its
// catalog ids with a per-test prefix so parallel/repeat runs never collide.

package cardrewards

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
)

func cardRewardsIntegrationStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	return NewStore(pool)
}

func cardRewardsPrefix(t *testing.T) string {
	t.Helper()
	return "cr-int-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
}

func strptr(s string) *string { return &s }
func intptr(n int) *int       { return &n }

// seedCatalogCard inserts a catalog card with the given suffix and returns its id.
func seedCatalogCard(t *testing.T, ctx context.Context, s *Store, prefix, suffix, cardType string) string {
	t.Helper()
	id := prefix + "-" + suffix
	err := s.CreateCatalogCard(ctx, &CatalogCard{
		ID:       id,
		Name:     "Test " + suffix,
		Issuer:   "TestIssuer",
		CardType: cardType,
		Source:   SourceSeed,
	})
	if err != nil {
		t.Fatalf("seed catalog card %s: %v", id, err)
	}
	return id
}

// SCN-083-B01 — create and read a user card.
func TestCardRewardsStore_CreateReadUserCard_B01(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)

	catalogID := seedCatalogCard(t, ctx, s, prefix, "citi-custom-cash", CardTypeUserSelected)

	uc := &UserCard{
		ID:            uuid.NewString(),
		CardCatalogID: catalogID,
		Nickname:      strptr("Everyday"),
		Note:          strptr("primary dining card"),
		Active:        true,
	}
	if err := s.CreateUserCard(ctx, uc); err != nil {
		t.Fatalf("CreateUserCard: %v", err)
	}

	got, err := s.GetUserCard(ctx, uc.ID)
	if err != nil {
		t.Fatalf("GetUserCard: %v", err)
	}
	if got == nil {
		t.Fatal("GetUserCard returned nil for a card that was just created")
	}
	if got.CardCatalogID != catalogID {
		t.Fatalf("card_catalog_id = %q, want %q", got.CardCatalogID, catalogID)
	}
	if got.Nickname == nil || *got.Nickname != "Everyday" {
		t.Fatalf("nickname = %v, want Everyday", got.Nickname)
	}
	if got.Note == nil || *got.Note != "primary dining card" {
		t.Fatalf("note = %v, want 'primary dining card'", got.Note)
	}
	if !got.Active {
		t.Fatal("active = false, want true")
	}
	if got.CatalogName != "Test citi-custom-cash" {
		t.Fatalf("resolved catalog_name = %q, want %q", got.CatalogName, "Test citi-custom-cash")
	}

	// List must include the new card.
	list, err := s.ListUserCards(ctx, false)
	if err != nil {
		t.Fatalf("ListUserCards: %v", err)
	}
	if !containsUserCard(list, uc.ID) {
		t.Fatalf("ListUserCards did not include %s", uc.ID)
	}
}

// SCN-083-B04 — custom (non-catalog) card creation writes a manual catalog row
// plus a wallet row, atomically.
func TestCardRewardsStore_CreateCustomCard_B04(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)

	cat := &CatalogCard{
		ID:       "manual-" + prefix,
		Name:     "Local Credit Union Visa",
		Issuer:   "Local CU",
		CardType: CardTypeFixed,
		Source:   SourceManual,
	}
	uc := &UserCard{ID: uuid.NewString(), CardCatalogID: cat.ID, Active: true}
	if err := s.CreateCustomCard(ctx, cat, uc); err != nil {
		t.Fatalf("CreateCustomCard: %v", err)
	}

	gotCat, err := s.GetCatalogCard(ctx, cat.ID)
	if err != nil {
		t.Fatalf("GetCatalogCard: %v", err)
	}
	if gotCat == nil {
		t.Fatal("custom catalog row not created")
	}
	if gotCat.Source != SourceManual {
		t.Fatalf("custom catalog source = %q, want manual", gotCat.Source)
	}
	gotUC, err := s.GetUserCard(ctx, uc.ID)
	if err != nil {
		t.Fatalf("GetUserCard: %v", err)
	}
	if gotUC == nil || gotUC.CardCatalogID != cat.ID {
		t.Fatalf("custom wallet row missing or mislinked: %+v", gotUC)
	}
}

// SCN-083-B05 — an offer with a shared_limit_group persists and is queryable
// both by user card and by shared_limit_group.
func TestCardRewardsStore_SharedLimitOffer_B05(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)

	catalogID := seedCatalogCard(t, ctx, s, prefix, "amex-gold", CardTypeFixed)
	uc := &UserCard{ID: uuid.NewString(), CardCatalogID: catalogID, Active: true}
	if err := s.CreateUserCard(ctx, uc); err != nil {
		t.Fatalf("CreateUserCard: %v", err)
	}

	group := "amex-dining-pool-" + prefix
	starts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ends := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	offer := &Offer{
		ID:               uuid.NewString(),
		UserCardID:       strptr(uc.ID),
		Title:            "Dining 4x",
		Category:         "Dining",
		Rate:             4,
		RateType:         RateTypeMultiplier,
		LimitCents:       intptr(150000),
		LimitPeriod:      strptr("year"),
		SharedLimitGroup: strptr(group),
		StartsOn:         &starts,
		EndsOn:           &ends,
	}
	if err := s.CreateOffer(ctx, offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	byCard, err := s.ListOffersByUserCard(ctx, uc.ID)
	if err != nil {
		t.Fatalf("ListOffersByUserCard: %v", err)
	}
	if len(byCard) != 1 || byCard[0].ID != offer.ID {
		t.Fatalf("ListOffersByUserCard = %+v, want exactly the created offer", byCard)
	}
	if byCard[0].SharedLimitGroup == nil || *byCard[0].SharedLimitGroup != group {
		t.Fatalf("shared_limit_group = %v, want %q", byCard[0].SharedLimitGroup, group)
	}

	byGroup, err := s.ListOffersBySharedLimitGroup(ctx, group)
	if err != nil {
		t.Fatalf("ListOffersBySharedLimitGroup: %v", err)
	}
	if len(byGroup) != 1 || byGroup[0].ID != offer.ID {
		t.Fatalf("ListOffersBySharedLimitGroup = %+v, want exactly the created offer", byGroup)
	}
	if byGroup[0].Rate != 4 || byGroup[0].RateType != RateTypeMultiplier {
		t.Fatalf("offer rate=%v type=%q, want 4 multiplier", byGroup[0].Rate, byGroup[0].RateType)
	}
}

// SCN-083-B06 — a tiered selection persists with its tier, period, and category.
func TestCardRewardsStore_TieredSelection_B06(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)

	catalogID := seedCatalogCard(t, ctx, s, prefix, "usbank-cashplus", CardTypeUserSelected)
	uc := &UserCard{ID: uuid.NewString(), CardCatalogID: catalogID, Active: true}
	if err := s.CreateUserCard(ctx, uc); err != nil {
		t.Fatalf("CreateUserCard: %v", err)
	}

	sel := &Selection{
		ID:          uuid.NewString(),
		UserCardID:  uc.ID,
		Category:    "Utilities",
		Tier:        intptr(1),
		PeriodLabel: "Q3 2026",
		Enrolled:    true,
	}
	if err := s.CreateSelection(ctx, sel); err != nil {
		t.Fatalf("CreateSelection: %v", err)
	}

	got, err := s.ListSelectionsByUserCard(ctx, uc.ID)
	if err != nil {
		t.Fatalf("ListSelectionsByUserCard: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("selection count = %d, want 1", len(got))
	}
	if got[0].Tier == nil || *got[0].Tier != 1 {
		t.Fatalf("tier = %v, want 1", got[0].Tier)
	}
	if got[0].PeriodLabel != "Q3 2026" {
		t.Fatalf("period_label = %q, want 'Q3 2026'", got[0].PeriodLabel)
	}
	if got[0].Category != "Utilities" {
		t.Fatalf("category = %q, want 'Utilities'", got[0].Category)
	}
}

// SCN-083-B07 — deleting a user card cascades to its offers, selections, and
// signup bonuses (ON DELETE CASCADE).
func TestCardRewardsStore_CascadeDelete_B07(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)

	catalogID := seedCatalogCard(t, ctx, s, prefix, "chase-sapphire", CardTypeFixed)
	uc := &UserCard{ID: uuid.NewString(), CardCatalogID: catalogID, Active: true}
	if err := s.CreateUserCard(ctx, uc); err != nil {
		t.Fatalf("CreateUserCard: %v", err)
	}

	if err := s.CreateOffer(ctx, &Offer{
		ID: uuid.NewString(), UserCardID: strptr(uc.ID), Title: "Travel 3x",
		Category: "Travel", Rate: 3, RateType: RateTypeMultiplier,
	}); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := s.CreateSelection(ctx, &Selection{
		ID: uuid.NewString(), UserCardID: uc.ID, Category: "Travel", PeriodLabel: "2026",
	}); err != nil {
		t.Fatalf("CreateSelection: %v", err)
	}
	if err := s.CreateSignupBonus(ctx, &SignupBonus{
		ID: uuid.NewString(), UserCardID: uc.ID, BonusType: BonusTypeSpend,
		Description: "Spend $4000 in 3 months", SpendRequiredCents: intptr(400000),
	}); err != nil {
		t.Fatalf("CreateSignupBonus: %v", err)
	}

	// Sanity: dependents exist before delete.
	if offers, _ := s.ListOffersByUserCard(ctx, uc.ID); len(offers) != 1 {
		t.Fatalf("pre-delete offers = %d, want 1", len(offers))
	}
	if sels, _ := s.ListSelectionsByUserCard(ctx, uc.ID); len(sels) != 1 {
		t.Fatalf("pre-delete selections = %d, want 1", len(sels))
	}
	if bons, _ := s.ListBonusesByUserCard(ctx, uc.ID); len(bons) != 1 {
		t.Fatalf("pre-delete bonuses = %d, want 1", len(bons))
	}

	deleted, err := s.DeleteUserCard(ctx, uc.ID)
	if err != nil {
		t.Fatalf("DeleteUserCard: %v", err)
	}
	if !deleted {
		t.Fatal("DeleteUserCard reported no row deleted")
	}

	// All dependents must be gone (cascade).
	if offers, _ := s.ListOffersByUserCard(ctx, uc.ID); len(offers) != 0 {
		t.Fatalf("post-delete offers = %d, want 0 (cascade failed)", len(offers))
	}
	if sels, _ := s.ListSelectionsByUserCard(ctx, uc.ID); len(sels) != 0 {
		t.Fatalf("post-delete selections = %d, want 0 (cascade failed)", len(sels))
	}
	if bons, _ := s.ListBonusesByUserCard(ctx, uc.ID); len(bons) != 0 {
		t.Fatalf("post-delete bonuses = %d, want 0 (cascade failed)", len(bons))
	}
	if got, _ := s.GetUserCard(ctx, uc.ID); got != nil {
		t.Fatal("post-delete GetUserCard returned a row, want nil")
	}
}

func containsUserCard(list []UserCard, id string) bool {
	for _, u := range list {
		if u.ID == id {
			return true
		}
	}
	return false
}
