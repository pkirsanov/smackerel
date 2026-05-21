//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

func TestQFPersonalEvidenceBundleAPIPacketContextRoundTrip(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	t.Cleanup(pool.Close)

	suffix := fmt.Sprintf("api-%d", time.Now().UnixNano())
	packetArtifactID := "qf-artifact-" + suffix
	cleanupQFEvidenceAPIState(t, pool, suffix, packetArtifactID)
	t.Cleanup(func() { cleanupQFEvidenceAPIState(t, pool, suffix, packetArtifactID) })
	insertQFEvidencePacketArtifact(t, pool, packetArtifactID, suffix)
	capabilityJSON, err := json.Marshal(e2eEvidenceCapability())
	if err != nil {
		t.Fatalf("marshal capability: %v", err)
	}
	stateStore := connector.NewStateStore(pool)
	if err := stateStore.SaveCapability(ctx, qfdecisions.DefaultConnectorID, string(capabilityJSON), time.Now().UTC(), qfdecisions.CapabilityStatusCompatible); err != nil {
		t.Fatalf("save persisted QF capability: %v", err)
	}

	var mu sync.Mutex
	postedBundles := make([]qfdecisions.PersonalEvidenceBundle, 0)
	deleteReasons := make(map[string]string)
	stopStub := startQFEvidenceBundleAPIStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			var bundle qfdecisions.PersonalEvidenceBundle
			if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
				t.Fatalf("decode API evidence bundle POST: %v", err)
			}
			payloadHash, err := qfdecisions.EvidenceBundlePayloadHash(bundle)
			if err != nil {
				t.Fatalf("hash API evidence bundle: %v", err)
			}
			mu.Lock()
			postedBundles = append(postedBundles, bundle)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{ExportID: bundle.ExportID, BundleID: bundle.BundleID, PayloadHash: payloadHash})
		case http.MethodDelete:
			exportID := strings.TrimPrefix(r.URL.Path, qfdecisions.PersonalEvidenceBundlesPath+"/")
			var req qfdecisions.EvidenceRevocationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode API evidence revoke DELETE: %v", err)
			}
			mu.Lock()
			deleteReasons[exportID] = req.Reason
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected QF API method %s", r.Method)
		}
	})
	defer stopStub()

	exportPayload := map[string]any{
		"packet_artifact_id":        packetArtifactID,
		"source_artifact_ids":       []string{"source-artifact-" + suffix},
		"source_refs":               []string{"https://example.test/source/" + suffix},
		"source_provenance_classes": []map[string]string{{"source_artifact_id": "source-artifact-" + suffix, "source_provenance_class": "smackerel_news"}},
		"extracted_claims":          []string{"API claim for " + suffix},
		"confidence":                0.92,
		"consent_scope":             "qf_packet_context",
		"sensitivity_tier":          "personal",
		"provenance":                map[string]any{"surface": "e2e_api"},
		"redaction_summary":         map[string]any{"raw_personal_content": "omitted"},
		"related_symbols":           []string{"MSFT"},
		"related_entities":          []string{"Federal Reserve"},
	}
	postResp, err := apiPostJSON(cfg, "/api/qf/evidence-bundles/", exportPayload)
	if err != nil {
		t.Fatalf("POST evidence bundle API: %v", err)
	}
	postBody, err := readBody(postResp)
	if err != nil {
		t.Fatalf("read export response: %v", err)
	}
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST evidence bundle status=%d body=%s", postResp.StatusCode, postBody)
	}
	var exportResponse struct {
		Record struct {
			ExportID          string `json:"export_id"`
			Status            string `json:"status"`
			TargetContextType string `json:"target_context_type"`
			PacketID          string `json:"packet_id"`
			AuditEnvelope     struct {
				Action               string `json:"action"`
				Outcome              string `json:"outcome"`
				AuditEnvelopeVersion string `json:"audit_envelope_version"`
			} `json:"audit_envelope"`
		} `json:"record"`
	}
	if err := json.Unmarshal(postBody, &exportResponse); err != nil {
		t.Fatalf("decode export response: %v body=%s", err, postBody)
	}
	if exportResponse.Record.Status != qfdecisions.EvidenceExportStatusAccepted || exportResponse.Record.TargetContextType != qfdecisions.TargetContextPacketContext || exportResponse.Record.PacketID != "packet-"+suffix {
		t.Fatalf("export response record = %+v", exportResponse.Record)
	}
	if exportResponse.Record.AuditEnvelope.Action != qfdecisions.AuditActionEvidenceExportAttempt || exportResponse.Record.AuditEnvelope.Outcome != qfdecisions.AuditOutcomeOK || exportResponse.Record.AuditEnvelope.AuditEnvelopeVersion != qfdecisions.AuditEnvelopeVersionV1 {
		t.Fatalf("export audit envelope = %+v, want %s ok v1", exportResponse.Record.AuditEnvelope, qfdecisions.AuditActionEvidenceExportAttempt)
	}
	mu.Lock()
	postedCount := len(postedBundles)
	postedTargetContext := ""
	if postedCount == 1 {
		postedTargetContext, _ = postedBundles[0].TargetContext[qfdecisions.TargetContextTypeKey].(string)
	}
	mu.Unlock()
	if postedCount != 1 || postedTargetContext != qfdecisions.TargetContextPacketContext {
		t.Fatalf("QF posted bundles count=%d target_context=%q", postedCount, postedTargetContext)
	}

	statusResp, err := apiGet(cfg, "/api/qf/evidence-bundles/"+exportResponse.Record.ExportID)
	if err != nil {
		t.Fatalf("GET evidence bundle status API: %v", err)
	}
	statusBody, err := readBody(statusResp)
	if err != nil {
		t.Fatalf("read status response: %v", err)
	}
	if statusResp.StatusCode != http.StatusOK || !strings.Contains(string(statusBody), qfdecisions.EvidenceExportStatusAccepted) {
		t.Fatalf("GET evidence bundle status=%d body=%s", statusResp.StatusCode, statusBody)
	}

	revokeResp, err := apiDeleteJSON(cfg, "/api/qf/evidence-bundles/"+exportResponse.Record.ExportID, map[string]any{"reason": qfdecisions.EvidenceRevokeReasonConsentRevoked})
	if err != nil {
		t.Fatalf("DELETE evidence bundle API: %v", err)
	}
	revokeBody, err := readBody(revokeResp)
	if err != nil {
		t.Fatalf("read revoke response: %v", err)
	}
	if revokeResp.StatusCode != http.StatusOK || !strings.Contains(string(revokeBody), qfdecisions.AuditActionEvidenceRevocation) {
		t.Fatalf("DELETE evidence bundle status=%d body=%s", revokeResp.StatusCode, revokeBody)
	}
	mu.Lock()
	deleteReason := deleteReasons[exportResponse.Record.ExportID]
	mu.Unlock()
	if deleteReason != qfdecisions.EvidenceRevokeReasonConsentRevoked {
		t.Fatalf("QF DELETE reason = %q, want consent_revoked", deleteReason)
	}
}

