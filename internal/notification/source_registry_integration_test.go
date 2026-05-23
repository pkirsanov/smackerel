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

func TestSourceRegistryPersistsHealthForSimultaneousInstances(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	store := NewStore(pool)
	now := time.Date(2026, 5, 22, 6, 20, 0, 0, time.UTC)
	prefix := "scope1-int-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM notification_source_instances WHERE source_instance_id LIKE $1", prefix+"%")
	})

	connectedEnabled := true
	degradedEnabled := true
	instances := []SourceInstanceConfig{
		{SourceType: "stream_fixture", SourceInstanceID: prefix + "-stream", SourceForm: SourceFormStream, Enabled: &connectedEnabled, ConfigHash: "sha256:stream", SecretRefNames: []string{"STREAM_TOKEN_REF"}, RedactedMetadata: map[string]string{"endpoint": "redacted"}},
		{SourceType: "polling_fixture", SourceInstanceID: prefix + "-polling", SourceForm: SourceFormPolling, Enabled: &degradedEnabled, ConfigHash: "sha256:polling", SecretRefNames: []string{"POLL_TOKEN_REF"}, RedactedMetadata: map[string]string{"window": "redacted"}},
	}
	for _, instance := range instances {
		if _, err := store.CreateSourceInstance(ctx, instance, now); err != nil {
			t.Fatalf("create source instance %s: %v", instance.SourceInstanceID, err)
		}
	}
	lastEvent := now.Add(-time.Minute)
	lastCheck := now.Add(-30 * time.Second)
	if err := store.RecordSourceHealth(ctx, SourceHealthReport{SourceType: "stream_fixture", SourceInstanceID: prefix + "-stream", SourceForm: SourceFormStream, State: SourceHealthConnected, LastEventAt: &lastEvent, LastSuccessfulCheckAt: &lastCheck, ObservedAt: now}); err != nil {
		t.Fatalf("record connected health: %v", err)
	}
	if err := store.RecordSourceHealth(ctx, SourceHealthReport{SourceType: "polling_fixture", SourceInstanceID: prefix + "-polling", SourceForm: SourceFormPolling, State: SourceHealthDegraded, LastEventAt: &lastEvent, RetryCount: 3, LastErrorKind: "transient_failure", LastErrorRedacted: "password=secret", ObservedAt: now}); err != nil {
		t.Fatalf("record degraded health: %v", err)
	}

	statuses, err := store.ListSourceStatuses(ctx)
	if err != nil {
		t.Fatalf("list source statuses: %v", err)
	}
	got := map[string]SourceStatus{}
	for _, status := range statuses {
		if strings.HasPrefix(status.Config.SourceInstanceID, prefix) {
			got[status.Config.SourceInstanceID] = status
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 persisted statuses for prefix %s, got %d", prefix, len(got))
	}
	if got[prefix+"-stream"].Health.State != SourceHealthConnected || got[prefix+"-stream"].Health.LastEventAt == nil {
		t.Fatalf("connected status not persisted with event time: %+v", got[prefix+"-stream"])
	}
	degraded := got[prefix+"-polling"].Health
	if degraded.State != SourceHealthDegraded || degraded.RetryCount != 3 || degraded.LastErrorRedacted != "transient source check failed" {
		t.Fatalf("degraded status not persisted redacted: %+v", degraded)
	}
}
