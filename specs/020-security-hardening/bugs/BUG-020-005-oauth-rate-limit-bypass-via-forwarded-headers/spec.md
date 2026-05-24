# Bug: BUG-020-005 — OAuth rate limiter bypassable via X-Forwarded-For header spoofing (chi.middleware.RealIP unconditionally trusts client headers)

## Classification

- **Type:** Runtime defect (network-edge trust model)
- **CWE:** CWE-290 (Authentication Bypass by Spoofing) / CWE-345 (Insufficient Verification of Data Authenticity)
- **Severity:** HIGH — defeats SCN-020-011 (`OAuth start endpoint is rate-limited`) on every deployment topology where any untrusted process can make HTTP requests to `smackerel-core`. In dev mode (single-host Docker) this is every container and every local user; in deploy-mode behind Caddy this is every device that reaches the tailnet IP.
- **Parent Spec:** 020 — Security Hardening — Docker Binding, Auth Enforcement, Crypto Hygiene
- **Workflow Mode:** bugfix-fastlane (parent-expanded child workflow `security-to-doc`)
- **Status:** Fixed
- **Parent Sweep:** `sweep-2026-05-23-r30` round 15 (trigger: security, target: 020-security-hardening)

## Problem Statement

Stochastic-quality-sweep round 15 of `sweep-2026-05-23-r30` ran a security probe (parent-expanded child workflow mode `security-to-doc`) against spec 020 R-004 (`Rate-Limit OAuth Start Endpoint`). The implemented control composes two off-the-shelf middlewares in `internal/api/router.go`:

```go
// router.go:24
r.Use(middleware.RealIP)
…
// router.go:209
r.Group(func(r chi.Router) {
    r.Use(httprate.LimitByIP(10, 1*time.Minute))
    r.Get("/auth/{provider}/start", deps.OAuthHandler.StartHandler)
    r.Get("/auth/{provider}/callback", deps.OAuthHandler.CallbackHandler)
})
```

`go-chi/chi/v5/middleware.RealIP` is applied **globally and unconditionally**. It reads `True-Client-IP`, `X-Real-IP`, and `X-Forwarded-For` headers in that order and overwrites `r.RemoteAddr` with the first value present. Then `httprate.LimitByIP` keys its token bucket on `r.RemoteAddr`. The combined effect: any client that can send HTTP headers controls the rate-limiter key. Rotating `X-Forwarded-For` per request produces an unbounded number of distinct keys → the per-IP rate limit is bypassed completely.

### F-SEC-R30-001 — OAuth rate limiter bypass via X-Forwarded-For (HIGH)

**Adversarial probe** (isolated test outside the repo, captured under `/tmp/sec-r15-probe/probe_test.go`, mirrors the router.go middleware stack one-for-one):

```text
$ cd /tmp/sec-r15-probe && go test -v -run . ./...
=== RUN   TestBaseline_SameRemoteAddr_GetsRateLimited
--- PASS: TestBaseline_SameRemoteAddr_GetsRateLimited (0.00s)
=== RUN   TestAdversarial_XForwardedFor_BypassesRateLimit
    probe_test.go:79: CONFIRMED bypass: 50 requests, zero 429. Sample statuses: [200 200 200 200 200]
--- PASS: TestAdversarial_XForwardedFor_BypassesRateLimit (0.00s)
=== RUN   TestAdversarial_XRealIP_BypassesRateLimit
--- PASS: TestAdversarial_XRealIP_BypassesRateLimit (0.00s)
PASS
```

50 consecutive requests from one real TCP RemoteAddr (`192.168.1.99:44444`) with rotating `X-Forwarded-For: 10.x.y.z` headers all returned 200 — zero 429. Baseline (same RemoteAddr, no header) gets a 429 within 12 requests as expected. `X-Real-IP` exhibits the same bypass.

**Concrete impact on spec 020 surfaces:**

