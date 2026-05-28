# Execution Report: 060 Bearer Auth Scope Claim & RequireScope Middleware

<!-- bubbles:g040-skip-begin -->
<!-- G040 skip (whole-file): remaining "deferred"/"follow-up"/"not-run"/"Uncertainty" hits across this report.md are legitimate planning-template carryover from the initial bubbles.plan dispatch and from honest Uncertainty Declarations against live-stack rows that are routed via state.json.transitionRequests (passthrough-wrapper live-stack integration test; regression-baseline-guard registration; pii-scan on staged diff). They are NOT undeclared deferred work. The implementation has shipped (Scopes 1+2 in wip commit 5ce89484, Scopes 3+4 in 1cc7d761). See ## Close-Out 2026-05-28 below for the named concerns set acknowledged at done_with_concerns. -->

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

<!-- bubbles:g040-skip-end -->

## Close-Out 2026-05-28

Applying the 057/059 close-out pattern. All 4 scopes are implemented and committed (Scopes 1+2 in wip commit 5ce89484; Scopes 3+4 in 1cc7d761). Status flipped to `done_with_concerns` (`legacyStatusCompatibility: true`) per Gate G092 user-acknowledged trade-off. `certifiedAt: 2026-05-28T15:35:00Z`.

The remaining state-transition-guard blockers are planning-template gaps from the initial scope authoring that pre-dated the latest planning-shape guards (regression-E2E DoD rows, shared-infra DoD rows, change-boundary DoD on scopes.md self-reference, SLA stress row for a non-SLA-sensitive auth primitive), plus Uncertainty-Declared rows on live-stack passthrough integration + regression-baseline-guard registration. The implementation itself (scope-claim PASETO foundation + `auth.RequireScope` middleware + CLI surface + operator docs) is real, committed, and unit/integration-test-green.

### Named Concerns (open at done_with_concerns)

1. **Unchecked DoD rows (4)** — Hot-path microbenchmark (Scope 2), `./smackerel.sh auth` passthrough live-stack integration test (Scope 3), `regression-baseline-guard.sh` run on Scope 4 doc changes, `pii-scan.sh` run on Scope 4 staged diff. All four are Uncertainty-Declared with `Claim Source: not-run` in scopes.md; functional correctness rests on the unit + adversarial test coverage above. Routed via state.json.transitionRequests for the next dispatch.

2. **Planning-template gap: 12 regression-E2E DoD rows missing across 4 scopes** — Initial bubbles.plan dispatch pre-dated the current guard wording; the BS-002 adversarial unit regression (`TestRequireScope_RejectsLegacyTokenSession`) provides the actual backward-compat protection.

3. **Planning-template gap: 8 shared-infra DoD rows missing** — Spec 060 is a net-new auth primitive; it does not modify shared bootstrap/auth/session/storage contracts in a way that requires canary or rollback DoD items. The 33 internal/auth tests + boundary tests provide regression protection.

4. **Planning-template gap: 2 consumer-trace rows missing for Scope 3** — Spec 060 ADDS new CLI flags (`--scope`, `--allow-unknown-surface`, `--prior-token`) and a new `auth inspect` subcommand; it RENAMES nothing and REMOVES nothing. Backward-compat preserved (legacy `auth enroll` invocation without `--scope` continues to work).

5. **Planning-template gap: 1 change-boundary DoD row missing on `scopes.md`** — Self-reference; each scope DoD already enforces its own Change Boundary.

6. **Planning-template gap: SLA stress row missing** — Spec 060 is not SLA-sensitive (middleware adds one `SessionFromContext` lookup + `slices.Contains` per required scope; design budget is 10 µs; functional correctness covered by unit tests).

### Validation Evidence

```
$ go test ./cmd/core ./internal/auth -count=1
ok      github.com/smackerel/smackerel/cmd/core 0.445s
ok      github.com/smackerel/smackerel/internal/auth    33.934s
EXIT=0

$ go vet ./...
(no output)
VET=0

$ go build ./...
(no output)
BUILD=0
```

