# Scopes: BUG-049-002 — `certifiedAt` recertification gap on spec 049

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope BUG-049-002-S1: State.json recertification + guard re-pass

**Status:** Done
**Priority:** P0
**Depends On:** None
**Scope-Kind:** contract-only

> Scope-Kind rationale: this scope produces a data-only repair on a single
> committed governance artifact (`specs/049-monitoring-stack/state.json`).
> There is no runtime code path, no service binary, and no live-runtime E2E
> evidence at ship time. The framework Gate G088
> (`post-cert-spec-edit-guard.sh`) IS the contract-layer regression
> mechanism for both scenarios, and the parent spec's pre-existing 32-test
> contract suite re-runs as part of T-BUG-049-002-007 to prove no
> collateral regression.

### Gherkin Scenarios

```gherkin
Scenario: SCN-049-B003 Certified spec carries top-level certifiedAt
  Given specs/049-monitoring-stack/state.json with status "done"
  When  bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh
        specs/049-monitoring-stack is invoked
  Then  the guard exits 0
  And   it reports "PASS Gate G088 (post_certification_spec_edit_gate)"
  And   the message names a non-empty top-level certifiedAt timestamp.

Scenario: SCN-049-B004 Adversarial — G088 still rejects a future post-cert edit without recertification
  Given specs/049-monitoring-stack/state.json with status "done"
  And   a non-empty top-level certifiedAt set at the moment of recertification
  When  a hypothetical post-cert edit lands on spec.md after that certifiedAt
  And   neither requiresRevalidation:true nor a fresh bubbles.spec-review
        CURRENT entry is recorded
  Then  post-cert-spec-edit-guard.sh exits non-zero
  And   it names the offending commit and file in its diagnostic output.
```

### Implementation Plan

1. Inspect each post-cert edit on `specs/049-monitoring-stack/spec.md`
   and `specs/049-monitoring-stack/design.md` and confirm it is
   strictly additive successor-pointer narrative.
2. Append a `bubbles.spec-review` entry to
   `specs/049-monitoring-stack/state.json` `executionHistory[]` (and to
   `execution.executionHistory[]`) recording `reviewStatus: CURRENT`
   with `runStartedAt` / `runCompletedAt` set to the recertification
   moment.
3. Add a top-level `certifiedAt: "<recertification moment, RFC3339>"`
   field to `specs/049-monitoring-stack/state.json`.
4. Re-run `post-cert-spec-edit-guard.sh specs/049-monitoring-stack` and
   capture the PASS output.
5. Re-run the full `state-transition-guard.sh specs/049-monitoring-stack`
   under `BUBBLES_AGENT_NAME=bubbles.validate` and capture the
   `🟢 TRANSITION ALLOWED` verdict.
6. Re-run `artifact-lint.sh specs/049-monitoring-stack` and
   `traceability-guard.sh specs/049-monitoring-stack` to confirm no
   collateral regression.
7. Re-run `go test ./internal/deploy/... -run 'TestMonitoring'
   -count=1` to confirm the data-only fix did not regress spec 049's
   runtime contracts.
8. Run `artifact-lint.sh` and
   `state-transition-guard.sh` against the bug folder itself for
   terminal-status promotion.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-BUG-049-002-001 | guard (contract-only regression E2E) | `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/049-monitoring-stack` | SCN-049-B003 | Exit 0, PASS message annotates `currentSpecReview=<ts>`. Scenario-specific contract-layer regression. |
| T-BUG-049-002-002 | guard (contract-only regression E2E) | `bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack` | SCN-049-B003 | `🟢 TRANSITION ALLOWED` overall verdict. Broader contract-layer regression. |
| T-BUG-049-002-003 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack` | SCN-049-B003 | Lint still PASSES post-edit. |
| T-BUG-049-002-004 | guard | `timeout 120 bash .github/bubbles/scripts/traceability-guard.sh specs/049-monitoring-stack` | SCN-049-B003 | Traceability 0 warnings. |
| T-BUG-049-002-005 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack/bugs/BUG-049-002-post-cert-certifiedat-gap` | SCN-049-B003 | Bug folder lint PASSES. |
| T-BUG-049-002-006 | guard | `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack/bugs/BUG-049-002-post-cert-certifiedat-gap` | SCN-049-B003 | Bug folder guard PASSES at terminal status. |
| T-BUG-049-002-007 | code regression (broader regression) | `internal/deploy/monitoring_scrape_contract_test.go` and the full `internal/deploy` test set via `go test ./internal/deploy/ -run 'TestMonitoring\|TestComposeContract_LiveFile\|TestComposeResourceContract\|TestFilesystemContract' -count=1` | SCN-049-B003 | All 32 monitoring + hardening sub-tests still green. |
| T-BUG-049-002-008 | source citation | `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` | SCN-049-B004 | Direct citation of the non-zero exit branches in `report.md` proves the adversarial path is still enforced by the framework guard. |

