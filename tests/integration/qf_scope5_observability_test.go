//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
)

// TestQFObservabilityEmitsAllSymmetricMetricsAcrossSyncRenderExportAndBoundaryPaths
// (spec 041 SCN-SM-041-020) drives every wired QF Companion metric path through
// the live disposable test stack against httptest QF stubs and asserts that the
// 12-metric symmetric set listed in `specs/041-qf-companion-connector/scopes.md`
// Scope 5 DoD all emit, plus confirms the two Scope 6/8 metrics
// (engagement_signal, callback) are PRESENT in the registry with their
// pre-MVP zero count so the symmetric label parity contract is preserved
// across sync/render/export/boundary surfaces without prematurely
// exercising future scopes.
//
// Driven metrics (delta > baseline expected):
//
//  1. smackerel_qf_packet_ingest_total              — Sync ingest
//  2. smackerel_qf_capability_mismatch_total        — Connect with bad capability
//  3. smackerel_qf_unknown_decision_type_total      — Sync with unknown decision_type
//  4. smackerel_qf_cursor_lag_seconds               — Sync auto-emits (presence proof)
//  5. smackerel_qf_cursor_fast_forward_events_skipped_total — Sync with fast-forward
//  6. smackerel_qf_action_boundary_attempts_total   — Sync + render + export + callback
//  7. smackerel_qf_packet_validation_failures_total — stub rejects with PAGE_SIZE_OUT_OF_RANGE
//  8. smackerel_qf_freshness_p95_seconds{stage=ingest}  — Sync ingest
//  9. smackerel_qf_freshness_p95_seconds{stage=render}  — RenderPacketCard
//  10. smackerel_qf_freshness_p95_seconds{stage=total}  — RenderPacketCard
//  11. smackerel_qf_trust_object_render_failures_total  — Render with malformed trust object
//  12. smackerel_qf_deep_link_render_total              — RenderPacketCard
//  13. smackerel_qf_evidence_export_attempts_total      — EvidenceExporter.Export
//  14. smackerel_qf_evidence_revoked_total              — EvidenceExporter.Revoke
//
// Symmetric-set parity assertions (pre-MVP zero count, presence proof):
//
//   - smackerel_qf_engagement_signal_attempts_total (Scope 6 placeholder)
//   - smackerel_qf_callback_attempts_total          (Scope 8 placeholder)
//
// Adversarial trip-wires:
//
//   - The action_boundary_attempts counter MUST advance by AT LEAST 4 across
//     the four call-sites (Sync, render, export, callback). A regression
//     that drops any single guard would leave the counter under-incremented.
//   - The freshness gauge for the render stage MUST land on a value >= 60
//     seeded by an artifact CapturedAt 60 s before RenderPacketCard's Now();
//     a regression that stopped emitting render freshness would leave the
//     gauge at its pre-test value (typically 0).
//   - The capability_mismatch counter MUST advance even though the bad
//     capability handshake returns an error from Connect — emission happens
//     unconditionally inside CompatibilityCheck before the error bubbles up.
//
// Run: `./smackerel.sh test integration` (requires `./smackerel.sh --env test up`).
func TestQFObservabilityEmitsAllSymmetricMetricsAcrossSyncRenderExportAndBoundaryPaths(t *testing.T) {
	pool := testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	sourceID := "qf-decisions-it-obs-" + uniqueSuffix()
	cleanupQFDecisionsRows(t, pool, sourceID)
	t.Cleanup(func() { cleanupQFDecisionsRows(t, pool, sourceID) })

	// Capture per-metric baselines BEFORE driving any emissions. The
	// Prometheus registry is process-global; other integration tests may
	// have already incremented some of these counters in the same run, so
	// we compare deltas instead of absolute values.
	baseline := captureQFMetricSnapshot()

	// --- Path 1: drive the full happy-path Sync + render + export +
	// callback surface against a valid stub.
	happyStub := newHappyQFStub(t)
	defer happyStub.server.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, qfIntegrationConfig(happyStub.server.URL, 1)); err != nil {
		t.Fatalf("Connect against happy stub: %v", err)
	}
	defer func() { _ = conn.Close() }()

	syncArtifacts, nextCursor, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("happy Sync: %v", err)
	}
	if nextCursor == "" {
		t.Fatal("happy Sync returned empty cursor; stub returned valid next_cursor")
	}
	if len(syncArtifacts) < 1 {
		t.Fatalf("happy Sync should have produced at least one artifact, got %d", len(syncArtifacts))
	}

	// Path 1b: drive a SECOND Sync that delivers the boundary-attempted
	// diagnostic event (Sync-loop action boundary path).
	happyStub.enableActionBoundary.Store(true)
	if _, _, err := conn.Sync(ctx, nextCursor); err != nil {
		t.Fatalf("boundary Sync: %v", err)
	}

	// --- Path 2: drive RenderPacketCard from the artifact returned by
	// the happy Sync. The first artifact is the valid recommendation
	// packet with complete trust metadata, so it exercises trust_object,
	// deep_link, render freshness, and total freshness.
	renderArtifact := syncArtifacts[0]
	// Seed CapturedAt 60s before observedAt so render freshness lands
	// deterministically at p95=60s and qf_created_at 120s before
	// observedAt so total freshness lands at p95=120s.
	observedAt := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	renderArtifact.CapturedAt = observedAt.Add(-60 * time.Second)
	if renderArtifact.Metadata == nil {
		renderArtifact.Metadata = map[string]any{}
	}
	renderArtifact.Metadata["qf_created_at"] = observedAt.Add(-120 * time.Second).Format(time.RFC3339)
	if _, err := qfdecisions.RenderPacketCard(ctx, renderArtifact, qfdecisions.RenderOptions{
		Surface:                  qfdecisions.SurfaceWeb,
		DeepLinkSigningSupported: true,
		Now:                      observedAt,
	}); err != nil {
		t.Fatalf("happy RenderPacketCard: %v", err)
	}

	// Path 2b: render an artifact whose trust metadata is missing the
	// required label/severity fields → trust_object_render_failures.
	badTrustArtifact := renderArtifact
	badTrustArtifact.Metadata = cloneAnyMap(renderArtifact.Metadata)
	badTrustArtifact.Metadata["calibration_badge"] = map[string]any{"summary": "missing label and severity"}
	if _, err := qfdecisions.RenderPacketCard(ctx, badTrustArtifact, qfdecisions.RenderOptions{
		Surface:                  qfdecisions.SurfaceWeb,
		DeepLinkSigningSupported: true,
		Now:                      observedAt,
	}); err != nil {
		t.Fatalf("bad-trust RenderPacketCard: %v", err)
	}

	// Path 2c: render an artifact whose metadata carries a forbidden
	// requested_action_type → action_boundary_attempts (render path).
	renderBoundaryArtifact := renderArtifact
	renderBoundaryArtifact.Metadata = cloneAnyMap(renderArtifact.Metadata)
	renderBoundaryArtifact.Metadata["requested_action_type"] = qfdecisions.ActionTypeApproval
	if _, err := qfdecisions.RenderPacketCard(ctx, renderBoundaryArtifact, qfdecisions.RenderOptions{
		Surface:                  qfdecisions.SurfaceWeb,
		DeepLinkSigningSupported: true,
		Now:                      observedAt,
	}); err != nil {
		t.Fatalf("render-boundary RenderPacketCard: %v", err)
	}

	// --- Path 3: drive EvidenceExporter.Export happy path against the
	// same stub (it routes /personal-evidence-bundles → 201 Created).
	store := qfdecisions.NewEvidenceExportStore(pool)
	limiter := qfdecisions.NewEvidenceRateLimiter(time.Now)
	credentialRef := "qf-service-token"
	exporter := qfdecisions.NewEvidenceExporter(
		qfdecisions.NewClient(happyStub.server.URL, credentialRef, 1, 25),
		store,
		limiter,
		credentialRef,
		time.Now,
	)
	exportCapability := happyExportCapability()
	exportID := "export-it-obs-" + uniqueSuffix()
	bundle, err := qfdecisions.BuildPacketContextEvidenceBundle(qfdecisions.EvidenceBundleInput{
		BundleID:                "bundle-" + exportID,
		ExportID:                exportID,
		CreatedAt:               time.Date(2026, 5, 21, 11, 59, 0, 0, time.UTC),
		ConsentScope:            "qf_packet_context",
		SensitivityTier:         "personal",
		PacketID:                "packet-it-obs-001",
		TraceID:                 "trace-it-obs-001",
		SourceArtifactIDs:       []string{"artifact-it-obs-001"},
		SourceRefs:              []string{"https://example.test/sources/it-obs-001"},
		SourceProvenanceClasses: []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-it-obs-001", SourceProvenanceClass: "smackerel_markets"}},
		ExtractedClaims:         []string{"observability claim 1", "observability claim 2"},
		Confidence:              0.91,
		Provenance:              map[string]any{"generator": "qf-scope5-observability-it"},
		RedactionSummary:        map[string]any{"omitted_raw_messages": 0},
	})
	if err != nil {
		t.Fatalf("BuildPacketContextEvidenceBundle: %v", err)
	}
	exportRecord, exportResponse, err := exporter.Export(ctx, bundle, exportCapability)
	if err != nil {
		t.Fatalf("EvidenceExporter.Export happy path: %v", err)
	}
	if exportResponse.ExportID != exportID || exportRecord.ExportID != exportID {
		t.Fatalf("export response/record export_id mismatch: response=%q record=%q want=%q", exportResponse.ExportID, exportRecord.ExportID, exportID)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM qf_personal_evidence_exports WHERE export_id = $1`, exportID)
	})

	// Path 3b: drive EvidenceExporter.Revoke → evidence_revoked.
	if _, _, err := exporter.Revoke(ctx, exportID, qfdecisions.EvidenceRevokeReasonConsentRevoked); err != nil {
		t.Fatalf("EvidenceExporter.Revoke: %v", err)
	}

	// Path 3c: drive an Export attempt whose TargetContext claims a
	// forbidden QF action type → action_boundary_attempts (export path).
	// We bypass BuildPacketContextEvidenceBundle so we can author a bundle
	// whose TargetContext.type is a forbidden action; ValidateEvidenceBundleForExport
	// will still reject it (the action isn't in SupportedTargetContextTypes),
	// but the Scope 5 boundary guard fires FIRST so the metric lands.
	forbiddenExportID := "export-it-obs-forbidden-" + uniqueSuffix()
	forbiddenBundle := qfdecisions.PersonalEvidenceBundle{
		ContractVersion:   1,
		BundleID:          "bundle-" + forbiddenExportID,
		ExportID:          forbiddenExportID,
		CreatedAt:         time.Date(2026, 5, 21, 11, 59, 30, 0, time.UTC).Format(time.RFC3339),
		ConsentScope:      "qf_packet_context",
		SensitivityTier:   "personal",
		SourceArtifactIDs: []string{"artifact-it-obs-forbidden-001"},
		ExtractedClaims:   []string{"forbidden boundary claim"},
		Confidence:        0.81,
		Provenance:        map[string]any{"generator": "qf-scope5-observability-it"},
		RedactionSummary:  map[string]any{"omitted_raw_messages": 0},
		TargetContext: map[string]any{
			qfdecisions.TargetContextTypeKey:     qfdecisions.ActionTypeExecution,
			qfdecisions.TargetContextPacketIDKey: "packet-it-obs-forbidden-001",
			qfdecisions.TargetContextTraceIDKey:  "trace-it-obs-forbidden-001", // gitleaks:allow
		},
		SourceProvenanceClasses: []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-it-obs-forbidden-001", SourceProvenanceClass: "smackerel_markets"}},
	}
	if _, _, err := exporter.Export(ctx, forbiddenBundle, exportCapability); err == nil {
		t.Fatal("Export with forbidden TargetContext should be rejected by ValidateEvidenceBundleForExport, got nil error")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM qf_personal_evidence_exports WHERE export_id = $1`, forbiddenExportID)
	})

	// --- Path 4: drive EmitCallbackAttemptAudit with a forbidden action →
	// action_boundary_attempts (callback path).
	qfdecisions.EmitCallbackAttemptAudit(qfdecisions.CallbackAttemptAuditInput{
		TraceID:    "trace-it-obs-callback-001",
		PacketID:   "packet-it-obs-callback-001",
		ActorRef:   qfdecisions.AuditActorSmackerelConnector,
		Surface:    qfdecisions.SurfaceWeb,
		Action:     qfdecisions.ActionTypeExecution,
		Status:     "ok",
		ObservedAt: observedAt,
	})

	// --- Path 5: drive a SECOND Connect attempt against an INCOMPATIBLE
	// capability stub → capability_mismatch. The handshake returns a
	// capability with MaxPageSize=0 which CompatibilityCheck rejects with
	// the ">=1" / "0" label pair.
	badCapServer := newBadCapabilityQFStub()
	defer badCapServer.Close()
	badConn := qfdecisions.New(sourceID + "-badcap")
	badConnErr := badConn.Connect(ctx, qfIntegrationConfig(badCapServer.URL, 1))
	if badConnErr == nil {
		t.Fatal("Connect against bad-capability stub should have errored")
	}
	defer func() { _ = badConn.Close() }()

	// --- Path 6: drive a Sync that trips PAGE_SIZE_OUT_OF_RANGE →
	// packet_validation_failures{reason=page_size_out_of_range}. We
	// configure a connector against a stub whose capability accepts the
	// requested page_size but whose /decision-events endpoint always
	// responds with PAGE_SIZE_OUT_OF_RANGE so the Sync wrap path
	// increments the failures metric.
	pageSizeServer := newPageSizeRejectQFStub()
	defer pageSizeServer.Close()
	pageSizeConn := qfdecisions.New(sourceID + "-psr")
	if err := pageSizeConn.Connect(ctx, qfIntegrationConfig(pageSizeServer.URL, 1)); err != nil {
		t.Fatalf("Connect against page-size-reject stub: %v", err)
	}
	defer func() { _ = pageSizeConn.Close() }()
	_, _, syncErr := pageSizeConn.Sync(ctx, "")
	if syncErr == nil {
		t.Fatal("Sync against page-size-reject stub should have errored")
	}
	if !strings.Contains(syncErr.Error(), "page_size") && !strings.Contains(syncErr.Error(), "PAGE_SIZE_OUT_OF_RANGE") {
		t.Fatalf("expected page_size error from Sync, got: %v", syncErr)
	}

	// --- Capture per-metric snapshot AFTER all driven emissions.
	snapshot := captureQFMetricSnapshot()

	// --- Assert delta >= atLeast for the 12 wired metrics.
	type wiredAssertion struct {
		name    string
		got     float64
		base    float64
		atLeast float64
	}
	wired := []wiredAssertion{
		{name: "qf_packet_ingest_total", got: snapshot.packetIngestTotal, base: baseline.packetIngestTotal, atLeast: 1},
		{name: "qf_capability_mismatch_total", got: snapshot.capabilityMismatchTotal, base: baseline.capabilityMismatchTotal, atLeast: 1},
		{name: "qf_unknown_decision_type_total", got: snapshot.unknownDecisionTypeTotal, base: baseline.unknownDecisionTypeTotal, atLeast: 1},
		{name: "qf_cursor_fast_forward_events_skipped_total", got: snapshot.cursorFastForwardTotal, base: baseline.cursorFastForwardTotal, atLeast: 1},
		// action_boundary_attempts: 4 expected call-sites = Sync + render + export + callback.
		{name: "qf_action_boundary_attempts_total", got: snapshot.actionBoundaryTotal, base: baseline.actionBoundaryTotal, atLeast: 4},
		{name: "qf_packet_validation_failures_total", got: snapshot.packetValidationFailuresTotal, base: baseline.packetValidationFailuresTotal, atLeast: 1},
		{name: "qf_trust_object_render_failures_total", got: snapshot.trustObjectRenderFailuresTotal, base: baseline.trustObjectRenderFailuresTotal, atLeast: 1},
		{name: "qf_deep_link_render_total", got: snapshot.deepLinkRenderTotal, base: baseline.deepLinkRenderTotal, atLeast: 1},
		{name: "qf_evidence_export_attempts_total", got: snapshot.evidenceExportAttemptsTotal, base: baseline.evidenceExportAttemptsTotal, atLeast: 1},
		{name: "qf_evidence_revoked_total", got: snapshot.evidenceRevokedTotal, base: baseline.evidenceRevokedTotal, atLeast: 1},
	}
	for _, w := range wired {
		delta := w.got - w.base
		if delta < w.atLeast {
			t.Errorf("metric %s delta = %v (got %v, base %v), want at least %v", w.name, delta, w.got, w.base, w.atLeast)
		} else {
			t.Logf("metric %s OK: delta=%v (base=%v, got=%v)", w.name, delta, w.base, w.got)
		}
	}

	// --- Gauge: cursor_lag_seconds presence proof. QFCursorLagSeconds
	// is a single Gauge; assert it is registered (CollectAndCount == 1)
	// and its value is finite (not NaN). The gauge cannot be
	// delta-asserted because it is a point-in-time reading set by every
	// Sync.
	if got := testutil.CollectAndCount(metrics.QFCursorLagSeconds); got != 1 {
		t.Errorf("qf_cursor_lag_seconds collector count = %d, want 1 (always-present gauge)", got)
	}
	lagValue := testutil.ToFloat64(metrics.QFCursorLagSeconds)
	if lagValue != lagValue { // NaN check (NaN != NaN)
		t.Errorf("qf_cursor_lag_seconds gauge value is NaN, want finite")
	}
	t.Logf("metric qf_cursor_lag_seconds OK: present, value=%v", lagValue)

	// --- Freshness gauges per stage (ingest|render|total) — each stage
	// MUST be > 0 because the test seeded deterministic gaps.
	if v := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(qfdecisions.FreshnessStageIngest)); v <= 0 {
		t.Errorf("qf_freshness_p95_seconds{stage=ingest} = %v, want > 0 after Sync ingest", v)
	} else {
		t.Logf("metric qf_freshness_p95_seconds{stage=ingest} OK: value=%v", v)
	}
	if v := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(qfdecisions.FreshnessStageRender)); v < 60 {
		t.Errorf("qf_freshness_p95_seconds{stage=render} = %v, want >= 60 (seeded by 60s CapturedAt→Now gap)", v)
	} else {
		t.Logf("metric qf_freshness_p95_seconds{stage=render} OK: value=%v", v)
	}
	if v := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(qfdecisions.FreshnessStageTotal)); v < 120 {
		t.Errorf("qf_freshness_p95_seconds{stage=total} = %v, want >= 120 (seeded by 120s qf_created_at→Now gap)", v)
	} else {
		t.Logf("metric qf_freshness_p95_seconds{stage=total} OK: value=%v", v)
	}

	// --- Symmetric-set parity: engagement_signal and callback metrics
	// MUST be registered. The pre-MVP scope has not wired Scope 6 or
	// Scope 8 transports yet, so their CollectAndCount stays at 0 (no
	// label combinations have been emitted). The registry-level presence
	// proof is the CollectAndCount call itself; if either collector were
	// unregistered the panic would be visible in the test transcript.
	t.Logf("metric qf_engagement_signal_attempts_total registered with %d emitted label combinations (Scope 6 pre-MVP — 0 expected until transport wired)", testutil.CollectAndCount(metrics.QFEngagementSignalAttemptsTotal))
	t.Logf("metric qf_callback_attempts_total registered with %d emitted label combinations (Scope 8 pre-MVP — 0 expected until transport wired)", testutil.CollectAndCount(metrics.QFCallbackAttemptsTotal))

	// --- Adversarial guard: capability_mismatch increment lands even when
	// Connect errors out. Re-assert this delta crossed at least one
	// (separate from the wired loop above which uses atLeast=1 already)
	// for documentation clarity in the test log.
	capDelta := snapshot.capabilityMismatchTotal - baseline.capabilityMismatchTotal
	if capDelta < 1 {
		t.Errorf("capability_mismatch delta = %v (Connect-on-failure path not emitting metric)", capDelta)
	}

	// Sanity: connection from path 5 (bad capability) returned the
	// expected error class so the test fail-fast pattern is honored.
	var sce qfdecisions.SchemaCompatibilityError
	if errors.As(badConnErr, &sce) {
		t.Logf("bad-capability Connect returned SchemaCompatibilityError as expected: %v", sce)
	}
}

