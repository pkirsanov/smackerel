# Smackerel Deployment

This directory implements the build-once-deploy-many pattern from the bubbles framework
(`bubbles-deployment-target-adapter` SKILL, gate G074).

The same `git SHA` produces:

1. **One immutable application image** per service (`smackerel-core`, `smackerel-ml`),
   identified by `repository@sha256:<digest>`, signed with cosign keyless and accompanied by
   SBOM + SLSA provenance attestations.
2. **One immutable config bundle per environment** (`dev`, `prod`, `home-lab`, ...),
   identified by `<env>-<sourceSha>`. Bundles are deterministic tarballs of the env files
   produced by `./smackerel.sh config generate --env <env> --bundle`.
3. **One mutable deployment manifest per target** (`deploy/<target>/manifest.yaml`),
   pinning a specific `<image digest>` + `<config bundle hash>` pair for that target.

CI publishes artifacts (1) and (2) and stops. Deploy is downstream of CI on a different
trust boundary, performed by a per-target adapter (`deploy/<target>/apply.sh`). `apply` MUST
NOT build, MUST NOT fall back to local build, MUST verify cosign signature, MUST verify
bundle hash, and MUST be idempotent. `rollback` is a pure pointer-swap on `manifest.yaml`'s
`previousManifest` field — no rebuild.

## Layout

```
deploy/
├── README.md           ← this file
├── contract.yaml       ← SST-derived build/deploy contract (consumed by adapters)
├── _example/
│   └── target-skeleton/  ← stubbed skeleton; copy to start a new target
└── home-lab/           ← per-target adapter: home-lab installation
    ├── README.md
    ├── params.yaml     ← target-specific knobs (rollout strategy, replicas, hostnames)
    ├── manifest.yaml   ← current deployment pointer (image digest + bundle hash)
    ├── preconditions.sh
    ├── bootstrap.sh
    ├── apply.sh
    ├── rollback.sh
    ├── verify.sh
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

## Required Bind-Address Contract

Deploy Compose uses fail-loud interpolation for host bind addresses:

```yaml
ports:
    - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
```

There is no `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback. A deploy adapter MUST write `HOST_BIND_ADDRESS` explicitly into the `app.env` it supplies before running Compose. Use `127.0.0.1` only as an explicit env value when loopback binding is intended; use the target's operator-owned bind address when tailnet-edge fronting requires it. Missing or empty values are expected to abort Compose at substitution time.

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
./smackerel.sh deploy-target home-lab apply \
    --image-core=sha256:abc123... \
    --image-ml=sha256:def456... \
    --config-bundle=home-lab-9f8a7b6c \
    --config-bundle-sha=<sha256-hex>   # BUG-047-001 / DEVOPS-HL-002 — copy from configBundles[env=home-lab].sha256 in the build manifest

# 3) Verify
./smackerel.sh deploy-target home-lab verify

# 4) On regression, pointer-swap rollback (no rebuild)
./smackerel.sh deploy-target home-lab rollback
```
