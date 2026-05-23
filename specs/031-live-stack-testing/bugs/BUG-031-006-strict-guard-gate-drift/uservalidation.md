# User Validation: BUG-031-006 Strict-Guard Gate Drift Closure

## Status

**Open — closure work not yet started.**

## Validation Plan

User-facing validation will be evaluated after Scope 5 lands. The validation criteria are operator-shaped, not end-user-shaped, since this bug closes governance/evidence drift, not user-facing behavior:

1. `bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing` exits 0 with zero BLOCK findings.
2. `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing` continues to exit 0.
3. `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/031-live-stack-testing --verbose` exits 0.
4. `./smackerel.sh test stress` passes the new `tests/stress/ml_readiness_timeout_stress_test.go` against the disposable test stack.
5. `git log --oneline | grep -E '^[0-9a-f]+ (spec\(031\)|bubbles\(031/)'` returns at least one closure commit.
6. No `--no-verify` is used in any closure push.
7. No G041 manipulation pattern (checkbox deletion, status rename, claim stripping) appears in any closure diff.

## Acceptance

Not yet evaluated. Pending closure of all 5 scopes in `scopes.md`.

## Notes

- Implementation on disk for spec 031 is real and is **NOT** the subject of validation. The validation only confirms that the **planning/evidence/provenance drift** is closed.
- Spec 055 (notification ntfy adapter) in-flight working-tree edits are excluded from this bug's Change Boundary and must remain untouched in every closure commit.
