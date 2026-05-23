//go:build stress

package stress

import (
	"testing"
	"time"
)

func TestCorrelationHandlesDuplicateBurstAsOneIncident(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	prefix := notificationStressPrefix()
	sourceID := prefix + "-correlation"
	const burstSize = 12
	first := notificationStressIngest(t, cfg, notificationStressPayload(prefix, sourceID, 0, "high", "outage"))
	for index := 1; index < burstSize; index++ {
		created := notificationStressIngest(t, cfg, notificationStressPayload(prefix, sourceID, index, "high", "outage"))
		if created.IncidentID != first.IncidentID {
			t.Fatalf("duplicate burst split into multiple incidents: first=%s next=%s", first.IncidentID, created.IncidentID)
		}
	}
	pool := notificationStressPool(t)
	persistence := notificationStressCount(t, pool, "SELECT persistence_count FROM notification_incidents WHERE id = $1", first.IncidentID)
	if persistence < burstSize {
		t.Fatalf("correlated incident did not preserve burst persistence: got=%d want-at-least=%d", persistence, burstSize)
	}
}
