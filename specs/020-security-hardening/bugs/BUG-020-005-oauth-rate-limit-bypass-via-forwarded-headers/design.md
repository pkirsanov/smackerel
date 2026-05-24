# Design: BUG-020-005 — Replace global `middleware.RealIP` with an SST-config-gated trusted-proxy middleware

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Current Truth (2026-05-23 Round 15 fact-finding)

- `internal/api/router.go:24` calls `r.Use(middleware.RealIP)` globally and unconditionally. There is no SST switch, no trusted-proxy gate, no CIDR allowlist.
- `internal/api/router.go:209` rate-limits `/auth/{provider}/start` and `/auth/{provider}/callback` via `httprate.LimitByIP(10, 1*time.Minute)`.
- `internal/api/router.go:223` rate-limits `/v1/web/login` and `/v1/web/logout` via `httprate.LimitByIP(20, 1*time.Minute)` (spec 044 surface — out of fix scope but benefits from the same patch).
- `httprate.LimitByIP` keys on `r.RemoteAddr`. With `middleware.RealIP` upstream, that value is whatever the FIRST entry in `X-Forwarded-For` (or `True-Client-IP` or `X-Real-IP`) says it is.
- The deploy compose binds `smackerel-core` to `${HOST_BIND_ADDRESS}:${CORE_HOST_PORT}`. No documented or enforced trusted-proxy contract; no test in `internal/api/` exercises the X-Forwarded-For path against the rate limiter.
- Independent in-tree test of the OAuth rate-limit control: `TestOAuthStart_RateLimited` (router_test.go:358) uses `req.RemoteAddr = "192.168.1.100:12345"` and never sets forwarded headers. Same for `TestOAuthCallback_RateLimited` and `TestOAuthStart_RateLimitPerIP`. The middleware composition was never adversarially tested with header spoofing.
- `chi.middleware.RealIP` source (`go-chi/chi/v5/middleware/realip.go`) is the unconditional version. It explicitly states it should ONLY be used when behind a trusted proxy that strips client-supplied values.

## Root Cause

`chi.middleware.RealIP` was added to the router for the legitimate use case of "see the real client IP in logs and rate-limit keys when behind a reverse proxy". But the middleware is unconditional: it trusts EVERY caller's forwarded headers. In Smackerel today there is no enforced upstream proxy contract, no SST `trusted_proxies` config, and at least one binding (dev `docker-compose.yml`) directly exposes the listener to processes that can send any headers they like. Composing `middleware.RealIP` with `httprate.LimitByIP` turns the rate limiter from per-client-IP into per-client-header-of-the-attacker's-choosing — i.e., not a rate limiter at all.

## Fix Approach

Introduce a single, narrow middleware — `trustedProxyRealIPMiddleware` — that ONLY honours forwarded headers when the actual TCP peer's IP is in the SST-configured `runtime.trusted_proxies` CIDR allowlist. When the allowlist is empty (the default for both dev and deploy bundles produced from `config/smackerel.yaml`), forwarded headers are IGNORED and `r.RemoteAddr` keeps its raw TCP-level value. Deploy adapters that DO front the stack with a trusted proxy add the proxy's IP/CIDR to their operator-private overlay; everything else stays correct by default.

The patch is the minimum that restores the spec 020 R-004 contract under adversarial inputs and preserves the legitimate "real client IP behind a known proxy" use case for future deploy adapters.

### Design Decisions

| ID | Decision | Rationale |
|---|---|---|
| DD-1 | New SST field `runtime.trusted_proxies` (YAML list of CIDR strings) emitted as `RUNTIME_TRUSTED_PROXIES` (comma-separated). | Mirrors the existing `cors.allowed_origins` pattern in `config/smackerel.yaml` and `scripts/commands/config.sh`. CSV env-var transport keeps `config/generated/*.env` shell-safe and avoids JSON-parsing inside the Go config loader. |
| DD-2 | Default value is `[]` (empty CSV → no trusted proxies). | Zero-defaults policy (Gate G028 / NO-DEFAULTS SST). Empty list = "ignore all forwarded headers" = secure by default. Operators behind a real proxy opt-in explicitly. |
| DD-3 | The middleware lives in a new file `internal/api/realip.go`, NOT in `health.go` or `router.go`. | Keeps the trust-boundary code isolated and reviewable. `realip.go` owns CIDR parsing once at construction time, exposes one `http.Handler` middleware, and one parse-error sentinel. |
| DD-4 | CIDR allowlist parsing happens ONCE at middleware construction (build time / wiring time), not per-request. Parse failures fail loud at wiring time. | Per-request CIDR parsing is wasteful and would silently swallow operator typos. Failing loud at wiring matches Gate G028. |
| DD-5 | The middleware honours the same header order as chi.middleware.RealIP: `True-Client-IP` → `X-Real-IP` → `X-Forwarded-For` (first comma-separated entry). | Preserves the legitimate "behind trusted proxy" UX without surprising operators who have prior chi knowledge. |
| DD-6 | The middleware also rewrites `r.RemoteAddr` (not just adds a new field) so DOWNSTREAM code (httprate, slog labels, future per-IP gates) does NOT need to be aware of trusted-proxy plumbing. | Single overwrite point. Avoids fan-out edits across every consumer of `r.RemoteAddr`. |
| DD-7 | When the TCP peer's IP cannot be parsed (malformed `r.RemoteAddr`), the middleware FAILS CLOSED: forwarded headers are ignored, raw `r.RemoteAddr` is preserved. | Conservative: never escalates trust on parse failure. The slog.Warn line is throttle-able by upstream logging filters. |
| DD-8 | Adversarial regression test asserts BOTH directions: (a) untrusted peer → headers ignored (rate limit triggers), (b) trusted peer → headers honoured (rate limit triggers per-spoofed-IP for trusted callers). The fidelity proof is: temporarily revert the gate so the middleware unconditionally honours headers; the new test (a) must FAIL. | Catches both regression of the fix and regression of the legitimate proxy-trust use case. |
| DD-9 | The new field on `internal/api/Dependencies` is `TrustedProxies []string` (CIDRs). The middleware constructor consumes that slice and returns the configured `func(http.Handler) http.Handler`. | Matches the existing `CORSAllowedOrigins []string` pattern on `Dependencies` (health.go:236). Keeps wiring symmetric. |

