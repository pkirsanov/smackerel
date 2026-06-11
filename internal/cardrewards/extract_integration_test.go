//go:build integration

// Spec 083 Card Rewards Companion (Scope 05) — live-PostgreSQL persistence tests
// for the extraction orchestrator. These prove the design §4 failure-mode
// contract end-to-end against a REAL ephemeral Postgres (no mocks of internal
// components — the Store and DB are real). Only the EXTERNAL model-gateway
// boundary (SidecarExtractor) is faked, with deterministic canned responses, so
// the adversarial scenarios are reproducible without a live model backend. The
// true sidecar→model round-trip is covered separately by the opt-in
// live-stack test tests/integration/cardrewards_extract_test.go (SCN-083-E08).
//
// Run via: ./smackerel.sh test integration --go-run CardRewardsExtract
// The runner sets DATABASE_URL to the disposable test Postgres and adds
// ./internal/cardrewards/... to the integration package list.

package cardrewards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// stubSidecar is a deterministic fake of the EXTERNAL model-gateway boundary
// (allowed under the no-internal-mocks policy — the model gateway is an out-of-
// process dependency). It returns a canned raw body or error per requested card.
type stubSidecar struct {
	byCard map[string]stubResp
}

type stubResp struct {
	raw string
	err error
}

func (s *stubSidecar) Extract(_ context.Context, req ExtractRequest) (json.RawMessage, error) {
	r, ok := s.byCard[req.CardID]
	if !ok {
		return nil, fmt.Errorf("stub: no canned response for card_id %q", req.CardID)
	}
	if r.err != nil {
		return nil, r.err
	}
	return json.RawMessage(r.raw), nil
}

func validRespFor(cardID, period string, confidence float64) string {
	return fmt.Sprintf(`{
  "card_id": %q,
  "period_label": %q,
  "period_start": "2026-07-01",
  "period_end": "2026-09-30",
  "categories": ["Restaurants", "PayPal"],
  "spend_limit": 1500,
  "activation_required": true,
  "confidence": %g,
  "source_evidence": "Q3 2026: Restaurants and PayPal; activate by Sept 30."
}`, cardID, period, confidence)
}

func inputFor(cardID, period string) ExtractInput {
	return ExtractInput{
		CardID:      cardID,
		IssuerHint:  "Discover",
		PeriodLabel: period,
		SourceName:  "Doctor of Credit",
		SourceURL:   "https://example.test/" + cardID + "-" + period,
		PageText:    "5% categories for " + period + " are Restaurants and PayPal.",
	}
}

// SCN-083-E01 + E07 (live PG) — a valid extraction persists exactly one
// observation row with full provenance, and an extract audit run.
func TestExtractorLivePG_StoresObservationWithProvenance_E01_E07(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	period := "Q3_2026"

	stub := &stubSidecar{byCard: map[string]stubResp{cardID: {raw: validRespFor(cardID, period, 0.92)}}}
	ex := NewExtractor(s, stub, 0.70, nil)

	res, err := ex.Run(ctx, []ExtractInput{inputFor(cardID, period)}, RunTriggerManual)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stored != 1 || res.Discarded != 0 || res.Skipped != 0 {
		t.Fatalf("counts: stored=%d discarded=%d skipped=%d, want 1/0/0", res.Stored, res.Discarded, res.Skipped)
	}
	if res.Run.Status != RunStatusSuccess {
		t.Errorf("run status = %q, want success", res.Run.Status)
	}
	if res.Run.SourcesAttempted != 1 || res.Run.SourcesSucceeded != 1 || res.Run.CategoriesExtracted != 2 {
		t.Errorf("run counts attempted/succeeded/categories = %d/%d/%d, want 1/1/2",
			res.Run.SourcesAttempted, res.Run.SourcesSucceeded, res.Run.CategoriesExtracted)
	}

	obs, err := s.ListObservationsByCardPeriod(ctx, cardID, period)
	if err != nil {
		t.Fatalf("ListObservationsByCardPeriod: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected exactly 1 observation persisted, got %d", len(obs))
	}
	o := obs[0]
	if !equalStrings(o.Categories, []string{"Restaurants", "PayPal"}) {
		t.Errorf("categories = %v, want [Restaurants PayPal]", o.Categories)
	}
	if o.LimitCents == nil || *o.LimitCents != 150000 {
		t.Errorf("limit_cents = %v, want 150000", o.LimitCents)
	}
	if o.SourceName != "Doctor of Credit" || o.SourceURL == "" || o.SourceEvidence == nil {
		t.Errorf("E07: provenance not fully persisted: name=%q url=%q evidence=%v", o.SourceName, o.SourceURL, o.SourceEvidence)
	}
	if o.ExtractionRunID != res.Run.ID {
		t.Errorf("observation extraction_run_id = %q, want run id %q", o.ExtractionRunID, res.Run.ID)
	}

	assertRunPersisted(t, ctx, s, res.Run.ID, RunTypeExtract, RunStatusSuccess, 1, 1, 2)
}

