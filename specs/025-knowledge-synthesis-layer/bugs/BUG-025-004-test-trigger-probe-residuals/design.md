# Design: BUG-025-004 Test trigger probe quality residuals

Links: [bug.md](bug.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Current Truth (brownfield evidence captured 2026-05-24 sweep round 19)

- `specs/025-knowledge-synthesis-layer/scopes.md` and `specs/025-knowledge-synthesis-layer/scenario-manifest.json` are the only spec artifacts touched by this packet.
- `internal/pipeline/synthesis_subscriber_test.go` line 103 is the only file with the `Flailed` typo (confirmed by `grep -rn "Flailed" --include="*.go" .`).
- `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` currently exits non-zero with 3 failures (2 G068 + 1 rollup).
- `go test ./internal/knowledge/... ./internal/api/... ./internal/intelligence/... ./internal/pipeline/... ./internal/telegram/...` is green at HEAD `96ad78f3`.
- `python3 -m pytest ml/tests/test_synthesis.py -q` reports `17 passed`.
- 13 stale `linkedTests` entries are listed in `bug.md`. Each renamed target test has been confirmed to exist via `grep -n "^func Test" <file>` or `grep -n "^def test" <file>`.

## Goal

Restore three artifact-quality invariants without changing any runtime behavior:

1. G068 DoD-Gherkin fidelity passes for `specs/025-knowledge-synthesis-layer/`.
2. Every `linkedTests` entry in `specs/025-knowledge-synthesis-layer/scenario-manifest.json` resolves to a real `func Test…` / `def test_…` definition in the file named by the same manifest entry.
3. The misspelled test function `…FailureMarksFlailed` is renamed to `…FailureMarksFailed` with no behavioral change.

## Non-Goals

- No change to `internal/knowledge/`, `internal/pipeline/`, `internal/api/`, `internal/telegram/`, `internal/web/`, `internal/digest/`, `internal/scheduler/`, `ml/app/synthesis.py`, or any runtime path beyond the single test function rename.
- No change to `.github/bubbles/scripts/traceability-guard.sh` (framework-managed file — immutable per repo policy). The trace guard already catches the G068 failures; it just does not yet validate `scenario-manifest.json::linkedTests` existence. That meta-improvement belongs in the framework repo, not here.
- No change to `specs/055-notification-source-ntfy-adapter/` or any other in-flight WIP.
- No re-opening of parent spec 025 certification. The parent stays `done`; this packet is a bug under it.

## Approach

Three independent scopes, executed in dependency order:

### Scope 1 — Rename test function typo

- Edit `internal/pipeline/synthesis_subscriber_test.go` line 103: `TestSynthesisExtractResponse_FailureMarksFlailed` → `TestSynthesisExtractResponse_FailureMarksFailed`.
- No body change. No assertion change. No reference exists anywhere else in the repository.
- Verify with `go test -run TestSynthesisExtractResponse_FailureMarksFailed ./internal/pipeline/...` and `grep -rn "Flailed" --include="*.go" .` returning zero matches.

### Scope 2 — Refresh scenario-manifest linkedTests

- Edit `specs/025-knowledge-synthesis-layer/scenario-manifest.json` to replace 13 stale `linkedTests` entries with current function names. Use the mapping table in `bug.md` → "Error Output" section.
- One entry (`SCN-025-21 `TestHandleFind_KnowledgeMatch` in `internal/telegram/bot_test.go`) requires updating BOTH the `linkedTests[]` and the `file` field, because the test moved to `internal/telegram/knowledge_test.go::TestHandleFind_WithKnowledgeMatch`.
- One entry (`SCN-025-15 `TestRetrySynthesisDecisionLogic` appearing twice in the same scenario) is collapsed to a single distinct test `TestClassifySynthesisRetry_BoundaryValues`, leaving `TestRetrySynthesisBacklog_MaxRetriesAbandoned` untouched (already valid).
- One entry (`SCN-025-06 `TestHandleSynthesized_Failure`) becomes `TestSynthesisExtractResponse_FailureMarksFailed` (after Scope 1 rename — that is why Scope 1 runs first).
- Final manifest has 0 missing `linkedTests`.
- Verify with the Python cross-check shown in `bug.md`.

### Scope 3 — Restore DoD-Gherkin fidelity

- Edit `specs/025-knowledge-synthesis-layer/scopes.md` Scope 2 "Definition of Done" to add a checked DoD line that faithfully preserves the Gherkin claim for SCN-025-05:
  > `- [x] Incremental concept page update preserves existing knowledge (claims appended not replaced, prior source_artifact_ids retained)`
  with `> **Phase:** implement` and `> **Evidence:**` referencing the upsert.go append behavior and `TestAddUnique` / `TestEnforceTokenCap_PreservesNewest` regressions.
- Edit `specs/025-knowledge-synthesis-layer/scopes.md` Scope 5 "Definition of Done" to add a checked DoD line that faithfully preserves the Gherkin claim for SCN-025-14:
  > `- [x] Lint detects contradictions and emits a high-severity finding referencing both claims and source artifacts`
  with `> **Phase:** implement` and `> **Evidence:**` referencing the existing `checkContradictions` implementation and `TestCheckContradictions_FindingShape` regression.
- These additions are evidence-cited and reflect already-shipped runtime behavior; they do not weaken or replace any existing DoD item.
- Verify with `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exit 0.

## Risks

- **Risk:** New DoD lines could be flagged as "DoD rewritten to match delivery" by future audits. *Mitigation:* both new lines use the Gherkin's own vocabulary first ("Incremental concept page update preserves existing knowledge", "Lint detects contradictions and emits a high-severity finding…"), then add evidence pointers — they restore fidelity, they do not rewrite delivery into the spec.
- **Risk:** Renaming the test could break a CI filter that pattern-matches the misspelled name. *Mitigation:* confirmed zero references via `grep -rn "Flailed"`; the rename touches only the function header.
- **Risk:** Scenario-manifest refresh could mismatch the real test file again after future renames. *Mitigation:* this packet does not add a manifest-validation guard (framework-owned), so the residual risk remains until a framework-side guard ships. Captured as a known follow-up.

## Out-of-Scope Follow-Ups (do NOT promote in this packet)

- Add a framework guard that validates `scenario-manifest.json::linkedTests[*]` resolves to `func Test…` / `def test_…` definitions in the file named by the same manifest entry. Belongs to the framework repo (`bubbles/scripts/`).
- Consider whether `TestEnforceTokenCap_PreservesNewest` is a stronger SCN-025-05 cover than `TestAddUnique` alone. Out of scope; both are valid covers and the manifest will list both.

## Testing Strategy

- Re-run `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` and expect exit 0.
- Re-run the Python `linkedTests` cross-check and expect zero `MISSING` lines.
- Re-run `go test ./internal/pipeline/...` and `go test ./internal/knowledge/... ./internal/api/... ./internal/intelligence/... ./internal/telegram/...` and expect green.
- Re-run `python3 -m pytest ml/tests/test_synthesis.py -q` and expect `17 passed`.
- Re-run `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer` and expect exit 0.
- Re-run `bash .github/bubbles/scripts/state-transition-guard.sh specs/025-knowledge-synthesis-layer/bugs/BUG-025-004-test-trigger-probe-residuals` before closing the bug.

## Commits

Single commit with prefix `spec(025):` per repo discipline. Path-limited `git add` for only:

- `internal/pipeline/synthesis_subscriber_test.go`
- `specs/025-knowledge-synthesis-layer/scopes.md`
- `specs/025-knowledge-synthesis-layer/scenario-manifest.json`
- `specs/025-knowledge-synthesis-layer/bugs/BUG-025-004-test-trigger-probe-residuals/**`

Index must be clean of `specs/055-*`, `cmd/core/**`, `internal/api/notifications*`, `internal/notification/source/**`, `config/smackerel.yaml`, `tests/e2e/notification_ntfy_source_api_test.go`, `tests/integration/notification_ntfy_runtime_test.go`, `tests/stress/notification_ntfy_source_stress_test.go`, and any other 055 WIP file before commit.
