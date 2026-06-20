# Bug Fix Design: BUG-069-001 — wire `PreFacadeChain` into the live assistant HTTP route

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Root Cause Analysis

### Investigation Summary

Independently re-verified the stochastic-quality-sweep R41 finding with real read-only commands (captured verbatim in [report.md](report.md) → "Discovery Evidence"). The verification chain:

1. `cmd/core/wiring_assistant_facade.go:314` calls `svc.assistantHTTPHandler.SetMiddleware(func(next http.Handler) http.Handler { return next })` — an identity pass-through. This is the **only** `SetMiddleware` call in `cmd/core`; `PreFacadeChain` does not appear in the file.
2. `internal/assistant/httpadapter/adapter.go:329` reads `body, err := io.ReadAll(r.Body)` with no `http.MaxBytesReader` — an unbounded read in `HTTPAdapter.ServeHTTP`.
3. `grep -rn 'PreFacadeChain' cmd/ internal/api/` → no matches: the chain is wired in NO production path. It exists only at its definition (`internal/assistant/httpadapter/middleware.go:48`), a doc comment (`internal/assistant/httpadapter/late_binding.go:36`), and in `tests/` (synthetic wiring).
4. `config/smackerel.yaml:995` sets `assistant.transports.http.enabled: true` (default ON), with `body_size_max_bytes: 65536`, `rate_limit_per_user_per_minute: 60`, `required_scope: "assistant:turn"`.
5. `internal/api/router.go:86` mounts `POST /api/assistant/turn` inside the bearer-auth group (router.go:74) under a global `middleware.Throttle(100)` (router.go:68). The mount comment even claims "the adapter enforces its own body cap and rate limits" — which is false for the live wiring.
6. `specs/069-assistant-http-transport/state.json` certifies `status: "done"` with `SCOPE-2` in `certification.completedScopes` and `scopeProgress.done: 7`.

### Root Cause

**SCOPE-2's production-wiring step was never completed.** SCOPE-1d intentionally installed an identity pass-through as a temporary placeholder to reach the HTTP-200 live-wiring target, explicitly documenting that "SCOPE-2 will replace this pass-through with the real auth/scope/body/rate/CORS middleware chain" (`cmd/core/wiring_assistant_facade.go:305-313`; `specs/069-assistant-http-transport/report.md:239`). SCOPE-2 then built `PreFacadeChain` + isolated integration tests but never performed the one-line swap at the wiring site, and was subsequently re-scoped to the USERID-BINDING work. Certification keyed on the USERID-BINDING live proof, not on a DoD item asserting the **live** route enforces 413/429/403, so the gap was promoted to `done`.

### Why Tests Missed It (test-gap analysis)

The SCOPE-2 integration tests construct a **synthetic** router rather than exercising the production one:

- `tests/integration/api/assistant_http_auth_test.go:122` defines `func mountScope2Route(t, facade, cfg, gate) http.Handler` which at line 136 does `r.Use(httpadapter.PreFacadeChain(cfg))` and wraps it with a `syntheticBearerGate` (lines 171, 191, 220).
- `tests/integration/api/assistant_http_limits_test.go` (lines 52, 86, 129) uses the same `mountScope2Route` helper.
- **Neither test calls `api.NewRouter`** (confirmed: `grep 'api.NewRouter'` across both files returns no matches), so neither exercises `cmd/core`'s `SetMiddleware(identity)` wiring. They prove `PreFacadeChain` *works when wired*, not that it *is wired in production*.
- The chaos / golden / transport-hint unit tests call `HTTPAdapter.ServeHTTP` directly, bypassing the late-bound chain entirely.

The structural fix for the test gap is a regression test that drives the **real** router wiring (the same `SetMiddleware` call `cmd/core` makes) and asserts 413/429/403 — so a future revert to the identity wrapper fails at test time.

### Impact Analysis

- **Affected component:** the live `POST /api/assistant/turn` handler chain (default-enabled).
- **Affected controls:** body-size cap (413 / CWE-400 / CWE-770 unbounded `io.ReadAll`), per-user rate limit (429), `assistant:turn` scope-claim gate (403 for per-user PASETO).
- **Affected users:** any holder of a valid bearer token (the route is behind bearer auth, so not anonymous). In shared-token single-user deployments the scope gate is a no-op by design (RequireScope bypass), so the *operative* prod-impact controls are the body cap (DoS) and the rate limit; the scope gate matters once per-user PASETO sessions are issued.
- **Affected data:** none (no corruption, no loss). This is an availability / authorization-hardening defect.
- **Partial mitigations already present:** bearer-auth group (no anonymous access) + global `Throttle(100)` (concurrency cap, not body-size or per-user) — see [bug.md](bug.md) → "Partial Mitigations".

