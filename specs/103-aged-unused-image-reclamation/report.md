# Report 103 — Age-Bounded Project-Scoped Unused-Image Reclamation (Smart Cleanup)

**Feature:** `103-aged-unused-image-reclamation`
**Owner:** smackerel
**Status:** implementation pass COMPLETE — all 4 scopes implemented to DoD with real `./smackerel.sh` evidence (below). Validation/certification remain the downstream owners (bubbles.implement does not self-certify to `done`).

> **Real evidence, no fabrication.** Every row below is backed by unfiltered
> terminal output from `./smackerel.sh --env dev clean test` and
> `DRY_RUN=true ./smackerel.sh --env dev clean smart`, captured during the
> implementation pass on 2026-07-13 (macOS, Docker daemon up, no smackerel dev
> stack running). `**Claim Source:** executed` for every item.
>
> **Working-tree note (transparency):** the smackerel working tree carried
> extensive PRE-EXISTING uncommitted changes (~60 files — spec 102
> target-deploy-hardening + related; HEAD lacks the alertmanager bridge, grep=0)
> before this pass. spec 103 was layered PURELY ADDITIVELY on top; no
> pre-existing work was modified or reverted. Per the operator directive and the
> spec's `autoCommit: off` policy, nothing was committed or pushed.

---

## Summary

Add ONE age-bounded, project-scoped, dev-plane-only, data-safe unused-image reclamation stage
to `./smackerel.sh --env dev clean smart` (after the existing volume-preserving
`smackerel_run_down` teardown), so orphaned smackerel image versions are reclaimed
automatically without a governance-noncompliant global host prune. Label finding: smackerel
images carry **no** owner label today (the "else" branch), so Scope 1 **adds**
`io.smackerel.lifecycle.owner=smackerel` to `Dockerfile` + `ml/Dockerfile`; the `smackerel-*`
name fallback is documented-optional.

| Scope | Title | Status |
|---|---|---|
| 1 | Label-add prerequisite (`Dockerfile` + `ml/Dockerfile`) + label finding | Done |
| 2 | SST config keys + fail-loud loader/emit extension | Done |
| 3 | Reclamation helper: pure argv builder + env=prod guard + executor (PROD-SAFETY) | Done |
| 4 | Wire into `clean smart` (gated) + `clean test` entrypoint; other levels unchanged | Done |

---

## Test Evidence

**Phase:** implement · **Claim Source:** executed · **Command:** `./smackerel.sh --env dev clean test` (exit 0, 26 PASS / 0 FAIL) unless noted. Full unfiltered output was captured in the terminal during the run; the harness is the FR-015 Docker-free entrypoint (intercepted before `require_docker`).

### Scope 1 — Label-add prerequisite

- [x] `test_owner_label_added` — `Dockerfile` (core) + `ml/Dockerfile` (runtime) each carry `LABEL io.smackerel.lifecycle.owner="smackerel"` — RED before the label add (3 assertions FAILED), GREEN after. `**Claim Source:** executed`
- [x] `test_owner_label_parity` — helper `SMACKEREL_IMAGE_OWNER_LABEL` matches the Dockerfile literal `io.smackerel.lifecycle.owner=smackerel`. `**Claim Source:** executed`

### Scope 2 — SST config keys (fail-loud)

- [x] `test_config_keys_present` / `test_generated_env_carries_cleanup` — the 3 `cleanup:` keys exist in `config/smackerel.yaml`; `config.sh` reads them via `required_value` + `validate_unused_image_policy` + emits `CLEANUP_*`; a full `--env dev` generation (config-validate OK) writes `CLEANUP_REMOVE_UNUSED_IMAGES=true`, `CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS=48`, `CLEANUP_UNUSED_IMAGE_SCOPE=project`. `**Claim Source:** executed`
- [x] `test_fail_loud_missing_key` — a temp `smackerel.yaml` copy with `unused_image_min_age_hours` stripped aborts non-zero (rc=1) with `Missing config key: cleanup.unused_image_min_age_hours`. `**Claim Source:** executed`
- [x] `test_fail_loud_invalid_{scope,age,bool}` — `unused_image_scope: everything` / `unused_image_min_age_hours: 0` / `remove_unused_images: maybe` each abort non-zero (rc=1) via `validate_unused_image_policy`. `**Claim Source:** executed`

### Scope 3 — Reclamation helper (argv + guard + executor)

