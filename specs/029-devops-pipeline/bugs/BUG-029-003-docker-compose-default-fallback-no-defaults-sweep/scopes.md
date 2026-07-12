# Scopes: BUG-029-003 — Dev `docker-compose.yml` violates Gate G028 NO-DEFAULTS via 14 `${VAR:-default}` substitutions

## Scope 1: Convert 10 of 14 dev-compose `${VAR:-default}` occurrences to fail-loud forms via SST emission, defer 4 volume-mount cases with inline documentation, and add an adversarial static-file lint test

**Status:** Done

**Files:**
- [scripts/commands/config.sh](../../../../scripts/commands/config.sh) (5-block resolution + 5 emission lines for `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`, `SMACKEREL_CORE_IMAGE`, `SMACKEREL_ML_IMAGE`)
- [docker-compose.yml](../../../../docker-compose.yml) (10 `${X:-default}` occurrences converted to `${X:?...}` or `${X?...}`; 4 volume-mount occurrences left with inline 11-line deferral comment)
- [internal/deploy/dev_compose_default_fallback_test.go](../../../../internal/deploy/dev_compose_default_fallback_test.go) (NEW; 1 lint test + 3 adversarial sub-cases)

### Use Cases

```gherkin
Feature: Dev compose file uses fail-loud SST substitutions for build metadata, env-file path, and image refs (Gate G028)
  Scenario: SCN-029-003-A — config generate emits build-metadata + image-ref vars into dev.env
    Given a developer invokes `bash scripts/commands/config.sh --env dev`
    When the SST generator runs against config/smackerel.yaml
    Then config/generated/dev.env contains lines for SMACKEREL_VERSION, SMACKEREL_COMMIT, SMACKEREL_BUILD_TIME, SMACKEREL_CORE_IMAGE, SMACKEREL_ML_IMAGE (and SMACKEREL_ENV_FILE which already existed)
    And the values are safe placeholders (`dev` / `unknown` / `unknown` / empty / empty) when no shell env override is present
    And shell env overrides (e.g., CI's SMACKEREL_VERSION=v1.2.3) take precedence

  Scenario: SCN-029-003-B — Compose substitution succeeds for the regenerated env file
    Given config/generated/dev.env contains the 5 new SST-emitted vars + the pre-existing SMACKEREL_ENV_FILE line
    When `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` runs
    Then it exits 0
    And the same check against config/generated/test.env also exits 0

  Scenario: SCN-029-003-C — Compose fails loud when the env file is missing or empty (RED proof)
    Given a developer invokes `docker compose --env-file /dev/null -f docker-compose.yml config -q`
    When Compose attempts variable substitution
    Then it exits non-zero
    And the error message names at least one of [SMACKEREL_VERSION, SMACKEREL_COMMIT, SMACKEREL_BUILD_TIME, SMACKEREL_ENV_FILE, SMACKEREL_CORE_IMAGE, SMACKEREL_ML_IMAGE] AS missing
    And the error message contains "Gate G028" or "HL-RESCAN-012" or "./smackerel.sh config generate"

  Scenario: SCN-029-003-D — TestDevComposeContract_NoUnauthorizedDefaultFallbacks rejects regression of any build-metadata / env-file / image-ref var to `:-` form
    Given the live docker-compose.yml is bit-identical to the post-fix state
    When `go test -run TestDevComposeContract_NoUnauthorizedDefaultFallbacks` runs
    Then it PASSes (zero unauthorized matches)
    And reverting any one of SMACKEREL_VERSION/COMMIT/BUILD_TIME/ENV_FILE/CORE_IMAGE/ML_IMAGE to `${X:-default}` form causes the test to FAIL with a message naming the regression-target var and HL-RESCAN-012

  Scenario: SCN-029-003-E — Adversarial test catches a novel non-allowlisted var in `:-` form
    Given a synthetic compose YAML fixture where a novel var (e.g., FAKE_VAR) appears in `${FAKE_VAR:-fallback}` form
    When findDevComposeUnauthorizedDefaultFallbacks runs against the fixture
    Then it returns a non-empty unauthorized list naming FAKE_VAR

  Scenario: SCN-029-003-F — Adversarial test enforces per-var (not per-line) allowlist gating
    Given a synthetic compose YAML fixture where an allowlisted var (e.g., BOOKMARKS_IMPORT_DIR) appears in `:-` form on the same line as a non-allowlisted var (e.g., FAKE_VAR) in `:-` form
    When findDevComposeUnauthorizedDefaultFallbacks runs against the fixture
    Then it returns an unauthorized list naming FAKE_VAR (not BOOKMARKS_IMPORT_DIR)

  Scenario: SCN-029-003-G — Comment-line skip prevents false positives from Gate G028 documentation
    Given a synthetic compose YAML fixture with a comment line "# Gate G028 forbids ${VAR:-default} form"
    When findDevComposeUnauthorizedDefaultFallbacks runs against the fixture
    Then it returns an empty unauthorized list (the comment line is skipped)
```

