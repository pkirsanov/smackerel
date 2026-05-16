//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

func TestQFDecisionsConnectorConfigRegistryAndHealthIntegration(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer qf-service-token" {
			t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case qfdecisions.CapabilitiesPath:
			// Connector now performs a capability handshake before any decision-events
			// call. Serve a valid capability so CompatibilityCheck() passes and the
			// connector proceeds to the existing decision-events flow.
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
		case qfdecisions.DecisionEventsPath:
			if r.URL.Query().Get("packet_version") != "1" || r.URL.Query().Get("limit") != "25" {
				t.Errorf("query = %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(qfdecisions.DecisionEventsResponse{
				Events:     []qfdecisions.QFDecisionEvent{},
				NextCursor: "qf-smackerel-v1:0",
				HasMore:    false,
				ServerTime: "2026-05-06T00:00:00Z",
			})
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	registry := connector.NewRegistry()
	qfConnector := qfdecisions.New(qfdecisions.DefaultConnectorID)
	if err := registry.Register(qfConnector); err != nil {
		t.Fatalf("register qf connector: %v", err)
	}
	if _, ok := registry.Get(qfdecisions.DefaultConnectorID); !ok {
		t.Fatal("qf-decisions connector missing from registry")
	}

	if err := qfConnector.Connect(ctx, qfIntegrationConfig(server.URL, 1)); err != nil {
		t.Fatalf("connect qf connector: %v", err)
	}
	if got := qfConnector.Health(ctx); got != connector.HealthHealthy {
		t.Fatalf("health = %s, want %s", got, connector.HealthHealthy)
	}

	artifacts := 0
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM artifacts WHERE source_id = $1`, qfdecisions.DefaultConnectorID).Scan(&artifacts); err != nil {
		t.Fatalf("count qf artifacts: %v", err)
	}
	if artifacts != 0 {
		t.Fatalf("Scope 1 must not publish qf artifacts; found %d", artifacts)
	}
}

func TestQFDecisionsConnectorSchemaMismatchIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(qfdecisions.BridgeErrorResponse{Code: "invalid_query_parameter", Message: "packet_version 99 is unsupported"})
	}))
	defer server.Close()

	qfConnector := qfdecisions.New(qfdecisions.DefaultConnectorID)
	err := qfConnector.Connect(context.Background(), qfIntegrationConfig(server.URL, 99))
	if err == nil {
		t.Fatal("expected schema mismatch")
	}
	if got := qfConnector.Health(context.Background()); got != connector.HealthDegraded {
		t.Fatalf("health = %s, want %s", got, connector.HealthDegraded)
	}
}

func TestQFDecisionsConnectorAuthFailureIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(qfdecisions.BridgeErrorResponse{Code: "invalid_token", Message: "credential is not accepted"})
	}))
	defer server.Close()

	qfConnector := qfdecisions.New(qfdecisions.DefaultConnectorID)
	err := qfConnector.Connect(context.Background(), qfIntegrationConfig(server.URL, 1))
	if err == nil {
		t.Fatal("expected auth failure")
	}
	var authErr qfdecisions.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %v, want AuthError", err)
	}
	if got := qfConnector.Health(context.Background()); got != connector.HealthError {
		t.Fatalf("health = %s, want %s", got, connector.HealthError)
	}
}

func qfIntegrationConfig(baseURL string, packetVersion int) connector.ConnectorConfig {
	return connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       baseURL,
			"packet_version": packetVersion,
			"page_size":      25,
		},
	}
}

// validQFIntegrationCapability returns a QFBridgeCapability that satisfies
// CompatibilityCheck() invariants required by the connector handshake.
// Mirrors internal/connector/qfdecisions/capability_test.go::validCapability().
// Kept here (not in helpers_test.go) because it is a QF-specific fixture.
func validQFIntegrationCapability() qfdecisions.QFBridgeCapability {
	return qfdecisions.QFBridgeCapability{
		SupportedPacketVersions:            []string{"v1"},
		SupportedEventTypes:                []string{"created", "updated", "badge_changed", "approval_state_changed", "archived", "superseded"},
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