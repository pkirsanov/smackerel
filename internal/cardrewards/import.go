package cardrewards

// Spec 083 Card Rewards Companion — Scope 03: one-time, idempotent data
// migration from the standalone CCManager JSON files into the PostgreSQL
// card-rewards schema (design §11). The importer reads a directory of
// CCManager `data/*.json` files and seeds card_catalog, category_aliases,
// user_cards, card_offers, card_selections, signup_bonuses,
// rotating_categories, card_recommendations, and card_runs.
//
// Contract (design §11, scopes.md Scope 03):
//   - Idempotent: a second run creates zero duplicate rows. Tables with a
//     usable unique key upsert (card_catalog by id, category_aliases by
//     canonical_category, rotating_categories by (card,period),
//     card_recommendations by (period,category)); the rest use the Store's
//     InsertXxxIfAbsent helpers keyed on their natural keys (SCN-083-C02).
//   - Partial-file tolerant: a missing/unreadable file imports what it can and
//     records the skip without aborting the whole import (SCN-083-C04).
//   - Imported rotating categories are seeded ManualOverride=true so the first
//     live LLM extraction augments rather than overwrites them (SCN-083-C03).
//   - One card_runs row with RunType="migration" is always written, with
//     Status="partial" when any file/row was skipped, else "success"
//     (SCN-083-C06).
//
// All persistence goes through the Scope 02 Store (no duplicate SQL); card name
// references are resolved with the Scope 02 ResolveCard resolver. Monetary
// amounts are converted to integer cents; nested benefit structures are passed
// through verbatim as jsonb.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// importResolveMinScore is the confidence floor for resolving a CCManager card
// name to a catalog id during import. Wallet/offer/selection references in the
// real data are exact aliases or names (score 1.0); this floor rejects noise
// without silently mis-attributing a row.
const importResolveMinScore = 0.5

// Canonical CCManager source file names (design §11).
const (
	fileCardsDatabase    = "cards-database.json"
	fileUserCards        = "user-cards.json"
	fileUserOffers       = "user-offers.json"
	fileUserSelections   = "user-selections.json"
	filePendingSelect    = "pending-selections.json"
	fileRotatingCats     = "rotating-categories.json"
	fileConfig           = "config.json"
	fileLatestReport     = "latest-report.json"
	fileRunHistory       = "run-history.json"
	dirMonthlyRecommends = "monthly-recommendations"
)

// SkippedRow records a single source row the importer could not persist, with
// the reason (unknown enum value, unresolved card, missing FK target, …). These
// are surfaced in the report, never silently dropped.
type SkippedRow struct {
	Table  string `json:"table"`
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// ImportReport summarizes one import run. Persisted is the per-table count of
// source rows successfully written this run (for upsert tables this is the
// processed count; for insert-if-absent tables it is the number of NEW rows).
// Idempotency is verified against live table counts, not this report.
type ImportReport struct {
	StartedAt      time.Time      `json:"started_at"`
	FinishedAt     time.Time      `json:"finished_at"`
	Status         string         `json:"status"` // success | partial
	Persisted      map[string]int `json:"persisted"`
	SkippedFiles   []string       `json:"skipped_files"`
	SkippedRows    []SkippedRow   `json:"skipped_rows"`
	MigrationRunID string         `json:"migration_run_id"`
}

func newImportReport() *ImportReport {
	return &ImportReport{Persisted: map[string]int{}}
}

func (r *ImportReport) persisted(table string) { r.Persisted[table]++ }
func (r *ImportReport) skipFile(name string)   { r.SkippedFiles = append(r.SkippedFiles, name) }
func (r *ImportReport) skipRow(table, key, reason string) {
	r.SkippedRows = append(r.SkippedRows, SkippedRow{Table: table, Key: key, Reason: reason})
}

// ---- pure transforms (unit-tested in import_transform_test.go) -------------

// MapCardType maps a CCManager card "type" to the card_catalog.card_type CHECK
// domain (rotating|fixed|user-selected). It is an explicit, total mapping over
// the known CCManager types — NOT a silent fallback: an unrecognized type
// returns ok=false so the caller skips and logs the card rather than guessing.
func MapCardType(ccType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(ccType)) {
	case "rotating":
		return CardTypeRotating, true
	case "user-selected", "tiered":
		// A tiered card (e.g. US Bank Cash+) is a user-selected card whose
		// selections are grouped into rate tiers.
		return CardTypeUserSelected, true
	case "fixed", "flat", "store", "hotel", "airline":
		// flat-rate, store, hotel, and airline cards all have fixed category
		// benefits (no rotation, no user selection).
		return CardTypeFixed, true
	default:
		return "", false
	}
}

// dollarsToCents converts a dollar amount to integer cents, rounding to the
// nearest cent to avoid binary-float drift (e.g. 19.99 → 1999).
func dollarsToCents(dollars float64) int {
	return int(math.Round(dollars * 100))
}

// centsPtr converts an optional dollar amount to an optional cents value.
func centsPtr(dollars *float64) *int {
	if dollars == nil {
		return nil
	}
	c := dollarsToCents(*dollars)
	return &c
}

// dateLayouts are tried in order by parseDate. CCManager mixes plain dates,
// RFC3339, and naive microsecond timestamps (no zone).
var dateLayouts = []string{
	"2006-01-02",
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
}

