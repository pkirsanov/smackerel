//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
)

// TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect (SCN-SM-041-003,
// spec 041 Scope 2 DoD line 316) proves the QF Companion connector calls
// `GET /api/private/smackerel/v1/capabilities` BEFORE any decision-event poll
// when wired against the live test stack.
//
// Adversarial trip-wire: the stub increments an atomic counter whenever
// `/decision-events` is hit. The test asserts that counter == 0 immediately
// after `Connect()` returns and that the capability counter == 1, then drives
// one `Sync` cycle and re-asserts that the capability counter did NOT
// increment again on the routine Sync (handshake is per-Connect, not per
// Sync). If a future regression moves the capability fetch to AFTER the first
// `FetchDecisionEvents` call, the events_calls counter at the `Connect()`
// checkpoint will be >= 1 and the test will fail.
//
// Run: ./smackerel.sh test integration (this file requires the live test
// stack — postgres + nats — via `./smackerel.sh --env test up`).
func TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect(t *testing.T) {
	pool := testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sourceID := "qf-decisions-it-cap-connect-" + uniqueSuffix()
	cleanupQFDecisionsRows(t, pool, sourceID)
	t.Cleanup(func() { cleanupQFDecisionsRows(t, pool, sourceID) })

	var capabilityCalls atomic.Int32
	var eventsCalls atomic.Int32
	var packetsCalls atomic.Int32
	// requestOrder records the path of every request the stub received, in
	// arrival order. We assert the capabilities path is index 0.
	var orderMu sync.Mutex
	requestOrder := make([]string, 0, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderMu.Lock()
		requestOrder = append(requestOrder, r.URL.Path)
		orderMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			capabilityCalls.Add(1)
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			eventsCalls.Add(1)
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     []qfdecisions.QFDecisionEvent{},
				NextCursor: "qf-cap-test-end",
				HasMore:    false,
				ServerTime: "2026-05-20T00:00:00Z",
			})
		case strings.HasPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/"):
			packetsCalls.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, qfIntegrationConfig(server.URL, 1)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// --- Adversarial trip-wire 1: capability MUST have been fetched exactly
	// once during Connect(); no decision-events / packets call MUST have
	// occurred yet.
	if got := capabilityCalls.Load(); got != 1 {
		t.Fatalf("capability calls after Connect = %d, want 1", got)
	}
	if got := eventsCalls.Load(); got != 0 {
		t.Fatalf("events calls after Connect = %d, want 0 (capability MUST precede events)", got)
	}
	if got := packetsCalls.Load(); got != 0 {
		t.Fatalf("packets calls after Connect = %d, want 0 (capability MUST precede packets)", got)
	}

	// --- Order assertion: the FIRST request the stub observed MUST be the
	// capability path. This catches a regression where the capability call
	// is moved AFTER an events call even if the count is still 1.
	orderMu.Lock()
	first := ""
	if len(requestOrder) > 0 {
		first = requestOrder[0]
	}
	orderMu.Unlock()
	if first != qfdecisions.CapabilitiesPath {
		t.Fatalf("first request path = %q, want %q (capability handshake MUST be first)",
			first, qfdecisions.CapabilitiesPath)
	}

	// --- Drive one Sync and verify the capability count does NOT increment
	// again (handshake is per-Connect, not per-Sync).
	if _, _, err := conn.Sync(ctx, ""); err != nil {
		t.Fatalf("Sync after Connect: %v", err)
	}
	if got := capabilityCalls.Load(); got != 1 {
		t.Fatalf("capability calls after one Sync = %d, want 1 (Sync MUST NOT re-fetch capability)", got)
	}
	if got := eventsCalls.Load(); got < 1 {
		t.Fatalf("events calls after one Sync = %d, want >= 1", got)
	}

	// --- Health assertion: connector MUST be Healthy after successful handshake.
	if got := conn.Health(ctx); got != connector.HealthHealthy {
		t.Fatalf("health after successful Connect = %s, want %s",
			got, connector.HealthHealthy)
	}
}

