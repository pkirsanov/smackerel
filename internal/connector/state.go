package connector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncState represents the sync state of a connector stored in the database.
type SyncState struct {
	SourceID    string `json:"source_id"`
	Enabled     bool   `json:"enabled"`
	LastSync    string `json:"last_sync,omitempty"`
	SyncCursor  string `json:"sync_cursor,omitempty"`
	ItemsSynced int    `json:"items_synced"`
	ErrorsCount int    `json:"errors_count"`
	LastError   string `json:"last_error,omitempty"`
}

// StateStore manages connector sync state in PostgreSQL.
type StateStore struct {
	Pool *pgxpool.Pool
}

// NewStateStore creates a new sync state store.
func NewStateStore(pool *pgxpool.Pool) *StateStore {
	return &StateStore{Pool: pool}
}

// Get retrieves the sync state for a connector.
func (s *StateStore) Get(ctx context.Context, sourceID string) (*SyncState, error) {
	var state SyncState
	err := s.Pool.QueryRow(ctx, `
		SELECT source_id, enabled, COALESCE(last_sync::text, ''), COALESCE(sync_cursor, ''),
		       items_synced, errors_count, COALESCE(last_error, '')
		FROM sync_state WHERE source_id = $1
	`, sourceID).Scan(&state.SourceID, &state.Enabled, &state.LastSync, &state.SyncCursor,
		&state.ItemsSynced, &state.ErrorsCount, &state.LastError)
	if err != nil {
		return nil, fmt.Errorf("get sync state: %w", err)
	}
	return &state, nil
}

// Save persists the sync state for a connector.
func (s *StateStore) Save(ctx context.Context, state *SyncState) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced, errors_count, last_error, last_sync, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (source_id) DO UPDATE SET
			enabled = $2, sync_cursor = $3, items_synced = sync_state.items_synced + EXCLUDED.items_synced,
			errors_count = $5, last_error = $6, last_sync = NOW(), updated_at = NOW()
	`, state.SourceID, state.Enabled, state.SyncCursor, state.ItemsSynced,
		state.ErrorsCount, state.LastError)
	if err != nil {
		return fmt.Errorf("save sync state: %w", err)
	}
	slog.Debug("sync state saved", "source_id", state.SourceID, "cursor", state.SyncCursor)
	return nil
}

// SaveCapability persists the QF capability handshake response, fetched-at
// timestamp, and compatibility status into the sync_state row owned by
// sourceID. The row is created if it does not exist. Existing Get/Save
// signatures are intentionally untouched; this is an additive helper used
// by capability-aware connectors (spec 041, SCN-SM-041-003) to land the
// handshake result in the three migration-034 columns: capability_response
// (JSONB), capability_fetched_at (TIMESTAMPTZ), capability_status (TEXT).
//
// responseJSON MUST be a JSON-marshaled capability envelope (or empty
// string to clear the column). fetchedAt is stored verbatim (UTC enforced
// at the call site). status is the connector-layer compatibility verdict
// (one of CapabilityStatusCompatible / CapabilityStatusIncompatible /
// CapabilityStatusUnfetched in package qfdecisions).
func (s *StateStore) SaveCapability(ctx context.Context, sourceID, responseJSON string, fetchedAt time.Time, status string) error {
	var (
		responseArg interface{}
		fetchedArg  interface{}
		statusArg   interface{}
	)
	if responseJSON == "" {
		responseArg = nil
	} else {
		responseArg = responseJSON
	}
	if fetchedAt.IsZero() {
		fetchedArg = nil
	} else {
		fetchedArg = fetchedAt.UTC()
	}
	if status == "" {
		statusArg = nil
	} else {
		statusArg = status
	}

	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sync_state (
			source_id, enabled, sync_cursor, items_synced, errors_count, last_error,
			capability_response, capability_fetched_at, capability_status,
			last_sync, updated_at
		) VALUES ($1, true, '', 0, 0, '', $2, $3, $4, NOW(), NOW())
		ON CONFLICT (source_id) DO UPDATE SET
			capability_response   = $2,
			capability_fetched_at = $3,
			capability_status     = $4,
			updated_at            = NOW()
	`, sourceID, responseArg, fetchedArg, statusArg)
	if err != nil {
		return fmt.Errorf("save capability state: %w", err)
	}
	slog.Debug("capability state saved",
		"source_id", sourceID,
		"status", status,
		"fetched_at", fetchedAt,
		"response_bytes", len(responseJSON))
	return nil
}

// GetCapability retrieves the persisted capability columns for sourceID.
// Returns empty values (responseJSON="", fetchedAt=zero time, status="")
// when the row exists but the capability columns are NULL, and a wrapped
// sql.ErrNoRows when the row itself does not exist. Existing Get behavior
// is intentionally untouched.
func (s *StateStore) GetCapability(ctx context.Context, sourceID string) (responseJSON string, fetchedAt time.Time, status string, err error) {
	var (
		response sql.NullString
		fetched  sql.NullTime
		stat     sql.NullString
	)
	err = s.Pool.QueryRow(ctx, `
		SELECT capability_response::text, capability_fetched_at, capability_status
		FROM sync_state WHERE source_id = $1
	`, sourceID).Scan(&response, &fetched, &stat)
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("get capability state: %w", err)
	}
	if response.Valid {
		responseJSON = response.String
	}
	if fetched.Valid {
		fetchedAt = fetched.Time
	}
	if stat.Valid {
		status = stat.String
	}
	return responseJSON, fetchedAt, status, nil
}

// RecordError increments the error count and stores the error message.
func (s *StateStore) RecordError(ctx context.Context, sourceID string, errMsg string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced, errors_count, last_error, last_sync, updated_at)
		VALUES ($1, true, '', 0, 1, $2, NOW(), NOW())
		ON CONFLICT (source_id) DO UPDATE SET
			errors_count = sync_state.errors_count + 1,
			last_error = $2,
			updated_at = NOW()
	`, sourceID, errMsg)
	if err != nil {
		return fmt.Errorf("record error: %w", err)
	}
	return nil
}
