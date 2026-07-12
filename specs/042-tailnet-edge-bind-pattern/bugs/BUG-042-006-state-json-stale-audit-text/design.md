# Design: BUG-042-006 — Reconcile spec 042 `state.json` stale audit text with current Gate G028 fail-loud policy

> **Status:** Finalized by `bubbles.design` 2026-05-15. Inherits the architecture from `bubbles.bug`'s discover-phase draft. DD-1 through DD-7 preserved verbatim. DD-6 refined (the spec-044 precedent shape extended to name the now-FORBIDDEN form by exact substring `:-127.0.0.1` so the marker is self-attesting). DD-8 amended (test file path: `internal/deploy/state_audit_reconciliation_test.go`; function: `TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003`; sub-tests A/B/C). DD-9 added (entry-shape schema reconciliation: spec 042 `state.json` has no top-level `phaseLog` array, so the reconciliation entry is appended to `execution.completedPhaseClaims` with the user-specified `{at, action, outcome, evidence}` field shape mapped onto the existing `completedPhaseClaims` schema). Marker literal, reconciliation entry shape, and regression test design are now FROZEN — the implement phase applies them verbatim.

## Problem Recap

`specs/042-tailnet-edge-bind-pattern/state.json` is the append-only version-3 control-plane audit history for the spec 042 bugfix-fastlane chain. Nine distinct narrative excerpts across four `execution.completedPhaseClaims[*].notes` records and five `execution.pendingTransitionRequests[*]` `reason` / `closeReason` fields praise the now-FORBIDDEN substitution form `${HOST_BIND_ADDRESS:-127.0.0.1}:` as the canonical loopback-default pattern. That praise was accurate when written (2026-05-09) but was reversed by BUG-029-003 (HEAD `eec1437c`, 2026-05-14) which made the `${VAR:-default}` form FORBIDDEN by Gate G028 NO-DEFAULTS / fail-loud SST policy. The audit history was never reconciled with the policy reversal. See [`spec.md`](./spec.md) for the line-precise stale-excerpt evidence table.

## Design Constraints (NON-NEGOTIABLE)

1. **Audit history is append-only.** No historical narrative substance may be deleted or rewritten. Every original substring MUST remain present in the file after the fix (verifiable by `git diff` substring presence check).
2. **No production-runtime change.** This fix touches a single audit document. It MUST NOT modify `deploy/compose.deploy.yml`, `internal/deploy/compose_contract_test.go`, `.github/instructions/smackerel-no-defaults.instructions.md`, `.github/copilot-instructions.md`, any other spec, or any runtime source code.
3. **JSON must remain valid.** The file is parsed by the Bubbles framework's state guards; a malformed JSON document would block every downstream agent invocation against spec 042. After the fix, `python3 -c 'import json; json.load(open("specs/042-tailnet-edge-bind-pattern/state.json"))'` MUST exit 0.
4. **Artifact-lint must remain clean.** After the fix, `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern/` MUST exit 0 (or fail ONLY with pre-existing advisory warnings unrelated to this fix).
5. **The fix must be self-attesting.** A reader who lands on any one of the 9 stale excerpts MUST be able to discover the policy reversal and the reconciliation entry without leaving the file. The leading supersession marker provides the local discovery path; the reconciliation entry provides the authoritative reconciliation narrative.
6. **The fix must be regression-locked.** A persistent static-file contract test MUST exist that fails RED if either (a) the reconciliation entry is removed or (b) any one of the 9 stale excerpts loses its leading supersession marker.

## Approach: Two-Part Reconciliation

### Part 1: Append a single authoritative reconciliation entry (FROZEN by bubbles.design)

**Target array:** `execution.completedPhaseClaims` (the existing 10 entries are preserved verbatim; the new entry is APPENDED at the tail — it MUST NOT be inserted in the middle).