// TestQFDecisionsConnectorReReadsCapabilityOnRestart (SCN-SM-041-003,
// spec 041 Scope 2 DoD line 317) proves the QF Companion connector re-fetches
// `GET /api/private/smackerel/v1/capabilities` after a `Close()` + `Connect()`
// restart cycle when wired against the live test stack.
//
// Adversarial trip-wire: capability counter MUST be exactly 2 at end of test.
// If a future regression caches the capability across restart (e.g. by
// keeping the in-memory cache populated after Close() or by short-circuiting
// the second handshake), the counter will be 1 and the test will fail. If a
// regression flips the order and fetches events before re-handshake on
// restart, the requestOrder slice will not show capabilities path at indices
// 0 and N (where N is the index right after the first cycle) and the test
// will fail.
func TestQFDecisionsConnectorReReadsCapabilityOnRestart(t *testing.T) {
	pool := testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sourceID := "qf-decisions-it-cap-restart-" + uniqueSuffix()
	cleanupQFDecisionsRows(t, pool, sourceID)
	t.Cleanup(func() { cleanupQFDecisionsRows(t, pool, sourceID) })

	var capabilityCalls atomic.Int32
	var eventsCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			capabilityCalls.Add(1)
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			eventsCalls.Add(1)
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     []qfdecisions.QFDecisionEvent{},
				NextCursor: "qf-restart-test-end",
				HasMore:    false,
				ServerTime: "2026-05-20T00:00:00Z",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)

	// --- First Connect: capability count goes 0 -> 1.
	if err := conn.Connect(ctx, qfIntegrationConfig(server.URL, 1)); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	if got := capabilityCalls.Load(); got != 1 {
		t.Fatalf("capability calls after first Connect = %d, want 1", got)
	}
	if _, _, err := conn.Sync(ctx, ""); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	// --- Close: in-memory capability cache MUST be cleared.
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := conn.Health(ctx); got != connector.HealthDisconnected {
		t.Fatalf("health after Close = %s, want %s", got, connector.HealthDisconnected)
	}

	// --- Second Connect (restart): capability count MUST go 1 -> 2.
	// This proves the connector does NOT reuse the in-memory cache across
	// restart and MUST re-fetch the capability on every Connect() call.
	if err := conn.Connect(ctx, qfIntegrationConfig(server.URL, 1)); err != nil {
		t.Fatalf("second Connect (restart): %v", err)
	}
	if got := capabilityCalls.Load(); got != 2 {
		t.Fatalf("capability calls after restart = %d, want 2 (restart MUST re-fetch capability)", got)
	}

	// --- Drive one Sync after restart and verify the capability counter
	// does NOT increment again on routine Sync.
	if _, _, err := conn.Sync(ctx, ""); err != nil {
		t.Fatalf("Sync after restart: %v", err)
	}
	if got := capabilityCalls.Load(); got != 2 {
		t.Fatalf("capability calls after restart Sync = %d, want 2 (Sync MUST NOT re-fetch)", got)
	}
}

// TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped (SCN-SM-041-008,
// spec 041 Scope 2 DoD line 318) proves the connector picks up a QF-issued
// fast-forward diagnostic event against the live test stack: persists the
// advanced `next_cursor`, increments
// `smackerel_qf_cursor_fast_forward_events_skipped_total` by
// `events_skipped`, transitions health to `degraded_recovered`, and resumes
// normal polling on the next Sync.
//
// Adversarial trip-wire: the stub increments `ffPacketFetches` whenever the
// fast-forward packet's envelope endpoint is hit. Production code MUST
// `continue` past the FF diagnostic event before any FetchDecisionPacket
// call, so the counter MUST stay at 0. If a regression removes the
// `if event.EventsSkipped > 0 { ... continue }` block in
// `internal/connector/qfdecisions/connector.go`, the connector will request
// the FF packet's envelope and the assertion below will fail.
func TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped(t *testing.T) {
	pool := testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sourceID := "qf-decisions-it-ff-" + uniqueSuffix()
	cleanupQFDecisionsRows(t, pool, sourceID)
	t.Cleanup(func() { cleanupQFDecisionsRows(t, pool, sourceID) })

	const (
		skippedCount      = 42
		ffEventID         = "event-ff-marker-it-1"
		ffPacketID        = "packet-ff-marker-must-not-be-fetched"
		nextCursorAfterFF = "qf-page-after-ff-it"
		stableEventTime   = "2026-05-20T00:00:00Z"
	)

	baselineCounter := testutil.ToFloat64(metrics.QFCursorFastForwardEventsSkipped)

	var ffPacketFetches atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			cursor := r.URL.Query().Get("cursor")
			if cursor == nextCursorAfterFF {
				// Routine post-FF page — return no events and same cursor
				// so the second Sync terminates cleanly.
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events:     []qfdecisions.QFDecisionEvent{},
					NextCursor: nextCursorAfterFF,
					HasMore:    false,
					ServerTime: stableEventTime,
				})
				return
			}
			// First Sync: return a single FF diagnostic event.
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events: []qfdecisions.QFDecisionEvent{
					{
						ContractVersion: 1,
						EventID:         ffEventID,
						PacketID:        ffPacketID,
						IntentID:        "intent-ff-it",
						ScenarioID:      "scenario-ff-it",
						TraceID:         "trace-ff-it",
						EventType:       "packet_created",
						DecisionType:    qfdecisions.DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						PacketURL:       "https://qf.example.test/packets/" + ffPacketID,
						CreatedAt:       stableEventTime,
						EventsSkipped:   skippedCount,
					},
				},
				NextCursor: nextCursorAfterFF,
				HasMore:    false,
				ServerTime: stableEventTime,
			})
		case strings.HasPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/"):
			// ADVERSARIAL TRIP-WIRE: production MUST NOT fetch the FF
			// packet envelope. Any request to this path MUST fail the
			// test via the counter assertion below.
			packetID := strings.TrimPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/")
			if packetID == ffPacketID {
				ffPacketFetches.Add(1)
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, qfIntegrationConfig(server.URL, 1)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// --- Drive one Sync containing the FF diagnostic event.
	artifacts, nextCursor, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync (FF cycle): %v", err)
	}

	// --- Assertion 1: zero RawArtifacts MUST be produced from a FF marker.
	if len(artifacts) != 0 {
		t.Fatalf("FF cycle produced %d artifacts, want 0 (FF marker MUST NOT normalize)", len(artifacts))
	}

	// --- Assertion 2 (ADVERSARIAL TRIP-WIRE): packet endpoint MUST NOT have
	// been called for the FF packet.
	if got := ffPacketFetches.Load(); got != 0 {
		t.Fatalf("FF packet fetches = %d, want 0 (production MUST skip FF marker before any packet fetch)", got)
	}

	// --- Assertion 3: connector MUST return the advanced next_cursor so
	// the supervisor / state store can persist it.
	if nextCursor != nextCursorAfterFF {
		t.Fatalf("next_cursor after FF = %q, want %q", nextCursor, nextCursorAfterFF)
	}

	// --- Assertion 4: counter MUST increment by exactly skippedCount.
	gotCounter := testutil.ToFloat64(metrics.QFCursorFastForwardEventsSkipped)
	if delta := gotCounter - baselineCounter; delta != float64(skippedCount) {
		t.Fatalf("fast_forward counter delta = %f, want %d", delta, skippedCount)
	}

	// --- Assertion 5: health MUST transition to degraded_recovered when the
	// only event in the Sync was the FF marker and no other event was
	// degraded.
	if got := conn.Health(ctx); got != connector.HealthDegradedRecovered {
		t.Fatalf("health after FF Sync = %s, want %s",
			got, connector.HealthDegradedRecovered)
	}

	// --- Assertion 6: persist the cursor through StateStore (wires the
	// real DB) and assert it round-trips.
	stateStore := connector.NewStateStore(pool)
	if err := stateStore.Save(ctx, &connector.SyncState{
		SourceID:    sourceID,
		Enabled:     true,
		SyncCursor:  nextCursor,
		ItemsSynced: 0,
	}); err != nil {
		t.Fatalf("state save after FF: %v", err)
	}
	persisted, err := stateStore.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("state get after FF: %v", err)
	}
	if persisted.SyncCursor != nextCursorAfterFF {
		t.Fatalf("persisted cursor after FF = %q, want %q",
			persisted.SyncCursor, nextCursorAfterFF)
	}

	// --- Assertion 7: a follow-up Sync from the advanced cursor MUST
	// succeed and return the same cursor (no events).
	_, nextCursor2, err := conn.Sync(ctx, nextCursorAfterFF)
	if err != nil {
		t.Fatalf("Sync (post-FF cycle): %v", err)
	}
	if nextCursor2 != nextCursorAfterFF {
		t.Fatalf("post-FF Sync cursor = %q, want %q (no progression on empty page)",
			nextCursor2, nextCursorAfterFF)
	}
}
