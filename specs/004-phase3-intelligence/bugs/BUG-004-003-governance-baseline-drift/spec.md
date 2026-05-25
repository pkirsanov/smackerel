# Bug: BUG-004-003 — Governance Baseline Drift (Spec Evidence Cites Pre-Refactor Monolithic File)

> **Parent Spec:** [specs/004-phase3-intelligence](../../spec.md)
> **Severity:** Medium
> **Found By:** bubbles.workflow (sweep-2026-05-25-r10 round 2, trigger=gaps, mappedMode=gaps-to-doc, executionModel=parent-expanded-child-mode)
> **Date:** 2026-05-25

## Problem

Stochastic sweep round 2 ran the `gaps` trigger of the parent-expanded
`gaps-to-doc` child workflow mode against `specs/004-phase3-intelligence`.
The strict `state-transition-guard.sh` baseline is already clean
(0 BLOCKs, 1 placeholder-paths warning), but a deeper structural probe
of spec-to-code traceability surfaced three artifact-integrity finding
classes that the current guard does not detect.

The Phase 3 Intelligence runtime was originally implemented as a
single monolithic `internal/intelligence/engine.go` file and the
spec/design artifacts were authored against that file. After the
initial certification on 2026-04-17, the package was refactored into
domain-specific files (`synthesis.go`, `briefs.go`, `alerts.go`,
`resurface.go`, later `alert_producers.go`), but the spec/design
artifacts were not updated to reflect the new layout. The runtime
continues to work — all 21 scenarios (SCN-004-001 through SCN-004-018b)
are exercised by tests in `internal/intelligence/*_test.go`,
`internal/digest/generator_test.go`, and the six
`tests/e2e/test_*.sh` E2E scripts that all exist on disk — but the
artifact evidence pointers now lie about file locations.

This is silent governance drift: the strict guard checks that evidence
blocks exist and that file paths are well-formed, but it cannot verify
that the cited function actually lives in the cited file. A reviewer
following a Scope 1 evidence pointer to
`internal/intelligence/engine.go RunSynthesis` would find that
`engine.go` is a 96-line types-only file containing only `NewEngine`
plus type definitions (`InsightType`, `SynthesisInsight`, `AlertType`,
`AlertStatus`, `Alert`, `Engine`). The actual `RunSynthesis`
implementation lives in `internal/intelligence/synthesis.go`.

The three finding classes (atomic counts in parentheses; 39 atomic
citations total):

1. **F1 — Function-reference drift in DoD evidence blocks (≈22 atomic citations).**
   DoD `> Evidence:` blocks in `scopes.md` Scopes 1, 2, 3, 4, 5, and 6
   attribute function implementations (`RunSynthesis`,
   `GenerateWeeklySynthesis`, `CheckOverdueCommitments`,
   `GeneratePreMeetingBriefs`, `CreateAlert`, `DismissAlert`,
   `SnoozeAlert`, `GetPendingAlerts`, `MarkAlertDelivered`,
   `assembleWeeklySynthesisText`, `assembleBriefText`) to
   `internal/intelligence/engine.go`, but those functions live in
   `synthesis.go`, `briefs.go`, and `alerts.go`. Constant/type
   citations (`InsightType`, `InsightContradiction`,
   `SynthesisInsight`, `AlertType`, `Alert`, `AlertBill`,
   `AlertReturnWindow`, `AlertTripPrep`, `AlertRelationship`,
   `AlertCommitmentOverdue`, `AlertMeetingBrief`) remain correct
   because those genuinely live in `engine.go` — only the function
   citations drift.

2. **F2 — `Implementation Files` lists understate the package surface
   (4 scope lists).** `scopes.md` Scope 1 (line 85), Scope 2
   (line 179), Scope 3 (line 274), and Scope 4 (line 371) each list
   only `internal/intelligence/engine.go` and
   `internal/intelligence/engine_test.go`. Scope 1 must additionally
   reference `synthesis.go` + `synthesis_test.go` (RunSynthesis,
   GenerateWeeklySynthesis). Scope 2 must additionally reference
   `briefs.go` + `briefs_test.go` (CheckOverdueCommitments) and
   `internal/digest/generator.go` (the `ActionItem` Go type lives in
   `digest/generator.go`, not `intelligence/engine.go`). Scope 3 must
   additionally reference `briefs.go` + `briefs_test.go`
   (GeneratePreMeetingBriefs, MeetingBrief, AttendeeBrief,
   assembleBriefText). Scope 4 must additionally reference `alerts.go`
   + `alerts_test.go` (CreateAlert, DismissAlert, SnoozeAlert,
   GetPendingAlerts, MarkAlertDelivered, HasStalePendingAlerts) and
   `alert_producers.go` + `alert_producers_test.go` (the four
   `Produce*Alerts` functions that BUG-021-003 just wired
   `AlertProducerFailures` metric into during sweep-2026-05-25-r10
   round 1). Scope 5 (line ~550) and Scope 6 (line 562) already list
   the correct files (`resurface.go`, `digest/generator.go`,
   `scheduler/scheduler.go`).

