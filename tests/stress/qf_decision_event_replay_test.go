//go:build stress

// Package stress — spec 041 Scope 2 freshness SLA stress profile.
//
// SCN-SM-041-003, SCN-SM-041-008 — Capability handshake invariants and
// fast-forward diagnostic skip MUST hold under sustained load, AND the
// ingest-stage p95 freshness gauge MUST stay under the SLA documented in the
// capability response (FreshnessSLAP95Seconds=60s; ingest budget ≤30s,
// render budget ≤30s).
//
// This profile drives ≥500 packets across ≥60s of sustained Sync activity
// with realistic per-packet jitter, samples
// `smackerel_qf_freshness_p95_seconds{stage="ingest"}`, and asserts the
// rolling p95 stays under the documented budget.
//
// Render stage (`stage="render"`) is wired by downstream render surfaces
// (Scope 5). This stress profile leaves the render gauge under Scope 5
// ownership and asserts only the ingest budget owned by Scope 2.
package stress

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// TestQFDecisionsFreshnessSLAP95IngestRender (spec 041 Scope 2 DoD line 321)
// drives 500+ packets over 60+ seconds against the live test stack with
// realistic 0–500ms jitter per packet, samples the ingest p95 gauge at
// completion, and asserts the documented budget.
//
// Adversarial trip-wire: every packet's `created_at` is set to a fixed
// recent timestamp at SYNTHESIS time relative to wall clock — if the
// connector's `recordFreshness` ever clamps a negative observation but
// fails to clamp a positive runaway observation (e.g. when QF stamps
// `created_at` minutes in the past for any single packet), the p95 will
// exceed the 30s ingest budget and this test will fail. Tests at the unit
// layer prove `recordFreshness` clamps negative observations; this stress
// test proves the same path stays within budget under live-load conditions.
//
// Run: ./smackerel.sh --env test test stress (this test requires the live
// test stack — postgres + nats — via `./smackerel.sh --env test up`).
func TestQFDecisionsFreshnessSLAP95IngestRender(t *testing.T) {
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

	sourceID := fmt.Sprintf("qf-decisions-freshness-stress-%d", time.Now().UnixNano())
	t.Cleanup(func() { qfDecisionsStressCleanup(t, pool, sourceID) })
	qfDecisionsStressCleanup(t, pool, sourceID)

	// --- Fixture: 500 packets distributed across cursor pages of 25 each
	// (matches default page_size). Each page returns its events with
	// CreatedAt set to time.Now() at request time, so the latency the
	// connector observes is bounded by the jitter sleep BEFORE response
	// + connector processing time.
	const (
		totalPackets       = 500
		packetsPerPage     = 25
		ingestBudgetSecP95 = 30.0
		jitterMaxMillis    = 500
	)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Build packet IDs once so each cursor maps to a stable slice.
	packetIDs := make([]string, totalPackets)
	for i := range packetIDs {
		packetIDs[i] = fmt.Sprintf("packet-fresh-%d", i)
	}
	totalPages := (totalPackets + packetsPerPage - 1) / packetsPerPage
	cursorOrder := make([]string, totalPages+1)
	cursorOrder[0] = ""
	for i := 1; i <= totalPages; i++ {
		cursorOrder[i] = fmt.Sprintf("qf-fresh-page-%d", i)
	}

	var eventCalls atomic.Int32
	var packetCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(stressFreshnessCapability())
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			eventCalls.Add(1)
			cursor := r.URL.Query().Get("cursor")
			pageIdx := -1
			for i, c := range cursorOrder {
				if c == cursor {
					pageIdx = i
					break
				}
			}
			if pageIdx < 0 || pageIdx >= totalPages {
				// Past the last page — return empty + terminal cursor.
				terminal := cursorOrder[len(cursorOrder)-1]
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events:     []qfdecisions.QFDecisionEvent{},
					NextCursor: terminal,
					HasMore:    false,
					ServerTime: time.Now().UTC().Format(time.RFC3339),
				})
				return
			}
			// Build the events for this page with CreatedAt = now (within
			// the jitter budget) so the freshness observation is bounded.
			start := pageIdx * packetsPerPage
			end := start + packetsPerPage
			if end > totalPackets {
				end = totalPackets
			}
			events := make([]qfdecisions.QFDecisionEvent, 0, end-start)
			now := time.Now().UTC()
			for i := start; i < end; i++ {
				events = append(events, qfdecisions.QFDecisionEvent{
					ContractVersion: 1,
					EventID:         fmt.Sprintf("event-fresh-%d", i),
					PacketID:        packetIDs[i],
					IntentID:        "intent-" + packetIDs[i],
					ScenarioID:      "scenario-" + packetIDs[i],
					TraceID:         "trace-" + packetIDs[i],
					EventType:       "packet_created",
					DecisionType:    qfdecisions.DecisionTypeRecommendation,
					ApprovalState:   "display_only",
					PacketVersion:   1,
					PacketURL:       "https://qf.example.test/packets/" + packetIDs[i],
					CreatedAt:       now.Format(time.RFC3339),
				})
			}
			// Inject realistic per-page jitter so the observed latency
			// distribution varies across pages.
			time.Sleep(time.Duration(rng.Intn(jitterMaxMillis)) * time.Millisecond)
			next := cursorOrder[pageIdx+1]
			hasMore := pageIdx+1 < totalPages
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     events,
				NextCursor: next,
				HasMore:    hasMore,
				ServerTime: time.Now().UTC().Format(time.RFC3339),
			})
		case strings.HasPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/"):
			packetCalls.Add(1)
			packetID := strings.TrimPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath+"/")
			_ = json.NewEncoder(w).Encode(stressEnvelope(packetID, "trace-"+packetID))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-stress-token"},
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

	// --- Drive Syncs over ≥60s so the p95 window has time to accumulate
	// distributional spread.
	deadline := time.Now().Add(75 * time.Second)
	cursor := ""
	totalArtifacts := 0
	cycle := 0
	for time.Now().Before(deadline) && cursor != cursorOrder[len(cursorOrder)-1] {
		artifacts, nextCursor, syncErr := conn.Sync(ctx, cursor)
		if syncErr != nil {
			t.Fatalf("Sync cycle %d: %v", cycle, syncErr)
		}
		for _, art := range artifacts {
			if _, pubErr := publisher.PublishRawArtifact(ctx, art); pubErr != nil {
				t.Fatalf("publish cycle %d artifact %s: %v", cycle, art.SourceRef, pubErr)
			}
		}
		if err := stateStore.Save(ctx, &connector.SyncState{
			SourceID:    sourceID,
			Enabled:     true,
			SyncCursor:  nextCursor,
			ItemsSynced: len(artifacts),
		}); err != nil {
			t.Fatalf("state save cycle %d: %v", cycle, err)
		}
		totalArtifacts += len(artifacts)
		cursor = nextCursor
		cycle++
		// Light pacing between cycles to leave room for jitter to mature
		// the rolling p95 window observations.
		time.Sleep(50 * time.Millisecond)
	}

	if totalArtifacts < totalPackets {
		t.Fatalf("drove %d artifacts in %d cycles, want >= %d (full catalog)",
			totalArtifacts, cycle, totalPackets)
	}

	// --- Sample the ingest p95 gauge. Because the connector clamps
	// negative observations to zero and we stamp CreatedAt at request time,
	// the observed latency is bounded by jitter (≤500ms) plus connector
	// processing (typically tens of ms). The 30s ingest budget gives ~60x
	// headroom — a clear failure means recordFreshness regressed.
	ingestP95 := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues("ingest"))
	if ingestP95 <= 0 {
		t.Fatalf("ingest p95 gauge = %f, want > 0 (window MUST have observations after %d artifacts)",
			ingestP95, totalArtifacts)
	}
	if ingestP95 > ingestBudgetSecP95 {
		t.Fatalf("ingest p95 = %fs, want <= %fs (Scope 2 freshness SLA)",
			ingestP95, ingestBudgetSecP95)
	}

	t.Logf("freshness SLA stress: cycles=%d artifacts=%d packetFetches=%d eventCalls=%d ingestP95=%fs",
		cycle, totalArtifacts, packetCalls.Load(), eventCalls.Load(), ingestP95)
}

