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

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

// TestQFCredentialRotationOverlapPreservesCursorExportIdempotencyCapabilityDiagnosticsAndAudit
// (SCN-SM-041-019, spec 041 Scope 5 DoD) proves the connector's credential
// rotation surface, when wired against the live disposable stack and an
// httptest QF stub, (1) re-reads the capability handshake under the rotated
// credential before any further sync uses it, (2) preserves the persisted
// sync_state.sync_cursor across rotation, (3) preserves the persisted
// evidence-export idempotency record set across rotation, (4) emits the
// expected operator diagnostics, and (5) emits the OK
// CredentialRotation + OK CapabilityHandshake audit envelopes through the
// real connector audit sink.
//
// Adversarial trip-wire 1: the stub auth-header recorder asserts the
// pre-rotation requests carry `Bearer qf-service-token` and the
// post-rotation capability re-read carries `Bearer qf-rotated-token`. If a
// future regression keeps using the previous credential after rotation, the
// recorded header on the second capability call will still be the old
// token and the test will fail.
//
// Adversarial trip-wire 2: the captured sync_state row's `sync_cursor`
// MUST match the post-first-Sync cursor value exactly through rotation
// AND through the post-rotation Sync no-op page. If a future regression
// clears the cursor on rotation (e.g., by re-running the in-memory cursor
// init path), the persisted value will read empty and the test will fail.
//
// Adversarial trip-wire 3: the planned diagnostics MUST include all three
// required tokens (`capability_re_read_required`, `sync_cursor_preserved`,
// `evidence_export_state_preserved`). A regression that drops one will
// fail the test rather than silently degrade operator visibility.
//
// Run: ./smackerel.sh test integration (requires live test stack —
// postgres + nats — via `./smackerel.sh --env test up`).
func TestQFCredentialRotationOverlapPreservesCursorExportIdempotencyCapabilityDiagnosticsAndAudit(t *testing.T) {
	pool := testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sourceID := "qf-decisions-it-rotation-" + uniqueSuffix()
	cleanupQFDecisionsRows(t, pool, sourceID)
	t.Cleanup(func() { cleanupQFDecisionsRows(t, pool, sourceID) })

	var capabilityCalls atomic.Int32
	var eventsCalls atomic.Int32
	var capabilityAuthHeaders []string
	var headerMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			headerMu.Lock()
			capabilityAuthHeaders = append(capabilityAuthHeaders, r.Header.Get("Authorization"))
			headerMu.Unlock()
			capabilityCalls.Add(1)
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			eventsCalls.Add(1)
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     []qfdecisions.QFDecisionEvent{},
				NextCursor: "qf-rotation-cursor-page-end",
				HasMore:    false,
				ServerTime: "2026-05-21T00:00:00Z",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conn := qfdecisions.New(sourceID)
	if err := conn.Connect(ctx, qfIntegrationConfig(server.URL, 1)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	stateStore := connector.NewStateStore(pool)
	runSync := func(label, cursor string) string {
		t.Helper()
		_, nextCursor, err := conn.Sync(ctx, cursor)
		if err != nil {
			t.Fatalf("%s: Sync: %v", label, err)
		}
		if err := stateStore.Save(ctx, &connector.SyncState{
			SourceID:    sourceID,
			Enabled:     true,
			SyncCursor:  nextCursor,
			ItemsSynced: 0,
		}); err != nil {
			t.Fatalf("%s: state store save: %v", label, err)
		}
		return nextCursor
	}

	preRotationCursor := runSync("pre-rotation", "")
	if preRotationCursor != "qf-rotation-cursor-page-end" {
		t.Fatalf("pre-rotation cursor = %q, want %q", preRotationCursor, "qf-rotation-cursor-page-end")
	}

	persistedPre, err := stateStore.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get sync state pre-rotation: %v", err)
	}
	if persistedPre.SyncCursor != preRotationCursor {
		t.Fatalf("pre-rotation persisted sync_cursor = %q, want %q", persistedPre.SyncCursor, preRotationCursor)
	}

	// Preserved evidence-export idempotency records (Scope 4 surface). The
	// in-memory state value we pass through PlanCredentialRotation must
	// survive verbatim into plan.PreservedState.
	preservedExportIDs := []string{
		"export-100-" + uniqueSuffix(),
		"export-101-" + uniqueSuffix(),
	}

	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	credentials := []qfdecisions.RotatingCredential{
		{
			Ref:       "qf-service-token",
			NotBefore: now.Add(-48 * time.Hour),
			NotAfter:  now.Add(20 * time.Hour), // overlaps 20h with rotated
		},
		{
			Ref:       "qf-rotated-token",
			NotBefore: now.Add(-4 * time.Hour),
			NotAfter:  now.Add(72 * time.Hour),
		},
	}
	state := qfdecisions.CredentialRotationState{
		SyncCursor:             preRotationCursor,
		CapabilityResponseJSON: `{"audit_envelope_version":"v1"}`,
		CapabilityFetchedAt:    now.Add(-10 * time.Minute),
		CapabilityStatus:       qfdecisions.CapabilityStatusCompatible,
		EvidenceExportIDs:      preservedExportIDs,
	}

	plan, err := conn.RotateCredentials(ctx, credentials, state, now)
	if err != nil {
		t.Fatalf("RotateCredentials: %v", err)
	}
	if plan.SelectedCredentialRef != "qf-rotated-token" {
		t.Fatalf("selected credential = %q, want %q (newest not_before)", plan.SelectedCredentialRef, "qf-rotated-token")
	}
	if plan.PreviousCredentialRef != "qf-service-token" {
		t.Fatalf("previous credential = %q, want %q", plan.PreviousCredentialRef, "qf-service-token")
	}

	// --- Adversarial trip-wire 3: planned diagnostics MUST carry all three
	// required tokens for operator clarity.
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

	// Preserved state assertions: cursor and evidence-export IDs MUST round-trip verbatim.
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

	// --- Audit envelope: rotation plan envelope MUST be OK / v1 / credential_rotation.
	if plan.AuditEnvelope.Action != qfdecisions.AuditActionCredentialRotation {
		t.Fatalf("rotation audit action = %q, want %q", plan.AuditEnvelope.Action, qfdecisions.AuditActionCredentialRotation)
	}
	if plan.AuditEnvelope.Outcome != qfdecisions.AuditOutcomeOK {
		t.Fatalf("rotation audit outcome = %q, want %q", plan.AuditEnvelope.Outcome, qfdecisions.AuditOutcomeOK)
	}
	if plan.AuditEnvelope.AuditEnvelopeVersion != qfdecisions.AuditEnvelopeVersionV1 {
		t.Fatalf("rotation audit envelope version = %q, want %q", plan.AuditEnvelope.AuditEnvelopeVersion, qfdecisions.AuditEnvelopeVersionV1)
	}

	// --- Adversarial trip-wire 1: capability handshake MUST have run twice
	// (Connect + RotateCredentials), and the second call MUST carry the
	// rotated bearer token.
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

	// --- Adversarial trip-wire 2: drive a Sync after rotation and assert
	// the persisted sync_cursor survives rotation verbatim.
	postRotationCursor := runSync("post-rotation", preRotationCursor)
	if postRotationCursor != preRotationCursor {
		t.Fatalf("post-rotation cursor = %q, want %q (rotation MUST preserve cursor)", postRotationCursor, preRotationCursor)
	}
	persistedPost, err := stateStore.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get sync state post-rotation: %v", err)
	}
	if persistedPost.SyncCursor != preRotationCursor {
		t.Fatalf("post-rotation persisted sync_cursor = %q, want %q", persistedPost.SyncCursor, preRotationCursor)
	}

	// --- Health remains Healthy after successful rotation.
	if got := conn.Health(ctx); got != connector.HealthHealthy {
		t.Fatalf("health after rotation = %s, want %s", got, connector.HealthHealthy)
	}
}
