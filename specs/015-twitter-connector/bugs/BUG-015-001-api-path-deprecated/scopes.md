# Scopes: [BUG-015-001] Twitter API Polling Path Deprecation

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Document the deprecation across all 015-twitter-connector artifacts

**Status:** Done
**Priority:** P2
**Depends On:** None

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: [Bug] BUG-015-001 — Twitter API Polling Path Deprecation

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

  Scenario: SCN-BUG015001-004 state.json certifies done with scope-6 deferred
    Given specs/015-twitter-connector/state.json
    When status and certification.status are read
    Then both equal "done"
    And completedScopes lists scope-01..scope-05 only
    And certification.scopeProgress entry for scope 6 has status "Deferred"
    And resolvedBugs contains an entry for BUG-015-001

  Scenario: SCN-BUG015001-005 governance scripts pass on the amended spec
    Given the amendments are applied
    When bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector runs
    Then it exits 0
    And bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector exits 0

  Scenario: SCN-BUG015001-006 unit tests remain green
    Given the amendments are applied
    When ./smackerel.sh test unit runs
    Then it exits 0
```

### Implementation Plan
1. Amend `specs/015-twitter-connector/spec.md`: relocate R-008 + SCN-TW-005 to a Deferred / Non-Goals — API Path section, add API-strategy banner, reword goal #5.
2. Amend `specs/015-twitter-connector/scopes.md`: Phase Order entry 6 → Deferred; Scope Summary row 6 → Deferred; replace Scope 06 section with deprecation block.
3. Amend `specs/015-twitter-connector/uservalidation.md`: item 13 → strikethrough non-applicable; disposition section updated.
4. Update `specs/015-twitter-connector/state.json`: status + cert.status → done, scope-6 → Deferred, resolvedBugs entry, priorReopens history.
5. Append BUG-015-001 deprecation resolution section to `specs/015-twitter-connector/report.md`.
6. Add `specs/015-twitter-connector/scenario-manifest.json` mapping 6 active scope-level scenarios.
7. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector`; expect EXIT=0.
8. Run `bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector`; expect EXIT=0.
9. Run `./smackerel.sh test unit`; expect EXIT=0 with twitter package green.

### Test Plan

| # | Type | Label | Test File / Command | Scenario |
|---|------|-------|---------------------|----------|
| 1 | Doc-grep | Deferred section in spec.md | `grep -n 'Deferred / Non-Goals — API Path' specs/015-twitter-connector/spec.md` | SCN-BUG015001-001 |
| 2 | Doc-grep | No VERIFIED FAIL in scopes/uservalidation | `grep -n '\[ \] VERIFIED FAIL' specs/015-twitter-connector/{scopes,uservalidation}.md` returns nothing | SCN-BUG015001-002, SCN-BUG015001-003 |
| 3 | jq | state.json status | `jq -r '.status, .certification.status' specs/015-twitter-connector/state.json` returns `done\ndone` | SCN-BUG015001-004 |
| 4 | jq | scope-6 deferred | `jq -r '.certification.scopeProgress[] | select(.scope==6) | .status' specs/015-twitter-connector/state.json` returns `Deferred` | SCN-BUG015001-004 |
| 5 | jq | resolvedBugs entry | `jq -r '.resolvedBugs[].bugId' specs/015-twitter-connector/state.json` returns `BUG-015-001` | SCN-BUG015001-004 |
| 6 | Governance | artifact-lint clean | `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` EXIT=0 | SCN-BUG015001-005 |
| 7 | Governance | traceability-guard clean | `bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector` EXIT=0 | SCN-BUG015001-005 |
| 8 | Test | unit suite green | `./smackerel.sh test unit` EXIT=0 | SCN-BUG015001-006 |
| 9 | Adversarial | no production code touched | `git diff --name-only HEAD -- internal/connector/twitter/` returns empty | (boundary check) |

### Definition of Done — 3-Part Validation

#### Part 1 — Core Items
- [x] spec.md has a `Deferred / Non-Goals — API Path` section linking to BUG-015-001
   - **Evidence:** `grep -n 'Deferred / Non-Goals — API Path\|BUG-015-001' specs/015-twitter-connector/spec.md` shows the new section heading and at least one back-link to the bug packet.
- [x] spec.md goal #5 is reworded to mark optional API polling as Deferred
   - **Evidence:** spec.md Goals list item 5 now reads "Optional API polling (Deferred — see BUG-015-001)…" instead of the original active wording.
- [x] spec.md `## ⚠️ API Access Strategy` carries a banner pointing to BUG-015-001
   - **Evidence:** First lines of the API Access Strategy section now begin with `> **DEPRECATED (2026-04-26):**` and link the bug packet.
