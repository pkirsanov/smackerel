# Spec 113 — Execution Receipts and In-Session Review Adoption

**Status:** SPEC ONLY — awaiting `bubbles.design` → `bubbles.plan`. No design, scopes, or execution packet exists yet.
**Depends on:** `bubbles` `improvements/IMP-048-in-session-review-and-execution-receipts.md` SCOPE-1, SCOPE-2, SCOPE-3, SCOPE-4, SCOPE-6, SCOPE-7.
**Owner on adoption:** `bubbles.plan`

## Problem

Smackerel's highest-value invariants are honesty invariants: an execution failure must never be rendered as a successful capture, a band-high turn must never emit the capture acknowledgement, and an ungrounded answer must refuse rather than claim. These are exactly the invariants where a passing test proves the least, because the failure mode is a response that LOOKS fine.

Three concrete weaknesses follow.

1. **The honesty invariants have no executed negative control.** `.github/bubbles-project.yaml` declares no `mutationExecution:` block. The repository documents the defect precisely — a provenance gate that ran on non-OK outcomes rewrote provider-error and timeout turns into `StatusSavedAsIdea` — and it added tests to prevent recurrence. Nothing currently proves those tests would fail if the gate were re-broadened.

2. **A known execution hazard is carried as tribal memory.** The canonical test runner must be executed alone; a concurrent command in a reused terminal can deliver `SIGINT` to the suite, producing status 130 and a partial run that is an INVALIDATED execution rather than a product failure. This is written down as an operator note, not enforced, so each session must rediscover it.

3. **Orphan-endpoint prevention is honestly unenforced.** The repository documents 517 route registrations and states plainly that no mechanical checker exists, explaining why the sibling repository's grep approach does not port to chi's nested `Route`/`Mount` composition (189 extracted paths, 64 of them bare fragments, one block mounted twice). That honesty is correct and must be preserved; what is missing is a periodic signal that the gap is still open rather than quietly forgotten.

## Outcome Contract

**Intent:** The invariants that keep Smackerel truthful are demonstrably able to fail, and the suites that prove them are never re-run wholesale or silently invalidated by terminal contention.

**Success Signal:** A mutation receipt exists for each honesty invariant; an interrupted canonical run is classified INVALIDATED rather than failed; and an unchanged Go or Python leaf is reported accepted rather than re-executed.

**Hard Constraints:**
- The NO-DEFAULTS SST policy is unchanged. No receipt, budget, or review field may be introduced with a fallback value; a missing required value fails loud.
- Assistant refusal shaping stays on one path. No parallel refusal or capture-acknowledgement path may be introduced for test convenience.
- The isolated-ML-sidecar boundary is unchanged: the Python tier gains no data-store credentials for fixture or receipt purposes.
- The honest statement that orphan-endpoint detection is unenforced MUST NOT be replaced by an over-reporting checker.

**Failure Condition:** A mutation receipt is produced for the honesty invariants, but by mutating a test rather than the production path — proving the suite is self-consistent rather than sensitive to real behavior.

## Actors

| Actor | Interest |
|---|---|
| End user of the assistant | Must never be told an idea was captured when the turn actually failed |
| Delivery agent | Needs to rerun only what changed, and to distinguish an interrupted run from a real failure |
| Reviewer | Needs proof the honesty tests would catch a regression |
| Operator | Needs the unenforced orphan gap to stay visible rather than fade |

## Requirements

### R1 — Mutation proof for the honesty invariants

- R1.1 `.github/bubbles-project.yaml` MUST declare a `mutationExecution:` block with a repository-owned runner and a measured timeout.
- R1.2 The following production mutants MUST each be killed by an existing test:

| Mutant | Perturbation | Must fail |
|---|---|---|
| `H1-gate-on-non-ok` | Run the provenance / capture-fallback gate on a non-OK outcome | The execution-error honesty test |
| `H2-high-band-capture` | Emit the capture acknowledgement for a band-high turn | The band-refusal invariant test |
| `H3-uncited-answer` | Return an ungrounded open-knowledge answer instead of refusing | The grounded-answer refusal test |
| `H4-lost-error-cause` | Drop `ErrorCause` when translating a failed turn | The outcome-translation test |

- R1.3 Each mutant MUST perturb PRODUCTION code, never a test fixture.
- R1.4 Mutation runs MUST occur in an isolated worktree or copied fixture.

### R2 — Interrupted runs are invalidated, not failed

- R2.1 The canonical suite MUST take an exclusive runtime lease for its duration.
- R2.2 A run terminated by an external signal, or reporting fewer executed cases than expected, MUST be classified INVALIDATED.
- R2.3 An INVALIDATED run MUST NOT be recorded as a product failure, and MUST NOT satisfy any evidence obligation.
- R2.4 Re-execution after invalidation MUST use the exact canonical command in isolation.

