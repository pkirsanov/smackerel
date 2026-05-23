//go:build integration

package notification

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
)

func notificationIntegrationStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration: DATABASE_URL not set")
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
	return NewStore(pool), pool
}

func notificationIntegrationPrefix(t *testing.T) string {
	t.Helper()
	return "notif-int-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
}

func seedNotificationIntegrationSource(t *testing.T, store *Store, prefix string) SourceInstanceConfig {
	t.Helper()
	enabled := true
	cfg := SourceInstanceConfig{SourceType: "manual_fixture", SourceInstanceID: prefix + "-source", SourceForm: SourceFormManual, Enabled: &enabled, ConfigHash: "sha256:" + prefix, SecretRefNames: []string{"MANUAL_INGEST_AUTH_CONTEXT"}, RedactedMetadata: map[string]string{"actor": "redacted"}}
	if err := store.EnsureSourceInstance(context.Background(), cfg, time.Now().UTC()); err != nil {
		t.Fatalf("ensure source: %v", err)
	}
	return cfg
}

func notificationIntegrationEnvelope(cfg SourceInstanceConfig, id string, body string, severity string) SourceEventEnvelope {
	return SourceEventEnvelope{SourceType: cfg.SourceType, SourceInstanceID: cfg.SourceInstanceID, SourceForm: cfg.SourceForm, SourceEventID: id, ObservedAt: time.Now().UTC(), RawPayloadKind: RawPayloadKindText, RawPayload: []byte(body), DeliveryMetadata: map[string]string{"actor": "integration"}, SourceSpecificFields: map[string]string{"severity": severity}, MappingHints: map[string]string{"title": body, "body": body, "severity": severity, "subject": "checkout-api", "service": "checkout-api", "domain": "ops", "intent": "investigate"}}
}

func notificationIntegrationService(t *testing.T, store *Store) *Service {
	t.Helper()
	engine, err := NewDecisionEngine(DecisionPolicy{PersistenceThreshold: 2, EscalationSeverity: SeverityHigh, LowConfidenceThreshold: 0.55, OutputChannels: []string{"dashboard"}, MaxRetries: 2})
	if err != nil {
		t.Fatalf("decision engine: %v", err)
	}
	return NewService(store, engine)
}
