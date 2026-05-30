//go:build integration

// Spec 061 SCOPE-06 — facade source-assembly hook integration test.
//
// This test drives Facade.Handle end-to-end against:
//
//   - REAL PostgreSQL (test stack via DATABASE_URL).
//   - REAL retrieval.NewFacadeAssembler bound to a REAL
//     *db.Postgres.GetArtifact lookup.
//   - REAL provenance.Enforce gate (via the facade's BandHigh
//     dispatch path).
//   - REAL contracts wiring (no in-memory shortcut for the assembler
//     seam — same closure shape that cmd/core/wiring_assistant_facade.go
//     installs in production).
//
// The executor is stubbed because driving the ml/ sidecar from a Go
// integration test would require fully reseeding the embeddings
// index. The stub returns the same retrieval-qa-v1 output shape
// (`{answer, cited_artifact_ids}`) the real executor produces, so
// the source-assembly seam, the AssembleSources function, the
// PostgreSQL lookup, and the provenance gate all execute on real
// substrate. This is the layer the production code actually
// integrates at; the only thing missing is the ml/ leg (covered by
// the BS-002 / BS-007 shell e2e tests when they land).
//
// Two cases:
//
//  1. BS-002 happy path — seeded artifacts exist in PG; assembler
//     populates resp.Sources with their (Title, CapturedAt); gate
//     passes through; body == synthesized answer.
//
//  2. BS-007 graph drift — cited IDs do NOT exist in PG; assembler
//     returns empty Sources; gate rewrites to canonical refusal with
//     CaptureRoute=true.
//
// Run with:
//
//	DATABASE_URL=postgres://... go test -tags integration \
//	    ./internal/assistant/ -run TestFacadeSourceAssemblyIntegration \
//	    -count=1 -v

package assistant

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/db"
)

func newIntegrationPostgres(t *testing.T) (*db.Postgres, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	ctx := context.Background()
	pg, err := db.Connect(ctx, databaseURL, 4, 1)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pg.Close)
	if err := db.Migrate(ctx, pg.Pool); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	return pg, pg.Pool
}

// seedArtifact inserts a minimal artifact row and registers a t.Cleanup
// that deletes it. Returns the (title, capturedAt) the lookup will
// surface so the test can assert exact-match on the resulting Source
// payload.
func seedArtifact(t *testing.T, pool *pgxpool.Pool, id string) (string, time.Time) {
	t.Helper()
	ctx := context.Background()
	title := "seed-title-" + id
	// PostgreSQL truncates timestamps to microsecond precision. Round
	// the expected value the same way so equality assertions succeed.
	capturedAt := time.Now().UTC().Round(time.Microsecond)
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, title, artifact_type, summary,
		                       content_hash, source_id,
		                       processing_status, created_at, updated_at)
		VALUES ($1, $2, 'note', 'integration-seed',
		        $4, 'integration-test',
		        'processed', $3, $3)
	`, id, title, capturedAt, "hash-"+id)
	if err != nil {
		t.Fatalf("seed artifact %s: %v", id, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id = $1`, id)
	})
	return title, capturedAt
}

// newPostgresArtifactLookupForTest mirrors the production lookup in
// cmd/core/wiring_assistant_facade.go::newPostgresArtifactLookup. Kept
// inline here (not exported from cmd/core) because the wiring helper
// lives in package main and pulling it into a library would invert the
// cmd → internal dependency direction.
func newPostgresArtifactLookupForTest(pg *db.Postgres) retrieval.ArtifactLookupFn {
	return func(ctx context.Context, artifactID string) (string, time.Time, bool, error) {
		a, err := pg.GetArtifact(ctx, artifactID)
		if err != nil {
			if pgxIsErrNoRows(err) {
				return "", time.Time{}, false, nil
			}
			return "", time.Time{}, false, err
		}
		if a == nil {
			return "", time.Time{}, false, nil
		}
		return a.Title, a.CreatedAt, true, nil
	}
}

func pgxIsErrNoRows(err error) bool {
	// db.Postgres.GetArtifact wraps with %w so errors.Is sees the
	// underlying pgx.ErrNoRows. The string contains check defends
	// against any wrapping that breaks the error chain.
	if err == nil {
		return false
	}
	if err == pgx.ErrNoRows {
		return true
	}
	return strings.Contains(err.Error(), "no rows in result set")
}

