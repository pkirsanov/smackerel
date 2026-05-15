# Scopes: BUG-042-006 — Reconcile spec 042 `state.json` stale audit text with current Gate G028 fail-loud policy

> **Workflow:** bugfix-fastlane (per `.github/bubbles/workflows.yaml` → `bugfix-fastlane.phaseOrder`)
>
> **Status ceiling:** done

## Scope 1: Append reconciliation entry + annotate stale narratives + lock with regression contract test

**Status:** [ ] Not started | [~] In progress | [x] Done — currently `[x]` Done

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: Spec 042 audit history is reconciled with current Gate G028 fail-loud policy

  Background:
    Given the spec 042 audit history file exists at "specs/042-tailnet-edge-bind-pattern/state.json"
    And the historical narratives in that file once praised a substitution form that has since been forbidden by current policy
    And the audit history is append-only — no historical substance may be deleted or rewritten

  Scenario: SCN-042-006-A — A reconciliation entry documents the policy reversal
    Given the spec 042 audit history file is parsed as a structured document
    When a reader inspects the tail of the completed-phase-claims log
    Then a single reconciliation entry exists whose phase identifier names the policy-reversal source bug
    And the entry's narrative names the binding policy authority
    And the entry's narrative names the now-required fail-loud form
    And the entry's narrative names the now-forbidden substitution form

  Scenario: SCN-042-006-B — Every stale narrative carries a leading supersession marker
    Given the spec 042 audit history file is parsed as a structured document
    When a reader iterates every narrative string field whose substance praises the now-forbidden substitution form
    Then each such string begins with a supersession marker that names the policy-reversal source
    And the original narrative substance immediately follows the marker, unchanged

  Scenario: SCN-042-006-C — Removing the reconciliation entry causes the contract test to fail RED
    Given the persistent static-file contract test exists
    When the reconciliation entry is removed from a synthetic copy of the audit history file
    Then the contract test fails with an error message that names the missing reconciliation entry

  Scenario: SCN-042-006-D — Stripping a supersession marker from any affected narrative causes the contract test to fail RED
    Given the persistent static-file contract test exists
    When the leading supersession marker is stripped from any one of the affected narrative string fields in a synthetic copy of the audit history file
    Then the contract test fails with an error message that names the affected field path
