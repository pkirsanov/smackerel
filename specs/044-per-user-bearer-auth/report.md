# Spec 044: Per-User Bearer Auth Foundation — Implementation Report

**Status:** in_progress (Scope 01 implement+test phases recorded; Scopes 02/03/04 pending)

This report records phased execution evidence for spec 044. Scope 01 SST Foundation + Token Subsystem has cleared the implement and test phases per Gate G022. Scopes 02/03/04 remain to be implemented per [`scopes.md`](./scopes.md).

---

## Summary

Spec 044-per-user-bearer-auth was scaffolded to close MIT-040-S-008 (carry-forward from MIT-040-S-003 partial close at commit `4e399a4`), MIT-038-S-003 (cloud-drive Connect body-sourced `owner_user_id`), and the actor-source segment of MIT-027-TRACE-001 (annotation actor_source). The analyst phase authored spec.md (11 scenarios, 21 functional requirements, 8 non-functional requirements, 11 acceptance criteria, 10 design-owned open questions). The design phase authored design.md (13 sections, 14 SST keys under `auth.*` block, 4-phase rollout plan, all 10 OQs resolved). The plan phase authored scopes.md (4 scopes matching the 4 design rollout phases). The implement phase landed Scope 01 SST Foundation + Token Subsystem (14 SST keys, `internal/auth/` + `internal/auth/revocation/` packages, `cmd/core/cmd_auth.go` CLI, admin HTTP handlers, DB migration 033, startup fail-loud validation). The test phase (this entry) executed the formal Gate G022 test commands — `./smackerel.sh check`, Go + Python unit tests, the live `TestAuth*` integration tests against the test stack, `go vet ./...`, and `bash .github/bubbles/scripts/artifact-lint.sh` — and recorded verbatim evidence in the Test Evidence section below.

## Completion Statement

This spec is **NOT yet complete**. Status remains `in_progress` until all 4 scopes are implemented, tested, validated, audited, and certified. The closure will be marked when:

- Scope 01 (SST Foundation + Token Subsystem) lands all 14 SST keys, the `internal/auth/` and `internal/auth/revocation/` packages, the `cmd/core/cmd_auth.go` CLI commands, the `internal/api/auth_handlers.go` admin HTTP endpoints, the DB migrations, and the startup fail-loud validation.
- Scope 02 (Hot-Path Middleware Integration + MIT Closures) refactors `bearerAuthMiddleware`, `MintReveal`, `drive.Connect`, and the annotation pipeline; closes MIT-040-S-008 in spec 040 state.json, MIT-038-S-003 in spec 038 state.json, and the MIT-027-TRACE-001 actor-source segment in spec 027 state.json.
- Scope 03 (Web Surfaces + Telegram Connector) updates PWA, extension, and Telegram connector to send/derive per-user PASETO tokens; admin token-management UI lands.
- Scope 04 (Deprecation Pathway + Documentation Freshness) defaults `auth.production_shared_token_fallback_enabled: false`; updates `docs/Operations.md`, `docs/Deployment.md`, `docs/Development.md`, `docs/smackerel.md`; lands Prometheus metrics emitters; runs regression-baseline-guard.

## Test Evidence

The following blocks capture verbatim terminal output for the formal Gate G022 test commands executed against commit `2e2a2b9c` (with BUG-001 ollama image-pin fix `ea2af19a` applied so the live test stack can boot). All commands were executed under `bubbles.test` for Scope 01.

### Gate 1 — `./smackerel.sh check` (config + env_file drift + scenario-lint)

```
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 5, rejected: 0
scenario-lint: OK
$ echo "exit=$?"
exit=0
```

**Claim Source:** executed.

### Gate 2a — `./smackerel.sh test unit --go` (full Go unit suite)

All Go unit packages pass; `internal/auth/` and `internal/auth/revocation/` produce 0 skips. Tail of the runner output (cached + freshly-resolved):

