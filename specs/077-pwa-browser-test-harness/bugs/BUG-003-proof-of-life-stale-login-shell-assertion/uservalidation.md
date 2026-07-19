# User Validation: BUG-003 proof_of_life e2e-ui stale 200-branch assertion (served login shell)

## Validation Criteria

1. **Real served identity asserted:** the proof_of_life 200-branch asserts the ACTUAL served login shell (`title "Sign in — Smackerel"`, `h1 "Sign in"`), not the stale `"Smackerel"`/`"Smackerel"` PWA-index expectation.
2. **Live GREEN:** `./smackerel.sh test e2e-ui proof_of_life.spec.ts` passes this session against the disposable stack.
3. **Adversarial, not silent-pass:** the 200-branch fails on a blank/error/other served `/` (proven by the before-fix RED) and carries an adversarial signal with 0 guard violations.
4. **No app change:** zero app/runtime/source edits; the `/` → `/login?next=/` redirect (spec 057) is left intact as intended behavior.
5. **Good-neighbor:** the e2e-ui stack was brought up only when no foreign `smackerel-test*` stack was running, and only its own `smackerel-test-e2e-ui` project was torn down.

## Validation Steps

1. `./smackerel.sh test e2e-ui proof_of_life.spec.ts` (before fix) → `1 failed`, exit 1, `Received "Sign in — Smackerel"`. ✅ captured this session (see [report.md](report.md#repro-before)).
2. `./smackerel.sh test e2e-ui proof_of_life.spec.ts` (after fix) → `1 passed`, exit 0. ✅ captured this session (see [report.md](report.md#repro-after)).
3. `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix web/pwa/tests/proof_of_life.spec.ts` → adversarial signal, 0 violations, exit 0. ✅ captured this session.
4. `git diff --stat` → only `web/pwa/tests/proof_of_life.spec.ts` changed (+19/-8). ✅ captured this session.

## Checklist

- [x] 200-branch asserts the real served login-shell title (`Sign in — Smackerel`) and h1 (`Sign in`)
- [x] Live e2e-ui proof_of_life GREEN this session (`1 passed`, exit 0)
- [x] Before-fix RED reproduced live this session (title mismatch, exit 1) — adversarial, not a silent-pass
- [x] `regression-quality-guard` plain + `--bugfix` exit 0 (adversarial signal)
- [x] Zero app/runtime/source changes — verified via `git status -s` / `git diff --stat` (one test file only)
- [x] Good-neighbor concurrency honored (own `smackerel-test-e2e-ui` project only; no foreign stack touched)
