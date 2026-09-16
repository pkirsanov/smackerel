//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// ensureSynthesisStreamForBackfill makes sure the SYNTHESIS stream exists on
// the disposable test NATS instance. Mirrors the stream config asserted by
// TestNATS_EnsureStreams (nats_stream_test.go) — creating/updating here too
// keeps this file runnable in isolation (`go test -run TestSynthesisBackfill`)
// without depending on test execution order.
func ensureSynthesisStreamForBackfill(t *testing.T, ctx context.Context, js jetstream.JetStream) {
	t.Helper()
	for _, sc := range smacknats.AllStreams() {
		if sc.Name != "SYNTHESIS" {
			continue
		}
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:      sc.Name,
			Subjects:  sc.Subjects,
			Retention: jetstream.WorkQueuePolicy,
			MaxAge:    7 * 24 * time.Hour,
			MaxBytes:  536870912,
			Storage:   jetstream.FileStorage,
		})
		if err != nil {
			t.Fatalf("ensure SYNTHESIS stream: %v", err)
		}
		return
	}
	t.Fatalf("SYNTHESIS stream not found in smacknats.AllStreams()")
}

// seedBackfillArtifact inserts one artifact row directly at
// synthesis_status='pending', simulating the historical incident backlog:
// content exists, but nothing durable in NATS points at it any more.
func seedBackfillArtifact(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, synthesis_status, created_at, updated_at)
		VALUES ($1, 'article', $2, 'a test summary', 'some raw content body', $3, $4, 'pending', NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET synthesis_status = 'pending', synthesis_error = NULL`,
		id, "backfill test artifact "+id, "hash-"+id, "src-"+id)
	if err != nil {
		t.Fatalf("seed backfill artifact %s: %v", id, err)
	}
}

// TestSynthesisBackfill_RepublishesStuckPendingArtifacts is the real,
// end-to-end proof for the backfill tool's core promise: an artifact stuck at
// synthesis_status='pending' with no corresponding in-flight NATS message
// gets a fresh synthesis.extract request published for it, built from the
// exact same contract the live SynthesisResultSubscriber consumes.
func TestSynthesisBackfill_RepublishesStuckPendingArtifacts(t *testing.T) {
	pool := testPool(t)
	js, nc := testJetStream(t)
	client := &smacknats.Client{Conn: nc, JetStream: js}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ensureSynthesisStreamForBackfill(t, ctx, js)

	artifactID := testID(t)
	seedBackfillArtifact(t, pool, artifactID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM artifacts WHERE id = $1", artifactID)
	})

	linterCfg := knowledge.LinterConfig{
		MaxSynthesisRetries:      5,
		PromptContractVersion:    "test/v1",
		MaxSynthesisContextItems: 10,
		MaxSynthesisContentChars: 4000,
	}
	store := knowledge.NewKnowledgeStore(pool)
	linter := knowledge.NewLinter(store, pool, linterCfg, client)

	// Durable pull consumer scoped to this test so we can positively assert
	// on the message the backfill run publishes, not just that the DB call
	// succeeded.
	consumerName := "backfill-test-" + artifactID
	consumer, err := js.CreateOrUpdateConsumer(ctx, "SYNTHESIS", jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: smacknats.SubjectSynthesisExtract,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatalf("create pull consumer: %v", err)
	}

	result, err := linter.RunSynthesisBackfill(ctx, knowledge.SynthesisBackfillOptions{
		BatchSize:    10,
		Cooldown:     time.Minute,
		MaxArtifacts: 1,
		TriggeredBy:  "backfill_integration_test",
	})
	if err != nil {
		t.Fatalf("RunSynthesisBackfill: %v", err)
	}
	if result.Processed != 1 {
		t.Fatalf("Processed = %d, want 1 (result=%+v)", result.Processed, result)
	}
	if result.Failed != 0 {
		t.Fatalf("Failed = %d, want 0", result.Failed)
	}

	msgs, err := consumer.Fetch(5, jetstream.FetchMaxWait(10*time.Second))
	if err != nil {
		t.Fatalf("fetch republished message: %v", err)
	}
	found := false
	for msg := range msgs.Messages() {
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &payload); err != nil {
			t.Fatalf("unmarshal republished payload: %v", err)
		}
		if payload["artifact_id"] == artifactID {
			found = true
			if payload["triggered_by"] != "backfill_integration_test" {
				t.Errorf("triggered_by = %v, want backfill_integration_test", payload["triggered_by"])
			}
			if payload["prompt_contract_version"] != "test/v1" {
				t.Errorf("prompt_contract_version = %v, want test/v1", payload["prompt_contract_version"])
			}
			if payload["content_raw"] == nil || payload["content_raw"] == "" {
				t.Errorf("content_raw should not be empty — must match the live SynthesisExtractRequest contract")
			}
		}
		_ = msg.Ack()
	}
	if !found {
		t.Fatalf("did not observe a republished synthesis.extract message for artifact %s", artifactID)
	}

	// The artifact must still be 'pending' (completion is reported only by
	// the live SynthesisResultSubscriber upon a real extraction result) but
	// must now carry the requeue marker so an immediate re-run does not
	// double-publish it.
	statuses, err := store.GetArtifactsBySynthesisStatus(ctx, []string{"pending"}, 100)
	if err != nil {
		t.Fatalf("GetArtifactsBySynthesisStatus: %v", err)
	}
	stillPending := false
	for _, a := range statuses {
		if a.ID == artifactID {
			stillPending = true
		}
	}
	if !stillPending {
		t.Fatalf("artifact %s should still be synthesis_status='pending' after requeue (only the live subscriber marks completion)", artifactID)
	}
}

// TestSynthesisBackfill_IdempotentAcrossRuns proves the idempotency
// requirement directly against the database: republishing the same
// still-pending artifact a second time, immediately, does NOT select it
// again — the cooldown-marker rule in GetSynthesisBackfillCandidates
// excludes it — so a second invocation of the CLI right after an
// interruption cannot double-publish.
func TestSynthesisBackfill_IdempotentAcrossRuns(t *testing.T) {
	pool := testPool(t)
	js, nc := testJetStream(t)
	client := &smacknats.Client{Conn: nc, JetStream: js}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ensureSynthesisStreamForBackfill(t, ctx, js)

	artifactID := testID(t)
	seedBackfillArtifact(t, pool, artifactID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM artifacts WHERE id = $1", artifactID)
	})

	store := knowledge.NewKnowledgeStore(pool)
	linter := knowledge.NewLinter(store, pool, knowledge.LinterConfig{
		MaxSynthesisRetries:   5,
		PromptContractVersion: "test/v1",
	}, client)

	opts := knowledge.SynthesisBackfillOptions{
		BatchSize:    10,
		Cooldown:     time.Hour, // long cooldown: second run must see zero candidates
		MaxArtifacts: 5,
		TriggeredBy:  "backfill_idempotency_test",
	}

	first, err := linter.RunSynthesisBackfill(ctx, opts)
	if err != nil {
		t.Fatalf("first RunSynthesisBackfill: %v", err)
	}
	if first.Processed != 1 {
		t.Fatalf("first run Processed = %d, want 1", first.Processed)
	}

	second, err := linter.RunSynthesisBackfill(ctx, opts)
	if err != nil {
		t.Fatalf("second RunSynthesisBackfill: %v", err)
	}
	if second.Processed != 0 {
		t.Fatalf("second run Processed = %d, want 0 — the cooldown marker must suppress re-selection", second.Processed)
	}
	if !second.Exhausted {
		t.Fatalf("second run should report Exhausted=true (no eligible candidates), got %+v", second)
	}
}

// TestSynthesisBackfill_SkipsAlreadyCompletedArtifacts proves the other half
// of idempotency: once an artifact has transitioned out of 'pending' (as the
// live subscriber does on a real extraction result), the backfill tool never
// selects it again, regardless of the cooldown marker.
func TestSynthesisBackfill_SkipsAlreadyCompletedArtifacts(t *testing.T) {
	pool := testPool(t)
	js, nc := testJetStream(t)
	client := &smacknats.Client{Conn: nc, JetStream: js}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ensureSynthesisStreamForBackfill(t, ctx, js)

	artifactID := testID(t)
	seedBackfillArtifact(t, pool, artifactID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM artifacts WHERE id = $1", artifactID)
	})

	store := knowledge.NewKnowledgeStore(pool)
	if err := store.UpdateArtifactSynthesisStatus(ctx, artifactID, "completed", ""); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	linter := knowledge.NewLinter(store, pool, knowledge.LinterConfig{
		MaxSynthesisRetries:   5,
		PromptContractVersion: "test/v1",
	}, client)

	result, err := linter.RunSynthesisBackfill(ctx, knowledge.SynthesisBackfillOptions{
		BatchSize:    10,
		Cooldown:     time.Minute,
		MaxArtifacts: 5,
		TriggeredBy:  "backfill_completed_test",
	})
	if err != nil {
		t.Fatalf("RunSynthesisBackfill: %v", err)
	}
	if result.Processed != 0 {
		t.Fatalf("Processed = %d, want 0 — a completed artifact must never be re-requeued", result.Processed)
	}
}