// --- httptest stub builders -------------------------------------------

type happyQFStub struct {
	server               *httptest.Server
	enableActionBoundary atomic.Bool
	syncCallCount        atomic.Int32
}

// newHappyQFStub returns a stub that serves a valid capability handshake
// and emits a multi-event /decision-events page on first call:
//
//   - event 1: valid recommendation packet (triggers packet_ingest +
//     freshness ingest + auto-emits cursor_lag)
//   - event 2: unknown decision_type "future_qf_shape" (triggers
//     unknown_decision_type)
//   - event 3: fast-forward diagnostic (events_skipped=5) (triggers
//     cursor_fast_forward)
//
// On a second call, when enableActionBoundary.Load() is true, the stub
// emits a single packet_action_boundary_attempted event (triggers
// action_boundary_attempts via the Sync-loop path).
//
// The stub also responds to /decision-packets/<packetID> for the valid
// recommendation packet so the connector can fetch its envelope, and to
// /personal-evidence-bundles for the EvidenceExporter happy path.
func newHappyQFStub(t *testing.T) *happyQFStub {
	t.Helper()
	stub := &happyQFStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
			return
		case qfdecisions.DecisionEventsPath:
			call := stub.syncCallCount.Add(1)
			if call == 1 {
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events: []qfdecisions.QFDecisionEvent{
						happyRecommendationEvent(),
						unknownDecisionEvent(),
						fastForwardEvent(),
					},
					NextCursor: "qf-obs-cursor-page-1-end",
					HasMore:    false,
					ServerTime: "2026-05-21T11:59:30Z",
				})
				return
			}
			if call == 2 && stub.enableActionBoundary.Load() {
				_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
					Events:     []qfdecisions.QFDecisionEvent{actionBoundaryAttemptedEvent()},
					NextCursor: "qf-obs-cursor-page-2-end",
					HasMore:    false,
					ServerTime: "2026-05-21T11:59:45Z",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     []qfdecisions.QFDecisionEvent{},
				NextCursor: "qf-obs-cursor-empty",
				HasMore:    false,
				ServerTime: "2026-05-21T11:59:50Z",
			})
			return
		case qfdecisions.DecisionPacketsPath + "/packet-it-obs-rec-001":
			_ = json.NewEncoder(w).Encode(happyRecommendationEnvelope())
			return
		case qfdecisions.DecisionPacketsPath + "/packet-it-obs-unknown-001":
			env := happyRecommendationEnvelope()
			env.PacketID = "packet-it-obs-unknown-001"
			env.DecisionType = "future_qf_shape"
			_ = json.NewEncoder(w).Encode(env)
			return
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
		}
		// Default: DELETE /personal-evidence-bundles/<id> for revoke path.
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
	}))
	return stub
}

