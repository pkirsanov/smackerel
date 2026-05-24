package ntfy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/notification"
)

const (
	PayloadRefHashOnly        = "hash_only"
	PayloadRefRawPayloadBytes = "raw_payload_bytes"
	PayloadRefSourceRawEvent  = "source_raw_event_ref"

	DeadLetterMalformedJSON      = "malformed_json"
	DeadLetterUnsupportedEvent   = "unsupported_event_type"
	DeadLetterOversizePayload    = "oversize_payload"
	DeadLetterRedactionFailed    = "redaction_failed"
	DeadLetterSinkUnavailable    = "sink_unavailable"
	DeadLetterSinkRejected       = "sink_rejected"
	DeadLetterAuthFailed         = "auth_failed"
	DeadLetterTopicNotConfigured = "topic_not_configured"
	DeadLetterUnknown            = "unknown"

	ReplayStatusNotReplayable = "not_replayable"
	ReplayStatusPending       = "pending"
	ReplayStatusReplayed      = "replayed"
	ReplayStatusFailed        = "replay_failed"
)

type Store struct {
	pool *pgxpool.Pool
}

type DeadLetterRecord struct {
	ID                 string
	SourceInstanceID   string
	Topic              string
	SourceEventID      string
	EventType          string
	ObservedAt         time.Time
	PayloadHash        string
	PayloadSizeBytes   int
	PayloadRefKind     string `json:"-"`
	RawPayload         []byte `json:"-"`
	SourceRawEventID   string
	SafePayloadPreview string
	CauseKind          string
	CauseRedacted      string
	ReplayEligible     bool
	ReplayStatus       string
	AttemptCount       int
	LastAttemptAt      *time.Time
	RedactionState     map[string]any
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ReplayAttemptRecord struct {
	ID               string
	DeadLetterID     string
	SourceInstanceID string
	IdempotencyKey   string
	ActorKind        string
	ActorRef         string
	Status           string
	RawEventID       string
	SinkStatus       string
	ErrorKind        string
	ErrorRedacted    string
	AttemptedAt      time.Time
	AlreadyReplayed  bool `json:"already_replayed,omitempty"`
}

type DeadLetterPage struct {
	Records    []DeadLetterRecord
	NextCursor string
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) UpsertSubscriptionState(ctx context.Context, state SubscriptionState) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("ntfy store: postgres pool is required")
	}
	if strings.TrimSpace(state.SourceInstanceID) == "" || strings.TrimSpace(state.Topic) == "" {
		return fmt.Errorf("ntfy subscription state: source instance id and topic are required")
	}
	if state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		return fmt.Errorf("ntfy subscription state: timestamps are required")
	}
	redactionJSON, err := json.Marshal(state.RedactionState)
	if err != nil {
		return fmt.Errorf("marshal ntfy subscription redaction state: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_ntfy_subscription_states (
    source_instance_id, topic, source_form, transport_mode, subscription_state,
    last_ntfy_event_id, last_event_at, last_open_at, last_keepalive_at,
    last_successful_check_at, lag_seconds, possible_gap, retry_count, retry_budget,
    last_error_kind, last_error_redacted, redaction_state, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
ON CONFLICT (source_instance_id, topic) DO UPDATE SET
    source_form = EXCLUDED.source_form,
    transport_mode = EXCLUDED.transport_mode,
    subscription_state = EXCLUDED.subscription_state,
    last_ntfy_event_id = EXCLUDED.last_ntfy_event_id,
    last_event_at = EXCLUDED.last_event_at,
    last_open_at = EXCLUDED.last_open_at,
    last_keepalive_at = EXCLUDED.last_keepalive_at,
    last_successful_check_at = EXCLUDED.last_successful_check_at,
    lag_seconds = EXCLUDED.lag_seconds,
    possible_gap = EXCLUDED.possible_gap,
    retry_count = EXCLUDED.retry_count,
    retry_budget = EXCLUDED.retry_budget,
    last_error_kind = EXCLUDED.last_error_kind,
    last_error_redacted = EXCLUDED.last_error_redacted,
    redaction_state = EXCLUDED.redaction_state,
    updated_at = EXCLUDED.updated_at`,
		state.SourceInstanceID, state.Topic, state.SourceForm, state.TransportMode, state.SubscriptionState,
		nullableString(state.LastNtfyEventID), state.LastEventAt, state.LastOpenAt, state.LastKeepaliveAt,
		state.LastSuccessfulCheckAt, state.LagSeconds, state.PossibleGap, state.RetryCount, state.RetryBudget,
		nullableString(state.LastErrorKind), nullableString(state.LastErrorRedacted), redactionJSON, state.CreatedAt, state.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert ntfy subscription state: %w", err)
	}
	return nil
}

func (s *Store) ListSubscriptionStates(ctx context.Context, sourceInstanceID string) ([]SubscriptionState, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("ntfy store: postgres pool is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT source_instance_id, topic, source_form, transport_mode, subscription_state,
       COALESCE(last_ntfy_event_id,''), last_event_at, last_open_at, last_keepalive_at,
       last_successful_check_at, lag_seconds, possible_gap, retry_count, retry_budget,
       COALESCE(last_error_kind,''), COALESCE(last_error_redacted,''), redaction_state, created_at, updated_at
FROM notification_ntfy_subscription_states
WHERE source_instance_id = $1
ORDER BY topic ASC`, sourceInstanceID)
	if err != nil {
		return nil, fmt.Errorf("list ntfy subscription states: %w", err)
	}
	defer rows.Close()
	return scanSubscriptionStates(rows)
}

