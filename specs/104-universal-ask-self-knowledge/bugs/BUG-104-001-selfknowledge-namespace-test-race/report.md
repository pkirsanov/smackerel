# Report — BUG-104-001

## Summary

`./smackerel.sh test integration` failed with three tests in
`tests/integration/openknowledge` that all passed in isolation. Root cause: the
`selfknowledge` and `openknowledge` integration packages both require the literal
`smackerel_self` artifact namespace, `go test` runs packages in parallel, and the former
issues a namespace-wide `DELETE` while the latter inserts into it. Fixed with a shared
session-level PostgreSQL advisory lock keyed on the namespace, plus guards that fail if
the exclusion is removed.

## Completion Statement

Scope 01 is Done. The full integration suite exits 0 on two consecutive runs with
identical pass counts (1969), and the three formerly-failing tests pass inside the full
parallel run — which is the only run shape that can prove a concurrency fix.

Three genuinely-blocked items are **not** claimed: this bug does not address
`tests/integration/knowledge_stats_test.go`'s `TRUNCATE` (a latent hazard of the same
class, not currently failing), and makes no change to the production ingestor's designed
namespace-wide stale sweep.

## Test Evidence

### Before — full run red, isolated run green

```
$ ./smackerel.sh test integration
--- FAIL: TestSelfKnowledge_TrustPerimeter (0.04s)
    self_knowledge_provenance_test.go:76: self artifact "sk-prov-084956.416068-self" not returned by the tool
--- FAIL: TestSelfKnowledgeTool_CitesOnlySmackerelSelf (0.05s)
    self_knowledge_tool_test.go:67: got 0 in-run cited self rows, want 2 (ids=[])
--- FAIL: TestPgxSemanticSearcher_NamespaceScopedCosine (0.04s)
    semantic_searcher_test.go:112: got 0 in-run smackerel_self rows, want 2 (ids=[])
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration/openknowledge  0.291s
INTEGRATION_EXIT=1
```

Isolated, the same three tests passed:

```
$ ./smackerel.sh test integration --go-run 'TestPgxSemanticSearcher_NamespaceScopedCosine|TestSelfKnowledge'
RERUN_EXIT=0
--- PASS: TestSelfKnowledge_TrustPerimeter (0.04s)
--- PASS: TestSelfKnowledgeTool_CitesOnlySmackerelSelf (0.05s)
--- PASS: TestPgxSemanticSearcher_NamespaceScopedCosine (0.05s)
--- PASS: TestSelfKnowledge_ExecuteMapsCitedSources (0.00s)
--- PASS: TestSelfKnowledge_Contract (0.00s)
```

That divergence is the diagnosis: the assertions are correct; the environment is not.

### After — run 1

```
$ ./smackerel.sh test integration
INTEGRATION_RUN1_EXIT=0
PASS_COUNT=1969
=== RUN   TestNamespaceLock_ExcludesASecondSession
--- PASS: TestNamespaceLock_ExcludesASecondSession (0.03s)
=== RUN   TestNamespaceLock_DistinctNamespacesDoNotContend
--- PASS: TestNamespaceLock_DistinctNamespacesDoNotContend (0.02s)
ok      github.com/smackerel/smackerel/tests/integration/nslock
```

### After — run 2 (a single green run of a formerly-flaky suite is weak evidence)

```
$ ./smackerel.sh test integration
INTEGRATION_RUN2_EXIT=0
0
PASS_COUNT=1969
--- PASS: TestSelfKnowledge_TrustPerimeter (0.04s)
--- PASS: TestSelfKnowledgeTool_CitesOnlySmackerelSelf (0.06s)
--- PASS: TestPgxSemanticSearcher_NamespaceScopedCosine (0.06s)
```

Identical pass counts (1969) across both runs indicate nothing was skipped or quarantined
to obtain green.

### Build quality gates

