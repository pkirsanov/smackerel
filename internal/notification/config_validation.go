package notification

import (
	"fmt"
	"strconv"
	"strings"
)

type NotificationConfig struct {
	Enabled                bool
	PersistenceThreshold   int
	EscalationSeverity     Severity
	LowConfidenceThreshold float64
	MaxRetries             int
	OutputChannels         []string
}

func LoadNotificationConfig(env map[string]string) (NotificationConfig, error) {
	enabledRaw, ok := env["NOTIFICATION_INTELLIGENCE_ENABLED"]
	if !ok || enabledRaw == "" {
		return NotificationConfig{}, fmt.Errorf("NOTIFICATION_INTELLIGENCE_ENABLED is required")
	}
	if enabledRaw != "true" && enabledRaw != "false" {
		return NotificationConfig{}, fmt.Errorf("NOTIFICATION_INTELLIGENCE_ENABLED must be true or false")
	}
	cfg := NotificationConfig{Enabled: enabledRaw == "true"}
	if !cfg.Enabled {
		return cfg, nil
	}
	var errs []string
	if raw := env["NOTIFICATION_PERSISTENCE_THRESHOLD"]; raw == "" {
		errs = append(errs, "NOTIFICATION_PERSISTENCE_THRESHOLD")
	} else if parsed, err := strconv.Atoi(raw); err != nil || parsed < 1 {
		errs = append(errs, "NOTIFICATION_PERSISTENCE_THRESHOLD")
	} else {
		cfg.PersistenceThreshold = parsed
	}
	if raw := env["NOTIFICATION_ESCALATION_SEVERITY"]; raw == "" || severityRank(ParseSeverity(raw)) == 0 {
		errs = append(errs, "NOTIFICATION_ESCALATION_SEVERITY")
	} else {
		cfg.EscalationSeverity = ParseSeverity(raw)
	}
	if raw := env["NOTIFICATION_LOW_CONFIDENCE_THRESHOLD"]; raw == "" {
		errs = append(errs, "NOTIFICATION_LOW_CONFIDENCE_THRESHOLD")
	} else if parsed, err := strconv.ParseFloat(raw, 64); err != nil || parsed <= 0 || parsed > 1 {
		errs = append(errs, "NOTIFICATION_LOW_CONFIDENCE_THRESHOLD")
	} else {
		cfg.LowConfidenceThreshold = parsed
	}
	if raw := env["NOTIFICATION_MAX_RETRIES"]; raw == "" {
		errs = append(errs, "NOTIFICATION_MAX_RETRIES")
	} else if parsed, err := strconv.Atoi(raw); err != nil || parsed < 1 {
		errs = append(errs, "NOTIFICATION_MAX_RETRIES")
	} else {
		cfg.MaxRetries = parsed
	}
	if raw := env["NOTIFICATION_OUTPUT_CHANNELS"]; raw == "" {
		errs = append(errs, "NOTIFICATION_OUTPUT_CHANNELS")
	} else {
		for _, channel := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(channel); trimmed != "" {
				cfg.OutputChannels = append(cfg.OutputChannels, trimmed)
			}
		}
		if len(cfg.OutputChannels) == 0 {
			errs = append(errs, "NOTIFICATION_OUTPUT_CHANNELS")
		}
	}
	if len(errs) > 0 {
		return NotificationConfig{}, fmt.Errorf("missing or invalid required notification configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}