func TestQFPersonalEvidenceBundleAPIRejectsMissingAndUnreadablePersistedCapability(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	t.Cleanup(pool.Close)

	suffix := fmt.Sprintf("capability-%d", time.Now().UnixNano())
	packetArtifactID := "qf-artifact-" + suffix
	cleanupQFEvidenceAPIState(t, pool, suffix, packetArtifactID)
	t.Cleanup(func() { cleanupQFEvidenceAPIState(t, pool, suffix, packetArtifactID) })
	insertQFEvidencePacketArtifact(t, pool, packetArtifactID, suffix)
	stateStore := connector.NewStateStore(pool)
	capabilityJSON, fetchedAt, status, err := stateStore.GetCapability(ctx, qfdecisions.DefaultConnectorID)
	if err == nil {
		t.Cleanup(func() {
			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer restoreCancel()
			_ = stateStore.SaveCapability(restoreCtx, qfdecisions.DefaultConnectorID, capabilityJSON, fetchedAt, status)
		})
	} else {
		t.Cleanup(func() {
			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer restoreCancel()
			_, _ = pool.Exec(restoreCtx, `DELETE FROM sync_state WHERE source_id = $1`, qfdecisions.DefaultConnectorID)
		})
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sync_state WHERE source_id = $1`, qfdecisions.DefaultConnectorID); err != nil {
		t.Fatalf("delete persisted QF capability: %v", err)
	}
	missingResp := postQFEvidenceAPITestPayload(t, cfg, packetArtifactID, suffix)
	missingBody, err := readBody(missingResp)
	if err != nil {
		t.Fatalf("read missing capability response: %v", err)
	}
	if missingResp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(missingBody), qfdecisions.EvidenceRejectCapabilityUnavailable) {
		t.Fatalf("missing capability status=%d body=%s", missingResp.StatusCode, missingBody)
	}
	if err := stateStore.SaveCapability(ctx, qfdecisions.DefaultConnectorID, "[]", time.Now().UTC(), qfdecisions.CapabilityStatusCompatible); err != nil {
		t.Fatalf("save unreadable persisted QF capability: %v", err)
	}
	unreadableResp := postQFEvidenceAPITestPayload(t, cfg, packetArtifactID, suffix)
	unreadableBody, err := readBody(unreadableResp)
	if err != nil {
		t.Fatalf("read unreadable capability response: %v", err)
	}
	if unreadableResp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(unreadableBody), qfdecisions.EvidenceRejectCapabilityUnavailable) || !strings.Contains(string(unreadableBody), "unreadable") {
		t.Fatalf("unreadable capability status=%d body=%s", unreadableResp.StatusCode, unreadableBody)
	}
}

func TestQFPersonalEvidenceBundleE2EPacketContextRejectsCollisionAndRevokes(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}

	suffix := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	cleanupQFEvidenceExportsE2E(t, pool, suffix)
	t.Cleanup(func() {
		cleanupQFEvidenceExportsE2E(t, pool, suffix)
		pool.Close()
	})

	var mu sync.Mutex
	postAttemptsByExportID := make(map[string]int)
	deleteReasons := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			var bundle qfdecisions.PersonalEvidenceBundle
			if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
				t.Fatalf("decode evidence bundle POST: %v", err)
			}
			payloadHash, err := qfdecisions.EvidenceBundlePayloadHash(bundle)
			if err != nil {
				t.Fatalf("hash evidence bundle: %v", err)
			}
			mu.Lock()
			postAttemptsByExportID[bundle.ExportID]++
			attempt := postAttemptsByExportID[bundle.ExportID]
			mu.Unlock()
			if bundle.ExportID == "export-collision-"+suffix {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(qfdecisions.BridgeErrorResponse{Code: qfdecisions.EvidenceBridgeExportIDReuseWithDifferentPayload, Message: "different payload for same export_id"})
				return
			}
			if attempt == 1 {
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.EvidenceExportResponse{ExportID: bundle.ExportID, BundleID: bundle.BundleID, PayloadHash: payloadHash})
		case http.MethodDelete:
			exportID := r.URL.Path[len(qfdecisions.PersonalEvidenceBundlesPath)+1:]
			var req qfdecisions.EvidenceRevocationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode revocation DELETE: %v", err)
			}
			mu.Lock()
			deleteReasons[exportID] = req.Reason
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected QF request method %s", r.Method)
		}
	}))
	defer server.Close()

	store := qfdecisions.NewEvidenceExportStore(pool)
	exporter := qfdecisions.NewEvidenceExporter(qfdecisions.NewClient(server.URL, "qf-service-token", 1, 25), store, qfdecisions.NewEvidenceRateLimiter(time.Now), "qf-service-token", time.Now)
	acceptedBundle := e2eEvidenceBundle(t, suffix, "stable", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "smackerel_news"}})
	acceptedRecord, response, err := exporter.Export(ctx, acceptedBundle, e2eEvidenceCapability())
	if err != nil {
		t.Fatalf("accepted packet-context export failed: %v", err)
	}
	if acceptedRecord.Status != qfdecisions.EvidenceExportStatusAccepted || acceptedRecord.TargetContextType != qfdecisions.TargetContextPacketContext {
		t.Fatalf("accepted record = %+v, want accepted packet_context", acceptedRecord)
	}
	if response.IdempotentReplay {
		t.Fatalf("first export response unexpectedly marked replay: %+v", response)
	}

	_, replayResponse, err := exporter.Export(ctx, acceptedBundle, e2eEvidenceCapability())
	if err != nil {
		t.Fatalf("idempotent replay export failed: %v", err)
	}
	if !replayResponse.IdempotentReplay {
		t.Fatalf("replay response = %+v, want IdempotentReplay", replayResponse)
	}

	rejectedBundle := e2eEvidenceBundle(t, suffix, "source-class", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "private_diary"}})
	rejectedRecord, _, err := exporter.Export(ctx, rejectedBundle, e2eEvidenceCapability())
	if err == nil {
		t.Fatal("expected source-class local rejection")
	}
	wantReason := qfdecisions.EvidenceSourceClassNotEligibleReason("private_diary")
	if rejectedRecord.Status != qfdecisions.EvidenceExportStatusLocalReject || rejectedRecord.Reason != wantReason {
		t.Fatalf("source-class reject record = %+v, want local_reject/%s", rejectedRecord, wantReason)
	}
	mu.Lock()
	rejectedAttempts := postAttemptsByExportID[rejectedBundle.ExportID]
	mu.Unlock()
	if rejectedAttempts != 0 {
		t.Fatalf("source-class local reject crossed QF boundary %d times", rejectedAttempts)
	}

	collisionBundle := e2eEvidenceBundle(t, suffix, "collision", []qfdecisions.SourceProvenanceClass{{SourceArtifactID: "artifact-" + suffix, SourceProvenanceClass: "smackerel_news"}})
	collisionRecord, _, err := exporter.Export(ctx, collisionBundle, e2eEvidenceCapability())
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
		t.Fatalf("collision attempts = %d, want exactly one non-retryable 409", collisionAttempts)
	}

	revokedRecord, _, err := exporter.Revoke(ctx, acceptedBundle.ExportID, qfdecisions.EvidenceRevokeReasonConsentRevoked)
	if err != nil {
		t.Fatalf("revocation failed: %v", err)
	}
	if revokedRecord.Status != qfdecisions.EvidenceExportStatusRevoked || revokedRecord.Reason != qfdecisions.EvidenceRevokeReasonConsentRevoked {
		t.Fatalf("revoked record = %+v, want revoked/consent_revoked", revokedRecord)
	}
	mu.Lock()
	deleteReason := deleteReasons[acceptedBundle.ExportID]
	mu.Unlock()
	if deleteReason != qfdecisions.EvidenceRevokeReasonConsentRevoked {
		t.Fatalf("QF DELETE reason = %q, want consent_revoked", deleteReason)
	}
}

func e2eEvidenceBundle(t *testing.T, suffix, variant string, sourceClasses []qfdecisions.SourceProvenanceClass) qfdecisions.PersonalEvidenceBundle {
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
		ExtractedClaims:         []string{"E2E claim for " + variant + " " + suffix},
		Confidence:              0.89,
		Provenance:              map[string]any{"generator": "e2e-qf-personal-evidence"},
		RedactionSummary:        map[string]any{"omitted_raw_messages": 1},
		RelatedSymbols:          []string{"MSFT"},
		RelatedEntities:         []string{"Federal Reserve"},
	})
	if err != nil {
		t.Fatalf("BuildPacketContextEvidenceBundle: %v", err)
	}
	return bundle
}

func e2eEvidenceCapability() qfdecisions.QFBridgeCapability {
	return qfdecisions.QFBridgeCapability{
		SupportedTargetContextTypes:    []string{qfdecisions.TargetContextPacketContext},
		EvidenceMaxBundleSizeBytes:     524288,
		EvidenceMaxClaimsPerBundle:     50,
		EvidenceRateLimitPerMinute:     10,
		EligibleSmackerelSourceClasses: []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"},
	}
}

func cleanupQFEvidenceExportsE2E(t *testing.T, pool *pgxpool.Pool, suffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DELETE FROM qf_personal_evidence_exports WHERE export_id LIKE $1`, "%"+suffix); err != nil {
		t.Logf("cleanup qf_personal_evidence_exports for %s: %v", suffix, err)
	}
}

