# Scopes: BUG-029-005 — Decouple connector enable-signal from volume-mount-path emptiness; convert the 4 remaining dev-compose `${VAR:-default}` volume-mount substitutions to fail-loud SST

## Scope 1: Apply SST repo-default fallback + convert 4 dev-compose volume-mount substitutions to fail-loud + decouple connector startup gate from path-emptiness + lock new contract with 3 static-file tests + commit `.gitkeep` fixture anchor files

**Status:** Done

**Files:**
- [scripts/commands/config.sh](../../../../scripts/commands/config.sh) (4-line fallback after the 4 mount-path `yaml_get` lines: `if [[ -z "$X" ]]; then X="./data/<repo-default>"; fi`)
- [docker-compose.yml](../../../../docker-compose.yml) (4 volume-mount substitution forms converted from `${X:-./data/...}` to `${X:?Gate G028 / HL-RESCAN-012 — must be SST-emitted ...}`; 4 env-override substitutions converted from `${X:+/data/...}` to bare literal `/data/...`; the 11-line prior-fix comment block above volumes replaced with a 5-line fail-loud contract comment)
- [cmd/core/connectors.go](../../../../cmd/core/connectors.go) (3 guard lines: drop `&& cfg.BookmarksImportDir != ""` from line 61, `&& cfg.BrowserHistoryPath != ""` from line 89, `&& cfg.MapsImportDir != ""` from line 122)
- [internal/deploy/dev_compose_default_fallback_test.go](../../../../internal/deploy/dev_compose_default_fallback_test.go) (allowlist emptied; docstring updated with BUG-029-005 reference; `TestDevComposeContract_AdversarialAllowlistRespected` updated to use synthetic non-allowlisted vars; 2 new test functions added: `TestDevComposeContract_FailLoudVolumeMounts` + `TestComposeEnvOverrides_ContainerInternalConstants`)
- [cmd/core/connectors_startup_gate_test.go](../../../../cmd/core/connectors_startup_gate_test.go) (NEW; static-file lint test `TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal` asserting `cmd/core/connectors.go` contains zero `<Flag>Enabled && cfg.<X> != ""` patterns for the 4 connectors)
- [.gitignore](../../../../.gitignore) (4 exception lines: `!data/bookmarks-import/.gitkeep`, `!data/maps-import/.gitkeep`, `!data/browser-history/History/.gitkeep`, `!data/twitter-archive/.gitkeep`)
- `data/bookmarks-import/.gitkeep` (NEW, force-add past `data/` ignore)
- `data/maps-import/.gitkeep` (NEW, force-add past `data/` ignore)
- `data/browser-history/History/.gitkeep` (NEW, force-add past `data/` ignore)
- `data/twitter-archive/.gitkeep` (NEW, force-add past `data/` ignore)

### Use Cases

