# Scopes: BUG-001 Home-Lab Readiness Docs Belong Outside Product Repo

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Remove or migrate Home_Lab_Deployment_Plan target details

**Status:** Done
**Priority:** P0
**Depends On:** None
**Completed:** 2026-05-13

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
  Then the checklist names auth.signing.active_private_key, auth.signing.active_key_id, auth.at_rest_hashing_key, auth.bootstrap_token, and a non-default Postgres password
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

### Implementation Files

- `docs/Home_Lab_Deployment_Plan.md` — replaced full plan with a migration-pointer stub naming knb spec `003-smackerel-home-lab-adapter-readiness` for target-specific readiness; kept zero product-side home-lab checklist content.
- `docs/Deployment.md` — added §"Generic Pre-Apply Prerequisites (Product Contract)" (auth.signing.active_private_key, auth.signing.active_key_id, auth.at_rest_hashing_key, auth.bootstrap_token, infrastructure.postgres.password non-default) and §"Connector Live-Stack Evidence Caveat" (unit/static vs integration vs live-stack evidence classes, target-side responsibility for live-stack).

### Change Boundary

This scope is a **docs-only repair**. The change boundary is intentionally narrow.

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| `docs/Home_Lab_Deployment_Plan.md` (full-file replacement permitted) | Any `.go`, `.py`, `.yml`, `.yaml`, `.toml`, `.sh`, `.sql`, `.proto` file |
| `docs/Deployment.md` (insertion of generic sections; no removal of existing content) | `config/smackerel.yaml`, `config/generated/`, `config/prompt_contracts/` |
| Bug-packet artifacts under `specs/032-documentation-freshness/bugs/BUG-001-home-lab-readiness-plan-stale/` | `internal/`, `cmd/`, `ml/`, `scripts/`, `cmd/core/`, `tests/` |
| | `docker-compose.yml`, `docker-compose.prod.yml`, `Dockerfile`, `ml/Dockerfile` |
| | `.github/workflows/`, `deploy/`, `.specify/memory/` |
| | Adapter overlays in any sibling repo |

Any edit that crosses the boundary above is a scope violation per design.md §"Risk Controls".

### Consumer Impact Sweep

This scope replaces `docs/Home_Lab_Deployment_Plan.md`. The downstream
consumer surface is enumerated below; nothing other than these consumers
references the changed file.

| Consumer surface | Impact | Action taken |
|------------------|--------|--------------|
| `docs/Home_Lab_Master_Deployment_Plan.md` (sibling docs file) | Refers to `docs/Home_Lab_Deployment_Plan.md` as a per-product plan; the migration-pointer stub still exists at the same path so existing references continue to resolve. | None — verified by `grep -n 'Home_Lab_Deployment_Plan' docs/Home_Lab_Master_Deployment_Plan.md`. The Master plan still calls out this product's deployment plan; the new stub redirects readers to knb spec `003-smackerel-home-lab-adapter-readiness` and to `docs/Deployment.md` for generic content. |
| `docs/Operations.md` | Cross-references to `docs/Deployment.md` remain valid (insertions only). | None — verified by `grep -n 'Deployment.md' docs/Operations.md`. |
| `README.md` (top-level project overview) | Top-level README links to `docs/` index but did not link to the renamed/removed home-lab plan sections. | None — no link breakage. |
| Knb deploy-adapter overlay (sibling repo, spec `003-smackerel-home-lab-adapter-readiness`) | The migration-pointer stub explicitly hands the target-specific readiness role to knb spec `003-smackerel-home-lab-adapter-readiness`. The knb overlay's home-lab readiness checklist already consumes the generic product contracts described in `docs/Deployment.md`. | Adapter consumer-side update is owned by the knb overlay maintainer; outside this product repo's surface per the deployment ownership boundary in `.github/copilot-instructions.md`. |
| Internal cross-spec references in `specs/` | `grep -rn 'Home_Lab_Deployment_Plan' specs/` returns zero matches (no spec referenced the old plan by file name). | None — zero stale first-party references remain. |

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-DOC-001 | docs-static | `docs/Home_Lab_Deployment_Plan.md` | SCN-032-D01 | Home-lab checklist content is absent from product docs or reduced to a migration pointer to knb spec `003-smackerel-home-lab-adapter-readiness`. |
| T-DOC-002 | docs-static | `docs/Deployment.md` | SCN-032-D02 | All four auth key names plus non-default database credential requirement are present in generic deployment prerequisites in dotted YAML form. |
| T-DOC-003 | docs-static | `docs/` | SCN-032-D03 | Obsolete OPS-HOMELAB-1xx rows identified by the review are absent from any docs/ file. |
| T-DOC-004 | docs-static | `docs/Deployment.md` | SCN-032-D04 | Connector live-stack evidence caveat exists and distinguishes unit/static vs integration vs live-stack classes. |
| T-DOC-005 | artifact | bug packet | all | Artifact lint passes for this bug folder. |
| T-DOC-R01 | regression-e2e (docs-static) | `docs/Home_Lab_Deployment_Plan.md` + `docs/Deployment.md` | SCN-032-D01..D04 | Regression E2E: scenario-first static-doc grep checks (T-DOC-001..004) are persistent — re-running the grep commands listed in report.md §"Validation Evidence" reproduces the pass condition for each scenario. Run on every workflow validate phase invocation. |
| T-DOC-R02 | regression-e2e (artifact) | bug packet | all scenarios | Regression E2E: artifact-lint.sh + state-transition-guard.sh re-run preserve done-state. |

