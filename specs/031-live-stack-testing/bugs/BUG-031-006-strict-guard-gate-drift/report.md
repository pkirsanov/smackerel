# Report: BUG-031-006 Strict-Guard Gate Drift Closure

## Summary

This report tracks closure of the 38 BLOCK findings + 2 warnings that `state-transition-guard.sh` raised against `specs/031-live-stack-testing/` after the spec was promoted to `done` under an earlier gate set.

**Origin:** Stochastic-quality-sweep `sweep-2026-05-23-r30` round 3 (parent), `reconcile-to-doc` child workflow (parent-expanded; nested `runSubagent` unavailable in this runtime).

**Status:** Open. Closure not yet started.

## Discovery Evidence

```
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing
...
🔴 TRANSITION BLOCKED: 38 failure(s), 2 warning(s)
state.json status MUST NOT be set to 'done'.
Fix ALL blocking failures above before attempting promotion.

$ bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing
... PASS
```

Divergence: `artifact-lint.sh` (loose) accepts `completedPhaseClaims` at face value; `state-transition-guard.sh` (strict) cross-references claims against `executionHistory` and rejects narrative-only attestation.

## Finding Catalog (38 BLOCK + 2 WARN)

See parent `specs/031-live-stack-testing/report.md` → "Reconcile-To-Doc Pass (2026-05-23 — sweep-2026-05-23-r30 round 3 of 30)" for the full categorized table. Categories:

- 1 × G060 (scenario-first TDD red→green markers)
- 4 × G022 Check 6 (required specialist phases missing: regression, simplify, stabilize, security)
- 5 × G022 Check 6B (phase impersonation: chaos, docs, test, audit, validate)
- 18 × G016 Check 8A (regression E2E planning items, 3 per scope × 6 scopes)
- 3 × Check 8D (Change Boundary containment)
- 1 × G053 Check 13B (Code Diff Evidence section)
- 1 × Check 5A (SLA stress coverage for Scope 6 60s ML readiness timeout)
- 1 × Check 17 (strict-mode commit prefix)
- 2 × WARN (advisory: no completedAt timestamps; no concrete test file paths in some scope Test Plans)

## Implementation Sanity Check

Implementation on disk is real; drift is governance/evidence-shaped:

```
$ find tests/integration -maxdepth 1 -name '*.go' | wc -l
17
$ find tests/e2e -maxdepth 1 -name '*.go' | wc -l
24
$ wc -l internal/api/ml_readiness.go
52 internal/api/ml_readiness.go
$ grep -c '^func Test' tests/integration/db_migration_test.go tests/integration/nats_stream_test.go tests/integration/artifact_crud_test.go tests/integration/ml_readiness_test.go
tests/integration/db_migration_test.go:8
tests/integration/nats_stream_test.go:7
tests/integration/artifact_crud_test.go:23
tests/integration/ml_readiness_test.go:5
```

## Per-Scope Execution Log

### Scope 1 — Regression E2E Planning Edits
- Status: Not Started

### Scope 2 — Change Boundary Section
- Status: Not Started

### Scope 3 — Scope 6 SLA Stress Test
- Status: Not Started

### Scope 4 — Code Diff Evidence Section
- Status: Not Started

### Scope 5 — Specialist Phase Re-Runs
- Status: Not Started

## Next Required Owner

`bubbles.design` — to design the closure across the 5 scopes (Phase A in `design.md`), then route to `bubbles.plan` for scope/DoD instantiation, then `bubbles.implement` for Scope 3 + 4 work, then the specialist phase re-runs in Scope 5.

## Outcome

**Open / Routed** — full 6-artifact planning packet created on 2026-05-23 by reconcile-to-doc. Awaiting `bubbles.design` to begin closure.
