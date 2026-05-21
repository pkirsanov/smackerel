//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail drives a
// QF-authored packet through connector ingest, persistence, and the live
// Smackerel artifact-detail/search APIs. It proves Scope 3's public qf_card
// model is generic/read-only for unknown decision types, preserves QF title
// and deep link, drops non-public trust internals, and treats preferred_surface
// as placement-only routing.
func TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail(t *testing.T) {
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

	sourceID := fmt.Sprintf("qf-decisions-e2e-surface-%d", time.Now().UnixNano())
	defer qfDecisionsCleanupSource(t, pool, sourceID)
	qfDecisionsCleanupSource(t, pool, sourceID)

	packetID := fmt.Sprintf("packet-e2e-surface-%d", time.Now().UnixNano())
	traceID := fmt.Sprintf("trace-e2e-surface-%d", time.Now().UnixNano())
	unknownDecisionType := "future_surface_decision_type"
	uniqueThesis := fmt.Sprintf("QF e2e surface thesis %d", time.Now().UnixNano())
	unsignedDeepLink := "https://qf.example.test/packets/" + packetID
	signedDeepLink := "https://qf.example.test/signed/" + packetID

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(qfdecisions.QFBridgeCapability{
				SupportedPacketVersions:            []string{"v1"},
				SupportedEventTypes:                []string{"packet_created", "packet_updated", "packet_trust_changed", "packet_archived", "packet_action_boundary_attempted"},
				SupportedDecisionTypes:             []string{"recommendation", "policy_denial", "analysis_note"},
				MaxPageSize:                        100,
				MinPageSize:                        1,
				SupportedTargetContextTypes:        []string{"packet_context"},
				EvidenceMaxBundleSizeBytes:         1048576,
				EvidenceMaxClaimsPerBundle:         50,
				EvidenceRateLimitPerMinute:         60,
				FreshnessSLAP95Seconds:             60,
				AuditEnvelopeVersion:               "v1",
				PreferredSurfaceHintSupported:      true,
				DeepLinkSigningSupported:           true,
				CredentialRotationOverlapSupported: true,
				WatchSignalDirection:               "qf_to_smackerel",
				EligibleSmackerelSourceClasses:     []string{"watch"},
			})
		case r.URL.Path == qfdecisions.DecisionEventsPath:
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events: []qfdecisions.QFDecisionEvent{{
					ContractVersion: 1,
					EventID:         "event-e2e-surface-1",
					PacketID:        packetID,
					IntentID:        "intent-e2e-surface",
					ScenarioID:      "scenario-e2e-surface",
					TraceID:         traceID,
					EventType:       "packet_created",
					DecisionType:    unknownDecisionType,
					ApprovalState:   "display_only",
					PacketVersion:   1,
					PacketURL:       unsignedDeepLink,
					SourceSurface:   "gateway-route",
					CreatedAt:       "2026-05-06T00:00:00Z",
				}},
				NextCursor: "qf-surface-end",
				HasMore:    false,
				ServerTime: "2026-05-06T00:00:00Z",
			})
		case r.URL.Path == qfdecisions.DecisionPacketsPath+"/"+packetID:
			_ = json.NewEncoder(w).Encode(qfdecisions.QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             packetID,
				IntentID:             "intent-e2e-surface",
				ScenarioID:           "scenario-e2e-surface",
				TraceID:              traceID,
				Thesis:               uniqueThesis,
				WhyNow:               "QF-authored timing for surface e2e",
				QuantifiedImpact:     surfaceTrustObject("impact", "high", "impact public summary"),
				ExpertAnalysisBundle: surfaceTrustObject("analysis", "medium", "analysis public summary"),
				CalibrationBadge:     surfaceTrustObject("calibration", "medium", "calibration public summary"),
				DataProvenanceBadge:  surfaceTrustObject("provenance", "low", "provenance public summary"),
				ApprovalState:        "display_only",
				DeepLink:             unsignedDeepLink,
				PacketURLSigned:      signedDeepLink,
				SignatureExpiresAt:   time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
				PreferredSurface:     qfdecisions.PreferredSurfaceSmackerelTelegram,
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

	artifacts, _, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("Sync artifacts = %d, want 1", len(artifacts))
	}

	artifactID, err := pipeline.NewRawArtifactPublisher(pool, natsClient).PublishRawArtifact(ctx, artifacts[0])
	if err != nil {
		t.Fatalf("PublishRawArtifact: %v", err)
	}
	if artifactID == "" {
		t.Fatal("PublishRawArtifact returned empty artifact ID")
	}

	detail := waitForQFCardInArtifactDetail(t, cfg, artifactID)
	assertSurfaceQFCard(t, detail, uniqueThesis, signedDeepLink)

	searchBody := waitForQFCardInSearch(t, cfg, uniqueThesis, artifactID)
	assertSurfaceQFCard(t, searchBody, uniqueThesis, signedDeepLink)

	webSearchBody := waitForQFCardInWebSearch(t, cfg, uniqueThesis)
	assertSurfaceQFHTML(t, webSearchBody, uniqueThesis, signedDeepLink)

	webDetailBody := waitForQFCardInWebDetail(t, cfg, artifactID)
	assertSurfaceQFHTML(t, webDetailBody, uniqueThesis, signedDeepLink)

	assertPWAQFBundleServed(t, cfg)
}

