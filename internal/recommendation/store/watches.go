package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/recommendation/policy"
)

// WatchInput is the persistence shape for a recommendation watch row, matching
// the public design contract in design.md (Watch Create/Edit Schema).
type WatchInput struct {
	ID                 string
	ActorUserID        string
	Name               string
	Kind               string // location_radius|topic_keyword|trip_context|price_drop
	Enabled            bool
	Scope              map[string]any
	Filters            map[string]any
	AllowedSources     []string
	Schedule           map[string]any
	MaxAlertsPerWindow int
	AlertWindowSeconds int
	CooldownSeconds    int
	QuietHours         map[string]any
	LocationPrecision  string
	DeliveryChannel    string
	QueuePolicy        string // queue|summarize|drop
	FreshnessSeconds   int
}

// WatchRecord is the stored watch row plus its current consent record.
type WatchRecord struct {
	WatchInput
	Consent      policy.ConsentRecord
	LastRunAt    *time.Time
	NextDueAt    *time.Time
	SilenceUntil *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// CreateWatch persists a brand-new watch with a freshly-applied consent
// revision. Caller MUST already have validated the consent confirmation against
// the consent decision. Returns the persisted watch record (including computed
// next_due_at).
func (s *Store) CreateWatch(ctx context.Context, input WatchInput, consent policy.ConsentRecord, now time.Time) (WatchRecord, error) {
	if s == nil || s.pool == nil {
		return WatchRecord{}, fmt.Errorf("recommendation store: postgres pool is required")
	}
	if input.ID == "" {
		id, err := newTextID("rec_watch")
		if err != nil {
			return WatchRecord{}, err
		}
		input.ID = id
	}
	consentJSON, err := policy.MarshalConsentRecord(consent)
	if err != nil {
		return WatchRecord{}, fmt.Errorf("marshal consent: %w", err)
	}
	scopeJSON, err := marshalAny(input.Scope)
	if err != nil {
		return WatchRecord{}, err
	}
	filtersJSON, err := marshalAny(input.Filters)
	if err != nil {
		return WatchRecord{}, err
	}
	scheduleJSON, err := marshalAny(input.Schedule)
	if err != nil {
		return WatchRecord{}, err
	}
	quietJSON, err := marshalAny(input.QuietHours)
	if err != nil {
		return WatchRecord{}, err
	}
	if input.AllowedSources == nil {
		input.AllowedSources = []string{}
	}
	nextDue := computeNextDue(input, now)
	_, err = s.pool.Exec(ctx, `
INSERT INTO recommendation_watches (
    id, actor_user_id, name, kind, enabled, consent, scope, filters, allowed_sources,
    schedule, max_alerts_per_window, alert_window_seconds, cooldown_seconds,
    quiet_hours, location_precision, delivery_channel, queue_policy, created_at, updated_at,
    next_due_at, freshness_seconds, queue_state
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19,
    $20, $21, '{}'::jsonb
)`,
		input.ID,
		input.ActorUserID,
		input.Name,
		input.Kind,
		input.Enabled,
		consentJSON,
		scopeJSON,
		filtersJSON,
		input.AllowedSources,
		scheduleJSON,
		input.MaxAlertsPerWindow,
		input.AlertWindowSeconds,
		input.CooldownSeconds,
		quietJSON,
		input.LocationPrecision,
		input.DeliveryChannel,
		input.QueuePolicy,
		now,
		now,
		nextDue,
		nullableFreshness(input.FreshnessSeconds),
	)
	if err != nil {
		return WatchRecord{}, fmt.Errorf("insert recommendation watch: %w", err)
	}
	return s.GetWatch(ctx, input.ID)
}

// UpdateWatchWithConsent updates a watch and writes a new consent revision in
// the same transaction. The caller must have already validated the consent
// decision via policy.CheckConfirmation.
func (s *Store) UpdateWatchWithConsent(ctx context.Context, input WatchInput, consent policy.ConsentRecord, now time.Time) (WatchRecord, error) {
	if s == nil || s.pool == nil {
		return WatchRecord{}, fmt.Errorf("recommendation store: postgres pool is required")
	}
	if input.ID == "" {
		return WatchRecord{}, fmt.Errorf("update watch: id required")
	}
	consentJSON, err := policy.MarshalConsentRecord(consent)
	if err != nil {
		return WatchRecord{}, fmt.Errorf("marshal consent: %w", err)
	}
	scopeJSON, _ := marshalAny(input.Scope)
	filtersJSON, _ := marshalAny(input.Filters)
	scheduleJSON, _ := marshalAny(input.Schedule)
	quietJSON, _ := marshalAny(input.QuietHours)
	if input.AllowedSources == nil {
		input.AllowedSources = []string{}
	}
	nextDue := computeNextDue(input, now)
	tag, err := s.pool.Exec(ctx, `
UPDATE recommendation_watches
SET name = $2, kind = $3, enabled = $4, consent = $5, scope = $6, filters = $7,
    allowed_sources = $8, schedule = $9, max_alerts_per_window = $10,
    alert_window_seconds = $11, cooldown_seconds = $12, quiet_hours = $13,
    location_precision = $14, delivery_channel = $15, queue_policy = $16,
    freshness_seconds = $17, next_due_at = $18, updated_at = $19
WHERE id = $1 AND deleted_at IS NULL`,
		input.ID,
		input.Name,
		input.Kind,
		input.Enabled,
		consentJSON,
		scopeJSON,
		filtersJSON,
		input.AllowedSources,
		scheduleJSON,
		input.MaxAlertsPerWindow,
		input.AlertWindowSeconds,
		input.CooldownSeconds,
		quietJSON,
		input.LocationPrecision,
		input.DeliveryChannel,
		input.QueuePolicy,
		nullableFreshness(input.FreshnessSeconds),
		nextDue,
		now,
	)
	if err != nil {
		return WatchRecord{}, fmt.Errorf("update recommendation watch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return WatchRecord{}, fmt.Errorf("watch %s not found", input.ID)
	}
	return s.GetWatch(ctx, input.ID)
}

// GetWatch loads one watch by id, including the parsed consent record.
func (s *Store) GetWatch(ctx context.Context, id string) (WatchRecord, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, actor_user_id, name, kind, enabled, consent, scope, filters, allowed_sources,
       schedule, max_alerts_per_window, alert_window_seconds, cooldown_seconds,
       quiet_hours, location_precision, delivery_channel, queue_policy,
       freshness_seconds, last_run_at, next_due_at, silence_until,
       created_at, updated_at, deleted_at
FROM recommendation_watches
WHERE id = $1`, id)
	return scanWatchRow(row)
}

// ListWatches returns all non-deleted watches for a user, ordered by name.
func (s *Store) ListWatches(ctx context.Context, actorUserID string) ([]WatchRecord, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, actor_user_id, name, kind, enabled, consent, scope, filters, allowed_sources,
       schedule, max_alerts_per_window, alert_window_seconds, cooldown_seconds,
       quiet_hours, location_precision, delivery_channel, queue_policy,
       freshness_seconds, last_run_at, next_due_at, silence_until,
       created_at, updated_at, deleted_at
FROM recommendation_watches
WHERE actor_user_id = $1 AND deleted_at IS NULL
ORDER BY name ASC, id ASC`, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("list recommendation watches: %w", err)
	}
	defer rows.Close()
	var records []WatchRecord
	for rows.Next() {
		record, err := scanWatchRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// DueWatches returns enabled, non-silenced watches whose next_due_at is at or
// before "as of". Ordered by next_due_at to keep eldest first.
func (s *Store) DueWatches(ctx context.Context, asOf time.Time) ([]WatchRecord, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, actor_user_id, name, kind, enabled, consent, scope, filters, allowed_sources,
       schedule, max_alerts_per_window, alert_window_seconds, cooldown_seconds,
       quiet_hours, location_precision, delivery_channel, queue_policy,
       freshness_seconds, last_run_at, next_due_at, silence_until,
       created_at, updated_at, deleted_at
FROM recommendation_watches
WHERE enabled = true
  AND deleted_at IS NULL
  AND (silence_until IS NULL OR silence_until <= $1)
  AND (next_due_at IS NULL OR next_due_at <= $1)
ORDER BY COALESCE(next_due_at, created_at) ASC, id ASC`, asOf)
	if err != nil {
		return nil, fmt.Errorf("query due watches: %w", err)
	}
	defer rows.Close()
	var records []WatchRecord
	for rows.Next() {
		record, err := scanWatchRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// PauseWatch sets enabled=false.
func (s *Store) PauseWatch(ctx context.Context, id string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE recommendation_watches SET enabled = false, updated_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, now)
	if err != nil {
		return fmt.Errorf("pause watch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("watch %s not found", id)
	}
	return nil
}

// ResumeWatch sets enabled=true.
func (s *Store) ResumeWatch(ctx context.Context, id string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE recommendation_watches SET enabled = true, updated_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, now)
	if err != nil {
		return fmt.Errorf("resume watch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("watch %s not found", id)
	}
	return nil
}

// SilenceWatch sets silence_until.
func (s *Store) SilenceWatch(ctx context.Context, id string, until time.Time, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE recommendation_watches SET silence_until = $2, updated_at = $3 WHERE id = $1 AND deleted_at IS NULL`, id, until, now)
	if err != nil {
		return fmt.Errorf("silence watch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("watch %s not found", id)
	}
	return nil
}

// DeleteWatch soft-deletes by setting deleted_at and disables the watch.
func (s *Store) DeleteWatch(ctx context.Context, id string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE recommendation_watches SET deleted_at = $2, enabled = false, updated_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, now)
	if err != nil {
		return fmt.Errorf("delete watch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("watch %s not found", id)
	}
	return nil
}

// FindWatchByName looks up a single non-deleted watch by name for an actor.
// Returns ErrNoRows-equivalent error when not found.
func (s *Store) FindWatchByName(ctx context.Context, actorUserID, name string) (WatchRecord, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, actor_user_id, name, kind, enabled, consent, scope, filters, allowed_sources,
       schedule, max_alerts_per_window, alert_window_seconds, cooldown_seconds,
       quiet_hours, location_precision, delivery_channel, queue_policy,
       freshness_seconds, last_run_at, next_due_at, silence_until,
       created_at, updated_at, deleted_at
FROM recommendation_watches
WHERE actor_user_id = $1 AND name = $2 AND deleted_at IS NULL
LIMIT 1`, actorUserID, name)
	rec, err := scanWatchRow(row)
	if err != nil {
		return WatchRecord{}, err
	}
	return rec, nil
}

// WatchRunInput is the persisted run-row for one watch evaluation.
type WatchRunInput struct {
	WatchID           string
	ScenarioID        string
	TraceID           string
	TriggerKind       string
	TriggerContext    map[string]any
	Status            string
	ProviderStatus    []map[string]any
	RawCandidateCount int
	DeliveredCount    int
	WithheldCount     int
	DeliveryDecision  string
	ErrorKind         string
	StartedAt         time.Time
	CompletedAt       time.Time
}

// WatchRunRecord is the persisted run row read back for audit views.
type WatchRunRecord struct {
	ID               string
	WatchID          string
	ScenarioID       string
	TraceID          string
	TriggerKind      string
	Status           string
	DeliveryDecision string
	ErrorKind        string
	RawCandidates    int
	Delivered        int
	Withheld         int
	StartedAt        time.Time
	CompletedAt      *time.Time
}

// PersistWatchRun writes one watch_run audit row, advances last_run_at and
// next_due_at on the watch, and increments the rate window when the run
// produced deliveries or withheld counts.
func (s *Store) PersistWatchRun(ctx context.Context, input WatchRunInput) (string, error) {
	if input.WatchID == "" {
		return "", fmt.Errorf("PersistWatchRun: watch id required")
	}
	if input.StartedAt.IsZero() {
		return "", fmt.Errorf("PersistWatchRun: started_at required")
	}
	runID, err := newTextID("rec_watch_run")
	if err != nil {
		return "", err
	}
	triggerJSON, err := marshalAny(input.TriggerContext)
	if err != nil {
		return "", err
	}
	providerJSON, err := marshalAny(input.ProviderStatus)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin watch run tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	completedAt := input.CompletedAt
	if completedAt.IsZero() {
		completedAt = input.StartedAt
	}
	traceRef := nullableText(input.TraceID)
	deliveryRef := nullableText(input.DeliveryDecision)
	errorRef := nullableText(input.ErrorKind)
	_, err = tx.Exec(ctx, `
INSERT INTO recommendation_watch_runs (
    id, watch_id, scenario_id, trace_id, trigger_kind, trigger_context,
    status, provider_status, raw_candidate_count, delivered_count, withheld_count,
    started_at, completed_at, delivery_decision, error_kind
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		runID,
		input.WatchID,
		input.ScenarioID,
		traceRef,
		input.TriggerKind,
		triggerJSON,
		input.Status,
		providerJSON,
		input.RawCandidateCount,
		input.DeliveredCount,
		input.WithheldCount,
		input.StartedAt,
		completedAt,
		deliveryRef,
		errorRef,
	)
	if err != nil {
		return "", fmt.Errorf("insert recommendation watch run: %w", err)
	}

	// Always update last_run_at and next_due_at; the next_due_at is
	// computed inside the SQL using the watch schedule + completion.
	_, err = tx.Exec(ctx, `
UPDATE recommendation_watches
SET last_run_at = $2::timestamptz,
    next_due_at = CASE
      WHEN alert_window_seconds > 0 THEN $2::timestamptz + (alert_window_seconds || ' seconds')::interval
      ELSE NULL
    END,
    updated_at = $2::timestamptz
WHERE id = $1`, input.WatchID, completedAt)
	if err != nil {
		return "", fmt.Errorf("advance watch schedule: %w", err)
	}

	if input.DeliveredCount > 0 || input.WithheldCount > 0 {
		windowStart := input.StartedAt.UTC().Truncate(time.Second)
		_, err = tx.Exec(ctx, `
INSERT INTO recommendation_watch_rate_windows (watch_id, window_start, delivered_count, withheld_count)
VALUES ($1, $2, $3, $4)
ON CONFLICT (watch_id, window_start)
DO UPDATE SET
    delivered_count = recommendation_watch_rate_windows.delivered_count + EXCLUDED.delivered_count,
    withheld_count = recommendation_watch_rate_windows.withheld_count + EXCLUDED.withheld_count`,
			input.WatchID,
			windowStart,
			input.DeliveredCount,
			input.WithheldCount,
		)
		if err != nil {
			return "", fmt.Errorf("update watch rate window: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit watch run: %w", err)
	}
	return runID, nil
}

// CountDeliveredInRateWindow returns the total delivered_count across all
// rate-window rows whose window_start is at or after windowStart for the
// given watch.
func (s *Store) CountDeliveredInRateWindow(ctx context.Context, watchID string, windowStart time.Time) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
SELECT COALESCE(SUM(delivered_count), 0)
FROM recommendation_watch_rate_windows
WHERE watch_id = $1 AND window_start >= $2`, watchID, windowStart).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count delivered rate window: %w", err)
	}
	return count, nil
}

// SeenStateInput is the upsert payload for repeat-cooldown tracking. The
// candidate is identified by its (category, canonical_key) pair so the seen
// state can reference the persisted recommendation_candidates.id row written
// inside the same outcome transaction.
type SeenStateInput struct {
	ActorUserID        string
	ContextKey         string
	Category           string
	CanonicalKey       string
	MaterialChangeHash string
	CooldownUntil      *time.Time
	Now                time.Time
}

// SeenStateRecord is the read-back row.
type SeenStateRecord struct {
	ActorUserID        string
	ContextKey         string
	CandidateID        string
	MaterialChangeHash string
	CooldownUntil      *time.Time
	DeliveryCount      int
	FirstSeenAt        time.Time
	LastSeenAt         time.Time
}

// GetSeenState reads the persisted seen-state row if any. The candidate is
// identified by its (category, canonical_key) pair which is resolved to the
// canonical recommendation_candidates.id inside the query.
func (s *Store) GetSeenState(ctx context.Context, actorUserID, contextKey, category, canonicalKey string) (SeenStateRecord, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT s.actor_user_id, s.context_key, s.candidate_id, s.material_change_hash,
       s.cooldown_until, s.delivery_count, s.first_seen_at, s.last_seen_at
FROM recommendation_seen_state s
JOIN recommendation_candidates c ON c.id = s.candidate_id
WHERE s.actor_user_id = $1 AND s.context_key = $2
  AND c.category = $3 AND c.canonical_key = $4`, actorUserID, contextKey, category, canonicalKey)
	var rec SeenStateRecord
	err := row.Scan(&rec.ActorUserID, &rec.ContextKey, &rec.CandidateID, &rec.MaterialChangeHash, &rec.CooldownUntil, &rec.DeliveryCount, &rec.FirstSeenAt, &rec.LastSeenAt)
	if err == pgx.ErrNoRows {
		return SeenStateRecord{}, false, nil
	}
	if err != nil {
		return SeenStateRecord{}, false, fmt.Errorf("load seen state: %w", err)
	}
	return rec, true, nil
}

// UpsertSeenState writes or updates the seen-state row for a delivered
// recommendation, incrementing the delivery count. The candidate row must
// already exist in recommendation_candidates (PersistWatchOutcome writes it
// before the evaluator records seen state).
func (s *Store) UpsertSeenState(ctx context.Context, input SeenStateInput) error {
	id, err := newTextID("rec_seen")
	if err != nil {
		return err
	}
	res, err := s.pool.Exec(ctx, `
INSERT INTO recommendation_seen_state (
    id, actor_user_id, context_key, candidate_id, first_seen_at, last_seen_at,
    material_change_hash, delivery_count, cooldown_until
)
SELECT $1, $2, $3, c.id, $6::timestamptz, $6::timestamptz, $4, 1, $5
FROM recommendation_candidates c
WHERE c.category = $7 AND c.canonical_key = $8
ON CONFLICT (actor_user_id, context_key, candidate_id) DO UPDATE
SET last_seen_at = EXCLUDED.last_seen_at,
    material_change_hash = EXCLUDED.material_change_hash,
    delivery_count = recommendation_seen_state.delivery_count + 1,
    cooldown_until = EXCLUDED.cooldown_until`,
		id, input.ActorUserID, input.ContextKey, input.MaterialChangeHash, input.CooldownUntil, input.Now, input.Category, input.CanonicalKey,
	)
	if err != nil {
		return fmt.Errorf("upsert seen state: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("upsert seen state: candidate not found for category=%s canonical_key=%s", input.Category, input.CanonicalKey)
	}
	return nil
}

// PersistWatchOutcomeInput captures the recommendation/withheld rows produced
// by a single watch evaluation. Provider facts are not persisted by Scope 4
// because the watch evaluator works against in-memory facts (real provider
// integration arrives in later scopes).
type PersistWatchOutcomeInput struct {
	WatchID          string
	WatchRunID       string
	ActorUserID      string
	ScenarioID       string
	ScenarioVersion  string
	ScenarioHash     string
	TraceID          string
	StartedAt        time.Time
	CompletedAt      time.Time
	Source           string
	Status           string
	ToolCalls        []ToolCallRecord
	ProviderFacts    []ProviderFactInput
	Candidates       []CandidateInput
	Recommendations  []RecommendationInput
	TriggerContext   map[string]any
	DeliveryDecision string
}

// PersistWatchOutcome persists the trace, provider facts, candidates, and
// recommendation rows for one watch evaluation, with each recommendation row
// linked to the originating watch_run.
func (s *Store) PersistWatchOutcome(ctx context.Context, input PersistWatchOutcomeInput) ([]string, error) {
	if input.WatchID == "" || input.WatchRunID == "" {
		return nil, fmt.Errorf("PersistWatchOutcome: watch_id and watch_run_id required")
	}
	if input.StartedAt.IsZero() {
		input.StartedAt = time.Now().UTC()
	}
	if input.CompletedAt.IsZero() {
		input.CompletedAt = time.Now().UTC()
	}
	scenarioSnapshot, _ := marshalObject(map[string]any{"id": input.ScenarioID, "scope": "scope-04-watches-and-scheduler"})
	inputEnvelope, _ := marshalObject(map[string]any{
		"watch_id":        input.WatchID,
		"watch_run_id":    input.WatchRunID,
		"trigger_context": input.TriggerContext,
	})
	routing, _ := marshalObject(map[string]any{"scenario_id": input.ScenarioID, "status": input.Status})
	finalOutput, _ := marshalObject(map[string]any{"watch_run_id": input.WatchRunID, "status": input.Status})
	outcomeDetail, _ := marshalObject(map[string]any{"delivery_decision": input.DeliveryDecision})
	toolCallsJSON, err := marshalAny(input.ToolCalls)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin watch outcome tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	traceID := input.TraceID
	if traceID == "" {
		traceID, err = newTextID("rec_trace")
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(ctx, `
INSERT INTO agent_traces (
    trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
    source, input_envelope, routing, tool_calls, turn_log,
    final_output, outcome, outcome_detail,
    provider, model, tokens_prompt, tokens_completion,
    latency_ms, started_at, ended_at, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13,
    $14, $15, $16, $17,
    $18, $19, $20, $21
)`,
		traceID,
		input.ScenarioID,
		input.ScenarioVersion,
		input.ScenarioHash,
		scenarioSnapshot,
		input.Source,
		inputEnvelope,
		routing,
		toolCallsJSON,
		[]byte("[]"),
		finalOutput,
		input.Status,
		outcomeDetail,
		"",
		"",
		0,
		0,
		int(input.CompletedAt.Sub(input.StartedAt).Milliseconds()),
		input.StartedAt,
		input.CompletedAt,
		input.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert watch trace: %w", err)
	}
	for seq, call := range input.ToolCalls {
		argsJSON, _ := marshalAny(call.Arguments)
		resultJSON, _ := marshalAny(call.Result)
		started := call.StartedAt
		if started.IsZero() {
			started = input.StartedAt
		}
		_, err = tx.Exec(ctx, `
INSERT INTO agent_tool_calls (
    trace_id, seq, tool_name, side_effect_class, arguments, result,
    rejection_reason, error, latency_ms, started_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			traceID,
			seq+1,
			call.Name,
			call.SideEffectClass,
			argsJSON,
			resultJSON,
			call.RejectionReason,
			call.Error,
			call.LatencyMillis,
			started,
		)
		if err != nil {
			return nil, fmt.Errorf("insert watch tool call %s: %w", call.Name, err)
		}
	}

	// Persist provider facts linked to the watch_run_id.
	factIDs := map[string]string{}
	for _, fact := range input.ProviderFacts {
		factID, err := newTextID("rec_fact")
		if err != nil {
			return nil, err
		}
		normalized, _ := marshalAny(fact.NormalizedFact)
		attribution, _ := marshalAny(fact.Attribution)
		restricted, _ := marshalAny(fact.RestrictedFlags)
		retrievedAt := fact.RetrievedAt
		if retrievedAt.IsZero() {
			retrievedAt = input.StartedAt
		}
		sponsoredState := fact.SponsoredState
		if sponsoredState == "" {
			sponsoredState = "none"
		}
		var persistedID string
		err = tx.QueryRow(ctx, `
INSERT INTO recommendation_provider_facts (
    id, request_id, watch_run_id, provider_id, provider_candidate_id,
    category, normalized_fact, source_retrieved_at, source_updated_at,
    source_payload_hash, raw_payload_expires_at, attribution,
    sponsored_state, restricted_flags, created_at
) VALUES (
    $1, NULL, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11,
    $12, $13, $14
)
ON CONFLICT (provider_id, provider_candidate_id, source_retrieved_at)
DO UPDATE SET
    watch_run_id = EXCLUDED.watch_run_id,
    normalized_fact = EXCLUDED.normalized_fact
RETURNING id`,
			factID,
			input.WatchRunID,
			fact.ProviderID,
			fact.ProviderCandidateID,
			fact.Category,
			normalized,
			retrievedAt,
			fact.SourceUpdatedAt,
			payloadHash(normalized),
			input.CompletedAt.Add(24*time.Hour),
			attribution,
			sponsoredState,
			restricted,
			input.CompletedAt,
		).Scan(&persistedID)
		if err != nil {
			return nil, fmt.Errorf("insert watch provider fact %s: %w", fact.LocalID, err)
		}
		factIDs[fact.LocalID] = persistedID
	}

	candidateIDs := map[string]string{}
	for _, candidate := range input.Candidates {
		candidateID, err := newTextID("rec_cand")
		if err != nil {
			return nil, err
		}
		canonicalFact, _ := marshalAny(candidate.CanonicalFact)
		dedupeReason, _ := marshalAny(candidate.DedupeReason)
		var persistedID string
		err = tx.QueryRow(ctx, `
INSERT INTO recommendation_candidates (
    id, category, canonical_key, title, canonical_url,
    canonical_fact, dedupe_reason, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (category, canonical_key)
DO UPDATE SET title = EXCLUDED.title, canonical_url = EXCLUDED.canonical_url,
    canonical_fact = EXCLUDED.canonical_fact, dedupe_reason = EXCLUDED.dedupe_reason,
    updated_at = EXCLUDED.updated_at
RETURNING id`,
			candidateID,
			candidate.Category,
			candidate.CanonicalKey,
			candidate.Title,
			nullableText(candidate.CanonicalURL),
			canonicalFact,
			dedupeReason,
			input.CompletedAt,
			input.CompletedAt,
		).Scan(&persistedID)
		if err != nil {
			return nil, fmt.Errorf("insert watch candidate %s: %w", candidate.LocalID, err)
		}
		candidateIDs[candidate.LocalID] = persistedID
		for _, factLocalID := range candidate.ProviderFactLocalIDs {
			factID, ok := factIDs[factLocalID]
			if !ok {
				continue
			}
			_, err = tx.Exec(ctx, `
INSERT INTO recommendation_candidate_provider_facts (candidate_id, provider_fact_id, merge_reason)
VALUES ($1, $2, $3)
ON CONFLICT (candidate_id, provider_fact_id) DO UPDATE SET merge_reason = EXCLUDED.merge_reason`,
				persistedID, factID, "watch-evaluation",
			)
			if err != nil {
				return nil, fmt.Errorf("link watch candidate fact: %w", err)
			}
		}
	}

	recommendationIDs := []string{}
	for _, rec := range input.Recommendations {
		candidateID, ok := candidateIDs[rec.CandidateLocalID]
		if !ok {
			return nil, fmt.Errorf("watch recommendation references unknown candidate %s", rec.CandidateLocalID)
		}
		recID, err := newTextID("rec")
		if err != nil {
			return nil, err
		}
		scoreJSON, _ := marshalAny(rec.ScoreBreakdown)
		rationaleJSON, _ := marshalAny(rec.Rationale)
		graphJSON, _ := marshalAny(rec.GraphSignalRefs)
		policyJSON, _ := marshalAny(rec.PolicyDecisions)
		qualityJSON, _ := marshalAny(rec.QualityDecisions)
		deliveryChannel := nullableText(rec.DeliveryChannel)
		var rankPosition any
		if rec.RankPosition > 0 {
			rankPosition = rec.RankPosition
		}
		var deliveredAt any
		if rec.Status == "delivered" {
			deliveredAt = input.CompletedAt
		}
		_, err = tx.Exec(ctx, `
INSERT INTO recommendations (
    id, actor_user_id, request_id, watch_id, watch_run_id,
    candidate_id, artifact_id, trace_id, rank_position, status,
    status_reason, score_breakdown, rationale, graph_signal_refs,
    policy_decisions, quality_decisions, delivery_channel, delivered_at, created_at
) VALUES (
    $1, $2, NULL, $3, $4,
    $5, NULL, $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $15, $16, $17
)`,
			recID,
			input.ActorUserID,
			input.WatchID,
			input.WatchRunID,
			candidateID,
			traceID,
			rankPosition,
			rec.Status,
			rec.StatusReason,
			scoreJSON,
			rationaleJSON,
			graphJSON,
			policyJSON,
			qualityJSON,
			deliveryChannel,
			deliveredAt,
			input.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert watch recommendation: %w", err)
		}
		recommendationIDs = append(recommendationIDs, recID)
		if rec.Status == "delivered" && rec.DeliveryChannel != "" {
			deliveryID, err := newTextID("rec_delivery")
			if err != nil {
				return nil, err
			}
			_, err = tx.Exec(ctx, `
INSERT INTO recommendation_delivery_attempts (
    id, recommendation_id, channel, destination_ref, outcome, error_kind, attempted_at
) VALUES ($1, $2, $3, $4, 'sent', NULL, $5)`,
				deliveryID, recID, rec.DeliveryChannel, input.ActorUserID, input.CompletedAt,
			)
			if err != nil {
				return nil, fmt.Errorf("insert watch delivery attempt: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit watch outcome: %w", err)
	}
	sort.Strings(recommendationIDs)
	return recommendationIDs, nil
}

func scanWatchRow(row pgx.Row) (WatchRecord, error) {
	var rec WatchRecord
	var consentJSON, scopeJSON, filtersJSON, scheduleJSON, quietJSON []byte
	if err := row.Scan(
		&rec.ID,
		&rec.ActorUserID,
		&rec.Name,
		&rec.Kind,
		&rec.Enabled,
		&consentJSON,
		&scopeJSON,
		&filtersJSON,
		&rec.AllowedSources,
		&scheduleJSON,
		&rec.MaxAlertsPerWindow,
		&rec.AlertWindowSeconds,
		&rec.CooldownSeconds,
		&quietJSON,
		&rec.LocationPrecision,
		&rec.DeliveryChannel,
		&rec.QueuePolicy,
		&rec.FreshnessSeconds,
		&rec.LastRunAt,
		&rec.NextDueAt,
		&rec.SilenceUntil,
		&rec.CreatedAt,
		&rec.UpdatedAt,
		&rec.DeletedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return WatchRecord{}, fmt.Errorf("watch not found")
		}
		return WatchRecord{}, fmt.Errorf("scan watch row: %w", err)
	}
	consent, err := policy.UnmarshalConsentRecord(consentJSON)
	if err != nil {
		return WatchRecord{}, err
	}
	rec.Consent = consent
	if err := json.Unmarshal(scopeJSON, &rec.Scope); err != nil {
		return WatchRecord{}, fmt.Errorf("decode watch scope: %w", err)
	}
	if err := json.Unmarshal(filtersJSON, &rec.Filters); err != nil {
		return WatchRecord{}, fmt.Errorf("decode watch filters: %w", err)
	}
	if err := json.Unmarshal(scheduleJSON, &rec.Schedule); err != nil {
		return WatchRecord{}, fmt.Errorf("decode watch schedule: %w", err)
	}
	if err := json.Unmarshal(quietJSON, &rec.QuietHours); err != nil {
		return WatchRecord{}, fmt.Errorf("decode watch quiet hours: %w", err)
	}
	return rec, nil
}

func computeNextDue(input WatchInput, now time.Time) time.Time {
	if !input.Enabled {
		return time.Time{}
	}
	if input.AlertWindowSeconds <= 0 {
		return now
	}
	// First-time creation: due immediately so the very first scheduler tick can
	// pick the watch up.
	return now
}

func nullableFreshness(value int) int {
	if value <= 0 {
		return 86400
	}
	return value
}
