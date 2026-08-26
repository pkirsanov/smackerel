# Report: [BUG-004-004] Synthesis Persistence And Health Are Not Truthful

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Synthesis ran, produced insights, and told the operator nothing had happened.
The output lived in memory, health read a never-run sentinel as `up`, and a
reader had no way to tell "we found nothing worth saying" from "we broke".

The packet closes that gap end to end. Synthesis output is now committed
transactionally and confirmed by a post-commit read-back, so success means a row
that can be read again rather than an object that was constructed. Health is
derived from that persistence outcome by a pure mapping, so a system that never
produced, failed to write, or could not be probed can no longer report green.
The scheduler records attempts and retries durably. Four read routes expose the
result, and the Today and Status pages render one of nine exclusive states from
the same reader, so the two surfaces cannot disagree.

All five scopes are Done with zero open Definition-of-Done rows. Every lane was
executed against the live stack in the current session.

## Completion Statement

Implementation and verification are complete and measured. Five of five scopes
Done, zero unchecked Definition-of-Done rows. Unit, integration, E2E API, E2E UI
and stress lanes all pass; `check`, `lint` and `format --check` are clean.

Certification is NOT claimed. The transition guard still reports outstanding
packet-level requirements, and the top-level status stays non-terminal until
those clear. What is asserted here is exactly what was executed and observed.

## RED Stage First

Two records, both from this session, both real. Each shows a check that FAILED
first against the code as it stood, and only cleared after the code changed.

### RED stage, accessibility

The `.action` class was used across the templates with no CSS rule defined
anywhere in the repository. Every control carrying it rendered below the minimum
touch target. The check failed with the measured size, not with a generic
assertion:

```text
RED: test fails
Error: expected height >= 24, received 95.109375x19
```

### GREEN stage, accessibility

After adding the `.action` rule in `internal/web/templates.go` (`2848cd2d`) the
same check passed with no other change to the test:

```text
GREEN: 89 passed (23.7s)
```

### RED stage, restart identity

A process-start nonce was injected into `LogicalKey` to prove the restart
durability check could actually fail. It did, and named the cause rather than
merely reporting a mismatch:

```text
RED: test fails
restart identity guard: output id changed across restart
```

Removing the nonce returned the check to green. A guard that cannot be made to
fail is not evidence, so each new guard in this packet was mutated once before
its result was recorded.

## Bug Reproduction - Before Fix

- **Claim Source:** executed. The state matrix check
  (`tests/e2e/synthesis_state_matrix_e2e_test.sh`) seeds each durable state
  directly through SQL and reads the rendered pages back, so the pre-fix
  confusions are reproduced as real states rather than described.
- **Root cause, and where it lived:**
  `internal/api/health.go::getCachedIntelligenceHealth` mapped the never-run
  epoch sentinel to `up`, and mapped a freshness-probe error to `up` as well, so
  a system that had never produced any synthesis still reported green.
  `internal/intelligence/synthesis.go::RunSynthesis` returned in-memory
  `SynthesisInsight` structs and wrote nothing, so there was no row for any
  reader to recover after a restart.
- **The reproduction that mattered most** was not in the original report at all:
  a mutation test on the browser suite PASSED when it should have failed, which
  exposed a session gap in `webAuthMiddleware`. Three admit paths called the next
  handler without attaching a session, so a logged-in reader saw `auth_required`
  on a page they were entitled to read.

## Decision Record

- Durable, read-back-verified rows — not object construction or a log count —
  define synthesis success. Commit alone is not success; only a passing
  post-commit read-back yields `persisted`.
- Health truth is a **pure function** of the persistence outcome, so it is
  unit-testable without a database via an injected `SynthesisPersistenceOutcome`
  seam.
- Never-run, probe-error, write-failure, partial output, and any non-OK
  read-back can NEVER be reported healthy / persisted / `up`.
- The nine view states are exclusive and precedence-ordered
  (stale > quiet > partial > current), because a reader who sees two states at
  once learns less than one who sees the worst true one.

### Placement decision (why `internal/intelligence`, not `internal/synthesis`)

`internal/synthesis` does not exist. Creating it would add a new top-level
`internal/` package, which `internal/docfreshness/doc_freshness_test.go`
(`TestDocFreshness_AllInternalPackagesDocumented`) requires to be listed in
`docs/Development.md`. The existing home of synthesis logic (`RunSynthesis`,
`GetLastSynthesisTime`, `SynthesisInsight`) is `internal/intelligence`, so the
pure health-truth mapping was added there.

## Implementation Record

### Code Diff Evidence

- **Claim Source:** executed. Verified against `git log` and `git show --stat`
  for the commits listed below, all pushed to `main` in this session.

Executed:

```text
$ git log --oneline -8 -- internal/intelligence internal/web internal/api cmd/core
2848cd2d fix(web): give class action a real rule with a 24px minimum target
525d7bad fix(auth): attach a session on every web admit path
9eb1fc15 feat(BUG-004-004): real restart durability and Today synthesis rendering
b76c0f89 fix(synthesis): make window identity stable against corpus drift
fda66f06 feat(BUG-004-004): operator retry route and SCOPE-05 synthesis projection
9d856cc7 feat(BUG-004-004): T004-06-ALERT synthesis state metric and alert rules
7ed6a321 feat(BUG-004-004): SCOPE-04 synthesis read API over the durable model
053738ca feat(BUG-004-004): SCOPE-04 health reads durable truth, not the legacy table
```

The auth fix a mutation test forced into the open:

```text
$ git show --stat --oneline 525d7bad
525d7bad fix(auth): attach a session on every web admit path
 internal/api/router.go                             |  20 ++-
 internal/web/handler.go                            |   4 +
 internal/web/templates.go                          |   1 +
 .../report.md                                      |  73 ++++++++
 web/pwa/tests/synthesis_truth.spec.ts              | 184 +++++++++++++++++++++
 5 files changed, 279 insertions(+), 3 deletions(-)
```

The whole packet:

```text
$ git diff --stat 8671533c~1..HEAD -- internal/ cmd/ tests/ web/pwa/tests/
 tests/stress/assistant_facade_p95_test.go          |  20 +
 tests/stress/synthesis_retry_stress_test.go        | 156 ++++++
 web/pwa/tests/synthesis_truth.spec.ts              | 425 +++++++++++++++
 42 files changed, 7069 insertions(+), 35 deletions(-)
```

| Commit | Surface | What changed |
|---|---|---|
| `4735c28b` | `tests/e2e/synthesis_api_e2e_test.go` | 12 live-API checks over the four read routes |
| `5773682a` | `internal/web/synthesis_projection.go` | Nine exclusive view states plus the precedence rule |
| `9eb1fc15` | `cmd/core/wiring.go`, `tests/e2e/synthesis_restart_durability_e2e_test.sh` | Un-coupled synthesis wiring from the QF gate; restart-durability check |
| `525d7bad` | `internal/api/router.go` | `admit()` closure attaching a session on every admit path |
| `b15bccbd` | `tests/e2e/synthesis_state_matrix_e2e_test.sh` | All seven durable states seeded and asserted on both pages |
| `2848cd2d` | `internal/web/templates.go` | The missing `.action` CSS rule; controls were 19px |
| `eac70361` | state matrix, `web/pwa/tests/synthesis_truth.spec.ts` | Provenance, retry and citation-disclosure assertions |
| `0d52a1e8` | `report.md`, `scopes.md`, `docs/` | Test Plan aligned to the tests that exist |

Two of those are worth naming as defects rather than as additions. `525d7bad`
fixed a real session gap that made every page render `auth_required` for a
logged-in reader. `9eb1fc15` moved synthesis wiring out of an
`if cfg.QFDecisionsEnabled` block, where disabling an unrelated integration
silently removed the synthesis API and its UI.

`2848cd2d` is the smallest and the most easily dismissed: the `.action` class was
used across the templates with no rule defined anywhere in the repository, so
every control carrying it rendered at 19px, below the minimum touch target. The
accessibility check failed with the exact measurement `95.109375x19`, which is
what turned an invisible styling omission into a reproducible defect.

## Test Evidence

### Unit — `./smackerel.sh test unit --go --go-run 'DeriveSynthesisHealth' --verbose`

- **Phase:** implement
- **Claim Source:** executed (this session, 2026-07-25)
- **Exit Code:** 0 (`UNIT_EXIT=0`)
- **Result:** PASSED — `ok github.com/smackerel/smackerel/internal/intelligence`

Raw output (excerpt, ≥10 lines):

```
--- PASS: TestDeriveSynthesisHealth_CommittedReadBackOKFull_IsHealthyPersisted (0.00s)
--- PASS: TestDeriveSynthesisHealth_CommittedReadBackOKQuiet_IsHealthyPersisted (0.00s)
--- PASS: TestDeriveSynthesisHealth_WriteFailure_IsNeverHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_WriteFailureWithPrior_IsNeverHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_PartialOutput_IsNeverHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_OrphanReadBackMismatch_IsNeverHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_ReadBackErrorAfterCommit_IsNeverHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_CommitWithoutReadBack_IsNeverPersisted (0.00s)
--- PASS: TestDeriveSynthesisHealth_ReadBackOKButNoOutputKind_IsNeverHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_StaleVerifiedOutput_IsDegradedStaleNotHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_Running_IsNeverHealthy (0.00s)
--- PASS: TestDeriveSynthesisHealth_Regression_NeverRunAndProbeErrorAreNeverUp (0.00s)
--- PASS: TestDeriveSynthesisHealth_ClosedVocabularyMapping (0.00s)
    synthesis_health_test.go:365: verified invariants over 320 outcome combinations
--- PASS: TestDeriveSynthesisHealth_Invariants (0.00s)
ok      github.com/smackerel/smackerel/internal/intelligence    0.019s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

### Check — `./smackerel.sh check`

- **Phase:** implement
- **Claim Source:** executed (this session, 2026-07-25)
- **Exit Code:** 0 (`CHECK_EXIT=0`)

Raw output (home path prefix redacted per repo PII policy):

```
config-validate: <repo>/config/generated/dev.env.tmp.<pid> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