```
$ ./smackerel.sh test unit --go
ok      github.com/smackerel/smackerel/cmd/core (cached)
ok      github.com/smackerel/smackerel/internal/auth    (cached)
ok      github.com/smackerel/smackerel/internal/auth/revocation (cached)
ok      github.com/smackerel/smackerel/internal/config  1.049s
ok      github.com/smackerel/smackerel/internal/connector/guesthost     (cached)
ok      github.com/smackerel/smackerel/internal/api     (cached)
ok      github.com/smackerel/smackerel/internal/db      (cached)
ok      github.com/smackerel/smackerel/internal/drive   (cached)
ok      github.com/smackerel/smackerel/internal/drive/google    (cached)
ok      github.com/smackerel/smackerel/internal/telegram        (cached)
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
$ echo "exit=$?"
exit=0
```

Skip-marker scan over the auth surface (must be empty):

```
$ grep -rn 't\.Skip\|SkipNow()\|t\.Skipf' internal/auth/
$ grep -rn 't\.Skip\|SkipNow()\|t\.Skipf' tests/integration/auth_*.go
tests/integration/auth_bootstrap_test.go:24:// No `t.Skip()` — when DATABASE_URL is unset, this test fails with a
$ echo "exit=$?"
exit=0
```

The single match in `tests/integration/auth_bootstrap_test.go:24` is an explanatory comment in the file header (`// No \`t.Skip()\` — when DATABASE_URL is unset, this test fails with a clear message`); no actual `t.Skip()` call is present in any auth source or test.

**Claim Source:** executed.

### Gate 2b — `./smackerel.sh test unit --python` (Python ML sidecar suite)

```
$ ./smackerel.sh test unit --python
[...uv install of pinned wheels per pyproject.toml — elided]
........................................................................ [ 17%]
........................................................................ [ 34%]
........................................................................ [ 51%]
........................................................................ [ 69%]
........................................................................ [ 86%]
.........................................................                [100%]
417 passed in 15.08s
$ echo "exit=$?"
exit=0
```

**Claim Source:** executed.

### Gate 2c — Targeted `internal/auth/...` race-mode rerun (T1-01..T1-10 verbose)

Verbatim verbose output for the full auth-package test set under `-race -count=1`. Captures every adversarial sub-test the user requested by name (T1-01 PASETO sign/verify happy path, T1-02 key-id mismatch rejection, T1-03 expired token rejection, T1-10 ValidateRuntimeAuthStartup fail-loud cases including empty-key-id / empty-hashing-key / hashing-key==signing-key OQ-8 case):