### R3 — Leaf-level validation receipts

- R3.1 Each Go package test, Python sidecar test, and live-category suite MUST record a receipt binding leaf identity, candidate digest, environment fingerprint, exit code, and output hash.
- R3.2 An unchanged leaf MUST be reported accepted and MUST NOT be re-executed.
- R3.3 A changed production owner MUST invalidate exactly its covering leaves.
- R3.4 The environment fingerprint MUST identify the disposable test stack, so an accepted live result can never have come from a persistent dev stack.

### R4 — Keep the unenforced gap visible

- R4.1 The documented absence of a mechanical orphan-endpoint checker MUST remain stated, including why the sibling approach does not port.
- R4.2 The in-session review MUST surface, as an operator-facing item, that route-to-consumer coverage is review-enforced only.
- R4.3 Any future checker MUST reconstruct full paths through chi's nesting before it may block; a fragment-based checker MUST NOT be shipped.

### R5 — Session discipline

- R5.1 A `sessionBudget` MUST be recorded for delivery work.
- R5.2 Crossing the soft boundary MUST emit a handoff recommendation without marking any spec blocked.
- R5.3 Turn snapshots MUST be appended for any run exceeding three turns.
- R5.4 The known terminal-isolation hazard MUST be expressible as an in-session adjustment that dispatched specialists inherit.

## Scenarios

```gherkin
Scenario: SCN-113-01 broadening the provenance gate breaks the honesty test
  Given the assistant renders a provider-error turn honestly
  When the provenance gate is mutated to run on a non-OK outcome
  Then the execution-error honesty test fails
  And restoring the gate returns the suite to green

Scenario: SCN-113-02 a high-band turn cannot be rendered as a capture
  Given a band-high turn produced an execution failure
  When the capture acknowledgement is mutated to apply to band-high turns
  Then the band-refusal invariant test fails

Scenario: SCN-113-03 an interrupted canonical run is invalidated
  Given the canonical suite is running under an exclusive lease
  When an external signal terminates it and fewer cases execute than expected
  Then the run is classified invalidated
  And it is not recorded as a product failure
  And it satisfies no evidence obligation

Scenario: SCN-113-04 an unchanged leaf is accepted
  Given a Go package test passed against a recorded candidate digest and environment fingerprint
  When validation runs again with those inputs unchanged
  Then the leaf is reported accepted and does not execute

Scenario: SCN-113-05 a live result names the disposable stack
  Given a live-category leaf records an environment fingerprint
  When the fingerprint is evaluated
  Then it identifies the disposable test stack
  And a leaf executed against the persistent dev stack is not accepted

Scenario: SCN-113-06 the unenforced orphan gap stays visible
  Given route-to-consumer coverage has no mechanical checker
  When the in-session review reports repository health
  Then it names the gap as review-enforced only
  And it does not claim mechanical coverage

Scenario: SCN-113-07 a mutated test does not satisfy a mutation obligation
  Given a mutation receipt is produced
  When its perturbation target is inspected
  Then a perturbation applied to a test rather than production code is rejected
```

## Non-Functional Requirements

- **Fail-loud configuration:** every new field is required or absent; no fallback values anywhere in the receipt or review surface.
- **Single refusal path:** refusal and capture shaping remain structurally distinguishable via status and citations, never via message text.
- **Sidecar boundary:** Python remains compute-only with no data-store credentials.
- **Honest gaps:** documented absences stay documented rather than being replaced by noisy checks.

## Outcomes

| # | Outcome | Proof |
|---|---|---|
| O1 | Honesty invariants are provably sensitive | Four killed-mutant receipts against production code |
| O2 | Terminal contention no longer produces false failures | Interrupted runs classified invalidated under an exclusive lease |
| O3 | Validation cost tracks change size | Accepted-leaf counts; no wholesale re-runs on unchanged candidates |
| O4 | Live results cannot come from the dev stack | Environment fingerprints naming the disposable stack |
| O5 | The orphan gap stays visible | Periodic operator-facing statement that coverage is review-enforced |
| O6 | Sessions end deliberately | Budget recorded; handoff emitted at the soft boundary |

## Out of Scope

- Framework implementation of receipts, review loop, or budgets — that is `IMP-048` in the bubbles repository.
- Building the chi-aware orphan-endpoint checker (this spec only preserves the honest gap and its visibility).
- New assistant, connector, or graph features.

## Next Owner

`bubbles.design`, then `bubbles.plan`.
