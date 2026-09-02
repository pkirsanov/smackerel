//go:build stress

// Package stress contains spec 041 Scope 2 stress profile for the
// QF Companion Connector.
//
// SCN-SM-041-005: Repeated QF cursor pages MUST NOT duplicate packet IDs or
// lose trace metadata. The stress profile drives the connector through many
// rapid Sync() cycles against an httptest QF bridge that paginates a stable
// catalog of decision events, and asserts that the persisted artifacts on the
// live PostgreSQL stack remain identity-stable (one row per packet_id, trace
// metadata preserved verbatim).
package stress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// The repeated-cursor packet-identity stress regression drives a long burst
// of sync cycles against a fake QF bridge that returns
// the same catalog of packets across many cursor pages and replays. The live
// PostgreSQL row count for the unique test source MUST stay equal to the
// number of distinct QF packet IDs, and trace_id metadata MUST be preserved
// for every persisted artifact across every cycle.
func TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("stress: DATABASE_URL not set — live stack DB is required")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Fatal("stress: NATS_URL not set — live stack NATS is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect stress database: %v", err)
	}
	t.Cleanup(pool.Close)

	natsClient, err := smacknats.Connect(ctx, natsURL, cfg.AuthToken)
	if err != nil {
		t.Fatalf("connect stress NATS: %v", err)
	}
	t.Cleanup(natsClient.Close)

	sourceID := fmt.Sprintf("qf-decisions-stress-%d", time.Now().UnixNano())
	t.Cleanup(func() { qfDecisionsStressCleanup(t, pool, sourceID) })
	qfDecisionsStressCleanup(t, pool, sourceID)

	// Fixture: 8 distinct packets distributed across 4 cursor pages.
	const totalPackets = 8
	const packetsPerPage = 2
	packetIDs := make([]string, 0, totalPackets)
	traceIDs := make(map[string]string, totalPackets)
	envelopes := make(map[string]qfdecisions.QFDecisionPacketEnvelope, totalPackets)
	pages := make(map[string][]qfdecisions.QFDecisionEvent)
	cursorOrder := []string{"", "qf-page-2", "qf-page-3", "qf-page-4"}
	runTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < totalPackets; i++ {
		pid := fmt.Sprintf("packet-stress-%d", i)
		tid := fmt.Sprintf("trace-stress-%d", i)
		packetIDs = append(packetIDs, pid)
		traceIDs[pid] = tid
		envelopes[pid] = stressEnvelope(pid, tid, runTimestamp)
		pageKey := cursorOrder[i/packetsPerPage]
		pages[pageKey] = append(pages[pageKey], stressEvent(pid, tid, i, runTimestamp))
	}

	var eventCalls atomic.Int32
	var packetCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			// Round 2N capability handshake — Connect() probes this path
			// before Sync() is permitted. Canonical shape mirrors the
			// resolved C-S2-006-E2E-STUB-ARM fix at
			// tests/e2e/qf_decisions_connector_api_test.go (case
			// qfdecisions.CapabilitiesPath) so CompatibilityCheck()
			// passes: audit_envelope_version=v1, packet_version v1, the
			// three required decision_types, and a valid [min,max]
			// page-size range that admits the stress page_size of 2.
			_ = json.NewEncoder(w).Encode(qfdecisions.QFBridgeCapability{
				SupportedPacketVersions:        []string{"v1"},
				SupportedEventTypes:            []string{"packet_created", "packet_updated", "packet_trust_changed", "packet_archived", "packet_action_boundary_attempted"},
				SupportedDecisionTypes:         []string{"recommendation", "policy_denial", "analysis_note"},
				MaxPageSize:                    100,
				MinPageSize:                    1,
				SupportedTargetContextTypes:    []string{"trip"},
				EvidenceMaxBundleSizeBytes:     1048576,
				EvidenceMaxClaimsPerBundle:     50,
				EvidenceRateLimitPerMinute:     60,
				FreshnessSLAP95Seconds:         60,
				AuditEnvelopeVersion:           "v1",
				WatchSignalDirection:           "qf_to_smackerel",
				EligibleSmackerelSourceClasses: []string{"watch"},
			})
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			eventCalls.Add(1)
			cursor := r.URL.Query().Get("cursor")
			events := pages[cursor]
			next := nextCursorAfter(cursorOrder, cursor)
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     events,
				NextCursor: next,
				HasMore:    next != cursor,
				ServerTime: time.Now().UTC().Format(time.RFC3339),
			})
		case strings.HasPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/"):
			packetCalls.Add(1)
			packetID := strings.TrimPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/")
			env, ok := envelopes[packetID]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(env)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       server.URL,
			"packet_version": 1,
			"page_size":      packetsPerPage,
		},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	stateStore := connector.NewStateStore(pool)
	publisher := pipeline.NewRawArtifactPublisher(pool, natsClient)

	// Drive the full pagination 6 times back-to-back; clear cursor between
	// "outer" cycles so we exercise both fresh sync and replay paths.
	const outerCycles = 6
	for cycle := 0; cycle < outerCycles; cycle++ {
		cursor := ""
		for safety := 0; safety < len(cursorOrder)+2; safety++ {
			artifacts, nextCursor, err := conn.Sync(ctx, cursor)
			if err != nil {
				t.Fatalf("cycle %d page %s Sync: %v", cycle, cursor, err)
			}
			for _, art := range artifacts {
				if art.SourceID != sourceID {
					t.Fatalf("cycle %d packet %s SourceID drift: got %q, want %q",
						cycle, art.SourceRef, art.SourceID, sourceID)
				}
				if got, ok := art.Metadata["trace_id"].(string); !ok || got != traceIDs[art.SourceRef] {
					t.Fatalf("cycle %d packet %s trace_id metadata drift: got %v, want %q",
						cycle, art.SourceRef, art.Metadata["trace_id"], traceIDs[art.SourceRef])
				}
				if _, pubErr := publisher.PublishRawArtifact(ctx, art); pubErr != nil {
					t.Fatalf("cycle %d packet %s PublishRawArtifact: %v", cycle, art.SourceRef, pubErr)
				}
			}
			if err := stateStore.Save(ctx, &connector.SyncState{
				SourceID:    sourceID,
				Enabled:     true,
				SyncCursor:  nextCursor,
				ItemsSynced: len(artifacts),
			}); err != nil {
				t.Fatalf("cycle %d page %s state save: %v", cycle, cursor, err)
			}
			if nextCursor == cursor || nextCursor == "" {
				break
			}
			cursor = nextCursor
		}
	}

	persistedRows := stressCountArtifactsBySourceURL(t, ctx, pool, sourceID)
	if persistedRows != totalPackets {
		t.Fatalf("persisted artifact rows for %s = %d, want %d (cursor replay must NOT duplicate packet identity)",
			sourceID, persistedRows, totalPackets)
	}

	for _, pid := range packetIDs {
		traceID, ok := stressLookupTraceForPacket(t, ctx, pool, sourceID, "https://qf.example.test/packets/"+pid)
		if !ok {
			t.Fatalf("packet %s missing from persisted artifacts", pid)
		}
		if traceID != traceIDs[pid] {
			t.Fatalf("packet %s trace_id drift: got %q, want %q", pid, traceID, traceIDs[pid])
		}
	}

	if eventCalls.Load() < int32(outerCycles) {
		t.Fatalf("event endpoint calls = %d, want >= %d", eventCalls.Load(), outerCycles)
	}
	if packetCalls.Load() < int32(outerCycles*totalPackets) {
		t.Fatalf("packet endpoint calls = %d, want >= %d", packetCalls.Load(), outerCycles*totalPackets)
	}
}

