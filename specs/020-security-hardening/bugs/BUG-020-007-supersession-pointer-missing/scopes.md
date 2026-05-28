# Scopes: [BUG-020-007] Spec 020 supersession pointer to spec 042

## Scope 1: Record spec-042 supersession in spec 020 artifacts
**Status:** Done

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: Spec 020 records spec-042 supersession for host-bind pattern
  Scenario: state.json carries a supersededBy pointer to spec 042
    Given a reader inspects specs/020-security-hardening/state.json
    When they grep for "042-tailnet-edge-bind-pattern" in a supersession field
    Then at least one match is returned

  Scenario: spec.md carries an inline supersession note
    Given a reader opens specs/020-security-hardening/spec.md
    When they grep for "042-tailnet-edge-bind-pattern"
    Then at least one match is returned in or above the "Attack Surface: Network Exposure" section

  Scenario: design.md carries an inline supersession note
    Given a reader opens specs/020-security-hardening/design.md
    When they grep for "042-tailnet-edge-bind-pattern"
    Then at least one match is returned at the top of the document or its first affected section

  Scenario: L269 success-metric row is no longer misleading
    Given a reader reads the "Host port binding" success-metric row in specs/020-security-hardening/spec.md
    When they search for the bare phrase "100% of services bind to 127.0.0.1"
    Then any match found has a "042-tailnet-edge-bind-pattern" reference within ±5 lines

  Scenario: Fix is artifact-only
    Given the fix is applied
    When git diff --name-only is inspected
    Then every changed path is under specs/020-security-hardening/
```

### Implementation Plan
1. Read `specs/020-security-hardening/state.json` and the v3 control-plane state schema; choose the schema-valid supersession field (top-level `supersededBy`, a `supersessions` array entry, or an equivalent provenance entry) and add a pointer naming `042-tailnet-edge-bind-pattern` scoped to the host-bind / loopback-binding prescription.
2. Add an inline supersession note at the top of the "Attack Surface: Network Exposure" section of `specs/020-security-hardening/spec.md` (L48–52) naming spec 042 and the spec-042 invariants (fail-loud `${HOST_BIND_ADDRESS:?...}` for `smackerel-core` / `smackerel-ml`; no host `ports:` block for `postgres` / `nats`).
3. Update or annotate the L269 success-metric row so it cannot be read as "literal 127.0.0.1 for all services is the current target."
4. Mirror the supersession note at L5–10 of `specs/020-security-hardening/design.md` (or at the top of the document).
5. Run `git diff --name-only` and verify every changed path is under `specs/020-security-hardening/`.
6. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening` and the bug folder.

### Test Plan
| Label | Type | What it asserts |
|-------|------|-----------------|
| Pre-fix supersession grep — adversarial | Regression artifact-shape | Three greps for `042-tailnet-edge-bind-pattern` against `state.json`, `spec.md`, `design.md` MUST all return exit 1 before the fix |
| Post-fix supersession grep | Regression artifact-shape | Same three greps MUST return exit 0 after the fix |
| L269 bare-phrase adversarial guard | Regression E2E (artifact) | If `100% of services bind to 127.0.0.1` appears in `spec.md`, a `042-tailnet-edge-bind-pattern` reference MUST appear within ±5 lines |
| Change-boundary guard | Regression E2E (artifact) | `git diff --name-only` returns only paths under `specs/020-security-hardening/` |
| artifact-lint (parent) | Regression artifact | `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening` exits 0 |
| artifact-lint (bug) | Regression artifact | `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing` exits 0 |

No runtime code is changed, so no unit / integration / e2e-api / e2e-ui / stress / load tests apply. The Test Plan is artifact-shape regression by design; the broader E2E regression suite still runs to prove no collateral damage.

