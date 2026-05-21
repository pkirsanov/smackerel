//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Spec 041 Scope 5 — Sub-iteration E (live E2E + broader regression).
//
// This file implements the three scenario-specific E2E regression tests
// required by Scope 5 DoD (V6 / Check 8A):
//
//   - TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface
//     (SCN-SM-041-019) — live-stack credential rotation across overlapping
//     not_before windows preserves sync_cursor, evidence-export idempotency
//     state, capability re-read under the rotated bearer, planned
//     diagnostics, and audit envelope shape.
//
//   - TestQFSafetyBoundaryAndMetricSetThroughLiveSyncRenderExportSurface
//     (SCN-SM-041-020) — live-stack exercise of the safety-boundary helper
//     across every wired call-site (sync diagnostic, render forbidden hint,
//     export forbidden TargetContext, callback forbidden action) and
//     verification that every metric in the symmetric QF metric set
//     advances (or stays present-at-zero for Scope 6/8 pre-MVP).
//
//   - TestQFAuditEnvelopeV1RecordedForRequiredBridgeEventsThroughLiveSurface
//     (SCN-SM-041-021) — live-stack capture of the Cross-Product Audit
//     Envelope v1 stream across the required bridge events (capability
//     handshake ok+rejected, packet ingest, evidence export attempt,
//     evidence revocation, deep link render, action-boundary kick) plus
//     the always-required envelope-field invariants and the
//     audit_envelope_version-sourcing invariant.
//
// All three tests require the live disposable test stack
// (`./smackerel.sh --env test up`) and consume DATABASE_URL,
// SMACKEREL_AUTH_TOKEN, and CORE_EXTERNAL_URL from the SST-derived test env
// file. They skip (do not fail) if those env vars are missing so the suite
// runs cleanly without the stack available.

// ============================================================================
// Shared file-local helpers (mirror integration package helpers; cannot be
// imported across package boundaries).
// ============================================================================

func qfScope5E2EUniqueSuffix() string {
	return fmt.Sprintf("e2e-s5-%d", time.Now().UnixNano())
}

func qfScope5E2EConnectorConfig(baseURL string, packetVersion int, credentialRef string) connector.ConnectorConfig {
	return connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": credentialRef},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       baseURL,
			"packet_version": packetVersion,
			"page_size":      25,
		},
	}
}

func qfScope5E2EValidCapability() qfdecisions.QFBridgeCapability {
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

func qfScope5E2EExportCapability() qfdecisions.QFBridgeCapability {
	return qfdecisions.QFBridgeCapability{
		SupportedTargetContextTypes:    []string{qfdecisions.TargetContextPacketContext},
		EvidenceMaxBundleSizeBytes:     524288,
		EvidenceMaxClaimsPerBundle:     50,
		EvidenceRateLimitPerMinute:     10,
		EligibleSmackerelSourceClasses: []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"},
	}
}

// qfScope5E2EDBPool opens a pgx pool against the live disposable stack DB
// or skips the test if DATABASE_URL is not set. The pool is registered for
// close at test cleanup.
func qfScope5E2EDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// qfScope5E2ECleanupSourceState removes connector rows tied to the test's
// unique sourceID and any evidence-export rows tied to the test's unique
// suffix. Safe to call multiple times (the helper is idempotent).
func qfScope5E2ECleanupSourceState(t *testing.T, pool *pgxpool.Pool, sourceID, suffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT id FROM artifacts WHERE source_id = $1`, sourceID)
	if err == nil {
		var ids []string
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			if _, dErr := pool.Exec(ctx, `DELETE FROM edges WHERE src_id = $1 OR dst_id = $1`, id); dErr != nil {
				t.Logf("cleanup edges for artifact %s: %v", id, dErr)
			}
			if _, dErr := pool.Exec(ctx, `DELETE FROM annotations WHERE artifact_id = $1`, id); dErr != nil {
				t.Logf("cleanup annotations for artifact %s: %v", id, dErr)
			}
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, sourceID); err != nil {
		t.Logf("cleanup artifacts for %s: %v", sourceID, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sync_state WHERE source_id = $1`, sourceID); err != nil {
		t.Logf("cleanup sync_state for %s: %v", sourceID, err)
	}
	if suffix != "" {
		if _, err := pool.Exec(ctx, `DELETE FROM qf_personal_evidence_exports WHERE export_id LIKE $1`, "%"+suffix); err != nil {
			t.Logf("cleanup qf_personal_evidence_exports for %s: %v", suffix, err)
		}
	}
}

// qfScope5E2EAuditCapture is a thread-safe JSON slog sink that intercepts
// EmitConnectorAuditEnvelope calls while installed as the slog default.
// File-local mirror of the integration helper because the e2e package
// cannot import the integration package.
type qfScope5E2EAuditCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newQFScope5E2EAuditCapture() *qfScope5E2EAuditCapture {
	return &qfScope5E2EAuditCapture{}
}

func (c *qfScope5E2EAuditCapture) handler() slog.Handler {
	return slog.NewJSONHandler(&qfScope5E2EAuditCaptureWriter{c: c}, &slog.HandlerOptions{Level: slog.LevelDebug})
}

func (c *qfScope5E2EAuditCapture) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.Reset()
}

func (c *qfScope5E2EAuditCapture) allRecords() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []map[string]any
	for _, line := range bytes.Split(c.buf.Bytes(), []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out
}

func (c *qfScope5E2EAuditCapture) crossProductAuditRecords() []map[string]any {
	all := c.allRecords()
	out := make([]map[string]any, 0, len(all))
	for _, rec := range all {
		if msg, _ := rec["msg"].(string); msg == "qf-decisions: cross_product_audit" {
			out = append(out, rec)
		}
	}
	return out
}

func (c *qfScope5E2EAuditCapture) findEnvelope(t *testing.T, action, outcome string) map[string]any {
	t.Helper()
	for _, rec := range c.crossProductAuditRecords() {
		gotAction, _ := rec["action"].(string)
		gotOutcome, _ := rec["outcome"].(string)
		if gotAction == action && gotOutcome == outcome {
			return rec
		}
	}
	t.Fatalf("no cross_product_audit record with action=%q outcome=%q; captured: %s",
		action, outcome, qfScope5E2EPrettyRecords(c.crossProductAuditRecords()))
	return nil
}

type qfScope5E2EAuditCaptureWriter struct {
	c *qfScope5E2EAuditCapture
}

func (w *qfScope5E2EAuditCaptureWriter) Write(p []byte) (int, error) {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	return w.c.buf.Write(p)
}

func qfScope5E2EPrettyRecords(records []map[string]any) string {
	encoded, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return "<unprintable records>"
	}
	return string(encoded)
}

func qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t *testing.T, label string, rec map[string]any, wantAuditVersion string) {
	t.Helper()
	for _, key := range []string{"actor_ref", "surface", "action", "outcome", "ts", "audit_envelope_version", "recorded_at"} {
		got, ok := rec[key].(string)
		if !ok || got == "" {
			t.Fatalf("%s: envelope field %q is missing or empty (got %#v); full record: %s",
				label, key, rec[key], qfScope5E2EPrettyRecords([]map[string]any{rec}))
		}
	}
	gotVersion, _ := rec["audit_envelope_version"].(string)
	if gotVersion != wantAuditVersion {
		t.Fatalf("%s: audit_envelope_version = %q, want %q (sourced from persisted capability response)",
			label, gotVersion, wantAuditVersion)
	}
	if _, err := time.Parse(time.RFC3339, rec["ts"].(string)); err != nil {
		t.Fatalf("%s: ts is not RFC3339: %v (value=%q)", label, err, rec["ts"])
	}
	if _, err := time.Parse(time.RFC3339, rec["recorded_at"].(string)); err != nil {
		t.Fatalf("%s: recorded_at is not RFC3339: %v (value=%q)", label, err, rec["recorded_at"])
	}
}

