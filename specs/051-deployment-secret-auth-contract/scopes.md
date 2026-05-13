# Scopes: Deployment Secret and Auth Contract

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md)

## Scope 1: Auth secret production-load gate (PASETO v4 / Ed25519, defense-in-depth)

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-051-S01 Missing auth signing key fails before runtime start
  Given runtime.environment=production AND auth.enabled=true
  And deployment configuration omits AUTH_SIGNING_ACTIVE_PRIVATE_KEY
  When config validation runs
  Then validation fails with a clear missing-secret error
  And the error names "AUTH_SIGNING_ACTIVE_PRIVATE_KEY"
  And the Smackerel runtime does not start
```

### Implementation Plan

1. Add the `AUTH_BOOTSTRAP_TOKEN` production-load gate to `loadAuthConfig` in [internal/config/config.go](../../internal/config/config.go) (right after the `at_rest_hashing_key` distinctness check). The error MUST name the env var without echoing its value.
2. Verify the existing production-mode block already covers `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, `AUTH_SIGNING_ACTIVE_KEY_ID`, and `AUTH_AT_REST_HASHING_KEY`. Add adversarial test coverage for any path that the existing tests do not exercise (e.g., bootstrap-token empty in production).
3. Confirm [internal/auth/startup.go](../../internal/auth/startup.go) `ValidateRuntimeAuthStartup` continues to enforce the wiring-time defense-in-depth check (no behavior change required for spec 051; the bootstrap-token gate fires earlier in `loadAuthConfig`).
4. Update [config/smackerel.yaml](../../config/smackerel.yaml) comment block above `auth.bootstrap_token` to reflect the new always-required-in-production semantics.

### Shared Infrastructure Impact Sweep

This scope modifies the shared auth-bootstrap contract consumed by every deployable target (PWA, Telegram bridge, ML sidecar). Downstream contract surfaces enumerated:

- `internal/auth/startup.go::ValidateRuntimeAuthStartup` (already enforces the same invariant at wiring time — unchanged).
- `cmd/core/wiring.go` startup sequence (consumes `cfg.Auth.BootstrapToken` indirectly via `ValidateRuntimeAuthStartup`).
- `internal/config/validate_test.go` `setRequiredEnv` shared fixture (must keep `AUTH_BOOTSTRAP_TOKEN` set to a non-empty value so unrelated tests still pass).
- `config/smackerel.yaml` comment contract (`auth.bootstrap_token` always-required-in-production semantics is operator-facing).

Rollback: revert the `loadAuthConfig` block; `ValidateRuntimeAuthStartup` continues to provide the wiring-time check. No data migration involved.

### Change Boundary (Scope 1)

Allowed file families:

- `internal/config/config.go` (`loadAuthConfig` production block only — single targeted insert).
- `internal/config/validate_test.go` (new test functions only — may extend `setRequiredEnv` defaults but must keep all existing tests green).
- `config/smackerel.yaml` (comment block above `auth.bootstrap_token` only — no schema or value changes).

Excluded surfaces (untouched by Scope 1 — enforced by review):

- `internal/auth/**` (Scope 1 must NOT modify the wiring-time enforcement layer).
- `cmd/core/wiring.go` (no startup-sequence changes).
- `scripts/commands/config.sh` (Scope 2 owns the SST-loader changes).
- Any docs file (Scope 3 owns docs changes).
- Any frontend / web / mobile source (out of contract).

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-051-001 | unit/config | [internal/config/validate_test.go](../../internal/config/validate_test.go) → `TestValidate_AuthConfig_FailsLoudOnMissingSigningKey_Production` (already exists) and a NEW `TestValidate_AuthConfig_FailsLoudOnMissingBootstrapToken_Production` | SCN-051-S01 | Missing each AUTH_* in production with `auth.enabled=true` produces a fail-loud error that names the var. |
| T-051-001-reg | Regression E2E (unit-tier durable regression) | [internal/config/validate_test.go](../../internal/config/validate_test.go) → the four `TestValidate_AuthConfig_FailsLoudOnMissing*_Production` and `TestValidate_AuthConfig_RejectsHashingKeyEqualsSigningKey_Production` cases run on every `./smackerel.sh test unit` invocation as the durable regression suite for SCN-051-S01. SCN-051-S01 has no API/UI surface (config validation is in-process and pre-startup), so an `e2e-api`/`e2e-ui` regression target is not applicable; this Test Plan row is the explicit Regression E2E equivalent for this contract dimension. | SCN-051-S01 | Persistent regression suite: every future change to `loadAuthConfig` must keep all five tests green on every push. Stress dimension is N/A — these are deterministic startup-time gates, not throughput/latency surfaces (no SLA/SLO contract attached). |
| T-051-002 | unit/config | [internal/config/validate_test.go](../../internal/config/validate_test.go) → NEW `TestLoadAuthConfig_BootstrapTokenRequiredWithEnabledProduction` | SCN-051-S01 / SCN-051-S03 | `AUTH_BOOTSTRAP_TOKEN=""` with `SMACKEREL_ENV=production` AND `AUTH_ENABLED=true` returns an error naming `AUTH_BOOTSTRAP_TOKEN`. Empty bootstrap token in dev/test does NOT error. |
| T-051-001-canary | Canary: shared `setRequiredEnv` fixture sanity (unit/config) | [internal/config/validate_test.go](../../internal/config/validate_test.go) → existing `TestValidate_AuthConfig_*_Production` suite | SCN-051-S01 | Independent canary unit suite proves spec 044 production-mode block continues to fire for the existing three keys before the bootstrap-token addition is asserted. |

