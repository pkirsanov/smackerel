# Scopes: BUG-020-005 — OAuth rate limit bypass via X-Forwarded-For header spoofing

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Gate forwarded-IP header trust behind an SST CIDR allowlist and add adversarial regression

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-SEC-FIX-005-001 OAuth start rate limit cannot be bypassed via X-Forwarded-For when no trusted proxies are configured
  Given Dependencies.TrustedProxies is empty (the SST default for dev and test bundles)
  And a single TCP peer at 192.168.1.99:44444 sends 50 GET /auth/google/start requests
  And each of those requests carries a distinct X-Forwarded-For value (10.0.0.1, 10.0.0.2, …, 10.0.0.50)
  When the requests are processed by the chi router with the trusted-proxy-gated RealIP middleware in place
  Then within the first 11 requests at least one response has status 429 Too Many Requests
  And rotating the X-Forwarded-For header does NOT extend the per-IP budget
  And r.RemoteAddr for every request equals "192.168.1.99:44444" downstream of the middleware

Scenario: SCN-SEC-FIX-005-002 Forwarded headers ARE honoured when the request peer is in the configured trusted-proxies CIDR allowlist
  Given Dependencies.TrustedProxies = ["127.0.0.0/8"]
  And a TCP peer at 127.0.0.1:55555 (the upstream Caddy) sends 25 GET /auth/google/start requests
  And the first 15 requests all carry X-Forwarded-For: 203.0.113.42 (one real client)
  And the next 10 requests carry X-Forwarded-For: 203.0.113.43 (a different real client)
  When the requests are processed by the chi router with the trusted-proxy-gated RealIP middleware in place
  Then the first real client trips the per-IP rate limit at 429 within its 15-request burst
  And the second real client is rate-limited independently of the first (the per-IP bucket is per real client, not per proxy peer)
  And r.RemoteAddr downstream of the middleware reflects the forwarded value (203.0.113.42 / 203.0.113.43)

Scenario: SCN-SEC-FIX-005-003 Forwarded headers from an UNTRUSTED peer are ignored even when trusted_proxies is non-empty
  Given Dependencies.TrustedProxies = ["10.42.0.0/16"]
  And a TCP peer at 192.168.1.99:44444 (NOT inside 10.42.0.0/16) sends 50 GET /auth/google/start requests
  And each request carries a distinct X-Forwarded-For value
  When the requests are processed by the chi router with the trusted-proxy-gated RealIP middleware in place
  Then within the first 11 requests at least one response has status 429
  And r.RemoteAddr for every request equals "192.168.1.99:44444" (forwarded value rejected)
