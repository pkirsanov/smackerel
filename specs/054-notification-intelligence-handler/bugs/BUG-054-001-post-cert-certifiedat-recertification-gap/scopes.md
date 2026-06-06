# Scopes: BUG-054-001 — `certifiedAt` recertification gap on spec 054

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope BUG-054-001-S1: State.json recertification + guard re-pass

**Status:** Done
**Priority:** P0
**Depends On:** None
**Scope-Kind:** contract-only

> Scope-Kind rationale: this scope produces a data-only repair on a
> single committed governance artifact
> (`specs/054-notification-intelligence-handler/state.json`).
> There is no runtime code path, no service binary, and no live-runtime
> E2E evidence at ship time. The framework Gate G088
> (`post-cert-spec-edit-guard.sh`) IS the contract-layer regression
> mechanism for both scenarios, and the parent spec's already-green
> contract suite (certified 2026-05-24, re-confirmed 2026-06-03)
> re-runs on every `./smackerel.sh test unit` invocation as the
> ongoing regression safety net.

### Gherkin Scenarios

```gherkin
Scenario: SCN-054-B001 Spec 054 G088 PASSES after recertification
  Given specs/054-notification-intelligence-handler/state.json with status "done"
  And   a top-level certifiedAt timestamp on or after the latest post-cert
        edit on scopes.md (2026-06-05T16:15:45Z)
  And   a bubbles.spec-review executionHistory entry with reviewStatus "CURRENT"
        whose runCompletedAt is on or before that certifiedAt
  When  bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh
        specs/054-notification-intelligence-handler is invoked
  Then  the guard exits 0
  And   it reports "PASS Gate G088 (post_certification_spec_edit_gate)"
  And   the message annotates certifiedAt and currentSpecReview with the
        new recertification timestamp.

Scenario: SCN-054-B002 Adversarial — a future post-cert planning-truth edit still trips G088
  Given specs/054-notification-intelligence-handler/state.json with status "done"
  And   the new certifiedAt set at the moment of this recertification
  When  a hypothetical post-cert edit lands on spec.md, design.md, or scopes.md
        after that certifiedAt
  And   neither requiresRevalidation:true nor a fresh bubbles.spec-review
        CURRENT entry is recorded
  Then  post-cert-spec-edit-guard.sh exits non-zero
  And   it names the offending commit, file, and subject in its diagnostic
        output.
```

### Implementation Plan

1. Inspect the post-cert commit `48ad42a79caa` on
   `specs/054-notification-intelligence-handler/scopes.md` and confirm
   it is strictly planning-text File/Location pointer drift in 4 Test
   Plan rows (SCN-054-004, SCN-054-005, SCN-054-006, SCN-054-022).
2. Append a `bubbles.spec-review` entry to
   `specs/054-notification-intelligence-handler/state.json` top-level
   `executionHistory[]` recording `reviewStatus: CURRENT` with
   `runStartedAt` and `runCompletedAt` set to the recertification
   moment (2026-06-06T01:25:00Z and 2026-06-06T01:29:00Z respectively).
3. Append a `bubbles.workflow` sweep-round entry to top-level
   `executionHistory[]` documenting round 15 / devops trigger /
   devops-to-doc mapped child mode / parent-expanded-child-mode
   execution.
4. Update top-level `certifiedAt`, `lastUpdatedAt`, `completedAt`,
   and `lastSpecialistVerdict` to reflect the recertification.
5. Update `certification.certifiedAt` and append
   `spec-review-recertification` to `certification.certifiedCompletedPhases`.
6. Re-run `post-cert-spec-edit-guard.sh
   specs/054-notification-intelligence-handler` and capture the PASS
   output (Exit 0, annotates certifiedAt and currentSpecReview).
7. Re-run the full `state-transition-guard.sh
   specs/054-notification-intelligence-handler` and capture the
   no-BLOCK verdict (Exit 0).
8. Re-run `artifact-lint.sh specs/054-notification-intelligence-handler`
   and `traceability-guard.sh specs/054-notification-intelligence-handler`
   to confirm no collateral regression.
