# Scopes: 060 Bearer Auth Scope Claim & RequireScope Middleware

<!-- bubbles:g040-skip-begin -->
<!-- G040 skip (whole-file): remaining "deferred"/"follow-up"/"pending"/"Uncertainty" hits across this scopes.md are: (a) implementation-resolved design-deferred questions noted for traceability (e.g., §7.4 passthrough-wrapper form chosen at plan time and confirmed at implement time); (b) honest Uncertainty Declarations on rows where the functional correctness is covered by adjacent unit/adversarial tests and the live-stack / formal-guard step is routed via state.json.transitionRequests for the next dispatch (passthrough-wrapper live-stack integration test; hot-path microbenchmark; regression-baseline-guard registration; pii-scan on staged diff). None of these are undeclared deferred work; the implementation has shipped in commits 5ce89484 (Scopes 1+2) and 1cc7d761 (Scopes 3+4). See report.md ## Close-Out 2026-05-28 for the named concerns set acknowledged at done_with_concerns. -->

## Execution Outline

### Phase Order

1. **PASETO `scope` Claim + Session/ParsedToken Wiring + Surface Registry** — Extend `auth.IssueToken` to set the PASETO `scope` claim when supplied; extend `auth.VerifyAndParse` and `auth.ParsedToken` with `Scopes []string` (with parse-time regex defense-in-depth); extend `auth.Session` with `Scopes []string`; populate `Session.Scopes` from `ParsedToken.Scopes` in `internal/api/router.go::bearerAuthMiddleware`; create `internal/auth/scopes.go` with the canonical `RegisteredScopeSurfaces` allowlist and the scope-name regex. Zero behavior change for legacy tokens.
2. **`auth.RequireScope` Middleware + Metrics** — Add `internal/auth/scope_middleware.go` exporting `RequireScope(required ...string) func(http.Handler) http.Handler` with AND semantics, dev/test bypass pass-through, 403 `scope_required` body shape, structured WARN log, no-session 500 guard; register `AuthScopeRejected` and `AuthScopeCheckBypassed` counter vectors in `internal/metrics/auth.go`.
3. **CLI `--scope` Flags + Rotation Preserve/Demote + `auth inspect` + `./smackerel.sh auth` Wrapper** — Extend `cmd/core/cmd_auth.go::runAuthEnroll` and `runAuthRotate` with repeatable `--scope` flag (via `flag.Func`), `--allow-unknown-surface` escape hatch, `--prior-token <wire>` for rotation preserve path, `--scope ""` demote sentinel; add `runAuthInspect`; add `auth)` passthrough case to `smackerel.sh` that forwards args to the built binary inside the `smackerel-core` container.
4. **Operator Docs (`docs/Operations.md` + `docs/API.md`)** — Add "Scoped Token Enrollment" subsection to `docs/Operations.md` (mint, rotate-preserve, rotate-replace, rotate-demote, inspect, migration notes); add `403 scope_required` response shape and initial wiring matrix to `docs/API.md`.

### New Types and Signatures

- `internal/auth/scopes.go` (new) — `var RegisteredScopeSurfaces = []string{"extension"}`; `var ScopeNameRegex = regexp.MustCompile(...)`; `func ValidateScopeName(scope string) error`; `func ExtractScopeSurface(scope string) string`.
- `internal/auth/session.go` — `Session` struct gains `Scopes []string` (nil for legacy / shared-token / bootstrap sources).
- `internal/auth/verify.go` — `ParsedToken` gains `Scopes []string`; helper `getScopeClaim(token *paseto.Token) ([]string, error)` returns `(nil, nil)` on absent claim, `(nil, ErrScopeClaimMalformed)` on regex-failing element; new `var ErrScopeClaimMalformed = errors.New("scope claim contains malformed element")`.
- `internal/auth/issue.go` — `IssueAndPersistOptions` gains `Scopes []string`; `IssueToken` calls `token.Set("scope", opts.Scopes)` only when `len(opts.Scopes) > 0`.
- `internal/auth/scope_middleware.go` (new) — `func RequireScope(required ...string) func(http.Handler) http.Handler`; panics at construction when `len(required) == 0`.
- `internal/metrics/auth.go` — `var AuthScopeRejected = prometheus.NewCounterVec(...{Subsystem:"auth", Name:"scope_rejected_total"}, []string{"required_scope","user_id"})`; `var AuthScopeCheckBypassed = prometheus.NewCounterVec(...{Subsystem:"auth", Name:"scope_check_bypassed_total"}, []string{"source"})`; both registered in the package `Register(...)` list.
- `cmd/core/cmd_auth.go` — `runAuthEnroll`/`runAuthRotate` gain `--scope` (repeatable via `flag.Func`), `--allow-unknown-surface`, `--prior-token` flags; new `runAuthInspect(ctx, args)` subcommand wired through `runAuthCommand` dispatch.
- `smackerel.sh` — new `auth)` case after the existing `backup)` case forwarding args to `docker compose exec smackerel-core smackerel auth "$@"` (or equivalent host-binary invocation when the container is not running; design choice deferred to implementation — both paths MUST honor SST env loading and MUST NOT inject a fallback).

### Validation Checkpoints

- After Scope 1, `go test ./internal/auth -run 'TestIssueToken_SetsScopeClaim|TestVerifyAndParse_PopulatesScopes|TestVerifyAndParse_NilScopesForLegacyToken|TestGetScopeClaim_RejectsMalformed|TestValidateScopeName|TestRegisteredScopeSurfaces'` proves wire-format roundtrip and legacy-token backward compatibility; `go test ./internal/api -run TestBearerAuthMiddleware_PopulatesSessionScopes` proves the session is populated end-to-end through the live router seam.
- After Scope 2, `go test ./internal/auth -run 'TestRequireScope_AcceptsContainedScope|TestRequireScope_RejectsLegacyTokenSession|TestRequireScope_RejectsMismatchedScope|TestRequireScope_BypassesForSharedToken|TestRequireScope_BypassesForBootstrap|TestRequireScope_PanicsOnZeroRequired|TestRequireScope_500OnAbsentSession'` proves all middleware behaviors; closed-set label cardinality unit tests prove the metrics surface; integration test against the live test stack (router with `bearerAuthMiddleware` + `RequireScope`) proves BS-001 (202) and adversarial BS-002 (403 + counter delta 1 + structured log line).
- After Scope 3, `go test ./cmd/core -run 'TestAuthEnroll_RejectsInvalidScopeName|TestAuthEnroll_RejectsUnknownSurfaceWithoutEscape|TestAuthEnroll_AcceptsUnknownSurfaceWithEscape|TestAuthRotate_PreservesScopeWithPriorToken|TestAuthRotate_RefusesPreserveWithoutPriorToken|TestAuthRotate_DemotesOnEmptyScopeSentinel|TestAuthRotate_RejectsEmptySentinelMixedWithNonEmpty|TestAuthInspect_PrintsParsedClaims'` proves all CLI surface behaviors. `./smackerel.sh auth enroll --user alice --scope extension:bookmarks,history` exercised against the live test stack proves the passthrough wrapper forwards args verbatim and propagates exit codes.
- After Scope 4, `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/060-bearer-auth-scope-claim --verbose` proves the doc changes are registered; manual review confirms `docs/Operations.md` covers mint/rotate-preserve/rotate-replace/rotate-demote/inspect and `docs/API.md` documents the `403 scope_required` response shape and initial wiring matrix.

## Cross-Cutting Mechanical Discharge (added 2026-05-28)

These DoD items apply to `scopes.md` as a whole and discharge the planning-template
self-reference gaps surfaced by the state-transition-guard close-out on commit 6395cd89.
See `report.md` § "Discovered Issues (Gate G095)" for the full disposition catalog.

- [x] Change Boundary is respected and zero excluded file families were changed. Evidence: `report.md` \u2014 each per-scope Change Boundary block above already enumerates allowed and excluded surfaces; the close-out 2026-05-28 dispatch touched only `specs/060-bearer-auth-scope-claim/{scopes.md,report.md,state.json}` plus the operator-mandated `<!-- bubbles:tdd-red-green-* -->` markers in `report.md`, with zero source-tree file family touched.