3. **F3 — `design.md` architecture diagram describes a non-existent
   sub-package layout (1 fabricated diagram, ~30 lines).**
   `design.md` lines ~120-145 under
   `### Intelligence Engine Components` show a 4-subdirectory layout:
   ```
   internal/intelligence/
       synthesis/{engine.go,clusterer.go,analyzer.go,contradiction.go}
       digest/{weekly.go,serendipity.go,patterns.go}
       alerts/{manager.go,premeeting.go,commitments.go,bills.go,types.go}
       commitments/{detector.go,tracker.go,resolver.go}
   ```
   None of these subdirectories or files exist. The actual layout is
   flat under `internal/intelligence/` with 17 `.go` source files:
   `engine.go`, `synthesis.go`, `briefs.go`, `alerts.go`,
   `alert_producers.go`, `resurface.go`, `annotations.go`,
   `expenses.go`, `expertise.go`, `learning.go`, `lists.go`,
   `lookups.go`, `monthly.go`, `people.go`, `people_forecast.go`,
   `subscriptions.go`, `vendor_seeds.go` (the last 11 are Phase 4/5
   surface, not spec 004). The fabricated diagram misleads
   architectural reviewers about the package structure and predates
   the as-built reality.

## Why This Drift Is Worth Closing

- **Auditability.** Future system reviews and stochastic gaps probes
  will keep re-surfacing this drift if not resolved. The R3/R5
  precedents in the previous sweep (`BUG-014-003`, `BUG-020-006`,
  `BUG-006-005`) established that artifact-integrity drift gets closed
  via single `validate-to-doc` packets; this round honors that
  precedent.
- **Onboarding cost.** A new engineer or reviewer following the spec
  evidence pointer to `engine.go RunSynthesis` would waste effort
  hunting for the function before discovering the post-refactor split.
- **Spec→code traceability.** The Bubbles framework treats DoD
  evidence pointers as the canonical mapping between spec acceptance
  and implementing code. When the file path is wrong, the mapping is
  broken for that bullet even if the function name still grep-resolves
  somewhere in the tree.

## Why This Drift Is Not A Functional Defect

- All 21 Phase 3 scenarios pass against the live stack
  (`tests/e2e/test_synthesis.sh`, `test_commitments.sh`,
  `test_premeeting.sh`, `test_alerts.sh`, `test_weekly_synthesis.sh`,
  `test_enhanced_digest.sh` all present and executable).
- All Go unit/integration tests for the intelligence + digest +
  scheduler packages are green (last confirmed during sweep-r10 R1
  full validation chain).
- No runtime behavior change is required to close this bug; the
  closure is exclusively artifact-text restoration.

## Severity Rationale

Medium, not High: there is no production behavior regression, no
security exposure, no test-suite failure. But the drift breaks the
spec-to-code traceability contract that Bubbles depends on and would
re-surface every time the gaps trigger probes spec 004, so it earns a
real bug packet rather than a deferral.

## Closure Mode

`validate-to-doc` artifact-only restoration. Zero runtime, config,
test, CI, deploy, framework, or docs files modified. Edit footprint
strictly contained inside:

- `specs/004-phase3-intelligence/scopes.md` — correct evidence-block
  function citations and extend `Implementation Files` lists for
  Scopes 1, 2, 3, 4.
- `specs/004-phase3-intelligence/design.md` — replace the fabricated
  4-subdirectory diagram with the actual flat layout.
- `specs/004-phase3-intelligence/state.json` — append `BUG-004-003` to
  `resolvedBugs[]` and update `lastUpdatedAt`.
- `specs/004-phase3-intelligence/bugs/BUG-004-003-governance-baseline-drift/**`
  — create the 7-artifact bug packet itself.
- `.specify/memory/sweep-2026-05-25-r10.json` — append round-2 ledger
  entry.

## Acceptance Criteria

- AC1: All function citations in `scopes.md` DoD evidence blocks
  resolve to the file they actually live in (grep-verifiable).
- AC2: `Implementation Files` lists for Scopes 1, 2, 3, 4 include the
  post-refactor split files alongside `engine.go`.
- AC3: `design.md` `### Intelligence Engine Components` reflects the
  actual flat `internal/intelligence/{engine,synthesis,briefs,alerts,alert_producers,resurface}.go`
  layout. Phase 4/5 surface files are not enumerated here; they remain
  out-of-scope for spec 004.
- AC4: `state-transition-guard.sh specs/004-phase3-intelligence`
  exits 0 with TRANSITION PERMITTED after edits; warning count does
  not increase from baseline.
- AC5: `state-transition-guard.sh` exits 0 with TRANSITION PERMITTED
  on the bug packet directory itself at the `validate-to-doc` ceiling.
- AC6: `artifact-lint.sh` and `traceability-guard.sh` both pass on
  parent spec 004 after edits.
- AC7: Staged diff contains only:
  - `specs/004-phase3-intelligence/scopes.md`
  - `specs/004-phase3-intelligence/design.md`
  - `specs/004-phase3-intelligence/state.json`
  - `specs/004-phase3-intelligence/bugs/BUG-004-003-governance-baseline-drift/**`
  - `.specify/memory/sweep-2026-05-25-r10.json`

No `internal/`, `cmd/`, `ml/`, `config/`, `deploy/`, `.github/`,
`tests/`, or `docs/` paths appear in the staged diff.