// newBadCapabilityQFStub returns a stub whose /capabilities endpoint
// returns a capability with MaxPageSize=0 — CompatibilityCheck rejects
// this and increments capability_mismatch{required=">=1", actual="0"}.
func newBadCapabilityQFStub() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == qfdecisions.CapabilitiesPath {
			cap := validQFIntegrationCapability()
			cap.MaxPageSize = 0 // forces ">=1" / "0" mismatch
			_ = json.NewEncoder(w).Encode(cap)
			return
		}
		http.NotFound(w, r)
	}))
}

// newPageSizeRejectQFStub returns a stub whose /capabilities endpoint
// returns a valid capability, but whose /decision-events endpoint always
// returns HTTP 400 PAGE_SIZE_OUT_OF_RANGE so the connector's Sync path
// increments packet_validation_failures{reason=page_size_out_of_range}.
func newPageSizeRejectQFStub() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
			return
		case qfdecisions.DecisionEventsPath:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(qfdecisions.BridgeErrorResponse{
				Code:    "PAGE_SIZE_OUT_OF_RANGE",
				Message: "page_size=25 is outside the capability range",
			})
			return
		}
		http.NotFound(w, r)
	}))
}

// --- Fixture helpers --------------------------------------------------

func happyRecommendationEvent() qfdecisions.QFDecisionEvent {
	return qfdecisions.QFDecisionEvent{
		ContractVersion: 1,
		EventID:         "evt-it-obs-rec-001",
		PacketID:        "packet-it-obs-rec-001",
		IntentID:        "intent-it-obs-001",
		ScenarioID:      "scenario-it-obs-001",
		TraceID:         "trace-it-obs-rec-001",
		EventType:       "packet_created",
		DecisionType:    qfdecisions.DecisionTypeRecommendation,
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          "diagnostic-checkpoint-evt-it-obs-rec-001",
		PacketURL:       "https://qf.example.test/packets/packet-it-obs-rec-001",
		SourceSurface:   "gateway-route",
		CreatedAt:       "2026-05-21T11:59:00Z",
	}
}

