# Spec: BUG-069-001 Live `POST /api/assistant/turn` MUST enforce the SCOPE-2 pre-facade controls (413 / 429 / 403) in production

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Context

Parent spec `specs/069-assistant-http-transport` introduced the assistant HTTP transport and (in SCOPE-2) the pre-facade middleware chain `httpadapter.PreFacadeChain` that composes a scope-claim gate, a per-user rate limit, and a body-size cap. The chain is built and isolated-tested, but the production wiring installs an identity pass-through, so none of the three controls run on the live, default-enabled route. This bug spec defines the expected production behavior the fix must restore.

## Expected Behavior

### EB-1 — Body-size cap enforced in production (413)

The live `POST /api/assistant/turn` MUST reject a request whose body exceeds `assistant.transports.http.body_size_max_bytes` (SST value 65536) with **HTTP 413** in a v1 wire envelope, **before** any JSON decode or `Facade.Handle` invocation, and MUST NOT buffer the whole oversized body into memory. This satisfies spec 069 **SCN-069-A10** and **Hard Constraint 3** and closes the CWE-400 / CWE-770 unbounded-`io.ReadAll` exposure at `internal/assistant/httpadapter/adapter.go:329`.

### EB-2 — Per-user rate limit enforced in production (429)

The live route MUST reject requests that exceed `assistant.transports.http.rate_limit_per_user_per_minute` (SST value 60) for a given authenticated user with **HTTP 429** in a v1 wire envelope, with no facade invocation. The limiter MUST be keyed per user (per spec 069 SCN-069-A10 / the SCOPE-2 `perUserRateLimit` key function), not globally — the existing global `middleware.Throttle(100)` does not satisfy this requirement.

### EB-3 — `assistant:turn` scope-claim gate enforced in production (403)

For a per-user PASETO session that lacks the `assistant:turn` scope, the live route MUST return **HTTP 403** without invoking the facade, satisfying spec 069 **Hard Constraint 2** and spec 060. The existing bearer-auth group already returns 401 for missing/invalid tokens (SCN-069-A02); this requirement adds the missing 403 for valid-token-but-wrong-scope.

### EB-4 — Dev / shared-token requests still pass (no regression)

A `SessionSourceSharedToken` or `SessionSourceBootstrap` request MUST continue to reach the facade after the fix. `auth.RequireScope` bypasses both sources (`internal/auth/scope_middleware.go:71-77`); the scope gate fails closed only for real per-user PASETO sessions. Single-user dev/test ergonomics and the one-shot enrollment flow MUST be unaffected.

### EB-5 — Production handler chain runs `PreFacadeChain`, proven through the real router

The live wiring MUST install `httpadapter.PreFacadeChain(transportCfg)` at the SetMiddleware site, and a regression test MUST drive the **real** `api.NewRouter` + `cmd/core`-equivalent wiring (NOT the synthetic `mountScope2Route`) to prove EB-1..EB-3. The test MUST FAIL against the identity-pass-through wiring and PASS after the swap (non-tautological / adversarial).

## Acceptance Criteria

- AC-1: With the fix applied, an over-cap body (> 65536 bytes) to the live route yields **413** and the facade is never invoked; memory is bounded (no full buffering of the oversized body). [EB-1, SCN-069-A10, HC-3]
- AC-2: Exceeding the per-user minute budget on the live route yields **429** with no facade invocation, keyed per user. [EB-2, SCN-069-A10]
- AC-3: A per-user PASETO session without `assistant:turn` yields **403** on the live route with no facade invocation. [EB-3, HC-2, spec 060]
- AC-4: A `SessionSourceSharedToken` request still reaches the facade (200/valid envelope) after the fix. [EB-4]
- AC-5: `grep -rn 'PreFacadeChain' cmd/ internal/api/` returns at least one production match (the wiring site), and `cmd/core/wiring_assistant_facade.go` no longer contains the identity pass-through `func(next http.Handler) http.Handler { return next }` at the assistant SetMiddleware call. [EB-5]
- AC-6: The regression test drives the real router wiring and is RED against the identity-wrapper wiring, GREEN after the swap. [EB-5]
- AC-7: `state.json` for the parent spec remains internally consistent; this bug's `state.json` reaches terminal-for-mode only via validate-owned certification after AC-1..AC-6 are proven.

## Out Of Scope

- Any change to `httpadapter.PreFacadeChain` internals, `HTTPAdapter.ServeHTTP`, the wire schema, or the SST contract — the chain is already correct and tested; only the wiring is missing.
- CORS handling (`cors_allowed_origins`) beyond what `PreFacadeChain` already composes — not part of this defect.
- Any edit to other in-flight specs, `internal/connector/*`, `ml/`, deploy surfaces, or `config/smackerel.yaml` (the SST values are already correct).
- Re-litigating the parent spec's `done` status; this packet documents and routes the wiring fix only.

## Product Principle Alignment

- **P8 Trust Through Transparency** — the live endpoint must actually enforce the controls its config advertises; a certified-`done` scope whose control runs only in synthetic tests is the transparency gap this bug closes.
- **P10 QF Companion Boundary** — unchanged; no financial-action surface is added. The scope gate (EB-3) strengthens the existing side-effect/auth boundary by enforcing `assistant:turn` for per-user sessions.