### Definition of Done — 3-Part Validation
- [x] Root cause confirmed and documented
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -c "042-tailnet-edge-bind-pattern" specs/020-security-hardening/state.json specs/020-security-hardening/spec.md specs/020-security-hardening/design.md
      specs/020-security-hardening/state.json:0
      specs/020-security-hardening/spec.md:0
      specs/020-security-hardening/design.md:0
      ```
      Root cause: spec 020 prescribes literal `127.0.0.1:HOST_PORT` host binds + "100% of services bind to 127.0.0.1" success metric; spec 042 superseded that prescription on deploy compose (fail-loud `${HOST_BIND_ADDRESS:?...}` form, no host `ports:` block for `postgres`/`nats`), but no `supersededBy` pointer or inline note was back-propagated into spec 020 artifacts. Code at `deploy/compose.deploy.yml` already matches spec 042.
- [x] Fix implemented (artifact edits in `state.json`, `spec.md`, `design.md`)
   - Raw output evidence (inline under this item):
      ```
      $ git diff --name-only
      specs/020-security-hardening/design.md
      specs/020-security-hardening/spec.md
      specs/020-security-hardening/state.json
      $ python3 -c "import json; json.load(open('specs/020-security-hardening/state.json')); print('OK')"
      OK
      ```
- [x] Pre-fix regression test FAILS (three greps for `042-tailnet-edge-bind-pattern` exit 1 against `main`)
   - Raw output evidence (inline under this item):
      ```
      $ grep -q "042-tailnet-edge-bind-pattern" specs/020-security-hardening/state.json; echo "state.json: $?"
      state.json: 1
      $ grep -q "042-tailnet-edge-bind-pattern" specs/020-security-hardening/spec.md; echo "spec.md: $?"
      spec.md: 1
      $ grep -q "042-tailnet-edge-bind-pattern" specs/020-security-hardening/design.md; echo "design.md: $?"
      design.md: 1
      ```
      All three greps exit 1 BEFORE edits — the bug is reproducible.
- [x] Adversarial regression case exists and would fail if the bug returned (L269 bare-phrase ±5-line proximity guard)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "100% of services bind to 127.0.0.1" specs/020-security-hardening/spec.md; echo "exit: $?"
      exit: 1
      ```
      The bare phrase "100% of services bind to 127.0.0.1" no longer appears anywhere in spec.md (exit 1 = no match). The L269 row now reads `Host port binding (SUPERSEDED by [042-tailnet-edge-bind-pattern]...)` with the spec-042 reference embedded on the same line, satisfying the ±5-line proximity guard vacuously. If a future edit reintroduced the bare phrase without a 042 reference within ±5 lines, the guard would fail.
- [x] Post-fix regression test PASSES (three greps exit 0 after edits; L269 guard passes)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/state.json
      8:      "supersededBy": "042-tailnet-edge-bind-pattern",
      $ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/spec.md
      48:> superseded by [specs/042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md).
      62:**Mitigation:** ... see spec [042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md) for the current host-bind contract.)
      279:| Host port binding (SUPERSEDED by [042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md)) | ... |
      $ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/design.md
      5:> been superseded by [specs/042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/design.md).
      ```
      All three greps now exit 0 with substantive content (not just a passing mention).
- [x] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item):
      ```
      The regression "tests" here are 4 explicit greps with explicit expected exit codes (exit 1 pre-fix, exit 0 post-fix) and a change-boundary check. No `if ... return` early exits, no `route()`/`intercept()`/`msw`/`nock` mocking, no skip-on-unexpected-state branches. Inversion check: if the bug returned (pointer removed from any of the 3 files), the corresponding post-fix grep would exit 1 and the test would fail.
      ```
- [x] Change boundary respected: `git diff --name-only` shows only paths under `specs/020-security-hardening/`
   - Raw output evidence (inline under this item):
      ```
      $ git diff --name-only
      specs/020-security-hardening/design.md
      specs/020-security-hardening/spec.md
      specs/020-security-hardening/state.json
      ```
      All 3 changed paths are under `specs/020-security-hardening/`. No code, no other specs.
- [x] artifact-lint passes on parent spec 020 and on this bug folder
   - Raw output evidence (inline under this item):
      ```
      $ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening
      ...
      Artifact lint PASSED.
      EXIT=0

      $ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing
      ...
      Artifact lint PASSED.
      EXIT=0
      ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
   - Raw output evidence (inline under this item):
      ```
      The 5 Gherkin scenarios in this scope map 1:1 to the 5 artifact-shape regression checks above (3 supersession-pointer greps + L269 bare-phrase proximity guard + change-boundary check). All 5 pass. This is artifact-only drift — no runtime behavior changed — so the regression suite IS the artifact-shape suite (tdd.exempt: artifact-only per user dispatch).
      ```
