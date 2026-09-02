package intelligence

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"
)

// SynthesisTriggerKind is the closed origin vocabulary for a run attempt.
type SynthesisTriggerKind string

const (
	TriggerScheduled     SynthesisTriggerKind = "scheduled"
	TriggerOperatorRetry SynthesisTriggerKind = "operator_retry"
)

// SynthesisEventType is the immutable lifecycle vocabulary persisted in
// synthesis_run_events. Events contain identities, bounded counts, and safe
// codes only; generated text and source identities never enter this structure.
type SynthesisEventType string

const (
	EventClaimed          SynthesisEventType = "claimed"
	EventAttemptStarted   SynthesisEventType = "attempt_started"
	EventIdempotent       SynthesisEventType = "idempotent"
	EventPersisted        SynthesisEventType = "persisted"
	EventQuiet            SynthesisEventType = "quiet"
	EventPartial          SynthesisEventType = "partial"
	EventRolledBack       SynthesisEventType = "rolled_back"
	EventRetryableFailure SynthesisEventType = "retryable_failure"
	EventFailed           SynthesisEventType = "failed"
	EventReadbackFailed   SynthesisEventType = "readback_failed"
	EventRecovered        SynthesisEventType = "recovered"
	EventSuperseded       SynthesisEventType = "superseded"
)

const (
	FailureTransaction SynthesisFailureCode = "transaction_failed"
	FailureReadback    SynthesisFailureCode = "readback_failed"
	FailureAudit       SynthesisFailureCode = "audit_persistence_failed"
)

// SynthesisAttempt identifies one execution attempt for one logical run.
type SynthesisAttempt struct {
	RunID       string
	LogicalKey  string
	AttemptNo   int
	TriggerKind SynthesisTriggerKind
	Key         SynthesisRunKey
	StartedAt   time.Time
}

// SynthesisRunEvent is one content-free immutable event.
type SynthesisRunEvent struct {
	RunID           string
	AttemptNo       int
	EventType       SynthesisEventType
	OutputID        string
	RelatedOutputID string
	FailureCode     string
	InsightCount    *int
	CitationCount   *int
	CreatedAt       time.Time
}

// SynthesisAuditPersistenceError reports that a required attempt or event fact
// could not be stored. Error deliberately omits the underlying database text so
// logs and HTTP boundaries cannot expose SQL or corpus-adjacent values.
type SynthesisAuditPersistenceError struct {
	Operation string
	Cause     error
}

func (e *SynthesisAuditPersistenceError) Error() string {
	return fmt.Sprintf("synthesis audit persistence failed during %s", e.Operation)
}

func (e *SynthesisAuditPersistenceError) Unwrap() error { return e.Cause }

// SynthesisReadbackError reports a committed aggregate that the production
// reader could not verify. The committed row remains forensic history but is
// not authoritative and is never returned to delivery callers.
type SynthesisReadbackError struct {
	OutputID string
	Cause    error
}

func (e *SynthesisReadbackError) Error() string {
	return "synthesis aggregate committed but production read-back did not verify it"
}

func (e *SynthesisReadbackError) Unwrap() error { return e.Cause }

func validateSynthesisRunKey(key SynthesisRunKey) error {
	if key.Cadence != CadenceDaily && key.Cadence != CadenceWeekly {
		return fmt.Errorf("unknown synthesis cadence %q", key.Cadence)
	}
	if key.Principal == "" {
		return errors.New("synthesis run requires a principal")
	}
	if key.PolicyVersion == "" {
		return errors.New("synthesis run requires a policy version")
	}
	if !key.WindowEnd.After(key.WindowStart) {
		return errors.New("synthesis window end must be after start")
	}
	return nil
}

// synthesisWindowLockKey intentionally excludes source and policy identity.
// Every possible replacement for one actor/cadence/window serializes on this
// key, while LogicalKey still distinguishes replay input.
func synthesisWindowLockKey(key SynthesisRunKey) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("smackerel/synthesis-window-lock/v1\x00"))
	for _, part := range []string{
		key.Principal,
		string(key.Cadence),
		key.WindowStart.UTC().Format(time.RFC3339Nano),
		key.WindowEnd.UTC().Format(time.RFC3339Nano),
	} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(part), part)
	}
	return int64(h.Sum64())
}

