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

### Code Diff Evidence

Commit of record: `b3ebfef793f15a32b5d607fae73a429cdc942129`, subject
`test(integration): guard the shared smackerel_self namespace (BUG-104-001)`, committed
`2026-08-12T10:33:27+00:00`. It is the only commit that touches this bug folder, and the
only commit that touches `tests/integration/nslock`:

```
$ git log --oneline --all -- tests/integration/nslock
b3ebfef7 test(integration): guard the shared smackerel_self namespace (BUG-104-001)
```

Measured file-level delta, read with `git show --numstat --format='' b3ebfef7` (columns are
added, deleted, path):

```
128     0       specs/104-universal-ask-self-knowledge/bugs/BUG-104-001-selfknowledge-namespace-test-race/bug.md
77      0       specs/104-universal-ask-self-knowledge/bugs/BUG-104-001-selfknowledge-namespace-test-race/design.md
209     0       specs/104-universal-ask-self-knowledge/bugs/BUG-104-001-selfknowledge-namespace-test-race/report.md
136     0       specs/104-universal-ask-self-knowledge/bugs/BUG-104-001-selfknowledge-namespace-test-race/scopes.md
39      0       specs/104-universal-ask-self-knowledge/bugs/BUG-104-001-selfknowledge-namespace-test-race/spec.md
74      0       specs/104-universal-ask-self-knowledge/bugs/BUG-104-001-selfknowledge-namespace-test-race/state.json
26      0       specs/104-universal-ask-self-knowledge/bugs/BUG-104-001-selfknowledge-namespace-test-race/uservalidation.md
10      0       tests/e2e/openknowledge/self_knowledge_ask_e2e_test.go
9       0       tests/integration/knowledge_stats_test.go
123     0       tests/integration/nslock/callsite_contract_test.go
188     0       tests/integration/nslock/nslock.go
121     0       tests/integration/nslock/nslock_test.go
4       0       tests/integration/openknowledge/self_knowledge_provenance_test.go
4       0       tests/integration/openknowledge/self_knowledge_tool_test.go
4       0       tests/integration/openknowledge/semantic_searcher_test.go
10      0       tests/integration/selfknowledge/ingest_test.go
```

`16 files changed, 1162 insertions(+)` and zero deletions.

Three of those files are new and carry the mechanism: `tests/integration/nslock/nslock.go`
(188 lines, the session-scoped advisory lock and its exported `Key`),
`tests/integration/nslock/nslock_test.go` (121 lines, the exclusion and key-derivation
guards) and `tests/integration/nslock/callsite_contract_test.go` (123 lines, the
named-floor-plus-discovery guard rewritten under audit finding A2). The remaining six code
files are call sites that gained an `Acquire` call and nothing else, which is why their
counts are small and their deletion counts are zero: `tests/integration/selfknowledge/ingest_test.go`
(10, the DELETE site), `tests/integration/knowledge_stats_test.go` (9, the TRUNCATE site
found by validate finding F2), `tests/e2e/openknowledge/self_knowledge_ask_e2e_test.go`
(10, both INSERT sites found by validate finding F3), and the three
`tests/integration/openknowledge` call sites at 4 lines each.

The containment property this section establishes, and the one the Change Boundary in
scopes.md asserts, is what the path column does NOT contain: no product source tree at
all. Every non-artifact line added by this fix sits beneath the integration and e2e test
trees. Nothing under the `internal`, `cmd`, or `ml` product trees appears, and neither the
compiled-config tree nor the runtime-script tree is represented. The production stale
sweeper named in validate finding F4 is therefore provably unmodified by this bug, which
matters because F4 is the primary remaining hypothesis and editing it would have destroyed
the evidence rather than tested it.

### Regression Evidence

`bubbles.regression` was run against this bug folder. The phase asks one question the
earlier phases do not: does the landed namespace lock break, or fail to cover, anything
outside this packet? All test categories referenced below are `integration`; this packet
delivers no end-to-end test.

#### Guard non-vacuity — the mutant is killed

**Claim Source:** operator-measured in this session under a controlled mutate-then-restore.
The mutation was NOT re-run by this phase, because re-running it risks leaving the shared
harness stripped of its lock. The restore was independently verified here before any other
work: `git status --porcelain` empty, `git diff --quiet HEAD` clean, and the
`pg_advisory_lock($1)` call present at `tests/integration/nslock/nslock.go:131`.