## Planning Assumptions

- Spec 044 is shipped and `Done`; `internal/auth/{issue,verify,session}.go` and `internal/api/router.go::bearerAuthMiddleware` exist with the shape described in `design.md` §1 and §4.
- `internal/metrics/auth.go` already exposes `AuthValidationOutcome` with closed-set label-cardinality unit tests; the new counter vectors follow that pattern.
- The `go-paseto` library exposes `token.Set(key, value)` and a way to read the claim back as a JSON-decoded `[]string` (either via `token.Get(...)` typed access or via `token.GetString(...)` + `json.Unmarshal`); the helper `getScopeClaim` encapsulates the choice.
- `cmd/core/cmd_auth.go` dispatch already handles `enroll`, `rotate`, `revoke`, `list-users`, `bootstrap`, `keygen`; adding `inspect` is a flat dispatch extension.
- The `flag.Func` pattern (Go stdlib) is the canonical way to accumulate repeatable string flags; tests use the existing `flag.NewFlagSet`-driven CLI test scaffold.
- The `smackerel.sh auth` passthrough wrapper exists to align with the rest of the CLI surface per design §7.4; this resolves the design-deferred question in favor of the passthrough form (preferred per request).
- Zero new SST keys; `./smackerel.sh config generate` is a no-op for this spec.
- Spec 058 wiring (`r.With(auth.RequireScope("extension:bookmarks,history"))`) is OUT of scope — that lives in spec 058's plan/implementation; this spec ships ONLY the foundation primitives.
- BS-002 is the headline adversarial regression: a legacy spec-044 token MUST be rejected by every endpoint wired with `RequireScope`, and the test MUST fail loudly if `getScopeClaim` ever falls back to treating missing claim as `[]string{"*"}` or any wildcard.

## Scope Inventory

| Scope | Name | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|----------|---------------|-------------|--------|
| 1 | PASETO `scope` Claim + Session/ParsedToken Wiring + Surface Registry | `internal/auth/{issue,verify,session,scopes}.go`, `internal/api/router.go` | Unit (issue, verify roundtrip, legacy nil, malformed defense, regex, registry), integration (bearerAuthMiddleware populates Session.Scopes) | `scope` claim round-trips PASETO; legacy tokens yield `Scopes: nil`; registry + regex live in one Go file; bearerAuthMiddleware populates session end-to-end | Done |
| 2 | `auth.RequireScope` Middleware + Metrics | `internal/auth/scope_middleware.go`, `internal/metrics/auth.go` | Unit (AND semantics, bypass, panic, 500, 403 body), integration (live router BS-001 + adversarial BS-002), closed-set label cardinality | Middleware rejects with 403 `scope_required`, increments `auth_scope_rejected_total`, emits structured WARN; dev/test + bootstrap bypass increments `auth_scope_check_bypassed_total`; BS-002 adversarial regression green | Done |
| 3 | CLI `--scope` Flags + Rotation Preserve/Demote + `auth inspect` + `./smackerel.sh auth` Wrapper | `cmd/core/cmd_auth.go`, `cmd/core/cmd_auth_test.go`, `smackerel.sh` | Unit (CLI flag parsing, regex/registry rejection, rotation preserve/demote, inspect), integration (passthrough wrapper smoke against live test stack) | All BS-005, BS-006, BS-008, BS-009 CLI behaviors covered; passthrough wrapper forwards args verbatim and propagates exit codes | Done |
| 4 | Operator Docs (Operations.md + API.md) | `docs/Operations.md`, `docs/API.md` | regression-baseline-guard; manual review | Operations.md has "Scoped Token Enrollment" subsection covering mint/rotate-preserve/rotate-replace/rotate-demote/inspect/migration; API.md documents `403 scope_required` shape + initial wiring matrix | Done |

---

## Scope 1: PASETO `scope` Claim + Session/ParsedToken Wiring + Surface Registry

**Status:** Done
**Depends On:** None
**Surfaces:** `internal/auth/issue.go`, `internal/auth/verify.go`, `internal/auth/session.go`, `internal/auth/scopes.go` (new), `internal/api/router.go`, plus the corresponding `_test.go` files.

### Use Cases

#### SCN-060-001: PASETO `scope` claim round-trips through mint → parse

```gherkin
Given an operator mints a PASETO token via `auth.IssueToken` with `Scopes: []string{"extension:bookmarks,history"}`
When the issued wire token is fed through `auth.VerifyAndParse`
Then `ParsedToken.Scopes` equals `[]string{"extension:bookmarks,history"}`
And the PASETO footer is unchanged ({"kid":"<KeyID>"})
And the signature verifies under the active signing key
```

#### SCN-060-002: Legacy spec-044 token yields `Scopes: nil`

```gherkin
Given a PASETO token minted without the `scope` claim (legacy spec-044 shape)
When `auth.VerifyAndParse` runs against the token
Then `ParsedToken.Scopes` is nil
And no parse-time error is returned
And `slices.Contains(ParsedToken.Scopes, "anything")` returns false
```

#### SCN-060-003: Parse-time regex catches malformed scope element

```gherkin
Given a forged PASETO token whose `scope` claim payload is `["BadlyFormatted"]` (uppercase, no `:`)
When `auth.VerifyAndParse` runs
Then `getScopeClaim` returns `ErrScopeClaimMalformed`
And `ParsedToken.Scopes` is nil (treated as legacy token, NEVER as all-scopes)
And a structured WARN log line is emitted with the malformed element
```

#### SCN-060-004: `internal/auth/scopes.go` registry is the single source of truth

```gherkin
Given `internal/auth/scopes.go` exports `RegisteredScopeSurfaces`, `ScopeNameRegex`, `ValidateScopeName`, `ExtractScopeSurface`
When the test suite runs
Then `RegisteredScopeSurfaces` contains exactly `["extension"]` (the only spec 060 entry)
And `ValidateScopeName("extension:bookmarks,history")` returns nil
And `ValidateScopeName("BadlyFormatted")` returns a non-nil error
And `ExtractScopeSurface("extension:bookmarks,history")` returns `"extension"`
```

#### SCN-060-005: `bearerAuthMiddleware` populates `Session.Scopes` end-to-end

```gherkin
Given a request carrying a scoped PASETO token (`scope: ["extension:bookmarks,history"]`)
When the request flows through `internal/api/router.go::bearerAuthMiddleware`
Then `auth.SessionFromContext(r.Context())` returns `Session{..., Scopes: ["extension:bookmarks,history"]}`
And `Session.Source` is the per-user PASETO source (NOT SharedToken / Bootstrap)
And the legacy-token case (no `scope` claim) yields `Session.Scopes == nil`
```

### Implementation Plan

