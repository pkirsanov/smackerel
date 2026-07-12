# Bug: BUG-002 — Stale `deploy/self-hosted/` references and policy-violating Master Plan after adapter extraction

## Classification

- **Type:** Documentation drift after deploy adapter extraction (BUG-001 carry-forward + system-review carry-forward)
- **Severity:** HIGH — operator following the documented workflow for "Adding a new deploy target" hits `cp -R deploy/self-hosted` and fails at step 1 (the source directory was extracted to the knb deploy-adapter overlay in commit `1b10dc23`); plus a 427-line operator-coupled file (`docs/Self_Hosted_Master_Deployment_Plan.md`) violates the `.github/copilot-instructions.md` "No Env-Specific Content In This Repo" non-negotiable policy with real Linux user, real Wi-Fi NIC name, real BIOS specs, real backup paths, real subdomain pattern, and a weak-password marker line.
- **Parent Spec:** 032 — Documentation Freshness
- **Workflow Mode:** docs-only (statusCeiling: docs_updated)
- **Status:** Resolved — implemented 2026-05-13. All scopes Done.
- **Discovered By:** `bubbles.system-review` execution on 2026-05-13 (see system-review summary in this session). Findings VA-001, VA-002, VA-003, VA-004, AU-001, AU-002, DOC-001, DOC-002, DOC-003, TR-002, SI-002, X-3, X-4, X-5.

## Problem Statement

Two compounding documentation/configuration problems remained after BUG-001 shipped (commit `899507be`):

### Problem A — Stale in-tree paths after adapter extraction

`deploy/self-hosted/` was deliberately extracted from this repo to the knb deploy-adapter overlay in commit `1b10dc23` ("deploy-target: STRICT DEPLOY_TARGETS_ROOT, extract self-hosted to private repo"). The strict `DEPLOY_TARGETS_ROOT` resolution rule landed in `scripts/commands/deploy_target.sh`, but four references to the extracted path remained in operator-facing surfaces:

| Location | Stale reference |
|----------|-----------------|
| [docs/Deployment.md](../../../../docs/Deployment.md) line 88 | `deploy/<target>/manifest.yaml \| **Yes** (pointer) \| \`deploy/<target>/apply.sh\`` |
| [docs/Deployment.md](../../../../docs/Deployment.md) line 169 | `Each \`deploy/<target>/apply.sh\` MUST:` |
| [docs/Deployment.md](../../../../docs/Deployment.md) line 192 | `1. \`cp -R deploy/self-hosted deploy/<new-target>\`` (the source directory does NOT exist in this repo) |
| [config/smackerel.yaml](../../../../config/smackerel.yaml) line 1031 | `# Self-hosted target on the operator's self-hosted machine. Builds are produced by .github/workflows/build.yml and applied via deploy/self-hosted/apply.sh.` |
| [scripts/commands/config.sh](../../../../scripts/commands/config.sh) line 1413 | `# Bundle layout (extracted by adapter \`apply.sh\` into <composeDir>/):` (reads as if `apply.sh` lives here) |

Operator effect: Following `docs/Deployment.md` "Adding a new deploy target" instructions fails at step 1. Following the line-88/line-169 references, the operator looks for in-tree adapter scripts that do not exist.

### Problem B — `Self_Hosted_Master_Deployment_Plan.md` policy violation

`docs/Self_Hosted_Master_Deployment_Plan.md` (427 lines) is a **multi-product cross-coordination plan** (covers `qf`, `s`, `gh`, `wa`) that has no business in a single-product repo, AND violates the `.github/copilot-instructions.md` "No Env-Specific Content In This Repo" non-negotiable policy:

