# Bug Specification: BUG-031-006 Strict-Guard Gate Drift on Spec 031

## Problem Statement

Spec 031 (Live Stack Testing) was promoted to `status: done` before the current set of strict mechanical gates was fully enforced. The implementation on disk is real (17 integration test files, 24 E2E test files, `internal/api/ml_readiness.go` for the readiness endpoint), but `state-transition-guard.sh` now rejects the prior promotion with 38 BLOCK findings + 2 warnings across 8 gate categories:

- **G060** (Check 3E): scenario-first TDD has no red→green markers in the scope/report evidence.
- **G022** (Check 6): four required specialist phases (`regression`, `simplify`, `stabilize`, `security`) never appeared in `completedPhaseClaims` or `executionHistory`.
- **G022 phase impersonation** (Check 6B): five phase claims (`chaos`, `docs`, `test`, `audit`, `validate`) appear in `completedPhaseClaims` without matching `bubbles.<phase>` `executionHistory` entries; narrative `Phase Agent: bubbles.X` lines in `report.md` are not the same as structured per-phase provenance.
- **G016** (Check 8A): 18 regression E2E planning items are missing — all 6 scopes lack the scenario-specific regression DoD item, the broader regression suite DoD item, and the explicit regression Test Plan row.
- **Check 8D** (Change Boundary containment): scopes.md lacks a `Change Boundary` section, change-boundary DoD item, and allowed/excluded surface enumeration. The trigger is the keyword `cleanup` appearing in test-helper context (`cleanupArtifact`, `cleanupList`, `cleanupAnnotation`) — likely a false-positive against helper naming, but the gate still blocks until either the keyword usage is rescoped or the Change Boundary surface is added.
- **G053** (Check 13B): `report.md` lacks a `### Code Diff Evidence` section.
- **Check 5A** (SLA stress coverage): Scope 6 explicitly references a 60-second configurable ML readiness timeout = SLA-sensitive, but no corresponding stress test exists.
- **Check 17** (strict-mode commit enforcement): `full-delivery` mode requires at least one structured commit message with the `spec(031)` or `bubbles(031/...)` prefix; none is present in git history.

`artifact-lint.sh` continues to PASS — it accepts `completedPhaseClaims` at face value. Only the strict mechanical guard catches the under-evidenced promotion.

## Outcome Contract

**Intent:** Spec 031 returns to `status: done` only after every one of the 38 BLOCK findings is closed with real, gate-verifiable evidence — no laundering, no checkbox stripping, no scope renames.

**Success Signal:**
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing` exits 0 with zero BLOCK findings.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing` continues to exit 0.
- `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/031-live-stack-testing --verbose` exits 0.
- A structured `spec(031)` or `bubbles(031/...)` commit lands the closure.
- All 6 scopes show real `bubbles.<phase>` provenance for each of `implement`, `test`, `regression`, `simplify`, `stabilize`, `security`, `validate`, `audit`, `chaos`, `docs`, and `finalize` in `executionHistory`.

**Hard Constraints:**
- **G041 anti-manipulation.** No DoD checkbox MAY be deleted, converted to a non-checkbox bullet, struck through, or italicized to dodge the count. No scope status MAY be renamed to a non-canonical value (`Not Started`, `In Progress`, `Done`, `Blocked` are the only legal strings). No `completedPhaseClaims` entry MAY be stripped.
- **Implementation is verified real.** No source file MAY be deleted or rewritten on the rationale that "the work was never done"; the regression suite, integration suite, and `ml_readiness.go` already exist. Edits flow forward.
- **NO-DEFAULTS SST.** Any new stress test or readiness probe MUST consume SST-derived environment variables, never hardcoded ports/URLs.
- **Test-environment isolation.** The new Scope 6 SLA stress test MUST run against the disposable test stack, never the persistent dev stack.
- **No `--no-verify` push.** Every commit MUST clear the standard pre-commit hooks; if `gitleaks` flags PII in evidence blocks, redact with `multi_replace_string_in_file` and re-stage.

**Failure Condition:** The bug remains open if `state-transition-guard.sh` continues to BLOCK promotion after closure, if any G041 manipulation pattern is detected, if the SLA stress test references hardcoded config, or if any required specialist phase still lacks a real `bubbles.<phase>` `executionHistory` entry.

## Goals

1. Close the 18 missing G016 regression E2E planning items across all 6 scopes (scope/DoD + Test Plan rows).
2. Close the 3 missing Check 8D Change Boundary items in `scopes.md` (section + DoD + allowed/excluded surface enumeration) — either by adding the section or by rescoping the `cleanup` helper naming to remove the false-positive trigger.
3. Close the 1 missing G053 `### Code Diff Evidence` section in `report.md`.
4. Add 1 SLA stress test for Scope 6's 60-second ML readiness timeout against the disposable test stack.
5. Add 1 scenario-first TDD red→green evidence pass to satisfy G060.
6. Run each of the 4 missing required specialist phases (`regression`, `simplify`, `stabilize`, `security`) and the 5 impersonated phases (`chaos`, `docs`, `test`, `audit`, `validate`) so each emits a real `bubbles.<phase>` `executionHistory` entry with `completedPhaseClaimDetails` provenance.
7. Land closure under a structured commit message: `spec(031): close strict-guard gate drift (BUG-031-006)` or `bubbles(031/BUG-031-006): close strict-guard gate drift`.

## Non-Goals

