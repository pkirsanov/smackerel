//go:build integration

package ntfy

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/notification"
)

func ntfyIntegrationStores(t *testing.T) (*Store, *notification.Store, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("integration: DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	return NewStore(pool), notification.NewStore(pool), pool
}

func ntfyIntegrationPrefix() string {
	return "ntfy-int-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
}

func ntfyIntegrationConfig(prefix string, sourceForm notification.SourceForm, topics []string) Config {
	cfg := testConfig()
	cfg.SourceInstanceID = prefix + "-source"
	cfg.SourceForm = sourceForm
	cfg.TransportMode = string(sourceForm)
	cfg.EndpointURL = "https://ntfy.integration.invalid"
	cfg.EndpointRefName = "NTFY_INTEGRATION_ENDPOINT_URL"
	cfg.Topics = append([]string(nil), topics...)
	cfg.Auth = AuthConfig{Mode: AuthModeNone}
	cfg.RedactedMetadata = map[string]string{"display_name": "ntfy integration source", "endpoint_label": "integration ntfy endpoint"}
	cfg.ConfigHash = "sha256:" + cfg.SourceInstanceID
	return cfg
}

func seedNtfyIntegrationSource(t *testing.T, store *notification.Store, cfg Config) {
	t.Helper()
	instance, err := cfg.SourceInstanceConfig()
	if err != nil {
		t.Fatalf("source instance config: %v", err)
	}
	if err := store.EnsureSourceInstance(context.Background(), instance, time.Now().UTC()); err != nil {
		t.Fatalf("ensure ntfy source: %v", err)
	}
}

func ntfyIntegrationService(t *testing.T, store *notification.Store) *notification.Service {
	t.Helper()
	engine, err := notification.NewDecisionEngine(notification.DecisionPolicy{PersistenceThreshold: 2, EscalationSeverity: notification.SeverityHigh, LowConfidenceThreshold: 0.55, OutputChannels: []string{"dashboard"}, MaxRetries: 2})
	if err != nil {
		t.Fatalf("decision engine: %v", err)
	}
	return notification.NewService(store, engine)
}