### Lint — `./smackerel.sh lint`

- **Phase:** implement
- **Claim Source:** executed (this session, 2026-07-25)
- **Exit Code:** 0 (`LINT_EXIT=0`)
- **Result:** `All checks passed!` — no findings on the new files.

Raw output (tail):

```
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
LINT_EXIT=0
```

---

## 2026-07-26 — /api/health Wiring + Live Health-Truth Proof (bubbles.implement)

### Wiring — internal/api/health.go

- **Claim Source:** executed (edited this session, 2026-07-26).
- The buggy `getCachedIntelligenceHealth` slow-path mapping (freshness-probe
  error → "up"; never-run epoch sentinel → "up") is REPLACED by routing the
  queryable synthesis run-state through the single HEALTH-TRUTH authority
  `intelligence.DeriveSynthesisHealth`:
  - `GetLastSynthesisTime` query error ⇒ `SynthesisPersistenceOutcome{Phase: PhaseProbeError}` ⇒ intelligence status `"down"`.
  - epoch/zero sentinel (no `synthesis_insights` row) ⇒ `{Phase: PhaseNoRun}` ⇒ `"down"`.
  - a real persisted last-synthesis timestamp ⇒ `{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull, Stale: since(last) > 48h}` ⇒ `"up"` when fresh, `"stale"` when older than the freshness budget.
- The `DeriveSynthesisHealth` pure mapping is UNCHANGED (reused, NOT
  reimplemented). The TTL cache, the nil-pool "down" branch, and the
  alert-delivery probe are preserved. Added const
  `intelligenceSynthesisFreshnessBudget = 48 * time.Hour` (the preserved
  threshold). No `internal/` package was added.

### Verify re-run (this session, 2026-07-26)

- `./smackerel.sh check` → `CHECK_EXIT=0`.
- `./smackerel.sh lint` → `LINT_EXIT=0` ("All checks passed!" + "Web validation passed").
- `./smackerel.sh test unit --go` → the two target packages pass with NO
  regression: `ok github.com/smackerel/smackerel/internal/api 5.446s` and
  `ok github.com/smackerel/smackerel/internal/intelligence (cached)`.

  The suite-level `UNIT_EXIT=1` is an ORTHOGONAL failure, NOT introduced by this
  change: `internal/docfreshness` `TestDocFreshness_AllInternalPackagesDocumented`
  reports `internal/acceptance` (tracked at HEAD, undocumented) and
  `internal/experience` (a CONCURRENT untracked package) missing from
  `docs/Development.md`. This change added ZERO new `internal/` package (only
  `internal/api/health.go` was edited); fixing docfreshness requires editing
  `docs/Development.md`, a forbidden/concurrent file for this packet.

```
ok      github.com/smackerel/smackerel/internal/api     5.446s
--- FAIL: TestDocFreshness_AllInternalPackagesDocumented (0.00s)
    doc_freshness_test.go:161: internal/ package freshness: 42 packages on disk, 2 undocumented
    doc_freshness_test.go:163: docs/Development.md is STALE: 2 internal/ package(s) exist on disk but are undocumented: acceptance, experience
FAIL    github.com/smackerel/smackerel/internal/docfreshness    0.011s
ok      github.com/smackerel/smackerel/internal/experience      0.009s
ok      github.com/smackerel/smackerel/internal/intelligence    (cached)
ok      github.com/smackerel/smackerel/internal/web     0.220s
UNIT_EXIT=1
```

### T004-05-HEALTH

- **Test:** `tests/integration/synthesis/health_test.go` · `TestNeverRunAndProbeFailureAreNeverUp` (SCN-004-004-05).
- **Claim Source:** executed (this session, 2026-07-26).
- **RED** — against the UNWIRED `health.go` (stores-only `test integration-light`
  lane, real Postgres): `RED_EXIT=1`. The live never-run assertion FAILED
  because the buggy mapping reported `"up"`, proving the test is a genuine
  adversarial live guard for the falsely-healthy bug.

```
=== RUN   TestNeverRunAndProbeFailureAreNeverUp
    health_test.go:102: never-run: GET /api/health services.intelligence.status = "up"
    health_test.go:104: FALSELY HEALTHY: never-run synthesis reported intelligence="up"; the pre-fix bug is present (want NOT "up")
--- FAIL: TestNeverRunAndProbeFailureAreNeverUp (0.01s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration/synthesis      0.136s
FAIL: go-integration-light (exit=1)
Running project-scoped integration-light stack teardown (exit cleanup, timeout 120s)...
 Container smackerel-test-nats-1  Removed
 Container smackerel-test-postgres-1  Removed
RED_EXIT=1
```

- **GREEN** — against the WIRED `health.go` (full `./smackerel.sh test integration`
  lane, real disposable stack, NO interception): `GREEN_EXIT=0`. Never-run and a
  real freshness-probe error (closed pool) both report `"down"`; a fresh
  persisted synthesis reports `"up"`.

```
=== RUN   TestNeverRunAndProbeFailureAreNeverUp
    health_test.go:102: never-run: GET /api/health services.intelligence.status = "down"
2026/07/26 02:45:09 WARN intelligence freshness check failed error="query last synthesis time: closed pool"
2026/07/26 02:45:09 WARN alert delivery freshness check failed error="query stale pending alerts: closed pool"
    health_test.go:120: probe-error: GET /api/health services.intelligence.status = "down"
    health_test.go:147: fresh-success: GET /api/health services.intelligence.status = "up"
--- PASS: TestNeverRunAndProbeFailureAreNeverUp (0.02s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/synthesis      0.127s
PASS: python-integration
GREEN_EXIT=0
```

- **Stack hygiene:** the full stack was torn down by the lane's exit trap (all
  `smackerel-test-*` containers, volumes, and network Removed). A defensive
  `docker ps --filter name=smackerel` + `./smackerel.sh down` (`DOWN_EXIT=0`)
  confirmed ZERO stray smackerel containers before and after.

## Live-DB Rows — resolved (was: outstanding)

Every row that this section once listed as unwritten is written and green. The
`/api/health` truthful-status wiring (`T004-05-HEALTH`) landed first; the durable
persistence foundation it was waiting on landed after it, and the rows that
depended on that foundation closed with it.

- `T004-05-API` — `tests/e2e/synthesis_api_e2e_test.go`, 12 checks, passing.
- `T004-06-ALERT` — live stale/failed alert clearing on verified recovery.
- `T004-07-08-API`, `T004-09-AUTH`, `T004-09-TELEMETRY` — all executed live.
- The full database derivation of the outcome — deriving a
  `SynthesisPersistenceOutcome` from the durable run ledger, with an atomic
  insight-plus-citation commit and a post-commit read-back gate, rather than the
  coarse `GetLastSynthesisTime` read-back that the first wiring used.
- All SCOPE-01 through SCOPE-05 rows.

## Uncertainty Declarations

- The pure mapping and its live wiring into `/api/health` are verified, and the
  full database derivation of the outcome from the durable run ledger — atomic
  insight-plus-citation commit with a post-commit read-back gate — is now
  implemented and proven against real PostgreSQL rather than assumed.
- What is NOT asserted: packet certification. The transition guard reports
  outstanding packet-level requirements, and nothing here claims those are met.
- The `synthesis-evidence/` screenshots are written outside `test-results/`
  because that directory is wiped between lane phases. An earlier attempt to
  keep them there lost them silently, which is worth recording so the next
  reader does not repeat it.

## Scenario Contract Evidence

`DeriveSynthesisHealth` implements the unit-level determination underlying
SCN-004-004-05 (never-run is not up) and SCN-004-004-06 (stale, failed or
unverified is never healthy). Both scenarios are now closed end to end by live
checks: the state matrix seeds each condition through SQL and reads the rendered
pages back, and the API checks exercise the same states through the four read
routes.

## Validation Summary

All lanes executed in the current session against the live stack, with the
quality guards that decide whether those lanes mean anything:

- unit, integration, E2E API, E2E UI, stress: all pass.
- `check`, `lint`, `format --check`: clean.
- Interception scan: 0 hits. Bailout scan: 0 hits.
- `regression-quality-guard`: 0 violations, exit 0.
- `artifact-lint`: PASSED. `traceability-guard`: PASSED, 0 warnings.

Certification is not claimed on the strength of this. The transition guard is
the authority on that, and it still reports work outstanding.

## Audit Verdict

No terminal audit verdict is claimed. The specialist audit phase has not been
recorded against this packet, and asserting a verdict without it would be the
exact category of untruth this bug is about.

## SCOPE-01 Implementation Phase

