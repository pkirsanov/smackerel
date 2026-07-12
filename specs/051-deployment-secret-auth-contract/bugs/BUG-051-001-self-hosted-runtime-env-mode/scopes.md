# Scopes: BUG-051-001 — self-hosted config bundle emits SMACKEREL_ENV=development

## Scope 1: Per-target self-hosted arm + adversarial 4-sub-test regression contract

**Status:** Done

**Files:**
- [scripts/commands/config.sh](../../../../scripts/commands/config.sh) (per-target case + comment block)
- [scripts/commands/config_self_hosted_runtime_env_test.sh](../../../../scripts/commands/config_self_hosted_runtime_env_test.sh) (NEW — 4-sub-test adversarial driver)
- [internal/config/sst_loader_self_hosted_runtime_env_test.go](../../../../internal/config/sst_loader_self_hosted_runtime_env_test.go) (NEW — Go driver mirroring sst_loader_test.go)

### Use Cases

```gherkin
Feature: self-hosted SST bundle emits production runtime mode for spec 044 + spec 051 defense-in-depth
  Scenario: SCN-051-001-A — TARGET_ENV=self-hosted emits SMACKEREL_ENV=production
    Given an SST yaml where infrastructure.postgres.password is a non-default value
    When scripts/commands/config.sh runs with --env self-hosted against that yaml
    Then config/generated/self-hosted.env contains the line "SMACKEREL_ENV=production"
    And the runtime defense-in-depth gates in internal/auth/startup.go::ValidateRuntimeAuthStartup and internal/config/config.go::Validate() will fire on next runtime startup

  Scenario: SCN-051-001-B — TARGET_ENV=dev still emits SMACKEREL_ENV=development (canary)
    Given an SST yaml where runtime.environment=development
    When scripts/commands/config.sh runs with --env dev against that yaml
    Then config/generated/dev.env contains the line "SMACKEREL_ENV=development"
    And the BUG-051-001 fix has not over-reached to the dev target

  Scenario: SCN-051-001-C — TARGET_ENV=test still emits SMACKEREL_ENV=test (canary, MIT-040-S-004 preserved)
    Given an SST yaml where runtime.environment=development
    When scripts/commands/config.sh runs with --env test against that yaml
    Then config/generated/test.env contains the line "SMACKEREL_ENV=test"
    And the pre-existing MIT-040-S-004 test-mode override is preserved

  Scenario: SCN-051-001-D — Defense-in-depth: FR-051-005 generator-side guard still fires for self-hosted against unpatched live yaml
    Given the live config/smackerel.yaml where infrastructure.postgres.password is the dev-default value "smackerel"
    When scripts/commands/config.sh runs with --env self-hosted against that yaml
    Then the loader exits non-zero
    And stderr names "infrastructure.postgres.password"
    And stderr references "spec 051" or "FR-051-005" or "dev-default" or "password"
    And no env file is produced for self-hosted
```

### Implementation Plan

1. Replace the single-arm `if [[ "$TARGET_ENV" == "test" ]]; then SMACKEREL_ENV="test"; fi` form in `scripts/commands/config.sh` with a per-target `case "$TARGET_ENV" in test) SMACKEREL_ENV="test" ;; self-hosted) SMACKEREL_ENV="production" ;; esac` form. Add a multi-line comment block above the resolution explaining the BUG-051-001 / SEC-HL-001 rationale, the downstream consumers that depend on `SMACKEREL_ENV=production`, and the masking interaction with the FR-051-005 generator-side guard.
2. Author `scripts/commands/config_self_hosted_runtime_env_test.sh` — a 4-sub-test adversarial shell driver that:
   - Backs up `config/generated/{dev,test,self-hosted}.env` (if present) into a temp dir, restores via `trap restore_generated EXIT INT TERM`.
   - Patches a temp copy of `config/smackerel.yaml` via `awk` to replace the 4-space-indented `    password: smackerel` line under `infrastructure: → postgres:` with a non-default value, with a sanity-check `grep` that aborts the test if the patch fails.
   - Sub-test 1: invokes `bash CONFIG_SH --env self-hosted --config TMP_YAML`, asserts `config/generated/self-hosted.env` contains `SMACKEREL_ENV=production`.
   - Sub-test 2 (canary): same for `--env dev`, asserts `SMACKEREL_ENV=development`.
   - Sub-test 3 (canary): same for `--env test`, asserts `SMACKEREL_ENV=test`.
   - Sub-test 4 (defense-in-depth): invokes `bash CONFIG_SH --env self-hosted --config LIVE_YAML` (unpatched), asserts the loader exits non-zero AND stderr matches `spec 051\|FR-051-005\|dev-default\|password`.
   - Exits 0 on full pass, 1 on any failure.
