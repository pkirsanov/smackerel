//go:build integration

// BUG-013-001 — F1 regression test (sweep-2026-05-25-r10 round 5 /
// stabilize-to-doc).
//
// Validates that HospitalityLinker counter mutations
// (GuestRepository.IncrementStay, PropertyRepository.IncrementBookings,
// PropertyRepository.UpdateIssueCount) are idempotent across NATS
// redelivery of the same artifacts.processed payload.
//
// Before the fix the linker called IncrementStay / IncrementBookings /
// UpdateIssueCount unconditionally on every LinkArtifact invocation.
// The artifacts.processed consumer
// (internal/pipeline/subscriber.go) runs with
// AckPolicy=AckExplicitPolicy, AckWait=30s, and MaxDeliver>1, so any of:
//   - AckWait expiry during slow DB latency
//   - Process crash between Processor.HandleProcessedResult's
//     UPDATE artifacts ... and msg.Ack()
//   - Handler-side transient error producing a Nak (retried against
//     MaxDeliver before dead-letter)
// drives a second pass through HandleProcessedResult →
// HospitalityLinker.LinkArtifact for the SAME artifact_id, drifting
// guests.total_stays / guests.total_spend and
// properties.total_bookings / properties.total_revenue /
// properties.issue_count upward (or downward, for issue_count when
// status="completed") on each redelivery.
//
// The fix introduces hospitality_counter_applications keyed on
// (artifact_id, op_kind) and gates each counter mutation behind a
// claim via INSERT … ON CONFLICT DO NOTHING. Edge writes are
// untouched because they were already idempotent via ON CONFLICT
// DO UPDATE on the edges unique constraint.
//
// This test exercises both the booking path (guest_stay_increment +
// property_booking_increment) and the task path (property_issue_delta)
// by invoking linker.LinkArtifact twice for the same artifact id and
// asserting the counters are applied exactly once. With the pre-fix
// code the assertion fails because counters are 2× the expected value.
//
// No t.Skip — when DATABASE_URL is unset the test fatals with an
// actionable message (mirrors the BUG-027-002 race-test no-skip
// precedent).

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/graph"
)

func hospitalityIdempotencyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("hospitality counter idempotency integration test requires DATABASE_URL — run via `./smackerel.sh test integration` which brings up the live test stack and exports DATABASE_URL")
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping DATABASE_URL: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// insertHospitalityArtifact writes an artifact row with a known
// content_raw payload and registers a Cleanup that removes the
// artifact plus any rows the linker created from it.
func insertHospitalityArtifact(t *testing.T, pool *pgxpool.Pool, artifactID, artifactType, contentRaw string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (
			id, artifact_type, title, content_hash, source_id, content_raw
		) VALUES (
			$1, $2, 'BUG-013-001 idempotency fixture', $3, 'guesthost', $4
		)
	`, artifactID, artifactType, artifactID+"-hash", contentRaw)
	if err != nil {
		t.Fatalf("insert hospitality fixture artifact: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Remove the dedup ledger entries first so a re-run of the
		// test against the same DB doesn't leak rows.
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM hospitality_counter_applications WHERE artifact_id = $1`, artifactID); err != nil {
			t.Logf("cleanup hospitality_counter_applications %s: %v", artifactID, err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM edges WHERE src_id = $1 OR dst_id = $1`, artifactID); err != nil {
			t.Logf("cleanup edges for artifact %s: %v", artifactID, err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM artifacts WHERE id = $1`, artifactID); err != nil {
			t.Logf("cleanup artifact %s: %v", artifactID, err)
		}
	})
}

// insertHospitalityGuest creates a guest row with the supplied email
// and returns nothing — the linker will look it up via UpsertByEmail.
// The test asserts post-state on this row.
func ensureGuestCleanup(t *testing.T, pool *pgxpool.Pool, email string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM guests WHERE email = $1 AND source = 'guesthost'`, email); err != nil {
			t.Logf("cleanup guest %s: %v", email, err)
		}
	})
}

func ensurePropertyCleanup(t *testing.T, pool *pgxpool.Pool, externalID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM properties WHERE external_id = $1 AND source = 'guesthost'`, externalID); err != nil {
			t.Logf("cleanup property %s: %v", externalID, err)
		}
	})
}

func readGuestCounters(t *testing.T, pool *pgxpool.Pool, email string) (totalStays int, totalSpend float64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.QueryRow(ctx, `
		SELECT total_stays, total_spend
		FROM guests
		WHERE email = $1 AND source = 'guesthost'
	`, email).Scan(&totalStays, &totalSpend); err != nil {
		t.Fatalf("read guest counters: %v", err)
	}
	return
}