```
$ go test -race -count=1 -v ./internal/auth/...
=== RUN   TestIssueToken_RoundTripWithVerify
--- PASS: TestIssueToken_RoundTripWithVerify (0.00s)
=== RUN   TestIssueToken_RejectsMissingFields
=== RUN   TestIssueToken_RejectsMissingFields/no-user-id
=== RUN   TestIssueToken_RejectsMissingFields/no-token-id
=== RUN   TestIssueToken_RejectsMissingFields/no-signing-key
=== RUN   TestIssueToken_RejectsMissingFields/no-key-id
=== RUN   TestIssueToken_RejectsMissingFields/no-issuer
=== RUN   TestIssueToken_RejectsMissingFields/zero-ttl
=== RUN   TestIssueToken_RejectsMissingFields/no-clock
--- PASS: TestIssueToken_RejectsMissingFields (0.00s)
    --- PASS: TestIssueToken_RejectsMissingFields/no-user-id (0.00s)
    --- PASS: TestIssueToken_RejectsMissingFields/no-token-id (0.00s)
    --- PASS: TestIssueToken_RejectsMissingFields/no-signing-key (0.00s)
    --- PASS: TestIssueToken_RejectsMissingFields/no-key-id (0.00s)
    --- PASS: TestIssueToken_RejectsMissingFields/no-issuer (0.00s)
    --- PASS: TestIssueToken_RejectsMissingFields/zero-ttl (0.00s)
    --- PASS: TestIssueToken_RejectsMissingFields/no-clock (0.00s)
=== RUN   TestSST_NoHardcodedAuthValues
    sst_grep_guard_test.go:236: SST guard OK: no production source file contains [auth.revocations paseto-v4-public] outside config/
--- PASS: TestSST_NoHardcodedAuthValues (0.13s)
=== RUN   TestSST_NoHardcodedAuthValues_Adversarial
    sst_grep_guard_test.go:300: adversarial OK: scanner reports 2 findings against the 2-literal fixture; [fakeprod/naughty.go:4: NATSSubject = "auth.revocations" fakeprod/naughty.go:5: TokenFormat = "paseto-v4-public"]
--- PASS: TestSST_NoHardcodedAuthValues_Adversarial (0.01s)
=== RUN   TestSST_NoHardcodedAuthValues_AllowlistAdversarial
    sst_grep_guard_test.go:326: allowlist OK: *_test.go fixture with literal 'auth.revocations' is correctly skipped
--- PASS: TestSST_NoHardcodedAuthValues_AllowlistAdversarial (0.00s)
=== RUN   TestValidateRuntimeAuthStartup
=== RUN   TestValidateRuntimeAuthStartup/production+enabled+well-formed_permitted
=== RUN   TestValidateRuntimeAuthStartup/production+disabled_bypasses_validation
=== RUN   TestValidateRuntimeAuthStartup/development+enabled+empty_material_permitted_(bootstrap-time)
=== RUN   TestValidateRuntimeAuthStartup/test+enabled+empty_material_permitted_(bootstrap-time)
=== RUN   TestValidateRuntimeAuthStartup/production+enabled+empty_signing_key_fails_loudly
=== RUN   TestValidateRuntimeAuthStartup/production+enabled+empty_key_id_fails_loudly
=== RUN   TestValidateRuntimeAuthStartup/production+enabled+empty_hashing_key_fails_loudly
=== RUN   TestValidateRuntimeAuthStartup/production+enabled+hashing_key_equals_signing_key_fails_loudly_(OQ-8)
--- PASS: TestValidateRuntimeAuthStartup (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/production+enabled+well-formed_permitted (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/production+disabled_bypasses_validation (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/development+enabled+empty_material_permitted_(bootstrap-time) (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/test+enabled+empty_material_permitted_(bootstrap-time) (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/production+enabled+empty_signing_key_fails_loudly (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/production+enabled+empty_key_id_fails_loudly (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/production+enabled+empty_hashing_key_fails_loudly (0.00s)
    --- PASS: TestValidateRuntimeAuthStartup/production+enabled+hashing_key_equals_signing_key_fails_loudly_(OQ-8) (0.00s)
=== RUN   TestVerifyAndParse_RejectsExpiredAndFutureAndForeignIssuer
--- PASS: TestVerifyAndParse_RejectsExpiredAndFutureAndForeignIssuer (0.01s)
=== RUN   TestVerifyAndParse_RotationGraceWindow_HonorsPriorKey
--- PASS: TestVerifyAndParse_RotationGraceWindow_HonorsPriorKey (0.01s)
=== RUN   TestVerifyAndParse_RejectsHalfRotationConfig
=== RUN   TestVerifyAndParse_RejectsHalfRotationConfig/only-prior-public-set
=== RUN   TestVerifyAndParse_RejectsHalfRotationConfig/only-prior-key-id-set
--- PASS: TestVerifyAndParse_RejectsHalfRotationConfig (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/auth    16.627s
=== RUN   TestRevocationCache_BootstrapAndPropagate
--- PASS: TestRevocationCache_BootstrapAndPropagate (0.00s)
=== RUN   TestRevocationCache_PropagatesLoaderErrors
--- PASS: TestRevocationCache_PropagatesLoaderErrors (0.00s)
=== RUN   TestRevocationCache_RejectsNilLoader
--- PASS: TestRevocationCache_RejectsNilLoader (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/auth/revocation 1.040s
$ echo "exit=$?"
exit=0
```

#### Adversarial assertion text — verbatim from source

