package ntfy

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyHealthTransitionsUseRealChecksAndRetryBudget(t *testing.T) {
	cfg := testConfig()
	now := time.Date(2026, 5, 24, 22, 15, 0, 0, time.UTC)
	recent := now.Add(-30 * time.Second)
	connected := HealthFromTopics(cfg, []SubscriptionState{{SourceInstanceID: cfg.SourceInstanceID, Topic: "self-hosted-alerts", SourceForm: cfg.SourceForm, TransportMode: cfg.TransportMode, SubscriptionState: SubscriptionConnected, LastEventAt: &recent, LastSuccessfulCheckAt: &recent, RetryBudget: cfg.Reconnect.RetryBudget, CreatedAt: recent, UpdatedAt: recent}}, now)
	if connected.State != notification.SourceHealthConnected || connected.LastErrorKind != "" {
		t.Fatalf("recent source event should produce connected health: %+v", connected)
	}

	stale := now.Add(-2 * time.Minute)
	degraded := HealthFromTopics(cfg, []SubscriptionState{{SourceInstanceID: cfg.SourceInstanceID, Topic: "self-hosted-alerts", SourceForm: cfg.SourceForm, TransportMode: cfg.TransportMode, SubscriptionState: SubscriptionConnected, LastSuccessfulCheckAt: &stale, RetryBudget: cfg.Reconnect.RetryBudget, CreatedAt: stale, UpdatedAt: stale}}, now)
	if degraded.State != notification.SourceHealthDegraded || degraded.LastErrorKind != ErrorConnectivityFailed {
		t.Fatalf("stale source check should produce degraded health: %+v", degraded)
	}

	exhausted := HealthFromTopics(cfg, []SubscriptionState{{SourceInstanceID: cfg.SourceInstanceID, Topic: "self-hosted-alerts", SourceForm: cfg.SourceForm, TransportMode: cfg.TransportMode, SubscriptionState: SubscriptionDisconnected, RetryCount: cfg.Reconnect.RetryBudget, RetryBudget: cfg.Reconnect.RetryBudget, LastErrorKind: ErrorRetryBudgetExhausted, CreatedAt: now, UpdatedAt: now}}, now)
	if exhausted.State != notification.SourceHealthDisconnected || exhausted.RetryCount != cfg.Reconnect.RetryBudget || exhausted.LastErrorRedacted == "" {
		t.Fatalf("retry exhaustion should produce disconnected redacted health: %+v", exhausted)
	}
}
