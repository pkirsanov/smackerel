# Runbook: [OPS-003] Self-Hosted Deploy Handoff — GAP-06 + BUG-067-001

> **Owner:** `bubbles.devops` · **Target:** `self-hosted` · **Deploy source SHA:** `78b293cc`
> Read [`objective.md`](objective.md) first for what is shipping and why.

This runbook is the operator activation packet for the two runtime-relevant
changes shipped this session. All target-specific values (`<...>`
placeholders) are filled by the operator-private self-hosted deploy adapter (knb
overlay), never by this repo.

---

## PRE-DEPLOY REQUIRED CONFIG

### 🔴 `ML_LOG_LEVEL` — the ML sidecar fails loud without it

BUG-067-001 removed the silent default for the ML sidecar log level. The ML
sidecar now **requires** the value at startup with **no literal fallback**.

| Property | Value |
|----------|-------|
| **SST key (dotted YAML path)** | `services.ml.log_level` |
| **Surfaced env var** | `ML_LOG_LEVEL` |
| **Recommended value** | `info` (the value already committed in `config/smackerel.yaml`) |
| **Allowed values** | `debug` \| `info` \| `warn` \| `error` |
| **Consumer** | `ml/app/main.py::_check_required_config` (reads `os.environ`; raises if missing/empty) |
| **Emitter** | `scripts/commands/config.sh` → `ML_LOG_LEVEL="$(required_value services.ml.log_level)"` |
| **Secret?** | **No** — ships as a literal value in the bundle `app.env` (NOT a `__SECRET_PLACEHOLDER__`) |

**Failure mode if missing:** the ML sidecar container exits at startup with an
explicit `_check_required_config` error naming `ML_LOG_LEVEL`. The core
`/api/health` ML probe then reports the sidecar down (`model_loaded` absent /
`status: degraded`).

### How `ML_LOG_LEVEL` flows into the bundle (verified in this repo @ `78b293cc`)

1. `config/smackerel.yaml` → `services.ml.log_level: info` (the SST source).
2. `scripts/commands/config.sh` reads it via `required_value services.ml.log_level`
   into the `ML_LOG_LEVEL` shell var (fail-loud — aborts if unset).
3. The single env-render template emits `ML_LOG_LEVEL=${ML_LOG_LEVEL}` into the
   generated env file.
4. The bundle path renames that generated env file to `app.env`
   (`grep -v '^# Generated: ' "$OUTPUT_FILE" > "$STAGE_DIR/app.env"`), so the
   **deploy bundle `app.env` carries `ML_LOG_LEVEL` automatically**.

> ✅ **Caveat verified — no gap in the bundle path.** `ML_LOG_LEVEL` IS wired
> into the bundle generation path for SHA `78b293cc`. The only way to deploy
> the new ML image without `ML_LOG_LEVEL` is to pair it with a **stale config
> bundle built before `78b293cc`**. To avoid that:
>
> - Use the `self-hosted-<sourceSha>` bundle whose `<sourceSha>` matches the
>   `ml` image you are deploying (both come from the same CI build of
>   `78b293cc` or later).
> - Do **not** reuse an older `self-hosted-<olderSha>` bundle with the new `ml`
>   image — that older bundle predates the SST key and will start the new ML
>   sidecar with no `ML_LOG_LEVEL` → fail-loud.

### Unchanged production-class prerequisites (context only — not new this session)

The self-hosted target is production-class, so the adapter must already supply
these (see `docs/Deployment.md` → "Generic Pre-Apply Prerequisites"). They are
**unchanged** by this handoff; listed so the operator does not confuse them
with the new `ML_LOG_LEVEL` requirement: `POSTGRES_PASSWORD`,
`AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, `AUTH_SIGNING_ACTIVE_KEY_ID`,
`AUTH_AT_REST_HASHING_KEY`, `AUTH_BOOTSTRAP_TOKEN` (these are managed secrets
emitted as `__SECRET_PLACEHOLDER__<KEY>__` and substituted by the adapter;
`ML_LOG_LEVEL` is NOT a secret and ships inline).

---

## Deploy Steps (Build-Once Deploy-Many — G081)

> **CI builds and signs; CI does NOT deploy.** The push triggers
> `.github/workflows/build.yml`, which builds + cosign-signs the `core` and
> `ml` images, generates per-env config bundles, and publishes
> `build-manifest-<sourceSha>.yaml`. The pipeline **stops at registry push**.
> The operator (different trust boundary) runs the apply.

```bash
# 0) Push the 6 commits (deferred until the host quiesces; do NOT --no-verify)
git push origin main

# 1) After CI is green, fetch the build manifest from the CI run on 78b293cc
gh run download <run-id> --name build-manifest-<sourceSha> --dir /tmp/sm-release

# 2) Promote to self-hosted (resolves core+ml digests + bundle ref from the
#    manifest, then calls apply). PREFERRED entrypoint.
bash scripts/deploy/promote.sh \
  --target self-hosted \
  --build-manifest /tmp/sm-release/build-manifest.yaml

# 2b) OR apply directly with explicit digests copied from the build manifest
./smackerel.sh deploy-target self-hosted apply \
  --image-core=sha256:<core-image-digest> \
  --image-ml=sha256:<ml-image-digest> \
  --config-bundle=self-hosted-<sourceSha> \
  --config-bundle-sha=<sha256-hex> \
  --source-sha=<sourceSha>

