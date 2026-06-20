# Report: BUG-002 E2E UI Card-Rewards Login Rate-Limit (Session Reuse Fix)

## Summary

The "E2E UI" CI lane was chronically red: 9/42 tests (all `cardrewards_*.spec.ts`) failed with `/v1/web/login ... got 429` because each card-rewards spec re-logged-in per test, exceeding the production `httprate.LimitByIP(20, 1*time.Minute)` web-login limiter (spec 070) on the shared CI runner IP. The fix is test-side session reuse only: a worker-scoped `auth_token` cookie cache in the shared `_support/cardrewards.ts` `login()`, so the suite performs at most one real login per worker (~2 total). The production limiter and the real-flow `auth_login.spec.ts` tests are untouched. All locally-runnable checks pass; the full e2e-ui lane (OOM-prone on this dev host) is confirmed by CI, which the parent owns.

## Discovery

- **Date:** 2026-06-20
- **Found By:** CI "E2E UI" GitHub Actions lane — run **27877380385**, commit **50c71583**
- **Symptom:** healthy disposable stack, 42 tests on 2 workers; 33 pass, **9 fail — ALL `web/pwa/tests/cardrewards_*.spec.ts`** — every one with:
  ```
  Error: /v1/web/login must accept the dev token; got 429
  expect(received).toContain(expected)   Expected value in [200,302,303]; Received: 429
  ```

## Root Cause Analysis

`/v1/web/login` is rate-limited by `httprate.LimitByIP(20, 1*time.Minute)` in `internal/api/router.go` (owned by spec 070, asserted by `internal/api/web_login_ratelimit_test.go`). Each card-rewards spec re-logs-in per test in a `beforeEach`; with ~40 logins from one shared CI runner IP, the suite blows past 20/IP/min and the later card-rewards tests get 429. In dev-token mode (`AuthConfig.Enabled=false`) every login yields an equivalent `auth_token` session, so re-logging-in per test is pure waste that only trips the limiter.

The limiter is a deliberate credential-stuffing defense and is itself under test — so the fix is **test-side session reuse only**; the limiter is NOT touched.

### Before Fix — real `/v1/web/login` POST call sites + per-test login invocations

```
$ grep -rnE 'request\.post\("/v1/web/login"' web/pwa/tests
web/pwa/tests/cardrewards_offers_selections.spec.ts:33:  const resp = await page.request.post("/v1/web/login", {
web/pwa/tests/cardrewards_categories.spec.ts:37:  const resp = await page.request.post("/v1/web/login", {
web/pwa/tests/cardrewards_wallet.spec.ts:43:  const resp = await page.request.post("/v1/web/login", {
web/pwa/tests/_support/cardrewards.ts:46:  const resp = await page.request.post("/v1/web/login", {
web/pwa/tests/auth_login.spec.ts:48:  const resp = await request.post("/v1/web/login", {   # real-flow (intentional)

$ grep -rnE 'await login\(' web/pwa/tests
web/pwa/tests/cardrewards_admin.spec.ts:40:    await login(page, "/cards/admin");
web/pwa/tests/cardrewards_recommendations.spec.ts:27:    await login(page, "/cards/recommendations");
web/pwa/tests/cardrewards_invites.spec.ts:29:    await login(page, "/cards/admin");
web/pwa/tests/cardrewards_bonuses.spec.ts:27:    await login(page, "/cards/bonuses");
web/pwa/tests/cardrewards_offers_selections.spec.ts:75:    await login(page);
web/pwa/tests/cardrewards_categories.spec.ts:54:    await login(page);
web/pwa/tests/cardrewards_rotating_verify.spec.ts:64:    await login(page, "/cards/rotating");
web/pwa/tests/cardrewards_wallet.spec.ts:95:    await login(page);
web/pwa/tests/cardrewards_chrome.spec.ts:27:    await login(page);
web/pwa/tests/cardrewards_dashboard.spec.ts:33:    await login(page);
```

Four per-test POST sites (3 local `login()` + 1 shared helper) feed ten card-rewards specs' `beforeEach` → ~40 logins/run.

## Fix Implementation

**Approach A — worker-scoped cookie cache in the shared `_support/cardrewards.ts` helper.** The first `login()` per Playwright worker POSTs `/v1/web/login` once and captures the `auth_token` cookie; every later `login()` in that worker replays the cached cookie via `BrowserContext.addCookies` with NO POST. All ten card-rewards specs route through this one cached helper → 1 real login per worker (~2 total), far under 20/IP/min.

### Code After (shared helper, abridged)