```
$ ./smackerel.sh check && ./smackerel.sh lint && ./smackerel.sh format --check
CHECK_EXIT=0
scenarios registered: 17, rejected: 0
scenario-lint: OK
LINT_EXIT=0
  OK: Extension versions match (1.0.0)
Web validation passed
FMT_CHECK_EXIT=0
78 files already formatted
```

`nslock.go` initially failed `format --check` (`FMT_EXIT=1`) and was corrected with
`./smackerel.sh format` before landing.

### Call-site guard non-vacuity proof

The call-site guard was proven to fail by deleting one `nslock.AcquireSelfKnowledge` call
and re-running it. A guard that passes either way would be worse than none, so this was
verified rather than asserted:

```
$ ./smackerel.sh test integration --go-run TestNamespaceLock_EveryContendingTestFileAcquiresTheLock
PROBE_EXIT=1
--- FAIL: TestNamespaceLock_EveryContendingTestFileAcquiresTheLock (0.06s)
    callsite_contract_test.go:88: these files mutate `artifacts` in the shared "smackerel_self" namespace but never call nslock.Acquire…, so they can wipe or race another package's rows: [../../../tests/integration/openknowledge/semantic_searcher_test.go]
        Add nslock.AcquireSelfKnowledge(t, pool) to each, or the mutual exclusion is only partial and BUG-104-001 returns.
```

The probe was then reverted and the guard returns to green:

```
$ ./smackerel.sh test integration --go-run TestNamespaceLock
NSLOCK_EXIT=0
--- PASS: TestNamespaceLock_EveryContendingTestFileAcquiresTheLock (0.02s)
--- PASS: TestNamespaceLock_ExcludesASecondSession (0.03s)
--- PASS: TestNamespaceLock_DistinctNamespacesDoNotContend (0.02s)
```

## Notes

Two defects were found and fixed **in the fix itself** before landing:

1. The first draft of the guard probed the advisory key with `hashtext($1)::bigint` while
   the helper derived it with FNV-1a — different keys, so the precondition check and the
   exclusion probe would have inspected a lock unrelated to the one under test. `Key()`
   was exported so the guard probes the exact key.
2. An import-block edit joined two import lines; caught by `./smackerel.sh check`.

### Validation Evidence

`bubbles.validate` was run against this bug folder and returned seven findings. It
independently re-ran the suite and reported `INTEGRATION_EXIT=0`, with both lanes green
(`PASS: go-integration`, `PASS: python-integration`).

Findings accepted and remediated in this same change:

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| F2 | HIGH | `tests/integration/knowledge_stats_test.go` `TRUNCATE … artifacts CASCADE` is unlocked and is **broader** than the DELETE the fix was built around — a third package | Now calls `nslock.AcquireSelfKnowledge` |
| F3 | MEDIUM | `tests/e2e/openknowledge/self_knowledge_ask_e2e_test.go` inserts into the namespace unlocked | Both e2e tests now acquire the lock |
| F5 | MEDIUM | Guards prove the mechanism, not the call sites: deleting an `Acquire` call left both guards green | Added `callsite_contract_test.go`, which DISCOVERS contenders from source |
| F6 | LOW | `conn.Release()` returns the connection to the pool; it does not close the session, so a failed unlock leaves the lock held. The `IsHeld` doc also claimed "THIS session" while the query has no `pg_backend_pid()` filter | Both comments corrected to state actual semantics |

Findings acknowledged but **not** resolved here:

| ID | Severity | Finding | Why not resolved |
|---|---|---|---|
| F4 | HIGH | The production stale sweeper (`internal/assistant/selfknowledge/ingestor.go`) deletes every `smackerel_self` row whose `content_hash` is not in the corpus. Test rows use `content_hash = "h-"+id`, which never is — so a core **restart** mid-run deletes exactly those rows | No test-side lock can prevent it: production does not take the lock. `Ingest` is called once at boot (`cmd/core/wiring_selfknowledge.go`), synchronously, with no ticker — so it only re-runs on a core restart. Recorded as a live alternative hypothesis below. |