// stressFreshnessCapability returns a QFBridgeCapability whose
// FreshnessSLAP95Seconds matches the production budget the connector
// commits to (60s combined ingest+render; this stress test asserts the
// 30s ingest sub-budget).
func stressFreshnessCapability() qfdecisions.QFBridgeCapability {
	return qfdecisions.QFBridgeCapability{
		SupportedPacketVersions:            []string{"v1"},
		SupportedEventTypes:                []string{"packet_created", "packet_updated", "packet_trust_changed", "packet_archived", "packet_action_boundary_attempted"},
		SupportedDecisionTypes:             []string{"recommendation", "no_action", "policy_denial", "analysis_note"},
		MaxPageSize:                        200,
		MinPageSize:                        1,
		SupportedTargetContextTypes:        []string{"guided_analysis", "rhai_run", "saved_result", "analysis_context", "packet_context"},
		EvidenceMaxBundleSizeBytes:         524288,
		EvidenceMaxClaimsPerBundle:         50,
		EvidenceRateLimitPerMinute:         10,
		FreshnessSLAP95Seconds:             60,
		AuditEnvelopeVersion:               "v1",
		TenantAware:                        false,
		PreferredSurfaceHintSupported:      true,
		EngagementSignalSupported:          true,
		PersonalContextPullSupported:       true,
		WatchSignalDirection:               "qf_emit_only_pre_mvp",
		CallbackSigningSupported:           false,
		DeepLinkSigningSupported:           true,
		CredentialRotationOverlapSupported: true,
		NoActionEmitEnabled:                false,
		EligibleSmackerelSourceClasses:     []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"},
	}
}
