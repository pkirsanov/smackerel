package cardrewards

// Spec 083 Card Rewards Companion (Scope 06) — T-06-01 / T-06-02 / T-06-03.
// Unit tests for the PURE reconciliation + lifecycle decisions (mergeObservations
// and the date-driven deriveLifecycle path). No database, no mocks — every
// adversarial scenario SCN-083-F01..F05 is decided by a function of its inputs.
//
// F02 and F03 are ADVERSARIAL: each feeds input that a naive merge (take the
// union / take the newest / overwrite the record) would accept, and asserts the
// reconciler instead flags needs_verification (F02) or refuses to touch the
// manual override (F03). They FAIL if that protection is removed — they are not
// tautological.

import (
	"testing"
	"time"
)

// ---- unit test helpers (also reused by the integration test file) ----------

func dateUTC(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
func ptrTime(t time.Time) *time.Time { return &t }
func ptrInt(n int) *int              { return &n }
func ptrBool(b bool) *bool           { return &b }

func obsFixture(card, period string, cats []string, conf float64, start, end *time.Time, source string) RotatingCategoryObservation {
	return RotatingCategoryObservation{
		ID:             "obs-" + source,
		CardCatalogID:  card,
		PeriodLabel:    period,
		PeriodStart:    start,
		PeriodEnd:      end,
		Categories:     cats,
		Confidence:     conf,
		SourceName:     source,
		SourceURL:      "https://example.test/" + source,
		SourceEvidence: ptrStr(source + " evidence"),
	}
}

func ptrStr(s string) *string { return &s }

// SCN-083-F01 — agreeing sources reconcile to a high-confidence record with
// source_count = number of sources and needs_verification = false.
func TestReconcile_MergeAgreement_F01(t *testing.T) {
	period := "Q3_2026"
	start, end := dateUTC(2026, 7, 1), dateUTC(2026, 9, 30)
	now := dateUTC(2026, 8, 15) // inside the period → active

	obs := []RotatingCategoryObservation{
		obsFixture("discover-it", period, []string{"PayPal", "Restaurants"}, 0.90, &start, &end, "DoctorOfCredit"),
		// same set, different order + casing → still agreement
		obsFixture("discover-it", period, []string{"restaurants", "paypal"}, 0.85, &start, &end, "USCreditCardGuide"),
	}

	out := mergeObservations(nil, obs, 0.70, now)
	if out.Record == nil {
		t.Fatal("F01: expected a reconciled record, got nil")
	}
	if out.OverrideProtected {
		t.Fatal("F01: no existing override, OverrideProtected must be false")
	}
	if out.Disagreement {
		t.Fatal("F01: agreeing sources must NOT be marked as disagreement")
	}
	if !equalStrings(out.Record.Categories, []string{"PayPal", "Restaurants"}) {
		t.Fatalf("F01: categories = %v, want [PayPal Restaurants] (sorted agreed set)", out.Record.Categories)
	}
	if out.Record.SourceCount != 2 {
		t.Errorf("F01: source_count = %d, want 2 (both sources agreed)", out.Record.SourceCount)
	}
	if out.Record.NeedsVerification {
		t.Error("F01: agreeing high-confidence sources must NOT need verification")
	}
	if out.Record.Confidence != 0.90 {
		t.Errorf("F01: confidence = %v, want 0.90 (max of agreeing sources)", out.Record.Confidence)
	}
	if out.Record.LifecycleState != LifecycleActive {
		t.Errorf("F01: lifecycle = %q, want active (now inside period)", out.Record.LifecycleState)
	}
	if out.Record.ManualOverride {
		t.Error("F01: a freshly reconciled record must not claim manual_override")
	}
}

// SCN-083-F02 (ADVERSARIAL) — disagreeing sources MUST flag needs_verification
// with the conservative (lower) confidence; both observations are retained
// (the merge does not fabricate agreement nor discard a source).
func TestReconcile_MergeDisagreement_F02(t *testing.T) {
	period := "Q3_2026"
	start, end := dateUTC(2026, 7, 1), dateUTC(2026, 9, 30)
	now := dateUTC(2026, 8, 15)

	obs := []RotatingCategoryObservation{
		obsFixture("discover-it", period, []string{"Restaurants"}, 0.92, &start, &end, "SourceA"),
		obsFixture("discover-it", period, []string{"Groceries"}, 0.88, &start, &end, "SourceB"),
	}

	out := mergeObservations(nil, obs, 0.70, now)
	if out.Record == nil {
		t.Fatal("F02: expected a record, got nil")
	}
	if !out.Disagreement {
		t.Fatal("F02 REGRESSION: two distinct category sets must be detected as disagreement")
	}
	if !out.Record.NeedsVerification {
		t.Fatal("F02 REGRESSION: disagreeing sources MUST flag needs_verification=true (no silent pick)")
	}
	if out.Record.Confidence != 0.88 {
		t.Errorf("F02: confidence = %v, want 0.88 (conservative lower confidence, UC-002 A3)", out.Record.Confidence)
	}
	if out.Record.SourceCount != 1 {
		t.Errorf("F02: source_count = %d, want 1 (only the plurality set's sources)", out.Record.SourceCount)
	}
	// Both raw observations are still the caller's to persist; the merge must not
	// have collapsed them into a fake 2-source agreement.
	if out.Record.SourceCount == len(obs) && !out.Record.NeedsVerification {
		t.Fatal("F02 REGRESSION: disagreement was silently reconciled as full agreement")
	}
}

// SCN-083-F03 (ADVERSARIAL) — a manual override is NEVER overwritten by a new
// extraction; the record's categories/confidence/flags are untouched and the
// observation is recorded for audit only (FR-CR-011 / UC-002 A5).
func TestReconcile_ManualOverrideNeverOverwritten_F03(t *testing.T) {
	existing := &RotatingCategory{
		ID:                "rc-override-1",
		CardCatalogID:     "discover-it",
		PeriodLabel:       "Q3_2026",
		Categories:        []string{"Gym Memberships"},
		Confidence:        1.0,
		NeedsVerification: false,
		ManualOverride:    true,
		SourceCount:       1,
		LifecycleState:    LifecycleActive,
	}
	// A new, HIGH-confidence observation that DISAGREES with the override. A
	// naive merge would replace the categories — this must not happen.
	obs := []RotatingCategoryObservation{
		obsFixture("discover-it", "Q3_2026", []string{"Restaurants", "PayPal"}, 0.99, nil, nil, "AggressiveSource"),
	}

	out := mergeObservations(existing, obs, 0.70, time.Now().UTC())
	if !out.OverrideProtected {
		t.Fatal("F03 REGRESSION: manual override must be protected (OverrideProtected=true)")
	}
	if out.Record == nil {
		t.Fatal("F03: expected the existing record back, got nil")
	}
	if !equalStrings(out.Record.Categories, []string{"Gym Memberships"}) {
		t.Fatalf("F03 REGRESSION: manual override categories were overwritten to %v", out.Record.Categories)
	}
	if !out.Record.ManualOverride {
		t.Error("F03: manual_override must remain true")
	}
	if out.Record.Confidence != 1.0 {
		t.Errorf("F03: override confidence must be unchanged, got %v want 1.0", out.Record.Confidence)
	}
	if out.Record.NeedsVerification {
		t.Error("F03: an extraction must not flip a manual override to needs_verification")
	}
}

// SCN-083-F04 + F05 — lifecycle_state is derived from the period window vs now:
// upcoming → active → expired. Uses the date-driven path of the shared
// deriveLifecycle (empty status forces date derivation).
func TestReconcile_LifecycleByDate_F04_F05(t *testing.T) {
	now := dateUTC(2026, 8, 15)
	cases := []struct {
		name      string
		start     *time.Time
		end       *time.Time
		want      string
		wantKnown bool
	}{
		{"upcoming: start in future", ptrTime(dateUTC(2026, 10, 1)), ptrTime(dateUTC(2026, 12, 31)), LifecycleUpcoming, true},
		{"F04 active: start past, end future", ptrTime(dateUTC(2026, 7, 1)), ptrTime(dateUTC(2026, 9, 30)), LifecycleActive, true},
		{"F05 expired: end in past", ptrTime(dateUTC(2026, 4, 1)), ptrTime(dateUTC(2026, 6, 30)), LifecycleExpired, true},
		{"active boundary: end == today", ptrTime(dateUTC(2026, 7, 1)), ptrTime(now), LifecycleActive, true},
		{"active boundary: start == today", ptrTime(now), ptrTime(dateUTC(2026, 9, 30)), LifecycleActive, true},
		{"undated: cannot determine", nil, nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := deriveLifecycle("", c.start, c.end, now)
			if ok != c.wantKnown {
				t.Fatalf("ok = %v, want %v", ok, c.wantKnown)
			}
			if got != c.want {
				t.Fatalf("lifecycle = %q, want %q", got, c.want)
			}
		})
	}

	// F05 end-to-end through the merge: a current observation whose period has
	// already ended reconciles to an EXPIRED record (excluded from current recs).
	expiredObs := []RotatingCategoryObservation{
		obsFixture("discover-it", "Q2_2026", []string{"Gas Stations"}, 0.95,
			ptrTime(dateUTC(2026, 4, 1)), ptrTime(dateUTC(2026, 6, 30)), "SourceA"),
	}
	out := mergeObservations(nil, expiredObs, 0.70, now)
	if out.Record == nil || out.Record.LifecycleState != LifecycleExpired {
		t.Fatalf("F05: merged past-period record lifecycle = %v, want expired", out.Record)
	}
}

