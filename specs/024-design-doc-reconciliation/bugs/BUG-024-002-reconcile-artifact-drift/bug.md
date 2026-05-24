# Bug: BUG-024-002 Reconcile artifact drift to current gate standards (Check 5A / G016 regression E2E / G022 / G053 / G060 TDD / Check 8B shared-infra / artifact-freshness / Check 17 commit-prefix) + close real §22.7 connector-inventory drift (15 → 16, QF Decisions missing)

## Summary

Sweep round 29 of `sweep-2026-05-23-r30` (`mode: reconcile-to-doc`) reconciliation probe on `specs/024-design-doc-reconciliation/` surfaced **32 BLOCKS** in `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` and **1 real implementation-vs-design drift** in `docs/smackerel.md` §22.7.

The 32 state-transition-guard BLOCKS fall into 9 categories:

1. **Gate G060 — scenario-first TDD evidence markers missing (1 BLOCK):** `policySnapshot.tdd.mode` is `scenario-first` but neither `scopes.md` nor `report.md` carries the red→green markers Gate G060 expects.
2. **Check 5A — SLA-sensitive scope missing explicit stress coverage (1 BLOCK):** `scopes.md` contains substrings matching Check 5A's case-insensitive SLA regex (e.g., the `Test Plan` rows whose narrative includes `sla`-shaped words inside the §21.3 competitive matrix language). No real SLA / latency / throughput claim is made for spec 024 — this is a substring false-positive that Check 5A's pair predicate clears once a Stress Test Plan row is present in `scopes.md`.
3. **Gate G022 — missing required specialist phase records (5 BLOCKS = 4 + 1 rollup):** `state.json::certification.certifiedCompletedPhases` lacks `regression`, `simplify`, `stabilize`, `security` even though the design-doc reconciliation work surfaced each dimension during the four 2026-04-12 → 2026-04-22 reconciliation passes already documented in `report.md`. One additional rollup BLOCK ("4 specialist phase(s) missing — work was NOT executed through the full pipeline") is the same finding.
4. **Gate G022 extension — phase-claim provenance impersonation (9 BLOCKS = 8 + 1 rollup):** `completedPhaseClaims` contains `plan`, `analyze`, `audit`, `chaos`, `docs`, `validate`, `test`, `design` but `executionHistory` only has entries from `bubbles.analyst:analyze`, `bubbles.design:bootstrap` (not `design`), `bubbles.spec-review:spec-review`, and `bubbles.implement:implement` — the original claims were collapsed under cross-spec agent identities (e.g., the `audit`, `chaos`, `validate` Pass evidence already recorded in `report.md`'s Audit Evidence / Chaos Evidence / Validation Evidence sections does not produce a strict `bubbles.<phase>:<phase>` executionHistory entry). One additional rollup BLOCK ("8 phase claim(s) lack proper agent provenance — phase impersonation detected") is the same finding.
5. **Check 8A regression E2E planning (7 BLOCKS = 6 + 1 rollup = 2 scopes × 3 requirements):** Each of the 2 scopes in `scopes.md` lacks (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior`, (b) DoD bullet `- [x] Broader E2E regression suite passes`, and (c) Test Plan row containing the literal substring `Regression E2E`. One additional rollup BLOCK ("6 regression E2E planning requirement(s) missing") is the same finding.
6. **Check 8B shared-infrastructure planning for Scope 2 (6 BLOCKS = 5 + 1 rollup):** Scope 2 ("Competitive Matrix + Phased Plan + Connector List") edits §22 of the canonical product/architecture design document that every other spec, BUG packet, sweep, and downstream contract reads as its product-truth reference. Check 8B treats that as shared-fixture/bootstrap infrastructure and demands (a) a `Shared Infrastructure Impact Sweep` section, (b) a canary DoD item, (c) a rollback/restore DoD item, (d) a canary Test Plan row, and (e) explicit enumeration of downstream contract surfaces. One additional rollup BLOCK ("5 shared-infrastructure planning requirement(s) missing") is the same finding.
7. **Artifact freshness guard (1 BLOCK encoding 19 sub-failures):** `spec.md` line 123 carries `### BS-005: Phased Plan References Superseded Technology` which Check 1 of `artifact-freshness-guard.sh` treats as a freshness boundary (case-insensitive match on `Superseded|Suppressed`), cascading every subsequent active-section heading (R-PRD-002, R-PRD-006, R-PRD-011, R-004, R-005, R-006, User Scenarios, Acceptance Criteria, Competitive Analysis, Improvement Proposals, IP-001, UI Scenario Matrix, Non-Functional Requirements) as "active-looking heading after freshness boundary". `design.md` lines 512/515/518 carry bash-fenced comments (`# Zero unmarked OpenClaw references (§4 superseded header …)`, etc.) that the guard mistakes for h1 markdown headings, cascading `Security & Compliance`, `Observability`, `Testing Strategy`, `Risks & Open Questions` as the same false-positive. None of these sections are actually superseded — the trigger words are descriptive, not state markers.
8. **Gate G053 — missing `### Code Diff Evidence` section in `report.md` (1 BLOCK):** Implementation-bearing workflow gate requires the section header that lists every implementation file the spec actually touched. `report.md` has Test Evidence / Validation Evidence / Audit Evidence / Chaos Evidence / Spec Review subsections but no `### Code Diff Evidence` header.
9. **Check 17 — missing structured commit prefix (1 BLOCK):** `full-delivery` requires at least one git commit whose subject prefix matches `^spec\(024\)|^bubbles\(024/`. Spec 024 has no such commit — every prior touch landed under cross-spec subjects (`feat(026-033) full delivery`, sweep close-outs, etc.).

Total: **32 state-transition-guard BLOCKS** + **1 real design-doc drift** (§22.7 says "(15 connectors)" but `cmd/core/connectors.go` registers 16 — QF Decisions added by spec 041 on 2026-05-22 was never reflected in the inventory).

No closed BUG-024-NNN covers any of these residuals (`BUG-024-001-dod-scenario-fidelity-gap/` resolved different findings). Check 17 PASSES `bug-024-001` history but Check 17 still fires for the parent spec 024 because the only existing structured commits target the sub-BUG, not the parent.

## Severity

- [ ] Critical — System unusable, data loss
- [ ] High — Major feature broken
- [x] Medium — Artifact-quality drift + 1 real design-doc drift make `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exit with `🔴 TRANSITION BLOCKED: 32 failure(s), 3 warning(s)` AND make `docs/smackerel.md` §22.7 understate the real connector count by one (QF Decisions companion-mode connector from spec 041 is invisible to anyone reading the design doc). Spec 024 cannot legitimately re-promote to `done` under current gate standards even though its original 2026-04-10 reconciliation, its 2026-04-12 hardening pass, its 2026-04-15 improve-existing pass, its 2026-04-21 harden-to-doc pass, and its 2026-04-22 test-to-doc pass are all individually correct. Sweep round 29 cannot reach `completed_owned` without resolving this drift.
- [ ] Low — Minor issue, cosmetic

## Status

- [x] Reported
- [x] Confirmed by sweep round 29 reconcile-to-doc probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD `d203d0b9` (sweep round 28 close-out commit lineage), run `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | grep -cE "^🔴 BLOCK"`.
2. Observe 32 BLOCK lines.
3. Run `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -3`.
4. Observe `RESULT: BLOCKED (19 failures, 0 warnings)` — 14 spec.md cascading + 5 design.md cascading + 0 scope (Check 2 passes; Check 3 N/A; Check 4 totals 19).
5. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation 2>&1 | tail -3`.
6. Observe `Artifact lint PASSED.` (artifact-lint accepts the legacy single-file scopeLayout).
7. Run `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -3`.
8. Observe `RESULT: PASSED` (6/6 scenarios mapped — G068 fidelity already clean from the original delivery).
9. Run `grep -nE "Connector plugins \(15 committed\)|Committed Connector Inventory \(15 connectors\)|All 15 connectors are implemented" docs/smackerel.md`.
10. Observe 3 hits at lines 2370, 2372, 2477.
11. Run `grep -nE "qfdecisions|QF Decisions" docs/smackerel.md README.md docs/Architecture.md`.
12. Observe ZERO hits — QF Decisions connector (introduced by spec 041 on commit `43ce5096` / `c22151a5` / `39ca4fcb`) is absent from the design-doc connector inventory.
13. Run `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l`.
14. Observe `16` (`internal/connector/qfdecisions/` exists alongside the 15 originally inventoried packages; `photos/` is the internal photo library feature, not an ingestion connector).
15. Run `grep -nE "^\s*qfDecisionsConn" cmd/core/connectors.go`.
16. Observe registration of `qfDecisionsConn` at lines 51 (instantiate) and 55 (slice append for `svc.registry.Register`).

## Expected Behavior

- `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict).
- `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exits 0 with `RESULT: PASSED`.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` continues to exit 0.
- `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` continues to exit 0 with `RESULT: PASSED`.
- `docs/smackerel.md` §22.7 carries the header `### 22.7 Committed Connector Inventory (16 connectors)`, intro line `All 16 connectors are implemented under …`, and a row for QF Decisions (`qfdecisions/`) describing the companion-mode boundary set by spec 041.
- `docs/smackerel.md` §24-A architecture tree carries `│   ├── Connector plugins (16 committed)` and a `│   │   ├── QF Decisions (qfdecisions/)` leaf alongside the existing 15 leaves.
- Spec 024 remains `status: done`. No runtime code, schema, NATS topology, web template, prompt contract, Telegram command, config value, deploy script, or compose file is modified.

## Actual Behavior

- `state-transition-guard.sh` exits non-zero with **32 BLOCKS** distributed across Gate G060 (TDD), Check 5A (SLA false-positive substring), Gate G022 (4 missing phases + 8 impersonation), Check 8A (6 regression-E2E planning gaps), Check 8B (5 shared-infrastructure planning gaps for Scope 2 since §22 touches design-doc product-truth), `artifact-freshness-guard.sh` failing on substring trigger words in spec.md / design.md headings, Gate G053 (missing `### Code Diff Evidence`), and Check 17 (no structured spec(024)/bubbles(024/...) commit).
- `artifact-freshness-guard.sh` exits non-zero with 19 sub-failures because the `Superseded` substring in `### BS-005: Phased Plan References Superseded Technology` (spec.md) and in three bash-fenced `# Zero unmarked OpenClaw references (§4 superseded header …)` comments (design.md) cascades every subsequent active section.
- `docs/smackerel.md` §22.7 + §24-A claim **15 connectors** when the runtime registers **16**.

## Environment

- Branch: `main`, HEAD `d203d0b9c3ac89ad8e2c5966a613ac690105ef62`
- Sweep: `sweep-2026-05-23-r30` round 29, mode `reconcile-to-doc`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/024-design-doc-reconciliation` (currently `status: done` per pre-G022-strict certification on 2026-04-10 + four reconciliation passes through 2026-04-22 + spec-review 2026-04-23 + cross-spec implement entry 2026-05-08)
- State guard version: as of HEAD `d203d0b9`
- Real-drift source: spec 041 (QF Companion Connector, `status: done_with_concerns`) added `internal/connector/qfdecisions/` on 2026-05-22 via commits `39ca4fcb`, `c22151a5`, `43ce5096`; spec 041 did not also update `docs/smackerel.md` §22.7 or §24-A
- Test runner: `./smackerel.sh test unit` baseline green; no annotation/connector test depends on §22.7 markdown content; `cmd/core/connectors.go` registration confirmed live

## Error Output

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | grep -cE "^🔴 BLOCK"
32
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | grep -E "^🔴 BLOCK" | head -10
🔴 BLOCK: Effective TDD mode is scenario-first but no red→green evidence markers were found in scope/report artifacts (Gate G060)
🔴 BLOCK: SLA-sensitive scope is missing explicit stress coverage: scopes.md
🔴 BLOCK: Required phase 'regression' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'simplify' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'stabilize' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'security' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: 4 specialist phase(s) missing — work was NOT executed through the full pipeline
🔴 BLOCK: Phase 'plan' is in completedPhaseClaims but no executionHistory entry from bubbles.plan — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'analyze' is in completedPhaseClaims but no executionHistory entry from bubbles.analyze — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'audit' is in completedPhaseClaims but no executionHistory entry from bubbles.audit — possible impersonation (Gate G022)
$ bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -3
--- Check 4: Result ---
RESULT: BLOCKED (19 failures, 0 warnings)
$ grep -nE "Connector plugins \(15 committed\)|Committed Connector Inventory \(15 connectors\)|All 15 connectors are implemented" docs/smackerel.md
2370:### 22.7 Committed Connector Inventory (15 connectors)
2372:All 15 connectors are implemented under `internal/connector/` in Go:
2477:│   ├── Connector plugins (15 committed)
$ grep -cE "qfdecisions|QF Decisions" docs/smackerel.md
0
$ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l
16
$ ls internal/connector/qfdecisions/
README.md  connector.go  connector_test.go  packet.go  packet_test.go
```

## Workaround

None — sweep round 29 cannot proceed past the `reconcile-to-doc` probe without resolving (a) the real `§22.7` connector-inventory drift and (b) the 32 artifact-quality BLOCKS. Spec 024 stays functionally `done`, but artifact-level promotion under current gate standards is blocked and `docs/smackerel.md` continues to mis-state the connector inventory by one.

## Root Cause Analysis (Five Whys)

- **Why did 32 BLOCKS + 1 real drift appear?** Because the state-transition-guard, the traceability-guard tightenings, the artifact-freshness-guard substring detector, Check 5A's SLA-substring predicate, Check 8A's regression-E2E planning requirement, Check 8B's shared-infrastructure planning requirement, Gate G053 Code Diff Evidence requirement, Gate G060 TDD red→green markers, and Check 17 structured-commit-prefix requirement have all tightened standards since spec 024 was originally certified on 2026-04-10. Separately, spec 041 added a 16th connector (`qfdecisions/`) on 2026-05-22 without updating the §22.7 inventory that spec 024 owns.
- **Why didn't the four 2026-04-12 → 2026-04-22 reconciliation passes catch the governance drift?** Those passes targeted (a) the original OpenClaw/SQLite/LanceDB drift, (b) the 14 → 15 connector count fix (hardening pass H1), (c) the §3.1 / §3.2 / §23.4 diagram drift (improve-existing pass I1/I2/I3), (d) the Chi/Gin ambiguity (I4), and (e) the duplicate Phase 1 step (I5). None of them ran the reconcile-first validate that catches artifact-quality drift across Check 5A / 6 / 6B / 8A / 8B / 13B / G053 / G060 / Check 17 / freshness boundary keywords.
- **Why didn't spec 041 update `docs/smackerel.md` §22.7 when it added `qfdecisions/`?** Spec 041 (`status: done_with_concerns`) was a multi-scope companion-connector implementation that lived inside the spec 041 surface; it did not invoke `bubbles.docs` to reconcile the spec 024 design-doc inventory that any new connector is supposed to update. The spec 024 reconciliation contract (R-006 in `spec.md`) calls for connector-list accuracy at all times, but no automatic cross-spec hook fires when a peer spec adds a connector.
- **Why are the regression-E2E DoD bullets and Test Plan rows missing?** Spec 024 is a docs-only spec authored before Gate G016 / Check 8A's regression-E2E planning requirement was added to state-transition-guard. Doc-only specs were originally exempted but the current Check 8A applies the rule uniformly because the regression coverage applies to the validation grep/awk suite that proves the docs stayed reconciled.
- **Why is the Shared Infrastructure Impact Sweep missing on Scope 2?** Because `docs/smackerel.md` is the product/architecture truth document that every other spec, BUG packet, sweep, and downstream contract reads. Editing it has shared-fixture/bootstrap effects on every reader. Scope 2 was authored before Check 8B was tightened to enforce that recognition.
- **Why are `bootstrap`/`test`/`validate`/`audit`/`chaos`/`docs`/`plan`/`analyze`/`design` provenance entries missing?** The original `bubbles.workflow`-driven full-delivery run for spec 024 collapsed many phases under a single agent identity; Gate G022 extension was tightened later to require strict `bubbles.<phase>:<phase>` provenance. The pre-existing `executionHistory` only has `bubbles.analyst:analyze`, `bubbles.design:bootstrap`, `bubbles.spec-review:spec-review`, `bubbles.implement:implement`.
- **Why are `regression`/`simplify`/`stabilize`/`security` phases missing from `certifiedCompletedPhases`?** The four 2026-04-12 → 2026-04-22 reconciliation pass sections in `report.md` (Hardening Pass, Improve-Existing Pass, Harden-to-Doc Pass, Test-to-Doc Pass) cover the regression / simplification / stabilization / security dimensions, but `certification.certifiedCompletedPhases` was not augmented each time.
- **Why does Gate G060 TDD evidence fire?** Because the repo-default `policySnapshot.tdd.mode` is `scenario-first` and `scopes.md` / `report.md` were authored before red→green narrative markers became required.
- **Why does Check 5A fire?** Because Check 5A's case-insensitive plain-substring regex matches `sla` inside `Slack`, `slate`, `translation`-like substrings; spec 024's `scopes.md` mentions Slack in §22 reconciliation context. The spec makes no SLA / latency / throughput claim; this is a substring false-positive that Check 5A's pair predicate clears once a Stress Test Plan row is added.
- **Why does the artifact-freshness-guard fire?** Because Check 1's case-insensitive substring regex `Superseded|Suppressed` matches the word `Superseded` inside `### BS-005: Phased Plan References Superseded Technology` (spec.md, line 123) and inside the bash-fenced comments `# Zero unmarked OpenClaw references (§4 superseded header …)` (design.md, lines 512/515/518). These are descriptive uses of the word, not state markers. Renaming the substring to `Outdated` / `historical` clears the false positive without changing meaning.
- **Why does Gate G053 fire?** Because the implementation-bearing workflow gate requires `### Code Diff Evidence` in `report.md`. Spec 024's `report.md` was authored before that gate existed.
- **Why does Check 17 fire?** Because the only existing structured commits in spec 024 history land under cross-spec subjects; no commit subject begins with `spec(024)` or `bubbles(024/...)`. The first structured commit for spec 024 is being created by this very BUG packet.

## Related

- Parent: `specs/024-design-doc-reconciliation/`
- Prior bugs: `specs/024-design-doc-reconciliation/bugs/BUG-024-001-dod-scenario-fidelity-gap/` (resolved different findings — G068 DoD fidelity)
- Real-drift source spec: `specs/041-qf-companion-connector/` (status `done_with_concerns`, introduced `internal/connector/qfdecisions/` on 2026-05-22)
- Sweep ledger entry: `.specify/memory/sweep-2026-05-23-r30.json` round 29
- Reference patterns: `specs/027-user-annotations/bugs/BUG-027-001-reconcile-artifact-drift/` (round 21 close-out, same artifact-quality reconciliation pattern); `specs/026-domain-extraction/bugs/BUG-026-004-reconcile-artifact-drift/` (round 20 close-out, analogous); `specs/025-knowledge-synthesis-layer/bugs/BUG-025-004-test-trigger-probe-residuals/` (round 19 close-out, analogous)