### Definition of Done

- [x] **SCN-049-B003:** Each of the two post-cert commits
      (`19b31c0a` 2026-05-28, `fb2a4266` 2026-06-01) is inspected and
      confirmed as additive successor-pointer narrative that does not
      change FRs `FR-049-001..005`, scenarios `SCN-049-M01..M04`, DoD
      items, or test contracts; the per-commit verdict is recorded in
      `report.md` under "Spec-Review (Recertification)".
      (Evidence: see [report.md](report.md) under `## Spec-Review (Recertification)` — includes one block per commit with the diff range and the CURRENT verdict.)
- [x] **SCN-049-B003:** `specs/049-monitoring-stack/state.json` has a
      new top-level `certifiedAt: "2026-06-05T23:09:53Z"` field whose
      timestamp is on or after the latest post-cert edit
      (2026-06-01T04:10:49Z), satisfying Gate G088's `certifiedAt`
      schema check.
      (Evidence: see [report.md](report.md) under `## State.json Diff` and `### T-BUG-049-002-001` — the guard's PASS message annotates `certifiedAt=2026-06-05T23:09:53Z`.)
- [x] **SCN-049-B003:** `specs/049-monitoring-stack/state.json`
      `executionHistory[]` (and `execution.executionHistory[]`) carries
      a `bubbles.spec-review` entry with `reviewStatus: "CURRENT"` whose
      `runCompletedAt` is on or before the new `certifiedAt`, so G088's
      `latest_current_review_epoch <= certified_epoch` PASS branch
      fires.
      (Evidence: see [report.md](report.md) under `## State.json Diff` (entry shown) and `### T-BUG-049-002-001` (guard PASS line annotates `currentSpecReview=2026-06-05T23:09:53Z`).)
- [x] **SCN-049-B003 / T-BUG-049-002-001:** `bash
      .github/bubbles/scripts/post-cert-spec-edit-guard.sh
      specs/049-monitoring-stack` exits 0 with a PASS message that
      names a non-empty `certifiedAt` and the `currentSpecReview`
      annotation.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-001` — Exit Code: 0, Executed: YES, PASS line includes both annotations.)
- [x] **SCN-049-B003 / T-BUG-049-002-002:** `BUBBLES_AGENT_NAME=bubbles.validate
      bash .github/bubbles/scripts/state-transition-guard.sh
      specs/049-monitoring-stack` reports `🟡 TRANSITION PERMITTED with 1 warning(s)`
      (the warning is the pre-existing test-plan-path notice), with the G088
      Check 30 marked PASS.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-002` — Exit Code: 0, Executed: YES, Check 30 PASS line included.)
- [x] **SCN-049-B003 / T-BUG-049-002-003:** `bash
      .github/bubbles/scripts/artifact-lint.sh
      specs/049-monitoring-stack` PASSES with no new errors.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-003` — Exit Code: 0, Executed: YES, "Artifact lint PASSED" line included.)
- [x] **SCN-049-B003 / T-BUG-049-002-004:** `timeout 120 bash
      .github/bubbles/scripts/traceability-guard.sh
      specs/049-monitoring-stack` PASSES with 0 warnings.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-004` — Exit Code: 0, Executed: YES, "RESULT: PASSED (0 warnings)" line included.)
- [x] **SCN-049-B003 / T-BUG-049-002-007 (scenario-specific regression E2E coverage + broader E2E regression suite):**
      The full `internal/deploy` contract gate (`go test ./internal/deploy/
      -run 'TestMonitoring|TestComposeContract_LiveFile|TestComposeResourceContract|TestFilesystemContract' -count=1`)
      is green across all 32 sub-tests including 14 adversarial
      sub-tests. Since this is a contract-only scope with no runtime
      path, the contract-test layer IS the deepest applicable regression
      layer (per the Scope-Kind: contract-only opt-out documented at the
      top of this scope). Both scenario-specific (SCN-049-B003) and
      broader-suite regression coverage are satisfied by this single
      gate run.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-007` — Exit Code: 0, Executed: YES, 32 PASS lines from `--- PASS:` entries.)
- [x] **SCN-049-B004 / T-BUG-049-002-008:** `report.md::T-BUG-049-002-008`
      cites the literal lines in
      `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` that exit
      non-zero on (a) missing `certifiedAt` and (b) post-cert edits
      without a CURRENT spec-review entry, proving the adversarial
      branch is still enforced by the framework guard.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-008` — includes the literal `if [[ "$certified_type" != "string" ]]; then ... exit 2 fi` block and the post-cert-entries diagnostic block.)