func validateTriggerKind(trigger SynthesisTriggerKind) error {
	switch trigger {
	case TriggerScheduled, TriggerOperatorRetry:
		return nil
	default:
		return fmt.Errorf("unknown synthesis trigger kind %q", trigger)
	}
}

// StartAttempt persists the attempt_started fact in the same transaction that
// allocates its run-linked attempt number. A caller may not start work unless
// this succeeds.
func (p *SynthesisPersistence) StartAttempt(
	ctx context.Context,
	key SynthesisRunKey,
	trigger SynthesisTriggerKind,
	holder string,
	leaseTTL time.Duration,
	now time.Time,
) (SynthesisAttempt, error) {
	if err := validateSynthesisRunKey(key); err != nil {
		return SynthesisAttempt{}, err
	}
	if err := validateTriggerKind(trigger); err != nil {
		return SynthesisAttempt{}, err
	}
	if holder == "" {
		return SynthesisAttempt{}, errors.New("synthesis attempt requires a holder identity")
	}
	if leaseTTL <= 0 {
		return SynthesisAttempt{}, errors.New("synthesis attempt requires a positive lease")
	}

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SynthesisAttempt{}, fmt.Errorf("begin synthesis attempt: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, synthesisWindowLockKey(key)); err != nil {
		return SynthesisAttempt{}, fmt.Errorf("lock synthesis window: %w", err)
	}

	logicalKey := key.LogicalKey()
	runID := ulid.Make().String()
	at := now.UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_runs (
			id, logical_key, cadence, principal, window_start, window_end,
			policy_version, source_set_digest, state, created_at, updated_at,
			lifecycle_state, attempt_count, lease_holder, lease_expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'running', $9, $9,
			'current', 0, $10, $11)
		ON CONFLICT (logical_key) DO NOTHING
	`, runID, logicalKey, string(key.Cadence), key.Principal,
		key.WindowStart.UTC(), key.WindowEnd.UTC(), key.PolicyVersion,
		key.SourceSetDigest(), at, holder, at.Add(leaseTTL)); err != nil {
		return SynthesisAttempt{}, fmt.Errorf("create synthesis run: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id FROM synthesis_runs WHERE logical_key = $1 FOR UPDATE`, logicalKey,
	).Scan(&runID); err != nil {
		return SynthesisAttempt{}, fmt.Errorf("load synthesis run: %w", err)
	}

	attemptNo := 0
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_no), 0) + 1
		FROM synthesis_run_attempts
		WHERE run_id = $1
	`, runID).Scan(&attemptNo); err != nil {
		return SynthesisAttempt{}, fmt.Errorf("allocate synthesis attempt number: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_run_attempts (
			logical_key, outcome, recorded_at, run_id, attempt_no,
			trigger_kind, state, started_at, included_source_classes,
			omitted_source_classes, insight_count, citation_count
		) VALUES ($1, 'running', $2, $3, $4, $5, 'running', $2,
			'{}', '{}', 0, 0)
	`, logicalKey, at, runID, attemptNo, string(trigger)); err != nil {
		return SynthesisAttempt{}, &SynthesisAuditPersistenceError{Operation: "attempt_start", Cause: err}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE synthesis_runs
		SET state = 'running', attempt_count = $2, lease_holder = $3,
			lease_expires_at = $4, updated_at = $5
		WHERE id = $1
	`, runID, attemptNo, holder, at.Add(leaseTTL), at); err != nil {
		return SynthesisAttempt{}, &SynthesisAuditPersistenceError{Operation: "attempt_summary_start", Cause: err}
	}

	event := SynthesisRunEvent{
		RunID: runID, AttemptNo: attemptNo, EventType: EventAttemptStarted,
		CreatedAt: at,
	}
	if err := appendSynthesisEventTx(ctx, tx, event); err != nil {
		return SynthesisAttempt{}, &SynthesisAuditPersistenceError{Operation: "attempt_started_event", Cause: err}
	}
	if err := tx.Commit(ctx); err != nil {
		return SynthesisAttempt{}, &SynthesisAuditPersistenceError{Operation: "attempt_start_commit", Cause: err}
	}

	return SynthesisAttempt{
		RunID: runID, LogicalKey: logicalKey, AttemptNo: attemptNo,
		TriggerKind: trigger, Key: key, StartedAt: at,
	}, nil
}

// AppendEvent appends one immutable event through the production persistence
// boundary. It exists for reconciliation/lifecycle operations that do not own a
// content transaction.
func (p *SynthesisPersistence) AppendEvent(ctx context.Context, event SynthesisRunEvent) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return &SynthesisAuditPersistenceError{Operation: "event_begin", Cause: err}
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := appendSynthesisEventTx(ctx, tx, event); err != nil {
		return &SynthesisAuditPersistenceError{Operation: "event_insert", Cause: err}
	}
	if err := tx.Commit(ctx); err != nil {
		return &SynthesisAuditPersistenceError{Operation: "event_commit", Cause: err}
	}
	return nil
}

func appendSynthesisEventTx(ctx context.Context, tx pgx.Tx, event SynthesisRunEvent) error {
	if event.RunID == "" || event.AttemptNo < 1 || event.EventType == "" || event.CreatedAt.IsZero() {
		return errors.New("synthesis event requires run, positive attempt, type, and time")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO synthesis_run_events (
			id, run_id, attempt_no, event_type, output_id,
			related_output_id, failure_code, insight_count,
			citation_count, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, ulid.Make().String(), event.RunID, event.AttemptNo,
		string(event.EventType), nullIfEmpty(event.OutputID),
		nullIfEmpty(event.RelatedOutputID), nullIfEmpty(event.FailureCode),
		event.InsightCount, event.CitationCount, event.CreatedAt.UTC())
	return err
}

func terminalEventForKind(kind SynthesisOutputKind) (SynthesisEventType, error) {
	switch kind {
	case OutputKindFull:
		return EventPersisted, nil
	case OutputKindQuiet:
		return EventQuiet, nil
	case OutputKindPartial:
		return EventPartial, nil
	default:
		return "", fmt.Errorf("unknown synthesis output kind %q", kind)
	}
}

func compatibilityOutcome(event SynthesisEventType) string {
	switch event {
	case EventPersisted, EventQuiet, EventPartial:
		return "succeeded"
	case EventIdempotent:
		return "idempotent_no_change"
	case EventRecovered:
		return "recovered"
	case EventRetryableFailure:
		return "retryable_failure"
	case EventRolledBack:
		return "rolled_back"
	case EventReadbackFailed:
		return "readback_failed"
	default:
		return "failed"
	}
}

func eventIsFailure(event SynthesisEventType) bool {
	switch event {
	case EventRolledBack, EventRetryableFailure, EventFailed, EventReadbackFailed:
		return true
	default:
		return false
	}
}

func eventIsSuccessfulTerminal(event SynthesisEventType) bool {
	switch event {
	case EventPersisted, EventQuiet, EventPartial, EventIdempotent, EventRecovered:
		return true
	default:
		return false
	}
}

// nonNilSynthesisSourceClasses keeps the attempt summary's NOT NULL TEXT[]
// columns non-null when pgx binds a valid nil source-class slice.
func nonNilSynthesisSourceClasses(classes []string) []string {
	if classes == nil {
		return []string{}
	}
	return classes
}

func (p *SynthesisPersistence) finishAttempt(
	ctx context.Context,
	attempt SynthesisAttempt,
	eventType SynthesisEventType,
	outputID string,
	relatedOutputID string,
	failureCode string,
	failureKind SynthesisFailureKind,
	included []string,
	omitted []string,
	insightCount int,
	citationCount int,
	now time.Time,
) error {
	// The actor/cadence/window advisory lock serializes every terminal summary.
	// READ COMMITTED gives a waiter a fresh snapshot after it acquires that lock;
	// SERIALIZABLE made otherwise-valid queued finishers form an SSI cycle with
	// the preceding content transaction and fail with SQLSTATE 40001.
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return &SynthesisAuditPersistenceError{Operation: "attempt_finish_begin", Cause: err}
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, synthesisWindowLockKey(attempt.Key)); err != nil {
		return &SynthesisAuditPersistenceError{Operation: "attempt_finish_lock", Cause: err}
	}

	failureClass := any(nil)
	failureMessage := any(nil)
	failureKindValue := any(nil)
	if eventIsFailure(eventType) {
		if failureCode == "" {
			failureCode = string(FailureTransaction)
		}
		failureClass = failureCode
		failureMessage = "attempt failed"
		failureKindValue = string(failureKind)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE synthesis_run_attempts
		SET outcome = $4, failure_class = $5, failure_kind = $6,
			failure_message = $7, state = $8, output_id = $9,
			finished_at = $10, failure_code = $5,
			included_source_classes = $11, omitted_source_classes = $12,
			insight_count = $13, citation_count = $14
		WHERE run_id = $1 AND attempt_no = $2 AND logical_key = $3
			AND state = 'running'
	`, attempt.RunID, attempt.AttemptNo, attempt.LogicalKey,
		compatibilityOutcome(eventType), failureClass, failureKindValue,
		failureMessage, string(eventType), nullIfEmpty(outputID), now.UTC(),
		nonNilSynthesisSourceClasses(included), nonNilSynthesisSourceClasses(omitted),
		insightCount, citationCount)
	if err != nil {
		return &SynthesisAuditPersistenceError{Operation: "attempt_summary_finish", Cause: err}
	}
	if tag.RowsAffected() != 1 {
		return &SynthesisAuditPersistenceError{Operation: "attempt_summary_finish", Cause: errors.New("attempt is absent or already terminal")}
	}

	countInsights := insightCount
	countCitations := citationCount
	event := SynthesisRunEvent{
		RunID: attempt.RunID, AttemptNo: attempt.AttemptNo,
		EventType: eventType, OutputID: outputID,
		FailureCode: failureCode, CreatedAt: now.UTC(),
	}
	if eventIsSuccessfulTerminal(eventType) || eventType == EventReadbackFailed {
		event.InsightCount = &countInsights
		event.CitationCount = &countCitations
	}
	if err := appendSynthesisEventTx(ctx, tx, event); err != nil {
		return &SynthesisAuditPersistenceError{Operation: "terminal_event", Cause: err}
	}
	if relatedOutputID != "" {
		superseded := SynthesisRunEvent{
			RunID: attempt.RunID, AttemptNo: attempt.AttemptNo,
			EventType: EventSuperseded, OutputID: outputID,
			RelatedOutputID: relatedOutputID, CreatedAt: now.UTC(),
		}
		if err := appendSynthesisEventTx(ctx, tx, superseded); err != nil {
			return &SynthesisAuditPersistenceError{Operation: "superseded_event", Cause: err}
		}
	}

	runState := "failed"
	if eventIsSuccessfulTerminal(eventType) {
		runState = "succeeded"
	}
	if eventIsFailure(eventType) {
		if _, err := tx.Exec(ctx, `
			UPDATE synthesis_runs r
			SET lifecycle_state = 'superseded', updated_at = $6
			WHERE r.id = $1
				AND EXISTS (
					SELECT 1 FROM synthesis_outputs o
					WHERE o.principal = $2 AND o.cadence = $3
						AND o.window_start = $4 AND o.window_end = $5
						AND o.lifecycle_state = 'current' AND o.run_id <> r.id
				)
		`, attempt.RunID, attempt.Key.Principal, string(attempt.Key.Cadence),
			attempt.Key.WindowStart.UTC(), attempt.Key.WindowEnd.UTC(), now.UTC()); err != nil {
			return &SynthesisAuditPersistenceError{Operation: "failed_run_lifecycle", Cause: err}
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE synthesis_runs
		SET state = $2, lease_holder = NULL, lease_expires_at = NULL,
			updated_at = $3
		WHERE id = $1
	`, attempt.RunID, runState, now.UTC()); err != nil {
		return &SynthesisAuditPersistenceError{Operation: "run_summary_finish", Cause: err}
	}

	if eventIsSuccessfulTerminal(eventType) {
		var currentCount int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM synthesis_outputs
			WHERE principal = $1 AND cadence = $2
				AND window_start = $3 AND window_end = $4
				AND lifecycle_state = 'current'
		`, attempt.Key.Principal, string(attempt.Key.Cadence),
			attempt.Key.WindowStart.UTC(), attempt.Key.WindowEnd.UTC()).Scan(&currentCount); err != nil {
			return &SynthesisAuditPersistenceError{Operation: "one_current_verify", Cause: err}
		}
		if currentCount != 1 {
			return &SynthesisAuditPersistenceError{Operation: "one_current_verify", Cause: fmt.Errorf("current output count is %d", currentCount)}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return &SynthesisAuditPersistenceError{Operation: "attempt_finish_commit", Cause: err}
	}
	return nil
}