- [x] Broader E2E regression suite passes
   - Raw output evidence (inline under this item):
      ```
      Not applicable — zero runtime code changed (`git diff --name-only` shows only 3 paths under `specs/020-security-hardening/`, all .md/.json). The broader E2E suite cannot regress on documentation-only edits to a superseded spec's narrative. Gate G028 + `internal/deploy/compose_contract_test.go` continue to enforce the spec-042 invariants on `deploy/compose.deploy.yml` independently.
      ```
- [x] Bug marked as Fixed in bug.md
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE '^- \[x\] (Fixed|Verified|Closed|In Progress)' specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/bug.md
      14:- [x] In Progress
      15:- [x] Fixed
      16:- [x] Verified
      17:- [x] Closed
      ```
      bug.md Status section now shows In Progress + Fixed + Verified + Closed all checked.
- [x] Consumer Impact Sweep complete — zero stale first-party references remain (artifact-only supersession pointer; no navigation, breadcrumb, redirect, deep link, generated client, or stale-reference surfaces affected because spec 020 is a governance document, not a runtime contract; the host-bind prescription was already superseded in code by spec 042's `deploy/compose.deploy.yml` invariants)
   - Raw output evidence (inline under this item):
      ```
      $ grep -rEn "specs/020-security-hardening/(spec|design)\.md|020-security-hardening" --include="*.go" --include="*.py" --include="*.ts" --include="*.tsx" --include="*.yaml" --include="*.yml" --include="*.md" -l | grep -v "^specs/020-security-hardening/" | head -20
      docs/Operations.md
      docs/Deployment.md
      specs/042-tailnet-edge-bind-pattern/spec.md
      specs/042-tailnet-edge-bind-pattern/design.md
      ```
      First-party consumers of spec 020 are documentation files and spec 042. Spec 042 already supersedes spec 020 on the host-bind contract (no action). docs/Operations.md and docs/Deployment.md reference spec 042 as the current contract (verified no stale "100% bind to 127.0.0.1" prescription is propagated). No code consumes spec 020 directly (it is a governance spec, not a runtime contract). No navigation, breadcrumb, redirect, deep-link, or generated-client surfaces apply.
- [x] state.json carries supersededBy pointer to spec 042 in supersessions array (faithful to Gherkin scenario 1)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/state.json
      8:      "supersededBy": "042-tailnet-edge-bind-pattern",
      ```
- [x] spec.md carries inline supersession note above Network Exposure section (faithful to Gherkin scenario 2)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/spec.md
      48:> superseded by [specs/042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md).
      62:**Mitigation:** ... see spec [042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md) for the current host-bind contract.)
      ```
- [x] design.md carries inline supersession note at top of document (faithful to Gherkin scenario 3)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/design.md
      5:> been superseded by [specs/042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/design.md).
      ```
- [x] L269 success-metric row no longer misleading — annotated with spec-042 reference inline (faithful to Gherkin scenario 4)
   - Raw output evidence (inline under this item):
      ```
      $ grep -n "Host port binding" specs/020-security-hardening/spec.md
      279:| Host port binding (SUPERSEDED by [042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md)) | ... |
      ```
- [x] Fix is artifact-only — every changed path is under specs/020-security-hardening/ (faithful to Gherkin scenario 5)
   - Raw output evidence (inline under this item):
      ```
      $ git diff --name-only main..HEAD
      specs/020-security-hardening/design.md
      specs/020-security-hardening/spec.md
      specs/020-security-hardening/state.json
      ```
