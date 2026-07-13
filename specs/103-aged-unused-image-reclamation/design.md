# Design 103 — Age-Bounded Project-Scoped Unused-Image Reclamation (Smart Cleanup)

**Feature:** `103-aged-unused-image-reclamation`
**Owner:** smackerel
**Status:** design authored (planning pass — no code, no Docker)
**Spec:** [`spec.md`](spec.md)

---

## Design Brief

Add ONE age-bounded, project-scoped, **dev-plane-only**, data-safe reclamation stage to the
existing `clean smart` command (`smackerel.sh` → the `clean)` → `smart)` arm) so orphaned
smackerel image *versions* are removed automatically on every `clean smart` run —
replacing the need for a governance-noncompliant global host prune with a label-scoped,
SST-gated, prod-guarded stage that runs **after** the existing stack teardown.

The change surface: **1 label-add prerequisite** (add
`io.smackerel.lifecycle.owner=smackerel` to `Dockerfile` + `ml/Dockerfile` — the genuine
difference from the QF/GH/WA references, whose labels already existed), **3 new SST keys**,
**1 pure argv builder**, **1 env=prod guard**, **1 executor**, **1 fail-loud validator + emit
extension**, **1 source line in `smackerel.sh`**, **1 gated wiring call in the `smart)` arm**,
and **1 thin repo-CLI `clean test` entrypoint**. `clean full`, `clean status`, `clean measure`,
and every existing runtime function are untouched.

Like the QuantitativeFinance reference (and unlike GuestHost, whose `cleanup.sh` module is
already sourceable), smackerel's `clean` logic lives inside the `smackerel.sh` **monolith**,
which is not sourceable in a unit test (its bottom `case "$COMMAND"` dispatch runs on
`source`). Therefore the three NEW functions live in a focused, sourceable helper
`scripts/lib/cleanup-image-reclamation.sh` — sourced once by `smackerel.sh` right after the
existing `source "$SCRIPT_DIR/scripts/lib/runtime.sh"` — so the pure argv builder + guard are
genuinely unit-testable Docker-free. No existing runtime function is relocated.

---

## Label Finding & Decision *(drives Scope 1)*

**Operator directive.** Verify read-only whether smackerel images carry an identity label
(QF/GH/WA use `io.<product>.lifecycle.owner=<product>`); if smackerel already emits one, use
it; **else add a label-add prerequisite scope + `smackerel-*` name fallback.**

**Read-only finding (see `spec.md` §Label Finding for the evidence table).**

- **No `docker-bake.hcl`.** Smackerel builds via `docker compose … build`
  (`scripts/lib/runtime.sh::smackerel_compose … build`, driven by `./smackerel.sh build`).
- **`Dockerfile` (core runtime stage)** stamps only OCI labels
  (`org.opencontainers.image.version/revision/created/title/source`; `title="smackerel-core"`).
  No `io.smackerel.lifecycle.owner`.
- **`ml/Dockerfile` (smackerel-ml runtime stage)** stamps only OCI labels
  (`org.opencontainers.image.version/revision/created/…`). No `io.smackerel.lifecycle.owner`.
- **No `io.smackerel.*` image identity label exists anywhere** (repo-wide read-only scan for
  `io.smackerel` / `LABEL` / `lifecycle.owner`). No image carries `io.smackerel.environment`.
- **Images are dash-prefixed** `smackerel-core` / `smackerel-ml` (also
  `ghcr.io/pkirsanov/smackerel-core` / `…-ml`), so a `smackerel-*` name pattern is applicable.
- **Plane signal is `SMACKEREL_ENV`** (development|test|production), emitted by
  `scripts/commands/config.sh` from `runtime.environment`, overridden to `test` for
  `TARGET_ENV=test` and to `production` for `TARGET_ENV=self-hosted`. Read from the generated
  `config/generated/<env>.env` via `smackerel_env_value`.

**DECISION ("else — add label + `smackerel-*` fallback" branch).** Because smackerel images
carry **no** identity/owner label, this is the branch the three references did not need:

1. **Add `io.smackerel.lifecycle.owner=smackerel`** to the runtime stages of **both**
   `Dockerfile` and `ml/Dockerfile` (Scope 1). The argv builder reads a module constant
   `SMACKEREL_IMAGE_OWNER_LABEL="io.smackerel.lifecycle.owner=smackerel"` that MUST match the
   Dockerfile `LABEL` literal (single logical identity; a test greps both to assert parity).
