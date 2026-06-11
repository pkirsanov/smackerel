//go:build integration

// Spec 083 Card Rewards Companion — Scope 03 live-PostgreSQL integration tests
// (T-03-01..T-03-04). They run the one-time CCManager → PostgreSQL importer
// against the hermetic testdata/ccmanager fixture (which mirrors the real
// CCManager data shapes and includes intentional skip cases) and assert
// catalog/alias/rotating/recommendation/run mapping, key field values, and
// idempotency on a real ephemeral database. No mocks.
//
// Run via: ./smackerel.sh test integration --go-run CardRewardsImport
// (DATABASE_URL is set to the disposable test Postgres by the runner.)
//
// Cross-test isolation: assertions are scoped to the importer's fixed catalog
// ids / period labels (discover-it, citi-custom-cash, "2026-01", …), which are
// distinct from the prefixed ids the Scope 02 store tests create, so a shared
// test database and idempotent re-runs do not perturb the counts.

package cardrewards

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureDir = "testdata/ccmanager"

// importTestStore reuses the Scope 02 integration harness (skips when
// DATABASE_URL is unset, runs migrations, returns a live Store).
func importTestStore(t *testing.T) *Store {
	return cardRewardsIntegrationStore(t)
}

// userCardIDFor returns the wallet user_card id the importer created for a
// catalog id (nickname NULL), failing the test if absent.
func userCardIDFor(t *testing.T, ctx context.Context, s *Store, catalogID string) string {
	t.Helper()
	var id string
	err := s.Pool.QueryRow(ctx,
		`SELECT id FROM user_cards WHERE card_catalog_id = $1 AND nickname IS NULL ORDER BY added_at LIMIT 1`,
		catalogID).Scan(&id)
	if err != nil {
		t.Fatalf("no wallet card for catalog id %q: %v", catalogID, err)
	}
	return id
}

