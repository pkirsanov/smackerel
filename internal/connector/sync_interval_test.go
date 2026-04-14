package connector

import (
	"testing"
	"time"
)

// SCN-023-09: parseSyncInterval handles cron minutes pattern.
func TestParseSyncInterval_CronMinutes(t *testing.T) {
	d := parseSyncInterval("*/30 * * * *")
	if d != 30*time.Minute {
		t.Errorf("expected 30m, got %v", d)
	}
}

func TestParseSyncInterval_CronHours(t *testing.T) {
	d := parseSyncInterval("0 */4 * * *")
	if d != 4*time.Hour {
		t.Errorf("expected 4h, got %v", d)
	}
}

func TestParseSyncInterval_GoDuration(t *testing.T) {
	d := parseSyncInterval("30m")
	if d != 30*time.Minute {
		t.Errorf("expected 30m, got %v", d)
	}
}

func TestParseSyncInterval_GoDurationHours(t *testing.T) {
	d := parseSyncInterval("2h")
	if d != 2*time.Hour {
		t.Errorf("expected 2h, got %v", d)
	}
}

func TestParseSyncInterval_Empty(t *testing.T) {
	d := parseSyncInterval("")
	if d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
}

func TestParseSyncInterval_ComplexCron(t *testing.T) {
	// Complex cron expressions that don't match simple patterns → 0
	d := parseSyncInterval("0 7,19 * * 1-5")
	if d != 0 {
		t.Errorf("expected 0 for complex cron, got %v", d)
	}
}

func TestParseSyncInterval_InvalidText(t *testing.T) {
	d := parseSyncInterval("every day")
	if d != 0 {
		t.Errorf("expected 0 for invalid text, got %v", d)
	}
}

// SCN-023-09: getSyncInterval reads from connector config.
func TestGetSyncInterval_FromConfig(t *testing.T) {
	registry := NewRegistry()
	supervisor := NewSupervisor(registry, nil)
	supervisor.SetConfig("rss", ConnectorConfig{
		SyncSchedule: "*/30 * * * *",
	})

	interval := supervisor.getSyncInterval("rss")
	if interval != 30*time.Minute {
		t.Errorf("expected 30m, got %v", interval)
	}
}

func TestGetSyncInterval_FromSourceConfig(t *testing.T) {
	registry := NewRegistry()
	supervisor := NewSupervisor(registry, nil)
	supervisor.SetConfig("weather", ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_interval": "1h",
		},
	})

	interval := supervisor.getSyncInterval("weather")
	if interval != 1*time.Hour {
		t.Errorf("expected 1h, got %v", interval)
	}
}

func TestGetSyncInterval_Default(t *testing.T) {
	registry := NewRegistry()
	supervisor := NewSupervisor(registry, nil)

	interval := supervisor.getSyncInterval("unknown")
	if interval != defaultSyncInterval {
		t.Errorf("expected default %v, got %v", defaultSyncInterval, interval)
	}
}

func TestGetSyncInterval_EmptySchedule(t *testing.T) {
	registry := NewRegistry()
	supervisor := NewSupervisor(registry, nil)
	supervisor.SetConfig("empty", ConnectorConfig{
		SyncSchedule: "",
	})

	interval := supervisor.getSyncInterval("empty")
	if interval != defaultSyncInterval {
		t.Errorf("expected default %v, got %v", defaultSyncInterval, interval)
	}
}

// DEV-003-001: OAuth connector sync schedules MUST be configurable via SST.
// Before the fix, IMAP/CalDAV/YouTube connectors were started with empty
// SyncSchedule, falling back to defaultSyncInterval (5m). IMAP should be 15m,
// YouTube should be 4h per spec R-202/R-203.
func TestGetSyncInterval_OAuthConnectorSchedules(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		schedule string
		expected time.Duration
	}{
		{"imap_15m", "gmail", "*/15 * * * *", 15 * time.Minute},
		{"caldav_15m", "google-calendar", "*/15 * * * *", 15 * time.Minute},
		{"youtube_4h", "youtube", "0 */4 * * *", 4 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			supervisor := NewSupervisor(registry, nil)
			supervisor.SetConfig(tt.id, ConnectorConfig{
				AuthType:     "oauth2",
				SyncSchedule: tt.schedule,
			})

			interval := supervisor.getSyncInterval(tt.id)
			if interval != tt.expected {
				t.Errorf("expected %v for %s, got %v (defaultSyncInterval=%v)",
					tt.expected, tt.id, interval, defaultSyncInterval)
			}
			if interval == defaultSyncInterval && tt.expected != defaultSyncInterval {
				t.Errorf("REGRESSION: %s fell back to default %v — SST schedule not applied",
					tt.id, defaultSyncInterval)
			}
		})
	}
}
