package qfdecisions

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

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/metrics"
)

func TestEvidenceBundleBuildsPacketContextTargetWithRequiredFields(t *testing.T) {
	bundle := mustTestEvidenceBundle(t, "export-build-001", []string{"claim one"}, []SourceProvenanceClass{
		{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"},
	})

	if got := bundle.TargetContext[TargetContextTypeKey]; got != TargetContextPacketContext {
		t.Fatalf("target_context_type = %v, want %s", got, TargetContextPacketContext)
	}
	if got := bundle.TargetContext[TargetContextRefKey]; got != "packet-build-001" {
		t.Fatalf("target_context_ref = %v, want packet-build-001", got)
	}
	if got := bundle.TargetContext[TargetContextTraceIDKey]; got != "trace-build-001" {
		t.Fatalf("trace_id = %v, want trace-build-001", got)
	}
	if bundle.Confidence <= 0 {
		t.Fatalf("confidence = %v, want positive bundle confidence", bundle.Confidence)
	}
	if len(bundle.SourceProvenanceClasses) != 1 || bundle.SourceProvenanceClasses[0].SourceProvenanceClass != "smackerel_news" {
		t.Fatalf("source provenance classes = %+v, want smackerel_news", bundle.SourceProvenanceClasses)
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	if !strings.Contains(string(encoded), `"source_provenance_classes"`) {
		t.Fatalf("bundle JSON missing source_provenance_classes: %s", encoded)
	}
	if strings.Contains(string(encoded), "data_provenance_badge") {
		t.Fatalf("pre-MVP evidence bundle must not attach DataProvenanceBadge-shaped metadata: %s", encoded)
	}
}

func TestEvidenceBundlePreflightRejectsBundleSizeClaimCountAndRateLimit(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	t.Run("bundle too large", func(t *testing.T) {
		bundle := mustTestEvidenceBundle(t, "export-large-001", []string{strings.Repeat("x", 128)}, []SourceProvenanceClass{
			{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"},
		})
		capability := validEvidenceCapability()
		capability.EvidenceMaxBundleSizeBytes = 10
		err := ValidateEvidenceBundleForExport(bundle, capability, NewEvidenceRateLimiter(func() time.Time { return now }), "credential-large")
		assertPreflightReason(t, err, EvidenceRejectBundleTooLarge)
	})

	t.Run("too many claims", func(t *testing.T) {
		bundle := mustTestEvidenceBundle(t, "export-claims-001", []string{"one", "two"}, []SourceProvenanceClass{
			{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"},
		})
		capability := validEvidenceCapability()
		capability.EvidenceMaxClaimsPerBundle = 1
		err := ValidateEvidenceBundleForExport(bundle, capability, NewEvidenceRateLimiter(func() time.Time { return now }), "credential-claims")
		assertPreflightReason(t, err, EvidenceRejectTooManyClaims)
	})

	t.Run("rate limit exceeded", func(t *testing.T) {
		bundle := mustTestEvidenceBundle(t, "export-rate-001", []string{"one"}, []SourceProvenanceClass{
			{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"},
		})
		capability := validEvidenceCapability()
		capability.EvidenceRateLimitPerMinute = 1
		limiter := NewEvidenceRateLimiter(func() time.Time { return now })
		if err := ValidateEvidenceBundleForExport(bundle, capability, limiter, "credential-rate"); err != nil {
			t.Fatalf("first preflight should pass: %v", err)
		}
		err := ValidateEvidenceBundleForExport(bundle, capability, limiter, "credential-rate")
		assertPreflightReason(t, err, EvidenceRejectRateLimitExceeded)
	})
}

func TestEvidenceBundlePreflightLocalRejectMetrics(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		reason string
		run    func(t *testing.T) error
	}{
		{
			name:   "bundle too large",
			reason: EvidenceRejectBundleTooLarge,
			run: func(t *testing.T) error {
				bundle := mustTestEvidenceBundle(t, "export-metric-large", []string{strings.Repeat("x", 128)}, []SourceProvenanceClass{{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"}})
				capability := validEvidenceCapability()
				capability.EvidenceMaxBundleSizeBytes = 10
				return ValidateEvidenceBundleForExport(bundle, capability, NewEvidenceRateLimiter(func() time.Time { return now }), "credential-metric-large")
			},
		},
		{
			name:   "too many claims",
			reason: EvidenceRejectTooManyClaims,
			run: func(t *testing.T) error {
				bundle := mustTestEvidenceBundle(t, "export-metric-claims", []string{"one", "two"}, []SourceProvenanceClass{{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"}})
				capability := validEvidenceCapability()
				capability.EvidenceMaxClaimsPerBundle = 1
				return ValidateEvidenceBundleForExport(bundle, capability, NewEvidenceRateLimiter(func() time.Time { return now }), "credential-metric-claims")
			},
		},
		{
			name:   "rate limit exceeded",
			reason: EvidenceRejectRateLimitExceeded,
			run: func(t *testing.T) error {
				bundle := mustTestEvidenceBundle(t, "export-metric-rate", []string{"one"}, []SourceProvenanceClass{{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"}})
				capability := validEvidenceCapability()
				capability.EvidenceRateLimitPerMinute = 1
				limiter := NewEvidenceRateLimiter(func() time.Time { return now })
				if err := ValidateEvidenceBundleForExport(bundle, capability, limiter, "credential-metric-rate"); err != nil {
					t.Fatalf("first preflight should pass: %v", err)
				}
				return ValidateEvidenceBundleForExport(bundle, capability, limiter, "credential-metric-rate")
			},
		},
		{
			name:   "source class not eligible",
			reason: EvidenceSourceClassNotEligibleReason("private_diary"),
			run: func(t *testing.T) error {
				bundle := mustTestEvidenceBundle(t, "export-metric-class", []string{"one"}, []SourceProvenanceClass{{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "private_diary"}})
				return ValidateEvidenceBundleForExport(bundle, validEvidenceCapability(), NewEvidenceRateLimiter(func() time.Time { return now }), "credential-metric-class")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := testutil.ToFloat64(metrics.QFEvidenceExportAttempts.WithLabelValues(EvidenceExportStatusLocalReject, TargetContextPacketContext, "personal"))
			err := tc.run(t)
			assertPreflightReason(t, err, tc.reason)
			after := testutil.ToFloat64(metrics.QFEvidenceExportAttempts.WithLabelValues(EvidenceExportStatusLocalReject, TargetContextPacketContext, "personal"))
			if after-before != 1 {
				t.Fatalf("local-reject metric delta for %s = %v, want 1", tc.reason, after-before)
			}
		})
	}
}

func TestEvidenceBundleRejectsIneligibleSourceClass(t *testing.T) {
	bundle := mustTestEvidenceBundle(t, "export-class-001", []string{"claim"}, []SourceProvenanceClass{
		{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "private_diary"},
	})
	err := ValidateEvidenceBundleForExport(bundle, validEvidenceCapability(), NewEvidenceRateLimiter(time.Now), "credential-class")
	assertPreflightReason(t, err, EvidenceSourceClassNotEligibleReason("private_diary"))
}

func TestEvidenceExportTreatsIdempotentReplayAsNoopSuccess(t *testing.T) {
	bundle := mustTestEvidenceBundle(t, "export-idempotent-001", []string{"claim"}, []SourceProvenanceClass{
		{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"},
	})
	payloadHash, err := EvidenceBundlePayloadHash(bundle)
	if err != nil {
		t.Fatalf("payload hash: %v", err)
	}
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != PersonalEvidenceBundlesPath {
			t.Fatalf("request = %s %s, want POST %s", r.Method, r.URL.Path, PersonalEvidenceBundlesPath)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(EvidenceExportResponse{ExportID: bundle.ExportID, BundleID: bundle.BundleID, PayloadHash: payloadHash})
	}))
	defer server.Close()

	client := NewClient(server.URL, "qf-service-token", 1, 25)
	response, err := client.ExportPersonalEvidenceBundle(context.Background(), bundle)
	if err != nil {
		t.Fatalf("ExportPersonalEvidenceBundle failed: %v", err)
	}
	if !response.IdempotentReplay {
		t.Fatalf("IdempotentReplay = false, want true for HTTP 200 same export_id/payload_hash")
	}
	if response.ExportID != bundle.ExportID || response.PayloadHash != payloadHash {
		t.Fatalf("response = %+v, want export_id=%s payload_hash=%s", response, bundle.ExportID, payloadHash)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want exactly one no-op replay attempt", attempts.Load())
	}
}

func TestEvidenceExportCollisionAbortsWithoutRetry(t *testing.T) {
	bundle := mustTestEvidenceBundle(t, "export-collision-001", []string{"claim"}, []SourceProvenanceClass{
		{SourceArtifactID: "artifact-build-001", SourceProvenanceClass: "smackerel_news"},
	})
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(BridgeErrorResponse{Code: EvidenceBridgeExportIDReuseWithDifferentPayload, Message: "export_id already belongs to another payload"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "qf-service-token", 1, 25)
	_, err := client.ExportPersonalEvidenceBundle(context.Background(), bundle)
	if err == nil {
		t.Fatal("expected export_id collision error")
	}
	var exportErr EvidenceExportError
	if !errors.As(err, &exportErr) {
		t.Fatalf("error = %T %v, want EvidenceExportError", err, err)
	}
	if exportErr.Code != EvidenceExportStatusExportIDCollision || exportErr.Retryable {
		t.Fatalf("export error = %+v, want non-retryable export_id_collision", exportErr)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want exactly one collision attempt with no retry", attempts.Load())
	}
}

func mustTestEvidenceBundle(t *testing.T, exportID string, claims []string, sourceClasses []SourceProvenanceClass) PersonalEvidenceBundle {
	t.Helper()
	bundle, err := BuildPacketContextEvidenceBundle(EvidenceBundleInput{
		BundleID:                "bundle-" + exportID,
		ExportID:                exportID,
		CreatedAt:               time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		ConsentScope:            "qf_packet_context",
		SensitivityTier:         "personal",
		PacketID:                "packet-build-001",
		TraceID:                 "trace-build-001",
		SourceArtifactIDs:       []string{"artifact-build-001"},
		SourceRefs:              []string{"https://example.test/source/artifact-build-001"},
		SourceProvenanceClasses: sourceClasses,
		ExtractedClaims:         claims,
		Confidence:              0.87,
		Provenance:              map[string]any{"generator": "smackerel-qf-evidence-test"},
		RedactionSummary:        map[string]any{"omitted_raw_messages": 2},
		RelatedSymbols:          []string{"MSFT"},
		RelatedEntities:         []string{"Federal Reserve"},
	})
	if err != nil {
		t.Fatalf("BuildPacketContextEvidenceBundle: %v", err)
	}
	return bundle
}

func validEvidenceCapability() QFBridgeCapability {
	return QFBridgeCapability{
		SupportedTargetContextTypes:    []string{TargetContextPacketContext},
		EvidenceMaxBundleSizeBytes:     524288,
		EvidenceMaxClaimsPerBundle:     50,
		EvidenceRateLimitPerMinute:     10,
		EligibleSmackerelSourceClasses: []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"},
	}
}

func assertPreflightReason(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected preflight error reason %s", want)
	}
	var preflight EvidencePreflightError
	if errors.As(err, &preflight) {
		if preflight.Reason != want {
			t.Fatalf("preflight reason = %s, want %s", preflight.Reason, want)
		}
		return
	}
	var preflightPtr *EvidencePreflightError
	if errors.As(err, &preflightPtr) {
		if preflightPtr.Reason != want {
			t.Fatalf("preflight reason = %s, want %s", preflightPtr.Reason, want)
		}
		return
	}
	t.Fatalf("error = %T %v, want EvidencePreflightError reason %s", err, err, want)
}