// FinishAttemptFailure persists one typed terminal failure. The event and
// mutable summary advance together or neither does.
func (p *SynthesisPersistence) FinishAttemptFailure(
	ctx context.Context,
	attempt SynthesisAttempt,
	eventType SynthesisEventType,
	failureCode string,
	kind SynthesisFailureKind,
	now time.Time,
) error {
	if !eventIsFailure(eventType) || eventType == EventReadbackFailed {
		return fmt.Errorf("event %q is not a no-output terminal failure", eventType)
	}
	return p.finishAttempt(ctx, attempt, eventType, "", "", failureCode, kind,
		[]string{}, []string{}, 0, 0, now)
}

func (p *SynthesisPersistence) latestOutputEvent(ctx context.Context, outputID string) (SynthesisEventType, error) {
	var raw string
	err := p.pool.QueryRow(ctx, `
		SELECT event_type
		FROM synthesis_run_events
		WHERE output_id = $1 AND event_type IN (
			'idempotent', 'persisted', 'quiet', 'partial',
			'readback_failed', 'recovered'
		)
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, outputID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read latest synthesis output event: %w", err)
	}
	return SynthesisEventType(raw), nil
}

// awaitExistingOutputEvent closes the short boundary between content commit
// and terminal-event publication for concurrent same-source attempts. The
// earlier finisher locks its attempt row while publishing the event; FOR UPDATE
// waits on that production row lock and avoids polling or time-based sleeps.
func (p *SynthesisPersistence) awaitExistingOutputEvent(
	ctx context.Context,
	attempt SynthesisAttempt,
	outputID string,
) (SynthesisEventType, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", fmt.Errorf("begin existing synthesis event wait: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var priorAttemptNo int
	err = tx.QueryRow(ctx, `
		SELECT attempt_no
		FROM synthesis_run_attempts
		WHERE run_id = $1 AND attempt_no <> $2
		ORDER BY attempt_no
		LIMIT 1
		FOR UPDATE
	`, attempt.RunID, attempt.AttemptNo).Scan(&priorAttemptNo)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("wait for existing synthesis attempt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("finish existing synthesis event wait: %w", err)
	}
	return p.latestOutputEvent(ctx, outputID)
}

func verifyAggregateReadback(agg *SynthesisAggregate, attempt SynthesisAttempt) error {
	if agg == nil || agg.OutputID == "" || agg.RunID != attempt.RunID || agg.LogicalKey != attempt.LogicalKey {
		return errors.New("aggregate identity mismatch")
	}
	if agg.InsightCount != len(agg.Insights) {
		return errors.New("aggregate insight count mismatch")
	}
	citations := 0
	for _, insight := range agg.Insights {
		citations += len(insight.SourceArtifactIDs)
	}
	if agg.CitationCount != citations {
		return errors.New("aggregate citation count mismatch")
	}
	return nil
}

type synthesisContentWrite struct {
	OutputID       string
	PriorOutputID  string
	StagedCurrent  bool
	ExistingOutput bool
}

func (p *SynthesisPersistence) commitAttemptContent(
	ctx context.Context,
	attempt SynthesisAttempt,
	candidate SynthesisCandidate,
	now time.Time,
) (synthesisContentWrite, error) {
	// This transaction is serialized by the same actor/cadence/window advisory
	// lock as terminal summaries. A fresh post-lock snapshot lets a queued
	// contender observe the replacement committed by the previous holder.
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return synthesisContentWrite{}, fmt.Errorf("begin synthesis content transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, synthesisWindowLockKey(attempt.Key)); err != nil {
		return synthesisContentWrite{}, fmt.Errorf("lock synthesis content window: %w", err)
	}

	var attemptState string
	if err := tx.QueryRow(ctx, `
		SELECT state FROM synthesis_run_attempts
		WHERE run_id = $1 AND attempt_no = $2 AND logical_key = $3
		FOR UPDATE
	`, attempt.RunID, attempt.AttemptNo, attempt.LogicalKey).Scan(&attemptState); err != nil {
		return synthesisContentWrite{}, fmt.Errorf("load synthesis attempt: %w", err)
	}
	if attemptState != "running" {
		return synthesisContentWrite{}, fmt.Errorf("synthesis attempt is %q, want running", attemptState)
	}

	write := synthesisContentWrite{}
	var ownLifecycle string
	err = tx.QueryRow(ctx, `
		SELECT id, lifecycle_state FROM synthesis_outputs WHERE run_id = $1
	`, attempt.RunID).Scan(&write.OutputID, &ownLifecycle)
	switch {
	case err == nil:
		write.ExistingOutput = true
	case errors.Is(err, pgx.ErrNoRows):
		write.OutputID = ulid.Make().String()
	default:
		return synthesisContentWrite{}, fmt.Errorf("read existing synthesis output: %w", err)
	}

	if write.ExistingOutput {
		latestEvent, err := latestOutputEventTx(ctx, tx, write.OutputID)
		if err != nil {
			return synthesisContentWrite{}, err
		}
		if latestEvent == EventPersisted || latestEvent == EventQuiet || latestEvent == EventPartial || latestEvent == EventRecovered || latestEvent == EventIdempotent {
			if err := tx.Commit(ctx); err != nil {
				return synthesisContentWrite{}, fmt.Errorf("commit idempotent synthesis lookup: %w", err)
			}
			return write, nil
		}
	}

	var priorRunID string
	err = tx.QueryRow(ctx, `
		SELECT o.id, o.run_id
		FROM synthesis_outputs o
		WHERE o.principal = $1 AND o.cadence = $2
			AND o.window_start = $3 AND o.window_end = $4
			AND o.lifecycle_state = 'current' AND o.id <> $5
		FOR UPDATE
	`, attempt.Key.Principal, string(attempt.Key.Cadence),
		attempt.Key.WindowStart.UTC(), attempt.Key.WindowEnd.UTC(), write.OutputID,
	).Scan(&write.PriorOutputID, &priorRunID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return synthesisContentWrite{}, fmt.Errorf("read prior current synthesis output: %w", err)
	}
	if write.PriorOutputID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE synthesis_outputs
			SET lifecycle_state = 'superseded', superseded_at = $2
			WHERE id = $1 AND lifecycle_state = 'current'
		`, write.PriorOutputID, now.UTC()); err != nil {
			return synthesisContentWrite{}, fmt.Errorf("supersede prior synthesis output: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE synthesis_runs SET lifecycle_state = 'superseded', updated_at = $2
			WHERE id = $1
		`, priorRunID, now.UTC()); err != nil {
			return synthesisContentWrite{}, fmt.Errorf("supersede prior synthesis run: %w", err)
		}
	}

	if write.ExistingOutput {
		if _, err := tx.Exec(ctx, `
			UPDATE synthesis_outputs
			SET lifecycle_state = 'current', superseded_at = NULL
			WHERE id = $1
		`, write.OutputID); err != nil {
			return synthesisContentWrite{}, fmt.Errorf("restore unverified synthesis output for read-back: %w", err)
		}
	} else if err := insertSynthesisAggregateTx(ctx, tx, attempt.RunID, write.OutputID, candidate, now); err != nil {
		return synthesisContentWrite{}, err
	}
	write.StagedCurrent = true

	if _, err := tx.Exec(ctx, `
		UPDATE synthesis_runs SET lifecycle_state = 'current', updated_at = $2
		WHERE id = $1
	`, attempt.RunID, now.UTC()); err != nil {
		return synthesisContentWrite{}, fmt.Errorf("mark replacement synthesis run current: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return synthesisContentWrite{}, fmt.Errorf("commit synthesis content transaction: %w", err)
	}
	return write, nil
}

func latestOutputEventTx(ctx context.Context, tx pgx.Tx, outputID string) (SynthesisEventType, error) {
	var raw string
	err := tx.QueryRow(ctx, `
		SELECT event_type FROM synthesis_run_events
		WHERE output_id = $1 AND event_type IN (
			'idempotent', 'persisted', 'quiet', 'partial',
			'readback_failed', 'recovered'
		)
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, outputID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read synthesis output event: %w", err)
	}
	return SynthesisEventType(raw), nil
}

func insertSynthesisAggregateTx(
	ctx context.Context,
	tx pgx.Tx,
	runID string,
	outputID string,
	candidate SynthesisCandidate,
	now time.Time,
) error {
	citationCount := 0
	for _, insight := range candidate.Insights {
		citationCount += len(dedupeStrings(insight.SourceArtifactIDs))
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_outputs (
			id, run_id, output_kind, insight_count, citation_count,
			evaluated_artifact_count, created_at, principal, cadence,
			window_start, window_end, lifecycle_state
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'current')
	`, outputID, runID, string(candidate.Kind), len(candidate.Insights), citationCount,
		candidate.EvaluatedArtifactCount, now.UTC(), candidate.Key.Principal,
		string(candidate.Key.Cadence), candidate.Key.WindowStart.UTC(),
		candidate.Key.WindowEnd.UTC()); err != nil {
		return fmt.Errorf("insert synthesis output: %w", err)
	}
	for _, sourceClass := range candidate.IncludedClasses {
		if _, err := tx.Exec(ctx, `
			INSERT INTO synthesis_output_source_classes (output_id, source_class, disposition)
			VALUES ($1, $2, 'included')
		`, outputID, sourceClass); err != nil {
			return fmt.Errorf("insert included source class: %w", err)
		}
	}
	for _, sourceClass := range candidate.OmittedClasses {
		if _, err := tx.Exec(ctx, `
			INSERT INTO synthesis_output_source_classes (output_id, source_class, disposition)
			VALUES ($1, $2, 'omitted')
		`, outputID, sourceClass); err != nil {
			return fmt.Errorf("insert omitted source class: %w", err)
		}
	}
	for ordinal, insight := range candidate.Insights {
		insightID := insight.ID
		if insightID == "" {
			insightID = ulid.Make().String()
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO synthesis_output_insights (
				id, output_id, ordinal, insight_type, through_line,
				key_tension, suggested_action, confidence, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, insightID, outputID, ordinal, string(insight.InsightType),
			insight.ThroughLine, nullIfEmpty(insight.KeyTension),
			nullIfEmpty(insight.SuggestedAction), insight.Confidence, now.UTC()); err != nil {
			return fmt.Errorf("insert synthesis insight %d: %w", ordinal, err)
		}
		for citationOrdinal, artifactID := range dedupeStrings(insight.SourceArtifactIDs) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO synthesis_citations (insight_id, artifact_id, ordinal)
				VALUES ($1, $2, $3)
			`, insightID, artifactID, citationOrdinal); err != nil {
				return fmt.Errorf("insert synthesis citation %d/%d: %w", ordinal, citationOrdinal, err)
			}
		}
	}
	return nil
}