| Sub-test (per scopes.md Test Plan) | Adversarial branch | Verbatim rejection text from `internal/auth/startup.go` |
|---|---|---|
| `TestValidateRuntimeAuthStartup/.../empty_signing_key_fails_loudly` (T1-10) | production + enabled, empty `auth.signing.active_private_key` | `auth: AUTH_SIGNING_ACTIVE_PRIVATE_KEY must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true` |
| `TestValidateRuntimeAuthStartup/.../empty_key_id_fails_loudly` (T1-10) | production + enabled, empty `auth.signing.active_key_id` | `auth: AUTH_SIGNING_ACTIVE_KEY_ID must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true` |
| `TestValidateRuntimeAuthStartup/.../empty_hashing_key_fails_loudly` (T1-10) | production + enabled, empty `auth.at_rest_hashing_key` | `auth: AUTH_AT_REST_HASHING_KEY must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true` |
| `TestValidateRuntimeAuthStartup/.../hashing_key_equals_signing_key_fails_loudly_(OQ-8)` (T1-10) | production + enabled, hashing key == signing key | `auth: AUTH_AT_REST_HASHING_KEY must differ from AUTH_SIGNING_ACTIVE_PRIVATE_KEY (spec 044 OQ-8)` |
| `TestVerifyAndParse_RotationGraceWindow_HonorsPriorKey` foreign-kid sub-case (T1-02 key-id mismatch) | foreign `kid` matching neither active nor prior | sentinel `ErrUnknownKeyID` from `internal/auth/verify.go`; assertion `errors.Is(err, ErrUnknownKeyID)` PASSES |
| `TestVerifyAndParse_RotationGraceWindow_HonorsPriorKey` forged-kid adversarial sub-case | prior-key-signed token whose footer `kid` is forged to match active kid | `forged-kid token MUST fail verification, but it passed` test fatal triggered if signature verifies under wrong key — runtime PASS confirms verifier rejects forgery (PASETO v4.public signature mismatch) |
| `TestVerifyAndParse_RejectsExpiredAndFutureAndForeignIssuer` expired sub-case (T1-03 expired token) | token whose `exp` claim is in the past beyond skew tolerance | sentinel `ErrTokenExpired` from `internal/auth/verify.go`; assertion `errors.Is(err, ErrTokenExpired)` PASSES |
| `TestVerifyAndParse_RejectsExpiredAndFutureAndForeignIssuer` future sub-case | token whose `nbf` is in the future beyond skew tolerance | sentinel `ErrTokenNotYetValid`; assertion `errors.Is(err, ErrTokenNotYetValid)` PASSES |
| `TestVerifyAndParse_RejectsExpiredAndFutureAndForeignIssuer` foreign-issuer sub-case | token whose `iss` differs from configured Issuer | sentinel `ErrIssuerMismatch`; assertion `errors.Is(err, ErrIssuerMismatch)` PASSES |
| `TestIssueToken_RoundTripWithVerify` (T1-01 PASETO sign/verify happy path) | well-formed token signed with active key | round-trip PASS: prefix `v4.public.`, `sub=user-alice`, `tid=tok-alice-001`, `kid=key-2026-05`, `iat`/`exp` honor configured TTL |

**Claim Source:** executed.

### Gate 3 — T1-08 live integration test against the test stack

Test stack brought up via `./smackerel.sh --env test up` (postgres healthy on `127.0.0.1:47001`, NATS healthy on `127.0.0.1:47002`). `DATABASE_URL` exported pointing at the host-port-bound postgres; auth migration `internal/db/migrations/033_auth_per_user_bearer.sql` applied by `db.Migrate` inside `authTestPool`.

```
$ ./smackerel.sh --env test up
[...container lifecycle output — postgres, nats, smackerel-ml, smackerel-core all reach Healthy]
 Container smackerel-test-postgres-1  Healthy
 Container smackerel-test-nats-1  Healthy
 Container smackerel-test-smackerel-ml-1  Healthy
 Container smackerel-test-smackerel-core-1  Healthy
$ docker ps --filter "name=smackerel-test-postgres" --format '{{.Names}}\t{{.Status}}\t{{.Ports}}'
smackerel-test-postgres-1       Up 2 minutes (healthy)  127.0.0.1:47001->5432/tcp
$ export DATABASE_URL='postgres://smackerel:${POSTGRES_PASSWORD}@127.0.0.1:47001/smackerel?sslmode=disable'
$ export POSTGRES_URL="$DATABASE_URL"
$ go test -count=1 -tags=integration -v -timeout=120s -run 'TestAuth' ./tests/integration/...
=== RUN   TestAuthBootstrap_FreshProduction_EnrollsFirstUser
--- PASS: TestAuthBootstrap_FreshProduction_EnrollsFirstUser (0.06s)
=== RUN   TestAuthBootstrap_PublicHexDerivation
--- PASS: TestAuthBootstrap_PublicHexDerivation (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.087s
$ echo "exit=$?"
exit=0
```