- [x] `test_argv_project` / `test_argv_all` / `test_argv_min_age_applied` / `test_argv_env_prod_excluded` — project argv == `image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod`; all argv drops the owner label; min-age drives only `until=<N>h`; both scopes keep the env=prod exclusion. `**Claim Source:** executed`
- [x] `test_guard_refuses_nondev` / `test_guard_allows_dev` — `assert_dev_plane` refuses `production`/`staging`/`` (empty) with the exact `refusing dev-plane image reclamation on a non-dev plane (SMACKEREL_ENV=production)` message and allows `development`/`test`. `**Claim Source:** executed`
- [x] `test_dry_run_no_exec` (shadowed-docker proof) — `DRY_RUN=true prune_unused_images_aged 48 project` prints `[DRY-RUN] Would execute: docker image prune -a -f --filter until=48h …` and the shadowed `docker` stub is NEVER invoked. `**Claim Source:** executed`
- [x] `test_no_volume_tokens` / `test_no_container_tokens` — the helper file is grep-clean of `docker volume` / `--volumes` / `docker container` / `docker rm`. `**Claim Source:** executed`

### Scope 4 — Wiring + `clean test` entrypoint

- [x] `test_smart_wires_stage` / `test_smart_gated_off` — the `smart)` arm calls `prune_unused_images_aged` AFTER `smackerel_run_down "$TARGET_ENV" false`, gated on `CLEANUP_REMOVE_UNUSED_IMAGES`; the disabled branch logs `Aged unused-image reclamation disabled (cleanup.remove_unused_images=false)`. `**Claim Source:** executed`
- [x] `test_smart_dry_run_plan` (SC-001, e2e via CLI) — `DRY_RUN=true ./smackerel.sh --env dev clean smart` (dev plane, exit 0) printed `Reclaiming aged unused smackerel images (scope=project, min age 48h)...` then `[DRY-RUN] Would execute: docker image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod` and executed nothing. `**Claim Source:** executed`
- [x] `test_full_unchanged` / `test_status_measure_unchanged` — `full)` still runs `smackerel_run_down "$TARGET_ENV" true`; `status)` still `smackerel_compose ps -a`; `measure)` still `docker system df`; NONE call `prune_unused_images_aged`. `**Claim Source:** executed`
- [x] `test_clean_test_entrypoint` — `./smackerel.sh --env dev clean test` execs the harness Docker-free (intercept BEFORE `require_docker`) and `clean test` is listed in `usage`. `**Claim Source:** executed`

### Governance evidence

- **terminal-discipline** — all runtime operations via `./smackerel.sh`; full unfiltered output; IDE tools for all file writes. `**Claim Source:** executed`
- **smackerel-no-defaults** — the 3 SST keys read via `required_value` (fail-loud), no `${VAR:-default}`; the only `${VAR:-}` forms are set-u empty-guards (display string / arg guard), proven by the fail-loud tests. `**Claim Source:** executed`
- **docker-lifecycle-governance / storage-policy / PROD-SAFETY** — image-only prune; label-scoped project filter; age bound; dev-plane guard; env=prod exclusion; helper grep-clean of volume/container tokens. `**Claim Source:** executed`
- **shellcheck** — `shellcheck -x scripts/lib/cleanup-image-reclamation.sh scripts/commands/clean_image_reclamation_test.sh` → CLEAN (exit 0). `**Claim Source:** executed`

---

## Completion Statement

> Implementation pass COMPLETE. All 4 scopes are implemented to their Definition of
> Done with real, unfiltered `./smackerel.sh` evidence (26/26 harness assertions PASS,
> exit 0, via the `clean test` FR-015 entrypoint; SC-001 dry-run plan captured via
> `DRY_RUN=true ./smackerel.sh --env dev clean smart`, exit 0). The PROD-SAFETY DoD is
> proven: the stage references no volume/container token (grep-clean helper), never
> prunes volumes or containers, previews under DRY_RUN without executing, fails loud on a
> missing SST key, and `assert_dev_plane` refuses any non-dev plane. `clean full` /
> `clean status` / `clean measure` are unchanged (test-proven).
>
> This is a `bubbles.implement` pass: the implement phase is recorded as an EXECUTION
> CLAIM; the spec is NOT self-certified to `done`. Downstream validation/certification
> (bubbles.validate / bubbles.audit) remain the owners of the terminal `done` status. No
> code was committed and nothing was pushed (operator directive + `autoCommit: off`).
