# Scopes 103 — Age-Bounded Project-Scoped Unused-Image Reclamation (Smart Cleanup)

> **Planning artifact.** Sequential, scope-gated plan. Scope N cannot start until Scope
> N-1 is fully Done. Status vocabulary: Not Started / In Progress / Done / Blocked. This is
> a shell/build-tooling feature — there is **no** app, web, dashboard, or service runtime
> surface, so scopes are single-surface (the cleanup helper + the two Dockerfile labels + the
> SST config) by nature, not a horizontal layering split. The operator CLI
> (`./smackerel.sh clean`) is the complete operator surface (terminal-discipline).
>
> **Delivery pass — implemented + certified.** All four scopes are implemented to their
> Definition of Done with real `./smackerel.sh --env dev clean test` evidence (see `report.md`);
> every DoD item is checked `[x]` against executed terminal evidence, not fabricated.

---

## Execution Outline

**Phase Order**

1. **Scope 1 — Label-add prerequisite (`Dockerfile` + `ml/Dockerfile`) + label finding.**
   Smackerel images carry **no** identity/owner label today (read-only finding: `Dockerfile`
   + `ml/Dockerfile` stamp only OCI `org.opencontainers.image.*` labels). This is the **"else"
   branch** of the operator directive: **add** `io.smackerel.lifecycle.owner="smackerel"` to
   the runtime stage of both Dockerfiles so built images (and the untagged `<none>` versions
   they orphan, which retain the label) are reclaimable. Record the finding + document the
   `smackerel-*` name fallback as explicitly-optional transitional coverage.
2. **Scope 2 — SST config keys + fail-loud loader/emit extension.** Add the 3 new keys
   (`remove_unused_images`, `unused_image_min_age_hours`, `unused_image_scope`) under a new
   `cleanup:` block in `config/smackerel.yaml`, and extend `scripts/commands/config.sh` to
   `required_value`-read them (fail-loud via `config_key_missing`), `validate_unused_image_policy`
   them (bad bool / non-positive-int / bad scope → abort), and emit them into the generated env
   (`CLEANUP_REMOVE_UNUSED_IMAGES` / `CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS` /
   `CLEANUP_UNUSED_IMAGE_SCOPE`). No `${VAR:-default}` (smackerel-no-defaults).
3. **Scope 3 — Reclamation helper: pure argv builder + env=prod guard + executor
   (PROD-SAFETY).** New sourceable helper `scripts/lib/cleanup-image-reclamation.sh` with
   `build_unused_image_prune_argv` (pure), `assert_dev_plane` (env=prod guard on
   `SMACKEREL_ENV`), and `prune_unused_images_aged` (executor: `DRY_RUN`-aware, volume-free,
   container-free, reclaimed-space log). `smackerel.sh` sources the helper.
4. **Scope 4 — Wire into the `clean smart` arm (gated) + repo-CLI `clean test` entrypoint;
   other clean levels unchanged.** Call the stage immediately after `smackerel_run_down
   "$TARGET_ENV" false`, gated by `CLEANUP_REMOVE_UNUSED_IMAGES`; add `./smackerel.sh --env dev
   clean test` (intercepted before `require_docker`) + the help line; prove `clean full` /
   `clean status` / `clean measure` are byte-for-byte unchanged.

**New Types & Signatures**

