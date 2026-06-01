//go:build integration

// Spec 074 SCOPE-2 — TP-074-05 + TP-074-06 / SCN-074-A02.
//
// Integration proof that:
//  1. The capturefallback.PostgresStore writes
//     artifact_capture_policy rows against a real Postgres pool.
//  2. Explicit (capture-explicit) and fallback (capture-as-fallback)
//     captures of the same normalized text remain provenance-distinct
//     and are independently queryable.
//  3. The fallback-only unique index in migration 051 does NOT
//     collide an explicit row against a fallback row (an explicit
//     row leaves dedup_bucket_start NULL, so the partial index
//     does not apply).
//
// Adversarial coverage: after writing one explicit + one fallback row
// for the same (user, normalized text), CountByProvenance reports 1
// for each provenance — proving the two captures did not collapse.
// If a future change merged the two provenances (e.g. by dropping the
// provenance column from the dedup index), the count would be 1/0
// instead of 1/1 and this test would trip.

package assistant_integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

const (
	scope2HashKey     = "tp-074-05-hmac-key-do-not-reuse"
	scope2DedupWindow = 5 * time.Minute
)

func openScope2Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// insertTestArtifact creates a minimal artifacts row so the
// artifact_capture_policy FK is satisfied. The unique partial index on
// artifacts.content_hash means we must vary content_hash per call.
func insertTestArtifact(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'idea', $2, $3, 'capture')
	`, id, "spec074-scope2-"+id, "h-"+id)
	if err != nil {
		t.Fatalf("insert test artifact %s: %v", id, err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, derr := pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, id); derr != nil {
			t.Logf("cleanup artifact %s: %v", id, derr)
		}
	})
}

// TestCaptureFallbackPolicy_TP_074_05_ExplicitCaptureWritesCaptureExplicitProvenance
// — TP-074-05 / SCN-074-A02.
func TestCaptureFallbackPolicy_TP_074_05_ExplicitCaptureWritesCaptureExplicitProvenance(t *testing.T) {
	pool := openScope2Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userID := fmt.Sprintf("scope2-user-explicit-%d", time.Now().UnixNano())
	artifactID := fmt.Sprintf("scope2-art-explicit-%d", time.Now().UnixNano())
	insertTestArtifact(t, pool, artifactID)

	payload := capturefallback.BuildExplicitPayload(capturefallback.ExplicitCaptureInput{
		ArtifactID:     artifactID,
		UserID:         userID,
		NormalizedText: capturefallback.NormalizeV1("Buy milk on the way home"),
		DedupHashKey:   scope2HashKey,
		SourceTurnID:   "api:" + artifactID,
		CreatedAt:      time.Now().UTC(),
	})
	if err := store.Record(ctx, payload); err != nil {
		t.Fatalf("Record explicit payload: %v", err)
	}

	got, err := store.GetByArtifactID(ctx, artifactID)
	if err != nil {
		t.Fatalf("GetByArtifactID: %v", err)
	}
	if got.Provenance != capturefallback.ProvenanceExplicit {
		t.Errorf("provenance = %q, want %q", got.Provenance, capturefallback.ProvenanceExplicit)
	}
	if got.FallbackCause != "" {
		t.Errorf("fallback_cause = %q, want empty for explicit row", got.FallbackCause)
	}
	if !got.DedupBucketStart.IsZero() {
		t.Errorf("dedup_bucket_start = %s, want zero/NULL for explicit row", got.DedupBucketStart)
	}
	if got.NormalizedTextHash == "" {
		t.Error("normalized_text_hash empty; explicit row must persist the hash for cross-provenance audit")
	}
}

// TestCaptureFallbackPolicy_TP_074_06_ExplicitAndFallbackDistinguishableByProvenanceQuery
// — TP-074-06 / SCN-074-A02. Writes one explicit + one fallback row for
// the SAME (user, normalized text) and proves CountByProvenance reports
// 1 for each provenance. If the two captures collapsed into a single
// provenance (regression), one of the counts would be 0.
func TestCaptureFallbackPolicy_TP_074_06_ExplicitAndFallbackDistinguishableByProvenanceQuery(t *testing.T) {
	pool := openScope2Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userID := fmt.Sprintf("scope2-user-distinct-%d", time.Now().UnixNano())
	const text = "lookup conference room c on monday"
	normalized := capturefallback.NormalizeV1(text)
	now := time.Now().UTC()

	explicitID := fmt.Sprintf("scope2-art-distinct-explicit-%d", time.Now().UnixNano())
	insertTestArtifact(t, pool, explicitID)
	explicit := capturefallback.BuildExplicitPayload(capturefallback.ExplicitCaptureInput{
		ArtifactID:     explicitID,
		UserID:         userID,
		NormalizedText: normalized,
		DedupHashKey:   scope2HashKey,
		SourceTurnID:   "api:" + explicitID,
		CreatedAt:      now,
	})
	if err := store.Record(ctx, explicit); err != nil {
		t.Fatalf("Record explicit: %v", err)
	}

	fallbackID := fmt.Sprintf("scope2-art-distinct-fallback-%d", time.Now().UnixNano()+1)
	insertTestArtifact(t, pool, fallbackID)
	fallback := capturefallback.CapturePayload{
		ArtifactID:             fallbackID,
		UserID:                 userID,
		Provenance:             capturefallback.ProvenanceFallback,
		FallbackCause:          capturefallback.CauseUnrouted,
		NormalizedText:         normalized,
		NormalizedTextHash:     capturefallback.HashNormalized(normalized, scope2HashKey),
		DedupBucketStart:       capturefallback.BucketStart(now, scope2DedupWindow),
		DedupWindowSeconds:     int(scope2DedupWindow / time.Second),
		SourceTurnID:           "test:" + fallbackID,
		AbandonedClarification: false,
		SchemaVersion:          capturefallback.SchemaVersion,
		CreatedAt:              now,
	}
	if err := store.Record(ctx, fallback); err != nil {
		t.Fatalf("Record fallback: %v", err)
	}

	explicitCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceExplicit)
	if err != nil {
		t.Fatalf("CountByProvenance(explicit): %v", err)
	}
	if explicitCount != 1 {
		t.Errorf("explicit count = %d, want 1", explicitCount)
	}
	fallbackCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceFallback)
	if err != nil {
		t.Fatalf("CountByProvenance(fallback): %v", err)
	}
	if fallbackCount != 1 {
		t.Errorf("fallback count = %d, want 1 (provenance separation regression — explicit and fallback rows collapsed)", fallbackCount)
	}

	// Adversarial cross-check: per-artifact lookup must report
	// distinct provenance values. If the rows were merged or one
	// was dropped, the second GetByArtifactID would fail or report
	// the wrong provenance.
	gotExplicit, err := store.GetByArtifactID(ctx, explicitID)
	if err != nil {
		t.Fatalf("GetByArtifactID(explicit): %v", err)
	}
	if gotExplicit.Provenance != capturefallback.ProvenanceExplicit {
		t.Errorf("explicit row provenance = %q, want %q", gotExplicit.Provenance, capturefallback.ProvenanceExplicit)
	}
	gotFallback, err := store.GetByArtifactID(ctx, fallbackID)
	if err != nil {
		t.Fatalf("GetByArtifactID(fallback): %v", err)
	}
	if gotFallback.Provenance != capturefallback.ProvenanceFallback {
		t.Errorf("fallback row provenance = %q, want %q", gotFallback.Provenance, capturefallback.ProvenanceFallback)
	}
	if gotFallback.FallbackCause != capturefallback.CauseUnrouted {
		t.Errorf("fallback row cause = %q, want %q", gotFallback.FallbackCause, capturefallback.CauseUnrouted)
	}
}

// TestCaptureFallbackPolicy_TP_074_10_CrossUserSameTextCreatesSeparateIdeas
// — TP-074-10 / SCN-074-A05.
//
// Drives the live PostgresDedupStore + Policy.CaptureForUser path with
// two distinct users sending the same normalized text in the same
// dedup bucket. Each user MUST receive their own artifact id; a
// regression that dropped user_id from the dedup key would collapse
// the two into one and the second CaptureForUser would return
// AlreadyCaptured.
//
// Adversarial coverage: the test also runs a SECOND turn for user A
// in the same bucket and asserts it DOES dedup against user A's first
// artifact — proving the test users are configured for the same bucket
// (otherwise the cross-user assertion would be vacuously true).
func TestCaptureFallbackPolicy_TP_074_10_CrossUserSameTextCreatesSeparateIdeas(t *testing.T) {
	pool := openScope2Pool(t)
	dedup := capturefallback.NewPostgresDedupStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const text = "Cross-user dedup-leak regression candidate"
	now := time.Now().UTC()
	stamp := now.UnixNano()
	userA := fmt.Sprintf("scope3-userA-%d", stamp)
	userB := fmt.Sprintf("scope3-userB-%d", stamp)

	cfg := capturefallback.Config{
		DedupWindow:         scope2DedupWindow,
		NormalizationPolicy: capturefallback.NormalizationPolicyV1,
		DedupHashKey:        scope2HashKey,
	}

	// Three artifact ids — userA-first, userB-first, userA-second.
	artA1 := fmt.Sprintf("scope3-art-A1-%d", stamp)
	artB1 := fmt.Sprintf("scope3-art-B1-%d", stamp+1)
	artA2 := fmt.Sprintf("scope3-art-A2-%d", stamp+2)
	insertTestArtifact(t, pool, artA1)
	insertTestArtifact(t, pool, artB1)
	insertTestArtifact(t, pool, artA2)

	// Use a single IdeaWriter stub that hands out the pre-inserted
	// artifact ids in order. The store/lookup path is the live
	// Postgres surface under test; the writer is stubbed only to
	// avoid pulling the SCOPE-4 facade into a SCOPE-3 test.
	writer := &orderedWriter{ids: []string{artA1, artB1, artA2}}

	policy, err := capturefallback.New(cfg, dedup, writer)
	if err != nil {
		t.Fatalf("New policy: %v", err)
	}

	mkReq := func(user, msgID string) capturefallback.Request {
		return capturefallback.Request{
			UserID:             user,
			Transport:          "telegram",
			TransportMessageID: msgID,
			OriginalText:       text,
			Cause:              capturefallback.CauseUnrouted,
			OccurredAt:         now,
		}
	}

	decA, err := policy.Decide(ctx, mkReq(userA, "tg:A1"))
	if err != nil {
		t.Fatalf("Decide userA: %v", err)
	}
	resA, err := policy.CaptureForUser(ctx, userA, decA)
	if err != nil {
		t.Fatalf("CaptureForUser userA: %v", err)
	}
	if resA.AlreadyCaptured {
		t.Fatalf("userA first capture wrongly reported AlreadyCaptured")
	}
	if resA.IdeaArtifactID != artA1 {
		t.Errorf("userA artifact = %q, want %q", resA.IdeaArtifactID, artA1)
	}

	// userB, same text, same bucket — MUST get a new artifact.
	decB, err := policy.Decide(ctx, mkReq(userB, "tg:B1"))
	if err != nil {
		t.Fatalf("Decide userB: %v", err)
	}
	if !decB.DedupBucketStart.Equal(decA.DedupBucketStart) {
		t.Fatalf("userB bucket diverged from userA: A=%s B=%s (test assumption broken)", decA.DedupBucketStart, decB.DedupBucketStart)
	}
	resB, err := policy.CaptureForUser(ctx, userB, decB)
	if err != nil {
		t.Fatalf("CaptureForUser userB: %v", err)
	}
	if resB.AlreadyCaptured {
		t.Fatalf("cross-user dedup leak: userB's first capture reported AlreadyCaptured (source=%q) — SCN-074-A05 regression", resB.AlreadyCapturedSourceID)
	}
	if resB.IdeaArtifactID != artB1 {
		t.Errorf("userB artifact = %q, want %q", resB.IdeaArtifactID, artB1)
	}

	// Adversarial in-window dedup probe: userA again, same bucket,
	// MUST dedup to artA1. Proves the dedup key is correctly
	// composed (otherwise the cross-user check above would be
	// vacuously true).
	decA2, err := policy.Decide(ctx, mkReq(userA, "tg:A2"))
	if err != nil {
		t.Fatalf("Decide userA second: %v", err)
	}
	resA2, err := policy.CaptureForUser(ctx, userA, decA2)
	if err != nil {
		t.Fatalf("CaptureForUser userA second: %v", err)
	}
	if !resA2.AlreadyCaptured {
		t.Fatalf("userA second in-window capture did NOT dedup; got new artifact %q (dedup-key composition regression — cross-user test is now vacuous)", resA2.IdeaArtifactID)
	}
	if resA2.AlreadyCapturedSourceID != artA1 {
		t.Errorf("userA second dedup source = %q, want %q", resA2.AlreadyCapturedSourceID, artA1)
	}
}

// orderedWriter returns pre-inserted artifact ids in FIFO order so
// SCOPE-3 integration tests can satisfy the artifacts FK without
// pulling in the SCOPE-4 facade IdeaWriter.
type orderedWriter struct {
	ids []string
	i   int
}

func (w *orderedWriter) WriteIdea(_ context.Context, _ string, _ string, _ capturefallback.Decision) (string, error) {
	if w.i >= len(w.ids) {
		return "", fmt.Errorf("orderedWriter: exhausted (%d ids consumed)", w.i)
	}
	id := w.ids[w.i]
	w.i++
	return id, nil
}

// --- Spec 074 SCOPE-04A TP-074-12 — facade fallback hook -----------

// pgIdeaWriter is the SCOPE-04A test IdeaWriter that inserts a real
// `artifacts` row so the artifact_capture_policy FK and the
// CountByProvenance assertion both hit live Postgres state.
type pgIdeaWriter struct {
	t         *testing.T
	pool      *pgxpool.Pool
	prefix    string
	mu        sync.Mutex
	artifacts []string
}

func newPGIdeaWriter(t *testing.T, pool *pgxpool.Pool, prefix string) *pgIdeaWriter {
	w := &pgIdeaWriter{t: t, pool: pool, prefix: prefix}
	t.Cleanup(func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, id := range w.artifacts {
			cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
			if _, derr := pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, id); derr != nil {
				t.Logf("cleanup artifact %s: %v", id, derr)
			}
			ccancel()
		}
	})
	return w
}

func (w *pgIdeaWriter) WriteIdea(ctx context.Context, _ string, normalizedText string, _ capturefallback.Decision) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	id := fmt.Sprintf("%s-%d-%d", w.prefix, time.Now().UnixNano(), len(w.artifacts))
	_, err := w.pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'idea', $2, $3, 'capture')
	`, id, "spec074-scope4a-"+id, "h-"+id+"-"+normalizedText[:1])
	if err != nil {
		return "", fmt.Errorf("pgIdeaWriter insert %s: %w", id, err)
	}
	w.artifacts = append(w.artifacts, id)
	return id, nil
}

