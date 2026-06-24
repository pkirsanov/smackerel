# Scopes: BUG-002 E2E UI Card-Rewards Login Rate-Limit (Session Reuse)

Single-scope test-infrastructure fix.

## Scope 1: Worker-Scoped Login Session Reuse

**Status:** In Progress (implementation complete and CI-verified GREEN on commit `0a4a13aa` — CI "E2E UI" run `27878481805`, `e2e-ui` job success; full done-certification pipeline deferred per the state-transition guard — see report.md Completion Statement)
**Priority:** P0
**Scope-Kind:** bugfix

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

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command |
|-----|----------|----------|---------------|-------------------|---------|
| TP-077-BUG-002-01 | SCN-077-BUG-002-01 | unit | `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` | `SCN-077-BUG-002-01 — login POSTs once per worker, then reuses the cached session` | `./smackerel.sh test unit` (driver: `tests/unit/web/bug_077_002_login_session_reuse_test.sh`) |
| TP-077-BUG-002-02 | SCN-077-BUG-002-02 | unit | `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` | `SCN-077-BUG-002-02 — no cardrewards spec reintroduces a per-test /v1/web/login POST` | `./smackerel.sh test unit` |
| TP-077-BUG-002-03 | SCN-077-BUG-002-01 | e2e-ui | `web/pwa/tests/cardrewards_wallet.spec.ts` (representative of the live card-rewards suite) | card-rewards suite authenticates without 429 | `./smackerel.sh test e2e-ui` (CI — parent-owned) |

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
- [x] Authoritative: CI "E2E UI" lane goes green (card-rewards 429s cleared) → Evidence: report.md "Test Evidence" §7 — CI "E2E UI" run `27878481805` conclusion **success**, `e2e-ui` job **success** (5m42s) on commit `0a4a13aa`; the 9 formerly-429 `cardrewards_*.spec.ts` tests now pass

> The final DoD item is now satisfied: the CI "E2E UI" lane confirmed GREEN on commit `0a4a13aa` (run `27878481805`, `e2e-ui` job success), clearing the card-rewards 429 failures. The fix's code landed via the parent commit (`50c71583` → HEAD `0a4a13aa`); the full e2e-ui harness was run on the live CI runner (not the OOM-prone dev host). The scope status is held at **In Progress** (not Done) because the bug-packet's full done-certification pipeline (regression/simplify/stabilize/security/validate/audit phases + scenario-specific E2E regression planning) was not executed; the real state-transition guard blocks a `done` promotion. Done-certification is deferred to the parent orchestrator.