```

### Implementation Plan

1. Add `runtime.trusted_proxies: []` to `config/smackerel.yaml` with an SST comment block explaining the empty-default semantics and the operator-overlay opt-in path. (DD-1, DD-2)
2. In `scripts/commands/config.sh`:
   - Mirror the `CORS_ALLOWED_ORIGINS` pattern: read `runtime.trusted_proxies` via `yaml_get_json`, convert the JSON array to a comma-separated string into `RUNTIME_TRUSTED_PROXIES`.
   - Add `RUNTIME_TRUSTED_PROXIES=${RUNTIME_TRUSTED_PROXIES}` to the generated env template, adjacent to the existing `CORS_ALLOWED_ORIGINS` line.
3. In `internal/config/config.go`:
   - Add `RuntimeTrustedProxies []string` to the `Config` struct, adjacent to `CORSAllowedOrigins`.
   - Add a parse block adjacent to the existing `CORS_ALLOWED_ORIGINS` parse: split `RUNTIME_TRUSTED_PROXIES` by comma, trim, append non-empty entries.
4. In `internal/api/health.go`:
   - Add `TrustedProxies []string` slot to the `Dependencies` struct, with an SST comment.
5. Create `internal/api/realip.go`:
   - Export an unexported method `(deps *Dependencies) trustedProxyRealIPMiddleware() func(http.Handler) http.Handler`.
   - On construction, parse every CIDR in `deps.TrustedProxies` ONCE with `net.ParseCIDR`. Invalid CIDRs are logged at startup as `slog.Error("trusted_proxies CIDR parse failed", …)` and the offending entry is dropped (fail-loud-once at construction time; preserves boot if a single typo slips in, but never silently grants trust).
   - When the parsed CIDR slice is empty, return an identity middleware (no-op pass-through).
   - When non-empty, return a middleware that:
     - Extracts the host portion of `r.RemoteAddr` via `net.SplitHostPort` (falls back to the raw value when no port is present).
     - Parses the peer IP via `net.ParseIP`. If parse fails, the middleware leaves `r.RemoteAddr` untouched (DD-7).
     - If the peer IP is NOT in any trusted CIDR, the middleware leaves `r.RemoteAddr` untouched.
     - If the peer IP IS in a trusted CIDR, the middleware reads (in order) `True-Client-IP`, `X-Real-IP`, then the FIRST comma-separated value of `X-Forwarded-For`. The first non-empty, parseable IP wins; it is written into `r.RemoteAddr` as `<ip>` (no port) so `httprate.LimitByIP` and downstream code see the forwarded client.
6. In `internal/api/router.go`:
   - Replace `r.Use(middleware.RealIP)` with `r.Use(deps.trustedProxyRealIPMiddleware())`.
   - Drop the `chi/middleware` import only if no other usage remains; otherwise leave it (other middleware.* calls are unchanged).
7. In `cmd/core/wiring.go`:
   - Wire `cfg.RuntimeTrustedProxies → deps.TrustedProxies` adjacent to the existing `CORSAllowedOrigins:` assignment.
8. In `internal/api/router_test.go`:
   - Add `TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor` covering SCN-SEC-FIX-005-001 (50 requests, rotating XFF, same RemoteAddr, expect at least one 429 within the first 11).
   - Add `TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer` covering SCN-SEC-FIX-005-002 (peer in 127.0.0.0/8, two distinct XFF values, each rate-limited independently).
   - Add `TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer` covering SCN-SEC-FIX-005-003 (peer NOT in 10.42.0.0/16, rotating XFF, expect at least one 429).
   - Add `TestSecR30_TrustedProxyMiddleware_PreservesRawRemoteAddrWhenAllowlistEmpty` — direct middleware-unit test that asserts `r.RemoteAddr` round-trips unchanged when TrustedProxies is `[]` even with all three forwarded-header variants set.
9. Prove adversarial fidelity: temporarily replace `r.Use(deps.trustedProxyRealIPMiddleware())` with `r.Use(middleware.RealIP)` (the pre-fix code path); re-run the four new R30 tests; confirm SCN-SEC-FIX-005-001 and SCN-SEC-FIX-005-003 FAIL; restore the gated middleware; confirm all four PASS again.
10. Run `./smackerel.sh config generate` to regenerate `config/generated/dev.env` and `config/generated/test.env` with the new `RUNTIME_TRUSTED_PROXIES=` line.
11. Run `./smackerel.sh test unit` and confirm the full Go unit suite is green.
12. Run `go vet ./internal/api/... ./internal/config/...` and `gofmt -l` over every touched Go file — both clean.
13. Append a "Security R30 — OAuth rate-limit header-spoof bypass" section to `specs/020-security-hardening/report.md` and one R30 entry to `specs/020-security-hardening/state.json::executionHistory`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-SEC-FIX-005-01 | `TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor` | unit | `internal/api/router_test.go` | 50 requests from one RemoteAddr with rotating XFF; at least one response within the first 11 has status 429; `r.RemoteAddr` observed downstream equals the raw TCP peer for all 50 | SCN-SEC-FIX-005-001 |
| T-SEC-FIX-005-02 | `TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer` | unit | `internal/api/router_test.go` | Peer in 127.0.0.0/8; two real-client XFF values; each gets its own per-IP rate-limit bucket (first hits 429 inside 15-request burst; second remains independent for its first 10) | SCN-SEC-FIX-005-002 |
| T-SEC-FIX-005-03 | `TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer` | unit | `internal/api/router_test.go` | TrustedProxies = ["10.42.0.0/16"]; peer at 192.168.1.99; rotating XFF; at least one 429 within first 11; downstream `r.RemoteAddr` equals raw TCP peer | SCN-SEC-FIX-005-003 |
| T-SEC-FIX-005-04 | `TestSecR30_TrustedProxyMiddleware_PreservesRawRemoteAddrWhenAllowlistEmpty` | unit | `internal/api/router_test.go` | Direct middleware-unit; TrustedProxies = []; all three forwarded headers set; downstream handler observes unchanged `r.RemoteAddr` | SCN-SEC-FIX-005-001 |
| T-SEC-FIX-005-05 | Full `internal/api` suite | unit | `internal/api/...` | `go test ./internal/api/... -count=1` exit 0; no regression in existing OAuth/web/bearer tests | All three |
| T-SEC-FIX-005-06 | Full `internal/config` suite | unit | `internal/config/...` | `go test ./internal/config/... -count=1` exit 0; new `RUNTIME_TRUSTED_PROXIES` parse path covered transitively | All three |
| T-SEC-FIX-005-07 | Adversarial fidelity proof | manual | (procedure documented in report.md) | Toggling the router back to `middleware.RealIP` causes T-SEC-FIX-005-01 and T-SEC-FIX-005-03 to FAIL; toggling back to the gated middleware returns all 4 R30 tests to PASS | All three |

### Definition of Done

- [x] `Scenario SCN-SEC-FIX-005-001 OAuth start rate limit cannot be bypassed via X-Forwarded-For when no trusted proxies are configured` — gated middleware ignores all forwarded headers; rate limit triggers on raw peer key. **Phase:** implement
  > Evidence: see `report.md` § "Security R30 fix verification" — `TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor` PASSES; bypass-count is 0 of 50 with at least one 429 inside the first 11.
- [x] `Scenario SCN-SEC-FIX-005-002 Forwarded headers ARE honoured when the request peer is in the configured trusted-proxies CIDR allowlist` — peer in 127.0.0.0/8 → XFF honoured per-real-client. **Phase:** implement
  > Evidence: see `report.md` § "Security R30 fix verification" — `TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer` PASSES.
- [x] `Scenario SCN-SEC-FIX-005-003 Forwarded headers from an UNTRUSTED peer are ignored even when trusted_proxies is non-empty` — peer NOT in 10.42.0.0/16 → XFF rejected, raw peer keyed. **Phase:** implement
  > Evidence: see `report.md` § "Security R30 fix verification" — `TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer` PASSES.
- [x] Adversarial regression tests are added that FAIL when the gated middleware is reverted to `middleware.RealIP`. **Phase:** test
  > Evidence: see `report.md` § "Security R30 adversarial fidelity proof".
- [x] `RUNTIME_TRUSTED_PROXIES` env var is emitted from `config/smackerel.yaml` via `scripts/commands/config.sh` for both dev and test bundles, and parsed by `internal/config/config.go` into `Config.RuntimeTrustedProxies`. **Phase:** implement
  > Evidence: see `report.md` § "SST contract evidence".
- [x] `Dependencies.TrustedProxies` is wired in `cmd/core/wiring.go` and consumed by `internal/api/realip.go`. **Phase:** implement
  > Evidence: see `report.md` § "Wiring evidence".
- [x] `./smackerel.sh test unit` is green; `go vet` and `gofmt -l` are clean for every touched file. **Phase:** validate
  > Evidence: see `report.md` § "Test and audit evidence".
- [x] Parent `specs/020-security-hardening/state.json` and `report.md` reference this bug under "Security R30 — OAuth rate-limit header-spoof bypass" history. **Phase:** docs
  > Evidence: see `report.md` § "Parent spec close-out".