#### Live DB row-count evidence (post-T1-08)

The integration test `TestAuthBootstrap_FreshProduction_EnrollsFirstUser` calls `resetAuthTables` at the start (rows = 0), then enrolls `user-bootstrap-001`, persists token `tok-bootstrap-001`, and round-trips through `VerifyAndParse`. After the test PASSES, the live DB shows exactly the expected end state:

```
$ docker exec smackerel-test-postgres-1 psql -U smackerel -d smackerel -c '\dt auth_*'
               List of relations
 Schema |       Name       | Type  |   Owner
--------+------------------+-------+-----------
 public | auth_revocations | table | smackerel
 public | auth_tokens      | table | smackerel
 public | auth_users       | table | smackerel
(3 rows)

$ docker exec smackerel-test-postgres-1 psql -U smackerel -d smackerel -c 'SELECT user_id, enrolled_by, status FROM auth_users;'
      user_id       |        enrolled_by         | status
--------------------+----------------------------+--------
 user-bootstrap-001 | bootstrap@integration-test | active
(1 row)

$ docker exec smackerel-test-postgres-1 psql -U smackerel -d smackerel -c 'SELECT token_id, user_id, key_id, status, issued_source, LENGTH(hashed_token) AS hash_len FROM auth_tokens;'
     token_id      |      user_id       |      key_id      | status | issued_source | hash_len
-------------------+--------------------+------------------+--------+---------------+----------
 tok-bootstrap-001 | user-bootstrap-001 | key-test-2026-05 | active | bootstrap     |       64
(1 row)

$ docker exec smackerel-test-postgres-1 psql -U smackerel -d smackerel -c 'SELECT COUNT(*) as revocation_count FROM auth_revocations;'
 revocation_count
------------------
                0
(1 row)
```

DB connection details (no PII — generic dev fixture credentials only): host `127.0.0.1`, port `47001` (test stack POSTGRES_HOST_PORT), DB `smackerel`, container `smackerel-test-postgres-1`. Migration `033_auth_per_user_bearer.sql` applied successfully (3 tables present). Row counts: before enrollment 0/0/0; after enrollment 1 user / 1 token / 0 revocations. Token hash length 64 chars = 32-byte HMAC-SHA-256 hex per `internal/auth/hash.go`.

#### T1-06 BearerStore.Enroll duplicate-user adversarial (live)

`TestAuthBootstrap_FreshProduction_EnrollsFirstUser` includes an adversarial second-`Enroll` of the same `user_id` immediately after the first succeeds. The test PASSES, proving the `auth_users.user_id UNIQUE` constraint surfaces a duplicate-user error (the test asserts the error message contains either `"duplicate"` or `"unique"` after lowercasing — pgx surfaces the violation as `ERROR: duplicate key value violates unique constraint "auth_users_user_id_key"` from the underlying postgres CHECK).

**Claim Source:** executed.

### Gate 4 — `go vet ./...` (full repo + integration tag)

```
$ go vet ./...
$ echo "vet_exit=$?"
vet_exit=0
$ go vet -tags=integration ./tests/integration/...
$ echo "vet_int_exit=$?"
vet_int_exit=0
```

Empty stdout from both invocations indicates zero diagnostics. Both build configurations (default tag set and `-tags=integration`) report clean.

**Claim Source:** executed.

