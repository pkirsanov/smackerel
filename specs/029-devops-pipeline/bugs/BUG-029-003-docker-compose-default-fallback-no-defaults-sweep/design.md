# Design: BUG-029-003 — Dev `docker-compose.yml` violates Gate G028 NO-DEFAULTS via 14 `${VAR:-default}` substitutions

## Approach

Three coordinated changes that together close all 10 of the convertible Gate G028 violations in `docker-compose.yml` (build metadata × 6 + env-file path × 2 + image refs × 2) while explicitly documenting the four deferred volume-mount cases as load-bearing exceptions:

1. **`scripts/commands/config.sh`** — add five SST-emitted vars (`SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`, `SMACKEREL_CORE_IMAGE`, `SMACKEREL_ML_IMAGE`) so the generated `config/generated/<env>.env` carries safe placeholders that satisfy the new fail-loud substitution forms in `docker-compose.yml`. The placeholders are only used when neither the shell env nor a future `config/smackerel.yaml` source provides a value, so CI / release pipelines that already export `SMACKEREL_VERSION` and `SMACKEREL_COMMIT` continue to flow through unchanged.
2. **`docker-compose.yml`** — convert the 6 build-metadata + 2 env-file path occurrences from `${X:-default}` to `${X:?error message}` (fail on unset OR empty); convert the 2 image-ref occurrences from `${X:-}` to `${X?error message}` (fail on unset only — empty is the build-from-source toggle). Leave the 4 volume-mount occurrences unchanged, with an inline comment block above them documenting the deferral path. Net result: 10 of 14 violations converted to fail-loud, 4 explicitly allowlisted.
3. **`internal/deploy/dev_compose_default_fallback_test.go`** (NEW FILE) — a static-file lint test in the same `package deploy` as the existing `compose_contract_test.go`. The test parses the live dev compose file line-by-line, regex-matches every `${VAR:-default}` occurrence, filters via an explicit per-var allowlist (the four volume-mount vars), and asserts zero unauthorized matches. Includes three adversarial sub-cases (regression-of-converted-vars, novel-non-allowlisted-var, per-var-not-per-line allowlist gating) plus a comment-line skip sub-case.

The fix is mechanically the same shape as BUG-042-005 (prometheus adversarial coverage) but targets the dev compose file instead of the prod compose file, and the allowlist is per-var (not per-line) so the test surface tolerates future refactoring of the connector code's volume-mount strategy.

## Design Decisions

### DD-1: Three-category split of the 14 occurrences (10 convert, 4 defer)

**Decision:** Classify the 14 forbidden `${VAR:-default}` occurrences in `docker-compose.yml` into four categories — build metadata (6), env-file path (2), image refs (2), volume mounts (4) — and convert exactly 10 to fail-loud forms (categories 1–3) while explicitly deferring the 4 volume-mount cases.

