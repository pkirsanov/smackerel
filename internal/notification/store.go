package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateSourceInstance(ctx context.Context, cfg SourceInstanceConfig, now time.Time) (SourceInstanceRecord, error) {
	if s == nil || s.pool == nil {
		return SourceInstanceRecord{}, fmt.Errorf("notification source store: postgres pool is required")
	}
	if err := cfg.Validate(); err != nil {
		return SourceInstanceRecord{}, err
	}
	if now.IsZero() {
		return SourceInstanceRecord{}, fmt.Errorf("notification source store: timestamp is required")
	}
	metadataJSON, err := json.Marshal(cfg.RedactedMetadata)
	if err != nil {
		return SourceInstanceRecord{}, fmt.Errorf("marshal source redacted metadata: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_source_instances (
    source_instance_id, source_type, source_form, enabled, config_hash,
    secret_ref_names, redacted_metadata, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		cfg.SourceInstanceID,
		cfg.SourceType,
		cfg.SourceForm,
		*cfg.Enabled,
		cfg.ConfigHash,
		cfg.SecretRefNames,
		metadataJSON,
		now,
		now,
	)
	if err != nil {
		return SourceInstanceRecord{}, fmt.Errorf("insert notification source instance %q: %w", cfg.SourceInstanceID, err)
	}
	return SourceInstanceRecord{
		SourceType:       cfg.SourceType,
		SourceInstanceID: cfg.SourceInstanceID,
		SourceForm:       cfg.SourceForm,
		Enabled:          *cfg.Enabled,
		ConfigHash:       cfg.ConfigHash,
		SecretRefNames:   append([]string(nil), cfg.SecretRefNames...),
		RedactedMetadata: cloneStringMap(cfg.RedactedMetadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *Store) RecordSourceHealth(ctx context.Context, report SourceHealthReport) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("notification source store: postgres pool is required")
	}
	redacted, err := RedactHealthReport(report)
	if err != nil {
		return err
	}
	healthID := "notif_health_" + uuid.NewString()
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_source_health_events (
    id, source_instance_id, source_type, source_form, state,
    last_event_at, last_successful_check_at, retry_count,
    last_error_kind, last_error_redacted, observed_at, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		healthID,
		redacted.SourceInstanceID,
		redacted.SourceType,
		redacted.SourceForm,
		redacted.State,
		redacted.LastEventAt,
		redacted.LastSuccessfulCheckAt,
		redacted.RetryCount,
		nullableString(redacted.LastErrorKind),
		nullableString(redacted.LastErrorRedacted),
		redacted.ObservedAt,
		redacted.ObservedAt,
	)
	if err != nil {
		return fmt.Errorf("insert notification source health for %q: %w", redacted.SourceInstanceID, err)
	}
	return nil
}

func (s *Store) ListSourceStatuses(ctx context.Context) ([]SourceStatus, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("notification source store: postgres pool is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT
    src.source_type,
    src.source_instance_id,
    src.source_form,
    src.enabled,
    src.config_hash,
    src.secret_ref_names,
    src.redacted_metadata,
    src.created_at,
    src.updated_at,
    health.state,
    health.last_event_at,
    health.last_successful_check_at,
    health.retry_count,
    health.last_error_kind,
    health.last_error_redacted,
    health.observed_at
FROM notification_source_instances src
LEFT JOIN LATERAL (
    SELECT state, last_event_at, last_successful_check_at, retry_count,
           last_error_kind, last_error_redacted, observed_at
    FROM notification_source_health_events h
    WHERE h.source_instance_id = src.source_instance_id
    ORDER BY h.observed_at DESC, h.created_at DESC, h.id DESC
    LIMIT 1
) health ON TRUE
ORDER BY src.source_instance_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list notification source statuses: %w", err)
	}
	defer rows.Close()

	statuses := []SourceStatus{}
	for rows.Next() {
		var record SourceInstanceRecord
		var metadataJSON []byte
		var state *string
		var lastEventAt *time.Time
		var lastCheckAt *time.Time
		var retryCount *int
		var errorKind *string
		var errorRedacted *string
		var observedAt *time.Time
		if err := rows.Scan(
			&record.SourceType,
			&record.SourceInstanceID,
			&record.SourceForm,
			&record.Enabled,
			&record.ConfigHash,
			&record.SecretRefNames,
			&metadataJSON,
			&record.CreatedAt,
			&record.UpdatedAt,
			&state,
			&lastEventAt,
			&lastCheckAt,
			&retryCount,
			&errorKind,
			&errorRedacted,
			&observedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification source status: %w", err)
		}
		if err := json.Unmarshal(metadataJSON, &record.RedactedMetadata); err != nil {
			return nil, fmt.Errorf("parse source redacted metadata for %q: %w", record.SourceInstanceID, err)
		}
		health := SourceHealthReport{
			SourceType:        record.SourceType,
			SourceInstanceID:  record.SourceInstanceID,
			SourceForm:        record.SourceForm,
			State:             SourceHealthDisconnected,
			LastErrorKind:     "no_health_report",
			LastErrorRedacted: "source health has not reported",
		}
		if state != nil {
			health = SourceHealthReport{
				SourceType:       record.SourceType,
				SourceInstanceID: record.SourceInstanceID,
				SourceForm:       record.SourceForm,
				State:            SourceHealthState(*state),
			}
			health.LastEventAt = lastEventAt
			health.LastSuccessfulCheckAt = lastCheckAt
			if retryCount != nil {
				health.RetryCount = *retryCount
			}
			if errorKind != nil {
				health.LastErrorKind = *errorKind
			}
			if errorRedacted != nil {
				health.LastErrorRedacted = *errorRedacted
			}
			if observedAt != nil {
				health.ObservedAt = *observedAt
			}
		}
		statuses = append(statuses, SourceStatus{Config: record, Health: health})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification source statuses: %w", err)
	}
	return statuses, nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
