# Home-Lab Master Deployment Plan — MIGRATED

> **Status (2026-05-13):** This file's previous contents — a cross-product
> coordination plan covering hardware, OS, networking, reverse proxy,
> backup destinations, port allocation, and a rollout schedule — have been
> migrated **out of the Smackerel product repo**.
>
> The cross-product home-lab Master Plan was operator-coupled
> (real Linux user, real Wi-Fi NIC name, real BIOS specs, real backup
> paths, real Tailscale subdomain pattern) and named multiple sibling
> projects. As such it violated the
> [`.github/copilot-instructions.md`](../.github/copilot-instructions.md)
> "No Env-Specific Content In This Repo" non-negotiable policy AND was
> structurally inconsistent with this repo's per-product scope.
>
> The companion file
> [`docs/Home_Lab_Deployment_Plan.md`](Home_Lab_Deployment_Plan.md) was
> migrated by spec 032 / BUG-001 (commit `899507be`) for the same
> reason. This file completes that migration via spec 032 / BUG-002.

---

## Where the operator-coupled content lives now

The home-lab cross-product coordination plan now lives in the operator-private
deploy-adapter overlay repo, alongside the home-lab adapter
implementation (`apply.sh`, `bootstrap.sh`, `params.yaml`,
`manifest.yaml`, `verify.sh`, `rollback.sh`, `teardown.sh`,
`preconditions.sh`). Per the deployment ownership boundary recorded in
[`.github/copilot-instructions.md`](../.github/copilot-instructions.md),
home-lab adapter content is owned by the deploy-adapter overlay, not by this
product repo.

| What you are looking for | Where it lives now |
|--------------------------|--------------------|
| The home-lab cross-product coordination plan (hardware, OS, networking, reverse-proxy contract, backup destinations, port allocation, rollout schedule, Uptime-Kuma monitor catalog, Tailscale ↔ Docker bridge mitigations) | Operator-private deploy-adapter overlay — home-lab cross-product Master Plan |
| The home-lab adapter implementation for Smackerel (`apply.sh`, `params.yaml`, `manifest.yaml`, etc.) | Operator-private deploy-adapter overlay, home-lab adapter readiness artifact |
| The generic deployment guide for Smackerel (CI pipeline, build-once-deploy-many, cosign keyless, signed bundles, adapter contract, Pre-Apply Prerequisites, Connector Live-Stack Evidence Caveat) | This repo — [`docs/Deployment.md`](Deployment.md) |
| The Smackerel-side per-product home-lab plan (also migrated to a pointer stub) | This repo — [`docs/Home_Lab_Deployment_Plan.md`](Home_Lab_Deployment_Plan.md) (60-line migration-pointer stub) |
| The product-side runbook (operator-facing daily ops, lifecycle, monitoring, backup, auth, connectors) | This repo — [`docs/Operations.md`](Operations.md) |
| Operator command surface (config generate, build, deploy-target, status, logs, etc.) | `./smackerel.sh` and the strict `DEPLOY_TARGETS_ROOT` resolution rule in [`scripts/commands/deploy_target.sh`](../scripts/commands/deploy_target.sh) |

---

## Why this repo no longer ships the cross-product Master Plan

This repo is a **single-product** deploy artifact producer. The
cross-product Master Plan named four sibling products (`qf`, `s`,
`gh`, `wa`) and embedded operator-coupled topology that would change
per operator and per host. Such content has no portable meaning inside
a single-product repo and would silently leak operator data on every
clone.

The Build-Once Deploy-Many architecture (gate G074) deliberately
splits responsibility:

- **This repo (`s`)** publishes immutable signed images, SBOM + SLSA
  attestations, and per-environment config bundles. It owns the
  generic adapter contract that ANY operator can implement for ANY
  target.
- **The deploy-adapter overlay** owns target-specific knowledge
  (real hostnames, real IPs, real Tailscale identifiers, real
  reverse-proxy site files, real ufw rules, real systemd unit names,
  real secret values) and consumes the published artifacts.

If you need the historical cross-product Master Plan, it has been
preserved in the deploy-adapter overlay's home-lab adapter documentation. If you operate Smackerel on a target other than
home-lab, this file is intentionally not relevant to you — see
[`docs/Deployment.md`](Deployment.md) and
[`deploy/README.md`](../deploy/README.md) for the generic adapter
contract and the locality rule for adding a new target.

