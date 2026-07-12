# Bug: BUG-029-005 — Decouple connector enable-signal from volume-mount-path emptiness; convert the 4 remaining dev-compose `${VAR:-default}` volume-mount substitutions to fail-loud SST

## Classification

- **Type:** DevOps defect — completion of the dev-compose Gate G028 sweep (operational hardening; follow-up to BUG-029-003)
- **Severity:** P3 — LOW (same severity floor as BUG-029-003: the dev compose file `docker-compose.yml` is used only by `./smackerel.sh up` / `./smackerel.sh test` workflows; the production / self-hosted compose file `deploy/compose.deploy.yml` is unaffected). The remaining risk surface is the same silent-fallback failure mode that BUG-029-003 closed for the other 10 occurrences: a developer who manually invokes `docker compose -f docker-compose.yml up` without first running `./smackerel.sh config generate` would silently mount the repo's `./data/<connector>` fixture dirs as the connector input source, masking real misconfiguration (e.g., they meant to point the connector at their actual host-side import directory but forgot to set the env var).
- **Parent Spec:** 029 — DevOps Pipeline & Image Governance (owns `./smackerel.sh up` workflow + dev `docker-compose.yml` + `./smackerel.sh config generate` SST chain)
- **Predecessor:** BUG-029-003 — Dev `docker-compose.yml` violates Gate G028 NO-DEFAULTS via 14 `${VAR:-default}` substitutions (this bug closes the 4 occurrences BUG-029-003 explicitly deferred)
- **Workflow Mode:** bugfix-fastlane
- **Status:** Open
- **Discovered By:** 2026-05-14 self-hosted readiness re-scan (finding HL-RESCAN-012; follow-up scope explicitly named in BUG-029-003 spec.md "Out of Scope" and tracked by allowlist entries in `internal/deploy/dev_compose_default_fallback_test.go`)

## Problem Statement