3. Author `internal/config/sst_loader_self_hosted_runtime_env_test.go` — a thin Go driver `TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001` that mirrors `internal/config/sst_loader_test.go`'s `TestSSTLoader_RejectsDevPostgresPassword_SelfHosted` pattern: `runtime.Caller(0)` → repo root → `exec.Command("bash", scriptPath)` with `REPO_ROOT=` env → `t.Logf` the captured output → `t.Fatalf` on non-zero exit.
4. `chmod +x scripts/commands/config_self_hosted_runtime_env_test.sh`.
5. Confine all changes to the three files above plus the bug-packet artifacts in `specs/051-deployment-secret-auth-contract/bugs/BUG-051-001-self-hosted-runtime-env-mode/`. No production runtime code, no compose, no doc edits, no `config/smackerel.yaml` edits.

### Test Plan

- **RED→GREEN proof:** Revert just the `self-hosted)` arm of the per-target case (preserve everything else) and re-run the shell test → Sub-test 1 FAILS with `FAIL: self-hosted.env does NOT contain SMACKEREL_ENV=production — actual: SMACKEREL_ENV=development`. Sub-tests 2, 3, 4 still PASS. Re-apply the arm → all 4 sub-tests PASS. Captured in report.md > Test Evidence > Red→Green proof.
- **Targeted Go-driver run:** `./smackerel.sh test unit --go --test-run 'TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001'` — internal/config package PASS.
- **Full SST-loader regression:** `./smackerel.sh test unit --go --test-run 'TestSSTLoader_'` — both `TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001` and `TestSSTLoader_RejectsDevPostgresPassword_SelfHosted` PASS — proves BUG-051-001 did not break FR-051-005.
- **Cross-package smoke:** `./smackerel.sh test unit --go` covers `internal/config/...`, `internal/auth/...`, `internal/api/...`, `internal/deploy/...` — all PASS. The BUG-042-001 + BUG-042-002 spec 042 contract regression coverage is preserved.
- **Static checks:** `./smackerel.sh lint` and `./smackerel.sh format --check` — both clean.
- **Standalone shell test:** `bash scripts/commands/config_self_hosted_runtime_env_test.sh` — exit 0, 4 PASS lines.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-051-001-A: TARGET_ENV=self-hosted → SMACKEREL_ENV=production | shell-driven Go unit | scripts/commands/config_self_hosted_runtime_env_test.sh + internal/config/sst_loader_self_hosted_runtime_env_test.go | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 1) | YES — fails RED if the self-hosted arm of the per-target case in config.sh is removed | Persistent in-tree shell-driven Go test that runs on every `./smackerel.sh test unit --go` invocation. The SST loader is `scripts/commands/config.sh`, a bash script — the canonical regression suite shape for this surface is the shell-driven-Go-unit pattern (mirrors `internal/config/sst_loader_test.go`). |
| SCN-051-001-B: TARGET_ENV=dev → SMACKEREL_ENV=development | shell-driven Go unit | same as above | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 2) | YES (canary) — proves Sub-test 1 isolated the self-hosted arm. If Sub-test 2 also fails, the fix may have over-reached. | Same as above. |
| SCN-051-001-C: TARGET_ENV=test → SMACKEREL_ENV=test | shell-driven Go unit | same as above | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 3) | YES (canary) — proves the pre-existing MIT-040-S-004 test arm is preserved. If Sub-test 3 fails, MIT-040-S-004 regressed. | Same as above. |
| SCN-051-001-D: Defense-in-depth FR-051-005 generator-side guard preserved for self-hosted | shell-driven Go unit | same as above | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 4) | YES — proves BUG-051-001 did not break the FR-051-005 generator-side guard | Orthogonal regression: pre-existing `TestSSTLoader_RejectsDevPostgresPassword_SelfHosted` in `internal/config/sst_loader_test.go` provides primary coverage for FR-051-005; Sub-test 4 here is a fast in-test sanity that the two layers compose correctly on the self-hosted target. |
| FR-051-005 generator-side rejection (orthogonal) | shell-driven Go unit | scripts/commands/config_secret_rejection_test.sh + internal/config/sst_loader_test.go | TestSSTLoader_RejectsDevPostgresPassword_SelfHosted | YES | Pre-existing spec 051 regression coverage; preserved unchanged. |
| Canary: TARGET_ENV=dev unaffected by BUG-051-001 fix | shell-driven Go unit | scripts/commands/config_self_hosted_runtime_env_test.sh + internal/config/sst_loader_self_hosted_runtime_env_test.go | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 2) | YES (canary) | Runs before broader Go unit suite — proves fix isolation to self-hosted arm. |
| Canary: TARGET_ENV=test MIT-040-S-004 override preserved | shell-driven Go unit | same as above | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 3) | YES (canary) | Runs before broader Go unit suite — proves MIT-040-S-004 preserved. |