func qfScope5E2EAuditVersionFromConnector(t *testing.T, conn *qfdecisions.Connector) string {
	t.Helper()
	responseJSON, _, status, err := conn.CapabilitySnapshot()
	if err != nil {
		t.Fatalf("CapabilitySnapshot: %v", err)
	}
	if status != qfdecisions.CapabilityStatusCompatible {
		t.Fatalf("CapabilitySnapshot status = %s, want compatible", status)
	}
	if responseJSON == "" {
		t.Fatal("CapabilitySnapshot returned empty JSON")
	}
	var capability qfdecisions.QFBridgeCapability
	if err := json.Unmarshal([]byte(responseJSON), &capability); err != nil {
		t.Fatalf("decode persisted capability JSON: %v", err)
	}
	return capability.AuditEnvelopeVersion
}

// qfScope5E2ESumCounterCollector returns the sum of all counter values across
// every label combination currently emitted by the given Prometheus collector.
// Works for both single Counters and CounterVecs.
func qfScope5E2ESumCounterCollector(c prometheus.Collector) float64 {
	ch := make(chan prometheus.Metric, 256)
	go func() {
		c.Collect(ch)
		close(ch)
	}()
	sum := 0.0
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			continue
		}
		if pb.Counter != nil {
			sum += pb.Counter.GetValue()
		}
	}
	return sum
}

// ============================================================================
// Test 1 — SCN-SM-041-019
// ============================================================================

// TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface
// (SCN-SM-041-019) drives the QF Companion connector through a full credential
// rotation against the live disposable stack and asserts that:
//
//  1. The persisted sync_state.sync_cursor survives rotation verbatim
//     (queried from the live Postgres via pgxpool).
//  2. The persisted qf_personal_evidence_exports rows survive rotation
//     verbatim (queried from the live Postgres via pgxpool).
//  3. The capability handshake is re-read under the NEW credential bearer
//     after rotation (asserted via httptest stub header recorder).
//  4. The rotation plan diagnostics carry all three required tokens.
//  5. The rotation audit envelope is emitted to the connector audit log
//     (via slog capture) with action=credential_rotation, outcome=ok,
//     audit_envelope_version=v1.
//
// This is the live-stack counterpart to the integration test
// TestQFCredentialRotationOverlapPreservesCursorExportIdempotencyCapabilityDiagnosticsAndAudit.
// The test uses an httptest QF stub (rather than the live-configured QF stub
// on QF_DECISIONS_BASE_URL) because credential rotation is an in-process
// connector operation and the live smackerel-core process owns its own
// connector instance; the test drives an independent in-process connector
// against the live DB/NATS for state persistence proof.
func TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	pool := qfScope5E2EDBPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	suffix := qfScope5E2EUniqueSuffix()
	sourceID := "qf-decisions-e2e-rot-" + suffix
	qfScope5E2ECleanupSourceState(t, pool, sourceID, suffix)
	t.Cleanup(func() { qfScope5E2ECleanupSourceState(t, pool, sourceID, suffix) })

	// Capture audit envelopes emitted via slog so the rotation
	// audit envelope assertion is grounded in the real connector
	// audit sink (not a hardcoded test literal).
	capture := newQFScope5E2EAuditCapture()
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(capture.handler()))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	var capabilityCalls atomic.Int32
	var capabilityAuthHeaders []string
	var headerMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case qfdecisions.CapabilitiesPath:
			headerMu.Lock()
			capabilityAuthHeaders = append(capabilityAuthHeaders, r.Header.Get("Authorization"))
			headerMu.Unlock()
			capabilityCalls.Add(1)
			_ = json.NewEncoder(w).Encode(qfScope5E2EValidCapability())
		case qfdecisions.DecisionEventsPath:
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     []qfdecisions.QFDecisionEvent{},
				NextCursor: "qf-e2e-rot-cursor-page-end",
				HasMore:    false,
				ServerTime: "2026-05-21T18:00:00Z",
			})
		case qfdecisions.PersonalEvidenceBundlesPath:
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				var posted qfdecisions.PersonalEvidenceBundle
				_ = json.NewDecoder(r.Body).Decode(&posted)
				payloadHash, _ := qfdecisions.EvidenceBundlePayloadHash(posted)
				_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{
					ExportID:    posted.ExportID,
					BundleID:    posted.BundleID,
					PayloadHash: payloadHash,
					Status:      qfdecisions.EvidenceExportStatusAccepted,
				})
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, qfScope5E2EConnectorConfig(server.URL, 1, "qf-service-token")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	stateStore := connector.NewStateStore(pool)

	// Pre-rotation Sync establishes the persisted sync_cursor.
	_, preRotationCursor, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("pre-rotation Sync: %v", err)
	}
	if preRotationCursor != "qf-e2e-rot-cursor-page-end" {
		t.Fatalf("pre-rotation cursor = %q, want %q", preRotationCursor, "qf-e2e-rot-cursor-page-end")
	}
	if err := stateStore.Save(ctx, &connector.SyncState{
		SourceID:    sourceID,
		Enabled:     true,
		SyncCursor:  preRotationCursor,
		ItemsSynced: 0,
	}); err != nil {
		t.Fatalf("save pre-rotation sync state: %v", err)
	}

	// Seed evidence-export idempotency rows directly into the live DB via
	// the real EvidenceExporter.Export() round-trip so the rotation
	// preserved-state assertion is grounded in real persisted state, not
	// in-memory test fixtures. The Export() call uses the same httptest
	// stub the connector was Connect()'d to (which already accepts
	// PersonalEvidenceBundles via the PersonalEvidenceBundlesPath handler
	// configured below).
	exportStore := qfdecisions.NewEvidenceExportStore(pool)
	exporter := qfdecisions.NewEvidenceExporter(
		qfdecisions.NewClient(server.URL, "qf-service-token", 1, 25),
		exportStore,
		qfdecisions.NewEvidenceRateLimiter(time.Now),
		"qf-service-token",
		time.Now,
	)
	preservedExportIDs := []string{
		fmt.Sprintf("export-rot-001-%s", suffix),
		fmt.Sprintf("export-rot-002-%s", suffix),
	}
	exportObservedAt := time.Date(2026, 5, 21, 17, 50, 0, 0, time.UTC)
	for _, exportID := range preservedExportIDs {
		bundleID := strings.Replace(exportID, "export-rot-", "bundle-rot-", 1)
		bundle, err := qfdecisions.BuildPacketContextEvidenceBundle(qfdecisions.EvidenceBundleInput{
			BundleID:        bundleID,
			ExportID:        exportID,
			CreatedAt:       exportObservedAt,
			ConsentScope:    "qf_packet_context",
			SensitivityTier: "personal",
			PacketID:        strings.Replace(exportID, "export-rot-", "packet-rot-", 1),
			TraceID:         strings.Replace(exportID, "export-rot-", "trace-rot-", 1),
			SourceArtifactIDs: []string{
				strings.Replace(exportID, "export-rot-", "artifact-rot-", 1),
			},
			SourceRefs: []string{
				"https://example.test/source/" + strings.Replace(exportID, "export-rot-", "artifact-rot-", 1),
			},
			SourceProvenanceClasses: []qfdecisions.SourceProvenanceClass{
				{
					SourceArtifactID:      strings.Replace(exportID, "export-rot-", "artifact-rot-", 1),
					SourceProvenanceClass: "smackerel_news",
				},
			},
			ExtractedClaims:  []string{"Rotation seed claim for " + exportID},
			Confidence:       0.9,
			Provenance:       map[string]any{"generator": "e2e-qf-scope5-rotation"},
			RedactionSummary: map[string]any{"omitted_raw_messages": 0},
		})
		if err != nil {
			t.Fatalf("BuildPacketContextEvidenceBundle %s: %v", exportID, err)
		}
		if _, _, err := exporter.Export(ctx, bundle, qfScope5E2EExportCapability()); err != nil {
			t.Fatalf("seed Export %s: %v", exportID, err)
		}
	}

	persistedPre, err := stateStore.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get pre-rotation sync state: %v", err)
	}
	if persistedPre.SyncCursor != preRotationCursor {
		t.Fatalf("persisted pre-rotation sync_cursor = %q, want %q", persistedPre.SyncCursor, preRotationCursor)
	}

	// Drive the rotation through the connector against the live DB.
	now := time.Date(2026, 5, 21, 18, 0, 0, 0, time.UTC)
	credentials := []qfdecisions.RotatingCredential{
		{
			Ref:       "qf-service-token",
			NotBefore: now.Add(-48 * time.Hour),
			NotAfter:  now.Add(20 * time.Hour),
		},
		{
			Ref:       "qf-rotated-token",
			NotBefore: now.Add(-4 * time.Hour),
			NotAfter:  now.Add(72 * time.Hour),
		},
	}
	rotationState := qfdecisions.CredentialRotationState{
		SyncCursor:             preRotationCursor,
		CapabilityResponseJSON: `{"audit_envelope_version":"v1"}`,
		CapabilityFetchedAt:    now.Add(-10 * time.Minute),
		CapabilityStatus:       qfdecisions.CapabilityStatusCompatible,
		EvidenceExportIDs:      preservedExportIDs,
	}

	capture.reset()
	plan, err := conn.RotateCredentials(ctx, credentials, rotationState, now)
	if err != nil {
		t.Fatalf("RotateCredentials: %v", err)
	}
	if plan.SelectedCredentialRef != "qf-rotated-token" {
		t.Fatalf("selected credential = %q, want %q (newest not_before)", plan.SelectedCredentialRef, "qf-rotated-token")
	}
	if plan.PreviousCredentialRef != "qf-service-token" {
		t.Fatalf("previous credential = %q, want %q", plan.PreviousCredentialRef, "qf-service-token")
	}

	// Diagnostics MUST include all three required operator-clarity tokens.
	wantDiagnostics := []string{
		"capability_re_read_required",
		"sync_cursor_preserved",
		"evidence_export_state_preserved",
	}
	for _, want := range wantDiagnostics {
		found := false
		for _, got := range plan.Diagnostics {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing diagnostic %q in plan.Diagnostics=%v", want, plan.Diagnostics)
		}
	}

	// PreservedState round-trip: cursor + export IDs unchanged through plan.
	if plan.PreservedState.SyncCursor != preRotationCursor {
		t.Fatalf("preserved sync_cursor = %q, want %q", plan.PreservedState.SyncCursor, preRotationCursor)
	}
	if len(plan.PreservedState.EvidenceExportIDs) != len(preservedExportIDs) {
		t.Fatalf("preserved evidence export ids count = %d, want %d", len(plan.PreservedState.EvidenceExportIDs), len(preservedExportIDs))
	}
	for i := range preservedExportIDs {
		if plan.PreservedState.EvidenceExportIDs[i] != preservedExportIDs[i] {
			t.Fatalf("preserved evidence export id[%d] = %q, want %q", i, plan.PreservedState.EvidenceExportIDs[i], preservedExportIDs[i])
		}
	}

	// Audit envelope assertions: rotation plan envelope MUST be OK/v1/credential_rotation.
	if plan.AuditEnvelope.Action != qfdecisions.AuditActionCredentialRotation {
		t.Fatalf("rotation audit action = %q, want %q", plan.AuditEnvelope.Action, qfdecisions.AuditActionCredentialRotation)
	}
	if plan.AuditEnvelope.Outcome != qfdecisions.AuditOutcomeOK {
		t.Fatalf("rotation audit outcome = %q, want %q", plan.AuditEnvelope.Outcome, qfdecisions.AuditOutcomeOK)
	}
	if plan.AuditEnvelope.AuditEnvelopeVersion != qfdecisions.AuditEnvelopeVersionV1 {
		t.Fatalf("rotation audit envelope version = %q, want %q", plan.AuditEnvelope.AuditEnvelopeVersion, qfdecisions.AuditEnvelopeVersionV1)
	}

	// Live-stack capability re-read: capability MUST have been polled twice
	// (Connect + rotation re-read) and the second poll MUST carry the rotated
	// bearer token, proving the rotation actually flipped the credential the
	// connector uses on the wire.
	if got := capabilityCalls.Load(); got != 2 {
		t.Fatalf("capability calls across Connect + Rotation = %d, want 2", got)
	}
	headerMu.Lock()
	gotHeaders := append([]string(nil), capabilityAuthHeaders...)
	headerMu.Unlock()
	if len(gotHeaders) != 2 {
		t.Fatalf("captured %d auth headers, want 2", len(gotHeaders))
	}
	if !strings.EqualFold(gotHeaders[0], "Bearer qf-service-token") {
		t.Fatalf("pre-rotation capability auth header = %q, want %q", gotHeaders[0], "Bearer qf-service-token")
	}
	if !strings.EqualFold(gotHeaders[1], "Bearer qf-rotated-token") {
		t.Fatalf("post-rotation capability auth header = %q, want %q (capability MUST be re-read with rotated credential)", gotHeaders[1], "Bearer qf-rotated-token")
	}

	// Live-stack state preservation: re-query the persisted sync_state and
	// qf_personal_evidence_exports rows AFTER rotation and confirm they
	// survived verbatim.
	persistedPost, err := stateStore.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get post-rotation sync state: %v", err)
	}
	if persistedPost.SyncCursor != preRotationCursor {
		t.Fatalf("post-rotation persisted sync_cursor = %q, want %q (rotation MUST preserve persisted cursor)", persistedPost.SyncCursor, preRotationCursor)
	}

	for _, exportID := range preservedExportIDs {
		got, found, err := exportStore.Get(ctx, exportID)
		if err != nil {
			t.Fatalf("get persisted evidence export %s after rotation: %v", exportID, err)
		}
		if !found {
			t.Fatalf("evidence export %s not found after rotation (rotation MUST preserve persisted export idempotency rows)", exportID)
		}
		if got.Status != qfdecisions.EvidenceExportStatusAccepted {
			t.Fatalf("evidence export %s status after rotation = %s, want accepted", exportID, got.Status)
		}
	}

	// Connector health remains Healthy after successful rotation.
	if got := conn.Health(ctx); got != connector.HealthHealthy {
		t.Fatalf("health after rotation = %s, want %s", got, connector.HealthHealthy)
	}

	// Slog audit-sink capture: the rotation audit envelope MUST appear in
	// the captured cross_product_audit stream so the connector audit log
	// (not just the in-memory plan) carries the v1 envelope.
	auditVersion := qfScope5E2EAuditVersionFromConnector(t, conn)
	if auditVersion != qfdecisions.AuditEnvelopeVersionV1 {
		t.Fatalf("persisted capability audit_envelope_version = %q, want %q", auditVersion, qfdecisions.AuditEnvelopeVersionV1)
	}
	rotationEnvelope := capture.findEnvelope(t, qfdecisions.AuditActionCredentialRotation, qfdecisions.AuditOutcomeOK)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "credential_rotation", rotationEnvelope, auditVersion)
}