### Definition of Done

- [ ] SCN-051-S01: Missing auth signing key fails before runtime start — T-051-001 passes and proves missing `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` fails before runtime start.
- [ ] SCN-051-S01: T-051-001 passes and proves missing `AUTH_SIGNING_ACTIVE_KEY_ID` fails before runtime start.
- [ ] SCN-051-S01: T-051-001 passes and proves missing `AUTH_AT_REST_HASHING_KEY` fails before runtime start.
- [ ] SCN-051-S01: T-051-002 passes and proves missing `AUTH_BOOTSTRAP_TOKEN` is rejected in production with `auth.enabled=true`.
- [ ] SCN-051-S01: T-051-002 passes and proves empty `AUTH_BOOTSTRAP_TOKEN` is accepted in dev/test (no regression of dev ergonomic).
- [ ] [config/smackerel.yaml](../../config/smackerel.yaml) `auth.bootstrap_token` comment reflects the always-required-in-production semantics.
- [ ] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior in this scope is added/maintained — for Scope 1 the persistent regression is the augmented `TestValidate_AuthConfig_FailsLoudOnMissingBootstrapToken_Production` adversarial unit suite acting as the unit-tier regression for the bootstrap-token gate (SCN-051-S01 has no UI/cross-process surface that would warrant an e2e-api or e2e-ui run; documented in design.md §Test Strategy).
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) with no new failures attributable to Scope 1.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns — the existing `TestValidate_AuthConfig_*_Production` cohort runs first and is green, proving the shared `setRequiredEnv` fixture still wires the spec 044 contract before the new gate is asserted.
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified — Shared Infrastructure Impact Sweep above documents the revert (delete the `loadAuthConfig` bootstrap-token block; runtime-tier `ValidateRuntimeAuthStartup` is unaffected) and a re-run of T-051-001/T-051-002 proves the rollback path works.
- [ ] Change Boundary is respected and zero excluded file families were changed — verified by `git diff --name-only HEAD~N HEAD` showing only the allowed file families enumerated in the Change Boundary (Scope 1) section above; no `internal/auth/**`, no `cmd/core/wiring.go`, no `scripts/commands/config.sh`, no docs, no frontend.

## Scope 2: Database secret defense-in-depth (SST loader + runtime)

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-051-S02 Default database password is rejected for deployment
  Given TARGET_ENV=home-lab
  And infrastructure.postgres.password is the local-dev default "smackerel"
  When the SST loader (scripts/commands/config.sh) runs
  Then config generation fails before any env file is written
  And the error names "infrastructure.postgres.password" without printing the value
  And the same defense-in-depth check at runtime startup also rejects the same value when SMACKEREL_ENV=production