### Touch List

| Path | Change |
|---|---|
| `config/smackerel.yaml` | Add `runtime.trusted_proxies: []` with an SST comment block explaining the semantics. |
| `scripts/commands/config.sh` | Read `runtime.trusted_proxies` (JSON via `yaml_get_json`), convert to CSV, emit as `RUNTIME_TRUSTED_PROXIES=…` in the generated env template. |
| `internal/config/config.go` | Add `RuntimeTrustedProxies []string` to `Config`, parse `RUNTIME_TRUSTED_PROXIES` CSV at load time. |
| `internal/api/health.go` | Add `TrustedProxies []string` slot to `Dependencies` struct (additive). |
| `internal/api/realip.go` (NEW) | The middleware. Parses CIDRs at construction time, exposes `Dependencies.trustedProxyRealIPMiddleware() func(http.Handler) http.Handler`. Returns a no-op identity middleware when the allowlist is empty. |
| `internal/api/router.go` | Replace `r.Use(middleware.RealIP)` with `r.Use(deps.trustedProxyRealIPMiddleware())`. Drop the unused `chi/middleware` import if no other usage remains. |
| `internal/api/router_test.go` | Add the 4 adversarial regression tests defined in scopes.md. |
| `cmd/core/wiring.go` | Wire `cfg.RuntimeTrustedProxies → deps.TrustedProxies`. |
| `config/generated/dev.env` and `config/generated/test.env` | Regenerated outputs containing `RUNTIME_TRUSTED_PROXIES=`. |
| `specs/020-security-hardening/state.json` | Append a R30 history entry naming this bug + finding. |
| `specs/020-security-hardening/report.md` | Append a "Security R30 — OAuth rate-limit header-spoof bypass" section. |

### Out of Scope

- Per-user / per-account rate limiting (a different control surface).
- Removing or restructuring the OAuth rate-limit budget (10 req/min/IP is unchanged).
- Web login (`/v1/web/login`) hardening beyond what the trusted-proxy gate already gives it transitively — spec 044's separate audit owns the per-user-bearer policy.
- ML sidecar `/metrics` and core `/metrics` endpoints — unauthenticated by design (Prometheus scrape pattern, documented in router.go:45 comment).
- Caddy / reverse-proxy configuration in the deploy-adapter overlay — out of this repo by deployment-target-adapter contract.
- mTLS for inter-service traffic (already deferred as IP-001 in spec 020 improvement proposals).

### Risk Assessment

- **Risk:** A deploy adapter that previously relied (silently and incorrectly) on `middleware.RealIP` for per-client rate limiting through a trusted proxy will, after this change, see all requests collapse to the proxy's IP and trip the rate limit globally. — **Mitigation:** That adapter simply adds the proxy's IP/CIDR to `runtime.trusted_proxies` in its operator-private overlay; the middleware then honours `X-Forwarded-For` from that proxy specifically. The repo's committed bundles (dev/test) ship with `[]` so the change is no-op for them. The adapter behaviour change is explicit-opt-in, fail-loud, and documented in the SST comment in `config/smackerel.yaml`.
- **Risk:** A test that depended on the previous unconditional behaviour breaks. — **Mitigation:** Grep `internal/api/` for `X-Forwarded-For` / `X-Real-IP` / `True-Client-IP` confirms zero pre-existing tests set those headers. The only new tests are the ones added under DD-8.
- **Risk:** A future contributor re-adds `middleware.RealIP` to the router. — **Mitigation:** The adversarial regression test under DD-8 case (a) would immediately fail because `middleware.RealIP` would re-honour the spoofed header and the rate limit would not trigger.