```gherkin
Feature: Dev compose volume mounts use fail-loud SST substitutions and connector startup gates on boolean alone (Gate G028 sweep completion)
  Scenario: SCN-029-005-A — SST emits non-empty repo-default host path when yaml is empty
    Given config/smackerel.yaml has `connectors.bookmarks.import_dir: ""` (empty)
    And no shell env override for BOOKMARKS_IMPORT_DIR is set
    When `bash scripts/commands/config.sh --env dev` runs
    Then config/generated/dev.env contains the line `BOOKMARKS_IMPORT_DIR=./data/bookmarks-import`
    And same SST-default emission applies to MAPS_IMPORT_DIR (`./data/maps-import`), BROWSER_HISTORY_PATH (`./data/browser-history/History`), TWITTER_ARCHIVE_DIR (`./data/twitter-archive`)

  Scenario: SCN-029-005-B — Yaml value wins over SST repo-default
    Given config/smackerel.yaml has `connectors.bookmarks.import_dir: "/srv/data/bookmarks"`
    And no shell env override for BOOKMARKS_IMPORT_DIR is set
    When `bash scripts/commands/config.sh --env dev` runs
    Then config/generated/dev.env contains the line `BOOKMARKS_IMPORT_DIR=/srv/data/bookmarks`

  Scenario: SCN-029-005-C — Shell env wins over yaml and SST repo-default
    Given config/smackerel.yaml has `connectors.bookmarks.import_dir: "/srv/data/bookmarks"`
    And the shell exports `BOOKMARKS_IMPORT_DIR=/srv/exports/bookmarks` before invoking config.sh
    When `bash scripts/commands/config.sh --env dev` runs
    Then config/generated/dev.env contains the line `BOOKMARKS_IMPORT_DIR=/srv/exports/bookmarks`

  Scenario: SCN-029-005-D — Compose substitution GREEN against SST-emitted env file
    Given config/generated/dev.env contains non-empty values for the 4 mount-path vars (per SCN-029-005-A)
    When `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` runs
    Then it exits 0
    And the same check against config/generated/test.env also exits 0

  Scenario: SCN-029-005-E — Compose substitution RED on missing env (fail-loud)
    Given a developer invokes `docker compose --env-file /dev/null -f docker-compose.yml config -q`
    When Compose attempts variable substitution for the 4 volume-mount vars
    Then it exits non-zero
    And the error message names at least one of [BOOKMARKS_IMPORT_DIR, MAPS_IMPORT_DIR, BROWSER_HISTORY_PATH, TWITTER_ARCHIVE_DIR]
    And the error message contains "Gate G028" or "HL-RESCAN-012" or "./smackerel.sh config generate"

  Scenario: SCN-029-005-F — TestDevComposeContract_FailLoudVolumeMounts asserts live file has all 4 fail-loud forms
    Given the live docker-compose.yml is at its post-fix state
    When `go test -run TestDevComposeContract_FailLoudVolumeMounts` runs
    Then it PASSes (all 4 volume-mount lines have `${X:?...}` form)
    And each error message contains "Gate G028", "HL-RESCAN-012", and the operator fix path
    And reverting any of the 4 to `${X:-...}` or `${X?...}` or bare `${X}` form causes the test to FAIL with a message naming the regressed var

  Scenario: SCN-029-005-G — TestComposeEnvOverrides_ContainerInternalConstants asserts 4 container-internal env overrides are bare-literal
    Given the live docker-compose.yml is at its post-fix state
    When `go test -run TestComposeEnvOverrides_ContainerInternalConstants` runs
    Then it PASSes (the 4 environment-block lines for BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR are bare-literal paths matching the AGENT_SCENARIO_DIR pattern)
    And reverting any of the 4 to `${X:+/data/...}` or `${X:-/data/...}` form causes the test to FAIL

  Scenario: SCN-029-005-H — TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal asserts cmd/core/connectors.go drops the redundant `&& cfg.<X> != ""` clause
    Given cmd/core/connectors.go is at its post-fix state
    When `go test -run TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal ./cmd/core/...` runs
    Then it PASSes (zero occurrences of `BookmarksEnabled && cfg.BookmarksImportDir != ""`, `BrowserHistoryEnabled && cfg.BrowserHistoryPath != ""`, `MapsEnabled && cfg.MapsImportDir != ""`, `TwitterEnabled && cfg.TwitterArchiveDir != ""`)
    And reintroducing any one of those patterns causes the test to FAIL with a message naming the regressed pattern

  Scenario: SCN-029-005-I — TestDevComposeContract_NoUnauthorizedDefaultFallbacks PASSes with empty allowlist
    Given the live docker-compose.yml is at its post-fix state
    And devComposeDefaultFallbackAllowlist is empty (zero allowlisted vars)
    When `go test -run TestDevComposeContract_NoUnauthorizedDefaultFallbacks` runs
    Then it PASSes (zero `${X:-default}` occurrences in the live file)

  Scenario: SCN-029-005-J — Cross-test canary: pre-existing dev-compose adversarial tests preserved
    Given the changes from this fix are applied
    When `go test -count=1 -v -run '^TestDevComposeContract' ./internal/deploy/...` runs
    Then `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` PASSes (positive canary)
    And `TestDevComposeContract_AdversarialUnauthorizedDefaultFallback` PASSes (the 4 BUG-029-003 sub-cases all still work)
    And `TestDevComposeContract_AdversarialAllowlistRespected` PASSes (updated to use synthetic non-allowlisted vars on both sides)
    And `TestDevComposeContract_AdversarialCommentLinesIgnored` PASSes (no change required)

  Scenario: SCN-029-005-K — Cross-test canary: prod-compose contract tests preserved
    Given the changes from this fix are applied
    When `go test -count=1 -v -run '^TestComposeContract' ./internal/deploy/...` runs
    Then all 9 pre-existing prod-compose tests (TestComposeContract_LiveFile + 8 adversarials) PASS unchanged

  Scenario: SCN-029-005-L — Fresh-clone bootstrap: `.gitkeep` files preserve fail-loud Compose substitution
    Given a fresh `git clone` of the smackerel repo
    When the developer runs `./smackerel.sh config generate --env dev` then `./smackerel.sh up`
    Then the 4 host paths exist on disk (`./data/bookmarks-import/`, `./data/maps-import/`, `./data/browser-history/History/`, `./data/twitter-archive/`)
    And `docker compose config -q` exits 0
    And the 4 volume mounts bind to existing-but-empty directories
```

### Implementation Plan

1. **`scripts/commands/config.sh` — Apply repo-default fallback after `yaml_get` for 4 vars:** After the existing line `BOOKMARKS_IMPORT_DIR="$(yaml_get connectors.bookmarks.import_dir 2>/dev/null)" || BOOKMARKS_IMPORT_DIR=""`, insert: `if [[ -z "$BOOKMARKS_IMPORT_DIR" ]]; then BOOKMARKS_IMPORT_DIR="./data/bookmarks-import"; fi`. Repeat for MAPS_IMPORT_DIR (default `./data/maps-import`), BROWSER_HISTORY_PATH (default `./data/browser-history/History`), TWITTER_ARCHIVE_DIR (default `./data/twitter-archive`). Lead the 4-block section with a 3-line comment block citing BUG-029-005 + Gate G028 + the SST-emission-time-default precedent (BUG-029-003 DD-2). The existing emission lines in the heredoc (`BOOKMARKS_IMPORT_DIR=${BOOKMARKS_IMPORT_DIR}`) require no change — they already pick up the resolved variable.

2. **`docker-compose.yml` — Volume-mount fail-loud conversion:** Convert lines 130–133 from `- ${BOOKMARKS_IMPORT_DIR:-./data/bookmarks-import}:/data/bookmarks-import:ro` to `- ${BOOKMARKS_IMPORT_DIR:?Gate G028 / HL-RESCAN-012 — must be SST-emitted; run ./smackerel.sh config generate or ./smackerel.sh up}:/data/bookmarks-import:ro`. Repeat for the other 3 volume-mount lines. Replace the 11-line prior-fix comment block above the volumes block (current lines 119–129) with a 5-line comment documenting the fail-loud SST contract and BUG-029-005 attribution.

3. **`docker-compose.yml` — Env-override bare-literal conversion:** Convert lines 103–106 from `BOOKMARKS_IMPORT_DIR: ${BOOKMARKS_IMPORT_DIR:+/data/bookmarks-import}` to `BOOKMARKS_IMPORT_DIR: /data/bookmarks-import` (bare literal, matching the existing `AGENT_SCENARIO_DIR: /app/prompt_contracts` pattern). Repeat for the other 3. Update the leading comment (line 99–101) to reflect the new pattern (architectural constant, not conditional substitution).