func NewDeadLetterRecord(cfg Config, event Event, payload []byte, causeKind string, cause string, replayEligible bool, observedAt time.Time) DeadLetterRecord {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	causeRedacted, causeState := notification.RedactText(cause)
	preview, previewState := notification.RedactText(string(payload))
	preview = strings.TrimSpace(preview)
	if len(preview) > 240 {
		preview = preview[:240]
	}
	categories := append([]string{}, causeState.Categories...)
	categories = append(categories, previewState.Categories...)
	replayStatus := ReplayStatusNotReplayable
	payloadRefKind := PayloadRefHashOnly
	var rawPayload []byte
	if replayEligible {
		replayStatus = ReplayStatusPending
		payloadRefKind = PayloadRefRawPayloadBytes
		rawPayload = append([]byte(nil), payload...)
	}
	return DeadLetterRecord{SourceInstanceID: cfg.SourceInstanceID, Topic: event.Topic, SourceEventID: event.ID, EventType: event.EventType, ObservedAt: observedAt, PayloadHash: notification.PayloadHash(payload), PayloadSizeBytes: len(payload), PayloadRefKind: payloadRefKind, RawPayload: rawPayload, SafePayloadPreview: preview, CauseKind: causeKind, CauseRedacted: causeRedacted, ReplayEligible: replayEligible, ReplayStatus: replayStatus, RedactionState: map[string]any{"status": "redacted", "categories": categories}, CreatedAt: observedAt, UpdatedAt: observedAt}
}

func (s *Store) CreateDeadLetter(ctx context.Context, record DeadLetterRecord) (DeadLetterRecord, error) {
	if s == nil || s.pool == nil {
		return DeadLetterRecord{}, fmt.Errorf("ntfy store: postgres pool is required")
	}
	if strings.TrimSpace(record.SourceInstanceID) == "" {
		return DeadLetterRecord{}, fmt.Errorf("ntfy dead letter: source instance id is required")
	}
	if record.ObservedAt.IsZero() || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return DeadLetterRecord{}, fmt.Errorf("ntfy dead letter: timestamps are required")
	}
	if record.ID == "" {
		record.ID = "ntfy_dlq_" + uuid.NewString()
	}
	if record.PayloadHash == "" {
		record.PayloadHash = notification.PayloadHash(record.RawPayload)
	}
	if record.ReplayStatus == "" {
		if record.ReplayEligible {
			record.ReplayStatus = ReplayStatusPending
		} else {
			record.ReplayStatus = ReplayStatusNotReplayable
		}
	}
	if record.PayloadRefKind == "" {
		record.PayloadRefKind = PayloadRefHashOnly
	}
	redactionJSON, err := json.Marshal(record.RedactionState)
	if err != nil {
		return DeadLetterRecord{}, fmt.Errorf("marshal ntfy dead letter redaction state: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_ntfy_dead_letters (
    id, source_instance_id, topic, source_event_id, event_type, observed_at,
    payload_hash, payload_size_bytes, payload_ref_kind, raw_payload_bytes,
    source_raw_event_id, safe_payload_preview, cause_kind, cause_redacted,
    replay_eligible, replay_status, attempt_count, last_attempt_at,
    redaction_state, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
		record.ID, record.SourceInstanceID, nullableString(record.Topic), nullableString(record.SourceEventID), nullableString(record.EventType), record.ObservedAt,
		record.PayloadHash, record.PayloadSizeBytes, record.PayloadRefKind, record.RawPayload, nullableString(record.SourceRawEventID), record.SafePayloadPreview,
		record.CauseKind, record.CauseRedacted, record.ReplayEligible, record.ReplayStatus, record.AttemptCount, record.LastAttemptAt, redactionJSON, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		return DeadLetterRecord{}, fmt.Errorf("insert ntfy dead letter: %w", err)
	}
	return record, nil
}

