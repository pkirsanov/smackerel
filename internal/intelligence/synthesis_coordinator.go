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

// BUG-004-004 SCOPE-03 — coordination, bounded retry, lifecycle.
//
// SCOPE-02 made a single run durable. This makes CONCURRENT and REPEATED runs
// safe: two processes must not both produce the same window, a crashed process
// must not park a window forever, and a retry must not be spent on a failure
// that will never succeed.

// SynthesisFailureKind separates retryable from hopeless.
//
// The distinction is load-bearing, not decorative. A validation rejection
// retried five times is five identical rejections and a delayed alert; a
// dropped connection retried once usually succeeds. Recording which kind
// occurred is what lets an operator tell a flaky database apart from a
// candidate that will never be accepted.
type SynthesisFailureKind string

const (
	// FailureTransient — the attempt could plausibly succeed if repeated:
	// connection loss, serialization conflict, lock timeout, deadline.
	FailureTransient SynthesisFailureKind = "transient"
	// FailureTerminal — repeating changes nothing: validation rejection,
	// unauthorized citation, malformed payload.
	FailureTerminal SynthesisFailureKind = "terminal"
)

// ClassifySynthesisFailure decides whether an error is worth retrying.
//
// The DEFAULT is terminal, deliberately. Treating an unrecognised error as
// retryable would burn the whole budget on something that was never going to
// work and delay the alert that says so.
func ClassifySynthesisFailure(err error) SynthesisFailureKind {
	if err == nil {
		return FailureTerminal
	}

	// A rejected candidate is the definitive terminal case: the content itself
	// is wrong, and no amount of repetition edits it.
	var ve *SynthesisValidationError
	if errors.As(err, &ve) {
		return FailureTerminal
	}

	// A cancelled or timed-out context is transient with respect to the
	// candidate -- the work was interrupted, not refused.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return FailureTransient
	}

	if isTransientPostgresError(err) {
		return FailureTransient
	}

	return FailureTerminal
}

// isTransientPostgresError recognises the SQLSTATE classes that a repeat can
// clear. Serialization failure is the important one: the persistence layer runs
// SERIALIZABLE precisely so concurrent runs conflict, and a conflict is the
// mechanism working, not a defect.
func isTransientPostgresError(err error) bool {
	code := postgresErrorCode(err)
	switch code {
	case "40001", // serialization_failure
		"40P01", // deadlock_detected
		"55P03", // lock_not_available
		"57014", // query_canceled
		"08000", // connection_exception
		"08003", // connection_does_not_exist
		"08006", // connection_failure
		"53300": // too_many_connections
		return true
	}
	return false
}

// SynthesisRetryPolicy bounds how hard a transient failure is retried.
//
// Every field is required and validated. There is no default budget: an
// unbounded or accidentally-zero retry loop is an operational hazard, so the
// operator states the numbers and a wrong one fails loudly at construction
// rather than silently at 3am.
type SynthesisRetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	LeaseTTL     time.Duration
}

// Validate rejects a policy that would loop forever, never retry, or hold a
// lease that cannot expire.
func (p SynthesisRetryPolicy) Validate() error {
	if p.MaxAttempts < 1 {
		return fmt.Errorf("synthesis retry policy requires MaxAttempts >= 1, got %d", p.MaxAttempts)
	}
	if p.InitialDelay <= 0 {
		return fmt.Errorf("synthesis retry policy requires a positive InitialDelay, got %s", p.InitialDelay)
	}
	if p.MaxDelay < p.InitialDelay {
		return fmt.Errorf("synthesis retry policy MaxDelay (%s) must be >= InitialDelay (%s)", p.MaxDelay, p.InitialDelay)
	}
	if p.LeaseTTL <= 0 {
		return fmt.Errorf("synthesis retry policy requires a positive LeaseTTL, got %s", p.LeaseTTL)
	}
	return nil
}

// backoffFor returns the delay before the given attempt, doubling and capped.
// Capping matters: without it a modest MaxAttempts still produces an
// unboundedly long total wait.
func (p SynthesisRetryPolicy) backoffFor(attempt int) time.Duration {
	delay := p.InitialDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= p.MaxDelay {
			return p.MaxDelay
		}
	}
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

// ErrRunClaimedElsewhere means another live holder owns this window.
var ErrRunClaimedElsewhere = errors.New("synthesis run is claimed by another holder")

// advisoryLockKey maps a logical key onto the int64 advisory-lock space.
//
// A hash collision would serialise two UNRELATED windows against each other.
// That costs a little concurrency and breaks nothing, whereas skipping the lock
// entirely would let two processes both believe they own a window. Correctness
// is preserved by the UNIQUE constraint regardless; the lock exists to avoid
// wasting work.
func advisoryLockKey(logicalKey string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(logicalKey))
	return int64(h.Sum64())
}

// SynthesisCoordinator serialises runs for a logical window and retries the
// failures that are worth retrying.
type SynthesisCoordinator struct {
	persistence *SynthesisPersistence
	policy      SynthesisRetryPolicy
	holder      string
	sleep       func(context.Context, time.Duration) error
}