### Implementation Plan

1. **`scripts/commands/config.sh` — Insert A (resolution block):** Append a 5-block `if [[ -z "${X+set}" ]]; then X="default"; fi` resolution section before the `mkdir -p "$REPO_ROOT/config/generated"` line. The blocks resolve `SMACKEREL_VERSION` (default `dev`), `SMACKEREL_COMMIT` (default `unknown`), `SMACKEREL_BUILD_TIME` (default `unknown`), `SMACKEREL_CORE_IMAGE` (default empty), `SMACKEREL_ML_IMAGE` (default empty). Lead with a comment block explaining the HL-RESCAN-012 / Gate G028 chain and the CI export precedence.
2. **`scripts/commands/config.sh` — Insert B (heredoc emission):** Inside the heredoc `cat > "$OUTPUT_FILE" <<EOF`, after the `SMACKEREL_ENV_FILE=...` line, add 5 emission lines for the same 5 vars. Each line is `SMACKEREL_X=${SMACKEREL_X}`.
3. **`docker-compose.yml` — Build-metadata + env-file conversion (smackerel-core):** convert `image: ${SMACKEREL_CORE_IMAGE:-}` → `image: ${SMACKEREL_CORE_IMAGE?must be set in env file (run ./smackerel.sh config generate); empty value is allowed for build-from-source dev}`; convert build-args VERSION/COMMIT_HASH/BUILD_TIME from `:-dev` / `:-unknown` / `:-unknown` to `:?must be set in env file (run ./smackerel.sh config generate)`; convert env_file `${SMACKEREL_ENV_FILE:-config/generated/dev.env}` → `${SMACKEREL_ENV_FILE:?must be set — use ./smackerel.sh up or export SMACKEREL_ENV_FILE=config/generated/dev.env}`. Lead with a one-line comment "# HL-RESCAN-012 / Gate G028 — fail-loud SST substitution".
4. **`docker-compose.yml` — Build-metadata + env-file conversion (smackerel-ml):** mechanically same as smackerel-core for `SMACKEREL_ML_IMAGE`, the build args, and the env_file reference.
5. **`docker-compose.yml` — Volume-mount inline comment:** add an 11-line comment block immediately above the four volume-mount lines (BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR) explaining (a) why these are not converted, (b) the load-bearing empty-value contract via the `${X:+/data/...}` env override pattern in connector code, (c) the subsequent connector-refactor packet (filed under its own bug outside this sweep).
6. **`internal/deploy/dev_compose_default_fallback_test.go` (NEW FILE):** declare `package deploy` and import fmt, os, path/filepath, regexp, sort, strings, testing. Define `devComposeDefaultFallbackAllowlist` map[string]string with the four volume-mount vars + justifications. Define `devComposeDefaultFallbackRegex = regexp.MustCompile(\`\$\{([A-Z][A-Z0-9_]*):-[^}]*\}\`)`. Implement `findDevComposeUnauthorizedDefaultFallbacks(yamlBytes []byte, allowlist map[string]string) []string` (line-by-line scan, skip comment lines, regex-match, filter via allowlist, return sorted dedup `<lineNum>: <fullMatch>` strings). Implement four test functions: `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` (live-file scan), `TestDevComposeContract_AdversarialUnauthorizedDefaultFallback` (4 sub-cases for build-metadata regression / env-file regression / image-ref regression / novel non-allowlisted var), `TestDevComposeContract_AdversarialAllowlistRespected` (per-var-not-per-line gating), `TestDevComposeContract_AdversarialCommentLinesIgnored` (comment-line skip). HL-RESCAN-012 attribution in package docstring + each `t.Fatalf` failure-case message.
7. **RED→GREEN proof (scenario-first TDD):** before committing, capture (a) the live-file PASS, (b) a temporary regression of one converted var to `:-` form → live-file test FAILS RED with the expected unauthorized-list message, (c) restoration → live-file test back to GREEN. Capture (d) `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with the named-var error (RED proof of fail-loud Compose substitution), and (e) `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0 (GREEN proof of well-formed env file).
8. **Confine the change boundary:** the only files modified are `scripts/commands/config.sh`, `docker-compose.yml`, the new `internal/deploy/dev_compose_default_fallback_test.go`, and the seven BUG-029-003 packet artifacts under `specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/`. No production runtime Go/Python code, no `deploy/compose.deploy.yml` change, no foreign-owned `specs/**` directory, no CI workflow change.

