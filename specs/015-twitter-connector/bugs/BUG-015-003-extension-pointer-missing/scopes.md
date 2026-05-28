# Scopes: [BUG-015-003] Spec 015 forward pointer to spec 056

## Scope 1: Record spec-056 extension pointer in spec 015 artifacts
**Status:** Done

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: Spec 015 records spec-056 as the API/hybrid implementation home
  Scenario: state.json carries a forward pointer to spec 056
    Given a reader inspects specs/015-twitter-connector/state.json
    When they grep for "056-twitter-api-connector" in a forward-pointer field
    Then at least one match is returned

  Scenario: spec.md carries a top-of-document forward-pointer banner
    Given a reader opens specs/015-twitter-connector/spec.md
    When they grep for "056-twitter-api-connector"
    Then at least two matches are returned (top-of-document banner + API Access Strategy section note)

  Scenario: design.md carries a top-of-document forward-pointer note
    Given a reader opens specs/015-twitter-connector/design.md
    When they grep for "056-twitter-api-connector"
    Then at least one match is returned at the top of the document

  Scenario: API Access Strategy section is no longer misleading
    Given a reader reads the "API Access Strategy" section in specs/015-twitter-connector/spec.md
    When they search for the heading "API Access Strategy"
    Then a "056-twitter-api-connector" reference appears within ±5 lines of the heading

  Scenario: Fix is artifact-only
    Given the fix is applied
    When git diff --name-only is inspected
    Then every changed path is under specs/015-twitter-connector/
```

### Implementation Plan
1. Read `specs/015-twitter-connector/state.json` end-to-end and identify the additive top-level field placement (mirror the `supersessions[]` recipe from BUG-020-007); add an `extensions[]` array (or `supersessions[]` fallback if schema requires) with one entry naming `056-twitter-api-connector`, scoped to `SyncModeAPI` + `SyncModeHybrid` implementation.
2. Add a top-of-document forward-pointer banner blockquote to `specs/015-twitter-connector/spec.md` near the front matter, naming spec 056 and clarifying that spec 015 ships the archive path only.
3. Add a second inline note above (or immediately under) the "API Access Strategy — Critical Design Decision" heading at L72 of `spec.md` pointing at spec 056 as the implementation home for Options B / C / E.
4. Mirror the spec.md banner at the top of `specs/015-twitter-connector/design.md`.
5. Run `git diff --name-only` and verify every changed path is under `specs/015-twitter-connector/`.
6. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` and the bug folder.

### Test Plan
| Label | Type | What it asserts |
|-------|------|-----------------|
| Pre-fix forward-pointer grep — adversarial | Regression artifact-shape | Three greps for `056-twitter-api-connector` against `state.json`, `spec.md`, `design.md` MUST all return exit 1 before the fix |
| Post-fix forward-pointer grep | Regression artifact-shape | Same three greps MUST return exit 0 after the fix; spec.md grep MUST return ≥ 2 matches |
| API Access Strategy proximity guard | Regression E2E (artifact) | The heading `API Access Strategy` in `spec.md` MUST have a `056-twitter-api-connector` reference within ±5 lines |
| Change-boundary guard | Regression E2E (artifact) | `git diff --name-only` returns only paths under `specs/015-twitter-connector/` |
| artifact-lint (parent) | Regression artifact | `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` exits 0 |
| artifact-lint (bug) | Regression artifact | `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing` exits 0 |

No runtime code is changed, so no unit / integration / e2e-api / e2e-ui / stress / load tests apply (`tdd.exempt: artifact-only` per `policySnapshot.tdd.mode = exempt`). The Test Plan is artifact-shape regression by design.

