# Scopes — BUG-104-001

## Scope Table

| Scope | Name | Status |
|---|---|---|
| SCOPE-01 | Mutual exclusion for the shared `smackerel_self` namespace | Done |

---

## Scope 01: Mutual exclusion for the shared `smackerel_self` namespace

**Status:** Done
**Depends On:** none

### Use Cases (Gherkin)

**SCN-104-001-01 — full-suite determinism**
Given `tests/integration/selfknowledge` wipes the `smackerel_self` namespace
And `tests/integration/openknowledge` inserts rows into that same namespace
When `./smackerel.sh test integration` runs both packages in parallel
Then every test passes, and the result does not depend on scheduling order

**SCN-104-001-02 — exclusion is real**
Given one session holds the namespace lock
When a second, independent database session attempts to take the same lock
Then the second session does not obtain it

**SCN-104-001-03 — unrelated namespaces do not serialise**
Given one session holds the lock for namespace alpha
When the lock state for namespace beta is inspected
Then beta is reported unlocked

### Implementation Plan

1. Add `tests/integration/nslock` with a session-level advisory lock keyed by namespace.
2. Acquire it in `TestIngestor_IdempotentWithStaleSweep` before its cleanup registration.
3. Acquire it in the three contending `openknowledge` tests.
4. Add exclusion and key-derivation guards.

### Test Plan

| Test Type | Category | File | Description | Command | Live System |
|---|---|---|---|---|---|
| Integration | `integration` | `tests/integration/nslock/nslock_test.go` | Second session excluded while lock held | `./smackerel.sh test integration` | Yes |
| Integration | `integration` | `tests/integration/nslock/nslock_test.go` | Distinct namespaces do not contend | `./smackerel.sh test integration` | Yes |
| Integration (regression) | `integration` | `tests/integration/openknowledge/*_test.go` | The three formerly-racing tests pass in a full parallel run | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] Root cause identified and confirmed by the isolation/full-run divergence
  - **Command:** `./smackerel.sh test integration --go-run 'TestPgxSemanticSearcher_NamespaceScopedCosine|TestSelfKnowledge'`
  - **Exit Code:** 0 (isolated) vs 1 (full run) — the divergence is the diagnosis
  - **Evidence:** full run produced `got 0 in-run smackerel_self rows, want 2 (ids=[])`,
    `got 0 in-run cited self rows, want 2 (ids=[])`, and
    `self artifact "sk-prov-084956.416068-self" not returned by the tool`; the same three
    tests passed alone. Zero rows (never partial) matches a namespace-wide wipe landing
    between INSERT and SEARCH, not a logic error in the assertions.

- [x] Mutual exclusion implemented in one shared importable package (no copy-paste)
  - **Command:** `./smackerel.sh check`
  - **Exit Code:** 0
  - **Evidence:** `tests/integration/nslock/nslock.go` is imported by both
    `tests/integration/selfknowledge/ingest_test.go` and the three
    `tests/integration/openknowledge` tests. Session-scoped lock on a pinned
    `*pgxpool.Conn`; released via `t.Cleanup` and, on panic/kill, by connection close.

- [x] `TP-104-001-01` exclusion guard passes — a second session cannot take a held lock
  - **Command:** `./smackerel.sh test integration`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestNamespaceLock_ExcludesASecondSession (0.03s)`.
    The guard probes `nslock.Key(ns)` — the exact key the helper locks — and fails if a
    second pool obtains it. Emptying `Acquire` makes this test fail.

- [x] `TP-104-001-02` key-derivation guard passes — distinct namespaces do not contend
  - **Command:** `./smackerel.sh test integration`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestNamespaceLock_DistinctNamespacesDoNotContend (0.02s)`.
    Prevents the helper degenerating into one global mutex, which would pass the
    exclusion guard while serialising unrelated tests and hiding a key bug.

- [x] The three formerly-failing tests pass in a FULL parallel run (targeted runs cannot
      prove a concurrency fix)
  - **Command:** `./smackerel.sh test integration`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestSelfKnowledge_TrustPerimeter (0.04s)`,
    `--- PASS: TestSelfKnowledgeTool_CitesOnlySmackerelSelf (0.06s)`,
    `--- PASS: TestPgxSemanticSearcher_NamespaceScopedCosine (0.06s)`; 1969 PASS, 0 FAIL.

- [x] Full integration suite green on TWO consecutive runs (one green run is weak evidence
      for a formerly-flaky suite)
  - **Command:** `./smackerel.sh test integration`
  - **Exit Code:** 0 on run 1 and 0 on run 2
  - **Evidence:** run 1 `INTEGRATION_RUN1_EXIT=0`, `PASS_COUNT=1969`; run 2
    `INTEGRATION_RUN2_EXIT=0`, FAIL count `0`, `PASS_COUNT=1969`. Identical pass counts
    across runs indicate no skipped or newly-quarantined tests.

- [x] Build Quality Gate: check, lint, format clean with zero warnings; no TODO/stub/default
  - **Command:** `./smackerel.sh check && ./smackerel.sh lint && ./smackerel.sh format --check`
  - **Exit Code:** 0, 0, 0
  - **Evidence:** `CHECK_EXIT=0`; `LINT_EXIT=0` with `Web validation passed`;
    `FMT_CHECK_EXIT=0` with `78 files already formatted`. `nslock.go` initially failed
    `format --check` and was corrected before landing.

- [x] Exclusion is COMPLETE across every discovered contender, not just the two packages
      in the original diagnosis (validate finding F2/F3)
  - **Command:** `./smackerel.sh test integration --go-run 'TestNamespaceLock'`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestNamespaceLock_EveryContendingTestFileAcquiresTheLock (0.02s)`.
    Locks added to `tests/integration/knowledge_stats_test.go` (`TRUNCATE … artifacts
    CASCADE`, strictly broader than the DELETE and in a THIRD package) and to both
    `tests/e2e/openknowledge/self_knowledge_ask_e2e_test.go` inserts.

- [x] Call-site guard is proven NON-VACUOUS by deleting a lock call and observing failure
  - **Command:** `./smackerel.sh test integration --go-run 'TestNamespaceLock_EveryContendingTestFileAcquiresTheLock'`
  - **Exit Code:** 1 with the call removed; 0 after restoring it
  - **Evidence:** `PROBE_EXIT=1`, `--- FAIL: TestNamespaceLock_EveryContendingTestFileAcquiresTheLock (0.06s)`
    naming `semantic_searcher_test.go`. The guard discovers contenders from source, so a
    NEW contending file is caught automatically rather than needing list maintenance.

- [x] Incorrect reasoning in helper comments corrected (validate finding F6)
  - **Command:** `./smackerel.sh check`
  - **Exit Code:** 0
  - **Evidence:** `conn.Release()` returns the connection to the POOL and does not close
    the session, so a failed unlock leaves the lock held until `pool.Close()`; the comment
    claiming "the session close below releases it" was wrong and is corrected. `IsHeld`'s
    doc claimed "THIS session" while the query has no `pg_backend_pid()` filter; corrected
    to "ANY backend", which is also the property the guard actually needs.

- [x] Residual risk recorded rather than silently absorbed (validate finding F4)
  - **Command:** `grep -n 'Ingest(ctx)' cmd/core/wiring_selfknowledge.go`
  - **Exit Code:** 0
  - **Evidence:** `res, err := ingestor.Ingest(ctx)` is called once at boot, synchronously,
    with no ticker — so the production namespace-wide stale sweep re-runs only on a core
    restart. No test-side lock can prevent it. Recorded in report.md as a live alternative
    hypothesis for the original red, which was observed once and never reproduced.
