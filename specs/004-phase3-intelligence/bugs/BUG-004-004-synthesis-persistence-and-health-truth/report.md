# Report: [BUG-004-004] Synthesis Persistence And Health Are Not Truthful

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning artifacts were initialized 2026-07-23/24. On 2026-07-25 a **disjoint,
unit-verifiable HEALTH-TRUTH slice** of SCOPE-04 was implemented by
`bubbles.implement`: the pure, database-free `DeriveSynthesisHealth` mapping that
turns a synthesis persistence **outcome** into a truthful health verdict, plus
real Go unit tests. All live-DB / integration / e2e work (the durable
transactional persistence rows) was **deferred** on 2026-07-25. On 2026-07-26
`bubbles.implement` WIRED that mapping into the live `/api/health` path
(`internal/api/health.go::getCachedIntelligenceHealth`), replacing the
never-run/probe-error → "up" logic with a `SynthesisPersistenceOutcome` routed
through `DeriveSynthesisHealth`, and PROVED it live (RED→GREEN) with a new
real-stack integration test (`tests/integration/synthesis/health_test.go`). The
packet remains `blocked` on the deferred durable transactional-persistence
scopes (SCOPE-01..03) and the remaining SCOPE-04 live API/alert/telemetry rows.
No git, deployment, or production mutation occurred.

## Completion Statement

Non-terminal. Packet status is `blocked`. The disjoint HEALTH-TRUTH slice is now
COMPLETE and live-proven: the pure `DeriveSynthesisHealth` mapping (2026-07-25)
PLUS its live wiring into `/api/health` (2026-07-26), verified by the
`T004-05-HEALTH` real-stack integration test (RED→GREEN; the disposable stack
was torn down on exit). Exactly one SCOPE-04 Test-Evidence DoD item
(`T004-05-HEALTH`) is checked with current-session raw evidence. All durable
transactional-persistence rows (SCOPE-01..03) and the remaining SCOPE-04 live
API/alert/telemetry rows remain deferred; no certification is claimed.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted (from `design.md` grounded root-cause analysis).
- **Executed by this invocation:** no live reproduction (live DB deferred).
- **Root cause preserved:** `internal/api/health.go::getCachedIntelligenceHealth`
  maps the never-run epoch sentinel to `up` and a freshness-probe error to `up`
  (lines ~671-679), so a system that never produced — or failed to persist — any
  synthesis still reports green. `internal/intelligence/synthesis.go::RunSynthesis`
  returns in-memory `SynthesisInsight` structs and persists nothing.
- **Evidence status:** the live SQL/health reproduction is a deferred integration
  row; this invocation delivered only the pure-logic fix + unit proof.

## Decision Record

- Durable, read-back-verified rows — not object construction or a log count —
  define synthesis success. Commit alone is not success; only a passing
  post-commit read-back yields `persisted`.
- Health truth is a **pure function** of the persistence outcome, so it is
  unit-testable without a database via an injected `SynthesisPersistenceOutcome`
  seam.
- Never-run, probe-error, write-failure, partial output, and any non-OK
  read-back can NEVER be reported healthy / persisted / `up`.

### Placement decision (why `internal/intelligence`, not `internal/synthesis`)

`internal/synthesis` does not exist. Creating it would add a new top-level
`internal/` package, which `internal/docfreshness/doc_freshness_test.go`
(`TestDocFreshness_AllInternalPackagesDocumented`) requires to be listed in
`docs/Development.md` — a file that is concurrently locked and out of scope for
this packet. Creating the package would therefore FAIL `./smackerel.sh test unit
--go` and force an edit to a forbidden file. The existing home of synthesis logic
(`RunSynthesis`, `GetLastSynthesisTime`, `SynthesisInsight`) is
`internal/intelligence`, so the pure health-truth mapping was added there as two
brand-new files (zero collision risk with concurrent edits to other files).

## Code Diff Evidence

- **Claim Source:** executed (files created this invocation).
- **Added** `internal/intelligence/synthesis_health.go` — the pure, no-I/O
  `DeriveSynthesisHealth(SynthesisPersistenceOutcome) SynthesisHealth` mapping and
  its closed vocabulary (`SynthesisHealthState`, `PersistencePhase`,
  `ReadBackResult`, `SynthesisOutputKind`). Guarantees: `Healthy` implies
  `Persisted`; `IntelligenceStatus == "up"` iff `Healthy`; write-failure /
  probe-error / never-run / partial / non-OK-read-back are never persisted,
  healthy, or `up`.
- **Added** `internal/intelligence/synthesis_health_test.go` — real, mock-free,
  DB-free unit tests: one per operator-required truth, an adversarial regression
  vs the pre-fix falsely-healthy mapping, a 14-case closed-vocabulary lock, and a
  320-combination invariant sweep.
- No other files were modified. `internal/api/health.go` wiring was NOT changed
  (that is the deferred live slice).

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

## Deferred (Live-DB) Rows — still blocking

The `/api/health` truthful-status WIRING (`T004-05-HEALTH`) is now DONE and
live-proven above. The remaining rows require the durable transactional
persistence foundation (SCOPE-01..03) and are the blocking reason:

- `T004-05-API` — `tests/e2e/synthesis_api_e2e_test.go` (live latest-API never-run).
- `T004-06-ALERT` — `tests/integration/synthesis/alert_test.go` (live stale/failed alert clearing on verified recovery).
- `T004-07-08-API`, `T004-09-AUTH`, `T004-09-TELEMETRY` — live API/auth/telemetry rows.
- The FULL DB-derivation of the outcome: deriving a `SynthesisPersistenceOutcome` from the durable run ledger (atomic insight+citation commit + per-insight/per-citation post-commit read-back) rather than the coarse `GetLastSynthesisTime`-based read-back wired now. This is the SCOPE-01 durable persistence foundation.
- All SCOPE-01 (durable transactional persistence), SCOPE-02 (producers), SCOPE-03 (scheduler/retry), and SCOPE-05 (UI) rows.

## Uncertainty Declarations

- The pure mapping AND its live wiring into `/api/health` are now fully verified
  (RED→GREEN real-stack integration, `T004-05-HEALTH`). What remains unverified
  (deferred) is the FULL DB-derivation of the outcome from the durable run
  ledger — atomic insight+citation commit and the per-insight/per-citation
  post-commit read-back gate (SCOPE-01..03). The wired derivation uses the
  coarse read-back available today (a successful `GetLastSynthesisTime` query).
- The suite-level `./smackerel.sh test unit --go` non-zero exit is an
  ORTHOGONAL, pre-existing/concurrent `internal/docfreshness` failure
  (`internal/acceptance` tracked-at-HEAD + `internal/experience` concurrent,
  both undocumented in the forbidden `docs/Development.md`); this change added
  no new `internal/` package and did not cause it.

## Scenario Contract Evidence

`DeriveSynthesisHealth` implements the unit-level determination underlying
SCN-004-004-05 (never-run is not up) and SCN-004-004-06 (stale/failed/unverified
never healthy). The scenario-manifest `linkedTests` remain empty pending the
live integration/e2e rows that close those scenarios end-to-end.

## Validation Summary

No completion validation or certification performed. Packet is `blocked` on the
deferred live-DB persistence rows.

## Audit Verdict

Not audited. No terminal verdict claimed.
