# Bug Design — BUG-002: Stale `deploy/self-hosted/` references and policy-violating Master Plan

## Design Goal

Restore consistency between the documented operator workflow and the actual repo state after the deploy adapter was extracted to the knb overlay (commit `1b10dc23`), and remove the operator-coupled multi-product `Self_Hosted_Master_Deployment_Plan.md` that violates the `.github/copilot-instructions.md` "No Env-Specific Content In This Repo" non-negotiable policy. Generic-only fixes per the user directive — self-hosted specifics are owned by the knb overlay.

## Design Decisions

### D-001: Generic-only edits in this repo

Honors the user directive received with the work item: this repo holds the **generic** product contract; self-hosted specifics live in the knb overlay. Therefore:

- Where a `deploy/self-hosted/...` path appears in a sentence whose meaning is "the adapter does X", reword to **adapter-contract form** (the path is owned by the adapter, not by this repo).
- Where the path appears in a sentence whose meaning is "operator runs `cp -R <source> <dest>`", replace the source with `deploy/_example/target-skeleton` (the actually-existing in-tree scaffold) and append the alternative out-of-tree pattern: `cp -R deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/smackerel/<new-target>"`.
- `Self_Hosted_Master_Deployment_Plan.md` collapses to a generic migration-pointer stub naming the knb overlay's self-hosted adapter spec as the owner of the operator-coupled cross-product coordination plan.
- The new "knb Deploy-Adapter Overlay Dependency" subsection in `docs/Deployment.md` MUST refer generically to "the knb deploy-adapter overlay's self-hosted adapter spec" without embedding any per-target topology.

### D-002: Stub the Master Plan, do not delete

