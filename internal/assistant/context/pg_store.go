// Spec 061 SCOPE-04 — PostgreSQL-backed conversation store.
//
// One row per (user_id, transport) in assistant_conversations
// (migration 041). JSONB blobs round-trip through encoding/json.
// LastActivityAt is authoritative from the caller — the store never
// substitutes NOW() so the facade controls the timestamp end-to-end.

package assistantctx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStore implements Store against PostgreSQL.
type PgStore struct {
	pool *pgxpool.Pool
}

// NewPgStore returns a PgStore that uses the supplied pool. The pool
// MUST outlive the PgStore; closing the pool while a Persist /
// SweepIdle is in flight is undefined.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	if pool == nil {
		panic("assistantctx: NewPgStore requires a non-nil pool")
	}
	return &PgStore{pool: pool}
}

// Load implements Store.Load.
func (s *PgStore) Load(ctx context.Context, userID, transport string) (Conversation, bool, error) {
	if userID == "" || transport == "" {
		return Conversation{}, false, errors.New("assistantctx: Load requires non-empty userID and transport")
	}
	const q = `
SELECT working_context, pending_confirm, pending_disambig, pending_clarify, last_activity_at, schema_version
  FROM assistant_conversations
 WHERE user_id = $1 AND transport = $2
`
	var (
		workingRaw  []byte
		confirmRaw  []byte
		disambigRaw []byte
		clarifyRaw  []byte
		lastActive  time.Time
		schemaVer   int
	)
	row := s.pool.QueryRow(ctx, q, userID, transport)
	err := row.Scan(&workingRaw, &confirmRaw, &disambigRaw, &clarifyRaw, &lastActive, &schemaVer)
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{UserID: userID, Transport: transport}, false, nil
	}
	if err != nil {
		return Conversation{}, false, fmt.Errorf("assistantctx: Load query: %w", err)
	}

	conv := Conversation{
		UserID:         userID,
		Transport:      transport,
		LastActivityAt: lastActive,
		SchemaVersion:  schemaVer,
	}
	if len(workingRaw) > 0 {
		if err := json.Unmarshal(workingRaw, &conv.WorkingContext); err != nil {
			return Conversation{}, false, fmt.Errorf("assistantctx: Load decode working_context: %w", err)
		}
	}
	if len(confirmRaw) > 0 {
		var pc PendingConfirm
		if err := json.Unmarshal(confirmRaw, &pc); err != nil {
			return Conversation{}, false, fmt.Errorf("assistantctx: Load decode pending_confirm: %w", err)
		}
		conv.PendingConfirm = &pc
	}
	if len(disambigRaw) > 0 {
		var pd PendingDisambig
		if err := json.Unmarshal(disambigRaw, &pd); err != nil {
			return Conversation{}, false, fmt.Errorf("assistantctx: Load decode pending_disambig: %w", err)
		}
		conv.PendingDisambig = &pd
	}
	if len(clarifyRaw) > 0 {
		var pc PendingClarify
		if err := json.Unmarshal(clarifyRaw, &pc); err != nil {
			return Conversation{}, false, fmt.Errorf("assistantctx: Load decode pending_clarify: %w", err)
		}
		conv.PendingClarify = &pc
	}
	return conv, true, nil
}

// Persist implements Store.Persist.
func (s *PgStore) Persist(ctx context.Context, conv Conversation) error {
	if conv.UserID == "" || conv.Transport == "" {
		return errors.New("assistantctx: Persist requires non-empty UserID and Transport")
	}
	if conv.LastActivityAt.IsZero() {
		return errors.New("assistantctx: Persist requires non-zero LastActivityAt (facade owns the timestamp)")
	}
	if conv.SchemaVersion == 0 {
		conv.SchemaVersion = 1
	}
	workingRaw, err := json.Marshal(conv.WorkingContext)
	if err != nil {
		return fmt.Errorf("assistantctx: Persist encode working_context: %w", err)
	}
	var confirmRaw []byte
	if conv.PendingConfirm != nil {
		confirmRaw, err = json.Marshal(conv.PendingConfirm)
		if err != nil {
			return fmt.Errorf("assistantctx: Persist encode pending_confirm: %w", err)
		}
	}
	var disambigRaw []byte
	if conv.PendingDisambig != nil {
		disambigRaw, err = json.Marshal(conv.PendingDisambig)
		if err != nil {
			return fmt.Errorf("assistantctx: Persist encode pending_disambig: %w", err)
		}
	}
	var clarifyRaw []byte
	if conv.PendingClarify != nil {
		clarifyRaw, err = json.Marshal(conv.PendingClarify)
		if err != nil {
			return fmt.Errorf("assistantctx: Persist encode pending_clarify: %w", err)
		}
	}
	const q = `
INSERT INTO assistant_conversations
    (user_id, transport, working_context, pending_confirm, pending_disambig, pending_clarify, last_activity_at, schema_version)
VALUES
    ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7, $8)
ON CONFLICT (user_id, transport) DO UPDATE
    SET working_context  = EXCLUDED.working_context,
        pending_confirm  = EXCLUDED.pending_confirm,
        pending_disambig = EXCLUDED.pending_disambig,
        pending_clarify  = EXCLUDED.pending_clarify,
        last_activity_at = EXCLUDED.last_activity_at,
        schema_version   = EXCLUDED.schema_version
`
	if _, err := s.pool.Exec(ctx, q,
		conv.UserID, conv.Transport,
		string(workingRaw),
		jsonbOrNull(confirmRaw),
		jsonbOrNull(disambigRaw),
		jsonbOrNull(clarifyRaw),
		conv.LastActivityAt,
		conv.SchemaVersion,
	); err != nil {
		return fmt.Errorf("assistantctx: Persist upsert: %w", err)
	}
	return nil
}