| Class of leak | Example from the file |
|---------------|----------------------|
| Real Linux username | `User \| \`selfhosted\`` (line 42); `Sudo \| NOPASSWD via /etc/sudoers.d/selfhosted` (line 43); home-directory backup paths under the user's `$HOME` (lines 135–138, 188, 294) |
| Real Wi-Fi NIC name | `wlp195s0` (lines 60–61) |
| Real BIOS hardware spec | iGPU VRAM, RAM totals, NPU TOPS rating (lines 14–22, 27) |
| Weak-password leak vector | `Password \| \`***REMOVED***\` (weak, date) \| **MUST change before public exposure**` (line 49) — the "(weak, date)" annotation is itself a structural hint that defeats the redaction |
| Real subdomain naming pattern | `*.self-hosted.<tailnet-domain>.ts.net` (lines 228, 233–242) |
| User group memberships | `selfhosted in docker, render, video` (line 156) |
| Real per-host install paths | per-host install path under the operator-user `$HOME` (line 188), e.g. `<HOME>/immich/library` |

This file's existence in a generic, public-facing product repo is the same class of issue BUG-001 fixed for `Self_Hosted_Deployment_Plan.md`. The BUG-001 commit message explicitly named this as a carry-forward.

### Problem C — Documentation drift on shipped specs

`specs/050-ml-sidecar-health-isolation/spec.md` (lines 3–5) reads `## Status\n\nIn Progress - planning packet created` even though `state.json::status = done` AND the runtime fix is shipped (`cmd/core/services.go:177` `WaitForMLReady`, `/readyz` handler in `internal/api/health_test.go:1750`). Spec text wasn't refreshed at close-out.

### Problem D — Operator onboarding leads with the wrong default

`docs/Operations.md` (line 7) `### First-Time Setup` walks the dev workflow (`git clone → ./smackerel.sh up`) and ends with "All 4 services should show as healthy". For a production-class deployment the system has invested in the build-once-deploy-many flow, but a new operator following First-Time Setup never encounters it. The "Pre-built Image Deployment" subsection (line 66) uses image-tag overrides instead of digest-based deploy-adapter flow. Effect: operators are taught the wrong default.

### Problem E — No knb-overlay breadcrumb for operators

The repo correctly delegates the self-hosted adapter to the knb overlay (`deploy/README.md` "Adapter Locality", BUG-001 migration-pointer stub), but offers no breadcrumb saying *what* the operator needs from the knb overlay (which spec, which adapter scripts, what status). Without this, operators can't verify whether the knb overlay's `003-smackerel-self-hosted-adapter-readiness` is shipped before attempting a deploy.

## D-001 (Design Constraint): Generic-only edits in this repo

**The fixes in this bug MUST stay generic.** Per the user directive received with this work item:

> "Generic deployment should be done in `s` repo, and any self-hosted environment specific settings should be configured using custom deployment in `knb` repo."

This bug:

- MUST replace stale `deploy/self-hosted/` references with the **adapter-contract** wording (the path is owned by the adapter, not by this repo).
- MUST reduce `Self_Hosted_Master_Deployment_Plan.md` to a migration-pointer stub (the same pattern BUG-001 applied to `Self_Hosted_Deployment_Plan.md`). The detailed multi-product self-hosted plan content moves to the knb overlay.
- MUST NOT reintroduce real hostnames, real IPs, real Linux usernames, real Tailscale identifiers, real NIC names, real BIOS specs, real per-host paths, or any weak-password markers.
- MUST add a generic knb-overlay breadcrumb pointing operators at the adapter-readiness spec they need from the knb overlay.
- MUST NOT touch any self-hosted adapter implementation (those scripts live in the knb overlay; this repo no longer ships them).

## Behavior Contract

**Pre-fix (defect):**

