//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

func TestQFPersonalEvidenceExportPersistsPacketContextAndCapabilityPreflightState(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := uniqueSuffix()
	cleanupQFEvidenceExports(t, pool, suffix)
	t.Cleanup(func() { cleanupQFEvidenceExports(t, pool, suffix) })

	var qfPostAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != qfdecisions.PersonalEvidenceBundlesPath {
			t.Fatalf("QF request = %s %s, want POST %s", r.Method, r.URL.Path, qfdecisions.PersonalEvidenceBundlesPath)
		}
		qfPostAttempts.Add(1)
		var bundle qfdecisions.PersonalEvidenceBundle
		if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
			t.Fatalf("decode QF evidence bundle: %v", err)
		}
		payloadHash, err := qfdecisions.EvidenceBundlePayloadHash(bundle)
		if err != nil {
			t.Fatalf("hash QF evidence bundle: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{ExportID: bundle.ExportID, BundleID: bundle.BundleID, PayloadHash: payloadHash})
	}))
	defer server.Close()

	store := qfdecisions.NewEvidenceExportStore(pool)
	exporter := qfdecisions.NewEvidenceExporter(qfdecisions.NewClient(server.URL, "qf-service-token", 1, 25), store, qfdecisions.NewEvidenceRateLimiter(time.Now), "qf-service-token", time.Now)
	acceptedBundle := integrationEvidenceBundle(t, suffix, "accepted", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "smackerel_news"}})
	record, _, err := exporter.Export(ctx, acceptedBundle, integrationEvidenceCapability())
	if err != nil {
		t.Fatalf("export accepted bundle: %v", err)
	}
	if record.Status != qfdecisions.EvidenceExportStatusAccepted {
		t.Fatalf("accepted record status = %s, want accepted", record.Status)
	}
	if record.TargetContextType != qfdecisions.TargetContextPacketContext || record.PacketID != "packet-"+suffix || record.TraceID != "trace-"+suffix {
		t.Fatalf("target context persisted incorrectly: %+v", record)
	}
	if len(record.SourceProvenanceClasses) != 1 || record.SourceProvenanceClasses[0].SourceProvenanceClass != "smackerel_news" {
		t.Fatalf("source provenance classes = %+v, want one smackerel_news entry", record.SourceProvenanceClasses)
	}

	rejectedBundle := integrationEvidenceBundle(t, suffix, "rejected", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "private_diary"}})
	rejectedRecord, _, err := exporter.Export(ctx, rejectedBundle, integrationEvidenceCapability())
	if err == nil {
		t.Fatal("expected local reject for ineligible source class")
	}
	wantReason := qfdecisions.EvidenceSourceClassNotEligibleReason("private_diary")
	if rejectedRecord.Status != qfdecisions.EvidenceExportStatusLocalReject || rejectedRecord.Reason != wantReason {
		t.Fatalf("local reject record = %+v, want status=local_reject reason=%s", rejectedRecord, wantReason)
	}
	if qfPostAttempts.Load() != 1 {
		t.Fatalf("QF post attempts = %d, want only the accepted export to cross local preflight", qfPostAttempts.Load())
	}
}

