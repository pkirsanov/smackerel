# Spec 103 — Age-Bounded Project-Scoped Unused-Image Reclamation (Smart Cleanup)

**Feature Branch:** `103-aged-unused-image-reclamation`
**Created:** 2026-07-13
**Status:** specs_scoped (authoring/planning pass — no code, no Docker, no push)
**Workflow Mode:** full-delivery
**Release Train:** mvp
**Owner:** smackerel (this session)
**Depends On:** the existing `clean` command (`smackerel.sh` → the `clean)` dispatch: `smart|full|status|measure`), the SST config pipeline (`config/smackerel.yaml` → `scripts/commands/config.sh` `required_value` / `config_key_missing` fail-loud loader → the generated `config/generated/<env>.env` consumed via `scripts/lib/runtime.sh::smackerel_env_value`), and the image build path (`Dockerfile` → `smackerel-core`, `ml/Dockerfile` → `smackerel-ml`, built via `./smackerel.sh build` → `smackerel_compose … build`).

> **This is an authoring pass.** It produces `spec.md` + `design.md` + `scopes.md`
> (plus skeleton `report.md` / `uservalidation.md` / `scenario-manifest.json` /
> `state.json`). It performs **no** code edits, **no** builds, and **no** Docker
> operations. The implementation is a later delivery pass driven by these scopes.
>
> This feature mirrors the completed WanderAide reference
> (`WanderAide/specs/162-aged-unused-image-reclamation`), the GuestHost reference
> (`GuestHost/specs/152-aged-unused-image-reclamation`), and the QuantitativeFinance
> reference (`QuantitativeFinance/specs/096-aged-unused-image-reclamation`) — same gap,
> same fix — adapted to smackerel's structure. Per the **read-only label finding** (see
> `## Label Finding`), smackerel images **do NOT** yet carry an identity/owner label (only
> OCI `org.opencontainers.image.*` labels), so — unlike the three references — this spec
> takes the **"else" branch** of the operator directive: a **label-add prerequisite scope**
> stamps `io.smackerel.lifecycle.owner=smackerel` on the built images, and the
> `smackerel-*` name-pattern fallback is documented as the transitional belt-and-suspenders.

---

## Problem

Smackerel's cleanup command (`./smackerel.sh --env <dev|test> clean <smart|full|status|measure>`)
is **stack-teardown oriented and volume-preserving** today:

1. **`clean status`** — `smackerel_compose "$TARGET_ENV" ps -a` (list project containers).
2. **`clean measure`** — `docker system df` (report daemon disk usage).
3. **`clean smart`** — `smackerel_run_down "$TARGET_ENV" false` →
   `docker compose down --timeout 30 --remove-orphans` (stop the stack; **volumes preserved**).
4. **`clean full`** — `smackerel_run_down "$TARGET_ENV" true` →
   `docker compose down --timeout 30 -v --remove-orphans` (stop the stack; **remove
   project-scoped volumes**).

There is **no `docker image prune` step anywhere in the `clean` command**. Nothing —
neither `smart` nor `full` — removes **UNUSED (non-dangling, orphaned) image *versions***.
`clean full`'s only extra reclamation over `smart` is destroying named volumes; it does
**not** reclaim orphaned image layers, and it has **no age bound**.

**Consequence.** Smackerel is a two-image product (`smackerel-core` from `Dockerfile`,
`smackerel-ml` from `ml/Dockerfile`), rebuilt frequently via `./smackerel.sh build`
(`docker compose … build`). Each rebuild retags `smackerel-core` / `smackerel-ml` and
orphans the previous image version (now untagged `<none>:<none>`, still holding its layers —
`smackerel-ml` in particular carries a multi-GB CPU-PyTorch + preloaded-embedding-model
layer set). These orphaned versions accumulate and are **never removed automatically** by
any `clean` level — disk silently fills until an operator runs a manual, unscoped host
prune.

**A global host prune is the wrong fix.** A host-level `docker image prune -a --filter
until=<h>h` is not project-scoped and can delete *peer products'* images on the shared
Docker daemon (WanderAide, GuestHost, QuantitativeFinance all share it). The correct fix is
a **PROJECT-SCOPED, age-bounded, dev-plane-only** reclamation stage **inside smackerel's own
`clean smart`**, gated by SST config — not a global host cron and not a destructive tier.

---

## Outcome Contract

