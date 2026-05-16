//go:build integration

package integration

import (
	"context"
	"encoding/json"
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

// TestQFDecisionsSyncThroughStateStoreAndArtifactPublisherWithStablePacketIDs
// proves that the QF Decisions connector, when wired through the real
// PostgreSQL-backed StateStore and the real RawArtifactPublisher
// (PostgreSQL + NATS), persists the response-level next_cursor as the
// canonical advancement value, normalizes valid QF packets into
// source-qualified artifacts whose identity is preserved verbatim across
// cursor pages and replays, and degrades incomplete envelopes without
// publishing trusted artifacts.
//
// SCN-SM-041-003, SCN-SM-041-005.
func TestQFDecisionsSyncThroughStateStoreAndArtifactPublisherWithStablePacketIDs(t *testing.T) {
	pool := testPool(t)
	natsClient := qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Isolation: scope this run by a unique source_id so the test cannot
	// pollute production rows for the canonical qf-decisions connector.
	sourceID := "qf-decisions-it-" + uniqueSuffix()
	cleanupQFDecisionsRows(t, pool, sourceID)
	t.Cleanup(func() { cleanupQFDecisionsRows(t, pool, sourceID) })

	type fakeQFState struct {
		eventCalls   atomic.Int32
		packetCalls  map[string]*atomic.Int32
		packetsServe map[string]qfdecisions.QFDecisionPacketEnvelope
	}

	state := &fakeQFState{
		packetCalls: map[string]*atomic.Int32{
			"packet-100": new(atomic.Int32),
			"packet-101": new(atomic.Int32),
			"packet-102": new(atomic.Int32),
		},
		packetsServe: map[string]qfdecisions.QFDecisionPacketEnvelope{
			"packet-100": validIntegrationEnvelope("packet-100", "intent-100", "scenario-100", "trace-100", "thesis 100"),
			// packet-101 is degraded — missing trace_id MUST NOT publish a trusted artifact.
			"packet-101": missingTraceEnvelope("packet-101", "intent-101", "scenario-101", "thesis 101"),
			"packet-102": validIntegrationEnvelope("packet-102", "intent-102", "scenario-102", "trace-102", "thesis 102"),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			// Connector performs a capability handshake before any decision-events
			// call. Serve a valid capability so CompatibilityCheck() passes and the
			// connector proceeds to the existing event + packet fetch flow.
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			state.eventCalls.Add(1)
			cursor := r.URL.Query().Get("cursor")
			// Page 1 returns three events (one degraded). Page 2 returns no events
			// and signals end-of-stream by repeating the same next_cursor.
			if cursor == "qf-page-2" {
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events:     []qfdecisions.QFDecisionEvent{},
					NextCursor: "qf-page-2",
					HasMore:    false,
					ServerTime: "2026-05-06T00:01:00Z",
				})
				return
			}
			events := []qfdecisions.QFDecisionEvent{
				eventForPacket("event-100", "packet-100", "intent-100", "scenario-100", "trace-100"),
				eventForPacket("event-101", "packet-101", "intent-101", "scenario-101", "trace-101"),
				eventForPacket("event-102", "packet-102", "intent-102", "scenario-102", "trace-102"),
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     events,
				NextCursor: "qf-page-2",
				HasMore:    false,
				ServerTime: "2026-05-06T00:00:00Z",
			})
		case strings.HasPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/"):
			packetID := strings.TrimPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/")
			if counter, ok := state.packetCalls[packetID]; ok {
				counter.Add(1)
			}
			env, ok := state.packetsServe[packetID]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(env)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	cfg := connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       server.URL,
			"packet_version": 1,
			"page_size":      25,
		},
	}
	if err := conn.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	stateStore := connector.NewStateStore(pool)
	publisher := pipeline.NewRawArtifactPublisher(pool, natsClient)

	runSync := func(label, cursor string) (string, []connector.RawArtifact, []string) {
		t.Helper()
		artifacts, nextCursor, err := conn.Sync(ctx, cursor)
		if err != nil {
			t.Fatalf("%s: Sync failed: %v", label, err)
		}
		ids := make([]string, 0, len(artifacts))
		for _, art := range artifacts {
			id, pubErr := publisher.PublishRawArtifact(ctx, art)
			if pubErr != nil {
				t.Fatalf("%s: PublishRawArtifact(%s): %v", label, art.SourceRef, pubErr)
			}
			if id != "" {
				ids = append(ids, id)
			}
		}
		if err := stateStore.Save(ctx, &connector.SyncState{
			SourceID:    sourceID,
			Enabled:     true,
			SyncCursor:  nextCursor,
			ItemsSynced: len(ids),
		}); err != nil {
			t.Fatalf("%s: state store save failed: %v", label, err)
		}
		return nextCursor, artifacts, ids
	}

	cursor1, artifacts1, _ := runSync("first", "")
	if cursor1 != "qf-page-2" {
		t.Fatalf("first Sync next_cursor = %q, want %q (response-level canonical advancement)", cursor1, "qf-page-2")
	}
	if len(artifacts1) != 2 {
		t.Fatalf("first Sync produced %d trusted artifacts, want 2 (third event must degrade)", len(artifacts1))
	}

	persisted, err := stateStore.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get sync state after first run: %v", err)
	}
	if persisted.SyncCursor != cursor1 {
		t.Fatalf("persisted sync_cursor = %q, want %q", persisted.SyncCursor, cursor1)
	}

	// Verify trusted artifacts persisted with QF identity preserved.
	for _, packetID := range []string{"packet-100", "packet-102"} {
		deepLink := "https://qf.example.test/packets/" + packetID
		if !artifactPersistedForSource(t, ctx, pool, sourceID, deepLink) {
			t.Fatalf("expected persisted artifact for source_id=%s deep_link=%s", sourceID, deepLink)
		}
	}
	// Degraded packet MUST NOT have been published as a trusted artifact.
	if artifactPersistedForSource(t, ctx, pool, sourceID, "https://qf.example.test/packets/packet-101") {
		t.Fatal("degraded packet-101 must not produce a trusted artifact")
	}

	// Replay: clear cursor → packet IDs MUST remain stable; no Smackerel-local IDs.
	cursor2, artifacts2, _ := runSync("replay", "")
	if cursor2 != cursor1 {
		t.Fatalf("replay next_cursor = %q, want %q", cursor2, cursor1)
	}
	if len(artifacts2) != len(artifacts1) {
		t.Fatalf("replay artifact count = %d, want %d", len(artifacts2), len(artifacts1))
	}
	for i := range artifacts2 {
		if artifacts2[i].SourceRef != artifacts1[i].SourceRef {
			t.Fatalf("replay packet identity drift: %q vs %q", artifacts2[i].SourceRef, artifacts1[i].SourceRef)
		}
		if artifacts2[i].SourceID != sourceID {
			t.Fatalf("replay must keep SourceID stable, got %q", artifacts2[i].SourceID)
		}
		if got, want := artifacts2[i].Metadata["trace_id"], artifacts1[i].Metadata["trace_id"]; got != want {
			t.Fatalf("replay trace_id drift on %q: got %v, want %v", artifacts2[i].SourceRef, got, want)
		}
	}

	// Counts: events called twice (initial + replay), each packet endpoint at
	// least twice (event-driven fetch on each pass).
	if got := state.eventCalls.Load(); got < 2 {
		t.Fatalf("event endpoint calls = %d, want >= 2", got)
	}
	for packetID, counter := range state.packetCalls {
		if got := counter.Load(); got < 2 {
			t.Fatalf("packet %s fetched %d times, want >= 2", packetID, got)
		}
	}
}

