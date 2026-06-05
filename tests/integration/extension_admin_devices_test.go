//go:build integration

// Spec 058 BUG-058 BLOCKER-2 (closure subset) — live-Postgres regression
// proof for `extensiondevices.PostgresStore.AggregateDevices`.
//
// Scope 5 ships GET /v1/admin/extension/devices, which aggregates
// raw_ingest_dedup rows by (owner_user_id, source_device_id) and
// returns:
//   - first_seen_at (MIN across the group)
//   - last_seen_at  (MAX across the group)
//   - visit_count_30d (SUM of visit_count where last_seen_at >= now-30d)
//
// Production safety contracts (design §3.2 + handler code):
//   - source_id is PINNED to 'browser-extension' so other future
//     ingest paths cannot pollute the admin view.
//   - When ownerUserIDFilter != "", non-admin callers see ONLY their
//     own owner_user_id rows.
//   - Rows older than 30 days still contribute MIN/MAX but their
//     visit_count is excluded from the 30-day window total.
//
// Adversarial cover:
//   - A row with source_id='bookmarks' MUST NOT appear in the view.
//   - A row whose last_seen_at is older than 30d contributes
//     first_seen_at/last_seen_at but visit_count_30d=0.
//   - Owner filter MUST NOT leak rows whose owner_user_id differs.
//   - Two rows with same (owner_user_id, source_device_id) and
//     different first_seen_at must collapse into one aggregated row
//     whose first_seen_at = the older value.
//
// Discharges BUG-058-EXTERNAL-INFRA-MISSING Blocker 2 (Scope 5 admin
// devices view live aggregation) deferred row.

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api/admin/extensiondevices"
)