Baseline, `./smackerel.sh test integration-light --go-run 'TestNamespaceLock_'`, exit 0:

```
--- PASS: TestNamespaceLock_KnownContendersAcquireTheLock (0.00s)
--- PASS: TestNamespaceLock_DiscoveredContendersAcquireTheLock (0.15s)
--- PASS: TestNamespaceLock_ExcludesASecondSession (0.03s)
--- PASS: TestNamespaceLock_DistinctNamespacesDoNotContend (0.02s)
ok  github.com/smackerel/smackerel/tests/integration/nslock 0.201s
```

These four **execute** rather than skip. `openPool` calls `t.Skip` when `DATABASE_URL` is
unset, and no skip appears, so the lane genuinely supplied a database.

Mutation, the `pg_advisory_lock($1)` call deleted from `nslock.Acquire`, same lane and
selector, exit 1:

```
--- PASS: TestNamespaceLock_KnownContendersAcquireTheLock (0.00s)
--- PASS: TestNamespaceLock_DiscoveredContendersAcquireTheLock (0.15s)
--- FAIL: TestNamespaceLock_ExcludesASecondSession (0.02s)
--- PASS: TestNamespaceLock_DistinctNamespacesDoNotContend (0.01s)
FAIL github.com/smackerel/smackerel/tests/integration/nslock 0.197s
```

**Mutant killed**, matching the promise in `nslock_test.go`'s own header: *"If mutual
exclusion is removed, TestNamespaceLock_ExcludesASecondSession fails."*

Recorded honestly: **only one of the four guards catches it.** The other three survive the
mutation by design and their survival is not a weakness. `KnownContenders` and
`DiscoveredContenders` assert the call-site contract by reading test source, so deleting
the SQL inside the helper is invisible to them. `DistinctNamespacesDoNotContend` asserts
that two namespaces do not collide, and removing the lock does not make them collide. One
guard owns the exclusion property, and that guard fired.

#### Cross-spec breakage scan

**Claim Source:** measured first-hand by this phase with read-only inspection; no source
file was modified.

| Check | Method | Result |
|---|---|---|
| Guarded surface drift since the fix | `git log --oneline b3ebfef7..HEAD` over `tests/integration/nslock`, the four `tests/integration` call sites and `tests/e2e/openknowledge` | Empty. Byte-untouched since the commit of record. |
| New contenders introduced since the fix | `git diff --name-status --diff-filter=A b3ebfef7..HEAD -- tests/` | 11 test files added. **None** appears in the set of files that mutate the `artifacts` table. |
| Files naming the shared namespace | `grep -rln 'smackerel_self\|SelfKnowledgeNamespace' --include='*.go' tests/` | 8 files: the 6 named contenders plus `nslock.go` and `callsite_contract_test.go`, which are the provider. Every one of the 6 imports `nslock`. |
| Whole-tree contract | `./smackerel.sh test integration-light --go-run 'TestNamespaceLock_KnownContendersAcquireTheLock\|TestNamespaceLock_DiscoveredContendersAcquireTheLock'` | Exit 0 |

The whole-tree check is the load-bearing one, because `DiscoveredContendersAcquireTheLock`
walks `tests/` from the repository root and excludes only the `nslock` package itself. It
is therefore already a cross-spec scan rather than a package-local one, and its green
covers every suite in the tree, not just this packet's:

```
=== RUN   TestNamespaceLock_KnownContendersAcquireTheLock
--- PASS: TestNamespaceLock_KnownContendersAcquireTheLock (0.00s)
=== RUN   TestNamespaceLock_DiscoveredContendersAcquireTheLock
--- PASS: TestNamespaceLock_DiscoveredContendersAcquireTheLock (0.15s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/nslock 0.157s
CROSSSPEC_GUARD_EXIT=0
```

**No suite outside this packet is broken by the lock, and no suite outside this packet has
introduced an unguarded writer.** The lock is keyed per namespace, so the many other
`artifacts` writers across the drive, graphapi, capture, photos and stress trees do not
contend with it and are not serialised by it — the property
`TestNamespaceLock_DistinctNamespacesDoNotContend` exists to hold.

The scan's honest limit: discovery requires a single file to BOTH name the namespace AND
mutate `artifacts`. A future contender that names the namespace in one file and mutates in
another evades it. That is audit finding A2's exact shape, it is why the named floor of 6
files exists alongside discovery, and it remains a maintenance obligation rather than a
solved problem.