// TestQFDecisionTrustObjectsRenderPublicFieldsAndFallbackOnMissingRequired
// publishes a QF-authored packet with an incomplete public trust object through
// the live persistence path and proves the API/search surfaces render the
// generic fallback card instead of leaking partial or numeric trust internals.
func TestQFDecisionTrustObjectsRenderPublicFieldsAndFallbackOnMissingRequired(t *testing.T) {
	cfg, pool, natsClient := setupQFSurfaceStores(t)

	sourceID := fmt.Sprintf("qf-decisions-e2e-missing-trust-%d", time.Now().UnixNano())
	defer qfDecisionsCleanupSource(t, pool, sourceID)
	qfDecisionsCleanupSource(t, pool, sourceID)

	envelope := qfSurfaceEnvelope("missing-trust", qfdecisions.PreferredSurfaceSmackerelDigest, true, time.Now().UTC().Add(30*time.Minute))
	envelope.DataProvenanceBadge = map[string]any{
		"label":   "provenance without severity",
		"summary": "source-qualified provenance is present but incomplete",
		"score":   0.87,
	}

	artifactID := publishQFSurfaceEnvelope(t, pool, natsClient, sourceID, envelope)

	detailCard := waitForDecodedQFCardInArtifactDetail(t, cfg, artifactID)
	assertMissingTrustFallbackCard(t, detailCard, envelope)

	searchCard := waitForDecodedQFCardInSearch(t, cfg, envelope.Thesis, artifactID)
	assertMissingTrustFallbackCard(t, searchCard, envelope)

	metricsBody := fetchMetricsBody(t, cfg)
	if got := metricValueWithLabels(metricsBody, "smackerel_qf_trust_object_render_failures_total", map[string]string{"reason": qfdecisions.TrustFallbackMissingRequiredField}); got <= 0 {
		t.Fatalf("missing trust fallback metric value = %f, want > 0\n%s", got, metricsBody)
	}
}

