# Bug: BUG-024-001 — DoD scenario fidelity gap (SCN-024-02/03/05/06)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 024 — Design Document Reconciliation
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard reported 10 failures against `specs/024-design-doc-reconciliation`:

- Gate G057/G059: `scenario-manifest.json` was missing for the 6 Gherkin scenarios in `scopes.md`.
- Gate G068 (Gherkin → DoD Content Fidelity): 4 of 6 scenarios had no faithful matching DoD item — `SCN-024-02`, `SCN-024-03`, `SCN-024-05`, `SCN-024-06`. The pre-existing DoD bullets accurately described the delivered behavior but did not embed the `SCN-024-NN` trace ID required by the content-fidelity matcher.
- Test Plan row check: 4 of 6 scenarios (SCN-024-03/04/05/06) had no concrete test file path. The Manual rows describing each verification did not include the `docs/smackerel.md` path token.

## Reproduction (Pre-fix)

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -10
ℹ️  DoD fidelity: 6 scenarios checked, 2 mapped to DoD, 4 unmapped
❌ DoD content fidelity gap: 4 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
RESULT: FAILED (10 failures, 0 warnings)
```

## Gap Analysis (per scenario)

For each missing scenario the bug investigator inspected `docs/smackerel.md` (the single deliverable for spec 024). All four behaviors are genuinely **delivered-but-undocumented at the trace-ID level** — the docs reconciliation is complete; the only gap is that DoD bullets did not embed the `SCN-024-NN` ID that the guard uses for fidelity matching, and Test Plan rows did not contain the `docs/smackerel.md` path token.

| Scenario | Behavior delivered? | Concrete artifact | Concrete source | Verification |
|---|---|---|---|---|
| SCN-024-02 | Yes — §8 storage diagram and §14 DDL all reference PostgreSQL + pgvector with PostgreSQL types (JSONB, TIMESTAMPTZ, BOOLEAN, vector(384)); only line 2249 (Apple Notes' own SQLite DB) appears outside §4 SUPERSEDED block | `docs/smackerel.md:200,306,328,359,396,933,1360-1540` | `docs/smackerel.md` §8, §14 | `grep -cE 'JSONB\|TIMESTAMPTZ\|vector\(384\)' docs/smackerel.md` → 30 |
| SCN-024-03 | Yes — §3 mermaid diagrams remain unchanged with Go core + Python sidecar + PostgreSQL + NATS, zero OpenClaw components | `docs/smackerel.md:140-415` | `docs/smackerel.md` §3 | `awk '/^## 3\./{s=1} /^## 4\./{s=0} s' docs/smackerel.md \| grep -cE 'OpenClaw'` → 0; PostgreSQL/pgvector/NATS count → 50 |
| SCN-024-05 | Yes — §19 Gantt and Phase 1 table reference Docker Compose + PostgreSQL + pgvector + NATS JetStream and phases carry delivery markers (Phase 1 ✅, Phase 2 ✅, Phase 3 🔜, Phase 4 ✅, Phase 5 ✅) | `docs/smackerel.md` §19 | `docs/smackerel.md` §19 phase tables | §19 stack-token count → 4; delivery-marker count → 5 |
| SCN-024-06 | Yes — §22.7 inventory titled "Committed Connector Inventory (15 connectors)" lists all 15 committed connectors and marks Notion/Obsidian/Slack/Outlook as planned; IMAP-based email noted | `docs/smackerel.md:2261` | `internal/connector/` (15 packages) | `find internal/connector -maxdepth 1 -mindepth 1 -type d \| wc -l` → 15 |

**Disposition:** All four scenarios are **delivered-but-undocumented** — artifact-only fix. No production code touched.

## Acceptance Criteria

- [x] Parent `specs/024-design-doc-reconciliation/scopes.md` has DoD bullets that explicitly contain `SCN-024-02`, `SCN-024-03`, `SCN-024-05`, `SCN-024-06` with raw grep/find evidence and `docs/smackerel.md` source-file pointers
- [x] Parent `specs/024-design-doc-reconciliation/scenario-manifest.json` exists and covers all 6 scenarios with `scenarioId`, `linkedTests`, `evidenceRefs`, and `linkedDoD`
- [x] Test Plan rows for SCN-024-03/04/05/06 contain the `docs/smackerel.md` path token (concrete-file-path check)
- [x] Parent `specs/024-design-doc-reconciliation/report.md` carries a `BUG-024-001` cross-reference section with per-scenario classification
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-001-dod-scenario-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` PASS
- [x] No production code changed (boundary)