2. **Use `io.smackerel.lifecycle.owner=smackerel` as the project-scope filter.**
   `docker image prune -a --filter label=io.smackerel.lifecycle.owner=smackerel` removes only
   images that carry the label → peer-product images (no smackerel owner label) are
   structurally untouched; un-labeled images are skipped (safe by omission).
3. **`smackerel-*` name fallback — documented, transitional, OPTIONAL / not implemented.**
   smackerel starts **without** the label, so images built before Scope 1 lack it and are
   skipped by the label filter. The dash-prefixed `smackerel-*` name pattern is the documented
   belt-and-suspenders for that finite transition window: those pre-label images are reclaimed
   by a one-time broader operator prune, or automatically once rebuilt (the rebuild stamps the
   label). No name-based removal function is added to the smart stage — keeping it narrow and
   data-safe (a name sweep would need a second `docker rmi` mechanism with its own safety
   surface; the label prune + one-time cleanup covers the tail). This mirrors how QF/GH/WA
   documented their `<product>-*` fallbacks as explicitly-optional.
4. **`io.smackerel.environment` is NOT stamped on images.** The env=prod exclusion
   (`--filter label!=io.smackerel.environment=prod`) is defense-in-depth behind the primary
   `assert_dev_plane` host guard, future-proofing for any operator/compose layer that later
   applies an env label — exactly the QF/GH posture.

---

## Current Truth (objective research — verified read-only 2026-07-13)

- **`smackerel.sh`** (the monolith; sources `scripts/lib/runtime.sh` at line 5; bottom
  `case "$COMMAND" in … esac` dispatch runs unconditionally on `source` → **not** sourceable
  in a unit test):
  - `--env` accepts **`dev`|`test`** only (`ERROR: --env requires dev or test`); `TARGET_ENV`
    defaults to `dev`. `self-hosted` is reachable only via the deploy path, not `clean`.
  - The `clean)` arm: `SUBCOMMAND="${1:-}"`; `require_docker`;
    `smackerel_generate_config "$TARGET_ENV" >/dev/null`; then
    `case "$SUBCOMMAND" in status) … measure) … smart) … full) … *) usage; exit 1 ;; esac`.
    - `status)` → `smackerel_compose "$TARGET_ENV" ps -a`.
    - `measure)` → `docker system df`.
    - `smart)` → `smackerel_run_down "$TARGET_ENV" false` (test env wraps in
      `smackerel_with_stack_lock`).
    - `full)` → `smackerel_run_down "$TARGET_ENV" true`.
  - `smackerel_run_down "$env" "$down_volumes"` (defined in `smackerel.sh`) →
    `docker compose down --timeout 30 [--remove-orphans | -v --remove-orphans]`. **No image
    prune, no age bound, volumes preserved in `smart`.**
- **`scripts/lib/runtime.sh`** (sourced by `smackerel.sh`):
  - `smackerel_env_file "$env"` → `config/generated/<env>.env`.
  - `smackerel_require_env_file "$env"` → path; regenerates if missing.
  - `smackerel_env_value "$env_file" "$key"` → `awk -F= '$1==key {print substr(...)}'`
    (reads a `KEY=value` line from the generated env). **This is how the stage reads
    `SMACKEREL_ENV` and the three `CLEANUP_*` values.**
  - `smackerel_generate_config "$env"` → `bash scripts/commands/config.sh --env "$env"`.
  - `smackerel_compose "$env" …` → `docker compose --project-name … --env-file … -f docker-compose.yml …`.
- **`scripts/commands/config.sh`** — the SST generator (`config/smackerel.yaml` → generated
  env). Fail-loud reader: `required_value <dotted.key>` → `yaml_get "$key"` → on failure
  `config_key_missing "$key"` → prints `Missing config key: <key>` and `exit 1`. This is the
  **fail-loud mechanism the new keys extend.** `SMACKEREL_ENV="$(required_value
  runtime.environment)"` (validated to development|test|production; `test`→`test`,
  `self-hosted`→`production`). The generator reads each value into a shell var, then writes
  the `KEY=value` env block; a new key needs BOTH a `required_value` read (+ validation) AND
  an emit line.