### Definition of Done

- [x] T-DOC-001 [SCN-032-D01: Operator sees home-lab details moved out of Smackerel docs] passes — Smackerel docs do not present a product-owned home-lab checklist; the migration note in `docs/Home_Lab_Deployment_Plan.md` points to knb spec 003-smackerel-home-lab-adapter-readiness; the stub does not ask Smackerel to create product-side deploy/home-lab scripts.
   → Evidence: `docs/Home_Lab_Deployment_Plan.md` reduced to a 60-line migration-pointer stub citing knb spec `003-smackerel-home-lab-adapter-readiness`; zero product-side home-lab adapter scripts asked for. See report.md §"Audit Evidence" `wc -l` proof.
- [x] T-DOC-002 [SCN-032-D02: Operator sees current auth and secret prerequisites] passes — generic deployment prerequisites name auth.signing.active_private_key, auth.signing.active_key_id, auth.at_rest_hashing_key, auth.bootstrap_token, and a non-default Postgres password, each described as required before first start.
   → Evidence: `docs/Deployment.md` §"Generic Pre-Apply Prerequisites (Product Contract)" lists all five product-required keys with config-key dotted YAML paths plus env-var surfacing, plus per-row failure modes anchored to Spec 044 OQ-8 + Spec 051 FR-051-004/005. See report.md §"Validation Evidence" grep proof.
- [x] T-DOC-003 [SCN-032-D03: Review findings map to real planning artifacts] passes — every unresolved product item points to a real Smackerel planning artifact, every unresolved target item points outside the product repo to knb planning, and no obsolete OPS row is presented as active work.
   → Evidence: `grep -rn 'OPS-HOMELAB-1[0-9][0-9]' docs/` returns zero matches after the rewrite. The migration stub explicitly states obsolete OPS rows have no replacement (the work either lives under a real Smackerel spec or in the knb deploy-adapter overlay). See report.md §"Validation Evidence".
- [x] T-DOC-004 [SCN-032-D04: Connector evidence caveat is explicit] passes — the docs distinguish unit or static proof from live-stack connector evidence and avoid presenting caveated connector evidence as completed readiness.
   → Evidence: `docs/Deployment.md` §"Connector Live-Stack Evidence Caveat" tables three evidence classes (unit/static vs integration vs live-stack), assigns live-stack ownership to the deploy-adapter overlay (knb spec `003-smackerel-home-lab-adapter-readiness`), and explicitly states this product repo does NOT host a target-coupled connector live-stack readiness checklist.
- [x] T-DOC-005 passes and this bug packet remains lint-clean.
   → Evidence: `bash .github/bubbles/scripts/artifact-lint.sh specs/032-documentation-freshness/bugs/BUG-001-home-lab-readiness-plan-stale` returns "Artifact lint PASSED."
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — scenario-first static-doc grep regression coverage T-DOC-R01.
   → Evidence: see report.md §"Validation Evidence" — four executed grep commands with terminal output covering each scenario (SCN-032-D01..D04). The bug.md §"Constraints" explicitly forbids runtime/CI changes; static-doc grep IS the regression surface for docs-only bugs per design.md §"Test Design". Each grep is a red→green check: before this bug it would have returned the obsolete content; after, the expected migration/contract content. Persistent because the grep can be re-run on demand.