The packet was parked with the live-DB rows unwritten. They are written now and
green against real PostgreSQL. Every property under test is a DATABASE property,
so none of them can be proven against a fake: a serializable transaction, a
UNIQUE constraint doing the idempotence work rather than a read-then-write guard
that would race, and a rollback that must still leave an audit row standing.
They run in the integration lane against a real server.

Executed this session:

```
$ ./smackerel.sh test unit --go --go-run 'TestLogicalKey|TestDedupeStrings'
--- PASS: TestLogicalKey_IsDeterministic (0.00s)
--- PASS: TestLogicalKey_IgnoresSourceOrder (0.00s)
--- PASS: TestLogicalKey_IgnoresDuplicateSources (0.00s)
--- PASS: TestLogicalKey_NormalizesWindowToUTC (0.00s)
--- PASS: TestLogicalKey_DistinguishesEveryField (0.00s)
--- PASS: TestLogicalKey_FieldBoundariesCannotBeShifted (0.00s)
--- PASS: TestDedupeStrings_PreservesFirstOccurrenceOrder (0.00s)

$ ./smackerel.sh test integration --go-run 'TestSynthesisPersistence'
--- PASS: TestSynthesisPersistence_CommitsOneCompleteAggregate (0.04s)
--- PASS: TestSynthesisPersistence_DuplicateLogicalRunIsIdempotent (0.04s)
--- PASS: TestSynthesisPersistence_RequiredWriteFailureRollsBackAtomically (0.04s)
--- PASS: TestSynthesisPersistence_RejectsMalformedAttempts (0.03s)
ok      github.com/smackerel/smackerel/tests/integration        0.381s

$ ./smackerel.sh check -> 0    $ ./smackerel.sh lint -> 0    format -> 0
```

### The defect, confirmed rather than restated

`bug.md` records Claim Source: interpreted — reported from operator-supplied
history with no query or scheduler run behind it. Verified directly:

```
$ grep -n 'func (e \*Engine) RunSynthesis' internal/intelligence/synthesis.go
50:func (e *Engine) RunSynthesis(ctx context.Context) ([]SynthesisInsight, error) {

$ grep -rn 'INSERT INTO synthesis_insights' internal/
(no output)

$ grep -n 'synthesis_insights' internal/intelligence/synthesis.go
403:  SELECT COALESCE(MAX(created_at), '1970-01-01'::timestamptz) FROM synthesis_insights

$ grep -n -A5 'RunSynthesis(ctx)' internal/scheduler/jobs.go
215:  insights, err := s.engine.RunSynthesis(ctx)
219:          slog.Info("synthesis complete", "insights", len(insights))
```

The producer builds structs, the scheduler counts them and drops them, and the
health query reads a table nothing writes. The S1 claim is accurate.

### T004-01-MIGRATE

Migration `064_synthesis_durable_persistence.sql` adds five tables. It is picked
up automatically — the runner reads the embedded FS and sorts by filename
(`internal/db/migrate.go:13`, `:46-54`), so no registry edit exists to forget.

The integration lane applies it via `db.Migrate` before every test in this file;
all four tests below run against the migrated schema, which is the applied-and-
usable evidence.

### T004-01-COMMIT, T004-02-IDEMPOTENT, T004-03-ROLLBACK

```
$ ./smackerel.sh test integration --go-run 'TestSynthesisPersistence'
=== RUN   TestSynthesisPersistence_CommitsOneCompleteAggregate
--- PASS: TestSynthesisPersistence_CommitsOneCompleteAggregate (0.04s)
=== RUN   TestSynthesisPersistence_DuplicateLogicalRunIsIdempotent
--- PASS: TestSynthesisPersistence_DuplicateLogicalRunIsIdempotent (0.04s)
=== RUN   TestSynthesisPersistence_RequiredWriteFailureRollsBackAtomically
--- PASS: TestSynthesisPersistence_RequiredWriteFailureRollsBackAtomically (0.04s)
=== RUN   TestSynthesisPersistence_RejectsMalformedAttempts
--- PASS: TestSynthesisPersistence_RejectsMalformedAttempts (0.02s)
ok      github.com/smackerel/smackerel/tests/integration        0.280s
INTEG_RC=0
```

What each proves beyond its name:

`SCN-01` asserts through the PRODUCTION reader rather than a test-only query,
and checks that identity does not drift between commit and read-back, that
insight ORDER survives the round trip, and that the stored row counts agree with
the denormalized counters. The last one matters because a future writer could
otherwise leave counter and rows disagreeing with nothing noticing.

`SCN-02` commits the logical run, then commits it again the way a restart or a
second scheduler would. Idempotence is enforced by a UNIQUE index on
`logical_key`, not by a read-then-write guard in Go, because the guard would
race and the index cannot. The retry resolves to the ALREADY-COMMITTED output
rather than to nothing.

`SCN-03` is adversarial by construction. The failure is forced at the SECOND
insight via a duplicate primary key, so the run row, the output row and the
FIRST insight are already inside the transaction when it fires. A layer that
committed incrementally, or opened a transaction per statement, would leave
those rows behind and fail. After the abort all four tables are empty, and the
failure attempt — written in its own transaction, which is why it survives — is
grepped for every candidate string and source id from the aborted attempt. "The
failure references no uncommitted content" is checked, not assumed.

### Unit coverage of the logical key

```
$ ./smackerel.sh test unit --go --go-run 'TestLogicalKey|TestSourceSetDigest|TestNewSynthesisPersistence|TestDedupeStrings'
--- PASS: TestLogicalKey_IsDeterministic (0.00s)
--- PASS: TestLogicalKey_IgnoresSourceOrder (0.00s)
--- PASS: TestLogicalKey_IgnoresDuplicateSources (0.00s)
--- PASS: TestLogicalKey_NormalizesWindowToUTC (0.00s)
--- PASS: TestLogicalKey_DistinguishesEveryField (0.00s)
--- PASS: TestLogicalKey_FieldBoundariesCannotBeShifted (0.00s)
--- PASS: TestSourceSetDigest_IsOrderIndependent (0.00s)
ok      github.com/smackerel/smackerel/internal/intelligence    0.132s
```

### T004-01-ADVERSARIAL — one case was tautological, and running it proved it

The boundary-collision test claimed to guard the length-prefixing in
`LogicalKey`. Mutating the prefix away left it PASSING:

```
$ # worktree at HEAD, length prefix replaced with a raw write
MUTATION: length prefix removed from LogicalKey
ok      github.com/smackerel/smackerel/internal/intelligence    0.029s
```

The original case shifted a character between `Principal` and `PolicyVersion`,
which are NOT adjacent in the hash order — the two window timestamps sit between
them — so no concatenation collision was possible either way. It asserted a true
statement the prefix had nothing to do with. Rewritten to use the adjacent pair:

```
$ # same mutation, corrected test
MUTATION: length prefix removed
--- FAIL: TestLogicalKey_FieldBoundariesCannotBeShifted (0.00s)
    synthesis_persistence_test.go:113: field boundary collision: ("daily","x") and ("dail","yx") produced the same key
FAIL    github.com/smackerel/smackerel/internal/intelligence    0.031s
```

Recorded rather than quietly corrected, because a reader deciding whether the
prefix is load-bearing deserves to know the first proof of it was wrong.

The remaining half of T004-01-ADVERSARIAL — demonstrating the suite RED against
the return-and-log producer end to end — is not claimed here. That needs
`RunSynthesis` wired to this layer, which is SCOPE-02/03 work, so the item stays
unchecked.

### Build quality

```
$ ./smackerel.sh check          -> 0
$ ./smackerel.sh lint           -> 0
$ ./smackerel.sh format --check -> 0
```

## SCOPE-02 Implementation Phase

The producer that closes the loop. Synthesis now ends in a durable, read-back
verified row instead of a log line. The defect was never a wrong count -- the
count was accurate -- it was that the count described work the database never
received, so a healthy-looking log line stood in for a missing row.

Candidate validation runs BEFORE any transaction opens, in
`internal/intelligence/synthesis_validator.go`, and is covered by 15 unit tests
in `internal/intelligence/synthesis_validator_test.go`:

```text
$ ./smackerel.sh test unit --go --go-run 'TestValidator_'
ok      github.com/smackerel/smackerel/internal/intelligence    0.043s
```

They pin each refusal reason separately — uncited insight, unauthorized
citation, empty through-line, confidence out of band, missing required source
class — because a validator that refused everything for one reason would satisfy
a single aggregate test while rejecting valid work in production.

Executed this session:

```
$ ./smackerel.sh test integration --go-run 'TestSynthesisProducer'
--- PASS: TestSynthesisProducer_PersistsWhereReturnAndLogDidNot (0.28s)
--- PASS: TestSynthesisProducer_RerunOfSameWindowIsIdempotent (0.24s)
--- PASS: TestSynthesisProducer_EmptyCorpusPersistsQuietOutput (0.10s)

$ ./smackerel.sh test integration --go-run 'TestSynthesisMigration'
--- PASS: TestSynthesisMigration_BootstrapCanaryCreatesFullShape (0.04s)
--- PASS: TestSynthesisMigration_IsNonDestructive (0.00s)
--- PASS: TestSynthesisMigration_LegacyInsightsAreNotReadAsDurableOutput (0.13s)

$ ./smackerel.sh test unit --go --go-run 'TestValidator_'
22 tests, 0 failures (17 validator + 5 logical-key/helper)

$ ./smackerel.sh check -> 0    $ ./smackerel.sh lint -> 0    format -> 0
```