### Definition of Done — 3-Part Validation
- [x] Root cause confirmed and documented
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ for f in specs/015-twitter-connector/state.json specs/015-twitter-connector/spec.md specs/015-twitter-connector/design.md; do echo "--- $f ---"; grep -c "056-twitter-api-connector" "$f"; done
      --- specs/015-twitter-connector/state.json ---
      0
      --- specs/015-twitter-connector/spec.md ---
      0
      --- specs/015-twitter-connector/design.md ---
      0
      ```
      Root cause: spec 015 declares `SyncMode = archive | api | hybrid` and dedicates the entire "API Access Strategy" section to the hybrid strategy, but ships archive-only code. Spec 056-twitter-api-connector (certified `specs_hardened` 2026-05-27) is the actual implementation of `SyncModeAPI` / `SyncModeHybrid` (code in `internal/connector/twitter/api.go` + `api_test.go`). No reverse forward-pointer was back-propagated into spec 015 artifacts.
      **Claim Source:** executed
- [x] Fix implemented (artifact edits in `state.json`, `spec.md`, `design.md`)
   - Raw output evidence (inline under this item):
      ```
      $ git diff --name-only
      specs/015-twitter-connector/design.md
      specs/015-twitter-connector/spec.md
      specs/015-twitter-connector/state.json
      $ python3 -c "import json; json.load(open('specs/015-twitter-connector/state.json')); print('OK')"
      OK
      ```
      **Claim Source:** executed
- [x] Pre-fix regression test FAILS (three greps for `056-twitter-api-connector` exit 1 against `main`)
   - Raw output evidence (inline under this item):
      ```
      === PRE-FIX GREP ===
      --- specs/015-twitter-connector/state.json ---
      0
      exit=1
      --- specs/015-twitter-connector/spec.md ---
      0
      exit=1
      --- specs/015-twitter-connector/design.md ---
      0
      exit=1
      ```
      All three greps exit 1 BEFORE edits — the bug is reproducible.
      **Claim Source:** executed
- [x] Adversarial regression case exists and would fail if the bug returned (API Access Strategy ±5-line proximity guard)
   - Raw output evidence (inline under this item):
      ```
      $ awk '/^## .*API Access Strategy/{h=NR} h && NR>=h-5 && NR<=h+5 && /056-twitter-api-connector/{print "match L"NR; found=1} END{exit found?0:1}' specs/015-twitter-connector/spec.md
      match L76
      ```
      The "API Access Strategy" heading is at L74; the spec-056 reference inserted at L76 is within ±5 lines, satisfying the proximity guard. If a future edit removed the inline note under the heading without leaving a 056 reference within ±5 lines, the guard would fail (exit 1).
      **Claim Source:** executed
- [x] Post-fix regression test PASSES (three greps exit 0 after edits; spec.md grep ≥ 2 matches)
   - Raw output evidence (inline under this item):
      ```
      === POST-FIX GREP ===
      --- specs/015-twitter-connector/state.json ---
      matches=2 exit=0
      --- specs/015-twitter-connector/spec.md ---
      matches=2 exit=0
      --- specs/015-twitter-connector/design.md ---
      matches=1 exit=0
      ```
      All three greps now exit 0. spec.md returns 2 matches (top-of-document banner + API Access Strategy section note), meeting the ≥ 2 requirement.
      **Claim Source:** executed
- [x] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item):
      ```
      The regression "tests" here are 4 explicit greps with explicit expected exit codes (exit 1 pre-fix, exit 0 post-fix), a proximity awk guard with explicit exit code, and a change-boundary check. No `if ... return` early exits, no `route()`/`intercept()`/`msw`/`nock` mocking, no skip-on-unexpected-state branches. Inversion check: if the bug returned (any of the 3 pointers removed), the corresponding post-fix grep would exit 1 and the test would fail.
      ```
      **Claim Source:** interpreted
- [x] Change boundary respected: `git diff --name-only` shows only paths under `specs/015-twitter-connector/`
   - Raw output evidence (inline under this item):
      ```
      $ git diff --name-only
      specs/015-twitter-connector/design.md
      specs/015-twitter-connector/spec.md
      specs/015-twitter-connector/state.json
      ```
      All 3 changed paths are under `specs/015-twitter-connector/`. No code, no other specs.
      **Claim Source:** executed
- [x] artifact-lint passes on parent spec 015 and on this bug folder
   - Raw output evidence (inline under this item):
      ```
      $ bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector
      ...
      Artifact lint PASSED.

      $ bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing
      ...
      Artifact lint PASSED.
      ```
      Both lints PASSED with full output captured in the terminal session preceding this commit.
      **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
   - Raw output evidence (inline under this item):
      ```
      The 5 Gherkin scenarios in this scope map 1:1 to the 5 artifact-shape regression checks above (3 forward-pointer greps + ±5-line proximity guard + change-boundary check). All 5 pass. This is artifact-only drift — no runtime behavior changed — so the regression suite IS the artifact-shape suite (tdd.exempt: artifact-only per user dispatch).
      ```
      **Claim Source:** interpreted
- [x] Broader E2E regression suite passes
   - Raw output evidence (inline under this item):
      ```
      Not applicable — zero runtime code changed (`git diff --name-only` shows only 3 paths under `specs/015-twitter-connector/`, all .md/.json). The broader E2E suite cannot regress on documentation-only edits to a governance spec. The actual API/hybrid runtime contract is owned by spec 056-twitter-api-connector and its tests in `internal/connector/twitter/api_test.go` continue to run independently.
      ```
      **Claim Source:** interpreted
- [x] Bug marked as Fixed in bug.md
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE '^- \[x\] (Fixed|Verified|Closed|In Progress)' specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/bug.md
      14:- [x] In Progress
      15:- [x] Fixed
      16:- [x] Verified
      17:- [x] Closed
      ```
      bug.md Status section shows In Progress + Fixed + Verified + Closed all checked.
      **Claim Source:** executed
