// Package web — Spec 064 SCOPE-16 circuit breaker for WebSearchProvider.
//
// CircuitBreaker is a concurrency-safe wrapper around any
// WebSearchProvider that short-circuits subsequent Search calls once
// a configurable number of consecutive transport-class failures has
// been observed. The breaker transitions through three states:
//
//   - Closed     — forwards every call to the inner provider; tracks
//     consecutive failures.
//   - Open       — short-circuits every call with ErrCircuitOpen
//     without invoking the inner provider. After
//     HalfOpenAfter has elapsed since the trip, the
//     next call transitions to HalfOpen.
//   - HalfOpen   — allows ONE probe call. Success returns to Closed
//     (and resets the failure counter); failure returns
//     to Open and rearms the HalfOpenAfter window.
//
// Failure classification (G028 — explicit, no inference):
//
//   - ErrProviderUnreachable + ErrQuotaExceeded count as failures.
//   - ErrInvalidQuery does NOT (caller bug, not a provider problem).
//   - ErrProviderNotConfigured, ErrMalformedResponse, ErrInvalidConfig
//     also do NOT count — they are config/protocol bugs the breaker
//     cannot help with.
//   - Any other error (default arm) does NOT count toward the
//     threshold — the breaker is intentionally conservative so a
//     stray classification bug cannot inflate the trip rate.
//
// Metrics (G021 — bounded cardinality via CircuitStateRecorder):
//
//   - openknowledge_circuit_state{provider} gauge — 0 closed,
//     1 half_open, 2 open. Set on every state transition.
//   - openknowledge_circuit_trips_total{provider} counter — emitted
//     when the breaker transitions Closed→Open or HalfOpen→Open.
//
// NO-DEFAULTS (G028): every config field is REQUIRED and validated by
// NewCircuitBreaker. Operator wiring threads the values from
// assistant.open_knowledge.circuit_breaker.* in config/smackerel.yaml.
package web

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCircuitOpen is returned by Search while the breaker is in the
// Open state and the half-open window has not yet elapsed. The agent
// loop (SCOPE-09) classifies this as TerminationToolUnavailable and
// the substrate Handler maps it to RefusalToolUnavailable so the user
// sees the canonical "tool isn't available right now" body.
var ErrCircuitOpen = errors.New("openknowledge/web: circuit breaker open")

// CircuitState is the externally observable state of a CircuitBreaker.
// Exposed through CircuitBreaker.State() for tests + future operator
// dashboards (alongside the openknowledge_circuit_state gauge).
type CircuitState int

// CircuitState enum values. The integer codes are the values emitted
// to the openknowledge_circuit_state gauge — kept stable so a
// dashboard built against them does not silently break.
const (
	CircuitClosed   CircuitState = 0
	CircuitHalfOpen CircuitState = 1
	CircuitOpen     CircuitState = 2
)

// String returns the lowercase label used in test failure messages
// and dashboards. The closed/half_open/open vocabulary matches the
// gauge documentation.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitHalfOpen:
		return "half_open"
	case CircuitOpen:
		return "open"
	default:
		return "unknown"
	}
}

// CircuitConfig bundles the operator-tunable breaker parameters.
// Every field is REQUIRED (G028 — no defaults); wiring threads them
// from assistant.open_knowledge.circuit_breaker.* in
// config/smackerel.yaml. NewCircuitBreaker rejects zero / negative
// values with ErrInvalidConfig.
type CircuitConfig struct {
	// FailureThreshold is the number of consecutive countable
	// failures (ErrProviderUnreachable | ErrQuotaExceeded) that
	// trips the breaker from Closed to Open. MUST be > 0.
	FailureThreshold int
	// OpenWindow is retained for documentation parity with the
	// design's "open window" terminology; functionally the breaker
	// transitions out of Open after HalfOpenAfter elapses. Kept as a
	// REQUIRED field so the operator must think about it. MUST be > 0.
	OpenWindow time.Duration
	// HalfOpenAfter is the duration the breaker stays Open before
	// the next Search call is allowed through as a HalfOpen probe.
	// MUST be > 0 and SHOULD be <= OpenWindow.
	HalfOpenAfter time.Duration
}

// CircuitStateRecorder is the narrow metric surface the breaker
// emits to. Implementations bound the `provider` label cardinality
// (G021 — no uncontrolled label leak). *metrics.Metrics satisfies
// this interface; tests inject a fake.
type CircuitStateRecorder interface {
	SetCircuitState(provider string, stateCode int)
	IncCircuitTrip(provider string)
}

// CircuitOption is the functional-options shape used by
// NewCircuitBreaker for test seams (clock injection + recorder
// installation). All knobs are optional; defaults are real-time
// clock and no metrics.
type CircuitOption func(*CircuitBreaker)

// WithCircuitClock injects a clock function for deterministic tests.
// fn=nil is silently ignored so the default time.Now stays in place.
func WithCircuitClock(fn func() time.Time) CircuitOption {
	return func(cb *CircuitBreaker) {
		if fn != nil {
			cb.clock = fn
		}
	}
}

// WithCircuitStateRecorder installs a recorder; nil is accepted as
// "no metrics" so tests can omit the dependency entirely.
func WithCircuitStateRecorder(r CircuitStateRecorder) CircuitOption {
	return func(cb *CircuitBreaker) {
		cb.recorder = r
	}
}