// PendingClarifyRow is a sweeper-only projection of one
// assistant_conversations row whose pending_clarify column is set and
// whose emit_time is older than the configured
// capture_as_fallback.clarify_abandon_timeout. ListAbandonedClarifies
// returns these so the SCOPE-074-04C ClarifyAbandonSweeper can call
// Policy.CaptureForUser directly without re-loading the full
// Conversation row (the working context is irrelevant to the capture
// decision; the per-row identity + payload is all the sweeper needs).
type PendingClarifyRow struct {
	UserID    string
	Transport string
	Payload   PendingClarify
}

// ListAbandonedClarifies returns every row whose pending_clarify is
// non-NULL AND whose (pending_clarify->>'emit_time')::timestamptz is
// strictly older than now - timeout. The query is bounded by the
// partial index idx_assistant_conversations_pending_clarify so the
// sweep is cheap even with a large assistant_conversations table.
func (s *PgStore) ListAbandonedClarifies(ctx context.Context, timeout time.Duration) ([]PendingClarifyRow, error) {
	if timeout <= 0 {
		return nil, errors.New("assistantctx: ListAbandonedClarifies requires a positive timeout")
	}
	secs := int64(timeout.Seconds())
	const q = `
SELECT user_id, transport, pending_clarify
  FROM assistant_conversations
 WHERE pending_clarify IS NOT NULL
   AND (pending_clarify->>'emit_time')::timestamptz <= NOW() - make_interval(secs => $1::double precision)
`
	rows, err := s.pool.Query(ctx, q, secs)
	if err != nil {
		return nil, fmt.Errorf("assistantctx: ListAbandonedClarifies: %w", err)
	}
	defer rows.Close()
	out := []PendingClarifyRow{}
	for rows.Next() {
		var (
			userID    string
			transport string
			payload   []byte
		)
		if err := rows.Scan(&userID, &transport, &payload); err != nil {
			return nil, fmt.Errorf("assistantctx: ListAbandonedClarifies scan: %w", err)
		}
		var pc PendingClarify
		if err := json.Unmarshal(payload, &pc); err != nil {
			return nil, fmt.Errorf("assistantctx: ListAbandonedClarifies decode pending_clarify: %w", err)
		}
		out = append(out, PendingClarifyRow{UserID: userID, Transport: transport, Payload: pc})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("assistantctx: ListAbandonedClarifies rows: %w", err)
	}
	return out, nil
}

// ClearPendingClarify removes pending_clarify (sets it to NULL) for
// the given (userID, transport) row WITHOUT touching any other column.
// Used by the sweeper after a successful capture (per design step 3:
// "clear pending_clarify in the same transaction as the capture write").
//
// Idempotent: the WHERE clause matches at most one row and clearing
// an already-NULL column is a no-op.
func (s *PgStore) ClearPendingClarify(ctx context.Context, userID, transport string) error {
	if userID == "" || transport == "" {
		return errors.New("assistantctx: ClearPendingClarify requires non-empty userID and transport")
	}
	const q = `UPDATE assistant_conversations SET pending_clarify = NULL WHERE user_id = $1 AND transport = $2`
	if _, err := s.pool.Exec(ctx, q, userID, transport); err != nil {
		return fmt.Errorf("assistantctx: ClearPendingClarify: %w", err)
	}
	return nil
}

// DeleteByKey implements Store.DeleteByKey.
func (s *PgStore) DeleteByKey(ctx context.Context, userID, transport string) error {
	if userID == "" || transport == "" {
		return errors.New("assistantctx: DeleteByKey requires non-empty userID and transport")
	}
	const q = `DELETE FROM assistant_conversations WHERE user_id = $1 AND transport = $2`
	if _, err := s.pool.Exec(ctx, q, userID, transport); err != nil {
		return fmt.Errorf("assistantctx: DeleteByKey: %w", err)
	}
	return nil
}

// SweepIdle implements Store.SweepIdle. PostgreSQL evaluates NOW() at
// statement start so the cutoff is computed inside the DB.
func (s *PgStore) SweepIdle(ctx context.Context, idleTTL time.Duration) (int64, error) {
	if idleTTL <= 0 {
		return 0, errors.New("assistantctx: SweepIdle requires a positive idleTTL")
	}
	// Cast the duration to seconds for the SQL interval expression.
	secs := int64(idleTTL.Seconds())
	const q = `
DELETE FROM assistant_conversations
 WHERE last_activity_at < NOW() - make_interval(secs => $1::double precision)
`
	tag, err := s.pool.Exec(ctx, q, secs)
	if err != nil {
		return 0, fmt.Errorf("assistantctx: SweepIdle: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CountActiveByTransport implements Store.CountActiveByTransport.
// Returns one entry per transport present in the table. Transports
// with zero rows are omitted from the returned map (the refresher
// fills in zeros for the known closed transport vocabulary).
func (s *PgStore) CountActiveByTransport(ctx context.Context) (map[string]int, error) {
	const q = `SELECT transport, COUNT(*)::bigint FROM assistant_conversations GROUP BY transport`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("assistantctx: CountActiveByTransport query: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var transport string
		var count int64
		if err := rows.Scan(&transport, &count); err != nil {
			return nil, fmt.Errorf("assistantctx: CountActiveByTransport scan: %w", err)
		}
		counts[transport] = int(count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("assistantctx: CountActiveByTransport rows: %w", err)
	}
	return counts, nil
}

// jsonbOrNull returns the supplied raw JSON or a typed nil so the
// JSONB column receives SQL NULL when the pending struct is absent.
func jsonbOrNull(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}