**Why `execution.completedPhaseClaims` and not a new top-level `phaseLog` array?** See DD-9 in the Design Decisions table below. Short version: spec 042 `state.json` has no `phaseLog` field; introducing one would create a parallel audit surface that downstream framework guards do not consume. `execution.completedPhaseClaims` is the existing audit-log array a reader scrolls through, so the reconciliation entry belongs there.

**Frozen JSON shape (the implement phase MUST write exactly this; only the `completedAt` ISO timestamp is filled at fix time):**

```json
{
  "phase": "spec_042_audit_reconciliation_post_BUG-029-003",
  "agent": "bubbles.implement",
  "scope": null,
  "completedAt": "<ISO-8601 timestamp set by bubbles.implement at fix time>",
  "evidenceRef": "specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/report.md#reconciliation-evidence",
  "notes": "[ACTION: supersede_loopback_default_praise_with_fail_loud_policy] Spec 042's original implementation chose ${HOST_BIND_ADDRESS:-127.0.0.1}: as the compose host-bind substitution form, and 4 phaseLog entries (lines 44/52/60/68) plus 5 transition-request fields (lines 212/222/226/232/234) praised that decision as 'preserving loopback default'. BUG-029-003 (closed at HEAD eec1437c on 2026-05-14) reversed that decision and converted the form to ${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}: per Gate G028 NO-DEFAULTS / fail-loud SST policy, codified in .github/instructions/smackerel-no-defaults.instructions.md and .github/copilot-instructions.md. The original phaseLog and transition-request entries are RETAINED FOR AUDIT HISTORY (append-only audit log immutability) but each has been prefixed with a [SUPERSEDED by BUG-029-003 ...] marker so future readers do not mistake the now-forbidden :-127.0.0.1 form for current correct policy. The 9 affected fields are: completedPhaseClaims[3].notes (line 44, regression specialist), completedPhaseClaims[4].notes (line 52, simplify specialist), completedPhaseClaims[5].notes (line 60, stabilize specialist), completedPhaseClaims[6].notes (line 68, security specialist), and pendingTransitionRequests entries at lines 212, 222, 226, 232, 234. Spec 042's compose contract on HEAD continues to be enforced by internal/deploy/compose_contract_test.go which now rejects the :-127.0.0.1 form via TestComposeContract_AdversarialDefaultFallback. [EVIDENCE: policyReversalCommit=eec1437c; policyReversalBug=BUG-029-003; bindingInstruction=.github/instructions/smackerel-no-defaults.instructions.md; bindingWorkspaceRule=.github/copilot-instructions.md; gate=G028; forbiddenForm=${HOST_BIND_ADDRESS:-127.0.0.1}:; requiredForm=${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:; supersededFields=completedPhaseClaims[3].notes,completedPhaseClaims[4].notes,completedPhaseClaims[5].notes,completedPhaseClaims[6].notes,pendingTransitionRequests[*]@line212,pendingTransitionRequests[*]@line222,pendingTransitionRequests[*]@line226,pendingTransitionRequests[*]@line232,pendingTransitionRequests[*]@line234; complianceTest=internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialDefaultFallback]"
}
```

