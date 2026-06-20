# User Validation: BUG-002 E2E UI Card-Rewards Login Rate-Limit (Session Reuse)

## Validation Criteria

1. **Rate limit cleared:** the card-rewards e2e-ui specs authenticate without any `/v1/web/login ... got 429` failures.
2. **Production limiter intact:** `httprate.LimitByIP(20, 1*time.Minute)` in `internal/api/router.go` and `internal/api/web_login_ratelimit_test.go` are byte-for-byte unchanged and stay green.
3. **Real-flow login intact:** `auth_login.spec.ts` TP-077-03-01/02/03/04 still log in for real.
4. **No control bypass:** no `trusted_proxies` / `X-Forwarded-For` spoofing — session reuse only.
5. **Durability:** a future card-rewards spec that reintroduces a per-test login POST is caught by the unit lane.

## Validation Steps

1. Run `./smackerel.sh test unit` → the BUG-002 regression (SCN-077-BUG-002-01/02) passes. ✅ locally captured (see [report.md](report.md)).
2. `cd web/pwa && npx playwright test --list` → 42 tests load, exit 0. ✅ locally captured.
3. `grep -rnE 'request\.post\("/v1/web/login"' web/pwa/tests` → only `_support/cardrewards.ts` + `auth_login.spec.ts`. ✅ locally captured.
4. **Authoritative:** the next CI "E2E UI" run after the parent commits + pushes shows the card-rewards specs green. ⏳ **parent-owned** (the full harness OOMs the dev host; not run locally).

## Checklist

- [x] Unit regression passes (2/2) and is proven adversarial (fails when the cache is disabled)
- [x] Suite loads via `playwright test --list` (42 tests, exit 0)
- [x] Static grep confirms per-test login POSTs removed
- [x] Zero changes under `internal/` (production limiter untouched) — verified via `git status`
- [ ] CI "E2E UI" lane green — **pending, parent-owned** (authoritative end-to-end confirmation)
