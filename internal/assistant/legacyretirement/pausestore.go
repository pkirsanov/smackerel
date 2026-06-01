// pausestore.go — spec 075 SCOPE-4 SQL-backed PauseStateStore.
//
// Persists rows in assistant_legacy_retirement_state (migration 046).
// Schema invariants enforced by the migration:
//
//   - state_id PRIMARY KEY (we use the window_id as the state_id;
//     one pause row per window).
//   - effective_state CHECK ('open','paused').
//   - consecutive_days_over_threshold NOT NULL.
//   - updated_at, updated_by, schema_version all NOT NULL.
//
// SCOPE-1 only declared the column shape. SCOPE-4 owns the writes
// (Pause / Resume) and the read used by the WindowStateResolver
// (IsPaused).
package legacyretirement

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pauseStateSchemaVersion pins the schema_version written by this
// package. Bumping this constant requires a migration plus a
// coordinated reader update.
const pauseStateSchemaVersion = 1

// pgxPauseQuerier is the minimal SQL surface the store needs.
type pgxPauseQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SQLPauseStateStore is the production PauseStateStore wired to the
// assistant Postgres pool.
type SQLPauseStateStore struct {
	db pgxPauseQuerier
}

// NewSQLPauseStateStore validates the pool and returns the store.
func NewSQLPauseStateStore(pool *pgxpool.Pool) (*SQLPauseStateStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLPauseStateStore: Pool is nil")
	}
	return &SQLPauseStateStore{db: pool}, nil
}

// IsPaused implements PauseStateReader.
func (s *SQLPauseStateStore) IsPaused(ctx context.Context, windowID string) (bool, error) {
	if windowID == "" {
		return false, fmt.Errorf("legacyretirement: IsPaused: windowID empty")
	}
	const q = `
		SELECT effective_state
		  FROM assistant_legacy_retirement_state
		 WHERE state_id = $1`
	var state string
	err := s.db.QueryRow(ctx, q, windowID).Scan(&state)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("legacyretirement: IsPaused select: %w", err)
	}
	return state == string(WindowPaused), nil
}

// Pause UPSERTs the row to effective_state='paused' with the
// supplied breaching command and consecutive-day count.
func (s *SQLPauseStateStore) Pause(ctx context.Context, windowID, command string, consecutiveDays int, now time.Time, updatedBy string) error {
	if windowID == "" {
		return fmt.Errorf("legacyretirement: Pause: windowID empty")
	}
	if command == "" {
		return fmt.Errorf("legacyretirement: Pause: command empty")
	}
	if consecutiveDays < 1 {
		return fmt.Errorf("legacyretirement: Pause: consecutiveDays=%d must be >= 1", consecutiveDays)
	}
	if updatedBy == "" {
		return fmt.Errorf("legacyretirement: Pause: updatedBy empty (audit required)")
	}

	nowUTC := now.UTC()
	startDay := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(consecutiveDays - 1))

	const q = `
		INSERT INTO assistant_legacy_retirement_state
		    (state_id, window_id, effective_state, paused_reason,
		     threshold_command, threshold_started_on,
		     consecutive_days_over_threshold,
		     updated_at, updated_by, schema_version)
		VALUES ($1, $2, 'paused', 'threshold_exceeded',
		        $3, $4, $5, $6, $7, $8)
		ON CONFLICT (state_id) DO UPDATE
		   SET window_id                       = EXCLUDED.window_id,
		       effective_state                 = 'paused',
		       paused_reason                   = 'threshold_exceeded',
		       threshold_command               = EXCLUDED.threshold_command,
		       threshold_started_on            = EXCLUDED.threshold_started_on,
		       consecutive_days_over_threshold = EXCLUDED.consecutive_days_over_threshold,
		       updated_at                      = EXCLUDED.updated_at,
		       updated_by                      = EXCLUDED.updated_by,
		       schema_version                  = EXCLUDED.schema_version`
	if _, err := s.db.Exec(ctx, q, windowID, windowID, command, startDay, consecutiveDays, nowUTC, updatedBy, pauseStateSchemaVersion); err != nil {
		return fmt.Errorf("legacyretirement: Pause upsert: %w", err)
	}
	return nil
}

// Resume flips the row back to effective_state='open' and resets
// consecutive_days_over_threshold to zero (SCN-075-A06). If no row
// exists, an empty-effect open row is inserted so the audit trail
// records the operator action.
func (s *SQLPauseStateStore) Resume(ctx context.Context, windowID string, now time.Time, updatedBy string) error {
	if windowID == "" {
		return fmt.Errorf("legacyretirement: Resume: windowID empty")
	}
	if updatedBy == "" {
		return fmt.Errorf("legacyretirement: Resume: updatedBy empty (audit required)")
	}
	const q = `
		INSERT INTO assistant_legacy_retirement_state
		    (state_id, window_id, effective_state, paused_reason,
		     threshold_command, threshold_started_on,
		     consecutive_days_over_threshold,
		     updated_at, updated_by, schema_version)
		VALUES ($1, $2, 'open', NULL, NULL, NULL, 0, $3, $4, $5)
		ON CONFLICT (state_id) DO UPDATE
		   SET window_id                       = EXCLUDED.window_id,
		       effective_state                 = 'open',
		       paused_reason                   = NULL,
		       threshold_command               = NULL,
		       threshold_started_on            = NULL,
		       consecutive_days_over_threshold = 0,
		       updated_at                      = EXCLUDED.updated_at,
		       updated_by                      = EXCLUDED.updated_by,
		       schema_version                  = EXCLUDED.schema_version`
	if _, err := s.db.Exec(ctx, q, windowID, windowID, now.UTC(), updatedBy, pauseStateSchemaVersion); err != nil {
		return fmt.Errorf("legacyretirement: Resume upsert: %w", err)
	}
	return nil
}