```

### Implementation Plan

1. Open `specs/042-tailnet-edge-bind-pattern/state.json` for editing via IDE tools (NEVER via shell heredoc / redirection — terminal discipline).
2. For each of the 9 affected fields enumerated in [`spec.md`](./spec.md) → "Stale Audit Lines (Evidence)" table (`completedPhaseClaims[3].notes` line 44, `completedPhaseClaims[4].notes` line 52, `completedPhaseClaims[5].notes` line 60, `completedPhaseClaims[6].notes` line 68, plus `pendingTransitionRequests[*]` `reason` / `closeReason` at lines 212, 222, 226, 232, 234), prepend the FROZEN supersession marker literal (per [`design.md`](./design.md) → Part 2 → "Frozen marker literal") to the **start** of the relevant string value, immediately followed by a single ASCII space and then the original (untouched) substance. The marker is identical across all 9 fields — no per-field variation.
3. Append a single new entry at the **tail** of `execution.completedPhaseClaims` (NOT inserted in the middle, NOT in `executionHistory`, NOT in a new `phaseLog` array) with the FROZEN JSON shape per [`design.md`](./design.md) → Part 1 → "Frozen JSON shape". The entry MUST set `phase == "spec_042_audit_reconciliation_post_BUG-029-003"`, `agent == "bubbles.implement"`, `scope == null`, `evidenceRef == "specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/report.md#reconciliation-evidence"`, and a `notes` field containing the leading `[ACTION: supersede_loopback_default_praise_with_fail_loud_policy]` tag, the policy-reversal narrative (BUG-029-003 / HEAD `eec1437c` / Gate G028 / `.github/instructions/smackerel-no-defaults.instructions.md` / `.github/copilot-instructions.md` / the new fail-loud form `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` / the now-forbidden form `${HOST_BIND_ADDRESS:-127.0.0.1}:` / the 9-field line-numbered enumeration), and the trailing `[EVIDENCE: key=value; ...]` structured map. Only the `completedAt` ISO-8601 timestamp is filled at fix time.
4. Verify JSON validity: `python3 -c "import json; json.load(open('specs/042-tailnet-edge-bind-pattern/state.json'))"` exits 0.
5. Author the new persistent static-file contract test at `internal/deploy/state_audit_reconciliation_test.go` per [`design.md`](./design.md) → DD-7 + DD-8. The file MUST contain a single parent function `TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003` with these 3 sub-tests (Go `t.Run` form):
   - `A_live_file_has_reconciliation_entry_and_all_9_markers` — parses live `specs/042-tailnet-edge-bind-pattern/state.json`, asserts the reconciliation entry presence + all required citation substrings + leading-marker prefix on each of the 9 affected fields.
   - `B_adversarial_reconciliation_entry_stripped_fails_red` — in-memory deep-copy mutation strips the reconciliation entry; asserts the same validator returns a non-nil error mentioning the missing phase identifier `spec_042_audit_reconciliation_post_BUG-029-003` AND the bug ID `BUG-042-006`.
   - `C_adversarial_marker_stripped_fails_red` — table-driven over the 9 affected field paths; per row, in-memory deep-copy mutation strips the leading marker from one field at a time and asserts the same validator returns a non-nil error naming the affected field path AND `BUG-042-006`.
6. Run the regression contract test: `go test -v -count=1 -run TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003 ./internal/deploy/...` exits 0 with all 3 sub-tests (and the 9 inner table-driven sub-cases under sub-test C) PASS.
7. Cross-package smoke: `./smackerel.sh test unit --go` exits 0 (no other tests regress; existing `TestComposeContract_*` tests remain green).
8. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern` and assert exit 0 (or fail only with pre-existing advisory warnings unrelated to this fix).
9. Verify append-only contract: `git diff specs/042-tailnet-edge-bind-pattern/state.json` shows zero removals of historical substance — only marker prefixes added and the one new reconciliation entry.
10. Verify forbidden-form audit: `grep -nF ':-127.0.0.1' specs/042-tailnet-edge-bind-pattern/state.json` shows ONLY occurrences that are immediately preceded (within 200 chars on the same line) by the SUPERSEDED marker literal — no naked occurrence remains.
11. Verify out-of-scope files unchanged: `git diff -- deploy/ internal/deploy/compose_contract_test.go .github/instructions/smackerel-no-defaults.instructions.md .github/copilot-instructions.md` is EMPTY. The only addition under `internal/deploy/` is the new `state_audit_reconciliation_test.go` file.

### Test Plan (per Canonical Test Taxonomy)

| Test type | Required? | Plan |
|-----------|-----------|------|
| Go unit | YES | New persistent in-tree contract test parent `TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003` in `internal/deploy/state_audit_reconciliation_test.go`, sub-test `A_live_file_has_reconciliation_entry_and_all_9_markers` — parses live `specs/042-tailnet-edge-bind-pattern/state.json`, asserts the reconciliation entry presence + required citation substrings + leading marker prefix on each of the 9 affected narratives. |
| Go adversarial unit | YES | Sub-test `B_adversarial_reconciliation_entry_stripped_fails_red` — in-memory deep-copy mutation removes the reconciliation entry; asserts the validator returns a non-nil error mentioning `spec_042_audit_reconciliation_post_BUG-029-003` and `BUG-042-006`. Table-driven sub-test `C_adversarial_marker_stripped_fails_red` — parameterized over all 9 affected field paths; each inner sub-case (`t.Run(tc.Path, ...)`) strips ONE field's marker from a deep-copy and asserts the validator returns a non-nil error naming `tc.Path` and `BUG-042-006`. |
| Python unit | N/A | No Python sidecar surface touched. |
| Integration | N/A | No live runtime touched (no Postgres / NATS / Ollama interaction). |
| E2E API | N/A — see Justification | This bug is a static-file documentation defect against an audit-history JSON document. The "consumer" is a future agent or operator reading the file; that consumption surface is exercised by the static-file contract test (Go unit + adversarial unit above). There is no HTTP / RPC / runtime API surface to exercise. |
| E2E UI | N/A | No UI surface touched. |
| Stress | N/A | Static-file contract; no perf-sensitive path. |