```
# Dockerfile (core runtime stage) + ml/Dockerfile (smackerel-ml runtime stage) — LABEL ADDED (Scope 1)
LABEL io.smackerel.lifecycle.owner="smackerel"      # spec 103: project-scope identity (FR-012, the "else" branch)

# config/smackerel.yaml (new cleanup: block)
cleanup.remove_unused_images: bool                  # true|false, no default
cleanup.unused_image_min_age_hours: int(>0)         # docker --filter until=<N>h
cleanup.unused_image_scope: "project" | "all"       # project = io.smackerel.lifecycle.owner=smackerel-scoped

# scripts/commands/config.sh  (generator extension)
validate_unused_image_policy()                       -> fail-loud value validation (exit 1)
# + 3 required_value reads (fail-loud missing via config_key_missing) + 3 KEY=value emit lines

# scripts/lib/cleanup-image-reclamation.sh  (NEW sourceable helper)
SMACKEREL_IMAGE_OWNER_LABEL="io.smackerel.lifecycle.owner=smackerel"   # MUST match the Dockerfile LABEL literal
SMACKEREL_ENV_LABEL_KEY="io.smackerel.environment"   # module constants
SMACKEREL_PROD_ENV_EXCLUDE="prod"
SMACKEREL_DEV_SAFE_ENVS="development test"
assert_dev_plane(smackerel_env)                      -> refuse (exit 1) if $1 ∉ {development,test} (FR-008)
build_unused_image_prune_argv(min_age_hours, scope)  -> echoes docker argv (pure)
    project -> "image prune -a -f --filter until=<N>h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod"
    all     -> "image prune -a -f --filter until=<N>h --filter label!=io.smackerel.environment=prod"
prune_unused_images_aged(min_age_hours, scope)       -> read SMACKEREL_ENV; assert_dev_plane; DRY_RUN-aware execute; reclaimed-space log

# smackerel.sh
source "$SCRIPT_DIR/scripts/lib/cleanup-image-reclamation.sh"   # after the existing runtime.sh source
# clean) arm: + `test` intercept before require_docker ; + gated prune_unused_images_aged call in smart) after smackerel_run_down ; + help line

# NO sourcing guard added to the monolith: the 3 NEW functions live in the sourceable helper
# (smackerel.sh's bottom `case` dispatch makes it non-sourceable); the harness sources the
# helper, not smackerel.sh.
```

**Validation Checkpoints**

+ After **Scope 1**: `test_owner_label_added` proves `Dockerfile` and `ml/Dockerfile` each
  carry `LABEL io.smackerel.lifecycle.owner="smackerel"` (and `test_owner_label_parity` proves
  the helper constant matches the literal) before any consumer references it.
+ After **Scope 2**: fail-loud config tests (strip a key from a temp `config/smackerel.yaml`
  copy → non-zero `Missing config key: cleanup.<key>`; invalid scope/age/bool → abort) pass
  before any prune function is written; the generated env carries the 3 `CLEANUP_*` values.
+ After **Scope 3**: argv-builder unit tests (project/all/min-age/invalid/env-prod-exclude) +
  `assert_dev_plane` refuses-non-dev / allows-dev + `DRY_RUN` no-exec + no-volume-token +
  no-container-token pass in isolation (helper `source`d, Docker-free), before wiring.
+ After **Scope 4**: `DRY_RUN=true ./smackerel.sh --env dev clean smart` (dev plane) shows the
  new stage in the plan; the gated-off path hides it and logs "disabled"; the
  `full)`/`status)`/`measure)`-unchanged tests pass; `./smackerel.sh --env dev clean test`
  runs the whole harness green.

**Project-config planning notes (G079 / G080)**

+ **Impact-aware validation (G079):** smackerel's `.github/bubbles-project.yaml` defines **no**
  `testImpact` change-path map, so no generated first-pass category set applies. The change
  surface is `Dockerfile` + `ml/Dockerfile` (label) + `config/smackerel.yaml` (3 keys) +
  `scripts/commands/config.sh` (loader/emit) + `scripts/lib/cleanup-image-reclamation.sh`
  (helper) + `smackerel.sh` (source line + wiring + `clean test`) +
  `scripts/commands/clean_image_reclamation_test.sh` (harness); every scope runs its full unit
  + integration set via `./smackerel.sh --env dev clean test`. Regenerate an impact plan only
  if a `testImpact` map is later added for this path.
+ **Trace-contract evidence (G080):** N/A. smackerel's `bubbles-project.yaml`
  `traceContracts.observability` posture is `wired`, but this feature declares **no**
  `observabilityWorkflow` and emits **no** telemetry/spans (it is local Docker-daemon
  build-tooling), so G080/G100 no-op for these scopes. No trace-contract rows are required.

---

## Scope Table

