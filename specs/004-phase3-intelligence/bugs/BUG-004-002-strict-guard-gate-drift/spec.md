# Bug Specification: BUG-004-002 Strict-Guard Gate Drift on Spec 004

## Problem Statement

Spec 004 (Phase 3 — Intelligence: Synthesis + Alerts + Pre-Meeting Briefs) was promoted to `status: done` before the current set of strict mechanical gates was fully enforced. The implementation on disk is real (`internal/intelligence/engine.go`, `internal/intelligence/resurface.go`, `internal/digest/generator.go`, `internal/scheduler/scheduler.go` with full Go unit suite + 6 schema-validation E2E scripts), and 78/78 DoD items across all 6 scopes are checked with evidence, but `state-transition-guard.sh` now rejects the prior promotion with 20 BLOCK findings + 1 warning across 2 gate categories:

- **G016 (Check 8A): 19 BLOCK findings.** All 6 spec-004 scopes lack the gate-required regression-E2E planning trio:
  - DoD item matching `^\- \[(x| )\] Scenario-specific E2E regression tests? for (EVERY|every) new/changed/fixed behavior` (6 missing — one per scope).
  - DoD item matching `^\- \[(x| )\] Broader E2E regression suite passes` (6 missing — one per scope).
  - Test Plan row matching `^\|.*Regression E2E` (6 missing — one per scope).
  - Plus one aggregate "18 regression E2E planning requirement(s) missing" failure.
- **Check 17: 1 BLOCK finding.** `full-delivery` workflow mode requires at least one structured commit message with the `spec(004)` or `bubbles(004/...)` prefix; existing history has `feat(004)`, `docs(004)`, `audit(004,005)`, `feat(004,005,006)`, but none matches the strict regex.

All other strict-guard checks (G041 anti-manipulation, G022 phase provenance, G027 phase-scope coherence, G028 implementation reality, G053 implementation delta evidence, G055 policy snapshot, G056 certification block, G057 scenario manifest, G060 scenario-first TDD evidence, G068 DoD-Gherkin fidelity, Check 14 TODO/FIXME, Check 17 commit count, Check 18 deferral language, Check 22 DoD fidelity) PASS. `artifact-lint.sh` continues to PASS — it accepts the existing DoD set at face value. Only the strict mechanical guard catches the under-evidenced promotion.

## Outcome Contract

**Intent:** Spec 004 returns to `status: done` only after every one of the 20 BLOCK findings is closed with real, gate-verifiable evidence — no laundering, no checkbox stripping, no scope renames.

**Success Signal:**
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/004-phase3-intelligence` exits 0 with zero BLOCK findings.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/004-phase3-intelligence` continues to exit 0.
- A structured `spec(004)` or `bubbles(004/...)` commit lands the closure (satisfies Check 17 by construction).
- All 6 spec-004 scopes show a real `Regression E2E` Test Plan row plus the two new DoD items per scope, each referencing existing test files on disk.

**Hard Constraints:**
- **G041 anti-manipulation.** No DoD checkbox MAY be deleted, converted to a non-checkbox bullet, struck through, or italicized to dodge the count. No scope status MAY be renamed to a non-canonical value (`Not Started`, `In Progress`, `Done`, `Blocked` are the only legal strings). No `completedPhaseClaims` entry MAY be stripped.
- **Implementation is verified real.** No source file MAY be deleted or rewritten on the rationale that "the work was never done"; the synthesis engine, commitment tracker, brief generator, alert manager, weekly synthesis, and enhanced daily digest are all already on disk with passing Go unit tests. Edits flow forward by adding regression-coverage planning items that reference existing test files.
- **NO-DEFAULTS SST.** Any new test reference MUST consume SST-derived environment variables, never hardcoded ports/URLs. The added planning rows reference existing test files only; no new test source is required.
- **Test-environment isolation.** Any new test reference MUST run against the disposable test stack, never the persistent dev stack. The existing `tests/e2e/test_*.sh` schema-validation scripts already meet this contract.
- **Spec 055 WIP preservation.** The 30-path working-tree changes under `cmd/core/`, `internal/api/`, `internal/notification/source/`, `internal/web/`, `internal/config/`, `internal/db/migrations/`, `tests/e2e/notification_ntfy_source_*`, `tests/stress/notification_ntfy_source_*`, `config/smackerel.yaml`, `docs/{API,Architecture,Development,Operations}.md`, `scripts/commands/config.sh`, and `specs/055-notification-source-ntfy-adapter/**` belong to an in-flight author commit boundary and MUST NOT be swept into this BUG's commits. Path-limited `git add` only.
- **No `--no-verify` push.** Every commit MUST clear the standard pre-commit hooks; if `gitleaks` flags PII in evidence blocks, redact `/home/<user>/` → `~/` with `multi_replace_string_in_file` and re-stage.