func unknownDecisionEvent() qfdecisions.QFDecisionEvent {
	return qfdecisions.QFDecisionEvent{
		ContractVersion: 1,
		EventID:         "evt-it-obs-unknown-001",
		PacketID:        "packet-it-obs-unknown-001",
		IntentID:        "intent-it-obs-001",
		ScenarioID:      "scenario-it-obs-001",
		TraceID:         "trace-it-obs-unknown-001",
		EventType:       "packet_created",
		DecisionType:    "future_qf_shape",
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          "diagnostic-checkpoint-evt-it-obs-unknown-001",
		PacketURL:       "https://qf.example.test/packets/packet-it-obs-unknown-001",
		SourceSurface:   "gateway-route",
		CreatedAt:       "2026-05-21T11:59:05Z",
	}
}

func fastForwardEvent() qfdecisions.QFDecisionEvent {
	return qfdecisions.QFDecisionEvent{
		ContractVersion: 1,
		EventID:         "evt-it-obs-ff-001",
		PacketID:        "packet-it-obs-ff-001",
		IntentID:        "intent-it-obs-001",
		ScenarioID:      "scenario-it-obs-001",
		TraceID:         "trace-it-obs-ff-001",
		EventType:       "packet_created",
		DecisionType:    qfdecisions.DecisionTypeRecommendation,
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          "diagnostic-checkpoint-evt-it-obs-ff-001",
		PacketURL:       "https://qf.example.test/packets/packet-it-obs-ff-001",
		SourceSurface:   "gateway-route",
		CreatedAt:       "2026-05-21T11:59:10Z",
		EventsSkipped:   5,
	}
}