1. **R-004 OAuth start rate limit (10 req/min/IP)** — bypassable. An attacker can issue unbounded `/auth/{provider}/start` requests, which exhausts CSRF state storage, makes the bot count toward OAuth-provider rate limits (potentially locking the legitimate user out of their own provider), and floods slog with junk.
2. **OAuth callback rate limit (10 req/min/IP, SEC-SWEEP-001 belt-and-brace)** — bypassable on the same `httprate.LimitByIP` mechanism.
3. **Auth-failure log forgery** — `webAuthMiddleware` and `bearerAuthMiddleware` log `slog.Warn("…", "remote_addr", r.RemoteAddr)`. Because `r.RemoteAddr` is now header-controlled, an attacker chooses what gets logged as the source IP for their failed auth attempts → logs become unreliable for incident response and IP-based abuse hunting.

**Why the existing tests didn't catch it:** `TestOAuthStart_RateLimited`, `TestOAuthCallback_RateLimited`, and `TestOAuthStart_RateLimitPerIP` all set `req.RemoteAddr` directly and never set `X-Forwarded-For` / `X-Real-IP`. The middleware composition was never exercised with header spoofing.

**Why `middleware.RealIP` is wrong as-applied:** it is documented as safe only when the Go server sits behind a trusted reverse proxy that strips and rewrites these headers. In Smackerel today there is no committed proxy contract that strips client-supplied `X-Forwarded-For`, no SST config for `trusted_proxies`, and the deploy compose binds `smackerel-core` directly to a host port (`${HOST_BIND_ADDRESS}:${CORE_HOST_PORT}`). In dev (`docker-compose.yml`) it binds to `127.0.0.1` reachable from every container on the docker network and every local process — all of which can send arbitrary headers.

## Acceptance Criteria

- [x] `internal/api/router.go` no longer unconditionally trusts `X-Forwarded-For` / `X-Real-IP` / `True-Client-IP` from any caller. The chi/middleware.RealIP call is replaced by an SST-config-gated middleware that only honours forwarded headers when the connecting TCP peer is in the configured `runtime.trusted_proxies` CIDR allowlist (F-SEC-R30-001).
- [x] A new SST config field `runtime.trusted_proxies` (list of CIDRs) is added to `config/smackerel.yaml`, emitted as the `RUNTIME_TRUSTED_PROXIES` env var by `scripts/commands/config.sh` for both dev and test environments, parsed in `internal/config/config.go`, propagated through `cmd/core/wiring.go` into `internal/api/Dependencies.TrustedProxies`, and read by the new middleware. Default is `[]` (no trusted proxies → forwarded headers always ignored), preserving zero-defaults policy (Gate G028).
- [x] Adversarial regression tests are added in `internal/api/router_test.go` that prove (a) rotating `X-Forwarded-For` cannot bypass the OAuth rate limit when trusted_proxies is empty, (b) the same headers ARE honoured when the request peer is inside a configured trusted CIDR, (c) the same headers are ignored when the request peer is OUTSIDE every configured trusted CIDR. Reverting the new middleware demonstrably breaks at least one of these tests.
- [x] All pre-existing `TestOAuthStart_*`, `TestOAuthCallback_*`, `TestBearerAuth_*`, `TestSecurityHeaders_*`, and `internal/config/*` tests continue to pass.
- [x] `go vet ./internal/api/... ./internal/config/...` and `gofmt -l` are clean for every file touched.
- [x] Parent `specs/020-security-hardening/state.json` and `report.md` reference this bug under a "Security R30 — OAuth rate-limit header-spoof bypass" history entry.

## Boundary

- No change to the OAuth handler signatures (`OAuthHandler.StartHandler`, `OAuthHandler.CallbackHandler`).
- No change to the `httprate.LimitByIP(10, 1*time.Minute)` budget or to its placement.
- No change to the `webAuthMiddleware` / `bearerAuthMiddleware` token-comparison contract (NFR-AUTH-008 constant-time preserved).
- No change to `deploy/compose.deploy.yml`. The new SST field is empty in both the dev and deploy bundles produced from `config/smackerel.yaml` — deploy adapters that DO front the stack with a trusted proxy will opt in by writing a non-empty `runtime.trusted_proxies` in the operator-private overlay.
- Spec 055 work-in-progress files are NOT staged in the close-out commit (per sweep policy).
- No change to `internal/api/health.go` Dependencies fields other than the new `TrustedProxies []string` slot (additive).