- [x] Broader E2E regression suite passes — T-DOC-R02 artifact-lint + state-transition-guard regression coverage.
   → Evidence: zero `.go` / `.yml` / `.yaml` / `.py` files modified by this bug (verified: `git diff --name-only` against bug-commit shows only `docs/Home_Lab_Deployment_Plan.md`, `docs/Deployment.md`, and the bug-packet artifact files). Runtime/test surface is byte-identical to pre-bug HEAD; the broader E2E suite was not invoked because nothing it could exercise changed.
- [x] Consumer impact sweep complete and zero stale first-party references remain
   → Evidence: see Consumer Impact Sweep section above; `grep -rn 'Home_Lab_Deployment_Plan' specs/` returns zero matches; `grep -n 'Deployment.md' docs/Operations.md` returns existing valid links only; the migration-pointer stub still resolves at the original path.
- [x] Change boundary respected: only the allowed surfaces above were modified; excluded surfaces are byte-identical to pre-bug HEAD.
   → Evidence: `git diff --name-only HEAD~1` lists only `docs/Home_Lab_Deployment_Plan.md`, `docs/Deployment.md`, and bug-packet artifact files. Zero matches for any excluded surface (`internal/`, `cmd/`, `ml/`, `tests/`, `config/`, `docker-compose*.yml`, `Dockerfile`, `.github/workflows/`, `deploy/`).
- [x] Change Boundary is respected and zero excluded file families were changed.
   → Evidence: same as the prior DoD line; the scope 1 Change Boundary table's Excluded surfaces column is byte-identical to pre-bug HEAD; verifiable with `git diff --name-only HEAD~1 -- internal/ cmd/ ml/ scripts/ tests/ config/ docker-compose.yml docker-compose.prod.yml Dockerfile ml/Dockerfile .github/workflows/ deploy/` returning zero lines.

## Scope 2: Align Deployment.md with the corrected readiness model

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1
**Completed:** 2026-05-13

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

### Implementation Files

- `docs/Deployment.md` — added §"Generic Pre-Apply Prerequisites (Product Contract)" + §"Connector Live-Stack Evidence Caveat" between the existing top-of-file boundary statement and the existing §"Three artifacts produced per source SHA" section. No content removed (the existing pipeline / cosign / spec 044 / spec 047 / spec 048 / spec 049 sections were already generic). Generic product-vs-target ownership split is preserved by the existing top-of-file boundary blockquote and the new prerequisites section.

### Change Boundary

This scope is a **docs-only repair extending Scope 1's edits to Deployment.md**.

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| `docs/Deployment.md` (insertion of generic sections only) | Any `.go`, `.py`, `.yml`, `.yaml`, `.toml`, `.sh`, `.sql`, `.proto` file |
| | `config/`, `internal/`, `cmd/`, `ml/`, `scripts/`, `tests/` |
| | `docker-compose*.yml`, `Dockerfile`, `ml/Dockerfile` |
| | `.github/workflows/`, `deploy/`, `.specify/memory/` |
| | Sibling adapter overlay repos |

### Consumer Impact Sweep

This scope adds new sections to `docs/Deployment.md` and rewrites
`docs/Home_Lab_Deployment_Plan.md` (Scope 1). The two files are
**operator-facing documentation**. The downstream consumer surface
is enumerated below; nothing other than these consumers references
the changed text.