### What changed

`RunSynthesis` still builds insights and still returns them; nothing about the
cluster query changed. What is new is `SynthesisProducer.RunAndPersist`, which
reads the authorized corpus, builds a candidate, validates it, commits it, reads
it back, and records an attempt. The scheduler calls that instead of logging a
count, and `cmd/core` treats a construction failure as fatal -- a synthesis job
that cannot persist should not run at all, because its log line would claim work
the database never received.

### T004-01-ADVERSARIAL RED And GREEN

The requirement is a test that fails against return-and-log behaviour and passes
after persistence. Rather than run it once and describe the result, the test
carries BOTH arms permanently, so it cannot rot into a tautology:

```
ARM 1  engine.RunSynthesis(ctx)      -> insights built, 0 rows in every table
ARM 2  producer.RunAndPersist(...)   -> same corpus, rows readable
```

The RED was produced mechanically by reverting the producer to the original
defect -- report a truthful count, write nothing:

```
MUTATED: producer reverted to return-and-log
    synthesis_producer_test.go:129: got 0 run rows, want 1
--- FAIL: TestSynthesisProducer_PersistsWhereReturnAndLogDidNot (0.08s)
RESTORED
```

Restored, the same test passes. ARM 1 also pins WHERE writing belongs: a change
that made `RunSynthesis` itself write would fail the control.

### Executed evidence

```
$ ./smackerel.sh test integration --go-run 'TestSynthesis'
--- PASS: TestSynthesisPersistence_CommitsOneCompleteAggregate (0.34s)
--- PASS: TestSynthesisPersistence_DuplicateLogicalRunIsIdempotent (0.09s)
--- PASS: TestSynthesisPersistence_RequiredWriteFailureRollsBackAtomically (0.11s)
--- PASS: TestSynthesisPersistence_RejectsMalformedAttempts (0.05s)
--- PASS: TestSynthesisPersistence_QuietOutputIsDurableNotMissing (0.08s)
--- PASS: TestSynthesisPersistence_PartialOutputNamesOmissions (0.08s)
--- PASS: TestSynthesisPersistence_InvalidCandidateNeverEntersPersistence (0.04s)
--- PASS: TestSynthesisProducer_PersistsWhereReturnAndLogDidNot (0.28s)
--- PASS: TestSynthesisProducer_RerunOfSameWindowIsIdempotent (0.24s)
--- PASS: TestSynthesisProducer_EmptyCorpusPersistsQuietOutput (0.10s)
ok      github.com/smackerel/smackerel/tests/integration        1.763s

$ ./smackerel.sh test integration --go-run 'TestSynthesisMigration'
--- PASS: TestSynthesisMigration_BootstrapCanaryCreatesFullShape (0.04s)
--- PASS: TestSynthesisMigration_IsNonDestructive (0.00s)
--- PASS: TestSynthesisMigration_LegacyInsightsAreNotReadAsDurableOutput (0.13s)

$ ./smackerel.sh check  -> 0
$ ./smackerel.sh lint   -> 0
$ ./smackerel.sh format -> 0
$ ./smackerel.sh test unit --go --go-run 'TestValidator_|TestLogicalKey|TestSynthesis' -> 0
```

The 22 unit tests cover the logical key and the validator. Each validator case
asserts the exact failure code rather than merely that an error occurred, and a
positive control asserts a valid candidate is ACCEPTED -- without it, a validator
that rejected everything would satisfy every negative case.

### SCN-004-004-04 — an ordering claim that is actually proven

The rejection test asserts the tables are empty afterwards. That alone does not
prove validation runs before the transaction opens, because a validator running
INSIDE the transaction would also end empty via rollback, and the failure code
survives being moved too. The distinguishing fact is that a pre-transaction
rejection never acquires a pooled connection, so each case pins the pool acquire
count. Moving `ValidateCandidate` below `BeginTx` fails all four subtests:

```
MUTATED: ValidateCandidate now runs AFTER BeginTx
    synthesis_persistence_test.go:549: rejection acquired 1 pooled connection(s)
--- FAIL: TestSynthesisPersistence_InvalidCandidateNeverEntersPersistence (0.03s)
```

### Reuse over a second spelling

`SynthesisOutputKind` already existed in `synthesis_health.go` with
`none/full/quiet/partial`. The first draft of the validator introduced a parallel
type using `complete`. That is a capability duplication -- two vocabularies for
one concept, guaranteed to disagree eventually -- so it was removed and migration
065 was changed to speak the same words the Go constants do.

### Honest scope

The cluster query is not window-filtered. The authorized corpus is therefore
every live artifact, not the window slice, and the code says so at the function
that computes it. The check that buys is real -- a citation to a deleted or
non-existent artifact is refused -- but it is not a claim that content is
window-scoped. Narrowing the corpus requires narrowing the query in the same
change, or every insight would be refused as unauthorized.

Legacy `synthesis_insights` rows are classified, not deleted: they stay in place
and are never read as durable output. The test seeds one and asserts it does not
appear in a verified aggregate, and that it still exists afterwards.

## SCOPE-03 Implementation Phase

Durable coordination, bounded retry and lifecycle. Migration 066 adds run
lifecycle, attempt count and a lease; the coordinator adds cross-process
claiming, typed failure classification, bounded retry, stale-lease recovery and
current/stale/superseded/archived transitions.

Executed this session:

```
$ ./smackerel.sh test integration --go-run 'TestSynthesisCoordinator'
--- PASS: TestSynthesisCoordinator_SecondHolderCannotClaimSameWindow (0.11s)
--- PASS: TestSynthesisCoordinator_SameHolderMayReclaimItsOwnLease (0.07s)
--- PASS: TestSynthesisCoordinator_ExpiredLeaseIsReclaimable (0.09s)
--- PASS: TestSynthesisCoordinator_ExhaustedRetriesLeaveNoOutput (0.08s)
--- PASS: TestSynthesisCoordinator_TerminalFailureIsNotRetried (0.07s)
--- PASS: TestSynthesisCoordinator_LifecycleTransitionsPreserveAudit (0.09s)

$ ./smackerel.sh test stress --go-run 'TestSynthesisConcurrentClaims'
--- PASS: TestSynthesisConcurrentClaims_ExactlyOneHolderWins (0.06s)

$ ./smackerel.sh test unit --go --go-run '<coordinator names>'
9 tests, 0 failures (classification, retry policy, advisory key, holder)

$ ./smackerel.sh check -> 0    lint -> 0    format -> 0
```

### Six defects the tests found in this scope's own implementation

Recorded because each one was a green-looking path that was wrong, and the
shape of the mistake is more useful than the fix.

1. `ClaimWindow` could never create a run row. Its INSERT selected FROM the very
   table it was inserting into, so a new window matched nothing, inserted
   nothing, and returned success. Two holders both won the same window.
   **The same-holder test PASSED against that broken code** -- both claims
   no-opped, so neither errored. A green test agreeing for the wrong reason.
2. `lifecycle_state` and `attempt_count` were NOT NULL with no default.
3. The claim never set `created_at`.
4. `MarkSuperseded` was unreachable from `stale`, and every transition no-opped
   silently when its WHERE matched nothing -- a caller could believe an output
   had been retired while it was still served as current.
5. Under real concurrency one loser received a raw serialization error instead
   of a clean refusal. Invisible to the sequential test.
6. `Commit` treated its own claim row as a duplicate.

### Why the stress test exists

The integration test proves two holders cannot both win, but SEQUENTIALLY:
holder A claims, then B is refused. That never puts two transactions inside the
critical section, so a coordinator that dropped the advisory lock entirely would
have passed it. Sixteen concurrent holders released through a shared start gate
found defect 5 immediately.

Under SERIALIZABLE two simultaneous claims can both be valid transactions that
PostgreSQL then declines to serialize. That is the isolation level working, not
a fault, and surfacing it raw hands the caller an opaque database error when the
honest answer is that someone else holds the window -- which a retry discovers
on its next pass.

### Design notes

A lease rather than only an advisory lock: an advisory lock dies with its
session, so a process killed mid-run would park a window in `running` forever
with nothing able to reclaim it. The lease expires on a wall clock, so recovery
needs no cooperation from the dead process.

Terminal failures do not consume the retry budget. Repeating a rejected
candidate cannot change the answer and only delays the alert.

Archiving is a label change, never a delete. The test asserts attempts, insights
and citations all survive it, because an audit trail that shrinks when a run ages
out is not an audit trail.

## SCOPE-04 Health Truth Phase

Health derived its verdict from `GetLastSynthesisTime`, which reads
`MAX(created_at) FROM synthesis_insights` -- the LEGACY table. Nothing writes
that table any more. Left alone it would have reported never-run indefinitely
while real output accumulated, which is the same divergence between the report
and the store this packet exists to close, pointing the other way.

Executed this session:

```
$ ./smackerel.sh test integration --go-run 'TestSynthesisHealth'
--- PASS: TestSynthesisHealth_NeverRunIsNotUp (0.05s)
--- PASS: TestSynthesisHealth_CommittedOutputIsUpAndReadable (0.05s)
--- PASS: TestSynthesisHealth_AgedOutputIsStaleNotUp (0.05s)
--- PASS: TestSynthesisHealth_FailedAttemptIsNotClearedByAnOlderOutput (0.06s)
--- PASS: TestSynthesisHealth_LegacyRowsDoNotMakeHealthUp (0.04s)

$ ./smackerel.sh test integration --go-run 'TestSynthesis'
25 passed, 0 failed

$ ./smackerel.sh lint -> 0
```

