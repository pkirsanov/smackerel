# Report: BUG-024-001 — DoD Scenario Fidelity Gap

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard reported 10 failures against `specs/024-design-doc-reconciliation`: the missing `scenario-manifest.json` (Gates G057/G059), 4 Test Plan rows lacking a concrete file path (SCN-024-03/04/05/06), and 4 DoD content-fidelity gaps under Gate G068 (`SCN-024-02`, `SCN-024-03`, `SCN-024-05`, `SCN-024-06`). Investigation confirmed the gap is artifact-only — every scenario is fully delivered in the reconciled `docs/smackerel.md` (and, for SCN-024-06, additionally evidenced by the `internal/connector/` directory carrying 15 packages). The DoD bullets simply did not embed the `SCN-024-NN` trace IDs that the guard's content-fidelity matcher requires, and the Manual Test Plan rows for SCN-024-03/04/05/06 did not include the `docs/smackerel.md` path token that the concrete-test-file check needs.

The fix added 4 trace-ID-bearing DoD bullets to `specs/024-design-doc-reconciliation/scopes.md` (SCN-024-02 and SCN-024-03 in Scope 1; SCN-024-05 and SCN-024-06 in Scope 2), embedded the `docs/smackerel.md` path token in the Manual Test Plan rows for SCN-024-03/04/05/06, generated `specs/024-design-doc-reconciliation/scenario-manifest.json` covering all 6 `SCN-024-*` scenarios, and appended a cross-reference section to `specs/024-design-doc-reconciliation/report.md`. No production code was modified; the boundary clause in the user prompt was honored.

## Completion Statement

All 9 DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The traceability-guard's pre-fix state (10 failures, 4 unmapped scenarios) has been replaced with a clean `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` invocations (parent and bug folder) succeed. No code-path or doc-path file outside `specs/024-design-doc-reconciliation/` was modified.

## Test Evidence

### Underlying behavior verification (regression-protection for the artifact fix)

Spec 024 is doc-only — its single deliverable is `docs/smackerel.md`. The verification commands embedded in the new DoD bullets re-execute the same checks the original spec author used:

```
$ awk '/^## 3\./{s=1} /^## 4\./{s=0} s' docs/smackerel.md | grep -cE 'OpenClaw'
0
$ awk '/^## 3\./{s=1} /^## 4\./{s=0} s' docs/smackerel.md | grep -cE 'PostgreSQL|pgvector|NATS'
50
$ grep -cE 'JSONB|TIMESTAMPTZ|vector\(384\)' docs/smackerel.md
30
$ awk '/^## 19\./{s=1} /^## 20\./{s=0} s' docs/smackerel.md | grep -cE 'Docker Compose|PostgreSQL \+ pgvector|NATS JetStream'
4
$ awk '/^## 19\./{s=1} /^## 20\./{s=0} s' docs/smackerel.md | grep -cE 'Delivered|In Progress'
5
$ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l
15
$ grep -nE 'Committed Connector Inventory.*15' docs/smackerel.md
2261:### 22.7 Committed Connector Inventory (15 connectors)
$ grep -nE 'PostgreSQL \+ pgvector' docs/smackerel.md | head -5
200:        PG[(PostgreSQL + pgvector)]
306:        D1[PostgreSQL + pgvector]
328:    participant PG as PostgreSQL + pgvector
359:    participant PG as PostgreSQL + pgvector
396:    participant PG as PostgreSQL + pgvector
$ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /SQLite|LanceDB/{print NR": "$0}' docs/smackerel.md
2249: | Apple Notes | ❌ Not viable | — | — | — | No public API. Locked to macOS SQLite DB. ...
```

**Claim Source:** executed.

### Validation Evidence