func TestQFDecisionsStressCleanupChildReturnsZeroOwnedRows(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("stress: DATABASE_URL not set — live stack DB is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect cleanup regression database: %v", err)
	}
	t.Cleanup(pool.Close)

	runID := time.Now().UnixNano()
	ownedSourceID := fmt.Sprintf("test-qf-cleanup-owned-%d", runID)
	ownedArtifactIDs := [2]string{
		fmt.Sprintf("test-qf-cleanup-owned-artifact-a-%d", runID),
		fmt.Sprintf("test-qf-cleanup-owned-artifact-b-%d", runID),
	}
	controlSourceID := fmt.Sprintf("test-qf-cleanup-control-%d", runID)
	controlArtifactID := fmt.Sprintf("test-qf-cleanup-control-artifact-%d", runID)
	t.Cleanup(func() { qfDecisionsStressCleanup(t, pool, controlSourceID) })

	if err := qfDecisionsStressSeedCleanupFixture(
		ctx,
		pool,
		controlSourceID,
		[]string{controlArtifactID},
		fmt.Sprintf("test-qf-cleanup-external-node-%d", runID),
	); err != nil {
		t.Fatalf("seed unowned cleanup control: %v", err)
	}

	if ok := t.Run("owned-child", func(t *testing.T) {
		t.Cleanup(func() { qfDecisionsStressCleanup(t, pool, ownedSourceID) })
		if err := qfDecisionsStressSeedCleanupFixture(
			ctx,
			pool,
			ownedSourceID,
			ownedArtifactIDs[:],
			controlArtifactID,
		); err != nil {
			t.Fatalf("seed source-owned cleanup fixture: %v", err)
		}
	}); !ok {
		t.Fatal("owned child cleanup regression failed before parent verification")
	}

	ownedCounts, err := qfDecisionsStressCleanupRowCounts(
		ctx,
		pool,
		ownedSourceID,
		ownedArtifactIDs[0],
		ownedArtifactIDs[1],
	)
	if err != nil {
		t.Fatalf("count source-owned rows after child cleanup: %v", err)
	}
	if ownedCounts != (qfDecisionsStressCleanupCounts{}) {
		t.Fatalf("source-owned rows survived child cleanup: %+v", ownedCounts)
	}

	controlCounts, err := qfDecisionsStressCleanupRowCounts(
		ctx,
		pool,
		controlSourceID,
		controlArtifactID,
		controlArtifactID,
	)
	if err != nil {
		t.Fatalf("count unowned control rows after child cleanup: %v", err)
	}
	wantControlCounts := qfDecisionsStressCleanupCounts{
		Artifacts:   1,
		Annotations: 1,
		Edges:       1,
		SyncState:   1,
	}
	if controlCounts != wantControlCounts {
		t.Fatalf("unowned control rows changed during child cleanup: got %+v, want %+v", controlCounts, wantControlCounts)
	}
}

