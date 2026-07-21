---
name: bubbles-quality-gates-catalog
description: Look up Bubbles quality gates (G024–G100+) by ID, by symptom, or by enforcement script. Use when a guard rejects work and you need to understand the failing gate, when designing a scope's DoD, or when wiring tests to cover gate-relevant scenarios. Covers the canonical test taxonomy, gate IDs, and which script enforces each one.
---

# Bubbles Quality Gates Catalog

## Goal
Resolve "what is gate G0XX" and "which script enforces this" without re-reading the entire governance corpus. Use this as the entry point into the gate catalog.

## When to use
- A guard (state-transition, artifact-lint, regression-quality, regression-baseline, etc.) failed with a `G0XX` label
- Designing a scope's DoD and need to know which gates apply
- Writing a test and need to know which gate IDs the test should cover
- Auditing whether a scope's DoD covers all gates relevant to the change

## v4.1.0 gate refinements

Gates G073, G090, G022, G041, G040, G008A, G009, G056 received non-breaking refinements in v4.1.0. See [`docs/v4.1.0-delivered-pending-activation.md`](../../docs/v4.1.0-delivered-pending-activation.md) for the per-gate behavior change and how scopes opt in via `Scope-Kind:`, `Lockdown-FRs:`, `deliverableFiles[]`, `phaseStubs{}`, and report.md anchor references.

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
| G042–G068 | Capability delegation, policy provenance, validate certification, scenario manifest, lockdown, regression contract, scenario TDD, rework packets, owner-only remediation, concrete results, workflow-runner authorization | `workflows.yaml` + targeted guards |
| G073 | Mode ceiling pre-flight (statusCeiling enforcement) | `state-transition-guard.sh` Check 3B |
| G079 | Impact-aware validation (testImpact config) | `test-impact-plan.sh` |
| G080 | Trace contracts (traceContracts config) | `trace-contract-guard.sh` |
| G082–G093 | Convergence cap, compaction discipline, pre-existing deferral, dogfood, orchestrator persistence, planning packet linkage, post-cert edits, inter-spec deps, retro convergence, planning chain, strict terminal status, delivery delta | targeted guards (each named after the gate) |
| G094 | Capability foundation gate | `capability-foundation-guard.sh` |
| G095 | Discovered-issue disposition (every observed issue is filed, not deferred) | `discovered-issue-disposition-guard.sh` |
| G097 | Requirement-mechanism correspondence (named mechanism ↔ code, warn-and-justify) | `requirement-mechanism-guard.sh` |
| G098 | Observability posture declared (wired/opted-out; undeclared WARN, project-flippable to block; fake-wired / opted-out-malformed / unsupported-schema fail loud; framework source EXEMPT) | `observability-posture-guard.sh` |
| G099 | Observability opt-out freshness (recorded + expiring opt-out; missing required optOut field fails loud; expired revisitAfter = non-blocking route-required reminder) | `observability-opt-out-guard.sh` |
| G100 | Observability SLO evidence (BLOCKING when posture: wired + an instrumented scope's `observabilityWorkflow` targets a workflow with an `slo:` link; captured `.specify/runtime/observability/<workflow>.slo.json` must MEET the `traceContracts.observability.slos` target; a contract-declared metric absent from `observed` is a breach; malformed / wrong-workflow evidence fails loud before numeric compare; missing jq/yq fails CLOSED; framework source EXEMPT). Division of labor: G026 ensures the stress/load test EXISTS + cites the SLO registry; G100 ensures the captured evidence MEETS the target. | `observability-slo-guard.sh` |
| G101 | Release-delivery reconciliation — every `delivery=required` feature in `docs/releases/<phase>/features.md` (machine-bound via `<!-- bubbles:feature id=… spec=… delivery=required -->` + a `bubbles:reconciled-packet` header) MUST map to a TERMINAL + VALIDATE-certified spec; promised-but-unspecced / non-terminal / blocked / implement-self-certified required feature = finding; WARN-grandfathered without the header, BLOCKING with header or `--require-coverage` (goal/sprint release-phase convergence); malformed reconciled packet fails loud; framework source EXEMPT; compile-time twin enforces `rootOutcome.targetReleasePacket` coverage in `scenario-compile-lint.sh` | `release-delivery-reconciliation-guard.sh` |
| G110–G126 | Release-train + upkeep + propagation + incident + framework gates: release-train discipline, flag-default-off-on-other-trains, backup / restore-drill / BCDR evidence, env-pollution isolation (G115), offsite-backup-required, audit-trail immutability, backup-retention / PII-classification / secret-rotation declared, propagation policy / validation / ledger, incident severity declared, framework-health evidence, model-tier floor | `release-train-guard.sh`, `env-pollution-scan.sh`, `propagation-policy-guard.sh`, `model-tier-advisory.sh` + targeted guards |
| G127 | Capability-consumer freshness (framework-source self-validation: every `state: shipped` capability in `bubbles/capability-ledger.yaml` declares a non-empty `consumers:` list whose every path exists on disk; blank-only list = ORPHAN, stray blank entry = MALFORMED, missing path = DANGLING; partial/proposed/deprecated exempt; repos with no ledger no-op; missing yq fails CLOSED only when a ledger is present; the G029 integration-completeness standard applied to the framework's own ledger) | `capability-consumer-freshness.sh` |
| G128 | Session-cap enforcement (aggregate whole-session sibling of G082): mechanically enforces the `sessionBudget` caps `maxTotalConvergenceIterations` / `maxWallClockMinutes` / `maxToolCalls` recorded in `.specify/memory/bubbles.session.json`; DEFAULT-OFF (all caps null → no-op), enforced only when a cap is set AND its aggregate is measurable; RFC3339 wall-clock via jq `fromdateiso8601` (GNU/BSD-identical); state-transition Check 40; no `--skip`/`--force`/`--ignore` bypass; a breach emits a `blocked` envelope with finding G128 and stops the session | `session-cap-guard.sh` |

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