- [x] Consumer Impact Sweep complete — zero stale first-party references remain
   - Raw output evidence (inline under this item):
      ```
      $ grep -rEln "specs/015-twitter-connector|015-twitter-connector" --include="*.go" --include="*.py" --include="*.ts" --include="*.tsx" --include="*.yaml" --include="*.yml" --include="*.md" | grep -v "^specs/015-twitter-connector/" | head -20
      specs/056-twitter-api-connector/spec.md
      specs/056-twitter-api-connector/design.md
      ```
      First-party consumers of spec 015 are spec 056 (which already references spec 015 as its predecessor — reverse pointer already exists). No runtime code consumes spec 015 directly (it is a governance spec). No navigation, breadcrumb, redirect, deep-link, or generated-client surfaces apply.
      **Claim Source:** executed
- [x] state.json carries forward pointer to spec 056 in additive top-level array (faithful to Gherkin scenario 1)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "056-twitter-api-connector" specs/015-twitter-connector/state.json
      7:      "extendedBy": "056-twitter-api-connector",
      9:      "note": "Spec 015 ships the archive-import path only (SyncModeArchive). The SyncModeAPI and SyncModeHybrid paths described in this spec's 'API Access Strategy' section are implemented under spec 056-twitter-api-connector ..."
      ```
      Additive top-level `extensions[]` array with one entry naming `056-twitter-api-connector` and scoping it to `SyncModeAPI` + `SyncModeHybrid`.
      **Claim Source:** executed
- [x] spec.md carries top-of-document banner + API Access Strategy section note (faithful to Gherkin scenario 2)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "056-twitter-api-connector" specs/015-twitter-connector/spec.md
      8:> **⚠️ Extension Pointer (2026-05-28):** ... implemented under [`specs/056-twitter-api-connector`](../056-twitter-api-connector/spec.md) ...
      76:> **⚠️ Implementation home (2026-05-28):** The SyncModeAPI and SyncModeHybrid implementation of this strategy (Options B, C, and E below) lives in [`specs/056-twitter-api-connector`](../056-twitter-api-connector/spec.md). ...
      ```
      Two matches: top-of-document banner (L8) + API Access Strategy section note (L76, directly under the L74 heading).
      **Claim Source:** executed
- [x] design.md carries top-of-document forward-pointer note (faithful to Gherkin scenario 3)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "056-twitter-api-connector" specs/015-twitter-connector/design.md
      8:> **⚠️ Extension Pointer (2026-05-28):** ... documented under [`specs/056-twitter-api-connector`](../056-twitter-api-connector/design.md) ...
      ```
      Top-of-document extension-pointer note immediately under the design.md front matter.
      **Claim Source:** executed
- [x] API Access Strategy heading has a spec-056 reference within ±5 lines (faithful to Gherkin scenario 4)
   - Raw output evidence (inline under this item):
      ```
      $ awk '/^## .*API Access Strategy/{h=NR} h && NR>=h-5 && NR<=h+5 && /056-twitter-api-connector/{print "match L"NR; found=1} END{exit found?0:1}' specs/015-twitter-connector/spec.md
      match L76
      ```
      Heading at L74; spec-056 reference at L76 (within ±5 lines).
      **Claim Source:** executed
- [x] Fix is artifact-only — every changed path is under specs/015-twitter-connector/ (faithful to Gherkin scenario 5)
   - Raw output evidence (inline under this item):
      ```
      $ git diff --name-only
      specs/015-twitter-connector/design.md
      specs/015-twitter-connector/spec.md
      specs/015-twitter-connector/state.json
      ```
      All 3 paths under `specs/015-twitter-connector/`. No code, no other specs.
      **Claim Source:** executed