// NewSynthesisCoordinator validates the policy up front so a misconfigured
// budget cannot reach production.
func NewSynthesisCoordinator(persistence *SynthesisPersistence, policy SynthesisRetryPolicy, holder string) (*SynthesisCoordinator, error) {
	if persistence == nil {
		return nil, errors.New("synthesis coordinator requires persistence")
	}
	if holder == "" {
		return nil, errors.New("synthesis coordinator requires a holder identity; an anonymous lease cannot be attributed or reclaimed")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &SynthesisCoordinator{
		persistence: persistence,
		policy:      policy,
		holder:      holder,
		sleep:       sleepWithContext,
	}, nil
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ClaimWindow takes exclusive ownership of a logical window.
//
// The advisory lock is transaction-scoped, so it is released whatever happens to
// this process. The durable lease outlives the session and is what makes
// recovery possible after a crash: a run whose lease has expired is reclaimable
// by anyone, with no cooperation needed from the holder that died.
// ClaimWindow takes the lease for a window, retrying transient conflicts.
//
// The retry is load-bearing under real contention. SERIALIZABLE means two
// simultaneous claims can BOTH be valid transactions that PostgreSQL then
// refuses to serialize, and that refusal is the isolation level working, not a
// defect. Surfacing it raw would hand a caller an opaque database error where
// the honest answer is "someone else holds this window" -- which is exactly
// what the retry discovers on its next pass. Found by the concurrency stress
// test; the sequential test could not reach it, because it never puts two
// transactions inside the critical section at once.
func (c *SynthesisCoordinator) ClaimWindow(ctx context.Context, key SynthesisRunKey, now time.Time) error {
	var lastErr error
	for attempt := 1; attempt <= c.policy.MaxAttempts; attempt++ {
		err := c.claimOnce(ctx, key, now)
		if err == nil || errors.Is(err, ErrRunClaimedElsewhere) {
			return err
		}
		if ClassifySynthesisFailure(err) != FailureTransient {
			return err
		}
		lastErr = err
		if attempt == c.policy.MaxAttempts {
			break
		}
		if sleepErr := c.sleep(ctx, c.policy.backoffFor(attempt)); sleepErr != nil {
			return sleepErr
		}
	}
	return fmt.Errorf("claim window: transient conflict persisted across %d attempt(s): %w", c.policy.MaxAttempts, lastErr)
}

func (c *SynthesisCoordinator) claimOnce(ctx context.Context, key SynthesisRunKey, now time.Time) error {
	logicalKey := key.LogicalKey()
	tx, err := c.persistence.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin claim transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, synthesisWindowLockKey(key)); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}

	var state, holder string
	var expires *time.Time
	err = tx.QueryRow(ctx, `
		SELECT state, COALESCE(lease_holder, ''), lease_expires_at
		FROM synthesis_runs WHERE logical_key = $1`, logicalKey).Scan(&state, &holder, &expires)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Nothing claimed this window yet.
	case err != nil:
		return fmt.Errorf("read existing run: %w", err)
	case state != "running":
		// Already terminal. The caller learns this from the UNIQUE constraint on
		// commit, which is the authoritative check; re-deciding it here would be
		// a second answer to the same question.
		return nil
	case expires != nil && expires.After(now) && holder != c.holder:
		return fmt.Errorf("%w: held by %q until %s", ErrRunClaimedElsewhere, holder, expires.UTC().Format(time.RFC3339))
	}

	leaseEnd := now.Add(c.policy.LeaseTTL)
	// The claim carries the full key so a first claim can CREATE the row. An
	// earlier version selected from synthesis_runs to populate the insert, which
	// matched nothing when the window was new -- so the first claim wrote nothing
	// and every subsequent holder also saw an unclaimed window.
	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_runs (
			id, logical_key, cadence, principal, window_start, window_end,
			policy_version, source_set_digest, state, lifecycle_state,
			attempt_count, lease_holder, lease_expires_at,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'running', 'current', 1, $9, $10, $11, $11)
		ON CONFLICT (logical_key) DO UPDATE
		SET state = 'running',
		    attempt_count = synthesis_runs.attempt_count + 1,
		    lease_holder = $9,
		    lease_expires_at = $10,
		    updated_at = $11`,
		ulid.Make().String(), logicalKey, string(key.Cadence), key.Principal,
		key.WindowStart.UTC(), key.WindowEnd.UTC(), key.PolicyVersion,
		key.SourceSetDigest(), c.holder, leaseEnd, now); err != nil {
		return fmt.Errorf("write lease: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit claim: %w", err)
	}
	return nil
}

// ReclaimExpiredLeases frees windows abandoned by processes that died holding
// them, and returns how many were freed.
//
// Without this a crash parks a window in 'running' permanently and no scheduler
// tick will ever pick it up again -- the failure would be silent and permanent,
// which is the worst combination.
func (c *SynthesisCoordinator) ReclaimExpiredLeases(ctx context.Context, now time.Time) (int64, error) {
	tag, err := c.persistence.pool.Exec(ctx, `
		UPDATE synthesis_runs
		SET lease_holder = NULL, lease_expires_at = NULL, state = 'failed', updated_at = $1
		WHERE state = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at <= $1`, now)
	if err != nil {
		return 0, fmt.Errorf("reclaim expired leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RunWithRetry executes op, retrying ONLY transient failures within budget.
//
// Every attempt is recorded, including the ones that fail, and the failure kind
// is recorded with them. A caller reading the audit log can therefore see how
// many attempts a window consumed and why it stopped, rather than only that it
// ended badly.
func (c *SynthesisCoordinator) RunWithRetry(
	ctx context.Context,
	logicalKey string,
	op func(ctx context.Context) error,
) error {
	var lastErr error
	for attempt := 1; attempt <= c.policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		kind := ClassifySynthesisFailure(err)
		if auditErr := c.recordFailedAttempt(ctx, logicalKey, kind, err); auditErr != nil {
			return auditErr
		}

		if kind == FailureTerminal {
			// Spending the remaining budget would delay the alert without any
			// chance of a different outcome.
			return err
		}
		if attempt == c.policy.MaxAttempts {
			break
		}
		if err := c.sleep(ctx, c.policy.backoffFor(attempt)); err != nil {
			return err
		}
	}
	return fmt.Errorf("synthesis retries exhausted after %d attempt(s): %w", c.policy.MaxAttempts, lastErr)
}

func (c *SynthesisCoordinator) recordFailedAttempt(ctx context.Context, logicalKey string, kind SynthesisFailureKind, cause error) error {
	failureClass := string(FailureInvalidPayload)
	var ve *SynthesisValidationError
	if errors.As(cause, &ve) {
		failureClass = string(ve.Code)
	}
	// The message is a fixed classification, never the error text: an error can
	// carry candidate content, and audit rows must stay content-free.
	if err := c.persistence.RecordAttemptWithKind(ctx, logicalKey, AttemptFailed, failureClass, string(kind), "attempt failed"); err != nil {
		return &SynthesisAuditPersistenceError{Operation: "legacy_retry_attempt", Cause: err}
	}
	return nil
}

// MarkSuperseded records that a newer run has replaced this window's answer.
//
// Provenance is append-preserving: the prior run keeps its rows, its attempts
// and its citations. Only its claim to be the CURRENT answer is withdrawn.
func (c *SynthesisCoordinator) MarkSuperseded(ctx context.Context, logicalKey string, now time.Time) error {
	// Reachable from stale as well as current: an output can age past its
	// freshness budget and THEN be replaced, and the replacement is the stronger
	// statement of the two.
	return c.transitionLifecycle(ctx, logicalKey, "superseded", []string{"current", "stale"}, now)
}

// MarkStale records that an output has aged past its freshness budget. It is
// still the latest answer, which is why this is distinct from superseded.
func (c *SynthesisCoordinator) MarkStale(ctx context.Context, logicalKey string, now time.Time) error {
	// Only from current. Going stale from superseded would be a downgrade --
	// a replaced answer does not become the latest one again by ageing.
	return c.transitionLifecycle(ctx, logicalKey, "stale", []string{"current"}, now)
}

// transitionLifecycle moves a run to target when it is in one of from, and
// reports when it could not. A silent no-op here would let a caller believe an
// output had been retired when it was still being served as current.
func (c *SynthesisCoordinator) transitionLifecycle(
	ctx context.Context,
	logicalKey string,
	target string,
	from []string,
	now time.Time,
) error {
	tag, err := c.persistence.pool.Exec(ctx, `
		UPDATE synthesis_runs SET lifecycle_state = $3, updated_at = $4
		WHERE logical_key = $1 AND lifecycle_state = ANY($2)`,
		logicalKey, from, target, now)
	if err != nil {
		return fmt.Errorf("mark %s: %w", target, err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	var current string
	err = c.persistence.pool.QueryRow(ctx,
		`SELECT lifecycle_state FROM synthesis_runs WHERE logical_key = $1`, logicalKey).Scan(&current)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("mark %s: no synthesis run for logical key", target)
	case err != nil:
		return fmt.Errorf("mark %s: read current lifecycle: %w", target, err)
	case current == target:
		return nil // already there; re-asserting a state is not a failure
	default:
		return fmt.Errorf("mark %s: run is %q, which is not a permitted source state", target, current)
	}
}

// ArchiveOlderThan moves aged runs out of the active set WITHOUT deleting them.
//
// Archival is a lifecycle transition, never a delete: the attempts and citation
// provenance an operator may need to explain a past answer survive it.
func (c *SynthesisCoordinator) ArchiveOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := c.persistence.pool.Exec(ctx, `
		UPDATE synthesis_runs SET lifecycle_state = 'archived', updated_at = NOW()
		WHERE created_at < $1 AND lifecycle_state IN ('current', 'stale', 'superseded')`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("archive old runs: %w", err)
	}
	return tag.RowsAffected(), nil
}
