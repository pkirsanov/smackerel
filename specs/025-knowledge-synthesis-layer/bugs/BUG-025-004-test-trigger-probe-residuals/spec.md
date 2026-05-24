# Spec: BUG-025-004 Test trigger probe quality residuals

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

This file restates the bug's specification surface in spec form so the framework's artifact-lint and state-transition-guard can read a `spec.md` next to the rest of the 6-artifact set.

## Business Context

`specs/025-knowledge-synthesis-layer/` certified as `done` on 2026-04-21 after a stochastic regression-to-doc sweep. Sweep round 19 of `sweep-2026-05-23-r30` (`mode: test-to-doc`) re-probed spec 025's test layer and surfaced three residual artifact-quality defects that were silently introduced by simplify and refactor passes after certification, and that current framework guards either catch (G068) or do not yet catch (`linkedTests` drift, test name typo).

## Use Cases

- **UC-01:** An engineer running `traceability-guard.sh specs/025-knowledge-synthesis-layer` needs the guard to pass (currently FAILS with 3 issues).
- **UC-02:** An engineer grepping `linkedTests` in `scenario-manifest.json` to find the regression for a given Gherkin scenario needs the linked name to resolve to a real test.
- **UC-03:** An engineer searching the codebase for failure-path coverage by grepping `Failed` should find `TestSynthesisExtractResponse_FailureMarksFailed`, not be tripped by the `Flailed` typo.

## Functional Requirements

- **FR-01:** `traceability-guard.sh specs/025-knowledge-synthesis-layer` MUST exit 0.
- **FR-02:** Every `linkedTests[]` entry in `specs/025-knowledge-synthesis-layer/scenario-manifest.json` MUST resolve to a real `func Test…` / `def test_…` definition in the file named by the same manifest entry.
- **FR-03:** The misspelled test function name `TestSynthesisExtractResponse_FailureMarksFlailed` MUST be renamed to `TestSynthesisExtractResponse_FailureMarksFailed` with no behavioral change.
- **FR-04:** No runtime code path, schema, API contract, NATS topology, config value, scheduler job, web template, or Telegram command may be changed by this packet.

## Gherkin Scenarios

See `scopes.md` for the full Gherkin set (one scenario per scope plus the closing trace guard scenario). The scenarios cover:

- SCN-B0254-01: Rename typo and verify zero residual occurrences.
- SCN-B0254-02: Refresh scenario-manifest `linkedTests` to current test names.
- SCN-B0254-03: Trace guard exits 0 with `RESULT: PASSED`.
- SCN-B0254-04: G068 DoD fidelity reports 0 unmapped scenarios.

## Acceptance Criteria

- **AC-01:** `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exits 0 and prints `RESULT: PASSED`.
- **AC-02:** `grep -rn "Flailed" --include="*.go" .` returns zero matches.
- **AC-03:** The Python `linkedTests` cross-check returns zero `MISSING` lines.
- **AC-04:** `go test ./internal/pipeline/... ./internal/knowledge/... ./internal/api/... ./internal/telegram/...` is green.
- **AC-05:** `python3 -m pytest ml/tests/test_synthesis.py -q` reports `17 passed`.
- **AC-06:** `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer` exits 0.
- **AC-07:** Single commit with prefix `spec(025):` and no `specs/055-*` or other 055 WIP files swept into the commit.

## Product Principle Alignment

This bug enforces **Principle 8 — Trust Through Transparency** (from `docs/Product-Principles.md` and the constitution Model Compensations table): scenario manifests and DoD items are the spec's source-of-truth attribution to regression coverage. When manifest `linkedTests` silently drift to stale names, the trust contract for "every claim has a citation" is weakened. This packet restores the citation chain without changing any runtime behavior.

## Non-Goals

- Adding a framework-side `linkedTests`-existence validator. The framework script lives in `.github/bubbles/scripts/` (immutable per repo policy). Captured as a documented out-of-scope follow-up in `design.md`.
- Re-opening or re-certifying parent spec 025. Parent stays `done`.
- Touching `specs/055-notification-source-ntfy-adapter/` or any other in-flight WIP.
