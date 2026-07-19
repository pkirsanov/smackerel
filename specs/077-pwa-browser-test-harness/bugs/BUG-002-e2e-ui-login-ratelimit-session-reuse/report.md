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

## This-Session Certification Evidence (2026-07-19)

Executed in-session by `bubbles.workflow`, parent-expanded by `bubbles.iterate` (runSubagent is unavailable in this runtime; documented smackerel precedent, BUG-025-005). The host had ~40 GiB free memory and no foreign `smackerel-test*` / e2e containers running, so the shared e2e-ui stack was free (good-neighbor verified). Only this bug's own `smackerel-test-e2e-ui` compose project was brought up and it was torn down on exit.

### e2e-ui-live-run

`./smackerel.sh test e2e-ui` built `smackerel-core`, brought up the disposable `smackerel-test-e2e-ui` stack (ollama nginx-stub + ML profile-gated OFF per spec-100 F-100-OPT-01/03; all containers reported Healthy), ran Playwright, then tore the stack down:

```
Running 52 tests using 4 workers
  ✓  4 …oke › TP-077-03-01 — login page renders form + CSP-clean baseline (3.6s)
  ✓ 12 …bmission sets session cookie and lands on post-login destination (1.8s)
  ✓ 15 …min link → generate → reveal → list → revoke → anonymous-blocked (2.0s)
  ✓ 17 …03-04 — logout clears the session cookie and redirects to /login (1.1s)
  ✓ 20 … › SCN-083-J06 — add and edit an offer with a shared limit group (2.1s)
  ✓ 21 …ations › SCN-083-K02 — add, edit, and star a recommendation (2.5s)
  ✓ 22 …n the login cycle fails the suite via the _support/csp.ts guard (504ms)
  ✓ 25 …1 — add a custom card; wallet lists nickname, type, note, active (2.0s)
  ✓ 26 …rify clears the flag and is not overwritten by a later reconcile (2.2s)
  ✓ 27 …e 10 — Offers & Selections › SCN-083-J07 — tiered selection save (1.6s)
  ✓ 44 … Card Rewards Wallet › SCN-083-J05 — toggle card activation off (541ms)
  ✘ 39 …1 › proof of life: served / route renders against the test stack (5.7s)
  1 failed
    proof_of_life.spec.ts:28:1 › proof of life: served / route renders against the test stack
  9 skipped
  42 passed (26.3s)
[web-e2e-ui] Tearing down disposable test stack (project smackerel-test-e2e-ui)...
```

**42 passed, 9 skipped (ENV-CONSTRAINED chaos journeys), 1 failed.** Every card-rewards login/wallet/categories/offers/recommendations/rotating test that previously returned HTTP 429 on `/v1/web/login` now passes — the worker-scoped session-reuse cache keeps the whole suite under the spec-070 `httprate.LimitByIP(20, 1*time.Minute)` limiter on the shared runner IP. The string `got 429` appears nowhere in the run. The single failure (`proof_of_life.spec.ts`) is unrelated and out-of-boundary (see "## Discovered Unrelated Failure" below).

### phase-regression

Fresh this-session node adversarial driver (`tests/unit/web/bug_077_002_login_session_reuse_test.sh`) + regression-quality guard:

```
[bug_077_002_login_session_reuse] node v22.22.0
TAP version 13
ok 1 - SCN-077-BUG-002-01 — login POSTs once per worker, then reuses the cached session
ok 2 - SCN-077-BUG-002-02 — no cardrewards spec reintroduces a per-test /v1/web/login POST
1..2
# tests 2
# pass 2
# fail 0
PASS: bug_077_002_login_session_reuse_test (SCN-077-BUG-002-01 / SCN-077-BUG-002-02)
DRIVER_EXIT=0

✅ Adversarial signal detected in web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
REGQUAL_EXIT=0
```

The regression is non-tautological: the committed RED proof (Test Evidence §5) shows the driver FAILS when the worker cache is disabled, and SCN-077-BUG-002-02 carries a built-in adversarial self-check that its detector regex matches a known-bad line.

### phase-simplify

The fix is minimal and proportional: one worker-scoped cache in the single shared `_support/cardrewards.ts` `login()` helper, three spec edits routing onto it, and removal of three duplicated local `login()` helpers + now-dead symbols. No new dependency, framework, schema, transport, or config was introduced. `git show --stat 0a4a13aa` confirms the change is test-only (`web/pwa/tests/**` + this bug packet), 739 insertions / 66 deletions across test files.

### phase-stabilize

The cache is deterministic: Playwright evaluates the module once per worker OS process, so `cachedAuthCookie` is naturally per-worker state; the first login POSTs once and captures, later logins replay via `addCookies`. The live e2e-ui suite is green and stable this session, and the harness teardown trap (`down --remove-orphans --volumes`) is idempotent and scoped to the dedicated `smackerel-test-e2e-ui` project only — the persistent dev stack and the `smackerel-test` integration stack are never touched.

### phase-security

The spec-070 credential-stuffing limiter is untouched by the fix and still present in current main:

```
$ git show 0a4a13aa --name-only --format='' -- internal/api/router.go internal/api/web_login_ratelimit_test.go internal/
(no internal/ lines == fix 0a4a13aa touched zero internal/ limiter code)

$ grep -nE 'LimitByIP\(20, 1\*time.Minute\)|/v1/web/login' internal/api/router.go
328:            r.Use(httprate.LimitByIP(20, 1*time.Minute))
329:            r.Post("/v1/web/login", deps.HandleWebLogin)
```