func newScope4APolicy(t *testing.T, pool *pgxpool.Pool, prefix string) (capturefallback.Policy, *pgIdeaWriter) {
	t.Helper()
	cfg := capturefallback.Config{
		DedupWindow:         scope2DedupWindow,
		NormalizationPolicy: capturefallback.NormalizationPolicyV1,
		DedupHashKey:        scope2HashKey,
	}
	writer := newPGIdeaWriter(t, pool, prefix)
	policy, err := capturefallback.New(cfg, capturefallback.NewPostgresDedupStore(pool), writer)
	if err != nil {
		t.Fatalf("capturefallback.New: %v", err)
	}
	return policy, writer
}

func newScope4AFacadeCfg(now time.Time) assistant.FacadeConfig {
	return assistant.FacadeConfig{
		BorderlineFloor:      0.75,
		AgentConfidenceFloor: 0.50,
		SourcesMax:           5,
		BodyMaxChars:         1000,
		WindowTurns:          5,
		DisambigMaxChoices:   3,
		DisambigTimeout:      30 * time.Second,
		Now:                  func() time.Time { return now },
	}
}

func buildScope4AFacade(t *testing.T, policy capturefallback.Policy, contextStore *assistant.InMemoryContextStore) *assistant.Facade {
	t.Helper()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	router := assistant.NewStubRouter()
	router.OK = false
	router.Decision = agent.RoutingDecision{Reason: agent.ReasonUnknownIntent}
	manifest, err := assistant.NewManifestForTest(map[string]assistant.ManifestEntryForTest{})
	if err != nil {
		t.Fatalf("NewManifestForTest: %v", err)
	}
	facade, err := assistant.NewFacade(
		newScope4AFacadeCfg(now),
		router,
		assistant.NewStubExecutor(),
		assistant.NewMapRegistry(map[string]*agent.Scenario{}),
		manifest,
		contextStore,
		assistant.NewRecordingAudit(),
	)
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}
	return facade.WithCaptureFallbackPolicy(policy)
}