4. **`cmd/core/connectors.go` — Drop 3 redundant guard clauses:**
   - Line 61: `if cfg.BookmarksEnabled && cfg.BookmarksImportDir != "" {` → `if cfg.BookmarksEnabled {`
   - Line 89: `if cfg.BrowserHistoryEnabled && cfg.BrowserHistoryPath != "" {` → `if cfg.BrowserHistoryEnabled {`
   - Line 122: `if cfg.MapsEnabled && cfg.MapsImportDir != "" {` → `if cfg.MapsEnabled {`
   - Twitter guard (line 253) already uses only the boolean — no change.

5. **`.gitignore` — Add 4 exception lines:** After the `data/` line, add:
   ```
   !data/bookmarks-import/.gitkeep
   !data/maps-import/.gitkeep
   !data/browser-history/History/.gitkeep
   !data/twitter-archive/.gitkeep
   ```

6. **Add 4 `.gitkeep` files via `create_file`:** Each file is empty (zero bytes). Use absolute paths: `data/bookmarks-import/.gitkeep`, `data/maps-import/.gitkeep`, `data/browser-history/History/.gitkeep`, `data/twitter-archive/.gitkeep`. After `create_file` they will be tracked because of the `!` exceptions in `.gitignore`.

7. **`internal/deploy/dev_compose_default_fallback_test.go` — Empty allowlist + update docstring + adversarial-test sub-case:**
   - Replace the existing `devComposeDefaultFallbackAllowlist` 4-entry map with an empty `map[string]string{}` literal.
   - Update the package docstring (the long comment block at the top) to remove the 4-var prior-fix commentary; add a one-line note "BUG-029-005 closes the previously-allowlisted volume-mount sweep; the allowlist is now intentionally empty."
   - Update `TestDevComposeContract_AdversarialAllowlistRespected` to use two non-allowlisted vars (e.g., `ROGUE_VAR_A` and `ROGUE_VAR_B`) on the same line, prove both are caught — the test still exercises the per-var-not-per-line gating logic even though the allowlist is empty.

8. **`internal/deploy/dev_compose_default_fallback_test.go` — Add `TestDevComposeContract_FailLoudVolumeMounts`:** A new top-level test function that reads the live `docker-compose.yml`, regex-matches the 4 mount-path vars in `${X:?...}` form, asserts (a) all 4 are present, (b) each error message contains `Gate G028`, `HL-RESCAN-012`, and `./smackerel.sh config generate` (the operator fix path). Include 3 adversarial sub-cases via `t.Run` for: (a) regression to `${X:-default}`, (b) regression to `${X?error}` (no colon — would allow empty), (c) regression to bare `${X}` (no fail-loud form at all).

9. **`internal/deploy/dev_compose_default_fallback_test.go` — Add `TestComposeEnvOverrides_ContainerInternalConstants`:** A new top-level test function that reads the live `docker-compose.yml`, finds the `environment:` block of `smackerel-core`, asserts the 4 lines for BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR contain bare-literal paths (matching `/data/<connector>` regex) with no `${X}` substitution. Include 1 adversarial sub-case proving regression to `${X:+/data/...}` form is caught.

10. **`cmd/core/connectors_startup_gate_test.go` (NEW FILE):** Create with `package main`, import `os`, `path/filepath`, `runtime`, `strings`, `testing`. Define a helper `readConnectorsSourceFile(t *testing.T) string` that resolves the file via `runtime.Caller(0)` + `filepath.Dir` + `filepath.Join(..., "connectors.go")` (no path concatenation surprises). Define top-level test `TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal` that scans the source for 4 forbidden patterns (`BookmarksEnabled && cfg.BookmarksImportDir != ""`, `BrowserHistoryEnabled && cfg.BrowserHistoryPath != ""`, `MapsEnabled && cfg.MapsImportDir != ""`, `TwitterEnabled && cfg.TwitterArchiveDir != ""`); fails if any are found. Include 1 adversarial sub-case via `t.Run` proving the lint catches a re-introduction (inject the pattern into a synthetic fixture string, run the same `findBadPatterns` helper against it, assert non-empty result).

11. **RED→GREEN proof (scenario-first TDD):** Before final commit, run the new tests against the post-fix state — all PASS GREEN. Then temporarily regress one fail-loud form (e.g., `${BOOKMARKS_IMPORT_DIR:?...}` → `${BOOKMARKS_IMPORT_DIR:-./data/bookmarks-import}` in docker-compose.yml), re-run `go test -run TestDevComposeContract_FailLoudVolumeMounts` and `go test -run TestDevComposeContract_NoUnauthorizedDefaultFallbacks`, observe both FAIL RED with the named-var error message. Restore via `replace_string_in_file`, re-run, observe both PASS GREEN.

12. **Compose-substitution proof:** After regenerating env files via `./smackerel.sh config generate`, run `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` (expect exit 0). Also run `docker compose --env-file /dev/null -f docker-compose.yml config -q` (expect non-zero exit + named-var error message proving fail-loud is active).

13. **Confine the change boundary:** the only files modified are `scripts/commands/config.sh`, `docker-compose.yml`, `cmd/core/connectors.go`, `internal/deploy/dev_compose_default_fallback_test.go`, `.gitignore`, the new `cmd/core/connectors_startup_gate_test.go`, the 4 new `.gitkeep` files, and the seven BUG-029-005 packet artifacts. No `deploy/compose.deploy.yml` change, no foreign-owned `specs/**` directory, no CI workflow change, no ML sidecar change, no schema migration.

### Test Plan

