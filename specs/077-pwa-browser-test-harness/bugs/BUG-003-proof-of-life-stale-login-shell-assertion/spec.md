# Bug: BUG-003 proof_of_life e2e-ui stale 200-branch assertion (served login shell)

**Status:** Fixed (certified done this session — see report.md)
**Severity:** Medium
**Found By:** spec-077 e2e-ui lane (surfaced during the BUG-077-002 `./smackerel.sh test e2e-ui` run this session; previously routed out-of-boundary by BUG-077-002 → report.md#312)
**Date:** 2026-07-19
**Root Cause:** `web/pwa/tests/proof_of_life.spec.ts` asserted that the served `/` document (200 branch) is the "Smackerel" PWA index shell (`toHaveTitle("Smackerel")` + `h1` `toHaveText("Smackerel")`). Per **spec 057 (browser-login-redirect)** an unauthenticated browser navigation to `/` is 303-redirected to `/login?next=/`, which Playwright follows, so the served 200 document is actually the **login shell** (`internal/api/admin_ui_static/login.html`: `<title>Sign in — Smackerel</title>`, `<h1>Sign in</h1>`). The 200-branch expectation was stale versus the real served identity.

---

## Problem Statement

The spec-077 PWA browser e2e-ui harness (`./smackerel.sh test e2e-ui` → `scripts/runtime/web-e2e-ui.sh` → Playwright under `web/pwa/`) runs `proof_of_life.spec.ts`, which does `page.goto("/")`, tolerates HTTP 200 or 401, and — **if 200** — asserts:

```ts
await expect(page).toHaveTitle("Smackerel");
await expect(page.locator("h1")).toHaveText("Smackerel");
```

Against the current disposable test stack this **fails**. The disposable e2e-ui stack runs dev-token mode (`AuthConfig.Enabled=false`), so `/` resolves to `200` rather than the production-default `401`. But the served `200` document is **not** the PWA index shell — it is the **login shell**:

- `page.goto("/")` (an unauthenticated browser navigation) is `303`-redirected to `/login?next=/` by the spec-057 browser-login-redirect middleware (`internal/api/auth_browser_redirect.go`; `tests/e2e/auth/browser_login_test.go`: "GET / → 303 to /login?next=/").
- Playwright follows the redirect, so `response.status()` is the **final** `200` and the rendered document is `internal/api/admin_ui_static/login.html`.
- That page's real identity is `<title>Sign in — Smackerel</title>` and `<h1>Sign in</h1>`.

So the 200-branch's `"Smackerel"`/`"Smackerel"` expectation was stale against the real served login shell.

### Why it stayed dormant until now

The stale assertion has existed since the test was authored (2026-06-02, commit `4072f30a`) but never actually executed against a live `200` because the e2e-ui disposable stack could not boot until **spec 100 (F-100-OPT-01/03)** de-weighted ollama (nginx stub) and profile-gated the ml sidecar OFF. Once the stack could come up, the proof-of-life 200-branch executed for the first time and surfaced the staleness. BUG-077-002 observed the same failure this session and correctly routed it out-of-boundary to the parent (`BUG-002/report.md#312`: "One unrelated out-of-boundary lane failure (proof_of_life.spec.ts) routed to parent").

## Reproduction

- **Before fix (live, this session):** `./smackerel.sh test e2e-ui proof_of_life.spec.ts` → `1 failed`, exit 1: `expect(locator).toHaveTitle` Expected `"Smackerel"`, **Received `"Sign in — Smackerel"`** (full capture in [report.md](report.md#repro-before)).

## Expected Behavior

The proof-of-life MUST assert the **real** served document identity for the 200 branch — the served login shell (`title "Sign in — Smackerel"`, `h1 "Sign in"`) — while keeping:

1. The 200-or-401 tolerance (`[200, 401]`) unchanged.
2. The adversarial nature: exact-match assertions that still **fail** if `/` serves nothing / an error page / any other document (no silent-pass bailout).
3. Zero changes to app/runtime/source, other spec test files, or the e2e-ui stack composition — this is a test-staleness fix, not an app change. The `/` → `/login?next=/` redirect is the ratified, intended spec-057 behavior.

## Fix Plan

Test-side assertion update only:

1. Update the `proof_of_life.spec.ts` 200-branch to `toHaveTitle("Sign in — Smackerel")` + `h1` `toHaveText("Sign in")` — the ACTUAL live-observed served values.
2. Correct the docstring to describe the spec-057 login-shell reality (unauthenticated `/` → 303 `/login?next=/` → served login shell at 200) instead of "the Smackerel PWA shell".

## Related Artifacts

- Parent spec: [spec.md](../../spec.md)
- Fixed file: `web/pwa/tests/proof_of_life.spec.ts`
- Real served source of truth: `internal/api/admin_ui_static/login.html` (`title "Sign in — Smackerel"`, `h1 "Sign in"`)
- Redirect behavior owner: spec 057 (browser-login-redirect) — `internal/api/auth_browser_redirect.go`
- Prior routing: `specs/077-pwa-browser-test-harness/bugs/BUG-002-e2e-ui-login-ratelimit-session-reuse/report.md` (routed this exact failure out-of-boundary)
- MUST-NOT-TOUCH: all app/runtime/source, `internal/api/admin_ui_static/login.html`, `web/pwa/index.html`, `web/pwa/assistant.html`, `auth_login.spec.ts`, `unified_journey.spec.ts`, the e2e-ui stack composition
- Test anchors: SCN-077-BUG-003-01, SCN-077-BUG-003-02
