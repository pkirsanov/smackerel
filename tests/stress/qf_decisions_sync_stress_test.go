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

// TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity
// drives a long burst of sync cycles against a fake QF bridge that returns
// the same catalog of packets across many cursor pages and replays. The live
// PostgreSQL row count for the unique test source MUST stay equal to the
// number of distinct QF packet IDs, and trace_id metadata MUST be preserved
// for every persisted artifact across every cycle.
func TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("stress: DATABASE_URL not set — live stack DB not available")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("stress: NATS_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect stress database: %v", err)
	}
	defer pool.Close()

	natsClient, err := smacknats.Connect(ctx, natsURL, cfg.AuthToken)
	if err != nil {
		t.Fatalf("connect stress NATS: %v", err)
	}
	defer natsClient.Close()

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
	for i := 0; i < totalPackets; i++ {
		pid := fmt.Sprintf("packet-stress-%d", i)
		tid := fmt.Sprintf("trace-stress-%d", i)
		packetIDs = append(packetIDs, pid)
		traceIDs[pid] = tid
		envelopes[pid] = stressEnvelope(pid, tid)
		pageKey := cursorOrder[i/packetsPerPage]
		pages[pageKey] = append(pages[pageKey], stressEvent(pid, tid, i))
	}

	var eventCalls atomic.Int32
	var packetCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
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

func stressEnvelope(packetID, traceID string) qfdecisions.QFDecisionPacketEnvelope {
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
		CreatedAt:            "2026-05-06T00:00:00Z",
		UpdatedAt:            "2026-05-06T00:00:00Z",
	}
}

func stressEvent(packetID, traceID string, i int) qfdecisions.QFDecisionEvent {
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
		CreatedAt:       "2026-05-06T00:00:00Z",
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT id FROM artifacts WHERE source_id = $1`, sourceID)
	if err != nil {
		t.Logf("cleanup query artifacts for %s: %v", sourceID, err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if _, err := pool.Exec(ctx, `DELETE FROM edges WHERE src_id = $1 OR dst_id = $1`, id); err != nil {
			t.Logf("cleanup edges for artifact %s: %v", id, err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM annotations WHERE artifact_id = $1`, id); err != nil {
			t.Logf("cleanup annotations for artifact %s: %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, sourceID); err != nil {
		t.Logf("cleanup artifacts for %s: %v", sourceID, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sync_state WHERE source_id = $1`, sourceID); err != nil {
		t.Logf("cleanup sync_state for %s: %v", sourceID, err)
	}
}
