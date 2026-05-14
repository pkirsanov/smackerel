# Bug: BUG-051-001 — Home-lab config bundle emits SMACKEREL_ENV=development, silently disabling production-mode runtime defense-in-depth

## Classification

- **Type:** Code defect — SST loader runtime-mode mapping gap (security-relevant defense-in-depth bypass)
- **Severity:** CRITICAL (the home-lab tailnet bundle with this defect would silently disable every spec 044 + spec 051 production-mode runtime check on the live deploy: `internal/auth/startup.go::ValidateRuntimeAuthStartup` returns `nil` unless `environment=="production"`; `internal/config/config.go` production-mode auth + DB-password fail-fast is gated on `cfg.Environment=="production"`. With `SMACKEREL_ENV=development`, an operator deploying to a real home-lab tailnet endpoint with `auth.enabled=true` would get bundle-generator-side rejection of the dev-default Postgres password (FR-051-005 generator-side guard) but ZERO runtime enforcement of the spec 044 PASETO signing material, the at-rest hashing key, the bootstrap token, or any of the other production-mode invariants. Defense-in-depth collapses to single-layer at the bundle generator. The home-lab readiness review 2026-05-13 surfaced this as **SEC-HL-001**.)
- **Parent Spec:** 051 — Deployment Secret and Auth Contract
- **Workflow Mode:** test-to-doc (parent: home-lab-deploy-readiness-sprint)
- **Status:** Fixed
- **Discovered By:** home-lab readiness review 2026-05-13, finding SEC-HL-001 (cross-referenced from `bubbles.system-review` four-lens synthesis: devops + security + audit + spec-freshness)

## Problem Statement

`scripts/commands/config.sh` resolved `SMACKEREL_ENV` from `config/smackerel.yaml`'s `runtime.environment` (which is `"development"` in the SST) and only overrode it for `TARGET_ENV=test`. There was NO override for `TARGET_ENV=home-lab`, so the home-lab bundle silently emitted `SMACKEREL_ENV=development` into `config/generated/home-lab.env`.

That single value silently disables every production-mode runtime check that spec 044 + spec 051 defense-in-depth depends on:

| Defense-in-depth layer | What it protects | Behavior with `SMACKEREL_ENV=development` |
|---|---|---|
| `internal/auth/startup.go::ValidateRuntimeAuthStartup` | Refuses startup if any spec 044 production signing material is missing or weak (active_private_key, active_key_id, at_rest_hashing_key, bootstrap_token) | Returns `nil` unconditionally — startup proceeds without checking any of the above. |
| `internal/config/config.go::Validate()` production-mode block (lines ~1309-1457) | Refuses startup if `auth.enabled=true` but signing material is empty/placeholder/dev-default | Block is skipped — `cfg.Environment != "production"` short-circuits the gate. |
| `internal/config/secrets.go` production-mode DB password rejection (FR-051-005 RUNTIME side) | Refuses runtime startup if `infrastructure.postgres.password` is a known dev-default value | Skipped — `cfg.Environment != "production"` short-circuits the gate. |
| Spec 044 production-mode PASETO v4 (Ed25519) signing-material requirements | Production deployments MUST have rotation-aware signing keys, never placeholder | All requirements gate on `cfg.Environment=="production"` — never fires. |

The FR-051-005 GENERATOR-side guard at `scripts/commands/config.sh` lines ~415-433 still fires (it gates on `TARGET_ENV` not on `SMACKEREL_ENV`), so the bundle WAS still rejected if the operator left `infrastructure.postgres.password = "smackerel"` in `smackerel.yaml`. That single check is what masked the deeper defect: the operator could not get a bundle past the generator, so the runtime-side bypass never manifested in dev-style smoke testing. But once an operator sets a non-default Postgres password (the obvious "fix" to the generator-side error), the generator emits a bundle with `SMACKEREL_ENV=development` and **every other production-mode runtime check is silently skipped**.

The defect is also strictly broader than any pre-existing FR-051-005 case: it disables the runtime-side enforcement of FR-051-001 (active_private_key), FR-051-002 (active_key_id), FR-051-003 (at_rest_hashing_key), and FR-051-004 (bootstrap_token) all at once. Spec 044 + spec 051 defense-in-depth collapses to single-layer.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | home-lab readiness review 2026-05-13, four-lens system review (devops + security + audit + spec-freshness) |
| Finding ID | SEC-HL-001 |
| File | [scripts/commands/config.sh](../../../../scripts/commands/config.sh) |
| Location | `SMACKEREL_ENV` resolution block (lines ~520-530 pre-fix) — `if [[ "$TARGET_ENV" == "test" ]]; then SMACKEREL_ENV="test"; fi` had no `home-lab` arm |
| Pre-existing test coverage | `internal/config/sst_loader_test.go::TestSSTLoader_RejectsDevPostgresPassword_HomeLab` (FR-051-005 generator-side guard); none exercised the `SMACKEREL_ENV` mapping for home-lab |
| Independent confirmation | `internal/auth/startup.go::ValidateRuntimeAuthStartup` source review confirms the early `if cfg.Environment != "production" { return nil }` gate; `internal/config/config.go` production-mode block (lines ~1309-1457) confirms the `cfg.Environment=="production"` gate |