// validIntegrationEnvelope returns a fully-populated envelope that the
// normalizer accepts.
func validIntegrationEnvelope(packetID, intentID, scenarioID, traceID, thesis string) qfdecisions.QFDecisionPacketEnvelope {
	return qfdecisions.QFDecisionPacketEnvelope{
		ContractVersion:      1,
		PacketID:             packetID,
		IntentID:             intentID,
		ScenarioID:           scenarioID,
		TraceID:              traceID,
		Thesis:               thesis,
		WhyNow:               "QF-authored timing for " + packetID,
		QuantifiedImpact:     map[string]any{"unit": "bps", "value": 12.5},
		ExpertAnalysisBundle: map[string]any{"ref": "qf-analysis-" + packetID},
		CalibrationBadge:     map[string]any{"state": "calibrated"},
		DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
		ApprovalState:        "display_only",
		DeepLink:             "https://qf.example.test/packets/" + packetID,
		PacketVersion:        1,
		DecisionType:         qfdecisions.DecisionTypeRecommendation,
		CreatedAt:            "2026-05-06T00:00:00Z",
		UpdatedAt:            "2026-05-06T00:00:01Z",
	}
}

// missingTraceEnvelope returns an envelope without trace_id so the normalizer
// will degrade it without publishing a trusted artifact.
func missingTraceEnvelope(packetID, intentID, scenarioID, thesis string) qfdecisions.QFDecisionPacketEnvelope {
	env := validIntegrationEnvelope(packetID, intentID, scenarioID, "stripped", thesis)
	env.TraceID = ""
	return env
}

// eventForPacket returns a decision event whose per-event cursor is set so
// the test asserts the connector ignores it for advancement.
func eventForPacket(eventID, packetID, intentID, scenarioID, traceID string) qfdecisions.QFDecisionEvent {
	return qfdecisions.QFDecisionEvent{
		ContractVersion: 1,
		EventID:         eventID,
		PacketID:        packetID,
		IntentID:        intentID,
		ScenarioID:      scenarioID,
		TraceID:         traceID,
		EventType:       "packet_created",
		DecisionType:    qfdecisions.DecisionTypeRecommendation,
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          "diagnostic-checkpoint-" + eventID,
		PacketURL:       "https://qf.example.test/packets/" + packetID,
		SourceSurface:   "gateway-route",
		CreatedAt:       "2026-05-06T00:00:00Z",
	}
}

func qfDecisionsNATSClient(t *testing.T) *smacknats.Client {
	t.Helper()
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set — live stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := smacknats.Connect(ctx, natsURL, authToken)
	if err != nil {
		t.Fatalf("connect to test NATS: %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

func cleanupQFDecisionsRows(t *testing.T, pool *pgxpool.Pool, sourceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT id FROM artifacts WHERE source_id = $1`, sourceID)
	if err != nil {
		t.Logf("cleanup query artifacts for %s: %v", sourceID, err)
	} else {
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
	}
	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, sourceID); err != nil {
		t.Logf("cleanup artifacts for %s: %v", sourceID, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sync_state WHERE source_id = $1`, sourceID); err != nil {
		t.Logf("cleanup sync_state for %s: %v", sourceID, err)
	}
}

func artifactPersistedForSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sourceID, sourceURL string) bool {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM artifacts WHERE source_id = $1 AND source_url = $2`, sourceID, sourceURL).Scan(&count); err != nil {
		t.Fatalf("count artifacts for %s/%s: %v", sourceID, sourceURL, err)
	}
	return count > 0
}

func uniqueSuffix() string {
	return time.Now().UTC().Format("20060102150405.000000000")
}
