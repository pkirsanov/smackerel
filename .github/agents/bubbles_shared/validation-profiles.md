# Validation Profiles

Use these sections as the single source of truth for agent-specific Tier 2 completion checks.

## Analyst

| ID | Check | Pass Criteria |
|----|-------|---------------|
| AN1 | Business scenarios documented | `spec.md` contains actor/use-case/scenario output |
| AN2 | Capability map grounded in code or sources | No speculative capability claims |
| AN3 | State updated when owned | `state.json` reflects analysis execution when applicable |
| AN4 | Active requirements reconciled | Invalidated requirement/scenario content is removed from active sections or explicitly marked superseded |

## Design

| ID | Check | Pass Criteria |
|----|-------|---------------|
| DE1 | Active design reconciled | `design.md` presents one active architecture/contract truth |
| DE2 | Spec-design coherence maintained | Active contracts, data models, and flows match current `spec.md` |
| DE3 | Superseded decisions isolated | Legacy design decisions are clearly marked superseded or removed from active sections |

## Docs

| ID | Check | Pass Criteria |
|----|-------|---------------|
| D1 | API doc cross-reference | Documented endpoints match route files |
| D2 | No orphaned documented endpoints | Every documented endpoint exists in code |
| D3 | Source-of-truth consistency | `spec.md`, `design.md`, `scopes.md`, and docs agree |

## UX

| ID | Check | Pass Criteria |
|----|-------|---------------|
| UX1 | UI Wireframes section exists | `spec.md` contains `## UI Wireframes` |
| UX2 | At least one wireframe exists | ASCII wireframe content present |
| UX3 | Interactions recorded | Every wireframe includes interactions |
| UX4 | Responsive behavior recorded | Every wireframe includes responsive notes |
| UX5 | Accessibility recorded | Every wireframe includes accessibility notes |
| UX6 | State updated when owned | `state.json` includes UX execution when applicable |
| UX7 | Active UX reconciled | Stale wireframes, screens, or flows are removed from active UX sections or explicitly marked superseded |

## Plan

| ID | Check | Pass Criteria |
|----|-------|---------------|
| P1 | Active scopes match current truth | Active scope inventory reflects current `spec.md` and `design.md` |
| P2 | Stale scopes invalidated | Invalid scopes are removed from active execution and preserved only as superseded history if needed |
| P3 | Reconciled scope/test/DoD parity | Rewritten scopes maintain Gherkin, Test Plan, and DoD coherence after reconciliation |

## Implement

| ID | Check | Pass Criteria |
|----|-------|---------------|
| I1 | Scope DoD evidence updated inline | Every completed DoD item has real evidence |
| I2 | Required tests pass | Impacted test suite passes |
| I3 | Docs synchronized | Required docs updated for changed behavior |
| I4 | Scope state coherent | Scope status, evidence, and `state.json` agree |
| I5 | No new policy violations | No defaults, stubs, fake data, or deferral introduced |

## Test

| ID | Check | Pass Criteria |
|----|-------|---------------|
| T1 | Required test types executed | All applicable categories run |
| T2 | Red-to-green trace captured | Changed behavior has failing then passing proof |
| T3 | Live-stack integrity preserved | No mocked live-system tests |
| T4 | Regression coverage added | Changed behavior has persistent regression coverage |
| T5 | Evidence recorded correctly | Raw output captured where required |

## Validate

| ID | Check | Pass Criteria |
|----|-------|---------------|
| V1 | Governance scripts pass | Required guard/lint/reality scans pass |
| V2 | Build, lint, and tests pass | Zero blocking failures |
| V3 | Contracts verified | Frontend/backend or spec/runtime contracts match |
| V4 | Freshness checks pass for UI scopes | Served bundle/build is current |
| V5 | Scope/DoD coherence verified | Required artifacts are compliant and Gherkin, Test Plan, DoD, and state align |
| V6 | Planned-behavior traceability verified | Every spec/Gherkin scenario maps to concrete non-proxy tests and executed evidence |
| V7 | Routing and re-validation closure enforced | Missing artifacts, tests, or implementation claims are routed to owners and validation does not pass until rerun checks succeed |

## Audit

| ID | Check | Pass Criteria |
|----|-------|---------------|
| A1 | State transition guard passes | No blocking gate failures |
| A2 | Independent verification rerun | Audit rechecks critical evidence independently |
| A3 | Evidence cross-reference clean | Completed items have genuine evidence |
| A4 | Fabrication heuristics clean | No fabrication indicators triggered |
| A5 | Reality scan and coherence clean | No stale done claims or fake implementations |
| A6 | Consumer-trace and regression fidelity | Renames/removals and changed behavior are fully traced |

## Iterate

| ID | Check | Pass Criteria |
|----|-------|---------------|
| IT1 | Scope selection respected dependencies | No out-of-order scope execution |
| IT2 | Scope completion recorded correctly | Scope/report/state updates agree |
| IT3 | Specialist outputs verified | No phase advanced on unverified subagent claims |
| IT4 | Remaining work classified correctly | Blocked vs next-eligible scopes are accurate |

## Workflow

| ID | Check | Pass Criteria |
|----|-------|---------------|
| W1 | Per-spec guard checks pass | Every done spec passes required gates |
| W2 | Specialist completion ledger coherent | Mode-required phases are recorded |
| W3 | Cross-agent outputs verified | No fabricated or missing specialist results |
| W4 | Sequential policy honored | No later spec started before earlier required completion |
| W5 | Zero deferral language in done work | Done specs contain no deferred work markers |

## Chaos

| ID | Check | Pass Criteria |
|----|-------|---------------|
| C1 | Chaos execution evidence exists | Real browser automation / HTTP probe output recorded |
| C2 | Bug artifacts complete when created | Every BUG directory has required artifacts |
| C3 | Findings report produced | Structured findings output exists |
| C4 | Fixture isolation verified | Mutations used owned fixtures or baseline snapshot-and-restore proof exists |

## Harden

| ID | Check | Pass Criteria |
|----|-------|---------------|
| H1 | Findings classified with evidence | No speculative hardening claims |
| H2 | Fixes verified | Impacted checks rerun after fixes |
| H3 | Required artifact updates made | Scope/report/state outputs reflect new work |

## Gaps

| ID | Check | Pass Criteria |
|----|-------|---------------|
| GA1 | Critical and high gaps resolved or explicitly retained as open findings | No unresolved blockers claimed as fixed |
| GA2 | Scenario/Test/DoD parity restored | No orphan scenarios or findings |
| GA3 | State coherence maintained | No stale done state remains |
| GA4 | Post-fix regression status verified | Test suite is not worse after updates |

## Stabilize

| ID | Check | Pass Criteria |
|----|-------|---------------|
| ST1 | Stability scan complete | All required domains reviewed |
| ST2 | Findings backed by evidence | Reproduction, logs, or metrics exist |
| ST3 | Fixes verified | Impacted checks rerun successfully |
| ST4 | Scope artifacts updated | New stability work is reflected in planning artifacts |

## Security

| ID | Check | Pass Criteria |
|----|-------|---------------|
| SE1 | Security coverage complete | Required categories were reviewed |
| SE2 | Dependency or scanner evidence recorded | Actual scan output exists |
| SE3 | Findings grounded in code or execution | No speculative vulnerabilities reported as facts |
| SE4 | Artifact updates made for open issues | Follow-up work is captured in scope artifacts |