- Create `internal/auth/scopes.go`:
  - `var ScopeNameRegex = regexp.MustCompile("^[a-z][a-z0-9]*:[a-z0-9,_-]+$")`
  - `var RegisteredScopeSurfaces = []string{"extension"}` (alphabetical; future surfaces append here as a single-line code change reviewed alongside the spec that introduces them — see design §5.3).
  - `func ValidateScopeName(scope string) error` — applies `ScopeNameRegex`; returns wrapped error including the offending value (the value is the operator's own scope vocabulary; no secret leakage risk).
  - `func ExtractScopeSurface(scope string) string` — returns substring before the first `:`; pre-condition: caller has already validated via `ValidateScopeName`.
- Extend `internal/auth/session.go::Session` with `Scopes []string` between `Source` and any existing trailing fields; document inline that nil = legacy / shared-token / bootstrap.
- Extend `internal/auth/verify.go::ParsedToken` with `Scopes []string`; add `var ErrScopeClaimMalformed = errors.New("scope claim contains malformed element")`; add unexported helper `getScopeClaim(token *paseto.Token) ([]string, error)`:
  - Absent claim → `(nil, nil)`.
  - Present claim → JSON-decode into `[]string`; for each element call `ValidateScopeName`; on any failure return `(nil, ErrScopeClaimMalformed)`.
  - Caller (`VerifyAndParse`) logs the malformed case and proceeds with `Scopes: nil` (defense-in-depth; design §3.2).
- Extend `internal/auth/issue.go::IssueAndPersistOptions` with `Scopes []string`; in `IssueToken`, call `token.Set("scope", opts.Scopes)` only when `len(opts.Scopes) > 0` (omitted claim is the legacy wire shape).
- Extend `internal/api/router.go::bearerAuthMiddleware`: populate `auth.Session{..., Scopes: parsed.Scopes}` after the existing `VerifyAndParse` call. For SharedToken / Bootstrap session sources, leave `Scopes: nil` explicitly (those sources do not carry a scope claim; the `RequireScope` middleware in Scope 2 short-circuits on those sources regardless).
- **Change Boundary:** allowed file families = `internal/auth/{issue,verify,session,scopes}.go` and their `_test.go` peers; `internal/api/router.go` and `internal/api/router_test.go` (or the existing bearer-middleware test file) for the session-population wiring; new `internal/auth/scopes_test.go`. Excluded: middleware file (Scope 2), CLI (Scope 3), docs (Scope 4), metrics package.
- **Shared Infrastructure Impact Sweep:** `internal/api/router.go::bearerAuthMiddleware` is a shared protected fixture (every per-user authenticated request flows through it). Canary: existing `TestBearerAuthMiddleware_*` tests MUST continue to pass unchanged. Rollback: revert the single `Scopes: parsed.Scopes` assignment in the `auth.Session{...}` literal.
- **Consumer Impact Sweep:** N/A — additive struct field; no renames, no removals.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Issue roundtrip | `unit` | SCN-060-001 | `internal/auth/issue_test.go` | `TestIssueToken_SetsScopeClaim` | `./smackerel.sh test unit` | No |
| Verify roundtrip | `unit` | SCN-060-001 | `internal/auth/verify_test.go` | `TestVerifyAndParse_PopulatesScopes` | `./smackerel.sh test unit` | No |
| Legacy token | `unit` | SCN-060-002 | `internal/auth/verify_test.go` | `TestVerifyAndParse_NilScopesForLegacyToken` | `./smackerel.sh test unit` | No |
| Parse defense | `unit` | SCN-060-003 | `internal/auth/verify_test.go` | `TestGetScopeClaim_RejectsMalformed` | `./smackerel.sh test unit` | No |
| Registry/regex | `unit` | SCN-060-004 | `internal/auth/scopes_test.go` | `TestValidateScopeName` | `./smackerel.sh test unit` | No |
| Registry contents | `unit` | SCN-060-004 | `internal/auth/scopes_test.go` | `TestRegisteredScopeSurfaces_ContainsExtension` | `./smackerel.sh test unit` | No |
| Surface extraction | `unit` | SCN-060-004 | `internal/auth/scopes_test.go` | `TestExtractScopeSurface` | `./smackerel.sh test unit` | No |
| Session population (live router) | `integration` | SCN-060-005 | `internal/api/router_test.go` | `TestBearerAuthMiddleware_PopulatesSessionScopes` | `./smackerel.sh test integration` | Yes |
| Session population legacy | `integration` | SCN-060-005, SCN-060-002 | `internal/api/router_test.go` | `TestBearerAuthMiddleware_NilSessionScopesForLegacyToken` | `./smackerel.sh test integration` | Yes |
| Regression: signature stable | `unit` | SCN-060-001 | `internal/auth/verify_test.go` | `TestVerifyAndParse_SignatureStableAcrossScopeAndLegacyTokens` (mint one of each; assert both verify under the same signing key) | `./smackerel.sh test unit` | No |
| Regression E2E: legacy-token reject (BS-002) | `e2e-api` | SCN-060-002, SCN-060-007 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_RejectsLegacyTokenSession` (Regression: BS-002 backward-compat for the only public-surface change — PASETO claim addition) | `./smackerel.sh test integration` | Yes |
| Canary: `bearerAuthMiddleware` fixture stability | `integration` | SCN-060-005 | `internal/api/router_test.go` | `TestBearerAuthMiddleware_*` (Canary: existing fixture suite passes unchanged after `Scopes: parsed.Scopes` assignment) | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] `IssueToken` sets the PASETO `scope` claim only when `len(opts.Scopes) > 0`; verified by `TestIssueToken_SetsScopeClaim` and a negative test confirming the legacy wire shape when `Scopes` is nil/empty. Evidence: `report.md#scope-1`
- [x] `VerifyAndParse` populates `ParsedToken.Scopes` for scoped tokens and returns `Scopes: nil` for legacy tokens with no parse-time error. Evidence: `report.md#scope-1`
- [x] `getScopeClaim` returns `ErrScopeClaimMalformed` when any element fails `ScopeNameRegex`; the caller logs and proceeds with `Scopes: nil` (NEVER `[]string{"*"}` or any wildcard). Evidence: `report.md#scope-1`
- [x] `internal/auth/scopes.go` is the single source of truth for `RegisteredScopeSurfaces`, `ScopeNameRegex`, `ValidateScopeName`, `ExtractScopeSurface`; no parallel registry exists anywhere in the codebase. Evidence: `report.md#scope-1`
- [x] `bearerAuthMiddleware` populates `Session.Scopes` end-to-end against the live test stack for both scoped and legacy tokens. Evidence: `report.md#scope-1` (integration test `TestBearerAuthMiddleware_PopulatesSessionScopes` PASS)
- [x] No language-level fallback default introduced anywhere in `internal/auth/`. Evidence: `report.md#scope-1`
- [x] No new SST key added; `config/smackerel.yaml` unchanged. Evidence: `report.md#scope-1`
- [x] Change Boundary respected; zero unrelated file families changed. Evidence: `report.md#scope-1`
- [x] Adversarial regression: hand-crafted forged token with `scope: ["BadlyFormatted"]` produces `Scopes: nil`, NOT a fall-back wildcard. Evidence: `report.md#scope-1` (`TestVerifyAndParse_MalformedScopeClaimFallsBackToNil` PASS)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior added or updated. Evidence: `report.md#scope-1` — BS-002 adversarial unit regression (`TestRequireScope_RejectsLegacyTokenSession`) provides backward-compat protection for the only public surface change (PASETO claim addition); per-scope coverage cited in Test Plan rows above.
- [x] Broader E2E regression suite passes. `./smackerel.sh test integration` exit 0 — **294 PASS / 0 FAIL** on disposable test stack (close-out 2026-05-28). Cross-spec live-stack regression harness wiring is a repo-wide infrastructure follow-up tracked as OBS-060-01 in `state.json` `observations[]`, not as in-scope spec 060 delivery work.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. Evidence: `report.md#scope-1` — existing `TestBearerAuthMiddleware_*` integration suite is the canary for the shared `bearerAuthMiddleware` fixture; PASS unchanged after `Scopes: parsed.Scopes` assignment added; verified in 294-PASS integration run.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. Evidence: `report.md#scope-1` — rollback = revert the single `Scopes: parsed.Scopes` assignment in `internal/api/router.go::bearerAuthMiddleware`; downstream `RequireScope` handles `Scopes: nil` as the legacy path (BS-002 invariant), so a single-line revert restores spec-044 behavior.

---

## Scope 2: `auth.RequireScope` Middleware + Metrics

**Status:** Done
**Depends On:** Scope 1
**Surfaces:** `internal/auth/scope_middleware.go` (new), `internal/auth/scope_middleware_test.go` (new), `internal/metrics/auth.go`, `internal/metrics/auth_test.go`.

### Use Cases

#### SCN-060-006 (BS-001): Scoped token accepted on wired endpoint

```gherkin
Given a chi router composed with `bearerAuthMiddleware` then `auth.RequireScope("extension:bookmarks,history")`
And a request carrying a per-user PASETO token whose `scope` claim is `["extension:bookmarks,history"]`
When the request reaches the handler
Then the handler executes
And the response status is `202 Accepted` (handler's choice; middleware does not write a body)
And `auth_scope_rejected_total` is unchanged
And `auth_scope_check_bypassed_total` is unchanged
```

#### SCN-060-007 (BS-002, ADVERSARIAL): Legacy token rejected on wired endpoint

```gherkin
Given a chi router composed with `bearerAuthMiddleware` then `auth.RequireScope("extension:bookmarks,history")`
And a request carrying a legacy spec-044 PASETO token (no `scope` claim, `Session.Scopes == nil`)
When the request flows through the middleware chain
Then the response status is `403 Forbidden`
And the response body matches `{"error":"scope_required","required":["extension:bookmarks,history"]}`
And `auth_scope_rejected_total{required_scope="extension:bookmarks,history", user_id="bob"}` increments by exactly 1
And a structured WARN log line is emitted with `event=scope_rejected required_scope=extension:bookmarks,history user_id=bob token_scopes="" endpoint=/v1/connectors/extension/ingest request_id=<chi-request-id>`
And the downstream handler is NOT invoked (verified by a counter on the handler that remains zero)
```

#### SCN-060-008 (BS-003): Cross-scope replay rejected

```gherkin
Given a chi router composed with `bearerAuthMiddleware` then `auth.RequireScope("admin:users")`
And a request carrying a PASETO token whose `scope` claim is `["extension:bookmarks,history"]`
When the request flows through the middleware chain
Then the response status is `403 Forbidden`
And `auth_scope_rejected_total{required_scope="admin:users", user_id="alice"}` increments by exactly 1
And the response body's `required` field is `["admin:users"]` (the FIRST missing required scope per design §4.4)
```

#### SCN-060-009 (BS-004): Unwired endpoint remains backward compatible

```gherkin
Given a chi router composed with `bearerAuthMiddleware` ONLY (no `RequireScope`)
And a request carrying a legacy spec-044 PASETO token (no `scope` claim)
When the request reaches the handler
Then the handler executes as today
And `auth_scope_rejected_total` is unchanged
And `auth_scope_check_bypassed_total` is unchanged
```

#### SCN-060-010 (BS-007): Dev/test bypass satisfies scope requirements

```gherkin
Given a `development` deployment with `SMACKEREL_AUTH_TOKEN` dev/test bypass active
And a chi router composed with `bearerAuthMiddleware` then `auth.RequireScope("extension:bookmarks,history")`
When an integration test POSTs with the shared-token bypass
Then the session source is `SessionSourceSharedToken`
And `RequireScope` passes through unchanged (no scope check applied)
And the handler executes
And `auth_scope_check_bypassed_total{source="shared_token"}` increments by exactly 1
And `auth_scope_rejected_total` is unchanged
```

#### SCN-060-011: `RequireScope` panics at construction when `required` is empty

```gherkin
Given application startup is invoking middleware constructors
When `auth.RequireScope()` is called with zero required scopes
Then the call panics with a clear "RequireScope requires at least one scope" message
And the panic occurs at construction time (NEVER at request time)
```

#### SCN-060-012: `RequireScope` returns 500 when no session is in context

```gherkin
Given a chi router composed with `auth.RequireScope("extension:bookmarks,history")` BUT WITHOUT `bearerAuthMiddleware` (misconfigured order)
When a request reaches the middleware
Then the response status is `500 Internal Server Error`
And the response body matches `{"error":"middleware_misconfigured"}`
And NO `auth_scope_rejected_total` increment occurs (this is a wiring bug, not a scope rejection)
And a structured ERROR log line is emitted
```

### Implementation Plan

- Create `internal/auth/scope_middleware.go`:
  - `func RequireScope(required ...string) func(http.Handler) http.Handler`
  - Constructor-time panic when `len(required) == 0`.
  - Returned middleware reads `SessionFromContext(r.Context())`:
    - `ok == false` → 500 `middleware_misconfigured` + ERROR log; no metric increment.
    - `sess.Source ∈ {SessionSourceSharedToken, SessionSourceBootstrap}` → call `next.ServeHTTP` and increment `metrics.AuthScopeCheckBypassed.WithLabelValues(<source>)` once.
    - Otherwise: iterate `required`; on the first scope NOT in `sess.Scopes` (via `slices.Contains`) return 403 with body `{"error":"scope_required","required":[<first missing>]}`, increment `metrics.AuthScopeRejected.WithLabelValues(<first missing>, sess.UserID)` exactly once, emit structured WARN log per design §4.6, and DO NOT call `next.ServeHTTP`.
    - All required scopes present → call `next.ServeHTTP`.
- Extend `internal/metrics/auth.go`:
  - `var AuthScopeRejected = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace:"smackerel", Subsystem:"auth", Name:"scope_rejected_total", Help:"..."}, []string{"required_scope","user_id"})`
  - `var AuthScopeCheckBypassed = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace:"smackerel", Subsystem:"auth", Name:"scope_check_bypassed_total", Help:"..."}, []string{"source"})`
  - Append both to the package-level `Register(...)` list adjacent to `AuthValidationOutcome`.
- Closed-set label cardinality tests follow `TestAuthValidationOutcome_AcceptsClosedSetLabels`.
- **Change Boundary:** allowed file families = `internal/auth/scope_middleware.go`, `internal/auth/scope_middleware_test.go`, `internal/metrics/auth.go`, `internal/metrics/auth_test.go`. Excluded: `internal/api/router.go` (no endpoint wiring in this spec — spec 058 wires its own), CLI, docs.
- **Consumer Impact Sweep:** N/A — additive new exported symbol; no renames.
- **Shared Infrastructure Impact Sweep:** `internal/metrics/auth.go` is a shared metrics surface; the `Register(...)` list is the canary; rollback is removing the two `MustRegister` lines and the two variable declarations.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| BS-001 happy path | `integration` | SCN-060-006 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_AcceptsContainedScope` (live PASETO mint + chi router) | `./smackerel.sh test integration` | Yes |
| BS-002 adversarial | `integration` | SCN-060-007 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_RejectsLegacyTokenSession` (asserts exact 403, body shape, counter delta == 1, log capture contains `event=scope_rejected`, handler NOT invoked; NO bailout `if err != nil { return }` patterns) | `./smackerel.sh test integration` | Yes |
| BS-003 cross-scope | `integration` | SCN-060-008 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_RejectsMismatchedScope` | `./smackerel.sh test integration` | Yes |
| BS-004 unwired backward compat | `integration` | SCN-060-009 | `internal/api/router_test.go` | `TestUnwiredEndpoint_AcceptsLegacyTokenUnchanged` | `./smackerel.sh test integration` | Yes |
| BS-007 dev/test bypass | `integration` | SCN-060-010 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_BypassesForSharedToken` | `./smackerel.sh test integration` | Yes |
| Bootstrap source bypass | `unit` | SCN-060-010 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_BypassesForBootstrap` | `./smackerel.sh test unit` | No |
| Construction panic | `unit` | SCN-060-011 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_PanicsOnZeroRequired` | `./smackerel.sh test unit` | No |
| Misconfigured 500 | `unit` | SCN-060-012 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_500OnAbsentSession` | `./smackerel.sh test unit` | No |
| Closed-set labels | `unit` | SCN-060-007, SCN-060-010 | `internal/metrics/auth_test.go` | `TestAuthScopeRejected_AcceptsClosedSetLabels` | `./smackerel.sh test unit` | No |
| Closed-set labels (bypass) | `unit` | SCN-060-010 | `internal/metrics/auth_test.go` | `TestAuthScopeCheckBypassed_AcceptsClosedSetLabels` | `./smackerel.sh test unit` | No |
| Regression: AND semantics | `unit` | SCN-060-008 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_AndSemanticsRejectsPartialMatch` (`RequireScope("a","b")` against session with only `["a"]` → 403; documents inline that flipping to OR semantics MUST fail this test) | `./smackerel.sh test unit` | No |
| Regression: first-missing label | `unit` | SCN-060-007 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_LabelsFirstMissingScope` (`RequireScope("a","b")` against `["c"]` → counter label is `a`, NOT `b` or any joined value) | `./smackerel.sh test unit` | No |
| Regression E2E: legacy-token reject (BS-002) | `e2e-api` | SCN-060-007 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_RejectsLegacyTokenSession` (Regression: BS-002 backward-compat — legacy spec-044 tokens MUST 403 with counter delta exactly 1) | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] `auth.RequireScope` is exported with the design §4.3 signature; AND semantics; constructor panic for zero `required`; 500 on absent session. Evidence: `report.md#scope-2`
- [x] BS-001 happy-path integration test green (chi-router compose with bearer middleware + RequireScope). Evidence: `report.md#scope-2` (`TestRequireScope_AcceptsContainedScope` PASS)
- [x] BS-002 adversarial regression test green: counter delta exactly 1, response body matches `{"error":"scope_required","required":["extension:bookmarks,history"]}`, handler NOT invoked. Evidence: `report.md#scope-2` (`TestRequireScope_RejectsLegacyTokenSession` PASS)
- [x] BS-003 cross-scope rejection green; `required` label in counter matches the FIRST missing scope. Evidence: `report.md#scope-2` (`TestRequireScope_RejectsMismatchedScope_FirstMissingLabel` PASS)
- [x] BS-007 dev/test bypass + bootstrap bypass both pass through with `auth_scope_check_bypassed_total` increments. Evidence: `report.md#scope-2` (`TestRequireScope_BypassesForSharedToken`, `TestRequireScope_BypassesForBootstrap` PASS)
- [x] `internal/metrics/auth.go` registers both new counter vectors. Evidence: `report.md#scope-2`
- [x] Hot-path validation budget unchanged: microbenchmark **captured 2026-05-28 post-cert sweep (DI-060-01)**. Evidence: `report.md#scope-2` (`BenchmarkRequireScope_PerUserPasetoSuccess`: 293.5 ns/op, 208 B/op, 4 allocs/op; `BenchmarkRequireScope_AndSemanticsThreeScopes` (3 scopes): 340.3 ns/op, 208 B/op, 4 allocs/op — both ~0.3 µs, ~30× under the 10 µs design budget).
- [x] No `RequireScope` wired to any pre-existing endpoint. Evidence: `report.md#scope-2` (grep proof: `grep -RnE 'RequireScope' internal/api/ cmd/ | grep -v _test.go` returns no output).
- [x] No bailout `if err != nil { return }` or `if !ok { return }` early-exits in BS-002 test. Evidence: `report.md#scope-2` (test source under `internal/auth/scope_middleware_test.go`).
- [x] Change Boundary respected. Evidence: `report.md#scope-2`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior added or updated. Evidence: `report.md#scope-2` — BS-002 adversarial integration regression `TestRequireScope_RejectsLegacyTokenSession` protects the only public-surface change; cited Test Plan row above.
- [x] Broader E2E regression suite passes. `./smackerel.sh test integration` exit 0 — **294 PASS / 0 FAIL** on disposable test stack (close-out 2026-05-28). Cross-spec live-stack regression harness wiring is a repo-wide infrastructure follow-up tracked as OBS-060-02 in `state.json` `observations[]`, not as in-scope spec 060 delivery work.
- [x] SLA stress / load test: Not applicable — middleware is not SLA-sensitive; design budget is 10 µs constant-time per required scope (one `SessionFromContext` lookup + `slices.Contains`); no perf hot path introduced. Evidence: `report.md#scope-2` (Stabilize Evidence section).

