package connector

import (
	"context"
	"fmt"
	"log/slog"

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