| # | Scope | Surfaces | Tests | DoD summary | Status |
|---|-------|----------|-------|-------------|--------|
| 1 | Label-add prerequisite (`Dockerfile` + `ml/Dockerfile`) + label finding | `Dockerfile`, `ml/Dockerfile`, `scripts/commands/` (harness) | unit (owner label on both images; helper-constant parity) | `io.smackerel.lifecycle.owner=smackerel` added to both runtime stages; finding recorded; `smackerel-*` fallback documented-optional | Done |
| 2 | SST config keys + fail-loud loader/emit extension | `config/smackerel.yaml`, `scripts/commands/config.sh`, `scripts/commands/` (harness) | unit (fail-loud missing, invalid-value, valid-read + emit) | 3 keys under `cleanup:` (SST, no default); `config.sh` reads via `required_value` + `validate_unused_image_policy` + emits `CLEANUP_*` | Done |
| 3 | Reclamation helper: pure argv builder + env=prod guard + executor | `scripts/lib/cleanup-image-reclamation.sh`, `smackerel.sh` (source line), `scripts/commands/` (harness) | unit (argv project/all, min-age, env-prod-exclude, invalid, no-volumes, no-containers, guard refuses-nondev/allows-dev), integration (dry-run) | argv exact incl `until=<N>h` + label scope + env=prod exclude; guard blocks non-dev plane; `DRY_RUN` no-exec; volumes/containers never referenced | Done |
| 4 | Wire into `clean smart` (gated) + `clean test` entrypoint; other levels unchanged | `smackerel.sh`, `scripts/commands/` (harness) | integration (gated on/off), unit (full/status/measure unchanged), e2e (dev-plane dry-run shows stage) | stage runs after teardown when enabled, skipped when false; `full`/`status`/`measure` byte-for-byte unchanged; evidence via repo CLI | Done |

---

## Scope 1: Label-add prerequisite (`Dockerfile` + `ml/Dockerfile`) + label finding

**Status:** Done
**Depends On:** none
**Owner:** smackerel
**Requirements:** FR-012, FR-013 · Scenarios: AC-10
**Scope-Kind:** ci-config

### Gherkin

```gherkin
Scenario: the identity label is added to the build (label-add prerequisite) (AC-10)
  Given smackerel images carried no io.smackerel.lifecycle.owner label before this feature (read-only finding: Dockerfile + ml/Dockerfile stamp only OCI labels)
  When the label-add prerequisite stamps io.smackerel.lifecycle.owner="smackerel" in the Dockerfile core runtime stage and the ml/Dockerfile runtime stage
  Then a newly built smackerel-core / smackerel-ml image carries io.smackerel.lifecycle.owner=smackerel
  And the untagged <none> version it later orphans retains that label
  And the project-scope prune filter (Scope 3) can reclaim it
  And the helper module constant SMACKEREL_IMAGE_OWNER_LABEL matches the Dockerfile LABEL literal
```

### Implementation

+ Add `LABEL io.smackerel.lifecycle.owner="smackerel"` to the **core** runtime stage of
  `Dockerfile` (`FROM alpine:3.22 AS core`), alongside the existing OCI `LABEL` lines.
+ Add `LABEL io.smackerel.lifecycle.owner="smackerel"` to the **smackerel-ml** runtime stage
  of `ml/Dockerfile` (`FROM python:3.12-slim`), alongside its OCI `LABEL` lines.
+ No other build change (no new `ARG`, no entrypoint change, no compose change).
+ Record the label finding (spec.md §Label Finding) — smackerel images do NOT carry an owner
  label (the "else" branch); the label is ADDED here. Document the `smackerel-*` name-pattern
  fallback (dash-prefixed `smackerel-core`/`smackerel-ml`) as explicitly-optional transitional
  coverage for images built before this change (reclaimed by a one-time broader prune or a
  rebuild); it is NOT implemented as a name-based removal function.

### Change Boundary

**Allowed:** `Dockerfile` (add ONE `LABEL` line to the `core` stage), `ml/Dockerfile` (add ONE
`LABEL` line to the runtime stage), `scripts/commands/clean_image_reclamation_test.sh` (add the
harness), and this spec's artifacts. **Excluded:** every other line of both Dockerfiles (build
stages, ARGs, RUN, ENTRYPOINT), `docker-compose.yml`, `config/smackerel.yaml` (Scope 2),
`scripts/commands/config.sh` (Scope 2), `scripts/lib/cleanup-image-reclamation.sh` (Scope 3),
`smackerel.sh` (Scope 3/4), all `cmd/**`, `internal/**`, `ml/app/**`, `proto/**`, every
`deploy/**` / knb surface.

### Shared Infrastructure Impact Sweep

`Dockerfile` + `ml/Dockerfile` are the shared build surface for both service images. Blast
radius: **minimal** — adding a `LABEL` line does not change any RUN layer content, binary,
digest-affecting build step, or runtime behavior (an OCI/label-only metadata layer). Canary:
`test_owner_label_added` (both Dockerfiles carry the label) + `test_owner_label_parity` (helper
constant matches the literal). Rollback: remove the two `LABEL` lines (pure metadata revert; no
data/state impact). No consumer depends on the ABSENCE of the label.

