# Report: [BUG-004-004] Synthesis Persistence And Health Are Not Truthful

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning artifacts were initialized 2026-07-23/24. On 2026-07-25 a **disjoint,
unit-verifiable HEALTH-TRUTH slice** of SCOPE-04 was implemented by
`bubbles.implement`: the pure, database-free `DeriveSynthesisHealth` mapping that
turns a synthesis persistence **outcome** into a truthful health verdict, plus
real Go unit tests. All live-DB / integration / e2e work (the durable
transactional persistence rows and the `/api/health` wiring) is **deferred** and
the packet remains `blocked` on it. No git, deployment, or production mutation
occurred.

## Completion Statement

Non-terminal. Packet status is `blocked`. Only the pure health-truth
determination (`internal/intelligence/synthesis_health.go` + its unit tests) is
implemented and unit-verified. No SCOPE-04 Definition-of-Done item is checked:
every SCOPE-04 DoD row is a live-system (integration/e2e) row that requires the
disposable PostgreSQL stack, which was intentionally not brought up in this
invocation. No certification is claimed.

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

## Deferred (Live-DB) Rows — NOT attempted this invocation

These SCOPE-04 (and prerequisite SCOPE-01..03) rows require the disposable
PostgreSQL Docker stack, which a concurrent agent was using for its own e2e run;
they are deferred and are the blocking reason:

- `T004-05-HEALTH` — `tests/integration/synthesis/health_test.go` (live never-run/probe-failure health).
- `T004-05-API` — `tests/e2e/synthesis_api_e2e_test.go` (live latest-API never-run).
- `T004-06-ALERT` — `tests/integration/synthesis/alert_test.go` (live stale/failed alert clearing on verified recovery).
- `T004-07-08-API`, `T004-09-AUTH`, `T004-09-TELEMETRY` — live API/auth/telemetry rows.
- The `/api/health` wiring that derives a real `SynthesisPersistenceOutcome` from the durable run ledger and calls `DeriveSynthesisHealth` (depends on the SCOPE-01 durable persistence foundation).
- All SCOPE-01 (durable transactional persistence), SCOPE-02 (producers), SCOPE-03 (scheduler/retry), and SCOPE-05 (UI) rows.

## Uncertainty Declarations

- The pure mapping is fully verified; its live wiring into `/api/health` and the
  DB-derivation of the outcome are unverified here (deferred).
- No live SQL counts, red/green integration output, or `/api/health` responses
  were produced in this invocation.

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