// SCN-083-E02 + E03 (live PG, ADVERSARIAL) — a malformed extraction stores
// NOTHING and the pre-existing reconciled record is preserved (categories,
// confidence, manual_override all unchanged) and only flagged needs_verification.
// This is the exact CCManager silent-fallback failure mode, proven fixed against
// a real database: the old scraper overwrote the record with a stale/placeholder
// value; here it is never touched except for the verification flag.
func TestExtractorLivePG_MalformedDiscardedNoOverwrite_E02_E03(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	period := "Q3_2026"

	// Seed an authoritative, manually-verified existing record.
	existingID := uuid.NewString()
	if err := s.UpsertRotatingCategory(ctx, &RotatingCategory{
		ID:                existingID,
		CardCatalogID:     cardID,
		PeriodLabel:       period,
		Categories:        []string{"Grocery Stores", "Streaming"},
		LifecycleState:    LifecycleActive,
		Confidence:        1.0,
		NeedsVerification: false,
		ManualOverride:    true,
		SourceCount:       1,
	}); err != nil {
		t.Fatalf("seed existing rotating_categories: %v", err)
	}

	// The sidecar returns garbage (the regex-fallback would have stored a
	// placeholder here).
	stub := &stubSidecar{byCard: map[string]stubResp{cardID: {raw: "Discover 5% — check the website"}}}
	ex := NewExtractor(s, stub, 0.70, nil)

	res, err := ex.Run(ctx, []ExtractInput{inputFor(cardID, period)}, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stored != 0 || res.Discarded != 1 {
		t.Fatalf("counts: stored=%d discarded=%d, want 0/1", res.Stored, res.Discarded)
	}
	if res.Run.Status != RunStatusPartial {
		t.Errorf("run status = %q, want partial", res.Run.Status)
	}
	if res.Flagged != 1 {
		t.Errorf("expected exactly 1 existing record flagged, got %d", res.Flagged)
	}

	// NOTHING stored.
	obs, err := s.ListObservationsByCardPeriod(ctx, cardID, period)
	if err != nil {
		t.Fatalf("ListObservationsByCardPeriod: %v", err)
	}
	if len(obs) != 0 {
		t.Fatalf("malformed extraction must persist 0 observations, got %d", len(obs))
	}

	// Existing record PRESERVED + flagged (NOT overwritten).
	rc, err := s.GetRotatingCategory(ctx, cardID, period)
	if err != nil || rc == nil {
		t.Fatalf("GetRotatingCategory: %v (rc=%v)", err, rc)
	}
	if !equalStrings(rc.Categories, []string{"Grocery Stores", "Streaming"}) {
		t.Errorf("E03: existing categories were overwritten to %v — MUST be preserved", rc.Categories)
	}
	if rc.Confidence != 1.0 {
		t.Errorf("E03: existing confidence changed to %v — MUST be preserved", rc.Confidence)
	}
	if !rc.ManualOverride {
		t.Errorf("E03: existing manual_override cleared — MUST be preserved")
	}
	if !rc.NeedsVerification {
		t.Errorf("E03: existing record MUST be flagged needs_verification after a failed extraction")
	}
}

// SCN-083-E04 (live PG) — a low-confidence valid record is stored and reported
// as below-threshold (the reconciler flags it in Scope 06).
func TestExtractorLivePG_LowConfidenceStored_E04(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	period := "Q3_2026"

	stub := &stubSidecar{byCard: map[string]stubResp{cardID: {raw: validRespFor(cardID, period, 0.40)}}}
	ex := NewExtractor(s, stub, 0.70, nil)

	res, err := ex.Run(ctx, []ExtractInput{inputFor(cardID, period)}, RunTriggerManual)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stored != 1 || res.LowConfidence != 1 {
		t.Fatalf("counts: stored=%d lowConfidence=%d, want 1/1", res.Stored, res.LowConfidence)
	}
	obs, err := s.ListObservationsByCardPeriod(ctx, cardID, period)
	if err != nil {
		t.Fatalf("ListObservationsByCardPeriod: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected the low-confidence observation to be stored, got %d", len(obs))
	}
	if obs[0].Confidence < 0.39 || obs[0].Confidence > 0.41 {
		t.Errorf("stored confidence = %v, want ~0.40", obs[0].Confidence)
	}
}

// SCN-083-E05 (live PG, ADVERSARIAL) — an observation naming an unknown card is
// skipped and NEVER mismapped onto a real card. Here a known card is also in the
// catalog; the run must not attach the unknown observation to it.
func TestExtractorLivePG_UnknownCardSkippedNoMismap_E05(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	knownID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	unknownID := prefix + "-ghost-card" // deliberately NOT seeded
	period := "Q3_2026"

	stub := &stubSidecar{byCard: map[string]stubResp{
		unknownID: {raw: validRespFor(unknownID, period, 0.95)},
	}}
	ex := NewExtractor(s, stub, 0.70, nil)

	res, err := ex.Run(ctx, []ExtractInput{inputFor(unknownID, period)}, RunTriggerManual)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Skipped != 1 || res.Stored != 0 {
		t.Fatalf("counts: skipped=%d stored=%d, want 1/0", res.Skipped, res.Stored)
	}
	if res.Run.Status != RunStatusPartial {
		t.Errorf("run status = %q, want partial", res.Run.Status)
	}
	// No observation for the unknown card …
	ghost, err := s.ListObservationsByCardPeriod(ctx, unknownID, period)
	if err != nil {
		t.Fatalf("list ghost obs: %v", err)
	}
	if len(ghost) != 0 {
		t.Fatalf("unknown card must persist 0 observations, got %d", len(ghost))
	}
	// … and crucially none mismapped onto the known card.
	mismapped, err := s.ListObservationsByCardPeriod(ctx, knownID, period)
	if err != nil {
		t.Fatalf("list known obs: %v", err)
	}
	if len(mismapped) != 0 {
		t.Fatalf("E05: unknown observation was mismapped onto the known card (%d rows)", len(mismapped))
	}
}

// SCN-083-E08 (live PG, audit portion) — every extraction batch records exactly
// one card_runs row with run_type=extract and the attempt/success/category
// counts. The true sidecar→model round-trip is exercised by the opt-in
// live-stack test; this proves the audit-run persistence deterministically.
func TestExtractorLivePG_ExtractionRunAudited_E08(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardA := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	cardB := seedCatalogCard(t, ctx, s, prefix, "chase-freedom", CardTypeRotating)
	period := "Q3_2026"

	before, err := s.CountRunsByType(ctx, RunTypeExtract)
	if err != nil {
		t.Fatalf("CountRunsByType before: %v", err)
	}

	stub := &stubSidecar{byCard: map[string]stubResp{
		cardA: {raw: validRespFor(cardA, period, 0.95)},
		cardB: {err: errors.New("source page unreachable")},
	}}
	ex := NewExtractor(s, stub, 0.70, nil)

	res, err := ex.Run(ctx, []ExtractInput{inputFor(cardA, period), inputFor(cardB, period)}, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	after, err := s.CountRunsByType(ctx, RunTypeExtract)
	if err != nil {
		t.Fatalf("CountRunsByType after: %v", err)
	}
	if after != before+1 {
		t.Fatalf("expected exactly 1 new extract run, before=%d after=%d", before, after)
	}
	// Mixed batch (one stored, one sidecar error) → partial.
	assertRunPersisted(t, ctx, s, res.Run.ID, RunTypeExtract, RunStatusPartial, 2, 1, 2)
}

func assertRunPersisted(t *testing.T, ctx context.Context, s *Store, runID, wantType, wantStatus string, wantAttempted, wantSucceeded, wantCategories int) {
	t.Helper()
	var (
		runType, trigger, status            string
		attempted, succeeded, categoriesExt int
	)
	err := s.Pool.QueryRow(ctx,
		`SELECT run_type, trigger, status, sources_attempted, sources_succeeded, categories_extracted
		 FROM card_runs WHERE id = $1`, runID,
	).Scan(&runType, &trigger, &status, &attempted, &succeeded, &categoriesExt)
	if err != nil {
		t.Fatalf("query card_runs %s: %v", runID, err)
	}
	if runType != wantType {
		t.Errorf("run_type = %q, want %q", runType, wantType)
	}
	if status != wantStatus {
		t.Errorf("status = %q, want %q", status, wantStatus)
	}
	if attempted != wantAttempted || succeeded != wantSucceeded || categoriesExt != wantCategories {
		t.Errorf("counts attempted/succeeded/categories = %d/%d/%d, want %d/%d/%d",
			attempted, succeeded, categoriesExt, wantAttempted, wantSucceeded, wantCategories)
	}
}
