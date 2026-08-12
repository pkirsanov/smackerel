// Package nslock provides mutual exclusion between integration-test
// packages that contend for a single shared artifact namespace in the
// shared test database.
//
// # WHY THIS EXISTS
//
// Three test files legitimately need the literal `smackerel_self` namespace:
//
//   - tests/integration/selfknowledge exercises the self-knowledge INGESTOR.
//     Its assertions are namespace-wide by nature: it wipes the namespace to
//     establish a known baseline (`DELETE FROM artifacts WHERE source_id =
//     'smackerel_self'`) and counts the namespace to assert what ingestion
//     produced. It cannot scope either to an id prefix, because the ingestor
//     generates its own ULIDs that the test does not choose.
//
//   - tests/integration/openknowledge exercises namespace-scoped SEARCH and
//     the self-knowledge TOOL. The tool hardcodes `smackerel_self`, so those
//     tests must insert into that literal namespace or they stop testing the
//     behaviour they exist to test.
//
//   - tests/integration/knowledge_stats_test.go issues `TRUNCATE … artifacts
//     CASCADE`, which is broader still: it removes every row in the table.
//
// # THIS IS DEFENCE-IN-DEPTH, NOT A FIX FOR AN OBSERVED FAILURE
//
// Be precise here, because an earlier draft of this comment asserted the
// opposite and was WRONG. It claimed `go test` runs these packages in
// parallel. It does not:
//
//   - scripts/runtime/go-integration.sh passes `-p 1`, which serialises
//     package TEST BINARIES.
//   - No contending file calls t.Parallel().
//   - go-integration and python-integration are sequential `docker run` calls.
//
// So these packages do NOT currently run concurrently, and the
// wipe-between-INSERT-and-SEARCH interleaving is NOT reachable by that path
// today.
//
// The lock is retained because it makes exclusion a property of the NAMESPACE
// rather than of a flag in a shell script. Dropping `-p 1` for speed — an
// ordinary future change — would make the race real and silent. It also covers
// an operator running `test integration` and `test e2e` against one database.
//
// It does NOT protect against the one writer that IS genuinely concurrent with
// these tests: the production stale sweeper in
// internal/assistant/selfknowledge/ingestor.go, which runs at smackerel-core
// boot against the same live stack and deletes every `smackerel_self` row whose
// content_hash is absent from the corpus — which is every row these tests
// insert. No test-side lock can prevent that, because production does not take
// the lock.
//
// # WHY A SESSION-LEVEL ADVISORY LOCK
//
// Transaction-scoped locks (`pg_advisory_xact_lock`) are unusable here: the
// contending tests issue many independent Exec/Query calls and are not
// wrapped in a single transaction, so a transaction lock would release at the
// first statement boundary and protect nothing.
//
// Session-level locks carry the property that matters for a test suite: if a
// test binary panics or is killed, the backend connection closes and
// PostgreSQL releases the lock automatically. A leaked lock would convert a
// flaky suite into a HUNG suite, which is strictly worse than the bug being
// fixed, so automatic release on connection loss is the deciding property.
//
// The lock MUST be held on one pinned connection. pgxpool hands out an
// arbitrary connection per call, so acquiring the lock via the pool and
// releasing it via the pool would frequently target different backends —
// the unlock would fail and the lock would be held until the pool closed.
// Acquire() therefore pins a single *pgxpool.Conn for the lock's lifetime.
package nslock

import (
	"context"
	"hash/fnv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SelfKnowledgeNamespace is the contended namespace. It is duplicated here
// as a plain string rather than imported from the production package so that
// this helper has no dependency on the code under test.
const SelfKnowledgeNamespace = "smackerel_self"

// acquireTimeout bounds how long a test will wait for the namespace. It is
// deliberately generous relative to the contending tests (which run in well
// under a second) but finite: an unbounded wait would hang CI with no
// diagnosis if the lock were ever leaked by a future change.
const acquireTimeout = 60 * time.Second

// Key derives a stable advisory-lock key from a namespace string.
//
// Exported so a guard test can probe the EXACT key this package locks. A
// guard that recomputed the key by another route (say `hashtext()`) would
// silently probe a different lock and assert nothing.
//
// Advisory locks share one global keyspace per database, so the key must be
// derived from the namespace rather than hand-picked; a hand-picked constant
// silently collides with any other advisory lock that happens to choose the
// same number. FNV-1a is used because it is stable across processes and Go
// versions — the two contending packages are separate binaries and MUST
// compute the same key, so a randomly-seeded hash (Go's maphash) would be
// incorrect here.
func Key(namespace string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(namespace))
	return int64(h.Sum64())
}

// Acquire takes the exclusive namespace lock and registers its release via
// t.Cleanup. It blocks until the lock is available or acquireTimeout elapses.
//
// Every test that wipes, counts, or inserts rows in the namespace MUST call
// this, or the mutual exclusion is only partial and the race returns.
func Acquire(t *testing.T, pool *pgxpool.Pool, namespace string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), acquireTimeout)
	defer cancel()

	// Pin one connection for the lock's whole lifetime. See package doc:
	// releasing on a different backend than the one holding the lock is a
	// silent no-op that leaves the lock held.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("nslock: acquire connection for namespace %q: %v", namespace, err)
	}

	k := Key(namespace)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, k); err != nil {
		conn.Release()
		t.Fatalf("nslock: pg_advisory_lock(%d) for namespace %q: %v", k, namespace, err)
	}

	t.Cleanup(func() {
		// Use a fresh context: the acquire context may already be expired,
		// and an unlock that silently failed would leak the lock.
		uctx, ucancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ucancel()
		if _, err := conn.Exec(uctx, `SELECT pg_advisory_unlock($1)`, k); err != nil {
			// NOTE: conn.Release() below returns the connection to the POOL; it
			// does not close the session, and pgx issues no DISCARD ALL, so a
			// failed unlock leaves the advisory lock held on a pooled
			// connection until pool.Close() runs. The contending tests register
			// pool.Close before this cleanup, and t.Cleanup is LIFO, so the
			// close does run after this and bounds the leak to one test — but
			// that is a property of the CALLER's ordering, not of Release.
			t.Logf("nslock: pg_advisory_unlock(%d) for namespace %q: %v (lock remains held until this pool is closed)", k, namespace, err)
		}
		conn.Release()
	})
}

// AcquireSelfKnowledge is the convenience form for the one namespace that is
// currently contended.
func AcquireSelfKnowledge(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	Acquire(t, pool, SelfKnowledgeNamespace)
}

// IsHeld reports whether ANY backend currently holds the namespace lock. It
// exists so a guard test can assert the critical section is genuinely
// protected rather than merely calling a function named "Acquire".
//
// It deliberately does NOT filter on pg_backend_pid(): the query runs on an
// arbitrary pooled connection, which is usually not the pinned connection
// holding the lock, so a same-session filter would report false for a lock
// that is genuinely held. "Any backend" is also the property the guard needs
// — exclusion is about the lock being unavailable to others.
//
// It queries pg_locks for an advisory lock matching the derived key held by
// the current backend. Advisory-lock keys are split across classid/objid in
// pg_locks for the two-argument form; the single-argument bigint form stores
// the high 32 bits in classid and the low 32 bits in objid.
func IsHeld(ctx context.Context, pool *pgxpool.Pool, namespace string) (bool, error) {
	k := Key(namespace)
	var held bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_locks
			WHERE locktype = 'advisory'
			  AND granted
			  AND ((classid::bigint << 32) | (objid::bigint & 4294967295)) = $1
		)
	`, k).Scan(&held)
	return held, err
}
