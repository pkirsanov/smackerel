package qfdecisions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
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
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		if r.URL.Path != DecisionEventsPath {
			t.Fatalf("request path = %q, want %q", r.URL.Path, DecisionEventsPath)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DecisionEventsResponse{
			Events:     []QFDecisionEvent{},
			NextCursor: "qf-smackerel-v1:0",
			HasMore:    false,
			ServerTime: "2026-05-06T00:00:00Z",
		})
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
	if !strings.Contains(gotQuery, "packet_version=1") || !strings.Contains(gotQuery, "limit=25") {
		t.Fatalf("validation query did not include packet_version and limit: %s", gotQuery)
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