```ts
type StoredCookie = Awaited<ReturnType<BrowserContext["cookies"]>>[number];
let cachedAuthCookie: StoredCookie | null = null; // per-worker (module evaluated once per worker)

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

The three local-`login()` specs (`wallet`, `categories`, `offers_selections`) had their local login removed and now `import { login } from "./_support/cardrewards"`. The seven specs already on the shared helper were not edited — they inherit the cache.

## Test Evidence

> **Honest scope note:** the full e2e-ui harness (`./smackerel.sh test e2e-ui` / `build` / `up`) needs the live stack + Ollama and OOMs this dev host, so it was **NOT** run locally. The checks below are the cheaply-and-honestly verifiable ones; **authoritative end-to-end confirmation is the next CI "E2E UI" run** after the parent commits + pushes.

### 1. After Fix — per-test login POSTs removed

```
$ grep -rnE 'request\.post\("/v1/web/login"' web/pwa/tests
web/pwa/tests/_support/cardrewards.ts:82:  const resp = await page.request.post("/v1/web/login", {
web/pwa/tests/auth_login.spec.ts:48:  const resp = await request.post("/v1/web/login", {

$ grep -rnE 'function login\(' web/pwa/tests
web/pwa/tests/_support/cardrewards.ts:77:export async function login(page: Page, next = "/cards"): Promise<void> {
```

Only the single shared-helper POST and the `auth_login.spec.ts` real-flow POST remain; all three local `login()` functions are gone.

### 2. Playwright loads the full suite (no syntax/type errors)

```
$ SMACKEREL_BASE_URL="http://127.0.0.1:9" SMACKEREL_AUTH_TOKEN="dummy-list-only" npx playwright test --list
  ... (42 test titles across 26 files; auth_login.spec.ts TP-077-03-01..07 still listed) ...
Total: 42 tests in 26 files
PLAYWRIGHT_LIST_EXIT=0
```

Re-checked after adding the regression file — still `Total: 42 tests in 26 files`; the new `_support/cardrewards_login_session_reuse.test.ts` is correctly NOT discovered as a spec (testMatch `**/*.spec.ts` + testIgnore `_support/**`).

### 3. TypeScript

The IDE TypeScript language service reports **no errors** on all four edited files. The CLI `npx tsc --noEmit` fails only with a **pre-existing, fix-independent** env gap:

```
$ npx tsc --noEmit
error TS2688: Cannot find type definition file for 'node'.   # tsconfig "types": ["node"]; @types/node not installed
```

`@types/node` is absent from `web/pwa/package.json` and from both `web/pwa/node_modules/@types` and the repo-root `node_modules` (confirmed), so TS2688 reproduces identically on the pristine pre-fix tree. It is unrelated to this change. `playwright test --list` (esbuild-transpiles every spec) and the node regression both load the edited files cleanly.

### 4. Adversarial regression (TP-077-BUG-002-01/02) — PASS

```
$ bash tests/unit/web/bug_077_002_login_session_reuse_test.sh
[bug_077_002_login_session_reuse] node v22.22.0
TAP version 13
ok 1 - SCN-077-BUG-002-01 — login POSTs once per worker, then reuses the cached session
ok 2 - SCN-077-BUG-002-02 — no cardrewards spec reintroduces a per-test /v1/web/login POST
1..2
# tests 2
# pass 2
# fail 0
PASS: bug_077_002_login_session_reuse_test (SCN-077-BUG-002-01 / SCN-077-BUG-002-02)
```

### 5. Adversarial proof — the regression FAILS when the bug is reintroduced

To prove the regression is non-tautological, the worker cache was temporarily disabled (`if (cachedAuthCookie)` → `if (false && cachedAuthCookie)`) and the driver re-run:

```
$ bash tests/unit/web/bug_077_002_login_session_reuse_test.sh   # with cache disabled
not ok 1 - SCN-077-BUG-002-01 — login POSTs once per worker, then reuses the cached session
  error: 'REGRESSION: login() POSTed /v1/web/login a second time instead of reusing the cached session — this is the BUG-002 rate-limit defect'
  stack: |-
    ~/smackerel/web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts:98:11
    Object.post (~/smackerel/web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts:54:32)
    login (~/smackerel/web/pwa/tests/_support/cardrewards.ts:82:35)
ok 2 - SCN-077-BUG-002-02 — no cardrewards spec reintroduces a per-test /v1/web/login POST
# tests 2
# pass 1
# fail 1
FAIL: BUG-002 regression node:test run failed (exit=1)
DRIVER_EXIT=1   # non-zero == regression correctly detects the reintroduced bug
```

The cache was then restored and the artifact verified gone + green again:

```
$ grep -n 'false &&' web/pwa/tests/_support/cardrewards.ts
clean: no 'false &&' artifact
$ bash tests/unit/web/bug_077_002_login_session_reuse_test.sh
# pass 2
# fail 0
DRIVER_EXIT=0
```

### 6. Production limiter untouched — `git status`

```
$ git status --porcelain
 M web/pwa/tests/_support/cardrewards.ts
 M web/pwa/tests/cardrewards_categories.spec.ts
 M web/pwa/tests/cardrewards_offers_selections.spec.ts
 M web/pwa/tests/cardrewards_wallet.spec.ts
?? tests/unit/web/bug_077_002_login_session_reuse_test.sh
?? web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts
```

Zero changes under `internal/` — `internal/api/router.go` and `internal/api/web_login_ratelimit_test.go` are untouched. `auth_login.spec.ts` is untouched.

### 7. CI Confirmation (2026-06-20) — "E2E UI" lane GREEN on commit `0a4a13aa`

The authoritative end-to-end signal — the CI "E2E UI" lane — is now confirmed GREEN on the live runner:

```
$ gh run view 27878481805 --json workflowName,conclusion,headSha,status,jobs
workflow:   E2E UI
status:     completed
conclusion: success
headSha:    0a4a13aa1a5173538f52dbeead876e7e9dc4580a
jobs:       e2e-ui = success   (✓ e2e-ui in 5m42s; ✓ Run PWA browser e2e-ui harness)
```

- **Run:** `27878481805` (`E2E UI` workflow) — conclusion **success**, status **completed**, `e2e-ui` job **success**.
- **Commit:** `0a4a13aa1a5173538f52dbeead876e7e9dc4580a` (current `origin/main` HEAD; the fix landed via parent commit `50c71583` and is green on HEAD `0a4a13aa`).
- The 9 `cardrewards_*.spec.ts` tests that previously failed with HTTP 429 on `/v1/web/login` (discovery run `27877380385`, commit `50c71583`) now pass — the worker-scoped session-reuse cache keeps the suite under the spec-070 `httprate.LimitByIP(20, 1*time.Minute)` web-login limiter on the shared CI runner IP.

Verified independently this session with `gh run view 27878481805 --json conclusion,headSha,jobs` (conclusion=`success`, headSha=`0a4a13aa…`, `e2e-ui` job=`success`).

## Completion Statement

Implementation and all locally-runnable verification are complete: the worker-scoped cache is in place, the three local logins are routed through the shared cached helper, the adversarial regression passes (2/2) and is proven to fail when the cache is disabled, `playwright test --list` loads all 42 tests, and `git status` confirms zero changes under `internal/` (the production limiter is untouched). The authoritative end-to-end signal — the CI "E2E UI" lane — is now GREEN: run `27878481805` (`e2e-ui` job) conclusion **success** on HEAD `0a4a13aa` cleared the 9 card-rewards 429 failures (see Test Evidence §7).

This bug is **not** marked `done`, however: the real state-transition guard (`bash .github/bubbles/scripts/state-transition-guard.sh`) BLOCKS a `done` promotion (29 findings) because this test-only fix was never taken through the full bugfix-fastlane certification pipeline — Gate G022 requires regression/simplify/stabilize/security/validate/audit phases (only implement+test were executed); Check 8A requires scenario-specific E2E regression DoD+TestPlan structure; Gate G093 requires an implementation/test delta in the certifying change (the code fix is in prior commit `50c71583`, so a specs-only close-out cannot satisfy it); plus G055/G057/G068/G053 artifact-shape gaps. Per the no-fabrication / no-bypass policy, status is HELD at `in_progress` (deferred) — matching the BUG-073-003 light-touch-fix precedent. Full done-certification is reserved for the parent orchestrator (bubbles.goal).

## Files Changed

| File | Change |
|------|--------|
| `web/pwa/tests/_support/cardrewards.ts` | Added worker-scoped `auth_token` cookie cache to `login()` (POST once per worker, then replay via `addCookies`) |
| `web/pwa/tests/cardrewards_wallet.spec.ts` | Removed local `login()`; imports the shared cached `login` |
| `web/pwa/tests/cardrewards_categories.spec.ts` | Removed local `login()` + now-dead `AUTH_TOKEN`/`requireAuthToken`/`type Page`; imports the shared cached `login` |
| `web/pwa/tests/cardrewards_offers_selections.spec.ts` | Removed local `login()`; imports the shared cached `login` |
| `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` | **New** — adversarial node regression (SCN-077-BUG-002-01 worker-cache behavior + SCN-077-BUG-002-02 structural guard) |
| `tests/unit/web/bug_077_002_login_session_reuse_test.sh` | **New** — auto-discovered driver running the regression under `node --experimental-strip-types --test` |

Card-rewards specs already routed through the shared helper and therefore needed NO edit (they inherit the cache): `dashboard`, `chrome`, `rotating_verify`, `bonuses`, `invites`, `recommendations`, `admin`.

## Pending (parent-owned)

- The code fix is already committed + pushed (`50c71583` → HEAD `0a4a13aa`) and CI-verified GREEN (CI "E2E UI" run `27878481805`, `e2e-ui` job success). This bug's functional outcome is achieved.
- Full bugfix-fastlane done-certification is deferred to the parent: it requires either (a) executing the remaining pipeline phases (regression/simplify/stabilize/security/validate/audit) and adding the scenario-manifest + scenario-specific E2E regression planning the state-transition guard demands, then certifying in the same change that carries the code delta (Gate G093); or (b) accepting the deferred `in_progress` state per the BUG-073-003 precedent for light-touch test-infrastructure fixes.