Deletion would silently break any external link or operator bookmark targeting `docs/Self_Hosted_Master_Deployment_Plan.md`. The migration-pointer stub preserves the path while explicitly explaining the relocation, exactly as BUG-001 did with `docs/Self_Hosted_Deployment_Plan.md`. The stub is approximately 60 lines (close to BUG-001's stub size).

### D-003: Reframe Operations.md by inserting a new "Production Deploy" subsection ahead of "First-Time Setup"

`docs/Operations.md` First-Time Setup is correct for **local dev**. Replacing it would lose the dev walkthrough. Instead, insert a new top-level subsection **before** First-Time Setup that:

- Names the production-class flow (`./smackerel.sh deploy-target ... apply` from a build-manifest, with cosign + bundle verification).
- Lists the production secret prerequisites (the same five keys from `docs/Deployment.md` §Generic Pre-Apply Prerequisites).
- Routes the operator to `docs/Deployment.md` for the full production guide.
- Explicitly labels First-Time Setup as the **local-dev** path so operators don't follow it for production.

### D-004: Spec 050 status fix is a one-line text refresh

`specs/050-ml-sidecar-health-isolation/spec.md` lines 3–5 read `## Status\n\nIn Progress - planning packet created`. `state.json::status` is `done`, certification is `done`, runtime fix shipped (`cmd/core/services.go:177`). Fix is purely text: change to `Resolved — implemented` with a backdated date that matches the state.json `lastCertifiedAt` for that spec.

### D-005: Test surface = static-doc grep red→green probes

Same pattern BUG-001 used. Each fix has a corresponding grep that:

- **Red (pre-fix)**: matches the stale reference and shows the defect.
- **Green (post-fix)**: returns zero matches OR matches only the fixed text, proving the change landed and would re-detect a regression.

No runtime tests are needed because no `.go` / `.py` / `.yml` / `.yaml` files change behavior — only operator-facing markdown surface in `docs/` is mutated.

### D-006: Change boundary discipline

Each scope declares a Change Boundary table listing **allowed** and **excluded** file families. Per the bubbles framework state-transition-guard contract, refactor/repair scopes that omit this section are blocked. Both scopes touch only documentation surface and same-line comments; runtime code (`internal/`, `cmd/`, `ml/`, `tests/`) is excluded.

### D-007: Consumer impact sweep per scope

Per the bubbles framework state-transition-guard contract for rename/removal scopes, each scope declares a Consumer Impact Sweep enumerating downstream consumers and the action taken. Scope 1 affects `docs/Self_Hosted_Master_Deployment_Plan.md` consumers (the existing `docs/Self_Hosted_Deployment_Plan.md` migration stub references it; `docs/Operations.md` and `README.md` may have generic links); Scope 2 affects `docs/Operations.md` readers and the new knb-overlay breadcrumb consumers.

## Scenarios

| ID | Title | Pre | Trigger | Expected | Owner |
|----|-------|-----|---------|----------|-------|
| SCN-032-D06 | Stale `deploy/self-hosted/` paths removed from `docs/Deployment.md` | Doc has 3 stale references at lines 88, 169, 192 | `grep -nE 'deploy/self-hosted' docs/Deployment.md` after fix | Returns zero matches OR only generic adapter-contract wording with no `deploy/self-hosted/...` literal | Scope 1 |
| SCN-032-D08 | `Self_Hosted_Master_Deployment_Plan.md` reduced to migration-pointer stub | File is 427 lines with operator-coupled detail | After fix: file is < 80 lines AND contains "knb deploy-adapter overlay" reference AND zero matches for `selfhosted\b`, `wlp195s0`, `***REMOVED***`, `<host-tailnet-ip>`, `<tailnet-domain>` | Stub names knb overlay as owner; zero env-specific content remains | Scope 1 |
| SCN-032-D09 | `docs/Operations.md` deployment surface leads with adapter flow | First-Time Setup is the first deployment subsection | After fix: a new "Production Deploy (Build-Once Deploy-Many)" subsection appears BEFORE First-Time Setup; First-Time Setup is explicitly labeled "Local Dev" | Operator following the doc top-down encounters the production-class flow first | Scope 2 |
| SCN-032-D10 | `docs/Deployment.md` adds knb-overlay breadcrumb | Doc has no pointer to knb overlay's adapter-readiness spec | After fix: a "knb Deploy-Adapter Overlay Dependency" subsection appears naming spec `003-smackerel-self-hosted-adapter-readiness` and the operator verification step | Operator can verify knb overlay is shipped before attempting deploy | Scope 2 |
| SCN-032-D11 | Spec 050 status text refreshed | `specs/050-ml-sidecar-health-isolation/spec.md::Status` reads `In Progress` | After fix: status reads `Resolved — implemented <date>` matching `state.json` | Spec text matches state.json; future readers don't think the spec is mid-flight | Scope 2 |

## Test Design

| Test ID | Type | Target | Assertion |
|---------|------|--------|-----------|
| T-DOC-009 | docs-static grep | `docs/Deployment.md` | Zero matches for `\bdeploy/self-hosted/(apply\|manifest)` |
| T-DOC-010 | docs-static grep | `docs/Deployment.md` | Zero matches for `cp -R deploy/self-hosted` (replaced with `deploy/_example/target-skeleton`) |
| T-DOC-013 | docs-static grep | `docs/Self_Hosted_Master_Deployment_Plan.md` | Zero matches for the policy-leak patterns: `\bselfhosted\b`, `wlp195s0`, `\*\*\*REMOVED\*\*\*`, `<host-tailnet-ip>`, `<tailnet-domain>`, `Wi-Fi 7 \(MediaTek MT7925\)` |
| T-DOC-014 | docs-static grep | `docs/Self_Hosted_Master_Deployment_Plan.md` | Contains "knb deploy-adapter overlay" AND total line count `< 80` |
| T-DOC-015 | docs-static grep | `docs/Operations.md` | A new "Production Deploy" subsection appears at line `< first-time-setup-line` (insertion order check) |
| T-DOC-016 | docs-static grep | `docs/Deployment.md` | Contains "knb Deploy-Adapter Overlay Dependency" subsection AND names `003-smackerel-self-hosted-adapter-readiness` |
| T-DOC-017 | docs-static grep | `specs/050-ml-sidecar-health-isolation/spec.md` | Status line matches `Resolved — implemented` form |
| T-DOC-R04 | regression docs-static | `docs/Self_Hosted_Deployment_Plan.md` | Migration-pointer stub still present (BUG-001 invariant preserved) |
| T-DOC-R05 | regression docs-static | `docs/Deployment.md` | Generic Pre-Apply Prerequisites section + Connector Live-Stack Evidence Caveat both still present (BUG-001 invariants preserved) |
| T-DOC-R06 | regression docs-static | All edited files | Zero policy-leak patterns introduced anywhere: real Linux user, real NIC name, real subdomain pattern, password marker |

## Risk Controls

| Risk | Mitigation |
|------|------------|
| Inadvertent removal of generic content from `Self_Hosted_Master_Deployment_Plan.md` | Stub size target ≈ 60 lines (matches BUG-001's `Self_Hosted_Deployment_Plan.md` stub); verify via line-count assertion T-DOC-014. |
| Operations.md edit accidentally drops the existing First-Time Setup walkthrough | Insertion-only edit per D-003; First-Time Setup section persists, only the heading changes to label it "Local Dev". |
| Knb overlay spec name (`003-smackerel-self-hosted-adapter-readiness`) drifts from the actual overlay structure | Use the same spec name BUG-001 cited (already verified by BUG-001's docs/Self_Hosted_Deployment_Plan.md stub); cite it in both BUG-001 and BUG-002 stubs for consistency. |
| Re-introduction of `***REMOVED***`-style password markers anywhere | T-DOC-R06 is a per-edited-file grep that asserts zero leaks. |
| BUG-001 invariants regress (the 60-line `Self_Hosted_Deployment_Plan.md` stub or the two new Deployment.md sections) | T-DOC-R04 and T-DOC-R05 run on every regression sweep. |
| Comment-only edits in `.sh` / `.yaml` files trigger Gate G053 (impl-delta evidence) requiring non-docs runtime paths | docs-only workflowMode (`statusCeiling: docs_updated`) does not require G053; documented in design. |

## D-001 Carry-Forward From BUG-001

BUG-001 explicitly named `docs/Self_Hosted_Master_Deployment_Plan.md` as the carry-forward item out of its scope. This bug closes that carry-forward.

## Out-Of-Scope (knb overlay's job)

Per the user directive, the following items are NOT touched by this bug and remain owned by the knb deploy-adapter overlay:

- Real self-hosted hardware/topology/network details (lived in `Self_Hosted_Master_Deployment_Plan.md` lines 1–110 and elsewhere; moved out via D-002).
- The `deploy/self-hosted/` adapter scripts (`apply.sh`, `bootstrap.sh`, `verify.sh`, `rollback.sh`, `params.yaml`, `manifest.yaml`).
- Real Caddy site files, real ufw rules, real systemd unit definitions, real Tailscale tailnet identifiers, real backup destinations.
- Real per-target secret values for `AUTH_*` and `POSTGRES_PASSWORD`.

This bug ONLY makes this repo correctly point at those overlay-owned items as **the operator's next stop**.
