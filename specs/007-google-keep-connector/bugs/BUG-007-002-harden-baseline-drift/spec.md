# Bug: BUG-007-002 — Harden baseline drift (state-transition-guard 13 BLOCKs)

## Classification

- **Type:** Artifact-only governance bug
- **Severity:** LOW (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 007 — Google Keep Connector
- **Workflow Mode:** harden-to-doc (child of stochastic-quality-sweep `sweep-2026-05-24-r10` round 7)
- **Status:** Fixed (artifact-only)

## Problem Statement

`bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector` returns `🔴 TRANSITION BLOCKED: 13 failure(s), 1 warning(s)` even though `state.json.status` is already `done` and all real Keep-connector behavior is implemented and covered by passing tests.

The 13 BLOCKs decompose into three governance-baseline-drift classes:

1. **(1) Commit-convention drift.** Full-delivery promotion requires at least one structured commit message for spec 007 with prefix `spec(007)` or `bubbles(007/...)`. Recent history does not include one.
2. **(1) Gate G040 deferral-language drift.** `report.md` accumulated three legitimate but flagged "Deferred"/"Future work" lines inside historical post-mortem sweep narratives (IMP-4 and IMP-5 rows in the 2026-04-14 Improve-Existing findings table, and H-R2-003 in the Round-N Documentary Observations).
3. **(10 + 1 aggregate) Gate G068 DoD-Gherkin content fidelity drift.** Ten Gherkin scenarios in `scopes.md` (`SCN-GK-003`, `005`, `008`, `010`, `012`, `019`, `020`, `024`, `027`, `030`) have no DoD item that meets the v3.8.0 G068 fuzzy-match threshold (≥3 significant-word overlap AND ≥50% of scenario significant words). Each scenario's underlying behavior is implemented and tested, but the DoD vocabulary drifted from the locked scenario titles, so the guard reports content fidelity gaps.

This is the harden probe finding for round 7 of `sweep-2026-05-24-r10`. The probe was dispatched by the parent `bubbles.workflow` stochastic sweep against `specs/007-google-keep-connector` with trigger `harden` and mapped child mode `harden-to-doc`.

## Reproduction (Pre-fix)

```
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector 2>&1 | grep -cE "^🔴 BLOCK"
13

$ bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector 2>&1 | grep -E "^🔴 BLOCK"
🔴 BLOCK: full-delivery requires at least one structured commit message for spec 007 (expected prefix: spec(007) or bubbles(007/...)
🔴 BLOCK: Report artifact contains 3 deferral language hit(s): report.md — evidence of deferred work (Gate G040)
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 01: Takeout Parser & Normalizer — scenario has no faithful DoD item: SCN-GK-003 Cursor-based filtering skips old notes
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 01: Takeout Parser & Normalizer — scenario has no faithful DoD item: SCN-GK-005 Corrupted JSON files produce partial results
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 02: Keep Connector, Config & Registry — scenario has no faithful DoD item: SCN-GK-008 Takeout sync produces artifacts in database
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 02: Keep Connector, Config & Registry — scenario has no faithful DoD item: SCN-GK-010 Trashed note archives existing artifact
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 03: Source Qualifiers & Processing Tiers — scenario has no faithful DoD item: SCN-GK-012 Full qualifier engine evaluation order
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 03: Source Qualifiers & Processing Tiers — scenario has no faithful DoD item: SCN-GK-030 Recently-archived note gets light tier despite recency
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 04: Label-to-Topic Mapping — scenario has no faithful DoD item: SCN-GK-019 Fuzzy match via pg_trgm handles variations
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 04: Label-to-Topic Mapping — scenario has no faithful DoD item: SCN-GK-020 Label removal deletes BELONGS_TO edge
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 05: gkeepapi Python Bridge — scenario has no faithful DoD item: SCN-GK-024 gkeepapi session caching avoids re-authentication
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 06: Image OCR Pipeline — scenario has no faithful DoD item: SCN-GK-027 Tesseract failure falls back to Ollama vision
🔴 BLOCK: 10 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of spec (Gate G068)
```

## Gap Analysis

| Finding | Class | Disposition |
|---|---|---|
| Commit convention | Governance | Naturally resolved by committing this bug-packet fix with prefix `bubbles(007/bug-007-002-harden-baseline-drift): ...`. |
| 3 deferral-language hits at `report.md:334`, `:335`, `:702` | Historical post-mortem narrative | Wrap each in `<!-- bubbles:g040-skip-begin --> ... <!-- bubbles:g040-skip-end -->` per state-transition-guard.sh CHECK 18's documented escape hatch. The records are intentionally preserved verbatim — they document past sweep outcomes — and the guard already excludes content between these HTML-comment markers from the deferral scan. |
| 10 SCN-GK-NNN scenarios lacking faithful DoD overlap | Locked-scenario vocabulary drift | Add one Scenario Fidelity DoD item per affected scope that embeds the `SCN-GK-NNN` identifier and the full scenario title verbatim, with an Evidence line pointing to the existing passing test. The underlying behavior is already implemented and tested; this only restores the v3.8.0 G068 fuzzy-match overlap. |

**Disposition:** Delivered-but-undocumented governance drift — artifact-only fix. No production code change required. No locked scenario IDs invalidated, no DoD evidence rewritten, no test classification changed.

## Acceptance Criteria

- [x] `bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector` returns 0 BLOCKs
- [x] All 10 G068 fidelity gaps cleared via additive Scenario Fidelity DoD items (one per affected scope, mapping each SCN-GK-NNN to existing passing tests) — no existing DoD item rewritten
- [x] All 3 G040 deferral-language hits cleared by wrapping the historical post-mortem narrative with `<!-- bubbles:g040-skip-begin --> ... <!-- bubbles:g040-skip-end -->` markers — historical record preserved verbatim
- [x] Commit-convention BLOCK cleared by committing this fix with prefix `bubbles(007/bug-007-002-harden-baseline-drift): ...`
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector/bugs/BUG-007-002-harden-baseline-drift` PASS
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix (boundary)
- [x] No locked scenario ID in `state.json.certification.lockdownState.lockedScenarioIds` was invalidated