// CircuitBreaker wraps a WebSearchProvider with the state machine
// described in the package doc. Safe for concurrent use; all state
// transitions are guarded by a single mutex.
type CircuitBreaker struct {
	inner    WebSearchProvider
	cfg      CircuitConfig
	recorder CircuitStateRecorder
	clock    func() time.Time

	mu             sync.Mutex
	state          CircuitState
	consecFailures int
	openedAt       time.Time
}

// NewCircuitBreaker validates cfg and wraps inner. Returns
// ErrInvalidConfig for nil inner or any non-positive cfg field.
func NewCircuitBreaker(inner WebSearchProvider, cfg CircuitConfig, opts ...CircuitOption) (*CircuitBreaker, error) {
	if inner == nil {
		return nil, fmt.Errorf("%w: NewCircuitBreaker: nil inner provider", ErrInvalidConfig)
	}
	if cfg.FailureThreshold <= 0 {
		return nil, fmt.Errorf("%w: NewCircuitBreaker: FailureThreshold must be > 0 (got %d)", ErrInvalidConfig, cfg.FailureThreshold)
	}
	if cfg.OpenWindow <= 0 {
		return nil, fmt.Errorf("%w: NewCircuitBreaker: OpenWindow must be > 0 (got %s)", ErrInvalidConfig, cfg.OpenWindow)
	}
	if cfg.HalfOpenAfter <= 0 {
		return nil, fmt.Errorf("%w: NewCircuitBreaker: HalfOpenAfter must be > 0 (got %s)", ErrInvalidConfig, cfg.HalfOpenAfter)
	}
	cb := &CircuitBreaker{
		inner: inner,
		cfg:   cfg,
		clock: time.Now,
		state: CircuitClosed,
	}
	for _, opt := range opts {
		opt(cb)
	}
	if cb.recorder != nil {
		cb.recorder.SetCircuitState(inner.Name(), int(CircuitClosed))
	}
	return cb, nil
}

// Name forwards to the inner provider so callers (the agent loop)
// see a single stable provider label.
func (cb *CircuitBreaker) Name() string { return cb.inner.Name() }

// State returns the current externally observable state. Provided
// for tests and future operator endpoints; the gauge is the
// production signal.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Search forwards to the inner provider subject to the state
// machine. The decision (forward vs short-circuit) is made under the
// mutex, the inner call runs unlocked, and the post-call accounting
// re-acquires the mutex.
func (cb *CircuitBreaker) Search(ctx context.Context, query string, k int) ([]WebSnippet, error) {
	cb.mu.Lock()
	now := cb.clock()
	switch cb.state {
	case CircuitOpen:
		if now.Sub(cb.openedAt) >= cb.cfg.HalfOpenAfter {
			cb.transitionTo(CircuitHalfOpen)
			// Fall through to forward this call as the probe.
		} else {
			cb.mu.Unlock()
			return nil, ErrCircuitOpen
		}
	case CircuitClosed, CircuitHalfOpen:
		// Forward.
	}
	cb.mu.Unlock()

	snippets, err := cb.inner.Search(ctx, query, k)

	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch {
	case err == nil:
		cb.onSuccess()
	case isCircuitCountableFailure(err):
		cb.onFailure(now)
	default:
		// Non-counting error (ErrInvalidQuery, ErrProviderNotConfigured,
		// ErrMalformedResponse, …): leave state untouched. The caller
		// still receives the typed error; the breaker just does not
		// pretend a caller-side bug is a provider outage.
	}
	return snippets, err
}

// isCircuitCountableFailure reports whether err is one of the
// transport-class sentinels that counts toward the failure threshold.
// Kept package-private — the inclusion list is part of the
// breaker's contract and must be checked end-to-end by tests.
func isCircuitCountableFailure(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrProviderUnreachable):
		return true
	case errors.Is(err, ErrQuotaExceeded):
		return true
	default:
		return false
	}
}

// onSuccess handles a non-error response from the inner provider.
// HalfOpen → Closed; in Closed simply resets the failure counter.
// Caller MUST hold cb.mu.
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case CircuitHalfOpen:
		cb.consecFailures = 0
		cb.transitionTo(CircuitClosed)
	case CircuitClosed:
		cb.consecFailures = 0
	case CircuitOpen:
		// Defensive: a forwarded probe should have set state to
		// HalfOpen first; if a stale state slips through, treat the
		// success as a recovery.
		cb.consecFailures = 0
		cb.transitionTo(CircuitClosed)
	}
}

// onFailure handles a countable failure. HalfOpen → Open (rearm);
// Closed increments and trips at threshold. Caller MUST hold cb.mu.
func (cb *CircuitBreaker) onFailure(now time.Time) {
	switch cb.state {
	case CircuitHalfOpen:
		cb.openedAt = now
		cb.transitionTo(CircuitOpen)
		cb.recordTrip()
	case CircuitClosed:
		cb.consecFailures++
		if cb.consecFailures >= cb.cfg.FailureThreshold {
			cb.openedAt = now
			cb.transitionTo(CircuitOpen)
			cb.recordTrip()
		}
	case CircuitOpen:
		// Should not happen — Search short-circuits in Open. Leave
		// state untouched.
	}
}

// transitionTo updates cb.state and emits the gauge if a recorder is
// installed. Caller MUST hold cb.mu.
func (cb *CircuitBreaker) transitionTo(s CircuitState) {
	cb.state = s
	if cb.recorder != nil {
		cb.recorder.SetCircuitState(cb.inner.Name(), int(s))
	}
}

// recordTrip emits the trips counter. Caller MUST hold cb.mu.
func (cb *CircuitBreaker) recordTrip() {
	if cb.recorder != nil {
		cb.recorder.IncCircuitTrip(cb.inner.Name())
	}
}
