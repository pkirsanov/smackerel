package qfdecisions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

func TestConnectorID(t *testing.T) {
	c := New(DefaultConnectorID)
	if c.ID() != DefaultConnectorID {
		t.Errorf("got ID %q, want %q", c.ID(), DefaultConnectorID)
	}
}

func TestParseConfigRequiresExplicitFields(t *testing.T) {
	valid := connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       "https://qf.example.test",
			"packet_version": 1,
			"page_size":      25,
		},
	}

	cases := []struct {
		name      string
		mutate    func(connector.ConnectorConfig) connector.ConnectorConfig
		wantError string
	}{
		{
			name: "missing base_url",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				delete(cfg.SourceConfig, "base_url")
				return cfg
			},
			wantError: "base_url",
		},
		{
			name: "missing credential_ref",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				cfg.Credentials = map[string]string{}
				return cfg
			},
			wantError: "credential_ref",
		},
		{
			name: "missing sync_schedule",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				cfg.SyncSchedule = ""
				return cfg
			},
			wantError: "sync_schedule",
		},
		{
			name: "missing packet_version",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				delete(cfg.SourceConfig, "packet_version")
				return cfg
			},
			wantError: "packet_version",
		},
		{
			name: "missing page_size",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				delete(cfg.SourceConfig, "page_size")
				return cfg
			},
			wantError: "page_size",
		},
		{
			name: "invalid URL",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				cfg.SourceConfig["base_url"] = "qf.example.test"
				return cfg
			},
			wantError: "base_url",
		},
		{
			name: "invalid page size",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				cfg.SourceConfig["page_size"] = 0
				return cfg
			},
			wantError: "page_size",
		},
		{
			name: "invalid cron",
			mutate: func(cfg connector.ConnectorConfig) connector.ConnectorConfig {
				cfg.SyncSchedule = "every five minutes"
				return cfg
			},
			wantError: "sync_schedule",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig(tc.mutate(valid))
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantError)
			}
		})
	}
}

func TestConnectValidConfigSetsHealthy(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case CapabilitiesPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		case DecisionEventsPath:
			// Connect must NOT probe /decision-events after Round 2D — the
			// capability handshake is the sole reachability/auth probe.
			t.Fatalf("Connect must not call %q after capability replaces Validate", r.URL.Path)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       srv.URL + "/",
			"packet_version": 1,
			"page_size":      25,
		},
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if gotAuth != "Bearer qf-service-token" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Fatalf("health = %s, want %s", c.Health(context.Background()), connector.HealthHealthy)
	}
}

func TestConnectAuthFailureSetsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(BridgeErrorResponse{Code: "unauthorized", Message: "authorization is required"})
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1))
	if err == nil {
		t.Fatal("expected auth failure")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Fatalf("health = %s, want %s", c.Health(context.Background()), connector.HealthError)
	}
}

func TestConnectSchemaMismatchSetsDegraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BridgeErrorResponse{Code: "invalid_query_parameter", Message: "packet_version 99 is unsupported"})
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 99))
	if err == nil {
		t.Fatal("expected schema compatibility failure")
	}
	if !strings.Contains(err.Error(), "packet_version") {
		t.Fatalf("error should name packet_version: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Fatalf("health = %s, want %s", c.Health(context.Background()), connector.HealthDegraded)
	}
}

func TestCloseDisconnectsConnector(t *testing.T) {
	c := New(DefaultConnectorID)
	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Fatalf("health = %s, want %s", c.Health(context.Background()), connector.HealthDisconnected)
	}
}

// TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity proves that:
//   - response.next_cursor is the canonical advancement value persisted by the connector;
//   - per-event QFDecisionEvent.cursor is diagnostic-only and never rewrites the
//     advancement value or the QF packet identity;
//   - cursor replay (calling Sync again with empty cursor or the same cursor) returns
//     the same QF packet IDs without inventing Smackerel-local recommendation IDs.
//
// SCN-SM-041-005.
func TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity(t *testing.T) {
	metrics.QFPacketIngestTotal.Reset()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		case DecisionEventsPath:
			calls++
			cursor := r.URL.Query().Get("cursor")
			// Two pages: first call returns events with next_cursor "qf-page-2",
			// second call returns the same events again (replay) with the same next_cursor.
			events := []QFDecisionEvent{
				{
					ContractVersion: 1,
					EventID:         "event-A",
					PacketID:        "packet-A",
					IntentID:        "intent-A",
					ScenarioID:      "scenario-A",
					TraceID:         "trace-A",
					EventType:       "packet_created",
					DecisionType:    DecisionTypeRecommendation,
					ApprovalState:   "display_only",
					PacketVersion:   1,
					// Per-event cursor MUST be ignored for advancement.
					Cursor:    "qf-event-A-checkpoint",
					PacketURL: "https://qf.example.test/packets/packet-A",
					CreatedAt: "2026-05-06T00:00:00Z",
				},
				{
					ContractVersion: 1,
					EventID:         "event-B",
					PacketID:        "packet-B",
					IntentID:        "intent-B",
					ScenarioID:      "scenario-B",
					TraceID:         "trace-B",
					EventType:       "packet_created",
					DecisionType:    DecisionTypeRecommendation,
					ApprovalState:   "display_only",
					PacketVersion:   1,
					Cursor:          "qf-event-B-checkpoint",
					PacketURL:       "https://qf.example.test/packets/packet-B",
					CreatedAt:       "2026-05-06T00:00:01Z",
				},
			}
			_ = cursor // intentionally not used by fake — every call returns the same identity
			_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
				Events:     events,
				NextCursor: "qf-page-2",
				HasMore:    false,
				ServerTime: "2026-05-06T00:01:00Z",
			})
		case DecisionPacketsPath + "/packet-A":
			_ = json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-A",
				IntentID:             "intent-A",
				ScenarioID:           "scenario-A",
				TraceID:              "trace-A",
				Thesis:               "thesis A",
				WhyNow:               "why-now A",
				QuantifiedImpact:     map[string]any{"unit": "bps"},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-A"},
				CalibrationBadge:     map[string]any{"state": "calibrated"},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-A",
				PacketVersion:        1,
				DecisionType:         DecisionTypeRecommendation,
				CreatedAt:            "2026-05-06T00:00:00Z",
				UpdatedAt:            "2026-05-06T00:00:00Z",
			})
		case DecisionPacketsPath + "/packet-B":
			_ = json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-B",
				IntentID:             "intent-B",
				ScenarioID:           "scenario-B",
				TraceID:              "trace-B",
				Thesis:               "thesis B",
				WhyNow:               "why-now B",
				QuantifiedImpact:     map[string]any{"unit": "bps"},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-B"},
				CalibrationBadge:     map[string]any{"state": "calibrated"},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-B",
				PacketVersion:        1,
				DecisionType:         DecisionTypeRecommendation,
				CreatedAt:            "2026-05-06T00:00:01Z",
				UpdatedAt:            "2026-05-06T00:00:01Z",
			})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1)); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	artifacts1, cursor1, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("first Sync failed: %v", err)
	}
	if cursor1 != "qf-page-2" {
		t.Fatalf("first Sync cursor = %q, want %q (response.next_cursor must be canonical)", cursor1, "qf-page-2")
	}
	if len(artifacts1) != 2 {
		t.Fatalf("first Sync produced %d artifacts, want 2", len(artifacts1))
	}
	ingestMetric := testutil.ToFloat64(metrics.QFPacketIngestTotal.WithLabelValues("packet_created", DecisionTypeRecommendation, "display_only", metricUnknown))
	if ingestMetric != 2 {
		t.Fatalf("packet ingest metric after first Sync = %v, want 2", ingestMetric)
	}
	for _, want := range []string{"packet-A", "packet-B"} {
		found := false
		for _, a := range artifacts1 {
			if a.SourceRef == want {
				found = true
				if a.SourceID != DefaultConnectorID {
					t.Fatalf("artifact %s SourceID = %q, want %q", want, a.SourceID, DefaultConnectorID)
				}
			}
		}
		if !found {
			t.Fatalf("first Sync missing packet %s", want)
		}
	}

	// Per-event cursor must NOT have leaked into the canonical advancement.
	if cursor1 == "qf-event-A-checkpoint" || cursor1 == "qf-event-B-checkpoint" {
		t.Fatalf("per-event cursor leaked into advancement: %q", cursor1)
	}

	// Replay (clear cursor): packet IDs MUST remain stable, no Smackerel-local IDs.
	artifacts2, cursor2, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("replay Sync failed: %v", err)
	}
	if cursor2 != cursor1 {
		t.Fatalf("replay Sync cursor = %q, want %q", cursor2, cursor1)
	}
	if len(artifacts2) != len(artifacts1) {
		t.Fatalf("replay artifact count = %d, want %d", len(artifacts2), len(artifacts1))
	}
	for i := range artifacts2 {
		if artifacts2[i].SourceRef != artifacts1[i].SourceRef {
			t.Fatalf("replay packet identity drift: artifacts2[%d].SourceRef = %q, want %q",
				i, artifacts2[i].SourceRef, artifacts1[i].SourceRef)
		}
		if artifacts2[i].SourceID != DefaultConnectorID {
			t.Fatalf("replay artifacts must remain qf-decisions, got %q", artifacts2[i].SourceID)
		}
		// trace_id metadata must be preserved verbatim — no Smackerel-local IDs.
		traceID, ok := artifacts2[i].Metadata["trace_id"].(string)
		if !ok || traceID == "" {
			t.Fatalf("replay artifact missing trace_id metadata: %v", artifacts2[i].Metadata)
		}
	}

	if calls < 2 {
		t.Fatalf("expected at least 2 calls to QF events endpoint, got %d", calls)
	}
}

func validConnectorConfig(baseURL string, packetVersion int) connector.ConnectorConfig {
	return connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       baseURL,
			"packet_version": packetVersion,
			"page_size":      25,
		},
	}
}

// defaultValidCapability returns a QFBridgeCapability that satisfies
// CompatibilityCheck (audit_envelope_version=v1, supported_packet_versions
// includes "v1", supported_decision_types includes the three required types,
// max_page_size>=1). Tests mutate fields on the returned value to simulate
// specific contract violations or capability bounds.
func defaultValidCapability() QFBridgeCapability {
	return QFBridgeCapability{
		SupportedPacketVersions:        []string{"v1"},
		SupportedEventTypes:            []string{"packet_created", "packet_updated", "packet_trust_changed", "packet_archived", "packet_action_boundary_attempted"},
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
	}
}

// --- Round 2D — capability lifecycle wiring ---

// TestConnect_FetchCapabilityFailureReturnsError proves that a 5xx from
// /capabilities causes Connect() to return an error and leaves the connector
// in CapabilityStatusUnfetched with a zero capabilityFetchedAt.
func TestConnect_FetchCapabilityFailureReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1))
	if err == nil {
		t.Fatal("expected capability fetch failure")
	}
	if !strings.Contains(err.Error(), "qf capability handshake") {
		t.Fatalf("error should name capability handshake: %v", err)
	}
	if c.capabilityStatus != CapabilityStatusUnfetched {
		t.Fatalf("capabilityStatus = %q, want %q", c.capabilityStatus, CapabilityStatusUnfetched)
	}
	if !c.capabilityFetchedAt.IsZero() {
		t.Fatalf("capabilityFetchedAt = %v, want zero on fetch failure", c.capabilityFetchedAt)
	}
}

