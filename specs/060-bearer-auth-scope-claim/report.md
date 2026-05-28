# Execution Report: 060 Bearer Auth Scope Claim & RequireScope Middleware

## Summary

Planning artifacts authored 2026-05-28 by `bubbles.plan`. Four scopes ordered with strict gating; implementation work has not started. This report holds evidence sections that will be populated by `bubbles.implement` and `bubbles.test` as each scope executes.

## Completion Statement

Planning-only execution. All four scopes are `Not started`. `scopes.md`, `scenario-manifest.json`, and `state.json` agree on the active scope inventory (4) and scenario contracts (SCN-060-001 through SCN-060-020). No implementation evidence is recorded yet.

Design §7.4 deferred question is resolved in this plan in favor of the `./smackerel.sh auth` passthrough wrapper (Scope 3); spec.md UC text continues to use the `./smackerel.sh auth …` form unchanged.

## Planning Validation Evidence

### Artifact Lint

To be populated by validation after scopes are written. Expected command:

```
bash .github/bubbles/scripts/artifact-lint.sh specs/060-bearer-auth-scope-claim
```

### Traceability Guard

To be populated by validation. Expected command:

```
timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/060-bearer-auth-scope-claim
```

## Scope 1

**Status:** Done (bubbles.implement, 2026-05-28). PASETO `scope` claim wired end-to-end through `IssueToken` → `VerifyAndParse` → `bearerAuthMiddleware` → `Session.Scopes`. Canonical registry at `internal/auth/scopes.go`.

### Files Changed

- `internal/auth/scopes.go` (new) — `RegisteredScopeSurfaces`, `ScopeNameRegex`, `ValidateScopeName`, `ExtractScopeSurface`, `IsRegisteredScopeSurface`.
- `internal/auth/scopes_test.go` (new) — registry + regex + surface extraction unit tests.
- `internal/auth/session.go` — `Session.Scopes []string` field (nil for legacy/shared/bootstrap; never wildcard).
- `internal/auth/verify.go` — `ParsedToken.Scopes`, `ErrScopeClaimMalformed`, `getScopeClaim` helper (parse-time regex defense-in-depth), populated in `VerifyAndParse`.
- `internal/auth/issue.go` — `IssueOptions.Scopes`, `IssueAndPersistOptions.Scopes`; `IssueToken` sets the PASETO `scope` claim only when `len(opts.Scopes) > 0`.
- `internal/auth/scope_claim_test.go` (new) — roundtrip + legacy + malformed-claim defense tests.
- `internal/api/router.go::bearerAuthMiddleware` — copies `parsed.Scopes` into `Session.Scopes` on the per-user PASETO branch.
- `internal/api/router_scope_test.go` (new) — live-middleware integration: scoped token yields scopes, legacy token yields nil.

### Test Evidence

**Claim Source:** executed.

`./smackerel.sh test unit` ran the full unit suite via the repo CLI. All Go packages and Python ML tests passed (457 py + all go packages `ok`).

```text
ok      github.com/smackerel/smackerel/internal/auth    15.197s
ok      github.com/smackerel/smackerel/internal/auth/revocation 0.007s
ok      github.com/smackerel/smackerel/internal/metrics 0.030s
ok      github.com/smackerel/smackerel/internal/api     9.297s
...
457 passed in 27.45s
[go-unit] go test ./... finished OK
[py-unit] pytest ml/tests finished OK
```

Scope-1 unit tests (selective):

```text
=== RUN   TestValidateScopeName                             --- PASS
=== RUN   TestRegisteredScopeSurfaces_ContainsExtension     --- PASS
=== RUN   TestExtractScopeSurface                           --- PASS
=== RUN   TestIssueToken_SetsScopeClaim                     --- PASS
=== RUN   TestVerifyAndParse_NilScopesForLegacyToken        --- PASS
=== RUN   TestVerifyAndParse_MalformedScopeClaimFallsBackToNil --- PASS
=== RUN   TestGetScopeClaim_AbsentReturnsNilNil             --- PASS
```

Scope-1 live-router integration (per-user PASETO populates `Session.Scopes`):

```text
=== RUN   TestBearerAuthMiddleware_PopulatesSessionScopes
=== RUN   TestBearerAuthMiddleware_PopulatesSessionScopes/scoped_token
=== RUN   TestBearerAuthMiddleware_PopulatesSessionScopes/legacy_token_yields_nil_scopes
--- PASS: TestBearerAuthMiddleware_PopulatesSessionScopes (0.00s)
    --- PASS: scoped_token (0.00s)
    --- PASS: legacy_token_yields_nil_scopes (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     0.035s
```

Full live-stack `./smackerel.sh test integration` ran end-to-end with the disposable test stack (postgres, NATS, ML sidecar, ollama, smackerel-core); **294 PASS / 0 FAIL**.

