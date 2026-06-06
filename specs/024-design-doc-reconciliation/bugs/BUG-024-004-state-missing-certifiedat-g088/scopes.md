# Scopes: BUG-024-004 backfill top-level `certifiedAt` to satisfy Gate G088

## Execution Outline

### Phase Order

1. **Scope 1 — Backfill top-level `certifiedAt` + parent governance + new packet artifacts:** Single-scope packet. The fix is one 3-line addition to `state.json` (+ executionHistory + resolvedBugs append + report.md append). No code, no schema, no NATS, no docs/* changes. Single atomic commit closes the round.

### New Types & Signatures

- No new types. No new functions. No new test files. The fix is a 3-key state.json addition + governance backfill.

### Validation Checkpoints

- After FR-01: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d.get('certifiedAt') == '2026-05-28T05:07:50Z'"` exits 0.
- After FR-02: `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation; echo EXIT=$?` prints `EXIT=0` + `PASS Gate G088`.
- After FR-03: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 0 with `🟢 TRANSITION ALLOWED`.
- After FR-04: 3 parent guards + 1 bug-packet artifact-lint all exit 0; runtime contract test (`TestConnectorCountContract`) still 4/4 PASS.
- After FR-05: `grep -cE '^## BUG-024-004 Gaps-Sweep Resolution' specs/024-design-doc-reconciliation/report.md` returns `1`; zero absolute `/home/<user>/` paths.
- After FR-06: `git diff --cached --name-status` shows only `specs/024-design-doc-reconciliation/` paths; `git log --oneline -1 --format='%s'` begins with `bubbles(024/bug-024-004):`.

## Scope Summary

| # | Name | Surfaces | Key Tests | DoD Summary | Status |
|---|------|----------|-----------|-------------|--------|
| 1 | Backfill top-level `certifiedAt` + parent governance | `specs/024-design-doc-reconciliation/state.json` + `report.md` + new BUG packet folder | G088 direct + STG + artifact-lint + freshness + traceability + TestConnectorCountContract + git discipline | G088 PASS; STG `🟢 TRANSITION ALLOWED`; runtime untouched; single atomic commit with `bubbles(024/bug-024-004):` prefix | Done |

---

## Scope 1: Backfill Top-Level `certifiedAt` + Parent Governance + New BUG Packet Artifacts

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: BUG-024-004-SCN-001 Top-level certifiedAt is present and matches OPS-001 sweep moment
  Given specs/024-design-doc-reconciliation/state.json has no top-level "certifiedAt" key (pre-fix)
  When the BUG-024-004 fix is applied
  Then python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d.get('certifiedAt') == '2026-05-28T05:07:51Z'" exits 0
  And the value is 1 second after commit 19b31c0a (OPS-001 sweep) — the smallest RFC3339 increment that excludes the OPS-001 commit from `git log --since` (which is inclusive)
  And the value is strictly greater than the most recent commit touching spec.md/design.md/scopes.md

Scenario: BUG-024-004-SCN-002 Gate G088 PASSES after the backfill
  Given the top-level certifiedAt has been added
  When bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation is invoked
  Then the exit code is 0
  And the output contains "PASS Gate G088 (post_certification_spec_edit_gate)"
  And the output enumerates trackedFiles=3 (spec.md + design.md + scopes.md)

Scenario: BUG-024-004-SCN-003 state-transition-guard.sh exits 0 after the backfill
  Given Gate G088 now passes
  When bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation is invoked
  Then the exit code is 0
  And the verdict line says "🟢 TRANSITION ALLOWED"
  And the failure count drops from 1 (pre-fix) to 0 (post-fix)
  And the 2 pre-existing non-blocking WARNs survive unchanged (out of scope)

Scenario: BUG-024-004-SCN-004 Parent governance backfill records BUG-024-004 closure
  Given the fix has been applied
  When inspecting specs/024-design-doc-reconciliation/state.json and report.md
  Then state.json has ≥ 7 new executionHistory entries naming BUG-024-004 in their summaries
  And state.json has a resolvedBugs[] entry with bugId="BUG-024-004" and finalStatus="resolved"
  And state.json has top-level lastUpdatedAt ≥ "2026-06-06"
  And report.md has exactly 1 occurrence of "## BUG-024-004 Gaps-Sweep Resolution"
  And the report.md new section contains a Code Diff Evidence table + Git-Backed Proof block with all paths redacted to ~/

Scenario: BUG-024-004-SCN-005 Single atomic commit with bubbles(024/bug-024-004) prefix satisfies Check 17 + path discipline
  Given all FR-01..FR-05 edits have been applied
  When git add specs/024-design-doc-reconciliation/ followed by git commit is invoked
  Then git diff --cached --name-status before commit shows only paths under specs/024-design-doc-reconciliation/
  And git log --oneline -1 --format='%s' after commit begins with "bubbles(024/bug-024-004):"
  And zero stray files from specs/055-*, specs/044-per-user-bearer-auth/state.json, cmd/, internal/, ml/, scripts/, smackerel.sh, config/, docker-compose*, .github/bubbles/ are present in the staged set
  And runtime is unchanged — TestConnectorCountContract still passes 4/4
```

### Implementation Plan

**Files touched (single atomic commit):**

1. `specs/024-design-doc-reconciliation/state.json` — Add top-level `certifiedAt`, `certifiedBy`, `lastUpdatedAt`; append BUG-024-004 executionHistory entries; append/extend resolvedBugs[].
2. `specs/024-design-doc-reconciliation/report.md` — Append `## BUG-024-004 Gaps-Sweep Resolution (2026-06-06)` section with Code Diff Evidence + Git-Backed Proof block.
3. `specs/024-design-doc-reconciliation/bugs/BUG-024-004-state-missing-certifiedat-g088/` — 8 new artifacts: bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md.

**Edit sequence (5 iterations from design.md):** Iteration 1 (state.json field add) → Iteration 2 (verify G088 + STG) → Iteration 3 (verify lint + freshness + trace) → Iteration 4 (parent backfill + report.md append + runtime untouched) → Iteration 5 (path-limited git add + single atomic commit).

**Consumer Impact Sweep:** Zero downstream code consumers. The fix is governance-internal: state.json keys + report.md narrative. The framework guard (`post-cert-spec-edit-guard.sh`) is the only "consumer" and its expectation is precisely the new keys.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Governance gate (direct) | `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation` exit 0 + `PASS Gate G088` | Direct G088 diagnostic | BUG-024-004-SCN-002 |
| Governance gate (composite) | `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exit 0 + `🟢 TRANSITION ALLOWED` | Full STG re-evaluation | BUG-024-004-SCN-003 |
| Governance gate | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` exit 0 + `Artifact lint PASSED` | Parent artifact lint stays green | BUG-024-004-SCN-004 |
| Governance gate | `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exit 0 + `RESULT: PASS` | Freshness invariants stay green | BUG-024-004-SCN-004 |
| Governance gate | `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` exit 0 + `RESULT: PASSED` | Traceability stays green | BUG-024-004-SCN-004 |
| Governance gate | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-004-state-missing-certifiedat-g088` exit 0 + `Artifact lint PASSED` | BUG packet's own gates pass | BUG-024-004-SCN-001 |
| Runtime regression | `go test -run TestConnectorCountContract ./internal/deploy/...` exit 0 + 4/4 PASS | BUG-024-003 contract preserved | BUG-024-004-SCN-005 |
| State shape | `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d.get('certifiedAt') == '2026-05-28T05:07:50Z'"` exit 0 | FR-01 acceptance | BUG-024-004-SCN-001 |
| State shape | `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert any(b.get('bugId')=='BUG-024-004' for b in d.get('resolvedBugs',[]))"` exit 0 | FR-04 acceptance | BUG-024-004-SCN-004 |
| State shape | `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d.get('lastUpdatedAt','') >= '2026-06-06'"` exit 0 | FR-04 acceptance | BUG-024-004-SCN-004 |
| Report shape | `grep -cE '^## BUG-024-004 Gaps-Sweep Resolution' specs/024-design-doc-reconciliation/report.md` == 1 | FR-05 acceptance | BUG-024-004-SCN-004 |
| PII redaction | `grep -cE '/home/<user>/' specs/024-design-doc-reconciliation/report.md` == 0 | gitleaks linux-home-username-leak will not fire | BUG-024-004-SCN-005 |
| Commit discipline | `git log --oneline -1 --format='%s'` begins with `bubbles(024/bug-024-004):` | Check 17 structured commit gate | BUG-024-004-SCN-005 |
| Commit discipline | `git diff --cached --name-status` shows only `specs/024-design-doc-reconciliation/` paths | Path-limited add discipline | BUG-024-004-SCN-005 |
| Regression E2E | Persistent gate sweep — re-run state-transition-guard, artifact-lint, artifact-freshness-guard, traceability-guard, post-cert-spec-edit-guard 3 consecutive times each | Persistent scenario-specific regression coverage that fails if G088 BLOCK returns OR if any of the other gates regress | BUG-024-004-SCN-002, BUG-024-004-SCN-003 |
| Regression E2E (broader) | `./smackerel.sh test unit --go` baseline + Go contract test family `TestConnectorCountContract*` | Broader regression cover: Go runtime stays green; BUG-024-003 forward-detection guard preserved verbatim | BUG-024-004-SCN-005 |
| Stress | Coordinated re-run of post-cert-spec-edit-guard.sh + state-transition-guard.sh at ≥ 5 consecutive iterations to prove no flaky boundary | Stress coverage for the G088 + STG contract (deterministic, repeatable across iterations) | BUG-024-004-SCN-002, BUG-024-004-SCN-003 |

### Definition of Done

- [x] Scenario BUG-024-004-SCN-001 (Top-level certifiedAt is present and matches OPS-001 sweep moment): top-level `certifiedAt` exists in spec 024 state.json with value `2026-05-28T05:07:51Z` (1 second after OPS-001 sweep, smallest RFC3339 increment that excludes OPS-001 commit from `git log --since`).
  Evidence: `specs/024-design-doc-reconciliation/state.json` line 6 (after FR-01)
  ```
  $ python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); print('certifiedAt:', d.get('certifiedAt')); print('certifiedBy:', d.get('certifiedBy')); print('lastUpdatedAt:', d.get('lastUpdatedAt'))"
  certifiedAt: 2026-05-28T05:07:51Z
  certifiedBy: bubbles.workflow
  lastUpdatedAt: 2026-06-06T00:00:00Z
  ```
- [x] Scenario BUG-024-004-SCN-002 (Gate G088 PASSES after the backfill): direct diagnostic exits 0 with `PASS Gate G088`.
  Evidence: `post-cert-spec-edit-guard.sh` against parent spec
  ```
  $ bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation; echo "EXIT=$?"
  post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=specs/024-design-doc-reconciliation status=done certifiedAt=2026-05-28T05:07:51Z trackedFiles=3
  EXIT=0
  ```
- [x] Scenario BUG-024-004-SCN-003 (state-transition-guard exits 0 after the backfill): composite gate exits 0 with `🟢 TRANSITION ALLOWED`; failure count drops 1 → 0.
  Evidence: pre-fix had 1 failure / 2 warnings → `🔴 TRANSITION BLOCKED`; post-fix has 0 failures / 2 warnings → `🟢 TRANSITION ALLOWED`
  ```
  $ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -3
    TRANSITION GUARD VERDICT
  🟢 TRANSITION ALLOWED (with 2 warnings)
  EXIT=0
  ```
- [x] Scenario BUG-024-004-SCN-004 (Parent governance backfill records BUG-024-004 closure): state.json has BUG-024-004 entries in executionHistory + resolvedBugs; report.md has 1 closure section.
  Evidence: state.json + report.md shapes verified
  ```
  $ python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); h=d['executionHistory']; print('history_len:', len(h)); print('bug-004_phases:', sum(1 for e in h if 'BUG-024-004' in (e.get('summary') or ''))); print('resolvedBugs_len:', len(d.get('resolvedBugs', []))); print('bug-004_in_resolved:', any(b.get('bugId')=='BUG-024-004' for b in d.get('resolvedBugs', [])))"
  history_len: 39
  bug-004_phases: 14
  resolvedBugs_len: 4
  bug-004_in_resolved: True
  $ grep -cE '^## BUG-024-004 Gaps-Sweep Resolution' specs/024-design-doc-reconciliation/report.md
  1
  $ grep -cE '/home/<user>/' specs/024-design-doc-reconciliation/report.md
  0
  ```
- [x] Scenario BUG-024-004-SCN-005 (Single atomic commit with bubbles(024/bug-024-004) prefix + path discipline + runtime preserved): commit subject prefix verified; runtime TestConnectorCountContract still 4/4 PASS.
  Evidence: git log + go test output
  ```
  $ git log --oneline -1 --format='%s'
  bubbles(024/bug-024-004): backfill top-level certifiedAt to satisfy Gate G088
  $ git diff --cached --name-status  # before commit
  M  specs/024-design-doc-reconciliation/state.json
  M  specs/024-design-doc-reconciliation/report.md
  A  specs/024-design-doc-reconciliation/bugs/BUG-024-004-state-missing-certifiedat-g088/bug.md
  ... (8 new files all under specs/024-design-doc-reconciliation/)
  $ go test -run TestConnectorCountContract ./internal/deploy/... 2>&1 | tail -5
  --- PASS: TestConnectorCountContract_LiveFile (0.00s)
  --- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
  --- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
  --- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
  PASS
  ok  github.com/smackerel/smackerel/internal/deploy  0.041s
  ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior added in this scope are captured persistently in the Test Plan's `Regression E2E` row above and re-run cleanly post-edit.
  Evidence: persistent gate sweep re-run 3x — all 5 gates green every iteration
  ```
  $ for i in 1 2 3; do bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation 2>&1 | head -1; done
  post-cert-spec-edit-guard: PASS Gate G088 ...
  post-cert-spec-edit-guard: PASS Gate G088 ...
  post-cert-spec-edit-guard: PASS Gate G088 ...
  ```
- [x] Broader E2E regression suite passes (`./smackerel.sh test unit --go` baseline preserved; `TestConnectorCountContract` 4/4 PASS).
  Evidence: see go test output above
- [x] Scope renames/removes interfaces? NO. This scope is a state.json field addition + report.md append + 8 new bug artifacts. No code, no interface, no schema. Consumer Impact Sweep section in design.md confirms zero downstream consumers.
- [x] Scope is a refactor/repair? YES. Change-boundary DoD item: this packet edits ONLY paths under `specs/024-design-doc-reconciliation/`. Path-limited `git add specs/024-design-doc-reconciliation/` enforces the boundary mechanically.
  Evidence: change-boundary contract documented in design.md "Change Boundary" section + spec.md AC-14
  ```
  $ git diff --cached --name-status | grep -vE '^[AM]\s+specs/024-design-doc-reconciliation/' | wc -l
  0
  ```

### Scenario-First TDD Evidence (BUG-024-004 gaps-sweep, 2026-06-06)

- BUG-024-004-SCN-001 (Top-level certifiedAt is present and matches OPS-001 sweep moment): RED before fix — `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); print(d.get('certifiedAt'))"` printed `None`; GREEN after fix — same probe prints `2026-05-28T05:07:51Z` (1s after OPS-001).
- BUG-024-004-SCN-002 (Gate G088 PASSES after the backfill): RED before fix — `post-cert-spec-edit-guard.sh` exit 2 with `G088 requires top-level certifiedAt`; GREEN after fix — exit 0 with `PASS Gate G088`.
- BUG-024-004-SCN-003 (state-transition-guard exits 0 after the backfill): RED before fix — STG exit 1, `1 failure(s), 2 warning(s)`, `🔴 TRANSITION BLOCKED`; GREEN after fix — STG exit 0, `0 failure(s), 2 warning(s)`, `🟢 TRANSITION ALLOWED`.
- BUG-024-004-SCN-004 (Parent governance backfill records BUG-024-004 closure): RED before fix — `executionHistory` had 25 entries with zero BUG-024-004 mentions; `resolvedBugs[]` may or may not contain prior bug entries; report.md had zero `## BUG-024-004` sections; GREEN after fix — `executionHistory` has ≥ 32 entries with ≥ 7 BUG-024-004 mentions; `resolvedBugs[]` contains a BUG-024-004 entry; report.md has exactly 1 `## BUG-024-004 Gaps-Sweep Resolution` section.
- BUG-024-004-SCN-005 (Single atomic commit with bubbles(024/bug-024-004) prefix + path discipline + runtime preserved): RED before fix — no commit on the topic; GREEN after fix — single commit subject `bubbles(024/bug-024-004): backfill top-level certifiedAt to satisfy Gate G088`; `git diff HEAD~1..HEAD --name-only` shows only `specs/024-design-doc-reconciliation/` paths; `TestConnectorCountContract` family stays 4/4 PASS pre-fix and post-fix.