func TestQFPersonalEvidenceExportIdempotencyCollisionAndRevocationState(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := uniqueSuffix()
	cleanupQFEvidenceExports(t, pool, suffix)
	t.Cleanup(func() { cleanupQFEvidenceExports(t, pool, suffix) })

	timestamps := []time.Time{
		time.Date(2026, 5, 19, 12, 10, 0, 0, time.UTC),
		time.Date(2026, 5, 19, 12, 11, 0, 0, time.UTC),
		time.Date(2026, 5, 19, 12, 12, 0, 0, time.UTC),
		time.Date(2026, 5, 19, 12, 13, 0, 0, time.UTC),
	}
	var timeIndex atomic.Int32
	now := func() time.Time {
		idx := int(timeIndex.Add(1)) - 1
		if idx >= len(timestamps) {
			return timestamps[len(timestamps)-1]
		}
		return timestamps[idx]
	}

	var mu sync.Mutex
	postAttemptsByExportID := make(map[string]int)
	deleteReasons := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			var bundle qfdecisions.PersonalEvidenceBundle
			if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
				t.Fatalf("decode QF bundle: %v", err)
			}
			payloadHash, err := qfdecisions.EvidenceBundlePayloadHash(bundle)
			if err != nil {
				t.Fatalf("hash QF bundle: %v", err)
			}
			mu.Lock()
			postAttemptsByExportID[bundle.ExportID]++
			attempt := postAttemptsByExportID[bundle.ExportID]
			mu.Unlock()
			if bundle.ExportID == "export-collision-"+suffix {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(qfdecisions.BridgeErrorResponse{Code: qfdecisions.EvidenceBridgeExportIDReuseWithDifferentPayload, Message: "same export_id, different payload"})
				return
			}
			if attempt == 1 {
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{ExportID: bundle.ExportID, BundleID: bundle.BundleID, PayloadHash: payloadHash})
		case http.MethodDelete:
			var req qfdecisions.EvidenceRevocationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode QF revocation: %v", err)
			}
			exportID := r.URL.Path[len(qfdecisions.PersonalEvidenceBundlesPath)+1:]
			mu.Lock()
			deleteReasons[exportID] = req.Reason
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected QF method %s", r.Method)
		}
	}))
	defer server.Close()

	store := qfdecisions.NewEvidenceExportStore(pool)
	exporter := qfdecisions.NewEvidenceExporter(qfdecisions.NewClient(server.URL, "qf-service-token", 1, 25), store, qfdecisions.NewEvidenceRateLimiter(time.Now), "qf-service-token", now)
	acceptedBundle := integrationEvidenceBundle(t, suffix, "stable", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "smackerel_news"}})
	firstRecord, firstResponse, err := exporter.Export(ctx, acceptedBundle, integrationEvidenceCapability())
	if err != nil {
		t.Fatalf("first export: %v", err)
	}
	if firstRecord.Status != qfdecisions.EvidenceExportStatusAccepted || firstRecord.TargetContextType != qfdecisions.TargetContextPacketContext {
		t.Fatalf("first record = %+v, want accepted packet_context", firstRecord)
	}
	if firstResponse.IdempotentReplay {
		t.Fatalf("first export response unexpectedly marked replay: %+v", firstResponse)
	}
	secondRecord, secondResponse, err := exporter.Export(ctx, acceptedBundle, integrationEvidenceCapability())
	if err != nil {
		t.Fatalf("idempotent replay export: %v", err)
	}
	if !secondResponse.IdempotentReplay {
		t.Fatal("idempotent replay response did not mark replay")
	}
	if secondRecord.AuditEnvelope.RecordedAt != firstRecord.AuditEnvelope.RecordedAt {
		t.Fatalf("idempotent replay changed audit envelope recorded_at from %s to %s", firstRecord.AuditEnvelope.RecordedAt, secondRecord.AuditEnvelope.RecordedAt)
	}
	if secondRecord.LastObservedAt == nil || !secondRecord.LastObservedAt.Equal(timestamps[1]) {
		t.Fatalf("last_observed_at = %v, want %s after replay", secondRecord.LastObservedAt, timestamps[1])
	}

	collisionBundle := integrationEvidenceBundle(t, suffix, "collision", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "smackerel_news"}})
	collisionRecord, _, err := exporter.Export(ctx, collisionBundle, integrationEvidenceCapability())
	if err == nil {
		t.Fatal("expected export_id collision")
	}
	if collisionRecord.Status != qfdecisions.EvidenceExportStatusExportIDCollision || collisionRecord.Reason != "EXPORT_ID_COLLISION" {
		t.Fatalf("collision record = %+v, want export_id_collision/EXPORT_ID_COLLISION", collisionRecord)
	}
	mu.Lock()
	collisionAttempts := postAttemptsByExportID[collisionBundle.ExportID]
	mu.Unlock()
	if collisionAttempts != 1 {
		t.Fatalf("collision attempts = %d, want one terminal 409 attempt", collisionAttempts)
	}

	revokedRecord, _, err := exporter.Revoke(ctx, acceptedBundle.ExportID, qfdecisions.EvidenceRevokeReasonConsentRevoked)
	if err != nil {
		t.Fatalf("revoke evidence export: %v", err)
	}
	if revokedRecord.Status != qfdecisions.EvidenceExportStatusRevoked || revokedRecord.Reason != qfdecisions.EvidenceRevokeReasonConsentRevoked || revokedRecord.RevokedAt == nil {
		t.Fatalf("revoked record = %+v, want revoked consent_revoked with timestamp", revokedRecord)
	}
	if revokedRecord.AuditEnvelope.Action != qfdecisions.AuditActionEvidenceRevocation || revokedRecord.AuditEnvelope.Outcome != qfdecisions.AuditOutcomeOK || revokedRecord.AuditEnvelope.Reason != qfdecisions.EvidenceRevokeReasonConsentRevoked || revokedRecord.AuditEnvelope.AuditEnvelopeVersion != qfdecisions.AuditEnvelopeVersionV1 {
		t.Fatalf("revocation audit envelope = %+v, want %s ok consent_revoked v1", revokedRecord.AuditEnvelope, qfdecisions.AuditActionEvidenceRevocation)
	}
	mu.Lock()
	deleteReason := deleteReasons[acceptedBundle.ExportID]
	mu.Unlock()
	if deleteReason != qfdecisions.EvidenceRevokeReasonConsentRevoked {
		t.Fatalf("QF delete reason = %q, want consent_revoked", deleteReason)
	}
}

