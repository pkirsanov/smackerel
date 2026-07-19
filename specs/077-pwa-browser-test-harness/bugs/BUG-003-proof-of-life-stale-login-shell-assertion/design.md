# Design: BUG-003 proof_of_life e2e-ui stale 200-branch assertion (served login shell)

## Overview

Update `web/pwa/tests/proof_of_life.spec.ts`'s 200-branch assertions so they match the **real** served `/` document identity (the spec-057 login shell) instead of the stale "Smackerel" PWA-index expectation. Pure test-side change; zero app/runtime/source edits.

## Current Truth

- `page.goto("/")` against the e2e-ui `baseURL` (= `CORE_EXTERNAL_URL`) hits the Go core. An **unauthenticated browser navigation** to `/` is `303`-redirected to `/login?next=/` by the spec-057 browser-login-redirect middleware (`internal/api/auth_browser_redirect.go`; asserted by `tests/e2e/auth/browser_login_test.go` "GET / → 303 to /login?next=/" and `specs/057-browser-login-redirect/uservalidation.md` "Browser visit to `/` redirects to `/login?next=/`").
- Playwright follows the `303`, so `response.status()` is the **final** `200` and the served document is `internal/api/admin_ui_static/login.html`:
  - `<title>Sign in — Smackerel</title>` (line 6)
  - `<h1>Sign in</h1>` (line 11 — a single `h1`, rendered OUTSIDE the `{{if not .AuthEnabled}}` conditional, so it is stable in both dev-bypass and auth-enabled modes)
- The disposable e2e-ui stack runs dev-token mode (`AuthConfig.Enabled=false`, per `scripts/runtime/web-e2e-ui.sh`), so `/` resolves to `200` here rather than the production-default `401`.
- The chi `/` route's own `SearchPage` handler (`internal/web/handler.go`, `search.html` → `title "{{.Title}} - Smackerel"`, `h1 "Ask Smackerel"`) is NOT what the browser sees at `/`, because the unauthenticated navigation is redirected to the login shell first.
- `proof_of_life.spec.ts` was authored 2026-06-02 (`4072f30a`) with a speculative `"Smackerel"`/`"Smackerel"` 200-branch. It never executed against a live `200` until spec 100 (F-100-OPT-01/03) let the e2e-ui stack boot, which surfaced the staleness. BUG-077-002 routed the exact failure out-of-boundary to the parent this session.

## Considered Approaches

| Approach | Change | Risk | Verdict |
|----------|--------|------|---------|
| **A. Update 200-branch to the real served login-shell identity (exact match)** | title `"Sign in — Smackerel"` + `h1 "Sign in"`; correct the docstring | Low — one test file, exact-match keeps it adversarial | **Chosen** |
| B. Loosen to `toHaveTitle(/Smackerel/)` + non-empty `h1` | regex/looseness | Medium — less faithful; the task asks for the ACTUAL served title/h1; looser check weakens the served-identity contract | Rejected |
| C. Treat `/` → login as an app bug and change routing so `/` serves search/assistant | app/runtime change | High — the `/` → `/login?next=/` redirect is ratified spec-057 intended behavior; changing it would be an out-of-scope, wrong "fix" of a real, correct behavior | Rejected |

Approach A is the minimal, faithful fix: it asserts the ACTUAL live-observed served values and stays adversarial. Approach C was rejected because the redirect is intended (spec 057), so `/` serving the login shell is correct — the staleness is entirely in the test's expectation.

## Fix Design

`web/pwa/tests/proof_of_life.spec.ts` — 200 branch:

```ts
if (status === 200) {
  // Spec 057 (browser-login-redirect): an unauthenticated browser
  // navigation to `/` is 303-redirected to `/login?next=/`, which
  // Playwright follows, so the served 200 document is the login shell
  // (internal/api/admin_ui_static/login.html) — NOT the PWA index. These
  // are the ACTUAL served identities; the exact matches keep the check
  // adversarial (a blank/error/other-page `/` fails), no silent-pass.
  await expect(page).toHaveTitle("Sign in — Smackerel");
  await expect(page.locator("h1")).toHaveText("Sign in");
}
```

The `[200, 401]` tolerance check above the branch is unchanged. The docstring is corrected to describe the spec-057 login-shell reality.

### Why this stays adversarial (no silent-pass)

Exact-string `toHaveTitle("Sign in — Smackerel")` + `toHaveText("Sign in")` **fail** if `/` serves a blank page, a 500/error page, the PWA index shell (regression away from the login redirect), or the chi `SearchPage`. There is no `if (...) return;` bailout, no optional/conditional assertion, and no URL-only fallback. The before-fix RED (which failed on the mismatched title) proves the branch actually executes and asserts. `regression-quality-guard --bugfix` reports an adversarial signal with 0 violations.

## Test Strategy

The authoritative proof is the live e2e-ui run of the single spec against the disposable `smackerel-test-e2e-ui` stack (good-neighbor gated), captured both before (RED) and after (GREEN) the fix this session:

- **SCN-077-BUG-003-01** — after the fix, `page.goto("/")` → `200` served login shell → `proof_of_life` passes GREEN (`internal/api/admin_ui_static/login.html` identity).
- **SCN-077-BUG-003-02** — the 200-branch is adversarial: it FAILED before the fix on the mismatched title (`Expected "Smackerel"`, `Received "Sign in — Smackerel"`), proving it is not a silent-pass; `regression-quality-guard` (plain + `--bugfix`) confirms 0 violations + adversarial signal.

## Change Boundary

- **Modified:** `web/pwa/tests/proof_of_life.spec.ts` (docstring + 200-branch assertions only)
- **Added:** this bug packet under `specs/077-pwa-browser-test-harness/bugs/BUG-003-proof-of-life-stale-login-shell-assertion/**`
- **Untouched (HARD constraint):** all Go/Python app/runtime/source, `internal/api/admin_ui_static/login.html`, `web/pwa/index.html`, `web/pwa/assistant.html`, `web/pwa/tests/auth_login.spec.ts`, `web/pwa/tests/unified_journey.spec.ts`, `web/pwa/playwright.config.ts`, `scripts/runtime/web-e2e-ui.sh`, the e2e-ui stack composition, the spec-057 redirect middleware.

## Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| The served login-shell title/h1 later changes | Low | Exact-match fails loudly if the served identity drifts — the assertion is a live contract, re-verified by the e2e-ui lane |
| `/` redirect behavior later changes (e.g., unauthenticated `/` serves search) | Low | That would be an intended app change owned by the routing spec; this test would fail loudly and be updated alongside it (correct coupling, not silent-pass) |