**Schema-mapping rationale (DD-9):** the user-supplied canonical content shape `{at, phase, action, outcome, evidence}` is mapped onto the existing `execution.completedPhaseClaims[*]` schema `{phase, agent, scope, completedAt, evidenceRef, notes}` as follows: `at` → `completedAt` (semantic synonym for the ISO-8601 fix-time timestamp); `phase` → `phase` (verbatim literal `spec_042_audit_reconciliation_post_BUG-029-003`); `action` → embedded as the leading `[ACTION: <slug>]` tag inside `notes` for greppability; `outcome` → the body of `notes`; `evidence` → embedded as the trailing `[EVIDENCE: key=value; ...]` structured map inside `notes` (single-line, semicolon-separated, also greppable). The schema-required fields `agent` and `scope` are filled with `bubbles.implement` and `null` respectively (they are not part of the user's specified content but are mandatory under the existing `completedPhaseClaims` schema).

**Forbidden variations (the implement phase MUST NOT do any of these):**

- Add the entry to a new top-level `phaseLog` array (would create a parallel audit surface — see DD-9).
- Add the entry to `executionHistory` (top-level run-log array — semantically wrong; the reconciliation is a phase claim, not a phase run).
- Insert the entry anywhere except the TAIL of `execution.completedPhaseClaims` (would violate append-only ordering).
- Modify the `phase`, the `[ACTION: ...]` tag literal, the `[EVIDENCE: ...]` block keys, or any of the substantive citation strings above.
- Rename or alias the `phase` literal — `spec_042_audit_reconciliation_post_BUG-029-003` is the regression-test anchor.

### Part 2: Annotate each stale excerpt with a leading supersession marker (FROZEN by bubbles.design)

For each of the 9 stale excerpts identified in [`spec.md`](./spec.md) → "Stale Audit Lines (Evidence)" table, the implement phase prepends the SAME literal marker string to the **start** of the relevant `notes` / `reason` / `closeReason` string, immediately followed by a single space and then the original (untouched) text.

**Frozen marker literal (the implement phase MUST apply this IDENTICAL string to all 9 fields — no per-field parameterization, no wording variation):**

```
[SUPERSEDED by BUG-029-003 (HEAD eec1437c) — fail-loud form ${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}: now binding per Gate G028; the :-127.0.0.1 form below is RETAINED FOR AUDIT HISTORY ONLY and is now FORBIDDEN. See .github/instructions/smackerel-no-defaults.instructions.md]
```

**Why this exact wording (citation checklist — every item below MUST be present in the marker):**

| Required element | Resolved by |
|------------------|-------------|
| Names the policy-reversal source bug ID | `BUG-029-003` |
| Names the policy-reversal source commit SHA | `HEAD eec1437c` |
| Names the now-required (binding) substitution form | `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` |
| Names the binding policy gate identifier | `Gate G028` |
| Names the now-FORBIDDEN form by exact substring | `:-127.0.0.1` |
| Self-attests that the historical substance below is retained for audit only | `RETAINED FOR AUDIT HISTORY ONLY and is now FORBIDDEN` |
| Names the binding instruction file (file-path-actionable from inside `state.json`) | `.github/instructions/smackerel-no-defaults.instructions.md` |

**Insertion convention:** prepend the marker literal plus a single ASCII space at index 0 of the field's string value. Do NOT modify the field's structural shape. Do NOT delete or rewrite any character that follows the marker. Example (regression specialist, line 44):

```diff
-      "notes": "Regression specialist diagnostic verification pass (no code, ...)..."
+      "notes": "[SUPERSEDED by BUG-029-003 (HEAD eec1437c) — fail-loud form ${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}: now binding per Gate G028; the :-127.0.0.1 form below is RETAINED FOR AUDIT HISTORY ONLY and is now FORBIDDEN. See .github/instructions/smackerel-no-defaults.instructions.md] Regression specialist diagnostic verification pass (no code, ...)..."
```

**Identical-literal contract:** The marker is intentionally identical across all 9 insertion points so the regression contract test can use a SINGLE literal string match for adversarial verification (DD-7 + Sub-test C below). The implement phase MUST NOT vary the marker per-field — any variation would break the table-driven adversarial sub-test contract.

**Forbidden variations (the implement phase MUST NOT do any of these):**

- Vary the marker per-field (would defeat the single-literal regression-test contract).
- Drop the `:-127.0.0.1` substring from the marker (the marker MUST name the now-forbidden form by exact substring so a future grep against `state.json` for the forbidden substring is always self-attested by the marker that immediately precedes it).
- Drop the `.github/instructions/smackerel-no-defaults.instructions.md` file path from the marker (the marker MUST be file-path-actionable from inside `state.json` per DD-4).
- Insert the marker anywhere except at index 0 of the field's string value (would not preempt a casual reader who scans only the first words of a `notes` field).

## Design Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| **DD-1** | Append-only — DO NOT delete or rewrite historical narrative substance | The audit-history-immutability contract is the foundation of bubbles' tamper-evident audit trail. Deleting or rewriting historical narrative would compromise every downstream guard that relies on the audit history (state-transition guard, traceability guard, regression baseline guard). |
| **DD-2** | Prefix-not-replace — leading marker on each stale excerpt, original substring preserved verbatim | Honors DD-1 while still providing local discovery: a reader who lands on the stale excerpt sees the supersession marker BEFORE they read the misleading substance, so they cannot mis-cite the substance as current authority. |
| **DD-3** | Single authoritative reconciliation entry as source-of-truth pointer | One entry, not nine. Per-excerpt per-field reconciliation narratives would re-introduce the very confusion the fix exists to eliminate: a reader would have to reconcile multiple per-narrative reconciliations against each other. The leading marker delegates the reconciliation narrative to a single authoritative entry. |
| **DD-4** | Cite the binding policy authority by file path | The marker and the reconciliation entry both name `.github/instructions/smackerel-no-defaults.instructions.md` and `.github/copilot-instructions.md` because those are the files a future agent MUST consult to resolve the canonical form. Naming Gate G028 alone is insufficient — Gate identifiers without file paths are not directly actionable from inside `state.json`. |
| **DD-5** | Cite the policy reversal commit by SHA + bug ID | `BUG-029-003 (HEAD eec1437c, 2026-05-14)` — the SHA pins the reversal to a single concrete commit; the bug ID pins it to a spec dossier; the date pins it to the self-hosted readiness re-scan window. All three are required for an unambiguous citation. |
| **DD-6** | Inherit the precedent's marker shape (`[SUPERSEDED ...]`) and extend it to name the now-FORBIDDEN form by exact substring | Spec 044's HL-RESCAN-007 close-out at HEAD `b715d143` established the square-bracketed `[SUPERSEDED ...]` syntactic shape. This fix preserves that shape (square-bracketed leading tag, "SUPERSEDED" verb, source bug + commit SHA citation, fail-loud form quoted verbatim, gate identifier, binding-instruction file path) and EXTENDS it to additionally name the now-FORBIDDEN form by the exact substring `:-127.0.0.1` and to self-attest "RETAINED FOR AUDIT HISTORY ONLY and is now FORBIDDEN". The extension is justified by the spec 042 surface having a higher density of stale praise (9 distinct excerpts vs spec 044's 3) — the stronger marker eliminates any chance of a future agent skim-reading the marker as a generic supersession tag and missing the per-form contract. |
| **DD-7** | Lock the fix with a persistent static-file regression contract test | A documentation fix without a regression test will silently rot the next time a maintainer hand-edits `state.json`. The test asserts (a) the reconciliation entry is present AND (b) every `notes` / `reason` / `closeReason` string in the file containing the substring `${HOST_BIND_ADDRESS:-127.0.0.1}` carries a leading `[SUPERSEDED by BUG-029-003 ...]` marker. Adversarial mutation tests prove the contract: removing the reconciliation entry MUST fail; stripping the marker from any one of the 9 excerpts MUST fail. |
| **DD-8** | Place the regression test in `internal/deploy/state_audit_reconciliation_test.go` (single function `TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003` with sub-tests A/B/C) | Co-locates with the existing `compose_contract_test.go` and `dev_compose_default_fallback_test.go` static-file contract suite that already enforces the runtime side of the same policy. Filename uses the shorter `state_audit_reconciliation_test.go` (vs the bug.draft `state_json_audit_reconciliation_contract_test.go`) for parity with the existing `compose_contract_test.go` naming pattern (no `_contract_` infix; `_test.go` suffix is sufficient under Go test conventions). The single-function-with-sub-tests shape (one `TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003` parent with `t.Run` sub-tests A/B/C) groups the live-file assertion + 2 adversarial assertions under one test name for cleaner failure-output triage. |
| **DD-9** | Map the user-specified `{at, phase, action, outcome, evidence}` reconciliation-entry shape onto the existing `execution.completedPhaseClaims` schema rather than introducing a new `phaseLog` array | Spec 042's `state.json` has no top-level `phaseLog` array (verified by `python3 -c 'json.load(...)'` against HEAD). Adding a new top-level array would create a parallel audit surface that downstream framework guards (state-transition guard, traceability guard) do not consume; future agents reading the file might also miss the new array. `execution.completedPhaseClaims` IS the audit-log array a reader scrolls through to see what each phase claimed. The user-specified field shape is mapped: `at` → `completedAt` (semantic synonym; ISO-8601 timestamp), `phase` → `phase` (verbatim), `action` → leading `[ACTION: ...]` tag inside `notes`, `outcome` → body of `notes`, `evidence` → trailing `[EVIDENCE: key=value; ...]` block inside `notes`. The schema-required fields `agent` and `scope` are filled with `bubbles.implement` and `null` (mandatory under the existing schema; not part of the user's specified content). All citation/enumeration content the user specified is preserved verbatim — only the field-shape skin changes. |

## Affected Files

| File | Change |
|------|--------|
| `specs/042-tailnet-edge-bind-pattern/state.json` | (1) append `spec_042_audit_reconciliation_post_BUG-029-003` entry to `execution.completedPhaseClaims` per Part 1's frozen JSON shape; (2) prepend the frozen marker literal (Part 2) to the 9 affected `notes` / `reason` / `closeReason` strings (verbatim original substance preserved after the marker plus single space) |
| `internal/deploy/state_audit_reconciliation_test.go` | NEW — persistent static-file regression contract test (Go) per DD-7 + DD-8. Single parent function `TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003` with 3 sub-tests (A: live-file; B: adversarial reconciliation entry stripped; C: table-driven adversarial marker stripped, parameterized over 9 fields). |

**Files NOT touched (out of scope per `spec.md` → "Out of Scope"):**

- `deploy/compose.deploy.yml`
- `internal/deploy/compose_contract_test.go`
- `.github/instructions/smackerel-no-defaults.instructions.md`
- `.github/copilot-instructions.md`
- Any spec other than `042`
- The spec 042 `report.md` per-phase evidence sections

## Regression Test Design (FROZEN by bubbles.design)

Persistent static-file contract test (Go), placement `internal/deploy/state_audit_reconciliation_test.go`. Single parent function with 3 sub-tests (Go `t.Run` form). The implement phase MUST NOT split the parent into 3 separate top-level functions — keeping them under one parent groups the failure output for triage.

**Function signature contract (frozen):**

```go
// TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003 enforces the
// audit-history reconciliation contract for spec 042 against the live
// specs/042-tailnet-edge-bind-pattern/state.json file. See bug packet
// specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/
// design.md for the binding marker literal, reconciliation-entry shape,
// and 9-field enumeration.
func TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003(t *testing.T) {
    // ... loads specs/042-tailnet-edge-bind-pattern/state.json once
    // ... parses as map[string]interface{} (no struct binding — the file's
    //     full schema is not the test's concern; the test only walks
    //     execution.completedPhaseClaims and execution.pendingTransitionRequests).
    // ... runs the 3 sub-tests under t.Run.
}
```

**Sub-test A — live-file (assertion against the on-disk file at HEAD after the implement phase commits):**

```text
t.Run("A_live_file_has_reconciliation_entry_and_all_9_markers", ...):
  Parse specs/042-tailnet-edge-bind-pattern/state.json
  Walk execution.completedPhaseClaims:
    Assert at least one entry has phase == "spec_042_audit_reconciliation_post_BUG-029-003"
    For that entry, assert all of the following are present in notes (case-sensitive substring match):
      - "BUG-029-003"
      - "eec1437c"
      - "Gate G028"
      - ".github/instructions/smackerel-no-defaults.instructions.md"
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:"  (the new fail-loud form, exact)
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:"                                       (the now-forbidden form, exact)
      - "completedPhaseClaims[3].notes" through "completedPhaseClaims[6].notes"  (all four)
      - "line212", "line222", "line226", "line232", "line234"                    (all five)
      - "complianceTest=internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialDefaultFallback"
  Walk all 9 affected field paths (4 completedPhaseClaims[3..6].notes + 5 pendingTransitionRequests[*] reason/closeReason at lines 212/222/226/232/234):
    Assert the field's string value starts with the literal marker prefix
    "[SUPERSEDED by BUG-029-003 (HEAD eec1437c) — fail-loud form ${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}: now binding per Gate G028; the :-127.0.0.1 form below is RETAINED FOR AUDIT HISTORY ONLY and is now FORBIDDEN. See .github/instructions/smackerel-no-defaults.instructions.md]"
    (single literal, used for all 9 fields — no per-field variation)
```

**Sub-test B — adversarial reconciliation entry stripped (synthetic, in-memory mutation):**

```text
t.Run("B_adversarial_reconciliation_entry_stripped_fails_red", ...):
  Load the parsed state.json into memory (deep copy)
  Walk execution.completedPhaseClaims and remove the entry whose phase == "spec_042_audit_reconciliation_post_BUG-029-003"
  Re-run the contract validator (the same validation function used by Sub-test A) against the mutated in-memory copy
  Assert the validator returns a non-nil error
  Assert the error message mentions "spec_042_audit_reconciliation_post_BUG-029-003" (the missing phase identifier)
  Assert the error message mentions "BUG-042-006" (the bug ID a future maintainer would consult to understand why the test exists)
```

**Sub-test C — adversarial marker stripped (synthetic, in-memory mutation, table-driven across the 9 affected fields):**

```text
t.Run("C_adversarial_marker_stripped_fails_red", ...):
  Define table:
    affectedFields := []struct{ Path string; }{
      {Path: "execution.completedPhaseClaims[3].notes"},
      {Path: "execution.completedPhaseClaims[4].notes"},
      {Path: "execution.completedPhaseClaims[5].notes"},
      {Path: "execution.completedPhaseClaims[6].notes"},
      {Path: "execution.pendingTransitionRequests[*]@line212"},  // resolved to the actual transition-request index by walking pendingTransitionRequests
      {Path: "execution.pendingTransitionRequests[*]@line222"},
      {Path: "execution.pendingTransitionRequests[*]@line226"},
      {Path: "execution.pendingTransitionRequests[*]@line232"},
      {Path: "execution.pendingTransitionRequests[*]@line234"},
    }
  For each tc := range affectedFields:
    t.Run(tc.Path, func(t *testing.T) {
      Load the parsed state.json into memory (deep copy)
      Locate the field at tc.Path (resolve the @line marker by walking pendingTransitionRequests in declaration order; the @line annotation is the file line at HEAD, used only for human-readable test naming — the actual mutation is by index)
      Strip the leading marker prefix from the field's string value (replace the first occurrence of the marker literal with the empty string)
      Re-run the contract validator against the mutated in-memory copy
      Assert the validator returns a non-nil error
      Assert the error message names tc.Path (so a future maintainer who silently strips a single marker sees exactly which field is offended)
      Assert the error message mentions "BUG-042-006" (the bug ID for context)
    })
```

**Non-tautology contract (per [`bubbles-test-integrity` skill](../../../../.github/skills/bubbles-test-integrity/SKILL.md)):**

- Sub-test A would fail RED against the file BEFORE the implement phase applies the marker prepend + reconciliation entry append (proves it is not vacuously satisfied by the current file — the bug exists, the test detects it).
- Sub-test B mutates the in-memory parsed JSON to strip the reconciliation entry; if the validator silently accepted the mutated copy, Sub-test B would NOT fire — proving the validator's positive-path assertion ("entry exists") is tied to a mutating adversary.
- Sub-test C mutates the in-memory parsed JSON to strip ONE marker from ONE field at a time, table-driven over the 9 fields; the test FAILS RED if ANY sub-case silently passes — proving the per-field marker assertion is independently locked.
- The validator function used by Sub-tests A/B/C is the SAME function (no separate validators per sub-test), so a regression in the validator itself surfaces in all three sub-tests simultaneously.
- No `t.Skip`, no early-`return`, no conditional bailouts — the test panics or fails on every adversarial path.

## Tech-agnostic Gherkin (BDD)

```gherkin
Feature: Spec 042 audit history is reconciled with current Gate G028 fail-loud policy

  Background:
    Given the spec 042 audit history file exists at "specs/042-tailnet-edge-bind-pattern/state.json"
    And the historical narratives in that file once praised a substitution form that has since been forbidden by current policy
    And the audit history is append-only — no historical substance may be deleted or rewritten

  Scenario: SCN-042-006-A — A reconciliation entry documents the policy reversal
    Given the spec 042 audit history file is parsed as a structured document
    When a reader inspects the tail of the completed-phase-claims log
    Then a single reconciliation entry exists whose phase identifier names "spec_042_audit_reconciliation_post_BUG-029-003"
    And the entry's narrative names the policy-reversal source bug
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

## Validation Strategy

| Check | Command | Expected |
|-------|---------|----------|
| JSON validity | `python3 -c 'import json; json.load(open("specs/042-tailnet-edge-bind-pattern/state.json"))'` | exit 0 |
| Contract test (Go) | `go test -count=1 -run TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003 ./internal/deploy/...` | exit 0, all 3 sub-tests PASS (A live-file, B adversarial-reconciliation-stripped, C table-driven adversarial-marker-stripped × 9) |
| Cross-package smoke | `./smackerel.sh test unit --go` | exit 0 |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern/` | exit 0 (or fail only with pre-existing advisory warnings unrelated to this fix) |
| Append-only verification | `git diff specs/042-tailnet-edge-bind-pattern/state.json` | no removals of historical substance; only additions of marker prefixes + the new reconciliation entry |
| No runtime change | `git diff -- deploy/ internal/deploy/compose_contract_test.go .github/instructions/smackerel-no-defaults.instructions.md .github/copilot-instructions.md` | EMPTY (other than the NEW `internal/deploy/state_audit_reconciliation_test.go` file) |

## Cross-Agent Routing

- **Discover (this entry):** `bubbles.bug` — creates the bug packet (this folder), identifies stale lines, drafts design and scopes, NEVER modifies `specs/042-tailnet-edge-bind-pattern/state.json` or any runtime file. Route to `bubbles.design` next.
- **Design:** `bubbles.design` — refines this design draft (refines marker wording, reconciliation entry shape, and regression contract test design); MUST preserve DD-1 through DD-8 unless a stronger rationale is documented. Route to `bubbles.plan` next.
- **Plan:** `bubbles.plan` — finalizes scopes.md (already drafted by `bubbles.bug`); confirms test plan; confirms DoD. Route to `bubbles.implement` next.
- **Implement:** `bubbles.implement` — applies the marker prepend × 9 + reconciliation entry append to `specs/042-tailnet-edge-bind-pattern/state.json`; authors the new `internal/deploy/state_json_audit_reconciliation_contract_test.go`. Uses IDE editing tools ONLY (no shell heredoc / redirection per terminal-discipline). Route to `bubbles.test` next.
- **Test → Validate → Audit → Finalize:** standard bugfix-fastlane chain.

## Acceptance Mapping

| AC | Met by |
|----|--------|
| AC-1 | Part 1 — appended reconciliation entry FROZEN by bubbles.design (DD-3, DD-4, DD-5, DD-9) |
| AC-2 | Part 2 — leading supersession markers FROZEN by bubbles.design (DD-2, DD-6) |
| AC-3 | DD-1 prefix-only edits + Validation Strategy → JSON validity check |
| AC-4 | Validation Strategy → artifact lint check |
| AC-5 | DD-7 + DD-8 → regression contract test (`TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003`) + 3 sub-tests A/B/C (SCN-042-006-A live-file, SCN-042-006-C reconciliation-stripped, SCN-042-006-D table-driven marker-stripped × 9) |
| AC-6 | DD-1 + DD-2 + Validation Strategy → append-only verification (git diff substring presence) |