### Probe outcome (pre-fix)

Running `bash scripts/commands/config.sh --env home-lab --config <yaml-with-strong-postgres-password>` emitted `config/generated/home-lab.env` containing `SMACKEREL_ENV=development`. With this value, the runtime startup path skips:

- `ValidateRuntimeAuthStartup` (returns `nil` immediately).
- The production-mode auth-validate block in `config.go::Validate()`.
- The runtime-side dev-default Postgres password rejection.
- All spec 044 production-mode signing-material checks.

The fix replaces the single `if [[ "$TARGET_ENV" == "test" ]]` form with a per-target `case "$TARGET_ENV" in test) ... ;; home-lab) SMACKEREL_ENV="production" ;; esac` form, gaining the home-lab arm and preserving the test arm. The arm is the smallest mechanically-correct closure of SEC-HL-001 — it gives the home-lab bundle the same runtime-mode invariants the spec 044 + spec 051 production-mode checks always assumed it would have.

## Behavior Contract

**Pre-fix (defect):**
- `bash scripts/commands/config.sh --env home-lab` resolves `SMACKEREL_ENV` from `runtime.environment` in `config/smackerel.yaml` (which is `"development"`).
- The generated `config/generated/home-lab.env` contains `SMACKEREL_ENV=development`.
- A runtime started with that env file silently disables `ValidateRuntimeAuthStartup`, the production-mode auth block in `config.go::Validate()`, the runtime-side dev-default DB-password rejection, and all spec 044 production-mode signing-material requirements.

**Post-fix (required behavior):**
- `bash scripts/commands/config.sh --env home-lab` always sets `SMACKEREL_ENV=production` regardless of `runtime.environment` in the SST.
- The generated `config/generated/home-lab.env` contains `SMACKEREL_ENV=production`.
- A runtime started with that env file fires the spec 044 + spec 051 production-mode defense-in-depth at startup. Missing or placeholder signing material, missing bootstrap token, or dev-default DB password each fail-loud with a clear key-naming error.
- The pre-existing `TARGET_ENV=test` override to `SMACKEREL_ENV=test` is preserved unchanged so integration/e2e/stress runs continue to use the dev-mode warn-and-continue ergonomic.
- The pre-existing FR-051-005 generator-side guard at `config.sh` lines ~415-433 is preserved and continues to fire when an operator leaves the dev-default Postgres password in `smackerel.yaml`.
- A new adversarial test `TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001` (Go driver `internal/config/sst_loader_home_lab_runtime_env_test.go`, shell impl `scripts/commands/config_home_lab_runtime_env_test.sh`) with four sub-cases locks the regression contract.

## Acceptance Criteria

| ID | Criterion |
|---|---|
| BUG-051-001-AC-1 | `scripts/commands/config.sh` per-target case has a `home-lab)` arm that sets `SMACKEREL_ENV="production"`. |
| BUG-051-001-AC-2 | The pre-existing `test)` arm is preserved unchanged (`SMACKEREL_ENV="test"`). |
| BUG-051-001-AC-3 | `bash scripts/commands/config.sh --env home-lab --config <patched-yaml>` (where the yaml has a non-default Postgres password so FR-051-005 does NOT block) emits `SMACKEREL_ENV=production` into `config/generated/home-lab.env`. |
| BUG-051-001-AC-4 | (canary) `bash scripts/commands/config.sh --env dev --config <patched-yaml>` continues to emit `SMACKEREL_ENV=development` into `config/generated/dev.env`. |
| BUG-051-001-AC-5 | (canary) `bash scripts/commands/config.sh --env test --config <patched-yaml>` continues to emit `SMACKEREL_ENV=test` into `config/generated/test.env`. |
| BUG-051-001-AC-6 | (defense-in-depth) `bash scripts/commands/config.sh --env home-lab` against the unpatched live `config/smackerel.yaml` (which still has the dev-default Postgres password `"smackerel"`) is still rejected by the FR-051-005 generator-side guard with a `spec 051` / `infrastructure.postgres.password` attribution and without echoing the literal value. |
| BUG-051-001-AC-7 | A non-tautological adversarial test `TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001` exists with four sub-cases (one per AC-3/AC-4/AC-5/AC-6) that fail if the `home-lab)` arm of the per-target case is removed (Sub-test 1 fails) while canaries (AC-4 / AC-5 / AC-6) continue to PASS — proving the test's isolation properties. |
| BUG-051-001-AC-8 | The pre-existing `internal/config/sst_loader_test.go::TestSSTLoader_RejectsDevPostgresPassword_HomeLab` continues to PASS — the FR-051-005 generator-side guard remains intact. |
| BUG-051-001-AC-9 | The full `internal/config/...` Go test suite remains green; no behavioral regression. No cross-package regression in `internal/auth` or `internal/api`. The full `internal/deploy/...` Go test suite (including BUG-042-001 + BUG-042-002 regression coverage) remains green. |
| BUG-051-001-AC-10 | The fix is the smallest viable change: one per-target case in `scripts/commands/config.sh` (replacing the single-arm `if`), one new shell adversarial test file, one new Go driver file. No production code, compose, runtime config (`config/smackerel.yaml`), or doc files are modified. |