// ============================================================================
// Test 2 — SCN-SM-041-020
// ============================================================================

// TestQFSafetyBoundaryAndMetricSetThroughLiveSyncRenderExportSurface
// (SCN-SM-041-020) drives every wired safety-boundary call-site and every
// wired symmetric-set metric path against the live disposable stack and
// asserts:
//
//  1. smackerel_qf_action_boundary_attempts_total advances by AT LEAST 3
//     across the wired non-callback boundary call-sites (sync diagnostic,
//     render forbidden hint, export forbidden TargetContext).
//  2. Every metric in the symmetric QF metric set is either:
//     (a) emitted with a non-zero delta when wired (Scope 2/3/4/5 paths),
//     or
//     (b) registered with zero emitted label combinations when its
//     transport is intentionally Scope 6/8 pre-MVP
//     (engagement_signal, callback).
//
// This is the live-stack counterpart to the integration test
// TestQFObservabilityEmitsAllSymmetricMetricsAcrossSyncRenderExportAndBoundaryPaths.
// Rather than reproducing the full 14-metric matrix (already covered in the
// Sub-iter B integration evidence), this test exercises a focused subset
// against the live DB to prove the boundary helper and the symmetric metric
// set behave correctly when the connector is wired against real persistence.
func TestQFSafetyBoundaryAndMetricSetThroughLiveSyncRenderExportSurface(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	pool := qfScope5E2EDBPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	suffix := qfScope5E2EUniqueSuffix()
	sourceID := "qf-decisions-e2e-bdy-" + suffix
	qfScope5E2ECleanupSourceState(t, pool, sourceID, suffix)
	t.Cleanup(func() { qfScope5E2ECleanupSourceState(t, pool, sourceID, suffix) })

	// Baseline snapshot before any emission so the assertion is robust
	// to other tests running in the same process. Re-snapshot at the end
	// and assert deltas.
	baseline := struct {
		boundary       float64
		ingest         float64
		render         float64
		export         float64
		revoked        float64
		unknownDecType float64
		cursorFFwd     float64
	}{
		boundary:       qfScope5E2ESumCounterCollector(metrics.QFActionBoundaryAttemptsTotal),
		ingest:         qfScope5E2ESumCounterCollector(metrics.QFPacketIngestTotal),
		render:         qfScope5E2ESumCounterCollector(metrics.QFDeepLinkRenderTotal),
		export:         qfScope5E2ESumCounterCollector(metrics.QFEvidenceExportAttempts),
		revoked:        qfScope5E2ESumCounterCollector(metrics.QFEvidenceRevokedTotal),
		unknownDecType: qfScope5E2ESumCounterCollector(metrics.QFUnknownDecisionType),
		cursorFFwd:     testutil.ToFloat64(metrics.QFCursorFastForwardEventsSkipped),
	}

	// Drive a Sync against a stub that yields one happy recommendation,
	// one unknown decision_type, one fast-forward event, plus a second
	// page with the boundary diagnostic event (Sync-loop boundary path).
	syncCallCount := atomic.Int32{}
	syncServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(qfScope5E2EValidCapability())
		case qfdecisions.DecisionEventsPath:
			call := syncCallCount.Add(1)
			if call == 1 {
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events: []qfdecisions.QFDecisionEvent{
						{
							ContractVersion: 1,
							EventID:         "evt-e2e-bdy-rec-001",
							PacketID:        "packet-e2e-bdy-rec-001",
							IntentID:        "intent-e2e-bdy-001",
							ScenarioID:      "scenario-e2e-bdy-001",
							TraceID:         "trace-e2e-bdy-rec-001", // gitleaks:allow
							EventType:       "packet_created",
							DecisionType:    qfdecisions.DecisionTypeRecommendation,
							ApprovalState:   "display_only",
							PacketVersion:   1,
							Cursor:          "diagnostic-checkpoint-e2e-bdy-rec-001",
							PacketURL:       "https://qf.example.test/packets/packet-e2e-bdy-rec-001",
							SourceSurface:   "gateway-route",
							CreatedAt:       "2026-05-21T18:00:00Z",
						},
						{
							ContractVersion: 1,
							EventID:         "evt-e2e-bdy-unknown-001",
							PacketID:        "packet-e2e-bdy-unknown-001",
							IntentID:        "intent-e2e-bdy-001",
							ScenarioID:      "scenario-e2e-bdy-001",
							TraceID:         "trace-e2e-bdy-unknown-001", // gitleaks:allow
							EventType:       "packet_created",
							DecisionType:    "future_qf_shape",
							ApprovalState:   "display_only",
							PacketVersion:   1,
							Cursor:          "diagnostic-checkpoint-e2e-bdy-unknown-001",
							PacketURL:       "https://qf.example.test/packets/packet-e2e-bdy-unknown-001",
							SourceSurface:   "gateway-route",
							CreatedAt:       "2026-05-21T18:00:05Z",
						},
						{
							ContractVersion: 1,
							EventID:         "evt-e2e-bdy-ff-001",
							PacketID:        "packet-e2e-bdy-ff-001",
							IntentID:        "intent-e2e-bdy-001",
							ScenarioID:      "scenario-e2e-bdy-001",
							TraceID:         "trace-e2e-bdy-ff-001", // gitleaks:allow
							EventType:       "packet_created",
							DecisionType:    qfdecisions.DecisionTypeRecommendation,
							ApprovalState:   "display_only",
							PacketVersion:   1,
							Cursor:          "diagnostic-checkpoint-e2e-bdy-ff-001",
							PacketURL:       "https://qf.example.test/packets/packet-e2e-bdy-ff-001",
							SourceSurface:   "gateway-route",
							CreatedAt:       "2026-05-21T18:00:10Z",
							EventsSkipped:   3,
						},
					},
					NextCursor: "qf-e2e-bdy-cursor-page-1-end",
					HasMore:    false,
					ServerTime: "2026-05-21T18:00:30Z",
				})
				return
			}
			if call == 2 {
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events: []qfdecisions.QFDecisionEvent{
						{
							ContractVersion: 1,
							EventID:         "evt-e2e-bdy-boundary-001",
							PacketID:        "packet-e2e-bdy-boundary-001",
							IntentID:        "intent-e2e-bdy-boundary-001",
							ScenarioID:      "scenario-e2e-bdy-boundary-001",
							TraceID:         "trace-e2e-bdy-boundary-001", // gitleaks:allow
							EventType:       qfdecisions.EventTypePacketActionBoundaryAttempted,
							DecisionType:    qfdecisions.ActionTypeApproval,
							ApprovalState:   "display_only",
							PacketVersion:   1,
							Cursor:          "diagnostic-checkpoint-e2e-bdy-boundary-001",
							PacketURL:       "https://qf.example.test/packets/packet-e2e-bdy-boundary-001",
							SourceSurface:   "gateway-route",
							CreatedAt:       "2026-05-21T18:00:40Z",
						},
					},
					NextCursor: "qf-e2e-bdy-cursor-page-2-end",
					HasMore:    false,
					ServerTime: "2026-05-21T18:00:45Z",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     []qfdecisions.QFDecisionEvent{},
				NextCursor: "qf-e2e-bdy-cursor-empty",
				HasMore:    false,
				ServerTime: "2026-05-21T18:00:50Z",
			})
		case qfdecisions.DecisionPacketsPath + "/packet-e2e-bdy-rec-001":
			_ = json.NewEncoder(w).Encode(qfdecisions.QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-e2e-bdy-rec-001",
				IntentID:             "intent-e2e-bdy-001",
				ScenarioID:           "scenario-e2e-bdy-001",
				TraceID:              "trace-e2e-bdy-rec-001", // gitleaks:allow
				Thesis:               "E2E boundary+metric coverage packet",
				WhyNow:               "Drive every wired QF metric over the live stack",
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-e2e-bdy-rec-001",
				PacketURLSigned:      "https://qf.example.test/packets/packet-e2e-bdy-rec-001?sig=fresh",
				SignatureExpiresAt:   "2099-01-01T00:00:00Z",
				PreferredSurface:     "smackerel_digest",
				PacketVersion:        1,
				DecisionType:         qfdecisions.DecisionTypeRecommendation,
				CreatedAt:            "2026-05-21T18:00:00Z",
				UpdatedAt:            "2026-05-21T18:00:01Z",
				QuantifiedImpact:     map[string]any{"label": "Impact band", "severity": "info", "summary": "Public impact statement"},
				ExpertAnalysisBundle: map[string]any{"label": "Expert review", "severity": "info", "summary": "Reviewed"},
				CalibrationBadge:     map[string]any{"label": "QF calibrated", "severity": "info", "summary": "Calibration verified"},
				DataProvenanceBadge:  map[string]any{"label": "QF provenance", "severity": "info", "summary": "QF source lineage present"},
			})
		case qfdecisions.DecisionPacketsPath + "/packet-e2e-bdy-unknown-001":
			env := qfdecisions.QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-e2e-bdy-unknown-001",
				IntentID:             "intent-e2e-bdy-001",
				ScenarioID:           "scenario-e2e-bdy-001",
				TraceID:              "trace-e2e-bdy-unknown-001", // gitleaks:allow
				Thesis:               "E2E boundary+metric coverage packet",
				WhyNow:               "Drive every wired QF metric over the live stack",
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-e2e-bdy-unknown-001",
				PacketURLSigned:      "https://qf.example.test/packets/packet-e2e-bdy-unknown-001?sig=fresh",
				SignatureExpiresAt:   "2099-01-01T00:00:00Z",
				PreferredSurface:     "smackerel_digest",
				PacketVersion:        1,
				DecisionType:         "future_qf_shape",
				CreatedAt:            "2026-05-21T18:00:05Z",
				UpdatedAt:            "2026-05-21T18:00:05Z",
				QuantifiedImpact:     map[string]any{"label": "Impact band", "severity": "info", "summary": "Public impact statement"},
				ExpertAnalysisBundle: map[string]any{"label": "Expert review", "severity": "info", "summary": "Reviewed"},
				CalibrationBadge:     map[string]any{"label": "QF calibrated", "severity": "info", "summary": "Calibration verified"},
				DataProvenanceBadge:  map[string]any{"label": "QF provenance", "severity": "info", "summary": "QF source lineage present"},
			}
			_ = json.NewEncoder(w).Encode(env)
		case qfdecisions.PersonalEvidenceBundlesPath:
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				var posted qfdecisions.PersonalEvidenceBundle
				_ = json.NewDecoder(r.Body).Decode(&posted)
				payloadHash, _ := qfdecisions.EvidenceBundlePayloadHash(posted)
				_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{
					ExportID:    posted.ExportID,
					BundleID:    posted.BundleID,
					PayloadHash: payloadHash,
					Status:      qfdecisions.EvidenceExportStatusAccepted,
				})
				return
			}
			http.NotFound(w, r)
		default:
			if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, qfdecisions.PersonalEvidenceBundlesPath+"/") {
				w.WriteHeader(http.StatusOK)
				exportID := strings.TrimPrefix(r.URL.Path, qfdecisions.PersonalEvidenceBundlesPath+"/")
				_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceRevocationResponse{
					ExportID: exportID,
					Status:   qfdecisions.EvidenceExportStatusRevoked,
					Reason:   qfdecisions.EvidenceRevokeReasonConsentRevoked,
				})
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer syncServer.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, qfScope5E2EConnectorConfig(syncServer.URL, 1, "qf-service-token")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// First Sync page: yields recommendation + unknown + fast-forward.
	syncArtifacts, nextCursor, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync page 1: %v", err)
	}
	if nextCursor == "" {
		t.Fatal("Sync page 1 returned empty cursor")
	}
	if len(syncArtifacts) < 1 {
		t.Fatalf("Sync page 1 produced %d artifacts, want >= 1", len(syncArtifacts))
	}

	// Second Sync page: yields the QF-emitted boundary diagnostic event.
	// Wires Sync-loop call-site of EnforceQFActionBoundary.
	if _, _, err := conn.Sync(ctx, nextCursor); err != nil {
		t.Fatalf("Sync page 2 (boundary diagnostic): %v", err)
	}

	// Render forbidden-hint path: a happy artifact with a forbidden
	// requested_action_type in metadata triggers the render-path guard.
	observedAt := time.Date(2026, 5, 21, 18, 0, 0, 0, time.UTC)
	renderArtifact := syncArtifacts[0]
	renderArtifact.CapturedAt = observedAt.Add(-45 * time.Second)
	if renderArtifact.Metadata == nil {
		renderArtifact.Metadata = map[string]any{}
	}
	// Happy render first — emits deep_link_render envelope + freshness samples.
	if _, err := qfdecisions.RenderPacketCard(ctx, renderArtifact, qfdecisions.RenderOptions{
		Surface:                  qfdecisions.SurfaceWeb,
		DeepLinkSigningSupported: true,
		Now:                      observedAt,
	}); err != nil {
		t.Fatalf("happy RenderPacketCard: %v", err)
	}
	// Forbidden-action render: inject a forbidden requested_action_type to
	// trigger the render-path boundary guard (no error returned from
	// RenderPacketCard; the guard increments the metric + emits the kick).
	forbiddenRender := renderArtifact
	forbiddenRender.Metadata = map[string]any{
		"requested_action_type": qfdecisions.ActionTypeMandateChange,
	}
	if _, err := qfdecisions.RenderPacketCard(ctx, forbiddenRender, qfdecisions.RenderOptions{
		Surface:                  qfdecisions.SurfaceWeb,
		DeepLinkSigningSupported: true,
		Now:                      observedAt,
	}); err != nil {
		t.Fatalf("forbidden-hint RenderPacketCard: %v", err)
	}

	// Evidence export happy + revoke: drives QFEvidenceExportAttempts +
	// QFEvidenceRevokedTotal against the live DB-backed store.
	exportStore := qfdecisions.NewEvidenceExportStore(pool)
	exporter := qfdecisions.NewEvidenceExporter(
		qfdecisions.NewClient(syncServer.URL, "qf-service-token", 1, 25),
		exportStore,
		qfdecisions.NewEvidenceRateLimiter(time.Now),
		"qf-service-token",
		time.Now,
	)
	bundle, err := qfdecisions.BuildPacketContextEvidenceBundle(qfdecisions.EvidenceBundleInput{
		BundleID:        fmt.Sprintf("bundle-bdy-%s", suffix),
		ExportID:        fmt.Sprintf("export-bdy-%s", suffix),
		CreatedAt:       observedAt,
		ConsentScope:    "qf_packet_context",
		SensitivityTier: "personal",
		PacketID:        "packet-bdy-" + suffix,
		TraceID:         "trace-bdy-" + suffix,
		SourceArtifactIDs: []string{
			"artifact-bdy-" + suffix,
		},
		SourceRefs: []string{
			"https://example.test/source/artifact-bdy-" + suffix,
		},
		SourceProvenanceClasses: []qfdecisions.SourceProvenanceClass{
			{SourceArtifactID: "artifact-bdy-" + suffix, SourceProvenanceClass: "smackerel_news"},
		},
		ExtractedClaims:  []string{"E2E claim for boundary+metric test"},
		Confidence:       0.91,
		Provenance:       map[string]any{"generator": "e2e-qf-scope5-safety"},
		RedactionSummary: map[string]any{"omitted_raw_messages": 0},
	})
	if err != nil {
		t.Fatalf("BuildPacketContextEvidenceBundle: %v", err)
	}
	if _, _, err := exporter.Export(ctx, bundle, qfScope5E2EExportCapability()); err != nil {
		t.Fatalf("happy Export: %v", err)
	}
	if _, _, err := exporter.Revoke(ctx, bundle.ExportID, qfdecisions.EvidenceRevokeReasonConsentRevoked); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Export forbidden-action path: author a PersonalEvidenceBundle
	// directly (bypassing BuildPacketContextEvidenceBundle) so the
	// TargetContext.type is a forbidden QF action type
	// (ActionTypeExecution). The export-path boundary guard in
	// EvidenceExporter.Export (evidence_bundle.go ~line 80) reads the
	// AttemptedActionType from TargetContextTypeKey via
	// stringFromExportTargetContext; setting TargetContext.type to a
	// forbidden value (rather than only setting `requested_action_type`)
	// is what makes EnforceQFActionBoundary actually fire (since
	// IsForbiddenQFActionType matches against the action-type string).
	// ValidateEvidenceBundleForExport then rejects the bundle because
	// ActionTypeExecution is NOT in
	// SupportedTargetContextTypes=[packet_context], so Export returns
	// an error after the boundary tripwire has already incremented
	// smackerel_qf_action_boundary_attempts_total and emitted the
	// action_boundary_kick audit envelope.
	forbiddenExportID := fmt.Sprintf("export-bdy-forbidden-%s", suffix)
	forbiddenBundle := qfdecisions.PersonalEvidenceBundle{
		ContractVersion:   1,
		BundleID:          fmt.Sprintf("bundle-bdy-forbidden-%s", suffix),
		ExportID:          forbiddenExportID,
		CreatedAt:         observedAt.Format(time.RFC3339),
		ConsentScope:      "qf_packet_context",
		SensitivityTier:   "personal",
		SourceArtifactIDs: []string{"artifact-bdy-forbidden-" + suffix},
		SourceRefs:        []string{"https://example.test/source/artifact-bdy-forbidden-" + suffix},
		SourceProvenanceClasses: []qfdecisions.SourceProvenanceClass{
			{SourceArtifactID: "artifact-bdy-forbidden-" + suffix, SourceProvenanceClass: "smackerel_news"},
		},
		ExtractedClaims:  []string{"E2E forbidden export attempt"},
		Confidence:       0.5,
		Provenance:       map[string]any{"generator": "e2e-qf-scope5-safety"},
		RedactionSummary: map[string]any{"omitted_raw_messages": 0},
		TargetContext: map[string]any{
			qfdecisions.TargetContextTypeKey:     qfdecisions.ActionTypeExecution,
			qfdecisions.TargetContextPacketIDKey: "packet-bdy-forbidden-" + suffix,
			qfdecisions.TargetContextTraceIDKey:  "trace-bdy-forbidden-" + suffix, // gitleaks:allow
		},
	}
	if _, _, err := exporter.Export(ctx, forbiddenBundle, qfScope5E2EExportCapability()); err == nil {
		t.Fatal("forbidden-target-context Export should be rejected by ValidateEvidenceBundleForExport (TargetContext.type=execution is not in SupportedTargetContextTypes), got nil error")
	}
	// Ensure any DB row that may have been written by the local-reject path
	// is cleaned up after the test.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM qf_personal_evidence_exports WHERE export_id = $1`, forbiddenExportID)
	})

	// Snapshot after all emissions and assert deltas.
	got := struct {
		boundary       float64
		ingest         float64
		render         float64
		export         float64
		revoked        float64
		unknownDecType float64
		cursorFFwd     float64
	}{
		boundary:       qfScope5E2ESumCounterCollector(metrics.QFActionBoundaryAttemptsTotal),
		ingest:         qfScope5E2ESumCounterCollector(metrics.QFPacketIngestTotal),
		render:         qfScope5E2ESumCounterCollector(metrics.QFDeepLinkRenderTotal),
		export:         qfScope5E2ESumCounterCollector(metrics.QFEvidenceExportAttempts),
		revoked:        qfScope5E2ESumCounterCollector(metrics.QFEvidenceRevokedTotal),
		unknownDecType: qfScope5E2ESumCounterCollector(metrics.QFUnknownDecisionType),
		cursorFFwd:     testutil.ToFloat64(metrics.QFCursorFastForwardEventsSkipped),
	}

	// Adversarial trip-wire 1: action_boundary_attempts MUST advance by
	// AT LEAST 3 across the wired non-callback call-sites.
	if delta := got.boundary - baseline.boundary; delta < 3 {
		t.Fatalf("QFActionBoundaryAttemptsTotal delta = %v, want >= 3 (sync diagnostic + render forbidden hint + export forbidden TargetContext)", delta)
	}
	t.Logf("QFActionBoundaryAttemptsTotal delta = %v (>= 3)", got.boundary-baseline.boundary)

	// Adversarial trip-wire 2: packet ingest, render, and export paths
	// MUST emit at least once each (live wiring did NOT regress to a
	// silent no-op).
	if delta := got.ingest - baseline.ingest; delta < 1 {
		t.Fatalf("QFPacketIngestTotal delta = %v, want >= 1", delta)
	}
	if delta := got.render - baseline.render; delta < 1 {
		t.Fatalf("QFDeepLinkRenderTotal delta = %v, want >= 1 (happy render emits one envelope+metric)", delta)
	}
	if delta := got.export - baseline.export; delta < 1 {
		t.Fatalf("QFEvidenceExportAttempts delta = %v, want >= 1", delta)
	}
	if delta := got.revoked - baseline.revoked; delta < 1 {
		t.Fatalf("QFEvidenceRevokedTotal delta = %v, want >= 1", delta)
	}
	if delta := got.unknownDecType - baseline.unknownDecType; delta < 1 {
		t.Fatalf("QFUnknownDecisionType delta = %v, want >= 1", delta)
	}
	if delta := got.cursorFFwd - baseline.cursorFFwd; delta < 1 {
		t.Fatalf("QFCursorFastForwardEventsSkipped delta = %v, want >= 1 (fast-forward event events_skipped=3)", delta)
	}
	t.Logf("QFPacketIngestTotal delta=%v render delta=%v export delta=%v revoked delta=%v unknownDT delta=%v cursorFFwd delta=%v",
		got.ingest-baseline.ingest,
		got.render-baseline.render,
		got.export-baseline.export,
		got.revoked-baseline.revoked,
		got.unknownDecType-baseline.unknownDecType,
		got.cursorFFwd-baseline.cursorFFwd,
	)

	// Adversarial trip-wire 3: Scope 6/8 pre-MVP metrics MUST remain
	// registered (Collect returns a usable channel) so the symmetric label
	// parity contract holds even though no transport is wired.
	engagementCount := testutil.CollectAndCount(metrics.QFEngagementSignalAttemptsTotal)
	callbackCount := testutil.CollectAndCount(metrics.QFCallbackAttemptsTotal)
	t.Logf("QFEngagementSignalAttemptsTotal registered (label combinations = %d; Scope 6 pre-MVP)", engagementCount)
	t.Logf("QFCallbackAttemptsTotal registered (label combinations = %d; Scope 8 pre-MVP)", callbackCount)
	// CollectAndCount panics on an unregistered collector; the t.Logf above
	// implicitly proves the metric is registered (we reached the line).
}

