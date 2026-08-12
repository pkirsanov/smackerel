# BUG-104-001 — `smackerel_self` namespace race between integration test packages

- **Spec:** 104-universal-ask-self-knowledge
- **Severity:** High (suite-level false red; blocks any DoD row that depends on a green integration tier)
- **Status:** Hardening landed; ROOT CAUSE NOT ESTABLISHED — see "Diagnosis correction"
- **Discovered:** 2026-08-12, during spec 108 verification

## Diagnosis correction (READ FIRST)

The original diagnosis below — cross-package concurrency — **is wrong**, and the audit
caught it. `scripts/runtime/go-integration.sh` passes `-p 1`, which serialises package test
binaries; no contending file calls `t.Parallel()`; and the go/python integration lanes are
sequential `docker run` calls. Those three serialisations were all in force during the
original red, so `tests/integration/selfknowledge` and `tests/integration/openknowledge`
**never execute concurrently** and the wipe-between-INSERT-and-SEARCH interleaving was not
reachable.

What landed is therefore **defence-in-depth**, not a fix for the observed failure. It is
worth keeping — it makes exclusion a property of the namespace rather than of a flag in a
shell script, so dropping `-p 1` for speed cannot silently reintroduce the hazard — but it
MUST NOT be recorded as having fixed the red.

**The leading hypothesis is now the production stale sweeper** (see "Unresolved" below): it
is the only genuinely concurrent writer identified anywhere in this investigation.

## Summary

`./smackerel.sh test integration` failed with exactly three failures, all in
`tests/integration/openknowledge`, while every one of them **passed in isolation**.

```
--- FAIL: TestSelfKnowledge_TrustPerimeter (0.04s)
    self_knowledge_provenance_test.go:76: self artifact "sk-prov-084956.416068-self" not returned by the tool
--- FAIL: TestSelfKnowledgeTool_CitesOnlySmackerelSelf (0.05s)
    self_knowledge_tool_test.go:67: got 0 in-run cited self rows, want 2 (ids=[])
--- FAIL: TestPgxSemanticSearcher_NamespaceScopedCosine (0.04s)
    semantic_searcher_test.go:112: got 0 in-run smackerel_self rows, want 2 (ids=[])
```

## Reproduction

```
./smackerel.sh test integration          # exit 1 — the three failures above
./smackerel.sh test integration --go-run 'TestPgxSemanticSearcher_NamespaceScopedCosine|TestSelfKnowledge'   # exit 0
```

The divergence between the full run and the targeted run is the whole signal: the
assertions are correct, the *environment* they run in is not.

## Root cause

Two packages contend for one shared namespace in the shared test database, and
`go test` runs distinct packages **in parallel**.

- `tests/integration/selfknowledge/ingest_test.go` assumes **exclusive ownership** of
  the `smackerel_self` namespace. `cleanupSelfKnowledge()` issues a namespace-wide
  `DELETE FROM artifacts WHERE source_id = 'smackerel_self'`, and `countSelfKnowledge()`
  asserts a namespace-wide `count(*)`.
- `tests/integration/openknowledge/{semantic_searcher,self_knowledge_tool,self_knowledge_provenance}_test.go`
  concurrently **INSERT** rows into that same namespace.

A namespace-wide wipe landing between another package's INSERT and its SEARCH yields
exactly **zero** rows — which is precisely the observed symptom (0, never a partial count).

## Why the obvious fixes are wrong

- **Give openknowledge its own namespace.** Rejected: the self-knowledge *tool* hardcodes
  `smackerel_self`, so those tests would stop exercising the behaviour they exist to test.
- **Scope `cleanupSelfKnowledge` to an id prefix.** Rejected: the ingestor under test
  generates its own ULIDs that the test does not choose, and `countSelfKnowledge`
  legitimately needs a namespace-wide count to assert what ingestion produced.

Neither usage is wrong. **The contention is the defect**, so the fix makes the mutual
exclusion explicit rather than rewriting either side's assertions.

## Fix

`tests/integration/nslock` — a shared helper both packages import, providing a PostgreSQL
**session-level advisory lock** keyed on the namespace.

Session-level rather than transaction-level because the contending tests issue many
independent statements and are not wrapped in one transaction; a
`pg_advisory_xact_lock` would release at the first statement boundary and protect
nothing. Session locks also release automatically when the backend connection closes,
so a panicking test cannot leak a lock and convert a flaky suite into a **hung** suite.

The lock is held on one explicitly pinned `*pgxpool.Conn`: `pgxpool` hands out an
arbitrary connection per call, so acquiring and releasing through the pool would
frequently target different backends, making the unlock a silent no-op.

## Non-vacuity guard

`TestNamespaceLock_ExcludesASecondSession` asserts the **observable exclusion property** —
that a second, independent session cannot take the lock while the first holds it — rather
than merely calling a function named `Acquire`. If mutual exclusion is removed, the second
session succeeds and the test fails.

`TestNamespaceLock_DistinctNamespacesDoNotContend` proves the key is derived from the
namespace, so the helper cannot degenerate into one global mutex that needlessly
serialises unrelated tests and hides a key-derivation bug.

`Key()` is exported specifically so the guard probes the **exact** advisory key the helper
locks; a guard that recomputed the key by another route (e.g. `hashtext()`) would silently
probe a different lock and assert nothing.

## Impact

Pre-existing. Not introduced by any change in this session — the three tests were failing
before any of this session's commits. The failure was intermittent-looking and read as a
logic bug in the assertions, which is the most expensive failure shape.

## Unresolved — the primary remaining hypothesis

`internal/assistant/selfknowledge/ingestor.go` deletes every `smackerel_self` row whose
`content_hash` is not in the corpus. Every row these tests insert uses
`content_hash = "h-"+id`, which never is — so that sweep deletes **exactly** the test rows,
yielding exactly `0`, which is the observed symptom.

`Ingest` is called once at core boot (`cmd/core/wiring_selfknowledge.go`), synchronously,
with no ticker — so it re-runs only if `smackerel-core` restarts. Unlike the test packages,
the core container is NOT serialised by `-p 1`; the lane starts the stack with
`KEEP_STACK_UP=1` and then runs `go test` against it, so a restart mid-run reproduces the
symptom precisely. **No test-side advisory lock can prevent this**, because production does
not take the lock.

Next step if the three tests fail again: check for more than one
`self-knowledge corpus ingested` log line, or a non-zero container restart count, during
the failing run. That confirms or eliminates this hypothesis directly.
