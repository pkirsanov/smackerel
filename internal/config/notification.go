package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type NotificationConfig struct {
	Enabled                bool
	PersistenceThreshold   int
	EscalationSeverity     string
	LowConfidenceThreshold float64
	MaxRetries             int
	OutputChannels         []string
}

func loadNotificationConfig() (NotificationConfig, error) {
	var cfg NotificationConfig
	var errs []string
	cfg.Enabled, errs = requiredBool("NOTIFICATION_INTELLIGENCE_ENABLED", errs)
	cfg.PersistenceThreshold, errs = parsePositiveInt("NOTIFICATION_PERSISTENCE_THRESHOLD", errs)
	cfg.EscalationSeverity, errs = requiredEnum("NOTIFICATION_ESCALATION_SEVERITY", map[string]struct{}{"medium": {}, "high": {}, "critical": {}}, "medium|high|critical", errs)
	cfg.LowConfidenceThreshold, errs = parseNotificationUnitFloat("NOTIFICATION_LOW_CONFIDENCE_THRESHOLD", errs)
	cfg.MaxRetries, errs = parsePositiveInt("NOTIFICATION_MAX_RETRIES", errs)
	cfg.OutputChannels, errs = requiredStringList("NOTIFICATION_OUTPUT_CHANNELS", errs)
	if len(cfg.OutputChannels) == 0 {
		errs = append(errs, "NOTIFICATION_OUTPUT_CHANNELS")
	}
	if len(errs) > 0 {
		return NotificationConfig{}, fmt.Errorf("missing or invalid required notification configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

func parseNotificationUnitFloat(key string, errs []string) (float64, []string) {
	value := os.Getenv(key)
	if value == "" {
		return 0, append(errs, key)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 || parsed > 1 {
		return 0, append(errs, key+" (must be a float in (0, 1]")
	}
	return parsed, errs
}
