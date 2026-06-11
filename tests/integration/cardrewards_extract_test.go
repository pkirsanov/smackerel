//go:build integration

// Spec 083 Card Rewards Companion (Scope 05) — T-05-05 / SCN-083-E08.
// End-to-end extraction audit over the live stack: live PostgreSQL + the real
// ML sidecar (whose /extract-card-categories route fronts the model gateway).
// The integration lane brings up the full disposable stack (Postgres + ML
// sidecar + Ollama) and injects ML_SIDECAR_URL, so this test runs the real
// orchestrator → sidecar round-trip whenever the sidecar is reachable and SKIPS
// HONESTLY (never fabricates) when it is not. Whether the model leg yields a
// schema-valid extraction (Stored) or an error/refusal (Discarded) is
// environment-dependent on the Ollama model availability and is NOT asserted —
// strict validation + needs_verification is the safety net; the asserted
// contract is the persisted extract audit run.
//
// The audit-run PERSISTENCE half of E08 (a card_runs row with run_type=extract
// and the attempt/success/category counts) is also proven deterministically,
// without any model backend, by
// internal/cardrewards/extract_integration_test.go::TestExtractorLivePG_ExtractionRunAudited_E08.
// This test adds the real sidecar round-trip on top.
//
// Run via: ./smackerel.sh test integration --go-run CardRewardsExtractLiveStack

package integration

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/cardrewards"
	"github.com/smackerel/smackerel/internal/db"
)

func TestCardRewardsExtractLiveStackAudited_E08(t *testing.T) {
	// The integration lane brings up the full disposable stack (Postgres + the
	// real ML sidecar + Ollama) and injects ML_SIDECAR_URL. This test runs the
	// real orchestrator → sidecar → model round-trip whenever the sidecar is
	// reachable, and skips HONESTLY (never fabricates a pass) when the live ML
	// sidecar is absent. It does not require the SMACKEREL_TEST_OLLAMA opt-in
	// because, unlike the e2e lane, the integration runner does not forward it
	// into the go-test container — gating on sidecar reachability is what lets
	// the round-trip actually execute against the live stack.
	sidecarURL := os.Getenv("ML_SIDECAR_URL")
	if strings.TrimSpace(sidecarURL) == "" {
		t.Skip("SCN-083-E08: ML_SIDECAR_URL not set — live ML sidecar not available (run via ./smackerel.sh test integration)")
	}

	// Probe the sidecar before committing to the test so an unprovisioned
	// model lane produces an honest skip, not a misleading failure.
	healthClient := &http.Client{Timeout: 3 * time.Second}
	healthReq, err := http.NewRequest(http.MethodGet, strings.TrimRight(sidecarURL, "/")+"/health", nil)
	if err != nil {
		t.Fatalf("build sidecar health request: %v", err)
	}
	resp, err := healthClient.Do(healthReq)
	if err != nil {
		t.Skipf("SCN-083-E08: ML sidecar not reachable at %s (%v) — live model lane not provisioned", sidecarURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("SCN-083-E08: ML sidecar health = HTTP %d — live model lane not provisioned", resp.StatusCode)
	}

	pool := testPool(t)
	ctx := context.Background()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := cardrewards.NewStore(pool)

	// The sidecar's verify_auth treats an empty SMACKEREL_AUTH_TOKEN as dev-mode
	// bypass; the HTTP extractor still requires a non-empty Bearer string, so we
	// pass a placeholder when the env token is empty.
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if authToken == "" {
		authToken = "dev-mode-bypass"
	}
	extractor, err := cardrewards.NewHTTPSidecarExtractor(sidecarURL, authToken, 120*time.Second)
	if err != nil {
		t.Fatalf("build sidecar extractor: %v", err)
	}

	// Seed (idempotently) a known card so the unknown-card skip path does not
	// fire for a model that correctly echoes the requested id.
	const cardID = "discover-it"
	const period = "Q3_2026"
	if err := store.UpsertCatalogCard(ctx, &cardrewards.CatalogCard{
		ID:       cardID,
		Name:     "Discover it Cash Back",
		Issuer:   "Discover",
		CardType: cardrewards.CardTypeRotating,
		Source:   cardrewards.SourceSeed,
	}); err != nil {
		t.Fatalf("seed catalog card: %v", err)
	}

	ex := cardrewards.NewExtractor(store, extractor, 0.70, nil)
	res, err := ex.Run(ctx, []cardrewards.ExtractInput{{
		CardID:      cardID,
		IssuerHint:  "Discover",
		PeriodLabel: period,
		SourceName:  "Live Stack Source",
		SourceURL:   "https://example.test/discover-q3-2026",
		PageText: "Discover it Cash Back — Q3 2026 (July 1 to September 30, 2026): " +
			"earn 5% cash back at Restaurants and on PayPal purchases, up to the " +
			"quarterly maximum of $1,500 in combined purchases, after you activate.",
	}}, cardrewards.RunTriggerManual)
	if err != nil {
		t.Fatalf("live extraction run: %v", err)
	}

	// Deterministic contract regardless of the small test model's accuracy:
	// the full sidecar→model→validate→persist-audit path executed and recorded
	// exactly one extract run with one attempted source. Whether the model's
	// output validated (Stored) or not (Discarded) is model-dependent and not
	// asserted here — strict validation + needs_verification is the safety net.
	if res.Run == nil {
		t.Fatal("expected a persisted extract run")
	}
	if res.Run.RunType != cardrewards.RunTypeExtract {
		t.Errorf("run_type = %q, want extract", res.Run.RunType)
	}
	if res.Run.SourcesAttempted != 1 {
		t.Errorf("sources_attempted = %d, want 1", res.Run.SourcesAttempted)
	}
	if res.Stored+res.Discarded+res.Skipped != 1 {
		t.Errorf("exactly one input must be accounted for, got stored=%d discarded=%d skipped=%d",
			res.Stored, res.Discarded, res.Skipped)
	}

	var runType string
	var attempted int
	if err := pool.QueryRow(ctx,
		`SELECT run_type, sources_attempted FROM card_runs WHERE id = $1`, res.Run.ID,
	).Scan(&runType, &attempted); err != nil {
		t.Fatalf("query persisted card_runs row: %v", err)
	}
	if runType != cardrewards.RunTypeExtract || attempted != 1 {
		t.Errorf("persisted run: run_type=%q attempted=%d, want extract/1", runType, attempted)
	}
	t.Logf("SCN-083-E08 live extraction: stored=%d discarded=%d skipped=%d lowConfidence=%d flagged=%d",
		res.Stored, res.Discarded, res.Skipped, res.LowConfidence, res.Flagged)
}
