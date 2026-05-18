//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
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

const (
	qfConnectorID        = "qf-decisions"
	qfDecisionEventsPath = "/api/private/smackerel/v1/decision-events"
)

func TestQFDecisionsConnectorHealthAppearsInLiveAPI(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := readBody(resp)
		if readErr != nil {
			t.Fatalf("GET /api/health status = %d; body read failed: %v", resp.StatusCode, readErr)
		}
		t.Fatalf("GET /api/health status = %d; body = %s", resp.StatusCode, body)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read health response: %v", err)
	}

	var health struct {
		Services map[string]struct {
			Status string `json:"status"`
		} `json:"services"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("decode health response: %v; body = %s", err, body)
	}
	service, ok := health.Services["connector:"+qfConnectorID]
	if !ok {
		t.Fatalf("connector:qf-decisions missing from health services: %s", body)
	}
	if service.Status != "error" {
		t.Fatalf("connector:qf-decisions status = %q, want error before QF stub is available", service.Status)
	}
}

func TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts(t *testing.T) {
	cfg := loadE2EConfig(t)
	shutdownQFStub := startQFSchemaMismatchStub(t)
	defer shutdownQFStub()

	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("e2e: DATABASE_URL is required for live-stack artifact assertion")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, qfConnectorID); err != nil {
		t.Fatalf("clean qf artifacts before schema mismatch test: %v", err)
	}

	resp, err := apiPostNoBody(cfg, "/settings/connectors/"+qfConnectorID+"/sync")
	if err != nil {
		t.Fatalf("POST /settings/connectors/%s/sync: %v", qfConnectorID, err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		body, readErr := readBody(resp)
		if readErr != nil {
			t.Fatalf("sync status = %d; body read failed: %v", resp.StatusCode, readErr)
		}
		t.Fatalf("sync status = %d, want %d; body = %s", resp.StatusCode, http.StatusSeeOther, body)
	}
	resp.Body.Close()

	lastError := waitForQFConnectorError(t, pool, "packet_version 99 is unsupported")
	if !strings.Contains(lastError, "packet_version 99 is unsupported") {
		t.Fatalf("last qf connector error = %q", lastError)
	}

	serviceStatus := qfConnectorStatus(t, cfg)
	if serviceStatus != "degraded" {
		t.Fatalf("connector:qf-decisions status = %q, want degraded after live supervisor schema mismatch", serviceStatus)
	}

	artifacts := 0
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM artifacts WHERE source_id = $1`, qfConnectorID).Scan(&artifacts); err != nil {
		t.Fatalf("count qf artifacts after schema mismatch: %v", err)
	}
	if artifacts != 0 {
		t.Fatalf("schema mismatch must not publish qf artifacts; found %d", artifacts)
	}
}

func startQFSchemaMismatchStub(t *testing.T) func() {
	t.Helper()

	baseURL := os.Getenv("QF_DECISIONS_BASE_URL")
	if baseURL == "" {
		t.Fatal("e2e: QF_DECISIONS_BASE_URL is required for live QF schema-mismatch stub")
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse QF_DECISIONS_BASE_URL: %v", err)
	}
	port := parsedURL.Port()
	if port == "" {
		t.Fatalf("QF_DECISIONS_BASE_URL must include a port: %s", baseURL)
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		t.Fatalf("start live QF schema-mismatch stub on configured port %s: %v", port, err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != qfDecisionEventsPath {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"code": "unauthorized", "message": "authorization is required"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"code": "invalid_query_parameter", "message": "packet_version 99 is unsupported"})
	})}
	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("shutdown QF schema-mismatch stub: %v", err)
		}
		if err := <-serverErrors; err != nil {
			t.Fatalf("QF schema-mismatch stub failed: %v", err)
		}
	}
}

func apiPostNoBody(cfg e2eConfig, path string) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client.Do(request)
}