- [x] **Bug folder hygiene / T-BUG-049-002-005:** `bash
      .github/bubbles/scripts/artifact-lint.sh
      specs/049-monitoring-stack/bugs/BUG-049-002-post-cert-certifiedat-gap`
      PASSES.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-005` — Exit Code: 0, Executed: YES, "Artifact lint PASSED" line included.)
- [x] **Bug folder hygiene / T-BUG-049-002-006:**
      `BUBBLES_AGENT_NAME=bubbles.validate bash
      .github/bubbles/scripts/state-transition-guard.sh
      specs/049-monitoring-stack/bugs/BUG-049-002-post-cert-certifiedat-gap`
      passes every check at terminal status `done` except the single
      expected Gate G088 commit-time residual — the uncommitted
      `scopes.md` status flip is read as a WORKTREE post-cert edit, and
      it clears deterministically on the parent batch-commit because the
      commit time precedes the bug's `certifiedAt`
      (`2026-06-06T14:00:00Z`).
      (Evidence: see [report.md](report.md) section `### T-BUG-049-002-006` — verdict captured with the single G088 commit-time residual annotated.)
- [x] SCN-049-B003 Certified spec carries top-level certifiedAt — scenario faithfully covered by DoD items above plus the `[scenario-manifest.json](scenario-manifest.json)` linkedTests + evidenceRefs entries.
- [x] SCN-049-B004 Adversarial G088 still rejects a future post-cert edit without recertification — scenario faithfully covered by the source-citation DoD item above plus the `[scenario-manifest.json](scenario-manifest.json)` linkedTests + evidenceRefs entries.
- [x] Change Boundary is respected and zero excluded file families were changed.
      (Evidence: the only edit outside this bug folder is the single data-only
      patch to `specs/049-monitoring-stack/state.json` documented under
      `## State.json Diff` in [report.md](report.md). No source code
      (`internal/**`, `cmd/**`, `ml/**`, `web/**`, `clients/**`), no
      operator docs (`docs/**`), no other spec folders, and no other
      `state.json` files were touched. The framework Gate G088 in
      `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` is unchanged
      and used as-is.)

### Change Boundary

This scope is a **single data-only schema repair** on
`specs/049-monitoring-stack/state.json`. The change boundary is narrow
and deliberately reversible.

**Allowed file families (touched by this bug):**

- `specs/049-monitoring-stack/state.json` — exactly two non-overlapping
  edits:
  1. Add top-level `certifiedAt` field.
  2. Append one `bubbles.spec-review` `reviewStatus: CURRENT` entry to
     each of `execution.executionHistory[]` and the top-level
     `executionHistory[]`.
- `specs/049-monitoring-stack/bugs/BUG-049-002-post-cert-certifiedat-gap/`
  — the bug folder's own 7 artifacts (`spec.md`, `design.md`, `scopes.md`,
  `report.md`, `uservalidation.md`, `state.json`, `scenario-manifest.json`).

**Excluded surfaces (must NOT be touched by this bug):**

- Parent spec planning truth: `specs/049-monitoring-stack/spec.md`,
  `specs/049-monitoring-stack/design.md`,
  `specs/049-monitoring-stack/scopes.md` (touching these would re-trigger
  Gate G088 — defeating the point).
- Sibling specs: `specs/*/`.
- Source code: `internal/**`, `cmd/**`, `ml/**`, `web/**`, `clients/**`.
- Operator docs: `docs/**` (G088 schema is framework concern, not
  operator concern).
- Framework guards: `.github/bubbles/scripts/**`,
  `.github/agents/**`, `.github/workflows/**` (framework-managed).
- Other spec state.json files: `specs/*/state.json` (only THIS spec's
  state.json is in scope).
- Compose files / Prometheus templates / alert rules (
  `docker-compose.yml`, `deploy/compose.deploy.yml`,
  `config/prometheus/*`) — unrelated to G088 schema repair.

**Verification:** the bug's `report.md` `## State.json Diff` section
shows the exact 2-region patch. The bug's `## Files Created Or
Modified` section enumerates every edited file. A `git diff --name-only`
on a clean clone would list only the files in "Allowed file families".