### Test Plan

- **Targeted dev-compose contract suite:** `go test -count=1 -v -run '^TestDevComposeContract' ./internal/deploy/...` runs all 4 new test functions plus all sub-tests — every one PASS in <1s wall-clock.
- **Targeted prod-compose canary suite:** `go test -count=1 -v -run '^TestComposeContract' ./internal/deploy/...` runs all 9 pre-existing top-level tests plus all sub-tests — every one PASS unchanged (the new dev-compose test is purely additive against a different file and does not over-reach into the prod-compose contract surface).
- **Cross-package smoke:** `./smackerel.sh test unit --go` covers the full unit suite — all PASS, no regression.
- **Static checks:** `go vet ./internal/deploy/...` exit 0; `gofmt -l internal/deploy/` empty.
- **SST emission:** `bash scripts/commands/config.sh --env dev` regenerates `config/generated/dev.env`. `grep -nE '^SMACKEREL_(VERSION|COMMIT|BUILD_TIME|CORE_IMAGE|ML_IMAGE|ENV_FILE)=' config/generated/dev.env` returns 6 lines proving the SST emission. Same for `--env test`.
- **Compose substitution (GREEN):** `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0. Same for test.env.
- **Compose substitution (RED proof of fail-loud):** `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with the named-var error message proving Gate G028 fail-loud is now active.
- **Lint regression test (RED proof of test):** revert one of the converted vars (e.g., `${SMACKEREL_VERSION:?...}` → `${SMACKEREL_VERSION:-dev}`), re-run `TestDevComposeContract_NoUnauthorizedDefaultFallbacks`, observe FAIL with `unauthorized default-fallback occurrence(s) in docker-compose.yml ... HL-RESCAN-012` message, restore via `replace_string_in_file`, re-run, observe PASS.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-029-003-A: SST emits 5 build-metadata + image-ref vars | execution evidence | (manual) | bash scripts/commands/config.sh --env dev → grep dev.env | NO (positive) | Captured in report.md > Validation Evidence > SST emission |
| SCN-029-003-B: Compose substitution GREEN | execution evidence | (manual) | docker compose --env-file config/generated/dev.env config -q | NO (positive) | Captured in report.md > Validation Evidence > Compose substitution GREEN |
| SCN-029-003-C: Compose substitution RED on missing env (fail-loud) | execution evidence | (manual) | docker compose --env-file /dev/null config -q | YES — exits non-zero with named-var error | Captured in report.md > Validation Evidence > Compose substitution RED proof |
| SCN-029-003-D: Lint test rejects regression of converted var | unit (Go static-file lint) | internal/deploy/dev_compose_default_fallback_test.go | TestDevComposeContract_AdversarialUnauthorizedDefaultFallback (4 sub-cases) | YES — fails RED if any converted var is reverted to `:-` form | Persistent in-tree adversarial Go test that runs on every `./smackerel.sh test unit --go` invocation. |
| SCN-029-003-D / live-file canary | unit (Go static-file lint) | same as above | TestDevComposeContract_NoUnauthorizedDefaultFallbacks | NO (positive canary against live file) | Same as above. |
| SCN-029-003-E: Adversarial novel non-allowlisted var | unit (Go static-file lint) | same as above | TestDevComposeContract_AdversarialUnauthorizedDefaultFallback (novel-var sub-case) | YES | Same as above. |
| SCN-029-003-F: Per-var (not per-line) allowlist gating | unit (Go static-file lint) | same as above | TestDevComposeContract_AdversarialAllowlistRespected | YES | Same as above. |
| SCN-029-003-G: Comment-line skip | unit (Go static-file lint) | same as above | TestDevComposeContract_AdversarialCommentLinesIgnored | YES | Same as above. |
| Canary: TestComposeContract_LiveFile preserved | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_LiveFile | NO (positive canary) | Pre-existing spec 042 contract; preserved unchanged. |
| Canary: BUG-042-001..005 prod-compose adversarials preserved | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialLiteralBind, AdversarialInfraHasPorts, AdversarialMultiPortsBypass, AdversarialMLMultiPortsBypass, AdversarialNetworkModeHostBypass, AdversarialOllamaLiteralBind, AdversarialDefaultFallbackBind, AdversarialPrometheusLiteralBindAndFallbackForms | YES (canaries) | Pre-existing prod-compose contract; preserved unchanged. |

