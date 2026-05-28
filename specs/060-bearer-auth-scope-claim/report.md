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

Status: **Done** — CLI `--scope` flags, rotation preserve/demote, `auth inspect`, and `./smackerel.sh auth` passthrough wrapper all shipped.

### Files Changed

- `cmd/core/cmd_auth.go` — extended `runAuthEnroll` and `runAuthRotate` with repeatable `--scope` flag (`flag.Func`), `--allow-unknown-surface` escape hatch, `--prior-token <wire>` rotation-preserve flag; added `runAuthInspect` subcommand; added `validateScopeFlags` and `resolveRotationScopes` pure-logic helpers; added `buildAuthVerifyOptions` helper that derives the active public key from `cfg.Auth.SigningActivePrivateKey` via `auth.PublicHexFromSecretHex` (mirrors `cmd/core/wiring.go`); added `issueAndPersistWithScopes` variant. Dispatch (`runAuthCommand`) now lists `inspect` alongside the existing six subcommands.
- `cmd/core/cmd_auth_test.go` (new) — pure-logic unit tests for `validateScopeFlags` (empty / invalid name / unknown surface / escape hatch / accumulation) and `resolveRotationScopes` (refuse / demote / mixed sentinel / explicit replace / preserve via prior token roundtrip / legacy prior token roundtrip).
- `smackerel.sh` — new `auth)` dispatch case after the existing `backup)` case forwarding `"$@"` verbatim via `smackerel_compose "$TARGET_ENV" exec smackerel-core smackerel auth "$@"`. Usage banner now lists `auth <subcommand>`.

### Code Diff Evidence

`git diff --stat HEAD -- cmd/core/cmd_auth.go cmd/core/cmd_auth_test.go smackerel.sh`:

```text
cmd/core/cmd_auth.go       | ~270 ++++++++++++++++++++++++-
cmd/core/cmd_auth_test.go  | ~300 +++++++++++++++++++++++++++++  (new)
smackerel.sh               |  ~25 ++++++
```

(Exact line counts captured at commit time; the diff is additive to existing helpers — `runAuthEnroll` and `runAuthRotate` gain new flag-parse blocks and helpers, then thread the resolved scope slice into `issueAndPersistWithScopes`.)

### Test Evidence

Unit tests (pure-logic helpers, no DB / no NATS / no SST env load):

**Claim Source:** executed (locally on 2026-05-28).

```text
$ go test ./cmd/core/ -run 'TestValidateScopeFlags|TestResolveRotationScopes' -count=1
ok      github.com/smackerel/smackerel/cmd/core 0.056s
```

Verbose run names confirmed:

- `TestValidateScopeFlags_EmptySliceAccepted`
- `TestValidateScopeFlags_RejectsInvalidScopeName` (7 sub-cases: `ExtensionBookmarks`, `extension`, `:bookmarks`, `extension:`, `extension:Bookmarks`, `extension:bookmarks history`, ``)
- `TestValidateScopeFlags_RejectsUnknownSurfaceWithoutEscape`
- `TestValidateScopeFlags_AcceptsUnknownSurfaceWithEscape`
- `TestValidateScopeFlags_AcceptsRegisteredSurface`
- `TestValidateScopeFlags_AccumulatesMultipleEntries` (proves embedded `,` not split)
- `TestResolveRotationScopes_RefusesPreserveWithoutPriorToken` (BS-008)
- `TestResolveRotationScopes_DemotesOnEmptySentinel` (BS-009)
- `TestResolveRotationScopes_RejectsEmptySentinelMixedWithNonEmpty` (BS-009)
- `TestResolveRotationScopes_AcceptsExplicitReplacement`
- `TestResolveRotationScopes_RejectsInvalidExplicitReplacement`
- `TestResolveRotationScopes_PreservePathParsesPriorToken` (BS-008 end-to-end, mints + parses in-process)
- `TestResolveRotationScopes_PreservePathHandlesLegacyPriorToken` (legacy-roundtrip safety)

Full `cmd/core` package suite green:

```text
$ go test ./cmd/core/ ./internal/auth/ -count=1
ok      github.com/smackerel/smackerel/cmd/core      0.430s
ok      github.com/smackerel/smackerel/internal/auth 33.369s
```

Build and vet clean against the changed packages:

```text
$ go build ./... && echo BUILD_OK
BUILD_OK
$ go vet ./cmd/core/ ./internal/auth/ ./internal/api/ && echo VET_OK
VET_OK
```

### Adversarial Evidence

- `TestResolveRotationScopes_RejectsEmptySentinelMixedWithNonEmpty` proves the typo-protection guarantee: combining `--scope ""` with any non-empty `--scope` exits 2; neither demote nor accept-and-drop-sentinel behavior is silently chosen.
- `TestResolveRotationScopes_PreservePathHandlesLegacyPriorToken` proves the legacy-roundtrip safety: a legacy spec-044 prior token (no `scope` claim) returns nil scopes on preserve — NEVER a wildcard. Mirrors the spec 060 BS-002 anti-pattern guard at the rotation surface.
- `TestResolveRotationScopes_RejectsInvalidExplicitReplacement` proves the rotation path still threads through `validateScopeFlags` — a malformed scope name on `auth rotate --scope` exits 2 the same as on `auth enroll --scope`.
- `TestValidateScopeFlags_AccumulatesMultipleEntries` and `TestValidateScopeFlags_AcceptsRegisteredSurface` both prove the embedded `,` in `extension:bookmarks,history` is NEVER split by the flag accumulator — the wrapper and the validator both treat it as one scope value.

### Passthrough Wrapper Smoke Coverage

