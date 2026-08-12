# Design — BUG-104-001 fix

## Root cause analysis

`go test` runs distinct packages in parallel. Two packages legitimately require the literal
`smackerel_self` namespace in the shared test database:

| Package | Operation | Why it cannot change |
|---|---|---|
| `tests/integration/selfknowledge` | namespace-wide `DELETE` + `count(*)` | The ingestor under test generates its own ULIDs; the test cannot scope to an id prefix it does not choose. |
| `tests/integration/openknowledge` | `INSERT` rows into the namespace | The self-knowledge tool hardcodes `smackerel_self`; another namespace would not exercise shipped behaviour. |

Interleaved, the wipe lands between the other package's INSERT and its SEARCH. The search
returns zero rows — matching the observed `got 0 …, want 2` exactly, and explaining why a
partial count was never seen.

## Chosen mechanism — session-level PostgreSQL advisory lock

`tests/integration/nslock` is a shared package both sides import. One implementation, one
key derivation; a copy-pasted helper in two packages would silently diverge and
reintroduce the race.

### Why session scope, not transaction scope

`pg_advisory_xact_lock` releases at transaction end. The contending tests issue many
independent `Exec`/`Query` calls outside any explicit transaction, so a transaction-scoped
lock would release at the first statement boundary and protect nothing.

Session locks also carry the property that decides the matter for a test suite: when the
backend connection closes — including on panic or kill — PostgreSQL releases the lock
automatically. A lock leaked on failure would turn a flaky suite into a hung suite, which
is strictly worse than the bug being fixed.

### Why the connection is pinned

`pgxpool` hands out an arbitrary connection per call. Acquiring via `pool.Exec` and
releasing via `pool.Exec` would frequently target *different* backends; the unlock would
be a silent no-op and the lock would persist until the pool closed. `Acquire()` therefore
pins one `*pgxpool.Conn` for the lock's whole lifetime and releases it in `t.Cleanup`.

### Why FNV-1a for the key

Advisory locks share one global keyspace per database, so a hand-picked constant would
collide with any other advisory lock choosing the same number — note `internal/db/migrate.go`
already uses key `42`. The key must therefore be derived from the namespace string.

FNV-1a is used because it is stable across processes and Go versions. The two contending
packages compile into **separate test binaries** and must compute the same key, so a
randomly-seeded hash (`maphash`) would be incorrect.

### Cleanup ordering

In `TestIngestor_IdempotentWithStaleSweep` the lock is acquired *before* the cleanup is
registered. `t.Cleanup` runs LIFO, so the unlock runs **last** — after the final
namespace wipe. Releasing earlier would expose the wipe to a concurrent inserter.

## Non-vacuity

`TestNamespaceLock_ExcludesASecondSession` asserts the observable exclusion property from
an independent pool: a second session must fail to take the lock while the first holds it.
Emptying `Acquire` makes the second session succeed and the test fail.

It first asserts the namespace is *unlocked* before acquiring; without that precondition a
lock leaked by an earlier run would make the exclusion assertion pass for the wrong reason.

`Key()` is exported so the guard probes the exact key the helper locks. A guard computing
the key independently (e.g. `hashtext()`) would probe a different lock and assert nothing —
a defect present in the first draft of this guard and corrected before landing.

## Rejected alternatives

| Alternative | Why rejected |
|---|---|
| Unique namespace per test | The tool hardcodes `smackerel_self`; tests would stop testing shipped behaviour. |
| Prefix-scoped cleanup | Ingestor-generated ULIDs are unpredictable; namespace-wide count is required. |
| `go test -p 1` for integration | Serialises the entire tier for one namespace's sake; large, permanent runtime cost. |
| Separate database per package | Heaviest option; the suite's stack lifecycle assumes one database. |