**Intent.** Add an age-bounded, project-scoped, dev-plane-only, data-safe reclamation stage
to `clean smart` that automatically reclaims smackerel's own orphaned, aged, unused image
versions — safely, on every `clean smart` run, on the DEV plane only — so disk never
silently fills with dead image versions and no operator ever needs a governance-noncompliant
global host prune.

**Success Signal (measurable).** On a **dev-plane** host with smackerel orphaned image
versions older than the configured minimum age, `./smackerel.sh --env dev clean smart`
reclaims them (observable in the `docker image prune` "Total reclaimed space" line), while:

- images newer than the minimum age are preserved,
- non-smackerel (peer-product) images are never touched,
- no volume is ever referenced or pruned,
- no container is ever referenced or pruned, and no running container's image is removed,
- on a **non-dev plane** (`SMACKEREL_ENV=production`, e.g. a `runtime.environment: production`
  config under `--env dev`) the stage refuses to run at all,
- `env=prod`-labeled images are excluded,
- and a `DRY_RUN` preview shows the exact planned `docker image prune` command and changes
  nothing.

**Hard Constraints.**

- **HC-1 — Project-scoped/label-aware.** Under `project` scope the prune MUST filter on the
  smackerel image identity label `io.smackerel.lifecycle.owner=smackerel` (added by the
  label-add prerequisite — see `## Label Finding`) and MUST NOT remove peer products' images
  (bubbles-docker-lifecycle-governance).
- **HC-2 — Age-bounded.** Only images older than `unused_image_min_age_hours` are eligible
  (`docker image prune --filter until=<N>h`).
- **HC-3 — SST + no-defaults.** The three new keys live under `cleanup:` in
  `config/smackerel.yaml` and are fail-loud — a missing key aborts via
  `required_value`/`config_key_missing` ("Missing config key: cleanup.<key>"), an invalid
  value aborts with an explicit validation error; no hardcoded fallback, no `${VAR:-default}`
  (bubbles-config-sst + smackerel-no-defaults).
- **HC-4 — Volumes never referenced or pruned** (data-safe; smackerel's persistent stores —
  the postgres/pgvector data volume and any NATS jetstream volume — MUST be structurally
  untouchable by an image-only prune; storage-policy).
- **HC-5 — Containers never referenced or pruned; a running container's image is never
  removed** (data-safe; `docker image prune -a` refuses in-use image layers; `-f` only skips
  the prompt).
- **HC-6 — Dev-plane only + env=prod guard.** The stage runs on the developer/CI DEV Docker
  daemon only. It MUST refuse to run when the resolved plane is not dev-safe (`SMACKEREL_ENV`
  ∉ {`development`, `test`}) and MUST exclude `env=prod`-labeled images.
- **HC-7 — Other `clean` levels unchanged.** `clean full`, `clean status`, and
  `clean measure` behave exactly as before; the new stage runs **only** in `clean smart`.
- **HC-8 — Repo-CLI only.** All operations via `./smackerel.sh`, full unfiltered output
  (terminal-discipline).

---

## Label Finding *(verified read-only — a required deliverable)*

The operator directive: verify read-only whether smackerel images carry an identity label
(QF/GH/WA use `io.<product>.lifecycle.owner=<product>`); **if smackerel already emits such a
label, use it; else add a label-add prerequisite scope + `smackerel-*` name fallback.**
Read-only verification of `Dockerfile`, `ml/Dockerfile`, `docker-compose.yml`, and the build
path (`smackerel.sh` / `scripts/lib/runtime.sh`):

