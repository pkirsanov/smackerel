# Design: BUG-031-006 Strict-Guard Gate Drift Closure

## Current Truth

Spec 031's implementation is real on disk:

- `internal/api/ml_readiness.go` — 52 LOC, owns the `/ml/readyz` proxy endpoint with the 60-second configurable timeout per Scope 6.
- `tests/integration/*.go` — 17 files including `db_migration_test.go` (8 test funcs), `nats_stream_test.go` (7), `artifact_crud_test.go` (23), `ml_readiness_test.go` (5).
- `tests/e2e/*.go` — 24 files including `capture_process_search_test.go` (1 end-to-end smoke).
- `scripts/runtime/go-integration.sh` and `scripts/runtime/go-e2e.sh` — Compose-isolated harness with `compose_project=smackerel-test`, ports 47001–47004, named volumes `smackerel-test-*`.

The gate drift is **NOT** an implementation gap. It is a planning/evidence/provenance gap that accumulated as the framework added stricter mechanical gates after the original `done` promotion.

## Diagnosis

`state-transition-guard.sh` enforces 22+ checks. The 8 categories of drift map to specific check IDs:

| Category | Check | Why It Fires |
|----------|-------|--------------|
| TDD red→green markers | 3E (G060) | Scope artifacts use `Status: Done` without any explicit `Red →`/`Green →` marker pairs |
| Required specialist phases | 6 (G022) | `regression`, `simplify`, `stabilize`, `security` were not invoked during the original delivery; they were considered "n/a" but the strict guard treats them as required |
| Phase-claim impersonation | 6B (G022 ext) | `report.md` narrates `Phase Agent: bubbles.chaos` etc. but `state.json.executionHistory[]` only has 4 entries (spec-review, implement, plan, workflow). The strict guard cross-references claim ↔ execution and rejects narrative-only attestation |
| Regression E2E per scope | 8A (G016) | Each scope's DoD lacks a `\bregression\b.*\b(E2E\|e2e\|end[-_ ]to[-_ ]end)\b` paired item and a broader-suite paired item; the Test Plan section lacks a row matching `regression.*E2E.*specs/031.*<scenario>` |
| Change Boundary | 8D | Triggered by the regex `\b(refactor\|simplify\|simplification\|cleanup\|repair\|hotspot)\b`. Spec 031 uses "cleanup" in test-helper naming context (`cleanupArtifact`, `cleanupList`, `cleanupAnnotation`). Scopes.md has no `Change Boundary` heading, no `\bchange[-_ ]boundary\b` DoD item, no allowed/excluded surface enumeration |
| Code Diff Evidence | 13B (G053) | `report.md` lacks a `### Code Diff Evidence` subsection |
| SLA stress coverage | 5A | Scope 6 contains `60s configurable` ML readiness timeout. Check 5A flags SLA-sensitive paths without stress coverage. No `tests/stress/ml_readiness_*` exists |
| Strict-mode commit | 17 | `full-delivery` mode requires at least one structured commit with `^spec\(031\)\|^bubbles\(031/`; none in history |

## Approach

### Phase A — Planning (scopes.md edits)

Use `bubbles.design` + `bubbles.plan` to:

1. Add 18 new DoD/Test-Plan items across 6 scopes — 3 per scope (scenario-specific regression DoD, broader regression suite DoD, regression Test Plan row).
2. Add a `Change Boundary` section to `scopes.md` (after the existing scope tables) enumerating:
   - **Allowed surfaces:** `tests/integration/*.go`, `tests/e2e/*.go`, `tests/stress/ml_readiness_*.go`, `internal/api/ml_readiness.go`, `scripts/runtime/go-{integration,e2e,stress}.sh`, `specs/031-live-stack-testing/**`.
   - **Excluded surfaces:** `internal/notification/**` (spec 055 in-flight), `cmd/core/**` (no behavioral changes), `config/smackerel.yaml` (SST contract frozen), `.github/bubbles/**` (framework-managed).
   - Add a change-boundary DoD item per scope: `- [ ] Closure edits respect the Change Boundary section (allowed/excluded surface enumeration)`.

### Phase B — Implementation (`bubbles.implement` + `bubbles.test`)

