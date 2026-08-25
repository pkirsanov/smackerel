package intelligence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// BUG-004-004 SCOPE-03 — T004-03-RETRY (classification and budget arithmetic).
//
// These are the decisions that determine whether a 3am failure alerts promptly
// or after a pointless five-minute retry storm, so they are tested as pure
// logic rather than inferred from an integration run.

func TestClassify_ValidationRejectionIsTerminal(t *testing.T) {
	// Retrying a rejected candidate produces the identical rejection. Treating it
	// as transient would spend the whole budget and delay the alert.
	err := &SynthesisValidationError{Code: FailureMissingCitation, Detail: "insight 0"}
	if got := ClassifySynthesisFailure(err); got != FailureTerminal {
		t.Fatalf("got %q, want terminal", got)
	}
}

func TestClassify_SerializationFailureIsTransient(t *testing.T) {
	// The persistence layer runs SERIALIZABLE on purpose, so a conflict is the
	// mechanism working. Classifying it terminal would turn correct concurrency
	// control into a permanent failure.
	err := &pgconn.PgError{Code: "40001", Message: "could not serialize access"}
	if got := ClassifySynthesisFailure(err); got != FailureTransient {
		t.Fatalf("got %q, want transient", got)
	}
}

func TestClassify_ConnectionAndDeadlockAreTransient(t *testing.T) {
	for _, code := range []string{"40P01", "55P03", "08006", "08003", "53300", "57014"} {
		err := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code})
		if got := ClassifySynthesisFailure(err); got != FailureTransient {
			t.Fatalf("SQLSTATE %s: got %q, want transient", code, got)
		}
	}
}

func TestClassify_ContextInterruptionIsTransient(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		if got := ClassifySynthesisFailure(fmt.Errorf("op: %w", err)); got != FailureTransient {
			t.Fatalf("%v: got %q, want transient", err, got)
		}
	}
}

// The default matters more than any individual case: an unrecognised error
// treated as retryable would burn the budget on something that was never going
// to work.
func TestClassify_UnknownErrorDefaultsToTerminal(t *testing.T) {
	if got := ClassifySynthesisFailure(errors.New("something nobody anticipated")); got != FailureTerminal {
		t.Fatalf("got %q, want terminal", got)
	}
	// A constraint violation is a data problem, not a flaky one.
	if got := ClassifySynthesisFailure(&pgconn.PgError{Code: "23505"}); got != FailureTerminal {
		t.Fatalf("unique violation: got %q, want terminal", got)
	}
}

func TestRetryPolicy_RejectsUnsafeConfiguration(t *testing.T) {
	base := SynthesisRetryPolicy{MaxAttempts: 3, InitialDelay: time.Second, MaxDelay: 10 * time.Second, LeaseTTL: time.Minute}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	for name, mutate := range map[string]func(*SynthesisRetryPolicy){
		"zero attempts":       func(p *SynthesisRetryPolicy) { p.MaxAttempts = 0 },
		"negative attempts":   func(p *SynthesisRetryPolicy) { p.MaxAttempts = -1 },
		"zero initial delay":  func(p *SynthesisRetryPolicy) { p.InitialDelay = 0 },
		"max below initial":   func(p *SynthesisRetryPolicy) { p.MaxDelay = time.Millisecond },
		"zero lease":          func(p *SynthesisRetryPolicy) { p.LeaseTTL = 0 },
		"negative lease":      func(p *SynthesisRetryPolicy) { p.LeaseTTL = -time.Second },
		"negative init delay": func(p *SynthesisRetryPolicy) { p.InitialDelay = -time.Second },
	} {
		t.Run(name, func(t *testing.T) {
			p := base
			mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatal("unsafe policy accepted")
			}
		})
	}
}

func TestRetryPolicy_BackoffDoublesAndCaps(t *testing.T) {
	p := SynthesisRetryPolicy{MaxAttempts: 6, InitialDelay: time.Second, MaxDelay: 4 * time.Second, LeaseTTL: time.Minute}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second, 4 * time.Second}
	for i, w := range want {
		if got := p.backoffFor(i + 1); got != w {
			t.Fatalf("attempt %d: got %s, want %s", i+1, got, w)
		}
	}
	// The cap is the point. Without it a modest attempt count still yields an
	// unboundedly long total wait.
	if got := p.backoffFor(20); got != 4*time.Second {
		t.Fatalf("attempt 20: got %s, want the 4s cap", got)
	}
}

func TestCoordinator_RejectsAnonymousHolder(t *testing.T) {
	// A lease with no holder cannot be attributed or reclaimed by name, which
	// defeats the recovery path it exists for.
	_, err := NewSynthesisCoordinator(&SynthesisPersistence{}, SynthesisRetryPolicy{
		MaxAttempts: 1, InitialDelay: time.Second, MaxDelay: time.Second, LeaseTTL: time.Minute,
	}, "")
	if err == nil {
		t.Fatal("anonymous holder accepted")
	}
}

func TestAdvisoryLockKey_IsStableAndDistinguishes(t *testing.T) {
	a := advisoryLockKey("cadence=daily|principal=op")
	if a != advisoryLockKey("cadence=daily|principal=op") {
		t.Fatal("advisory lock key is not stable across calls; two processes would not agree on the lock")
	}
	if a == advisoryLockKey("cadence=weekly|principal=op") {
		t.Fatal("distinct windows collided; unrelated runs would serialise against each other")
	}
}