### Gate 5 — `bash .github/bubbles/scripts/artifact-lint.sh specs/044-per-user-bearer-auth`

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/044-per-user-bearer-auth
[...full per-check trace — all ✅ PASS rows elided here for brevity; complete output captured in scopes.md DoD evidence below]

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
$ echo "exit=$?"
exit=0
```

**Claim Source:** executed.

---

## Planned Implementation Order

Per [`design.md`](./design.md) §12 Rollout Plan and [`scopes.md`](./scopes.md):

1. **Scope 01 — SST Foundation + Token Subsystem** — pending (bubbles.implement)
2. **Scope 02 — Hot-Path Middleware Integration + MIT Closures** — pending (bubbles.implement)
3. **Scope 03 — Web Surfaces + Telegram Connector** — pending (bubbles.implement)
4. **Scope 04 — Deprecation Pathway + Documentation Freshness** — pending (bubbles.implement, bubbles.docs)

---

## Planned Evidence References (placeholders for trace-guard)

The following test files will be authored as scopes are implemented:

- `internal/config/validate_test.go` — Scope 1 SST validation tests
- `internal/auth/issue_test.go` — Scope 1 token issuance tests
- `internal/auth/verify_test.go` — Scope 1+2 PASETO verification tests
- `internal/auth/revocation/cache_test.go` — Scope 1+2 revocation cache tests
- `internal/auth/sst_grep_guard_test.go` — Scope 1 SST grep guard
- `internal/api/router_test.go` — Scope 2 middleware tests
- `internal/metrics/auth_metrics_test.go` — Scope 4 Prometheus metrics tests
- `tests/integration/auth_bootstrap_test.go` — Scope 1 bootstrap integration test
- `tests/integration/auth_startup_test.go` — Scope 1 startup fail-loud tests
- `tests/integration/auth_mintreveal_test.go` — Scope 2 MintReveal claim-binding + adversarial regression tests
- `tests/integration/auth_drive_connect_test.go` — Scope 2 drive.Connect claim-binding tests
- `tests/integration/auth_annotation_test.go` — Scope 2 annotation pipeline claim-binding tests
- `tests/integration/auth_rotation_test.go` — Scope 2 rotation grace window tests
- `tests/integration/auth_revocation_test.go` — Scope 2 revocation propagation tests
- `tests/integration/auth_no_body_header_actor_id_test.go` — Scope 2 AC-11 grep guard
- `tests/e2e/auth/pwa_per_user_test.go` — Scope 3 PWA E2E test
- `tests/e2e/auth/extension_per_user_test.go` — Scope 3 extension E2E test
- `tests/e2e/auth/telegram_per_user_test.go` — Scope 3 Telegram bridge E2E test
- `tests/e2e/auth/admin_ui_test.go` — Scope 3 admin UI E2E test

---

## Cross-Spec Closure Plan

This spec's completion will close the following routed backlog items:

- **MIT-040-S-008** (routed in spec 040 commit `4e399a4` carry-forward from MIT-040-S-003 partial close) — fully resolved when Scope 2 lands.
- **MIT-038-S-003** — cloud-drive Connect body-sourced `owner_user_id` resolved when Scope 2 lands.
- **MIT-027-TRACE-001 actor-source segment** — annotation actor_source resolved when Scope 2 lands.
- **VAL-FINDING-040-S-003** — header-trust workaround eliminated in production when Scope 2 lands; AC-11 grep guard provides ongoing enforcement.

---

## References

- [`spec.md`](./spec.md) — feature specification (11 SCN-AUTH-NNN scenarios + 21 FR-AUTH-NNN requirements + 8 NFR-AUTH-NNN + 11 AC + 10 OQ)
- [`design.md`](./design.md) — 13-section design (system context, component diagram, SST plan, lifecycle, hot-path anatomy, failure modes, performance budget, backward compat, security, risks, rollout, OQ resolutions)
- [`scopes.md`](./scopes.md) — 4 scopes per design rollout plan
- [`scenario-manifest.json`](./scenario-manifest.json) — scenario → evidence-ref manifest (planned status)
- `specs/040-cloud-photo-libraries/state.json` — MIT-040-S-008 routing entry (closure target)
- `specs/038-cloud-drives-integration/state.json` — MIT-038-S-003 routing entry (closure target)
- `specs/027-user-annotations/state.json` — MIT-027-TRACE-001 actor-source segment (closure target)
- `.github/skills/bubbles-config-sst/SKILL.md` — SST zero-defaults compliance
- `.github/skills/bubbles-test-environment-isolation/SKILL.md` — test-isolated DB pattern