- **Targeted dev-compose contract suite:** `go test -count=1 -v -run '^TestDevComposeContract' ./internal/deploy/...` runs all 4 pre-existing test functions + 2 new test functions (`TestDevComposeContract_FailLoudVolumeMounts` with 4 sub-cases, `TestComposeEnvOverrides_ContainerInternalConstants` with 1 sub-case) — every one PASS in <1s wall-clock.
- **Targeted connectors startup-gate suite:** `go test -count=1 -v -run TestConnectorStartupGate ./cmd/core/...` runs the new test function with 1 adversarial sub-case — both PASS in <1s wall-clock.
- **Targeted prod-compose canary suite:** `go test -count=1 -v -run '^TestComposeContract' ./internal/deploy/...` runs all 9 pre-existing top-level tests + their sub-tests — every one PASS unchanged.
- **Cross-package smoke:** `./smackerel.sh test unit --go` covers the full unit suite — all PASS, no regression.
- **Static checks:** `go vet ./internal/deploy/... ./cmd/core/...` exit 0; `gofmt -l internal/deploy/ cmd/core/` empty.
- **SST emission:** `bash scripts/commands/config.sh --env dev` regenerates `config/generated/dev.env`. `grep -nE '^(BOOKMARKS_IMPORT_DIR|MAPS_IMPORT_DIR|BROWSER_HISTORY_PATH|TWITTER_ARCHIVE_DIR)=' config/generated/dev.env` returns 4 lines with the SST-resolved values. Same for `--env test`.
- **Compose substitution (GREEN):** `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0. Same for test.env.
- **Compose substitution (RED proof of fail-loud):** `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with at least one of the 4 mount-path vars named in the error message + the `Gate G028` / `HL-RESCAN-012` attribution.
- **Lint regression test (RED proof):** revert one of the 4 fail-loud volume-mount lines (e.g., `${BOOKMARKS_IMPORT_DIR:?...}` → `${BOOKMARKS_IMPORT_DIR:-./data/bookmarks-import}`), re-run `TestDevComposeContract_FailLoudVolumeMounts` + `TestDevComposeContract_NoUnauthorizedDefaultFallbacks`, observe FAIL with the named-var messages, restore via `replace_string_in_file`, re-run, observe PASS.
- **`.gitkeep` bootstrap:** `git ls-files data/bookmarks-import/ data/maps-import/ data/browser-history/History/ data/twitter-archive/` returns the 4 `.gitkeep` paths; `git check-ignore -v data/bookmarks-import/.gitkeep` reports the `!data/bookmarks-import/.gitkeep` exception.

#### Test Plan Coverage Matrix

| Test Type | Scenario / Behavior | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| execution evidence | SCN-029-005-A: SST emits repo-default when yaml empty | (manual) | `bash scripts/commands/config.sh --env dev` → grep dev.env | NO (positive) | Captured in report.md > Validation Evidence > SST emission |
| execution evidence | SCN-029-005-B: Yaml value wins over SST default | (manual) | manipulate smackerel.yaml + regen + grep | NO (positive) | Captured in report.md > Validation Evidence > SST yaml-precedence |
| execution evidence | SCN-029-005-C: Shell env wins over yaml + SST default | (manual) | export var + regen + grep | NO (positive) | Captured in report.md > Validation Evidence > SST shell-env-precedence |
| execution evidence | SCN-029-005-D: Compose substitution GREEN | (manual) | docker compose --env-file config/generated/dev.env config -q | NO (positive) | Captured in report.md > Validation Evidence > Compose substitution GREEN |
| execution evidence | SCN-029-005-E: Compose substitution RED on missing env | (manual) | docker compose --env-file /dev/null config -q | YES — exits non-zero with named-var error | Captured in report.md > Validation Evidence > Compose substitution RED proof |
| unit (Go static-file lint) | SCN-029-005-F: Live file has 4 fail-loud volume-mount forms with attribution | internal/deploy/dev_compose_default_fallback_test.go | TestDevComposeContract_FailLoudVolumeMounts (1 live-file assertion + 3 adversarial sub-cases) | YES — fails RED if any reverted to `:-` / `?` / bare `${X}` form | Persistent in-tree adversarial Go test runs on every `./smackerel.sh test unit --go` invocation |
| unit (Go static-file lint) | SCN-029-005-G: Container-internal env overrides are bare-literal | internal/deploy/dev_compose_default_fallback_test.go | TestComposeEnvOverrides_ContainerInternalConstants (1 live-file assertion + 1 adversarial sub-case) | YES — fails RED if any reverted to `${X:+/...}` form | Same as above |
| unit (Go static-file lint) | SCN-029-005-H: Connector startup gates on boolean alone | cmd/core/connectors_startup_gate_test.go | TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal (1 live-file assertion + 1 adversarial sub-case) | YES — fails RED if any of the 4 `<Flag>Enabled && cfg.<X> != ""` patterns is re-introduced | Persistent in-tree adversarial Go test runs on every `./smackerel.sh test unit --go` invocation |
| unit (Go static-file lint) | SCN-029-005-I: No-unauthorized-default-fallbacks with empty allowlist | internal/deploy/dev_compose_default_fallback_test.go | TestDevComposeContract_NoUnauthorizedDefaultFallbacks | NO (positive canary against live file) | Pre-existing test now exercised with empty allowlist; runs on every test unit |
| unit (Go static-file lint) | SCN-029-005-J: Pre-existing dev-compose adversarials preserved | internal/deploy/dev_compose_default_fallback_test.go | TestDevComposeContract_AdversarialUnauthorizedDefaultFallback / AdversarialAllowlistRespected / AdversarialCommentLinesIgnored | YES (canaries) | Pre-existing BUG-029-003 contract; preserved unchanged in semantics |
| unit (Go static-file lint) | SCN-029-005-K: Prod-compose contract preserved | internal/deploy/compose_contract_test.go | TestComposeContract_LiveFile + 8 adversarials | YES (canaries) | Pre-existing spec 042 + BUG-042-001..005 contract; preserved unchanged |
| execution evidence | SCN-029-005-L: Fresh-clone bootstrap with `.gitkeep` anchor files | (manual) | git ls-files data/ + git check-ignore -v | NO (positive) | Captured in report.md > Validation Evidence > Gitkeep bootstrap |

### Definition of Done

