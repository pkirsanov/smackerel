# Feature: [BUG-015-001] Twitter API Polling Path Deprecation

## Problem Statement
Twitter Scope 6 (API Client / Opt-In) DoD asserted an implemented API polling path (`FetchBookmarks`, `FetchLikes`, rate-limit logging) that does not exist in `internal/connector/twitter/twitter.go`. The Sync method only delegates to `syncArchive()`. After review, the team chose to formally deprecate the API path instead of implementing it, because the archive-import path already covers the real user value and the X API free tier is no longer a stable foundation.

## Outcome Contract
**Intent:** Reconcile the Twitter connector's spec, scopes, user validation, and state with what is actually built and certified — the archive-import surface — by retiring Scope 6 and the optional API path as a Non-Goal / Deferred capability.

**Success Signal:** After this bug closes:
- `specs/015-twitter-connector/spec.md` lists the API polling path under "Deferred / Non-Goals" with rationale and a back-link to BUG-015-001.
- `specs/015-twitter-connector/scopes.md` shows Scope 6 with status `Deferred` and a rationale block; Scopes 1-5 remain `Done`.
- `specs/015-twitter-connector/uservalidation.md` item 13 is converted to a non-applicable entry pointing to this bug, and the disposition note reflects archive-only certification.
- `specs/015-twitter-connector/state.json` shows `status: done`, `certification.status: done`, scopes 1-5 in `completedScopes`, scope-6 explicitly tracked as `Deferred`, and a `resolvedBugs` entry for BUG-015-001 with priorReopens history.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` passes.
- `./smackerel.sh test unit` is green (no production code change).

**Hard Constraints:**
- Documentation/spec amendment only. NO production code may be modified in `internal/connector/twitter/`.
- The unused `bearer_token` and `api_enabled` config fields stay in `config/smackerel.yaml` and `parseTwitterConfig` — out of scope for this bug, tracked separately if cleanup is desired later.
- Historical context must be preserved (do not delete API requirements; mark them deferred with rationale).
- Locked scenarios (`SCN-TW-ARC-001`, `SCN-TW-ARC-002`, `SCN-TW-THR-001`, `SCN-TW-THR-002`, `SCN-TW-CONN-001`, `SCN-TW-CONN-002`, `SCN-TW-001`, `SCN-TW-002`) remain locked.

**Failure Condition:** Documentation still claims an implemented API path, or state.json promotes to `done` without the resolvedBugs entry / scope-6 deferral marker, or the archive-path test suite regresses.

## Goals
- Move R-008 (Optional API polling) from Requirements into a `## Deferred / Non-Goals — API Path` section in spec.md with explicit rationale and the BUG-015-001 link.
- Update spec.md goals/non-goals to reflect archive-only as the certified surface.
- Annotate or remove the API-specific Gherkin scenario (SCN-TW-005) so the spec only ships scenarios backed by real code.
- Retire scopes.md Scope 6 (status: Deferred) with a rationale block, leaving the historical DoD intact for traceability but removing all "verified-fail" claims.
- Convert uservalidation.md item 13 to a non-applicable entry with strikethrough + note linking to BUG-015-001.
- Update state.json: `status: done`, `certification.status: done`, scopes 1-5 in `completedScopes`, scope-6 marked `Deferred`, `resolvedBugs` entry added, `priorReopens` history preserved, `currentPhase: finalize`.
- Append a deprecation-resolution section to specs/015-twitter-connector/report.md.
- Add a `scenario-manifest.json` mapping the 6 active scope-level scenarios to scopes 1, 2, 4 (so traceability-guard scenario-manifest cross-check is satisfied).

## Non-Goals
- Implementing any Twitter API v2 client logic. (Use option (a) in a future spec if/when X API tier reliability changes.)
- Removing the unused `bearer_token` / `api_enabled` config fields. (Cleanup tracked separately; out of scope for documentation deprecation.)
- Modifying `internal/connector/twitter/twitter.go`, `internal/connector/twitter/twitter_test.go`, or any wiring in `cmd/core/connectors.go`.
- Re-running heavy integration / e2e / stress suites. Only `./smackerel.sh test unit` is required to confirm no production code changed.

