# Bug: BUG-069-001 Spec 069 SCOPE-2 pre-facade middleware chain (`PreFacadeChain`) is built + isolated-tested but NEVER wired into the live `POST /api/assistant/turn` route — production installs an identity pass-through, leaving the default-enabled endpoint with no body-size cap (unbounded `io.ReadAll`, CWE-400/770), no per-user rate limit, and no `assistant:turn` scope-claim gate

## Summary

Spec 069 SCOPE-2 landed a self-contained pre-facade middleware chain `httpadapter.PreFacadeChain` (scope-claim gate → per-user rate limit → body-size cap) and proved it with isolated integration tests, but the production wiring at `cmd/core/wiring_assistant_facade.go:314` installs an **identity pass-through** (`func(next http.Handler) http.Handler { return next }`) instead of `PreFacadeChain`. The default-enabled live route `POST /api/assistant/turn` therefore runs through an **unbounded `io.ReadAll(r.Body)`** (`internal/assistant/httpadapter/adapter.go:329`) with none of the three SCOPE-2 controls active — yet `state.json` certifies SCOPE-2 `done`.

## Severity

- [ ] Critical — System unusable, data loss
- [x] **High** — A default-enabled, authenticated production endpoint reads request bodies into memory with no upper bound (CWE-400 / CWE-770 uncontrolled resource consumption / memory-exhaustion DoS), and two further spec-mandated controls (per-user rate limit, `assistant:turn` scope-claim gate) are absent in production. Partially mitigated by the bearer-auth group + global `Throttle(100)` (so this is an *authenticated*-DoS / authz-gap, not anonymous), which is why it is High rather than Critical. No data loss; no silent corruption.
- [ ] Medium
- [ ] Low

## Status

- [x] Reported
- [x] Confirmed (reproduced by stochastic-quality-sweep round R41 `harden-to-doc`; re-verified independently by `bubbles.bug` with real commands at the current working tree — output captured below)
- [x] In Progress (discovery + documentation + root-cause complete; fix routed)
- [x] Fixed
- [x] Verified
- [ ] Closed

> **Fix applied + verified.** `cmd/core/wiring_assistant_facade.go:329` installs `svc.assistantHTTPHandler.SetMiddleware(httpadapter.PreFacadeChain(transportCfg))` (the identity pass-through swap landed in commit `ada0efc1`); the real-wiring regression `cmd/core/wiring_assistant_http_prefacade_regression_test.go` (commit `eadfada7`) drives the production `wireAssistantHTTPAdapter` seam and is RED against the identity wrapper / GREEN after the swap (413 / 429 / 403 / shared-token-200, re-verified on the current tree 2026-06-24). Discovery + root-cause were by `bubbles.bug`; the implement/test/regression/security/validate/audit phases drove it to terminal-for-mode (`bugfix-fastlane` → `done`) via validate-owned certification. `Closed` is left unchecked pending the orchestrator's review + push.

## Reproduction Steps

1. From the current working tree, confirm the live wiring installs only an identity pass-through (no `PreFacadeChain`):
   `grep -n 'SetMiddleware\|PreFacadeChain' cmd/core/wiring_assistant_facade.go`
   → line 314 is `svc.assistantHTTPHandler.SetMiddleware(func(next http.Handler) http.Handler { return next })`; `PreFacadeChain` does not appear.
2. Confirm the adapter's `ServeHTTP` reads the body with no upper bound:
   `grep -n 'io.ReadAll\|MaxBytesReader' internal/assistant/httpadapter/adapter.go`
   → line 329 `body, err := io.ReadAll(r.Body)`; no `MaxBytesReader` anywhere in the file.
3. Confirm `PreFacadeChain` is wired NOWHERE in any production path:
   `grep -rn 'PreFacadeChain' cmd/ internal/api/`
   → no matches. `PreFacadeChain` appears only in its own definition (`internal/assistant/httpadapter/middleware.go`), a doc comment (`internal/assistant/httpadapter/late_binding.go:36`), and test files under `tests/`.
