# Scopes: BUG-002 E2E UI Card-Rewards Login Rate-Limit (Session Reuse)

Single-scope test-infrastructure fix.

## Scope 1: Worker-Scoped Login Session Reuse

**Status:** In Progress
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
| TP-077-BUG-002-03 | SCN-077-BUG-002-01 | e2e-ui | `web/pwa/tests/cardrewards_*.spec.ts` (live lane) | card-rewards suite authenticates without 429 | `./smackerel.sh test e2e-ui` (CI — parent-owned) |

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
- [ ] Authoritative: CI "E2E UI" lane goes green (card-rewards 429s cleared) — **parent-owned** (commit + push + CI), pending

> The final DoD item is intentionally unchecked: the full e2e-ui harness needs the live stack + Ollama and OOMs the dev host, and commit/push/CI are reserved to the parent orchestrator. The scope status stays **In Progress** (not Done) until the parent's CI "E2E UI" run confirms green.
