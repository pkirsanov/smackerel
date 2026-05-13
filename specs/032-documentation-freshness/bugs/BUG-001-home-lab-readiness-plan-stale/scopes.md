# Scopes: BUG-001 Home-Lab Readiness Docs Belong Outside Product Repo

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Remove or migrate Home_Lab_Deployment_Plan target details

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

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

### Implementation Plan

1. Read the current `docs/Home_Lab_Deployment_Plan.md` and identify target-specific sections tied to V-006, V-010, V-020, V-004, DOC-001, and D-001.
2. Remove, retire, or migrate the home-lab-specific checklist content out of Smackerel product docs.
3. Keep only generic product-owned deployment readiness contracts in Smackerel.
4. Add the generic auth and first-start secret prerequisite entries without target-specific values or paths.
5. Add the connector live-stack evidence caveat.
6. Remove or replace obsolete OPS rows that were never created.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-DOC-001 | docs-static | Smackerel docs | SCN-032-D01 | Home-lab checklist content is absent from product docs or reduced to a migration pointer to knb spec 003-smackerel-home-lab-adapter-readiness. |
| T-DOC-002 | docs-static | Smackerel docs | SCN-032-D02 | All auth key names plus non-default database credential requirement are present in generic deployment prerequisites. |
| T-DOC-003 | docs-static | Smackerel docs | SCN-032-D03 | Obsolete OPS rows identified by the review are absent or replaced by real artifacts. |
| T-DOC-004 | docs-static | Smackerel docs | SCN-032-D04 | Connector live-stack evidence caveat exists. |
| T-DOC-005 | artifact | bug packet | all | Artifact lint passes for this bug folder. |

### Definition of Done

- [ ] T-DOC-001 passes and home-lab checklist details are removed from Smackerel or migrated to knb.
- [ ] T-DOC-002 passes and generic deployment prerequisites include auth and secret provisioning requirements.
- [ ] T-DOC-003 passes and obsolete OPS rows are removed or replaced by real planning links.
- [ ] T-DOC-004 passes and connector live-stack evidence is caveated honestly.
- [ ] T-DOC-005 passes and this bug packet remains lint-clean.

## Scope 2: Align Deployment.md with the corrected readiness model

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-032-D05 Deployment docs preserve generic product-vs-target split
  Given an operator reads docs/Deployment.md
  When they compare it with the migrated target-specific planning
  Then Deployment.md describes Smackerel product pipeline ownership separately from target adapter ownership
  And it does not contain a home-lab-specific checklist
```

### Implementation Plan

1. Read `docs/Deployment.md` after Scope 1 establishes the corrected terms.
2. Remove home-lab-specific checklist or host instructions from product-level deployment docs.
3. Add critical generic readiness prerequisites that Deployment.md must surface before apply: auth provisioning, non-default database credentials, target adapter ownership, and connector evidence caveat.
4. Verify terminology is consistent with the migration/removal of `docs/Home_Lab_Deployment_Plan.md` target-specific contents.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-DOC-006 | docs-static | `docs/Deployment.md` | SCN-032-D05 | Product CI pipeline and target adapter ownership are separate. |
| T-DOC-007 | docs-static | `docs/Deployment.md` | SCN-032-D05 | Deployment.md does not ask Smackerel to create product-side home-lab adapter scripts or checklist entries. |
| T-DOC-008 | docs-static | `docs/Deployment.md` | SCN-032-D05 | Critical generic deployment prerequisites align with the corrected Smackerel product contracts. |

### Definition of Done

- [ ] T-DOC-006 passes and Deployment.md separates product pipeline ownership from target adapter ownership.
- [ ] T-DOC-007 passes and Deployment.md preserves the D-001 correction.
- [ ] T-DOC-008 passes and Deployment.md aligns with generic deployment product contracts.
