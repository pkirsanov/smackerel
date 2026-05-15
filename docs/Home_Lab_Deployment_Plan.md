# Home-Lab Deployment Plan — Migrated

> **Status:** Migrated. This document no longer hosts target-specific home-lab
> readiness content.
>
> **Last reviewed:** 2026-05-13 (BUG-001 — `specs/032-documentation-freshness/bugs/BUG-001-home-lab-readiness-plan-stale/`).

## What changed

The previous version of this document blended generic Smackerel product
deployment guidance with target-specific home-lab operator instructions
(host paths, OPS-row planning placeholders, target-coupled volume
contracts, target-coupled secret flows, target-coupled host firewall
rules). Per the project's deployment ownership boundary
(see [.github/copilot-instructions.md → "Deployment Ownership Boundary"](../.github/copilot-instructions.md#deployment-ownership-boundary-non-negotiable))
that mix is a policy violation: anything that would change when a
different operator deploys Smackerel to a different machine MUST live
in the deploy-adapter overlay, not in this product repo.

## Where the content moved

| Old content | New owner |
|-------------|-----------|
| Home-lab adapter readiness checklist (apply / verify / rollback / bootstrap / host singleton / operator-host details / Caddy / tailnet exposure / host monitoring / backup timer paths) | Operator-private deploy-adapter overlay readiness artifact |
| Generic Smackerel CI Build-Once Deploy-Many pipeline behavior | [`docs/Deployment.md`](Deployment.md) |
| Generic Smackerel runtime config contracts (SST, fail-loud auth provisioning, non-default DB credentials) | [`docs/Deployment.md`](Deployment.md) and [`docs/Operations.md`](Operations.md) |
| Generic Smackerel backup contract (dump, retention, status file, restore drill) | [`docs/Deployment.md`](Deployment.md) §"Spec 048 — Deploy Adapter Backup Contract" + [`docs/Operations.md`](Operations.md#backup--restore) |
| Connector framework + per-connector specs | [`specs/`](../specs/) (one folder per connector spec) |
| Obsolete OPS-HOMELAB-1xx planning rows that were never created as artifacts | Removed. No replacement; the work either lives under a real Smackerel spec already, or is target-adapter work in the deploy-adapter overlay. |

## Why this repo no longer hosts a home-lab plan

- This product repo MUST stay deployable to ANY target by ANY operator.
- The per-target adapter (in the deploy-adapter overlay) binds Smackerel
  to a specific machine. That binding includes hostnames, IP addresses,
  Tailscale tailnet identity, reverse-proxy site files, host firewall rules,
  systemd unit names, secret values, and the per-target `manifest.yaml` /
  `params.yaml`.
- Adding any of those operator-coupled values back into THIS repo is a
  blocking policy violation per
  [.github/copilot-instructions.md → "No Env-Specific Content In This Repo"](../.github/copilot-instructions.md#no-env-specific-content-in-this-repo-non-negotiable).

## Where to go

- Operator deploying Smackerel to home-lab: read the operator-private
  deploy-adapter overlay's target-readiness artifact for the target-specific
  apply / verify / rollback / bootstrap / monitoring / backup-shipping plan.
  That artifact composes the generic product contracts documented here with
  the operator's concrete host topology.
- Operator wanting to understand what Smackerel itself produces and
  contracts on (independent of any target): read
  [`docs/Deployment.md`](Deployment.md). It covers the Build-Once
  Deploy-Many CI pipeline, the immutable artifacts (signed images +
  per-env config bundles), the adapter contract surface, the Spec 044
  per-user bearer-auth deployment posture, the Spec 047 vulnerability
  gate, the Spec 048 deploy-adapter backup contract, and the Spec 049
  monitoring profile.
- Day-2 runbook (CLI surface, alert runbook, backup & restore, key
  rotation, enrollment): [`docs/Operations.md`](Operations.md).