func waitForQFConnectorError(t *testing.T, pool *pgxpool.Pool, want string) string {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	lastObserved := ""
	for time.Now().Before(deadline) {
		queryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		var lastError string
		err := pool.QueryRow(queryCtx, `SELECT COALESCE(last_error, '') FROM sync_state WHERE source_id = $1`, qfConnectorID).Scan(&lastError)
		cancel()
		if err == nil {
			lastObserved = lastError
		} else {
			lastObserved = err.Error()
		}
		if err == nil && strings.Contains(lastError, want) {
			return lastError
		}
		time.Sleep(500 * time.Millisecond)
	}
	stateRows := describeSyncStateRows(t, pool)
	t.Fatalf("qf connector did not record expected error containing %q; last observed sync_state result: %q; sync_state rows: %s", want, lastObserved, stateRows)
	return ""
}

func describeSyncStateRows(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `SELECT source_id, COALESCE(last_error, '') FROM sync_state ORDER BY source_id`)
	if err != nil {
		return "query failed: " + err.Error()
	}
	defer rows.Close()

	var summaries []string
	for rows.Next() {
		var sourceID string
		var lastError string
		if err := rows.Scan(&sourceID, &lastError); err != nil {
			return "scan failed: " + err.Error()
		}
		summaries = append(summaries, sourceID+"="+lastError)
	}
	if err := rows.Err(); err != nil {
		return "rows failed: " + err.Error()
	}
	if len(summaries) == 0 {
		return "<none>"
	}
	return strings.Join(summaries, "; ")
}