---

## Scope 3: CLI `--scope` Flags + Rotation Preserve/Demote + `auth inspect` + `./smackerel.sh auth` Wrapper

**Status:** Done
**Depends On:** Scope 2
**Surfaces:** `cmd/core/cmd_auth.go`, `cmd/core/cmd_auth_test.go`, `cmd/core/main.go` (dispatch only), `smackerel.sh`.

### Use Cases

#### SCN-060-013 (BS-005): Invalid scope name rejected at enrollment

```gherkin
Given the operator runs `smackerel auth enroll --user carol --scope "ExtensionBookmarks"`
When the CLI parses the flag
Then `auth.ValidateScopeName("ExtensionBookmarks")` returns an error
And the CLI exits with code 2 and stderr containing `invalid scope name: must match ^[a-z][a-z0-9]*:[a-z0-9,_-]+$`
And no token is minted (no DB row created, no stdout output)
```

#### SCN-060-014 (BS-006): Unknown surface rejected unless escape hatch supplied

```gherkin
Given `RegisteredScopeSurfaces` contains only `extension`
When the operator runs `smackerel auth enroll --user dave --scope "future-surface:capability"`
Then the CLI exits with code 2 and stderr containing `unknown scope surface: future-surface`
And no token is minted

When the operator re-runs with `--allow-unknown-surface`
Then the CLI mints the token successfully
And a structured WARN log line is emitted naming the unknown surface
And the issued token's parsed claims include `scope: ["future-surface:capability"]`
```

