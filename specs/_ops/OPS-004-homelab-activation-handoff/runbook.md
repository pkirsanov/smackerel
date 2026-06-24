# Runbook: [OPS-004] Home-Lab Activation Handoff (Consolidated)

> **Owner:** `bubbles.devops` · **Target:** `home-lab` · **Deploy source SHA:** `e0e7bc4f`
> Read [`objective.md`](objective.md) first for what is shipping and why.
> **Supersedes [`OPS-003`](../OPS-003-gap06-bug067-homelab-deploy-handoff/) for the live deploy.**

This runbook is the operator activation packet for deploy source `e0e7bc4f`.
All target-specific values (`<...>` placeholders) are filled by the
operator-private home-lab deploy adapter (the `knb` overlay, resolved via
`DEPLOY_TARGETS_ROOT` → `${DEPLOY_TARGETS_ROOT}/smackerel/home-lab/`), never by
this repo. The deploy host is referred to ONLY as `home-lab` / `<deploy-host>`
/ `<host-bind-address>`.

> **CI is NOT the producer.** The GitHub workflows (`build.yml` / CI / E2E /
> client) are `disabled_manually`. The images + per-env config bundle are built
> by the in-repo **local-operator** path (below). Do NOT trigger GitHub CI.

---

## PRE-DEPLOY REQUIRED CONFIG

### 🔴 1. `ML_LOG_LEVEL` — the ML sidecar fails loud without it (BUG-067-001)

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

> ✅ **Bundle path verified — no gap.** `ML_LOG_LEVEL` IS wired into the bundle
> generation path. The single failure mode is reusing a **stale config bundle
> built before the SST key landed** with the new `ml` image. Avoid that by
> using the `home-lab-<sourceSha>` bundle whose `<sourceSha>` matches the `ml`
> image you deploy — both come from the **same** local-operator build of
> `e0e7bc4f`. Never pair the new `ml` image with an older `home-lab-<olderSha>`
> bundle.

### 🔴 2. Synthesis model selection MUST be `qwen3:30b-a3b` (NOT the retired `deepseek-r1:7b`)

The home-lab open-knowledge **synthesis** default is `qwen3:30b-a3b` — the
operator-optimized standing default (2026-06-21), superseding the retired
`deepseek-r1:7b` A/B candidate (per OPS-003) and the interim `gpt-oss:20b` /
spec-089's `deepseek-r1:32b`.

| Property | Value |
|----------|-------|
| **Adapter key (operator-private `params.yaml`)** | `model_selection.assistant_open_knowledge_synthesis_model_id` |
| **Required value** | `qwen3:30b-a3b` |
| **Must NOT be** | `deepseek-r1:7b` (retired), `gpt-oss:20b` (interim), `deepseek-r1:32b` (superseded) |
| **Envelope** | `20480` MiB; co-resident with `gemma4:26b` gather (`18432`) = `38912` MiB inside the `49152` MiB / 48G home-lab envelope |
| **Enforcement** | `internal/config/config.go::validateModelEnvelopes` — the standing-default co-residence guard fails loud (Gate G028) if the selection breaches the envelope |

> The model **name** is a generic public Ollama model class (not host-specific),
> so it is named here. The actual `model_selection` block lives in the
> operator-private adapter `params.yaml`; this packet only states the required
> resolved value.

### Unchanged production-class prerequisites (context only — not new this activation)

The home-lab target is production-class, so the adapter must already supply
these managed secrets (see [`docs/Deployment.md`](../../../docs/Deployment.md) →
"Generic Pre-Apply Prerequisites"). They are **unchanged** by this handoff,
listed so the operator does not confuse them with the two new requirements
above: `POSTGRES_PASSWORD`, `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`,
`AUTH_SIGNING_ACTIVE_KEY_ID`, `AUTH_AT_REST_HASHING_KEY`, `AUTH_BOOTSTRAP_TOKEN`
(emitted as `__SECRET_PLACEHOLDER__<KEY>__` and substituted by the adapter).
The adapter also supplies the fail-loud deploy-compose env the home-lab stack
requires: `HOST_BIND_ADDRESS`, the per-service CPU/memory limits
(`CORE_*`/`ML_*`/`POSTGRES_*`/`NATS_*`), and the ROCm GPU group GIDs
(`OLLAMA_RENDER_GID` / `OLLAMA_VIDEO_GID`) — all NO-DEFAULT / fail-loud (G028).