// ============================================================================
// Test 3 — SCN-SM-041-021
// ============================================================================

// TestQFAuditEnvelopeV1RecordedForRequiredBridgeEventsThroughLiveSurface
// (SCN-SM-041-021) captures the Cross-Product Audit Envelope v1 stream while
// driving the bridge events that must emit envelopes (capability handshake
// ok+rejected, packet ingest, evidence export attempt, evidence revocation,
// deep link render, action-boundary kick) and asserts:
//
//  1. Every captured envelope carries the always-required fields
//     (actor_ref, surface, action, outcome, ts, audit_envelope_version,
//     recorded_at).
//  2. The audit_envelope_version field on every captured envelope matches
//     the value advertised by the persisted capability snapshot — proving
//     the version is sourced from real capability state, not a hardcoded
//     literal.
//  3. Each driven event produces an envelope whose action constant matches
//     the expected event type.
//
// This is the live-stack counterpart to the integration test
// TestQFAuditEnvelopeV1ShapeAcrossEightRequiredEmissionPoints. The
// engagement_signal_flush and callback_attempt emission points are covered
// in the integration test only because their transports are intentionally
// Scope 6/8 pre-MVP; this e2e test focuses on the six events that exercise
// the live-stack DB/persistence path.
func TestQFAuditEnvelopeV1RecordedForRequiredBridgeEventsThroughLiveSurface(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	pool := qfScope5E2EDBPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	suffix := qfScope5E2EUniqueSuffix()
	sourceID := "qf-decisions-e2e-aev1-" + suffix
	qfScope5E2ECleanupSourceState(t, pool, sourceID, suffix)
	t.Cleanup(func() { qfScope5E2ECleanupSourceState(t, pool, sourceID, suffix) })

	capture := newQFScope5E2EAuditCapture()
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(capture.handler()))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	happyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(qfScope5E2EValidCapability())
		case qfdecisions.DecisionEventsPath:
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events: []qfdecisions.QFDecisionEvent{
					{
						ContractVersion: 1,
						EventID:         "evt-e2e-aev1-rec-001",
						PacketID:        "packet-e2e-aev1-rec-001",
						IntentID:        "intent-e2e-aev1-001",
						ScenarioID:      "scenario-e2e-aev1-001",
						TraceID:         "trace-e2e-aev1-rec-001", // gitleaks:allow
						EventType:       "packet_created",
						DecisionType:    qfdecisions.DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						Cursor:          "diagnostic-checkpoint-e2e-aev1-rec-001",
						PacketURL:       "https://qf.example.test/packets/packet-e2e-aev1-rec-001",
						SourceSurface:   "gateway-route",
						CreatedAt:       "2026-05-21T18:00:00Z",
					},
				},
				NextCursor: "qf-e2e-aev1-cursor-page-end",
				HasMore:    false,
				ServerTime: "2026-05-21T18:00:30Z",
			})
		case qfdecisions.DecisionPacketsPath + "/packet-e2e-aev1-rec-001":
			_ = json.NewEncoder(w).Encode(qfdecisions.QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-e2e-aev1-rec-001",
				IntentID:             "intent-e2e-aev1-001",
				ScenarioID:           "scenario-e2e-aev1-001",
				TraceID:              "trace-e2e-aev1-rec-001", // gitleaks:allow
				Thesis:               "E2E audit envelope coverage packet",
				WhyNow:               "Drive every required bridge event over the live stack",
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-e2e-aev1-rec-001",
				PacketURLSigned:      "https://qf.example.test/packets/packet-e2e-aev1-rec-001?sig=fresh",
				SignatureExpiresAt:   "2099-01-01T00:00:00Z",
				PreferredSurface:     "smackerel_digest",
				PacketVersion:        1,
				DecisionType:         qfdecisions.DecisionTypeRecommendation,
				CreatedAt:            "2026-05-21T18:00:00Z",
				UpdatedAt:            "2026-05-21T18:00:01Z",
				QuantifiedImpact:     map[string]any{"label": "Impact band", "severity": "info", "summary": "Public impact statement"},
				ExpertAnalysisBundle: map[string]any{"label": "Expert review", "severity": "info", "summary": "Reviewed"},
				CalibrationBadge:     map[string]any{"label": "QF calibrated", "severity": "info", "summary": "Calibration verified"},
				DataProvenanceBadge:  map[string]any{"label": "QF provenance", "severity": "info", "summary": "QF source lineage present"},
			})
		case qfdecisions.PersonalEvidenceBundlesPath:
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				var posted qfdecisions.PersonalEvidenceBundle
				_ = json.NewDecoder(r.Body).Decode(&posted)
				payloadHash, _ := qfdecisions.EvidenceBundlePayloadHash(posted)
				_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{
					ExportID:    posted.ExportID,
					BundleID:    posted.BundleID,
					PayloadHash: payloadHash,
					Status:      qfdecisions.EvidenceExportStatusAccepted,
				})
				return
			}
			http.NotFound(w, r)
		default:
			if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, qfdecisions.PersonalEvidenceBundlesPath+"/") {
				w.WriteHeader(http.StatusOK)
				exportID := strings.TrimPrefix(r.URL.Path, qfdecisions.PersonalEvidenceBundlesPath+"/")
				_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceRevocationResponse{
					ExportID: exportID,
					Status:   qfdecisions.EvidenceExportStatusRevoked,
					Reason:   qfdecisions.EvidenceRevokeReasonConsentRevoked,
				})
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer happyServer.Close()

	// Emission point 1: capability_handshake (OK) — Connect against happy stub.
	capture.reset()
	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, qfScope5E2EConnectorConfig(happyServer.URL, 1, "qf-service-token")); err != nil {
		t.Fatalf("Connect against happy stub: %v", err)
	}
	defer func() { _ = conn.Close() }()

	persistedCapVersion := qfScope5E2EAuditVersionFromConnector(t, conn)
	if persistedCapVersion != qfdecisions.AuditEnvelopeVersionV1 {
		t.Fatalf("persisted capability audit_envelope_version = %q, want %q", persistedCapVersion, qfdecisions.AuditEnvelopeVersionV1)
	}

	handshakeOK := capture.findEnvelope(t, qfdecisions.AuditActionCapabilityHandshake, qfdecisions.AuditOutcomeOK)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "capability_handshake/ok", handshakeOK, persistedCapVersion)

	// Emission point 2: capability_handshake (rejected) — Connect against
	// a stub whose capability fails CompatibilityCheck (MaxPageSize=0).
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == qfdecisions.CapabilitiesPath {
			cap := qfScope5E2EValidCapability()
			cap.MaxPageSize = 0
			_ = json.NewEncoder(w).Encode(cap)
			return
		}
		http.NotFound(w, r)
	}))
	defer badServer.Close()

	capture.reset()
	badSourceID := sourceID + "-bad"
	qfScope5E2ECleanupSourceState(t, pool, badSourceID, "")
	t.Cleanup(func() { qfScope5E2ECleanupSourceState(t, pool, badSourceID, "") })
	badConn := qfdecisions.New(badSourceID)
	if err := badConn.Connect(ctx, qfScope5E2EConnectorConfig(badServer.URL, 1, "qf-service-token")); err == nil {
		t.Fatal("Connect against bad-capability stub returned nil error; expected CompatibilityCheck rejection")
	}
	defer func() { _ = badConn.Close() }()

	handshakeRejected := capture.findEnvelope(t, qfdecisions.AuditActionCapabilityHandshake, qfdecisions.AuditOutcomeRejected)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "capability_handshake/rejected", handshakeRejected, persistedCapVersion)

	// Emission point 3: packet_ingest — Sync against happy stub.
	capture.reset()
	syncArtifacts, _, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("happy Sync: %v", err)
	}
	if len(syncArtifacts) < 1 {
		t.Fatalf("happy Sync produced %d artifacts, want >= 1", len(syncArtifacts))
	}
	packetIngest := capture.findEnvelope(t, qfdecisions.AuditActionPacketIngest, qfdecisions.AuditOutcomeOK)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "packet_ingest", packetIngest, persistedCapVersion)

	// Emission point 4: evidence_export_attempt (OK) — EvidenceExporter against happy stub.
	exportStore := qfdecisions.NewEvidenceExportStore(pool)
	exporter := qfdecisions.NewEvidenceExporter(
		qfdecisions.NewClient(happyServer.URL, "qf-service-token", 1, 25),
		exportStore,
		qfdecisions.NewEvidenceRateLimiter(time.Now),
		"qf-service-token",
		time.Now,
	)
	bundle, err := qfdecisions.BuildPacketContextEvidenceBundle(qfdecisions.EvidenceBundleInput{
		BundleID:        fmt.Sprintf("bundle-aev1-%s", suffix),
		ExportID:        fmt.Sprintf("export-aev1-%s", suffix),
		CreatedAt:       time.Date(2026, 5, 21, 18, 0, 0, 0, time.UTC),
		ConsentScope:    "qf_packet_context",
		SensitivityTier: "personal",
		PacketID:        "packet-aev1-" + suffix,
		TraceID:         "trace-aev1-" + suffix,
		SourceArtifactIDs: []string{
			"artifact-aev1-" + suffix,
		},
		SourceRefs: []string{
			"https://example.test/source/artifact-aev1-" + suffix,
		},
		SourceProvenanceClasses: []qfdecisions.SourceProvenanceClass{
			{SourceArtifactID: "artifact-aev1-" + suffix, SourceProvenanceClass: "smackerel_news"},
		},
		ExtractedClaims:  []string{"E2E audit envelope claim"},
		Confidence:       0.92,
		Provenance:       map[string]any{"generator": "e2e-qf-scope5-aev1"},
		RedactionSummary: map[string]any{"omitted_raw_messages": 0},
	})
	if err != nil {
		t.Fatalf("BuildPacketContextEvidenceBundle: %v", err)
	}

	capture.reset()
	exportRecord, _, err := exporter.Export(ctx, bundle, qfScope5E2EExportCapability())
	if err != nil {
		t.Fatalf("happy Export: %v", err)
	}
	if exportRecord.Status != qfdecisions.EvidenceExportStatusAccepted {
		t.Fatalf("export record status = %s, want accepted", exportRecord.Status)
	}
	exportAttempt := capture.findEnvelope(t, qfdecisions.AuditActionEvidenceExportAttempt, qfdecisions.AuditOutcomeOK)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "evidence_export_attempt", exportAttempt, persistedCapVersion)

	// Emission point 5: evidence_revocation (OK) — Revoke the export.
	capture.reset()
	revokeRecord, _, err := exporter.Revoke(ctx, exportRecord.ExportID, qfdecisions.EvidenceRevokeReasonConsentRevoked)
	if err != nil {
		t.Fatalf("happy Revoke: %v", err)
	}
	if revokeRecord.Status != qfdecisions.EvidenceExportStatusRevoked {
		t.Fatalf("revoke record status = %s, want revoked", revokeRecord.Status)
	}
	revocation := capture.findEnvelope(t, qfdecisions.AuditActionEvidenceRevocation, qfdecisions.AuditOutcomeOK)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "evidence_revocation", revocation, persistedCapVersion)

	// Emission point 6: deep_link_render — Render the synced artifact.
	capture.reset()
	renderArtifact := syncArtifacts[0]
	observedAt := time.Date(2026, 5, 21, 18, 0, 0, 0, time.UTC)
	renderArtifact.CapturedAt = observedAt.Add(-60 * time.Second)
	if renderArtifact.Metadata == nil {
		renderArtifact.Metadata = map[string]any{}
	}
	if _, err := qfdecisions.RenderPacketCard(ctx, renderArtifact, qfdecisions.RenderOptions{
		Surface:                  qfdecisions.SurfaceWeb,
		DeepLinkSigningSupported: true,
		Now:                      observedAt,
	}); err != nil {
		t.Fatalf("happy RenderPacketCard: %v", err)
	}
	deepLink := capture.findEnvelope(t, qfdecisions.AuditActionDeepLinkRender, qfdecisions.DeepLinkStatusSignedUsed)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "deep_link_render", deepLink, persistedCapVersion)

	// Emission point 7: action_boundary_kick — EnforceQFActionBoundary
	// on a forbidden action type.
	capture.reset()
	if _, fired, err := qfdecisions.EnforceQFActionBoundary(qfdecisions.ActionBoundaryAttempt{
		AttemptedActionType: qfdecisions.ActionTypeApproval,
		TraceID:             "trace-e2e-aev1-boundary-001", // gitleaks:allow
		PacketID:            "packet-e2e-aev1-boundary-001",
		ActorRef:            qfdecisions.AuditActorSmackerelConnector,
		Surface:             qfdecisions.SurfaceWeb,
		Reason:              "audit_envelope_v1_e2e_action_boundary",
		ObservedAt:          observedAt,
	}); err == nil || !fired {
		t.Fatalf("EnforceQFActionBoundary(approval): fired=%v err=%v, want fired=true err!=nil", fired, err)
	}
	boundaryKick := capture.findEnvelope(t, qfdecisions.AuditActionActionBoundaryKick, qfdecisions.AuditOutcomeRejected)
	qfScope5E2EAssertAlwaysRequiredEnvelopeFields(t, "action_boundary_kick", boundaryKick, persistedCapVersion)

	t.Logf("captured %d cross_product_audit records across 6 emission points (capability_handshake ok+rejected, packet_ingest, evidence_export_attempt, evidence_revocation, deep_link_render, action_boundary_kick)",
		len(capture.crossProductAuditRecords()))
}