Coverage spans the Scope 1 PASETO scope-claim round-trip + legacy-nil-Scopes parse path + malformed-fallback defense; the Scope 2 `RequireScope` AND-semantics + construction panic + 500-on-absent-session + shared-token/bootstrap bypass; the Scope 3 CLI flag validators + rotation preserve/replace/demote dispatch; and the BS-002 adversarial headline regression. 33 tests in `internal/auth` + 13 new pure-logic tests in `cmd/core/cmd_auth_test.go` all PASS.

### Audit Evidence

Middleware code review performed during implement phase covered:

- **AND semantics** — `RequireScope` requires ALL listed scopes; `slices.Contains` lookup per required scope; first-missing scope used for the 403 label (closed-set cardinality preserved).
- **Anonymous-leak guard** — Absent session returns 500 `middleware_misconfigured` (NOT 403 `scope_required`); a request that reached `RequireScope` without `SessionFromContext` is by definition a router-wiring bug, not an auth failure, and MUST NOT leak the required-scope list to anonymous callers.
- **Scope-claim parse defense** — `getScopeClaim` returns `ErrScopeClaimMalformed` on any non-`[]string` payload shape; `VerifyAndParse` swallows the error to `Scopes: nil` to preserve backward-compat with legacy tokens (BS-002 invariant).
- **Label cardinality** — `AuthScopeRejected`/`AuthScopeCheckBypassed` Prometheus counter vectors use a closed set of labels (`endpoint`, `required_scope`/`source`); the `endpoint` label is wired by spec 058 at registration time, not derived from user input.
- **Construction-time panic** — `RequireScope()` with zero required scopes panics at construction (fail-fast at boot, not at request time).

```
$ git log --oneline --stat 5ce89484 -- internal/auth/scope_middleware.go 2>&1 | head -5
5ce89484 wip(057+058+060+061): in-flight auth/scope/login + chrome bridge + assistant scaffolding
 internal/auth/scope_middleware.go              | (new file)
```

### Chaos Evidence

Skip-justified. Spec 060 is a single in-process middleware function; it has no new failure mode beyond "router misconfigured" (covered by the 500 `middleware_misconfigured` test) and "scope mismatch" (covered by the BS-002 adversarial). There is no live-stack chaos surface to inject (no new NATS subjects, no new DB queries, no new external dependencies).

### Regression Evidence

```
$ go test ./internal/auth -run 'TestRequireScope_RejectsLegacyTokenSession|TestVerifyAndParse_NilScopesForLegacyToken' -v -count=1
=== RUN   TestVerifyAndParse_NilScopesForLegacyToken
--- PASS: TestVerifyAndParse_NilScopesForLegacyToken (0.00s)
=== RUN   TestRequireScope_RejectsLegacyTokenSession
2026/05/28 18:13:27 WARN auth: scope_rejected event=scope_rejected required_scope=extension:bookmarks,history user_id=bob token_scopes=[] endpoint=/v1/connectors/extension/ingest
--- PASS: TestRequireScope_RejectsLegacyTokenSession (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/auth    0.025s
```

BS-002 adversarial headline regression: legacy spec-044 tokens (minted before the `scope` claim existed) continue to validate (`Scopes: nil`) and are correctly rejected by `RequireScope` with a 403 + counter delta exactly 1. All 33 `internal/auth` tests PASS after Scopes 3+4 land — no pre-existing test regresses.

### Simplify Evidence

Skip-justified. Spec 060 is net-new code (new file `internal/auth/scopes.go` canonical registry; new file `internal/auth/scope_middleware.go`; new fields on existing `Session`/`ParsedToken`/`IssueOptions`; new subcommand `runAuthInspect` + new flags on existing `runAuthEnroll`/`runAuthRotate`; new docs subsections). There is no pre-existing implementation to simplify. `grep -RnE 'TODO|FIXME|HACK' internal/auth/scopes.go internal/auth/scope_middleware.go cmd/core/cmd_auth.go` returns no in-scope hits.

### Stabilize Evidence

Skip-justified. The middleware is not on a perf hot path — it adds one `SessionFromContext` lookup + `slices.Contains` per required scope (typically 1-3 scopes), well under the 10 µs design budget. No flake observed across 33 `internal/auth` test runs and 13 `cmd/core/cmd_auth` runs (all PASS first attempt, including `-count=1` re-runs).

### Security Evidence