### Definition of Done

- [x] `scripts/commands/config.sh` resolves `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`, `SMACKEREL_CORE_IMAGE`, `SMACKEREL_ML_IMAGE` from shell env with safe seed values if unset (`if [[ -z "${X+set}" ]]; then X="default"; fi`). [SCN-029-003-A]
   → Evidence: `grep -n 'SMACKEREL_VERSION\|SMACKEREL_COMMIT\|SMACKEREL_BUILD_TIME\|SMACKEREL_CORE_IMAGE\|SMACKEREL_ML_IMAGE' scripts/commands/config.sh` returns the 5-block resolution. See report.md > Code Diff Evidence.
- [x] `scripts/commands/config.sh` heredoc emits the 5 vars into `config/generated/<env>.env`. [SCN-029-003-A]
   → Evidence: `grep -n 'SMACKEREL_VERSION=\|SMACKEREL_COMMIT=\|SMACKEREL_BUILD_TIME=\|SMACKEREL_CORE_IMAGE=\|SMACKEREL_ML_IMAGE=' scripts/commands/config.sh` returns the 5 emission lines inside the heredoc. See report.md > Code Diff Evidence.
- [x] `bash scripts/commands/config.sh --env dev` regenerates `config/generated/dev.env` with the 5 new SST-emitted vars + the pre-existing SMACKEREL_ENV_FILE line (6 total). [SCN-029-003-A]
   → Evidence: `grep -nE '^SMACKEREL_(VERSION|COMMIT|BUILD_TIME|CORE_IMAGE|ML_IMAGE|ENV_FILE)=' config/generated/dev.env` returns 6 lines. See report.md > Validation Evidence > SST emission.
- [x] Same emission flows through for `--env test`. [SCN-029-003-A]
   → Evidence: same grep against `config/generated/test.env` returns 6 lines. See report.md > Validation Evidence > SST emission.
- [x] `docker-compose.yml` smackerel-core service uses `${SMACKEREL_CORE_IMAGE?...}` (no colon, allows empty for build-from-source) for the image ref. [SCN-029-003-D]
   → Evidence: `grep -n 'SMACKEREL_CORE_IMAGE' docker-compose.yml` shows the no-colon `?` form. See report.md > Code Diff Evidence.
- [x] `docker-compose.yml` smackerel-core service uses `${X:?...}` (colon, fails on unset OR empty) for SMACKEREL_VERSION, SMACKEREL_COMMIT, SMACKEREL_BUILD_TIME build args. [SCN-029-003-D]
   → Evidence: `grep -n 'SMACKEREL_VERSION:\?\|SMACKEREL_COMMIT:\?\|SMACKEREL_BUILD_TIME:\?' docker-compose.yml` shows 6 hits (2 services × 3 vars). See report.md > Code Diff Evidence.