// TestConnect_CapabilityIncompatibleReturnsError proves that a fetched-but-
// incompatible capability (audit_envelope_version != v1) causes Connect() to
// return CapabilityMismatchError, set capabilityStatus = Incompatible, and
// stamp capabilityFetchedAt to a non-zero time.
func TestConnect_CapabilityIncompatibleReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case CapabilitiesPath:
			cap := defaultValidCapability()
			cap.AuditEnvelopeVersion = "v2" // INCOMPATIBLE — Smackerel only consumes v1
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cap)
		default:
			t.Fatalf("unexpected request path %q during capability mismatch test", r.URL.Path)
		}
	}))
	defer srv.Close()

	before := time.Now().UTC()
	c := New(DefaultConnectorID)
	err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1))
	if err == nil {
		t.Fatal("expected capability mismatch")
	}
	var mismatch CapabilityMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error is not a CapabilityMismatchError: %v", err)
	}
	if mismatch.Field != "audit_envelope_version" {
		t.Fatalf("mismatch field = %q, want audit_envelope_version", mismatch.Field)
	}
	if mismatch.Required != "v1" || mismatch.Actual != "v2" {
		t.Fatalf("mismatch values = required=%q actual=%q, want v1/v2", mismatch.Required, mismatch.Actual)
	}
	if c.capabilityStatus != CapabilityStatusIncompatible {
		t.Fatalf("capabilityStatus = %q, want %q", c.capabilityStatus, CapabilityStatusIncompatible)
	}
	if c.capabilityFetchedAt.IsZero() {
		t.Fatal("capabilityFetchedAt should be non-zero after a fetched-but-incompatible capability")
	}
	if c.capabilityFetchedAt.Before(before) {
		t.Fatalf("capabilityFetchedAt = %v, must be at or after %v", c.capabilityFetchedAt, before)
	}
}

// TestConnect_CapabilityCompatibleSucceeds proves the happy path: a valid v1
// capability response causes Connect() to return nil, set status =
// Compatible, populate capability fields, and stamp capabilityFetchedAt.
func TestConnect_CapabilityCompatibleSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case CapabilitiesPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		default:
			t.Fatalf("unexpected request path %q during capability success test", r.URL.Path)
		}
	}))
	defer srv.Close()

	before := time.Now().UTC()
	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1)); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.capabilityStatus != CapabilityStatusCompatible {
		t.Fatalf("capabilityStatus = %q, want %q", c.capabilityStatus, CapabilityStatusCompatible)
	}
	if c.capabilityFetchedAt.IsZero() {
		t.Fatal("capabilityFetchedAt should be non-zero after a compatible capability")
	}
	if c.capabilityFetchedAt.Before(before) {
		t.Fatalf("capabilityFetchedAt = %v, must be at or after %v", c.capabilityFetchedAt, before)
	}
	if c.capability.AuditEnvelopeVersion != "v1" {
		t.Fatalf("capability.AuditEnvelopeVersion = %q, want v1", c.capability.AuditEnvelopeVersion)
	}
	if c.capability.MaxPageSize != 100 {
		t.Fatalf("capability.MaxPageSize = %d, want 100", c.capability.MaxPageSize)
	}
	if !containsString(c.capability.SupportedDecisionTypes, "recommendation") {
		t.Fatalf("capability.SupportedDecisionTypes missing 'recommendation': %v", c.capability.SupportedDecisionTypes)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Fatalf("health = %s, want %s", c.Health(context.Background()), connector.HealthHealthy)
	}
}

// TestSync_ClampsPageSizeToCapabilityMax proves that when the QF-advertised
// max_page_size is below the configured page_size, the connector issues
// /decision-events requests with limit clamped to capability.MaxPageSize.
func TestSync_ClampsPageSizeToCapabilityMax(t *testing.T) {
	var observedLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case CapabilitiesPath:
			cap := defaultValidCapability()
			cap.MaxPageSize = 50 // Cap below configured page_size of 100
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cap)
		case DecisionEventsPath:
			observedLimit = r.URL.Query().Get("limit")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
				Events:     []QFDecisionEvent{},
				NextCursor: "qf-page-1",
				HasMore:    false,
				ServerTime: "2026-05-06T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       srv.URL,
			"packet_version": 1,
			// 100 is the maximum parseConfig accepts; capability.MaxPageSize=50
			// must clamp it further before Sync issues the FetchDecisionEvents call.
			"page_size": 100,
		},
	}
	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if _, _, err := c.Sync(context.Background(), ""); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if observedLimit != "50" {
		t.Fatalf("outbound limit = %q, want 50 (clamped from configured 100 to capability.MaxPageSize=50)", observedLimit)
	}
}

func TestSync_PageSizeOutOfRangeMarksDegradedAndAlertsWithoutRetry(t *testing.T) {
	metrics.QFPacketValidationFailures.Reset()

	var eventRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		case DecisionEventsPath:
			eventRequests++
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(BridgeErrorResponse{
				Code:    "PAGE_SIZE_OUT_OF_RANGE",
				Message: "requested page_size is outside the current capability range",
			})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1)); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected page-size out-of-range sync error, got nil")
	}
	var oor PageSizeOutOfRangeError
	if !errors.As(err, &oor) {
		t.Fatalf("expected PageSizeOutOfRangeError, got %T: %v", err, err)
	}
	if eventRequests != 1 {
		t.Fatalf("expected exactly one decision-events request with no retry, got %d", eventRequests)
	}
	if got := c.Health(context.Background()); got != connector.HealthDegraded {
		t.Fatalf("health = %s, want %s", got, connector.HealthDegraded)
	}
	if got := testutil.ToFloat64(metrics.QFPacketValidationFailures.WithLabelValues("page_size_out_of_range")); got != 1 {
		t.Fatalf("smackerel_qf_packet_validation_failures_total{reason=page_size_out_of_range} = %v, want 1", got)
	}
}