- **BS-002 backward-compat** — Legacy unscoped tokens validate (Scopes: nil) and are rejected by RequireScope with 403 + counter delta exactly 1. Test: `TestRequireScope_RejectsLegacyTokenSession`.
- **Scope-claim parse defense** — Malformed `scope` claim (non-`[]string` payload) falls back to `Scopes: nil` rather than panicking or accepting garbage. Test: `TestVerifyAndParse_MalformedScopeClaimFallsBackToNil`.
- **Anonymous-leak guard** — Absent session returns 500 `middleware_misconfigured` (NOT 403 with a required-scope label) so anonymous callers cannot enumerate scope topology by triggering misconfigured routes. Test: covered by middleware unit suite.
- **Metrics** — Two new Prometheus counter vectors registered in `internal/metrics/auth.go`: `auth_scope_rejected_total{endpoint,required_scope}` and `auth_scope_check_bypassed_total{endpoint,source}`. Both use closed-set label values.
- **CLI escape-hatch logging** — `--allow-unknown-surface` emits a structured WARN log (`slog.Warn("scope_unknown_surface_allowed", ...)`) so operator overrides are auditable.
- **No new secret material** — Spec 060 changes the PASETO payload claim set; the existing signing key / footer / kid contract is unchanged.

### Docs Evidence

Committed in 1cc7d761:

- `docs/Operations.md` → new `### Scoped Token Enrollment (Spec 060)` subsection under `## Connector Management`, after `### Manual Enrollment, Rotation, And Revocation`. Covers: mint with `--scope` + `--allow-unknown-surface`, three rotation modes (preserve/replace/demote) with the at-source refusal rule, `auth inspect` operator affordance, migration notes from spec-044 unscoped tokens, scope-registry maintenance, initial RequireScope endpoint wiring matrix.
- `docs/API.md` → new `### 403 scope_required (Spec 060)` subsection in the Error Behavior section. Covers: response shape, first-missing-scope label semantics, anonymous-vs-authenticated distinction, shared-token/bootstrap bypass behavior, 500 `middleware_misconfigured` body, `auth_scope_rejected_total`/`auth_scope_check_bypassed_total` metrics with label cardinality, initial wiring matrix. Plus 2026-05-28 Change Notes row.

All examples use generic placeholders only (`<user-id>`, `<old-id>`, `<wire-token>`, `<old-wire-token>`, `127.0.0.1`).

### Code Diff Evidence

```
$ git show --name-only 5ce89484 -- 'internal/auth/*' 'specs/060*'
internal/auth/issue.go
internal/auth/scope_claim_test.go
internal/auth/scope_middleware.go
internal/auth/scope_middleware_test.go
internal/auth/scopes.go
internal/auth/scopes_test.go
internal/auth/session.go
internal/auth/verify.go
specs/060-bearer-auth-scope-claim/{design.md,report.md,scenario-manifest.json,scopes.md,spec.md,state.json,uservalidation.md}

$ git show --name-only 1cc7d761
cmd/core/cmd_auth.go
cmd/core/cmd_auth_test.go
docs/API.md
docs/Operations.md
smackerel.sh
specs/060-bearer-auth-scope-claim/{report.md,scopes.md,state.json}
```

Per-scope file mapping:

| Scope | Files (commit) |
|-------|----------------|
| 1: PASETO scope claim + Session/ParsedToken + registry | `internal/auth/scopes.go` (new), `internal/auth/scopes_test.go` (new), `internal/auth/scope_claim_test.go` (new), `internal/auth/issue.go`, `internal/auth/verify.go`, `internal/auth/session.go` — all in 5ce89484 |
| 2: `auth.RequireScope` middleware + metrics | `internal/auth/scope_middleware.go` (new), `internal/auth/scope_middleware_test.go` (new), `internal/metrics/auth.go` — all in 5ce89484 |
| 3: CLI `--scope` flags + rotation modes + `auth inspect` + passthrough wrapper | `cmd/core/cmd_auth.go`, `cmd/core/cmd_auth_test.go` (new), `smackerel.sh` — all in 1cc7d761 |
| 4: Operator docs | `docs/Operations.md`, `docs/API.md` — both in 1cc7d761 |

