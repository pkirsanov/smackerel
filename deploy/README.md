# Smackerel Deployment

This directory implements the build-once-deploy-many pattern from the bubbles framework
(`bubbles-deployment-target-adapter` SKILL, gate G074).

The same `git SHA` produces:

1. **One immutable application image** per service (`smackerel-core`, `smackerel-ml`),
   identified by `repository@sha256:<digest>`, signed with cosign keyless and accompanied by
   SBOM + SLSA provenance attestations.
2. **One immutable config bundle per environment** (`dev`, `test`, `self-hosted`),
   identified by `<env>-<sourceSha>`. Bundles are deterministic tarballs of the env files
   produced by `./smackerel.sh config generate --env <env> --bundle`. CI builds exactly
   these three environments (`.github/workflows/build.yml` `build-bundles` matrix); there
   is no `prod` bundle.
3. **One mutable deployment manifest per target** (`deploy/<target>/manifest.yaml`),
   pinning a specific `<image digest>` + `<config bundle hash>` pair for that target.

CI publishes artifacts (1) and (2) and stops. Deploy is downstream of CI on a different
trust boundary, performed by a per-target adapter (`deploy/<target>/apply.sh`). `apply` MUST
NOT build, MUST NOT fall back to local build, MUST verify cosign signature, MUST verify
bundle hash, and MUST be idempotent. `rollback` is a pure pointer-swap on `manifest.yaml`'s
`previousManifest` field — no rebuild.

## Layout

This in-tree `deploy/` directory holds only the **generic, target-agnostic**
deploy surface — the SST-derived contract, the deploy Compose file, the copyable
skeleton, and the observability overlay. It deliberately contains **no
operator-coupled adapter** (no real FQDNs/IPs/site files):

```
deploy/
├── README.md            ← this file
├── contract.yaml        ← SST-derived build/deploy contract (consumed by adapters)
├── compose.deploy.yml   ← deploy-time Compose (fail-loud HOST_BIND_ADDRESS interpolation)
├── _example/
│   └── target-skeleton/ ← stubbed skeleton; copy to start a new target
└── observability/       ← telemetry overlay consumed by adapters
```

The operator-coupled **self-hosted adapter is out-of-tree**: it lives in the
operator-private overlay (the `knb` repo) and is resolved by `DEPLOY_TARGETS_ROOT`
(see *Adapter Locality* below), never in this repo. Its internal shape is the
[`_example/target-skeleton/`](_example/target-skeleton/) expanded with real
target values:

```
<deployment-owner>/<product>/<target>/   ← per-target adapter (operator-private)
├── README.md
├── params.yaml      ← target-specific knobs (rollout strategy, replicas, hostnames)
├── manifest.yaml    ← current deployment pointer (image digest + bundle hash)
├── preconditions.sh
├── bootstrap.sh
├── apply.sh
├── rollback.sh
├── verify.sh
├── status.sh        ← optional read-only adapter status; product CLI falls back if absent
└── teardown.sh
```

## Adapter Locality (In-Tree vs Out-of-Tree)

Per-target adapters carry **operator-coupled topology** (real FQDNs, real LAN/VPN IPs, real reverse-proxy site filenames). The CLI supports two locality modes; it never silently falls back between them.

| Mode | Adapter Root | When To Use |
|------|--------------|-------------|
| **In-tree** (default) | `<repo>/deploy/<target>/` | Generic / shareable targets only (skeletons, vendor templates, `local-dev`). Safe for public repos. |
| **Out-of-tree** (operator-private) | `${DEPLOY_TARGETS_ROOT}/smackerel/<target>/` | Operator-owned targets that name a real host. **Required** when this repo is or will be public. |

### CLI Resolution Rule (STRICT — no silent fallback)

| `DEPLOY_TARGETS_ROOT` | Resolved adapter directory |
|-----------------------|----------------------------|
| **unset**             | `<repo>/deploy/<target>/` only (in-tree) |
| **set**               | `${DEPLOY_TARGETS_ROOT}/smackerel/<target>/` only (out-of-tree) |

If the resolved path is missing, the CLI fails with a structured error listing the path it tried, the path it deliberately did NOT try, and the opt-in/opt-out hint for `DEPLOY_TARGETS_ROOT`. Setting `DEPLOY_TARGETS_ROOT` is an explicit operator opt-in: "all my adapters live out-of-tree now". The CLI will NOT fall back to a stale in-tree leftover.

## Status action

`./smackerel.sh deploy-target <target> status` first resolves the adapter through
the same strict locality rule. If the resolved adapter provides an executable
`status.sh`, the CLI delegates to it and passes through any remaining status
arguments. This keeps manifest, runtime, edge, and contract-drift knowledge in
the adapter that owns the target.