func TestQFDecisionsStressCleanupReturnsErrorForClosedPool(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("stress: DATABASE_URL not set — live stack DB is required")
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("create cleanup regression pool: %v", err)
	}
	pool.Close()

	sourceID := fmt.Sprintf("test-qf-cleanup-closed-pool-%d", time.Now().UnixNano())
	cleanupErr := qfDecisionsStressCleanupSource(pool, sourceID)
	if cleanupErr == nil {
		t.Fatal("cleanup with a closed pool returned nil, want contextual error")
	}
	if !strings.Contains(cleanupErr.Error(), "begin cleanup transaction") {
		t.Fatalf("cleanup with a closed pool returned %q, want begin-transaction context", cleanupErr)
	}
}

func stressEnvelope(packetID, traceID, createdAt string) qfdecisions.QFDecisionPacketEnvelope {
	return qfdecisions.QFDecisionPacketEnvelope{
		ContractVersion:      1,
		PacketID:             packetID,
		IntentID:             "intent-" + packetID,
		ScenarioID:           "scenario-" + packetID,
		TraceID:              traceID,
		Thesis:               "QF stress thesis " + packetID,
		WhyNow:               "QF stress timing " + packetID,
		QuantifiedImpact:     map[string]any{"unit": "bps", "value": 5.0},
		ExpertAnalysisBundle: map[string]any{"ref": "qf-stress-" + packetID},
		CalibrationBadge:     map[string]any{"state": "calibrated"},
		DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
		ApprovalState:        "display_only",
		DeepLink:             "https://qf.example.test/packets/" + packetID,
		PacketVersion:        1,
		DecisionType:         qfdecisions.DecisionTypeRecommendation,
		CreatedAt:            createdAt,
		UpdatedAt:            createdAt,
	}
}

func stressEvent(packetID, traceID string, i int, createdAt string) qfdecisions.QFDecisionEvent {
	return qfdecisions.QFDecisionEvent{
		ContractVersion: 1,
		EventID:         fmt.Sprintf("event-stress-%d", i),
		PacketID:        packetID,
		IntentID:        "intent-" + packetID,
		ScenarioID:      "scenario-" + packetID,
		TraceID:         traceID,
		EventType:       "packet_created",
		DecisionType:    qfdecisions.DecisionTypeRecommendation,
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          fmt.Sprintf("event-checkpoint-%d", i),
		PacketURL:       "https://qf.example.test/packets/" + packetID,
		CreatedAt:       createdAt,
	}
}