---

## BUILD (local-operator path — produces the signed artifacts + manifest)

> The operator runs this on the build machine. It is the **artifact producer**
> in place of the disabled GitHub CI. NOTHING in this packet runs it for you.

```bash
# Operator key + passphrase (NEVER echo the value; type it into the terminal).
# scripts/commands/build-home-lab.sh requires OPERATOR_COSIGN_KEY /
# OPERATOR_COSIGN_PUBKEY (default ~/.config/knb/operator-keys/cosign-operator.{key,pub})
# and COSIGN_PASSWORD. read -rs is silent by design.
read -rs COSIGN_PASSWORD && export COSIGN_PASSWORD

# Build, Trivy-gate, sign, attest, and write the local build manifest for e0e7bc4f.
./smackerel.sh build --target home-lab
```

What this produces (all from source SHA `e0e7bc4f`):

| Artifact | Identifier |
|----------|-----------|
| `smackerel-core` image | `ghcr.io/pkirsanov/smackerel-core@sha256:<digest>` (pushed, **operator-signed**, SBOM + attested) |
| `smackerel-ml` image | `ghcr.io/pkirsanov/smackerel-ml@sha256:<digest>` (pushed, **operator-signed**, SBOM + attested) |
| Config bundle | `ghcr.io/pkirsanov/smackerel-config-bundles:home-lab-<sourceSha>` (carries `ML_LOG_LEVEL` in `app.env`) |
| Build manifest | `dist/local-build-manifests/local-build-manifest-<sourceSha>.yaml` (`trustModel: local-operator`) |

> **BUG-047-003 scope 4 (CVE re-proof) happens HERE.** Build step `[3/7]` runs
> `trivy image --severity CRITICAL,HIGH --exit-code 1 --ignore-unfixed` against
> the freshly re-baked `smackerel-core` (which carries the
> `apk upgrade libssl3 libcrypto3` fix for **CVE-2026-45447**). A clean build
> IS the full-image Trivy re-scan re-proof; a regressed OpenSSL CVE with an
> available fix fails the build with `[F017-BUILD-04]`. Capture the build's
> Trivy `PASS smackerel-core...` line as the scope-4 evidence.

---

## PROMOTE / APPLY / VERIFY (exact commands)

The operator points `DEPLOY_TARGETS_ROOT` at the `knb` overlay so the strict
adapter resolver finds `${DEPLOY_TARGETS_ROOT}/smackerel/home-lab/params.yaml`.

```bash
# 0) Tell the CLI where the operator-private adapter lives (knb overlay).
export DEPLOY_TARGETS_ROOT=<path-to-knb-deploy-targets-root>   # operator-private

# 1) Promote (PREFERRED). Resolves core+ml digests, the home-lab bundle ref,
#    its sha256, and trustModel=local-operator from the local manifest, then
#    execs `smackerel.sh deploy-target home-lab apply ...`.
bash scripts/deploy/promote.sh \
  --target home-lab \
  --build-manifest dist/local-build-manifests/local-build-manifest-<sourceSha>.yaml

# 1b) OR apply directly with explicit values copied from the local manifest.
./smackerel.sh deploy-target home-lab apply \
  --image-core=sha256:<core-image-digest> \
  --image-ml=sha256:<ml-image-digest> \
  --config-bundle=home-lab-<sourceSha> \
  --config-bundle-sha=<sha256-hex> \
  --source-sha=<sourceSha> \
  --trust-model=local-operator

# 2) Verify (adapter-driven health gate).
./smackerel.sh deploy-target home-lab verify
```