- [x] `scripts/commands/config.sh` resolves BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR to a non-empty value (shell-env > yaml > SST repo-default). [SCN-029-005-A, B, C]
   → Evidence: `grep -nE 'BOOKMARKS_IMPORT_DIR|MAPS_IMPORT_DIR|BROWSER_HISTORY_PATH|TWITTER_ARCHIVE_DIR' scripts/commands/config.sh` returns the 4 `yaml_get` lines + 4 `if [[ -z ... ]]; then ... fi` fallback lines. See report.md > Code Diff Evidence.
- [x] Generated env files emit the 4 vars with non-empty repo-default values when yaml is empty. [SCN-029-005-A]
   → Evidence: `grep -nE '^(BOOKMARKS_IMPORT_DIR|MAPS_IMPORT_DIR|BROWSER_HISTORY_PATH|TWITTER_ARCHIVE_DIR)=' config/generated/dev.env` returns 4 non-empty lines. Same for test.env. See report.md > Validation Evidence > SST emission.
- [x] `docker-compose.yml` smackerel-core volumes block uses fail-loud `${X:?...}` form for all 4 mount-path vars; each error message contains "Gate G028", "HL-RESCAN-012", and "./smackerel.sh config generate". [SCN-029-005-F]
   → Evidence: `grep -nE 'BOOKMARKS_IMPORT_DIR|MAPS_IMPORT_DIR|BROWSER_HISTORY_PATH|TWITTER_ARCHIVE_DIR' docker-compose.yml | grep ':?'` returns the 4 volume-mount lines. See report.md > Code Diff Evidence.
- [x] `docker-compose.yml` smackerel-core environment block uses bare-literal container paths for all 4 vars (no `${X}` substitution, matching the AGENT_SCENARIO_DIR pattern). [SCN-029-005-G]
   → Evidence: `grep -nE 'BOOKMARKS_IMPORT_DIR:|MAPS_IMPORT_DIR:|BROWSER_HISTORY_PATH:|TWITTER_ARCHIVE_DIR:' docker-compose.yml` shows lines with `/data/<connector>` bare-literal values. See report.md > Code Diff Evidence.
- [x] `docker-compose.yml` retains ZERO `${X:-default}` occurrences (sweep complete). [SCN-029-005-I]
   → Evidence: `grep -nE '\$\{[A-Z_]+:-' docker-compose.yml` returns zero matches. See report.md > Code Diff Evidence.
- [x] `cmd/core/connectors.go` drops `&& cfg.BookmarksImportDir != ""`, `&& cfg.BrowserHistoryPath != ""`, `&& cfg.MapsImportDir != ""` from the 3 connector auto-start guards. [SCN-029-005-H]
   → Evidence: `grep -nE 'Enabled \&\& cfg\.[A-Z]+(ImportDir|Path) != \"\"' cmd/core/connectors.go` returns zero matches; `grep -nE 'if cfg\.(Bookmarks|BrowserHistory|Maps)Enabled \{' cmd/core/connectors.go` returns 3 matches. See report.md > Code Diff Evidence.
- [x] `internal/deploy/dev_compose_default_fallback_test.go::devComposeDefaultFallbackAllowlist` is empty. [SCN-029-005-I]
   → Evidence: `grep -nA5 'devComposeDefaultFallbackAllowlist' internal/deploy/dev_compose_default_fallback_test.go` shows the allowlist with zero entries (or `map[string]string{}` literal). See report.md > Code Diff Evidence.
- [x] `TestDevComposeContract_FailLoudVolumeMounts` exists and PASSes against the live file with 1 positive assertion + 3 adversarial sub-cases. [SCN-029-005-F]
   → Evidence: `go test -count=1 -v -run TestDevComposeContract_FailLoudVolumeMounts ./internal/deploy/...` PASS output. See report.md > Test Evidence.
- [x] `TestComposeEnvOverrides_ContainerInternalConstants` exists and PASSes against the live file with 1 positive assertion + 1 adversarial sub-case. [SCN-029-005-G]
   → Evidence: `go test -count=1 -v -run TestComposeEnvOverrides_ContainerInternalConstants ./internal/deploy/...` PASS output. See report.md > Test Evidence.
- [x] `TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal` exists and PASSes against the live file with 1 positive assertion + 1 adversarial sub-case. [SCN-029-005-H]
   → Evidence: `go test -count=1 -v -run TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal ./cmd/core/...` PASS output. See report.md > Test Evidence.
- [x] `TestDevComposeContract_AdversarialAllowlistRespected` updated to use synthetic non-allowlisted vars (no allowlisted-member fixture); still proves per-var-not-per-line gating. [SCN-029-005-J]
   → Evidence: `git diff internal/deploy/dev_compose_default_fallback_test.go` shows the adversarial-test fixture using ROGUE_VAR_A / ROGUE_VAR_B instead of BOOKMARKS_IMPORT_DIR. See report.md > Code Diff Evidence.
- [x] `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` PASSes with the empty allowlist (live file has zero `${X:-default}` occurrences). [SCN-029-005-I]
   → Evidence: `go test -count=1 -v -run TestDevComposeContract_NoUnauthorizedDefaultFallbacks ./internal/deploy/...` PASS output. See report.md > Test Evidence.
- [x] `.gitignore` carries 4 `!data/<dir>/.gitkeep` exception lines. [SCN-029-005-L]
   → Evidence: `grep -n 'data/.*\.gitkeep' .gitignore` returns 4 lines. See report.md > Code Diff Evidence.
- [x] 4 `.gitkeep` files tracked under `data/bookmarks-import/`, `data/maps-import/`, `data/browser-history/History/`, `data/twitter-archive/`. [SCN-029-005-L]
   → Evidence: `git ls-files data/bookmarks-import/ data/maps-import/ data/browser-history/History/ data/twitter-archive/` returns the 4 paths. See report.md > Code Diff Evidence.
