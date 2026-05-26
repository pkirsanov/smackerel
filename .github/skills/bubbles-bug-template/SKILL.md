---
name: bubbles-bug-template
description: Use the canonical Bubbles bug artifact template when filing or working a bug under specs/<feature>/bugs/BUG-NNN-description/. Use when reporting a regression, picking up a bug, writing the reproduction evidence, designing the fix, or auditing whether the bug folder has all required artifacts before work begins.
---

# Bubbles Bug Template

## Goal
Every bug folder has all 6 required artifacts before fix work begins, includes a reproducible repro, and produces an adversarial regression test that would fail if the bug were reintroduced.

## When to use
- Filing a new bug under `specs/<feature>/bugs/BUG-NNN-description/` (or `specs/bugs/BUG-NNN-...` for cross-feature bugs)
- Picking up an open bug; verifying it has all 6 artifacts
- Writing reproduction evidence (BEFORE the fix) and verification evidence (AFTER the fix)
- Designing the regression test for the bug

## Required artifacts (every bug)
| File | Notes |
|------|-------|
| `bug.md` | description, reproduction steps, observed vs expected, severity |
| `spec.md` | expected behavior specification (what the code SHOULD do) |
| `design.md` | root cause analysis + fix design |
| `scopes.md` | fix scope(s) with DoD checklist + Test Plan |
| `report.md` | execution evidence: bug reproduced BEFORE fix, fix verified AFTER fix, regression test execution |
| `state.json` | bug lifecycle state |

If any file is missing, `bubbles.bug` (or whichever agent is handling the bug) MUST create it before proceeding.

## Bug-reproduction gate (Gate 0)
1. Reproduce the bug FIRST — capture raw terminal output in `report.md` under a `Before Fix` section.
2. Apply the fix.
3. Re-execute the same reproduction sequence — capture output in `report.md` under `After Fix`.
4. Both captures MUST be in the same session and ≥10 lines each.

## Adversarial regression test (NON-NEGOTIABLE)
Every bug-fix regression test MUST include at least one adversarial test case — an input that would FAIL if the bug were reintroduced. Tautological regressions (all fixtures already satisfy the broken code path) do not count. Enforced by `regression-quality-guard.sh --bugfix`.

## Anti-patterns
- ❌ Filing a bug with only `bug.md` and skipping spec.md/design.md/scopes.md/report.md/state.json
- ❌ Writing a regression test where every fixture would also fail if you reintroduced the bug differently (tautology)
- ❌ Calling the bug closed without an AFTER-fix reproduction capture
- ❌ Adding a bypass like `if (page.url().includes('/login')) return` in the regression test

## Authoritative modules
- `agents/bubbles_shared/bug-templates.md` — full template text
- `agents/bubbles_shared/artifact-lifecycle.md` — lifecycle rules
- `agents/bubbles_shared/quality-gates.md` — adversarial regression requirement
- `agents/bubbles_shared/e2e-regression.md` — regression expectations
- `bubbles/scripts/regression-quality-guard.sh` — bugfix mode enforcement
- `bubbles/scripts/regression-baseline-guard.sh` — baseline expectations
