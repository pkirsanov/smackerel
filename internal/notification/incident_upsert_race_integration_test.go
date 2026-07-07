//go:build integration

package notification

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
)

// TestUpsertIncidentConcurrentSameKeyBurstConvergesWithoutRawUniqueViolation is
// the RED->GREEN regression for the notification_incidents dual-unique-constraint
// race.
//
// notification_incidents carries TWO unique indexes: the `id` primary key and the
// `incident_key` unique index. The live ingest path derives `id` as a
// deterministic bijection of `incident_key` (correlation.go:
// id = "inc_" + hashParts("incident", incident_key)), so a concurrent same-key
// burst issues N inserts that collide on BOTH indexes at once. INSERT ...
// ON CONFLICT (incident_key) can arbitrate only that ONE index; when a racing
// speculative insert trips the non-arbiter `id` primary key first, Postgres
// raises a raw unique_violation (23505 notification_incidents_pkey) instead of
// folding into DO UPDATE — which the ingest surfaces as HTTP 400 and drops the
// event. Swapping the arbiter to (id) does not help: it merely moves the raw
// 23505 to notification_incidents_incident_key_key. A single-arbiter ON CONFLICT
// cannot resolve a dual-unique-constraint collision.
//
// This test repeatedly (rounds independent bursts, each with its own fresh key)
// releases `workers` goroutines simultaneously through a barrier, each upserting
// ONE shared fresh incident (same id, same incident_key), against a real
// Postgres. Repeating the burst makes the speculative-insert race fire reliably
// rather than depending on a single scheduling window. Each round asserts the
// ingest contract: zero upsert errors, exactly one persisted row for the key, and
// persistence_count == workers (each request folds into exactly one logical
// upsert).
//
//   - RED (unfixed single-shot upsert): some upserts return the raw 23505 and the
//     assertions fail.
//   - GREEN (bounded retry-on-23505 fix): every request converges — the raw 23505
//     only fires while the winning `id` row is still uncommitted, so on retry the
//     now-committed winner is found via the incident_key arbiter and the request
//     folds into DO UPDATE.
func TestUpsertIncidentConcurrentSameKeyBurstConvergesWithoutRawUniqueViolation(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	ctx := context.Background()

	const (
		workers = 64
		rounds  = 10
	)

	// Dedicated pool sized to `workers` so every goroutine holds a live
	// connection when the barrier releases. Maximizing the number of concurrent
	// uncommitted speculative inserts of the same id is what makes the raw 23505
	// reproduce deterministically rather than flakily.
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.MaxConns = int32(workers)
	cfg.MinConns = int32(workers)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	store := NewStore(pool)

	// Pre-open all `workers` physical connections before the race so connection
	// acquisition is not part of the timing window and the burst hits Postgres
	// with maximal true concurrency.
	warm := make([]*pgxpool.Conn, 0, workers)
	for i := 0; i < workers; i++ {
		conn, acquireErr := pool.Acquire(ctx)
		if acquireErr != nil {
			t.Fatalf("prewarm acquire connection %d: %v", i, acquireErr)
		}
		warm = append(warm, conn)
	}
	for _, conn := range warm {
		conn.Release()
	}

	// Repeat independent bursts, each with its own fresh incident_key whose id is
	// derived exactly as correlation.go does (a deterministic bijection of
	// incident_key) so the burst reproduces the live dual-unique-constraint
	// collision rather than an artificial one.
	stamp := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
	for round := 0; round < rounds; round++ {
		key := fmt.Sprintf("incident-key:%s:race:%d", stamp, round)
		id := "inc_" + strings.TrimPrefix(hashParts("incident", key), "sha256:")
		now := time.Now().UTC()

		buildIncident := func() Incident {
			return Incident{
				ID:                id,
				IncidentKey:       key,
				State:             IncidentObserving,
				Title:             "checkout-api degraded",
				Subject:           "checkout-api",
				Service:           "checkout-api",
				Severity:          SeverityHigh,
				Domain:            DomainOps,
				Intent:            IntentInvestigate,
				RiskLevel:         RiskMedium,
				FirstEventAt:      now,
				LastEventAt:       now,
				PersistenceCount:  1,
				SourceInstanceIDs: []string{"race-src"},
				StateReason:       "created from concurrent same-key race regression",
				RedactionState:    map[string]any{},
				CreatedAt:         now,
				UpdatedAt:         now,
			}
		}

		start := make(chan struct{})
		var (
			wg   sync.WaitGroup
			mu   sync.Mutex
			errs []error
		)
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				<-start // barrier: block until every goroutine is ready
				// NotificationID points at the (non-existent) incident id purely to
				// satisfy the link parameter; UpsertIncident's incident_events insert
				// is best-effort (ON CONFLICT DO NOTHING, result discarded), so its FK
				// outcome does not affect the incident-upsert error under test.
				_, upsertErr := store.UpsertIncident(ctx, buildIncident(), IncidentEventLink{
					IncidentID:       id,
					NotificationID:   id,
					CorrelationKind:  CorrelationSameSubject,
					CorrelationScore: 0.75,
					Rationale:        "concurrent same-key race regression",
					CreatedAt:        now,
				})
				if upsertErr != nil {
					mu.Lock()
					errs = append(errs, upsertErr)
					mu.Unlock()
				}
			}()
		}
		close(start) // release all goroutines simultaneously
		wg.Wait()

		if len(errs) != 0 {
			t.Fatalf("round %d/%d: concurrent same-key upsert burst returned %d/%d error(s); first: %v", round+1, rounds, len(errs), workers, errs[0])
		}

		var rowCount int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_incidents WHERE incident_key = $1", key).Scan(&rowCount); err != nil {
			t.Fatalf("round %d/%d: count incident rows: %v", round+1, rounds, err)
		}
		if rowCount != 1 {
			t.Fatalf("round %d/%d: expected exactly 1 incident row for key %q, got %d", round+1, rounds, key, rowCount)
		}

		stored, err := store.GetIncidentByKey(ctx, key)
		if err != nil {
			t.Fatalf("round %d/%d: get incident by key: %v", round+1, rounds, err)
		}
		if stored.PersistenceCount != workers {
			t.Fatalf("round %d/%d: persistence_count = %d, want %d (one logical upsert per concurrent request)", round+1, rounds, stored.PersistenceCount, workers)
		}
	}
}