4. Confirm the route is enabled by default and mounted inside the bearer-auth group:
   `awk 'NR>=994 && NR<=1003' config/smackerel.yaml` → `assistant.transports.http.enabled: true`, `body_size_max_bytes: 65536`, `rate_limit_per_user_per_minute: 60`, `required_scope: "assistant:turn"`.
   `internal/api/router.go:86` mounts `POST /api/assistant/turn` inside the `r.Use(deps.bearerAuthMiddleware)` group (router.go:74), under a global `middleware.Throttle(100)` (router.go:68).
5. Confirm SCOPE-2 is nevertheless certified `done`:
   `specs/069-assistant-http-transport/state.json` → top-level `status: "done"`, `certification.completedScopes` includes `SCOPE-2`, `certification.scopeProgress.done: 7`.
6. Read the placeholder admission the implementer left behind:
   `cmd/core/wiring_assistant_facade.go:305-313` and `specs/069-assistant-http-transport/report.md:239` both state the identity wrapper is temporary and "SCOPE-2 will replace this pass-through with the real auth/scope/body/rate/CORS middleware chain" — but that replacement never landed.

## Expected Behavior

The live, default-enabled `POST /api/assistant/turn` MUST enforce, in production (not only in synthetic tests), the three SCOPE-2 controls that `PreFacadeChain` composes:

- **Body-size cap → HTTP 413** before any JSON decode or facade call, bounded by `assistant.transports.http.body_size_max_bytes` (65536) — satisfying spec 069 **SCN-069-A10** and **Hard Constraint 3** (NO-DEFAULTS body-size cap).
- **Per-user rate limit → HTTP 429** keyed per authenticated user, bounded by `assistant.transports.http.rate_limit_per_user_per_minute` (60) — satisfying spec 069 **SCN-069-A10**.
- **`assistant:turn` scope-claim gate → HTTP 403** for per-user PASETO sessions lacking the scope — satisfying spec 069 **Hard Constraint 2** and spec 060 (scope claims).

Dev / single-user shared-token requests MUST still pass: `auth.RequireScope` deliberately bypasses `SessionSourceSharedToken` and `SessionSourceBootstrap` (`internal/auth/scope_middleware.go:71-77`), so the scope gate fails closed only for real per-user PASETO sessions, which is the intended behavior.

## Actual Behavior

The production handler chain is `bearerAuthMiddleware → identity-pass-through → HTTPAdapter.ServeHTTP`. Because the late-bound handler applies its chain only when one is set and the installed chain is the identity function (`internal/assistant/httpadapter/late_binding.go:55-59`, `(*c)(a) == a`), the adapter serves directly:

- **No body bound** — `io.ReadAll(r.Body)` (adapter.go:329) reads the entire request body into a `[]byte` with no `http.MaxBytesReader`; a large or slow body is fully buffered (CWE-400 / CWE-770).
- **No per-user rate limit** — the SCN-069-A10 429 path exists only inside `PreFacadeChain.perUserRateLimit`, which is not in the live chain.
- **No `assistant:turn` scope gate** — `auth.RequireScope(cfg.RequiredScope)` exists only inside `PreFacadeChain`; a valid bearer token without the `assistant:turn` scope (a real per-user PASETO session) is admitted to the facade, violating Hard Constraint 2.

## Environment

- Repo: smackerel (Go core runtime), current working tree (uncommitted in-flight specs present elsewhere — untouched by this packet).
- Sweep: stochastic-quality-sweep round **R41**, mode `harden-to-doc`, parent spec `specs/069-assistant-http-transport` (`status: done`, `workflowMode: full-delivery`).
- Defective production wiring: `cmd/core/wiring_assistant_facade.go:314`.
- Unbounded read: `internal/assistant/httpadapter/adapter.go:329` (`HTTPAdapter.ServeHTTP`).
- Built-but-unwired control: `internal/assistant/httpadapter/middleware.go:48` (`PreFacadeChain`), `bodySizeCap`→`http.MaxBytesReader` at middleware.go:101/109.
- Route mount: `internal/api/router.go:86` (inside bearer-auth group, router.go:74; global throttle router.go:68).
- SST default: `config/smackerel.yaml:995` (`http.enabled: true`).
- Certification: `specs/069-assistant-http-transport/state.json` (SCOPE-2 `done`).