- **`Dockerfile`** — multi-stage; the `core` runtime stage (`FROM alpine:3.22 AS core`)
  stamps 5 OCI labels, runs as non-root, `ENTRYPOINT ["smackerel-core"]`. **The place to add
  `LABEL io.smackerel.lifecycle.owner="smackerel"`.**
- **`ml/Dockerfile`** — multi-stage; the runtime stage (`FROM python:3.12-slim`) stamps OCI
  labels. **The place to add the same owner label for `smackerel-ml`.**
- **Test convention** — smackerel shell unit tests are standalone bash scripts under
  `scripts/commands/*_test.sh` (`set -uo pipefail`; compute `REPO_ROOT`; config tests export
  `SMACKEREL_GENERATED_DIR` to a private temp dir + operate on a temp `smackerel.yaml` copy;
  echo PASS/FAIL; non-zero exit on failure). Several are additionally wired into the Go unit
  tier via an `internal/**/*_test.go` driver under `./smackerel.sh test unit --go`.
- **`config/release-trains.yaml`** — `mvp` (active, self-hosted) + `next` (active, staging).
  No flags introduced by this feature.

---

## D1 — Label-Add Prerequisite (`Dockerfile` + `ml/Dockerfile`) — FR-012

Add the identity label to the runtime stage of each Dockerfile (the only build change in the
feature):

```dockerfile
# Dockerfile — core runtime stage (FROM alpine:3.22 AS core), alongside the OCI labels
LABEL io.smackerel.lifecycle.owner="smackerel"

# ml/Dockerfile — smackerel-ml runtime stage (FROM python:3.12-slim), alongside the OCI labels
LABEL io.smackerel.lifecycle.owner="smackerel"
```

- Every image built after this (`./smackerel.sh build` → `docker compose … build`) carries
  the owner label. When a rebuild retags `smackerel-core`/`smackerel-ml`, the previous (now
  `<none>:<none>`) image **retains** its owner label → the label-scoped prune reclaims exactly
  those orphaned versions.
- **`smackerel-*` name fallback (documented, not implemented).** Images built *before* this
  change lack the label; they are skipped by the label filter (safe by omission) and reclaimed
  by a one-time broader operator prune or by a rebuild. The `smackerel-*` dash-prefix pattern
  is the documented reference for that finite window (FR-013). No name-based removal function
  is added.
- Scope 1 also records the read-only finding (spec.md §Label Finding). Parity between the
  Dockerfile `LABEL` literal and the helper's `SMACKEREL_IMAGE_OWNER_LABEL` constant is
  asserted by test `test_owner_label_parity`.

---

## D2 — New SST Config Keys (`config/smackerel.yaml`) — FR-001

Add a `cleanup:` block to the single SST source `config/smackerel.yaml` (fail-loud; no
defaults). Smackerel has **one** SST file (no `.template` to keep in lockstep):

```yaml
# ----------------------------------------------------------------------
# Age-bounded, project-scoped, DEV-PLANE-ONLY UNUSED image reclamation
# (spec 103). Removes orphaned smackerel image *versions* left by frequent
# rebuilds, on every `clean smart` run, so they never accumulate. SST + NO
# DEFAULTS (smackerel-no-defaults): all three keys are REQUIRED and fail-loud
# via required_value/config_key_missing (no fallback, no ${VAR:-default}).
# ----------------------------------------------------------------------
cleanup:
  # Master switch for the smart reclamation stage (true|false, no default).
  remove_unused_images: true
  # Keep anything newer than this (docker image prune --filter until=<N>h).
  # Positive integer hours; data-safe lower bound on eligibility.
  unused_image_min_age_hours: 48
  # project = label-scoped to io.smackerel.lifecycle.owner=smackerel (added by
  #           the Scope 1 label-add); peer-product images are structurally untouched.
  # all     = system-wide, still age-bounded + still env=prod-excluded
  #           (opt-in breadth escape hatch).
  unused_image_scope: project
```

**Contract:**

- All three keys are **required**. Missing/empty → `config_key_missing` prints
  `Missing config key: cleanup.<key>` and exits non-zero.
- `remove_unused_images`: `true` → run stage; `false` → skip (logged); any other value →
  fail loud (`validate_unused_image_policy`).
- `unused_image_min_age_hours`: positive integer; else fail loud.
- `unused_image_scope`: `project` | `all`; else fail loud.

---

## D3 — Loader + Emit Extension (`scripts/commands/config.sh`, fail-loud) — FR-001/FR-017