**Rationale:** The four volume-mount vars (`BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `TWITTER_ARCHIVE_DIR`) have a load-bearing empty-value contract: when the env var is empty, the connector code consumes that as "no import configured" via the `${X:+/data/...}` env override pattern in the connector code itself (the `:+` form substitutes a non-empty replacement only when the var is set AND non-empty). Converting these to fail-loud forms would break the "no import configured" signal: with `${BOOKMARKS_IMPORT_DIR:?...}`, an empty value would cause Compose to abort, eliminating the connector's ability to be optional. Converting them to `${X?...}` (no colon — accepts empty) would technically work for Compose substitution but is semantically misleading because the connector code is what consumes the empty signal, not Compose.

The right path for the volume-mount vars is a connector-side refactor (filed as a follow-up bug): emit a separate `BOOKMARKS_IMPORT_ENABLED` flag from the SST, then have the connector check the flag instead of testing the volume-mount path for emptiness. That refactor is OUT OF SCOPE for HL-RESCAN-012 (P3).

The other 10 occurrences have no load-bearing empty-value contract — they are silent fallbacks that mask real misconfiguration, exactly the failure mode Gate G028 is designed to catch. Converting them is mechanical and safe.

**Alternatives rejected:**
- Convert all 14 to fail-loud: rejected because it breaks the connector "no import configured" signal for the four volume-mount vars.
- Convert none of the 14 (defer the whole sweep): rejected because the build-metadata / env-file / image-ref categories are pure silent-fallback violations with no load-bearing semantics; deferring them indefinitely leaves a P3 contract gap.
- Convert build-metadata only (defer env-file + image refs): rejected because env-file path and image refs are also pure silent-fallback violations; their fixes are mechanically identical to the build-metadata fix and adding them now is essentially zero marginal cost.

### DD-2: SST emission added to `scripts/commands/config.sh` for the five build-metadata + image-ref vars

**Decision:** Add a 5-block resolution section to `scripts/commands/config.sh` (before the `mkdir -p "$REPO_ROOT/config/generated"` line) that resolves `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`, `SMACKEREL_CORE_IMAGE`, `SMACKEREL_ML_IMAGE` from shell env (with safe placeholders if unset), then add 5 emission lines to the heredoc that writes `config/generated/<env>.env` so each var lands in the env file.

**Rationale:** The five vars must reach Docker Compose's variable-substitution context for the new fail-loud forms in `docker-compose.yml` to find them. The cleanest plumbing is the SST: `./smackerel.sh up` invokes `scripts/lib/runtime.sh::smackerel_compose` which always passes `--env-file config/generated/dev.env` to Compose. So if the env file carries the vars, Compose's substitution context has them. The alternative (export the vars from `runtime.sh` directly via `export SMACKEREL_VERSION=...`) would couple the runtime helper to the env-file generator, breaking SST single-source-of-truth.

The resolution uses the `if [[ -z "${X+set}" ]]; then X="default"; fi` idiom — the `+set` form (no colon, with plus) is `set if X is set, empty if X is unset`, so `[[ -z ... ]]` is true exactly when X is unset (not when X is set-but-empty). This is the right semantics for build metadata: if CI exports `SMACKEREL_VERSION=v1.2.3`, we use the CI value; if the developer hasn't exported anything, we use the safe placeholder (`dev`); if for some reason CI exports `SMACKEREL_VERSION=""` (explicitly empty), we honor the explicit empty (don't override with `dev`). The `+set` form is NOT a Gate G028 violation because the policy is about Compose / shell substitution-context defaults, not about helper-script resolution defaults at SST emission time.

For the image refs (`SMACKEREL_CORE_IMAGE` / `SMACKEREL_ML_IMAGE`), the placeholder is empty string. This is the build-from-source signal: when the env file emits `SMACKEREL_CORE_IMAGE=` (empty), the new fail-on-unset-only `${SMACKEREL_CORE_IMAGE?...}` form in `docker-compose.yml` is satisfied (the var is set, just to empty), and Compose proceeds to build from the local Dockerfile context. When CI / release pipelines override with `SMACKEREL_CORE_IMAGE=ghcr.io/.../smackerel-core@sha256:...`, the env file carries the digest reference and Compose pulls the pinned image.

**Alternatives rejected:**
- Don't emit the vars from the SST; make `./smackerel.sh up` `export` them inline: rejected per the SST coupling rationale above.
- Don't emit the vars from the SST; require the developer to set them in the shell before invoking Compose: rejected because it puts the burden on the developer for what is a runtime contract surface, and a missing var would now hard-fail Compose instead of silently falling back — net regression in developer ergonomics.
- Use the colon form `${X:-default}` in the resolution block: rejected because `:-` substitutes the default when X is set-but-empty, which would override CI's explicit-empty signal for the image refs.

### DD-3: Fail-loud form choice — `${X:?error}` for build metadata + env-file, `${X?error}` for image refs

**Decision:** In `docker-compose.yml`, use `${X:?error message}` (colon — fails on unset OR empty) for `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`, `SMACKEREL_ENV_FILE`. Use `${X?error message}` (no colon — fails on unset only, allows empty) for `SMACKEREL_CORE_IMAGE` and `SMACKEREL_ML_IMAGE`.

**Rationale:** The semantics differ on the empty case:

- For build metadata, an empty value (e.g., `SMACKEREL_VERSION=""`) makes no sense — the binary's `--version` output would be a literal empty string, which is worse than the safe placeholder `dev` and worse than no version at all. Compose should abort rather than substitute empty. Hence the colon form.
- For env-file path, an empty value makes no sense for the same reason — `env_file: ""` would either silently work (load no env file at all, breaking every other var lookup) or fail with a confusing later error. Compose should abort upfront with the named message. Hence the colon form.
- For image refs, an explicitly-empty value IS the build-from-source toggle. The colon form would over-block. The no-colon form correctly distinguishes "unset (operator forgot to source the env file or pass `--env-file`)" from "empty (operator wants to build from source)". Hence the no-colon form.

Both forms satisfy Gate G028: the policy forbids `${X:-default}` and `${X-default}` (silent fallback) but permits `${X:?...}`, `${X?...}`, and `${X+...}` (fail-loud or substitute-on-set).

**Alternatives rejected:**
- Use `${X:?...}` uniformly: rejected for the image-ref case per the rationale above.
- Use `${X?...}` uniformly: rejected for the build-metadata + env-file case because empty values are real misconfiguration that should abort.
- Add a Gate G028 carve-out comment explaining the no-colon choice for image refs, but keep the colon form: rejected because the carve-out would still be wrong; the no-colon form is correct and the inline error message documents it.

### DD-4: Volume mounts deferred with inline comment block, not silently allowlisted

**Decision:** Leave the four volume-mount `${X:-./data/...}` occurrences unchanged in `docker-compose.yml`, but add an 11-line comment block immediately above them that explicitly documents (a) why they are not converted, (b) the load-bearing empty-value contract with the connector code, (c) the planned follow-up bug to refactor the connector to consume an SST-emitted "explicit empty" signal.

**Rationale:** Silent allowlisting (just leaving them in `:-` form with no comment) would invite a future maintainer to either (a) "fix" them and break the connector's "no import configured" signal, or (b) copy the `:-` pattern to a new var thinking it's the established convention. The inline comment block prevents both failure modes by making the deferral explicit and machine-greppable.

The comment block also pairs with the new lint test's per-var allowlist: the test names the same four vars with the same justification, so `grep BOOKMARKS_IMPORT_DIR` finds both the compose-file inline comment AND the test's allowlist entry — keeping the rationale colocated with the surface it documents.

**Alternatives rejected:**
- Silent allowlist (no comment): rejected per the rationale above.
- Convert to `${X?...}` (no colon, accepts empty): rejected because while it would technically work for Compose substitution, it is semantically misleading — the connector code (not Compose) consumes the empty signal, and the no-colon form would mask the fact that the connector is the consumer.
- Convert to `${X:?...}` (colon, fails on empty): rejected because it would break the connector "no import configured" signal — every connector with no import dir set would cause `./smackerel.sh up` to abort.
- Move the volume mounts to a separate Compose profile that is only enabled when the import dir is configured: rejected as out of scope (architectural refactor).

### DD-5: Per-var (not per-line) allowlist in the new lint test

**Decision:** The new `internal/deploy/dev_compose_default_fallback_test.go` uses a `map[string]string` allowlist keyed by var name (`BOOKMARKS_IMPORT_DIR` → "connector-fixture mount; empty value = no import (consumed via `${X:+/data/...}` env override)") and the lint logic filters on var name, not on line number or line content.

**Rationale:** Per-var gating tolerates legitimate refactoring of the compose file structure (moving the volume mounts, splitting them across profiles, renaming a service) without requiring a corresponding test edit. Per-line gating would fail any time someone reformatted the compose file. The trade-off is that a maliciously-named var (e.g., `BOOKMARKS_IMPORT_DIR_FAKE` collision) could theoretically smuggle a `:-` default if someone aliased it to `BOOKMARKS_IMPORT_DIR` — but that requires deliberate evasion and would be caught at code review.

The adversarial test `TestDevComposeContract_AdversarialAllowlistRespected` proves the per-var (not per-line) gating: it asserts that adding a *different* var in `:-` form on the same line as an allowlisted var still triggers a violation.

**Alternatives rejected:**
- Per-line allowlist (`map[int]string` keyed by line number): rejected because it would break on every formatting change.
- Per-line-content allowlist (regex match on the full line text): rejected for the same reason as per-line, plus brittleness around whitespace.
- Hash-of-line allowlist: rejected as overengineered.

### DD-6: HL-RESCAN-012 attribution in test docstring + failure-case error message

**Decision:** Mention `HL-RESCAN-012` in the test file's package docstring AND in each adversarial sub-case's failure-case `t.Fatalf` message AND in the lint failure message produced by the `findDevComposeUnauthorizedDefaultFallbacks` helper.

**Rationale:** Same rationale as BUG-042-005 DD-5: the breadcrumb belongs in the meta-evidence (docstring + fail message), not in the contract-surface assertion. A future maintainer hitting either the lint-failure case or the adversarial-test-failure case sees the `HL-RESCAN-012` reference and can navigate to this bug packet for context.

**Alternatives rejected:**
- Skip HL-RESCAN-012 attribution: rejected because future maintainers benefit from the breadcrumb back to the discovering finding.
- Pin HL-RESCAN-012 in the lint helper's error message only (not in the adversarial sub-cases): rejected because the adversarial-test failure path is the most likely to be hit by a regressing change, so the attribution is most valuable there.

## Trade-offs

- The fix only converts 10 of 14 violations. The 4 deferred volume-mount cases are explicitly documented and tracked under a follow-up connector-side refactor bug. This is a deliberate scope-bounded fix that closes the silent-fallback gap on the build-metadata / env-file / image-ref surfaces without paying the cost of a connector refactor.
- The new lint test is a static-file inspection — no Compose invocation, no Docker daemon, no network. It runs as part of `./smackerel.sh test unit --go` in <1s wall-clock. The compose file's runtime behavior is independently validated by `docker compose config -q` against the regenerated env file (captured in evidence) but is not part of the lint test's harness.
- The SST emission adds five lines to every regenerated `config/generated/<env>.env` file. This is gitignored; no commit-surface impact.
- The `${X:?...}` form for `SMACKEREL_ENV_FILE` introduces a new failure mode for developers who run `docker compose -f docker-compose.yml up` directly without first running `./smackerel.sh config generate`. This is the intended behavior (Gate G028 fail-loud) — the failure message names the fix path (`run ./smackerel.sh config generate or ./smackerel.sh up`). Sanctioned `./smackerel.sh up` invocations always supply `--env-file config/generated/dev.env`, so the new fail-loud form does not change ergonomics for anyone using the documented workflow.