The change is session-reuse only — no `trusted_proxies` / `X-Forwarded-For` spoofing, no new auth surface, no secret material, no new egress. `implementation-reality-scan.sh` (including Scan 7 IDOR / auth-bypass and Scan 8 silent-decode) reported **0 violations** (REALITY_EXIT=0). The `auth_login.spec.ts` real-login-flow tests are untouched, so the real login surface is still exercised.

### phase-validate

Independent in-session re-verification: the live e2e-ui card-rewards regression is GREEN (zero 429); the node regression driver is GREEN (2/2) with a proven RED→GREEN; the regression-quality and implementation-reality guards exit 0; the real code delta is evidenced below in "### Code Diff Evidence" at commit `0a4a13aa`; artifact-lint exits 0; and the state-transition guard exits 0 at `done`. The single unrelated `proof_of_life.spec.ts` lane failure was root-caused as a current-main baseline, out-of-boundary parent-spec harness failure (not a BUG-002 regression) and routed to the parent.

### phase-audit

Final governance audit: the state-transition guard (`bash .github/bubbles/scripts/state-transition-guard.sh <bugdir>`) exits 0 — the 26 prior findings (G057 scenario-manifest, G022 pipeline phases, G053 Code Diff Evidence, G027 phase-scope coherence, G040 deferral language, G068 DoD-Gherkin fidelity, G093 delivery delta, Check-5, Check-8A, Check-8D) are all cleared with the evidence above; `artifact-lint.sh <bugdir>` exits 0. The Change Boundary was respected: `git show 0a4a13aa` changed only `web/pwa/tests/**` + this bug packet — zero excluded file families.

### Code Diff Evidence

The card-rewards login-session-reuse fix landed at commit `0a4a13aa` (`fix(077): e2e-ui login session reuse to avoid /v1/web/login 429 (BUG-002)`). Changed delivery files (all test-family):

- `web/pwa/tests/_support/cardrewards.ts`
- `web/pwa/tests/cardrewards_wallet.spec.ts`
- `web/pwa/tests/cardrewards_categories.spec.ts`
- `web/pwa/tests/cardrewards_offers_selections.spec.ts`
- `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts`
- `tests/unit/web/bug_077_002_login_session_reuse_test.sh`

The core helper change (`git show 0a4a13aa -- web/pwa/tests/_support/cardrewards.ts`):

```diff
+import { expect, type BrowserContext, type Page } from "@playwright/test";
+type StoredCookie = Awaited<ReturnType<BrowserContext["cookies"]>>[number];
+let cachedAuthCookie: StoredCookie | null = null;
 export async function login(page: Page, next = "/cards"): Promise<void> {
+  if (cachedAuthCookie) {
+    await page.context().addCookies([cachedAuthCookie]);
+    return;
+  }
   const resp = await page.request.post("/v1/web/login", {
     headers: { "Content-Type": "application/x-www-form-urlencoded" },
   });
   expect([200, 302, 303], `/v1/web/login must accept the dev token; got ${resp.status()}`).toContain(resp.status());
+  const auth = (await page.context().cookies()).find((c) => c.name === "auth_token");
+  if (!auth) { throw new Error("/v1/web/login succeeded but no auth_token cookie was set; cannot establish a reusable session…"); }
+  cachedAuthCookie = auth;
 }
```

## Discovered Unrelated Failure (routed, out-of-boundary)

The this-session e2e-ui lane produced exactly one failure that is NOT part of BUG-002's surface:

- `web/pwa/tests/proof_of_life.spec.ts:28` asserts the unauthenticated served `/` route has title `Smackerel`, but the app rendered `Sign in — Smackerel`. This is an app-routing/title expectation, independent of the card-rewards login-session-reuse test helper (proof_of_life never calls `login()`).
- It is pre-existing on the current-main baseline: the tested tree was clean HEAD `d83a4fef`, and the fix commit `0a4a13aa` changed only `web/pwa/tests/cardrewards*` + this bug packet (`git show 0a4a13aa --name-only` shows no app/auth/proof_of_life files). `proof_of_life.spec.ts` was last edited in June 2026, unrelated to BUG-002.
- It belongs to the parent feature spec-077 harness surface, outside this bug's Change Boundary. Per the parallel-isolation contract it is left untouched and surfaced to the parent (`bubbles.iterate`) for independent disposition. It is NOT a BUG-002 regression and does not gate this fix.

## Completion Statement

BUG-002 is certified **done** this session. The worker-scoped `auth_token` cookie cache in the shared `_support/cardrewards.ts` `login()` collapses the card-rewards suite from ~40 per-test `/v1/web/login` POSTs to at most one real login per Playwright worker, keeping it far under the spec-070 `httprate.LimitByIP(20, 1*time.Minute)` limiter. The full certification pipeline ran in-session (implement, test, regression, simplify, stabilize, security, validate, audit; parent-expanded by `bubbles.iterate`): the authoritative live e2e-ui regression is GREEN (42 passed, every card-rewards login test green, zero 429); the node adversarial regression is GREEN (2/2) with a proven RED→GREEN; the spec-070 limiter and `auth_login.spec.ts` real-flow tests are byte-untouched; and the state-transition guard exits 0 at `done`. The lone unrelated `proof_of_life.spec.ts` lane failure is a current-main baseline, out-of-boundary parent-spec issue, correctly attributed and routed to the parent.

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

## Certification (this session)

- The code fix landed at `0a4a13aa` (`50c71583` → `0a4a13aa`) and this session ran the full bugfix-fastlane pipeline in-session (parent-expanded by `bubbles.iterate`), with the live e2e-ui card-rewards regression GREEN (zero 429) as the authoritative core evidence.
- The state-transition guard and `artifact-lint` both exit 0 for this bug packet; `state.json` is promoted to `status: done` after the certification planning-truth commit (G088 ordering).