Notes:

- `<sourceSha>` is `e0e7bc4f` (the full 40-char SHA in the manifest filename).
  The `ml` image digest and the `home-lab-<sourceSha>` bundle MUST come from
  the **same** build so `ML_LOG_LEVEL` is present (see the caveat above).
- `--config-bundle-sha=<sha256-hex>` is **required** for direct `apply` (the
  adapter verifies bundle bytes before mounting; without it the bundle-tamper
  defense collapses). `promote.sh` reads it automatically from the manifest and
  refuses to promote if it is missing or not a 64-char hex digest.
- `--trust-model=local-operator` is what the adapter uses to verify the images
  against the **operator** cosign key (not CI keyless / Rekor). `promote.sh`
  forwards it from the manifest's `trustModel` field automatically.
- The adapter pins images by `sha256:<digest>` only — never `:latest` / `:main`.
- Home-lab host ports are `41001` (`smackerel-core`) and `41002`
  (`smackerel-ml`); the dev ports `40001` / `40002` MUST NOT be copied into a
  home-lab runbook. Infra (`postgres` / `nats`) publish **no** host port —
  reach them via `tailscale ssh <deploy-host> -- docker exec ...` (Pattern P1).

---

## Post-Deploy Live Verifications (per-bug, with pass signals)

Run `./smackerel.sh deploy-target home-lab verify` first, then confirm each item
below. Endpoint paths are the **actual** repo routes: core liveness is
`/api/health` (NOT `/healthz`), core readiness is `/readyz`, ML health is
`/health`. `<host-bind-address>` is the adapter-set bind IP (tailnet IP or
loopback).

### 1. Core + ML health — closes BUG-067-001 (`ML_LOG_LEVEL`)

```bash
# Core full liveness (proxies the ML sidecar /health and surfaces model_loaded)
curl -fsS http://<host-bind-address>:41001/api/health

# Core lightweight readiness probe
curl -fsS http://<host-bind-address>:41001/readyz

# ML sidecar health directly — expect {"status":"up","nats":"connected","model_loaded":true}
curl -fsS http://<host-bind-address>:41002/health
```

- **Pass signal:** ML `/health` returns `status: up` with `model_loaded: true`.
  If `ML_LOG_LEVEL` were missing the sidecar would not start and `/health` would
  be unreachable (core `/api/health` would report the ML probe failing). A
  healthy ML `/health` confirms `ML_LOG_LEVEL` is present in the deployed
  `app.env`.

### 2. BUG-047-003 scope 5 — live `/ask` after the CVE-fix core deploy

After the CVE-fixed `smackerel-core` is live (scope 4 re-proof captured at build
time), confirm the running core actually serves an open-knowledge `/ask`:

- **Pass signal:** a live `/ask` open-knowledge query (see step 3 below for the
  exact request + fields) returns a **sourced answer** from the redeployed
  core — proving the OpenSSL-patched image is healthy and serving, not just
  passing a static scan.

### 3. BUG-064-001 / BUG-064-002 — live `/ask` sourced answer (NOT refusal)

Issue an open-knowledge question through the canonical programmatic surface
(`POST /api/assistant/turn`, spec 069) — or the Telegram `/ask` transport — on
the live GPU stack:

```bash
# Canonical programmatic surface (per-user bearer auth supplied by the operator).
curl -fsS -X POST http://<host-bind-address>:41001/api/assistant/turn \
  -H "Authorization: Bearer <user-bearer-token>" \
  -H "Content-Type: application/json" \
  -d '{"transport":"web","message":"/ask compare X and Y and recommend one"}'
```

- **Pass signal (BUG-064-001):** the response is a real **sourced answer**, NOT
  the canonical refusal. Confirm the open-knowledge result carries
  `scenario_id=open_knowledge`, `num_sources > 0`, and
  `termination_reason != cap_usd`.