### States that must not collapse into one another

Each test pins one pair the old mapping could confuse:

| Test | Separates |
|---|---|
| `NeverRunIsNotUp` | never-run from healthy |
| `CommittedOutputIsUpAndReadable` | a real commit from a model that returns nothing |
| `AgedOutputIsStaleNotUp` | stale from current |
| `FailedAttemptIsNotClearedByAnOlderOutput` | a current failure from an older success |
| `LegacyRowsDoNotMakeHealthUp` | durable output from legacy rows |

The second row is the control. Without it the never-run assertion could pass
simply because the model never returns anything at all.

### Honest scope

These prove the read MODEL. That the `/api/health` handler consumes it is
compile-verified but not behaviour-tested; `T004-05-API` covers that and remains
open, along with the authorization and telemetry rows, which need the read API
routes this scope has not yet added.

## SCOPE-04 Read API Phase

Three routes over the one durable read model, so the API, the health probe and
the alert evaluator cannot disagree about the state of synthesis.

Executed this session:

```
$ ./smackerel.sh test e2e --go-run 'TestSynthesisAPI'
--- PASS: TestSynthesisAPI_LatestReportsAnExplicitState (0.04s)
--- PASS: TestSynthesisAPI_RunsListIsWellFormed (0.04s)
--- PASS: TestSynthesisAPI_RejectsInvalidLimit (0.03s)
--- PASS: TestSynthesisAPI_DeniesUnauthenticatedCallers (0.03s)
--- PASS: TestSynthesisAPI_UnknownRunIsNotFound (0.03s)

$ ./smackerel.sh test integration --go-run 'TestSynthesis'
25 passed, 0 failed

$ ./smackerel.sh check -> 0    lint -> 0
```

### Two mistakes the e2e run caught in my own work

Recorded because the second one is the more dangerous kind.

1. The routes were mounted under `/api` and the tests written against
   `/api/v1`. Every route returned 404.
2. `TestSynthesisAPI_UnknownRunIsNotFound` **PASSED against those unmounted
   routes.** It wanted a 404 and the router's "404 page not found" obliged. A
   status code alone cannot distinguish a handler's not-found from a routing
   miss. It now asserts the structured error code only the handler emits, so
   the one test whose job is proving the detail route exists can no longer stay
   green if that route is deleted.

### Response shapes and why

| Decision | Reason |
|---|---|
| never-run is 200 with an explicit state, not 404 | A 404 says the endpoint has nothing to say; the honest answer is that synthesis has a state and it is never-run |
| latest and history carry no text | A caller needing counts never has to receive content it would then have to be trusted with |
| detail is a separate route, not a flag | Same reason, enforced by routing rather than by a parameter |
| `runs` serialises as `[]`, never `null` | A null reads as a missing field rather than as "no runs" |
| `ErrSynthesisOutputNotFound` sentinel | Without it the API answers 500 for both a bad id and a broken database |
| limit capped server-side, non-numeric refused | Coercion would hide a client bug and make the bound untestable from outside |

### What these prove that the integration tests cannot

The integration tests prove the read MODEL. Only these prove the handler is
wired to it and that the auth gate is real. A model can be correct while the
route returns a hardcoded shape, and nothing below the transport would notice.

## SCOPE-04 Alert And Telemetry Phase

Synthesis health existed only as a field inside an `/api/health` JSON response.
Nothing scraped it and no rule could fire on it, so an operator learned that
synthesis had stopped producing by noticing.

Executed this session:

```
$ ./smackerel.sh test unit --go --go-run 'TestMetricStateFor_|TestSynthesisAlertRules_'
--- PASS: TestMetricStateFor_StatesAreExclusiveAndOrdered (0.00s)
--- PASS: TestMetricStateFor_EveryStateIsDistinct (0.00s)
--- PASS: TestSynthesisAlertRules_ReferenceLivePublishedMetrics (0.00s)
--- PASS: TestMetricStateFor_StalenessDoesNotDowngradeAFailure (0.00s)

$ ./smackerel.sh test integration --go-run 'TestSynthesisTelemetry'
--- PASS: TestSynthesisTelemetry_RejectionAuditCarriesNoSynthesisText (0.05s)
--- PASS: TestSynthesisTelemetry_ReadTypesExposeNoContentFields (0.00s)

$ ./smackerel.sh test integration --go-run 'TestSynthesis'
27 passed, 0 failed

$ ./smackerel.sh check -> 0    lint -> 0
```

### One gauge, not a labelled series

A labelled gauge would let two states be non-zero during a scrape race, so an
alert on `state="failed"` could fire while `state="up"` was also set. One number
cannot be in two states at once, which makes exclusivity structural rather than
something each rule has to assume.

`MetricStateFor` lives beside the read model that produces the outcome, so the
gauge and the `/api/health` verdict are two renderings of ONE derivation rather
than two derivations that can drift.

### Precedence, and why it is in that order

| Rank | State | Reason |
|---|---|---|
| highest | probe-error | An unreadable database means the other fields describe nothing |
| | failed | A current failure is the more urgent fact; time passing must not soften it |
| | stale | Aged past budget, but the output is real |
| | partial | Durable and honest, but never full health |
| lowest | up | Verified and inside budget |

A commit whose read-back did NOT succeed maps to **failed**, not up. Reporting
it as up is exactly the claim this packet exists to remove.

### Three guards against silent failure, each mutation-verified

Silent is the operative word. None of these protect against a loud break.

1. **Every state distinct.** A mapping returning one value for two states would
   satisfy each individual case above while leaving one alert unreachable.
2. **Every `state == N` reachable.** A rule on `== 7` would be permanently dead.
3. **Rules name published metrics.** Prometheus never complains about a rule
   whose series does not exist. It simply never fires, which is
   indistinguishable from a healthy system. Renaming the metric in the rules
   only:

   ```
   --- FAIL: TestSynthesisAlertRules_ReferenceLivePublishedMetrics (0.00s)
       alert rules never mention "smackerel_synthesis_last_verified_output_unixtime";
       the state it describes cannot be alerted on
   ```

Plus a control asserting the four alerts exist by name, since all three checks
would pass on a file containing no synthesis rules at all.

### Telemetry stays content-free

An insight's through-line is user content derived from their corpus. If it
reaches a failure message, an audit row or a metric label, it has left the
boundary the API is careful about and landed somewhere with no access control.

The structural guard covers the case nobody would notice: `SynthesisLatest` and
`SynthesisHistoryEntry` are rendered in listings, logs and metric labels, so a
future field named `ThroughLine` would silently ship content everywhere those
appear. Adding exactly that field fails the test:

```
--- FAIL: TestSynthesisTelemetry_ReadTypesExposeNoContentFields (0.00s)
    SynthesisLatest.ThroughLine looks like a content field; latest and history
    are rendered in places with no access control
```

Its control asserts `SynthesisAggregate` still HAS `Insights` -- without that, a
codebase that had lost the ability to return content at all would pass the
content-free check trivially.

## SCOPE-04 Retry Route Phase

The last functional Scope 4 surface. An operator can now re-run a window and be
told what happened to it.

Executed this session:

```
$ ./smackerel.sh test e2e --go-run 'TestSynthesisAPI'
--- PASS: TestSynthesisAPI_LatestReportsAnExplicitState (0.04s)
--- PASS: TestSynthesisAPI_RunsListIsWellFormed (0.03s)
--- PASS: TestSynthesisAPI_RejectsInvalidLimit (0.04s)
--- PASS: TestSynthesisAPI_DeniesUnauthenticatedCallers (0.03s)
--- PASS: TestSynthesisAPI_UnknownRunIsNotFound (0.06s)
--- PASS: TestSynthesisAPI_RetryReportsAPersistedOutcome (0.46s)
--- PASS: TestSynthesisAPI_RetryRefusesUnknownCadence (0.18s)
--- PASS: TestSynthesisAPI_RetryDeniesUnauthenticatedCallers (0.04s)

$ ./smackerel.sh check -> 0    lint -> 0
```

### It reports the outcome, not the request

A 202 "accepted" would be the same shape of claim the original defect made:
true about the request, silent about whether anything was stored. The outcome
vocabulary is closed -- `persisted`, or `claimed-elsewhere` when another process
holds the window, which is coordination succeeding rather than an error.

The test does not stop at the status code. When the route reports `persisted`
it reads the returned output id back through the detail route, because a
reported commit that cannot be read is exactly the lie being repaired.

An unrecognised cadence is refused rather than defaulted. Silently running
daily would produce a real output answering a question nobody asked.

The retry principal is deliberately the SAME as the scheduled run. A distinct
one would make the retry a different logical key, so it would produce a second
output instead of converging on the existing one -- defeating the idempotence
the whole packet rests on.

## SCOPE-05 Projection Phase

The Today and Status projection, modelled on the existing `DigestPageModel`: one
pure classifier, a closed state vocabulary, content-bearing fields populated
only in states that legitimately carry content.

Executed this session:

```
$ ./smackerel.sh test unit --go --go-run 'TestSynthesisProjection_'
--- PASS: TestSynthesisProjection_ClosedStatesAreExclusive (0.00s)
--- PASS: TestSynthesisProjection_EveryStateIsReachable (0.00s)
--- PASS: TestSynthesisProjection_UnauthorizedModelIsStructurallyEmpty (0.00s)
--- PASS: TestSynthesisProjection_HasContentIsTrueForExactlyTheContentStates (0.00s)
--- PASS: TestSynthesisProjection_FailureWithPriorOutputRendersNoContent (0.00s)

$ ./smackerel.sh check -> 0    lint -> 0
```

### Nine exclusive states

`never_run`, `current`, `quiet`, `stale`, `partial`, `failed_without_output`,
`failed_with_prior_output`, `unavailable`, `auth_required`. A test asserts every
one is reachable, because an unreachable state is a UI branch no test can ever
exercise.

### Privacy clearing is structural, not a later pass

The unauthorized branch returns BEFORE any stored field is copied in. Not
populated-then-blanked -- never populated, so no later edit to the function can
leak it. The test walks the struct by reflection, so a field added later is
covered without anyone remembering to extend a list.

Keeping just `OutputID` and `InsightCount` on an unauthorized model -- the
plausible-looking mistake, since neither is prose -- fails it:

```
--- FAIL: TestSynthesisProjection_UnauthorizedModelIsStructurallyEmpty (0.00s)
    unauthorized model carries OutputID = out-1; synthesis-derived fields must
    never be populated for an unauthorized reader
```

### Failure keeps its prior output at arm's length

`failed_with_prior_output` renders no content but still NAMES that a prior
output exists. That is what lets a reader tell "nothing has ever worked" from
"the latest run failed", without the older output being presented as the current
answer. Committed-but-unverified maps to failed rather than rendering, because
showing it would put prose in front of a reader that was never proven to be in
the database.

## SCOPE-02 E2E Phase

Both rows below were held open while the read API did not exist. SCOPE-04
delivered that API, so the two scenarios are now reachable end to end and are
proven here against the live stack rather than at the integration seam alone.

Lane used for both:

```text
./smackerel.sh down
./smackerel.sh test e2e --go-run 'TestSynthesisAPI_NoOutputIsEverHalfWritten|TestSynthesisAPI_QuietWindowReadsAsRunNotBroken'
=== RUN   TestSynthesisAPI_NoOutputIsEverHalfWritten
--- PASS: TestSynthesisAPI_NoOutputIsEverHalfWritten (0.12s)
=== RUN   TestSynthesisAPI_QuietWindowReadsAsRunNotBroken
--- PASS: TestSynthesisAPI_QuietWindowReadsAsRunNotBroken (0.04s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.533s
```

### T004-04-NOWRITE

SCN-004-004-04 through the live API. A refused candidate must leave nothing
behind, so the check walks every run the list route returns and reads each
detail aggregate. It fails if a declared count outruns the rows actually
carried, if any insight arrives without a type, or if any insight arrives
uncited.

The guard is mutation-verified. Inflating the detail insight count by one in
`internal/api/synthesis.go` produced:

```text
synthesis_api_e2e_test.go:469: output 01M0Y4KR9D4KGBWRKFVAQ1N0XV declares 2
insights but carries 1; a count that outruns its rows is the signature of a
partial write
--- FAIL: TestSynthesisAPI_NoOutputIsEverHalfWritten (0.15s)
```

That failure text also settles a second question. "carries 1" means the corpus
behind the e2e stack yields a real cited insight, so the uncited-insight and
citation-count assertions run against a populated output. They are live checks,
not assertions that pass because there is nothing to inspect. The mutation was
reverted and the lane re-run clean before this evidence was recorded.

### T004-07-QUIET-E2E

```text
$ ./smackerel.sh test e2e --go-run 'TestSynthesisAPI_QuietWindowReadsAsRunNotBroken'
--- PASS: TestSynthesisAPI_QuietWindowReadsAsRunNotBroken (0.09s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.208s
EXIT=0
```

SCN-004-004-07 through the live API. A quiet window means the system looked and
found nothing worth saying, which is a successful run. The check drives a real
trigger, requires it to name an output id, then reads `/api/synthesis/latest`
and fails if that committed output reads back as `never-run` or as `failed`.
It also fails if a `quiet` state reports a non-zero insight count, which would
mean the kind and the payload disagree.

This is the precise confusion this bug exists to remove: emptiness reported as
absence, or as breakage.

Requiring the trigger to name an output id first is what keeps the rest
non-vacuous. Without it the check could run against a system that had committed
nothing, where "not never-run" would be asserting something about an empty
database rather than about a real quiet output.

The insight-count assertion guards the opposite failure: a state labelled quiet
while carrying content would be just as untruthful as content labelled empty.

## SCOPE-03 Restart Durability Phase

Both rows below were held open with the note that the e2e harness did not
perform a process restart. That was true of the Go e2e lane, which runs inside a
toolchain container with no access to the container runtime. It was not true of
the shell e2e lane, which runs on the host. The capability was therefore built
rather than the rows being left annotated.

`tests/e2e/synthesis_restart_durability_e2e_test.sh` restarts the real core
container through the repo-standard `smackerel_compose` helper and then asks the
running system the same questions again.

One detail decides whether this test is worth anything. A single health probe
immediately after a restart can succeed against the OLD process, which is still
holding the socket for a moment before it goes down. The first working version
did exactly that, reported "healthy after 0s", and then failed on a connection
reset. The gate now requires three consecutive healthy probes spaced two seconds
apart, which a listener that is about to disappear cannot satisfy.

```text
./smackerel.sh test e2e --shell-run synthesis_restart_durability_e2e_test.sh
=== T004-02-RESTART / T004-06-RECOVERY: durability across a real restart ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
committed output before restart: 01M0Y8M4HT9MK0JJKHG9CT6E63
--- restarting the core process ---
core process restarted and serving again
PASS: T004-02-RESTART — window identity 01M0Y8M4HT9MK0JJKHG9CT6E63 survived a real process restart
PASS: T004-06-RECOVERY — health and history both recovered 01M0Y8M4HT9MK0JJKHG9CT6E63 from storage (state 'quiet')
=== durability across a real restart: complete ===
  PASS: synthesis_restart_durability_e2e_test.sh
  Total:  1
  Passed: 1
  Failed: 0
```

### T004-02-RESTART

The same window must resolve to the same durable identity after the process that
created it is gone. The check triggers a real synthesis, records the output id,
restarts the core process, triggers the same window again, and requires the two
ids to be equal.

Mutation-verified against the exact defect it exists to catch. Adding a
process-start nonce to `SynthesisRunKey.LogicalKey` simulates identity living in
process memory rather than in the database, and produced:

```text
committed output before restart: 01M0Y8F06C9DG23KGRASK7QS0P
--- restarting the core process ---
core process restarted and serving again
FAIL: the same window resolved to 01M0Y8F06C9DG23KGRASK7QS0P before the restart
and 01M0Y8FEGKTZYFH5JF05GRT4R5 after it;
      identity did not survive the process, so it was never durable
```

The nonce was reverted and the lane re-run clean before this evidence was
recorded.

### T004-06-RECOVERY

After the restart the read path must recover what happened from storage. The
check requires that `/api/synthesis/latest` does not report never-run, that it
names the same output id committed before the restart, and that the same id is
present in the run history.

The history assertion is not redundant. A recovery that restored the single
latest view but not the history would leave the two read surfaces telling a
reader different stories about the same window, which is a quieter version of
the same dishonesty.

Naming the SAME id matters more than finding any id. A read path that recovered
a different run would still answer confidently, and a test that only checked for
non-emptiness would accept it.

The never-run check is stated explicitly rather than folded into the id
comparison, because never-run is the exact failure this bug is about: real work
committed, then reported to a reader as though it had never occurred.

## SCOPE-05 Browser Rendering Phase

Today and Status now render the durable synthesis projection, and
`web/pwa/tests/synthesis_truth.spec.ts` exercises it in a real browser against
the live stack with no request interception.

```text
./smackerel.sh test e2e-ui
✓ synthesis_truth.spec.ts:61:1 › Today renders exactly one synthesis state drawn from the closed set (1.0s)
✓ synthesis_truth.spec.ts:68:1 › a real committed synthesis is rendered from storage, never as empty or broken (883ms)
✓ synthesis_truth.spec.ts:126:1 › Today and Status report the same durable synthesis state (1.2s)
✓ synthesis_truth.spec.ts:141:1 › an unauthenticated visitor is never shown synthesis content (923ms)
✓ synthesis_truth.spec.ts:164:1 › the retry control is offered only when synthesis has actually failed (881ms)
81 passed (40.6s)
```

### The first version of this suite was vacuous, and the mutation is what proved it

The suite initially passed while the durable reader was deliberately unwired.
That is the outcome a green suite is supposed to make impossible, so the pass
was worthless until it was explained.

The cause was a real defect, not a test artifact. `webAuthMiddleware` had three
paths that admitted a request by calling `next.ServeHTTP(w, r)` without
attaching an `auth.Session`, while the bearer middleware alongside it did attach
one. A handler asking the context whether the caller was authorized therefore
got FALSE for a perfectly valid browser session, so every page rendered
`auth_required` — including when the reader was correctly wired.

`auth_required` clears every content-bearing field by design. That is right for
a real auth failure and fatal for a test: the suite was asserting "no synthesis
prose is present" against a page that was never going to contain any, and would
have passed no matter what the durable state was.