## Fix Design

### Solution Approach (chosen)

Swap the identity pass-through for the already-built, self-contained chain at the single wiring site:

```go
// cmd/core/wiring_assistant_facade.go  (the SetMiddleware call, currently line 314)
//
// before (the defect):
svc.assistantHTTPHandler.SetMiddleware(func(next http.Handler) http.Handler { return next })
//
// after:
svc.assistantHTTPHandler.SetMiddleware(httpadapter.PreFacadeChain(transportCfg))
```

Why this is correct and minimal:

- `transportCfg` (type `httpadapter.HTTPTransportConfig`) is already constructed and in scope at the wiring site — it is passed to `httpadapter.NewHTTPAdapter(Options{... Config: transportCfg})` and logged by the following `slog.Info` (`BodySizeMaxBytes` / `RateLimitPerUserPerMinute` / `RequiredScope`). No new value needs plumbing.
- `httpadapter.PreFacadeChain(cfg)` is self-contained (no new imports beyond the already-imported `httpadapter` package): it validates `RequiredScope` / `RateLimitPerUserPerMinute` / `BodySizeMaxBytes` fail-loud at construction (`middleware.go:49-58`) and returns `scope(rate(body(next)))` (`middleware.go:62-64`).
- `LateBoundHandler.ServeHTTP` already applies the installed chain (`internal/assistant/httpadapter/late_binding.go:55-59`: `if c := h.chain.Load(); c != nil { handler = (*c)(a) }`). With `PreFacadeChain` installed, `(*c)(a)` becomes `scope(rate(body(adapter)))`, so the body cap runs **before** the adapter's `io.ReadAll`.
- `bodySizeCap` uses `http.MaxBytesReader` (`middleware.go:101/109`) and emits 413 before decode, so the unbounded `io.ReadAll` at adapter.go:329 only ever sees a bounded body.

### Dev / shared-token compatibility (no regression)

`auth.RequireScope` (the first layer of `PreFacadeChain`) passes `SessionSourceSharedToken` and `SessionSourceBootstrap` through with a bypass counter (`internal/auth/scope_middleware.go:71-77`). So after the swap, dev/test shared-token requests and the one-shot enrollment flow still reach the facade; only per-user PASETO sessions lacking `assistant:turn` are rejected (403). This is the spec-060 intended behavior (re-affirmed by sweep R20).

### Regression test (the test-gap close)

`bubbles.implement` authors a test that builds the router the way `cmd/core` does — installing `PreFacadeChain(transportCfg)` via the same `SetMiddleware` path through `api.NewRouter` / the late-bound handler — and asserts:

- an over-cap body → 413 (no facade call); **adversarial**: the same request against the identity-wrapper wiring reaches the facade / is not 413 → the test FAILS pre-fix.
- per-user budget exceeded → 429 (no facade call).
- per-user PASETO without `assistant:turn` → 403; shared-token still → 200/valid envelope.

The test MUST be RED against the identity pass-through and GREEN after the swap (proving it is non-tautological and would catch a future revert).

### Alternative Approaches Considered

1. **Move body/rate/scope enforcement into `HTTPAdapter.ServeHTTP` directly** — Rejected. Duplicates logic that already exists in `PreFacadeChain`, violates the spec's layered design (Hard Constraint: controls are "layered on top by SCOPE-2"), and leaves `PreFacadeChain` dead. The chain is the intended seam.
2. **Add the body cap at the chi router layer (`internal/api/router.go`)** — Rejected. Splits SCOPE-2's three controls across two files, bypasses the SST-driven `PreFacadeChain` config validation, and does not provide the per-user rate limit or scope gate. The wiring site is the correct single seam.
3. **Leave the identity wrapper and document the gap only** — Rejected. The endpoint is default-enabled in production; a documented-but-unfixed unbounded `io.ReadAll` (CWE-400/770) on a live route is not an acceptable terminal state. Discovery routes the fix; it does not waive it.

## Complexity Tracking

None — simplest viable fix used. The production change is a one-line `SetMiddleware` argument swap to an already-built, already-tested, self-contained chain; the only added surface is the missing real-router regression test (the test-gap close).

| Decision | Simpler fix considered | Why rejected |
|----------|------------------------|--------------|
| Add a real-router regression test in addition to the one-line swap | Swap only, no new test | A swap with no real-router test would leave the exact synthetic-vs-production test gap that allowed the defect; the adversarial regression test is required by bug-fix DoD and is the structural recurrence guard. |