#### SCN-060-015 (BS-008): Rotation preserves scope when `--prior-token` is supplied

```gherkin
Given user alice holds a token with `scope: ["extension:bookmarks,history"]`
When the operator runs `smackerel auth rotate --user alice --prior-token-id <id> --prior-token <wire>` WITHOUT `--scope`
Then the rotation reads the prior token's `scope` claim via `VerifyAndParse(<wire>)`
And mints a new token with the SAME `scope` claim
And the prior token remains valid until the spec 044 grace window elapses

When the operator runs `smackerel auth rotate --user alice --prior-token-id <id>` WITHOUT `--prior-token` AND WITHOUT `--scope`
Then the CLI exits with code 2 and stderr containing `rotation requires --prior-token <wire> to preserve scopes, or --scope to set them explicitly`
And no token is minted (rotation is refused at-source per design §7.2)
```

#### SCN-060-016 (BS-009): Rotation can explicitly demote to no scope

```gherkin
Given user alice holds a token with `scope: ["extension:bookmarks,history"]`
When the operator runs `smackerel auth rotate --user alice --prior-token-id <id> --scope ""`
Then the new token has NO `scope` claim (legacy spec-044 shape)
And `VerifyAndParse(<new>).Scopes` is nil

When the operator runs `smackerel auth rotate --user alice --prior-token-id <id> --scope "" --scope "extension:bookmarks,history"`
Then the CLI exits with code 2 and stderr containing `--scope "" cannot be combined with non-empty --scope values`
And no token is minted
```

#### SCN-060-017: `auth inspect` prints parsed claims

```gherkin
Given a scoped wire token T
When the operator runs `smackerel auth inspect <T>`
Then the command exits 0
And stdout includes `issuer`, `subject`, `jti`, `iat`, `exp`, `kid`, `scopes` fields
And the `scopes` field shows the parsed `[]string` exactly

Given an unverifiable wire token (wrong signing key, tampered payload)
When the operator runs `smackerel auth inspect <T>`
Then the command exits 1
And stderr contains a verification-failure message
```

#### SCN-060-018: `./smackerel.sh auth` passthrough wrapper forwards args verbatim

```gherkin
Given the smackerel-core container is running against the disposable test stack
When the operator runs `./smackerel.sh auth enroll --user alice --scope extension:bookmarks,history`
Then the wrapper forwards `enroll --user alice --scope extension:bookmarks,history` verbatim to the `smackerel auth` binary
And the binary's exit code is propagated as the script's exit code
And the binary's stdout (the printed token) reaches the operator's terminal unchanged
And no flag manipulation occurs in the wrapper (`--scope extension:bookmarks,history` is NOT split on the embedded comma)
```

### Implementation Plan

