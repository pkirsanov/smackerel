# Design: BUG-029-005 — Decouple connector enable-signal from volume-mount-path emptiness; convert the 4 remaining dev-compose `${VAR:-default}` volume-mount substitutions to fail-loud SST

## Approach

Five coordinated changes that together close the four remaining Gate G028 violations in `docker-compose.yml` (the volume-mount substitutions for `BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `TWITTER_ARCHIVE_DIR` that BUG-029-003 explicitly deferred) by decoupling two concerns that were conflated into one env var:

1. **`scripts/commands/config.sh`** — extend the 4 existing `yaml_get` resolution lines for the connector mount-path vars to apply a repo-default host fixture path (`./data/bookmarks-import`, etc.) when both the shell env override is unset AND the yaml value is empty. The defaults are SST emission-time placeholders (auditable in the generated env file), not Compose substitution-time defaults — they do not constitute a Gate G028 violation per BUG-029-003 DD-2 precedent.

2. **`docker-compose.yml`** — convert the 4 volume-mount substitutions from `${X:-./data/<connector>}` to `${X:?Gate G028 / HL-RESCAN-012 — must be SST-emitted; run ./smackerel.sh config generate or ./smackerel.sh up}` (fail-loud), and replace the 4 `${X:+/data/<connector>}` env-override substitutions in the `environment:` block with bare-literal container paths (`BOOKMARKS_IMPORT_DIR: /data/bookmarks-import`, etc.) — matching the existing `AGENT_SCENARIO_DIR: /app/prompt_contracts` architectural-constant pattern. The 11-line deferral comment block above the volumes block becomes a 5-line comment documenting the fail-loud SST contract.

3. **`cmd/core/connectors.go`** — drop the redundant `&& cfg.<X> != ""` clause from the three connector auto-start guards (bookmarks, browser-history, maps). The boolean enable flag becomes the SOLE load-bearing signal, matching the existing twitter pattern. Drops the implicit "empty path = disabled" overload in favor of the explicit "boolean = enabled/disabled" contract.

4. **`internal/deploy/dev_compose_default_fallback_test.go`** — empty the `devComposeDefaultFallbackAllowlist` map (the 4 deferrals are gone). Update the package docstring to remove the deferral commentary and add a one-line BUG-029-005 reference. Update `TestDevComposeContract_AdversarialAllowlistRespected` to use a synthetic non-allowlisted var (so the per-var-not-per-line gating logic remains exercised). Add two new persistent adversarial tests: (a) `TestDevComposeContract_FailLoudVolumeMounts` asserts the live file has the 4 fail-loud forms with the required attribution; (b) `TestComposeEnvOverrides_ContainerInternalConstants` asserts the 4 env overrides are bare-literal.

5. **`cmd/core/connectors_startup_gate_test.go`** (NEW FILE) — a static-file lint test that asserts `cmd/core/connectors.go` does NOT contain any `<Flag>Enabled && cfg.<X> != ""` pattern for the 4 connectors, locking the new "boolean is the sole load-bearing signal" contract.

6. **`.gitignore` + `data/<connector>/.gitkeep` × 4** — add `!data/bookmarks-import/.gitkeep` (and equivalents for the other 3) exception lines to `.gitignore`, then force-add 4 placeholder files so a fresh clone has the directories present. This makes the new `${X:?...}` fail-loud volume-mount substitution resolve to an existing-but-empty host directory, preserving the developer ergonomics of `./smackerel.sh up` working out-of-the-box.

The fix is the natural completion of BUG-029-003: where that bug converted 10 of 14 silent-fallback violations to fail-loud, this bug converts the remaining 4 by replacing the load-bearing implicit "empty = disabled" overload with an explicit boolean + always-non-empty-path design.

## Design Decisions

### DD-1: Decouple "is the connector enabled?" from "is the import path set?"

**Decision:** Make the boolean enable flag (`BookmarksEnabled`, etc.) the SOLE load-bearing signal for connector startup. Always emit a non-empty mount path from the SST. Drop the redundant `&& cfg.<X> != ""` clauses in `cmd/core/connectors.go`.

**Rationale:** The current design overloads the empty-string state of the path env var to mean two different things at two different layers:

- Layer 1 (Compose): empty path means "fall back to repo's `./data/<connector>` fixture for the volume-mount source"
- Layer 2 (Container env override via `${X:+/data/...}`): empty host path means "don't surface the container path; let the connector see empty"
- Layer 3 (Connector startup): empty container path means "skip auto-starting this connector"

The overload is fragile and violates Gate G028 at Layer 1. The clean design has one signal for each concern:

- "Should the connector start?" → `BookmarksEnabled` (boolean, SST-emitted)
- "Where does the host fixture live?" → `BOOKMARKS_IMPORT_DIR` (always non-empty host path, SST-emitted with repo-default fallback)
- "Where does the container read from?" → architectural constant `/data/bookmarks-import` (hardcoded in compose, matches existing `AGENT_SCENARIO_DIR` pattern)

After this refactor, each signal has exactly one source and one consumer. The 4 vars become well-formed SST values that satisfy Gate G028's fail-loud Compose substitution requirement without breaking any startup semantics.

**Alternatives rejected:**
- Keep the `&& cfg.X != ""` clause but make the path always non-empty: the clause becomes dead code — always true after the SST refactor. Removing the clause makes the intent explicit and avoids future-maintainer confusion about whether the clause is load-bearing.
- Introduce a new `BOOKMARKS_HOST_DIR` separate from `BOOKMARKS_IMPORT_DIR`: rejected as over-engineering. The existing name is fine; what changes is whether it's allowed to be empty. The connector code only sees the container's view of the env var (post-override) which is now always `/data/bookmarks-import`.
- Make the connector code consume `os.Getenv("BOOKMARKS_ENABLED")` directly and ignore the path entirely: rejected because the path is still legitimately consumed by the connector's `Connect()` method (the connector reads files from that path). Removing the path consumption would break the connector.

### DD-2: SST emits repo-default host path when yaml is empty (not Compose-default)

**Decision:** Add a 4-block fallback resolution to `scripts/commands/config.sh`: after the existing `yaml_get` line for each of the 4 vars, add `if [[ -z "$X" ]]; then X="./data/<repo-default>"; fi`. The repo-default value is then heredoc-emitted into `config/generated/<env>.env`.

**Rationale:** Gate G028 forbids defaults at the **Compose substitution layer** (`${X:-default}` syntax in `docker-compose.yml`) because that layer is invisible to the developer at runtime — the silent fallback masks misconfiguration. Defaults at the **SST emission layer** (in `scripts/commands/config.sh`) are explicitly auditable: the developer sees the resolved value in `config/generated/dev.env` and can diff it against expectations. BUG-029-003 DD-2 established this distinction (the `+set` resolution idiom for build-metadata vars was accepted there); this bug extends the same pattern to the 4 connector mount-path vars.

The repo-default host paths (`./data/bookmarks-import`, etc.) are chosen so a fresh clone with `./smackerel.sh up` "just works" without any user-side configuration — the operator can drop fixture files into `data/<connector>/` and the connector picks them up. If the operator points the yaml at a different path (e.g., `~/my-bookmarks-export`), the yaml value wins. If the operator exports `BOOKMARKS_IMPORT_DIR=/srv/data/bookmarks` in the shell before invoking `./smackerel.sh config generate`, that wins over both.

**Alternatives rejected:**
- Make the repo-default an absolute path (`/srv/data/<connector>`): rejected because absolute paths break dev workflow (the path must exist on the developer's machine and the dev compose file mounts it from the host).
- Make the SST emit empty and let the connector check for emptiness: this is the pre-fix state; rejected because it carries forward the Gate G028 violation and the implicit-signal overload.
- Make the SST emit a uniquely-recognizable sentinel (e.g., `BOOKMARKS_IMPORT_DIR=__SST_DEFAULT__`) that the connector recognizes: rejected as over-engineering — the value is just a path; let it be a path.

### DD-3: Container-internal mount paths are architectural constants, not SST values

**Decision:** Hardcode the container-internal paths in the `environment:` block of `docker-compose.yml`:

```yaml
environment:
  BOOKMARKS_IMPORT_DIR: /data/bookmarks-import
  MAPS_IMPORT_DIR: /data/maps-import
  BROWSER_HISTORY_PATH: /data/browser-history/History
  TWITTER_ARCHIVE_DIR: /data/twitter-archive
```

No `${X}` substitution, no fail-loud form needed — these are bare literals.

**Rationale:** Container-internal mount paths are part of the dev compose contract — the same as the Dockerfile WORKDIR, the COPY destination paths, or the volume-mount targets on the right side of the `:` in the volumes block. They are architectural constants, not configuration values. The existing pattern is well-established by:

```yaml
environment:
  AGENT_SCENARIO_DIR: /app/prompt_contracts
  PROMPT_CONTRACTS_DIR: /app/prompt_contracts
```

Both of these appear in the same `environment:` block as bare literals (no SST substitution), paired with:

```yaml
volumes:
  - ./config/prompt_contracts:/app/prompt_contracts:ro
```

This is the same shape we're adopting for the 4 connector mount paths. Gate G028's "no hardcoded ports / URLs / hostnames" is about runtime-configurable values that should originate from `config/smackerel.yaml` — container-internal paths are not configurable (they're baked into the volume-target convention), so they don't fall under the policy.

**Alternatives rejected:**
- SST-emit the container paths too (`CORE_CONTAINER_BOOKMARKS_PATH=/data/bookmarks-import`): rejected as over-engineering. The path is invariant across all environments; there is no use case for the operator to override it.
- Keep the `${X:+/data/...}` form but switch the var name (e.g., `BOOKMARKS_HOST_DIR` for the host path, `BOOKMARKS_IMPORT_DIR` stays as container path with `:+` substitution): rejected because the `${X:+/data/...}` substitution still has the "container value depends on host value's emptiness" overload, even if it's now safe (because the host value is always non-empty after the SST refactor) — removing the substitution makes the intent explicit.

### DD-4: Empty allowlist + new fail-loud-assertion test, not just toggling the existing allowlist

**Decision:** Empty the `devComposeDefaultFallbackAllowlist` map in `internal/deploy/dev_compose_default_fallback_test.go` (zero allowlisted vars) AND add a new persistent adversarial test `TestDevComposeContract_FailLoudVolumeMounts` that asserts the 4 volume-mount lines contain the `${X:?...}` fail-loud form with the required `Gate G028` / `HL-RESCAN-012` / operator-fix-path attribution.

**Rationale:** Emptying the allowlist alone would prove "no `${X:-...}` form exists" but would NOT prove "the fail-loud form IS present with the right error message". A future maintainer could regress the 4 vars to `${X}` (bare substitution, no fail-loud) and the existing `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` would still PASS — bare substitution doesn't match the `${X:-...}` regex. The new positive-assertion test closes that gap.

The new test also serves as the long-term canary: a future refactor that moves the 4 volume mounts to a different compose file, splits them across profiles, or otherwise relocates them must preserve the fail-loud form. The test's failure message names each var that fails the assertion, making regressions easy to diagnose.

**Alternatives rejected:**
- Only empty the allowlist, rely on the existing `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` to catch regressions: rejected per the bare-substitution gap above.
- Combine the two tests into one (`TestDevComposeContract_NoUnauthorizedDefaultFallbacks_AndFailLoudPresent`): rejected because the two assertions are semantically distinct (absence of `:-` vs. presence of `:?` with attribution); separating them gives clearer failure messages.
- Move the new test into a separate file (`internal/deploy/dev_compose_fail_loud_volume_mount_test.go`): rejected because the existing file is the natural home for dev-compose contract tests and BUG-029-003 established that convention.

### DD-5: `.gitkeep` placeholders for the 4 fixture directories

**Decision:** Add 4 `.gitkeep` files (one per connector data directory), with `!data/<dir>/.gitkeep` exception lines in `.gitignore` so they're tracked despite the `data/` ignore pattern.

**Rationale:** The new fail-loud volume-mount form requires the host path to exist (Docker Compose errors out if the source path doesn't exist when the `:ro` flag is set on some systems, and even when it auto-creates the path the result is owned by root which causes subsequent permission errors). The cleanest way to ensure the 4 default paths exist on every fresh clone is to commit placeholder files inside them.

`.gitkeep` is the established convention (zero-byte file, ignored by file-content tools, recognized by most IDEs as a "this is just a directory placeholder" marker). The `data/` gitignore pattern is preserved (so user data still doesn't get committed) and the 4 `.gitkeep` exceptions are minimal and explicit.

**Alternatives rejected:**
- Auto-create the directories in `./smackerel.sh up` or `./smackerel.sh config generate`: rejected because it puts side-effect filesystem behavior in CLI commands that should be pure-configuration. Side effects in `config generate` would also break dry-run / inspection workflows.
- Move the default to a path that's not gitignored (e.g., `./testdata/<connector>`): rejected because `testdata/` is conventionally for Go test fixtures, not for runtime connector input.
- Don't add placeholders; let the first `./smackerel.sh up` error with "host path not found" and tell the user to `mkdir -p data/...`: rejected as user-hostile (the dev compose file should work out-of-the-box after a fresh clone).

### DD-6: Connector-startup-gate static-file test in `cmd/core/`, not `internal/deploy/`

**Decision:** The new `TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal` test lives in `cmd/core/connectors_startup_gate_test.go` (new file), in `package main`, scanning `cmd/core/connectors.go` as plain text for the forbidden `<Flag>Enabled && cfg.<X> != ""` pattern.

**Rationale:** The static-file-lint pattern established by `internal/deploy/compose_contract_test.go` (scanning a config file from inside the package that owns the contract) extends naturally to source-code contracts within `cmd/core/`. Putting the test in `cmd/core/` keeps the lint colocated with the code it protects, so a maintainer editing `connectors.go` sees the test in the same directory and gets the failing-test signal locally without needing to know about cross-package locks.

The test reads the file via `os.ReadFile` (resolved relative to the test's working directory using `filepath.Join(filepath.Dir(b), "...")` with `runtime.Caller(0)` for portability — same trick `internal/deploy/compose_contract_test.go` already uses). The forbidden patterns are exact-string regexes (not Go AST parsing) because the patterns are small and adding a Go AST parser is over-engineering for this scope.

**Alternatives rejected:**
- Put the test in `internal/deploy/` alongside the compose-contract tests: rejected because it crosses a package boundary unnecessarily; `cmd/core/` owns `connectors.go` and should own the contract test for it.
- Use a Go AST parser (`go/parser`, `go/ast`) to find the BinaryExpr patterns: rejected as over-engineering. Plain-text regex catches the forbidden patterns and the test docstring explains the limitation (textual matching, not AST-aware).
- Skip the static-file test and rely on code review: rejected because static-file lint catches regressions deterministically and runs in every `./smackerel.sh test unit --go` invocation.

## Trade-offs

| Trade-off | Decision | Reasoning |
|---|---|---|
| Behavior change: 3 connectors now start with always-non-empty path (was: skipped when path was empty) | Accept | The boolean is now the sole gate; an operator who wants the connector NOT to start sets `enabled: false` (load-bearing) instead of leaving the path empty (load-bearing). Migration path documented in design DD-1. |
| Repo size +4 `.gitkeep` files (zero bytes each) | Accept | Negligible cost; required for the fail-loud Compose substitution to work out-of-the-box on a fresh clone. |
| New test file `cmd/core/connectors_startup_gate_test.go` | Accept | Source-code static-file lint deserves a dedicated home; matches the convention established by `internal/deploy/compose_contract_test.go`. |
| `TestDevComposeContract_AdversarialAllowlistRespected` semantics shift (no more allowlisted-var sub-case) | Accept | The per-var-not-per-line gating logic is still exercised by the rogue-var sub-case; the removed allowlist-honored sub-case becomes a no-op assertion (empty allowlist trivially excludes all vars). |
| Compose env-override block loses the `:+` conditional substitution semantics | Accept | The substitution was load-bearing only because the path could be empty. After the SST refactor, the path is always non-empty, so the conditional collapses to "always set" — making the bare-literal form the simplest correct form. |
| Operator-private deploy adapter overlays that set `BOOKMARKS_IMPORT_DIR` to a specific path continue to work unchanged | Accept | The env-var name is preserved; only the empty-value semantics changed (was: "skip connector"; now: "use SST-resolved repo default"). Operator who relied on empty-value-as-disable-signal must switch to `BOOKMARKS_ENABLED=false`. |

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Existing dev-stack workflows rely on empty-path-as-disable signal | Low | Medium | The boolean `BookmarksEnabled` already exists and defaults to `false` in `config/smackerel.yaml`; the dev-default workflow is "connectors disabled until operator opts in" which is preserved. |
| Operator's yaml is empty for `import_dir` AND the operator did NOT intend to use the repo's `./data/bookmarks-import` fixture | Low | Low | The generated env file shows `BOOKMARKS_IMPORT_DIR=./data/bookmarks-import` (auditable); operator sees the SST default and can override via yaml or shell env. The connector is also gated on `enabled: false` by default, so the path is only consumed when the operator explicitly opts in. |
| `.gitkeep` files pollute the data/ directory listing | Low | Negligible | Standard convention; most IDEs hide `.gitkeep` files. |
| New tests catch a regression that's actually intentional behavior change | Low | Low | The tests have clear failure messages naming the contract and pointing to BUG-029-005; a maintainer who intentionally wants to change the contract knows where to update the test. |

## File Touch Inventory

| Path | Change Type | Justification |
|---|---|---|
| [scripts/commands/config.sh](../../../../scripts/commands/config.sh) | Modify (4-line addition) | Apply repo-default fallback to 4 mount-path vars after `yaml_get`. |
| [docker-compose.yml](../../../../docker-compose.yml) | Modify (4 volume lines + 4 env lines + 1 comment block) | Convert volume mounts to fail-loud, env overrides to bare literals, update comment block. |
| [cmd/core/connectors.go](../../../../cmd/core/connectors.go) | Modify (3 guard lines) | Drop redundant `&& cfg.<X> != ""` clauses. |
| [internal/deploy/dev_compose_default_fallback_test.go](../../../../internal/deploy/dev_compose_default_fallback_test.go) | Modify (allowlist + docstring + adversarial-test sub-case) + Add (2 new test functions) | Empty allowlist, update docstring, add `TestDevComposeContract_FailLoudVolumeMounts` + `TestComposeEnvOverrides_ContainerInternalConstants`. |
| [cmd/core/connectors_startup_gate_test.go](../../../../cmd/core/connectors_startup_gate_test.go) | Add (NEW FILE) | Static-file lint test for connector startup-guard contract. |
| [.gitignore](../../../../.gitignore) | Modify (4 exception lines) | Allow `.gitkeep` files under `data/<connector>/` past the `data/` ignore pattern. |
| `data/bookmarks-import/.gitkeep` | Add (NEW FILE, force-add) | Directory placeholder so fresh clone has the path. |
| `data/maps-import/.gitkeep` | Add (NEW FILE, force-add) | Same. |
| `data/browser-history/History/.gitkeep` | Add (NEW FILE, force-add) | Same. |
| `data/twitter-archive/.gitkeep` | Add (NEW FILE, force-add) | Same. |
| `specs/029-devops-pipeline/bugs/BUG-029-005-connector-volume-mount-fail-loud-sweep/` | Add (NEW PACKET, 7 files) | Standard bug packet artifacts: spec / design / scopes / scenario-manifest / state / report / uservalidation. |

## Dependencies

- BUG-029-003 (DONE): closed 10 of 14 Gate G028 violations; established the `internal/deploy/dev_compose_default_fallback_test.go` lint pattern and the per-var allowlist that this bug now reduces to empty.
- Spec 029 (active): owns the dev compose file + SST generator chain.
- No new external dependencies.

## Backward Compatibility

- Operator who has NEVER touched the 4 mount vars: no behavior change. Connectors are disabled by default (`enabled: false` in yaml); the SST emits the repo-default host paths which mount existing-but-empty directories; no connector tries to read anything.
- Operator who has set `connectors.bookmarks.enabled: true` in yaml AND set `connectors.bookmarks.import_dir: /custom/path`: no behavior change. SST resolution order is (a) shell env, (b) yaml value, (c) repo default; yaml value wins.
- Operator who has set `connectors.bookmarks.enabled: true` in yaml AND left `connectors.bookmarks.import_dir: ""` empty: behavior change. Pre-fix: connector was silently skipped (`if cfg.BookmarksEnabled && cfg.BookmarksImportDir != ""` guard). Post-fix: connector starts and reads the repo's `./data/bookmarks-import` directory (which is gitkept and empty unless the operator drops files there). The operator who wants the connector NOT to start must now set `enabled: false`. This is the intended semantic correction — `enabled: true` should mean "start the connector", not "start the connector if path is also non-empty".
- Operator-private deploy adapter overlay that exports `BOOKMARKS_IMPORT_DIR=/srv/data/bookmarks`: no behavior change. Shell env wins over yaml and over SST default.

## Validation Plan

1. Edit `scripts/commands/config.sh` to apply repo-default fallback to 4 vars.
2. Edit `docker-compose.yml` to switch volume mounts to fail-loud + env overrides to bare literals.
3. Edit `cmd/core/connectors.go` to drop the 3 redundant guards.
4. Add `.gitkeep` exceptions + force-add 4 `.gitkeep` files.
5. Edit `internal/deploy/dev_compose_default_fallback_test.go`: empty allowlist + update docstring + update adversarial-test sub-case + add 2 new tests.
6. Create `cmd/core/connectors_startup_gate_test.go` with the static-file lint.
7. Regenerate env files: `./smackerel.sh config generate --env dev` + `--env test`. Verify the 4 vars are emitted with the repo-default values.
8. Compose substitution GREEN: `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0. Same for test.env.
9. Compose substitution RED proof: `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero, error message names at least one of the 4 vars + `Gate G028` + `HL-RESCAN-012`.
10. Unit-test GREEN: `./smackerel.sh test unit --go` PASSes — all 8 prod-compose adversarial tests + all dev-compose tests + the new 3 tests added by this bug.
11. RED→GREEN proof per scenario (regress one fail-loud var → test fails RED → restore → test passes GREEN).
12. Bubbles validators: state-transition-guard PERMITTED, artifact-lint PASSED, traceability-guard PASSED, regression-baseline-guard PASSED.