func (p *SynthesisPersistence) compensateUnverifiedOutput(
	ctx context.Context,
	attempt SynthesisAttempt,
	write synthesisContentWrite,
	now time.Time,
) error {
	if !write.StagedCurrent {
		return nil
	}
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin synthesis compensation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, synthesisWindowLockKey(attempt.Key)); err != nil {
		return fmt.Errorf("lock synthesis compensation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE synthesis_outputs
		SET lifecycle_state = 'superseded', superseded_at = $2
		WHERE id = $1
	`, write.OutputID, now.UTC()); err != nil {
		return fmt.Errorf("withdraw unverified synthesis output: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE synthesis_runs SET lifecycle_state = 'superseded', updated_at = $2
		WHERE id = $1
	`, attempt.RunID, now.UTC()); err != nil {
		return fmt.Errorf("withdraw unverified synthesis run: %w", err)
	}
	if write.PriorOutputID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE synthesis_outputs
			SET lifecycle_state = 'current', superseded_at = NULL
			WHERE id = $1
		`, write.PriorOutputID); err != nil {
			return fmt.Errorf("restore prior synthesis output: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE synthesis_runs r SET lifecycle_state = 'current', updated_at = $2
			FROM synthesis_outputs o
			WHERE o.id = $1 AND r.id = o.run_id
		`, write.PriorOutputID, now.UTC()); err != nil {
			return fmt.Errorf("restore prior synthesis run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit synthesis compensation: %w", err)
	}
	return nil
}