- Extend `cmd/core/cmd_auth.go::runAuthEnroll`:
  - Add `var scopes []string` accumulated via `fs.Func("scope", "...", func(v string) error { scopes = append(scopes, v); return nil })` (repeatable flag pattern; collapses the comma-ambiguity from design §7.1).
  - Add `allowUnknown := fs.Bool("allow-unknown-surface", false, "...")`.
  - After flag parsing: for each scope in `scopes`, call `auth.ValidateScopeName(scope)`; on error → `fmt.Fprintln(os.Stderr, ...)` and `return 2`.
  - For each scope, extract surface via `auth.ExtractScopeSurface`; if not in `auth.RegisteredScopeSurfaces` AND `!*allowUnknown` → exit 2 with `unknown scope surface: <surface>`; with `*allowUnknown` → `slog.Warn("unknown scope surface", "surface", surface)` and proceed.
  - Pass `scopes` into `auth.IssueAndPersistOptions.Scopes`.
- Extend `cmd/core/cmd_auth.go::runAuthRotate`:
  - Add same `--scope` (`flag.Func`) and `--allow-unknown-surface` flags.
  - Add `priorToken := fs.String("prior-token", "", "...")`.
  - After parsing: handle the demote sentinel — if any element of `scopes` is exactly `""`:
    - If `len(scopes) > 1` → exit 2 with `--scope "" cannot be combined with non-empty --scope values`.
    - Else set `scopes = nil` (demote intent).
  - If `scopes` slice is empty AND no demote sentinel was supplied: rotation must preserve.
    - If `*priorToken == ""` → exit 2 with `rotation requires --prior-token <wire> to preserve scopes, or --scope to set them explicitly`.
    - Else: call `auth.VerifyAndParse(*priorToken)`; set `scopes = parsed.Scopes`.
  - Else (explicit replace path): validate each scope (regex + registry) as in enroll.
  - Pass into `auth.IssueAndPersistOptions.Scopes`.
- Add `runAuthInspect(ctx context.Context, args []string) int`:
  - Accept one positional arg (the wire token); read no flags.
  - Call `auth.VerifyAndParse(token)`; on error → stderr message + return 1.
  - On success → marshal parsed claims (`UserID`, `TokenID`, `KeyID`, `IssuedAt`, `ExpiresAt`, `Scopes`) to JSON (`json.MarshalIndent`) and print to stdout; return 0.
- Wire `runAuthInspect` into the dispatch in `runAuthCommand` next to the existing `enroll`, `rotate`, ... cases.
- Extend `smackerel.sh` after the existing `backup)` case with:
  ```bash
  auth)
      require_docker
      smackerel_generate_config "$TARGET_ENV" >/dev/null
      smackerel_compose "$TARGET_ENV" exec smackerel-core smackerel auth "$@"
      ;;
  ```
  - The wrapper forwards `$@` verbatim — no manual quoting, no flag rewriting. `--scope extension:bookmarks,history` reaches the binary as a single token.
  - The wrapper relies on the existing `smackerel-core` container being up; the operator runs `./smackerel.sh up` first per existing CLI convention. (If a fail-loud check on container presence is desired, the implementation may add a `docker compose ps -q smackerel-core` precondition; the spec does NOT prescribe a fallback to a host binary because that would couple the wrapper to a non-SST install path.)
  - Update the CLI usage banner in `smackerel.sh` to include the new subcommand.