- [x] `docker-compose.yml` smackerel-core service uses `${SMACKEREL_ENV_FILE:?...}` (colon, fails on unset OR empty) for env_file reference. [SCN-029-003-D]
   → Evidence: `grep -n 'SMACKEREL_ENV_FILE' docker-compose.yml` shows the colon-? form on both services (2 hits). See report.md > Code Diff Evidence.
- [x] Same fail-loud forms applied symmetrically to smackerel-ml service (image / build args / env_file). [SCN-029-003-D]
   → Evidence: same grep covers both services. See report.md > Code Diff Evidence.
- [x] `docker-compose.yml` retains exactly four `${X:-default}` occurrences (volume mounts BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR), each immediately preceded by an inline 11-line deferral comment block. [DD-4]
   → Evidence: `grep -nE '\$\{[A-Z_]+:-' docker-compose.yml` returns 4 matches; `grep -B11 'BOOKMARKS_IMPORT_DIR:-' docker-compose.yml` returns the comment block. See report.md > Code Diff Evidence.
- [x] `internal/deploy/dev_compose_default_fallback_test.go` exists, compiles, and declares `package deploy`. [SCN-029-003-D-G]
   → Evidence: `go test -run TestDevComposeContract ./internal/deploy/...` discovers and runs the new tests. See report.md > Test Evidence > Targeted dev-compose contract suite.
- [x] `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` PASSes against the live `docker-compose.yml`. [SCN-029-003-D]
   → Evidence: same go test invocation. See report.md > Test Evidence > Targeted dev-compose contract suite.
- [x] `TestDevComposeContract_AdversarialUnauthorizedDefaultFallback` has 4 sub-cases (build-metadata / env-file / image-ref / novel non-allowlisted var) and all PASS — each fixture causes `findDevComposeUnauthorizedDefaultFallbacks` to return a non-empty unauthorized list. [SCN-029-003-D, E]
   → Evidence: `go test -count=1 -v -run TestDevComposeContract_AdversarialUnauthorizedDefaultFallback ./internal/deploy/...` shows 4 `--- PASS:` sub-case lines. See report.md > Test Evidence.
- [x] `TestDevComposeContract_AdversarialAllowlistRespected` PASSes — adversarial test enforces per-var (not per-line) allowlist gating; it builds a fixture where an allowlist member (e.g., BOOKMARKS_IMPORT_DIR) and a non-allowlist var (e.g., FAKE_VAR) both appear in `:-` form on the same line, and proves the lint catches the non-allowlist var while skipping the allowlist member. [SCN-029-003-F]
   → Evidence: same go test invocation. See report.md > Test Evidence.
- [x] `TestDevComposeContract_AdversarialCommentLinesIgnored` PASSes — comment-line skip prevents false positives from Gate G028 documentation; it builds a fixture with a comment line containing the literal `${VAR:-default}` text (mimicking Gate G028 explanatory documentation in the compose file) and proves the lint skips comment lines so documentation never trips a false positive. [SCN-029-003-G]
   → Evidence: same go test invocation. See report.md > Test Evidence.
- [x] HL-RESCAN-012 attribution is present in either the test docstring or the failure-case `t.Fatalf` message. [SCN-029-003-D-G]
   → Evidence: `grep -n 'HL-RESCAN-012' internal/deploy/dev_compose_default_fallback_test.go` returns multiple hits (package docstring + failure-case messages). See report.md > Code Diff Evidence.
- [x] RED proof captured (scenario-first TDD): temporarily reverting any one of the 10 converted vars (e.g., `${SMACKEREL_VERSION:?...}` → `${SMACKEREL_VERSION:-dev}`) causes `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` to FAIL with the expected unauthorized-list message naming the regressed var. Restoration returns the test to PASS. [SCN-029-003-D]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- [x] Compose substitution GREEN: `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0; same for test.env. [SCN-029-003-B]
   → Evidence: see report.md > Validation Evidence > Compose substitution GREEN.
- [x] Compose substitution RED proof: `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with a Compose error message that names at least one of the new fail-loud-substituted vars, proving Gate G028 fail-loud is now active. [SCN-029-003-C]
   → Evidence: see report.md > Validation Evidence > Compose substitution RED proof.