Extend the generator with three `required_value` reads (fail-loud on missing), a small value
validator, and three emit lines so the values reach the generated env:

```bash
# --- read (fail-loud on missing via config_key_missing) ---
CLEANUP_REMOVE_UNUSED_IMAGES="$(required_value cleanup.remove_unused_images)"
CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS="$(required_value cleanup.unused_image_min_age_hours)"
CLEANUP_UNUSED_IMAGE_SCOPE="$(required_value cleanup.unused_image_scope)"

# --- validate values (fail-loud on invalid; NO default) ---
validate_unused_image_policy   # defined next to the other validators in config.sh

# --- emit into the generated env block (KEY=value lines) ---
CLEANUP_REMOVE_UNUSED_IMAGES=${CLEANUP_REMOVE_UNUSED_IMAGES}
CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS=${CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS}
CLEANUP_UNUSED_IMAGE_SCOPE=${CLEANUP_UNUSED_IMAGE_SCOPE}
```

`validate_unused_image_policy` (new, small; lives in `config.sh` next to the existing
`SMACKEREL_ENV` case validation):

```bash
validate_unused_image_policy() {
  case "$CLEANUP_REMOVE_UNUSED_IMAGES" in
    true|false) ;;
    *) echo "Error: cleanup.remove_unused_images must be true|false, got '$CLEANUP_REMOVE_UNUSED_IMAGES'" >&2; exit 1 ;;
  esac
  if ! [[ "$CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS" =~ ^[1-9][0-9]*$ ]]; then
    echo "Error: cleanup.unused_image_min_age_hours must be a positive integer, got '$CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS'" >&2; exit 1
  fi
  case "$CLEANUP_UNUSED_IMAGE_SCOPE" in
    project|all) ;;
    *) echo "Error: cleanup.unused_image_scope must be project|all, got '$CLEANUP_UNUSED_IMAGE_SCOPE'" >&2; exit 1 ;;
  esac
}
```

Note: `remove_unused_images: false` is a **present** value → `required_value` passes and the
stage is *skipped* (disabled), which is different from a *missing* key → *abort*. No
`${VAR:-default}` is introduced anywhere (smackerel-no-defaults).

---

## D4 — Function Contract (`scripts/lib/cleanup-image-reclamation.sh`) — FR-002/004/005/006/008/014

A new, focused, sourceable helper holds the three NEW functions + module constants.
`smackerel.sh` sources it once, right after the existing `source
"$SCRIPT_DIR/scripts/lib/runtime.sh"`:

```bash
# smackerel.sh, near line 5:
source "$SCRIPT_DIR/scripts/lib/runtime.sh"
source "$SCRIPT_DIR/scripts/lib/cleanup-image-reclamation.sh"   # spec 103
```

Module constants:

```bash
# Project-scope identity — MUST match the LABEL literal added to Dockerfile + ml/Dockerfile
# (Scope 1). A fixed build-identity constant, NOT a runtime-config fallback (smackerel-no-
# defaults governs runtime VALUES; this pairs 1:1 with the Dockerfile literal — test-asserted).
SMACKEREL_IMAGE_OWNER_LABEL="io.smackerel.lifecycle.owner=smackerel"
SMACKEREL_ENV_LABEL_KEY="io.smackerel.environment"   # runtime env classification label
SMACKEREL_PROD_ENV_EXCLUDE="prod"                     # env=prod image exclusion token
SMACKEREL_DEV_SAFE_ENVS="development test"            # dev-plane allow-list (fail-safe)
```

### `assert_dev_plane(smackerel_env)` — PROD-SAFETY guard (FR-008)

```bash
assert_dev_plane() {
  # $1 = the resolved SMACKEREL_ENV (read by the caller from the generated env file).
  # Fail-safe: only run on a known dev-safe plane; anything else (production, empty, unknown)
  # aborts BEFORE any prune.
  local env="${1:-}"
  local safe
  for safe in $SMACKEREL_DEV_SAFE_ENVS; do
    [[ "$env" == "$safe" ]] && return 0
  done
  echo "ERROR: refusing dev-plane image reclamation on a non-dev plane (SMACKEREL_ENV=${env:-<unset>}); this stage runs only on the developer/CI dev Docker daemon" >&2
  exit 1
}
```

