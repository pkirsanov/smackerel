# User Validation: BUG-020-006 — Governance Baseline Drift Remediation

> **Parent Spec:** [specs/020-security-hardening](../../spec.md)
> **Bug:** [spec.md](spec.md)

## Acceptance

This bug is an **artifact-only governance baseline remediation** discovered
by `bubbles.validate` during stochastic sweep round 3
(`sweep-2026-05-24-r10`). It does not change runtime behavior, user-visible
functionality, or any production code path in `internal/`, `cmd/`, `ml/`,
`web/`, `config/`, or `docker-compose*.yml`.

User-facing acceptance reduces to:

1. ✅ Spec 020 security hardening behavior is unchanged from its certified
   `done` state (R-001..R-007 + BUG-020-002 + BUG-020-004 + BUG-020-005 +
   SEC-R68-001 all intact on `main`).
2. ✅ Parent spec `specs/020-security-hardening` is once again guard-clean
   under the current `state-transition-guard.sh`, `artifact-lint.sh`, and
   `traceability-guard.sh` contracts so future workflow runs against spec
   020 can proceed without rationalizing skipped checks.
3. ✅ The bug packet itself satisfies the bug 6-artifact contract and its
   own `state.json` is guard-clean at the `validate-to-doc` ceiling.

## Checklist

- [x] Bug packet 6-artifact set exists (spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json)
- [x] Parent spec `specs/020-security-hardening` returns to guard-clean state
- [x] Bug `state.json.status` is `validated` (validate-to-doc ceiling) with guard-clean executionHistory
- [x] No production code, test, config, or CI/CD files staged in the remediation commit
- [x] Owner sign-off recorded below

## Sign-Off

- **Owner:** bubbles.workflow (parent-expanded child mode `reconcile-to-doc`)
- **Sweep:** sweep-2026-05-24-r10 round 3
- **Status:** Accepted
- **Date:** 2026-05-24