// TestCaptureFallbackPolicy_TP_074_12_FacadeHookCreatesOneFallbackIdea
// — TP-074-12 / SCN-074-A01.
//
// Drives the live Facade with capturefallback.Policy wired and an
// unrouted text turn. Asserts exactly one fallback Idea is persisted
// via the artifact_capture_policy row count and that the response
// shape is the canonical "saved as idea" envelope. Adversarial probe:
// CountByProvenance(explicit) MUST be 0 (proves the row was written
// with the fallback provenance, not silently promoted to explicit).
func TestCaptureFallbackPolicy_TP_074_12_FacadeHookCreatesOneFallbackIdea(t *testing.T) {
	pool := openScope2Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	policy, _ := newScope4APolicy(t, pool, "scope4a-tp12-art")
	contextStore := assistant.NewInMemoryContextStore()
	facade := buildScope4AFacade(t, policy, contextStore)

	userID := fmt.Sprintf("scope4a-tp12-user-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := facade.Handle(ctx, contracts.AssistantMessage{
		UserID:    userID,
		Transport: "telegram",
		Text:      "random unrouted thought worth keeping",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Status != contracts.StatusSavedAsIdea {
		t.Errorf("Status = %q, want %q", resp.Status, contracts.StatusSavedAsIdea)
	}
	if !resp.CaptureRoute {
		t.Error("CaptureRoute = false; BandLow fallback MUST set CaptureRoute=true")
	}

	fallbackCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceFallback)
	if err != nil {
		t.Fatalf("CountByProvenance(fallback): %v", err)
	}
	if fallbackCount != 1 {
		t.Errorf("fallback count = %d, want 1 (facade hook must write exactly one Idea per unrouted turn)", fallbackCount)
	}
	explicitCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceExplicit)
	if err != nil {
		t.Fatalf("CountByProvenance(explicit): %v", err)
	}
	if explicitCount != 0 {
		t.Errorf("explicit count = %d, want 0 (facade hook MUST persist provenance=capture-as-fallback)", explicitCount)
	}
}