- Authoritative prod protection: any value outside {development, test} — including
  `production`, empty, or unknown — aborts non-zero before any prune. This is the real
  prod-leak guard: `--env dev` combined with `runtime.environment: production` in the yaml
  emits `SMACKEREL_ENV=production` (the `case "$TARGET_ENV"` in `config.sh` overrides only
  `test`/`self-hosted`, so `dev` passes `production` straight through), which this guard
  catches.

### `build_unused_image_prune_argv(min_age_hours, scope)` — PURE, testable (FR-002)

Echoes the docker argv (space-joined, **without** the leading `docker`), no side effects.
Validates inputs (positive-int age; scope ∈ {project, all}); fails loud on invalid input.

| `scope` | Emitted argv |
|---|---|
| `project` | `image prune -a -f --filter until=<N>h --filter label=$SMACKEREL_IMAGE_OWNER_LABEL --filter label!=$SMACKEREL_ENV_LABEL_KEY=$SMACKEREL_PROD_ENV_EXCLUDE` |
| `all` | `image prune -a -f --filter until=<N>h --filter label!=$SMACKEREL_ENV_LABEL_KEY=$SMACKEREL_PROD_ENV_EXCLUDE` |

With the constants that renders (project): `image prune -a -f --filter until=48h --filter
label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod`.

- `-a` (remove unused, not just dangling) + `until=<N>h` (age bound) + `label=…` (project
  scope) + `label!=…=prod` (env=prod exclusion) compose with AND semantics in
  `docker image prune`.

### `prune_unused_images_aged(min_age_hours, scope)` — executor (FR-004/005/006/007)

```
local smackerel_env
smackerel_env="$(smackerel_env_value "$(smackerel_require_env_file "$TARGET_ENV")" SMACKEREL_ENV)"
assert_dev_plane "$smackerel_env"                  # PROD-SAFETY: refuse on non-dev plane
local -a argv
read -r -a argv <<< "$(build_unused_image_prune_argv "$min_age_hours" "$scope")"
echo "Reclaiming aged unused smackerel images (scope=$scope, min age ${min_age_hours}h)..."
if smackerel_is_truthy "${DRY_RUN:-}"; then
  echo "[DRY-RUN] Would execute: docker ${argv[*]}"
else
  docker "${argv[@]}"                              # full, unfiltered output (terminal-discipline)
  echo "Aged unused-image reclamation complete (scope=$scope)"
fi
```

**Invariants:**

- Calls `assert_dev_plane` FIRST (prod-safety), before building or running anything.
- NEVER references **volumes** (no `docker volume`, no `--volumes`) — FR-005. The persistent
  postgres/pgvector + NATS jetstream data volumes are structurally out of reach of an
  `image prune`.
- NEVER references **containers** (no `docker container prune`, no `docker rm`) — FR-006.
  (`docker image prune -a` inherently refuses images backing any container; `-f` only skips
  the prompt.)
- Honors the `DRY_RUN` convention via the existing `smackerel_is_truthy` helper
  (`true/TRUE/yes/1/on`) — `[DRY-RUN] Would execute: …`, execute nothing.
- Prints the unfiltered `docker image prune` output (which ends with `Total reclaimed space:
  …`) + a completion line.
- Idempotent / safe on "nothing to reclaim" (Docker reports 0B, exits clean).

### Sourcing (FR-014)

`smackerel.sh` sources `scripts/lib/cleanup-image-reclamation.sh` once. The unit harness
(`scripts/commands/clean_image_reclamation_test.sh`) sources the SAME helper directly, after
defining minimal shims (`smackerel_is_truthy` is trivially re-defined or the harness sources
`runtime.sh` too) + setting `SMACKEREL_ENV` + `SMACKEREL_IMAGE_OWNER_LABEL`, then calls the
pure builder / guard in isolation — **no Docker, no monolith**. This is the smackerel-specific
structural choice (the monolith is not sourceable; the helper is), the analog of the
QuantitativeFinance helper split.

---

## D5 — Wiring Into the `smart)` Arm — FR-010

Insert the gated call **immediately after** the existing `smackerel_run_down "$TARGET_ENV"
false` teardown in the `smart)` arm of `smackerel.sh`, so it runs on **every** `clean smart`
when enabled. Running it AFTER the teardown is intentional: `docker compose down` releases the
running containers' image references first, so aged orphaned versions become eligible while
the just-built current image (< min age) is preserved by the `until` filter:

```bash
      smart)
        if [[ "$TARGET_ENV" == "test" ]]; then
          smackerel_with_stack_lock "$TARGET_ENV" smackerel_run_down "$TARGET_ENV" false
        else
          smackerel_run_down "$TARGET_ENV" false
        fi
        # NEW (spec 103): age-bounded, project-scoped, dev-plane-only unused-image
        # reclamation. Runs after teardown; gated by SST; dev plane only.
        env_file="$(smackerel_require_env_file "$TARGET_ENV")"
        remove_unused="$(smackerel_env_value "$env_file" "CLEANUP_REMOVE_UNUSED_IMAGES")"
        if [[ "$remove_unused" == "true" ]]; then
          prune_unused_images_aged \
            "$(smackerel_env_value "$env_file" "CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS")" \
            "$(smackerel_env_value "$env_file" "CLEANUP_UNUSED_IMAGE_SCOPE")"
        else
          echo "Aged unused-image reclamation disabled (cleanup.remove_unused_images=false)"
        fi
        ;;
```

The `clean)` arm already runs `smackerel_generate_config "$TARGET_ENV" >/dev/null` before the
subcommand `case`, so the generated env carries the `CLEANUP_*` values (after Scope 2) and
`SMACKEREL_ENV` when the stage runs. `TARGET_ENV` is `dev` or `test` (CLI-enforced); the
`assert_dev_plane` guard inside the executor additionally checks the resolved `SMACKEREL_ENV`
(defense-in-depth against a `runtime.environment: production` config).

**Other clean levels untouched.** The `full)`, `status)`, and `measure)` arms are not edited;
the new stage runs in **none** of them (proven by test AC-7). `smackerel_run_down` itself is
unchanged.

---

## D6 — Repo-CLI Test Entrypoint (`clean test`) — FR-015

Add a `test` case to the `clean)` subcommand `case`, **intercepted before**
`require_docker` / `smackerel_generate_config` so the Docker-free unit harness runs without a
Docker daemon or generated env. Structurally: check for `test` at the top of the `clean)` arm
before `require_docker`:

```bash
  clean)
    SUBCOMMAND="${1:-}"
    # spec 103 — repo-CLI unit harness entrypoint (Docker-free; intercept before preflight).
    if [[ "$SUBCOMMAND" == "test" ]]; then
      exec bash "$SCRIPT_DIR/scripts/commands/clean_image_reclamation_test.sh"
    fi
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    case "$SUBCOMMAND" in
      # status) measure) smart) full) ... unchanged ...
    esac
    ;;
```

Invoked as `./smackerel.sh --env dev clean test` (dev plane; terminal-discipline; full
unfiltered output). Also add a `clean test` line to the `clean` help block in `usage`.

`scripts/commands/clean_image_reclamation_test.sh` (smackerel harness style — `set -uo
pipefail`, `REPO_ROOT` compute, `SMACKEREL_GENERATED_DIR` isolation for config tests, echo
PASS/FAIL, non-zero on failure):

- Sources `scripts/lib/cleanup-image-reclamation.sh` and asserts the pure builder + guard
  behavior in isolation (with a shadowed `docker` stub proving non-invocation under DRY_RUN).
- Greps `config/smackerel.yaml` for the 3 `cleanup:` keys and `scripts/commands/config.sh` for
  the 3 `required_value` reads + `validate_unused_image_policy` + the 3 emit lines.
- Runs a temp-copy config-generation with a stripped key → asserts non-zero + the
  `Missing config key: cleanup.<key>` message (via `SMACKEREL_GENERATED_DIR` isolation).
- Greps `smackerel.sh` for the wiring (gated call in the `smart)` arm after `smackerel_run_down`)
  and that `full)`/`status)`/`measure)` do NOT call `prune_unused_images_aged`.
- Greps both Dockerfiles for `LABEL io.smackerel.lifecycle.owner="smackerel"` and asserts the
  helper constant matches the literal (`test_owner_label_parity`).
- Asserts the stage functions are grep-clean of `docker volume` / `--volumes` /
  `docker container` / `docker rm`.

---

## D7 — Test Strategy