func nextCursorAfter(order []string, current string) string {
	for i, c := range order {
		if c == current {
			if i+1 < len(order) {
				return order[i+1]
			}
			return current // terminal page repeats itself
		}
	}
	return current
}

func stressCountArtifactsBySourceURL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sourceID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(DISTINCT source_url) FROM artifacts WHERE source_id = $1`, sourceID).Scan(&count); err != nil {
		t.Fatalf("count distinct source_url for %s: %v", sourceID, err)
	}
	return count
}

func stressLookupTraceForPacket(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sourceID, sourceURL string) (string, bool) {
	t.Helper()
	var contentRaw string
	err := pool.QueryRow(ctx, `SELECT content_raw FROM artifacts WHERE source_id = $1 AND source_url = $2 LIMIT 1`, sourceID, sourceURL).Scan(&contentRaw)
	if err != nil {
		return "", false
	}
	var envelope qfdecisions.QFDecisionPacketEnvelope
	if err := json.Unmarshal([]byte(contentRaw), &envelope); err != nil {
		t.Fatalf("decode persisted envelope for %s: %v", sourceURL, err)
	}
	return envelope.TraceID, true
}

func qfDecisionsStressCleanup(t *testing.T, pool *pgxpool.Pool, sourceID string) {
	t.Helper()
	if err := qfDecisionsStressCleanupSource(pool, sourceID); err != nil {
		t.Fatalf("cleanup QF decision stress rows for source %s: %v", sourceID, err)
	}
}

const qfDecisionsStressDeleteChildrenSQL = `
	WITH deleted_edges AS (
		DELETE FROM edges
		WHERE src_id = ANY($1::text[]) OR dst_id = ANY($1::text[])
		RETURNING id
	)
	DELETE FROM annotations
	WHERE artifact_id = ANY($1::text[])