// TestSync_EmitsUnknownDecisionTypeMetricForUnsupportedType proves that when
// an event arrives with a decision_type that is NOT in the capability-
// advertised supported_decision_types list, the connector increments the
// smackerel_qf_unknown_decision_type_total counter labelled with the
// offending value.
func TestSync_EmitsUnknownDecisionTypeMetricForUnsupportedType(t *testing.T) {
	// Reset the metric to isolate this test's increment from other tests.
	metrics.QFUnknownDecisionType.Reset()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case CapabilitiesPath:
			// The capability declares the standard 3 supported decision types.
			// CompatibilityCheck requires recommendation/policy_denial/analysis_note
			// to be present, so we keep them. The runtime event below uses a
			// value OUTSIDE this set to trigger the unknown_decision_type counter.
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		case DecisionEventsPath:
			_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
				Events: []QFDecisionEvent{
					{
						ContractVersion: 1,
						EventID:         "event-X",
						PacketID:        "packet-X",
						IntentID:        "intent-X",
						ScenarioID:      "scenario-X",
						TraceID:         "trace-X",
						EventType:       "packet_created",
						DecisionType:    "experimental_decision_type", // NOT in capability supported list
						ApprovalState:   "display_only",
						PacketVersion:   1,
						PacketURL:       "https://qf.example.test/packets/packet-X",
						CreatedAt:       "2026-05-06T00:00:00Z",
					},
				},
				NextCursor: "qf-page-1",
				HasMore:    false,
				ServerTime: "2026-05-06T00:00:00Z",
			})
		case DecisionPacketsPath + "/packet-X":
			_ = json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-X",
				IntentID:             "intent-X",
				ScenarioID:           "scenario-X",
				TraceID:              "trace-X",
				Thesis:               "thesis X",
				WhyNow:               "why-now X",
				QuantifiedImpact:     map[string]any{"unit": "bps"},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-X"},
				CalibrationBadge:     map[string]any{"state": "calibrated"},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-X",
				PacketVersion:        1,
				DecisionType:         "experimental_decision_type",
				CreatedAt:            "2026-05-06T00:00:00Z",
				UpdatedAt:            "2026-05-06T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1)); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	got := testutil.ToFloat64(metrics.QFUnknownDecisionType.WithLabelValues("experimental_decision_type"))
	if got != 1 {
		t.Fatalf("metric counter for experimental_decision_type = %v, want 1", got)
	}

	// design.md §F8 + Round 2L: the unknown decision_type event is no
	// longer rejected. It MUST surface as a real RawArtifact carrying the
	// canonical qf/decision-packet content type AND
	// metadata.unknown_decision_type=true so downstream consumers can
	// route it through the generic packet card variant. The capability
	// gate is no longer the metric source; the normalizer owns both the
	// metric increment and the metadata persistence.
	if len(artifacts) != 1 {
		t.Fatalf("expected exactly 1 RawArtifact for the unknown decision_type packet (design.md §F8 forbids rejection), got %d", len(artifacts))
	}
	art := artifacts[0]
	if art.ContentType != ContentTypeDecisionPacket {
		t.Fatalf("artifact.ContentType = %q, want %q (unknown decision_type MUST fall through to canonical qf/decision-packet)", art.ContentType, ContentTypeDecisionPacket)
	}
	flag, ok := art.Metadata["unknown_decision_type"].(bool)
	if !ok {
		t.Fatalf("artifact.Metadata[unknown_decision_type] = %v (%T), want bool true", art.Metadata["unknown_decision_type"], art.Metadata["unknown_decision_type"])
	}
	if !flag {
		t.Fatal("artifact.Metadata[unknown_decision_type] = false, want true")
	}
	if got, _ := art.Metadata["decision_type"].(string); got != "experimental_decision_type" {
		t.Fatalf("artifact.Metadata[decision_type] = %q, want %q (raw unknown value MUST be preserved)", got, "experimental_decision_type")
	}
}

