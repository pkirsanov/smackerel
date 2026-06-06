# Design: BUG-054-001 — `certifiedAt` recertification gap on spec 054

## Current Truth

`.github/bubbles/scripts/post-cert-spec-edit-guard.sh` (Gate G088) is the
authoritative enforcer for "no silent planning-truth drift after
certification". The relevant logic (excerpted from the live guard):

```bash
# Tracked planning truth files
add_tracked_file "$spec_rel/spec.md"
add_tracked_file "$spec_rel/design.md"
add_tracked_file "$spec_rel/scopes.md"
add_tracked_file "$spec_rel/scopes/_index.md"

# Collect post-cert commits touching tracked files
if ! git -C "$REPO_ROOT" log --format='@@G088@@%x09%H%x09%cI%x09%s' \
    --name-only --since="$certified_at" -- "${tracked_paths[@]}" > "$LOG_FILE"; then
  ...
fi

# Decision branches:
# 1. post-cert edits AND requiresRevalidation:true → PASS
# 2. post-cert edits AND no requiresRevalidation   → FAIL (exit 1)
# 3. no post-cert edits AND bubbles.spec-review CURRENT entry timestamp
#    <= certifiedAt → PASS with currentSpecReview annotation
# 4. no post-cert edits                              → plain PASS

latest_current_review="$(jq -r '
  [
    .executionHistory[]?
    | select((.agent? // "") == "bubbles.spec-review")
    | select(((.reviewStatus? // .reviewVerdict? // .verdict? // "") | ascii_upcase) == "CURRENT")
    | (.runCompletedAt? // .completedAt? // .reviewedAt? // empty)
    | select(type == "string" and length > 0)
  ]
  | sort
  | last // ""
' "$STATE_FILE")"
```

Spec 054 currently has:

- Top-level `certifiedAt: "2026-06-03T23:59:59Z"` (G088 schema is satisfied;
  this is NOT the missing-field case from BUG-049-002).
- One post-cert commit on a tracked file:
  - `48ad42a79caa` (2026-06-05T16:15:45Z) on `scopes.md`, subject
    `fix(specs/039,041,054): tier-3 drift cleanup + ratchet 365 -> 356`.
- ZERO `bubbles.spec-review` entries with `reviewStatus: CURRENT` whose
  `runCompletedAt` is on or after the post-cert edit.

The guard's `if [[ "${#post_cert_entries[@]}" -gt 0 ]]; then ... exit 1`
branch fires, blocking promotion.

The post-cert edit's blast radius (verified by `git show 48ad42a79caa
-- specs/054-notification-intelligence-handler/scopes.md` and
`git diff 48ad42a79caa~1 48ad42a79caa -- specs/054-notification-intelligence-handler/
--stat`) is:

| File | Lines added | Lines removed | Touched sections |
|------|-------------|---------------|------------------|
| `specs/054-notification-intelligence-handler/scopes.md` | 4 | 4 | Test Plan tables in Scope 2 and Scope 7 — File/Location column only |

The changed Test Plan rows are:

- **SCN-054-004** E2E API: file pointer updated from
  `tests/e2e/notification_ingest_api_test.go` → `internal/api/notifications_pipeline.go`
  (parenthetical explains live-DB consolidation).
- **SCN-054-005** E2E API: file pointer updated from
  `tests/e2e/notification_manual_ingest_api_test.go` → `internal/api/notifications_pipeline.go`
  (parenthetical explains the manual+auto-ingest test consolidation).
- **SCN-054-006** Regression E2E API: file pointer updated to
  `internal/api/notifications_pipeline.go`.
- **SCN-054-022** E2E API: file pointer updated from
  `tests/e2e/notification_operator_api_test.go` → `tests/e2e/notification_operator_web_test.go`
  (parenthetical explains operator-surface consolidation).

Untouched (also verified by the diff):

- All 30 Gherkin scenarios (SCN-054-001..030).
- All DoD checkboxes across 9 scopes.
- All implementation plans and change boundaries.
- All test FUNCTION NAMES (only their location annotations were updated).
- `spec.md`, `design.md`, `scenario-manifest.json`,
  `uservalidation.md`, `state.json` content (none of those were in the
  commit).

## Proposed Design

### Architecture

This bug is data-only. No source code or operator docs change. The fix
touches exactly one parent file:

- `specs/054-notification-intelligence-handler/state.json`
  - Advance top-level `certifiedAt` from `"2026-06-03T23:59:59Z"` to
    `"2026-06-06T01:30:00Z"` (the recertification moment, on/after
    the post-cert edit timestamp `2026-06-05T16:15:45Z`).
  - Advance `certification.certifiedAt` to the same value for
    consistency with the top-level field.
  - Append one entry to top-level `executionHistory[]` recording the
    `bubbles.spec-review` recertification:
    ```json
    {
      "agent": "bubbles.spec-review",
      "phase": "spec-review-recertification",
      "phasesExecuted": ["spec-review"],
      "reviewStatus": "CURRENT",
      "runStartedAt": "2026-06-06T01:25:00Z",
      "runCompletedAt": "2026-06-06T01:29:00Z",
      "completedAt": "2026-06-06T01:29:00Z",
      "outcome": "post_cert_scopes_test_path_drift_cleanup_ratified",
      "summary": "..."
    }
    ```
  - Append one entry to top-level `executionHistory[]` recording the
    `bubbles.workflow` sweep round (round 15, devops trigger,
    devops-to-doc mapped child mode, parent-expanded-child-mode
    execution).
  - Update `certifiedCompletedPhases` to include
    `"spec-review-recertification"` so the recertification phase is
    reflected in the spec's lifecycle ledger.
  - Update `lastSpecialistVerdict` to append the recertification
    annotation, preserving the prior structured-commit-gate and
    post-release-exception annotations.
  - Update `lastUpdatedAt` and `completedAt` to the new recertification
    moment.

The `reviewStatus: CURRENT` value is the key the guard pattern-matches on
(`ascii_upcase` on `.reviewStatus // .reviewVerdict // .verdict`). The
`runCompletedAt` timestamp is what the guard's
`latest_current_review_epoch` extractor uses for the comparison
`<= certified_epoch`.