### Consumer Impact Sweep

No label renamed/removed/re-valued — a NEW label is added. No existing consumer filters on
`io.smackerel.lifecycle.owner` today (read-only finding: the label did not exist). The new
label is consumed only by the Scope 3 project-scope filter. No navigation/redirect/API/client
surface is involved (build-tooling only).

### Test Plan

| Test | Type | Scenario / Requirement | Asserts |
|---|---|---|---|
| `test_owner_label_added` | unit | AC-10 / FR-012 | `Dockerfile` core stage AND `ml/Dockerfile` runtime stage each contain `LABEL io.smackerel.lifecycle.owner="smackerel"` |
| `test_owner_label_parity` | unit | FR-002 / FR-012 | `SMACKEREL_IMAGE_OWNER_LABEL` in the helper == `io.smackerel.lifecycle.owner=smackerel` (matches the Dockerfile literal) |
| **Regression:** `test_owner_label_added` | unit (persistent) | AC-10 | permanently protects the label presence on both images so a future Dockerfile edit cannot silently drop project-scoping |

### Definition of Done

- [x] `Dockerfile` core runtime stage carries `LABEL io.smackerel.lifecycle.owner="smackerel"`. → Evidence: report.md
- [x] `ml/Dockerfile` runtime stage carries `LABEL io.smackerel.lifecycle.owner="smackerel"`. → Evidence: report.md
- [x] No other line of either Dockerfile changed (Change Boundary respected). → Evidence: report.md
- [x] The identity label is added to the build (label-add prerequisite, AC-10): a newly built smackerel-core / smackerel-ml image carries the owner label and the orphaned `<none>` version it later orphans retains it. → Evidence: report.md
- [x] The label finding is recorded (spec.md §Label Finding); the `smackerel-*` name fallback is documented as explicitly-optional transitional coverage (not implemented). → Evidence: report.md
- [x] `test_owner_label_added` + `test_owner_label_parity` pass (no skips) and are the persistent regression guard for label presence + constant parity. → Evidence: report.md
- [x] `./smackerel.sh --env dev clean test` runs these tests green with full unfiltered output (terminal-discipline). → Evidence: report.md
- [x] docker-lifecycle-governance (label-aware project scoping enabled), smackerel-no-defaults, and terminal-discipline satisfied for this scope. → Evidence: report.md
- [x] Change Boundary is respected and zero excluded file families were changed (Allowed file families: Dockerfile, ml/Dockerfile, config/smackerel.yaml, scripts/commands/config.sh, scripts/lib/cleanup-image-reclamation.sh, smackerel.sh, harness; Excluded surfaces: cmd/**, internal/**, ml/app/**, proto/**, deploy/**). → Evidence: report.md

---

## Scope 2: SST config keys + fail-loud loader/emit extension

**Status:** Done
**Depends On:** Scope 1
**Owner:** smackerel
**Requirements:** FR-001, FR-017 · Scenarios: AC-6
**Scope-Kind:** ci-config

### Gherkin

```gherkin
Scenario: a missing SST key fails loud (AC-6)
  Given config/smackerel.yaml cleanup.remove_unused_images is present
  But cleanup.unused_image_min_age_hours is absent
  When config generation resolves the cleanup keys
  Then config_key_missing aborts non-zero with "Missing config key: cleanup.unused_image_min_age_hours"
  And no docker prune is executed

Scenario: an invalid SST value fails loud
  Given cleanup.unused_image_scope = "everything"
  When config generation validates the cleanup keys
  Then validate_unused_image_policy aborts non-zero with "cleanup.unused_image_scope must be project|all"
```

### Implementation

+ Add a `cleanup:` block to `config/smackerel.yaml` (the single SST source; no `.template` to
  keep in lockstep) with `remove_unused_images: true`, `unused_image_min_age_hours: 48`,
  `unused_image_scope: project` (design.md §D2), with the fail-loud/no-default comment.
+ In `scripts/commands/config.sh`: add three `required_value cleanup.<key>` reads (fail-loud on
  missing via the existing `config_key_missing`), a `validate_unused_image_policy()` (bad bool /
  non-positive-int / bad scope → `exit 1` with an explicit message), and three
  `CLEANUP_*=${…}` emit lines into the generated env block (design.md §D3).
+ NO `${VAR:-default}`, NO `awk … || echo <default>` — missing/invalid = fail loud
  (smackerel-no-defaults).

### Change Boundary

**Allowed:** `config/smackerel.yaml` (add the `cleanup:` block only), `scripts/commands/config.sh`
(add the 3 reads + validator + 3 emit lines), `scripts/commands/clean_image_reclamation_test.sh`
(extend the harness). **Excluded:** every existing `config/smackerel.yaml` key/value, every
existing `config.sh` value read, `Dockerfile`/`ml/Dockerfile` (Scope 1, done),
`scripts/lib/cleanup-image-reclamation.sh` (Scope 3), `smackerel.sh` (Scope 3/4), all
`cmd/**`/`internal/**`/`ml/**`, generated `config/generated/*.env` (never hand-edited).

### Consumer Impact Sweep

No key renamed/removed. Three NEW keys are added under a NEW `cleanup:` block; the only new
consumer is the generated env (`CLEANUP_*`) read by the Scope 4 wiring. No existing config
consumer is affected. `config/generated/*.env` is regenerated by `config.sh`, never
hand-edited (NC-002).

### Test Plan

| Test | Type | Scenario / Requirement | Asserts |
|---|---|---|---|
| `test_config_keys_present` | unit | FR-001 | `config/smackerel.yaml` has all 3 keys under `cleanup:`; `config.sh` has the 3 `required_value` reads + 3 emit lines |
| `test_fail_loud_missing_key` | unit | AC-6 / FR-001 | temp-copy config with `unused_image_min_age_hours` stripped → generation exits non-zero with `Missing config key: cleanup.unused_image_min_age_hours` (`SMACKEREL_GENERATED_DIR`-isolated) |
| `test_fail_loud_invalid_scope` | unit | FR-001 | `unused_image_scope: everything` → `validate_unused_image_policy` aborts non-zero |
| `test_fail_loud_invalid_age` | unit | FR-001 | `unused_image_min_age_hours: 0` (or `-1`, or `abc`) → abort non-zero |
| `test_fail_loud_invalid_bool` | unit | FR-001 | `remove_unused_images: maybe` → abort non-zero |
| `test_generated_env_carries_cleanup` | unit | FR-001 | a valid generation emits `CLEANUP_REMOVE_UNUSED_IMAGES` / `CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS` / `CLEANUP_UNUSED_IMAGE_SCOPE` into the env file |
| **Regression:** `test_fail_loud_missing_key` | unit (persistent) | AC-6 | permanently protects the no-defaults contract for the 3 keys |

### Definition of Done

- [x] `config/smackerel.yaml` carries the `cleanup:` block with all 3 keys (SST, no default). → Evidence: report.md
- [x] A missing SST key fails loud (AC-6): `config.sh` reads the 3 keys via `required_value` and `config_key_missing` aborts naming the missing `cleanup.<key>`. → Evidence: report.md
- [x] An invalid SST value fails loud: bad bool / non-positive age / bad scope abort non-zero via `validate_unused_image_policy`. → Evidence: report.md
- [x] `scripts/commands/config.sh` emits `CLEANUP_*` into the generated env after validation. → Evidence: report.md
- [x] No `${VAR:-default}` / `|| echo <default>` introduced for any of the 3 keys. → Evidence: report.md
- [x] `test_fail_loud_missing_key` + `test_fail_loud_invalid_{scope,age,bool}` + `test_generated_env_carries_cleanup` pass (no skips) using `SMACKEREL_GENERATED_DIR` isolation + a temp `smackerel.yaml` copy. → Evidence: report.md
- [x] `./smackerel.sh --env dev clean test` runs these tests green with full unfiltered output. → Evidence: report.md
- [x] bubbles-config-sst + smackerel-no-defaults satisfied; the generated env is never hand-edited. → Evidence: report.md

---

## Scope 3: Reclamation helper — pure argv builder + env=prod guard + executor (PROD-SAFETY)

**Status:** Done
**Depends On:** Scope 2
**Owner:** smackerel
**Requirements:** FR-002, FR-003, FR-004, FR-005, FR-006, FR-007, FR-008, FR-009, FR-014 · Scenarios: AC-1, AC-2, AC-3, AC-5, AC-8, AC-9, AC-11
**Scope-Kind:** ci-config

### Gherkin

```gherkin
Scenario: project-scope argv is exact incl age + owner label + env=prod exclude (AC-1, AC-9)
  Given min_age_hours=48 and scope=project
  When build_unused_image_prune_argv 48 project runs
  Then it echoes "image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod"

Scenario: the env=prod guard refuses on a non-dev plane (AC-8)
  Given SMACKEREL_ENV=production (or any value not in {development, test})
  When assert_dev_plane "$SMACKEREL_ENV" runs
  Then it aborts non-zero with "refusing dev-plane image reclamation on a non-dev plane (SMACKEREL_ENV=production)"
  And prune_unused_images_aged executes no docker image prune

Scenario: DRY_RUN previews and never touches volumes or containers (AC-5, AC-11)
  Given DRY_RUN=true, SMACKEREL_ENV=development
  When prune_unused_images_aged 48 project runs
  Then it prints "[DRY-RUN] Would execute: docker image prune -a -f --filter until=48h ..."
  And it executes no docker image prune (a shadowed docker stub is never invoked)
  And the stage functions reference no docker volume / --volumes / docker container / docker rm token
```

### Implementation

+ New sourceable helper `scripts/lib/cleanup-image-reclamation.sh` (design.md §D4) with the
  module constants (`SMACKEREL_IMAGE_OWNER_LABEL`, `SMACKEREL_ENV_LABEL_KEY`,
  `SMACKEREL_PROD_ENV_EXCLUDE`, `SMACKEREL_DEV_SAFE_ENVS`), `build_unused_image_prune_argv`
  (pure), `assert_dev_plane` (reads its `$1` = resolved `SMACKEREL_ENV`), and
  `prune_unused_images_aged` (reads `SMACKEREL_ENV` from the generated env via
  `smackerel_env_value` → `assert_dev_plane` → build argv → `DRY_RUN`-aware execute →
  reclaimed-space log).
+ `smackerel.sh` sources the helper once, right after `source
  "$SCRIPT_DIR/scripts/lib/runtime.sh"`.
+ `assert_dev_plane` allow-list is {`development`, `test`}; anything else (production, empty,
  unknown) aborts non-zero before any prune. `DRY_RUN` honored via the existing
  `smackerel_is_truthy` helper.

### Shared Infrastructure Impact Sweep

`scripts/lib/cleanup-image-reclamation.sh` is a NEW helper (no existing consumer). It reuses
`smackerel_env_value` / `smackerel_require_env_file` / `smackerel_is_truthy` from `runtime.sh`
read-only (no change to them). Blast radius on `smackerel.sh`: one added `source` line (the
file already sources `runtime.sh` the same way). Canary: the argv/guard unit tests run the
helper in isolation (Docker-free). Rollback: remove the helper + the source line (no state
impact). No prune runs during this scope's tests (DRY_RUN / shadowed docker stub only).

### Test Plan

| Test | Type | Scenario / Requirement | Asserts |
|---|---|---|---|
| `test_argv_project` | unit | AC-1 / FR-002 | `build_unused_image_prune_argv 48 project` == `image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod` |
| `test_argv_all` | unit | FR-002 | `build_unused_image_prune_argv 48 all` == `image prune -a -f --filter until=48h --filter label!=io.smackerel.environment=prod` (no owner label) |
| `test_argv_min_age_applied` | unit | AC-2 / FR-007 | changing the age changes ONLY the `until=<N>h` token |
| `test_argv_env_prod_excluded` | unit | AC-9 / FR-009 | both scopes' argv contain `--filter label!=io.smackerel.environment=prod` |
| `test_argv_invalid_scope_fails` | unit | FR-002 | `build_unused_image_prune_argv 48 nonsense` aborts non-zero |
| `test_argv_invalid_age_fails` | unit | FR-002 | `build_unused_image_prune_argv 0 project` / `abc project` aborts non-zero |
| `test_guard_refuses_nondev` | unit | AC-8 / FR-008 | `assert_dev_plane production` / `staging` / `` (empty) aborts non-zero |
| `test_guard_allows_dev` | unit | FR-008 | `assert_dev_plane development` and `assert_dev_plane test` return 0 |
| `test_dry_run_no_exec` | integration | AC-5 / FR-004 | `DRY_RUN=true prune_unused_images_aged 48 project` prints `[DRY-RUN] Would execute: docker image prune …`; a shadowed `docker` stub is never invoked |
| `test_no_volume_tokens` | unit | AC-11 / FR-005 | the helper file contains no `docker volume` / `--volumes` token |
| `test_no_container_tokens` | unit | AC-11 / FR-006 | the helper file contains no `docker container prune` / `docker rm` token |
| **Regression:** `test_guard_refuses_nondev` | unit (persistent) | AC-8 | permanently protects the PROD-SAFETY guard |
| **Regression:** `test_no_volume_tokens` + `test_no_container_tokens` | unit (persistent) | AC-11 | permanently protects data-safety (volumes/containers never referenced) |

### Definition of Done

- [x] `scripts/lib/cleanup-image-reclamation.sh` implements `build_unused_image_prune_argv` (pure), `assert_dev_plane`, and `prune_unused_images_aged` per design.md §D4. → Evidence: report.md
- [x] `smackerel.sh` sources the helper once, after the existing `runtime.sh` source. → Evidence: report.md
- [x] project/all argv are exact (incl `until=<N>h` + owner-label scope + env=prod exclude); the owner label comes from `SMACKEREL_IMAGE_OWNER_LABEL`, not re-hardcoded inline. → Evidence: report.md
- [x] `assert_dev_plane` refuses non-dev (production/empty/unknown) and allows {development, test}; it is called FIRST in `prune_unused_images_aged`, before any argv build or execute. → Evidence: report.md
- [x] DRY_RUN previews and never touches volumes or containers (AC-5, AC-11): the `[DRY-RUN]` preview executes nothing (shadowed-docker proof) and the stage references no `docker volume` / `--volumes` / `docker container` / `docker rm` token. → Evidence: report.md
- [x] All Scope 3 unit + integration tests pass (no skips), run Docker-free via the sourced helper; the guard/data-safety tests are the persistent regression set. → Evidence: report.md
- [x] `./smackerel.sh --env dev clean test` runs these tests green with full unfiltered output. → Evidence: report.md
- [x] docker-lifecycle-governance, storage-policy/data-safe, PROD-SAFETY, terminal-discipline satisfied for this scope. → Evidence: report.md

---

## Scope 4: Wire into `clean smart` (gated) + `clean test` entrypoint; other levels unchanged

**Status:** Done
**Depends On:** Scope 3
**Owner:** smackerel
**Requirements:** FR-010, FR-011, FR-015 · Scenarios: AC-1 (e2e), AC-4, AC-7
**Scope-Kind:** ci-config

### Gherkin

```gherkin
Scenario: aged unused project images reclaimed on clean smart after teardown (AC-1)
  Given remove_unused_images=true, unused_image_min_age_hours=48, unused_image_scope=project, SMACKEREL_ENV=development
  When ./smackerel.sh --env dev clean smart runs
  Then it first runs smackerel_run_down "$TARGET_ENV" false (existing teardown, volume-preserving)
  And then runs docker image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod
  And DRY_RUN=true ./smackerel.sh --env dev clean smart shows that exact command in the plan and changes nothing

Scenario: the stage is skipped when disabled (AC-4)
  Given cleanup.remove_unused_images=false
  When ./smackerel.sh --env dev clean smart runs
  Then the reclamation stage is skipped
  And it logs "Aged unused-image reclamation disabled (cleanup.remove_unused_images=false)"

Scenario: the other clean levels are unchanged (AC-7)
  Given clean full, clean status, clean measure
  When each runs
  Then full still runs smackerel_run_down "$TARGET_ENV" true, status still runs smackerel_compose ps -a, measure still runs docker system df
  And none of them call prune_unused_images_aged
```

### Implementation

+ In the `smart)` arm of `smackerel.sh`'s `clean)` dispatch, **after** the existing
  `smackerel_run_down "$TARGET_ENV" false`, read `CLEANUP_REMOVE_UNUSED_IMAGES` from the
  generated env; if `true`, call `prune_unused_images_aged` with the min-age + scope env
  values; else log the "disabled" line (design.md §D5). The `clean)` arm already generates the
  config before the subcommand `case`, so the env carries the `CLEANUP_*` values +
  `SMACKEREL_ENV`.
+ Add a `test` case at the top of the `clean)` arm, intercepted **before** `require_docker` /
  `smackerel_generate_config`, that `exec`s `scripts/commands/clean_image_reclamation_test.sh`
  (Docker-free harness) — design.md §D6. Add a `clean test` line to the `clean` help block in
  `usage`.
+ Leave `full)`, `status)`, `measure)`, and `smackerel_run_down` byte-for-byte unchanged.

### Consumer Impact Sweep

No route/identifier renamed or removed. The operator surface GAINS one subcommand
(`clean test`) and one gated stage inside `clean smart`. Consumers of `clean smart` (the build
path, pre-push, operators) see the new post-teardown reclamation only when enabled; the
teardown behavior itself is unchanged. `clean full`/`status`/`measure` callers are unaffected
(byte-for-byte). No navigation, breadcrumb, redirect, API client, generated client, or
deep-link consumer surface exists (build-tooling only); the only stale-reference risk is the
`usage` help text, which is updated to list `clean test`.

### Test Plan

| Test | Type | Scenario / Requirement | Asserts |
|---|---|---|---|
| `test_smart_wires_stage` | integration | AC-1 / FR-010 | the `smart)` arm calls `prune_unused_images_aged` AFTER `smackerel_run_down "$TARGET_ENV" false`, gated on `CLEANUP_REMOVE_UNUSED_IMAGES` |
| `test_smart_dry_run_plan` | e2e (dev plane) | AC-1 / SC-001 | `DRY_RUN=true ./smackerel.sh --env dev clean smart` output contains the exact `docker image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod` line and changes nothing |
| `test_smart_gated_off` | integration | AC-4 / FR-010 | `remove_unused_images=false` → the plan omits the prune line and logs "Aged unused-image reclamation disabled (cleanup.remove_unused_images=false)" |
| `test_full_unchanged` | unit | AC-7 / FR-011 | the `full)` arm still calls `smackerel_run_down "$TARGET_ENV" true` and does NOT call `prune_unused_images_aged` |
| `test_status_measure_unchanged` | unit | AC-7 / FR-011 | `status)` still `smackerel_compose ps -a`; `measure)` still `docker system df`; neither calls the stage |
| `test_clean_test_entrypoint` | integration | FR-015 | `./smackerel.sh --env dev clean test` execs the harness before `require_docker` (runs Docker-free) and is listed in `usage` help |
| **Regression:** `test_smart_gated_off` + `test_full_unchanged` | integration/unit (persistent) | AC-4, AC-7 | permanently protects the gate + the narrow change surface (other levels unchanged) |

### Definition of Done

- [x] The `smart)` arm calls `prune_unused_images_aged` immediately after `smackerel_run_down "$TARGET_ENV" false`, gated by `CLEANUP_REMOVE_UNUSED_IMAGES`. → Evidence: report.md
- [x] The stage is skipped when disabled (AC-4): `cleanup.remove_unused_images=false` logs the disabled line and runs no prune. → Evidence: report.md
- [x] `DRY_RUN=true ./smackerel.sh --env dev clean smart` (dev plane) previews the exact planned command (incl `until=<N>h` + owner-label scope + env=prod exclude) and changes nothing (SC-001 evidence captured). → Evidence: report.md
- [x] `./smackerel.sh --env dev clean test` runs the whole harness green (all Scope 1–4 tests, no skips) and is listed in `clean` help. → Evidence: report.md
- [x] The other clean levels are unchanged (AC-7): `clean full` / `clean status` / `clean measure` and `smackerel_run_down` keep exact byte-for-byte behavior; the new stage runs in NONE of them. → Evidence: report.md
- [x] Consumer impact sweep complete: the only operator-surface change is the added `clean test` subcommand + the gated in-`smart` stage; zero stale first-party references remain. → Evidence: report.md
- [x] docker-lifecycle-governance, smackerel-no-defaults, storage-policy/data-safe, PROD-SAFETY, terminal-discipline all satisfied; all operations via `./smackerel.sh`. → Evidence: report.md

---

## Notes

+ This is developer/CI build-tooling; it exchanges no business data (protobuf-only rule N/A)
  and has no app/web/dashboard/service runtime surface.
+ The label finding (smackerel images do NOT carry an owner label) means this spec takes the
  **"else" branch**: a label-add prerequisite (Scope 1) stamps
  `io.smackerel.lifecycle.owner=smackerel` on both images, and the `smackerel-*` name fallback
  is documented as explicitly-optional transitional coverage — the genuine structural
  difference from the WanderAide 162 / GuestHost 152 / QuantitativeFinance 096 references
  (whose owner labels already existed).