// TestConnectorEmitsLagBreachEventAboveThreshold (SCN-SM-041-007) proves
// that when the server-reported cursor lag exceeds the configured threshold,
// Sync() emits a structured slog "lag_breach" warning carrying ALL four
// required diagnostic fields AND never auto-advances the cursor.
//
// The DoD invariants enforced by this test:
//
//  1. slog WARN record with msg "qf-decisions: lag_breach"
//  2. event="lag_breach" (allows downstream dashboards to filter)
//  3. cursor_lag_seconds > 0 (the actual lag)
//  4. threshold_seconds equals the configured threshold (so operators see
//     the budget that was exceeded)
//  5. last_event_id identifies the event whose timestamp produced the lag
//  6. connector_id identifies the connector instance
//
// AND the no-auto-fast-forward invariant: the cursor returned by Sync() is
// the response-level next_cursor verbatim — the connector does NOT
// synthesize a new cursor or skip ahead just because the lag exceeded the
// threshold. Auto-recovery is the operator's responsibility via QF's
// POST /api/private/smackerel/v1/cursor:fast-forward endpoint (F13).
func TestConnectorEmitsLagBreachEventAboveThreshold(t *testing.T) {
	// Capture slog output via a JSON handler so the assertions can inspect
	// the full record structure (msg + all attributes). Restore the default
	// logger on test exit so other tests are not affected.
	prevLogger := slog.Default()
	var logBuf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	// Configure: last event @ T-2h, server_time @ T → lag = 7200s.
	// Threshold = 60s. 7200 > 60 → lag_breach MUST fire.
	const (
		lastEventCreatedAt  = "2026-05-06T00:00:00Z"
		serverTime          = "2026-05-06T02:00:00Z"
		expectedLagSeconds  = 7200.0
		thresholdSeconds    = 60
		expectedLastEventID = "event-lag-1"
		expectedNextCursor  = "qf-page-lag-1"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		case DecisionEventsPath:
			_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
				Events: []QFDecisionEvent{
					{
						ContractVersion: 1,
						EventID:         expectedLastEventID,
						PacketID:        "packet-lag-1",
						IntentID:        "intent-lag-1",
						ScenarioID:      "scenario-lag-1",
						TraceID:         "trace-lag-1",
						EventType:       "packet_created",
						DecisionType:    DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						PacketURL:       "https://qf.example.test/packets/packet-lag-1",
						CreatedAt:       lastEventCreatedAt,
					},
				},
				NextCursor: expectedNextCursor,
				HasMore:    false,
				ServerTime: serverTime,
			})
		case DecisionPacketsPath + "/packet-lag-1":
			_ = json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-lag-1",
				IntentID:             "intent-lag-1",
				ScenarioID:           "scenario-lag-1",
				TraceID:              "trace-lag-1",
				Thesis:               "thesis lag-1",
				WhyNow:               "why-now lag-1",
				QuantifiedImpact:     map[string]any{"unit": "bps"},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-lag-1"},
				CalibrationBadge:     map[string]any{"state": "calibrated"},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-lag-1",
				PacketVersion:        1,
				DecisionType:         DecisionTypeRecommendation,
				CreatedAt:            lastEventCreatedAt,
				UpdatedAt:            lastEventCreatedAt,
			})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	// Build a config with cursor_lag_threshold_seconds set explicitly so this
	// test does not depend on the connector's 3600s default.
	cfg := validConnectorConfig(srv.URL, 1)
	cfg.SourceConfig["cursor_lag_threshold_seconds"] = thresholdSeconds

	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	_, cursorAfter, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// --- Invariant 1-6: parse captured slog records and find the lag_breach.
	type slogRecord struct {
		Time             string  `json:"time"`
		Level            string  `json:"level"`
		Msg              string  `json:"msg"`
		Event            string  `json:"event"`
		CursorLagSeconds float64 `json:"cursor_lag_seconds"`
		ThresholdSeconds int     `json:"threshold_seconds"`
		LastEventID      string  `json:"last_event_id"`
		ConnectorID      string  `json:"connector_id"`
	}

	var found *slogRecord
	scanner := bytes.NewReader(logBuf.Bytes())
	dec := json.NewDecoder(scanner)
	for {
		var rec slogRecord
		if err := dec.Decode(&rec); err != nil {
			break
		}
		if rec.Event == "lag_breach" {
			r := rec
			found = &r
			break
		}
	}

	if found == nil {
		t.Fatalf("expected slog lag_breach record in captured output; got:\n%s", logBuf.String())
	}
	if found.Level != "WARN" {
		t.Errorf("level = %q, want WARN", found.Level)
	}
	if !strings.Contains(found.Msg, "lag_breach") {
		t.Errorf("msg = %q, want to contain lag_breach", found.Msg)
	}
	if found.Event != "lag_breach" {
		t.Errorf("event = %q, want lag_breach", found.Event)
	}
	if found.CursorLagSeconds != expectedLagSeconds {
		t.Errorf("cursor_lag_seconds = %v, want %v", found.CursorLagSeconds, expectedLagSeconds)
	}
	if found.ThresholdSeconds != thresholdSeconds {
		t.Errorf("threshold_seconds = %d, want %d", found.ThresholdSeconds, thresholdSeconds)
	}
	if found.LastEventID != expectedLastEventID {
		t.Errorf("last_event_id = %q, want %q", found.LastEventID, expectedLastEventID)
	}
	if found.ConnectorID != DefaultConnectorID {
		t.Errorf("connector_id = %q, want %q", found.ConnectorID, DefaultConnectorID)
	}

	// --- No-auto-fast-forward invariant: the cursor returned by Sync() MUST
	// be the response-level next_cursor verbatim. The connector MUST NOT
	// synthesize a new cursor or skip ahead based on lag alone.
	if cursorAfter != expectedNextCursor {
		t.Errorf("cursor after Sync = %q, want %q (response-level next_cursor verbatim — no auto-fast-forward)", cursorAfter, expectedNextCursor)
	}

	// --- Defensive: the lag gauge must have been published at the observed
	// value. The gauge is the operator-visible surface; the slog record is
	// the audit trail.
	gotGauge := testutil.ToFloat64(metrics.QFCursorLagSeconds)
	if gotGauge != expectedLagSeconds {
		t.Errorf("smackerel_qf_cursor_lag_seconds = %v, want %v", gotGauge, expectedLagSeconds)
	}
}