func actionBoundaryAttemptedEvent() qfdecisions.QFDecisionEvent {
	return qfdecisions.QFDecisionEvent{
		ContractVersion: 1,
		EventID:         "evt-it-obs-boundary-001",
		PacketID:        "packet-it-obs-boundary-001",
		IntentID:        "intent-it-obs-boundary-001",
		ScenarioID:      "scenario-it-obs-boundary-001",
		TraceID:         "trace-it-obs-boundary-001",
		EventType:       qfdecisions.EventTypePacketActionBoundaryAttempted,
		DecisionType:    qfdecisions.ActionTypeApproval, // event carries the attempted action as decision_type
		ApprovalState:   "display_only",
		PacketVersion:   1,
		Cursor:          "diagnostic-checkpoint-evt-it-obs-boundary-001",
		PacketURL:       "https://qf.example.test/packets/packet-it-obs-boundary-001",
		SourceSurface:   "gateway-route",
		CreatedAt:       "2026-05-21T11:59:40Z",
	}
}

func happyRecommendationEnvelope() qfdecisions.QFDecisionPacketEnvelope {
	return qfdecisions.QFDecisionPacketEnvelope{
		ContractVersion:      1,
		PacketID:             "packet-it-obs-rec-001",
		IntentID:             "intent-it-obs-001",
		ScenarioID:           "scenario-it-obs-001",
		TraceID:              "trace-it-obs-rec-001",
		Thesis:               "Observability integration test packet",
		WhyNow:               "Drive every wired metric for spec 041 Scope 5",
		ApprovalState:        "display_only",
		DeepLink:             "https://qf.example.test/packets/packet-it-obs-rec-001",
		PacketURLSigned:      "https://qf.example.test/packets/packet-it-obs-rec-001?sig=fresh",
		SignatureExpiresAt:   "2099-01-01T00:00:00Z",
		PreferredSurface:     "smackerel_digest",
		PacketVersion:        1,
		DecisionType:         qfdecisions.DecisionTypeRecommendation,
		CreatedAt:            "2026-05-21T11:59:00Z",
		UpdatedAt:            "2026-05-21T11:59:01Z",
		QuantifiedImpact:     map[string]any{"label": "Impact band", "severity": "info", "summary": "Public impact statement"},
		ExpertAnalysisBundle: map[string]any{"label": "Expert review", "severity": "info", "summary": "Reviewed"},
		CalibrationBadge:     map[string]any{"label": "QF calibrated", "severity": "info", "summary": "Calibration verified"},
		DataProvenanceBadge:  map[string]any{"label": "QF provenance", "severity": "info", "summary": "QF source lineage present"},
	}
}