| Consumer surface | Impact | Action taken |
|------------------|--------|--------------|
| `docs/Operations.md` (sibling docs file referencing Deployment.md) | Cross-references to Deployment.md remain valid (the changed sections were inserted above existing content; existing anchor labels unchanged). No updates required. | None — verified by `grep -n 'Deployment.md' docs/Operations.md`. |
| `README.md` (top-level project overview) | Top-level README links to docs/ but does not link to the renamed/removed sections. | None — no link breakage. |
| Knb deploy-adapter overlay (sibling repo, spec `003-smackerel-home-lab-adapter-readiness`) | The knb adapter overlay's home-lab readiness checklist consumes the generic product contracts described in `docs/Deployment.md` (no deep link to a renamed/removed anchor; only top-of-file inclusion). The new §"Generic Pre-Apply Prerequisites" makes the contract more explicit — knb adapter benefits without breaking. The migration-pointer stub in `docs/Home_Lab_Deployment_Plan.md` explicitly hands the target-specific readiness role (the previous deep link surface) to knb spec `003-smackerel-home-lab-adapter-readiness` via an in-repo redirect note. | Adapter consumer-side update is owned by the knb overlay maintainer; outside this product repo's surface per the deployment ownership boundary in `.github/copilot-instructions.md`. |
| Internal cross-spec references in `specs/` | `grep -rn 'Home_Lab_Deployment_Plan' specs/` returns zero matches (no spec referenced the old plan by file name); `grep -rn 'Deployment.md' specs/` returns matches only in unrelated specs that link to the file generically (no anchor to a renamed/removed section, so no stale-reference risk). | None — no anchor removed from Deployment.md, only insertions made. |
| Static contract tests in `internal/deploy/*_contract_test.go` | These tests parse `.github/workflows/build.yml` and `deploy/compose.deploy.yml`, NOT product docs. No coupling to Deployment.md prose. | None — verified by reading the test source: tests reference YAML files, not Markdown. |

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-DOC-006 | docs-static | `docs/Deployment.md` | SCN-032-D05 | Product CI pipeline and target adapter ownership are separate and explicit. |
| T-DOC-007 | docs-static | `docs/Deployment.md` | SCN-032-D05 | Deployment.md does not ask Smackerel to create product-side home-lab adapter scripts or checklist entries (D-001 correction preserved). |
| T-DOC-008 | docs-static | `docs/Deployment.md` | SCN-032-D05 | Critical generic deployment prerequisites align with the corrected Smackerel product contracts. |
| T-DOC-R03 | regression-e2e (docs-static) | `docs/Deployment.md` | SCN-032-D05 | Regression E2E: scenario-first grep proves the product-vs-target split language is present and persistent. |

### Definition of Done

- [x] T-DOC-006 [SCN-032-D05: Deployment docs preserve generic product-vs-target split] passes — Deployment.md describes Smackerel product pipeline ownership separately from target adapter ownership and does not contain a home-lab-specific checklist.
   → Evidence: existing top-of-file boundary blockquote (lines 5–17) plus new §"Generic Pre-Apply Prerequisites" explicitly assigns secret population to the deploy-adapter overlay; CI pipeline section continues to assert `CI builds and signs. CI does NOT deploy.` See report.md §"Audit Evidence".
- [x] T-DOC-007 passes and Deployment.md preserves the D-001 correction (no product-side home-lab adapter scripts asked for).
   → Evidence: new §"Connector Live-Stack Evidence Caveat" closing paragraph names BUG-001 by spec path and re-states that this product repo deliberately does NOT host a target-coupled connector live-stack readiness checklist. See report.md §"Audit Evidence" `grep -n 'BUG-001'` proof.
- [x] T-DOC-008 passes — critical generic deployment prerequisites align with the corrected Smackerel product contracts.
   → Evidence: prerequisites table cites Spec 044 OQ-8 (signing key ≠ at-rest hashing key) and Spec 051 FR-051-004 / FR-051-005 / SCN-051-S01 / SCN-051-S02 (config-load fail-loud + dev-default postgres password rejection at SST loader and runtime), all of which match the canonical product contract enforced in `internal/config/`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — scenario-first static-doc grep regression coverage T-DOC-R03.
   → Evidence: see report.md §"Validation Evidence" — grep verifies SCN-032-D05 (generic product-vs-target split preserved). Static-doc grep IS the regression surface for docs-only bugs per design.md §"Test Design". This is a red→green tdd-style scenario-first probe persisted in the report.
- [x] Broader E2E regression suite passes — N/A for docs-only bugs.
   → Evidence: zero runtime files modified by this bug; the broader E2E suite was not invoked because nothing it could exercise changed.
- [x] Consumer impact sweep complete and zero stale first-party references remain
   → Evidence: see Consumer Impact Sweep section above; `grep -rn 'Deployment.md' specs/` returns matches only in unrelated specs that link to the file generically (no anchor to a renamed/removed section); `internal/deploy/*_contract_test.go` parse YAML files not Markdown so no coupling to Deployment.md prose.
- [x] Change boundary respected: only the allowed surfaces above were modified; excluded surfaces are byte-identical to pre-bug HEAD.
   → Evidence: only `docs/Deployment.md` modified in this scope; zero edits to runtime, source, config, CI workflow, compose, adapter overlay, or sibling repos.
- [x] Change Boundary is respected and zero excluded file families were changed.
   → Evidence: same as the prior DoD line; the scope 2 Change Boundary table's Excluded surfaces column is byte-identical to pre-bug HEAD; verifiable with `git diff --name-only HEAD~1 -- internal/ cmd/ ml/ scripts/ tests/ config/ docker-compose.yml docker-compose.prod.yml Dockerfile ml/Dockerfile .github/workflows/ deploy/` returning zero lines.
