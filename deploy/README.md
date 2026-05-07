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

## Adding a new target

1. Create `deploy/<target>/` mirroring `deploy/home-lab/`
2. Define target-specific knobs in `deploy/<target>/params.yaml`
3. Implement the seven required scripts (preconditions, bootstrap, apply, rollback, verify,
   teardown — plus `manifest.yaml` is written by `apply`)
4. Wire the target into `./smackerel.sh deploy-target <target> <action>`
5. Document the trust boundary (who runs `apply`, with what credentials) in
   `deploy/<target>/README.md`

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
    --config-bundle=home-lab-9f8a7b6c

# 3) Verify
./smackerel.sh deploy-target home-lab verify

# 4) On regression, pointer-swap rollback (no rebuild)
./smackerel.sh deploy-target home-lab rollback
```