func readPropertyCounters(t *testing.T, pool *pgxpool.Pool, externalID string) (totalBookings int, totalRevenue float64, issueCount int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.QueryRow(ctx, `
		SELECT total_bookings, total_revenue, issue_count
		FROM properties
		WHERE external_id = $1 AND source = 'guesthost'
	`, externalID).Scan(&totalBookings, &totalRevenue, &issueCount); err != nil {
		t.Fatalf("read property counters: %v", err)
	}
	return
}

func newHospitalityLinker(pool *pgxpool.Pool) *graph.HospitalityLinker {
	guestRepo := db.NewGuestRepository(pool)
	propertyRepo := db.NewPropertyRepository(pool)
	baseLinker := graph.NewLinker(pool)
	return graph.NewHospitalityLinker(guestRepo, propertyRepo, pool, baseLinker)
}

// TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery asserts that
// a second LinkArtifact call against the same booking artifact does NOT
// double-count guests.total_stays / guests.total_spend or
// properties.total_bookings / properties.total_revenue.
func TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery(t *testing.T) {
	pool := hospitalityIdempotencyPool(t)
	defer pool.Close()

	artifactID := "art-booking-" + uuid.NewString()
	guestEmail := "idem-guest-" + uuid.NewString() + "@example.test"
	propertyExternalID := "idem-property-" + uuid.NewString()

	contentRaw := `{
		"propertyId":"` + propertyExternalID + `",
		"propertyName":"Idempotency Test Property",
		"guestEmail":"` + guestEmail + `",
		"guestName":"Idempotency Test Guest",
		"bookingId":"b-` + propertyExternalID + `",
		"checkinDate":"2026-06-01",
		"checkoutDate":"2026-06-05",
		"totalAmount":1234.56,
		"status":"confirmed"
	}`

	insertHospitalityArtifact(t, pool, artifactID, "booking", contentRaw)
	ensureGuestCleanup(t, pool, guestEmail)
	ensurePropertyCleanup(t, pool, propertyExternalID)

	linker := newHospitalityLinker(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First delivery — counters MUST be applied once.
	if err := linker.LinkArtifact(ctx, artifactID); err != nil {
		t.Fatalf("first LinkArtifact: %v", err)
	}
	stays1, spend1 := readGuestCounters(t, pool, guestEmail)
	bookings1, revenue1, _ := readPropertyCounters(t, pool, propertyExternalID)
	if stays1 != 1 {
		t.Fatalf("after first delivery: guests.total_stays = %d, want 1", stays1)
	}
	if spend1 < 1234.55 || spend1 > 1234.57 {
		t.Fatalf("after first delivery: guests.total_spend = %f, want ~1234.56", spend1)
	}
	if bookings1 != 1 {
		t.Fatalf("after first delivery: properties.total_bookings = %d, want 1", bookings1)
	}
	if revenue1 < 1234.55 || revenue1 > 1234.57 {
		t.Fatalf("after first delivery: properties.total_revenue = %f, want ~1234.56", revenue1)
	}

	// Second delivery (simulates NATS redelivery: AckWait expiry,
	// crash-before-ack, or transient handler-error Nak retry).
	// Counters MUST NOT change.
	if err := linker.LinkArtifact(ctx, artifactID); err != nil {
		t.Fatalf("second LinkArtifact (redelivery): %v", err)
	}
	stays2, spend2 := readGuestCounters(t, pool, guestEmail)
	bookings2, revenue2, _ := readPropertyCounters(t, pool, propertyExternalID)
	if stays2 != 1 {
		t.Fatalf("after redelivery: guests.total_stays = %d, want 1 (counter drifted)", stays2)
	}
	if spend2 < 1234.55 || spend2 > 1234.57 {
		t.Fatalf("after redelivery: guests.total_spend = %f, want ~1234.56 (counter drifted)", spend2)
	}
	if bookings2 != 1 {
		t.Fatalf("after redelivery: properties.total_bookings = %d, want 1 (counter drifted)", bookings2)
	}
	if revenue2 < 1234.55 || revenue2 > 1234.57 {
		t.Fatalf("after redelivery: properties.total_revenue = %f, want ~1234.56 (counter drifted)", revenue2)
	}

	// Third delivery to prove the dedup ledger is durable across N
	// redeliveries, not just two.
	if err := linker.LinkArtifact(ctx, artifactID); err != nil {
		t.Fatalf("third LinkArtifact (redelivery): %v", err)
	}
	stays3, _ := readGuestCounters(t, pool, guestEmail)
	bookings3, _, _ := readPropertyCounters(t, pool, propertyExternalID)
	if stays3 != 1 || bookings3 != 1 {
		t.Fatalf("after third redelivery: guests.total_stays=%d properties.total_bookings=%d, want 1/1 (durability lost)", stays3, bookings3)
	}
}

// TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery asserts that
// the task-status delta applied to properties.issue_count is also idempotent
// across redelivery.
func TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery(t *testing.T) {
	pool := hospitalityIdempotencyPool(t)
	defer pool.Close()

	propertyExternalID := "idem-task-property-" + uuid.NewString()
	artifactID := "art-task-" + uuid.NewString()

	// Status "open" — delta = +1
	contentRaw := `{
		"propertyId":"` + propertyExternalID + `",
		"propertyName":"Task Property",
		"status":"open"
	}`

	insertHospitalityArtifact(t, pool, artifactID, "task", contentRaw)
	ensurePropertyCleanup(t, pool, propertyExternalID)

	linker := newHospitalityLinker(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := linker.LinkArtifact(ctx, artifactID); err != nil {
		t.Fatalf("first LinkArtifact: %v", err)
	}
	_, _, issue1 := readPropertyCounters(t, pool, propertyExternalID)
	if issue1 != 1 {
		t.Fatalf("after first delivery: properties.issue_count = %d, want 1", issue1)
	}

	// Redelivery 1 — must NOT increment
	if err := linker.LinkArtifact(ctx, artifactID); err != nil {
		t.Fatalf("second LinkArtifact (redelivery): %v", err)
	}
	_, _, issue2 := readPropertyCounters(t, pool, propertyExternalID)
	if issue2 != 1 {
		t.Fatalf("after redelivery: properties.issue_count = %d, want 1 (delta drifted)", issue2)
	}

	// Redelivery 2 — still must NOT increment
	if err := linker.LinkArtifact(ctx, artifactID); err != nil {
		t.Fatalf("third LinkArtifact (redelivery): %v", err)
	}
	_, _, issue3 := readPropertyCounters(t, pool, propertyExternalID)
	if issue3 != 1 {
		t.Fatalf("after third redelivery: properties.issue_count = %d, want 1 (durability lost)", issue3)
	}
}

// TestHospitalityLinker_DifferentArtifactsApplyIndependently asserts that the
// idempotency ledger does NOT block legitimate multi-artifact accumulation —
// two distinct booking artifacts for the same guest must still result in
// total_stays == 2 even though each artifact_id is only applied once.
func TestHospitalityLinker_DifferentArtifactsApplyIndependently(t *testing.T) {
	pool := hospitalityIdempotencyPool(t)
	defer pool.Close()

	guestEmail := "idem-multi-guest-" + uuid.NewString() + "@example.test"
	propertyExternalID := "idem-multi-property-" + uuid.NewString()

	ensureGuestCleanup(t, pool, guestEmail)
	ensurePropertyCleanup(t, pool, propertyExternalID)

	linker := newHospitalityLinker(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		artifactID := "art-multi-booking-" + uuid.NewString()
		contentRaw := `{
			"propertyId":"` + propertyExternalID + `",
			"propertyName":"Multi Test Property",
			"guestEmail":"` + guestEmail + `",
			"guestName":"Multi Test Guest",
			"bookingId":"` + artifactID + `",
			"totalAmount":100.0,
			"status":"confirmed"
		}`
		insertHospitalityArtifact(t, pool, artifactID, "booking", contentRaw)

		if err := linker.LinkArtifact(ctx, artifactID); err != nil {
			t.Fatalf("LinkArtifact #%d: %v", i, err)
		}
		// Redeliver the same artifact a second time to prove only one
		// counter application happens per distinct artifact.
		if err := linker.LinkArtifact(ctx, artifactID); err != nil {
			t.Fatalf("LinkArtifact #%d redelivery: %v", i, err)
		}
	}

	stays, spend := readGuestCounters(t, pool, guestEmail)
	bookings, revenue, _ := readPropertyCounters(t, pool, propertyExternalID)
	if stays != 3 {
		t.Fatalf("after 3 distinct bookings: guests.total_stays = %d, want 3 (dedup over-blocked)", stays)
	}
	if spend < 299.99 || spend > 300.01 {
		t.Fatalf("after 3 distinct bookings: guests.total_spend = %f, want ~300.0 (dedup over-blocked)", spend)
	}
	if bookings != 3 {
		t.Fatalf("after 3 distinct bookings: properties.total_bookings = %d, want 3 (dedup over-blocked)", bookings)
	}
	if revenue < 299.99 || revenue > 300.01 {
		t.Fatalf("after 3 distinct bookings: properties.total_revenue = %f, want ~300.0 (dedup over-blocked)", revenue)
	}
}
