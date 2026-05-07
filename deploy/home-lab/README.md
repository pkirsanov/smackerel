# Home-Lab Adapter

Installs Smackerel onto a single home-lab host. Trust boundary: an operator with SSH access
to the host runs `apply` from a workstation, OR runs `apply` directly on the host. CI does
NOT have SSH credentials and does NOT invoke `apply`.

## Rollout strategy

`recreate` — accepts brief downtime during image swap. This is the simplest strategy and is
appropriate for a single-tenant home-lab installation. To upgrade to `blue-green`, change
`rolloutStrategy` in `params.yaml` and add a second compose project / nginx upstream switch.

## Files

| File | Purpose |
|------|---------|
| `params.yaml`        | Target knobs: rollout strategy, hostnames, replica counts, host paths |
| `manifest.yaml`      | Current deployment pointer (image digests + bundle hash) — written by `apply` |
| `preconditions.sh`   | Verifies host has docker, cosign, syft, expected paths |
| `bootstrap.sh`       | One-time host setup (creates dirs, installs systemd unit) |
| `apply.sh`           | Pulls images by digest, verifies signatures, swaps manifest pointer, restarts stack |
| `rollback.sh`        | Pointer-swap to `previousManifest`; no rebuild |
| `verify.sh`          | Post-deploy health checks |
| `teardown.sh`        | Removes ONLY what bootstrap/apply created |

## CLI

```bash
./smackerel.sh deploy-target home-lab preconditions
./smackerel.sh deploy-target home-lab bootstrap
./smackerel.sh deploy-target home-lab apply --image-core=sha256:... --image-ml=sha256:... --config-bundle=home-lab-<sourceSha>
./smackerel.sh deploy-target home-lab verify
./smackerel.sh deploy-target home-lab rollback
./smackerel.sh deploy-target home-lab status
./smackerel.sh deploy-target home-lab manifest
./smackerel.sh deploy-target home-lab params
./smackerel.sh deploy-target home-lab teardown
```
