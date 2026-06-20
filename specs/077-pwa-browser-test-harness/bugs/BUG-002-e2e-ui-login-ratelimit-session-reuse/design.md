# Design: BUG-002 E2E UI Card-Rewards Login Rate-Limit (Session Reuse)

## Overview

Collapse the card-rewards e2e-ui suite's ~40 per-test `/v1/web/login` POSTs to at most one real login per Playwright worker by caching the `auth_token` cookie at worker scope and replaying it on subsequent logins. Pure test-side change; the production rate limiter is untouched.

## Current Truth

- `/v1/web/login` is rate-limited by `httprate.LimitByIP(20, 1*time.Minute)` in `internal/api/router.go` (spec 070). It is asserted by `internal/api/web_login_ratelimit_test.go` — a deliberate, tested credential-stuffing defense.
- Ten `cardrewards_*.spec.ts` specs call `login()` in a `beforeEach`:
  - `wallet`, `categories`, `offers_selections` defined their OWN local `async function login(page)` (each a real POST).
  - `dashboard`, `chrome`, `rotating_verify`, `bonuses`, `invites`, `recommendations`, `admin` call the shared `_support/cardrewards.ts` `login()`.
- `auth_login.spec.ts` exercises the real login FLOW (TP-077-03-01..04) via its own `postLoginForm`; it is intentionally a real login and is OUT OF SCOPE.
- The disposable e2e-ui stack runs dev-token mode (`AuthConfig.Enabled=false`); each login exchanges the same `SMACKEREL_AUTH_TOKEN` for an equivalent `auth_token` cookie.

## Considered Approaches

| Approach | Net logins | Churn | Risk | Verdict |
|----------|-----------|-------|------|---------|
| **A. Worker-scoped cookie cache in shared helper** | 1 / worker (~2 total) | Low — one helper + 3 spec edits; `auth_login` untouched | Low | **Chosen** |
| B. Playwright `globalSetup` + `storageState` | ~1 total | Higher — new globalSetup, config `use.storageState`, MUST exclude `auth_login` (real-flow) AND would silently authenticate `photos`/`assistant`/`qf` specs that currently never log in | Medium (over-reach: changes the auth posture of unrelated specs) | Rejected |

Approach A is surgical: only the specs that already log in are touched; `auth_login.spec.ts` and every non-card-rewards spec keep their exact current behavior. Approach B was rejected because a global `storageState` would alter the authentication posture of specs that deliberately run unauthenticated, exceeding the minimal-fix mandate.

## Fix Design

`_support/cardrewards.ts`:

```ts
type StoredCookie = Awaited<ReturnType<BrowserContext["cookies"]>>[number];
// Per-worker: Playwright evaluates the module once per worker process.
let cachedAuthCookie: StoredCookie | null = null;

export async function login(page: Page, next = "/cards"): Promise<void> {
  if (cachedAuthCookie) {
    await page.context().addCookies([cachedAuthCookie]); // replay, NO POST
    return;
  }
  const resp = await page.request.post("/v1/web/login", { /* unchanged */ });
  expect([200, 302, 303], `…got ${resp.status()}`).toContain(resp.status());
  const auth = (await page.context().cookies()).find((c) => c.name === "auth_token");
  if (!auth) throw new Error("…no auth_token cookie…cannot establish a reusable session");
  cachedAuthCookie = auth; // capture once per worker
}
```

- `cardrewards_wallet.spec.ts`, `cardrewards_categories.spec.ts`, `cardrewards_offers_selections.spec.ts`: delete the local `login()` and `import { login } from "./_support/cardrewards"`. In `categories` the now-dead `AUTH_TOKEN`/`requireAuthToken`/`type Page` import are also removed (in `wallet`/`offers_selections` `requireAuthToken` is still used by the API seed helpers, so it stays).

### Why worker-scope is correct

Playwright runs each worker in its own OS process and evaluates each module once per worker, so a module-level `let` is naturally per-worker state. `next` only affects the (ignored, `maxRedirects:0`) redirect target of the FIRST POST; per-test navigation is still each test's own `page.goto(...)`. Because dev-token mode issues an equivalent session every time, one captured cookie authenticates the whole worker. `page.request` shares the browser context's cookie jar, so the login response's `Set-Cookie` lands on `page.context()` and is readable via `cookies()`.

## Test Strategy

Node-level adversarial regression (`node:test`, `--experimental-strip-types`, mirrors `csp.test.ts`; NO live stack, runs on the OOM-prone dev host):

- **SCN-077-BUG-002-01** — drive `login()` twice with stub pages. The first POSTs once and caches; the SECOND stub's POST THROWS, so a cache miss (bug reintroduced) fails the test. Asserts exactly one POST total + cookie replayed via `addCookies`.
- **SCN-077-BUG-002-02** — `fs`-scan all `cardrewards_*.spec.ts` and assert none carry a per-test `/v1/web/login` POST, with an adversarial self-check that the detector regex matches a known-bad line (non-tautology guard).

Driver `tests/unit/web/bug_077_002_login_session_reuse_test.sh` is auto-discovered by `./smackerel.sh test unit` (spec 077 SCOPE-2 `tests/unit/web/*.sh` convention) and exports `SMACKEREL_AUTH_TOKEN` (the helper reads it at module-load).

## Change Boundary

- **Modified:** `web/pwa/tests/_support/cardrewards.ts`; `web/pwa/tests/cardrewards_{wallet,categories,offers_selections}.spec.ts`
- **Added:** `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts`; `tests/unit/web/bug_077_002_login_session_reuse_test.sh`
- **Untouched (HARD constraint):** `internal/api/router.go`, `internal/api/web_login_ratelimit_test.go`, `auth_login.spec.ts`, all product/runtime code, the e2e-ui stack composition, `web/pwa/playwright.config.ts`, `scripts/runtime/web-e2e-ui.sh`.

## Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Cached cookie expires mid-suite | Very low | Suite runs in seconds; dev-token session is long-lived; capture is per-worker on first login |
| A future card-rewards spec re-adds a per-test login | Medium over time | SCN-077-BUG-002-02 structural guard fails the unit lane if any `cardrewards_*.spec.ts` reintroduces a `/v1/web/login` POST |
| Cross-test data bleed from shared session | None new | Every login already used the same dev user; tests isolate via unique suffixes (unchanged) |
