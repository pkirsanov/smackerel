# Bug: BUG-001 - Home-lab readiness docs belong outside product repo

## Classification

- **Type:** Documentation freshness bug
- **Severity:** High
- **Parent Spec:** 032 - Documentation Freshness
- **Findings:** V-006, V-010, V-020, V-004, DOC-001
- **Ownership correction:** D-001 is not a Smackerel product-side defect. The home-lab adapter and target-specific checklist are intentionally owned by the knb deploy-adapter overlay, where spec `003-smackerel-home-lab-adapter-readiness` tracks that work.

## Problem Statement

The home-lab readiness review found that Smackerel documentation still presents target-specific home-lab deployment material as product-repo documentation. That is no longer accurate.

The product repo owns generic runtime contracts, CI Build-Once Deploy-Many pipeline behavior, generated configuration contracts, and product documentation that can be installed anywhere. The knb deploy-adapter overlay owns target-specific apply, verify, rollback, bootstrap, host singleton, operator-host details, concrete secret injection paths, Caddy/tailnet exposure, host monitoring wiring, and backup timer paths. The docs need to reflect that split by moving or removing home-lab-specific checklists from Smackerel and keeping only generic deployment docs in the product repo.

The stale docs also under-report generic auth key provisioning, overstate live-stack connector evidence, and retain obsolete OPS rows for work packets that were never created. Those issues make the documentation unreliable as an execution map.

## D-001 Correction

The earlier review language that implied Smackerel was missing `deploy/home-lab/` adapter scripts has been corrected. Smackerel must not create product-side adapter folders or target-specific home-lab deployment checklists. The adapter surface belongs to the knb deploy-adapter overlay and is represented there by spec `003-smackerel-home-lab-adapter-readiness`.

This bug packet therefore plans only Smackerel documentation corrections: the product docs should remove or migrate home-lab-specific checklist material out of Smackerel, keep generic deployment contracts in Smackerel, and point target-specific operators to the knb planning packet.

## Affected Documentation

- `docs/Home_Lab_Deployment_Plan.md` (remove, retire, or migrate target-specific contents out of the product repo)
- `docs/Deployment.md`

## Acceptance Criteria

- Home-lab-specific readiness checklist content is removed from Smackerel docs or migrated to knb target-adapter planning/docs.
- Smackerel docs keep only generic deployment contracts that can apply to any target.
- Generic deployment docs include the product-required auth key names without values: `auth.signing.hmac_key`, `auth.signing.issuer`, `auth.at_rest_hashing_key`, and `auth.bootstrap_token`.
- Generic deployment docs document non-default database credential requirements without target-specific values or paths.
- Generic deployment docs include the connector live-stack evidence caveat from the readiness review.
- Obsolete OPS rows that were never created are removed or replaced with real existing planning links.
- `docs/Deployment.md` reflects the same generic product-vs-target-adapter ownership split and readiness caveats.

## Constraints

- Do not create Smackerel target adapter work for D-001.
- Do not edit runtime source, config, CI workflows, compose files, or adapter-overlay files in this bug.
- Documentation must describe implemented or explicitly planned work honestly; no capability claims without a source artifact.