func apiDeleteJSON(cfg e2eConfig, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodDelete, cfg.CoreURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}

func startQFEvidenceBundleAPIStub(t *testing.T, handler http.HandlerFunc) func() {
	t.Helper()
	baseURL := os.Getenv("QF_DECISIONS_BASE_URL")
	if baseURL == "" {
		t.Fatal("e2e: QF_DECISIONS_BASE_URL is required for live QF evidence stub")
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
		t.Fatalf("start live QF evidence stub on configured port %s: %v", port, err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && !strings.Contains(err.Error(), "Server closed") {
			t.Errorf("QF evidence API stub failed: %v", err)
		}
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
}

func insertQFEvidencePacketArtifact(t *testing.T, pool *pgxpool.Pool, artifactID, suffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	metadata, err := json.Marshal(map[string]any{
		"packet_id": "packet-" + suffix,
		"trace_id":  "trace-" + suffix,
	})
	if err != nil {
		t.Fatalf("marshal qf artifact metadata: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, metadata, processing_status, created_at, updated_at)
		VALUES ($1, 'qf/decision-packet', $2, $3, $4, $5, $6, $7::jsonb, 'processed', NOW(), NOW())
	`, artifactID, "QF packet "+suffix, "QF evidence API packet", "QF packet body "+suffix, "hash-"+artifactID, qfdecisions.DefaultConnectorID, string(metadata)); err != nil {
		t.Fatalf("insert QF packet artifact: %v", err)
	}
}

func cleanupQFEvidenceAPIState(t *testing.T, pool *pgxpool.Pool, suffix, artifactID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DELETE FROM qf_personal_evidence_exports WHERE export_id LIKE $1 OR bundle_id LIKE $1`, "%"+suffix); err != nil {
		t.Logf("cleanup qf evidence exports for %s: %v", suffix, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE id = $1 OR content_hash = $2`, artifactID, "hash-"+artifactID); err != nil {
		t.Logf("cleanup qf evidence artifact %s: %v", artifactID, err)
	}
}

func postQFEvidenceAPITestPayload(t *testing.T, cfg e2eConfig, packetArtifactID, suffix string) *http.Response {
	t.Helper()
	resp, err := apiPostJSON(cfg, "/api/qf/evidence-bundles/", map[string]any{
		"packet_artifact_id":        packetArtifactID,
		"source_artifact_ids":       []string{"source-artifact-" + suffix},
		"source_refs":               []string{"https://example.test/source/" + suffix},
		"source_provenance_classes": []map[string]string{{"source_artifact_id": "source-artifact-" + suffix, "source_provenance_class": "smackerel_news"}},
		"extracted_claims":          []string{"Capability preflight claim for " + suffix},
		"confidence":                0.91,
		"consent_scope":             "qf_packet_context",
		"sensitivity_tier":          "personal",
		"provenance":                map[string]any{"surface": "e2e_api"},
		"redaction_summary":         map[string]any{"raw_personal_content": "omitted"},
		"related_symbols":           []string{"MSFT"},
		"related_entities":          []string{"Federal Reserve"},
	})
	if err != nil {
		t.Fatalf("POST evidence bundle API: %v", err)
	}
	return resp
}