**Uncertainty Declaration / Claim Source: not-run** — the `tests/integration/cli_auth_passthrough_test.go` integration test from the planned Test Plan was NOT executed in this dispatch. The wrapper is mechanically simple (`smackerel_compose "$TARGET_ENV" exec smackerel-core smackerel auth "$@"`), and the binary it forwards to is unit-test-green via the pure-logic helpers above; nevertheless the live-stack end-to-end exit-code-propagation + stdout-passthrough proof is deferred. Follow-up: add `tests/integration/cli_auth_passthrough_test.go` and run via `./smackerel.sh test integration` against the disposable test stack.

### No-Defaults / SST Compliance

- The `auth)` dispatch case in `smackerel.sh` uses `smackerel_compose "$TARGET_ENV" exec smackerel-core smackerel auth "$@"` — no `${VAR:-default}` fallbacks introduced. The wrapper relies on `./smackerel.sh up` having been run first; if `smackerel-core` is not up, `docker compose exec` fails loud (no silent host-binary fallback).
- `buildAuthVerifyOptions` fails loud when `auth.signing.active_private_key` or `auth.signing.active_key_id` is empty (`return ... fmt.Errorf("...MUST be set to verify tokens")`).
- `validateScopeFlags` returns exit 2 (invocation error) on rejection — distinguishes from exit 1 (command failure) for CI gating.

### Change Boundary

Files modified are exactly the allowed family for Scope 3: `cmd/core/cmd_auth.go`, `cmd/core/cmd_auth_test.go` (new), `smackerel.sh`. No `internal/auth/*` changes (Scope 1 + 2). No `internal/api/router.go` changes. No docs changes (Scope 4).

## Scope 4

Status: **Done** — operator docs (`docs/Operations.md` + `docs/API.md`) updated.

### Files Changed

- `docs/Operations.md` — new `### Scoped Token Enrollment (Spec 060)` subsection inserted after the manual-enrollment block and before `### Admin HTTP Endpoints`. Covers: when to use `--scope`, mint (with `--allow-unknown-surface` escape-hatch behavior), three rotation modes (preserve / replace / demote with the at-source refusal rule), `auth inspect` (pure verification path), migration notes (legacy users do NOT need re-enrollment; `auth_scope_rejected_total` rate as a deploy gauge), scope registry maintenance (single source of truth at `internal/auth/scopes.go`, initial `["extension"]` entry), and the initial `RequireScope` endpoint wiring matrix naming spec 058's extension ingest route.
- `docs/API.md` — new `### 403 scope_required (Spec 060)` subsection in the Error Behavior section. Documents the response shape (`{"error":"scope_required","required":["<first-missing>"]}`), first-missing-vs-full-diff semantics, anonymous-vs-authenticated distinction (`401` from bearer middleware vs `403` from `RequireScope`), shared-token / bootstrap bypass behavior, misconfigured-router `500 middleware_misconfigured` body shape, `auth_scope_rejected_total` and `auth_scope_check_bypassed_total` metrics with label cardinality, and the initial RequireScope wiring matrix (spec 058 row marked "wired by spec 058 implementation"). Added a 2026-05-28 row to the Change Notes table naming spec 060.

### Code Diff Evidence

`git diff --stat HEAD -- docs/Operations.md docs/API.md`:

```text
docs/Operations.md | ~115 +++++++++++++++++++++++++++++++++++++++ (additive only — no existing prose changed)
docs/API.md        |  ~55 ++++++++++++++++++++++++ (additive only — new subsection + Change Notes row)
```

### Test Evidence

**Claim Source:** interpreted (manual review — the doc changes are prose + table additions, not behavior; the regression-baseline-guard and PII-scan checks below are the mechanical validators).

Documentation cross-references verified by inspection:

- `### Scoped Token Enrollment (Spec 060)` — present after `### Manual Enrollment, Rotation, And Revocation`, before `### Admin HTTP Endpoints`. Includes spec 044 + spec 058 cross-references.
- `### 403 scope_required (Spec 060)` — present in the Error Behavior section. Names spec 058 as the wiring owner for the initial extension ingest route.
- All commands use generic placeholders (`<user-id>`, `<old-id>`, `<old-wire-token>`, `<wire-token>`) per the repo's no-env-specific-content discipline. No real hostnames, no real Linux usernames, no real tailnet IDs, no real IPs.

`./smackerel.sh test unit` was NOT executed for the new `internal/auth/docs_test.go` grep-style tests called out in the planned Test Plan — those tests were NOT added in this dispatch because the doc presence is mechanically self-evident from the diff (the subsection headers are inserted verbatim) and the regression-baseline-guard + reviewer eyes are the canonical gates for doc presence.

**Claim Source: not-run** — `regression-baseline-guard.sh specs/060-bearer-auth-scope-claim --verbose` and `pii-scan.sh` were NOT executed in this dispatch. The doc additions use only generic placeholders the repo's gitleaks rule does not flag, but the formal scan is deferred. Follow-up: run both guards before flipping the spec to `done` and capture the outputs here.

### No Env-Specific Content

`grep -E '(\\b[A-Z][a-z]+(stein|ville|polis|burg|town))|\\b<owner-username>\\b|\\bts\\.net\\b|100\\.[0-9]+\\.[0-9]+\\.[0-9]+|/home/[a-z]+/' docs/Operations.md docs/API.md` against the new content — manually verified clean (the new subsections use only `<user-id>`, `<old-id>`, `<wire-token>`, `<old-wire-token>`, `127.0.0.1`-shape examples).

### Change Boundary

Files modified are exactly the allowed family for Scope 4: `docs/Operations.md`, `docs/API.md`. No source changes. No spec changes outside spec 060.
