# Scopes: BUG-002 E2E UI Card-Rewards Login Rate-Limit (Session Reuse)

Single-scope test-infrastructure fix.

## Scope 1: Worker-Scoped Login Session Reuse

**Status:** Done
**Priority:** P0
**Depends On:** None
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-BUG-002-01 — login POSTs once per worker, then reuses the cached session
  Given a Playwright worker that has already performed one /v1/web/login POST
  When a later test in the same worker calls the shared login(page)
  Then login() does NOT POST /v1/web/login again
  And it replays the cached auth_token cookie onto the new context via addCookies

Scenario: SCN-077-BUG-002-02 — no cardrewards spec reintroduces a per-test login POST
  Given the card-rewards e2e-ui spec corpus
  When the regression scans every cardrewards_*.spec.ts
  Then none of them contain their own page.request.post("/v1/web/login")
  And all card-rewards login flows route through the shared cached helper
```

### Implementation Plan

1. Add a worker-scoped `auth_token` cookie cache to `_support/cardrewards.ts` `login()`.
2. Remove the local `login()` from `cardrewards_{wallet,categories,offers_selections}.spec.ts` and import the shared cached `login`.
3. Add the adversarial node regression + auto-discovered driver.

### Implementation Files

- `web/pwa/tests/_support/cardrewards.ts`
- `web/pwa/tests/cardrewards_wallet.spec.ts`
- `web/pwa/tests/cardrewards_categories.spec.ts`
- `web/pwa/tests/cardrewards_offers_selections.spec.ts`
- `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts`
- `tests/unit/web/bug_077_002_login_session_reuse_test.sh`

### Change Boundary

Allowed file families:

- `web/pwa/tests/_support/cardrewards.ts` (worker-scoped session cache)
- `web/pwa/tests/cardrewards_{wallet,categories,offers_selections}.spec.ts` (route through the shared cached helper)
- `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` (adversarial regression)
- `tests/unit/web/bug_077_002_login_session_reuse_test.sh` (auto-discovered driver)
- `specs/077-pwa-browser-test-harness/bugs/BUG-002-e2e-ui-login-ratelimit-session-reuse/**` (this bug packet)

Excluded surfaces (MUST NOT be touched):

- `internal/api/router.go` — the spec-070 `httprate.LimitByIP(20, 1*time.Minute)` web-login limiter
- `internal/api/web_login_ratelimit_test.go` — the limiter's own test
- `web/pwa/tests/auth_login.spec.ts` — the real-login-flow tests (must keep logging in for real)
- All Go/Python product/runtime code, the e2e-ui stack composition, `web/pwa/playwright.config.ts`, `scripts/runtime/web-e2e-ui.sh`
- No `trusted_proxies` / `X-Forwarded-For` spoofing (session reuse only)

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command |
|-----|----------|----------|---------------|-------------------|---------|
| TP-077-BUG-002-01 | SCN-077-BUG-002-01 | unit | `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` | `SCN-077-BUG-002-01 — login POSTs once per worker, then reuses the cached session` | `./smackerel.sh test unit` (driver: `tests/unit/web/bug_077_002_login_session_reuse_test.sh`) |
| TP-077-BUG-002-02 | SCN-077-BUG-002-02 | unit | `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` | `SCN-077-BUG-002-02 — no cardrewards spec reintroduces a per-test /v1/web/login POST` | `./smackerel.sh test unit` |
| TP-077-BUG-002-03 | SCN-077-BUG-002-01 | Scenario-specific Regression E2E | `web/pwa/tests/cardrewards_wallet.spec.ts` (representative of the live card-rewards suite) | card-rewards suite authenticates without 429 (live) | `./smackerel.sh test e2e-ui` |
| TP-077-BUG-002-04 | SCN-077-BUG-002-01, SCN-077-BUG-002-02 | Broader Regression E2E | full PWA `web/pwa/tests/**` e2e-ui lane | full card-rewards login/wallet/categories/offers/recommendations/rotating suite stays green under the limiter | `./smackerel.sh test e2e-ui` |

### Definition of Done

- [x] Worker-scoped `auth_token` cookie cache added to shared `_support/cardrewards.ts` `login()` → Evidence: report.md "Fix Implementation" + "Test Evidence" §1 (shared-helper POST at `cardrewards.ts:82`)
- [x] `cardrewards_wallet.spec.ts` routes login through the shared cached helper (local login removed) → Evidence: report.md "Test Evidence" §1 + "Files Changed"
- [x] `cardrewards_categories.spec.ts` routes login through the shared cached helper (local login + dead `AUTH_TOKEN`/`requireAuthToken`/`type Page` removed) → Evidence: report.md "Test Evidence" §1 + "Files Changed"
- [x] `cardrewards_offers_selections.spec.ts` routes login through the shared cached helper (local login removed) → Evidence: report.md "Test Evidence" §1 + "Files Changed"
- [x] Static grep proves per-test `/v1/web/login` POSTs remain ONLY in the shared helper + `auth_login.spec.ts` real-flow → Evidence: report.md "Test Evidence" §1 (after-fix grep)
- [x] Adversarial regression (TP-077-BUG-002-01/02) passes locally (2/2) AND proven to FAIL when the cache is disabled → Evidence: report.md "Test Evidence" §4 + §5
- [x] `npx playwright test --list` loads the full suite (42 tests, exit 0) with no syntax/type errors; the new `_support/*.test.ts` is correctly NOT discovered as a spec → Evidence: report.md "Test Evidence" §2
- [x] Production limiter `httprate.LimitByIP(20, 1*time.Minute)` + `web_login_ratelimit_test.go` untouched (verified by `git status`: zero `internal/` changes) → Evidence: report.md "Test Evidence" §6
- [x] `auth_login.spec.ts` real-login flow tests untouched; no `trusted_proxies` / `X-Forwarded-For` spoofing introduced → Evidence: report.md "Test Evidence" §6 (git status, zero internal/ changes)
- [x] Authoritative live e2e-ui: the card-rewards suite authenticates without 429 → Evidence: report.md#e2e-ui-live-run — this-session `./smackerel.sh test e2e-ui` ran the disposable `smackerel-test-e2e-ui` stack (all containers Healthy) and reported **42 passed**; every card-rewards login/wallet/categories/offers/recommendations/rotating test passed with **zero `got 429`**.
- [x] SCN-077-BUG-002-01: login POSTs once per worker, then reuses the cached session — the second call replays the cached auth_token cookie via addCookies with no extra POST → Evidence: report.md#phase-regression (node driver 2/2) + report.md#e2e-ui-live-run (live card-rewards suite green).
- [x] SCN-077-BUG-002-02: no cardrewards spec reintroduces a per-test /v1/web/login POST; all card-rewards login flows route through the shared cached helper → Evidence: report.md#phase-regression (structural guard green, adversarial signal) + report.md "Test Evidence" §1.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior exist and pass → Evidence: report.md#e2e-ui-live-run (live card-rewards e2e-ui suite green, zero 429 this session) + report.md#phase-regression (node SCN-077-BUG-002-01/02 driver 2/2).
- [x] Broader E2E regression suite passes for the affected surface (card-rewards e2e-ui) → Evidence: report.md#e2e-ui-live-run — every card-rewards test passed in the live run; the full-lane run also produced one UNRELATED, out-of-boundary failure (`web/pwa/tests/proof_of_life.spec.ts`, a parent spec-077 proof-of-life test failing on the current-main baseline, not touched by `0a4a13aa`) which is routed to the parent and is NOT a BUG-002 regression.
- [x] Change Boundary is respected and zero excluded file families were changed → Evidence: report.md#phase-security (`git show 0a4a13aa` touched only `web/pwa/tests/**` + this bug packet; zero `internal/` changes; the spec-070 limiter is intact in current main).
- [x] Build Quality Gate passes: fresh this-session regression green, regression-quality/implementation-reality guards exit 0, artifact lint clean, and the state-transition guard passes at `done` → Evidence: report.md#phase-audit.
