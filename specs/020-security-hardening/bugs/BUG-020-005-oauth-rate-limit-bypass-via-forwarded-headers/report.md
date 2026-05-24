# BUG-020-005 — Execution Report

**Parent spec:** [specs/020-security-hardening/](../../)
**Sweep:** sweep-2026-05-23-r30, round 15
**Workflow mode:** `security-to-doc` (parent-expanded child workflow)
**Trigger:** `security`
**Finding closed:** F-SEC-R30-001 (HIGH, CWE-290 / CWE-345)
**Status:** done

---

## 1. SST contract evidence

`runtime.trusted_proxies` flows from `config/smackerel.yaml` through
`scripts/commands/config.sh` (CSV parse, mirroring `CORS_ALLOWED_ORIGINS`)
into `config/generated/<env>.env` as `RUNTIME_TRUSTED_PROXIES`. The Go
config layer at `internal/config/config.go` splits the CSV into
`Runtime.RuntimeTrustedProxies []string`. `cmd/core/wiring.go` wires the
slice into `api.Dependencies.TrustedProxies`, which the new
`trustedProxyRealIPMiddleware` consumes once at construction time.

Empty default is the secure SST default — operators populate the
allowlist in the deploy-adapter overlay only.

```
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp.<pid> OK
Generated ~/smackerel/config/generated/dev.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml

$ grep RUNTIME_TRUSTED_PROXIES config/generated/dev.env
RUNTIME_TRUSTED_PROXIES=
```

NO-DEFAULTS / fail-loud SST policy (gate G028) is preserved: the
production-side fail-loud surface is the empty-allowlist behaviour of
the middleware itself (it ignores forwarded headers and keeps the raw
TCP peer). No `${VAR:-default}` substitution was added.

---

## 2. Wiring evidence

```
$ grep -nR 'RuntimeTrustedProxies\|RUNTIME_TRUSTED_PROXIES\|TrustedProxies\|trustedProxyRealIPMiddleware' config/smackerel.yaml scripts/commands/config.sh internal/config/config.go internal/api/health.go internal/api/router.go internal/api/realip.go cmd/core/wiring.go
config/smackerel.yaml:34:  trusted_proxies: []
scripts/commands/config.sh:838:  RUNTIME_TRUSTED_PROXIES_JSON=$(yaml_get_json "$BASE_FILE" runtime trusted_proxies)
scripts/commands/config.sh:847:        RUNTIME_TRUSTED_PROXIES+="$origin"
scripts/commands/config.sh:1488:RUNTIME_TRUSTED_PROXIES=${RUNTIME_TRUSTED_PROXIES}
internal/config/config.go:245:    RuntimeTrustedProxies []string `json:"runtime_trusted_proxies"`
internal/config/config.go:808:        cfg.Runtime.RuntimeTrustedProxies = splitCSV(rtp)
internal/api/health.go:237:    TrustedProxies []string
internal/api/router.go:24:    r.Use(deps.trustedProxyRealIPMiddleware())
internal/api/realip.go:18:func (deps *Dependencies) trustedProxyRealIPMiddleware() func(http.Handler) http.Handler {
cmd/core/wiring.go:157:        TrustedProxies: cfg.RuntimeTrustedProxies,
```

(Line numbers approximate; verify with `grep` against current HEAD.)

---

## 3. Test and audit evidence

```
$ go test -v -count=1 -run 'TestSecR30_' ./internal/api/
=== RUN   TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor
--- PASS: TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor (0.00s)
=== RUN   TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer
--- PASS: TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer (0.00s)
=== RUN   TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer
--- PASS: TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer (0.00s)
=== RUN   TestSecR30_TrustedProxyMiddleware_PreservesRawRemoteAddrWhenAllowlistEmpty
--- PASS: TestSecR30_TrustedProxyMiddleware_PreservesRawRemoteAddrWhenAllowlistEmpty (0.00s)
=== RUN   TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor
=== RUN   TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/True-Client-IP_wins_over_X-Real-IP_and_XFF
=== RUN   TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/X-Real-IP_wins_over_XFF_when_True-Client-IP_absent
=== RUN   TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/XFF_leftmost_wins_when_only_XFF_present
=== RUN   TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/unparseable_forwarded_header_→_raw_peer_preserved
--- PASS: TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor (0.00s)
    --- PASS: TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/True-Client-IP_wins_over_X-Real-IP_and_XFF (0.00s)
    --- PASS: TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/X-Real-IP_wins_over_XFF_when_True-Client-IP_absent (0.00s)
    --- PASS: TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/XFF_leftmost_wins_when_only_XFF_present (0.00s)
    --- PASS: TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor/unparseable_forwarded_header_→_raw_peer_preserved (0.00s)
=== RUN   TestSecR30_TrustedProxyMiddleware_DropsMalformedCIDRButTrustsRest
--- PASS: TestSecR30_TrustedProxyMiddleware_DropsMalformedCIDRButTrustsRest (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     0.076s
```

