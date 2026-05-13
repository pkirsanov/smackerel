# BUG-001 - Home-Lab Readiness Docs Belong Outside Product Repo

## Status

Resolved — implemented 2026-05-13. Both scopes Done. See [report.md](report.md).

## Problem Statement

The Smackerel documentation set has drifted from the current ownership model and from the remaining review findings. It currently needs correction across three themes:

1. Product-side deployment contracts and home-lab-specific target readiness are blended together.
2. Recent product-side prerequisites, especially generic auth and first-start secret provisioning, are missing or incomplete.
3. Review caveats about connector live-stack evidence and obsolete OPS rows are not represented clearly enough for an operator to trust the plan.

This bug belongs under spec 032 because the work is documentation freshness: the runtime and adapter implementation are intentionally unchanged by this packet.

## Outcome Contract

**Intent:** Refresh Smackerel deployment documentation so it removes or migrates home-lab-specific target details out of the product repo, keeps generic deployment contracts in Smackerel, and reflects the remaining reviewed product-side gaps.

**Success Signal:** A reader can open Smackerel deployment docs and see generic product-owned deployment contracts only; any home-lab-specific checklist, secret injection path, Caddy/tailnet exposure, host monitoring wiring, backup timer, or operator-host detail is migrated to knb spec `003-smackerel-home-lab-adapter-readiness` or target-adapter documentation.

**Hard Constraints:**

- Planning and implementation must not create a Smackerel `deploy/home-lab/` adapter surface or product-owned home-lab checklist.
- Documentation must not claim spec 030, connector live-stack evidence, or auth provisioning are complete unless execution evidence supports that claim.
- Documentation must not reference obsolete OPS rows as if they exist.
- Runtime, CI, compose, config, and source files are not part of this bug's change surface.

**Failure Condition:** The refreshed docs still keep home-lab-specific checklists in Smackerel, imply that Smackerel owns target adapter scripts, omit generic auth provisioning, present unproven connector live-stack evidence as complete, or point operators to non-existent OPS rows.

## Requirements

- **FR-DOC-001:** Smackerel docs MUST remove or migrate home-lab-specific checklist content out of the product repo.
- **FR-DOC-002:** Any remaining migration note MUST identify knb spec `003-smackerel-home-lab-adapter-readiness` as the target-specific planning location for the D-001 correction.
- **FR-DOC-003:** Generic Smackerel docs MUST update spec 030 status truthfully and describe any remaining observability caveat.
- **FR-DOC-004:** Generic Smackerel docs MUST list product-required auth key names and non-default database credential requirements without target-specific values or paths.
- **FR-DOC-005:** Generic Smackerel docs MUST document the connector live-stack evidence caveat without overstating readiness.
- **FR-DOC-006:** The docs MUST remove obsolete OPS rows that were never created or replace them with real planning artifact links.
- **FR-DOC-007:** `docs/Deployment.md` MUST reflect the same product-vs-adapter ownership model.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-032-D01 Operator sees home-lab details moved out of Smackerel docs
  Given the operator reads Smackerel deployment documentation
  When they look for target-specific home-lab readiness details
  Then Smackerel docs do not present a product-owned home-lab checklist
  And any migration note points to knb spec 003-smackerel-home-lab-adapter-readiness
  And it does not ask Smackerel to create product-side deploy/home-lab scripts

Scenario: SCN-032-D02 Operator sees current auth and secret prerequisites
  Given the operator reads generic deployment prerequisites
  When they review production-class authentication requirements
  Then the checklist names auth.signing.hmac_key, auth.signing.issuer, auth.at_rest_hashing_key, auth.bootstrap_token, and a non-default Postgres password
  And each value is described as required before first start

Scenario: SCN-032-D03 Review findings map to real planning artifacts
  Given the readiness plan lists unresolved items
  When the operator follows the links
  Then every unresolved product item points to a real Smackerel planning artifact
  And every unresolved target item points outside the product repo to knb planning
  And no obsolete OPS row is presented as active work

Scenario: SCN-032-D04 Connector evidence caveat is explicit
  Given the docs discuss connector readiness
  When the operator reads the live-stack evidence section
  Then the docs distinguish unit or static proof from live-stack connector evidence
  And the docs avoid presenting caveated connector evidence as completed readiness
```

## Product Principle Alignment

This bug supports Product Principle 8, Trust Through Transparency, by requiring docs to show the actual readiness evidence and ownership boundary. It also preserves Principle 5, One Graph, Many Views, by keeping the readiness plan as an honest view over the planning artifacts instead of a second truth source.