// TestExtensionDevices_AggregateDevices_LiveAggregationRespectsContract
// is the multi-row aggregation regression: SUM/MIN/MAX + source_id
// pinning + 30d window all evaluated against live Postgres.
func TestExtensionDevices_AggregateDevices_LiveAggregationRespectsContract(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	owner := "owner-" + tid
	now := time.Now().UTC().Truncate(time.Second)

	// Pin the store's clock so the 30-day window is deterministic.
	store := extensiondevices.NewPostgresStore(pool).WithNow(func() time.Time { return now })

	// Seed fixtures. Each row uses a deterministic dedup_key derived
	// from the test id so cleanup deletes only this test's rows.
	fixtures := []struct {
		name           string
		ownerUserID    string
		sourceID       string
		sourceDeviceID string
		visitCount     int
		firstSeen      time.Time
		lastSeen       time.Time
	}{
		// Device A, owner-test, 2 rows aggregating to visit_count_30d=15
		{name: "A-day3", ownerUserID: owner, sourceID: "browser-extension",
			sourceDeviceID: "device-A", visitCount: 5,
			firstSeen: now.AddDate(0, 0, -10), lastSeen: now.AddDate(0, 0, -3)},
		{name: "A-day1", ownerUserID: owner, sourceID: "browser-extension",
			sourceDeviceID: "device-A", visitCount: 10,
			firstSeen: now.AddDate(0, 0, -5), lastSeen: now.AddDate(0, 0, -1)},

		// Device B, owner-test, one row at 45d ago — MIN/MAX must
		// still include it but visit_count_30d must be 0.
		{name: "B-day45-stale", ownerUserID: owner, sourceID: "browser-extension",
			sourceDeviceID: "device-B", visitCount: 99,
			firstSeen: now.AddDate(0, 0, -50), lastSeen: now.AddDate(0, 0, -45)},

		// Device C, ANOTHER owner — must appear with filter="" but
		// must be excluded when filter=owner.
		{name: "C-otherowner", ownerUserID: "owner-other-" + tid,
			sourceID: "browser-extension", sourceDeviceID: "device-C",
			visitCount: 7,
			firstSeen:  now.AddDate(0, 0, -2), lastSeen: now.AddDate(0, 0, -1)},

		// Device D, owner-test but source_id != 'browser-extension'
		// — MUST NOT appear in either query.
		{name: "D-bookmarks-leak", ownerUserID: owner, sourceID: "bookmarks",
			sourceDeviceID: "device-D", visitCount: 1000,
			firstSeen: now.AddDate(0, 0, -1), lastSeen: now},
	}

	insertedKeys := make([][]byte, 0, len(fixtures))
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		for _, k := range insertedKeys {
			_, _ = pool.Exec(cctx, `DELETE FROM raw_ingest_dedup WHERE dedup_key = $1`, k)
		}
	})

	for _, f := range fixtures {
		key := sha256Of(fmt.Sprintf("bug058-aggregate-%s-%s", tid, f.name))
		insertedKeys = append(insertedKeys, key)
		// artifact_id is just a synthetic string here — the view
		// query never JOINs to artifacts, so no FK seeding is needed.
		if _, err := pool.Exec(ctx, `
			INSERT INTO raw_ingest_dedup
				(dedup_key, owner_user_id, source_id, content_type,
				 source_device_id, artifact_id, first_seen_at, last_seen_at, visit_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, key, f.ownerUserID, f.sourceID, "browser_history_visit",
			f.sourceDeviceID, "synthetic-"+f.name, f.firstSeen, f.lastSeen, f.visitCount); err != nil {
			t.Fatalf("seed fixture %s: %v", f.name, err)
		}
	}

	// Query 1: filter empty → admin view, all owners.
	allDevices, err := store.AggregateDevices(ctx, "")
	if err != nil {
		t.Fatalf("AggregateDevices(all): %v", err)
	}

	gotA := pickDevice(allDevices, owner, "device-A")
	gotB := pickDevice(allDevices, owner, "device-B")
	gotC := pickDevice(allDevices, "owner-other-"+tid, "device-C")
	gotD := pickDevice(allDevices, owner, "device-D")

	if gotA == nil {
		t.Fatal("device-A (owner-test, browser-extension) missing from admin view")
	}
	if gotA.VisitCount30d != 15 {
		t.Fatalf("device-A visit_count_30d: got %d want 15 (5+10 within window)", gotA.VisitCount30d)
	}
	// MIN of -10d and -5d must be -10d.
	wantAFirst := now.AddDate(0, 0, -10)
	if !gotA.FirstSeenAt.Equal(wantAFirst) {
		t.Fatalf("device-A first_seen_at: got %v want %v", gotA.FirstSeenAt, wantAFirst)
	}
	// MAX of -3d and -1d must be -1d.
	wantALast := now.AddDate(0, 0, -1)
	if !gotA.LastSeenAt.Equal(wantALast) {
		t.Fatalf("device-A last_seen_at: got %v want %v", gotA.LastSeenAt, wantALast)
	}

	if gotB == nil {
		t.Fatal("device-B (45d stale) MUST still appear in the admin view (MIN/MAX) even when its rows are outside the 30d window")
	}
	if gotB.VisitCount30d != 0 {
		t.Fatalf("device-B visit_count_30d: got %d want 0 (single row 45d old must be excluded from the 30d sum)", gotB.VisitCount30d)
	}

	if gotC == nil {
		t.Fatal("device-C (owner-other) MUST appear when filter is empty (admin view)")
	}
	if gotC.VisitCount30d != 7 {
		t.Fatalf("device-C visit_count_30d: got %d want 7", gotC.VisitCount30d)
	}

	if gotD != nil {
		t.Fatalf("device-D (source_id=bookmarks) MUST NOT appear — admin view is pinned to source_id='browser-extension'. Got %+v", gotD)
	}

	// Query 2: filter to owner-test → must scope rows correctly.
	scoped, err := store.AggregateDevices(ctx, owner)
	if err != nil {
		t.Fatalf("AggregateDevices(filter=%q): %v", owner, err)
	}

	if pickDevice(scoped, "owner-other-"+tid, "device-C") != nil {
		t.Fatal("owner-filter leak: owner-other's device-C appeared in owner-test's scoped view")
	}
	if pickDevice(scoped, owner, "device-D") != nil {
		t.Fatal("source_id leak under owner filter: device-D (bookmarks) appeared in scoped view")
	}
	if pickDevice(scoped, owner, "device-A") == nil {
		t.Fatal("owner filter wrongly excluded device-A (owner matches)")
	}
	if pickDevice(scoped, owner, "device-B") == nil {
		t.Fatal("owner filter wrongly excluded device-B (owner matches, 45d-stale row should still show)")
	}
}

func pickDevice(devices []extensiondevices.Device, owner, deviceID string) *extensiondevices.Device {
	for i := range devices {
		if devices[i].OwnerUserID == owner && devices[i].SourceDeviceID == deviceID {
			return &devices[i]
		}
	}
	return nil
}