### Shared Infrastructure Impact Sweep

`scripts/commands/config.sh` is the SST loader — a shared bootstrap helper consumed by every `./smackerel.sh config generate --env <env>` invocation across all targets (dev, test, self-hosted, and any future production targets). The 1-arm addition to its per-target `case` statement has the following blast radius:

- **Direct downstream consumers (production runtime):** `cmd/core/wiring.go` reads `cfg.Environment`; `internal/auth/startup.go::ValidateRuntimeAuthStartup` gates on `cfg.Environment == "production"` (defense-in-depth that fires at process startup); `internal/config/config.go::Validate()` gates production-mode auth + DB-password fail-fast on the same condition; `internal/api/router.go` bearer middleware reads the value to decide warn-and-continue vs hard-fail on missing token; `internal/config/secrets.go` runtime-side guard rejects dev-default Postgres password when `cfg.Environment == "production"`; `ml/app/main.py` lifespan asserts the same. Every consumer assumed `SMACKEREL_ENV=production` would be present on a real self-hosted deployment — the fix delivers that signal.
- **Test infrastructure (canary surface):** the existing MIT-040-S-004 `TARGET_ENV=test → SMACKEREL_ENV=test` arm in the same case statement is preserved unchanged; Sub-test 3 of the new shell driver is the persistent in-tree canary that proves it.
- **Generated-file contract:** the new arm produces a single new line in `config/generated/self-hosted.env` (`SMACKEREL_ENV=production`); no other env-file contents change. The generator's exit-code contract is unchanged (still 0 on success, non-zero on validation failure).
- **Bootstrap contract for downstream specs:** spec 044 (signing material), spec 051 (auth + secret contracts), and the spec 020 NO-DEFAULTS / fail-loud SST policy all assume the runtime-mode signal is correctly delivered. The fix closes the gap between SST generator-side guards (already correct) and runtime-side guards (which were previously a no-op on self-hosted).
- **Rollback path:** see the corresponding DoD item — single `replace_string_in_file` of the new arm; pre-existing `test)` arm and surrounding code untouched.
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The fix executes once per `./smackerel.sh config generate --env self-hosted` invocation, before any container starts. No long-running daemon state, no shared cache, no cross-process ordering concern.

### Definition of Done

- [x] `scripts/commands/config.sh` per-target case has a `self-hosted)` arm that sets `SMACKEREL_ENV="production"`. [SCN-051-001-A]
   → Evidence: `grep -n 'self-hosted' scripts/commands/config.sh | head -5` returns the arm. See report.md > Implementation Code Diff.
- [x] The pre-existing `test)` arm is preserved unchanged (`SMACKEREL_ENV="test"`). [SCN-051-001-C]
   → Evidence: same grep returns the test arm immediately above the self-hosted arm. See report.md > Implementation Code Diff.
- [x] `bash scripts/commands/config.sh --env self-hosted --config <patched-yaml>` emits `SMACKEREL_ENV=production`. [SCN-051-001-A]
   → Evidence: shell test Sub-test 1 PASS. See report.md > Test Evidence > Green state output.
- [x] `bash scripts/commands/config.sh --env dev --config <patched-yaml>` continues to emit `SMACKEREL_ENV=development`. [SCN-051-001-B]
   → Evidence: shell test Sub-test 2 PASS. See report.md > Test Evidence > Green state output.
- [x] `bash scripts/commands/config.sh --env test --config <patched-yaml>` continues to emit `SMACKEREL_ENV=test`. [SCN-051-001-C]
   → Evidence: shell test Sub-test 3 PASS. See report.md > Test Evidence > Green state output.