## Requirements
- R1: spec.md MUST relocate R-008 and the SCN-TW-005 Gherkin scenario into a `Deferred / Non-Goals — API Path` section that includes the BUG-015-001 link and the four-bullet deprecation rationale.
- R2: spec.md goal #5 ("Optional API polling") MUST be re-worded as deferred, and the `## ⚠️ API Access Strategy` section MUST carry a top banner pointing to BUG-015-001.
- R3: scopes.md Scope Summary table row 6 status MUST change to `Deferred (BUG-015-001)`. The Phase Order entry for Scope 6 MUST be marked deferred.
- R4: scopes.md Scope 06 section MUST be replaced with a deprecation block: it carries status `Deferred`, rationale matching bug.md, a back-link to this bug, and removes the prior `[ ] VERIFIED FAIL (BUG-015-001)` checkbox lines. The historical DoD list is replaced with a single non-applicable note, not deleted from history (the report.md replay record retains the verified-fail audit trail).
- R5: uservalidation.md item 13 MUST become a non-applicable entry (`- ~~[ ] Optional API polling…~~ — Deferred per BUG-015-001`) with an inline note. The disposition section MUST be updated to "Validated — archive-only surface; API path deferred per BUG-015-001."
- R6: state.json MUST be updated atomically:
  - `status: "done"`
  - `certification.status: "done"`
  - `certification.certifiedAt`: 2026-04-26 timestamp
  - `certification.certifiedBy`: `bubbles.workflow` (bugfix-fastlane)
  - `execution.completedScopes`: scope-01..scope-05
  - `certification.completedScopes`: same
  - `execution.currentPhase: "finalize"`, `execution.currentScope: null`
  - `certification.scopeProgress[scope=6].status: "Deferred"` with `deferredReason` and `deferredAt`
  - `resolvedBugs`: array entry for `BUG-015-001` with `priorReopens` history
  - `certification.reopenReason`: cleared (or kept as historical with closure note)
- R7: report.md MUST gain a `## BUG-015-001 Deprecation Resolution (2026-04-26)` section documenting the option-(b) selection, listing affected files, and pointing to the bug packet.
- R8: A `scenario-manifest.json` MUST exist at `specs/015-twitter-connector/scenario-manifest.json` covering the 6 scope-level scenarios (SCN-TW-ARC-001/002, SCN-TW-THR-001/002, SCN-TW-CONN-001/002) so the traceability guard scenario-manifest cross-check passes.
- R9: `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` MUST exit 0.
- R10: `./smackerel.sh test unit` MUST exit 0 (no regression in twitter package).

## User Scenarios (Gherkin)

```gherkin
Feature: BUG-015-001 — Twitter API Polling Path Deprecation

  Scenario: SCN-BUG015001-001 spec.md flags API path as deferred with bug back-link
    Given specs/015-twitter-connector/spec.md
    When the Deferred / Non-Goals section is read
    Then it lists the X API polling path
    And it contains the BUG-015-001 deprecation rationale
    And requirement R-008 is no longer in the live Requirements list

  Scenario: SCN-BUG015001-002 scopes.md Scope 6 is Deferred not Reopened
    Given specs/015-twitter-connector/scopes.md
    When Scope 06 is read
    Then its status reads "Deferred"
    And the rationale links to BUG-015-001
    And no checkbox in Scope 06 is marked "[ ] VERIFIED FAIL"

  Scenario: SCN-BUG015001-003 uservalidation item 13 is non-applicable not failing
    Given specs/015-twitter-connector/uservalidation.md
    When the checklist is read
    Then item 13 is rendered as a strikethrough non-applicable entry
    And it points to BUG-015-001
    And the disposition section says "archive-only surface; API path deferred per BUG-015-001"

  Scenario: SCN-BUG015001-004 state.json certifies done with scope-6 deferred
    Given specs/015-twitter-connector/state.json
    When status and certification.status are read
    Then both equal "done"
    And completedScopes lists scope-01..scope-05 (not scope-06)
    And certification.scopeProgress entry for scope 6 has status "Deferred"
    And resolvedBugs contains an entry for BUG-015-001

  Scenario: SCN-BUG015001-005 governance scripts pass on the amended spec
    Given the amendments are applied
    When bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector runs
    Then it exits 0
    And bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector exits 0

  Scenario: SCN-BUG015001-006 unit tests remain green (no production code change)
    Given the amendments are applied
    When ./smackerel.sh test unit runs
    Then it exits 0
    And the twitter package passes its 127 sub-tests
```

## Acceptance Criteria
- AC1: Reading spec.md confirms API polling is in a Deferred section with BUG-015-001 link; R-008 is no longer in active Requirements.
- AC2: Reading scopes.md confirms Scope 6 status `Deferred` with rationale; no `[ ] VERIFIED FAIL` lines remain.
- AC3: Reading uservalidation.md confirms item 13 is non-applicable (strikethrough) with bug back-link; disposition reflects archive-only certification.
- AC4: state.json `status` and `certification.status` are both `done`; `completedScopes` is scopes 1-5; scope-6 progress entry has `status: Deferred`; resolvedBugs lists BUG-015-001.
- AC5: `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` exits 0.
- AC6: `bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector` exits 0.
- AC7: `./smackerel.sh test unit` exits 0 with twitter package green.
- AC8: No file under `internal/connector/twitter/` was modified (verifiable via `git diff --name-only`).