Two changes fixed it. The middleware now attaches a session on every admit path,
so `SessionFromContext` is a reliable "this request cleared the gate" signal.
And the spec now refuses `auth_required` and `unavailable` for an authenticated
reader, because both mean the page never reached durable state:

```text
Error: /digest reported 'unavailable' for an authenticated reader; the page is
not reading durable state, so every other assertion here would be vacuous
  at requireAnsweredState (web/pwa/tests/synthesis_truth.spec.ts:42:9)
```

### Mutation-verified in both directions

With `svc.webHandler.SynthesisReader` left unwired in `cmd/core/wiring.go`:

```text
✘ synthesis_truth.spec.ts:61:1 › Today renders exactly one synthesis state drawn from the closed set (754ms)
✘ synthesis_truth.spec.ts:68:1 › a real committed synthesis is rendered from storage, never as empty or broken (1.0s)
✘ synthesis_truth.spec.ts:126:1 › Today and Status report the same durable synthesis state (570ms)
✓ synthesis_truth.spec.ts:141:1 › an unauthenticated visitor is never shown synthesis content (808ms)
✘ synthesis_truth.spec.ts:164:1 › the retry control is offered only when synthesis has actually failed (733ms)
```

Four of five fail. The fifth passing is correct rather than a gap: the
unauthenticated check does not depend on the reader being wired, so a mutation
of the wiring should not disturb it. A mutation that broke every test equally
would have told us less, because it would not distinguish the tests that read
durable state from the one that deliberately does not.

The wiring was restored and the lane re-run clean before this evidence was
recorded.

### One reader, two pages

Both pages call the same `synthesisModel` through the same injected reader, and
one test compares the state Today reports against the state Status reports. Two
pages computing this separately is precisely how they would drift into telling a
reader different stories, so the agreement is asserted rather than assumed.

### T004-05-07-UI

The browser suite proves the rendering path works for whatever state the live
stack happens to be in. On a healthy stack that is one state, which left stale,
partial and both failure shapes asserted nowhere.

`tests/e2e/synthesis_state_matrix_e2e_test.sh` drives the database into each
durable state in turn, through the same columns the read model reads, and
asserts what the server actually renders on BOTH pages:

```text
./smackerel.sh test e2e --shell-run synthesis_state_matrix_e2e_test.sh
=== SCN-004-004-05: durable synthesis state matrix ===
PASS: never-run renders 'never_run' on both Today and Status
PASS: quiet window renders 'quiet' on both Today and Status
PASS: quiet carries no prose (asserted inside assert_state)
PASS: current renders 'current' on both Today and Status
PASS: current renders persisted prose with citation disclosure
PASS: stale renders 'stale' on both Today and Status
PASS: partial renders 'partial' on both Today and Status
PASS: failure with no prior output renders 'failed_without_output' on both Today and Status
PASS: failure with a prior output renders 'failed_with_prior_output' on both Today and Status
=== durable synthesis state matrix: all seven states render exclusively ===
  PASS: synthesis_state_matrix_e2e_test.sh
  Passed: 1
  Failed: 0
```

Exclusivity is asserted by COUNT, not just by value: each page must carry
exactly one `data-synthesis-state` marker. A page showing two states at once
would satisfy a value check and fail this one.

Mutation-verified. Removing the `outcome.Stale` branch from the projection makes
a backdated output claim to be current:

```text
FAIL: /digest rendered state 'current', want 'stale'
```

The branch was restored and the matrix re-run clean before this evidence was
recorded.

One seeding attempt was refused by the database rather than by the test, which
is worth recording because it is the schema defending its own invariant: a
failed attempt must carry a `failure_class`, and the first version of the seed
omitted it (`synthesis_run_attempts_failure_fields`). The seed was corrected to
supply one rather than the constraint being worked around.

### T004-09-10-UI

Accessibility, reflow and keyboard behavior are asserted in the real browser:

```text
./smackerel.sh test e2e-ui
✓ the synthesis section is reachable and labelled in the accessibility tree (593ms)
✓ auth loss removes synthesis content from the accessibility tree, not just the DOM (332ms)
✓ the synthesis section reflows at 320px without horizontal scroll (436ms)
✓ the synthesis section reflows at 200% zoom without horizontal scroll (483ms)
✓ the retry control class meets the minimum target size (474ms)
88 passed (23.2s)
```

The accessibility assertions query by ROLE rather than by DOM id. That matters
for the auth case in particular: content can be visually gone and still be
announced, so absence is asserted in the tree assistive technology actually
reads, including the citation counts that would otherwise leak the existence of
a synthesis the reader is not entitled to.

Mutation-verified. Removing `aria-labelledby` from the section fails exactly one
test and leaves the rest passing, which is the correct shape — the others do not
depend on that attribute:

```text
✘ the synthesis section is reachable and labelled in the accessibility tree (6.0s)
Error: the section must be labelled by its heading so assistive technology can announce it
```

### A real accessibility defect this found

The target-size check did not pass and then get recorded. It FAILED, and the
failure was correct:

```text
Error: a control with class "action" renders 95.109375x19, below the 24px
minimum target size
```

`class="action"` was in use across several templates with NO rule behind it
anywhere in the stylesheet, so every such control rendered 19px tall. The fix
adds the missing `.action` rule with a 24px minimum, which repairs the retry
control and every other existing user of that class at once.

An earlier version of this check tried to measure live `.action` controls on the
page and refused to run, reporting that no control was present to size-check.
That refusal was the non-vacuity guard working: the retry control only renders
in a failure state a healthy stack does not reach. The check now measures what
the class is worth in the stylesheet the page just shipped, and the matrix test
asserts separately that the failure states render a real `<button>` carrying
that class.

### T004-RETRY-UI

```text
✓ the retry control is offered only when synthesis has actually failed (481ms)
✓ a retry reports a persisted outcome and the page agrees with it (427ms)
✓ reading run history never triggers a run and never masquerades as no history (205ms)
PASS: failed_without_output offers a real retry button
PASS: failed_with_prior_output offers a real retry button
```

Retry is asserted in both directions. A failure state must offer it, as a
focusable button rather than text that reads like one; a healthy state must NOT,
because implying something is wrong when it is not is the same dishonesty
pointing the other way.

The history check asserts that reading never writes. A history view that quietly
triggered a run would inflate the very record it claims to report, so the run
count is compared across repeated reads including a narrowed one, and the first
read must be non-empty so an empty history could not satisfy it.

### T004-01-UI

The content assertions and a captured screenshot of the real rendered section.
The screenshot is written to `web/pwa/synthesis-evidence/` rather than
`test-results/`, because a later lane phase wipes the latter and the images were
disappearing before the run finished.

```text
✓ a real committed synthesis is rendered from storage, never as empty or broken (657ms)
PASS: current renders persisted prose with citation disclosure
89 passed (23.7s)
```

The captured quiet-state render reads:

```text
Synthesis
Quiet window — synthesis ran over 17 item(s) and found nothing worth reporting.
Persisted 2026-08-26T17:30:35Z · run 01M0ZHZBBY9AAR54F826GKGJN4
```

That is the whole point of this bug in one frame. A window that produced nothing
does not render as "no data" or as an error; it names how many items it
evaluated, when it was persisted, and which run it was.

### T004-02-UI

```text
✓ a rerun of the same window adds no duplicate to Today or run history (481ms)
```

Triggering the same window twice must resolve to ONE identity, and that identity
must appear exactly once in history.

The occurrence count is asserted rather than mere presence. That distinction is
the whole test: a duplicated record would still be "present", so a membership
check would pass while history silently double-counted the same work. Counting
occurrences is the only form of the assertion that can fail for the reason it
exists.

The check also re-reads Today afterwards and requires exactly one rendered
section, because a duplicate could surface either in the history list or as a
second rendering on the page, and only asserting one of those would leave the
other free to regress.

### T004-03-04-UI

A rejected or failed candidate must expose neither prose nor the citation counts
that would reveal a synthesis exists at all:

```text
PASS: failed_without_output offers a real retry button and leaks no citation disclosure
PASS: failed_with_prior_output names the prior run without presenting it as the answer
```

Asserting the absence of citation counts matters separately from the absence of
prose. A count is an existence hint: it tells a reader a synthesis happened even
when its text is withheld, which is a smaller leak of the same kind.

The two failure states are checked separately rather than together. They differ
in exactly one respect — whether an older verified output exists — and that
difference is what decides whether the page may name a prior run at all. A
shared assertion over both would not notice if one state started borrowing the
other's behavior.

Both are seeded through the same columns the read model reads, so the states are
real rather than simulated at the template layer.

### T004-06-08-UI

```text
PASS: stale names the durable output behind it
PASS: partial names the durable output behind it
PASS: failed_with_prior_output names the prior run without presenting it as the answer
```

A degraded state must still say WHICH run it is describing. Rendering a
limitation without naming the output behind it leaves a reader unable to tell
one stale answer from another, and unable to check what they are looking at
against the run history.

The assertion is that the specific output id appears in the rendered HTML, not
that some id appears. An implementation that emitted a placeholder or the most
recent id regardless of which run the state describes would satisfy a looser
check and still mislead.

`partial` is included alongside `stale` because both are degraded-but-durable:
they carry real content whose limits must be stated, which is a different
obligation from a failure that carries no content at all.

### T004-BROAD