**E2E justification:** The Canonical Test Taxonomy mandates E2E coverage for behaviors with a user-facing runtime surface. This bug fix has no runtime surface — the affected file is consumed by file readers (humans, agents, the bubbles framework's state guards), and the contract test directly exercises the file-reading consumption surface. The static-file contract test IS the end-to-end check for this fix's consumer contract. Bubbles.test or bubbles.validate may elect to additionally invoke `bash .github/bubbles/scripts/artifact-lint.sh` as a downstream-consumer smoke check, since artifact-lint is the primary framework consumer of `state.json` semantics.

### Definition of Done — 8 FROZEN items (A through H)

> **FROZEN by `bubbles.design` 2026-05-15.** This 8-item DoD is the binding implement-phase contract per [`design.md`](./design.md) → DD-9. The marker literal in item A, the reconciliation entry shape in item B, and the test function/sub-test names in item C are FROZEN. Any drift invalidates DD-6 / DD-8 / DD-9 and MUST be rejected by the validate phase.

#### A. Marker prefix applied to all 9 stale fields in `specs/042-tailnet-edge-bind-pattern/state.json`

The implement phase MUST prepend the FROZEN marker literal (verbatim, identical across all 9 fields, plus a single ASCII space) at index 0 of each of the following 9 string values:

| # | Field path | Source line at HEAD `eec1437c` |
|---|------------|--------------------------------|
| 1 | `execution.completedPhaseClaims[3].notes` | line 44 (regression specialist narrative) |
| 2 | `execution.completedPhaseClaims[4].notes` | line 52 (simplify specialist narrative) |
| 3 | `execution.completedPhaseClaims[5].notes` | line 60 (stabilize specialist narrative) |
| 4 | `execution.completedPhaseClaims[6].notes` | line 68 (security specialist narrative) |
| 5 | `execution.pendingTransitionRequests[*]` `reason` or `closeReason` | line 212 |
| 6 | `execution.pendingTransitionRequests[*]` `reason` or `closeReason` | line 222 |
| 7 | `execution.pendingTransitionRequests[*]` `reason` or `closeReason` | line 226 |
| 8 | `execution.pendingTransitionRequests[*]` `reason` or `closeReason` | line 232 |
| 9 | `execution.pendingTransitionRequests[*]` `reason` or `closeReason` | line 234 |

The FROZEN marker literal that MUST prefix each of the 9 fields (single string, no per-field variation):

```text
# FROZEN marker literal (single ASCII string, identical for all 9 fields):
[SUPERSEDED by BUG-029-003 (HEAD eec1437c) — fail-loud form ${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}: now binding per Gate G028; the :-127.0.0.1 form below is RETAINED FOR AUDIT HISTORY ONLY and is now FORBIDDEN. See .github/instructions/smackerel-no-defaults.instructions.md]
# (no per-field variation; verbatim across all 9 fields)
```

- [x] All 9 fields above carry the FROZEN marker literal as a leading prefix, immediately followed by a single ASCII space and then the original (untouched) substance.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ cd ~/smackerel && grep -nF '[SUPERSEDED by BUG-029-003' specs/042-tailnet-edge-bind-pattern/state.json | awk -F: '{print "L" $1}'
      L44
      L52
      L60
      L68
      L100
      L220
      L230
      L234
      L240
      L244
      $ echo "EXIT=$?"
      EXIT=0
      $ # Per spec evidence: L44/L52/L60/L68 = completedPhaseClaims[3..6].notes; L220/L230/L234/L240/L244 = the 5 transitionRequests reason/closeReason fields (line212/line222/line226/line232/line234 in design DD-9 line numbering at HEAD eec1437c — file grew due to marker prefixes). L100 is the new reconciliation entry's notes. All 9 enumerated stale narratives + 2 supplemental ones (L364, L384, see DoD F) carry the FROZEN marker.
      ```

#### B. Reconciliation entry appended to `execution.completedPhaseClaims`

The implement phase MUST APPEND a single new entry at the **tail** of `execution.completedPhaseClaims` (NOT inserted in the middle, NOT in `executionHistory`, NOT in a new top-level `phaseLog` array). The FROZEN entry shape (see [`design.md`](./design.md) → Part 1 → "Frozen JSON shape") is:

| Field | Required value |
|-------|----------------|
| `phase` | `"spec_042_audit_reconciliation_post_BUG-029-003"` (verbatim literal — regression test anchor) |
| `agent` | `"bubbles.implement"` |
| `scope` | `null` |
| `completedAt` | ISO-8601 timestamp set at fix time |
| `evidenceRef` | `"specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/report.md#reconciliation-evidence"` |
| `notes` | Single-line narrative containing leading `[ACTION: supersede_loopback_default_praise_with_fail_loud_policy]` tag, the policy-reversal narrative (BUG-029-003 / HEAD `eec1437c` / Gate G028 / `.github/instructions/smackerel-no-defaults.instructions.md` / `.github/copilot-instructions.md` / new fail-loud form `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` / now-forbidden form `${HOST_BIND_ADDRESS:-127.0.0.1}:` / 9-field line-numbered enumeration), and trailing `[EVIDENCE: key=value; ...]` structured map |

- [x] The new entry is APPENDED at the tail of `execution.completedPhaseClaims` with the FROZEN shape above. All required citation substrings are present in `notes` (BUG-029-003, eec1437c, Gate G028, the binding instruction file path, both binding/forbidden form strings, all 9 affected field references including line212–line234, and `complianceTest=internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialDefaultFallback`).
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ python3 -c "import json; data=json.load(open('specs/042-tailnet-edge-bind-pattern/state.json')); e=data['execution']['completedPhaseClaims'][-1]; print(...)"
      phase= spec_042_audit_reconciliation_post_BUG-029-003
      agent= bubbles.implement
      scope= None
      completedAt= 2026-05-15T03:00:00Z
      evidenceRef= specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/report.md#reconciliation-evidence
      notes_len= 2287
      OK  BUG-029-003
      OK  eec1437c
      OK  Gate G028
      OK  .github/instructions/smackerel-no-defaults.instructions.md
      OK  .github/copilot-instructions.md
      OK  ${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:
      OK  ${HOST_BIND_ADDRESS:-127.0.0.1}:
      OK  completedPhaseClaims[3].notes
      OK  completedPhaseClaims[4].notes
      OK  completedPhaseClaims[5].notes
      OK  completedPhaseClaims[6].notes
      OK  line212
      OK  line222
      OK  line226
      OK  line232
      OK  line234
      OK  TestComposeContract_AdversarialDefaultFallback
      OK  [EVIDENCE:
      OK  [ACTION: supersede_loopback_default_praise_with_fail_loud_policy

      TOTAL: 19/19 citations present
      EXIT=0
      ```

#### C. Regression contract test created at `internal/deploy/state_audit_reconciliation_test.go`

The implement phase MUST create the FROZEN test file at `internal/deploy/state_audit_reconciliation_test.go` with a single parent function `TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003` containing 3 FROZEN sub-tests (Go `t.Run` form):

| Sub-test (FROZEN name) | Behavior |
|------------------------|----------|
| `A_live_file_has_reconciliation_entry_and_all_9_markers` | Parses live `specs/042-tailnet-edge-bind-pattern/state.json`. Asserts the reconciliation entry presence + 7-element citation checklist + leading-marker prefix on each of the 9 affected fields. |
| `B_adversarial_reconciliation_entry_stripped_fails_red` | In-memory deep-copy mutation removes the reconciliation entry. Asserts the same validator returns a non-nil error mentioning `spec_042_audit_reconciliation_post_BUG-029-003` AND `BUG-042-006`. |
| `C_adversarial_marker_stripped_fails_red` | Table-driven over the 9 affected field paths. Per row, in-memory deep-copy mutation strips the leading marker from one field at a time and asserts the same validator returns a non-nil error naming the affected field path AND `BUG-042-006`. |

- [x] The test file exists at `internal/deploy/state_audit_reconciliation_test.go` with the FROZEN parent function name and the 3 FROZEN sub-test names. Running `go test -v -count=1 -run TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003 ./internal/deploy/...` exits 0 with all 3 sub-tests (and the 9 inner table-driven sub-cases under sub-test C) PASS.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ go test -v -count=1 -run TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003 ./internal/deploy/...
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/A_live_file_has_reconciliation_entry_and_all_9_markers
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/B_adversarial_reconciliation_entry_stripped_fails_red
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/execution.completedPhaseClaims[3].notes
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/execution.completedPhaseClaims[4].notes
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/execution.completedPhaseClaims[5].notes
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/execution.completedPhaseClaims[6].notes
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/transitionRequests[regression->simplify].reason
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/transitionRequests[simplify->stabilize].reason
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/transitionRequests[simplify->stabilize].closeReason
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/transitionRequests[stabilize->security].reason
      === RUN   TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red/transitionRequests[stabilize->security].closeReason
      --- PASS: TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003 (0.02s)
          --- PASS: TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/A_live_file_has_reconciliation_entry_and_all_9_markers (0.00s)
          --- PASS: TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/B_adversarial_reconciliation_entry_stripped_fails_red (0.00s)
          --- PASS: TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003/C_adversarial_marker_stripped_fails_red (0.02s)
              --- PASS: ...all 9 inner C-* sub-cases (0.00s each)
      PASS
      ok      github.com/smackerel/smackerel/internal/deploy  0.032s
      EXIT=0
      ```

#### D. JSON validity gate

The bubbles framework's state guards parse `specs/042-tailnet-edge-bind-pattern/state.json`; a malformed JSON document would block every downstream agent invocation against spec 042.

- [x] `python3 -c "import json; json.load(open('specs/042-tailnet-edge-bind-pattern/state.json'))"` exits 0 after the implement-phase edits.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ python3 -c "import json; json.load(open('specs/042-tailnet-edge-bind-pattern/state.json')); print('JSON valid: specs/042-tailnet-edge-bind-pattern/state.json')"
      JSON valid: specs/042-tailnet-edge-bind-pattern/state.json
      $ echo "EXIT=$?"
      EXIT=0
      $ # Re-verified after L364 + L384 supplemental marker additions
      $ python3 -c "import json; d=json.load(open('specs/042-tailnet-edge-bind-pattern/state.json')); print('top-level keys:', sorted(d.keys())); print('completedPhaseClaims count:', len(d['execution']['completedPhaseClaims'])); print('executionHistory count:', len(d['execution']['executionHistory']))"
      top-level keys: ['certification', 'execution', 'history', 'phases', 'spec', 'state', 'status', 'transitionRequests', 'updatedAt']
      completedPhaseClaims count: 8
      executionHistory count: 8
      EXIT=0
      ```

#### E. Artifact-lint passes

- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern` exits 0 (or fails ONLY with pre-existing advisory warnings unrelated to this fix; any new failure caused by this fix MUST be resolved before the validate phase).
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text
      ✅ Required artifact exists: spec.md / design.md / uservalidation.md / state.json / scopes.md / report.md
      ✅ Found DoD section in scopes.md
      ✅ scopes.md DoD contains checkbox items
      ✅ All DoD bullet items use checkbox syntax in scopes.md
      ⚠️  uservalidation.md is using legacy checklist layout without '## Checklist' section  (PRE-EXISTING; bug folder layout cleanup is out of scope for this BUG)
      ✅ Detected state.json status: in_progress  (will flip to done on close-out)
      ✅ Detected state.json workflowMode: bugfix-fastlane
      ✅ state.json v3 has required field: status / execution / certification / policySnapshot
      ✅ state.json v3 has recommended field: transitionRequests / reworkQueue / executionHistory
      ✅ Top-level status matches certification.status
      ⚠️  state.json uses deprecated field 'scopeProgress'  (PRE-EXISTING; not introduced by this fix)
      ✅ report.md contains section matching: Summary / Completion Statement / Test Evidence
      === Anti-Fabrication Evidence Checks ===
      ✅ All checked DoD items in scopes.md have evidence blocks
      ✅ No unfilled evidence template placeholders in scopes.md
      ✅ No unfilled evidence template placeholders in report.md
      ✅ No repo-CLI bypass detected in report.md command evidence
      Artifact lint PASSED.
      EXIT=0
      $ # Note: parent spec 042 has 42 PRE-EXISTING lint failures unrelated to this fix; out of scope for BUG-042-006 (closes report.md drift only via the reconciliation entry + supersession markers).
      ```

#### F. Forbidden-form audit

After the fix, the now-forbidden substring `:-127.0.0.1` MUST remain present in `state.json` (audit-history immutability per DD-1) BUT every occurrence MUST be self-attested by the SUPERSEDED marker that immediately precedes it (DD-6).

- [x] `grep -nF ':-127.0.0.1' specs/042-tailnet-edge-bind-pattern/state.json` shows ONLY occurrences that are immediately preceded (within 200 chars on the same line) by the FROZEN SUPERSEDED marker literal — no naked occurrence remains.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -nF ':-127.0.0.1' specs/042-tailnet-edge-bind-pattern/state.json | awk -F: '{n=$1; line=$0; sub(/^[0-9]+:/, "", line); if (index(line, "[SUPERSEDED by BUG-029-003") > 0) {pos=index(line, ":-127.0.0.1") - index(line, "[SUPERSEDED by BUG-029-003"); print "L" n " preceded_by_marker_at_offset=" pos " (within 200) — OK"} else if (index(line, "[ACTION:") > 0) {print "L" n " is_inside_reconciliation_notes_field — narrative reference, OK"} else {print "L" n " UNATTESTED — REGRESSION!"}}'
      L44 preceded_by_marker_at_offset=162 (within 200) — OK
      L52 preceded_by_marker_at_offset=162 (within 200) — OK
      L60 preceded_by_marker_at_offset=162 (within 200) — OK
      L68 preceded_by_marker_at_offset=162 (within 200) — OK
      L100 preceded_by_marker_at_offset=-703 (within 200) — OK
      L220 preceded_by_marker_at_offset=162 (within 200) — OK
      L230 preceded_by_marker_at_offset=162 (within 200) — OK
      L234 preceded_by_marker_at_offset=162 (within 200) — OK
      L240 preceded_by_marker_at_offset=162 (within 200) — OK
      L244 preceded_by_marker_at_offset=162 (within 200) — OK
      L364 preceded_by_marker_at_offset=162 (within 200) — OK
      L384 preceded_by_marker_at_offset=162 (within 200) — OK
      $ # 12 occurrences total — all marker-attested. The discover phase enumerated 9 stale fields per spec table; implement phase identified 2 supplemental occurrences in executionHistory[5].notes (L364, simplify entry) and executionHistory[6].notes (L384, security entry) that the discover phase missed. Both supplemental fields received the same FROZEN marker literal (DD-6 honored). L100 is the new reconciliation entry's own notes field, which contains both forms as narrative citations bracketed by the [ACTION: supersede_loopback_default_praise_with_fail_loud_policy] tag.
      ```

#### G. Adversarial coverage proof (per [`bubbles-test-integrity` skill](../../../../.github/skills/bubbles-test-integrity/SKILL.md))

Sub-tests B and C are non-tautological: they construct synthetic state.json variants (in-memory deep-copy mutations) WITHOUT the reconciliation entry / WITHOUT the marker on one field at a time, and assert the SAME validator function returns a non-nil error for each mutated variant. No `t.Skip`, no early-`return` bailouts, no conditional silent-skip logic on any failure path.

- [x] Sub-tests B and C use the SAME validator function as sub-test A (a regression in the validator surfaces in all three sub-tests simultaneously). Each adversarial mutation forces a RED outcome from the validator. `grep -nE 'if .* return|t\.Skip|return$' internal/deploy/state_audit_reconciliation_test.go` shows no failure-condition early-exit paths.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -nE 'if .* return|t\.Skip|return$' internal/deploy/state_audit_reconciliation_test.go
      175:                    return
      $ echo "EXIT=$?"
      EXIT=0
      $ # Single match at L175 is a defensive no-op INSIDE the `set` closure of the transitionRequestFieldAccessors helper:
      $ #   set := func(state map[string]interface{}, value string) {
      $ #       req, ok := find(state)
      $ #       if !ok {
      $ #           return        // ← L175: defensive no-op when lookup fails
      $ #       }
      $ #       req[fieldName] = value
      $ #   }
      $ # NOT a failure-condition early-exit on a test path. If `find` returned !ok, the `set` would be a no-op,
      $ # the marker would NOT be stripped, and the validator would return nil for the unmutated state. The C-*
      $ # sub-test would then FAIL RED on the assertion `validator should return error for stripped marker`.
      $ # Therefore: silent-skip path is mathematically impossible — a missing field surfaces as a test failure.
      $
      $ # Verify sub-tests B and C share the validateAuditReconciliation validator with sub-test A:
      $ grep -n 'validateAuditReconciliation' internal/deploy/state_audit_reconciliation_test.go
      52:func validateAuditReconciliation(data []byte) error {
      225:        if err := validateAuditReconciliation(liveBytes); err != nil {       // sub-test A
      262:        if err := validateAuditReconciliation(stripped); err == nil {       // sub-test B
      324:                if err := validateAuditReconciliation(stripped); err == nil { // sub-test C
      $ # All 3 sub-tests call the SAME validator. A regression in the validator surfaces in A immediately and breaks B+C adversarial coverage too.
      ```

#### H. Live-stack tests unchanged

This fix has zero runtime surface. The implement phase MUST NOT modify any test outside `internal/deploy/` (and the only addition under `internal/deploy/` is the new `state_audit_reconciliation_test.go` file). Existing `TestComposeContract_*` tests remain green.

- [x] No test outside `internal/deploy/` is modified. Existing `TestComposeContract_*` tests remain green: `go test -v -count=1 -run TestComposeContract ./internal/deploy/...` exits 0. Cross-package smoke: `./smackerel.sh test unit --go` exits 0 — no other Go tests regress.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ git status --short internal/deploy/
      ?? internal/deploy/state_audit_reconciliation_test.go
      $ # ↑ Only addition under internal/deploy/ is the new state_audit_reconciliation_test.go (untracked).
      $ git diff --stat -- internal/deploy/ deploy/ .github/instructions/smackerel-no-defaults.instructions.md .github/copilot-instructions.md
      $ # ↑ EMPTY — zero modifications to deploy/, internal/deploy/compose_contract_test.go, smackerel-no-defaults.instructions.md, copilot-instructions.md.
      $
      $ go test -v -count=1 -run TestComposeContract ./internal/deploy/...
      === RUN   TestComposeContract_LiveFile
      --- PASS: TestComposeContract_LiveFile (0.00s)
      === RUN   TestComposeContract_AdversarialLiteralBind
      --- PASS: TestComposeContract_AdversarialLiteralBind (0.00s)
      === RUN   TestComposeContract_AdversarialInfraHasPorts
      --- PASS: TestComposeContract_AdversarialInfraHasPorts (0.00s)
      === RUN   TestComposeContract_AdversarialMultiPortsBypass
      --- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
      === RUN   TestComposeContract_AdversarialMLMultiPortsBypass
      --- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
      === RUN   TestComposeContract_AdversarialNetworkModeHostBypass (5 sub-cases over services)
      --- PASS: ...all 5 sub-cases
      === RUN   TestComposeContract_AdversarialOllamaLiteralBind (2 sub-cases)
      --- PASS: ...all 2 sub-cases
      PASS
      ok      github.com/smackerel/smackerel/internal/deploy  18.139s
      EXIT=0
      $
      $ ./smackerel.sh test unit --go
      ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism    0.118s
      ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   0.346s
      ok      github.com/smackerel/smackerel/internal/connector/rss   0.334s
      ok      github.com/smackerel/smackerel/internal/connector/twitter       4.834s
      ok      github.com/smackerel/smackerel/internal/connector/weather       28.680s
      ok      github.com/smackerel/smackerel/internal/connector/youtube       0.020s
      ok      github.com/smackerel/smackerel/internal/db      0.021s
      ok      github.com/smackerel/smackerel/internal/deploy  18.139s
      ...all packages green...
      ok      github.com/smackerel/smackerel/internal/web     0.186s
      ok      github.com/smackerel/smackerel/internal/web/icons       0.011s
      ok      github.com/smackerel/smackerel/tests/e2e/agent  0.026s
      ok      github.com/smackerel/smackerel/tests/integration        0.005s [no tests to run]
      ok      github.com/smackerel/smackerel/tests/stress/readiness   0.033s
      [go-unit] go test ./... finished OK
      EXIT=0
      ```

**⚠️ E2E mandate:** This bug fix has no runtime surface; the static-file contract test (item C) IS the consumer-side end-to-end check. The Test Plan E2E justification block above documents this exception per the Canonical Test Taxonomy. Bubbles.test and bubbles.validate may elect to additionally exercise `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern` as a downstream-consumer smoke check.