- **Pass signal (BUG-064-002):** the persisted capture title does **not** carry
  a leaked `/ask ` prefix, and the answer is **not** a 3× snippet-dump and has
  **no** `thinking…` header — it reads as a single synthesized answer.

### 4. GAP-06 (spec 054) — surfacing budget controller cohesion

After at least one notification has flowed through the decision engine
post-deploy, the `producer="notification"` surfacing fingerprint MUST appear on
the **core** `/metrics`:

```bash
# Expect at least one smackerel_surfacing_* series carrying producer="notification".
curl -fsS http://<host-bind-address>:41001/metrics \
  | grep -E '^smackerel_surfacing_.*producer="notification"'

# The shared budget gauge should also be present (one global budget across
# scheduler + notification producers).
curl -fsS http://<host-bind-address>:41001/metrics \
  | grep -E '^smackerel_surfacing_budget_remaining'
```

- **Pass signal:** at least one `smackerel_surfacing_*{producer="notification"}`
  series exists. Its **absence** after a user-facing notification decision means
  the decision engine bypassed the shared controller (GAP-06 regression —
  direct dispatch reintroduced).

### 5. specs 084 / 087 / 088 — open-knowledge GPU A/B re-verify

These three specs are `blocked` in-repo **solely** on a live GPU re-verify:

- **084** open-knowledge reasoning-loop (multi-hop: `max_iterations` tool turns
  + 1 forced-synthesis turn).
- **087** genuine-synthesis (the tools-stripped forced-final synthesis turn
  using `qwen3:30b-a3b`).
- **088** runtime-switchable-models (per-request `--model=` / API `model`
  against the home-lab synthesis-switch allowlist).

- **Pass signal:** an operator **A/B comparison** on the live stack shows the
  open-knowledge path performing genuine multi-hop reasoning and synthesis (not
  a snippet dump), and a runtime model switch on the synthesis turn takes effect
  — with the standing synthesis default resolving to `qwen3:30b-a3b`.

### 6. BUG-069-002 — client-disconnect durability

Prove a durable write survives `r.Context()` cancellation:

1. Issue a `/api/assistant/turn` (or a capture) request to the live core.
2. **Drop the client mid-flight** (kill the curl / close the socket before the
   response completes).
3. Assert the capture STILL persisted — read it back from Postgres and confirm
   the NATS durable write landed (Postgres/NATS have no host port; use
   Pattern P1):

```bash
# Postgres (Pattern P1 — no host port; reach via tailscale ssh + docker exec).
tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "SELECT id, created_at FROM captures ORDER BY created_at DESC LIMIT 5;"
```

- **Pass signal:** the capture row exists in Postgres (and the corresponding
  NATS subject received it) **even though the client disconnected** before the
  HTTP response finished — the durable write is decoupled from the request
  context.

---

## ROLLBACK (pointer-swap — NEVER rebuilds)

If verification fails or a regression is observed, roll back with a pure
manifest pointer-swap (no rebuild, no new digest pull):

```bash
./smackerel.sh deploy-target home-lab rollback
```

- Restores the previous `deploy/<target>/manifest.yaml` pointer (previous
  core+ml digests + previous bundle ref) and restarts. It does **not** rebuild.
- Because the previous deployment's bundle is also restored, rollback returns to
  the previously-valid `ML_LOG_LEVEL` and model-selection state automatically.

---

## Day-1 Ops (first live home-lab operational cycle)

Today only DEV-stack SLO evidence exists; the live home-lab needs its first
real cycle. Treat these as **activation gates**, not assumed-covered:

1. **First post-apply backup** of the live home-lab stack (T1/T2). See
   [`docs/Upkeep_Runbook.md`](../../../docs/Upkeep_Runbook.md).
2. **Restore-drill** into an ephemeral, isolated namespace (torn down on exit,
   success or failure) — per the upkeep / BCDR doctrine.