- [x] `bash scripts/commands/config.sh --env self-hosted` against the unpatched live yaml is still rejected by the FR-051-005 generator-side guard with `spec 051` / `infrastructure.postgres.password` attribution and without echoing the literal dev-default value. [SCN-051-001-D]
   → Evidence: shell test Sub-test 4 PASS. See report.md > Test Evidence > Green state output.
- [x] A non-tautological adversarial test `TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001` exists with four sub-cases (one per AC-3/AC-4/AC-5/AC-6) that fail Sub-test 1 RED if the self-hosted arm of the per-target case is removed (and Sub-tests 2/3/4 continue to PASS — proving isolation). [SCN-051-001-A through SCN-051-001-D]
   → Evidence: the RED state was empirically captured by reverting the self-hosted arm and re-running the shell test — Sub-test 1 FAIL, Sub-tests 2/3/4 PASS. See report.md > Test Evidence > Red→Green proof.
- [x] The pre-existing `internal/config/sst_loader_test.go::TestSSTLoader_RejectsDevPostgresPassword_SelfHosted` continues to PASS. [SCN-051-001-D]
   → Evidence: full Go unit suite PASS for `internal/config`. See report.md > Test Evidence > Cross-package smoke.
- [x] The full `internal/config/...` Go test suite remains green; no behavioral regression. [SCN-051-001-A through SCN-051-001-D]
   → Evidence: `./smackerel.sh test unit --go` shows `ok github.com/smackerel/smackerel/internal/config 16.565s`. See report.md > Test Evidence > Cross-package smoke.
