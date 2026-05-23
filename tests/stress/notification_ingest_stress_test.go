//go:build stress

package stress

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNotificationIngestSustainsBurstWithoutRawRecordLoss(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	pool := notificationStressPool(t)
	prefix := notificationStressPrefix()
	sourceID := prefix + "-ingest"
	const burstSize = 20

	var wg sync.WaitGroup
	for index := 0; index < burstSize; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			notificationStressIngest(t, cfg, notificationStressPayload(prefix, sourceID, index, "low", "routine"))
		}()
	}
	wg.Wait()

	rawCount := notificationStressCount(t, pool, "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = $1", sourceID)
	normalizedCount := notificationStressCount(t, pool, "SELECT COUNT(*) FROM normalized_notifications WHERE source_instance_id = $1", sourceID)
	if rawCount != burstSize || normalizedCount != burstSize {
		t.Fatalf("notification burst lost raw or normalized records: raw=%d normalized=%d want=%d", rawCount, normalizedCount, burstSize)
	}
	if missing := notificationStressCount(t, pool, "SELECT COUNT(*) FROM notification_raw_events raw WHERE raw.source_instance_id = $1 AND NOT EXISTS (SELECT 1 FROM normalized_notifications nn WHERE nn.raw_event_id = raw.id)", sourceID); missing != 0 {
		t.Fatalf("notification burst left raw records without normalized audit path: missing=%d", missing)
	}
	if _, err := pool.Exec(context.Background(), "SELECT 1"); err != nil {
		t.Fatalf("notification stress database connection became unhealthy after burst: %v", err)
	}
}