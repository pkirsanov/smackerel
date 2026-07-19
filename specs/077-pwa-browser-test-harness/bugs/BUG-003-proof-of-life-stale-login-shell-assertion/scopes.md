# Scopes: BUG-003 proof_of_life e2e-ui stale 200-branch assertion (served login shell)

Single-scope test-assertion fix.

## Scope 1: Correct the proof_of_life 200-branch to the real served login shell

**Status:** Done
**Priority:** P1
**Depends On:** None
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-BUG-003-01 — proof_of_life 200 branch matches the real served login shell
  Given the disposable smackerel-test-e2e-ui stack running dev-token mode (AuthConfig.Enabled=false)
  And an unauthenticated Playwright browser navigation to "/"
  When "/" is 303-redirected to "/login?next=/" (spec 057 browser-login-redirect) and Playwright follows it
  Then the served 200 document is the login shell (internal/api/admin_ui_static/login.html)
  And proof_of_life asserts title "Sign in — Smackerel" and h1 "Sign in" and passes GREEN

Scenario: SCN-077-BUG-003-02 — the corrected 200 branch is adversarial, not a silent-pass
  Given the corrected proof_of_life 200-branch with exact-match title/h1 assertions
  When "/" serves anything other than the login shell (blank / error / PWA index / search page)
  Then the exact-match assertion FAILS (no bailout, no optional/conditional assertion, no URL-only fallback)
  And the before-fix run proves this by FAILING on the mismatched title ("Smackerel" vs "Sign in — Smackerel")