## Error Output

```text
$ grep -n 'SetMiddleware\|PreFacadeChain' cmd/core/wiring_assistant_facade.go
306:	// install an identity wrapper so SetMiddleware is called
314:	svc.assistantHTTPHandler.SetMiddleware(func(next http.Handler) http.Handler { return next })

$ grep -n 'io.ReadAll\|MaxBytesReader' internal/assistant/httpadapter/adapter.go
329:	body, err := io.ReadAll(r.Body)

$ grep -rn 'PreFacadeChain' cmd/ internal/api/
(no matches — PreFacadeChain absent from all production paths)

$ awk 'NR>=994 && NR<=1003 {printf "%d: %s\n", NR, $0}' config/smackerel.yaml
994:     http:
995:       enabled: true # REQUIRED: strict bool ("true"|"false")
996:       schema_version: "v1" # REQUIRED: pinned wire schema version
997:       body_size_max_bytes: 65536 # REQUIRED: integer >= 1
998:       rate_limit_per_user_per_minute: 60 # REQUIRED: integer >= 1
999:       cors_allowed_origins: [] # REQUIRED: explicit origin list (empty = same-origin only)
1000:       conversation_ttl_seconds: 86400 # REQUIRED: integer >= 1
1001:       transport_hint_allowlist: [ "web", "mobile", "bridge" ] # REQUIRED: non-empty closed-vocabulary list
1002:       required_scope: "assistant:turn" # REQUIRED: spec 060 scope-claim label

$ grep -n '"status"\|SCOPE-2\|"done": 7' specs/069-assistant-http-transport/state.json | head
6:  "status": "done",
47:    "status": "done",
55:      "done": 7,
62:      "SCOPE-2",
```

## Partial Mitigations (honest accounting — why High, not Critical)

- **Bearer-auth group (`internal/api/router.go:74`).** `POST /api/assistant/turn` is mounted behind `deps.bearerAuthMiddleware`, so anonymous callers get 401 and never reach the unbounded read. The exposure is to *any holder of a valid bearer token* — in single-user shared-token deployments that is effectively one app credential; in multi-user PASETO deployments it is any enrolled user. This bounds the blast radius but does not remove it (a single authenticated client can OOM the core process).
- **Global `middleware.Throttle(100)` (`internal/api/router.go:68`).** chi's `Throttle` caps *concurrent in-flight* `/api` requests at 100 across all routes. It does NOT bound body size (100 concurrent unbounded `io.ReadAll`s still exhaust memory) and is NOT per-user (one client can occupy all 100 slots), so it substitutes for neither the body-size cap nor the per-user rate limit SCN-069-A10 requires.
- **`SCN-069-A02` (401 auth-mandatory) is satisfied** in production by the bearer-auth group. The defect is specifically the absent **413 / 429 / 403** controls, not the 401.

## Root Cause Analysis (Five Whys)

