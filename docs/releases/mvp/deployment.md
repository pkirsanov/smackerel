# Deployment — Smackerel MVP

> Per [`bubbles-deployment-target-adapter`](../../../.github/skills/bubbles-deployment-target-adapter/SKILL.md) + Gate G081, this packet is the **narrative** of MVP deployment. Technical correctness of signed-image promotion, per-target adapter behavior, cosign verification, config bundle artifacts, and manifest-pointer mechanics must be **validated by `bubbles.devops` (Tommy Bean)** before any external publication.

## Operational plan to ship MVP

MVP ships under the existing Build-Once Deploy-Many architecture (Gate G081, [`docs/Deployment.md`](../../Deployment.md)). No new deployment surface is introduced. No env-specific content lives in this repo (per `.github/copilot-instructions.md` "No Env-Specific Content" rule).

### Artifacts produced

Per [`docs/Deployment.md`](../../Deployment.md):

| Artifact | Identifier | Source |
|----------|-----------|--------|
| `smackerel-core` image | `ghcr.io/<owner>/smackerel-core@sha256:<digest>` | CI on commit to trunk |
| `smackerel-ml` image | `ghcr.io/<owner>/smackerel-ml@sha256:<digest>` | CI on commit to trunk |
| Config bundle per env | `ghcr.io/<owner>/smackerel-config-bundles:<env>-<sourceSha>` | CI via `./smackerel.sh config generate --env <env> --bundle --source-sha <sha>` |
| Build manifest | `build-manifest-<sourceSha>.yaml` | CI artifact |
| Per-target deployment manifest | `<deploy-overlay-repo>/<target>/manifest.yaml` | Operator-controlled (overlay repo) |

### Infrastructure requirements

| Layer | Component | Source-of-truth spec / doc |
|-------|-----------|----------------------------|
| Container runtime | Docker / Compose v2 | [`docker-compose.yml`](../../../docker-compose.yml), [`deploy/compose.deploy.yml`](../../../deploy/compose.deploy.yml) |
| Datastore | PostgreSQL + pgvector + pg_trgm | spec 002 |
| Bus | NATS JetStream | spec 002, 046 |
| ML sidecar | Python FastAPI + Ollama | spec 002, 050 |
| Reverse-proxy / tailnet edge | Caddy on host fronting `${HOST_BIND_ADDRESS}` (set explicitly by deploy adapter; fails loud if missing per `smackerel-no-defaults`) | spec 042, `.github/instructions/smackerel-no-defaults.instructions.md` |
| Observability | Prometheus + adapter contract (`bubbles-observability-adapter`) | spec 030, 049 |
| Backup | Tiered T1 (ZFS) + T2 (host-local restic) + T3/T4 (optional `OFFSITE_BACKEND` swap) | spec 048 |

### Release-train alignment

MVP gating runs on the **`mvp` train** per [`config/release-trains.yaml`](../../../config/release-trains.yaml) and [`config/feature-flags.mvp.yaml`](../../../config/feature-flags.mvp.yaml). New flags introduced by M1a / M1b / M1c / M2 adjustments MUST be:
- Default-ON in `mvp` bundle
- Default-OFF in `next` bundle (and any other future train)
- Validated by `release-train-guard.sh` at pre-push (`.github/instructions/bubbles-release-trains.instructions.md`)

`bubbles.train` is the sole writer of `release-trains.yaml` and `feature-flags.*.yaml`. Spec-adjustment dispatches MUST packet flag-bundle changes to `bubbles.train` (not write them directly).

### Rollout sequence

1. Spec adjustments (M1a–M5d) merge to trunk under default-OFF flags on `mvp`.
2. As each adjustment lands, its flag flips default-ON in `mvp` bundle (packet to `bubbles.train`).
3. M3 ratification flips after all enforcement-relevant code has been audited against principles 1–10 grep gates (or staged per OQ-7).
4. M4 026 drift fix is independent and can land at any point.
5. M5* MINOR_DRIFT items can land in any order, parallel to M1/M2.
6. Once all M-item dispatches reach terminal-for-mode AND OPS-1 idempotence verification passes, the operator may externally declare "MVP delivered". `bubbles.releases` does NOT make this claim from this packet.

### Rollback strategy

Pure pointer-swap rollback per Gate G081 — `./smackerel.sh deploy-target <target> rollback` reads the prior manifest pointer and re-applies. **No rebuilds during rollback.** Flag-bundle rollback uses `bubbles.train` rollback operation (`.github/instructions/bubbles-release-trains.instructions.md`).

### Health-check + observability requirements

Per spec 049 + [`bubbles-observability-adapter`](../../../.github/skills/bubbles-observability-adapter/SKILL.md):
- Core `/healthz` + ML `/healthz` exposed and scraped
- New M1a surfacing-controller telemetry MUST emit: nudges-per-day counter (per channel + total), acted-on-rate gauge, false-positive ratio gauge, queue-depth gauge per channel
- M1b/M1c telemetry MUST emit: brief-produced counter, reminder-fired counter, reminder-condition-evaluated counter
- M2 wiki telemetry MUST emit: graph-traversal counter (per pivot), annotation-write counter
- All metrics labeled `env=` honoring `bubbles-env-pollution-isolation` rules (test stacks NEVER write to prod metrics endpoint)

**SLO promotion to alerts is RELEASE-V1 scope** — MVP exposes the metrics, does not yet wire them as paged alerts in monitoring stack.

### Test environment isolation

Per `.github/instructions/bubbles-test-environment-isolation.instructions.md` and `bubbles-env-pollution-isolation`:
- All M-item integration/e2e/stress tests use ephemeral test stacks
- No test writes to prod prometheus, prod loki, prod backup paths, or knb manifest
- `env-pollution-scan.sh` blocks PRs that violate

### NO-DEFAULTS / SST compliance

Per `.github/instructions/smackerel-no-defaults.instructions.md`:
- M-item flags MUST be read from env with fail-loud forms (`os.Getenv` + empty-check; `os.environ["X"]`)
- No `${VAR:-default}` fallbacks introduced
- `HOST_BIND_ADDRESS` continues to be set explicitly by deploy adapter; Compose substitution stays fail-loud

### Cross-agent handoff

**bubbles.devops verification required** before external MVP claim:
- Validate that any per-target adapter changes triggered by M-item flag-bundle updates conform to Gate G081
- Validate cosign verification still passes for the MVP build manifest
- Validate config bundle determinism (two CI runs on same SHA produce same bundle)
- Validate no plaintext secrets in MVP bundle
