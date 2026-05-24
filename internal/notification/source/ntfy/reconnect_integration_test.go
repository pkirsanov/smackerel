//go:build integration

package ntfy

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyReconnectLagAndGapPersistInTopicState(t *testing.T) {
	ntfyStore, notificationStore, _ := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	cfg := ntfyIntegrationConfig(prefix, notification.SourceFormStream, []string{"home-lab-alerts"})
	seedNtfyIntegrationSource(t, notificationStore, cfg)
	now := time.Date(2026, 5, 24, 23, 20, 0, 0, time.UTC)
	lastCheck := now.Add(-2 * time.Minute)
	state := FinalizeSubscriptionState(cfg, SubscriptionState{SourceInstanceID: cfg.SourceInstanceID, Topic: "home-lab-alerts", SourceForm: cfg.SourceForm, TransportMode: cfg.TransportMode, SubscriptionState: SubscriptionConnected, LastSuccessfulCheckAt: &lastCheck, RetryBudget: cfg.Reconnect.RetryBudget, RedactionState: emptyRedactionState(), CreatedAt: lastCheck, UpdatedAt: now}, now)
	if err := ntfyStore.UpsertSubscriptionState(context.Background(), state); err != nil {
		t.Fatalf("upsert stale state: %v", err)
	}
	states, err := ntfyStore.ListSubscriptionStates(context.Background(), cfg.SourceInstanceID)
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	if len(states) != 1 || states[0].SubscriptionState != SubscriptionStalled || states[0].LagSeconds < cfg.Lag.DegradedAfterSeconds || !states[0].PossibleGap {
		t.Fatalf("lag state not persisted with degraded possible-gap detail: %+v", states)
	}
	report := HealthFromTopics(cfg, states, now)
	if report.State != notification.SourceHealthDegraded || report.LastErrorKind != ErrorConnectivityFailed {
		t.Fatalf("lag health report mismatch: %+v", report)
	}
}
