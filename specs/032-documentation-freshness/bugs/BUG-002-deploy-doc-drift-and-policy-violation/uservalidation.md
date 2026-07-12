# BUG-002 — Stale `deploy/self-hosted/` references and policy-violating Master Plan: User Validation

## Scope

Validate that the deployment-doc drift discovered by the 2026-05-13 system review has been resolved generically in this repo, with self-hosted specifics owned by the knb deploy-adapter overlay.

## Validation Statement

| Aspect | Check | Result |
|--------|-------|--------|
| User intent honored | "Generic deployment in `s` repo, self-hosted specifics in `knb` repo" | ✅ Accepted — D-001 design constraint enforces this; all stale paths reworded to adapter-contract form; Master Plan stubbed; knb overlay named as the owner. |
| Operator workflow restored | Following `docs/Deployment.md` "Adding a new deploy target" no longer fails at step 1 | ✅ Accepted — line 192 references `deploy/_example/target-skeleton` which exists in-tree. |
| Policy compliance restored | Zero env-specific content in `Self_Hosted_Master_Deployment_Plan.md` (or any edited file) | ✅ Accepted — file reduced from 427 lines to ≈60-line generic stub; T-DOC-013 grep returns zero leak patterns. |
| Onboarding leads with the right default | `docs/Operations.md` no longer leads with the dev-only path for production-class operators | ✅ Accepted — new "Production Deploy" subsection inserted before First-Time Setup; First-Time Setup labeled "Local Dev". |
| Knb overlay dependency made explicit | Operators can verify the knb overlay's adapter-readiness spec before deploying | ✅ Accepted — `docs/Deployment.md` "knb Deploy-Adapter Overlay Dependency" subsection names the spec. |
| Spec freshness | Spec 050 status text matches state.json | ✅ Accepted — Status reads "Resolved — implemented". |
| BUG-001 invariants preserved | The 60-line `Self_Hosted_Deployment_Plan.md` stub and the two new Deployment.md sections from BUG-001 still present | ✅ Accepted — T-DOC-R04 / T-DOC-R05 confirm. |

## Implementation Acknowledgement (2026-05-13)

| Acknowledgement | Detail |
|-----------------|--------|
| Both scopes Done | Scope 1 sweeps stale paths and stubs the Master Plan; Scope 2 reframes Operations.md, adds the knb breadcrumb, and refreshes spec 050 status. |
| Generic-only constraint honored | Per D-001, no real hostnames, IPs, Linux usernames, NIC names, BIOS specs, subdomain patterns, or password markers introduced. |
| Adapter-overlay boundary respected | The knb overlay's `003-smackerel-self-hosted-adapter-readiness` spec is named as the owner of all self-hosted-specific topology and adapter scripts. |

## Approval

Validated 2026-05-13.

## Checklist

- [x] User intent honored: generic deployment in `s` repo, self-hosted specifics in `knb` repo
- [x] Operator workflow restored: `cp -R deploy/_example/target-skeleton ...` step succeeds
- [x] Policy compliance restored: zero env-specific content in any edited file
- [x] Onboarding leads with the right default: Operations.md Production Deploy subsection precedes First-Time Setup
- [x] Knb overlay dependency made explicit: Deployment.md names spec `003-smackerel-self-hosted-adapter-readiness`
- [x] Spec freshness: spec 050 Status text matches state.json
- [x] BUG-001 invariants preserved (T-DOC-R04, T-DOC-R05 green)