// parseDate parses a CCManager date/timestamp string, returning nil for an
// empty or unparseable value (the target columns are nullable).
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// parsePeriodRange splits a CCManager "YYYY-MM-DD to YYYY-MM-DD" period string
// into start and end dates (either may be nil if absent/unparseable).
func parsePeriodRange(s string) (start, end *time.Time) {
	parts := strings.Split(s, " to ")
	if len(parts) != 2 {
		return nil, nil
	}
	return parseDate(parts[0]), parseDate(parts[1])
}

// deriveLifecycle maps a CCManager quarter "status" to a rotating_categories
// lifecycle_state. Known statuses map directly; an unknown/empty status is
// derived deterministically from the period dates relative to now. ok=false
// (skip) only when neither the status nor any date can determine the state.
func deriveLifecycle(status string, start, end *time.Time, now time.Time) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case LifecycleActive:
		return LifecycleActive, true
	case LifecycleExpired:
		return LifecycleExpired, true
	case LifecycleUpcoming:
		return LifecycleUpcoming, true
	}
	switch {
	case end != nil && end.Before(now):
		return LifecycleExpired, true
	case start != nil && start.After(now):
		return LifecycleUpcoming, true
	case start != nil || end != nil:
		return LifecycleActive, true
	default:
		return "", false
	}
}

// mapRunType maps a CCManager run "type" to the card_runs.run_type CHECK
// domain. Unmappable CCManager-only types (user_change, github_sync, …) return
// ok=false so the importer skips+logs them rather than violating the CHECK.
func mapRunType(ccType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(ccType)) {
	case RunTypeScrape, RunTypeExtract, RunTypeReconcile, RunTypeOptimize,
		RunTypeCalendarSync, RunTypeMigration, RunTypeDiscovery:
		return strings.ToLower(strings.TrimSpace(ccType)), true
	default:
		return "", false
	}
}

// mapRunTrigger maps a CCManager run "trigger" to the card_runs.trigger CHECK
// domain (scheduled|manual). Unknown triggers return ok=false (skip+log).
func mapRunTrigger(ccTrigger string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(ccTrigger)) {
	case "auto", "scheduled", "schedule":
		return RunTriggerScheduled, true
	case "manual", "ui", "user":
		return RunTriggerManual, true
	default:
		return "", false
	}
}

// runStatusFromSuccess maps a CCManager run success flag to a card_runs.status.
func runStatusFromSuccess(success bool) string {
	if success {
		return RunStatusSuccess
	}
	return RunStatusFailed
}

// normalizeOfferRateType maps a CCManager offer rate_type to the
// card_offers.rate_type CHECK domain. An empty value (e.g. a spend-threshold
// promo) is a cashback percent offer; miles are points. Unknown → skip.
func normalizeOfferRateType(rt string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(rt)) {
	case "", RateTypePercent:
		return RateTypePercent, true
	case RateTypePoints, "miles":
		return RateTypePoints, true
	case RateTypeMultiplier:
		return RateTypeMultiplier, true
	default:
		return "", false
	}
}

// quarterLabel formats a date as a CCManager-style "Q<n>_<year>" period label.
func quarterLabel(t time.Time) string {
	q := (int(t.Month())-1)/3 + 1
	return fmt.Sprintf("Q%d_%d", q, t.Year())
}

// monthLabel formats a date as a "YYYY-MM" recommendation period label.
func monthLabel(t time.Time) string {
	return t.Format("2006-01")
}

// ---- CCManager file DTOs ---------------------------------------------------

type ccCatalogCard struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Issuer             string          `json:"issuer"`
	Type               string          `json:"type"`
	AnnualFee          float64         `json:"annual_fee"`
	Requires           *string         `json:"requires"`
	BaseBenefits       json.RawMessage `json:"base_benefits"`
	RotatingBenefits   json.RawMessage `json:"rotating_benefits"`
	SelectableBenefits json.RawMessage `json:"selectable_benefits"`
	TieredBenefits     json.RawMessage `json:"tiered_benefits"`
	Perks              json.RawMessage `json:"perks"`
	Aliases            []string        `json:"aliases"`
}

type ccCardsDatabase struct {
	Cards map[string]ccCatalogCard `json:"cards"`
}

type ccSpendBonus struct {
	Amount        float64 `json:"amount"`
	SpendRequired float64 `json:"spend_required"`
	StartDate     string  `json:"start_date"`
	Deadline      string  `json:"deadline"`
	Completed     bool    `json:"completed"`
	Notes         string  `json:"notes"`
}

type ccFirstYearBonus struct {
	Category   string  `json:"category"`
	Rate       float64 `json:"rate"`
	NormalRate float64 `json:"normal_rate"`
	StartDate  string  `json:"start_date"`
	EndDate    string  `json:"end_date"`
	Notes      string  `json:"notes"`
}

type ccSignupBonuses struct {
	SpendBonus     *ccSpendBonus     `json:"spend_bonus"`
	FirstYearBonus *ccFirstYearBonus `json:"first_year_bonus"`
}

type ccCustomCard struct {
	Name         string          `json:"name"`
	Issuer       string          `json:"issuer"`
	Type         string          `json:"type"`
	BaseBenefits json.RawMessage `json:"base_benefits"`
}

type ccUserCards struct {
	Cards         []string                   `json:"cards"`
	Notes         map[string]string          `json:"notes"`
	SignupBonuses map[string]ccSignupBonuses `json:"signup_bonuses"`
	CustomCards   map[string]ccCustomCard    `json:"custom_cards"`
}