`

func qfDecisionsStressCleanupSource(pool *pgxpool.Pool, sourceID string) (cleanupErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cleanup transaction for source %s: %w", sourceID, err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rollbackCancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil {
			cleanupErr = errors.Join(
				cleanupErr,
				fmt.Errorf("rollback cleanup transaction for source %s: %w", sourceID, rollbackErr),
			)
		}
	}()

	var artifactIDs []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(id ORDER BY id), ARRAY[]::text[])
		FROM artifacts
		WHERE source_id = $1
	`, sourceID).Scan(&artifactIDs); err != nil {
		return fmt.Errorf("load artifact IDs for source %s: %w", sourceID, err)
	}

	if _, err := tx.Exec(ctx, qfDecisionsStressDeleteChildrenSQL, artifactIDs); err != nil {
		return fmt.Errorf("delete edges for source %s: %w", sourceID, err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, sourceID); err != nil {
		return fmt.Errorf("delete artifacts for source %s: %w", sourceID, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sync_state WHERE source_id = $1`, sourceID); err != nil {
		return fmt.Errorf("delete sync state for source %s: %w", sourceID, err)
	}
	if _, err := tx.Exec(ctx, qfDecisionsStressDeleteChildrenSQL, artifactIDs); err != nil {
		return fmt.Errorf("delete children created during parent cleanup for source %s: %w", sourceID, err)
	}

	var artifacts int64
	var syncState int64
	if err := tx.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM artifacts WHERE source_id = $1),
			(SELECT COUNT(*) FROM sync_state WHERE source_id = $1)
	`, sourceID).Scan(&artifacts, &syncState); err != nil {
		return fmt.Errorf("verify parent cleanup for source %s: %w", sourceID, err)
	}
	if artifacts != 0 || syncState != 0 {
		return fmt.Errorf(
			"verify parent cleanup for source %s: artifacts=%d sync_state=%d, want zero",
			sourceID,
			artifacts,
			syncState,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cleanup transaction for source %s: %w", sourceID, err)
	}
	committed = true
	return qfDecisionsStressDrainLateChildren(pool, sourceID, artifactIDs)
}

func qfDecisionsStressDrainLateChildren(pool *pgxpool.Pool, sourceID string, artifactIDs []string) error {
	if len(artifactIDs) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	const requiredStableScans = 5
	stableScans := 0
	var childEdges int64
	var childAnnotations int64
	for stableScans < requiredStableScans {
		if _, err := pool.Exec(ctx, qfDecisionsStressDeleteChildrenSQL, artifactIDs); err != nil {
			return fmt.Errorf("drain late children for source %s: %w", sourceID, err)
		}
		if err := pool.QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*) FROM edges WHERE src_id = ANY($1::text[]) OR dst_id = ANY($1::text[])),
				(SELECT COUNT(*) FROM annotations WHERE artifact_id = ANY($1::text[]))
		`, artifactIDs).Scan(&childEdges, &childAnnotations); err != nil {
			return fmt.Errorf("verify late child cleanup for source %s: %w", sourceID, err)
		}
		if childEdges == 0 && childAnnotations == 0 {
			stableScans++
		} else {
			stableScans = 0
		}
		if stableScans == requiredStableScans {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"drain late children for source %s: edges=%d annotations=%d stable_scans=%d/%d: %w",
				sourceID,
				childEdges,
				childAnnotations,
				stableScans,
				requiredStableScans,
				ctx.Err(),
			)
		case <-ticker.C:
		}
	}
	return nil
}

type qfDecisionsStressCleanupCounts struct {
	Artifacts   int64
	Annotations int64
	Edges       int64
	SyncState   int64
}

func qfDecisionsStressCleanupRowCounts(
	ctx context.Context,
	pool *pgxpool.Pool,
	sourceID string,
	firstArtifactID string,
	secondArtifactID string,
) (qfDecisionsStressCleanupCounts, error) {
	var counts qfDecisionsStressCleanupCounts
	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM artifacts WHERE source_id = $1),
			(SELECT COUNT(*) FROM annotations WHERE artifact_id IN ($2, $3)),
			(SELECT COUNT(*) FROM edges WHERE src_id IN ($2, $3) OR dst_id IN ($2, $3)),
			(SELECT COUNT(*) FROM sync_state WHERE source_id = $1)
	`, sourceID, firstArtifactID, secondArtifactID).Scan(
		&counts.Artifacts,
		&counts.Annotations,
		&counts.Edges,
		&counts.SyncState,
	)
	if err != nil {
		return qfDecisionsStressCleanupCounts{}, fmt.Errorf("query cleanup row counts for source %s: %w", sourceID, err)
	}
	return counts, nil
}

func qfDecisionsStressSeedCleanupFixture(
	ctx context.Context,
	pool *pgxpool.Pool,
	sourceID string,
	artifactIDs []string,
	edgeDestinationID string,
) error {
	if len(artifactIDs) == 0 {
		return fmt.Errorf("seed cleanup fixture for source %s: at least one artifact ID is required", sourceID)
	}
	for _, artifactID := range artifactIDs {
		if _, err := pool.Exec(ctx, `
			INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
			VALUES ($1, 'note', $1, $2, $3)
		`, artifactID, "hash-"+artifactID, sourceID); err != nil {
			return fmt.Errorf("seed artifact %s for source %s: %w", artifactID, sourceID, err)
		}
	}

	annotationID := "annotation-" + sourceID
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotations (id, artifact_id, annotation_type, note, source_channel)
		VALUES ($1, $2, 'note', 'cleanup integrity fixture', 'api')
	`, annotationID, artifactIDs[len(artifactIDs)-1]); err != nil {
		return fmt.Errorf("seed annotation for source %s: %w", sourceID, err)
	}

	edgeID := "edge-" + sourceID
	if _, err := pool.Exec(ctx, `
		INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type)
		VALUES ($1, 'artifact', $2, 'artifact', $3, 'cleanup_integrity')
	`, edgeID, artifactIDs[0], edgeDestinationID); err != nil {
		return fmt.Errorf("seed edge for source %s: %w", sourceID, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced)
		VALUES ($1, TRUE, 'cleanup-integrity', $2)
	`, sourceID, len(artifactIDs)); err != nil {
		return fmt.Errorf("seed sync state for source %s: %w", sourceID, err)
	}
	return nil
}