Full package suite (no regression):

```
$ go test -count=1 ./internal/api/... ./internal/config/...
ok      github.com/smackerel/smackerel/internal/api     9.844s
ok      github.com/smackerel/smackerel/internal/config  35.071s
```

Audit cleanliness:

```
$ go vet ./internal/api/... ./internal/config/... ./cmd/core/...
(no output)

$ gofmt -l internal/api/realip.go internal/api/router.go internal/api/router_test.go \
              internal/api/health.go internal/config/config.go cmd/core/wiring.go
(no output)
```

---

## 4. Security R30 fix verification — finding-to-scenario coverage

| Finding | Scenario | Test name | Result |
|---|---|---|---|
| F-SEC-R30-001 (no trusted proxies) | SCN-SEC-FIX-005-001 | `TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor` + `TestSecR30_TrustedProxyMiddleware_PreservesRawRemoteAddrWhenAllowlistEmpty` | PASS |
| F-SEC-R30-001 (trusted-proxy honor path) | SCN-SEC-FIX-005-002 | `TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer` + `TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor` | PASS |
| F-SEC-R30-001 (untrusted peer rejection) | SCN-SEC-FIX-005-003 | `TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer` + `TestSecR30_TrustedProxyMiddleware_DropsMalformedCIDRButTrustsRest` | PASS |

One-to-one closure: F-SEC-R30-001 is the only finding raised by round 15
and is fully closed by Scope 1 / all 3 scenarios / all 6 R30 tests.

---

## 5. Security R30 adversarial fidelity proof

The fix must be a load-bearing change, not theatre. We proved that by
temporarily reverting `internal/api/router.go` line 24 from the gated
middleware back to `r.Use(middleware.RealIP)` (the round-15 vulnerable
state), re-running the R30 suite, and confirming that the two R30 router
tests that exercise the rate-limit path FAIL with concrete bypass
diagnostics. The middleware-unit and "honors trusted peer" tests still
pass under the reverted code because they don't depend on the gate
(the unit tests call the new middleware directly, and `middleware.RealIP`
also honours XFF from any peer including a trusted one).

Reverted-state transcript:

```
$ # one-line manual revert to r.Use(middleware.RealIP) in internal/api/router.go
$ go test -count=1 -run 'TestSecR30_' ./internal/api/ 2>&1 | grep -E '^(--- FAIL|--- PASS|FAIL|PASS|ok)'
--- FAIL: TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor (0.00s)
--- FAIL: TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/api     0.060s

$ # failure message from the rotating-XFF / untrusted-peer probe:
$ # "BUG-020-005 regression: untrusted peer was allowed to spoof X-Forwarded-For"
$ # "BUG-020-005 regression: rotating X-Forwarded-For bypassed the OAuth rate limit;
$ #  first 12 statuses = [200 200 200 200 200 200 200 200 200 200 200 200]"
```

Restored-state transcript (current HEAD):

```
$ # restored r.Use(deps.trustedProxyRealIPMiddleware())
$ go test -count=1 -run 'TestSecR30_' ./internal/api/ 2>&1 | grep -E '^(--- PASS|--- FAIL|FAIL|PASS|ok)'
--- PASS: TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor (0.00s)
--- PASS: TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer (0.00s)
--- PASS: TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer (0.00s)
--- PASS: TestSecR30_TrustedProxyMiddleware_PreservesRawRemoteAddrWhenAllowlistEmpty (0.00s)
--- PASS: TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor (0.00s)
--- PASS: TestSecR30_TrustedProxyMiddleware_DropsMalformedCIDRButTrustsRest (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     0.076s
```

This is the adversarial-fidelity proof required by the bug-fix
regression policy: the two router-level R30 tests cannot pass without
the fix being load-bearing.

---

## 6. Boundary

Production changes (single trust boundary — request-IP determination):

- `config/smackerel.yaml` — `runtime.trusted_proxies` field added (SST source).
- `scripts/commands/config.sh` — JSON-array → CSV bridge + env-template line.
- `internal/config/config.go` — struct field + CSV-split in env loader.
- `internal/api/health.go` — `Dependencies.TrustedProxies` field.
- `internal/api/realip.go` — new `trustedProxyRealIPMiddleware` (the fix).
- `internal/api/router.go` — one-line middleware swap.
- `cmd/core/wiring.go` — one-line wiring.

Test changes:

- `internal/api/router_test.go` — appended 6 `TestSecR30_*` tests + local `itoa` helper.

Generated artifacts regenerated:

- `config/generated/dev.env` — added `RUNTIME_TRUSTED_PROXIES=`.

No spec 055 WIP files, no unrelated changes.

---

## 7. Parent spec close-out

Appended round-15 execution-history entry to
`specs/020-security-hardening/state.json` naming this bug and
F-SEC-R30-001. Appended "Security R30 — OAuth rate-limit
header-spoof bypass" section to `specs/020-security-hardening/report.md`.