- Re-implementing the live-stack test harness itself (already real on disk).
- Rewriting the test-environment isolation contract from spec 037 / OPS-XXX.
- Touching the spec 055 (notification ntfy adapter) working-tree edits currently in-flight (`cmd/core/services.go`, `internal/notification/source/`, `tests/{e2e,stress}/notification_ntfy_source_*`, etc.).
- Editing generated config under `config/generated/` by hand.
- Adding new feature surface beyond what the original 6 scopes describe.

## Requirements

- **R-BUG-031-006-001:** `state-transition-guard.sh` MUST exit 0 against `specs/031-live-stack-testing/` after closure.
- **R-BUG-031-006-002:** Each of the 6 scopes MUST gain the 3 required regression E2E planning items (scenario-specific DoD, broader suite DoD, Test Plan row) — 18 items total.
- **R-BUG-031-006-003:** `scopes.md` MUST satisfy Check 8D containment, either by adding a `Change Boundary` section with allowed/excluded surface enumeration and a change-boundary DoD item, or by rescoping the keyword usage that triggers the check.
- **R-BUG-031-006-004:** `report.md` MUST contain a `### Code Diff Evidence` section enumerating real implementation deltas with file paths, line counts, and gate-verifiable references.
- **R-BUG-031-006-005:** A new SLA stress test MUST cover the Scope 6 60-second ML readiness timeout, consume SST-derived env, and run against the disposable test stack only.
- **R-BUG-031-006-006:** Each of `regression`, `simplify`, `stabilize`, `security`, `chaos`, `docs`, `test`, `audit`, `validate` MUST emit a real `bubbles.<phase>` `executionHistory` entry with `completedPhaseClaimDetails` provenance (agent, timestamp, phase, summary).
- **R-BUG-031-006-007:** Scenario-first TDD red→green markers MUST appear in scope evidence and `report.md` for the new SLA stress test (G060).
- **R-BUG-031-006-008:** Closure MUST land via a structured commit message with the `spec(031)` or `bubbles(031/...)` prefix.
- **R-BUG-031-006-009:** No G041 manipulation pattern (checkbox deletion, status rename, claim stripping, scope-status invention) may appear in the closure diff.
- **R-BUG-031-006-010:** Adversarial regression coverage MUST include at least one test case that would fail if the SLA timeout were silently bypassed and at least one that would fail if the readiness endpoint regressed to always-200.

## User Scenarios (Gherkin)

```gherkin
Feature: BUG-031-006 strict-guard gate drift closure

  Scenario: state-transition-guard accepts the closure
    Given specs/031-live-stack-testing has all 38 BLOCK findings closed with real evidence
    And no G041 manipulation pattern is present in the closure diff
    When bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing runs
    Then the script exits 0 with zero BLOCK findings
    And artifact-lint.sh continues to exit 0
    And regression-baseline-guard.sh exits 0

  Scenario: G022 phase provenance is real
    Given each of regression, simplify, stabilize, security, chaos, docs, test, audit, validate has run
    When state.json executionHistory is inspected
    Then each phase has a structured bubbles.<phase> entry with timestamp and summary
    And completedPhaseClaimDetails enumerates per-phase agent provenance
    And no narrative-only "Phase Agent" line is the sole evidence

  Scenario: G016 regression E2E planning is complete per scope
    Given all 6 scopes describe at least one user-visible behavior
    When scopes.md is inspected
    Then each scope has a scenario-specific regression E2E DoD item
    And each scope has a broader regression suite DoD item
    And each scope has an explicit regression Test Plan row referencing the test file

  Scenario: Check 8D Change Boundary containment
    Given scopes.md is inspected after closure
    When the Change Boundary section is checked
    Then either a Change Boundary section enumerates allowed/excluded surfaces and a change-boundary DoD item is present
    Or the keyword usage that triggers Check 8D has been rescoped to remove the false-positive
    And Check 8D no longer fires

  Scenario: Scope 6 SLA stress coverage exists
    Given the disposable test stack is healthy
    When the new Scope 6 SLA stress test runs against the 60-second ML readiness timeout
    Then the test asserts the timeout fires at the configured boundary
    And the test consumes SMACKEREL_ML_READINESS_TIMEOUT from SST env
    And the test never touches the persistent dev stack

  Scenario: Adversarial regression catches silent bypass
    Given the new SLA stress test exists
    When a hypothetical change silently bypasses the timeout
    Then the adversarial test case fails before merge
    And when a hypothetical change makes the readiness endpoint always return 200
    Then the second adversarial case fails before merge

  Scenario: Structured commit lands closure
    Given the closure is staged
    When git commit runs without --no-verify
    Then the commit message matches ^(spec\(031\)|bubbles\(031/.*\)):
    And all pre-commit hooks (gitleaks, etc.) pass on the first attempt
```

## Risks

- **Check 8D false-positive remediation may require widespread test-helper renames.** Adding a Change Boundary section is the cheaper path and is the recommended remediation.
- **Re-running 9 specialist phases per scope is high-volume work.** Closure should be batched into a single delivery sprint via `bugfix-fastlane` or a dedicated `bubbles.workflow` invocation against `bugs/BUG-031-006-strict-guard-gate-drift/`.
- **The SLA stress test for the 60-second ML readiness timeout will be slow.** Test should use a configurable `MOCK_ML_READINESS_TIMEOUT` SST override at the disposable-stack level to compress the loop while still proving the boundary.
- **Pre-existing unrelated working-tree edits for spec 055** must remain untouched during closure work. Each closure commit MUST use path-limited `git add` and verify with `git diff --cached --name-status` before landing.