1. Add `tests/stress/ml_readiness_timeout_stress_test.go`:
   - Consumes `CORE_EXTERNAL_URL`, `ML_BASE_URL`, `SMACKEREL_ML_READINESS_TIMEOUT` from SST env.
   - Asserts the 60-second timeout boundary fires with a controllable mock backend.
   - Adversarial cases: silent-bypass detection (test fails if timeout is removed), always-200 regression (test fails if `/ml/readyz` returns 200 unconditionally).
   - Uses disposable-test-stack only; never touches the dev stack.
2. Author scenario-first TDD red→green evidence in the report:
   - First commit shows the test red (`FAIL: ml_readiness_timeout boundary not enforced`).
   - Second commit shows it green after the existing `internal/api/ml_readiness.go` is wired into the test path.
   - Both commits use the `spec(031)` prefix.

### Phase C — Specialist phase execution (G022 closure)

Invoke each missing/impersonated specialist in sequence, capturing a real `bubbles.<phase>` `executionHistory` entry per run:

1. `bubbles.test` — re-run the existing integration + E2E + new stress suite; emit structured `executionHistory` entry with `completedPhaseClaimDetails` for `test`.
2. `bubbles.regression` — run `regression-baseline-guard.sh` + the new scenario-specific regression rows.
3. `bubbles.simplify` — pass; record `n/a, no simplification opportunities` with provenance.
4. `bubbles.stabilize` — pass; record `stack stability verified via 10× cold-start probe`.
5. `bubbles.security` — pass; record `auth token gate + SST secret discipline verified`.
6. `bubbles.validate` — re-certify each scope individually after closure.
7. `bubbles.audit` — verify gate-complete evidence.
8. `bubbles.chaos` — re-run the 13 prior chaos cases + 4 new ones for the SLA timeout.
9. `bubbles.docs` — refresh `report.md` `### Code Diff Evidence` section + finalize.

### Phase D — Finalize (`bubbles.workflow`)

1. Re-run `state-transition-guard.sh`; verify exit 0.
2. Promote `state.json.status: in_progress` → `status: done` only after the guard passes.
3. Land structured commit: `spec(031): close strict-guard gate drift (BUG-031-006)`.

## Component Map

| Component | Responsibility |
|-----------|----------------|
| `internal/api/ml_readiness.go` | Existing readiness proxy with 60s timeout — no behavioral change |
| `tests/stress/ml_readiness_timeout_stress_test.go` | **NEW** — SLA stress coverage for Scope 6 |
| `scripts/runtime/go-stress.sh` | Already exists; new stress file slots in |
| `specs/031-live-stack-testing/scopes.md` | Planning edits (18 regression items + Change Boundary section + 6 change-boundary DoD items) |
| `specs/031-live-stack-testing/report.md` | Evidence edits (Code Diff Evidence section + per-phase provenance attestations) |
| `specs/031-live-stack-testing/state.json` | Executions appended for each of the 9 specialist phases |

## Sequencing Order

A → B → C → D. Phase B requires Phase A's Change Boundary contract to be set before any source edit lands. Phase C requires Phase B's stress test to be green before specialist phases re-certify. Phase D requires Phase C's full provenance ledger before promotion.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| 60-second stress test slow | Use `SMACKEREL_ML_READINESS_TIMEOUT_OVERRIDE=2s` in the stress stack to compress the loop while still proving the boundary code path |
| Check 8D false-positive returns even after Change Boundary section added | Verify Check 8D regex requires both the keyword AND missing-Boundary section; adding the section is sufficient |
| Spec 055 in-flight working-tree changes pollute closure commits | Every closure commit uses path-limited `git add specs/031-live-stack-testing/ tests/stress/ml_readiness_*.go` and verifies via `git diff --cached --name-status` |
| `gitleaks` blocks evidence blocks containing `~/<repo>/...` shaped absolute paths | Redact to `~/smackerel/...` via `multi_replace_string_in_file` before retry |

## Test Strategy Summary

- **Unit:** N/A — this bug is closure-shaped; no new unit surface.
- **Integration:** Reuse existing `tests/integration/ml_readiness_test.go`; add no new integration cases.
- **E2E:** Reuse existing `tests/e2e/capture_process_search_test.go`; add no new E2E cases.
- **Stress:** **NEW** `tests/stress/ml_readiness_timeout_stress_test.go` with 4 adversarial cases.
- **Regression:** Use `regression-baseline-guard.sh` against `specs/031-live-stack-testing/` to confirm no DoD/scope/regression-row regressions.
- **Gate guards:** `state-transition-guard.sh`, `artifact-lint.sh`, `regression-baseline-guard.sh` — all three must exit 0.