func (s *Store) CountDeadLetters(ctx context.Context, sourceInstanceID string) (int, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("ntfy store: postgres pool is required")
	}
	if strings.TrimSpace(sourceInstanceID) == "" {
		return 0, fmt.Errorf("ntfy dead letter count: source instance id is required")
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notification_ntfy_dead_letters WHERE source_instance_id = $1`, sourceInstanceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count ntfy dead letters: %w", err)
	}
	return count, nil
}

func (s *Store) ListDeadLetters(ctx context.Context, sourceInstanceID string, limit int, cursor string) (DeadLetterPage, error) {
	if s == nil || s.pool == nil {
		return DeadLetterPage{}, fmt.Errorf("ntfy store: postgres pool is required")
	}
	if limit < 1 {
		return DeadLetterPage{}, fmt.Errorf("ntfy dead letter list: positive limit is required")
	}
	rows, err := s.pool.Query(ctx, `
WITH cursor_row AS (
    SELECT observed_at, id FROM notification_ntfy_dead_letters WHERE id = $3 AND source_instance_id = $1
)
SELECT id, source_instance_id, COALESCE(topic,''), COALESCE(source_event_id,''), COALESCE(event_type,''), observed_at,
       payload_hash, payload_size_bytes, payload_ref_kind, raw_payload_bytes, COALESCE(source_raw_event_id,''),
       safe_payload_preview, cause_kind, cause_redacted, replay_eligible, replay_status, attempt_count,
       last_attempt_at, redaction_state, created_at, updated_at
FROM notification_ntfy_dead_letters dlq
WHERE dlq.source_instance_id = $1
  AND ($3 = '' OR EXISTS (
      SELECT 1 FROM cursor_row c
      WHERE dlq.observed_at < c.observed_at OR (dlq.observed_at = c.observed_at AND dlq.id > c.id)
  ))
ORDER BY dlq.observed_at DESC, dlq.id ASC
LIMIT $2`, sourceInstanceID, limit+1, cursor)
	if err != nil {
		return DeadLetterPage{}, fmt.Errorf("list ntfy dead letters: %w", err)
	}
	defer rows.Close()
	records, err := scanDeadLetters(rows)
	if err != nil {
		return DeadLetterPage{}, err
	}
	page := DeadLetterPage{Records: records}
	if len(records) > limit {
		page.NextCursor = records[limit-1].ID
		page.Records = records[:limit]
	}
	return page, nil
}

func (s *Store) GetDeadLetter(ctx context.Context, sourceInstanceID string, id string) (DeadLetterRecord, error) {
	if s == nil || s.pool == nil {
		return DeadLetterRecord{}, fmt.Errorf("ntfy store: postgres pool is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, source_instance_id, COALESCE(topic,''), COALESCE(source_event_id,''), COALESCE(event_type,''), observed_at,
       payload_hash, payload_size_bytes, payload_ref_kind, raw_payload_bytes, COALESCE(source_raw_event_id,''),
       safe_payload_preview, cause_kind, cause_redacted, replay_eligible, replay_status, attempt_count,
       last_attempt_at, redaction_state, created_at, updated_at
FROM notification_ntfy_dead_letters
WHERE source_instance_id = $1 AND id = $2`, sourceInstanceID, id)
	if err != nil {
		return DeadLetterRecord{}, fmt.Errorf("get ntfy dead letter: %w", err)
	}
	defer rows.Close()
	records, err := scanDeadLetters(rows)
	if err != nil {
		return DeadLetterRecord{}, err
	}
	if len(records) == 0 {
		return DeadLetterRecord{}, pgx.ErrNoRows
	}
	return records[0], nil
}