### Adversarial Evidence

`TestVerifyAndParse_MalformedScopeClaimFallsBackToNil` mints a token whose `scope` claim is `["BadlyFormatted"]` and asserts `parsed.Scopes == nil` — defense-in-depth proof that a forged claim CANNOT upgrade a session into a scoped one (BS-002 invariant).

### No-Defaults / SST Compliance

- No `os.Getenv` fallback patterns introduced anywhere.
- No new SST keys added; `config/smackerel.yaml` unchanged.
- New surface registry is compiled-in code, not config.

### Change Boundary

Files modified are exactly the allowed family for Scope 1 plus the corresponding `_test.go` peers and the `bearerAuthMiddleware` session-population line. Excluded surfaces (middleware file, metrics, CLI, docs) were not edited in Scope 1.

## Scope 2

**Status:** Done (bubbles.implement, 2026-05-28). `auth.RequireScope` middleware + two new Prometheus counter vectors. BS-002 adversarial regression headline test green.

### Files Changed

- `internal/auth/scope_middleware.go` (new) — `RequireScope(required ...string)` exporter. AND semantics, construction-time panic when `len(required) == 0`, 500 on absent session, dev/test bypass for `SessionSourceSharedToken` and `SessionSourceBootstrap`, 403 `scope_required` body shape, structured WARN log.
- `internal/auth/scope_middleware_test.go` (new) — all middleware behaviors covered: BS-001 happy path, BS-002 adversarial legacy-token reject (counter delta, body, log shape), BS-003 cross-scope reject (first-missing label), AND semantics, shared-token + bootstrap bypass with counter increments, construction panic, 500-on-absent-session.
- `internal/metrics/auth.go` — `AuthScopeRejected` (`required_scope`, `user_id`) + `AuthScopeCheckBypassed` (`source`) counter vectors registered via `prometheus.MustRegister`.

### Test Evidence

**Claim Source:** executed.

Selective scope-2 unit results (`go test ./internal/auth/...`):

```text
=== RUN   TestRequireScope_PanicsOnZeroRequired              --- PASS
=== RUN   TestRequireScope_AcceptsContainedScope             --- PASS
=== RUN   TestRequireScope_RejectsLegacyTokenSession         --- PASS  (BS-002)
=== RUN   TestRequireScope_RejectsMismatchedScope_FirstMissingLabel --- PASS
=== RUN   TestRequireScope_AndSemanticsRejectsPartialMatch   --- PASS
=== RUN   TestRequireScope_BypassesForSharedToken            --- PASS
=== RUN   TestRequireScope_BypassesForBootstrap              --- PASS
=== RUN   TestRequireScope_500OnAbsentSession                --- PASS
ok      github.com/smackerel/smackerel/internal/auth    15.197s
```

Full `./smackerel.sh test unit` + `./smackerel.sh test integration` ran with **0 failures** (integration 294 PASS / 0 FAIL on the disposable test stack).

### BS-002 Adversarial Headline Evidence

`TestRequireScope_RejectsLegacyTokenSession` asserts ALL of:

1. Response status == `403 Forbidden`
2. Body == `{"error":"scope_required","required":["extension:bookmarks,history"]}`
3. `auth_scope_rejected_total{required_scope="extension:bookmarks,history",user_id="bob"}` delta == 1
4. Downstream handler NOT invoked (the test relies on body inspection — the handler would return 202 otherwise)

If a future refactor causes `getScopeClaim` to treat missing/malformed scope claim as a wildcard, the assertion that the counter delta == 1 AND the body content `scope_required` will both fail. The test has NO bailout `if err != nil { return }` patterns.

### Counter Cardinality

- `AuthScopeRejected` labels: `required_scope` ∈ operator-controlled scope registry (closed set); `user_id` ∈ enrolled user set. Bounded.
- `AuthScopeCheckBypassed` labels: `source` ∈ `{"shared_token","bootstrap"}` (closed set, asserted in `TestRequireScope_BypassesForSharedToken` + `..._BypassesForBootstrap`).

### No Endpoint Wiring

Grep proof — `RequireScope` is not invoked from `internal/api/` or `cmd/`:

```text
$ grep -RnE 'RequireScope' internal/api/ cmd/ | grep -v _test.go
(no output)
```

(Spec 058 wires its own endpoint per the spec 060 contract.)

### Change Boundary

Files modified are exactly the allowed family for Scope 2 (`internal/auth/scope_middleware*.go`, `internal/metrics/auth.go`). Router file, CLI, docs untouched.

## Scope 3

Not started — out of this dispatch (CLI flags + passthrough wrapper; ships in a follow-up).

## Scope 4

Not started — out of this dispatch (operator docs; ships in a follow-up).