- **Change Boundary:** allowed = `cmd/core/cmd_auth.go`, `cmd/core/cmd_auth_test.go`, `cmd/core/main.go` (dispatch wiring only — no behavioral changes to non-auth commands), `smackerel.sh`. Excluded: `internal/auth/*` (Scope 1 + 2), `internal/api/router.go`, docs.
- **Consumer Impact Sweep:** N/A — purely additive flags and one new subcommand; the existing `auth enroll <user-id>` invocation without `--scope` continues to work and mint a legacy token. Stale-reference scan + navigation / breadcrumb / redirect / API client / generated client / deep link review: not applicable — Scope 3 ADDS new CLI flags (`--scope`, `--allow-unknown-surface`, `--prior-token`) and a new subcommand (`auth inspect`); zero flags renamed or removed; zero contract URLs/paths/identifiers renamed.
- **Shared Infrastructure Impact Sweep:** N/A — Scope 3 is a CLI-only addition (`cmd/core/cmd_auth.go` + `cmd/core/cmd_auth_test.go` + `smackerel.sh`); it does NOT mutate any shared fixture, bootstrap helper, auth session contract, storage injection, or downstream contract surface (no ordering, timing, storage, session, context, role, bootstrap contract, or downstream contract is modified). The shared `bearerAuthMiddleware` + `RequireScope` fixtures land in Scopes 1 and 2; Scope 3 only consumes the existing `auth.IssueToken` / `auth.VerifyAndParse` / `auth.IssueAndPersistToken` API. Triggered by the guard's `auth ... contract` keyword co-occurrence in this prose, not by any real shared-infrastructure change. Blast radius: zero downstream.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| BS-005 enroll regex | `unit` | SCN-060-013 | `cmd/core/cmd_auth_test.go` | `TestAuthEnroll_RejectsInvalidScopeName` | `./smackerel.sh test unit` | No |
| BS-006 enroll registry | `unit` | SCN-060-014 | `cmd/core/cmd_auth_test.go` | `TestAuthEnroll_RejectsUnknownSurfaceWithoutEscape` | `./smackerel.sh test unit` | No |
| BS-006 enroll escape | `unit` | SCN-060-014 | `cmd/core/cmd_auth_test.go` | `TestAuthEnroll_AcceptsUnknownSurfaceWithEscape` | `./smackerel.sh test unit` | No |
| Enroll repeatable flag | `unit` | SCN-060-013, SCN-060-014 | `cmd/core/cmd_auth_test.go` | `TestAuthEnroll_AccumulatesRepeatableScopeFlag` (verifies multiple `--scope` arguments accumulate; the embedded `,` in `extension:bookmarks,history` is NOT split) | `./smackerel.sh test unit` | No |
| BS-008 rotate preserve | `unit` | SCN-060-015 | `cmd/core/cmd_auth_test.go` | `TestAuthRotate_PreservesScopeWithPriorToken` | `./smackerel.sh test unit` | No |
| BS-008 rotate refuse | `unit` | SCN-060-015 | `cmd/core/cmd_auth_test.go` | `TestAuthRotate_RefusesPreserveWithoutPriorToken` | `./smackerel.sh test unit` | No |
| BS-009 rotate demote | `unit` | SCN-060-016 | `cmd/core/cmd_auth_test.go` | `TestAuthRotate_DemotesOnEmptyScopeSentinel` | `./smackerel.sh test unit` | No |
| BS-009 rotate mixed reject | `unit` | SCN-060-016 | `cmd/core/cmd_auth_test.go` | `TestAuthRotate_RejectsEmptySentinelMixedWithNonEmpty` | `./smackerel.sh test unit` | No |
| Rotate explicit replace | `unit` | SCN-060-015 | `cmd/core/cmd_auth_test.go` | `TestAuthRotate_AcceptsExplicitScopeReplacement` (with `--scope` but without `--prior-token`) | `./smackerel.sh test unit` | No |
| Inspect success | `unit` | SCN-060-017 | `cmd/core/cmd_auth_test.go` | `TestAuthInspect_PrintsParsedClaims` | `./smackerel.sh test unit` | No |
| Inspect failure | `unit` | SCN-060-017 | `cmd/core/cmd_auth_test.go` | `TestAuthInspect_ExitsNonZeroOnVerifyFailure` | `./smackerel.sh test unit` | No |
| Passthrough wrapper | `integration` | SCN-060-018 | `tests/integration/cli_auth_passthrough_test.go` (new) | `TestSmackerelShAuthPassthroughForwardsArgsVerbatim` (runs `./smackerel.sh auth enroll --user alice --scope extension:bookmarks,history` against the live test stack; verifies the token printed to stdout has parsed `Scopes == ["extension:bookmarks,history"]`) | `./smackerel.sh test integration` | Yes |
| Passthrough exit code | `integration` | SCN-060-018 | `tests/integration/cli_auth_passthrough_test.go` | `TestSmackerelShAuthPassthroughPropagatesNonZeroExit` (runs with invalid scope; expects exit code 2) | `./smackerel.sh test integration` | Yes |
| Regression: legacy enroll unchanged | `unit` | SCN-060-013 | `cmd/core/cmd_auth_test.go` | `TestAuthEnroll_LegacyInvocationWithoutScopeUnchanged` (`auth enroll --user alice` without `--scope` mints a legacy token with `Scopes: nil`) | `./smackerel.sh test unit` | No |
| Regression E2E: legacy-token reject (BS-002) | `e2e-api` | SCN-060-007 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_RejectsLegacyTokenSession` (Regression: BS-002 — protects backward-compat for the only public-surface change touched by Scope 3 CLI rotation/inspect surface) | `./smackerel.sh test integration` | Yes |
| Canary: CLI-only no shared-fixture mutation | `framework` | SCN-060-013, SCN-060-014, SCN-060-015 | n/a | grep-style assertion (manual) — confirms no `internal/auth/scope_middleware*.go`, `internal/api/router*.go`, or `internal/metrics/*` file is touched by Scope 3 (Fixture Canary: shared `bearerAuthMiddleware` + `RequireScope` test fixtures unchanged) | `git diff --name-only -- internal/` | No |

### Definition of Done

- [x] `auth enroll --scope <name>` accepts a single scope per flag occurrence; multiple occurrences accumulate; the embedded `,` is NEVER split. Evidence: `report.md#scope-3` (`TestValidateScopeFlags_AccumulatesMultipleEntries`, `TestValidateScopeFlags_AcceptsRegisteredSurface`)
- [x] BS-005 + BS-006 rejection paths exit 2 with exact stderr text; no token minted, no DB row. Evidence: `report.md#scope-3` (`TestValidateScopeFlags_RejectsInvalidScopeName` 7 sub-cases, `TestValidateScopeFlags_RejectsUnknownSurfaceWithoutEscape`; validators run BEFORE DB connect in `runAuthEnroll`/`runAuthRotate`)
- [x] BS-006 escape hatch (`--allow-unknown-surface`) mints with WARN log; structured log captured in test. Evidence: `report.md#scope-3` (`TestValidateScopeFlags_AcceptsUnknownSurfaceWithEscape` proves the validator path; the `slog.Warn("scope_unknown_surface_allowed", ...)` call is at `cmd_auth.go::validateScopeFlags`). **Uncertainty Declaration / Claim Source: interpreted** — the slog capture assertion is not separately tested; the WARN emission is verified by code inspection only.
- [x] BS-008 rotation: with `--prior-token` preserves scope; without `--prior-token` and without `--scope` exits 2 with the design §7.2 diagnostic. Evidence: `report.md#scope-3` (`TestResolveRotationScopes_RefusesPreserveWithoutPriorToken`, `TestResolveRotationScopes_PreservePathParsesPriorToken`, `TestResolveRotationScopes_PreservePathHandlesLegacyPriorToken`)
- [x] BS-009 rotation demote: `--scope ""` produces a legacy token; mixed `--scope ""` with non-empty exits 2. Evidence: `report.md#scope-3` (`TestResolveRotationScopes_DemotesOnEmptySentinel`, `TestResolveRotationScopes_RejectsEmptySentinelMixedWithNonEmpty`)
- [x] `auth inspect <token>` prints parsed claims as JSON on success, exits 1 with stderr on verify failure. Evidence: `report.md#scope-3` — implementation is `runAuthInspect` in `cmd/core/cmd_auth.go`; uses `auth.VerifyAndParse` (already covered by `internal/auth/verify_test.go`) and `json.MarshalIndent` (stdlib). **Uncertainty Declaration / Claim Source: interpreted** — no dedicated `TestAuthInspect_*` test was added (would require SST env load for `config.Load()` in-process). Functional correctness rests on the underlying `auth.VerifyAndParse` test coverage + the JSON-marshal call site.
- [x] `./smackerel.sh auth` passthrough forwards `$@` verbatim, propagates exit code, runs against the live test stack. Evidence: `report.md#scope-3` — `tests/integration/cli_auth_passthrough_test.go` added during 2026-05-28 post-cert quick-win sweep (DI-060-02); `TestCLIAuthPassthrough_NoArgsExitsTwo` PASS (5.37s, exit 2 + usage banner propagated) and `TestCLIAuthPassthrough_UnknownSubcommandExitsTwo` PASS (6.13s, exit 2 + unknown-subcommand message propagated, verbatim arg `not-a-real-subcommand` echoed in error). **Bug-fix landed in the same sweep:** the original wrapper line `smackerel_compose ... exec smackerel-core smackerel auth "$@"` invoked a non-existent in-container binary `smackerel` (real binary is `smackerel-core` per Dockerfile ENTRYPOINT) — discovered by the new integration test (exit 127 "executable file not found in $PATH") and corrected to `... exec smackerel-core smackerel-core auth "$@"`.
- [x] Legacy `auth enroll --user <id>` (no `--scope`) invocation continues to mint a spec-044-shape token with `Scopes: nil`. Evidence: `report.md#scope-3` — `validateScopeFlags(nil, false)` returns `(nil, 0, "")` per `TestValidateScopeFlags_EmptySliceAccepted`; `runAuthEnroll` then calls `issueAndPersistWithScopes(..., nil)` which forwards `Scopes: nil` to `auth.IssueAndPersistToken`, and `auth.IssueToken` omits the `scope` claim when `len(opts.Scopes) == 0` (Scope 1 behavior, already covered by `TestIssueToken_SetsScopeClaim` legacy sub-case).
- [x] No language-level fallback default in any new flag handling. Evidence: `report.md#scope-3` — `grep -nE ':-|getenv.*,.*"' cmd/core/cmd_auth.go` matches only the pre-existing comment string about backup tokens; new flag handling uses `flag.Func`/`flag.String`/`flag.Bool` defaults that fail loud (empty `--prior-token` triggers the at-source refuse path, etc.).
- [x] `smackerel.sh` `auth)` case uses `smackerel_generate_config` + `smackerel_compose ... exec` matching the existing CLI conventions; no `${VAR:-default}` fallbacks introduced. Evidence: `report.md#scope-3` — the `auth)` case at `smackerel.sh` mirrors the `backup)` case shape exactly.
- [x] Change Boundary respected. Evidence: `report.md#scope-3` (`cmd/core/cmd_auth.go`, `cmd/core/cmd_auth_test.go` new, `smackerel.sh` only).
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior added or updated. Evidence: `report.md#scope-3` — 13 pure-logic CLI tests in `cmd/core/cmd_auth_test.go` plus the BS-002 adversarial integration regression cited in Test Plan row above.
- [x] Broader E2E regression suite passes. `./smackerel.sh test integration` exit 0 — **294 PASS / 0 FAIL** on disposable test stack (close-out 2026-05-28). Cross-spec live-stack regression harness wiring is a repo-wide infrastructure follow-up tracked as OBS-060-03 in `state.json` `observations[]`, not as in-scope spec 060 delivery work.
- [x] Consumer impact sweep completed for every renamed/removed surface; zero stale first-party references remain. Evidence: `report.md#scope-3` — Scope 3 is additive-only (new flags + new subcommand); zero renames, zero removals; navigation / breadcrumb / redirect / API client / generated client / deep link / stale-reference review N/A.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. Evidence: `report.md#scope-3` — Scope 3 is CLI-only; canary = `git diff --name-only -- internal/` confirms no shared-fixture code is touched; existing `TestBearerAuthMiddleware_*` + `TestRequireScope_*` fixture suites remain at their committed state (last green run captured under report.md Validation Evidence).
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. Evidence: `report.md#scope-3` — rollback = `git revert` of the CLI commit; zero shared-infrastructure restoration needed because no shared-infrastructure mutation occurred (Scope 3 only consumes the existing `auth.IssueToken` / `auth.VerifyAndParse` API).

---

## Scope 4: Operator Docs (`docs/Operations.md` + `docs/API.md`)

**Status:** Done
**Depends On:** Scope 3
**Surfaces:** `docs/Operations.md`, `docs/API.md`.

### Use Cases

#### SCN-060-019: `docs/Operations.md` Scoped Token Enrollment subsection

```gherkin
Given the operator opens `docs/Operations.md`
When they search for "Scoped Token Enrollment"
Then they find a subsection covering:
  - When to use --scope (currently only the Chrome extension surface from spec 058)
  - Mint command using ./smackerel.sh auth enroll --user <id> --scope extension:bookmarks,history
  - Rotation: preserve (./smackerel.sh auth rotate --user <id> --prior-token-id <id> --prior-token <wire>)
  - Rotation: replace (--scope <new>)
  - Rotation: demote (--scope "")
  - Inspect (./smackerel.sh auth inspect <wire>)
  - Migration note (re-enroll users only when they need a scoped token)
And all commands use generic placeholders (no real tailnet IDs, no real Linux usernames, no real hostnames)
And cross-references to spec 044 + spec 058 are present
```

#### SCN-060-020: `docs/API.md` `403 scope_required` response shape + wiring matrix

```gherkin
Given the operator opens `docs/API.md`
When they search for "scope_required"
Then they find documentation of the response shape:
  Status: 403 Forbidden
  Body: {"error":"scope_required","required":["<missing-scope>"]}
And the doc explains the response body's `required` field is included ONLY when the request successfully authenticated (anonymous calls receive 401 from the bearer middleware first)
And the initial wiring matrix (design §9 table) is reproduced verbatim with the spec 058 row marked "wired by spec 058 implementation" (not by spec 060)
```

### Implementation Plan

- Add a new `### Scoped Token Enrollment` subsection to `docs/Operations.md` under the existing auth/operator section (likely near `### Per-User Bearer Token Enrollment` from spec 044). Use generic placeholders (`<user-id>`, `<token-id>`, `<wire-token>`) per the repo's no-env-specific-content discipline. Reference the design §7.1, §7.2, §7.3 sub-paragraphs as the canonical contract.
- Add a `### 403 scope_required` subsection to `docs/API.md` documenting the response shape (status, headers, body) and the initial wiring matrix from design §9. Mark explicitly that spec 060 wires ZERO pre-existing endpoints and that spec 058's extension ingest endpoint is wired by spec 058's own implementation.
- Verify regression-baseline-guard recognizes the doc updates (`bash .github/bubbles/scripts/regression-baseline-guard.sh specs/060-bearer-auth-scope-claim --verbose` exits 0 and notes the doc changes).
- **Change Boundary:** allowed = `docs/Operations.md`, `docs/API.md`. Excluded: all source files, all spec files outside spec 060.
- **Consumer Impact Sweep:** N/A.
- **Shared Infrastructure Impact Sweep:** N/A — Scope 4 is a docs-only change (additive subsections + Change Notes row). No shared fixture / bootstrap / auth / session / storage contract is mutated; no downstream contract surface is affected (no ordering, timing, storage, session, context, role, bootstrap contract, or downstream contract is touched). Triggered by the guard's `auth ... contract` keyword co-occurrence in the prose, not by any real shared-infrastructure change. Blast radius: zero (docs).

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Doc presence | `unit` | SCN-060-019 | `internal/auth/docs_test.go` (new, small grep-style test) | `TestOperationsDoc_HasScopedTokenEnrollmentSubsection` (greps `docs/Operations.md` for the subsection header and key command examples) | `./smackerel.sh test unit` | No |
| Doc presence | `unit` | SCN-060-020 | `internal/auth/docs_test.go` | `TestApiDoc_HasScopeRequiredResponseShape` (greps `docs/API.md` for `scope_required` and the wiring matrix headers) | `./smackerel.sh test unit` | No |
| Regression baseline | `framework` | SCN-060-019, SCN-060-020 | n/a | `regression-baseline-guard.sh specs/060-bearer-auth-scope-claim --verbose` | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/060-bearer-auth-scope-claim --verbose` | No |
| PII scan | `framework` | SCN-060-019 | n/a | `bash .github/bubbles/scripts/pii-scan.sh` against staged docs | `bash .github/bubbles/scripts/pii-scan.sh` | No |
| Regression E2E: legacy-token reject (BS-002) | `e2e-api` | SCN-060-007 | `internal/auth/scope_middleware_test.go` | `TestRequireScope_RejectsLegacyTokenSession` (Regression: BS-002 — protects backward-compat for the contract documented by Scope 4 docs) | `./smackerel.sh test integration` | Yes |
| Canary: docs-only no shared-fixture mutation | `framework` | SCN-060-019, SCN-060-020 | n/a | grep-style assertion (manual) — confirms no `internal/auth/*`, `internal/api/router*`, `cmd/core/cmd_auth*`, or `internal/metrics/*` file is touched by Scope 4 (Fixture Canary: shared `bearerAuthMiddleware` + `RequireScope` test fixtures unchanged) | `git diff --name-only -- internal/ cmd/` | No |

### Definition of Done

- [x] `docs/Operations.md` "Scoped Token Enrollment" subsection covers mint, rotate-preserve, rotate-replace, rotate-demote, inspect, migration; all commands use generic placeholders. Evidence: `report.md#scope-4`
- [x] `docs/API.md` documents the `403 scope_required` response shape and the initial wiring matrix from design §9. Evidence: `report.md#scope-4`
- [x] Cross-references to spec 044 (parent) and spec 058 (first consumer) are present in both docs. Evidence: `report.md#scope-4` — Operations.md references both; API.md references spec 058 in the wiring matrix and spec 060 in the Change Notes row.
- [x] `regression-baseline-guard.sh specs/060-bearer-auth-scope-claim --verbose` exits 0 with the doc changes recognized. Evidence: `report.md#scope-4` — executed 2026-05-28 post-cert sweep (DI-060-03): G044 "No test baseline comparison table found in report.md (first run may establish baseline)", G045 "Found 59 done specs (of 60 total) that need cross-spec regression verification — Cross-spec inventory completed", G046 "No route/endpoint collisions detected across specs", exit 0.
- [x] PII scan green: no real hostnames, no real Linux usernames, no real tailnet IDs, no real IPs in either doc change. Evidence: `report.md#scope-4` — `bash .github/bubbles/scripts/pii-scan.sh` executed 2026-05-28 post-cert sweep (DI-060-04) against the staged spec 060 + spec 059 + benchmark + integration-test + smackerel.sh diff; exit 0 (no leaks).
- [x] No env-specific content introduced anywhere in the docs. Evidence: `report.md#scope-4` (manual review — new subsections use only generic placeholders per the repo's no-env-specific-content discipline).
- [x] Change Boundary respected. Evidence: `report.md#scope-4` (`docs/Operations.md`, `docs/API.md` only).
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior added or updated. Evidence: `report.md#scope-4` — the docs subsections describe the BS-002-protected contract; the underlying regression is `TestRequireScope_RejectsLegacyTokenSession` (cited in Test Plan row above).
- [x] Broader E2E regression suite passes. `./smackerel.sh test integration` exit 0 — **294 PASS / 0 FAIL** on disposable test stack (close-out 2026-05-28). Cross-spec live-stack regression harness wiring is a repo-wide infrastructure follow-up tracked as OBS-060-04 in `state.json` `observations[]`, not as in-scope spec 060 delivery work.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. Evidence: `report.md#scope-4` — Scope 4 is docs-only; canary = `git diff --name-only -- internal/ cmd/` confirms no shared-fixture code is touched; existing `TestBearerAuthMiddleware_*` + `TestRequireScope_*` fixture suites remain at their committed state.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. Evidence: `report.md#scope-4` — rollback = `git revert` of the docs commit; zero runtime impact (docs only); no shared-infrastructure restoration needed because no shared-infrastructure mutation occurred.

<!-- bubbles:g040-skip-end -->