func TestQFPersonalEvidenceRevocationRecordsRemoteMissingAuditState(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := uniqueSuffix()
	cleanupQFEvidenceExports(t, pool, suffix)
	t.Cleanup(func() { cleanupQFEvidenceExports(t, pool, suffix) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			var bundle qfdecisions.PersonalEvidenceBundle
			if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
				t.Fatalf("decode QF bundle: %v", err)
			}
			payloadHash, err := qfdecisions.EvidenceBundlePayloadHash(bundle)
			if err != nil {
				t.Fatalf("hash QF bundle: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{ExportID: bundle.ExportID, BundleID: bundle.BundleID, PayloadHash: payloadHash})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(qfdecisions.BridgeErrorResponse{Code: qfdecisions.EvidenceBridgeExportIDNotFound, Message: "already missing"})
		default:
			t.Fatalf("unexpected QF method %s", r.Method)
		}
	}))
	defer server.Close()

	store := qfdecisions.NewEvidenceExportStore(pool)
	exporter := qfdecisions.NewEvidenceExporter(qfdecisions.NewClient(server.URL, "qf-service-token", 1, 25), store, qfdecisions.NewEvidenceRateLimiter(time.Now), "qf-service-token", time.Now)
	bundle := integrationEvidenceBundle(t, suffix, "remote-missing", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "smackerel_news"}})
	if _, _, err := exporter.Export(ctx, bundle, integrationEvidenceCapability()); err != nil {
		t.Fatalf("first export: %v", err)
	}
	revokedRecord, response, err := exporter.Revoke(ctx, bundle.ExportID, qfdecisions.EvidenceRevokeReasonConsentRevoked)
	if err != nil {
		t.Fatalf("remote-missing revoke should settle local state: %v", err)
	}
	if !response.RemoteMissing || revokedRecord.Status != qfdecisions.EvidenceExportStatusRevokedRemoteMissing || revokedRecord.Reason != "remote_missing" {
		t.Fatalf("remote-missing revocation response=%+v record=%+v", response, revokedRecord)
	}
	if revokedRecord.AuditEnvelope.Action != qfdecisions.AuditActionEvidenceRevocation || revokedRecord.AuditEnvelope.Outcome != qfdecisions.AuditOutcomeOK || revokedRecord.AuditEnvelope.Reason != "remote_missing" || revokedRecord.AuditEnvelope.AuditEnvelopeVersion != qfdecisions.AuditEnvelopeVersionV1 {
		t.Fatalf("remote-missing audit envelope = %+v, want %s ok remote_missing v1", revokedRecord.AuditEnvelope, qfdecisions.AuditActionEvidenceRevocation)
	}
}

func integrationEvidenceBundle(t *testing.T, suffix, variant string, sourceClasses []qfdecisions.SourceProvenanceClass) qfdecisions.PersonalEvidenceBundle {
	t.Helper()
	bundle, err := qfdecisions.BuildPacketContextEvidenceBundle(qfdecisions.EvidenceBundleInput{
		BundleID:                fmt.Sprintf("bundle-%s-%s", variant, suffix),
		ExportID:                fmt.Sprintf("export-%s-%s", variant, suffix),
		CreatedAt:               time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		ConsentScope:            "qf_packet_context",
		SensitivityTier:         "personal",
		PacketID:                "packet-" + suffix,
		TraceID:                 "trace-" + suffix,
		SourceArtifactIDs:       []string{"artifact-" + suffix},
		SourceRefs:              []string{"https://example.test/source/artifact-" + suffix},
		SourceProvenanceClasses: sourceClasses,
		ExtractedClaims:         []string{"Claim for " + variant + " " + suffix},
		Confidence:              0.91,
		Provenance:              map[string]any{"generator": "integration-qf-personal-evidence"},
		RedactionSummary:        map[string]any{"omitted_raw_messages": 1},
		RelatedSymbols:          []string{"MSFT"},
		RelatedEntities:         []string{"Federal Reserve"},
	})
	if err != nil {
		t.Fatalf("BuildPacketContextEvidenceBundle: %v", err)
	}
	return bundle
}

func integrationEvidenceCapability() qfdecisions.QFBridgeCapability {
	capability := validQFIntegrationCapability()
	capability.SupportedTargetContextTypes = []string{qfdecisions.TargetContextPacketContext}
	capability.EvidenceMaxBundleSizeBytes = 524288
	capability.EvidenceMaxClaimsPerBundle = 50
	capability.EvidenceRateLimitPerMinute = 10
	capability.EligibleSmackerelSourceClasses = []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"}
	return capability
}

func cleanupQFEvidenceExports(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, suffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DELETE FROM qf_personal_evidence_exports WHERE export_id LIKE $1`, "%"+suffix); err != nil {
		t.Logf("cleanup qf_personal_evidence_exports for %s: %v", suffix, err)
	}
}