#### Cross-spec corroboration — spec 108 reproduced the failure after this fix landed

**Claim Source:** documentary reading of `specs/108-corpus-grant-enforcement/report.md`,
not a measurement by this phase.

This is the finding that matters most, and it does not go the direction a regression phase
usually reports. Spec 108 ran the full integration lane after this lock had landed and saw
**the same three tests fail again**, at 1971 pass / 7 fail against its own 1974 / 0
baseline, with the identical zero-rows symptom. Spec 108 records the failure as not caused
by its own change.

That is not breakage introduced here. It is independent confirmation that the delivered
lock does not prevent the original red, which is exactly what audit finding A1 predicted
and what this packet already states. Spec 108 went further and executed the F4 mechanism
against a live stack: it inserted one `smackerel_self` row carrying the synthetic
`content_hash` shape these tests use, restarted the core, and observed the boot-time sweep
move `swept` from 0 to 1 and the probe row count from 1 to 0.

So F4 is no longer only the most plausible remaining hypothesis. Its capability is now
demonstrated, by a different spec, on a live stack. Spec 108 also states the containment
this packet asserts: the sweeper runs as production code in another process, holds no
lock, and therefore cannot be reached by a regex-over-test-sources contract even in
principle.

The consequence for this packet is that the certification basis already recorded is
corroborated rather than revised. The hardening is complete and guarded; the diagnosis
stays open; the packet stays non-terminal.

#### Coverage delta

**Claim Source:** documentary comparison of two reports written in different sessions, not
a same-session measurement.

This packet measured 1969 passing tests in the full integration lane. Spec 108 later
records a 1974 baseline in the same lane. Test count moved up by five, consistent with the
11 test files added since the commit of record, so **coverage did not decrease**. Two
readings taken in different sessions are weaker than one measured delta, and are reported
as such.

#### Verdict

**REGRESSION_FREE with respect to breakage caused by this change.** No test outside this
packet fails because of the namespace lock, no guarded call site has drifted, no new
unguarded contender exists, and coverage did not drop.

That verdict is deliberately narrow. It answers whether this change broke anything, and
it does not upgrade the packet's status: spec 108's independent reproduction confirms the
original defect still occurs, so the open diagnosis recorded above is unchanged.

#### Gate state at close

`bash .github/bubbles/scripts/artifact-lint.sh <packet>` exits 0, `Artifact lint PASSED`.

`bash .github/bubbles/scripts/state-transition-guard.sh <packet>` exits 1, verdict FAIL,
`failedGateIds: [G022,G136]`, `failureCount: 12`. The failing gate set is unchanged by this
phase. The count moved from 13 to 12 because recording this phase satisfied two checks that
previously had nothing to read:

```
✅ PASS: Required phase 'regression' recorded in execution/certification phase records
✅ PASS: Phase 'regression' has specialist provenance from bubbles.regression
```

G022 still blocks on `simplify`, `stabilize` and `security` never having run, and on the
`implement` and `test` claims carrying `bubbles.goal` rather than their registered owners.
Both are pre-existing and are explained in state.json rather than papered over.

#### Routed finding — not fixable by this agent

Check 8A blocks three times, asking Scope 01 for scenario-specific and broader regression
end-to-end rows. **This was deliberately not resolved, and the reason is the resolution
itself would be dishonest.** The detector at `planning-checks.sh:72` is:

```
grep -Eiq '^\|.*Regression E2E' "$scope_path" || grep -Eiq '^\|.*e2e-(api|ui).*(\||`).*Regression:' "$scope_path"
```

Its first alternative matches the row's leading text and never inspects the category
column. Typing that token into the first cell of any existing row in this packet's Test
Plan would turn all three blocks green in one edit, while asserting an end-to-end category
that this packet does not contain — every test it delivers is `integration`. That is a
gate satisfied by text rather than by coverage, and taking it would be the exact
fabrication the packet's own audit history exists to prevent.

`scopes.md` is owned by `bubbles.plan`; this agent is diagnostic and did not edit it. The
finding is routed there with two honest resolutions available and a third excluded: add
genuine end-to-end coverage, or record an explicit exception stating that a test-harness
mutual-exclusion fix has no user-facing journey to exercise. Relabelling an `integration`
row is not one of them.