- [x] Fresh-clone bootstrap: after `git clone` + `./smackerel.sh config generate --env dev`, the 4 host paths exist on disk so `docker compose config -q` exits 0 and the 4 volume mounts bind to existing-but-empty directories. [SCN-029-005-L]
   → Evidence: `ls -la data/bookmarks-import/.gitkeep data/maps-import/.gitkeep data/browser-history/History/.gitkeep data/twitter-archive/.gitkeep` (all present) + `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0. See report.md > Validation Evidence > Gitkeep bootstrap.
- [x] RED→GREEN proof captured (scenario-first TDD): temporarily reverting one fail-loud volume-mount form causes the new positive-assertion test AND the existing no-unauthorized-fallbacks test to FAIL RED with named-var messages; restoration returns both to PASS. [SCN-029-005-F, I]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- [x] Compose substitution GREEN: `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0; same for test.env. [SCN-029-005-D]
   → Evidence: see report.md > Validation Evidence > Compose substitution GREEN.
- [x] Compose substitution RED proof: `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with at least one of the 4 mount-path vars named in the error message + the `Gate G028` / `HL-RESCAN-012` attribution. [SCN-029-005-E]
   → Evidence: see report.md > Validation Evidence > Compose substitution RED proof.
- [x] `TestComposeContract_LiveFile` and the 8 pre-existing `TestComposeContract_Adversarial*` adversarial tests (prod compose, BUG-042-001..005) continue PASS GREEN unchanged. [SCN-029-005-K]
   → Evidence: `go test -count=1 -v -run '^TestComposeContract' ./internal/deploy/...` shows all PASS. See report.md > Validation Evidence > Targeted prod-compose canary suite.
- [x] All pre-existing `TestDevComposeContract_*` adversarial tests from BUG-029-003 continue PASS GREEN with the empty-allowlist semantics. [SCN-029-005-J]
   → Evidence: `go test -count=1 -v -run '^TestDevComposeContract' ./internal/deploy/...` shows all PASS. See report.md > Test Evidence.
- [x] Cross-package smoke clean: full `./smackerel.sh test unit --go` PASS. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Cross-package smoke.
- [x] Static checks: `go vet ./internal/deploy/... ./cmd/core/...` exit 0; `gofmt -l internal/deploy/ cmd/core/` empty. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Static checks.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — persistent in-tree `TestDevComposeContract_FailLoudVolumeMounts` + `TestComposeEnvOverrides_ContainerInternalConstants` + `TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal` run on every `./smackerel.sh test unit --go` invocation (CI + developer pre-push). Compose substitution evidence is supplementary execution-time proof. [SCN-029-005-F, G, H]
   → Evidence: see report.md > Audit Evidence > Regression Evidence.
- [x] Broader E2E regression suite passes — full `internal/deploy/...` Go test suite plus cross-package smoke against every other internal/* package PASS, including the BUG-042-001..005 prod-compose contract canary and the BUG-029-003 dev-compose contract canary. [Broader regression]
   → Evidence: `./smackerel.sh test unit --go` returns `[go-unit] go test ./... finished OK`. See report.md > Audit Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. [Spec 042 + BUG-042-001..005 prod-compose canary + spec 029 + BUG-029-003 dev-compose canary + spec 047 vuln-gate canary + BUG-047-001 bundle-hash canary + BUG-047-002 ml-os-upgrade canary]
   → Evidence: targeted `^TestComposeContract|^TestDevComposeContract|^TestVulnGateContract|^TestBundleHashContract|^TestMLDockerfile` suite runs all canaries PASS unchanged. The new tests are purely additive against different surfaces (new test in same file, new test in new file) so the canaries cannot regress as a side effect.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: rollback is a single `git revert` of the BUG-029-005 commit. The change is bounded to (a) 4 SST fallback lines in `scripts/commands/config.sh` (purely additive at SST-emission time), (b) 8 substitution-form changes + 1 comment-block change in `docker-compose.yml` (revert restores `:-` / `:+` forms and 11-line prior-fix comment), (c) 3 guard-clause changes in `cmd/core/connectors.go` (revert re-adds `&& cfg.X != ""`), (d) 1 new file (`cmd/core/connectors_startup_gate_test.go`), (e) 2 new test functions + 1 docstring update + 1 adversarial-fixture update in `internal/deploy/dev_compose_default_fallback_test.go`, (f) 4 `.gitignore` exception lines + 4 `.gitkeep` files (revert removes them; data/ directories remain locally on operator machines). No production runtime Go/Python code, no live deploy/compose.deploy.yml content, no other test files touched. See report.md > Validation Evidence > Rollback dry-run for the explicit revert-plan dry-run.oy.yml, no schema migration. Verified by the RED proof step.
- [x] Consumer impact sweep complete and zero stale first-party references remain. [Consumer Impact Sweep]
   → Evidence: see Consumer Impact Sweep section below.
- [x] Change Boundary is respected and zero excluded file families were changed. [Change Boundary]
   → Evidence: see Change Boundary section below; `git diff --stat` reports exactly four source files modified (`scripts/commands/config.sh`, `docker-compose.yml`, `cmd/core/connectors.go`, `internal/deploy/dev_compose_default_fallback_test.go`) plus `.gitignore` plus 5 new files (cmd/core/connectors_startup_gate_test.go + 4 .gitkeep) plus the seven BUG-029-005 packet artifacts. Every file family in the Excluded surfaces list is bit-identical to HEAD.
- [x] Stress coverage assessment (Gate G026): explicit stress/load coverage is NOT REQUIRED for this fix. The change is a static-file invariant test + SST emission additions + Compose substitution-form changes + connector startup-gate code simplification; there is no latency, throughput, p95/p99, response-time, sla, or slo dimension that the change can move. The new tests run in <1s wall-clock; no daemon, no concurrency, no sustained load. This DoD line documents the assessment for the Gate G026 lint. [Broader regression]
   → Evidence: the new tests run in <1s (see Test Evidence > Targeted dev-compose contract suite). No stress dimension applies.

### Shared Infrastructure Impact Sweep

`scripts/commands/config.sh` is the **SST generator**, the canonical helper that produces every developer's `config/generated/<env>.env` file from `config/smackerel.yaml`. Changes to its emission surface affect every developer who runs `./smackerel.sh up`, `./smackerel.sh test`, or `./smackerel.sh check`. `docker-compose.yml` is the **dev-stack compose file**, the entrypoint Compose file that `./smackerel.sh up` invokes. `cmd/core/connectors.go` is the **connector wiring code**, executed at smackerel-core startup. The BUG-029-005 fix has the following blast radius:

- **Direct downstream consumers:** every developer + every CI job that runs `./smackerel.sh up` (or `./smackerel.sh test`) executes `scripts/lib/runtime.sh::smackerel_compose` which always passes `--env-file config/generated/dev.env` to Compose. The new `${X:?...}` substitution forms in `docker-compose.yml` are satisfied by the SST-emitted env file (proven by `docker compose config -q` exit 0). Sanctioned workflow ergonomics unchanged.
- **Indirect downstream consumers:** developers who run `docker compose -f docker-compose.yml up` directly (without first running `./smackerel.sh config generate` or `./smackerel.sh up`) will now see Compose abort with the named-var error message naming `Gate G028` and the operator fix path. This is the intended Gate G028 fail-loud behavior; same pattern as BUG-029-003.
- **Connector startup behavior change:** operators who set `connectors.bookmarks.enabled: true` AND left `connectors.bookmarks.import_dir: ""` empty (post-BUG-029-005) will now see the bookmarks connector START (reading the repo's `./data/bookmarks-import` directory, which is gitkept and empty unless the operator drops files there). Pre-fix the connector was silently skipped because of the `&& cfg.BookmarksImportDir != ""` guard. This is the intended semantic correction documented in design DD-1. Operators who want the connector NOT to start must set `enabled: false`. The `data/` directories are gitignored so no fixture data is committed; the connector reads empty dir and idles.
- **Operator-side fan-out:** none. The dev compose file is not part of the build-once-deploy-many surface; only `deploy/compose.deploy.yml` is. Operators do not consume `docker-compose.yml` in production.
- **Adapter-side fan-out:** none. Same reason as above.
- **Test infrastructure (canary surface):** all 9 pre-existing `TestComposeContract_*` adversarial tests (prod compose, BUG-042-001..005) + 4 pre-existing `TestDevComposeContract_*` adversarial tests (dev compose, BUG-029-003) + `TestVulnGateContract_*` + `TestBundleHashContract_*` + `TestMLDockerfileOSUpgradeContract` (BUG-047-002) PASS unchanged.
- **Generated-artifact contract:** `config/generated/dev.env` and `config/generated/test.env` gain SST-resolved values for the 4 mount-path vars (was: empty strings; now: non-empty paths). Both files are gitignored; no commit-surface change. Operators whose CI exports these vars continue to flow through unchanged (the SST resolution honors shell env overrides).
- **Bootstrap contract for downstream specs:** spec 042 (tailnet-edge bind, prod-compose), spec 047 (CI image vulnerability gate), and the BUG-047-001 / BUG-047-002 contracts are unaffected — the new tests target different files. Spec 029 (this fix's parent) completes the dev-compose Gate G028 sweep.
- **Rollback or restore plan:** see the corresponding DoD item — single `git revert`; no live-file or runtime-behavior mismatch possible because the SST emission change is purely additive (developers who already have non-empty BOOKMARKS_IMPORT_DIR in their env file would have seen the silent fallback before; now they see the same path resolved through the new SST default chain).
- **Ordering / timing / storage / session / context / role / impact surface:** no impact. The fix is a static-file invariant + SST emission additions + Compose substitution-form changes + 3-line connector-guard simplification + 4 `.gitkeep` anchor files; no daemon state, no shared cache, no cross-process ordering concern.

### Consumer Impact Sweep

This bug fix does **not** rename or remove any externally-visible interface, route, endpoint, contract, API, URL, slug, public symbol, deep link, breadcrumb, navigation entry, or generated client. The change is bounded to:

- **`scripts/commands/config.sh`:** purely additive 4-block fallback resolution. No removal, no rename. Pre-existing emissions are bit-identical to HEAD.
- **`docker-compose.yml`:** the substitution-form changes (volume: `${X:-default}` → `${X:?error}`; env override: `${X:+/path}` → bare literal `/path`) are substitution-semantics changes, not interface changes. The same vars are still consumed; only the failure mode on absence is different (silent fallback → fail-loud with named-var Compose error).
- **`cmd/core/connectors.go`:** the 3 guard-clause simplifications are semantic — the boolean is now the sole gate — not interface changes. The connector public API is unchanged.
- **`internal/deploy/dev_compose_default_fallback_test.go`:** purely additive new test functions + 1 docstring update + 1 adversarial-fixture update. Test functions are not importable across packages.
- **`cmd/core/connectors_startup_gate_test.go`:** purely additive new test file. Test functions are not importable across packages.
- **`.gitignore` + 4 `.gitkeep` files:** purely additive directory anchor files + ignore exceptions. No removal of any existing entry.
- **No public API change.** No HTTP route, no NATS subject, no CLI flag, no env-var name removed, no config-key removed, no URL path, no breadcrumb, no redirect surface, no generated client regeneration. The 4 env-var names (BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR) are preserved with the same SST-emission contract; only the empty-value semantics changed.
- **Affected consumer surfaces enumerated:** the consumers of the new substitution forms + new bare-literal env overrides + simplified connector guards are `docker compose` (dev workflow), `./smackerel.sh up`, `./smackerel.sh test`, smackerel-core startup (`cmd/core/connectors.go`), and the new lint tests. All consumer surfaces are explicitly named and validated by the test plan. No documentation, no operator runbook references the new lint test by name.
- **Cross-package consumer surface:** zero. Test functions are not importable.
- **Stale-reference scan:** the BUG-029-003 packet's allowlist references the 4 mount-path vars; this bug updates the allowlist to empty but does not edit the BUG-029-003 packet (foreign-bug-owned content; outside this fix's scope). The BUG-029-003 packet's text refers to the work being completed in a subsequent bug — this fix IS that subsequent bug. The reference is forward-looking and remains accurate.

### Change Boundary

**Allowed file families (this fix may modify):**

- `scripts/commands/config.sh` — the SST generator (4-line fallback resolution added)
- `docker-compose.yml` — the dev-stack compose file (8 substitution-form changes + 1 comment-block update)
- `cmd/core/connectors.go` — the connector wiring code (3 guard-clause simplifications)
- `internal/deploy/dev_compose_default_fallback_test.go` — pre-existing dev-compose lint test (allowlist emptied + 1 adversarial-fixture update + 1 docstring update + 2 NEW test functions appended)
- `cmd/core/connectors_startup_gate_test.go` — NEW persistent in-tree static-file lint test
- `.gitignore` — 4 `.gitkeep` exception lines added
- `data/bookmarks-import/.gitkeep`, `data/maps-import/.gitkeep`, `data/browser-history/History/.gitkeep`, `data/twitter-archive/.gitkeep` — NEW directory anchor files
- `specs/029-devops-pipeline/bugs/BUG-029-005-connector-volume-mount-fail-loud-sweep/**` — this bug packet's seven artifacts

**Excluded surfaces (this fix MUST NOT touch):**

- `internal/deploy/compose_contract_test.go` — the prod-compose contract test; bit-identical to HEAD (the new dev-compose tests target a different live file)
- `deploy/compose.deploy.yml` — the prod / self-hosted compose file; bit-identical to HEAD (already locked by spec 042 + BUG-042-001..005)
- `config/smackerel.yaml` — the SST source; bit-identical to HEAD (the SST generator change does not require a new SST source key — the 4 mount-path vars are already present in the yaml)
- `specs/029-devops-pipeline/spec.md`, `specs/029-devops-pipeline/design.md`, `specs/029-devops-pipeline/scopes.md`, `specs/029-devops-pipeline/state.json`, `specs/029-devops-pipeline/uservalidation.md`, `specs/029-devops-pipeline/report.md` — foreign-owned parent-spec content; outside `bugfix-fastlane` edit scope
- `specs/029-devops-pipeline/bugs/BUG-029-003-*/**` — foreign-bug-owned content (the predecessor bug is closed and immutable)
- `specs/042-tailnet-edge-bind-pattern/**` — foreign-owned (different parent spec)
- `specs/047-ci-image-vulnerability-gate/**` — foreign-owned (different parent spec)
- Production runtime Go code OUTSIDE the 3 connector guards in `cmd/core/connectors.go` — bit-identical to HEAD
- Python ML sidecar under `ml/...` — unrelated
- `Dockerfile`, `ml/Dockerfile` — unrelated
- `.github/workflows/*` — unrelated; the existing `unit-tests` job picks up the new test automatically
- `scripts/lib/runtime.sh` — bit-identical to HEAD (the `--env-file` invocation pattern is already correct)
- Any other `specs/**` directory — single-bug-scope discipline

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-029-005-F: live file has 4 fail-loud volume-mount forms with attribution | TestDevComposeContract_FailLoudVolumeMounts (1 live-file assertion + 3 adversarial sub-cases) | internal/deploy/dev_compose_default_fallback_test.go | unit (Go static-file lint) | YES |
| SCN-029-005-G: container-internal env overrides are bare-literal | TestComposeEnvOverrides_ContainerInternalConstants (1 live-file assertion + 1 adversarial sub-case) | same as above | unit (Go static-file lint) | YES |
| SCN-029-005-H: connector startup gates on boolean alone | TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal (1 live-file assertion + 1 adversarial sub-case) | cmd/core/connectors_startup_gate_test.go | unit (Go static-file lint) | YES |
| SCN-029-005-I: empty-allowlist no-unauthorized-default-fallbacks | TestDevComposeContract_NoUnauthorizedDefaultFallbacks | internal/deploy/dev_compose_default_fallback_test.go | unit (Go static-file lint) | NO (positive canary) |
| Canary: BUG-029-003 dev-compose adversarials preserved | TestDevComposeContract_AdversarialUnauthorizedDefaultFallback, AdversarialAllowlistRespected (updated), AdversarialCommentLinesIgnored | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: TestComposeContract_LiveFile (prod compose) preserved | TestComposeContract_LiveFile | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | NO (positive canary) |
| Canary: BUG-042-001..005 prod-compose adversarials preserved | TestComposeContract_AdversarialLiteralBind, AdversarialInfraHasPorts, AdversarialMultiPortsBypass, AdversarialMLMultiPortsBypass, AdversarialNetworkModeHostBypass (5 sub-cases), AdversarialOllamaLiteralBind (2 sub-cases), AdversarialDefaultFallbackBind (3 sub-cases), AdversarialPrometheusLiteralBindAndFallbackForms (2 sub-cases) | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: spec 047 vuln-gate contract preserved | TestVulnGateContract_LiveFile + 10 sub-tests | internal/deploy/build_workflow_vuln_gate_contract_test.go | unit (Go static-file lint) | YES (canaries) |
| Canary: BUG-047-001 bundle-hash contract preserved | TestBundleHashContract_LiveFile + 4 sub-tests | internal/deploy/build_workflow_bundle_hash_contract_test.go | unit (Go static-file lint) | YES (canaries) |
| Canary: BUG-047-002 ml-os-upgrade contract preserved | TestMLDockerfileOSUpgradeContract (LiveFile + 3 adversarials) | internal/deploy/ml_dockerfile_os_upgrade_contract_test.go | unit (Go static-file lint) | YES (canaries) |