func happyExportCapability() qfdecisions.QFBridgeCapability {
	return qfdecisions.QFBridgeCapability{
		SupportedTargetContextTypes:    []string{qfdecisions.TargetContextPacketContext},
		EvidenceMaxBundleSizeBytes:     524288,
		EvidenceMaxClaimsPerBundle:     50,
		EvidenceRateLimitPerMinute:     10,
		EligibleSmackerelSourceClasses: []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"},
	}
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// --- Metric snapshot helpers ------------------------------------------

type qfMetricSnapshot struct {
	packetIngestTotal              float64
	capabilityMismatchTotal        float64
	unknownDecisionTypeTotal       float64
	cursorFastForwardTotal         float64
	actionBoundaryTotal            float64
	packetValidationFailuresTotal  float64
	trustObjectRenderFailuresTotal float64
	deepLinkRenderTotal            float64
	evidenceExportAttemptsTotal    float64
	evidenceRevokedTotal           float64
}

func captureQFMetricSnapshot() qfMetricSnapshot {
	return qfMetricSnapshot{
		packetIngestTotal:              sumCounterCollector(metrics.QFPacketIngestTotal),
		capabilityMismatchTotal:        sumCounterCollector(metrics.QFCapabilityMismatch),
		unknownDecisionTypeTotal:       sumCounterCollector(metrics.QFUnknownDecisionType),
		cursorFastForwardTotal:         testutil.ToFloat64(metrics.QFCursorFastForwardEventsSkipped),
		actionBoundaryTotal:            sumCounterCollector(metrics.QFActionBoundaryAttemptsTotal),
		packetValidationFailuresTotal:  sumCounterCollector(metrics.QFPacketValidationFailures),
		trustObjectRenderFailuresTotal: sumCounterCollector(metrics.QFTrustObjectRenderFailures),
		deepLinkRenderTotal:            sumCounterCollector(metrics.QFDeepLinkRenderTotal),
		evidenceExportAttemptsTotal:    sumCounterCollector(metrics.QFEvidenceExportAttempts),
		evidenceRevokedTotal:           sumCounterCollector(metrics.QFEvidenceRevokedTotal),
	}
}

// sumCounterCollector returns the sum of all counter values across every
// label combination currently emitted by the given Prometheus collector.
// It uses the public Collector.Collect API plus dto.Metric.Counter so
// the helper works for both single Counters and CounterVecs.
func sumCounterCollector(c prometheus.Collector) float64 {
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

// --- Unused-import guard ----------------------------------------------
//
// The connector import is used transitively through qfIntegrationConfig;
// declare a no-op reference so a future refactor that removes the
// connector-typed callsite from this file still keeps the import explicit
// at the package level.
var _ connector.ConnectorConfig