| Category | What it proves | Where |
|---|---|---|
| unit — argv builder | project/all argv exact string incl `until=<N>h` + owner label + env=prod exclude; min-age changes only the `until` token; invalid age/scope fail loud | harness (sources helper) |
| unit — guard | `assert_dev_plane` returns 0 for {development,test}; aborts for production/empty/unknown | harness (sources helper) |
| unit — data-safety grep | stage functions contain no `docker volume` / `--volumes` / `docker container` / `docker rm` token | harness (grep helper file) |
| unit — label-add finding | `Dockerfile` + `ml/Dockerfile` carry `LABEL io.smackerel.lifecycle.owner="smackerel"`; helper constant matches the literal | harness (grep Dockerfiles) |
| unit — fail-loud config | strip a key from a temp `smackerel.yaml` copy → non-zero `Missing config key: cleanup.<key>`; invalid bool/age/scope → abort | harness (SMACKEREL_GENERATED_DIR isolation) |
| integration — dry-run | `DRY_RUN=true prune_unused_images_aged 48 project` prints `[DRY-RUN] Would execute: docker image prune …`; shadowed `docker` stub proves non-invocation | harness |
| integration — gated off | `remove_unused_images=false` → stage skipped + "disabled" log; prune line absent | harness / grep |
| unit — other levels unchanged | `full)`/`status)`/`measure)` do not reference `prune_unused_images_aged`; `smackerel_run_down` unchanged | harness (grep monolith) |
| e2e (dev plane, manual/CI) | `DRY_RUN=true ./smackerel.sh --env dev clean smart` shows the exact stage command in the plan | report evidence |

---

## D8 — Governance Mapping

| Governance | How this design satisfies it |
|---|---|
| bubbles-docker-lifecycle-governance | Project-scoped label filter (D4, FR-002/003); age-bound (D4, FR-007); protect persistent volumes / never prune volumes (D4 invariants, FR-005); project-scoped reclamation, disposable rebuildable image layers vs protected stores |
| bubbles-config-sst + smackerel-no-defaults | 3 fail-loud SST keys read via `required_value`/`config_key_missing` + `validate_unused_image_policy`; NO `${VAR:-default}`; single `config/smackerel.yaml` source; generated env never hand-edited (D2/D3, FR-001/017) |
| storage-policy | Image-only prune structurally cannot reach the postgres/pgvector or NATS jetstream data volumes; volumes never referenced (D4, HC-4/NC-005) |
| PROD-SAFETY / data-safe | `assert_dev_plane` refuse-on-non-dev keyed on `SMACKEREL_ENV` (D4, FR-008); env=prod exclusion (D4, FR-009); no volumes/containers (D4, FR-005/006); running-image layers inherently protected |
| terminal-discipline | All ops via `./smackerel.sh … clean …`; harness via `clean test`; full unfiltered output; IDE tools for file writes (D6, FR-015) |

---

## D9 — What This Design Explicitly Does NOT Do

- Does NOT change `clean full`, `clean status`, or `clean measure` (FR-011).
- Does NOT modify `smackerel_run_down` (the new stage is additive, after the teardown call).
- Does NOT add a `smackerel-*` name-based `docker rmi` removal function to the smart stage —
  the label prune covers labeled images; the transition tail is covered by a one-time broader
  prune or a rebuild (FR-013).
- Does NOT stamp `io.smackerel.environment` on images — the env=prod exclusion is
  defense-in-depth behind `assert_dev_plane`.
- Does NOT touch volumes, containers, networks, or any persistent store.
- Does NOT introduce a feature flag (SST-config-gated, train-agnostic).
- Does NOT relocate any existing runtime function into the new helper (only the 3 NEW
  functions live there).

## Single-Capability / Single-Implementation Justification

- **Single capability:** one reclamation stage (`prune_unused_images_aged`) for one outcome
  (reclaim aged, unused, project-scoped smackerel images in the `clean smart` flow). It is a
  new sibling of the existing teardown step — not a new subsystem, not a new hook, not a new
  cron.
- **Single implementation:** one executor + one pure argv-builder + one guard, reusing the
  script's existing `smackerel_env_value` / `smackerel_is_truthy` helpers and the `config.sh`
  fail-loud loader. No new abstraction layer, no provider/adapter split — the variation axis
  (`project` vs `all`) is a single config value handled by one branch in the builder.

## Out of Scope

- Implementing the `smackerel-*` name-pattern fallback as executable removal — documented as
  explicitly-optional transitional coverage only (FR-013).
- Removing any host-level cron/manual prune — that decommission is an operator/knb host action
  outside this product repo; this feature provides the compliant in-product replacement.
- Any change to `smackerel_run_down`, the deploy path, backup/restore, or volume handling.
