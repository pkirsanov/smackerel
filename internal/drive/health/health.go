package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive"
)

// Policy defines how many consecutive provider failures move a connection
// from healthy to degraded and then failing.
type Policy struct {
	DegradedAfter int
	FailingAfter  int
}

// DefaultPolicy is intentionally explicit and code-owned, not config-owned:
// the thresholds are Scope 2 health semantics, not operator tunables yet.
var DefaultPolicy = Policy{DegradedAfter: 1, FailingAfter: 3}

// RetryableWork records provider work that must remain visible after an
// outage/rate-limit failure.
type RetryableWork struct {
	WorkType  string
	LastError string
	Attempts  int
	UpdatedAt time.Time
}

// Snapshot is the health state returned after a provider success/failure is
// recorded.
type Snapshot struct {
	Status             drive.HealthStatus
	Reason             string
	FailureCount       int
	RetryableWorkCount int
}

// Tracker is a deterministic in-memory health tracker used by unit tests and
// by callers that do not need persistence.
type Tracker struct {
	policy   Policy
	mu       sync.Mutex
	failures map[string]int
	work     map[string][]RetryableWork
}

// NewTracker returns a new in-memory tracker.
func NewTracker(policy Policy) *Tracker {
	return &Tracker{policy: normalizePolicy(policy), failures: map[string]int{}, work: map[string][]RetryableWork{}}
}

// RecordProviderError records a provider failure and keeps the work retryable.
func (tracker *Tracker) RecordProviderError(connectionID string, workType string, err error) Snapshot {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	failureCount := tracker.failures[connectionID] + 1
	tracker.failures[connectionID] = failureCount
	tracker.work[connectionID] = append(tracker.work[connectionID], RetryableWork{
		WorkType:  workType,
		LastError: errString(err),
		Attempts:  1,
		UpdatedAt: time.Now(),
	})
	return Snapshot{Status: statusForFailures(tracker.policy, failureCount), Reason: errString(err), FailureCount: failureCount, RetryableWorkCount: len(tracker.work[connectionID])}
}

// RecordProviderSuccess returns the connection to healthy while preserving
// retryable work rows until a worker completes them.
func (tracker *Tracker) RecordProviderSuccess(connectionID string) Snapshot {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.failures[connectionID] = 0
	return Snapshot{Status: drive.HealthHealthy, Reason: "provider call succeeded", RetryableWorkCount: len(tracker.work[connectionID])}
}

// RetryableWork returns a copy of the retryable work list for a connection.
func (tracker *Tracker) RetryableWork(connectionID string) []RetryableWork {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	items := tracker.work[connectionID]
	out := make([]RetryableWork, len(items))
	copy(out, items)
	return out
}

// PostgresRecorder persists health transitions and retryable work to the live
// drive read model.
type PostgresRecorder struct {
	pool   *pgxpool.Pool
	policy Policy
}

// NewPostgresRecorder returns a persistent health recorder.
func NewPostgresRecorder(pool *pgxpool.Pool, policy Policy) *PostgresRecorder {
	return &PostgresRecorder{pool: pool, policy: normalizePolicy(policy)}
}

// RecordProviderError inserts a retryable work row and updates connection
// health according to the configured thresholds.
func (recorder *PostgresRecorder) RecordProviderError(ctx context.Context, connectionID string, workType string, err error) (Snapshot, error) {
	if recorder.pool == nil {
		return Snapshot{}, fmt.Errorf("drive health: postgres pool is nil")
	}
	lastError := errString(err)
	if _, execErr := recorder.pool.Exec(ctx,
		`INSERT INTO drive_provider_work_queue
		 (id, connection_id, work_type, status, attempts, last_error)
		 VALUES ($1, $2, $3, 'retryable', 1, $4)`,
		uuid.New(), connectionID, workType, lastError,
	); execErr != nil {
		return Snapshot{}, fmt.Errorf("drive health: insert retryable work: %w", execErr)
	}
	var retryableCount int
	if scanErr := recorder.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM drive_provider_work_queue WHERE connection_id=$1 AND status IN ('queued', 'retryable', 'running')`,
		connectionID,
	).Scan(&retryableCount); scanErr != nil {
		return Snapshot{}, fmt.Errorf("drive health: count retryable work: %w", scanErr)
	}
	status := statusForFailures(recorder.policy, retryableCount)
	if _, execErr := recorder.pool.Exec(ctx,
		`UPDATE drive_connections SET status=$2, last_health_reason=$3, updated_at=now() WHERE id=$1`,
		connectionID, string(status), lastError,
	); execErr != nil {
		return Snapshot{}, fmt.Errorf("drive health: update connection status: %w", execErr)
	}
	return Snapshot{Status: status, Reason: lastError, FailureCount: retryableCount, RetryableWorkCount: retryableCount}, nil
}

// RecordProviderSuccess sets the connection healthy while preserving queued
// retryable work for workers to retry explicitly.
func (recorder *PostgresRecorder) RecordProviderSuccess(ctx context.Context, connectionID string) (Snapshot, error) {
	if recorder.pool == nil {
		return Snapshot{}, fmt.Errorf("drive health: postgres pool is nil")
	}
	var retryableCount int
	if err := recorder.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM drive_provider_work_queue WHERE connection_id=$1 AND status IN ('queued', 'retryable', 'running')`,
		connectionID,
	).Scan(&retryableCount); err != nil {
		return Snapshot{}, fmt.Errorf("drive health: count retryable work: %w", err)
	}
	if _, err := recorder.pool.Exec(ctx,
		`UPDATE drive_connections SET status='healthy', last_health_reason='provider call succeeded', updated_at=now() WHERE id=$1`, connectionID,
	); err != nil {
		return Snapshot{}, fmt.Errorf("drive health: update healthy status: %w", err)
	}
	return Snapshot{Status: drive.HealthHealthy, Reason: "provider call succeeded", RetryableWorkCount: retryableCount}, nil
}

func normalizePolicy(policy Policy) Policy {
	if policy.DegradedAfter <= 0 {
		policy.DegradedAfter = DefaultPolicy.DegradedAfter
	}
	if policy.FailingAfter <= 0 {
		policy.FailingAfter = DefaultPolicy.FailingAfter
	}
	if policy.FailingAfter < policy.DegradedAfter {
		policy.FailingAfter = policy.DegradedAfter
	}
	return policy
}

func statusForFailures(policy Policy, failureCount int) drive.HealthStatus {
	policy = normalizePolicy(policy)
	if failureCount >= policy.FailingAfter {
		return drive.HealthFailing
	}
	if failureCount >= policy.DegradedAfter {
		return drive.HealthDegraded
	}
	return drive.HealthHealthy
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