type ccOffer struct {
	ID                 string   `json:"id"`
	Card               string   `json:"card"`
	Categories         []string `json:"categories"`
	Rate               *float64 `json:"rate"`
	RateType           string   `json:"rate_type"`
	Limit              *float64 `json:"limit"`
	LimitPeriod        string   `json:"limit_period"`
	LimitShared        bool     `json:"limit_shared"`
	StartDate          string   `json:"start_date"`
	EndDate            string   `json:"end_date"`
	ActivationRequired bool     `json:"activation_required"`
	Activated          bool     `json:"activated"`
	Notes              string   `json:"notes"`
	PromoType          string   `json:"promo_type"`
	SpendRequired      *float64 `json:"spend_required"`
	RewardAmount       *float64 `json:"reward_amount"`
	RewardType         string   `json:"reward_type"`
}

type ccOffers struct {
	Offers []ccOffer `json:"offers"`
}

type ccSelection struct {
	Categories []string `json:"categories"`
	LockUntil  string   `json:"lock_until"`
	Notes      string   `json:"notes"`
}

type ccTier struct {
	TierIndex  int      `json:"tier_index"`
	TierName   string   `json:"tier_name"`
	Categories []string `json:"categories"`
	Rate       float64  `json:"rate"`
	RateType   string   `json:"rate_type"`
}

type ccTieredSelection struct {
	Tiers     []ccTier `json:"tiers"`
	LockUntil string   `json:"lock_until"`
}

type ccSelections struct {
	Selections       map[string]ccSelection       `json:"selections"`
	TieredSelections map[string]ccTieredSelection `json:"tiered_selections"`
}

type ccConfigEquivalents map[string][]string

type ccConfigCategories struct {
	Starred     []string            `json:"starred"`
	Priority    []string            `json:"priority"`
	BuiltIn     []string            `json:"built_in"`
	Equivalents ccConfigEquivalents `json:"equivalents"`
}

type ccConfig struct {
	Categories ccConfigCategories `json:"categories"`
}

type ccQuarter struct {
	Categories         []string `json:"categories"`
	Period             string   `json:"period"`
	Limit              *float64 `json:"limit"`
	Status             string   `json:"status"`
	ActivationRequired bool     `json:"activation_required"`
}

type ccRecCategory struct {
	CardName  string  `json:"card_name"`
	CardID    string  `json:"card_id"`
	Rate      float64 `json:"rate"`
	RateType  string  `json:"rate_type"`
	Source    string  `json:"source"`
	Notes     string  `json:"notes"`
	IsStarred bool    `json:"is_starred"`
}

type ccMonthlyRec struct {
	Month      string                   `json:"month"`
	Quarter    string                   `json:"quarter"`
	Categories map[string]ccRecCategory `json:"categories"`
}

type ccLatestReport struct {
	Date         string `json:"date"`
	Optimization struct {
		Recommendations map[string]ccRecCategory `json:"recommendations"`
	} `json:"optimization"`
}