// syncFreshnessTestServer returns an httptest.Server that emits one valid
// QFDecisionEvent and matching envelope with the supplied createdAt
// timestamp. It is the minimum fake needed to drive Sync() through to
// `artifacts = append(...)` so the freshness ingest observation runs.
// SCN-SM-041-003.
func syncFreshnessTestServer(t *testing.T, createdAt string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		case DecisionEventsPath:
			_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
				Events: []QFDecisionEvent{
					{
						ContractVersion: 1,
						EventID:         "event-F",
						PacketID:        "packet-F",
						IntentID:        "intent-F",
						ScenarioID:      "scenario-F",
						TraceID:         "trace-F",
						EventType:       "packet_created",
						DecisionType:    DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						PacketURL:       "https://qf.example.test/packets/packet-F",
						CreatedAt:       createdAt,
					},
				},
				NextCursor: "qf-page-1",
				HasMore:    false,
				ServerTime: createdAt,
			})
		case DecisionPacketsPath + "/packet-F":
			_ = json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             "packet-F",
				IntentID:             "intent-F",
				ScenarioID:           "scenario-F",
				TraceID:              "trace-F",
				Thesis:               "thesis F",
				WhyNow:               "why-now F",
				QuantifiedImpact:     map[string]any{"unit": "bps"},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-F"},
				CalibrationBadge:     map[string]any{"state": "calibrated"},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/packet-F",
				PacketVersion:        1,
				DecisionType:         DecisionTypeRecommendation,
				CreatedAt:            createdAt,
				UpdatedAt:            createdAt,
			})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
}

// TestSyncRecordsIngestFreshness_FreshPacket verifies that a packet emitted
// "just now" by QF produces a small ingest-stage freshness p95 (well under
// the design.md §F12 budget of 30s). This proves that the connector wires
// `time.Since(event.CreatedAt)` through the rolling window to the gauge.
// SCN-SM-041-003.
func TestSyncRecordsIngestFreshness_FreshPacket(t *testing.T) {
	metrics.QFFreshnessP95Seconds.Reset()

	// Emit time is "now" so the measured latency is dominated by test overhead
	// (single-digit milliseconds), well below the 30s ingest SLA.
	createdAt := time.Now().UTC().Format(time.RFC3339)
	srv := syncFreshnessTestServer(t, createdAt)
	defer srv.Close()

	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1)); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if _, _, err := c.Sync(context.Background(), ""); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	got := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageIngest))
	if got < 0 {
		t.Fatalf("ingest p95 = %v, must be non-negative (clock-skew clamp)", got)
	}
	// RFC3339 truncates to whole seconds, so the parsed timestamp can be up
	// to ~1s earlier than the original wall clock. 5s gives ample headroom
	// without losing the assertion that a fresh packet stays well under the
	// 30s SLA.
	if got >= 5 {
		t.Fatalf("ingest p95 for fresh packet = %v, want < 5s (well below 30s SLA)", got)
	}
}

// TestSyncRecordsIngestFreshness_DelayedPacket verifies that a packet whose
// QF emit timestamp is 25s in the past produces an ingest-stage p95 near
// that age, approaching but not exceeding the 30s SLA budget. This proves
// the observation captures real latency, not a synthetic constant.
// SCN-SM-041-003.
func TestSyncRecordsIngestFreshness_DelayedPacket(t *testing.T) {
	metrics.QFFreshnessP95Seconds.Reset()

	delay := 25 * time.Second
	createdAt := time.Now().UTC().Add(-delay).Format(time.RFC3339)
	srv := syncFreshnessTestServer(t, createdAt)
	defer srv.Close()

	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1)); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if _, _, err := c.Sync(context.Background(), ""); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	got := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageIngest))
	// RFC3339 second-truncation can shave up to ~1s from the parsed emit
	// time, so allow 24s as the lower bound. Upper bound is generous (60s)
	// to absorb test scheduler jitter while still proving the observation
	// is correlated with the actual age of the packet.
	if got < 24 {
		t.Fatalf("ingest p95 for 25s-delayed packet = %v, want ≥ 24s", got)
	}
	if got > 60 {
		t.Fatalf("ingest p95 for 25s-delayed packet = %v, want ≤ 60s (test jitter ceiling)", got)
	}
}

// TestRecordFreshness_PerStageIsolation verifies that observations recorded
// against one stage do not bleed into another stage's gauge. design.md §F12
// requires independent SLA budgets for ingest (30s), render (30s), and
// total (60s); cross-stage contamination would silently mask breaches.
// This test also exercises the negative-clamp behavior for clock skew.
// SCN-SM-041-003, SCN-SM-041-008.
func TestRecordFreshness_PerStageIsolation(t *testing.T) {
	metrics.QFFreshnessP95Seconds.Reset()
	c := New(DefaultConnectorID)

	c.recordFreshness(FreshnessStageIngest, 5.0)
	c.recordFreshness(FreshnessStageRender, 12.5)
	c.recordFreshness(FreshnessStageTotal, 47.0)

	ingest := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageIngest))
	render := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageRender))
	total := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageTotal))

	// Single sample → nearest-rank p95 = that sample (ceil(0.95*1)-1 = 0).
	if ingest != 5.0 {
		t.Fatalf("ingest p95 = %v, want 5.0 (single sample)", ingest)
	}
	if render != 12.5 {
		t.Fatalf("render p95 = %v, want 12.5 (single sample)", render)
	}
	if total != 47.0 {
		t.Fatalf("total p95 = %v, want 47.0 (single sample)", total)
	}

	// Adding a second sample to render only must not move ingest or total.
	c.recordFreshness(FreshnessStageRender, 18.0)
	if got := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageIngest)); got != 5.0 {
		t.Fatalf("ingest p95 after render update = %v, want 5.0 (no cross-stage bleed)", got)
	}
	if got := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageTotal)); got != 47.0 {
		t.Fatalf("total p95 after render update = %v, want 47.0 (no cross-stage bleed)", got)
	}
	// With 2 samples, nearest-rank p95 = sample at index ceil(0.95*2)-1 = 1
	// (the larger sample after sort).
	if got := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageRender)); got != 18.0 {
		t.Fatalf("render p95 after 2 samples = %v, want 18.0 (max of {12.5, 18.0})", got)
	}

	// Negative observations (clock skew) clamp to 0 — they must not
	// pull the p95 below zero.
	c2 := New(DefaultConnectorID)
	metrics.QFFreshnessP95Seconds.Reset()
	c2.recordFreshness(FreshnessStageIngest, -3.0)
	if got := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageIngest)); got != 0 {
		t.Fatalf("ingest p95 after negative sample = %v, want 0 (clock-skew clamp)", got)
	}
}

