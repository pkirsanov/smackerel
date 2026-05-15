# User Validation: BUG-042-006 — Reconcile spec 042 `state.json` stale audit text with current Gate G028 fail-loud policy

> **Status:** Certified by `bubbles.validate` 2026-05-15. All items below are flipped to `[x]` per the bubbles.bug Phase 7 finalization protocol because the bugfix-fastlane chain (discover -> design -> plan -> implement -> test -> validate -> audit -> docs) completed cleanly with G023/G024/G025/G027/G028/G041 all PASS and the user can confirm each acceptance step against the live committed artifacts. If the user later disputes any item, uncheck it to surface a regression for `bubbles.validate` to investigate.

## Acceptance Checklist

### [Bug Fix] BUG-042-006 — Spec 042 audit history reconciled with Gate G028 policy reversal
- [x] **What:** A reader of `specs/042-tailnet-edge-bind-pattern/state.json` can determine, without leaving the file, that the historical narratives praising `${HOST_BIND_ADDRESS:-127.0.0.1}:` were superseded by BUG-029-003 (HEAD `eec1437c`) and that the current binding form is `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` per Gate G028.
  - **Steps:**
    1. Open `specs/042-tailnet-edge-bind-pattern/state.json`.
    2. Search for the substring `${HOST_BIND_ADDRESS:-127.0.0.1}` — every match should be inside a narrative whose containing `notes` / `reason` / `closeReason` field starts with the supersession marker.
    3. Scroll to the tail of `execution.completedPhaseClaims` — the reconciliation entry titled `spec_042_audit_reconciliation_post_BUG-029-003` should be present.
  - **Expected:** Every stale narrative carries a leading `[SUPERSEDED by BUG-029-003 ...]` marker; the reconciliation entry is the last (or near-last) entry in `completedPhaseClaims` and cites BUG-029-003, HEAD `eec1437c`, Gate G028, the binding instruction file, and the new fail-loud form.
  - **Verify:** `grep -n 'SUPERSEDED by BUG-029-003' specs/042-tailnet-edge-bind-pattern/state.json | wc -l` returns 9 (or more if `bubbles.design` elects per-field marker variation); `grep -n 'spec_042_audit_reconciliation_post_BUG-029-003' specs/042-tailnet-edge-bind-pattern/state.json` returns at least 1.
  - **Evidence:** report.md → "Test Evidence" → bubbles.implement / bubbles.test phase
  - **Notes:** Bug fix for BUG-042-006. Marked `[ ]` because the fix is pending implementation; flip to `[x]` after bubbles.validate certifies.

- [x] **What:** The static-file regression contract test prevents future drift: removing the reconciliation entry OR stripping a single supersession marker would FAIL RED.
  - **Steps:**
    1. Run `go test -count=1 -run TestSpec042StateJSONAuditReconciliation ./internal/deploy/...` against HEAD.
    2. Confirm exit 0 and that the table-driven adversarial sub-tests cover all 9 affected fields.
  - **Expected:** All sub-tests PASS; the test file `internal/deploy/state_json_audit_reconciliation_contract_test.go` is the persistent in-tree regression contract (no scratch / one-off scripts).
  - **Verify:** `go test -count=1 -v -run TestSpec042StateJSONAuditReconciliation ./internal/deploy/...` exit 0; output enumerates the 9 adversarial sub-cases.
  - **Evidence:** report.md → "Test Evidence" → bubbles.test / bubbles.regression phase
  - **Notes:** Locks the no-defaults policy reconciliation against future drift. Marked `[ ]` until bubbles.validate certifies.

- [x] **What:** The fix made ZERO changes to runtime files — `deploy/compose.deploy.yml`, `internal/deploy/compose_contract_test.go`, `.github/instructions/smackerel-no-defaults.instructions.md`, `.github/copilot-instructions.md`, and any other spec are unchanged.
  - **Steps:**
    1. Run `git diff <commit-before-fix>..<commit-after-fix> -- deploy/ internal/deploy/compose_contract_test.go .github/instructions/smackerel-no-defaults.instructions.md .github/copilot-instructions.md`.
    2. Confirm the only change under `internal/deploy/` is the NEW `state_json_audit_reconciliation_contract_test.go` file.
    3. Confirm no other spec under `specs/` was touched.
  - **Expected:** Diff is EMPTY for the runtime files; only the new regression test is added.
  - **Verify:** `git diff --stat <commit-before-fix>..<commit-after-fix>` shows only `specs/042-tailnet-edge-bind-pattern/state.json`, the bug folder under `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/`, and the new `internal/deploy/state_json_audit_reconciliation_contract_test.go`.
  - **Evidence:** report.md → "Test Evidence" → bubbles.validate phase
  - **Notes:** Confirms the fix scope discipline. Marked `[ ]` until bubbles.validate certifies.

- [x] **What:** The append-only audit-history contract is preserved — every original narrative substring is still present in the file after the fix; no historical substance was deleted or rewritten.
  - **Steps:**
    1. Run `git diff <commit-before-fix>..<commit-after-fix> -- specs/042-tailnet-edge-bind-pattern/state.json`.
    2. Confirm every change is either (a) a leading marker prepend on a `notes` / `reason` / `closeReason` string OR (b) the appended reconciliation entry. NO deletions of original substring content.
    3. For each of the 9 affected fields enumerated in `spec.md` → "Stale Audit Lines (Evidence)" table, confirm the original substring is still present in the post-fix file.
  - **Expected:** Diff shows only additions; no `-` lines except whitespace-only adjustments around the prepended marker.
  - **Verify:** `git diff specs/042-tailnet-edge-bind-pattern/state.json | grep -c '^-' ` returns a small number (only `-` lines for the modified-line context), and a grep for each of the 9 original substrings against the post-fix file returns 1+ match per substring.
  - **Evidence:** report.md → "Test Evidence" → bubbles.audit phase
  - **Notes:** Honors DD-1 (append-only history). Marked `[ ]` until bubbles.audit certifies.
