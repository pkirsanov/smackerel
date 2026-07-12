# Design: BUG-001 Self-Hosted Readiness Docs Belong Outside Product Repo

## Design Brief

This bug is a documentation repair for target-specific self-hosted material that currently lives in the Smackerel product repo. The design is deliberately limited to documentation surfaces and planning traceability. It does not change runtime behavior, target adapters, CI workflows, compose files, generated config, or source code.

## Current Truth

| Finding | Current truth | Documentation correction |
|---------|---------------|--------------------------|
| D-001 | Target-specific self-hosted adapter work and readiness checklists belong to the knb deploy-adapter overlay, not Smackerel product source. | Remove or migrate product-repo self-hosted checklist content and point migration readers to knb spec `003-smackerel-self-hosted-adapter-readiness`. |
| V-006 | The readiness plan overstates or misstates product readiness details. | Rewrite status language to match actual planning artifacts and evidence. |
| V-010 | Product CI Build-Once Deploy-Many readiness and target adapter readiness are blended together. | Keep generic Smackerel product pipeline docs in the product repo and move target apply/verify/rollback details to knb. |
| V-020 | Auth provisioning requirements are not integrated into generic deployment docs. | Add auth signing, issuer, at-rest hashing, bootstrap token, and non-default database credential requirements without values. |
| V-004 | Connector evidence needs caveated live-stack language. | Make unit/static/live-stack evidence classes explicit. |
| DOC-001 | Obsolete OPS rows remain in docs even though the work packets were never created. | Remove those rows or replace them with real spec or bug artifact links. |

## Documentation Change Plan

1. Remove, retire, or migrate `docs/Self_Hosted_Deployment_Plan.md` target-specific checklist content out of the Smackerel product repo.
2. Keep any replacement Smackerel text generic: product deployment contracts, not self-hosted operator instructions.
3. Add the knb spec `003-smackerel-self-hosted-adapter-readiness` cross-reference only as a migration pointer for target-specific readiness.
4. Add generic auth and secret provisioning requirements to deployment prerequisites without target-specific values or paths.
5. Add connector live-stack evidence caveat language.
6. Remove obsolete OPS rows that were never created.
7. Mirror the ownership split and critical prerequisites into `docs/Deployment.md`.

## Test Design

The implementation phase should use documentation grep and artifact lint checks instead of runtime tests. Each test should prove a specific documentation claim exists or a stale claim is absent.

| Test ID | Type | Target | Assertion |
|---------|------|--------|-----------|
| T-DOC-001 | docs-static | Smackerel docs | self-hosted checklist content is absent from product docs or reduced to a migration pointer to knb spec `003-smackerel-self-hosted-adapter-readiness`. |
| T-DOC-002 | docs-static | Smackerel docs | Generic deployment prerequisites contain all four auth key names and non-default database credential requirement. |
| T-DOC-003 | docs-static | Smackerel docs | Connector live-stack evidence caveat language exists. |
| T-DOC-004 | docs-static | Smackerel docs | Obsolete OPS row tokens identified in the review are absent. |
| T-DOC-005 | artifact | bug folder | `artifact-lint.sh` passes for this bug packet. |

## Risk Controls

- The implementation phase must read the current docs before editing, because the worktree is known to be dirty.
- The implementation phase must not touch source, config, CI, compose, or adapter files.
- The implementation phase must preserve the product-repo versus knb target-adapter ownership correction.