BUG-029-003 closed 10 of the 14 Gate G028 violations in the dev `docker-compose.yml` but explicitly deferred the four volume-mount occurrences (`BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `TWITTER_ARCHIVE_DIR`) because converting them required a coordinated connector-side refactor. Those four occurrences currently look like:

```yaml
volumes:
  - ${BOOKMARKS_IMPORT_DIR:-./data/bookmarks-import}:/data/bookmarks-import:ro
  - ${MAPS_IMPORT_DIR:-./data/maps-import}:/data/maps-import:ro
  - ${BROWSER_HISTORY_PATH:-./data/browser-history/History}:/data/browser-history/History:ro
  - ${TWITTER_ARCHIVE_DIR:-./data/twitter-archive}:/data/twitter-archive:ro
```

Each one violates Gate G028 ("ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase") because the `:-` form silently substitutes a hardcoded host path (`./data/bookmarks-import`, etc.) when the env var is unset OR empty.

The deferral was load-bearing: a paired set of `${X:+/data/<connector>}` substitutions in the `environment:` block of `smackerel-core` used the same env vars as a "no import configured" signal — when the host var was empty, the `:+` substitution rendered the container's value as empty, which the connector startup code in `cmd/core/connectors.go` consumed as "skip starting this connector" via guards like `if cfg.BookmarksEnabled && cfg.BookmarksImportDir != ""`. So the four host-path vars were doing double duty: they were the volume-mount source AND the implicit-disable signal. Converting them to fail-loud forms would have broken the "skip when empty" signal.

The right long-term design — and the one this bug implements — decouples the two concerns:

1. **The boolean per-connector enable flag is the SOLE load-bearing signal** for whether the connector starts (`BookmarksEnabled`, `BrowserHistoryEnabled`, `MapsEnabled`, `TwitterEnabled`). These flags already exist and are already SST-emitted.
2. **The mount-path env var is always non-empty** after SST resolution. If the user's `config/smackerel.yaml` is empty for `connectors.bookmarks.import_dir`, the SST emits the repo-default host path (`./data/bookmarks-import`). The repo ships those directories as gitkept fixtures so a fresh clone has a valid mount source.
3. **The container-internal mount target is an architectural constant** (`/data/bookmarks-import` etc.) — the same convention already in use for `/app/prompt_contracts` (`AGENT_SCENARIO_DIR`). It is hardcoded in the `environment:` block (no `${X:+...}` substitution) because the container path is part of the dev-compose contract between volume-target and connector code, not a configurable runtime value.
4. **The 4 volume-mount substitutions become fail-loud** `${X:?error message}` per the Gate G028 contract — Compose aborts at substitution time with a named error if the SST-emitted env file is missing the var (i.e., if the developer bypassed `./smackerel.sh config generate`).

The connector startup code (`cmd/core/connectors.go`) is updated to drop the redundant `&& cfg.<X>Dir != ""` guard for the three connectors that have it (`BookmarksEnabled`, `BrowserHistoryEnabled`, `MapsEnabled`) — the boolean is now the sole gate, exactly as it already was for `TwitterEnabled`.

After this fix, the dev compose file has ZERO Gate G028 violations and the allowlist in `internal/deploy/dev_compose_default_fallback_test.go` is empty (or removed entirely).

## Detection

| Aspect | Detail |
|---|---|
| Trigger | BUG-029-003 close-out (2026-05-14) explicitly named this follow-up as the remaining surface area to close |
| Finding | HL-RESCAN-012 (same finding as BUG-029-003; this bug closes the deferred portion) |
| Severity | P3 (same as BUG-029-003; sanctioned `./smackerel.sh up` workflow already supplies the env file, so the gap is silent fallback in unsanctioned manual invocations and SST drift) |
| Audit method | `grep -nE '\$\{(BOOKMARKS_IMPORT_DIR\|MAPS_IMPORT_DIR\|BROWSER_HISTORY_PATH\|TWITTER_ARCHIVE_DIR):-' docker-compose.yml` returns 4 matches (lines 130–133). `internal/deploy/dev_compose_default_fallback_test.go::devComposeDefaultFallbackAllowlist` confirms these 4 vars are the explicitly-allowlisted deferrals. Cross-referenced with `cmd/core/connectors.go` lines 61 / 89 / 122 / 253 to confirm three connectors have the redundant `&& cfg.X != ""` guard and twitter does not. Gate G028 reference: `.github/copilot-instructions.md` SST Zero-Defaults Enforcement table. |

## Acceptance Criteria

- AC-1: `scripts/commands/config.sh` resolves `BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `TWITTER_ARCHIVE_DIR` to a non-empty host path before heredoc emission. Resolution order: (a) shell env value if set and non-empty, else (b) `config/smackerel.yaml` value via `yaml_get` if non-empty, else (c) repo-default host path (`./data/bookmarks-import` / `./data/maps-import` / `./data/browser-history/History` / `./data/twitter-archive`). The defaults are SST emission-time placeholders, not Compose substitution-time defaults — they are visible in the generated env file (auditable) and do not constitute a Gate G028 violation per BUG-029-003 DD-2 precedent (helper-script resolution defaults are out of policy scope; Compose substitution defaults are in scope).
- AC-2: `docker-compose.yml` converts the 4 volume-mount substitutions from `${X:-./data/<connector>}` to `${X:?Gate G028 / HL-RESCAN-012 — must be SST-emitted; run ./smackerel.sh config generate or ./smackerel.sh up}`. Each error message names the regression-target var AND `Gate G028` AND `HL-RESCAN-012` AND the operator fix path.
- AC-3: `docker-compose.yml` replaces the four `${X:+/data/<connector>}` env-override substitutions (`environment:` block) with hardcoded container paths (`BOOKMARKS_IMPORT_DIR: /data/bookmarks-import`, etc.), matching the existing architectural-constant pattern already in use for `AGENT_SCENARIO_DIR: /app/prompt_contracts`. The 11-line deferral comment block above the volumes block is replaced with a 5-line comment documenting the fail-loud SST contract.
- AC-4: `cmd/core/connectors.go` drops the redundant `&& cfg.BookmarksImportDir != ""` from the bookmarks auto-start guard (line 61), `&& cfg.BrowserHistoryPath != ""` from the browser-history guard (line 89), and `&& cfg.MapsImportDir != ""` from the maps guard (line 122). The boolean enable flag becomes the SOLE load-bearing signal, matching the existing twitter pattern (line 253: `if cfg.TwitterEnabled`).
- AC-5: `internal/deploy/dev_compose_default_fallback_test.go::devComposeDefaultFallbackAllowlist` is reduced to an empty map (i.e., zero allowlisted vars). The package docstring is updated to remove the 4-var deferral commentary and add a one-line reference to BUG-029-005 noting that the fail-loud sweep is complete. `TestDevComposeContract_AdversarialAllowlistRespected` is updated to use a synthetic non-allowlisted var (e.g., `ROGUE_VAR`) on both sides (since the allowlist is now empty); the per-var-not-per-line gating logic remains exercised because the test still proves that a rogue var is caught regardless of which line it appears on.
- AC-6: A new persistent adversarial test `TestDevComposeContract_FailLoudVolumeMounts` in the same file asserts that the live `docker-compose.yml` contains the four `${X:?...}` fail-loud substitutions on the volume-mount lines AND that each error message contains `Gate G028`, `HL-RESCAN-012`, and the operator fix path. Adversarial sub-cases prove the test catches a regression of any one of the 4 vars back to `${X:-...}` or `${X?...}` or removing the `:?` form entirely (i.e., bare `${X}` substitution without fail-loud semantics).
- AC-7: A new persistent adversarial test `TestComposeEnvOverrides_ContainerInternalConstants` asserts that the four container-internal env overrides (`BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `TWITTER_ARCHIVE_DIR`) in the `environment:` block of `smackerel-core` are bare-literal (no `${X}` substitution), matching the convention of `AGENT_SCENARIO_DIR: /app/prompt_contracts`. Adversarial sub-case proves the test catches a regression to any `${X:+/...}` or `${X:-/...}` form.
- AC-8: A new persistent adversarial test `TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal` in `cmd/core/` asserts that the four auto-start guards in `connectors.go` test ONLY the boolean enable flag (no `&& cfg.<X> != ""` clause). The test scans `cmd/core/connectors.go` as plain text (consistent with the static-file-lint pattern already established by `internal/deploy/compose_contract_test.go`) and rejects any occurrence of `BookmarksEnabled && cfg.BookmarksImportDir != ""`, `BrowserHistoryEnabled && cfg.BrowserHistoryPath != ""`, `MapsEnabled && cfg.MapsImportDir != ""`, or `TwitterEnabled && cfg.TwitterArchiveDir != ""`.
- AC-9: `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0 after `./smackerel.sh config generate --env dev` regenerates the env file. Same check for `config/generated/test.env`. RED proof: `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with a Compose error message that names at least one of the 4 vars and the Gate G028 / HL-RESCAN-012 attribution.
- AC-10: `./smackerel.sh test unit --go` PASSes — all pre-existing tests (including `TestComposeContract_LiveFile` + 8 prod-compose adversarials, `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` + existing dev-compose adversarials) continue GREEN, and the new tests added by this bug PASS.
- AC-11: The four repo-default host fixture directories (`data/bookmarks-import/`, `data/maps-import/`, `data/browser-history/History/`, `data/twitter-archive/`) carry `.gitkeep` files force-added past the `data/` gitignore entry, so a fresh `git clone` results in the directories existing and the `${X:?...}` fail-loud substitution resolving to an existing-but-empty mount source. The `.gitignore` carries an explicit `!data/<dir>/.gitkeep` exception per directory. Cross-spec impact analysis: the data/ ignore pattern is shared across all phases; force-adding 4 placeholder files does not leak any user data and is the minimum-surface change needed to make the dev compose volume mounts work out-of-the-box on a fresh clone.

## Out of Scope

- Editing `deploy/compose.deploy.yml` (the prod / self-hosted compose file). Already locked by spec 042 + BUG-042-001..005 with `internal/deploy/compose_contract_test.go`. The 4 connectors covered by this bug are dev-only fixture imports and are not part of the self-hosted deployment surface.
- Editing `specs/029-devops-pipeline/spec.md`, `specs/029-devops-pipeline/design.md`, `specs/029-devops-pipeline/scopes.md`, `specs/029-devops-pipeline/state.json`, `specs/029-devops-pipeline/uservalidation.md`, or `specs/029-devops-pipeline/report.md` — foreign-owned parent-spec content; outside `bugfix-fastlane` edit scope.
- Refactoring the per-connector source-config map building (e.g., the `"import_dir": cfg.BookmarksImportDir` keys in `cmd/core/connectors.go`) — that surface is consumed by each connector's `Connect()` method and the existing tests pass through the host path unchanged; changing it would expand the change boundary beyond the Gate G028 sweep.
- Adding new connectors or per-connector defaults to `config/smackerel.yaml` — the four connectors covered here already have `enabled: false` defaults and empty `import_dir` placeholders.
- Updating ML-sidecar (`ml/`) connector code — the ML sidecar does not consume these 4 vars (they are core-only fixture imports).
- Renaming the env vars themselves (e.g., to `BOOKMARKS_HOST_IMPORT_DIR`) — keeping the existing names preserves backward compatibility with any existing operator-private deploy adapter overlays that may already export these vars.