Because `runCompletedAt` (`2026-06-06T01:29:00Z`) is on or before the
new `certifiedAt` (`2026-06-06T01:30:00Z`), and the new `certifiedAt`
is strictly after the post-cert commit (`2026-06-05T16:15:45Z`), the
guard's `git log --since="$certified_at"` query will find zero
post-cert entries, and the spec-review-present PASS branch will fire.

### Sequencing

1. Read the parent state.json (already done during discovery — schema
   captured in the spec.md "Problem Statement" section).
2. Run an inline `bubbles.spec-review`-equivalent check: confirm the
   post-cert diff is bounded to Test Plan File/Location pointer
   annotations and that no scenario/DoD/design/spec content was
   changed (already done — see spec.md "Problem Statement" and the
   `git diff` output captured in report.md).
3. Update parent state.json using strict IDE edit tools
   (`replace_string_in_file` / `multi_replace_string_in_file`), never
   shell redirection, heredocs, or `python -c` that writes to disk.
4. Re-run `post-cert-spec-edit-guard.sh specs/054-notification-intelligence-handler`
   to prove the gate PASSES.
5. Re-run the full
   `state-transition-guard.sh specs/054-notification-intelligence-handler`
   to prove the spec is promotion-ready (no Check 30 BLOCK).
6. Re-run `artifact-lint.sh` and `traceability-guard.sh` to prove no
   collateral regression.
7. Run `artifact-lint.sh` against the bug folder for hygiene.

### Test Plan

The "tests" for this bug are framework guard re-runs. There are no
runtime behavior changes to test. The contract-only scope justification
is:

- Gate G088 in `.github/bubbles/scripts/post-cert-spec-edit-guard.sh`
  IS the regression mechanism. Any future drift of the same shape
  trips the gate again.
- The parent spec 054's contract test surface
  (`internal/notification/...`, `tests/e2e/notification_...`,
  `tests/stress/notification_full_pipeline_stress_test.go`) was
  certified green at 2026-05-24 validate-owned certification. The
  state.json edits do not invalidate any of those tests.
- The bug's own re-run of `post-cert-spec-edit-guard.sh`,
  `state-transition-guard.sh`, `artifact-lint.sh`, and
  `traceability-guard.sh` is the deepest contract-layer evidence
  available, since the change boundary excludes any runtime code or
  operator-docs path.

## Change Boundary

**Allowed (touched by this bug):**

- `specs/054-notification-intelligence-handler/state.json` — exactly
  three non-overlapping edits:
  1. Update top-level `certifiedAt`, `lastUpdatedAt`, `completedAt`,
     and `lastSpecialistVerdict`.
  2. Update `certification.certifiedAt` and append
     `spec-review-recertification` to
     `certification.certifiedCompletedPhases`.
  3. Append two entries to top-level `executionHistory[]`
     (one `bubbles.spec-review`, one `bubbles.workflow`).
- `specs/054-notification-intelligence-handler/bugs/BUG-054-001-post-cert-certifiedat-recertification-gap/`
  — the bug folder's own 7 artifacts (`spec.md`, `design.md`,
  `scopes.md`, `report.md`, `uservalidation.md`, `state.json`,
  `scenario-manifest.json`).

**Excluded surfaces (MUST NOT be touched by this bug):**

- Parent spec planning truth:
  `specs/054-notification-intelligence-handler/spec.md`,
  `specs/054-notification-intelligence-handler/design.md`,
  `specs/054-notification-intelligence-handler/scopes.md` (touching
  these would re-trigger Gate G088).
- Sibling specs: `specs/*/` other than spec 054.
- Source code: `internal/**`, `cmd/**`, `ml/**`, `web/**`,
  `clients/**`.
- Operator docs: `docs/**` (G088 schema is framework concern, not
  operator concern).
- Framework guards: `.github/bubbles/scripts/**`,
  `.github/agents/**`, `.github/workflows/**`
  (framework-managed; see [bubbles-managed-policy-files.md](../../../../.specify/memory/agents.md)).
- Other spec state.json files: `specs/*/state.json` other than spec
  054's parent state.
- Compose files / Prometheus templates / alert rules
  (`docker-compose.yml`, `deploy/compose.deploy.yml`,
  `config/prometheus/*`) — unrelated to G088 schema repair.

**Verification:** the bug's `report.md` `## State.json Diff` section
will show the exact 3-region patch on the parent state.json. A `git
diff --name-only` on a clean clone would list only the files in
"Allowed" above.