// UC-002 A2 — a single, agreeing-but-LOW-confidence observation is still stored
// (one source = agreement) yet flagged needs_verification because the aggregate
// confidence is below threshold.
func TestReconcile_LowConfidenceFlags_A2(t *testing.T) {
	start, end := dateUTC(2026, 7, 1), dateUTC(2026, 9, 30)
	now := dateUTC(2026, 8, 15)
	obs := []RotatingCategoryObservation{
		obsFixture("discover-it", "Q3_2026", []string{"Restaurants"}, 0.40, &start, &end, "SourceA"),
		obsFixture("discover-it", "Q3_2026", []string{"Restaurants"}, 0.35, &start, &end, "SourceB"),
	}
	out := mergeObservations(nil, obs, 0.70, now)
	if out.Disagreement {
		t.Fatal("A2: identical sets must agree")
	}
	if !out.Record.NeedsVerification {
		t.Fatal("A2: aggregate confidence below threshold must flag needs_verification")
	}
	if out.Record.SourceCount != 2 {
		t.Errorf("A2: source_count = %d, want 2", out.Record.SourceCount)
	}
}

// Edge — with no new observations the existing record is returned untouched
// (the reconciler never invents data).
func TestReconcile_NoObservationsKeepsExisting(t *testing.T) {
	existing := &RotatingCategory{
		ID: "rc-1", CardCatalogID: "discover-it", PeriodLabel: "Q3_2026",
		Categories: []string{"Travel"}, Confidence: 0.8, LifecycleState: LifecycleActive,
	}
	out := mergeObservations(existing, nil, 0.70, time.Now().UTC())
	if out.Record != existing {
		t.Fatal("expected the existing record returned unchanged when there are no observations")
	}
	if out.OverrideProtected || out.Disagreement {
		t.Fatal("no-observation path must not flag override/disagreement")
	}
}