When `status.sh` is missing or not executable, the CLI prints an explicit
`adapter status script unavailable` message and shows only a generic read-only
Docker container summary for `smackerel-<target>`. The fallback does not replace
adapter drift checks and must not be treated as readiness proof.

## Required Bind-Address Contract

Deploy Compose uses fail-loud interpolation for host bind addresses:

```yaml
ports:
    - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
```

There is no `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback. A deploy adapter MUST write `HOST_BIND_ADDRESS` explicitly into the `app.env` it supplies before running Compose. Use `127.0.0.1` only as an explicit env value when loopback binding is intended; use the target's operator-owned bind address when tailnet-edge fronting requires it. Missing or empty values are expected to abort Compose at substitution time.

## Adapter-Supplied Host Env (not in the SST bundle)

A small set of values are HOST-SPECIFIC (they change per operator/host) and are
therefore NOT emitted into the SST config bundle. The deploy adapter MUST write
them explicitly into `app.env` before running Compose; each uses the fail-loud
`${VAR:?...}` form so a missing value aborts Compose at substitution time
(Gate G028, NO-DEFAULTS):

| Env var | Meaning | How the adapter resolves it |
|---------|---------|------------------------------|
| `HOST_BIND_ADDRESS` | Host bind address for published ports | `127.0.0.1` (loopback) or the target's tailnet IP |
| `OLLAMA_RENDER_GID` | Host `render` group GID for `/dev/dri` access (ROCm GPU inference) | `getent group render \| cut -d: -f3` on the real host |
| `OLLAMA_VIDEO_GID` | Host `video` group GID for `/dev/dri` access (ROCm GPU inference) | `getent group video \| cut -d: -f3` on the real host |

The `OLLAMA_RENDER_GID` / `OLLAMA_VIDEO_GID` pair is only consulted when the
`ollama` Compose profile is active (spec 082 SCOPE-082-09 routed these out of
the generic compose because the GIDs vary per host/distro).

## Adding a new target

The skeleton at [`_example/target-skeleton/`](_example/target-skeleton/) is a fully-stubbed adapter that exits 1 on every action until filled in.

```bash
# In-tree (generic, shareable, safe for public repos):
cp -r deploy/_example/target-skeleton deploy/<your-target>

# Out-of-tree (operator-coupled, public-repo-safe):
mkdir -p "${DEPLOY_TARGETS_ROOT}/smackerel"
cp -r deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/smackerel/<your-target>"
```

Then replace every `<placeholder>` in `params.yaml`, implement each script's `TODO(operator)` block, and wire the target name into `./smackerel.sh deploy-target <target> <action>`. Document the trust boundary (who runs `apply`, with what credentials) in `<your-target>/README.md`.

See [`bubbles-deployment-target.instructions.md`](../.github/instructions/bubbles-deployment-target.instructions.md) for the full contract and the public-repo safety checklist.

## Operator workflow

```bash
# 1) Build artifacts in CI (out of band) → produces image digest + bundle hash
#    Artifacts published to: ghcr.io/pkirsanov/smackerel-core@sha256:...
#                            ghcr.io/pkirsanov/smackerel-ml@sha256:...
#                            ghcr.io/pkirsanov/smackerel-config-bundles/<env>-<sourceSha>.tar.gz

# 2) Operator picks a release (image digests + bundle hash) and applies to the target
./smackerel.sh deploy-target self-hosted apply \
    --image-core=sha256:abc123... \
    --image-ml=sha256:def456... \
    --config-bundle=self-hosted-9f8a7b6c \
    --config-bundle-sha=<sha256-hex>   # BUG-047-001 / DEVOPS-HL-002 — copy from configBundles[env=self-hosted].sha256 in the build manifest

# 3) Verify
./smackerel.sh deploy-target self-hosted verify

# 4) On regression, pointer-swap rollback (no rebuild)
./smackerel.sh deploy-target self-hosted rollback
```

## Per-Spec SST Key Catalogs

`contract.yaml` carries a `sstKeyCatalog:` section enumerating
per-spec REQUIRED config keys (with `secret: true|false` annotation)
that the runtime expects in the mounted env bundle. The catalog is
informational — adapters do NOT default these values; they consume
the bundle as-is and inject secrets at apply time via the spec 052
secret-injection path. Currently catalogged: spec 064 (open-knowledge
agent). See [`docs/Deployment.md`](../docs/Deployment.md#spec-064-deployment-notes-open-knowledge-agent)
for the operator-facing rollout/rollback notes.