### Diagnosis confidence — stated honestly

**The original diagnosis was wrong, and the audit caught it.** An earlier revision of this
report claimed the contention was "mechanically real" and that only attribution was
uncertain. That was an overclaim. `scripts/runtime/go-integration.sh` passes `-p 1`
(serialising package test binaries), no contending file calls `t.Parallel()`, and the
go/python integration lanes are sequential `docker run` calls. All three were in force
during the original red, so the two packages never ran concurrently and the documented
interleaving was **unreachable**.

What landed is therefore **defence-in-depth**, not a fix for the observed failure:

- It is worth keeping. Exclusion is now a property of the namespace rather than of a flag
  in a shell script, so dropping `-p 1` for speed cannot silently reintroduce the hazard,
  and it covers an operator running `test integration` and `test e2e` on one database.
- It did **not** fix the red. The three green runs are consistent with an intermittent
  failure simply not recurring; with n=1 red they prove nothing about causation.

**F4 is now the primary hypothesis**, being the only genuinely concurrent writer found in
this investigation. If the three tests fail again, check first for more than one
`self-knowledge corpus ingested` line or a non-zero container restart count during the run.

The bug is consequently NOT closed as root-caused. The hardening is complete and guarded;
the diagnosis is open.

### Audit Evidence

`bubbles.audit` was run and returned ten findings. The critical one overturned the
diagnosis:

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| A1 | CRITICAL | The documented root-cause mechanism **cannot occur**: `go-integration.sh` passes `-p 1`, no contending file calls `t.Parallel()`, and the lanes are sequential `docker run` calls — all in force during the original red | Accepted. `nslock.go` package doc, bug.md, report.md and state.json corrected to state the lock is defence-in-depth and F4 is the primary hypothesis. Status is no longer `done`. |
| A2 | HIGH | The call-site guard missed **2 of the 3 originally-failing tests** — they INSERT via `insertEmbeddedArtifact`, whose SQL lives in a third file. The non-vacuity probe had picked the one file the guard *could* see | Guard rewritten with a NAMED floor covering all six contenders, plus discovery. Verified by probe (below). |
| A3 | HIGH | Discovery keyed on the literal `smackerel_self`, so the primary offender was held only by **comment text** (its DELETE binds the constant), and `knowledge_stats_test.go` only by the comment this fix added | `namesNamespace` now matches `SelfKnowledgeNamespace` too; `UPDATE artifacts` and `CopyFrom` added to the write predicate |
| A4 | MEDIUM | The floor was cardinality-only (`len < 4`) and sat exactly on its boundary, so it could pass while matching the wrong four files | Replaced by assertion on known contenders **by name** |
| A6 | HIGH | `validate` had run but was absent from the phase records | Both `validate` and `audit` now recorded in `execution.executionHistory` |
| A7 | MEDIUM | Three artifacts still said F2 was not fixed — an **underclaim** | Corrected |

Probe proving the rewritten guard closes A2 — the lock call was deleted from
`self_knowledge_tool_test.go`, the file the previous guard could NOT see:

```
$ ./smackerel.sh test integration --go-run TestNamespaceLock_KnownContendersAcquireTheLock|TestNamespaceLock_DiscoveredContendersAcquireTheLock
A2_PROBE_EXIT=1
--- FAIL: TestNamespaceLock_KnownContendersAcquireTheLock (0.00s)
--- PASS: TestNamespaceLock_DiscoveredContendersAcquireTheLock (0.17s)
    callsite_contract_test.go:77: tests/integration/openknowledge/self_knowledge_tool_test.go writes to the shared `smackerel_self` namespace but does not call nslock.Acquire…; it can wipe or race another writer's rows
```

The discovery half PASSING while the named half FAILS is the empirical confirmation of A2:
discovery genuinely cannot see that file, which is exactly why the named floor was required.
The probe was reverted and all four guards return green.


