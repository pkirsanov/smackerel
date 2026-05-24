package notification

import (
	"context"
	"fmt"
	"strings"
)

type HealthStore interface {
	RecordSourceHealth(ctx context.Context, report SourceHealthReport) error
}

type HealthService struct {
	store HealthStore
}

func NewHealthService(store HealthStore) *HealthService {
	return &HealthService{store: store}
}

func (s *HealthService) Report(ctx context.Context, report SourceHealthReport) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("notification source health: store is required")
	}
	redacted, err := RedactHealthReport(report)
	if err != nil {
		return err
	}
	return s.store.RecordSourceHealth(ctx, redacted)
}

func RedactHealthReport(report SourceHealthReport) (SourceHealthReport, error) {
	if err := report.Validate(); err != nil {
		return SourceHealthReport{}, err
	}
	report.SourceType = strings.TrimSpace(report.SourceType)
	report.SourceInstanceID = strings.TrimSpace(report.SourceInstanceID)
	report.LastErrorKind = strings.TrimSpace(report.LastErrorKind)
	if report.State == SourceHealthConnected {
		report.LastErrorKind = ""
		report.LastErrorRedacted = ""
		return report, nil
	}
	if report.LastErrorKind == "" {
		report.LastErrorKind = "source_error"
	}
	report.LastErrorRedacted = redactedHealthMessage(report.LastErrorKind)
	return report, nil
}

func redactedHealthMessage(kind string) string {
	switch strings.TrimSpace(kind) {
	case "auth_failed", "invalid_credentials", "credential_ref_missing":
		return "source authentication failed"
	case "connectivity_failed", "timeout", "dns_failed":
		return "source connectivity check failed"
	case "dead_letter_pressure":
		return "source dead-letter pressure threshold exceeded"
	case "invalid_config", "missing_source_instance_id", "missing_source_form", "missing_transport_mode", "missing_endpoint", "missing_topics", "invalid_auth_mode", "missing_config_hash", "missing_redacted_metadata":
		return "source configuration missing required field"
	case "transient_failure", "rate_limited", "upstream_5xx":
		return "transient source check failed"
	default:
		return "source health check failed"
	}
}
