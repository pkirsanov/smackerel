# User Validation: BUG-003 — No Image Push to Container Registry

## Validation Status: Pending (closure-only)

This bug is being recommended for closure as **Resolved Elsewhere** — the requested capability already shipped under parent spec 029 Scope 7. Validation here is therefore reduced to confirming the fix really exists in the codebase, not a fresh implementation.

## Checklist

- [x] **Spec permits GHCR push** — `specs/029-devops-pipeline/spec.md:33` lists GHCR push on tagged releases as supported.
  - **Verify:** `grep -n "GHCR" specs/029-devops-pipeline/spec.md`
  - **Evidence:** report.md → "Bootstrap & Analysis"
- [x] **CI workflow pushes images to GHCR on tag** — `.github/workflows/ci.yml` has login + push steps gated on `startsWith(github.ref, 'refs/tags/v')`.
  - **Verify:** `sed -n '45,90p' .github/workflows/ci.yml`
  - **Evidence:** report.md → "Bootstrap & Analysis"
- [x] **Parent feature scope is Done** — Scope 7 "GHCR Image Push on Tagged Releases" is `Done` in `specs/029-devops-pipeline/state.json`.
  - **Verify:** `grep -n "GHCR Image Push" specs/029-devops-pipeline/state.json`
  - **Evidence:** report.md → "Bootstrap & Analysis"
- [x] **(Closure step, owned by next agent)** Bug `state.json` promoted to `status: done` with `certification.status: done` and `transitionRequests` cleared.
  - **Verify:** `jq '{status, certification: .certification.status, tr: (.transitionRequests | length)}' specs/029-devops-pipeline/bugs/BUG-003-no-ghcr-image-push/state.json`
  - **Evidence:** report.md → "Closure Evidence (2026-04-26)" — state.json updated with `status: done`, `certification.status: done`, transitionRequests[0].status=closed.
- [x] **(Closure step, owned by next agent)** Artifact lint passes for `specs/029-devops-pipeline`.
  - **Verify:** `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-003-no-ghcr-image-push`
  - **Evidence:** report.md → "Closure Evidence (2026-04-26)" — `Artifact lint PASSED. EXIT=0`.

## Notes

- If the closure agent finds the `docker-compose.yml` image-override env vars or the `docs/Operations.md` pull-based instructions are missing, file separate follow-up bugs; do **not** re-open BUG-003.