> Phase agent: bubbles.validate
> Executed: YES

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -15
✅ Scope 2: Competitive Matrix + Phased Plan + Connector List scenario maps to DoD item: SCN-024-04 Competitive matrix distinguishes implemented vs planned
✅ Scope 2: Competitive Matrix + Phased Plan + Connector List scenario maps to DoD item: SCN-024-05 Phased plan reflects actual technology and delivery state
✅ Scope 2: Competitive Matrix + Phased Plan + Connector List scenario maps to DoD item: SCN-024-06 Connector ecosystem accurately lists all 15 connectors
ℹ️  DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 6
ℹ️  Test rows checked: 15
ℹ️  Scenario-to-row mappings: 6
ℹ️  Concrete test file references: 6
ℹ️  Report evidence references: 6
ℹ️  DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)

RESULT: PASSED (0 warnings)
```

**Claim Source:** executed.

### Audit Evidence

> Phase agent: bubbles.audit
> Executed: YES

Captured at end of bug remediation. See `### Final Verification` section below for the canonical paste recorded in the closing run.

## Pre-fix Reproduction

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -10
ℹ️  DoD fidelity: 6 scenarios checked, 2 mapped to DoD, 4 unmapped
❌ DoD content fidelity gap: 4 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

--- Traceability Summary ---
ℹ️  Scenarios checked: 6
ℹ️  Test rows checked: 15
ℹ️  Scenario-to-row mappings: 6
ℹ️  Concrete test file references: 2
ℹ️  Report evidence references: 2
ℹ️  DoD fidelity scenarios: 6 (mapped: 2, unmapped: 4)

RESULT: FAILED (10 failures, 0 warnings)
```

**Claim Source:** executed (initial guard invocation captured at the start of this bug investigation, before any artifact edits).

## Final Verification

> Phase agent: bubbles.audit
> Executed: YES

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation 2>&1 | tail -10
✅ Required specialist phase 'test' recorded in execution/certification phase records
✅ Required specialist phase 'docs' recorded in execution/certification phase records
✅ Required specialist phase 'validate' recorded in execution/certification phase records
✅ Required specialist phase 'audit' recorded in execution/certification phase records
✅ Required specialist phase 'chaos' recorded in execution/certification phase records
✅ Spec-review phase recorded for 'full-delivery' (specReview enforcement)

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-001-dod-scenario-fidelity-gap 2>&1 | tail -10
✅ All 3 evidence blocks in report.md contain legitimate terminal output
✅ No narrative summary phrases detected in report.md
✅ Required specialist phase 'implement' recorded in execution/certification phase records
✅ Required specialist phase 'test' recorded in execution/certification phase records
✅ Required specialist phase 'validate' recorded in execution/certification phase records
✅ Required specialist phase 'audit' recorded in execution/certification phase records

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -15
✅ Scope 1: OpenClaw Reconciliation + Storage Layer Update scenario maps to DoD item: SCN-024-03 System architecture section is verified unchanged
✅ Scope 2: Competitive Matrix + Phased Plan + Connector List scenario maps to DoD item: SCN-024-04 Competitive matrix distinguishes implemented vs planned
✅ Scope 2: Competitive Matrix + Phased Plan + Connector List scenario maps to DoD item: SCN-024-05 Phased plan reflects actual technology and delivery state
✅ Scope 2: Competitive Matrix + Phased Plan + Connector List scenario maps to DoD item: SCN-024-06 Connector ecosystem accurately lists all 15 connectors
ℹ️  DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 6
ℹ️  Test rows checked: 15
ℹ️  Scenario-to-row mappings: 6
ℹ️  Concrete test file references: 6
ℹ️  Report evidence references: 6
ℹ️  DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)

RESULT: PASSED (0 warnings)
```

```
$ git status --short | grep "024-design-doc-reconciliation"
 M specs/024-design-doc-reconciliation/report.md
 M specs/024-design-doc-reconciliation/scopes.md
?? specs/024-design-doc-reconciliation/bugs/
?? specs/024-design-doc-reconciliation/scenario-manifest.json
```

**Claim Source:** executed. Boundary preserved: zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or `docs/` for spec 024. All edits confined to `specs/024-design-doc-reconciliation/*` and the bug folder.