- [x] No cross-package regression. The full `internal/deploy/...` Go test suite (including BUG-042-001 + BUG-042-002 regression coverage) remains green. [SCN-051-001-A through SCN-051-001-D]
   → Evidence: `./smackerel.sh test unit --go` shows `ok ... internal/deploy ...` and `ok ... internal/auth ...` and `ok ... internal/api ...`. See report.md > Test Evidence > Cross-package smoke.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. [SCN-051-001-A through SCN-051-001-D]
   → Evidence: persistent in-tree `TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001` with four sub-cases — one per scenario — runs on every `./smackerel.sh test unit --go` invocation. The SST loader is `scripts/commands/config.sh`, a bash script — the canonical regression suite shape for this surface is the shell-driven-Go-unit pattern (mirrors `internal/config/sst_loader_test.go`'s `TestSSTLoader_RejectsDevPostgresPassword_SelfHosted`). See report.md > Regression Evidence.
- [x] Broader E2E regression suite passes — full `internal/config/...` + `internal/auth/...` + `internal/api/...` + `internal/deploy/...` Go test suites all PASS, including the BUG-042-001 + BUG-042-002 spec 042 contract regression coverage.
   → Evidence: `./smackerel.sh test unit --go` returns `ok` for every package across the `internal/...` tree. See report.md > Regression Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. [SCN-051-001-B + SCN-051-001-C]
   → Evidence: shell test Sub-tests 2 (TARGET_ENV=dev → SMACKEREL_ENV=development) and 3 (TARGET_ENV=test → SMACKEREL_ENV=test) are isolated canary cases that PASS before the broader Go unit suite reruns. They prove the BUG-051-001 fix did not over-reach to the dev or test target — if either canary failed, the fix would be rejected and reverted before any broad suite was touched. See report.md > Test Evidence > Green state output.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: the BUG-051-001 fix is a 1-arm addition to a `case` statement in `scripts/commands/config.sh`. Rollback is a single `replace_string_in_file` of the `self-hosted) SMACKEREL_ENV="production" ;;` line, leaving the pre-existing `test)` arm and surrounding code untouched. The shell test driver `scripts/commands/config_self_hosted_runtime_env_test.sh` backs up `config/generated/{dev,test,self-hosted}.env` into a tmpdir on entry and restores them via `trap restore_generated EXIT INT TERM` — verified by re-running `./smackerel.sh config generate --env dev` after the test driver and confirming the dev-env file is unchanged from its pre-test state. See report.md > Code Diff Evidence + Test Evidence > Red→Green proof.
- [x] Scenario-specific adversarial regression tests for EVERY new/changed behavior. [SCN-051-001-A through SCN-051-001-D]
   → Evidence: persistent in-tree `TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001` with four sub-cases — one per scenario — runs on every `./smackerel.sh test unit --go` invocation. See report.md > Regression Evidence.
- [x] Static checks: `./smackerel.sh lint` clean and `./smackerel.sh format --check` clean.
   → Evidence: both commands exit 0 with no diagnostics. See report.md > Validation Evidence.
- [x] Consumer impact sweep complete. The fix is confined to the SST loader bash script and the new test files; no production code consumers are affected. The downstream consumers of `SMACKEREL_ENV` (`internal/auth/startup.go::ValidateRuntimeAuthStartup`, `internal/config/config.go::Validate()`, `internal/config/secrets.go` runtime-side guard, ml/app/main.py lifespan) all continue to honor the spec 044 + spec 051 production-mode contract — the fix gives them the runtime-mode signal they always assumed they would receive.
   → Evidence: `grep -rn 'SMACKEREL_ENV\|cfg.Environment' --include='*.go' --include='*.py' .` shows the consumers all read the value via the standard env or config path. See report.md > Consumer Impact Sweep.
- [x] Change Boundary respected. The fix is confined to one production-script edit (`scripts/commands/config.sh`) plus two new test files (one shell, one Go) plus the seven bug-packet artifacts. No production runtime code, no compose, no doc edits, no `config/smackerel.yaml` edits.
   → Evidence: `git status --short` shows the bounded change set. See report.md > Code Diff Evidence.
- [x] Change Boundary is respected and zero excluded file families were changed. [Allowed file families + Excluded surfaces enumerated below]
   → Evidence: `git status --short` shows only allowed-family files (the SST loader script, two new test files, and bug-packet artifacts under `specs/051-deployment-secret-auth-contract/bugs/BUG-051-001-self-hosted-runtime-env-mode/`). Zero changes to excluded surfaces (no production runtime Go/Python code, no `docker-compose*.yml`, no `deploy/**`, no `docs/**`, no `config/smackerel.yaml`, no other `specs/**`). See report.md > Code Diff Evidence.

### Change Boundary

**Allowed file families (this fix may modify):**

- `scripts/commands/config.sh` — the SST loader script being patched (BUG-051-001 fix point)
- `scripts/commands/config_self_hosted_runtime_env_test.sh` (NEW) — the adversarial 4-sub-test shell driver
- `internal/config/sst_loader_self_hosted_runtime_env_test.go` (NEW) — the Go unit-test driver that wraps the shell driver into the `./smackerel.sh test unit --go` suite
- `specs/051-deployment-secret-auth-contract/bugs/BUG-051-001-self-hosted-runtime-env-mode/**` — this bug packet's seven artifacts (spec / design / scopes / scenario-manifest / report / uservalidation / state)

**Excluded surfaces (this fix MUST NOT touch):**

- Production runtime Go code under `internal/auth/...`, `internal/config/...` (other than the new `*_test.go` driver above), `internal/api/...`, `internal/deploy/...`, `cmd/...` — the runtime defense-in-depth code is already correct; the bug is purely a missing per-target arm in the SST loader
- Python ML sidecar under `ml/...` — the lifespan-mode assertion is already correct
- `docker-compose*.yml` and `deploy/**` — the compose contract is unrelated to this fix
- `config/smackerel.yaml` — the SST source is intentionally `runtime.environment: development` for dev ergonomics; the fix is the per-target override layer in the loader, NOT the SST source
- `docs/**` — docs follow a separate close-out cadence
- Any other `specs/**` directory — single-bug-scope discipline


### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-051-001-A: TARGET_ENV=self-hosted → SMACKEREL_ENV=production | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 1) | scripts/commands/config_self_hosted_runtime_env_test.sh + internal/config/sst_loader_self_hosted_runtime_env_test.go | shell-driven Go unit | YES — fails RED if self-hosted arm removed |
| SCN-051-001-B: TARGET_ENV=dev → SMACKEREL_ENV=development (canary) | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 2) | same as above | shell-driven Go unit | YES (canary) |
| SCN-051-001-C: TARGET_ENV=test → SMACKEREL_ENV=test (canary, MIT-040-S-004 preserved) | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 3) | same as above | shell-driven Go unit | YES (canary) |
| SCN-051-001-D: Defense-in-depth FR-051-005 generator-side preserved for self-hosted | TestSSTLoader_SelfHostedEmitsProductionRuntimeEnv_BUG051001 (Sub-test 4) | same as above | shell-driven Go unit | YES |
| FR-051-005 generator-side rejection (orthogonal) | TestSSTLoader_RejectsDevPostgresPassword_SelfHosted | scripts/commands/config_secret_rejection_test.sh + internal/config/sst_loader_test.go | shell-driven Go unit | YES (pre-existing) |