// TestQFDecisionDeepLinkAndPreferredSurfaceBranchMatrix proves the live
// qf_card API renderer handles every Scope 3 deep-link status and every
// preferred_surface placement branch without mutating card content, trust
// metadata, read-only state, or action eligibility.
func TestQFDecisionDeepLinkAndPreferredSurfaceBranchMatrix(t *testing.T) {
	cfg, pool, natsClient := setupQFSurfaceStores(t)

	sourceID := fmt.Sprintf("qf-decisions-e2e-branch-matrix-%d", time.Now().UnixNano())
	defer qfDecisionsCleanupSource(t, pool, sourceID)
	qfDecisionsCleanupSource(t, pool, sourceID)

	t.Run("deep_link_statuses", func(t *testing.T) {
		cases := []struct {
			name              string
			includeSigned     bool
			signatureExpiry   time.Time
			wantStatus        string
			wantURLFromSigned bool
		}{
			{
				name:              "signed_used",
				includeSigned:     true,
				signatureExpiry:   time.Now().UTC().Add(30 * time.Minute),
				wantStatus:        qfdecisions.DeepLinkStatusSignedUsed,
				wantURLFromSigned: true,
			},
			{
				name:              "signed_expired_fallback_unsigned",
				includeSigned:     true,
				signatureExpiry:   time.Now().UTC().Add(-30 * time.Minute),
				wantStatus:        qfdecisions.DeepLinkStatusSignedExpiredFallbackUnsigned,
				wantURLFromSigned: false,
			},
			{
				name:              "unsigned_only",
				includeSigned:     false,
				wantStatus:        qfdecisions.DeepLinkStatusUnsignedOnly,
				wantURLFromSigned: false,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				envelope := qfSurfaceEnvelope("deep-link-"+tc.name, qfdecisions.PreferredSurfaceAny, tc.includeSigned, tc.signatureExpiry)
				artifactID := publishQFSurfaceEnvelope(t, pool, natsClient, sourceID, envelope)
				card := waitForDecodedQFCardInSearch(t, cfg, envelope.Thesis, artifactID)

				if card.DeepLink.Status != tc.wantStatus {
					t.Fatalf("deep link status = %q, want %q in card %#v", card.DeepLink.Status, tc.wantStatus, card)
				}
				if tc.wantURLFromSigned && card.DeepLink.URL != envelope.PacketURLSigned {
					t.Fatalf("deep link URL = %q, want signed %q", card.DeepLink.URL, envelope.PacketURLSigned)
				}
				if !tc.wantURLFromSigned && card.DeepLink.URL != envelope.DeepLink {
					t.Fatalf("deep link URL = %q, want unsigned %q", card.DeepLink.URL, envelope.DeepLink)
				}
				assertAnyPlacement(t, card.Placement)
				assertTrustAndActionBoundary(t, card, envelope)
			})
		}
	})

	t.Run("preferred_surface_placements", func(t *testing.T) {
		cases := []struct {
			name             string
			preferredSurface string
			want             qfdecisions.PlacementRender
		}{
			{
				name:             "smackerel_digest",
				preferredSurface: qfdecisions.PreferredSurfaceSmackerelDigest,
				want: qfdecisions.PlacementRender{
					PrimarySurface:          qfdecisions.PreferredSurfaceSmackerelDigest,
					IncludeInDigest:         true,
					IncludeInSearch:         true,
					IncludeInArtifactDetail: true,
				},
			},
			{
				name:             "smackerel_telegram",
				preferredSurface: qfdecisions.PreferredSurfaceSmackerelTelegram,
				want: qfdecisions.PlacementRender{
					PrimarySurface:          qfdecisions.PreferredSurfaceSmackerelTelegram,
					QueueTelegram:           true,
					IncludeInSearch:         true,
					IncludeInArtifactDetail: true,
				},
			},
			{
				name:             "qf_dashboard",
				preferredSurface: qfdecisions.PreferredSurfaceQFDashboard,
				want: qfdecisions.PlacementRender{
					PrimarySurface:          qfdecisions.PreferredSurfaceQFDashboard,
					ShowInQFDashboard:       true,
					IncludeInSearch:         true,
					IncludeInArtifactDetail: true,
				},
			},
			{
				name:             "any",
				preferredSurface: qfdecisions.PreferredSurfaceAny,
				want: qfdecisions.PlacementRender{
					PrimarySurface:          qfdecisions.PreferredSurfaceAny,
					IncludeInDigest:         true,
					QueueTelegram:           true,
					ShowInQFDashboard:       true,
					IncludeInSearch:         true,
					IncludeInArtifactDetail: true,
				},
			},
			{
				name:             "missing_hint_defaults_to_qf_dashboard_for_recommendation",
				preferredSurface: "",
				want: qfdecisions.PlacementRender{
					PrimarySurface:          qfdecisions.PreferredSurfaceQFDashboard,
					ShowInQFDashboard:       true,
					IncludeInSearch:         true,
					IncludeInArtifactDetail: true,
				},
			},
		}

		var baseline *qfdecisions.PacketCard
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				envelope := qfSurfaceEnvelope("preferred-"+tc.name, tc.preferredSurface, true, time.Now().UTC().Add(30*time.Minute))
				envelope.Thesis = "QF preferred surface placement invariant"
				envelope.WhyNow = "Preferred-surface hints route placement only"
				artifactID := publishQFSurfaceEnvelope(t, pool, natsClient, sourceID, envelope)
				card := waitForDecodedQFCardInSearch(t, cfg, envelope.Thesis, artifactID)

				assertPlacement(t, card.Placement, tc.want)
				assertTrustAndActionBoundary(t, card, envelope)
				if card.DeepLink.Status != qfdecisions.DeepLinkStatusSignedUsed || card.DeepLink.URL != envelope.PacketURLSigned {
					t.Fatalf("preferred_surface=%q changed deep link: %#v", tc.preferredSurface, card.DeepLink)
				}
				if baseline == nil {
					baseline = &card
					return
				}
				assertPreferredSurfaceDidNotMutatePublicCard(t, *baseline, card)
			})
		}
	})

	metricsBody := fetchMetricsBody(t, cfg)
	for _, status := range []string{
		qfdecisions.DeepLinkStatusSignedUsed,
		qfdecisions.DeepLinkStatusSignedExpiredFallbackUnsigned,
		qfdecisions.DeepLinkStatusUnsignedOnly,
	} {
		if got := metricValueWithLabels(metricsBody, "smackerel_qf_deep_link_render_total", map[string]string{"surface": qfdecisions.SurfaceSearch, "status": status}); got <= 0 {
			t.Fatalf("deep link metric status=%s value = %f, want > 0\n%s", status, got, metricsBody)
		}
	}
}