9. Run `artifact-lint.sh` against the bug folder itself for hygiene.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-BUG-054-001-001 | guard (contract-only regression E2E) | `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/054-notification-intelligence-handler` | SCN-054-B001 | Exit 0; PASS message annotates new `certifiedAt` and `currentSpecReview` timestamps. Scenario-specific contract-layer regression. |
| T-BUG-054-001-002 | guard (broader contract regression) | `bash .github/bubbles/scripts/state-transition-guard.sh specs/054-notification-intelligence-handler` | SCN-054-B001 | Exit 0; Check 30 (Gate G088) prints PASS line; overall verdict TRANSITION ALLOWED. |
| T-BUG-054-001-003 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler` | SCN-054-B001 | Exit 0; "Artifact lint PASSED" line. |
| T-BUG-054-001-004 | guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/054-notification-intelligence-handler` | SCN-054-B001 | Exit 0; "RESULT: PASSED (0 warnings)" line. |
| T-BUG-054-001-005 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler/bugs/BUG-054-001-post-cert-certifiedat-recertification-gap` | SCN-054-B001 | Exit 0; bug folder lint PASSES. |
| T-BUG-054-001-006 | source citation | `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` lines around `post_cert_entries+=` and `exit 1` | SCN-054-B002 | Direct citation in `report.md` of the non-zero exit branch proves the adversarial path is still enforced by the framework guard without trying to mutate planning truth in this bug folder. |

### Definition of Done

- [x] **SCN-054-B001:** The post-cert commit `48ad42a79caa` on
      `scopes.md` is inspected and confirmed as planning-text Test Plan
      File/Location pointer drift across 4 rows (SCN-054-004,
      SCN-054-005, SCN-054-006, SCN-054-022) that does NOT change
      Gherkin scenarios, DoD checkboxes, design.md, spec.md, scenario
      IDs, test function names, or any business logic; the verdict is
      recorded in `report.md` under "Spec-Review (Recertification)".
      (Evidence: see [report.md](report.md) under `## Spec-Review (Recertification)`.)
- [x] **SCN-054-B001:** `specs/054-notification-intelligence-handler/state.json`
      top-level `certifiedAt` is advanced to `"2026-06-06T01:30:00Z"`
      (or any RFC3339 timestamp on or after `2026-06-05T16:15:45Z`,
      whichever the actual edit applies), and the same value is mirrored
      to `certification.certifiedAt`.
      (Evidence: see [report.md](report.md) under `## State.json Diff` and `### T-BUG-054-001-001`.)
- [x] **SCN-054-B001:** `specs/054-notification-intelligence-handler/state.json`
      top-level `executionHistory[]` carries a `bubbles.spec-review`
      entry with `reviewStatus: "CURRENT"` whose `runCompletedAt` is
      on or before the new `certifiedAt`, so G088's
      `latest_current_review_epoch <= certified_epoch` PASS branch can
      fire.
      (Evidence: see [report.md](report.md) under `## State.json Diff` and `### T-BUG-054-001-001`.)
- [x] **SCN-054-B001 / T-BUG-054-001-001:** `bash
      .github/bubbles/scripts/post-cert-spec-edit-guard.sh
      specs/054-notification-intelligence-handler` exits 0 with a PASS
      message that annotates the new `certifiedAt` and the
      `currentSpecReview` timestamp.
      (Evidence: see [report.md](report.md) section `### T-BUG-054-001-001`.)
- [x] **SCN-054-B001 / T-BUG-054-001-002:** `bash
      .github/bubbles/scripts/state-transition-guard.sh
      specs/054-notification-intelligence-handler` exits 0; Check 30
      (Gate G088) prints `PASS: Post-certification spec edit guard
      satisfied`.
      (Evidence: see [report.md](report.md) section `### T-BUG-054-001-002`.)
- [x] **SCN-054-B001 / T-BUG-054-001-003:** `bash
      .github/bubbles/scripts/artifact-lint.sh
      specs/054-notification-intelligence-handler` PASSES.
      (Evidence: see [report.md](report.md) section `### T-BUG-054-001-003`.)