- **Why does the live endpoint lack body/rate/scope controls?** Because the production handler chain installs an identity pass-through (`cmd/core/wiring_assistant_facade.go:314`), not `PreFacadeChain`, so the late-bound handler wraps the adapter in a no-op and the adapter's unbounded `io.ReadAll` runs first.
- **Why is the identity pass-through still there?** Because SCOPE-1d intentionally installed it as a temporary placeholder to reach the HTTP-200 live-wiring target ("SCOPE-2 will replace this pass-through with the real auth/scope/body/rate/CORS chain" — `cmd/core/wiring_assistant_facade.go:305-313`, `report.md:239`), and the SCOPE-2 production-wiring step that was supposed to swap it never landed.
- **Why did SCOPE-2 land the chain but not the wiring?** Because SCOPE-2's deliverable was split: the chain (`PreFacadeChain`) + its isolated tests were written, but the one-line swap at the wiring site was the final step and was missed. SCOPE-2 was later re-scoped to the USERID-BINDING work (`state.json` notes), and certification keyed on the USERID-BINDING live proof rather than on the body/rate/scope wiring.
- **Why didn't tests catch it?** Because the SCOPE-2 integration tests build a **synthetic** router `mountScope2Route` (`tests/integration/api/assistant_http_auth_test.go:122`) that hand-wires `r.Use(httpadapter.PreFacadeChain(cfg))` + a fake bearer gate (`assistant_http_auth_test.go:136`); they never exercise `api.NewRouter` + the real `cmd/core` wiring. The chaos / golden / translate unit tests hit `HTTPAdapter.ServeHTTP` directly. No test drives the *production* handler chain, so the identity-wrapper wiring passes every test.
- **Why did certification pass with the gap?** Because `state.json` was promoted to `done` (all 7 scopes) on the strength of the USERID-BINDING live-stack PASS and the SCOPE-1d adapter-bind proof; no scope's DoD pinned "the *live* route enforces 413/429/403 through `api.NewRouter`", so the synthetic-only coverage read as complete.

## Fix Routing (this packet does NOT fix — discovery + documentation + root-cause only)

One-line, self-contained, no new dependencies. At `cmd/core/wiring_assistant_facade.go:314` swap:

```go
// before (identity pass-through — the defect)
svc.assistantHTTPHandler.SetMiddleware(func(next http.Handler) http.Handler { return next })

// after (wire the already-built SCOPE-2 chain)
svc.assistantHTTPHandler.SetMiddleware(httpadapter.PreFacadeChain(transportCfg))
```

`transportCfg` is already in scope at the wiring site (the next `slog.Info` logs its `BodySizeMaxBytes` / `RateLimitPerUserPerMinute` / `RequiredScope`). `PreFacadeChain(cfg)` validates its config fail-loud at construction (`middleware.go:49-58`) and composes `scope(rate(body(next)))`. `auth.RequireScope` bypasses `SessionSourceSharedToken` + `SessionSourceBootstrap` (`internal/auth/scope_middleware.go:71-77`), so dev shared-token requests still pass; the gate fails closed only for real per-user PASETO sessions — the intended behavior.

Owner sequence: `bubbles.implement` (apply the swap + author a regression test that drives the **real** `api.NewRouter` wiring and asserts 413 / 429 / 403, written to FAIL against the identity-wrapper wiring and PASS after the swap) → `bubbles.test` (RED-before / GREEN-after proof + full regression suite).

## Related

- Feature: `specs/069-assistant-http-transport/` (parent; `status: done`)
- Spec scenarios: SCN-069-A10 (413/429 body+rate), SCN-069-A02 (401 auth), Hard Constraint 2 (spec-060 `assistant:turn` scope-claim → 403), Hard Constraint 3 (NO-DEFAULTS body-size cap)
- Depends-on specs: spec 044 (per-user bearer auth), spec 060 (bearer-auth scope claim)
- CWE: CWE-400 (Uncontrolled Resource Consumption), CWE-770 (Allocation of Resources Without Limits or Throttling)
- Built-but-unwired symbol: `httpadapter.PreFacadeChain` (`internal/assistant/httpadapter/middleware.go:48`)

## Root Cause (one line)

SCOPE-2 built `PreFacadeChain` + isolated tests but never completed the production-wiring swap at `cmd/core/wiring_assistant_facade.go:314`; the temporary SCOPE-1d identity pass-through remains live, so the default-enabled `POST /api/assistant/turn` runs an unbounded `io.ReadAll` with no body cap, no per-user rate limit, and no `assistant:turn` scope gate — and the synthetic `mountScope2Route` test wiring (never `api.NewRouter`) masks it.