func countRecommendationsByPeriod(t *testing.T, ctx context.Context, s *Store, period string) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM card_recommendations WHERE period_label = $1`, period).Scan(&n); err != nil {
		t.Fatalf("count recommendations %q: %v", period, err)
	}
	return n
}

func hasSkipReason(report *ImportReport, table, reasonContains string) bool {
	for _, sr := range report.SkippedRows {
		if sr.Table == table && reasonContains != "" && strings.Contains(sr.Reason, reasonContains) {
			return true
		}
	}
	return false
}

// stageFixtures copies the fixture tree into a temp dir, omitting the named
// top-level files, so a missing-file scenario (SCN-083-C04) can be exercised
// without mutating the committed fixtures.
func stageFixtures(t *testing.T, omit ...string) string {
	t.Helper()
	omitSet := map[string]bool{}
	for _, o := range omit {
		omitSet[o] = true
	}
	dst := t.TempDir()
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	for _, e := range entries {
		if omitSet[e.Name()] {
			continue
		}
		src := filepath.Join(fixtureDir, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dstPath, err)
			}
			sub, err := os.ReadDir(src)
			if err != nil {
				t.Fatalf("read subdir %s: %v", src, err)
			}
			for _, se := range sub {
				copyFile(t, filepath.Join(src, se.Name()), filepath.Join(dstPath, se.Name()))
			}
			continue
		}
		copyFile(t, src, dstPath)
	}
	return dst
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

// T-03-01 / SCN-083-C01 + SCN-083-C05 — catalog and category aliases imported,
// with all mapped tables' row counts matching the source fixture.
func TestCardRewardsImport_CatalogAndAliases_C01_C05(t *testing.T) {
	s := importTestStore(t)
	ctx := context.Background()

	report, err := RunImport(ctx, s, fixtureDir)
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// --- C01: card_catalog seeded with each fixture card (source=seed), the
	// custom card (source=manual), and the unknown-type card skipped. ---
	seedIDs := []string{"discover-it", "amazon-prime-visa", "citi-custom-cash", "us-bank-cash-plus", "capital-one-quicksilver", "home-depot-consumer"}
	for _, id := range seedIDs {
		card, err := s.GetCatalogCard(ctx, id)
		if err != nil || card == nil {
			t.Fatalf("catalog card %q missing: card=%v err=%v", id, card, err)
		}
		if card.Source != SourceSeed {
			t.Errorf("catalog %q source = %q, want seed", id, card.Source)
		}
	}
	if !ValidCardType(mustCard(t, ctx, s, "us-bank-cash-plus").CardType) {
		t.Errorf("us-bank-cash-plus has invalid mapped card_type")
	}
	if got := mustCard(t, ctx, s, "us-bank-cash-plus").CardType; got != CardTypeUserSelected {
		t.Errorf("tiered card mapped to %q, want user-selected", got)
	}
	if got := mustCard(t, ctx, s, "capital-one-quicksilver").CardType; got != CardTypeFixed {
		t.Errorf("flat card mapped to %q, want fixed", got)
	}
	if got := mustCard(t, ctx, s, "discover-it").AnnualFeeCents; got != 0 {
		t.Errorf("discover annual fee cents = %d, want 0", got)
	}
	custom := mustCard(t, ctx, s, "signify-business-card")
	if custom.Source != SourceManual {
		t.Errorf("custom card source = %q, want manual", custom.Source)
	}
	if mystery, _ := s.GetCatalogCard(ctx, "mystery-card"); mystery != nil {
		t.Errorf("unknown-type card should have been skipped, but it was imported")
	}
	if report.Persisted["card_catalog"] != 7 {
		t.Errorf("report card_catalog = %d, want 7 (6 seed + 1 manual)", report.Persisted["card_catalog"])
	}
	if !hasSkipReason(report, "card_catalog", "unknown card type") {
		t.Errorf("expected a skipped card_catalog row for the unknown type; skips=%+v", report.SkippedRows)
	}

	// --- C05: category_aliases reflect canonical names, equivalents, starred,
	// priority. ---
	aliases, err := s.ListCategoryAliases(ctx)
	if err != nil {
		t.Fatalf("ListCategoryAliases: %v", err)
	}
	byName := map[string]CategoryAlias{}
	for _, a := range aliases {
		byName[a.CanonicalCategory] = a
	}
	dining, ok := byName["Dining"]
	if !ok || !dining.Starred || !dining.BuiltIn {
		t.Fatalf("Dining alias missing/wrong: %+v", dining)
	}
	if dining.Priority == nil || *dining.Priority != 2 {
		t.Errorf("Dining priority = %v, want 2", dining.Priority)
	}
	if len(dining.Equivalents) != 2 {
		t.Errorf("Dining equivalents = %v, want 2 entries", dining.Equivalents)
	}
	gas, ok := byName["gas"]
	if !ok || len(gas.Equivalents) != 1 || gas.Equivalents[0] != "fuel" {
		t.Errorf("equivalents-only 'gas' alias wrong: %+v", gas)
	}
	if report.Persisted["category_aliases"] != 8 {
		t.Errorf("report category_aliases = %d, want 8", report.Persisted["category_aliases"])
	}

	// --- row counts for the insert-if-absent tables (DB-scoped to our data). ---
	amazonUC := userCardIDFor(t, ctx, s, "amazon-prime-visa")
	offers, err := s.ListOffersByUserCard(ctx, amazonUC)
	if err != nil {
		t.Fatalf("ListOffersByUserCard: %v", err)
	}
	if len(offers) != 3 {
		t.Errorf("amazon offers = %d, want 3 (multi-category expansion)", len(offers))
	}
	for _, o := range offers {
		if o.SharedLimitGroup == nil {
			t.Errorf("amazon combo offer should carry a shared-limit group: %+v", o)
		}
		if o.LimitCents == nil || *o.LimitCents != 100000 {
			t.Errorf("amazon offer limit cents = %v, want 100000", o.LimitCents)
		}
	}
	citiUC := userCardIDFor(t, ctx, s, "citi-custom-cash")
	sels, err := s.ListSelectionsByUserCard(ctx, citiUC)
	if err != nil {
		t.Fatalf("ListSelectionsByUserCard(citi): %v", err)
	}
	if len(sels) != 1 {
		t.Errorf("citi selections = %d, want 1", len(sels))
	}
	usbankUC := userCardIDFor(t, ctx, s, "us-bank-cash-plus")
	tieredSels, err := s.ListSelectionsByUserCard(ctx, usbankUC)
	if err != nil {
		t.Fatalf("ListSelectionsByUserCard(usbank): %v", err)
	}
	if len(tieredSels) != 3 {
		t.Errorf("usbank tiered selections = %d, want 3", len(tieredSels))
	}
	tierSet := false
	for _, sel := range tieredSels {
		if sel.Tier != nil {
			tierSet = true
		}
	}
	if !tierSet {
		t.Errorf("tiered selections should have a non-nil tier on at least one row")
	}
	bonuses, err := s.ListBonusesByUserCard(ctx, citiUC)
	if err != nil {
		t.Fatalf("ListBonusesByUserCard: %v", err)
	}
	if len(bonuses) != 2 {
		t.Errorf("citi signup bonuses = %d, want 2 (spend + first-year)", len(bonuses))
	}
	if got := countRecommendationsByPeriod(t, ctx, s, "2026-01"); got != 2 {
		t.Errorf("recommendations 2026-01 = %d, want 2", got)
	}
	if got := countRecommendationsByPeriod(t, ctx, s, "2025-12"); got != 1 {
		t.Errorf("recommendations 2025-12 (latest-report) = %d, want 1", got)
	}

	// An unresolved wallet name and a skipped offer/run are recorded, not dropped silently.
	if !hasSkipReason(report, "user_cards", "did not resolve") {
		t.Errorf("expected the unresolvable wallet card to be recorded as skipped; skips=%+v", report.SkippedRows)
	}
}

// T-03-03 / SCN-083-C03 — imported rotating categories are flagged
// manual_override=true with the known discover-it Q1_2026 categories.
func TestCardRewardsImport_RotatingManualOverride_C03(t *testing.T) {
	s := importTestStore(t)
	ctx := context.Background()

	if _, err := RunImport(ctx, s, fixtureDir); err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	rcs, err := s.ListRotatingCategoriesByCard(ctx, "discover-it")
	if err != nil {
		t.Fatalf("ListRotatingCategoriesByCard: %v", err)
	}
	if len(rcs) != 2 {
		t.Fatalf("discover-it rotating rows = %d, want 2 (Q4_2025, Q1_2026)", len(rcs))
	}
	var q1 *RotatingCategory
	for i := range rcs {
		if rcs[i].PeriodLabel == "Q1_2026" {
			q1 = &rcs[i]
		}
		if !rcs[i].ManualOverride {
			t.Errorf("rotating row %s must be ManualOverride=true (SCN-083-C03)", rcs[i].PeriodLabel)
		}
		if rcs[i].Confidence != 1.0 {
			t.Errorf("imported rotating confidence = %v, want 1.0", rcs[i].Confidence)
		}
	}
	if q1 == nil {
		t.Fatalf("discover-it Q1_2026 rotating row not found")
	}
	wantCats := []string{"Grocery Stores", "Wholesale Clubs", "Streaming"}
	if len(q1.Categories) != len(wantCats) {
		t.Fatalf("Q1_2026 categories = %v, want %v", q1.Categories, wantCats)
	}
	for i, c := range wantCats {
		if q1.Categories[i] != c {
			t.Errorf("Q1_2026 category[%d] = %q, want %q", i, q1.Categories[i], c)
		}
	}
	if q1.LifecycleState != LifecycleActive {
		t.Errorf("Q1_2026 lifecycle = %q, want active", q1.LifecycleState)
	}
	if q1.LimitCents == nil || *q1.LimitCents != 150000 {
		t.Errorf("Q1_2026 limit cents = %v, want 150000", q1.LimitCents)
	}
}

// T-03-04 / SCN-083-C04 + SCN-083-C06 — a missing file imports what it can,
// records the skipped file, completes without aborting, and writes a migration
// run row.
func TestCardRewardsImport_PartialFileToleranceAndRunLogged_C04_C06(t *testing.T) {
	s := importTestStore(t)
	ctx := context.Background()

	staged := stageFixtures(t, "user-offers.json")
	report, err := RunImport(ctx, s, staged)
	if err != nil {
		t.Fatalf("RunImport with missing user-offers.json must not abort: %v", err)
	}

	// C04: missing file recorded; other files still imported.
	foundSkip := false
	for _, f := range report.SkippedFiles {
		if f == "user-offers.json" {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Errorf("expected user-offers.json in SkippedFiles, got %v", report.SkippedFiles)
	}
	if report.Status != RunStatusPartial {
		t.Errorf("status with a missing file = %q, want partial", report.Status)
	}
	// Catalog still imported despite the missing offers file.
	if card, _ := s.GetCatalogCard(ctx, "discover-it"); card == nil {
		t.Errorf("catalog should still import when user-offers.json is missing")
	}

	// C06: a migration run row is written.
	if report.MigrationRunID == "" {
		t.Errorf("migration run id not recorded in report")
	}
	migCount, err := s.CountRunsByType(ctx, RunTypeMigration)
	if err != nil {
		t.Fatalf("CountRunsByType(migration): %v", err)
	}
	if migCount < 1 {
		t.Errorf("expected at least one card_runs row with run_type=migration, got %d", migCount)
	}
	// The mappable historical run imported; the unmappable ones were skipped.
	if !hasSkipReason(report, "card_runs", "not in card_runs domain") {
		t.Errorf("expected unmappable run types (user_change/github_sync) to be skipped; skips=%+v", report.SkippedRows)
	}
}

// T-03-02 / SCN-083-C02 — a second import creates zero duplicate data rows
// (idempotent). The migration audit row is intentionally appended each run
// (Principle 8), so only that count grows.
func TestCardRewardsImport_Idempotent_C02(t *testing.T) {
	s := importTestStore(t)
	ctx := context.Background()

	// Run A.
	if _, err := RunImport(ctx, s, fixtureDir); err != nil {
		t.Fatalf("RunImport (A): %v", err)
	}
	before := importScopedCounts(t, ctx, s)
	migBefore, err := s.CountRunsByType(ctx, RunTypeMigration)
	if err != nil {
		t.Fatalf("CountRunsByType(migration) before: %v", err)
	}
	calBefore, err := s.CountRunsByType(ctx, RunTypeCalendarSync)
	if err != nil {
		t.Fatalf("CountRunsByType(calendar_sync) before: %v", err)
	}

	// Run B (idempotent re-run).
	reportB, err := RunImport(ctx, s, fixtureDir)
	if err != nil {
		t.Fatalf("RunImport (B): %v", err)
	}
	after := importScopedCounts(t, ctx, s)

	for table, beforeN := range before {
		if after[table] != beforeN {
			t.Errorf("idempotency: %s count changed %d -> %d on re-run", table, beforeN, after[table])
		}
	}
	// Insert-if-absent tables: re-run inserts nothing new.
	for _, table := range []string{"user_cards", "card_offers", "card_selections", "signup_bonuses"} {
		if reportB.Persisted[table] != 0 {
			t.Errorf("idempotency: re-run reported %d new %s rows, want 0", reportB.Persisted[table], table)
		}
	}
	// The historical calendar_sync run is idempotent; the migration audit row grows by exactly 1.
	calAfter, err := s.CountRunsByType(ctx, RunTypeCalendarSync)
	if err != nil {
		t.Fatalf("CountRunsByType(calendar_sync) after: %v", err)
	}
	if calAfter != calBefore {
		t.Errorf("idempotency: calendar_sync historical runs changed %d -> %d", calBefore, calAfter)
	}
	migAfter, err := s.CountRunsByType(ctx, RunTypeMigration)
	if err != nil {
		t.Fatalf("CountRunsByType(migration) after: %v", err)
	}
	if migAfter != migBefore+1 {
		t.Errorf("migration audit rows = %d, want %d (exactly one appended per run)", migAfter, migBefore+1)
	}
}

// importScopedCounts captures row counts scoped to the importer's fixed ids /
// period labels (immune to other tests' prefixed rows).
func importScopedCounts(t *testing.T, ctx context.Context, s *Store) map[string]int {
	t.Helper()
	counts := map[string]int{}

	rcs, err := s.ListRotatingCategoriesByCard(ctx, "discover-it")
	if err != nil {
		t.Fatalf("rotating count: %v", err)
	}
	counts["rotating_categories(discover-it)"] = len(rcs)

	amazonUC := userCardIDFor(t, ctx, s, "amazon-prime-visa")
	offers, err := s.ListOffersByUserCard(ctx, amazonUC)
	if err != nil {
		t.Fatalf("offer count: %v", err)
	}
	counts["card_offers(amazon)"] = len(offers)

	citiUC := userCardIDFor(t, ctx, s, "citi-custom-cash")
	sels, err := s.ListSelectionsByUserCard(ctx, citiUC)
	if err != nil {
		t.Fatalf("selection count: %v", err)
	}
	counts["card_selections(citi)"] = len(sels)

	usbankUC := userCardIDFor(t, ctx, s, "us-bank-cash-plus")
	tieredSels, err := s.ListSelectionsByUserCard(ctx, usbankUC)
	if err != nil {
		t.Fatalf("tiered selection count: %v", err)
	}
	counts["card_selections(usbank)"] = len(tieredSels)

	bonuses, err := s.ListBonusesByUserCard(ctx, citiUC)
	if err != nil {
		t.Fatalf("bonus count: %v", err)
	}
	counts["signup_bonuses(citi)"] = len(bonuses)

	counts["card_recommendations(2026-01)"] = countRecommendationsByPeriod(t, ctx, s, "2026-01")
	counts["card_recommendations(2025-12)"] = countRecommendationsByPeriod(t, ctx, s, "2025-12")
	return counts
}

func mustCard(t *testing.T, ctx context.Context, s *Store, id string) *CatalogCard {
	t.Helper()
	card, err := s.GetCatalogCard(ctx, id)
	if err != nil || card == nil {
		t.Fatalf("catalog card %q missing: %v", id, err)
	}
	return card
}
