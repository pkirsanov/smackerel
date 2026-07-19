# Bug: BUG-002 E2E UI Card-Rewards Login Rate-Limit (Session Reuse)

**Status:** Fixed (certified done this session — see report.md)
**Severity:** High
**Found By:** CI "E2E UI" GitHub Actions lane (run 27877380385, commit 50c71583)
**Date:** 2026-06-20
**Root Cause:** Per-test `/v1/web/login` POSTs across the card-rewards e2e-ui specs exceed the production web-login rate limit on the shared CI runner IP.

---

## Problem Statement

The "E2E UI" lane (`./smackerel.sh test e2e-ui` → `scripts/runtime/web-e2e-ui.sh` → Playwright under `web/pwa/`) brings up a healthy disposable stack and runs 42 tests on 2 workers. 33 pass; **9 fail, ALL in `web/pwa/tests/cardrewards_*.spec.ts`**, every one with the identical error:

```
Error: /v1/web/login must accept the dev token; got 429
expect(received).toContain(expected)   Expected value in [200,302,303]; Received: 429
```

`429` is HTTP Too Many Requests. The web-login limiter is `httprate.LimitByIP(20, 1*time.Minute)`, hardcoded in `internal/api/router.go` for the `/v1/web/login` + `/v1/web/register` group and **owned by spec 070** (`web-username-password-login`). It is a deliberate credential-stuffing defense and is itself under test in `internal/api/web_login_ratelimit_test.go`, so it MUST NOT be changed.

Each card-rewards spec performs a real `/v1/web/login` POST in a per-test `beforeEach`. Three specs defined their OWN local `login()` (`wallet`, `categories`, `offers_selections`); the other seven call the shared `_support/cardrewards.ts` `login()`. With 42 tests on 2 workers sharing ONE runner IP, cumulative logins blow past 20/IP/min, so the later-numbered card-rewards tests (indices ~23–42) receive 429.

The disposable stack runs in dev-token mode (`AuthConfig.Enabled=false`): every login exchanges the SAME shared `SMACKEREL_AUTH_TOKEN` for an equivalent `auth_token` session cookie — so a single authenticated session is valid for the whole worker, and re-logging-in per test is pure waste that only serves to trip the limiter.

## Reproduction

- **Authoritative:** CI "E2E UI" run 27877380385 (commit 50c71583) — 9/42 failures, all `cardrewards_*.spec.ts`, all `got 429`.
- **Local static evidence (before fix):** four real `/v1/web/login` POST sites run per-test, and ten card-rewards specs invoke `login()` in a `beforeEach` (full capture in [report.md](report.md)).

> The full e2e-ui harness requires the live stack + Ollama and OOMs the dev host; it was NOT run locally. CI is the authoritative reproduction + confirmation surface.

## Expected Behavior

The card-rewards e2e-ui suite MUST authenticate without tripping the production web-login rate limit, WITHOUT weakening that limiter. Concretely:

1. The number of real `/v1/web/login` POSTs is reduced from ~40 to at most one-per-worker (~2 total), keeping the suite far below 20/IP/min.
2. The production limiter (`httprate.LimitByIP(20, 1*time.Minute)`) and `internal/api/web_login_ratelimit_test.go` remain byte-for-byte unchanged.
3. The real login FLOW tests (`auth_login.spec.ts` TP-077-03-01/02/03/04) keep logging in for real.
4. No `trusted_proxies` / `X-Forwarded-For` spoofing — that would defeat the per-IP control. Session reuse only.

## Fix Plan

Test-side session reuse only — **Approach A (worker-scoped cookie cache in the shared helper)**:

1. Cache the `auth_token` cookie at Playwright worker scope inside `_support/cardrewards.ts` `login()`. The first call per worker POSTs once and captures the cookie; later calls replay it via `BrowserContext.addCookies` with NO POST.
2. Route the three local-`login()` specs (`wallet`, `categories`, `offers_selections`) through the shared cached helper; delete their per-test login POSTs and now-dead helpers. The seven specs already on the shared helper benefit automatically.
3. Add an adversarial node regression (worker-cache behavior + structural "no per-test login POST" guard).

## Related Artifacts

- Parent spec: [spec.md](../../spec.md)
- Affected files: `web/pwa/tests/_support/cardrewards.ts`, `web/pwa/tests/cardrewards_{wallet,categories,offers_selections}.spec.ts`
- Regression: `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` + `tests/unit/web/bug_077_002_login_session_reuse_test.sh`
- MUST-NOT-TOUCH: `internal/api/router.go` (spec 070 limiter), `internal/api/web_login_ratelimit_test.go`, `auth_login.spec.ts` real-flow tests
- Test anchors: SCN-077-BUG-002-01, SCN-077-BUG-002-02