# 3) Verify
./smackerel.sh deploy-target self-hosted verify
```

Notes:

- `<sourceSha>` is the source SHA of the CI build (the `78b293cc` tip or a
  descendant). The `ml` image digest and the `self-hosted-<sourceSha>` bundle MUST
  come from the **same** build so `ML_LOG_LEVEL` is present (see caveat above).
- `--config-bundle-sha=<sha256-hex>` is **required** for direct `apply`; copy
  `configBundles[*].sha256` for the `self-hosted` environment out of the build
  manifest. `promote.sh` reads it automatically. Without it the adapter cannot
  verify bundle bytes and the bundle-tamper defense collapses.
- The adapter pins images by `sha256:<digest>` only — never `:latest` / `:main`.
- self-hosted host ports are `41001` (`smackerel-core`) and `41002`
  (`smackerel-ml`); the dev ports `40001` / `40002` MUST NOT be copied into a
  self-hosted runbook.

---

## Post-Deploy Verification

Run `./smackerel.sh deploy-target self-hosted verify` first (adapter-driven health
gate), then confirm the specifics below. Endpoint paths are the **actual** repo
routes (note: the core liveness route is `/api/health`, not `/healthz`).

### 1. Core + ML health

```bash
# Core full liveness check (proxies the ML sidecar /health and surfaces model_loaded)
curl -fsS http://<host-bind-address>:41001/api/health

# Core lightweight readiness probe
curl -fsS http://<host-bind-address>:41001/readyz

# ML sidecar health directly — expect {"status":"up","nats":"connected","model_loaded":true}
curl -fsS http://<host-bind-address>:41002/health
```

- **BUG-067-001 cohesion check:** the ML sidecar comes up healthy with
  `model_loaded: true` and `status: up`. If `ML_LOG_LEVEL` were missing, the
  sidecar would not start and `/health` would be unreachable (core
  `/api/health` would report the ML probe failing). A healthy ML `/health`
  confirms `ML_LOG_LEVEL` is present in the deployed `app.env`.

### 2. GAP-06 surfacing-controller cohesion

After at least one notification has flowed through the decision engine
post-deploy, the `producer="notification"` surfacing fingerprint MUST appear on
the **core** `/metrics`:

```bash
# Expect at least one smackerel_surfacing_* series carrying producer="notification",
# e.g. smackerel_surfacing_nudges_delivered_total{channel="web_push",producer="notification"}
curl -fsS http://<host-bind-address>:41001/metrics \
  | grep -E '^smackerel_surfacing_.*producer="notification"'
```

- If **no** `smackerel_surfacing_*{producer="notification"}` series appears
  after a user-facing notification decision, the decision engine bypassed the
  shared controller (GAP-06 regression — direct dispatch reintroduced).
- The shared budget gauge `smackerel_surfacing_budget_remaining` should also be
  present and reflect both scheduler and notification producers drawing on the
  one global budget (the GAP-06 cohesion: one `Controller` + one `InMemoryAck`
  across `cmd/core`).

### 3. No regression in existing scheduler producers

Confirm the pre-existing scheduler surfacing producers still emit their
`smackerel_surfacing_*` series (the GAP-06 change shares — does not replace —
the controller, so scheduler producers must be unaffected):

```bash
curl -fsS http://<host-bind-address>:41001/metrics \
  | grep -E '^smackerel_surfacing_' | grep -v 'producer="notification"'
```

---

## Rollback (pointer-swap — NEVER rebuilds)

If verification fails or a regression is observed, roll back with a pure
manifest pointer-swap (no rebuild, no image pull of a new digest):

```bash
./smackerel.sh deploy-target self-hosted rollback
```

- This restores the previous `deploy/<target>/manifest.yaml` pointer (previous
  core+ml digests + previous bundle ref) and restarts. It does **not** rebuild.
- Because the previous deployment's bundle is also restored, a rollback returns
  to the previously-valid `ML_LOG_LEVEL` state automatically.

---

## Related Pending Deploys (operator's sequencing call)

The following specs are also **blocked on the owner-directed `bubbles.devops`
self-hosted deploy handoff** and may be bundled with — or sequenced after — this
packet. This packet (GAP-06 + BUG-067-001) is independent of them and can ship
first; the operator decides ordering.

| Spec | Theme | Note |
|------|-------|------|
| `specs/084-open-knowledge-reasoning-loop/` | Open-knowledge reasoning loop | Awaiting self-hosted activation |
| `specs/087-open-knowledge-genuine-synthesis/` | Open-knowledge genuine synthesis | Awaiting self-hosted activation |
| `specs/088-runtime-switchable-models/` | Runtime-switchable models + GPU A/B | Awaiting self-hosted activation |

> **Reconcile at apply time:** the A/B synthesis model was **superseded** —
> `deepseek-r1:7b` → adapter-delegated `qwen3:30b-a3b`. When the open-knowledge
> / runtime-switchable-models specs are activated on self-hosted, confirm the
> adapter's model selection reflects the superseding `qwen3:30b-a3b` and not the
> retired `deepseek-r1:7b`. This OPS-003 packet does not depend on that
> reconciliation.

---

## Quick Reference

| Item | Value |
|------|-------|
| New required SST key | `services.ml.log_level` → env `ML_LOG_LEVEL` = `info` |
| Deploy source SHA | `78b293cc` |
| Core host port (self-hosted) | `41001` |
| ML host port (self-hosted) | `41002` |
| Core liveness route | `GET /api/health` |
| Core readiness route | `GET /readyz` |
| ML health route | `GET /health` |
| GAP-06 fingerprint | `smackerel_surfacing_*{producer="notification"}` on core `/metrics` |
| Promote entrypoint | `bash scripts/deploy/promote.sh --target self-hosted --build-manifest <path>` |
| Verify | `./smackerel.sh deploy-target self-hosted verify` |
| Rollback | `./smackerel.sh deploy-target self-hosted rollback` |