```

### Implementation Plan

1. Add the SST-layer dev-default-rejection check to [scripts/commands/config.sh](../../scripts/commands/config.sh) immediately after `POSTGRES_PASSWORD="$(required_value infrastructure.postgres.password)"`. When `TARGET_ENV` is `home-lab` (or any other non-dev/test target), reject the value if it matches a known dev default. Echo the KEY name in stderr; never echo the VALUE.
2. Create [internal/config/secrets.go](../../internal/config/secrets.go) (NEW) with the canonical `DevDBPasswords` slice. Export `IsDevDBPassword(pw string) bool`.
3. Add the runtime-layer dev-default-rejection check to `Validate()` in [internal/config/config.go](../../internal/config/config.go). When `c.Environment == "production"`, parse the `DATABASE_URL` password component; if it matches a known dev default, fail loud with a clear error that names `DATABASE_URL`/`POSTGRES_PASSWORD` without echoing the value.

### Shared Infrastructure Impact Sweep

This scope modifies the shared SST loader (`scripts/commands/config.sh`) which is the bootstrap entry for every environment build (dev, test, home-lab). Downstream contract surfaces enumerated:

- `scripts/commands/config.sh` (the SST loader itself — central bootstrap helper).
- `config/smackerel.yaml` (`infrastructure.postgres.password` is the SST source of truth).
- `internal/config/config.go::Validate` (runtime-side rejection mirrors the SST-side rejection).
- `internal/config/secrets.go` (NEW — single Go-side source of truth for `DevDBPasswords`).
- All `./smackerel.sh up` / `./smackerel.sh build` flows: dev and test paths must continue to accept the dev default; only `home-lab` and runtime production must reject it.

Rollback: revert the `scripts/commands/config.sh` rejection block AND the `Validate()` rejection block; delete `internal/config/secrets.go`. Postgres data is not touched. Re-run `./smackerel.sh config generate --env dev` to confirm dev path still works.

### Change Boundary (Scope 2)

Allowed file families:

- `scripts/commands/config.sh` (single insert immediately after `POSTGRES_PASSWORD="$(required_value ...)"` only).
- `internal/config/secrets.go` (NEW — contains only the `DevDBPasswords` slice and `IsDevDBPassword` helper).
- `internal/config/config.go` (`Validate()` only — add the DATABASE_URL dev-default check at end of production-mode block).
- `internal/config/validate_test.go` (new test functions only).
- `internal/config/sst_loader_test.go` (NEW driver), `scripts/commands/config_secret_rejection_test.sh` (NEW shell test).

Excluded surfaces (untouched by Scope 2):

- `internal/config/config.go::loadAuthConfig` (Scope 1's contract — do not co-edit).
- `internal/auth/**` (no auth wiring changes).
- Any docs file (Scope 3 owns docs changes).
- The Postgres image, Postgres init scripts, or any Compose service definition.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-051-003 | unit/config | [internal/config/validate_test.go](../../internal/config/validate_test.go) → NEW `TestValidate_RejectsDevDBPassword_Production` | SCN-051-S02 | A `DATABASE_URL` whose password component matches a known dev default produces an error that names `POSTGRES_PASSWORD` and does NOT echo the value when `SMACKEREL_ENV=production`. The same input passes in dev/test. |
| T-051-006 | shell | [scripts/commands/config_secret_rejection_test.sh](../../scripts/commands/config_secret_rejection_test.sh) (NEW) executed from [internal/config/sst_loader_test.go](../../internal/config/sst_loader_test.go) | SCN-051-S02 | SST loader exits non-zero for `TARGET_ENV=home-lab` with the dev-default Postgres value; stderr names the offending key without echoing the value. |
| T-051-006-canary | Regression E2E (shell canary) | [scripts/commands/config_secret_rejection_test.sh](../../scripts/commands/config_secret_rejection_test.sh) executed from [internal/config/sst_loader_test.go](../../internal/config/sst_loader_test.go) | SCN-051-S02 | Independent canary: SST loader still produces a clean dev env file with the same dev-default value when `TARGET_ENV=dev` (no regression of the dev path before the home-lab assertion fires). |

### Definition of Done

- [ ] SCN-051-S02: Default database password is rejected for deployment — T-051-003 passes and proves runtime rejection of the dev-default DB password in production.
- [ ] SCN-051-S02: T-051-003 passes and proves dev/test still accept the dev-default value (no regression of dev ergonomic).
- [ ] SCN-051-S02: T-051-006 passes and proves SST loader rejection for `TARGET_ENV=home-lab`.
- [ ] SCN-051-S02: T-051-006 passes and proves SST loader stderr does NOT echo the dev-default value.
- [ ] [internal/config/secrets.go](../../internal/config/secrets.go) is the single Go-side source of truth for `DevDBPasswords`; the parallel shell list in [scripts/commands/config.sh](../../scripts/commands/config.sh) is documented as a defense-in-depth duplicate.
- [ ] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior in this scope is added/maintained — Scope 2 is exercised by the shell-level canary T-051-006-canary which proves the dev path remains functional plus the home-lab rejection assertion in T-051-006; the unit-tier T-051-003 acts as the runtime-side regression. Together they cover the SST-loader and runtime-validate boundaries that SCN-051-S02 spans (no UI surface).
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) with no new failures attributable to Scope 2.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns — T-051-006-canary asserts the dev SST path is still green before the home-lab rejection assertion runs.
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified — Shared Infrastructure Impact Sweep above enumerates the revert recipe; T-051-006-canary acts as the rollback-readiness probe (any breakage in the dev SST path is detected immediately).
- [ ] Change Boundary is respected and zero excluded file families were changed — verified by `git diff --name-only HEAD~N HEAD` showing only the allowed file families enumerated in the Change Boundary (Scope 2) section above; no `loadAuthConfig` co-edits, no `internal/auth/**` changes, no docs changes, no Compose / Postgres image changes.

## Scope 3: Secret-safe docs and log redaction proof

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1, Scope 2

### Gherkin Scenarios

```gherkin
Scenario: SCN-051-S03 Bootstrap token is required and never logged
  Given runtime.environment=production AND auth.enabled=true
  And AUTH_BOOTSTRAP_TOKEN is empty
  When config validation runs
  Then validation fails and names "AUTH_BOOTSTRAP_TOKEN"
  And no error message contains the value of any auth secret or the database password
  When AUTH_BOOTSTRAP_TOKEN is provided by the secret injection path
  Then config validation passes
  And startup error paths never include the raw bootstrap token value
```

### Implementation Plan

1. Create [internal/config/log_redaction_test.go](../../internal/config/log_redaction_test.go) (NEW). Use sentinel values with unique substrings (e.g., `LEAKCANARY-bootstrap-XXXX`). Drive every error path in `loadAuthConfig`, `Validate`, and `auth.ValidateRuntimeAuthStartup`. Assert no sentinel substring appears in any returned error; assert the offending KEY names DO appear.
2. Create [internal/config/docs_required_keys_test.go](../../internal/config/docs_required_keys_test.go) (NEW). Read [docs/Deployment.md](../../docs/Deployment.md) and [docs/Operations.md](../../docs/Operations.md). Assert each canonical key name appears at least once; assert each forbidden alias (`auth.signing.hmac_key`, `auth.signing.issuer`, `signing_secret`, `at_rest_hmac_key`, `bootstrap_secret`, `enrollment_token`) does NOT appear.
3. Confirm [docs/Deployment.md](../../docs/Deployment.md) and [docs/Operations.md](../../docs/Operations.md) already mention every canonical key. If a name is missing, add it inline using the existing table format. Do not commit any secret values.
4. Add a paragraph to [docs/Deployment.md](../../docs/Deployment.md) Per-User Bearer Auth section that explicitly references spec 051's defense-in-depth contract and the SCN-051-S03 log-redaction guarantee.

### Shared Infrastructure Impact Sweep

This scope adds two shared tests that pin operator-facing contracts touched by every spec that adds a new secret env var. Downstream contract surfaces enumerated:

- [internal/config/log_redaction_test.go](../../internal/config/log_redaction_test.go) (NEW — every future secret env var added to `loadAuthConfig` or `Validate` must keep this test green by NOT echoing values into error messages).
- [internal/config/docs_required_keys_test.go](../../internal/config/docs_required_keys_test.go) (NEW — every future canonical secret key name added to the auth contract must be referenced in `docs/Deployment.md` and `docs/Operations.md`, and every retired alias must be removed).
- [docs/Deployment.md](../../docs/Deployment.md) Per-User Bearer Auth section and [docs/Operations.md](../../docs/Operations.md) auth env-var table — content surface that the docs-static lint pins.

Rollback: delete `internal/config/log_redaction_test.go` and `internal/config/docs_required_keys_test.go`; revert the doc paragraph addition. Doc files retain their existing canonical key references because they were already correct under spec 044.

### Consumer Impact Sweep (Scope 3)

Scope 3 retires the spec-044-incompatible auth aliases (`auth.signing.hmac_key`, `auth.signing.issuer`, `signing_secret`, `at_rest_hmac_key`, `bootstrap_secret`, `enrollment_token`) from the operator-facing docs surface. Affected first-party consumer surfaces:

- `docs/Deployment.md` Per-User Bearer Auth section (operator navigation surface for env-var contract).
- `docs/Operations.md` auth env-var table (operator navigation surface for runtime contract).
- `internal/config/docs_required_keys_test.go` (NEW — the docs-static lint that pins the canonical names AND forbids the retired aliases on every future commit, eliminating stale-reference drift).
- No code routes, API endpoints, generated clients, breadcrumbs, deep links, or redirect targets are renamed (the live wire contract has always been spec 044 names; only doc text aliases are being retired).

### Change Boundary (Scope 3)

Allowed file families:

- `internal/config/log_redaction_test.go` (NEW — security-static test only).
- `internal/config/docs_required_keys_test.go` (NEW — docs-static lint only).
- `docs/Deployment.md` and `docs/Operations.md` (text-only edits to retire forbidden aliases and add a single contract-reference paragraph; no code, schema, or value changes).

Excluded surfaces (untouched by Scope 3):

- `internal/config/config.go` (Scopes 1 and 2 own runtime behavior).
- `internal/auth/**` (no wiring changes).
- `scripts/commands/config.sh` (Scope 2 owns the SST loader).
- Any source file outside `internal/config/` and any doc file outside `docs/Deployment.md`/`docs/Operations.md`.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-051-004 | security-static | [internal/config/log_redaction_test.go](../../internal/config/log_redaction_test.go) (NEW) | SCN-051-S03 | No sentinel auth/DB-password substring appears in any error returned by `loadAuthConfig`, `Validate`, or `ValidateRuntimeAuthStartup`. The offending KEY names DO appear. |
| T-051-005 | docs-static | [internal/config/docs_required_keys_test.go](../../internal/config/docs_required_keys_test.go) (NEW) | SCN-051-S03 | Both `docs/Deployment.md` and `docs/Operations.md` mention every canonical key name and no forbidden alias. |
| T-051-007 | artifact | [specs/051-deployment-secret-auth-contract/](.) | all | Artifact lint, traceability guard, and state-transition guard pass with EXIT=0. |
| T-051-005-canary | Regression E2E (docs-static canary) | [internal/config/docs_required_keys_test.go](../../internal/config/docs_required_keys_test.go) (NEW) | SCN-051-S03 | Independent canary: docs-static lint still passes against the unmodified `docs/Deployment.md` and `docs/Operations.md` baseline before any new key is introduced — proves the lint is real (would fail if a canonical key is removed). |

### Definition of Done

- [ ] SCN-051-S03: Bootstrap token is required and never logged — T-051-004 passes and proves no auth secret value appears in any returned error.
- [ ] SCN-051-S03: T-051-004 passes and proves no DB password value appears in any returned error.
- [ ] SCN-051-S03: T-051-004 passes and proves the offending KEY names DO appear (so operators can act).
- [ ] SCN-051-S03: T-051-005 passes and proves canonical key names appear in `docs/Deployment.md` and `docs/Operations.md`.
- [ ] SCN-051-S03: T-051-005 passes and proves no forbidden aliases (`auth.signing.hmac_key`, `auth.signing.issuer`, etc.) appear in either doc.
- [ ] T-051-007 passes — artifact lint, traceability guard, and state-transition guard each exit 0.
- [ ] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior in this scope is added/maintained — Scope 3's persistent regression coverage is the new `internal/config/log_redaction_test.go` (security-static) and `internal/config/docs_required_keys_test.go` (docs-static) suites. They run on every `./smackerel.sh test unit --go` invocation and are the authoritative regression gates for every future secret addition (no e2e-api/e2e-ui surface — these are contract tests against in-process error strings and on-disk docs files).
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) with no new failures attributable to Scope 3.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns — T-051-005-canary proves the docs-static lint is real and green against the baseline before any new key is introduced.
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified — Shared Infrastructure Impact Sweep above documents the revert (delete the two new test files; revert the doc paragraph); the docs themselves retain their canonical content.
- [ ] Consumer impact sweep complete: zero stale first-party references remain to the retired auth aliases (`auth.signing.hmac_key`, `auth.signing.issuer`, `signing_secret`, `at_rest_hmac_key`, `bootstrap_secret`, `enrollment_token`) anywhere in `docs/`, verified by `grep -nrE 'auth\.signing\.hmac_key|auth\.signing\.issuer|signing_secret|at_rest_hmac_key|bootstrap_secret|enrollment_token' docs/`.
- [ ] Change Boundary is respected and zero excluded file families were changed — verified by `git diff --name-only HEAD~N HEAD` showing only the allowed file families enumerated in the Change Boundary (Scope 3) section above; no `internal/config/config.go` edits, no `internal/auth/**` edits, no `scripts/commands/config.sh` edits, no source files outside `internal/config/`, no doc files outside `docs/Deployment.md` and `docs/Operations.md`.