// TestSyncSkipsFastForwardDiagnosticEventAndIncrementsCounter (SCN-SM-041-008,
// Round 2Q unit-layer cover) proves the positive fast-forward recovery path
// in `internal/connector/qfdecisions/connector.go:281-296,387-388`.
//
// Background: When an operator invokes QF's
// `POST /api/private/smackerel/v1/cursor:fast-forward`, QF advances its
// internal cursor and emits a single diagnostic event carrying
// `events_skipped > 0`. The connector MUST treat that diagnostic event as a
// recovery marker — it MUST NOT normalize the marker into a RawArtifact, MUST
// increment `metrics.QFCursorFastForwardEventsSkipped` by `events_skipped`,
// MUST emit a structured `fast_forward_recovered` slog warning, and (when no
// other event in the same Sync was degraded) MUST transition health to
// `HealthDegradedRecovered`.
//
// The DoD-named integration test
// `tests/integration/qf_decisions_sync_test.go::TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped`
// remains absent (Round 2P classification B) and is blocked at the live-stack
// layer by spec-045. This unit test closes the in-process gap by exercising
// the production fast-forward branch end-to-end through `Sync()`.
//
// Adversarial design: the diagnostic event uses a unique `packet_id` and the
// fake QF server records every fetch against that path. If the production
// `if event.EventsSkipped > 0 { ... continue }` block is removed (or the
// inequality flipped), the connector will issue a packet-envelope fetch for
// the diagnostic event's packet_id and the test will fail on `ffPacketFetches
// != 0`. The counter delta assertion (`+skippedCount` exactly) catches a
// regression that drops the `metrics...Add(...)` call. The
// `HealthDegradedRecovered` assertion catches a regression that drops the
// `fastForwardObserved = true` toggle or the precedence rule at line 387.
func TestSyncSkipsFastForwardDiagnosticEventAndIncrementsCounter(t *testing.T) {
	// Capture slog as JSON so we can assert the structured fields on the
	// `fast_forward_recovered` record.
	prevLogger := slog.Default()
	var logBuf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	// Counter is package-global and prometheus.Counter does not support
	// reset; use baseline + delta to isolate this test's increment.
	baseline := testutil.ToFloat64(metrics.QFCursorFastForwardEventsSkipped)

	const (
		skippedCount     = 42
		ffEventID        = "event-ff-marker-1"
		ffPacketID       = "packet-FF-MARKER-MUST-NOT-BE-FETCHED"
		normalEventID    = "event-normal-1"
		normalPacketID   = "packet-normal-1"
		stableTimestamp  = "2026-05-06T00:00:00Z"
		nextCursorWanted = "qf-page-after-ff-marker"
	)

	// ffPacketFetches is the adversarial trip-wire. Production code MUST
	// `continue` past the FF diagnostic event before any packet-envelope
	// fetch, so the count MUST stay at zero.
	var ffPacketFetches int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(defaultValidCapability())
		case DecisionEventsPath:
			_ = json.NewEncoder(w).Encode(DecisionEventsResponse{
				Events: []QFDecisionEvent{
					{
						// Fast-forward diagnostic event — EventsSkipped > 0.
						// The connector MUST treat this as a recovery
						// marker, increment the counter, and skip
						// normalization (no packet fetch, no artifact).
						ContractVersion: 1,
						EventID:         ffEventID,
						PacketID:        ffPacketID,
						IntentID:        "intent-ff-marker",
						ScenarioID:      "scenario-ff-marker",
						TraceID:         "trace-ff-marker",
						EventType:       "packet_created",
						DecisionType:    DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						PacketURL:       "https://qf.example.test/packets/" + ffPacketID,
						CreatedAt:       stableTimestamp,
						EventsSkipped:   skippedCount,
					},
					{
						// Normal event in the same response — MUST be
						// normalized into exactly one artifact. Verifies the
						// `continue` past the FF marker does not abort the
						// loop (i.e., recovery is genuinely gated on the
						// per-event field, not on response-level state).
						ContractVersion: 1,
						EventID:         normalEventID,
						PacketID:        normalPacketID,
						IntentID:        "intent-normal-1",
						ScenarioID:      "scenario-normal-1",
						TraceID:         "trace-normal-1",
						EventType:       "packet_created",
						DecisionType:    DecisionTypeRecommendation,
						ApprovalState:   "display_only",
						PacketVersion:   1,
						PacketURL:       "https://qf.example.test/packets/" + normalPacketID,
						CreatedAt:       stableTimestamp,
					},
				},
				NextCursor: nextCursorWanted,
				HasMore:    false,
				ServerTime: stableTimestamp,
			})
		case DecisionPacketsPath + "/" + ffPacketID:
			// ADVERSARIAL TRIP-WIRE: production MUST NOT request this
			// packet because the FF marker event must be skipped before
			// any FetchDecisionPacket call. Counting reaches the assert
			// below. We still respond with a valid envelope so the
			// failure surfaces as a clean assertion failure rather than
			// an HTTP error masking the real cause.
			ffPacketFetches++
			_ = json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             ffPacketID,
				IntentID:             "intent-ff-marker",
				ScenarioID:           "scenario-ff-marker",
				TraceID:              "trace-ff-marker",
				Thesis:               "should-not-be-requested",
				WhyNow:               "should-not-be-requested",
				QuantifiedImpact:     map[string]any{"unit": "bps"},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-ff"},
				CalibrationBadge:     map[string]any{"state": "calibrated"},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/" + ffPacketID,
				PacketVersion:        1,
				DecisionType:         DecisionTypeRecommendation,
				CreatedAt:            stableTimestamp,
				UpdatedAt:            stableTimestamp,
			})
		case DecisionPacketsPath + "/" + normalPacketID:
			_ = json.NewEncoder(w).Encode(QFDecisionPacketEnvelope{
				ContractVersion:      1,
				PacketID:             normalPacketID,
				IntentID:             "intent-normal-1",
				ScenarioID:           "scenario-normal-1",
				TraceID:              "trace-normal-1",
				Thesis:               "thesis-normal-1",
				WhyNow:               "why-now-normal-1",
				QuantifiedImpact:     map[string]any{"unit": "bps"},
				ExpertAnalysisBundle: map[string]any{"ref": "qf-normal-1"},
				CalibrationBadge:     map[string]any{"state": "calibrated"},
				DataProvenanceBadge:  map[string]any{"source": "qf-owned"},
				ApprovalState:        "display_only",
				DeepLink:             "https://qf.example.test/packets/" + normalPacketID,
				PacketVersion:        1,
				DecisionType:         DecisionTypeRecommendation,
				CreatedAt:            stableTimestamp,
				UpdatedAt:            stableTimestamp,
			})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(DefaultConnectorID)
	if err := c.Connect(context.Background(), validConnectorConfig(srv.URL, 1)); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	artifacts, cursorAfter, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// --- Adversarial trip-wire: FF marker packet MUST NOT be fetched.
	// If the `if event.EventsSkipped > 0 { ... continue }` block is removed
	// or the inequality flipped, the connector will issue a packet fetch
	// for ffPacketID and this assertion will fail.
	if ffPacketFetches != 0 {
		t.Fatalf("connector fetched the fast-forward diagnostic packet %d time(s); production MUST `continue` past EventsSkipped>0 events before any packet-envelope fetch", ffPacketFetches)
	}

	// --- The FF event MUST NOT appear in artifacts; the normal event MUST.
	if len(artifacts) != 1 {
		t.Fatalf("artifacts length = %d, want 1 (FF marker must be skipped, normal event must be ingested); artifacts=%+v", len(artifacts), artifacts)
	}
	if got := artifacts[0].SourceRef; got != normalPacketID {
		t.Fatalf("artifacts[0].SourceRef = %q, want %q (FF packet ID must NEVER appear as a published artifact)", got, normalPacketID)
	}

	// --- Cursor MUST advance to the response-level next_cursor verbatim.
	// The connector must not auto-synthesize a new cursor based on the FF
	// marker (cursor:fast-forward is QF-side; the connector just consumes
	// the diagnostic).
	if cursorAfter != nextCursorWanted {
		t.Errorf("cursorAfter = %q, want %q (response.next_cursor verbatim)", cursorAfter, nextCursorWanted)
	}

	// --- Counter MUST have incremented by exactly skippedCount.
	// Catches a regression that removes the `metrics...Add(...)` call.
	delta := testutil.ToFloat64(metrics.QFCursorFastForwardEventsSkipped) - baseline
	if delta != float64(skippedCount) {
		t.Errorf("smackerel_qf_cursor_fast_forward_events_skipped_total delta = %v, want %v", delta, float64(skippedCount))
	}

	// --- Health MUST be degraded_recovered (degraded==0 + fastForwardObserved
	// → connector.HealthDegradedRecovered per the precedence rule at
	// connector.go:380-388).
	if got := c.Health(context.Background()); got != connector.HealthDegradedRecovered {
		t.Errorf("Health = %q, want %q (degraded==0 + fastForwardObserved → degraded_recovered)", got, connector.HealthDegradedRecovered)
	}

	// --- slog MUST contain a fast_forward_recovered WARN record carrying
	// events_skipped, event_id, and connector_id verbatim.
	type slogRecord struct {
		Time          string `json:"time"`
		Level         string `json:"level"`
		Msg           string `json:"msg"`
		Event         string `json:"event"`
		EventsSkipped int    `json:"events_skipped"`
		EventID       string `json:"event_id"`
		ConnectorID   string `json:"connector_id"`
	}

	var found *slogRecord
	dec := json.NewDecoder(bytes.NewReader(logBuf.Bytes()))
	for {
		var rec slogRecord
		if err := dec.Decode(&rec); err != nil {
			break
		}
		if rec.Event == "fast_forward_recovered" {
			r := rec
			found = &r
			break
		}
	}

	if found == nil {
		t.Fatalf("expected slog fast_forward_recovered record; captured log:\n%s", logBuf.String())
	}
	if found.Level != "WARN" {
		t.Errorf("slog level = %q, want WARN", found.Level)
	}
	if !strings.Contains(found.Msg, "fast_forward_recovered") {
		t.Errorf("slog msg = %q, want to contain fast_forward_recovered", found.Msg)
	}
	if found.EventsSkipped != skippedCount {
		t.Errorf("slog events_skipped = %d, want %d", found.EventsSkipped, skippedCount)
	}
	if found.EventID != ffEventID {
		t.Errorf("slog event_id = %q, want %q", found.EventID, ffEventID)
	}
	if found.ConnectorID != DefaultConnectorID {
		t.Errorf("slog connector_id = %q, want %q", found.ConnectorID, DefaultConnectorID)
	}
}