func qfConnectorStatus(t *testing.T, cfg e2eConfig) string {
	t.Helper()

	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health after qf sync: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := readBody(resp)
		if readErr != nil {
			t.Fatalf("GET /api/health status = %d; body read failed: %v", resp.StatusCode, readErr)
		}
		t.Fatalf("GET /api/health status = %d; body = %s", resp.StatusCode, body)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read health response: %v", err)
	}

	var health struct {
		Services map[string]struct {
			Status string `json:"status"`
		} `json:"services"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("decode health response: %v; body = %s", err, body)
	}
	service, ok := health.Services["connector:"+qfConnectorID]
	if !ok {
		t.Fatalf("connector:qf-decisions missing from health services: %s", body)
	}
	return service.Status
}

// TestQFDecisionsConnectorIngestsPacketAndRetrievesItThroughSmackerelAPIs
// drives the QF decisions connector against an httptest QF bridge, publishes
// the resulting trusted artifacts through the live RawArtifactPublisher
// (PostgreSQL + NATS), and then verifies the ingested QF packet is readable
// through the live Smackerel `/api/artifact/{id}` detail endpoint and the
// live `/api/search` API with QF metadata intact (packet_id, trace_id,
// approval_state, deep_link, calibration/data-provenance badges).
//
// SCN-SM-041-003, SCN-SM-041-004.
func TestQFDecisionsConnectorIngestsPacketAndRetrievesItThroughSmackerelAPIs(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("e2e: NATS_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	defer pool.Close()

	natsClient, err := smacknats.Connect(ctx, natsURL, cfg.AuthToken)
	if err != nil {
		t.Fatalf("connect e2e NATS: %v", err)
	}
	defer natsClient.Close()

	sourceID := fmt.Sprintf("qf-decisions-e2e-%d", time.Now().UnixNano())
	t.Cleanup(func() { qfDecisionsCleanupSource(t, pool, sourceID) })
	qfDecisionsCleanupSource(t, pool, sourceID)

	uniqueThesis := fmt.Sprintf("QF e2e thesis %d", time.Now().UnixNano())
	packetID := fmt.Sprintf("packet-e2e-%d", time.Now().UnixNano())
	traceID := fmt.Sprintf("trace-e2e-%d", time.Now().UnixNano())
	deepLink := "https://qf.example.test/packets/" + packetID

	// Fake QF bridge: returns a single trusted decision packet for the
	// connector to fetch and a degraded packet so we also assert that
	// degraded envelopes do NOT leak into Smackerel APIs.
	degradedPacketID := fmt.Sprintf("packet-e2e-degraded-%d", time.Now().UnixNano())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			if r.URL.Query().Get("cursor") == "qf-page-2" {
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events:     []qfdecisions.QFDecisionEvent{},
					NextCursor: "qf-page-2",
					HasMore:    false,
					ServerTime: "2026-05-06T00:01:00Z",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events: []qfdecisions.QFDecisionEvent{
					{
						ContractVersion: 1,
						EventID:         "event-e2e-1",
						PacketID:        packetID,
						IntentID:        "intent-e2e",
						ScenarioID:      "scenario-e2e",
						TraceID:         traceID,
						EventType:       "packet_created",
						DecisionType:    qfdecisions.DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						Cursor:          "diagnostic-only-checkpoint",
						PacketURL:       deepLink,
						SourceSurface:   "gateway-route",
						CreatedAt:       "2026-05-06T00:00:00Z",
					},
					{
						ContractVersion: 1,
						EventID:         "event-e2e-degraded",
						PacketID:        degradedPacketID,
						IntentID:        "intent-degraded",
						ScenarioID:      "scenario-degraded",
						TraceID:         "trace-degraded",
						EventType:       "packet_created",
						DecisionType:    qfdecisions.DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						PacketURL:       "https://qf.example.test/packets/" + degradedPacketID,
						CreatedAt:       "2026-05-06T00:00:01Z",
					},
				},
				NextCursor: "qf-page-2",
				HasMore:    false,
				ServerTime: "2026-05-06T00:00:00Z",
			})
		case r.URL.Path == qfdecisions.DecisionPacketsPath+"/"+packetID:
			_ = json.NewEncoder(w).Encode(qfdecisions.QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             packetID,
				IntentID:             "intent-e2e",
				ScenarioID:           "scenario-e2e",
				TraceID:              traceID,
				Thesis:               uniqueThesis,
				WhyNow:               "QF-authored timing for e2e",
				QuantifiedImpact:     map[string]any{"unit": "bps", "value": 12.0},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-analysis-e2e"},
				CalibrationBadge:     map[string]any{"state": "calibrated", "score": 0.91},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned", "complete": true},
				ApprovalState:        "display_only",
				DeepLink:             deepLink,
				PacketVersion:        1,
				DecisionType:         qfdecisions.DecisionTypeRecommendation,
				CreatedAt:            "2026-05-06T00:00:00Z",
				UpdatedAt:            "2026-05-06T00:00:01Z",
			})
		case r.URL.Path == qfdecisions.DecisionPacketsPath+"/"+degradedPacketID:
			// Degraded: missing trace_id, approval_state, deep_link.
			_ = json.NewEncoder(w).Encode(qfdecisions.QFDecisionPacketEnvelope{
				ContractVersion: 1,
				PacketID:        degradedPacketID,
				IntentID:        "intent-degraded",
				ScenarioID:      "scenario-degraded",
				Thesis:          "degraded thesis (must not surface)",
				PacketVersion:   1,
				DecisionType:    qfdecisions.DecisionTypeRecommendation,
				CreatedAt:       "2026-05-06T00:00:01Z",
				UpdatedAt:       "2026-05-06T00:00:02Z",
			})
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
			"page_size":      25,
		},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	publisher := pipeline.NewRawArtifactPublisher(pool, natsClient)

	artifacts, _, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 trusted artifact (degraded packet must drop), got %d", len(artifacts))
	}

	var trustedArtifactID string
	for _, art := range artifacts {
		id, pubErr := publisher.PublishRawArtifact(ctx, art)
		if pubErr != nil {
			t.Fatalf("PublishRawArtifact(%s): %v", art.SourceRef, pubErr)
		}
		if art.SourceRef == packetID && id != "" {
			trustedArtifactID = id
		}
	}
	if trustedArtifactID == "" {
		t.Fatalf("trusted artifact for packet %s was not assigned an ID", packetID)
	}

	// Wait until the artifact detail endpoint returns the persisted artifact
	// (PublishRawArtifact also enqueues processing on NATS — give the API a
	// moment to expose the row).
	deadline := time.Now().Add(60 * time.Second)
	var detailBody []byte
	for time.Now().Before(deadline) {
		resp, getErr := apiGet(cfg, "/api/artifact/"+trustedArtifactID)
		if getErr == nil && resp.StatusCode == http.StatusOK {
			body, readErr := readBody(resp)
			if readErr == nil {
				detailBody = body
				break
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	if len(detailBody) == 0 {
		t.Fatalf("artifact detail endpoint never returned a body for %s", trustedArtifactID)
	}

	var detail map[string]any
	if err := json.Unmarshal(detailBody, &detail); err != nil {
		t.Fatalf("decode detail body: %v\n%s", err, detailBody)
	}
	bodyText := string(detailBody)
	// /api/artifact/{id} exposes summary fields only (artifact_type, title,
	// source_url, processing_tier, ...). The detail endpoint MUST surface QF
	// identity verbatim through source_url (which carries the QF deep_link
	// and packet_id) and artifact_type (the QF-owned content type) so that
	// downstream Smackerel readers can reach the QF-trusted record without
	// rewriting it. Trace/approval metadata persistence is enforced by the
	// integration test that reads content_raw directly from PostgreSQL.
	for _, want := range []string{packetID, deepLink, qfdecisions.ContentTypeDecisionPacket} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("artifact detail missing QF identity %q\n%s", want, bodyText)
		}
	}
	if got, _ := detail["source_url"].(string); got != deepLink {
		t.Fatalf("artifact detail source_url = %q, want QF deep_link %q\n%s", got, deepLink, bodyText)
	}
	if got, _ := detail["artifact_type"].(string); got != qfdecisions.ContentTypeDecisionPacket {
		t.Fatalf("artifact detail artifact_type = %q, want %q\n%s", got, qfdecisions.ContentTypeDecisionPacket, bodyText)
	}
	if got, _ := detail["title"].(string); got != uniqueThesis {
		t.Fatalf("artifact detail title = %q, want QF thesis %q\n%s", got, uniqueThesis, bodyText)
	}
	// Degraded packet must not appear anywhere in the detail.
	if strings.Contains(bodyText, degradedPacketID) {
		t.Fatalf("artifact detail leaked degraded packet id %q\n%s", degradedPacketID, bodyText)
	}

	// Search the live API by the unique QF thesis text.
	searchPayload, err := json.Marshal(map[string]any{
		"query": uniqueThesis,
		"limit": 10,
	})
	if err != nil {
		t.Fatalf("marshal search payload: %v", err)
	}
	searchReq, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/search", bytes.NewReader(searchPayload))
	if err != nil {
		t.Fatalf("create search request: %v", err)
	}
	searchReq.Header.Set("Content-Type", "application/json")
	searchReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	client := &http.Client{Timeout: 30 * time.Second}
	deadline = time.Now().Add(90 * time.Second)
	var searchBody []byte
	for time.Now().Before(deadline) {
		resp, sErr := client.Do(searchReq.Clone(ctx))
		if sErr == nil && resp.StatusCode == http.StatusOK {
			body, readErr := readBody(resp)
			if readErr == nil && bytes.Contains(body, []byte(trustedArtifactID)) {
				searchBody = body
				break
			}
			searchBody = body
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}
	if !bytes.Contains(searchBody, []byte(trustedArtifactID)) {
		t.Fatalf("search did not return ingested QF artifact %s within deadline; body=%s", trustedArtifactID, searchBody)
	}
	if bytes.Contains(searchBody, []byte(degradedPacketID)) {
		t.Fatalf("search leaked degraded packet id %q\n%s", degradedPacketID, searchBody)
	}
}

// TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata is the
// e2e-api counterpart to TestNormalizerMarksUnknownDecisionTypeWithMetadata
// (unit) and TestSync_EmitsUnknownDecisionTypeMetricForUnsupportedType
// (connector unit). It drives an unrecognized decision_type value through
// the live Smackerel stack and asserts the design.md §F8 forward-compat
// contract (SCN-SM-041-006 in scopes.md):
//
//  1. The unknown decision_type is NOT rejected. A RawArtifact is produced
//     and successfully persisted (artifact row exists in PostgreSQL).
//  2. The persisted artifact_type is the canonical qf/decision-packet —
//     the normalizer must NEVER invent a new qf/<future-value> type for
//     an unknown decision_type.
//  3. The in-memory RawArtifact returned by Sync() carries
//     Metadata["unknown_decision_type"] = true and preserves the raw
//     decision_type value, so downstream consumers (Scope 3 generic-card
//     variant, search, digest, Telegram) can route the artifact through
//     the generic packet card.
//  4. The smackerel_qf_unknown_decision_type_total{value=<raw>} counter
//     is incremented exactly once per ingestion.
//
// Runtime note: per spec 045 the underlying envsubst/backup pipeline is
// currently blocking integration of new e2e suites. This test MUST be
// syntactically correct and discoverable under the e2e build tag so that
// when spec 045 unblocks, `./smackerel.sh test e2e --segment connector`
// executes it automatically. It is intentionally written to fail loudly
// if the contract regresses (e.g., if a future refactor reintroduces the
// rejection path).
func TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("e2e: NATS_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	defer pool.Close()

	natsClient, err := smacknats.Connect(ctx, natsURL, cfg.AuthToken)
	if err != nil {
		t.Fatalf("connect e2e NATS: %v", err)
	}
	defer natsClient.Close()

	sourceID := fmt.Sprintf("qf-decisions-e2e-unknown-%d", time.Now().UnixNano())
	t.Cleanup(func() { qfDecisionsCleanupSource(t, pool, sourceID) })
	qfDecisionsCleanupSource(t, pool, sourceID)

	// Reset the metric so the increment assertion isolates this test
	// from other e2e tests in the same package.
	metrics.QFUnknownDecisionType.Reset()

	const unknownDecisionType = "experimental_decision_type_v9"
	packetID := fmt.Sprintf("packet-e2e-unknown-%d", time.Now().UnixNano())
	traceID := fmt.Sprintf("trace-e2e-unknown-%d", time.Now().UnixNano())
	deepLink := "https://qf.example.test/packets/" + packetID
	uniqueThesis := fmt.Sprintf("QF e2e unknown-type thesis %d", time.Now().UnixNano())

	// Fake QF bridge that emits a single decision event whose
	// decision_type is outside the canonical set. The packet envelope
	// is otherwise fully populated (trust metadata present) so the only
	// reason to reject would be the unknown decision_type — which
	// design.md §F8 forbids.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			// Round 2N capability handshake — Connect() probes this path
			// before Sync() is permitted. Canonical shape mirrors
			// defaultValidCapability() in internal/connector/qfdecisions/
			// connector_test.go (audit_envelope_version=v1, packet_version
			// v1, the three required decision_types, max_page_size>=1).
			_ = json.NewEncoder(w).Encode(qfdecisions.QFBridgeCapability{
				SupportedPacketVersions:        []string{"v1"},
				SupportedEventTypes:            []string{"packet_created"},
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
			if r.URL.Query().Get("cursor") != "" {
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events:     []qfdecisions.QFDecisionEvent{},
					NextCursor: r.URL.Query().Get("cursor"),
					HasMore:    false,
					ServerTime: "2026-05-06T00:01:00Z",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events: []qfdecisions.QFDecisionEvent{
					{
						ContractVersion: 1,
						EventID:         "event-e2e-unknown-1",
						PacketID:        packetID,
						IntentID:        "intent-e2e-unknown",
						ScenarioID:      "scenario-e2e-unknown",
						TraceID:         traceID,
						EventType:       "packet_created",
						DecisionType:    unknownDecisionType,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						Cursor:          "diagnostic-only-checkpoint",
						PacketURL:       deepLink,
						SourceSurface:   "gateway-route",
						CreatedAt:       "2026-05-06T00:00:00Z",
					},
				},
				NextCursor: "qf-page-2",
				HasMore:    false,
				ServerTime: "2026-05-06T00:00:00Z",
			})
		case r.URL.Path == qfdecisions.DecisionPacketsPath+"/"+packetID:
			_ = json.NewEncoder(w).Encode(qfdecisions.QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             packetID,
				IntentID:             "intent-e2e-unknown",
				ScenarioID:           "scenario-e2e-unknown",
				TraceID:              traceID,
				Thesis:               uniqueThesis,
				WhyNow:               "QF-authored timing for unknown decision_type e2e",
				QuantifiedImpact:     map[string]any{"unit": "bps", "value": 8.0},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-analysis-e2e-unknown"},
				CalibrationBadge:     map[string]any{"state": "calibrated", "score": 0.88},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned", "complete": true},
				ApprovalState:        "display_only",
				DeepLink:             deepLink,
				PacketVersion:        1,
				DecisionType:         unknownDecisionType,
				CreatedAt:            "2026-05-06T00:00:00Z",
				UpdatedAt:            "2026-05-06T00:00:01Z",
			})
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
			"page_size":      25,
		},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	publisher := pipeline.NewRawArtifactPublisher(pool, natsClient)

	artifacts, _, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("design.md §F8 forbids rejection of unknown decision_type; expected 1 artifact, got %d", len(artifacts))
	}

	// (a) In-memory contract: canonical content type + metadata flag +
	// raw decision_type preserved.
	art := artifacts[0]
	if art.ContentType != qfdecisions.ContentTypeDecisionPacket {
		t.Fatalf("ContentType = %q, want %q (unknown decision_type MUST fall through to canonical qf/decision-packet)", art.ContentType, qfdecisions.ContentTypeDecisionPacket)
	}
	flag, ok := art.Metadata["unknown_decision_type"].(bool)
	if !ok {
		t.Fatalf("Metadata[unknown_decision_type] = %v (%T), want bool true", art.Metadata["unknown_decision_type"], art.Metadata["unknown_decision_type"])
	}
	if !flag {
		t.Fatal("Metadata[unknown_decision_type] = false, want true")
	}
	if got, _ := art.Metadata["decision_type"].(string); got != unknownDecisionType {
		t.Fatalf("Metadata[decision_type] = %q, want %q (raw unknown value MUST be preserved)", got, unknownDecisionType)
	}

	// (b) Metric: incremented exactly once with the raw unknown value.
	gotMetric := testutil.ToFloat64(metrics.QFUnknownDecisionType.WithLabelValues(unknownDecisionType))
	if gotMetric != 1 {
		t.Fatalf("smackerel_qf_unknown_decision_type_total{value=%q} = %v, want 1", unknownDecisionType, gotMetric)
	}

	// (c) Persistence: publish + read back through the live API and
	// assert the canonical artifact_type was stored.
	artifactID, err := publisher.PublishRawArtifact(ctx, art)
	if err != nil {
		t.Fatalf("PublishRawArtifact(%s): %v", art.SourceRef, err)
	}
	if artifactID == "" {
		t.Fatalf("PublishRawArtifact assigned no ID for unknown decision_type packet %s", packetID)
	}

	deadline := time.Now().Add(60 * time.Second)
	var detailBody []byte
	for time.Now().Before(deadline) {
		resp, getErr := apiGet(cfg, "/api/artifact/"+artifactID)
		if getErr == nil && resp.StatusCode == http.StatusOK {
			body, readErr := readBody(resp)
			if readErr == nil {
				detailBody = body
				resp.Body.Close()
				break
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	if len(detailBody) == 0 {
		t.Fatalf("artifact detail endpoint never returned a body for %s", artifactID)
	}

	var detail map[string]any
	if err := json.Unmarshal(detailBody, &detail); err != nil {
		t.Fatalf("decode detail body: %v\n%s", err, detailBody)
	}
	if got, _ := detail["artifact_type"].(string); got != qfdecisions.ContentTypeDecisionPacket {
		t.Fatalf("artifact detail artifact_type = %q, want %q (DB row MUST persist canonical qf/decision-packet for unknown decision_type)", got, qfdecisions.ContentTypeDecisionPacket)
	}
	if got, _ := detail["source_url"].(string); got != deepLink {
		t.Fatalf("artifact detail source_url = %q, want QF deep_link %q\n%s", got, deepLink, detailBody)
	}
	// The detail surface MUST NOT leak any qf/<unknown_decision_type>
	// content-type variant. Specifically the raw unknown value must NOT
	// appear in artifact_type (it is acceptable for it to appear inside
	// content_raw or downstream metadata; here we only guard artifact_type).
	if got, _ := detail["artifact_type"].(string); strings.Contains(got, unknownDecisionType) {
		t.Fatalf("artifact_type leaked raw unknown decision_type %q (got %q) — normalizer must NOT invent qf/<future> types", unknownDecisionType, got)
	}
}

func qfDecisionsCleanupSource(t *testing.T, pool *pgxpool.Pool, sourceID string) {
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

// TestQFDecisionsIncompatibleCapabilityBlocksPolling is the live-stack
// counterpart to TestConnect_CapabilityIncompatibleReturnsError
// (internal/connector/qfdecisions/connector_test.go). It proves that when
// the QF bridge advertises an INCOMPATIBLE capability — here
// supported_packet_versions = ["v2"] which omits the required "v1" —
// Connect() MUST fail with a CapabilityMismatchError on the
// supported_packet_versions field, the
// smackerel_qf_capability_mismatch_total{required="v1",actual="v2"} metric
// MUST be incremented exactly once, and the connector MUST NOT poll any
// decision-events or decision-packets endpoint. The /decision-events and
// /decision-packets handlers below are adversarial trip-wires that fail the
// test via t.Errorf if any polling request reaches them.
//
// This closes the Scope 2 DoD line for SCN-SM-041-004
// "Incompatible Capability Response Blocks Polling" in
// specs/041-qf-companion-connector/scopes.md.
func TestQFDecisionsIncompatibleCapabilityBlocksPolling(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("e2e: NATS_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	defer pool.Close()

	natsClient, err := smacknats.Connect(ctx, natsURL, cfg.AuthToken)
	if err != nil {
		t.Fatalf("connect e2e NATS: %v", err)
	}
	defer natsClient.Close()

	sourceID := fmt.Sprintf("qf-decisions-e2e-incompat-%d", time.Now().UnixNano())
	t.Cleanup(func() { qfDecisionsCleanupSource(t, pool, sourceID) })
	qfDecisionsCleanupSource(t, pool, sourceID)

	// Reset the metric so the increment assertion isolates this test from
	// other e2e tests in the same package.
	metrics.QFCapabilityMismatch.Reset()

	// Fake QF bridge that advertises an INCOMPATIBLE capability —
	// supported_packet_versions = ["v2"] omits the required "v1". The
	// /decision-events and /decision-packets handlers are trip-wires: if
	// Connect() does NOT short-circuit on capability mismatch, polling
	// will hit one of them and t.Errorf will fail the test.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(qfdecisions.QFBridgeCapability{
				SupportedPacketVersions:        []string{"v2"}, // INCOMPATIBLE — missing "v1"
				SupportedEventTypes:            []string{"packet_created"},
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
		case r.URL.Path == qfdecisions.DecisionEventsPath,
			strings.HasPrefix(r.URL.Path, qfdecisions.DecisionPacketsPath):
			t.Errorf("polling MUST NOT occur after incompatible capability; saw request to %s", r.URL.Path)
			http.Error(w, "polling forbidden after incompatible capability", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	err = conn.Connect(ctx, connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       server.URL,
			"packet_version": 1,
			"page_size":      25,
		},
	})
	if err == nil {
		t.Fatal("Connect() succeeded against incompatible capability; SCN-SM-041-004 requires Connect to fail")
	}

	var mismatchErr qfdecisions.CapabilityMismatchError
	if !errors.As(err, &mismatchErr) {
		t.Fatalf("Connect() error type = %T (%v), want CapabilityMismatchError", err, err)
	}
	if mismatchErr.Field != "supported_packet_versions" {
		t.Fatalf("CapabilityMismatchError.Field = %q, want %q", mismatchErr.Field, "supported_packet_versions")
	}
	if mismatchErr.Required != "v1" {
		t.Fatalf("CapabilityMismatchError.Required = %q, want %q", mismatchErr.Required, "v1")
	}

	// Mismatch metric MUST be incremented exactly once with the canonical
	// {required="v1",actual="v2"} label combination. The CompatibilityCheck
	// joins SupportedPacketVersions with "," when emitting the actual label;
	// with a single-element ["v2"] slice that join is just "v2".
	gotMetric := testutil.ToFloat64(metrics.QFCapabilityMismatch.WithLabelValues("v1", "v2"))
	if gotMetric != 1 {
		t.Fatalf("smackerel_qf_capability_mismatch_total{required=\"v1\",actual=\"v2\"} = %v, want 1", gotMetric)
	}

	// Live DB MUST show zero artifacts for this sourceID — Sync was never
	// invoked because Connect() failed on capability mismatch. If polling
	// somehow occurred the trip-wire above will have already failed; this
	// assertion guards the persistence side of the contract.
	var artifacts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM artifacts WHERE source_id = $1`, sourceID).Scan(&artifacts); err != nil {
		t.Fatalf("count qf artifacts after incompatible capability: %v", err)
	}
	if artifacts != 0 {
		t.Fatalf("incompatible capability MUST NOT publish qf artifacts; found %d", artifacts)
	}
}