3. **Operate-plane SLO capture** on the live stack (latency / availability) so
   the home-lab has real operate-plane evidence, not just DEV. See
   [`docs/Operations.md`](../../../docs/Operations.md).

> **Known limitation (NOT a blocker):** offsite **T3/T4 + BCDR remain WARN**
> (`offsite_required: false`) until backup hardware lands. The promote path does
> not block on offsite while `offsite_required` is `false`; the operator flips
> it when USB/cloud arrives.

---

## Separate Operator Action — Tagged Release (NOT home-lab)

**BUG-058 (chrome-extension bridge)** is NOT a home-lab deploy and does NOT flow
through `promote.sh` / the adapter. It unblocks ONLY when the operator cuts a
**tagged release**:

```bash
# Operator-cut signed tag — triggers CI keyless-cosign signing of the
# chrome-bridge zip to PUBLIC Rekor (refs/tags/v* path).
git tag -s v<x.y.z> -m "release v<x.y.z>"
git push origin v<x.y.z>
```

- **Why separate:** the chrome-bridge zip is signed via **CI keyless cosign**
  to public Rekor on the `refs/tags/v*` path — a CI / release action with a
  different trust boundary from the local-operator home-lab deploy. Sequence it
  independently of this home-lab activation.

---

## Appendix — Out-of-knb Follow-up (in-repo cert matter, NOT this handoff)

These 5 bugs are **fix-complete + verified-green in-repo** but still
`in_progress` because the `state-transition-guard` **G022 done-cert pipeline was
never run** (light-touch fixes). They are an **in-repo framework-cert matter —
NOT a knb / home-lab deploy** — and are listed only so nothing falls through:

| Bug | Nature |
|-----|--------|
| **BUG-034-004** | expense-rows err unchecked (light-touch fix) |
| **BUG-076-001** | ML agent logs raw conversational content (light-touch fix) |
| **BUG-095-001** | route-guard compiler provenance (light-touch fix) |
| **BUG-073-003** | canary CI toolchain gating (light-touch fix) |
| **BUG-077-002** | (spec 077 light-touch fix) |

**Resolution path (in-repo, not here):** run a real
`bubbles.workflow bugfix-fastlane` certification for each (so the G022 done-cert
pipeline executes and flips status to terminal), OR explicitly accept them as
cert-deferred. Either way it is **out of scope for this home-lab handoff**.

---

## Quick Reference

| Item | Value |
|------|-------|
| Deploy source SHA | `e0e7bc4f` (current `origin/main` HEAD; supersedes OPS-003's `78b293cc`) |
| Artifact producer | local-operator: `./smackerel.sh build --target home-lab` (GitHub CI is `disabled_manually`) |
| Local build manifest | `dist/local-build-manifests/local-build-manifest-<sourceSha>.yaml` (`trustModel: local-operator`) |
| Required SST key | `services.ml.log_level` → env `ML_LOG_LEVEL` = `info` (fail-loud) |
| Required synthesis model | `qwen3:30b-a3b` (NOT `deepseek-r1:7b`) |
| Core host port (home-lab) | `41001` |
| ML host port (home-lab) | `41002` |
| Core liveness route | `GET /api/health` |
| Core readiness route | `GET /readyz` |
| ML health route | `GET /health` |
| GAP-06 fingerprint | `smackerel_surfacing_*{producer="notification"}` on core `/metrics` |
| `/ask` pass signal | `scenario_id=open_knowledge`, `num_sources>0`, `termination_reason != cap_usd` |
| Promote entrypoint | `bash scripts/deploy/promote.sh --target home-lab --build-manifest <path>` |
| Verify | `./smackerel.sh deploy-target home-lab verify` |
| Rollback | `./smackerel.sh deploy-target home-lab rollback` |
| Tagged-release (separate) | `git tag -s v<x.y.z>` → CI keyless-cosign signs chrome-bridge (BUG-058) |