| Question | Finding (read-only) |
|---|---|
| Is there a `docker-bake.hcl`? | **NO.** Smackerel has no `docker-bake.hcl`. Images are built via `docker compose … build` (`scripts/lib/runtime.sh::smackerel_compose … build`, invoked by `./smackerel.sh build`). |
| Does `Dockerfile` (core) stamp an identity/owner label? | **NO.** The `core` runtime stage stamps only OCI labels: `org.opencontainers.image.version/revision/created/title/source` (`title="smackerel-core"`). No `io.smackerel.lifecycle.owner`. |
| Does `ml/Dockerfile` stamp an identity/owner label? | **NO.** The `smackerel-ml` runtime stage stamps only OCI labels (`org.opencontainers.image.version/revision/created/…`). No `io.smackerel.lifecycle.owner`. |
| **Do smackerel images carry ANY `io.smackerel.*` identity label?** | **NO.** A read-only scan for `io.smackerel` / `LABEL` / `lifecycle.owner` across the repo found **no** image-identity label on either image. Smackerel is the **"else" branch** of the directive. |
| How are the images named? | `smackerel-core` and `smackerel-ml` — a **dash-prefix** naming scheme (also published as `ghcr.io/pkirsanov/smackerel-core` / `…/smackerel-ml`). So a `smackerel-*` name-pattern fallback is **applicable** (unlike QF's slash namespace). |
| Is there an env classification label on images? | **NO** image carries `io.smackerel.environment`. The plane is a runtime signal (`SMACKEREL_ENV`, see below), not a build-time image property. |
| What is the concrete plane signal? | **`SMACKEREL_ENV`** — emitted by `scripts/commands/config.sh` from `runtime.environment` (allowed: `development`\|`test`\|`production`), overridden to `test` when `TARGET_ENV=test` and to `production` when `TARGET_ENV=self-hosted`. Read from the generated `config/generated/<env>.env` via `smackerel_env_value`. **This is the no-fabrication signal `assert_dev_plane` keys off** (dev-safe allow-list {`development`, `test`}; fail-safe refuse otherwise). |

**DECISION (the "else — add a label-add prerequisite scope + `smackerel-*` name fallback"
branch of the operator directive).** Because smackerel images do **not** carry an
identity/owner label:

- **Add the identity label `io.smackerel.lifecycle.owner=smackerel`** to the runtime stages
  of **both** `Dockerfile` (core) and `ml/Dockerfile` (ml) — this is the **label-add
  prerequisite (Scope 1)**, the genuine structural difference from the three references
  (whose labels already existed). Every image built after Scope 1 carries the label; the
  untagged `<none>` versions those images later orphan **retain** the label, so the
  label-scoped prune reclaims exactly the orphaned versions the current `clean` never removes.
- **Use `io.smackerel.lifecycle.owner=smackerel` as the project-scope image filter.**
  `docker image prune -a --filter label=io.smackerel.lifecycle.owner=smackerel` only removes
  images that **carry** the label, so peer-product images (no smackerel owner label) are
  structurally untouched, and any un-labeled image is simply **skipped** (safe by omission,
  never over-removed).
- **`smackerel-*` name-pattern fallback — documented, transitional, explicitly OPTIONAL.**
  Because smackerel starts **without** the label, images built *before* Scope 1 lack it and
  are skipped by the label filter. The dash-prefixed `smackerel-*` name pattern is the
  documented belt-and-suspenders for that finite transition window: those pre-label images
  are reclaimed either by a one-time broader operator prune, or automatically once rebuilt
  (the rebuild stamps the label). The name fallback is **documented** in `design.md`
  (mirroring how the references documented their `<product>-*` fallbacks as
  explicitly-optional) — it is **not** implemented as a second, reference-based removal
  mechanism in the smart stage, to keep the change narrow and data-safe.
- **`env=prod` exclusion is defense-in-depth.** `--filter label!=io.smackerel.environment=prod`
  is added to **both** scopes. No smackerel image carries that label today (env is a runtime
  classification, not a build property), so the exclusion is future-proofing behind the
  primary `assert_dev_plane` host guard — exactly the posture the QF/GH references adopted.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Aged orphaned smackerel image versions are reclaimed automatically (Priority: P1)

An operator, the build path, or the pre-push flow runs `clean smart` on a dev host. Orphaned
smackerel image versions older than the configured minimum age are reclaimed after the stack
teardown, without a manual host prune, and without touching anything else on the shared
daemon.

**Why this priority**: This is the core reclamation the feature exists to deliver; it removes
the need for a noncompliant global host prune.

**Independent Test**: Configure `remove_unused_images: true`, `unused_image_min_age_hours: 48`,
`unused_image_scope: project`; run `DRY_RUN=true ./smackerel.sh --env dev clean smart` and
assert the planned command is `docker image prune -a -f --filter until=48h --filter
label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod`.

**Acceptance Scenarios**:

```gherkin
Scenario: AC-1 aged unused project images are reclaimed on clean smart
  Given config/smackerel.yaml sets cleanup.remove_unused_images = true
  And cleanup.unused_image_min_age_hours = 48
  And cleanup.unused_image_scope = project
  And SMACKEREL_ENV = development
  When clean smart runs the new reclamation stage after the stack teardown
  Then it executes: docker image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod
  And it logs the reclaimed space

Scenario: AC-2 images newer than the minimum age are preserved
  Given a smackerel unused image created 3 hours ago
  And unused_image_min_age_hours = 48
  When the reclamation stage runs
  Then the "until=48h" filter excludes that image
  And the image is preserved
```

---

### User Story 2 — The reclamation stage is project-scoped, data-safe, and off by config (Priority: P1)

The stage never removes peer-product images, never references volumes or containers, and can
be turned off entirely from SST config. A dry run previews the exact command and changes
nothing.

**Why this priority**: Data-safety and project-scoping are the governance gate; without them
the stage is no better than the global host prune it replaces.

**Independent Test**: Set `unused_image_scope: project`, `DRY_RUN=true`, and assert the
`--filter label=io.smackerel.lifecycle.owner=smackerel` filter is present and no
`docker volume` / `--volumes` / `docker container` / `docker rm` token appears anywhere in
the plan or the stage functions.

**Acceptance Scenarios**:

```gherkin
Scenario: AC-3 peer-product images are never removed under project scope
  Given a non-smackerel image (no io.smackerel.lifecycle.owner=smackerel label) exists
  And unused_image_scope = project
  When the reclamation stage runs
  Then the label filter scopes the prune to owner-labeled smackerel images only
  And the non-smackerel image is not referenced and not removed

Scenario: AC-4 the stage is skipped when disabled
  Given cleanup.remove_unused_images = false
  When clean smart runs
  Then the reclamation stage is skipped
  And it logs "Aged unused-image reclamation disabled (cleanup.remove_unused_images=false)"

Scenario: AC-5 DRY_RUN previews the plan and never touches volumes, containers, or state
  Given DRY_RUN is set
  When the reclamation stage runs
  Then it prints "[DRY-RUN] Would execute: docker image prune -a -f --filter until=<N>h ..."
  And it executes no docker image prune
  And no docker volume command is referenced anywhere in the stage
  And no docker container / docker rm command is referenced anywhere in the stage
```

---

### User Story 3 — SST keys are fail-loud and the other clean levels are untouched (Priority: P2)

The three new keys are single-source-of-truth and fail loud when missing/invalid; `clean full`,
`clean status`, and `clean measure` keep their exact current behavior.

**Why this priority**: Preserves the no-defaults contract and guarantees a narrow,
non-regressive change surface.

**Independent Test**: Remove `unused_image_min_age_hours` from a temp copy of
`config/smackerel.yaml` and run config generation against it; assert it aborts non-zero with
"Missing config key: cleanup.unused_image_min_age_hours".

**Acceptance Scenarios**:

```gherkin
Scenario: AC-6 a missing SST key fails loud
  Given cleanup.remove_unused_images is present
  But cleanup.unused_image_min_age_hours is absent
  When config generation resolves the cleanup keys
  Then config_key_missing aborts non-zero with "Missing config key: cleanup.unused_image_min_age_hours"
  And no docker prune is executed

Scenario: AC-7 the other clean levels are unchanged
  Given clean full, clean status, and clean measure
  When each runs
  Then clean full still runs smackerel_run_down "$TARGET_ENV" true (down -v) unchanged
  And clean status still runs smackerel_compose ps -a unchanged
  And clean measure still runs docker system df unchanged
  And the new reclamation stage runs in NONE of them
```

---

### User Story 4 — The stage is dev-plane only and prod-safe (Priority: P1)

The stage refuses to run when the resolved plane is not dev-safe, excludes
`env=prod`-labeled images, and never removes a running container's image or any volume.

**Why this priority**: This is the PROD-SAFETY gate. A reclamation stage that could run on a
prod host or touch prod images/volumes is a production-data hazard and is unacceptable
regardless of how well the rest works.

**Independent Test**: Generate a config with `runtime.environment: production` (so
`SMACKEREL_ENV=production` under `--env dev`) and run the stage; assert `assert_dev_plane`
aborts non-zero with a "refusing … non-dev plane" message and executes no docker prune.

**Acceptance Scenarios**:

```gherkin
Scenario: AC-8 the env=prod guard refuses to run on a non-dev plane
  Given SMACKEREL_ENV = production (or any value not in {development, test})
  When the reclamation stage runs
  Then assert_dev_plane aborts non-zero with "refusing dev-plane image reclamation on a non-dev plane (SMACKEREL_ENV=production)"
  And no docker image prune is executed
  And no image is removed

Scenario: AC-9 env=prod-labeled images are excluded
  Given the reclamation stage builds its docker argv
  When project or all scope is used
  Then the argv contains "--filter label!=io.smackerel.environment=prod"
  And an image labeled io.smackerel.environment=prod is excluded from removal

Scenario: AC-10 the identity label is added to the build (label-add prerequisite)
  Given smackerel images carried no io.smackerel.lifecycle.owner label before this feature (read-only finding)
  When the label-add prerequisite stamps io.smackerel.lifecycle.owner=smackerel in Dockerfile and ml/Dockerfile
  Then a newly built smackerel-core / smackerel-ml image carries io.smackerel.lifecycle.owner=smackerel
  And the untagged <none> version it later orphans retains that label
  And the project-scope label filter reclaims it

Scenario: AC-11 never prune volumes or containers; never remove a running container's image
  Given the reclamation stage runs (real or dry-run)
  When it operates
  Then it references no docker volume / --volumes token
  And it references no docker container prune / docker rm token
  And docker image prune -a leaves any image backing a container in place (Docker refuses in-use layers)
  And the persistent postgres/pgvector and NATS jetstream volumes are never referenced
```

### Edge Cases

- **Invalid `unused_image_scope`** (not `project`/`all`) → fail loud (no silent default).
- **Non-integer / non-positive `unused_image_min_age_hours`** → fail loud.
- **Invalid `remove_unused_images`** (not `true`/`false`) → fail loud.
- **`unused_image_scope: all`** → age-bounded system prune (`until=<N>h`, no identity label
  filter) — still `env=prod`-excluded, still volume/container-safe, still dev-plane-guarded.
  A documented, config-opt-in breadth escape hatch, NOT the default.
- **`SMACKEREL_ENV` unset, empty, or `production`** → `assert_dev_plane` refuses (fail-safe:
  anything outside {development, test} aborts).
- **No eligible images** → the stage logs "nothing to reclaim" (Docker reports 0B), exits 0,
  sets no error.
- **An orphaned version whose layers back a running container** → Docker refuses to remove
  in-use images; no additional container reference is needed.
- **Untagged `<none>` orphans built *before* the label-add (Scope 1)** → carry neither the
  owner label nor a running reference; skipped by the label filter (safe by omission). This
  finite transition tail is reclaimed by a one-time broader operator prune, or automatically
  once the image is rebuilt (the rebuild stamps the label). Documented limitation, not a
  regression — this is precisely why the `smackerel-*` name fallback is documented.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `config/smackerel.yaml` MUST define three new keys under a `cleanup:` block:
  `remove_unused_images` (bool), `unused_image_min_age_hours` (positive int),
  `unused_image_scope` (`project` | `all`). Each is SST and **fail-loud** — a missing/empty
  key aborts via `required_value`/`config_key_missing` ("Missing config key: cleanup.<key>");
  an invalid value aborts with an explicit validation error (`validate_unused_image_policy`).
  No default, no fallback, no `${VAR:-default}`.
- **FR-002**: A new pure builder `build_unused_image_prune_argv(min_age_hours, scope)` MUST
  echo the docker argv: `image prune -a -f --filter until=<min_age_hours>h` plus, when
  `scope=project`, `--filter label=io.smackerel.lifecycle.owner=smackerel` (from a module
  constant), plus — in **both** scopes — `--filter label!=io.smackerel.environment=prod`
  (env=prod exclusion). The identity label MUST come from the module constant, not
  re-hardcoded inline in the argv.
- **FR-003**: Under `scope=project` the stage MUST use the
  `label=io.smackerel.lifecycle.owner=smackerel` filter and MUST NOT remove any image lacking
  that owner label (no peer-product removal).
- **FR-004**: A new executor `prune_unused_images_aged(min_age_hours, scope)` MUST honor a
  `DRY_RUN` convention (when `DRY_RUN` is truthy: print `[DRY-RUN] Would execute: <cmd>` and
  execute nothing) and MUST print the unfiltered `docker image prune` output and a
  reclaimed-space summary.
- **FR-005**: The stage MUST NEVER reference or prune **volumes** (no `docker volume`, no
  `--volumes` token anywhere in the stage functions).
- **FR-006**: The stage MUST NEVER reference or prune **containers** (no
  `docker container prune`, no `docker rm`), and MUST NEVER remove a running container's
  image (`docker image prune -a` refuses in-use layers; `-f` only skips the prompt).
- **FR-007**: The stage MUST be age-bounded: images newer than `unused_image_min_age_hours`
  are preserved by the `until` filter.
- **FR-008**: The stage MUST be **dev-plane only**. `assert_dev_plane()` MUST refuse
  (non-zero, execute nothing) when the resolved plane is not dev-safe — i.e. when
  `SMACKEREL_ENV` (read from the generated `config/generated/<env>.env`) is not in
  {`development`, `test`}. It MUST be called before any prune in `prune_unused_images_aged`.
- **FR-009**: The prune MUST exclude `env=prod`-labeled images via
  `--filter label!=io.smackerel.environment=prod` in **both** scopes.
- **FR-010**: The stage MUST be wired into the `clean smart` arm of `smackerel.sh`,
  **immediately after** the existing `smackerel_run_down "$TARGET_ENV" false` teardown, gated
  by `remove_unused_images`. It MUST run on **every** `clean smart` when enabled.
- **FR-011**: `clean full`, `clean status`, and `clean measure` behavior MUST be unchanged;
  the new stage MUST run in **none** of them.
- **FR-012**: **Label-add prerequisite (Scope 1)** — because smackerel images carry no
  identity label today (read-only finding), `Dockerfile` (the `core` runtime stage) **and**
  `ml/Dockerfile` (the `smackerel-ml` runtime stage) MUST each add
  `LABEL io.smackerel.lifecycle.owner="smackerel"`. Newly built images (and the untagged
  `<none>` versions they later orphan, which retain their labels) then carry the project-scope
  filter's label. No other build change.
- **FR-013**: **`smackerel-*` name-pattern fallback (documented, OPTIONAL)** — smackerel
  images are dash-prefixed (`smackerel-core` / `smackerel-ml`), so a `smackerel-*` name
  pattern is applicable for the finite transition window before all images are rebuilt with
  the label. This fallback is **documented** in `design.md` as an explicitly-optional
  belt-and-suspenders; it is **not** implemented as a second reference-based removal mechanism
  in the smart stage (the label prune plus a one-time broader operator prune, or a rebuild,
  cover the transition tail).
- **FR-014**: The pure builder `build_unused_image_prune_argv` and the guard
  `assert_dev_plane` MUST be unit-testable **without invoking Docker and without executing the
  `smackerel.sh` monolith** (whose bottom `case "$COMMAND"` dispatch runs on `source`). They
  live in a focused, sourceable helper `scripts/lib/cleanup-image-reclamation.sh` (sourced by
  `smackerel.sh`, alongside the existing `source scripts/lib/runtime.sh`); the unit harness
  sources that helper directly (with `log_*` / `die` shims + injected `SMACKEREL_ENV`).
- **FR-015**: A repo-CLI entrypoint MUST run the cleanup unit harness via `./smackerel.sh`
  (terminal-discipline) — a new `clean test` subcommand (`./smackerel.sh --env dev clean test`)
  routing to `scripts/commands/clean_image_reclamation_test.sh`, intercepted **before** the
  `require_docker` / config-generation preflight so the harness runs Docker-free.

### Quality and Completeness Requirements

- **FR-016**: All required test categories for a shell/build-tooling feature (unit +
  integration for the new functions, the wiring, the label-add finding, the prod-safety
  guard, and the fail-loud config contract) MUST be defined with zero skipped tests.
- **FR-017**: No defaults, fallbacks, stubs, or placeholders are introduced for any new SST
  key (no `${VAR:-default}`, no `awk … || echo <default>` for the three keys); missing =
  fail loud (smackerel-no-defaults).

### Non-Negotiable Project Constraints *(mandatory)*

- **NC-001**: All build/test/deploy/cleanup workflows MUST run via `./smackerel.sh` (no direct
  `docker`, `go`, `python`, `pytest`, `docker compose` for the operator surface) —
  terminal-discipline.
- **NC-002**: All configuration MUST come from `config/smackerel.yaml` (SST); no hardcoded
  thresholds, no defaults/fallbacks — fail loud on missing keys (bubbles-config-sst +
  smackerel-no-defaults; hand-editing `config/generated/*.env` is forbidden).
- **NC-003**: Protobuf-only business-data rule — **not applicable** (this feature exchanges
  no business data; it is build/ops tooling).
- **NC-004**: Tests are mandatory (unit + integration for the new functions, the wiring, the
  label-add finding, and the fail-loud config contract), zero skips.
- **NC-005**: Data-safety (storage-policy) — the stage MUST NEVER reference or prune volumes;
  the persistent postgres/pgvector data volume and any NATS jetstream volume are structurally
  out of reach of an image-only prune.

### Key Entities

- **smackerel cleanup config** (`config/smackerel.yaml`, new `cleanup:` block): gains
  `remove_unused_images`, `unused_image_min_age_hours`, `unused_image_scope`; emitted into the
  generated env as `CLEANUP_REMOVE_UNUSED_IMAGES` / `CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS` /
  `CLEANUP_UNUSED_IMAGE_SCOPE`.
- **smackerel image-identity label**: `io.smackerel.lifecycle.owner=smackerel`, **added** by
  the label-add prerequisite to `Dockerfile` + `ml/Dockerfile`, consumed as the project-scope
  filter.
- **smackerel env label**: `io.smackerel.environment` (value `prod` excluded via
  `--filter label!=io.smackerel.environment=prod`; defense-in-depth, not stamped on images
  today).
- **Plane signal**: `SMACKEREL_ENV` (development|test|production; dev-safe allow-list
  {development, test}).
- **Reclamation stage**: `build_unused_image_prune_argv` + `assert_dev_plane` +
  `prune_unused_images_aged` (in `scripts/lib/cleanup-image-reclamation.sh`),
  `validate_unused_image_policy` + the loader/emit extension (in `scripts/commands/config.sh`),
  the gated wiring + `clean test` intercept (in `smackerel.sh`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With the stage enabled on a dev plane,
  `DRY_RUN=true ./smackerel.sh --env dev clean smart` prints the exact planned command
  including `until=<N>h`, the project owner-label filter, and the env=prod exclusion — and
  changes nothing.
- **SC-002**: Orphaned smackerel image versions older than the configured age are reclaimed by
  `clean smart` (reclaimed space appears in the summary), eliminating the need for a global
  host prune.
- **SC-003**: Zero non-smackerel images are removed by the project-scoped stage, and zero
  volumes and zero containers are referenced (proven by test).
- **SC-004**: A missing SST key aborts config generation non-zero (fail-loud), proven by test;
  `clean full` / `clean status` / `clean measure` behavior is byte-for-byte unchanged; on a
  non-dev plane the guard refuses.

## Release Train

**Train:** `mvp` (declared in `config/release-trains.yaml`, `phase: active`,
`target_slot: self-hosted`). This feature introduces **no feature flags**
(`flagsIntroduced: []`); it is a build-tooling change gated by SST config keys, not by a
release-train flag. Behavior on other trains (`next`) is identical because the change is
train-agnostic developer/CI tooling.

## Governance Cited (enforced by design + scopes)

- **bubbles-docker-lifecycle-governance** — project-scoped/label-aware pruning over broad
  prune (HC-1, FR-002/003); protect persistent volumes (HC-4, FR-005; storage-policy);
  project-scoped cleanup, age-bound eligibility rather than proof-of-freshness by timestamp;
  disposable rebuildable image layers vs protected persistent stores.
- **bubbles-config-sst + smackerel-no-defaults** — the three new keys are SST and fail-loud
  (HC-3, FR-001/FR-017); no hardcoded fallback, no `${VAR:-default}`; the `config.sh`
  `required_value`/`config_key_missing` loader already fails fast on missing keys; generated
  `config/generated/*.env` is never hand-edited.
- **storage-policy** — never touch the persistent postgres/pgvector or NATS jetstream data
  volumes; an image-only prune structurally cannot reach them (HC-4, NC-005).
- **Data-safe / PROD-SAFETY** — age-bounded (HC-2, FR-007), volumes never referenced (HC-4,
  FR-005), containers never referenced and running-image layers inherently protected (HC-5,
  FR-006), dev-plane-only guard (HC-6, FR-008), env=prod exclusion (FR-009).
- **terminal-discipline** — all operations via `./smackerel.sh`, full unfiltered output
  (HC-8, FR-015); IDE tools for file writes; no output truncation in evidence.