- Operator following `docs/Deployment.md` "Adding a new deploy target" tries `cp -R deploy/self-hosted deploy/<new-target>` and the command fails because the source directory does not exist in this repo.
- Operator reading `config/smackerel.yaml::environments.self-hosted` comment is told builds are "applied via `deploy/self-hosted/apply.sh`" — a path that does not exist in this repo.
- Operator browsing `docs/Self_Hosted_Master_Deployment_Plan.md` sees real Linux users, real Wi-Fi adapter names, real backup paths, real subdomain patterns, and a `***REMOVED***` weak-password marker — a non-negotiable policy violation per `.github/copilot-instructions.md`.
- Operator opening `specs/050-ml-sidecar-health-isolation/spec.md` sees `In Progress` even though the spec is shipped.
- Operator following `docs/Operations.md` "First-Time Setup" is taught the dev-only `up`/`down` workflow without ever encountering the build-once-deploy-many flow.
- Operator preparing to deploy to self-hosted has no breadcrumb telling them which knb-overlay spec must be ready.

**Post-fix (required behavior):**

- All five stale `deploy/self-hosted/` references in `docs/Deployment.md`, `config/smackerel.yaml`, and `scripts/commands/config.sh` either reword to the adapter-contract form (the path is owned by the adapter), or replace `deploy/self-hosted` with `deploy/_example/target-skeleton` where a copy-source path is meaningful in-tree.
- `docs/Self_Hosted_Master_Deployment_Plan.md` reduces to a generic migration-pointer stub naming the knb deploy-adapter overlay's self-hosted adapter spec as the owner of the operator-coupled cross-product coordination plan. Zero real Linux users, NIC names, BIOS specs, backup paths, subdomain patterns, password markers, or operator-coupled topology remain in the file.
- `specs/050-ml-sidecar-health-isolation/spec.md::Status` reads `Resolved — implemented 2026-05-12` (or equivalent done-form status that matches `state.json`).
- `docs/Operations.md` "Deployment" section restructures to lead with the production-class deploy-adapter flow and demote the dev `git clone → ./smackerel.sh up` walkthrough to a clearly-labeled local-dev subsection. The pre-deploy secret prerequisites surface near the new top.
- `docs/Deployment.md` adds a "knb Deploy-Adapter Overlay Dependency" subsection naming the adapter-readiness spec operators need from the knb overlay before attempting a self-hosted deploy.

## Scope Of Fix

Two scopes — see [scopes.md](scopes.md):

1. **Scope 1**: Sweep stale `deploy/self-hosted/` references out of `docs/Deployment.md`, `config/smackerel.yaml`, and `scripts/commands/config.sh`; replace `Self_Hosted_Master_Deployment_Plan.md` with a migration-pointer stub.
2. **Scope 2**: Reframe `docs/Operations.md` Deployment section to lead with the deploy-adapter flow; add a knb overlay breadcrumb to `docs/Deployment.md`; refresh stale `Status: In Progress` text on `specs/050-ml-sidecar-health-isolation/spec.md`.

## References

- [docs/Deployment.md](../../../../docs/Deployment.md) (713 lines, post-BUG-001)
- [docs/Operations.md](../../../../docs/Operations.md) (1946 lines)
- [docs/Self_Hosted_Master_Deployment_Plan.md](../../../../docs/Self_Hosted_Master_Deployment_Plan.md) (427 lines, target of Scope 1)
- [docs/Self_Hosted_Deployment_Plan.md](../../../../docs/Self_Hosted_Deployment_Plan.md) (60-line migration-pointer stub, BUG-001 reference shape)
- [config/smackerel.yaml](../../../../config/smackerel.yaml) (line 1031 stale comment)
- [scripts/commands/config.sh](../../../../scripts/commands/config.sh) (line 1413 stale comment)
- [deploy/README.md](../../../../deploy/README.md) (Adapter Locality + DEPLOY_TARGETS_ROOT contract)
- [deploy/_example/target-skeleton/](../../../../deploy/_example/target-skeleton/) (existing in-tree skeleton; the legitimate `cp -R` source)
- [.github/copilot-instructions.md](../../../../.github/copilot-instructions.md) "No Env-Specific Content In This Repo" non-negotiable policy
- BUG-001 commit `899507be` (the carry-forward note explicitly named the Master Plan as not part of BUG-001 but in scope for a separate bug)
- Sibling product repo: knb deploy-adapter overlay, spec `003-smackerel-self-hosted-adapter-readiness`
