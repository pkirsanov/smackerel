package cardrewards

// Spec 083 Scope 03 — unit tests for the CCManager → PostgreSQL import
// transforms (JSON→row mapping: card-type mapping, cents conversion, jsonb
// shaping, alias/equivalents flattening, lifecycle/run derivation, and
// boundary/missing-field handling). These run in the normal unit suite
// (./smackerel.sh test unit) — no live database required.

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMapCardType(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"rotating", CardTypeRotating, true},
		{"user-selected", CardTypeUserSelected, true},
		{"tiered", CardTypeUserSelected, true}, // tiered → user-selected
		{"fixed", CardTypeFixed, true},
		{"flat", CardTypeFixed, true}, // flat-rate → fixed
		{"store", CardTypeFixed, true},
		{"hotel", CardTypeFixed, true},
		{"airline", CardTypeFixed, true},
		{"  Rotating ", CardTypeRotating, true}, // trimmed + case-insensitive
		{"mystery", "", false},                  // unknown → skip
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := MapCardType(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("MapCardType(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestDollarsToCents(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{0, 0},
		{95, 9500},
		{1500, 150000},
		{19.99, 1999}, // rounding, no float drift
		{0.1, 10},
		{2.005, 201}, // half-up rounding (math.Round)
	}
	for _, c := range cases {
		if got := dollarsToCents(c.in); got != c.want {
			t.Errorf("dollarsToCents(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCentsPtr(t *testing.T) {
	if centsPtr(nil) != nil {
		t.Errorf("centsPtr(nil) should be nil")
	}
	v := 1500.0
	got := centsPtr(&v)
	if got == nil || *got != 150000 {
		t.Errorf("centsPtr(1500) = %v, want 150000", got)
	}
}

func TestParseDate(t *testing.T) {
	if parseDate("") != nil {
		t.Errorf("parseDate(\"\") should be nil")
	}
	if parseDate("not-a-date") != nil {
		t.Errorf("parseDate(invalid) should be nil")
	}
	d := parseDate("2026-01-01")
	if d == nil || d.Year() != 2026 || d.Month() != time.January || d.Day() != 1 {
		t.Errorf("parseDate(2026-01-01) = %v", d)
	}
	// naive microsecond timestamp (no zone), as in CCManager run-history.
	ts := parseDate("2026-03-02T18:21:06.493649")
	if ts == nil || ts.Year() != 2026 || ts.Hour() != 18 {
		t.Errorf("parseDate(naive ts) = %v", ts)
	}
}

func TestParsePeriodRange(t *testing.T) {
	start, end := parsePeriodRange("2026-01-01 to 2026-03-31")
	if start == nil || end == nil {
		t.Fatalf("parsePeriodRange returned nil; start=%v end=%v", start, end)
	}
	if start.Month() != time.January || end.Month() != time.March {
		t.Errorf("parsePeriodRange months = %v..%v", start.Month(), end.Month())
	}
	if s, e := parsePeriodRange("garbage"); s != nil || e != nil {
		t.Errorf("parsePeriodRange(garbage) should be nil,nil; got %v,%v", s, e)
	}
}

func TestDeriveLifecycle(t *testing.T) {
	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	past := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	// Explicit status wins.
	if s, ok := deriveLifecycle("active", nil, nil, now); !ok || s != LifecycleActive {
		t.Errorf("status active = (%q,%v)", s, ok)
	}
	if s, ok := deriveLifecycle("expired", nil, nil, now); !ok || s != LifecycleExpired {
		t.Errorf("status expired = (%q,%v)", s, ok)
	}
	// Derived from dates when status unknown.
	if s, ok := deriveLifecycle("", nil, &past, now); !ok || s != LifecycleExpired {
		t.Errorf("derived past end = (%q,%v), want expired", s, ok)
	}
	if s, ok := deriveLifecycle("", &future, nil, now); !ok || s != LifecycleUpcoming {
		t.Errorf("derived future start = (%q,%v), want upcoming", s, ok)
	}
	// Unknown status AND no dates → skip.
	if _, ok := deriveLifecycle("weird", nil, nil, now); ok {
		t.Errorf("unknown status with no dates should not derive a lifecycle")
	}
}

func TestMapRunTypeAndTrigger(t *testing.T) {
	if rt, ok := mapRunType("calendar_sync"); !ok || rt != RunTypeCalendarSync {
		t.Errorf("mapRunType(calendar_sync) = (%q,%v)", rt, ok)
	}
	if _, ok := mapRunType("user_change"); ok {
		t.Errorf("mapRunType(user_change) should be unmappable")
	}
	if _, ok := mapRunType("github_sync"); ok {
		t.Errorf("mapRunType(github_sync) should be unmappable")
	}
	if tr, ok := mapRunTrigger("auto"); !ok || tr != RunTriggerScheduled {
		t.Errorf("mapRunTrigger(auto) = (%q,%v), want scheduled", tr, ok)
	}
	if tr, ok := mapRunTrigger("ui"); !ok || tr != RunTriggerManual {
		t.Errorf("mapRunTrigger(ui) = (%q,%v), want manual", tr, ok)
	}
	if _, ok := mapRunTrigger("cosmic"); ok {
		t.Errorf("mapRunTrigger(cosmic) should be unmappable")
	}
	if runStatusFromSuccess(true) != RunStatusSuccess || runStatusFromSuccess(false) != RunStatusFailed {
		t.Errorf("runStatusFromSuccess mapping wrong")
	}
}

func TestNormalizeOfferRateType(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", RateTypePercent, true}, // spend-threshold promo → percent
		{"percent", RateTypePercent, true},
		{"points", RateTypePoints, true},
		{"miles", RateTypePoints, true}, // miles → points
		{"multiplier", RateTypeMultiplier, true},
		{"weird", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeOfferRateType(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("normalizeOfferRateType(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestQuarterAndMonthLabel(t *testing.T) {
	d := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	if got := quarterLabel(d); got != "Q1_2026" {
		t.Errorf("quarterLabel(Feb 2026) = %q, want Q1_2026", got)
	}
	if got := quarterLabel(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); got != "Q4_2026" {
		t.Errorf("quarterLabel(Nov 2026) = %q, want Q4_2026", got)
	}
	if got := monthLabel(d); got != "2026-02" {
		t.Errorf("monthLabel(Feb 2026) = %q, want 2026-02", got)
	}
}

func TestBuildCatalogCard_TypeMappingAndJSONBPlacement(t *testing.T) {
	// Tiered card: tiered_benefits must land in SelectableBenefits, type → user-selected.
	tiered := ccCatalogCard{
		ID: "us-bank-cash-plus", Name: "US Bank Cash+", Issuer: "US Bank", Type: "tiered",
		AnnualFee:      0,
		BaseBenefits:   json.RawMessage(`[{"category":"Everything","rate":1}]`),
		TieredBenefits: json.RawMessage(`{"tiers":[{"name":"5%"}]}`),
		Perks:          json.RawMessage(`["No annual fee"]`),
		Aliases:        []string{"us bank cash+"},
	}
	card, ok := buildCatalogCard(tiered)
	if !ok {
		t.Fatalf("buildCatalogCard(tiered) ok=false")
	}
	if card.CardType != CardTypeUserSelected {
		t.Errorf("tiered card type = %q, want user-selected", card.CardType)
	}
	if string(card.SelectableBenefits) != `{"tiers":[{"name":"5%"}]}` {
		t.Errorf("tiered_benefits not placed in SelectableBenefits: %s", card.SelectableBenefits)
	}
	if card.Source != SourceSeed {
		t.Errorf("seed catalog source = %q, want seed", card.Source)
	}

	// Rotating card: rotating_benefits preserved; annual fee → cents.
	rotating := ccCatalogCard{
		ID: "discover-it", Name: "Discover it", Issuer: "Discover", Type: "rotating",
		AnnualFee:        95,
		RotatingBenefits: json.RawMessage(`{"limit":1500}`),
	}
	rc, ok := buildCatalogCard(rotating)
	if !ok {
		t.Fatalf("buildCatalogCard(rotating) ok=false")
	}
	if rc.CardType != CardTypeRotating {
		t.Errorf("rotating type = %q", rc.CardType)
	}
	if rc.AnnualFeeCents != 9500 {
		t.Errorf("annual fee cents = %d, want 9500", rc.AnnualFeeCents)
	}
	if string(rc.RotatingBenefits) != `{"limit":1500}` {
		t.Errorf("rotating_benefits not preserved: %s", rc.RotatingBenefits)
	}

	// Unknown type → skip.
	if _, ok := buildCatalogCard(ccCatalogCard{ID: "x", Type: "mystery"}); ok {
		t.Errorf("unknown card type should not build")
	}
}

func TestBuildCategoryAliases_Flattening(t *testing.T) {
	cats := ccConfigCategories{
		Starred:  []string{"Groceries", "Gas Stations", "Dining"},
		Priority: []string{"Groceries", "Gas Stations", "Dining", "Amazon"},
		BuiltIn:  []string{"Dining", "Restaurants", "Gas Stations", "Groceries", "Travel", "Amazon", "Streaming"},
		Equivalents: ccConfigEquivalents{
			"dining": {"restaurants", "food"},
			"gas":    {"fuel"},
		},
	}
	aliases := buildCategoryAliases(cats)
	byName := map[string]*CategoryAlias{}
	for _, a := range aliases {
		byName[a.CanonicalCategory] = a
	}

	// 7 built-in names + 1 equivalents-only key ("gas") = 8 distinct.
	if len(aliases) != 8 {
		t.Fatalf("expected 8 aliases, got %d: %+v", len(aliases), byName)
	}
	// Dining: starred, priority index 2, built_in, equivalents from config.
	dining := byName["Dining"]
	if dining == nil || !dining.Starred || !dining.BuiltIn {
		t.Fatalf("Dining alias missing starred/built_in: %+v", dining)
	}
	if dining.Priority == nil || *dining.Priority != 2 {
		t.Errorf("Dining priority = %v, want 2", dining.Priority)
	}
	if len(dining.Equivalents) != 2 {
		t.Errorf("Dining equivalents = %v, want [restaurants food]", dining.Equivalents)
	}
	// Groceries: starred, priority 0, built_in, no equivalents.
	groc := byName["Groceries"]
	if groc == nil || !groc.Starred || groc.Priority == nil || *groc.Priority != 0 {
		t.Errorf("Groceries alias wrong: %+v", groc)
	}
	// "gas" is equivalents-only: not starred, not built_in, no priority.
	gas := byName["gas"]
	if gas == nil || gas.Starred || gas.BuiltIn || gas.Priority != nil {
		t.Errorf("gas alias should be equivalents-only: %+v", gas)
	}
	if len(gas.Equivalents) != 1 || gas.Equivalents[0] != "fuel" {
		t.Errorf("gas equivalents = %v, want [fuel]", gas.Equivalents)
	}
	// Restaurants is built_in only.
	rest := byName["Restaurants"]
	if rest == nil || !rest.BuiltIn || rest.Starred || rest.Priority != nil {
		t.Errorf("Restaurants should be built_in-only: %+v", rest)
	}
}

func TestBuildRotatingCategory(t *testing.T) {
	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	limit := 1500.0
	q := ccQuarter{
		Categories:         []string{"Grocery Stores", "Wholesale Clubs", "Streaming"},
		Period:             "2026-01-01 to 2026-03-31",
		Limit:              &limit,
		Status:             "active",
		ActivationRequired: true,
	}
	rc, ok, reason := buildRotatingCategory("discover-it", "Q1_2026", q, now)
	if !ok {
		t.Fatalf("buildRotatingCategory ok=false reason=%s", reason)
	}
	if !rc.ManualOverride {
		t.Errorf("imported rotating category must be ManualOverride=true (SCN-083-C03)")
	}
	if rc.Confidence != 1.0 {
		t.Errorf("imported confidence = %v, want 1.0", rc.Confidence)
	}
	if rc.LifecycleState != LifecycleActive {
		t.Errorf("lifecycle = %q, want active", rc.LifecycleState)
	}
	if rc.LimitCents == nil || *rc.LimitCents != 150000 {
		t.Errorf("limit cents = %v, want 150000", rc.LimitCents)
	}
	if len(rc.Categories) != 3 || rc.Categories[0] != "Grocery Stores" {
		t.Errorf("categories = %v", rc.Categories)
	}
	if rc.PeriodStart == nil || rc.PeriodEnd == nil {
		t.Errorf("period dates not parsed: start=%v end=%v", rc.PeriodStart, rc.PeriodEnd)
	}

	// No categories → skip.
	if _, ok, _ := buildRotatingCategory("x", "Q1_2026", ccQuarter{Period: "2026-01-01 to 2026-03-31"}, now); ok {
		t.Errorf("quarter with no categories should be skipped")
	}
}

func TestBuildSignupBonuses(t *testing.T) {
	spend := 1500.0
	sb := ccSignupBonuses{
		SpendBonus:     &ccSpendBonus{Amount: 200, SpendRequired: spend, Deadline: "2026-03-26", Completed: true, Notes: "spend $1500"},
		FirstYearBonus: &ccFirstYearBonus{Category: "selected", Rate: 6, NormalRate: 3, EndDate: "2026-12-26", Notes: "6% first year"},
	}
	out := buildSignupBonuses("uc-1", sb)
	if len(out) != 2 {
		t.Fatalf("expected 2 bonuses, got %d", len(out))
	}
	var spendBonus, fyBonus *SignupBonus
	for _, b := range out {
		switch b.BonusType {
		case BonusTypeSpend:
			spendBonus = b
		case BonusTypeFirstYearRate:
			fyBonus = b
		}
	}
	if spendBonus == nil || spendBonus.SpendRequiredCents == nil || *spendBonus.SpendRequiredCents != 150000 {
		t.Errorf("spend bonus cents wrong: %+v", spendBonus)
	}
	if !spendBonus.Met {
		t.Errorf("spend bonus completed=true should map to Met=true")
	}
	if spendBonus.Deadline == nil {
		t.Errorf("spend bonus deadline not parsed")
	}
	if fyBonus == nil || fyBonus.RewardDescription == nil {
		t.Errorf("first-year bonus reward missing: %+v", fyBonus)
	}
}

func TestBuildOffers_MultiCategoryAndSharedLimit(t *testing.T) {
	catalog := []CatalogCard{{ID: "amazon-prime-visa", Name: "Amazon Prime Rewards Visa Signature", Aliases: []string{"amazon prime"}}}
	wallet := map[string]string{"amazon-prime-visa": "uc-amazon"}
	rate := 5.0
	limit := 1000.0
	o := ccOffer{
		ID: "combo", Card: "amazon prime",
		Categories: []string{"Groceries", "Restaurants", "Gas Stations"},
		Rate:       &rate, RateType: "percent", Limit: &limit, LimitPeriod: "total", LimitShared: true,
		StartDate: "2025-12-01", EndDate: "2025-12-31", ActivationRequired: true, Activated: true,
		Notes: "December promo",
	}
	rows, ok, reason := buildOffers(o, catalog, wallet)
	if !ok {
		t.Fatalf("buildOffers ok=false reason=%s", reason)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 offer rows (one per category), got %d", len(rows))
	}
	for _, r := range rows {
		if r.UserCardID == nil || *r.UserCardID != "uc-amazon" {
			t.Errorf("offer not linked to wallet card: %+v", r)
		}
		if r.SharedLimitGroup == nil || *r.SharedLimitGroup != "combo" {
			t.Errorf("shared-limit offer should carry shared group: %+v", r.SharedLimitGroup)
		}
		if r.LimitCents == nil || *r.LimitCents != 100000 {
			t.Errorf("limit cents = %v, want 100000", r.LimitCents)
		}
		if r.Title != "December promo" {
			t.Errorf("title = %q, want notes-derived", r.Title)
		}
	}

	// spend_threshold promo → rate 0, percent, enriched notes.
	spend, reward := 400.0, 50.0
	st := ccOffer{
		ID: "thresh", Card: "amazon prime", Categories: []string{"Gas Stations"},
		PromoType: "spend_threshold", SpendRequired: &spend, RewardAmount: &reward, RewardType: "statement_credit",
	}
	stRows, ok, _ := buildOffers(st, catalog, wallet)
	if !ok || len(stRows) != 1 {
		t.Fatalf("spend_threshold offer build failed: ok=%v rows=%d", ok, len(stRows))
	}
	if stRows[0].Rate != 0 || stRows[0].RateType != RateTypePercent {
		t.Errorf("spend_threshold rate/type = %v/%q, want 0/percent", stRows[0].Rate, stRows[0].RateType)
	}
	if stRows[0].Notes == nil || stRows[0].UserCardID == nil {
		t.Errorf("spend_threshold notes/link missing: %+v", stRows[0])
	}

	// No categories → skip.
	if _, ok, _ := buildOffers(ccOffer{ID: "empty", Card: "amazon prime"}, catalog, wallet); ok {
		t.Errorf("offer with no categories should be skipped")
	}

	// Unresolvable card → general offer (nil user_card), still imported.
	gen := ccOffer{ID: "gen", Card: "no such card", Categories: []string{"Travel"}, RateType: "percent"}
	genRows, ok, _ := buildOffers(gen, catalog, wallet)
	if !ok || len(genRows) != 1 || genRows[0].UserCardID != nil {
		t.Errorf("unresolved offer should import with nil user_card: ok=%v rows=%d", ok, len(genRows))
	}
}

func TestBuildHistoricalRun(t *testing.T) {
	run, ok, _ := buildHistoricalRun(ccRun{Timestamp: "2026-03-02T18:21:06.493649", Type: "calendar_sync", Trigger: "manual", Success: true, Message: "ok"})
	if !ok {
		t.Fatalf("calendar_sync run should map")
	}
	if run.RunType != RunTypeCalendarSync || run.Trigger != RunTriggerManual || run.Status != RunStatusSuccess {
		t.Errorf("run mapping wrong: %+v", run)
	}
	if run.StartedAt == nil {
		t.Errorf("run started_at not parsed")
	}
	// Unmappable type → skip.
	if _, ok, _ := buildHistoricalRun(ccRun{Type: "user_change", Trigger: "ui"}); ok {
		t.Errorf("user_change run should be skipped")
	}
}

func TestResolveCatalogID_ConfidenceFloor(t *testing.T) {
	catalog := []CatalogCard{
		{ID: "discover-it", Name: "Discover it Cash Back", Aliases: []string{"discover", "discover it"}},
		{ID: "amazon-prime-visa", Name: "Amazon Prime Rewards Visa Signature", Aliases: []string{"amazon prime"}},
	}
	if id, ok := resolveCatalogID("discover", catalog); !ok || id != "discover-it" {
		t.Errorf("resolveCatalogID(discover) = (%q,%v), want discover-it", id, ok)
	}
	if _, ok := resolveCatalogID("totally unrelated xyzzy", catalog); ok {
		t.Errorf("unrelated text should not resolve above the confidence floor")
	}
}