- [x] scopes.md Scope Summary row 6 status is `Deferred`
   - **Evidence:** `grep -n '| 6 |' specs/015-twitter-connector/scopes.md` returns the row with `Deferred` in the Status column.
- [x] scopes.md Phase Order entry 6 is marked Deferred
   - **Evidence:** Phase Order list item 6 ends with "(Deferred — see BUG-015-001)".
- [x] scopes.md `## Scope 06: API Client (Opt-In)` is replaced with a Deferred deprecation block
   - **Evidence:** Scope 06 section now contains "Status: Deferred", the deprecation rationale, and a back-link to BUG-015-001; no `[ ] VERIFIED FAIL` lines remain.
- [x] uservalidation.md item 13 is a strikethrough non-applicable entry pointing to BUG-015-001
   - **Evidence:** Item 13 now reads `- ~~[ ] Optional API polling respects free-tier rate limits~~ — **Deferred (BUG-015-001)**` with a one-line evidence block linking the bug packet.
- [x] uservalidation.md disposition section reflects archive-only certification
   - **Evidence:** Validation Disposition section now states "12 verified-pass + 1 deferred per BUG-015-001; spec status promoted in_progress → done".
- [x] state.json `status` and `certification.status` are both `done`
   - **Evidence:** `jq -r '.status, .certification.status' specs/015-twitter-connector/state.json` → `done\ndone`.
- [x] state.json `completedScopes` is exactly scope-01..scope-05
   - **Evidence:** `jq -r '.execution.completedScopes' specs/015-twitter-connector/state.json` returns the 5-element list excluding scope-06.
- [x] state.json `certification.scopeProgress[scope=6].status` is `Deferred`
   - **Evidence:** `jq` query against scopeProgress returns `Deferred` plus a `deferredReason` carrying the option-(b) rationale and a `deferredAt` 2026-04-26 timestamp.
- [x] state.json `resolvedBugs` contains a BUG-015-001 entry with priorReopens history
   - **Evidence:** `jq -r '.resolvedBugs[].bugId' specs/015-twitter-connector/state.json` → `BUG-015-001`; the entry carries `resolution: "deprecated"`, `link`, `resolvedAt`, and `priorReopens`.
- [x] report.md has a `## BUG-015-001 Deprecation Resolution (2026-04-26)` section
   - **Evidence:** `grep -n 'BUG-015-001 Deprecation Resolution' specs/015-twitter-connector/report.md` returns 1 match.
- [x] scenario-manifest.json exists at the parent feature root with 6 active scope-level scenarios
   - **Evidence:** `jq '.scenarios | length' specs/015-twitter-connector/scenario-manifest.json` → 6; `jq -r '.scenarios[].scenarioId' …` → SCN-TW-ARC-001/002, SCN-TW-THR-001/002, SCN-TW-CONN-001/002.
- [x] No files under `internal/connector/twitter/` are modified
   - **Evidence:** `git diff --name-only HEAD -- internal/connector/twitter/` returns an empty set across the entire bug execution.
- [x] Adversarial regression case exists and would fail if the bug returned
   - **Evidence:** Test #2 (`grep -n '\[ \] VERIFIED FAIL' specs/015-twitter-connector/{scopes,uservalidation}.md returns nothing`) and test #9 (`git diff --name-only HEAD -- internal/connector/twitter/` empty) both flip to a failure if the deprecation is reverted or if production code is silently changed.
- [x] Regression checks contain no silent-pass bailout patterns
   - **Evidence:** All checks are literal grep / jq value comparisons with non-zero exit on mismatch; no `if (… includes …) return;` early-exit patterns.
- [x] All existing tests pass (no regressions)
   - **Evidence:** `./smackerel.sh test unit` EXIT=0 captured in report.md test evidence section.

#### Part 2 — Build Quality Gate
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector`
   - **Evidence:** EXIT=0 captured in report.md audit evidence.
- [x] Traceability guard clean: `bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector`
   - **Evidence:** EXIT=0 captured in report.md audit evidence; scenario-manifest cross-check now passes.
- [x] Unit tests clean: `./smackerel.sh test unit`
   - **Evidence:** EXIT=0 captured in report.md test evidence.
- [x] Docs aligned with implementation
   - **Evidence:** spec.md, scopes.md, uservalidation.md, state.json, report.md all consistently report archive-only certification with Scope 6 Deferred and BUG-015-001 closed.

**E2E tests are MANDATORY for code-changing bug fixes — this is a documentation-only deprecation, so the unit-test sweep + governance scripts serve as the regression boundary per the option-(b) decision.**