```

### Implementation Plan

1. Replace the `proof_of_life.spec.ts` 200-branch `toHaveTitle("Smackerel")` + `h1 toHaveText("Smackerel")` with the ACTUAL live-observed served login-shell identity: `toHaveTitle("Sign in — Smackerel")` + `h1 toHaveText("Sign in")`.
2. Correct the docstring to describe the spec-057 login-shell reality (unauthenticated `/` → 303 `/login?next=/` → served login shell at 200) instead of "the Smackerel PWA shell".
3. Keep the `[200, 401]` tolerance and the adversarial exact-match nature (no silent-pass bailout).

### Implementation Files

- `web/pwa/tests/proof_of_life.spec.ts`

### Change Boundary

Allowed file families:

- `web/pwa/tests/proof_of_life.spec.ts` (docstring + 200-branch assertions)
- `specs/077-pwa-browser-test-harness/bugs/BUG-003-proof-of-life-stale-login-shell-assertion/**` (this bug packet)

Excluded surfaces (MUST NOT be touched):

- All Go/Python app/runtime/source
- `internal/api/admin_ui_static/login.html` (the served login shell — the source of truth being matched, not changed)
- `web/pwa/index.html`, `web/pwa/assistant.html`
- `web/pwa/tests/auth_login.spec.ts`, `web/pwa/tests/unified_journey.spec.ts` (parallel-session-owned spec-100 F1-F7 remediation)
- `web/pwa/playwright.config.ts`, `scripts/runtime/web-e2e-ui.sh`, the e2e-ui stack composition
- The spec-057 `/` → `/login?next=/` browser-login-redirect middleware (ratified, intended behavior)

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command |
|-----|----------|----------|---------------|-------------------|---------|
| TP-077-BUG-003-01 | SCN-077-BUG-003-01 | e2e-ui | `web/pwa/tests/proof_of_life.spec.ts` | `proof of life: served / route renders against the test stack` | `./smackerel.sh test e2e-ui proof_of_life.spec.ts` |
| TP-077-BUG-003-02 | SCN-077-BUG-003-02 | e2e-ui (adversarial before/after) + guard | `web/pwa/tests/proof_of_life.spec.ts` | before-fix RED (title mismatch) → after-fix GREEN; `regression-quality-guard --bugfix` | `./smackerel.sh test e2e-ui proof_of_life.spec.ts` + `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix web/pwa/tests/proof_of_life.spec.ts` |
| TP-077-BUG-003-03 | SCN-077-BUG-003-01 | Scenario-specific Regression E2E | `web/pwa/tests/proof_of_life.spec.ts` | `proof of life: served / route renders against the test stack` (persistent e2e-ui regression) | `./smackerel.sh test e2e-ui proof_of_life.spec.ts` |
| TP-077-BUG-003-04 | SCN-077-BUG-003-01, SCN-077-BUG-003-02 | Broader Regression E2E | full PWA `web/pwa/tests/**` e2e-ui lane | full PWA suite discovers/compiles (52 tests, 28 files) + lane infra intact via the after-fix stack bring-up | `npx playwright test --list` (discovery) + `./smackerel.sh test e2e-ui proof_of_life.spec.ts` (lane) |

### Definition of Done

- [x] `proof_of_life.spec.ts` 200-branch asserts `toHaveTitle("Sign in — Smackerel")` (the ACTUAL live-observed served title) → Evidence: report.md#repro-before (Received `"Sign in — Smackerel"`) + report.md#code-diff-evidence
- [x] `proof_of_life.spec.ts` 200-branch asserts `h1` `toHaveText("Sign in")` (matches `internal/api/admin_ui_static/login.html:11`, single stable `h1`) → Evidence: report.md#code-diff-evidence
- [x] Docstring corrected to the spec-057 login-shell reality (`/` → 303 `/login?next=/` → served login shell at 200) → Evidence: report.md#code-diff-evidence
- [x] `[200, 401]` tolerance preserved; assertion is adversarial with no silent-pass bailout → Evidence: report.md#phase-regression (`regression-quality-guard` plain 0/0 + `--bugfix` adversarial signal, exit 0)
- [x] Before-fix reproduction (RED) captured live this session: `proof_of_life` FAILS, observed served title `"Sign in — Smackerel"` ≠ stale expected `"Smackerel"`, exit 1 → Evidence: report.md#repro-before
- [x] After-fix (GREEN) captured live this session: `./smackerel.sh test e2e-ui proof_of_life.spec.ts` → `1 passed`, exit 0, against the disposable `smackerel-test-e2e-ui` stack (all containers Healthy) → Evidence: report.md#repro-after
- [x] Change Boundary respected: only `proof_of_life.spec.ts` (docstring + 200-branch) + this bug packet; zero app/runtime/source or other-spec-test changes → Evidence: report.md#change-boundary (`git status -s`, `git diff --stat` = 1 file, +19/-8) + report.md#code-diff-evidence
- [x] Good-neighbor concurrency honored: e2e-ui stack brought up only when no foreign `smackerel-test*` stack was running; own project (`smackerel-test-e2e-ui`) torn down only → Evidence: report.md#repro-before + report.md#repro-after (good-neighbor gate output + scoped teardown)
- [x] Build Quality Gate: `regression-quality-guard` exit 0 (plain + `--bugfix`), `artifact-lint.sh` exit 0, `state-transition-guard.sh` exit 0 at `done` → Evidence: report.md#phase-regression + report.md#build-quality
- [x] SCN-077-BUG-003-01: proof_of_life 200 branch matches the real served login shell — unauthenticated `/` → 303 `/login?next=/` → served login shell, asserting title `"Sign in — Smackerel"` + `h1 "Sign in"`, passes GREEN → Evidence: report.md#repro-after
- [x] SCN-077-BUG-003-02: the corrected 200 branch is adversarial, not a silent-pass — it FAILS on a blank/error/other served `/` (proven by the before-fix RED title mismatch; `regression-quality-guard --bugfix` adversarial signal) → Evidence: report.md#repro-before + report.md#phase-regression
- [x] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior exists and passes — the persistent `proof_of_life.spec.ts` e2e-ui regression is GREEN this session → Evidence: report.md#repro-after
- [x] Broader E2E regression suite passes for the affected surface — the full PWA `web/pwa/tests/**` lane discovers/compiles clean (52 tests, 28 files, exit 0), and the after-fix run exercised the full disposable stack end-to-end; the isolated one-file diff is confined to a single test assertion (+19/-8), affecting no other spec or module → Evidence: report.md#phase-broader-regression + report.md#change-boundary
