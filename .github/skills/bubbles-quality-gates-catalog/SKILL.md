---
name: bubbles-quality-gates-catalog
description: Look up Bubbles quality gates (G024–G095+) by ID, by symptom, or by enforcement script. Use when a guard rejects work and you need to understand the failing gate, when designing a scope's DoD, or when wiring tests to cover gate-relevant scenarios. Covers the canonical test taxonomy, gate IDs, and which script enforces each one.
---

# Bubbles Quality Gates Catalog

## Goal
Resolve "what is gate G0XX" and "which script enforces this" without re-reading the entire governance corpus. Use this as the entry point into the gate catalog.

## When to use
- A guard (state-transition, artifact-lint, regression-quality, regression-baseline, etc.) failed with a `G0XX` label
- Designing a scope's DoD and need to know which gates apply
- Writing a test and need to know which gate IDs the test should cover
- Auditing whether a scope's DoD covers all gates relevant to the change

## High-frequency gate IDs (memorize these)
| Gate | Topic | Enforced by |
|------|-------|-------------|
| G023 | State transition guard envelope | `state-transition-guard.sh` |
| G024 | Spec cannot be `done` until all scopes `Done` | `state-transition-guard.sh`, `artifact-lint.sh` |
| G025 | Per-DoD-item inline raw evidence ≥10 lines | `artifact-lint.sh`, `state-transition-guard.sh` |
| G026 | Stress tests required when scope defines latency SLAs | `artifact-lint.sh` |
| G027 | Phase-Scope coherence (completedPhases ↔ completedScopes) | `state-transition-guard.sh` |
| G028 | Implementation reality scan (no stubs/fakes/hardcoded data) | `implementation-reality-scan.sh` |
| G029 | Integration completeness (every artifact has a consumer) | `state-transition-guard.sh` |
| G035 | Vertical slice completeness (cross-layer scopes) | `state-transition-guard.sh` |
| G040 | Deferral language detection | `state-transition-guard.sh` Check 18 |
| G042–G068 | Capability delegation, policy provenance, validate certification, scenario manifest, lockdown, regression contract, scenario TDD, rework packets, owner-only remediation, concrete results, child-workflow depth | `workflows.yaml` + targeted guards |
| G073 | Mode ceiling pre-flight (statusCeiling enforcement) | `state-transition-guard.sh` Check 3B |
| G079 | Impact-aware validation (testImpact config) | `test-impact-plan.sh` |
| G080 | Trace contracts (traceContracts config) | `trace-contract-guard.sh` |
| G082–G093 | Convergence cap, compaction discipline, pre-existing deferral, dogfood, orchestrator persistence, planning packet linkage, post-cert edits, inter-spec deps, retro convergence, planning chain, strict terminal status, delivery delta | targeted guards (each named after the gate) |
| G094 | Capability foundation gate | `capability-foundation-guard.sh` |

## Canonical Test Taxonomy (binding)
| Category | Means | Real-stack? | Mocks permitted? |
|----------|-------|-------------|------------------|
| `unit` | Isolated logic, fast | No | Yes (internal) |
| `functional` | Real-dep functions, may touch DB/filesystem | Optional | External only |
| `integration` | Multi-component, real deps, real test DB | **Yes** | **External only** |
| `ui-unit` | Component tests with mocked backend | No | Yes (backend) |
| `e2e-api` | API workflow vs live stack | **Yes** | **External only** |
| `e2e-ui` | UI workflow vs live stack | **Yes** | **External only** |
| `stress` | Burst load vs live stack | **Yes** | **External only** |
| `load` | Sustained load vs live stack | **Yes** | **External only** |

A test that uses `page.route()`, `context.route()`, `cy.intercept()`, `msw`, `nock`, `wiremock`, or any internal-service mock and claims to be `integration` / `e2e-api` / `e2e-ui` is misclassified. Reclassify it as `ui-unit` (or `unit`) and write a real live-stack test to fill the gap. The framework treats this misclassification as blocking.

## Adversarial regression for bug fixes
Bug-fix regression tests MUST include at least one adversarial test case — input that would FAIL if the bug were reintroduced. Tautological regressions (all fixtures already satisfy the broken code path) do not count. Enforced by `regression-quality-guard.sh --bugfix`.

## Resolving a gate failure
1. Note the gate ID printed by the guard.
2. Open the enforcing script in `bubbles/scripts/` and read the failing check's prose.
3. Fix the underlying gap (in the spec, scope, evidence, test, or code).
4. Re-run the guard. Do not change the test/assertion to silence the failure.

## Authoritative modules
- `agents/bubbles_shared/quality-gates.md` — full gate catalog with rationale
- `agents/bubbles_shared/state-gates.md` — state-claim integrity gates
- `agents/bubbles_shared/test-fidelity.md` — live-stack authenticity
- `agents/bubbles_shared/e2e-regression.md` — persistent regression expectations
- `bubbles/workflows.yaml` — gate-to-mode wiring
- `bubbles/scripts/gate-id-grep.sh` — surface every gate ID referenced in the repo