- [x] `TestComposeContract_LiveFile` and the eight pre-existing `TestComposeContract_*` adversarial tests continue to PASS GREEN unchanged. [Canary]
   → Evidence: `go test -count=1 -v -run '^TestComposeContract' ./internal/deploy/...` shows all 9 PASS. See report.md > Validation Evidence > Targeted prod-compose canary suite.
- [x] Cross-package smoke clean: full `./smackerel.sh test unit --go` PASS. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Cross-package smoke.
- [x] Static checks: `go vet ./internal/deploy/...` exit 0; `gofmt -l internal/deploy/` empty. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Static checks.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — persistent in-tree `TestDevComposeContract_*` (4 functions + sub-cases) runs on every `./smackerel.sh test unit --go` invocation (CI + developer pre-push). The dev compose contract surface is a static-file invariant; the regression suite IS the Go test suite itself. Compose substitution evidence is supplementary execution-time proof. [SCN-029-003-D-G]
   → Evidence: see report.md > Audit Evidence > Regression Evidence.
- [x] Broader E2E regression suite passes — full `internal/deploy/...` Go test suite plus cross-package smoke against every other internal/* package PASS, including the BUG-042-001..005 prod-compose contract canary. [Broader regression]
   → Evidence: `./smackerel.sh test unit --go` returns `[go-unit] go test ./... finished OK`. See report.md > Audit Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. [Spec 042 + BUG-042-001..005 prod-compose canary + spec 047 vuln-gate canary + BUG-047-001 bundle-hash canary]
   → Evidence: targeted `^TestComposeContract|^TestVulnGateContract|^TestBundleHashContract` suite runs all canaries PASS unchanged. The new dev-compose test is purely additive against a different file (`docker-compose.yml`), so the canaries cannot regress as a side effect.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: rollback is a single `git revert` of the BUG-029-003 commit. The change is bounded to (a) one new resolution+emission block in `scripts/commands/config.sh` (purely additive at SST-emission time), (b) ten substitution-form changes in `docker-compose.yml` (revert restores `:-` form), (c) one new test file (revert removes it). No production runtime Go/Python code, no live deploy/compose.deploy.yml, no schema migration. Verified by the RED proof step which temporarily reverts one of the converted forms, observes expected FAIL output, then restores.
- [x] Consumer impact sweep complete and zero stale first-party references remain. [Consumer Impact Sweep]
   → Evidence: see Consumer Impact Sweep section below.
- [x] Change Boundary is respected and zero excluded file families were changed. [Change Boundary]
   → Evidence: see Change Boundary section below; `git diff --stat` reports exactly three source files modified (`scripts/commands/config.sh`, `docker-compose.yml`, `internal/deploy/dev_compose_default_fallback_test.go`) plus the seven BUG-029-003 packet artifacts. Every file family in the Excluded surfaces list is bit-identical to HEAD.
- [x] Stress coverage assessment (Gate G026): explicit stress/load coverage is NOT REQUIRED for this fix. The change is a static-file invariant test + SST emission additions + Compose substitution-form changes; there is no latency, throughput, p95/p99, response-time, sla, or slo dimension that the change can move. The test runs in <1s wall-clock; no daemon, no concurrency, no sustained load. This DoD line documents the assessment for the Gate G026 lint. [Broader regression]
   → Evidence: the new tests run in <1s (see Validation Evidence > Targeted dev-compose contract suite). No stress dimension applies.

### Shared Infrastructure Impact Sweep

`scripts/commands/config.sh` is the **SST generator**, the canonical helper that produces every developer's `config/generated/<env>.env` file from `config/smackerel.yaml`. Changes to its emission surface affect every developer who runs `./smackerel.sh up`, `./smackerel.sh test`, or `./smackerel.sh check`. `docker-compose.yml` is the **dev-stack compose file**, the entrypoint Compose file that `./smackerel.sh up` invokes. The BUG-029-003 fix has the following blast radius:

- **Direct downstream consumers:** every developer + every CI job that runs `./smackerel.sh up` (or `./smackerel.sh test`) executes `scripts/lib/runtime.sh::smackerel_compose` which always passes `--env-file config/generated/dev.env` to Compose. The new `${X:?...}` and `${X?...}` substitution forms in `docker-compose.yml` are satisfied by the SST-emitted env file (proven by `docker compose config -q` exit 0). Sanctioned workflow ergonomics unchanged.
- **Indirect downstream consumers:** developers who run `docker compose -f docker-compose.yml up` directly (without first running `./smackerel.sh config generate` or `./smackerel.sh up`) will now see Compose abort with the named-var error message. This is the intended Gate G028 fail-loud behavior; the error message names the fix path (`run ./smackerel.sh config generate or ./smackerel.sh up`).
- **Operator-side fan-out:** none. The dev compose file is not part of the build-once-deploy-many surface; only `deploy/compose.deploy.yml` is. Operators do not consume `docker-compose.yml` in production.
- **Adapter-side fan-out:** none. Same reason as above.
- **Test infrastructure (canary surface):** all 9 pre-existing `TestComposeContract_*` adversarial tests + `TestComposeContract_LiveFile` + `TestVulnGateContract_*` + `TestBundleHashContract_*` PASS unchanged.
- **Generated-artifact contract:** `config/generated/dev.env` and `config/generated/test.env` gain 5 new SST-emitted lines each. Both files are gitignored; no commit-surface change. CI pipelines that already export `SMACKEREL_VERSION` and `SMACKEREL_COMMIT` continue to flow through unchanged (the SST resolution honors shell env overrides).
- **Bootstrap contract for downstream specs:** spec 042 (tailnet-edge bind, prod-compose) is unaffected — the new test targets a different file. Spec 029 (this fix's parent) gains a static-file lint that locks dev-compose Gate G028 compliance against future drift.
- **Rollback or restore plan:** see the corresponding DoD item — single `git revert`; no live-file or runtime-behavior mismatch possible because the SST emission is purely additive (developers without the new vars in their env file would have seen the silent fallback before; now they see the named-var Compose error pointing at `./smackerel.sh config generate`).
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The fix is a static-file invariant + SST emission additions + Compose substitution-form changes; no daemon state, no shared cache, no cross-process ordering concern.

### Consumer Impact Sweep

This bug fix does **not** rename or remove any externally-visible interface, route, endpoint, contract, API, URL, slug, public symbol, deep link, breadcrumb, navigation entry, or generated client. The change is bounded to:

- **`scripts/commands/config.sh`:** purely additive emission of 5 new env-file lines. No removal, no rename. Pre-existing emissions (PROJECT_NAME, COMPOSE_PROJECT, POSTGRES_*, NATS_*, etc.) are bit-identical to HEAD.
- **`docker-compose.yml`:** the substitution-form change (from `:-default` to `:?error` or `?error`) is a substitution-semantics change, not an interface change. The same vars are still consumed (SMACKEREL_VERSION, SMACKEREL_COMMIT, SMACKEREL_BUILD_TIME, SMACKEREL_ENV_FILE, SMACKEREL_CORE_IMAGE, SMACKEREL_ML_IMAGE); only the failure mode on absence is different (silent fallback → fail-loud with named-var Compose error).
- **`internal/deploy/dev_compose_default_fallback_test.go`:** purely additive new test file. Test functions are not importable across packages.
- **No public API change.** No HTTP route, no NATS subject, no CLI flag, no env-var name, no config-key, no URL path, no breadcrumb, no redirect surface, no generated client regeneration.
- **Affected consumer surfaces enumerated:** the consumers of the new substitution forms are `docker compose` (dev workflow), `./smackerel.sh up`, `./smackerel.sh test`, and the new lint test. All consumer surfaces are explicitly named and validated by the test plan. No documentation, no operator runbook references the new lint test by name.
- **Cross-package consumer surface:** zero. Test functions are not importable.
- **Stale-reference scan:** zero stale first-party references remain. The pre-existing `${X:-default}` form in the dev compose file did not have any test that referenced it by name, so there is nothing to update.

### Change Boundary

**Allowed file families (this fix may modify):**

- `scripts/commands/config.sh` — the SST generator (5-block resolution + 5 emission lines added)
- `docker-compose.yml` — the dev-stack compose file (10 substitution forms changed + 11-line deferral comment added)
- `internal/deploy/dev_compose_default_fallback_test.go` — NEW persistent in-tree static-file lint test
- `specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/**` — this bug packet's seven artifacts

**Excluded surfaces (this fix MUST NOT touch):**

- `internal/deploy/compose_contract_test.go` — the prod-compose contract test; bit-identical to HEAD (the new dev-compose test is in a separate file targeting a different live file)
- `deploy/compose.deploy.yml` — the prod / self-hosted compose file; bit-identical to HEAD (already locked by spec 042 + BUG-042-001..005)
- `config/smackerel.yaml` — the SST source; bit-identical to HEAD (the SST generator change does not require a new SST source key — the build-metadata vars come from shell env at config-generate time, not from `smackerel.yaml`)
- `specs/029-devops-pipeline/spec.md`, `specs/029-devops-pipeline/design.md`, `specs/029-devops-pipeline/scopes.md`, `specs/029-devops-pipeline/state.json`, `specs/029-devops-pipeline/uservalidation.md`, `specs/029-devops-pipeline/report.md` — foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope
- `specs/042-tailnet-edge-bind-pattern/**` — foreign-owned (different parent spec)
- Production runtime Go code under `internal/auth/...`, `internal/config/...`, `internal/api/...`, `cmd/...` — the bug is in the dev-compose contract surface, not in the runtime
- Python ML sidecar under `ml/...` — unrelated
- `Dockerfile`, `ml/Dockerfile` — unrelated
- `.github/workflows/*` — unrelated; the existing `unit-tests` job picks up the new test automatically. Per the active sprint queue, no CI workflow changes are made in this packet (HL-RESCAN-011 is owned by a parallel session).
- `scripts/lib/runtime.sh` — bit-identical to HEAD (the `--env-file` invocation pattern is already correct)
- Any other `specs/**` directory — single-bug-scope discipline

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-029-003-D: live dev compose has zero unauthorized `${X:-default}` | TestDevComposeContract_NoUnauthorizedDefaultFallbacks | internal/deploy/dev_compose_default_fallback_test.go | unit (Go static-file lint) | NO (positive canary) |
| SCN-029-003-D: regression of converted var to `:-` form caught | TestDevComposeContract_AdversarialUnauthorizedDefaultFallback (4 sub-cases) | same as above | unit (Go static-file lint) | YES — fails RED if any converted var is reverted |
| SCN-029-003-E: novel non-allowlisted var caught | TestDevComposeContract_AdversarialUnauthorizedDefaultFallback (novel-var sub-case) | same as above | unit (Go static-file lint) | YES |
| SCN-029-003-F: per-var (not per-line) allowlist gating | TestDevComposeContract_AdversarialAllowlistRespected | same as above | unit (Go static-file lint) | YES |
| SCN-029-003-G: comment-line skip prevents false positives | TestDevComposeContract_AdversarialCommentLinesIgnored | same as above | unit (Go static-file lint) | YES |
| Canary: TestComposeContract_LiveFile (prod compose) preserved | TestComposeContract_LiveFile | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | NO (positive canary) |
| Canary: BUG-042-001..005 prod-compose adversarials preserved | TestComposeContract_AdversarialLiteralBind, AdversarialInfraHasPorts, AdversarialMultiPortsBypass, AdversarialMLMultiPortsBypass, AdversarialNetworkModeHostBypass (5 sub-cases), AdversarialOllamaLiteralBind (2 sub-cases), AdversarialDefaultFallbackBind (3 sub-cases), AdversarialPrometheusLiteralBindAndFallbackForms (2 sub-cases) | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: spec 047 vuln-gate contract preserved | TestVulnGateContract_LiveFile + 10 sub-tests | internal/deploy/build_workflow_vuln_gate_contract_test.go | unit (Go static-file lint) | YES (canaries) |
| Canary: BUG-047-001 bundle-hash contract preserved | TestBundleHashContract_LiveFile + 4 sub-tests | internal/deploy/build_workflow_bundle_hash_contract_test.go | unit (Go static-file lint) | YES (canaries) |
