# User Validation: BUG-029-003 — Dev `docker-compose.yml` violates Gate G028 NO-DEFAULTS via 14 `${VAR:-default}` substitutions

## Checklist

- [x] AC-1: SST emits 5 build-metadata + image-ref vars into the generated env file (verified — see Acceptance Criteria Verification table below)
- [x] AC-2: smackerel-core fail-loud forms (`${X:?...}` build args, `${SMACKEREL_ENV_FILE:?...}`, `${SMACKEREL_CORE_IMAGE?...}` image)
- [x] AC-3: smackerel-ml symmetric fail-loud forms
- [x] AC-4: 4 volume-mount `${X:-default}` retained with inline 11-line documenting comment block
- [x] AC-5: new `internal/deploy/dev_compose_default_fallback_test.go` exists, compiles, gofmt/vet clean
- [x] AC-6: `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` PASSes against the live file
- [x] AC-7: RED→GREEN proof captured (regression of one converted var → test FAILs RED with named-var message; restore → test PASSes)
- [x] AC-8: `docker compose ... config -q` GREEN both env files; RED proof against `/dev/null` exits non-zero with named-var error
- [x] AC-9: 9 pre-existing `TestComposeContract_*` + spec 047 + BUG-047-001 canaries PASS unchanged; cross-package smoke PASS

## Acceptance Criteria Verification

**Claim Source:** executed

| AC | Description | Result | Evidence |
|---|---|---|---|
| AC-1 | `scripts/commands/config.sh` resolves `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`, `SMACKEREL_CORE_IMAGE`, `SMACKEREL_ML_IMAGE` from shell env with safe placeholders if unset, then emits them into the generated env file. | PASS | [report.md](report.md) > Code Diff Evidence > "SST emission additions" + > Validation Evidence > "SST emission" — `grep` against the regenerated `config/generated/dev.env` returns 6 lines (5 new SST-emitted vars + pre-existing `SMACKEREL_ENV_FILE`); same for test.env. |
| AC-2 | `docker-compose.yml` smackerel-core service uses fail-loud `${X:?...}` (colon, fails on unset OR empty) for SMACKEREL_VERSION / SMACKEREL_COMMIT / SMACKEREL_BUILD_TIME build args, `${SMACKEREL_ENV_FILE:?...}` for env_file, and `${SMACKEREL_CORE_IMAGE?...}` (no colon, allows empty) for image. | PASS | [report.md](report.md) > Code Diff Evidence > "Fail-loud forms in `docker-compose.yml`" — `grep -nE 'SMACKEREL_(VERSION\|COMMIT\|BUILD_TIME\|ENV_FILE\|CORE_IMAGE\|ML_IMAGE)' docker-compose.yml` shows the converted forms on smackerel-core lines 59 / 75-77 / 108. |
| AC-3 | Same fail-loud forms applied symmetrically to smackerel-ml service (image / build args / env_file). | PASS | [report.md](report.md) > Code Diff Evidence — same `grep` shows symmetric forms on smackerel-ml lines 146 / 162-164 / 194. |
| AC-4 | `docker-compose.yml` retains exactly four `${X:-default}` occurrences (volume mounts BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR), each immediately preceded by an inline 11-line deferral comment block documenting the load-bearing empty-value contract. | PASS | [report.md](report.md) > Code Diff Evidence > "Volume-mount deferral" — `grep -nE '\$\{[A-Z_]+:-' docker-compose.yml` returns exactly 4 matches (lines 130-133); the comment block at lines 115-125 documents the connector `${X:+/data/...}` consumer pattern. |
| AC-5 | `internal/deploy/dev_compose_default_fallback_test.go` exists, compiles, declares `package deploy`, and reuses the existing `repoRoot(t)` helper from `internal/deploy/compose_contract_test.go`. The new lint test file declares one allowlist (`devComposeDefaultFallbackAllowlist`), one regex (`devComposeDefaultFallbackRegex`), one helper (`findDevComposeUnauthorizedDefaultFallbacks`), and four `Test*` functions. | PASS | [report.md](report.md) > Code Diff Evidence > "New test file structure" — `grep` against the new file shows the declarations at the named line numbers; `go vet ./internal/deploy/...` exits 0; `gofmt -l internal/deploy/` is empty. |
| AC-6 | `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` PASSes against the live `docker-compose.yml`. | PASS | [report.md](report.md) > Validation Evidence > "Targeted dev-compose contract suite" — test logs `contract OK: docker-compose.yml has zero unauthorized ${VAR:-default} forms (allowlist size = 4, ...)`. |
| AC-7 | RED proof captured (scenario-first TDD): temporarily reverting any one of the 10 converted vars to `${X:-default}` form causes `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` to FAIL with the named-var unauthorized-list message + HL-RESCAN-012 breadcrumb. Restoration returns the test to PASS. | PASS | [report.md](report.md) > Test Evidence > "Red→Green proof (scenario-first TDD)" — RED phase shows `docker-compose.yml violates Gate G028 (NO-DEFAULTS / fail-loud SST policy) — HL-RESCAN-012:\n  - 75:${SMACKEREL_VERSION:-dev} (use ${VAR:?error} fail-loud form, or add VAR to devComposeDefaultFallbackAllowlist with justification)`; GREEN phase shows return-to-PASS. |
| AC-8 | Compose substitution GREEN: `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0; same for test.env. Compose substitution RED proof: `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with a Compose error that names at least one of the new fail-loud-substituted vars. | PASS | [report.md](report.md) > Validation Evidence > "Compose substitution GREEN" + "Compose substitution RED proof of fail-loud" — GREEN side both env files exit 0; RED side returns `error while interpolating services.smackerel-core.env_file.[]: required variable SMACKEREL_ENV_FILE is missing a value: must be set — use ./smackerel.sh up or export SMACKEREL_ENV_FILE=config/generated/dev.env` and exits 1. |
| AC-9 | All 9 pre-existing `TestComposeContract_*` adversarial tests + `TestComposeContract_LiveFile` + spec 047 `TestVulnGateContract_*` + BUG-047-001 `TestBundleHashContract_*` PASS unchanged (canary). Cross-package smoke `./smackerel.sh test unit --go` PASS. | PASS | [report.md](report.md) > Validation Evidence > "Targeted prod-compose canary suite" + > "Cross-package smoke" — both sets PASS unchanged; cross-package smoke ends with `[go-unit] go test ./... finished OK`. |