// CommitAttempt writes or reuses one output, verifies it through the production
// aggregate reader, and only then appends the attempt's terminal event. A
// terminal-event failure is returned as a typed audit error and any staged
// replacement is compensated so unverified content is never authoritative.
func (p *SynthesisPersistence) CommitAttempt(
	ctx context.Context,
	attempt SynthesisAttempt,
	candidate SynthesisCandidate,
	policy SourceClassPolicy,
	authorizedSources []string,
	now time.Time,
) (*SynthesisAggregate, error) {
	if candidate.Key.LogicalKey() != attempt.LogicalKey || candidate.Key.Principal != attempt.Key.Principal || candidate.Key.Cadence != attempt.Key.Cadence {
		err := errors.New("candidate identity does not match the started attempt")
		if auditErr := p.FinishAttemptFailure(ctx, attempt, EventFailed, string(FailureInvalidPayload), FailureTerminal, now); auditErr != nil {
			return nil, auditErr
		}
		return nil, err
	}
	if err := ValidateCandidate(candidate, policy, authorizedSources); err != nil {
		failureCode := string(FailureInvalidPayload)
		var validationErr *SynthesisValidationError
		if errors.As(err, &validationErr) {
			failureCode = string(validationErr.Code)
		}
		if auditErr := p.FinishAttemptFailure(ctx, attempt, EventFailed, failureCode, FailureTerminal, now); auditErr != nil {
			return nil, auditErr
		}
		return nil, err
	}

	write, err := p.commitAttemptContent(ctx, attempt, candidate, now)
	if err != nil {
		kind := ClassifySynthesisFailure(err)
		eventType := EventFailed
		if kind == FailureTransient {
			eventType = EventRetryableFailure
		}
		if auditErr := p.FinishAttemptFailure(ctx, attempt, eventType, string(FailureTransaction), kind, now); auditErr != nil {
			return nil, auditErr
		}
		return nil, err
	}

	agg, readErr := p.ReadAggregate(ctx, write.OutputID)
	if readErr == nil {
		readErr = verifyAggregateReadback(agg, attempt)
	}
	if readErr != nil {
		if compensateErr := p.compensateUnverifiedOutput(context.WithoutCancel(ctx), attempt, write, now); compensateErr != nil {
			return nil, fmt.Errorf("read-back failed and prior current output could not be restored: %w", compensateErr)
		}
		insightCount := len(candidate.Insights)
		citationCount := 0
		for _, insight := range candidate.Insights {
			citationCount += len(dedupeStrings(insight.SourceArtifactIDs))
		}
		if auditErr := p.finishAttempt(context.WithoutCancel(ctx), attempt, EventReadbackFailed,
			write.OutputID, "", string(FailureReadback), FailureTransient,
			candidate.IncludedClasses, candidate.OmittedClasses,
			insightCount, citationCount, now); auditErr != nil {
			return nil, auditErr
		}
		return nil, &SynthesisReadbackError{OutputID: write.OutputID, Cause: readErr}
	}

	eventType, err := terminalEventForKind(candidate.Kind)
	if err != nil {
		return nil, err
	}
	if write.ExistingOutput {
		latestEvent, eventErr := p.latestOutputEvent(ctx, write.OutputID)
		if eventErr != nil {
			return nil, eventErr
		}
		if latestEvent == "" {
			latestEvent, eventErr = p.awaitExistingOutputEvent(ctx, attempt, write.OutputID)
			if eventErr != nil {
				return nil, eventErr
			}
		}
		if latestEvent == EventReadbackFailed {
			eventType = EventRecovered
		} else if latestEvent != "" {
			eventType = EventIdempotent
		}
	}

	if err := p.finishAttempt(ctx, attempt, eventType, write.OutputID,
		write.PriorOutputID, "", "", candidate.IncludedClasses,
		candidate.OmittedClasses, agg.InsightCount, agg.CitationCount, now); err != nil {
		if compensateErr := p.compensateUnverifiedOutput(context.WithoutCancel(ctx), attempt, write, now); compensateErr != nil {
			return nil, fmt.Errorf("terminal audit failed and prior current output could not be restored: %w", compensateErr)
		}
		return nil, err
	}
	agg.Outcome = eventType
	return agg, nil
}