**Failure Condition:** The bug remains open if `state-transition-guard.sh` continues to BLOCK promotion after closure, if any G041 manipulation pattern is detected in the closure diff, if any added regression test reference points at a non-existent file, or if the closure commit prefix fails the Check 17 regex.

## Goals

1. Close the 18 missing G016 regression E2E planning items across all 6 spec-004 scopes (scenario-specific DoD + broader suite DoD + Test Plan row = 3 items × 6 scopes), each referencing an existing test file on disk.
2. Close the 1 aggregate Check 8A failure (auto-cleared when all 18 individual items are closed).
3. Land closure under a structured commit message: `spec(004,bug-004-002): close strict-guard gate drift` — satisfies Check 17 on the spec 004 commit history.

## Non-Goals

- Re-implementing the intelligence engine, commitment tracker, brief generator, alert manager, weekly synthesis, or enhanced daily digest (all already real on disk; the Go unit suite passes).
- Adding new behavioral E2E or integration tests (the existing test surface is enumerated by the prior phase-3 implementation; this BUG adds planning-row references only).
- Touching the spec 055 (notification ntfy adapter) working-tree edits currently in-flight.
- Editing generated config under `config/generated/` by hand.
- Adding new feature surface beyond what the original 6 scopes describe.
- Modifying any Go production source file under `internal/intelligence/`, `internal/digest/`, or `internal/scheduler/` (the BUG change manifest is planning-only).

## Requirements

- **R-BUG-004-002-001:** `state-transition-guard.sh` MUST exit 0 against `specs/004-phase3-intelligence/` after closure.
- **R-BUG-004-002-002:** Each of the 6 spec-004 scopes MUST gain the 3 required regression E2E planning items (scenario-specific DoD, broader suite DoD, Test Plan row) — 18 items total — and each MUST reference an existing test file on disk.
- **R-BUG-004-002-003:** Closure MUST land via a structured commit message with the `spec(004)` or `bubbles(004/...)` prefix.
- **R-BUG-004-002-004:** No G041 manipulation pattern (checkbox deletion, status rename, claim stripping, scope-status invention) may appear in the closure diff.
- **R-BUG-004-002-005:** Each added Test Plan row MUST name the closing finding (`BUG-004-002:Scope-1`) inline so a future maintainer can trace the row to this bug.
- **R-BUG-004-002-006:** `artifact-lint.sh` MUST continue to exit 0 against both `specs/004-phase3-intelligence/` and the BUG packet folder.

## User Scenarios (Gherkin)

```gherkin
Feature: BUG-004-002 strict-guard gate drift closure

  Scenario: SCN-BUG-004-002-001 state-transition-guard accepts the closure
    Given specs/004-phase3-intelligence has all 20 BLOCK findings closed with real evidence
    And no G041 manipulation pattern is present in the closure diff
    When bash .github/bubbles/scripts/state-transition-guard.sh specs/004-phase3-intelligence runs
    Then the script exits 0 with zero BLOCK findings
    And artifact-lint.sh continues to exit 0

  Scenario: SCN-BUG-004-002-002 G016 regression E2E planning is complete per scope
    Given all 6 spec-004 scopes describe at least one user-visible behavior
    When scopes.md is inspected
    Then each scope has a scenario-specific regression E2E DoD item with the exact gate-required phrase
    And each scope has a broader regression suite DoD item with the exact gate-required phrase
    And each scope has an explicit Regression E2E Test Plan row referencing an existing test file
    And every referenced test file exists on disk

  Scenario: SCN-BUG-004-002-003 Structured commit lands closure
    Given the closure diff is staged path-limited (no spec 055 WIP swept in)
    When git commit runs without --no-verify
    Then the commit message matches ^(spec\(004\)|bubbles\(004/.*\)):
    And the pre-commit hook (gitleaks) passes
    And Check 17 PASSes on the next state-transition-guard.sh run
```