// TestCaptureFallbackPolicy_TP_074_12_EligibilityGateBlocksConfirmAndDisambigStates
// — TP-074-12 adversarial coverage for the SCOPE-04A eligibility rule.
//
// Pre-loads the in-memory context store with a PendingConfirm and a
// PendingDisambig for two separate users, drives an unrouted turn for
// each, and asserts NO fallback Idea was written (because the
// eligibility gate must suppress the policy hook for those states).
// Without the eligibility gate the test would see fallback rows
// appear and CountByProvenance would report >0.
func TestCaptureFallbackPolicy_TP_074_12_EligibilityGateBlocksConfirmAndDisambigStates(t *testing.T) {
	pool := openScope2Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	policy, _ := newScope4APolicy(t, pool, "scope4a-tp12-elig-art")
	contextStore := assistant.NewInMemoryContextStore()
	facade := buildScope4AFacade(t, policy, contextStore)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	userConfirm := fmt.Sprintf("scope4a-tp12-elig-confirm-%d", time.Now().UnixNano())
	userDisambig := fmt.Sprintf("scope4a-tp12-elig-disambig-%d", time.Now().UnixNano())

	confirmConv := assistantctx.Conversation{
		UserID:         userConfirm,
		Transport:      "telegram",
		SchemaVersion:  1,
		LastActivityAt: now,
		PendingConfirm: &assistantctx.PendingConfirm{ConfirmRef: "cf-tp12"},
	}
	if err := contextStore.Persist(ctx, confirmConv); err != nil {
		t.Fatalf("seed pending-confirm conv: %v", err)
	}
	disambigConv := assistantctx.Conversation{
		UserID:          userDisambig,
		Transport:       "telegram",
		SchemaVersion:   1,
		LastActivityAt:  now,
		PendingDisambig: &assistantctx.PendingDisambig{DisambiguationRef: "d-tp12", ExpiresAt: now.Add(time.Hour)},
	}
	if err := contextStore.Persist(ctx, disambigConv); err != nil {
		t.Fatalf("seed pending-disambig conv: %v", err)
	}

	for _, u := range []string{userConfirm, userDisambig} {
		if _, err := facade.Handle(ctx, contracts.AssistantMessage{
			UserID:    u,
			Transport: "telegram",
			Text:      "reply that should not be captured as fallback",
			Kind:      contracts.KindText,
		}); err != nil {
			t.Fatalf("Handle(%s): %v", u, err)
		}
		got, err := store.CountByProvenance(ctx, u, capturefallback.ProvenanceFallback)
		if err != nil {
			t.Fatalf("CountByProvenance(fallback, %s): %v", u, err)
		}
		if got != 0 {
			t.Errorf("user %s fallback count = %d, want 0 (eligibility gate regression: pending-state turn was captured)", u, got)
		}
	}
}
