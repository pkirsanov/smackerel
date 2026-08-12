//go:build integration

// Non-vacuity guard for the namespace advisory lock.
//
// A test that merely CALLS Acquire proves nothing: if the body of Acquire
// were emptied, or the `pg_advisory_lock` statement deleted, such a test
// would still pass and the cross-package race would silently return. These
// tests assert the lock's OBSERVABLE EXCLUSION PROPERTY instead — that a
// second, independent database session cannot take the same lock while the
// first holds it.
//
// If mutual exclusion is removed, TestNamespaceLock_ExcludesASecondSession
// fails, because the second session succeeds in taking a lock it should not
// have been able to take.

package nslock_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/tests/integration/nslock"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestNamespaceLock_ExcludesASecondSession is the guard that gives the fix
// its teeth. It holds the namespace lock, then proves from an INDEPENDENT
// pool (a genuinely separate backend session, which is what the contending
// test packages are) that the lock cannot be taken concurrently.
func TestNamespaceLock_ExcludesASecondSession(t *testing.T) {
	pool := openPool(t)
	const ns = "nslock-guard-namespace"

	// Precondition: nobody holds it yet. Without this, a leaked lock from an
	// earlier run would make the exclusion assertion below pass for the
	// wrong reason.
	other := openPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := other.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire second session: %v", err)
	}
	defer conn.Release()

	var freeBefore bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, nslock.Key(ns)).Scan(&freeBefore); err != nil {
		t.Fatalf("probe before: %v", err)
	}
	if !freeBefore {
		t.Fatalf("precondition failed: namespace %q was already locked before the test acquired it; the exclusion assertion would be meaningless", ns)
	}
	// Release the probe lock so the real Acquire below can take it.
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, nslock.Key(ns)); err != nil {
		t.Fatalf("release probe lock: %v", err)
	}

	// Take the real lock through the helper under test.
	nslock.Acquire(t, pool, ns)

	// It must now be observably held, AND a second independent session must
	// be unable to take it. The second half is what fails if mutual
	// exclusion is ever removed from Acquire.
	held, err := nslock.IsHeld(ctx, pool, ns)
	if err != nil {
		t.Fatalf("IsHeld: %v", err)
	}
	if !held {
		t.Error("nslock.Acquire returned but no advisory lock is granted for the namespace — the critical section is unprotected and the cross-package race is live again")
	}

	var tookIt bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, nslock.Key(ns)).Scan(&tookIt); err != nil {
		t.Fatalf("probe after: %v", err)
	}
	if tookIt {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, nslock.Key(ns))
		t.Error("a SECOND database session acquired the namespace lock while the first held it — mutual exclusion is not in force, so tests/integration/selfknowledge can still wipe the namespace mid-flight under tests/integration/openknowledge")
	}
}

// TestNamespaceLock_DistinctNamespacesDoNotContend proves the lock is keyed
// on the namespace rather than being a single global mutex. Without this, a
// helper that locked one constant key for every namespace would pass the
// exclusion test above while needlessly serialising unrelated tests — and,
// worse, would hide a future key-derivation bug.
func TestNamespaceLock_DistinctNamespacesDoNotContend(t *testing.T) {
	pool := openPool(t)
	nslock.Acquire(t, pool, "nslock-guard-alpha")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	held, err := nslock.IsHeld(ctx, pool, "nslock-guard-beta")
	if err != nil {
		t.Fatalf("IsHeld(beta): %v", err)
	}
	if held {
		t.Error("locking namespace alpha reported namespace beta as locked; the advisory key is not derived from the namespace, so unrelated namespaces would serialise against each other")
	}
}