func (s *Store) ReplayDeadLetter(ctx context.Context, cfg Config, id string, sink notification.SourceEventSink, actorRef string, observedAt time.Time) (ReplayAttemptRecord, error) {
	if s == nil || s.pool == nil {
		return ReplayAttemptRecord{}, fmt.Errorf("ntfy store: postgres pool is required")
	}
	if sink == nil {
		return ReplayAttemptRecord{}, fmt.Errorf("ntfy replay: source event sink is required")
	}
	if strings.TrimSpace(actorRef) == "" {
		return ReplayAttemptRecord{}, fmt.Errorf("ntfy replay: actor ref is required")
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReplayAttemptRecord{}, fmt.Errorf("begin ntfy replay transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	record, err := s.getDeadLetterForReplay(ctx, tx, cfg.SourceInstanceID, id)
	if err != nil {
		return ReplayAttemptRecord{}, err
	}
	attempt := ReplayAttemptRecord{DeadLetterID: record.ID, SourceInstanceID: record.SourceInstanceID, IdempotencyKey: replayIdempotencyKey(record), ActorKind: "operator", ActorRef: actorRef, AttemptedAt: observedAt}
	if record.ReplayStatus == ReplayStatusReplayed {
		existing, err := s.getReplayAttemptByKey(ctx, tx, attempt.IdempotencyKey)
		if err != nil {
			return ReplayAttemptRecord{}, err
		}
		existing.AlreadyReplayed = true
		if err := tx.Commit(ctx); err != nil {
			return ReplayAttemptRecord{}, fmt.Errorf("commit ntfy already-replayed transaction: %w", err)
		}
		return existing, nil
	}
	if !record.ReplayEligible || len(record.RawPayload) == 0 {
		attempt.Status = "rejected"
		attempt.SinkStatus = "not_replayable"
		attempt.ErrorKind = "not_replayable"
		attempt.ErrorRedacted = "dead letter is not replay eligible"
		return s.finishReplayAttempt(ctx, tx, attempt, ReplayStatusNotReplayable, observedAt, record.ID)
	}
	event, err := ParseEvent(record.RawPayload, cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		attempt.Status = "failed"
		attempt.SinkStatus = "parse_failed"
		attempt.ErrorKind = DeadLetterMalformedJSON
		attempt.ErrorRedacted = "dead-letter payload could not be parsed for replay"
		return s.finishReplayAttempt(ctx, tx, attempt, ReplayStatusFailed, observedAt, record.ID)
	}
	envelope, err := MapEvent(ctx, cfg, event, observedAt)
	if err != nil {
		attempt.Status = "rejected"
		attempt.SinkStatus = "mapping_failed"
		attempt.ErrorKind = DeadLetterUnsupportedEvent
		attempt.ErrorRedacted = "dead-letter payload could not be mapped for replay"
		return s.finishReplayAttempt(ctx, tx, attempt, ReplayStatusFailed, observedAt, record.ID)
	}
	receipt, err := sink.SubmitSourceEvent(ctx, envelope)
	if err != nil {
		attempt.Status = "failed"
		attempt.SinkStatus = "sink_unavailable"
		attempt.ErrorKind = DeadLetterSinkUnavailable
		attempt.ErrorRedacted = "source sink was unavailable during replay"
		return s.finishReplayAttempt(ctx, tx, attempt, ReplayStatusFailed, observedAt, record.ID)
	}
	attempt.Status = "rejected"
	replayStatus := ReplayStatusFailed
	if receipt.Accepted {
		attempt.Status = "accepted"
		replayStatus = ReplayStatusReplayed
	}
	attempt.RawEventID = receipt.RawEventID
	attempt.SinkStatus = receipt.Status
	return s.finishReplayAttempt(ctx, tx, attempt, replayStatus, observedAt, record.ID)
}

func (s *Store) finishReplayAttempt(ctx context.Context, tx pgx.Tx, attempt ReplayAttemptRecord, replayStatus string, observedAt time.Time, deadLetterID string) (ReplayAttemptRecord, error) {
	storedAttempt, err := recordReplayAttempt(ctx, tx, attempt)
	if err != nil {
		return storedAttempt, err
	}
	_, err = tx.Exec(ctx, `UPDATE notification_ntfy_dead_letters SET replay_status = $1, attempt_count = attempt_count + 1, last_attempt_at = $2, updated_at = $2 WHERE id = $3`, replayStatus, observedAt, deadLetterID)
	if err != nil {
		return storedAttempt, fmt.Errorf("update ntfy dead letter replay status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return storedAttempt, fmt.Errorf("commit ntfy replay transaction: %w", err)
	}
	return storedAttempt, nil
}

type replayAttemptExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type replayAttemptQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func recordReplayAttempt(ctx context.Context, executor replayAttemptExecutor, attempt ReplayAttemptRecord) (ReplayAttemptRecord, error) {
	if attempt.ID == "" {
		attempt.ID = "ntfy_replay_" + uuid.NewString()
	}
	if strings.TrimSpace(attempt.ActorKind) == "" {
		return ReplayAttemptRecord{}, fmt.Errorf("ntfy replay attempt: actor kind is required")
	}
	if strings.TrimSpace(attempt.ActorRef) == "" {
		return ReplayAttemptRecord{}, fmt.Errorf("ntfy replay attempt: actor ref is required")
	}
	_, err := executor.Exec(ctx, `
INSERT INTO notification_ntfy_replay_attempts (
    id, dead_letter_id, source_instance_id, idempotency_key, actor_kind, actor_ref,
    status, raw_event_id, sink_status, error_kind, error_redacted, attempted_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (idempotency_key) DO UPDATE SET
    status = EXCLUDED.status,
    raw_event_id = EXCLUDED.raw_event_id,
    sink_status = EXCLUDED.sink_status,
    error_kind = EXCLUDED.error_kind,
    error_redacted = EXCLUDED.error_redacted,
    attempted_at = EXCLUDED.attempted_at`, attempt.ID, attempt.DeadLetterID, attempt.SourceInstanceID, attempt.IdempotencyKey, attempt.ActorKind, attempt.ActorRef, attempt.Status, nullableString(attempt.RawEventID), attempt.SinkStatus, nullableString(attempt.ErrorKind), nullableString(attempt.ErrorRedacted), attempt.AttemptedAt)
	if err != nil {
		return ReplayAttemptRecord{}, fmt.Errorf("record ntfy replay attempt: %w", err)
	}
	return attempt, nil
}

func (s *Store) getDeadLetterForReplay(ctx context.Context, tx pgx.Tx, sourceInstanceID string, id string) (DeadLetterRecord, error) {
	rows, err := tx.Query(ctx, `
SELECT id, source_instance_id, COALESCE(topic,''), COALESCE(source_event_id,''), COALESCE(event_type,''), observed_at,
       payload_hash, payload_size_bytes, payload_ref_kind, raw_payload_bytes, COALESCE(source_raw_event_id,''),
       safe_payload_preview, cause_kind, cause_redacted, replay_eligible, replay_status, attempt_count,
       last_attempt_at, redaction_state, created_at, updated_at
FROM notification_ntfy_dead_letters
WHERE source_instance_id = $1 AND id = $2
FOR UPDATE`, sourceInstanceID, id)
	if err != nil {
		return DeadLetterRecord{}, fmt.Errorf("lock ntfy dead letter for replay: %w", err)
	}
	defer rows.Close()
	records, err := scanDeadLetters(rows)
	if err != nil {
		return DeadLetterRecord{}, err
	}
	if len(records) == 0 {
		return DeadLetterRecord{}, pgx.ErrNoRows
	}
	return records[0], nil
}

func (s *Store) getReplayAttemptByKey(ctx context.Context, queryer replayAttemptQueryer, key string) (ReplayAttemptRecord, error) {
	var attempt ReplayAttemptRecord
	if err := queryer.QueryRow(ctx, `
SELECT id, dead_letter_id, source_instance_id, idempotency_key, actor_kind, actor_ref,
       status, COALESCE(raw_event_id,''), sink_status, COALESCE(error_kind,''), COALESCE(error_redacted,''), attempted_at
FROM notification_ntfy_replay_attempts
WHERE idempotency_key = $1`, key).Scan(&attempt.ID, &attempt.DeadLetterID, &attempt.SourceInstanceID, &attempt.IdempotencyKey, &attempt.ActorKind, &attempt.ActorRef, &attempt.Status, &attempt.RawEventID, &attempt.SinkStatus, &attempt.ErrorKind, &attempt.ErrorRedacted, &attempt.AttemptedAt); err != nil {
		if err == pgx.ErrNoRows {
			return ReplayAttemptRecord{}, fmt.Errorf("ntfy replay: replayed dead letter is missing replay attempt audit row")
		}
		return ReplayAttemptRecord{}, fmt.Errorf("load ntfy replay attempt: %w", err)
	}
	return attempt, nil
}

func scanSubscriptionStates(rows pgx.Rows) ([]SubscriptionState, error) {
	states := []SubscriptionState{}
	for rows.Next() {
		var state SubscriptionState
		var form string
		var redactionJSON []byte
		if err := rows.Scan(&state.SourceInstanceID, &state.Topic, &form, &state.TransportMode, &state.SubscriptionState, &state.LastNtfyEventID, &state.LastEventAt, &state.LastOpenAt, &state.LastKeepaliveAt, &state.LastSuccessfulCheckAt, &state.LagSeconds, &state.PossibleGap, &state.RetryCount, &state.RetryBudget, &state.LastErrorKind, &state.LastErrorRedacted, &redactionJSON, &state.CreatedAt, &state.UpdatedAt); err != nil {
			return nil, err
		}
		state.SourceForm = notification.SourceForm(form)
		redactionState, err := decodeNtfyRedactionState(redactionJSON, fmt.Sprintf("subscription redaction state for source_instance_id=%s topic=%s", state.SourceInstanceID, state.Topic))
		if err != nil {
			return nil, err
		}
		state.RedactionState = redactionState
		states = append(states, state)
	}
	return states, rows.Err()
}

func scanDeadLetters(rows pgx.Rows) ([]DeadLetterRecord, error) {
	records := []DeadLetterRecord{}
	for rows.Next() {
		var record DeadLetterRecord
		var redactionJSON []byte
		if err := rows.Scan(&record.ID, &record.SourceInstanceID, &record.Topic, &record.SourceEventID, &record.EventType, &record.ObservedAt, &record.PayloadHash, &record.PayloadSizeBytes, &record.PayloadRefKind, &record.RawPayload, &record.SourceRawEventID, &record.SafePayloadPreview, &record.CauseKind, &record.CauseRedacted, &record.ReplayEligible, &record.ReplayStatus, &record.AttemptCount, &record.LastAttemptAt, &redactionJSON, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, err
		}
		redactionState, err := decodeNtfyRedactionState(redactionJSON, fmt.Sprintf("dead-letter redaction state for id=%s source_instance_id=%s", record.ID, record.SourceInstanceID))
		if err != nil {
			return nil, err
		}
		record.RedactionState = redactionState
		records = append(records, record)
	}
	return records, rows.Err()
}

func decodeNtfyRedactionState(redactionJSON []byte, owner string) (map[string]any, error) {
	var redactionState map[string]any
	if err := json.Unmarshal(redactionJSON, &redactionState); err != nil {
		return nil, fmt.Errorf("decode ntfy %s: %w", owner, err)
	}
	if redactionState == nil {
		return nil, fmt.Errorf("decode ntfy %s: expected JSON object", owner)
	}
	return redactionState, nil
}

func replayIdempotencyKey(record DeadLetterRecord) string {
	return notification.PayloadHash([]byte(strings.Join([]string{record.SourceInstanceID, record.Topic, record.SourceEventID, record.PayloadHash, record.ID}, "\x00")))
}

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