type ccRun struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Trigger   string `json:"trigger"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
}

type ccRunHistory struct {
	Runs []ccRun `json:"runs"`
}

// readJSONFile reads dir/name and unmarshals it into dst. It returns found=false
// (and nil error) when the file does not exist, so the caller can record a
// skipped file and continue (SCN-083-C04). A present-but-malformed file returns
// found=true and a non-nil error.
func readJSONFile(dir, name string, dst any) (found bool, err error) {
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return true, fmt.Errorf("parse %s: %w", name, err)
	}
	return true, nil
}

// RunImport executes the one-time CCManager JSON → PostgreSQL migration against
// dataDir. It is idempotent and partial-file tolerant; the returned report
// records per-table counts, skipped files, and skipped rows. An error is
// returned only for a hard failure (empty dataDir, unreadable directory, or a
// store write error that is not row-level recoverable).
func RunImport(ctx context.Context, store *Store, dataDir string) (*ImportReport, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("cardrewards import: data directory is required")
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return nil, fmt.Errorf("cardrewards import: data directory %q: %w", dataDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cardrewards import: %q is not a directory", dataDir)
	}

	report := newImportReport()
	report.StartedAt = time.Now().UTC()

	// 1 + 2. Seed card_catalog from cards-database.json (source=seed) and from
	// user-cards.json custom_cards (source=manual). The custom cards must be in
	// the catalog before wallet resolution so user_cards referencing them are
	// FK-valid.
	var userCards ccUserCards
	userCardsFound, err := readJSONFile(dataDir, fileUserCards, &userCards)
	if err != nil {
		return nil, err
	}
	if !userCardsFound {
		report.skipFile(fileUserCards)
	}

	if err := importCatalog(ctx, store, dataDir, &userCards, report); err != nil {
		return nil, err
	}

	// Load the seeded catalog once for name resolution.
	catalog, err := store.ListCatalogCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("cardrewards import: load catalog: %w", err)
	}

	// 3. category_aliases from config.json.
	if err := importCategoryAliases(ctx, store, dataDir, report); err != nil {
		return nil, err
	}

	// 4. user_cards from the wallet name list (+ signup bonuses). Build a
	// catalog-id → user_card-id map for offer/selection/bonus linking.
	wallet := map[string]string{}
	if userCardsFound {
		if err := importUserCards(ctx, store, &userCards, catalog, wallet, report); err != nil {
			return nil, err
		}
		if err := importSignupBonuses(ctx, store, &userCards, wallet, report); err != nil {
			return nil, err
		}
	}

	// 5. offers, 6. selections, 7. rotating, 8. recommendations, 9. runs.
	if err := importOffers(ctx, store, dataDir, catalog, wallet, report); err != nil {
		return nil, err
	}
	if err := importSelections(ctx, store, dataDir, catalog, wallet, report); err != nil {
		return nil, err
	}
	if err := importRotatingCategories(ctx, store, dataDir, report); err != nil {
		return nil, err
	}
	if err := importRecommendations(ctx, store, dataDir, catalog, wallet, report); err != nil {
		return nil, err
	}
	if err := importRunHistory(ctx, store, dataDir, report); err != nil {
		return nil, err
	}

	// 10. Always write the migration audit run (Principle 8, SCN-083-C06).
	report.FinishedAt = time.Now().UTC()
	report.Status = RunStatusSuccess
	if len(report.SkippedFiles) > 0 || len(report.SkippedRows) > 0 {
		report.Status = RunStatusPartial
	}
	runID := uuid.NewString()
	var errDetail *string
	if report.Status == RunStatusPartial {
		d := fmt.Sprintf("skipped_files=%d skipped_rows=%d", len(report.SkippedFiles), len(report.SkippedRows))
		errDetail = &d
	}
	migRun := &CardRun{
		ID:                  runID,
		RunType:             RunTypeMigration,
		Trigger:             RunTriggerManual,
		Status:              report.Status,
		SourcesAttempted:    1,
		SourcesSucceeded:    1,
		CategoriesExtracted: totalPersisted(report),
		EventsWritten:       totalPersisted(report),
		ErrorDetail:         errDetail,
		StartedAt:           &report.StartedAt,
		FinishedAt:          &report.FinishedAt,
	}
	if err := store.CreateRun(ctx, migRun); err != nil {
		return nil, fmt.Errorf("cardrewards import: write migration run: %w", err)
	}
	report.MigrationRunID = runID
	report.persisted("card_runs")
	return report, nil
}

func totalPersisted(r *ImportReport) int {
	total := 0
	for table, n := range r.Persisted {
		if table == "card_runs" {
			continue
		}
		total += n
	}
	return total
}

// importCatalog seeds card_catalog from cards-database.json (source=seed) and
// user-cards.json custom_cards (source=manual).
func importCatalog(ctx context.Context, store *Store, dataDir string, userCards *ccUserCards, report *ImportReport) error {
	var db ccCardsDatabase
	found, err := readJSONFile(dataDir, fileCardsDatabase, &db)
	if err != nil {
		return err
	}
	if !found {
		report.skipFile(fileCardsDatabase)
	}

	// Stable ordering for deterministic skip logs / counts.
	slugs := sortedKeys(db.Cards)
	for _, slug := range slugs {
		cc := db.Cards[slug]
		card, ok := buildCatalogCard(cc)
		if !ok {
			report.skipRow("card_catalog", cc.ID, fmt.Sprintf("unknown card type %q", cc.Type))
			continue
		}
		if err := store.UpsertCatalogCard(ctx, card); err != nil {
			return fmt.Errorf("upsert catalog card %s: %w", card.ID, err)
		}
		report.persisted("card_catalog")
	}

	// Custom cards from the wallet file → catalog (source=manual).
	for _, id := range sortedKeys(userCards.CustomCards) {
		cc := userCards.CustomCards[id]
		cardType, ok := MapCardType(cc.Type)
		if !ok {
			report.skipRow("card_catalog", id, fmt.Sprintf("unknown custom card type %q", cc.Type))
			continue
		}
		card := &CatalogCard{
			ID:           id,
			Name:         cc.Name,
			Issuer:       cc.Issuer,
			CardType:     cardType,
			BaseBenefits: cc.BaseBenefits,
			Aliases:      []string{strings.ToLower(cc.Name)},
			Source:       SourceManual,
		}
		if err := store.UpsertCatalogCard(ctx, card); err != nil {
			return fmt.Errorf("upsert custom catalog card %s: %w", id, err)
		}
		report.persisted("card_catalog")
	}
	return nil
}

// buildCatalogCard converts a CCManager catalog card to a CatalogCard, placing
// the rotating/selectable/tiered benefit structures into the right jsonb column
// for the mapped type. Returns ok=false for an unmappable card type.
func buildCatalogCard(cc ccCatalogCard) (*CatalogCard, bool) {
	cardType, ok := MapCardType(cc.Type)
	if !ok {
		return nil, false
	}
	card := &CatalogCard{
		ID:             cc.ID,
		Name:           cc.Name,
		Issuer:         cc.Issuer,
		CardType:       cardType,
		AnnualFeeCents: dollarsToCents(cc.AnnualFee),
		Requires:       cc.Requires,
		BaseBenefits:   cc.BaseBenefits,
		Perks:          cc.Perks,
		Aliases:        cc.Aliases,
		Source:         SourceSeed,
	}
	// Rotating cards carry rotating_benefits; user-selected/tiered cards carry
	// selectable_benefits (a tiered card's tiered_benefits live there).
	card.RotatingBenefits = cc.RotatingBenefits
	switch {
	case len(cc.SelectableBenefits) > 0:
		card.SelectableBenefits = cc.SelectableBenefits
	case len(cc.TieredBenefits) > 0:
		card.SelectableBenefits = cc.TieredBenefits
	}
	return card, true
}

// importCategoryAliases seeds category_aliases from config.json categories.*.
func importCategoryAliases(ctx context.Context, store *Store, dataDir string, report *ImportReport) error {
	var cfg ccConfig
	found, err := readJSONFile(dataDir, fileConfig, &cfg)
	if err != nil {
		return err
	}
	if !found {
		report.skipFile(fileConfig)
		return nil
	}
	for _, a := range buildCategoryAliases(cfg.Categories) {
		if err := store.UpsertCategoryAlias(ctx, a); err != nil {
			return fmt.Errorf("upsert category alias %s: %w", a.CanonicalCategory, err)
		}
		report.persisted("category_aliases")
	}
	return nil
}

// buildCategoryAliases flattens config.json categories.{built_in,starred,
// priority,equivalents} into one CategoryAlias per distinct (case-insensitive)
// category name, merging the starred/priority/built_in/equivalents attributes.
// Deterministic: ordered by canonical name.
func buildCategoryAliases(cats ccConfigCategories) []*CategoryAlias {
	type acc struct {
		canonical   string
		equivalents []string
		starred     bool
		priority    *int
		builtIn     bool
	}
	byKey := map[string]*acc{}
	get := func(name string) *acc {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			return nil
		}
		a := byKey[key]
		if a == nil {
			a = &acc{canonical: strings.TrimSpace(name)}
			byKey[key] = a
		}
		return a
	}

	for _, name := range cats.BuiltIn {
		if a := get(name); a != nil {
			a.builtIn = true
		}
	}
	for _, name := range cats.Starred {
		if a := get(name); a != nil {
			a.starred = true
		}
	}
	for i, name := range cats.Priority {
		if a := get(name); a != nil {
			p := i // 0-based priority; lower sorts first
			a.priority = &p
		}
	}
	for name, equivs := range cats.Equivalents {
		if a := get(name); a != nil {
			a.equivalents = append(a.equivalents, equivs...)
		}
	}

	out := make([]*CategoryAlias, 0, len(byKey))
	for _, a := range byKey {
		out = append(out, &CategoryAlias{
			ID:                uuid.NewString(),
			CanonicalCategory: a.canonical,
			Equivalents:       a.equivalents,
			Starred:           a.starred,
			Priority:          a.priority,
			BuiltIn:           a.builtIn,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CanonicalCategory < out[j].CanonicalCategory })
	return out
}

// importUserCards resolves each wallet card name to a catalog id and find-or-
// creates the user_cards row, populating the catalog-id → user_card-id wallet
// map for later offer/selection/bonus linking.
func importUserCards(ctx context.Context, store *Store, userCards *ccUserCards, catalog []CatalogCard, wallet map[string]string, report *ImportReport) error {
	for _, name := range userCards.Cards {
		catalogID, ok := resolveCatalogID(name, catalog)
		if !ok {
			report.skipRow("user_cards", name, "card name did not resolve to a catalog card")
			continue
		}
		var note *string
		if n := strings.TrimSpace(userCards.Notes[catalogID]); n != "" {
			note = &n
		}
		uc := &UserCard{ID: uuid.NewString(), CardCatalogID: catalogID, Note: note, Active: true}
		id, created, err := store.GetOrCreateUserCardByCatalog(ctx, uc)
		if err != nil {
			return fmt.Errorf("import user card %s: %w", catalogID, err)
		}
		wallet[catalogID] = id
		if created {
			report.persisted("user_cards")
		}
	}
	return nil
}

// importSignupBonuses imports per-card spend and first-year signup bonuses,
// linking each to the wallet entry for its catalog id.
func importSignupBonuses(ctx context.Context, store *Store, userCards *ccUserCards, wallet map[string]string, report *ImportReport) error {
	for _, catalogID := range sortedKeys(userCards.SignupBonuses) {
		ucID, ok := wallet[catalogID]
		if !ok {
			report.skipRow("signup_bonuses", catalogID, "no wallet card for catalog id")
			continue
		}
		sb := userCards.SignupBonuses[catalogID]
		for _, bonus := range buildSignupBonuses(ucID, sb) {
			inserted, err := store.InsertSignupBonusIfAbsent(ctx, bonus)
			if err != nil {
				return fmt.Errorf("import signup bonus %s: %w", catalogID, err)
			}
			if inserted {
				report.persisted("signup_bonuses")
			}
		}
	}
	return nil
}

// buildSignupBonuses converts a CCManager signup-bonus block into SignupBonus
// rows (spend bonus and/or first-year-rate bonus).
func buildSignupBonuses(userCardID string, sb ccSignupBonuses) []*SignupBonus {
	var out []*SignupBonus
	if sb.SpendBonus != nil {
		b := sb.SpendBonus
		desc := b.Notes
		if desc == "" {
			desc = fmt.Sprintf("$%.0f bonus after $%.0f spend", b.Amount, b.SpendRequired)
		}
		req := b.SpendRequired
		out = append(out, &SignupBonus{
			ID:                 uuid.NewString(),
			UserCardID:         userCardID,
			BonusType:          BonusTypeSpend,
			Description:        desc,
			SpendRequiredCents: centsPtr(&req),
			Met:                b.Completed,
			Deadline:           parseDate(b.Deadline),
		})
	}
	if sb.FirstYearBonus != nil {
		b := sb.FirstYearBonus
		desc := b.Notes
		if desc == "" {
			desc = fmt.Sprintf("first-year %.0f%% on %s (normally %.0f%%)", b.Rate, b.Category, b.NormalRate)
		}
		reward := fmt.Sprintf("%.0f%% on %s", b.Rate, b.Category)
		out = append(out, &SignupBonus{
			ID:                uuid.NewString(),
			UserCardID:        userCardID,
			BonusType:         BonusTypeFirstYearRate,
			Description:       desc,
			RewardDescription: &reward,
			Deadline:          parseDate(b.EndDate),
		})
	}
	return out
}

// importOffers imports promo/elevated-rate offers, expanding each multi-category
// CCManager offer into one card_offers row per category (sharing a limit group
// when the CCManager limit is shared).
func importOffers(ctx context.Context, store *Store, dataDir string, catalog []CatalogCard, wallet map[string]string, report *ImportReport) error {
	var offers ccOffers
	found, err := readJSONFile(dataDir, fileUserOffers, &offers)
	if err != nil {
		return err
	}
	if !found {
		report.skipFile(fileUserOffers)
		return nil
	}
	for _, o := range offers.Offers {
		rows, ok, reason := buildOffers(o, catalog, wallet)
		if !ok {
			report.skipRow("card_offers", o.ID, reason)
			continue
		}
		for _, row := range rows {
			inserted, err := store.InsertOfferIfAbsent(ctx, row)
			if err != nil {
				return fmt.Errorf("import offer %s: %w", o.ID, err)
			}
			if inserted {
				report.persisted("card_offers")
			}
		}
	}
	return nil
}

// buildOffers converts a CCManager offer into one Offer per category. The
// owning wallet card is resolved from the offer's card name (nil user_card for
// an offer whose card is not in the wallet — a general offer). Returns ok=false
// with a reason when the offer has no usable category or rate type.
func buildOffers(o ccOffer, catalog []CatalogCard, wallet map[string]string) ([]*Offer, bool, string) {
	if len(o.Categories) == 0 {
		return nil, false, "offer has no categories"
	}
	rateType, ok := normalizeOfferRateType(o.RateType)
	if !ok {
		return nil, false, fmt.Sprintf("unknown rate_type %q", o.RateType)
	}
	var userCardID *string
	if catalogID, resolved := resolveCatalogID(o.Card, catalog); resolved {
		if ucID, inWallet := wallet[catalogID]; inWallet {
			userCardID = &ucID
		}
	}
	title := strings.TrimSpace(o.Notes)
	if title == "" {
		title = o.ID
	}
	rate := 0.0
	if o.Rate != nil {
		rate = *o.Rate
	}
	notes := o.Notes
	if o.PromoType == "spend_threshold" && o.SpendRequired != nil && o.RewardAmount != nil {
		extra := fmt.Sprintf("spend $%.0f → $%.0f %s", *o.SpendRequired, *o.RewardAmount, o.RewardType)
		if notes == "" {
			notes = extra
		} else {
			notes = notes + " (" + extra + ")"
		}
	}
	var limitPeriod, sharedGroup, notesPtrVal *string
	if lp := strings.TrimSpace(o.LimitPeriod); lp != "" {
		limitPeriod = &lp
	}
	if o.LimitShared {
		g := o.ID
		sharedGroup = &g
	}
	if n := strings.TrimSpace(notes); n != "" {
		notesPtrVal = &n
	}
	starts := parseDate(o.StartDate)
	ends := parseDate(o.EndDate)
	limitCents := centsPtr(o.Limit)

	out := make([]*Offer, 0, len(o.Categories))
	for _, cat := range o.Categories {
		cat = strings.TrimSpace(cat)
		if cat == "" {
			continue
		}
		out = append(out, &Offer{
			ID:                 uuid.NewString(),
			UserCardID:         userCardID,
			Title:              title,
			Category:           cat,
			Rate:               rate,
			RateType:           rateType,
			LimitCents:         limitCents,
			LimitPeriod:        limitPeriod,
			SharedLimitGroup:   sharedGroup,
			StartsOn:           starts,
			EndsOn:             ends,
			ActivationRequired: o.ActivationRequired,
			Activated:          o.Activated,
			Notes:              notesPtrVal,
		})
	}
	if len(out) == 0 {
		return nil, false, "offer has no non-empty categories"
	}
	return out, true, ""
}

// importSelections imports user category selections (flat and tiered) from
// user-selections.json. pending-selections.json is read for tolerance but the
// real snapshot carries no pending rows to import.
func importSelections(ctx context.Context, store *Store, dataDir string, catalog []CatalogCard, wallet map[string]string, report *ImportReport) error {
	var sels ccSelections
	found, err := readJSONFile(dataDir, fileUserSelections, &sels)
	if err != nil {
		return err
	}
	if !found {
		report.skipFile(fileUserSelections)
	}

	// pending-selections.json: read for partial-tolerance; nothing to map in
	// the current snapshot shape, but a missing file is still recorded.
	var pending map[string]json.RawMessage
	pendingFound, perr := readJSONFile(dataDir, filePendingSelect, &pending)
	if perr != nil {
		return perr
	}
	if !pendingFound {
		report.skipFile(filePendingSelect)
	}

	if !found {
		return nil
	}

	for _, catalogID := range sortedKeys(sels.Selections) {
		ucID, ok := walletFor(catalogID, catalog, wallet)
		if !ok {
			report.skipRow("card_selections", catalogID, "no wallet card for selection")
			continue
		}
		sel := sels.Selections[catalogID]
		period := deriveSelectionPeriod(sel.LockUntil)
		end := parseDate(sel.LockUntil)
		for _, cat := range sel.Categories {
			cat = strings.TrimSpace(cat)
			if cat == "" {
				continue
			}
			row := &Selection{
				ID:           uuid.NewString(),
				UserCardID:   ucID,
				Category:     cat,
				PeriodLabel:  period,
				Enrolled:     true,
				EffectiveEnd: end,
			}
			inserted, err := store.InsertSelectionIfAbsent(ctx, row)
			if err != nil {
				return fmt.Errorf("import selection %s/%s: %w", catalogID, cat, err)
			}
			if inserted {
				report.persisted("card_selections")
			}
		}
	}

	for _, catalogID := range sortedKeys(sels.TieredSelections) {
		ucID, ok := walletFor(catalogID, catalog, wallet)
		if !ok {
			report.skipRow("card_selections", catalogID, "no wallet card for tiered selection")
			continue
		}
		ts := sels.TieredSelections[catalogID]
		period := deriveSelectionPeriod(ts.LockUntil)
		end := parseDate(ts.LockUntil)
		for _, tier := range ts.Tiers {
			tierIdx := tier.TierIndex
			for _, cat := range tier.Categories {
				cat = strings.TrimSpace(cat)
				if cat == "" {
					continue
				}
				tierVal := tierIdx
				row := &Selection{
					ID:           uuid.NewString(),
					UserCardID:   ucID,
					Category:     cat,
					Tier:         &tierVal,
					PeriodLabel:  period,
					Enrolled:     true,
					EffectiveEnd: end,
				}
				inserted, err := store.InsertSelectionIfAbsent(ctx, row)
				if err != nil {
					return fmt.Errorf("import tiered selection %s/%s: %w", catalogID, cat, err)
				}
				if inserted {
					report.persisted("card_selections")
				}
			}
		}
	}
	return nil
}

// deriveSelectionPeriod derives a stable period label for a selection from its
// lock-until date (the quarter it locks through), or "current" when absent.
func deriveSelectionPeriod(lockUntil string) string {
	if t := parseDate(lockUntil); t != nil {
		return quarterLabel(*t)
	}
	return "current"
}

// importRotatingCategories seeds rotating_categories from rotating-categories.json.
// Imported rows are ManualOverride=true with Confidence=1 so the first live
// extraction augments rather than overwrites them (SCN-083-C03). Quarters for a
// card not present in the catalog are skipped+logged (FK safety / partial
// tolerance).
func importRotatingCategories(ctx context.Context, store *Store, dataDir string, report *ImportReport) error {
	var raw map[string]json.RawMessage
	found, err := readJSONFile(dataDir, fileRotatingCats, &raw)
	if err != nil {
		return err
	}
	if !found {
		report.skipFile(fileRotatingCats)
		return nil
	}
	now := time.Now().UTC()
	cardIDs := make([]string, 0, len(raw))
	for k := range raw {
		if k == "current_quarter" {
			continue // top-level scalar, not a card
		}
		cardIDs = append(cardIDs, k)
	}
	sort.Strings(cardIDs)

	for _, cardID := range cardIDs {
		// FK safety: only seed quarters for a card that exists in the catalog.
		cat, err := store.GetCatalogCard(ctx, cardID)
		if err != nil {
			return fmt.Errorf("rotating: lookup catalog %s: %w", cardID, err)
		}
		if cat == nil {
			report.skipRow("rotating_categories", cardID, "no catalog card for rotating history")
			continue
		}
		var quarters map[string]ccQuarter
		if err := json.Unmarshal(raw[cardID], &quarters); err != nil {
			report.skipRow("rotating_categories", cardID, "unparseable quarter map")
			continue
		}
		for _, periodLabel := range sortedKeys(quarters) {
			q := quarters[periodLabel]
			rc, ok, reason := buildRotatingCategory(cardID, periodLabel, q, now)
			if !ok {
				report.skipRow("rotating_categories", cardID+"/"+periodLabel, reason)
				continue
			}
			if err := store.UpsertRotatingCategory(ctx, rc); err != nil {
				return fmt.Errorf("import rotating %s/%s: %w", cardID, periodLabel, err)
			}
			report.persisted("rotating_categories")
		}
	}
	return nil
}

// buildRotatingCategory converts a CCManager quarter into a RotatingCategory
// seeded with ManualOverride=true and Confidence=1.
func buildRotatingCategory(cardID, periodLabel string, q ccQuarter, now time.Time) (*RotatingCategory, bool, string) {
	if len(q.Categories) == 0 {
		return nil, false, "quarter has no categories"
	}
	start, end := parsePeriodRange(q.Period)
	lifecycle, ok := deriveLifecycle(q.Status, start, end, now)
	if !ok {
		return nil, false, fmt.Sprintf("cannot derive lifecycle from status %q", q.Status)
	}
	return &RotatingCategory{
		ID:                 uuid.NewString(),
		CardCatalogID:      cardID,
		PeriodLabel:        periodLabel,
		PeriodStart:        start,
		PeriodEnd:          end,
		Categories:         q.Categories,
		LimitCents:         centsPtr(q.Limit),
		ActivationRequired: q.ActivationRequired,
		LifecycleState:     lifecycle,
		Confidence:         1.0,
		NeedsVerification:  false,
		ManualOverride:     true,
		SourceCount:        1,
	}, true, ""
}

// importRecommendations seeds card_recommendations from the monthly-recommendations
// directory and latest-report.json. Both are idempotent upserts keyed on
// (period_label, category).
func importRecommendations(ctx context.Context, store *Store, dataDir string, catalog []CatalogCard, wallet map[string]string, report *ImportReport) error {
	monthlyDir := filepath.Join(dataDir, dirMonthlyRecommends)
	entries, derr := os.ReadDir(monthlyDir)
	if derr != nil {
		if os.IsNotExist(derr) {
			report.skipFile(dirMonthlyRecommends + "/")
		} else {
			return fmt.Errorf("read %s: %w", dirMonthlyRecommends, derr)
		}
	} else {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			var mr ccMonthlyRec
			if _, err := readJSONFile(monthlyDir, name, &mr); err != nil {
				report.skipRow("card_recommendations", dirMonthlyRecommends+"/"+name, "unparseable monthly recommendation")
				continue
			}
			period := strings.TrimSpace(mr.Month)
			if period == "" {
				report.skipRow("card_recommendations", name, "monthly recommendation missing month")
				continue
			}
			for _, cat := range sortedKeys(mr.Categories) {
				rec := buildRecommendation(period, cat, mr.Categories[cat], catalog, wallet)
				if err := store.UpsertRecommendation(ctx, rec); err != nil {
					return fmt.Errorf("import recommendation %s/%s: %w", period, cat, err)
				}
				report.persisted("card_recommendations")
			}
		}
	}

	// latest-report.json (different shape; period derived from its date).
	var lr ccLatestReport
	found, err := readJSONFile(dataDir, fileLatestReport, &lr)
	if err != nil {
		return err
	}
	if !found {
		report.skipFile(fileLatestReport)
		return nil
	}
	period := "latest"
	if t := parseDate(lr.Date); t != nil {
		period = monthLabel(*t)
	}
	for _, cat := range sortedKeys(lr.Optimization.Recommendations) {
		rec := buildRecommendation(period, cat, lr.Optimization.Recommendations[cat], catalog, wallet)
		if err := store.UpsertRecommendation(ctx, rec); err != nil {
			return fmt.Errorf("import latest recommendation %s/%s: %w", period, cat, err)
		}
		report.persisted("card_recommendations")
	}
	return nil
}

// buildRecommendation converts a CCManager recommendation entry into a
// CardRecommendation, resolving the recommended wallet card by catalog id or
// card name (nil when not in the wallet).
func buildRecommendation(period, category string, rc ccRecCategory, catalog []CatalogCard, wallet map[string]string) *CardRecommendation {
	var userCardID *string
	// Prefer a direct catalog-id match (latest-report uses real ids); fall back
	// to resolving the card name (monthly files use name-slug ids).
	if id, ok := wallet[rc.CardID]; ok {
		userCardID = &id
	} else if catalogID, ok := resolveCatalogID(rc.CardName, catalog); ok {
		if id, inWallet := wallet[catalogID]; inWallet {
			userCardID = &id
		}
	}
	reason := strings.TrimSpace(rc.Source)
	if rc.Notes != "" {
		if reason == "" {
			reason = rc.Notes
		} else {
			reason = reason + ": " + rc.Notes
		}
	}
	if reason == "" {
		reason = "imported"
	}
	return &CardRecommendation{
		ID:                    uuid.NewString(),
		PeriodLabel:           period,
		Category:              category,
		RecommendedUserCardID: userCardID,
		Rate:                  rc.Rate,
		Reason:                reason,
		Starred:               rc.IsStarred,
		GeneratedAt:           time.Now().UTC(),
	}
}

// importRunHistory imports the mappable subset of CCManager run-history.json.
// CCManager-only run types (user_change, github_sync, …) and triggers are
// skipped+logged rather than violating the card_runs CHECK constraints.
func importRunHistory(ctx context.Context, store *Store, dataDir string, report *ImportReport) error {
	var hist ccRunHistory
	found, err := readJSONFile(dataDir, fileRunHistory, &hist)
	if err != nil {
		return err
	}
	if !found {
		report.skipFile(fileRunHistory)
		return nil
	}
	for i, r := range hist.Runs {
		run, ok, reason := buildHistoricalRun(r)
		if !ok {
			report.skipRow("card_runs", fmt.Sprintf("%s@%s", r.Type, r.Timestamp), reason)
			continue
		}
		inserted, err := store.InsertRunIfAbsent(ctx, run)
		if err != nil {
			return fmt.Errorf("import run %d: %w", i, err)
		}
		if inserted {
			report.persisted("card_runs")
		}
	}
	return nil
}

// buildHistoricalRun converts a CCManager run-history entry into a CardRun,
// returning ok=false (skip) for an unmappable run type or trigger.
func buildHistoricalRun(r ccRun) (*CardRun, bool, string) {
	runType, ok := mapRunType(r.Type)
	if !ok {
		return nil, false, fmt.Sprintf("run type %q not in card_runs domain", r.Type)
	}
	trigger, ok := mapRunTrigger(r.Trigger)
	if !ok {
		return nil, false, fmt.Sprintf("run trigger %q not in card_runs domain", r.Trigger)
	}
	var detail *string
	if msg := strings.TrimSpace(r.Message); msg != "" {
		detail = &msg
	}
	return &CardRun{
		ID:          uuid.NewString(),
		RunType:     runType,
		Trigger:     trigger,
		Status:      runStatusFromSuccess(r.Success),
		ErrorDetail: detail,
		StartedAt:   parseDate(r.Timestamp),
	}, true, ""
}

// resolveCatalogID resolves a free-text CCManager card reference to a catalog
// id using the Scope 02 resolver, accepting only matches at or above the
// import confidence floor.
func resolveCatalogID(name string, catalog []CatalogCard) (string, bool) {
	candidates := ResolveCard(name, catalog)
	if len(candidates) == 0 || candidates[0].Score < importResolveMinScore {
		return "", false
	}
	return candidates[0].CardID, true
}

// walletFor resolves a CCManager card key (a catalog id or a card name) to a
// wallet user_card id, trying a direct catalog-id hit first, then name
// resolution.
func walletFor(key string, catalog []CatalogCard, wallet map[string]string) (string, bool) {
	if id, ok := wallet[key]; ok {
		return id, true
	}
	if catalogID, ok := resolveCatalogID(key, catalog); ok {
		if id, inWallet := wallet[catalogID]; inWallet {
			return id, true
		}
	}
	return "", false
}

// sortedKeys returns the keys of a string-keyed map in sorted order, for
// deterministic iteration (stable counts and skip logs across runs).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