```text
✓ Today and Status report the same durable synthesis state (541ms)
89 passed (23.7s)
8 passed (7.6s)
8 passed (6.7s)
```

The whole browser suite passes alongside the pre-existing Today and Status
journeys, and the two surfaces are asserted to agree rather than merely to work
in isolation.

Agreement is the load-bearing claim. Each page could pass every one of its own
assertions while reading a different row, and no per-page test would notice.
Comparing the state Today reports against the state Status reports is what makes
the shared reader observable from outside.

The three trailing counts are the other lane phases in the same run, included
because a change that fixed synthesis while breaking an unrelated journey would
still be a regression this packet introduced.

## SCOPE-05 Packet Closeout

```text
unit --go            FAIL lines: 0
integration --go     FAIL lines: 0
e2e                  EXIT=0  Passed: 36  Failed: 0  Skipped: 0
e2e-ui               89 passed (13 synthesis tests)
stress               EXIT=0  FAIL lines: 0
check                compile errors: 0
lint                 lint findings: 0
format --check       unformatted: 0
interception hits    0
bailout hits         0
regression-quality-guard  0 violations, exit 0
```

Every lane in the packet was executed in this session against the live stack,
with the quality guards that decide whether those lanes are worth anything.

The lanes and the guards are recorded together deliberately. A green lane proves
nothing on its own if the tests inside it intercept their own traffic or bail
out early, so the interception and bailout scans are part of the same claim
rather than a separate nicety.

The subsections below give the raw output for each, plus one line of e2e output
that looks like a failure and is not, because a reflexive grep would otherwise
make an honest green run appear red.

Docs are included in the same closeout because an operator reading `unavailable`
at three in the morning needs to know it points at the database rather than at
synthesis, and that distinction lives in prose rather than in a test.

### Every lane, current session

```text
./smackerel.sh test unit --go            FAIL lines: 0
./smackerel.sh test integration --go     FAIL lines: 0
./smackerel.sh test e2e                  E2E_EXIT=0   Passed: 36  Failed: 0  Skipped: 0
./smackerel.sh test e2e-ui               89 passed
./smackerel.sh test stress               STRESS_EXIT=0  test FAIL lines: 0
./smackerel.sh check                     compile errors: 0
./smackerel.sh lint                      lint findings: 0
./smackerel.sh format --check            unformatted: 0
```

One line in the e2e output reads `FAIL: Services did not become healthy within
8s`, and it is not a failure. It is the expected output of a check that stops
postgres deliberately:

```text
Stopping postgres to force a readiness failure...
 Container smackerel-test-postgres-1  Stopped
Waiting for services to be healthy (max 8s)...
FAIL: Services did not become healthy within 8s
PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=1)
```

Recording it matters because a grep for `FAIL` would otherwise make an honest
green run look red, and the reflex fix would be to filter the pattern rather
than read it.

### Test-quality guards

```text
interception hits: 0
bailout hits: 0
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 1
GUARD_EXIT=0
```

No `page.route`, `context.route`, `cy.intercept`, `msw`, `nock` or `wiremock`
appears in the browser suite, so every assertion runs against the real stack.
No early-return or `test.skip` bailout appears either, which is what stops a
missing feature from being reported as a pass.

### Docs

`docs/Operations.md` gains an operator section covering the four read routes,
the nine states with what an operator should read into each, and the commands
that verify the surface. It names the distinction that matters most in practice:
`quiet` versus `never_run` versus `failed_without_output` all show no prose, and
conflating them is the confusion this packet removed.

`docs/Development.md` documents migrations 064, 065 and 066, including why the
two defaults in 066 exist — they let the migration add NOT NULL columns to rows
that already exist, and every write path sets both explicitly rather than
relying on them.

## Specialist Phase Records

The six specialist phases below were executed against the committed tree. A
subagent dispatch for the regression phase returned no output, so each phase was
performed directly and its commands and results are recorded here verbatim
rather than summarised from a delegated report.

### PHASE-REGRESSION

The riskiest change in this packet is the `webAuthMiddleware` fix, because it
now attaches an `auth.Session` on three admit paths that previously called
`next.ServeHTTP` bare. Any handler branching on the presence of a session could
change behaviour. Twenty-six non-test call sites read that session:

```console
$ grep -rn 'SessionFromContext' internal/ --include='*.go' | grep -v '_test.go' | wc -l
26
```

The two call sites that can refuse a request were read directly. Both treat an
absent session as a wiring defect rather than as a denial:

```console
$ sed -n '89,95p' internal/api/corpus_grant_gate.go
        sess, ok := auth.SessionFromContext(r.Context())
        if !ok {
                // Wiring defect — bearerAuthMiddleware must run before this gate
```

```console
$ sed -n '55,62p' internal/auth/scope_middleware.go
                        sess, ok := SessionFromContext(r.Context())
                        if !ok {
                                // Wiring bug — bearerAuthMiddleware should always
                                writeScopeError(w, http.StatusInternalServerError, "middleware_misconfigured", nil)
```

So the change converts a latent HTTP 500 into the documented shared-token path;
it does not widen access. The scope gate is reached only under the outer
`bearerAuthMiddleware`, which already attached a session before this packet, so
no route that `RequireScope` guards is affected by the web middleware at all:

```console
$ sed -n '210,222p' internal/api/router.go | grep -n 'bearerAuthMiddleware'
20:                             // scope claim (spec 060). The outer bearerAuthMiddleware
```

The CSS rule added for `.action` cannot break a selector, because it introduces
no class name and no DOM change; four call sites across two files gain a minimum
size they previously lacked, and the browser lane stayed green at 89 passed. The
synthesis handlers remain nil-guarded at their registration site, so moving the
wiring out of the QF gate registers routes without a nil dereference:

```console
$ grep -n 'SynthesisHandlers != nil' internal/api/router.go
394:                    if deps.SynthesisHandlers != nil {
```

Three references to the superseded `synthesis_insights` table remain, and all
three are a comment or migration DDL rather than a live read, so no surface
still depends on the table this packet replaced.

### PHASE-SECURITY

Every statement in the durable read and write paths is parameterised. The only
`Sprintf` in the new intelligence code builds a validation message, not SQL:

```console
$ grep -nE 'Sprintf' internal/intelligence/synthesis_*.go | grep -iE 'select|insert|from|where'
internal/intelligence/synthesis_validator.go:113: Detail: fmt.Sprintf("required source class %q is absent
```

No log line in the new code emits a token, a secret, or synthesised prose. The
grep for that class of leak across the intelligence, API and web files returns
nothing. Refusal of unauthenticated callers is proven by execution rather than
by inspection: `TestSynthesisAPI_DeniesUnauthenticatedCallers` and
`TestSynthesisAPI_RetryDeniesUnauthenticatedCallers` both pass in the e2e lane,
and the browser suite additionally refuses to treat `auth_required` as an
answered state, so an unauthenticated render can no longer satisfy a content
assertion vacuously.

### PHASE-SIMPLIFY

The Today and Status surfaces share one projection rather than each deriving
state. Both handlers call the same helper, and exactly one classifier exists:

```console
$ grep -n 'synthesisModel\|func ClassifySynthesisView' internal/web/*.go | grep -v '_test.go'
internal/web/handler.go:421:    model.Synthesis = h.synthesisModel(r.Context(), requestAuthorized(r))
internal/web/handler.go:618:            "Synthesis": h.synthesisModel(r.Context(), requestAuthorized(r)),
internal/web/synthesis_projection.go:118:func ClassifySynthesisView(
```

That single seam is what makes the two surfaces structurally unable to disagree,
which was the defect this packet set out to remove. No duplicated classification
branch remains to drift.

### PHASE-STABILIZE

Every durable read is bounded. Each of the three read paths terminates in
`ORDER BY … LIMIT 1` rather than scanning a growing table:

```console
$ grep -nE 'LIMIT 1' internal/intelligence/synthesis_readmodel.go
64:             LIMIT 1`).Scan(&outputKind, &createdAt)
80:             LIMIT 1`).Scan(&outcome)
157:            LIMIT 1`).Scan(
```

Six indexes across migrations 064 and 066 cover the run, attempt, insight,
citation and lifecycle lookups. The stress lane completed with exit code 0 and
no failing line, so the bounded reads hold under the load the lane applies.

### PHASE-VALIDATE

The repository-standard command surface was run end to end on this tree. `check`
reported zero errors, `lint` zero findings and `format` zero unformatted files.
The five test lanes were each executed separately, because two stack lanes
cannot share a host: unit and integration reported no failing line, the e2e lane
exited 0 with 36 passed and 0 failed, the browser lane reported 89 passed
including the 13 synthesis specs, and the stress lane exited 0. The one `FAIL`
string inside the e2e output belongs to an intentional negative path that stops
postgres to force a readiness failure and then reports `PASS`.

### PHASE-AUDIT

All five scopes carry `Status: Done`, and the Definition of Done rows are fully
closed with no row left open:

```console
$ grep -c '^- \[ \]' scopes.md ; grep -c '^- \[x\]' scopes.md
0
88
```

The audit's substantive finding is recorded in the mutation evidence rather than
here: a suite that passed while the durable reader was unwired was not
acceptable, and the packet was not advanced until the decisive mutation made
four of five browser specs fail for the right reason. Human acceptance in
`uservalidation.md` remains unchecked, because the author records it and an
agent recording it on the author's behalf would assert a review that did not
happen.