func waitForQFCardInArtifactDetail(t *testing.T, cfg e2eConfig, artifactID string) []byte {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var lastBody []byte
	for time.Now().Before(deadline) {
		resp, err := apiGet(cfg, "/api/artifact/"+artifactID)
		if err == nil && resp.StatusCode == http.StatusOK {
			body, readErr := readBody(resp)
			resp.Body.Close()
			if readErr == nil {
				lastBody = body
				if bytes.Contains(body, []byte("qf_card")) {
					return body
				}
			}
		} else if resp != nil {
			body, readErr := readBody(resp)
			resp.Body.Close()
			if readErr == nil {
				lastBody = body
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("artifact detail never exposed qf_card for %s; last body=%s", artifactID, lastBody)
	return nil
}

func waitForQFCardInSearch(t *testing.T, cfg e2eConfig, query, artifactID string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"query": query, "limit": 10})
	if err != nil {
		t.Fatalf("marshal search payload: %v", err)
	}
	deadline := time.Now().Add(90 * time.Second)
	var lastBody []byte
	client := &http.Client{Timeout: 30 * time.Second}
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/search", bytes.NewReader(payload))
		if reqErr != nil {
			t.Fatalf("create search request: %v", reqErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
		resp, doErr := client.Do(req)
		if doErr == nil && resp.StatusCode == http.StatusOK {
			body, readErr := readBody(resp)
			resp.Body.Close()
			if readErr == nil {
				lastBody = body
				if bytes.Contains(body, []byte(artifactID)) && bytes.Contains(body, []byte("qf_card")) {
					return body
				}
			}
		} else if resp != nil {
			body, readErr := readBody(resp)
			resp.Body.Close()
			if readErr == nil {
				lastBody = body
			}
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("search never exposed qf_card for %s; last body=%s", artifactID, lastBody)
	return nil
}

func assertSurfaceQFCard(t *testing.T, body []byte, title, signedDeepLink string) {
	t.Helper()
	bodyText := string(body)
	for _, want := range []string{
		`"qf_card"`,
		`"card_kind":"` + qfdecisions.CardKindGenericPacket + `"`,
		`"read_only":true`,
		`"action_eligible":false`,
		`"title":"` + title + `"`,
		`"status":"` + qfdecisions.DeepLinkStatusSignedUsed + `"`,
		signedDeepLink,
		`"queue_telegram":true`,
		`"include_in_digest":false`,
		`"show_in_qf_dashboard":false`,
		"calibration public summary",
		"provenance public summary",
	} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("surface body missing %q\n%s", want, bodyText)
		}
	}
	for _, forbidden := range []string{"confidence", "score", "value"} {
		if strings.Contains(bodyText, forbidden) {
			t.Fatalf("surface body leaked non-public trust field %q\n%s", forbidden, bodyText)
		}
	}
}

func waitForQFCardInWebSearch(t *testing.T, cfg e2eConfig, query string) []byte {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	client := &http.Client{Timeout: 30 * time.Second}
	var lastBody []byte
	for time.Now().Before(deadline) {
		form := url.Values{"query": []string{query}}
		req, reqErr := http.NewRequest(http.MethodPost, cfg.CoreURL+"/search", strings.NewReader(form.Encode()))
		if reqErr != nil {
			t.Fatalf("create web search request: %v", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, doErr := client.Do(req)
		if doErr == nil && resp.StatusCode == http.StatusOK {
			body, readErr := readBody(resp)
			if readErr == nil {
				lastBody = body
				if bytes.Contains(body, []byte("qf-card")) && bytes.Contains(body, []byte(query)) {
					return body
				}
			}
		} else if resp != nil {
			body, readErr := readBody(resp)
			if readErr == nil {
				lastBody = body
			}
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("web search never exposed qf-card for %q; last body=%s", query, lastBody)
	return nil
}

func waitForQFCardInWebDetail(t *testing.T, cfg e2eConfig, artifactID string) []byte {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var lastBody []byte
	for time.Now().Before(deadline) {
		resp, err := apiGet(cfg, "/artifact/"+artifactID)
		if err == nil && resp.StatusCode == http.StatusOK {
			body, readErr := readBody(resp)
			if readErr == nil {
				lastBody = body
				if bytes.Contains(body, []byte("qf-card")) {
					return body
				}
			}
		} else if resp != nil {
			body, readErr := readBody(resp)
			if readErr == nil {
				lastBody = body
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("web detail never exposed qf-card for %s; last body=%s", artifactID, lastBody)
	return nil
}

func assertSurfaceQFHTML(t *testing.T, body []byte, title, signedDeepLink string) {
	t.Helper()
	bodyText := string(body)
	for _, want := range []string{
		"qf-card",
		"read-only",
		title,
		signedDeepLink,
		"calibration public summary",
		"provenance public summary",
	} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("QF HTML surface missing %q\n%s", want, bodyText)
		}
	}
	qfFragment := bodyText
	if start := strings.Index(bodyText, `<div class="qf-card"`); start >= 0 {
		qfFragment = bodyText[start:]
	}
	for _, forbidden := range []string{"confidence", "score", "value"} {
		if strings.Contains(qfFragment, forbidden) {
			t.Fatalf("QF HTML surface leaked non-public trust field %q\n%s", forbidden, qfFragment)
		}
	}
}

func assertPWAQFBundleServed(t *testing.T, cfg e2eConfig) {
	t.Helper()
	checks := map[string][]string{
		"/pwa/drive-search.html":          {"qf-result-template", "QF Companion", "Read-only"},
		"/pwa/drive-search.js":            {"qf_card", "trust_objects", "type=qf"},
		"/pwa/drive-artifact-detail.html": {"qf-packet-panel", "qf-trust-list", "qf-deep-link", "qf-evidence-builder-link", "qf-evidence-revoke"},
		"/pwa/drive-artifact-detail.js":   {"loadQFDetail", "/api/artifact/", "trust_objects", "/api/qf/evidence-bundles/", "consent_revoked"},
	}
	for path, wants := range checks {
		resp, err := apiGet(cfg, path)
		if err != nil {
			t.Fatalf("load PWA asset %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := readBody(resp)
			t.Fatalf("PWA asset %s status=%d body=%s", path, resp.StatusCode, body)
		}
		body, err := readBody(resp)
		if err != nil {
			t.Fatalf("read PWA asset %s: %v", path, err)
		}
		bodyText := string(body)
		for _, want := range wants {
			if !strings.Contains(bodyText, want) {
				t.Fatalf("PWA asset %s missing %q\n%s", path, want, bodyText)
			}
		}
	}
}

func setupQFSurfaceStores(t *testing.T) (e2eConfig, *pgxpool.Pool, *smacknats.Client) {
	t.Helper()
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
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	t.Cleanup(pool.Close)

	natsClient, err := smacknats.Connect(ctx, natsURL, cfg.AuthToken)
	if err != nil {
		t.Fatalf("connect e2e NATS: %v", err)
	}
	t.Cleanup(natsClient.Close)

	return cfg, pool, natsClient
}

func qfSurfaceEnvelope(suffix, preferredSurface string, includeSigned bool, signatureExpiresAt time.Time) qfdecisions.QFDecisionPacketEnvelope {
	now := time.Now().UTC()
	packetID := fmt.Sprintf("packet-e2e-%s-%d", suffix, now.UnixNano())
	unsignedDeepLink := "https://qf.example.test/packets/" + packetID
	envelope := qfdecisions.QFDecisionPacketEnvelope{
		ContractVersion:      1,
		PacketID:             packetID,
		IntentID:             "intent-e2e-" + suffix,
		ScenarioID:           "scenario-e2e-" + suffix,
		TraceID:              fmt.Sprintf("trace-e2e-%s-%d", suffix, now.UnixNano()),
		Thesis:               fmt.Sprintf("QF e2e %s thesis", suffix),
		WhyNow:               "QF-authored timing stays source-qualified",
		QuantifiedImpact:     surfaceTrustObject("impact", "high", "impact public summary"),
		ExpertAnalysisBundle: surfaceTrustObject("analysis", "medium", "analysis public summary"),
		CalibrationBadge:     surfaceTrustObject("calibration", "medium", "calibration public summary"),
		DataProvenanceBadge:  surfaceTrustObject("provenance", "low", "provenance public summary"),
		ApprovalState:        "display_only",
		DeepLink:             unsignedDeepLink,
		PreferredSurface:     preferredSurface,
		PacketVersion:        1,
		DecisionType:         qfdecisions.DecisionTypeRecommendation,
		CreatedAt:            now.Format(time.RFC3339),
		UpdatedAt:            now.Add(time.Second).Format(time.RFC3339),
	}
	if includeSigned {
		envelope.PacketURLSigned = "https://qf.example.test/signed/" + packetID
		envelope.SignatureExpiresAt = signatureExpiresAt.Format(time.RFC3339)
	}
	return envelope
}

func publishQFSurfaceEnvelope(t *testing.T, pool *pgxpool.Pool, natsClient *smacknats.Client, sourceID string, envelope qfdecisions.QFDecisionPacketEnvelope) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	event := qfdecisions.QFDecisionEvent{
		ContractVersion: 1,
		EventID:         "event-" + envelope.PacketID,
		PacketID:        envelope.PacketID,
		IntentID:        envelope.IntentID,
		ScenarioID:      envelope.ScenarioID,
		TraceID:         envelope.TraceID,
		EventType:       "packet_created",
		DecisionType:    envelope.DecisionType,
		ApprovalState:   envelope.ApprovalState,
		PacketVersion:   envelope.PacketVersion,
		PacketURL:       envelope.DeepLink,
		SourceSurface:   "gateway-route",
		CreatedAt:       envelope.CreatedAt,
	}
	artifact, diagnostic := qfdecisions.NewNormalizer(sourceID, 1).Normalize(event, envelope, time.Now().UTC())
	if diagnostic != nil {
		t.Fatalf("Normalize diagnostic: %#v", diagnostic)
	}
	if artifact == nil {
		t.Fatal("Normalize returned nil artifact")
	}
	artifactID, err := pipeline.NewRawArtifactPublisher(pool, natsClient).PublishRawArtifact(ctx, *artifact)
	if err != nil {
		t.Fatalf("PublishRawArtifact: %v", err)
	}
	if artifactID == "" {
		t.Fatal("PublishRawArtifact returned empty artifact ID")
	}
	return artifactID
}

func waitForDecodedQFCardInArtifactDetail(t *testing.T, cfg e2eConfig, artifactID string) qfdecisions.PacketCard {
	t.Helper()
	body := waitForQFCardInArtifactDetail(t, cfg, artifactID)
	return decodeQFCard(t, body, artifactID)
}

func waitForDecodedQFCardInSearch(t *testing.T, cfg e2eConfig, query, artifactID string) qfdecisions.PacketCard {
	t.Helper()
	body := waitForQFCardInSearch(t, cfg, query, artifactID)
	return decodeQFCard(t, body, artifactID)
}

func decodeQFCard(t *testing.T, body []byte, artifactID string) qfdecisions.PacketCard {
	t.Helper()
	var detail struct {
		QFCard *qfdecisions.PacketCard `json:"qf_card"`
	}
	if err := json.Unmarshal(body, &detail); err == nil && detail.QFCard != nil {
		return *detail.QFCard
	}

	var search struct {
		Results []struct {
			ArtifactID string                  `json:"artifact_id"`
			ID         string                  `json:"id"`
			QFCard     *qfdecisions.PacketCard `json:"qf_card"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &search); err != nil {
		t.Fatalf("decode qf_card payload for artifact %s: %v\n%s", artifactID, err, body)
	}
	for _, result := range search.Results {
		if result.QFCard != nil && (result.ArtifactID == artifactID || result.ID == artifactID) {
			return *result.QFCard
		}
	}
	for _, result := range search.Results {
		if result.QFCard != nil {
			return *result.QFCard
		}
	}
	t.Fatalf("qf_card missing for artifact %s in payload\n%s", artifactID, body)
	return qfdecisions.PacketCard{}
}

func assertMissingTrustFallbackCard(t *testing.T, card qfdecisions.PacketCard, envelope qfdecisions.QFDecisionPacketEnvelope) {
	t.Helper()
	if card.CardKind != qfdecisions.CardKindGenericPacket {
		t.Fatalf("fallback card_kind = %q, want %q in card %#v", card.CardKind, qfdecisions.CardKindGenericPacket, card)
	}
	if card.FallbackReason != qfdecisions.TrustFallbackMissingRequiredField {
		t.Fatalf("fallback_reason = %q, want %q in card %#v", card.FallbackReason, qfdecisions.TrustFallbackMissingRequiredField, card)
	}
	if len(card.TrustObjects) != 0 {
		t.Fatalf("fallback card rendered incomplete trust objects: %#v", card.TrustObjects)
	}
	if card.PacketID != envelope.PacketID || card.TraceID != envelope.TraceID || card.ApprovalState != envelope.ApprovalState {
		t.Fatalf("fallback card lost packet identity: got %#v envelope %#v", card, envelope)
	}
	if card.Thesis != envelope.Thesis || card.WhyNow != envelope.WhyNow || card.Title != envelope.Thesis {
		t.Fatalf("fallback card lost QF-authored content: got %#v envelope %#v", card, envelope)
	}
	if !card.ReadOnly || card.ActionEligible {
		t.Fatalf("fallback card violated read-only action boundary: %#v", card)
	}
	if card.DeepLink.Status != qfdecisions.DeepLinkStatusSignedUsed || card.DeepLink.URL != envelope.PacketURLSigned {
		t.Fatalf("fallback card deep link = %#v, want signed %q", card.DeepLink, envelope.PacketURLSigned)
	}
}

func assertTrustAndActionBoundary(t *testing.T, card qfdecisions.PacketCard, envelope qfdecisions.QFDecisionPacketEnvelope) {
	t.Helper()
	if card.CardKind != qfdecisions.CardKindQFPacket {
		t.Fatalf("card_kind = %q, want %q in card %#v", card.CardKind, qfdecisions.CardKindQFPacket, card)
	}
	if card.Title != envelope.Thesis || card.Thesis != envelope.Thesis || card.WhyNow != envelope.WhyNow {
		t.Fatalf("card content mutated: got %#v envelope %#v", card, envelope)
	}
	if card.PacketID != envelope.PacketID || card.TraceID != envelope.TraceID || card.ApprovalState != envelope.ApprovalState {
		t.Fatalf("card identity mutated: got %#v envelope %#v", card, envelope)
	}
	if !card.ReadOnly || card.ActionEligible {
		t.Fatalf("card violated read-only action boundary: %#v", card)
	}
	if len(card.TrustObjects) != 4 {
		t.Fatalf("trust object count = %d, want 4 in card %#v", len(card.TrustObjects), card)
	}
	for _, trust := range card.TrustObjects {
		if trust.Label == "" || trust.Severity == "" || trust.Summary == "" {
			t.Fatalf("trust object missing public fields: %#v", trust)
		}
		if strings.Contains(trust.Summary, "0.91") || strings.Contains(trust.Summary, "42") || strings.Contains(trust.Summary, "12.5") {
			t.Fatalf("trust object leaked numeric internals: %#v", trust)
		}
	}
}

func assertAnyPlacement(t *testing.T, placement qfdecisions.PlacementRender) {
	t.Helper()
	assertPlacement(t, placement, qfdecisions.PlacementRender{
		PrimarySurface:          qfdecisions.PreferredSurfaceAny,
		IncludeInDigest:         true,
		QueueTelegram:           true,
		ShowInQFDashboard:       true,
		IncludeInSearch:         true,
		IncludeInArtifactDetail: true,
	})
}

func assertPlacement(t *testing.T, got, want qfdecisions.PlacementRender) {
	t.Helper()
	if got.PrimarySurface != want.PrimarySurface ||
		got.IncludeInDigest != want.IncludeInDigest ||
		got.QueueTelegram != want.QueueTelegram ||
		got.ShowInQFDashboard != want.ShowInQFDashboard ||
		got.IncludeInSearch != want.IncludeInSearch ||
		got.IncludeInArtifactDetail != want.IncludeInArtifactDetail {
		t.Fatalf("placement = %#v, want %#v", got, want)
	}
}

func assertPreferredSurfaceDidNotMutatePublicCard(t *testing.T, baseline, card qfdecisions.PacketCard) {
	t.Helper()
	if card.Title != baseline.Title || card.Thesis != baseline.Thesis || card.WhyNow != baseline.WhyNow ||
		card.ReadOnly != baseline.ReadOnly || card.ActionEligible != baseline.ActionEligible ||
		card.DecisionType != baseline.DecisionType || card.ApprovalState != baseline.ApprovalState {
		t.Fatalf("preferred_surface mutated public content/action state: baseline=%#v got=%#v", baseline, card)
	}
	if card.DeepLink.Status != baseline.DeepLink.Status {
		t.Fatalf("preferred_surface mutated deep-link status: baseline=%#v got=%#v", baseline.DeepLink, card.DeepLink)
	}
	if len(card.TrustObjects) != len(baseline.TrustObjects) {
		t.Fatalf("preferred_surface mutated trust object count: baseline=%#v got=%#v", baseline.TrustObjects, card.TrustObjects)
	}
	for i := range card.TrustObjects {
		gotTrust := card.TrustObjects[i]
		wantTrust := baseline.TrustObjects[i]
		if gotTrust.Kind != wantTrust.Kind || gotTrust.Label != wantTrust.Label || gotTrust.Severity != wantTrust.Severity || gotTrust.Summary != wantTrust.Summary || gotTrust.Detail != wantTrust.Detail {
			t.Fatalf("preferred_surface mutated trust object %d: baseline=%#v got=%#v", i, wantTrust, gotTrust)
		}
	}
}

func fetchMetricsBody(t *testing.T, cfg e2eConfig) string {
	t.Helper()
	resp, err := apiGet(cfg, "/metrics")
	if err != nil {
		t.Fatalf("scrape /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status=%d body=%s", resp.StatusCode, body)
	}
	return string(body)
}

func metricValueWithLabels(metricsBody, metricName string, labels map[string]string) float64 {
	for _, line := range strings.Split(metricsBody, "\n") {
		if !strings.HasPrefix(line, metricName) {
			continue
		}
		matched := true
		for label, value := range labels {
			if !strings.Contains(line, label+"=\""+value+"\"") {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		return value
	}
	return 0
}

func surfaceTrustObject(label, severity, summary string) map[string]any {
	return map[string]any{
		"label":      label,
		"severity":   severity,
		"summary":    summary,
		"detail":     summary + " detail",
		"links":      []any{map[string]any{"label": "QF", "url": "https://qf.example.test/reference"}},
		"confidence": 0.91,
		"score":      42,
		"value":      12.5,
	}
}