## Bounded-Scope Validation

**Claim Source:** executed

The fix is bounded to:

- **`scripts/commands/config.sh`** (+35 / -0 lines) — purely additive 5-block resolution + 5 emission lines for the build-metadata + image-ref vars
- **`docker-compose.yml`** (+47 / -10 lines) — 10 substitution-form changes (`:-` → `:?` or `?`) plus 11-line volume-mount deferral comment block
- **`internal/deploy/dev_compose_default_fallback_test.go`** (NEW, 270 lines) — persistent in-tree static-file lint test; package-bounded
- **7 BUG-029-003 packet artifacts** under `specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/`

The fix does **not** modify:

- `internal/deploy/compose_contract_test.go` — the prod-compose contract test (different live file target)
- `deploy/compose.deploy.yml` — the prod / home-lab compose file
- `config/smackerel.yaml` — the SST source
- Foreign-owned parent-spec content (`specs/029-devops-pipeline/spec.md`, `design.md`, `scopes.md`, `state.json`, `uservalidation.md`, `report.md`)
- Production runtime Go code under `internal/auth/...`, `internal/config/...`, `internal/api/...`, `cmd/...`
- Python ML sidecar under `ml/...`
- `Dockerfile`, `ml/Dockerfile`
- `.github/workflows/*` (HL-RESCAN-011 is a separate sprint item)
- `scripts/lib/runtime.sh`

**Verification:** `git diff --stat HEAD -- scripts/commands/config.sh docker-compose.yml` returns exactly 2 modified files (`docker-compose.yml | 47 ++++++++++++++++++++++++++++++++++++----------` and `scripts/commands/config.sh | 35 ++++++++++++++++++++++++++++++++++`, totaling 72 insertions / 10 deletions). `git status --porcelain internal/deploy/dev_compose_default_fallback_test.go` shows `?? internal/deploy/dev_compose_default_fallback_test.go` (1 new untracked file).

## Impact on Sanctioned Workflow

**Sanctioned developer workflow** (`./smackerel.sh up`, `./smackerel.sh test`, `./smackerel.sh check`): **unchanged**. The runtime helper `scripts/lib/runtime.sh::smackerel_compose` always invokes Compose with `--env-file config/generated/dev.env`, and the regenerated env file now contains the 5 new SST-emitted vars. Compose substitution validates clean (`config -q` exit 0).

**Direct unsanctioned workflow** (`docker compose -f docker-compose.yml up` without prior `./smackerel.sh config generate`): **was silently broken, is now loud**. Pre-fix, Compose silently substituted `dev / unknown / unknown` for build metadata and `config/generated/dev.env` for `SMACKEREL_ENV_FILE` — masking real misconfiguration. Post-fix, Compose aborts with the named-var error message that points the developer at the fix path (`run ./smackerel.sh config generate or ./smackerel.sh up`). This is the intended Gate G028 fail-loud behavior.

**Operator / production workflow:** **unchanged**. The dev compose file `docker-compose.yml` is not part of the build-once-deploy-many surface; only `deploy/compose.deploy.yml` is. Operators do not consume `docker-compose.yml` in production deploys.

**CI workflow:** **unchanged**. CI's `unit-tests` job already invokes `go test ./...`, which auto-discovers the new `TestDevComposeContract_*` tests. No `.github/workflows/*` change is required.

## Sign-off

**Status: SHIP_IT**

The bug is fixed at three layers (SST emission + Compose substitution form + persistent in-tree lint test). The Gate G028 NO-DEFAULTS / fail-loud SST policy compliance for the dev compose file is now mechanically locked against future drift by `TestDevComposeContract_*` (4 functions + sub-cases). The remaining 4 unconverted occurrences (the volume-mount vars) are explicitly allowlisted with inline 11-line justification + per-var allowlist gate in the test, with a clear follow-up path documented in the deferral comment block.

The change boundary is tight (3 source files), the test surface is purely additive, the canary suite is GREEN, the cross-package smoke is GREEN, and the RED→GREEN proof confirms the test would catch a real-world regression with a precise, actionable failure message.

HL-RESCAN-012 (P3) is closed by this packet. Sequential per-finding sprint continues with HL-RESCAN-013 (`ml/app/auth.py` SMACKEREL_AUTH_TOKEN module-import-time fail-loud) and HL-RESCAN-014 (`cmd/core/helpers.go` delete unused parseFloatEnv / parseJSONArrayEnv / parseJSONObjectEnv) under their own per-finding bug packets. HL-RESCAN-011 (CI workflow restructuring) remains deferred until the parallel session lands the ci.yml work.