- [x] **SCN-054-B001 / T-BUG-054-001-004:** `timeout 600 bash
      .github/bubbles/scripts/traceability-guard.sh
      specs/054-notification-intelligence-handler` PASSES with 0
      warnings.
      (Evidence: see [report.md](report.md) section `### T-BUG-054-001-004`.)
- [x] **SCN-054-B001 / T-BUG-054-001-005:** `bash
      .github/bubbles/scripts/artifact-lint.sh
      specs/054-notification-intelligence-handler/bugs/BUG-054-001-post-cert-certifiedat-recertification-gap`
      PASSES.
      (Evidence: see [report.md](report.md) section `### T-BUG-054-001-005`.)
- [x] **SCN-054-B002 / T-BUG-054-001-006:** `report.md` cites the
      literal lines in `.github/bubbles/scripts/post-cert-spec-edit-guard.sh`
      that exit non-zero on post-cert edits without a CURRENT
      spec-review entry, proving the adversarial branch is still
      enforced by the framework guard.
      (Evidence: see [report.md](report.md) section `### T-BUG-054-001-006`.)
- [x] SCN-054-B001 Spec 054 G088 PASSES after recertification —
      scenario faithfully covered by DoD items above plus the
      [scenario-manifest.json](scenario-manifest.json) `linkedTests`
      and `evidenceRefs` entries.
- [x] SCN-054-B002 Adversarial G088 still rejects a future post-cert
      edit without recertification — scenario faithfully covered by
      the source-citation DoD item above plus the
      [scenario-manifest.json](scenario-manifest.json) `linkedTests`
      and `evidenceRefs` entries.
- [x] Change Boundary is respected and zero excluded file families were changed.
      (Evidence: the only edit outside this bug folder is the single
      data-only patch to
      `specs/054-notification-intelligence-handler/state.json`
      documented under `## State.json Diff` in [report.md](report.md).
      No source code, no operator docs, no other spec folders, no
      other state.json files, and no compose/Prometheus/alert files
      were touched. The framework Gate G088 in
      `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` is
      unchanged and used as-is.)

### Change Boundary

This scope is a **single data-only schema repair** on
`specs/054-notification-intelligence-handler/state.json`. The change
boundary is narrow and deliberately reversible.

**Allowed file families (touched by this bug):**

- `specs/054-notification-intelligence-handler/state.json` — exactly
  three non-overlapping edits:
  1. Update top-level `certifiedAt`, `lastUpdatedAt`, `completedAt`,
     and `lastSpecialistVerdict`.
  2. Update `certification.certifiedAt` and append
     `"spec-review-recertification"` to
     `certification.certifiedCompletedPhases`.
  3. Append two entries to top-level `executionHistory[]`
     (one `bubbles.spec-review`, one `bubbles.workflow`).
- `specs/054-notification-intelligence-handler/bugs/BUG-054-001-post-cert-certifiedat-recertification-gap/`
  — the bug folder's own 7 artifacts.

**Excluded surfaces (MUST NOT be touched by this bug):**

- Parent spec planning truth:
  `specs/054-notification-intelligence-handler/spec.md`,
  `specs/054-notification-intelligence-handler/design.md`,
  `specs/054-notification-intelligence-handler/scopes.md` (touching
  these would re-trigger Gate G088).
- Sibling specs: `specs/*/` other than spec 054.
- Source code: `internal/**`, `cmd/**`, `ml/**`, `web/**`,
  `clients/**`.
- Operator docs: `docs/**`.
- Framework guards: `.github/bubbles/scripts/**`,
  `.github/agents/**`, `.github/workflows/**`.
- Other spec state.json files: `specs/*/state.json` other than spec
  054's parent state.
- Compose files / Prometheus templates / alert rules.

**Verification:** the bug's `report.md` `## State.json Diff` section
shows the exact 3-region patch. A `git diff --name-only` on a clean
clone would list only the files in "Allowed" above.