func TestFacadeSourceAssemblyIntegration_BS002_HighConfidenceWithRealArtifacts(t *testing.T) {
	pg, pool := newIntegrationPostgres(t)

	prefix := "asm-int-bs002-" + time.Now().UTC().Format("20060102150405.000000000")
	idA := prefix + "-A"
	idB := prefix + "-B"
	titleA, capturedAtA := seedArtifact(t, pool, idA)
	titleB, capturedAtB := seedArtifact(t, pool, idB)

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"retrieval_qa": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel:    "ask",
			SlashShortcut:      "/ask",
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skill.retrieval_qa.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	// Real retrieval-qa-v1 output schema.
	answer := "Three captured notes mention Tailscale routes."
	rawJSON := fmt.Sprintf(`{"answer":%q,"cited_artifact_ids":[%q,%q]}`, answer, idA, idB)
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-bs002-int",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(rawJSON),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "retrieval_qa", TopScore: 0.93,
			Considered: []agent.CandidateScore{{ScenarioID: "retrieval_qa", Score: 0.93}},
		},
		ok: true,
	}

	cfg := defaultFacadeConfig(now)
	// Wire the REAL retrieval-side assembler over the REAL PG lookup —
	// same closure shape cmd/core installs in production.
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"retrieval_qa": retrieval.NewFacadeAssembler(
			"retrieval_qa",
			newPostgresArtifactLookupForTest(pg),
			cfg.SourcesMax,
		),
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: prefix + "-u", Transport: "telegram",
		Text: "what do my notes say about tailscale", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	// BS-002 expectations:
	//   - body is the synthesized answer (assembler's body override),
	//     NOT the raw JSON envelope.
	//   - sources contains BOTH seeded artifacts with the (title,
	//     capturedAt) tuple from PG.
	//   - gate passes through (no canonical refusal).
	if resp.Body != answer {
		t.Errorf("Body mismatch:\n  got:  %q\n  want: %q", resp.Body, answer)
	}
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute=true; gate fired unexpectedly (BS-002 happy path)")
	}
	if len(resp.Sources) != 2 {
		t.Fatalf("Sources len = %d; want 2 (both seeded artifacts present)", len(resp.Sources))
	}
	byID := map[string]contracts.Source{}
	for _, s := range resp.Sources {
		byID[s.ID] = s
	}
	for _, want := range []struct {
		id, title  string
		capturedAt time.Time
	}{
		{idA, titleA, capturedAtA},
		{idB, titleB, capturedAtB},
	} {
		got, ok := byID[want.id]
		if !ok {
			t.Errorf("missing seeded artifact %s in resp.Sources", want.id)
			continue
		}
		if got.Title != want.title {
			t.Errorf("Source[%s].Title = %q; want %q", want.id, got.Title, want.title)
		}
		if got.Kind != contracts.SourceArtifact {
			t.Errorf("Source[%s].Kind = %q; want %q", want.id, got.Kind, contracts.SourceArtifact)
			continue
		}
		ref, ok := got.Ref.(contracts.ArtifactRef)
		if !ok {
			t.Errorf("Source[%s].Ref is %T; want contracts.ArtifactRef", want.id, got.Ref)
			continue
		}
		if !ref.CapturedAt.Equal(want.capturedAt) {
			t.Errorf("Source[%s].Ref.CapturedAt = %v; want %v", want.id, ref.CapturedAt, want.capturedAt)
		}
		if ref.ArtifactID != want.id {
			t.Errorf("Source[%s].Ref.ArtifactID = %q; want %q", want.id, ref.ArtifactID, want.id)
		}
	}
	if resp.SourcesOverflowCount != 0 {
		t.Errorf("SourcesOverflowCount = %d; want 0 (2 cited, sourcesMax=5)", resp.SourcesOverflowCount)
	}
	if executor.invocations != 1 {
		t.Errorf("executor.invocations = %d; want 1", executor.invocations)
	}
}

func TestFacadeSourceAssemblyIntegration_BS007_GraphDriftTriggersRefusal(t *testing.T) {
	pg, _ := newIntegrationPostgres(t)

	prefix := "asm-int-bs007-" + time.Now().UTC().Format("20060102150405.000000000")
	missingA := prefix + "-missingA"
	missingB := prefix + "-missingB"
	// Intentionally NOT seeded — both IDs are graph drift.

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"retrieval_qa": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel:    "ask",
			SlashShortcut:      "/ask",
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skill.retrieval_qa.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	answer := "Three captured notes mention Tailscale routes."
	rawJSON := fmt.Sprintf(`{"answer":%q,"cited_artifact_ids":[%q,%q]}`, answer, missingA, missingB)
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-bs007-int",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(rawJSON),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "retrieval_qa", TopScore: 0.91,
			Considered: []agent.CandidateScore{{ScenarioID: "retrieval_qa", Score: 0.91}},
		},
		ok: true,
	}

	cfg := defaultFacadeConfig(now)
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"retrieval_qa": retrieval.NewFacadeAssembler(
			"retrieval_qa",
			newPostgresArtifactLookupForTest(pg),
			cfg.SourcesMax,
		),
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: prefix + "-u", Transport: "telegram",
		Text: "what do my notes say about tailscale", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	// BS-007 expectations: both cited IDs are missing → assembler
	// returns empty Sources → provenance.Enforce rewrites to canonical
	// refusal and sets CaptureRoute=true.
	const canonicalRefusal = "I don't have a sourced answer for that."
	if resp.Body != canonicalRefusal {
		t.Errorf("Body mismatch:\n  got:  %q\n  want: %q", resp.Body, canonicalRefusal)
	}
	if !resp.CaptureRoute {
		t.Errorf("CaptureRoute=false; gate did NOT fire on graph drift")
	}
	if len(resp.Sources) != 0 {
		t.Errorf("Sources len = %d; want 0 (both cited IDs are graph drift)", len(resp.Sources))
	}
	if resp.Status != contracts.StatusSavedAsIdea {
		t.Errorf("Status = %q; want %q", resp.Status, contracts.StatusSavedAsIdea)
	}
	if executor.invocations != 1 {
		t.Errorf("executor.invocations = %d; want 1", executor.invocations)
	}
}
