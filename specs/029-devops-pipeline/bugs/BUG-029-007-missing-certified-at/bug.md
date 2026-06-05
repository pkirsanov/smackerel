# BUG-029-007 — Missing Top-Level certifiedAt After Post-Cert OPS-001 Spec.md Banner Sweep

**Spec:** 029-devops-pipeline
**Severity:** Governance (artifact-only; zero runtime change)
**Status:** open → resolved
**Discovered:** 2026-06-05
**Discovered by:** stochastic-quality-sweep round 7 of 20 (trigger=`regression`, mapped child workflow=`regression-to-doc`, executionModel=`parent-expanded-child-mode`)
**Closure Mode:** bugfix-fastlane (artifact reconciliation only; mirrors BUG-029-006 / BUG-028-003 precedent)

---

## Summary

`state-transition-guard.sh specs/029-devops-pipeline` returns 1 BLOCK against the legacy spec 029 artifacts at HEAD `e05aef1b`: Check 30 / Gate G088 (Post-Certification Spec Edit Detection) fails because (a) `specs/029-devops-pipeline/state.json` does not declare a top-level `certifiedAt` string field and (b) the OPS-001 banner-sweep commit `19b31c0a9a67d38443e47a5823cd7baf42654094` ("bubbles(ops/OPS-001): sweep spec.md status banners across 54 certified specs", 2026-05-28T05:07:50+00:00) modified `specs/029-devops-pipeline/spec.md` AFTER the BUG-029-006 reconcile (2026-05-24T18:12:35+00:00) brought the spec to `state.json status=done`. The OPS-001 edit was a workspace-wide cosmetic banner reconciliation (inserted `**Status:** Done (certified per state.json)` after the H1 of 28 specs including 029) and changed zero planning truth, but G088 mechanically detects any post-cert commit touching `spec.md|design.md|scopes.md|scopes/_index.md|scopes/*/scope.md` and requires either (a) top-level `certifiedAt` AFTER the edit, (b) `requiresRevalidation: true`, or (c) demotion out of `done` / `done_with_concerns`.

The probe of the live spec 029 surface returns **zero functional regression**:

- `internal/deploy/ci_workflow_no_parallel_publish_test.go` — GREEN (CI workflow parallel-publish contract preserved)
- `internal/deploy/build_workflow_vuln_gate_contract_test.go` — GREEN (build workflow Trivy gate + cosign + SLSA + bundle-hash contract preserved)
- `internal/deploy/compose_contract_test.go` — GREEN (deploy compose contract preserved)
- `internal/deploy/dev_compose_default_fallback_test.go` — GREEN (NO-DEFAULTS fail-loud SST preserved)
- `internal/api/health_test.go` — GREEN (Scope 4 build metadata via ldflags + OCI labels preserved)
- `ml/tests/` — GREEN (173 pytest cases — Scope 5 ML sidecar surface preserved)
- `artifact-lint.sh specs/029-devops-pipeline` — PASSED
- `traceability-guard.sh specs/029-devops-pipeline` — PASSED (15/15 scenarios mapped to concrete test files)

This BUG packet closes the 1 BLOCK by:

1. Running a fresh parent-expanded `bubbles.spec-review` against HEAD that certifies all 7 scopes of spec 029 are CURRENT (spec.md/design.md/scopes.md align with `.github/workflows/`, `Dockerfile`, `ml/Dockerfile`, `docker-compose.yml`, `deploy/compose.deploy.yml`, `scripts/commands/config.sh`, `docs/Branch_Protection.md`, `docs/Deployment.md`, `docs/Operations.md`, `internal/api/health.go`, `internal/deploy/*`).
2. Appending the bubbles.spec-review CURRENT entry to `state.json::executionHistory` with `reviewStatus: CURRENT` and `runCompletedAt: 2026-06-05T22:00:00Z`.
3. Adding top-level `certifiedAt: 2026-06-05T22:00:00Z` to `state.json` (AFTER the OPS-001 edit at 2026-05-28T05:07:50Z, satisfying G088).
4. Appending `resolvedBugs[]` entry for BUG-029-007 with sweep provenance.
5. Adding a `### BUG-029-007 Recertification Evidence` subsection to parent spec `report.md` with the pre→post guard re-runs.
6. Committing with the structured `bubbles(029/bug-029-007)` prefix.

Total BLOCK trajectory: **1 → 0** (artifact-only mutation set; no source, no test, no config, no docs/ outside the packet itself and the parent spec report.md subsection).

---

## Root Cause

Spec 029 was certified `done` in April 2026 (original close-out) and re-certified after the BUG-029-006 reconcile sweep on 2026-05-24T18:12:35Z. The state.json structure at that time did NOT include a top-level `certifiedAt` field — that requirement was introduced by Gate G088 (Post-Certification Spec Edit Detection) which now requires it for any spec with `status: done` or legacy `done_with_concerns` whose planning files were touched after certification.

On 2026-05-28T05:07:50Z, the workspace-wide OPS-001 banner sweep (`19b31c0a9a67d38443e47a5823cd7baf42654094` — "bubbles(ops/OPS-001): sweep spec.md status banners across 54 certified specs") inserted the canonical `**Status:** Done (certified per state.json)` banner after the H1 of 28 specs (021-024, 029-037, 039, 042-055), including `specs/029-devops-pipeline/spec.md`. This edit was cosmetic — it brought the banner into alignment with `state.json::status=done` and changed zero planning truth — but G088 mechanically treats ANY edit to `spec.md|design.md|scopes.md|scopes/_index.md|scopes/*/scope.md` after `certifiedAt` as a post-cert edit that requires either fresh recertification or `requiresRevalidation:true`.

Because spec 029's state.json never had a `certifiedAt` field at all, the guard fails at the FIRST gate (Check 30 → G088 missing `certifiedAt` for `status=done`) with exit code 2 and the message `post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec specs/029-devops-pipeline (status=done)`. This is **pure artifact governance drift** — the same pattern handled in BUG-029-006 (sweep round 23), BUG-028-003 (sweep round 22), BUG-027-001 (sweep round 21), and BUG-026-004 (sweep round 20).

---

## Scope

**In-scope (artifact mutations only):**

- `specs/029-devops-pipeline/state.json` — add top-level `certifiedAt`, append bubbles.spec-review CURRENT entry to executionHistory, append resolvedBugs entry for BUG-029-007
- `specs/029-devops-pipeline/report.md` — append `### BUG-029-007 Recertification Evidence` subsection
- `specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at/**` — this packet (8 artifacts)

**Out-of-scope (NOT touched):**

- `specs/029-devops-pipeline/spec.md` — NOT touched (would trigger another G088 cycle)
- `specs/029-devops-pipeline/design.md` — NOT touched
- `specs/029-devops-pipeline/scopes.md` — NOT touched
- `specs/029-devops-pipeline/scenario-manifest.json` — NOT touched
- `specs/029-devops-pipeline/uservalidation.md` — NOT touched
- Any `.go`, `.py`, `.yaml`, `.sql`, `.sh`, `.ts`, `.tsx` source under `cmd/`, `internal/`, `ml/`, `tests/`, `scripts/`, `web/`, `config/`, `.github/workflows/`, `deploy/`, `smackerel.sh`
- Any other spec folder
- Any pre-existing BUG packet under `specs/029-devops-pipeline/bugs/BUG-029-00{1..6}/`
- Workspace pre-existing dirty paths under other specs (003, 009, 016, 037, 067, bookmarks, weather, tests/integration/policy) — intentionally left alone; not in this sweep round's scope

---

## Acceptance

`state-transition-guard.sh specs/029-devops-pipeline` returns **0 BLOCKs** (the 2 pre-existing non-blocking advisory warnings about completedAt timestamps and Test Plan path heuristics remain unchanged — they are non-blocking and not in scope for this packet).
`artifact-lint.sh specs/029-devops-pipeline` returns **PASSED**.
`traceability-guard.sh specs/029-devops-pipeline` returns **PASSED**.
`state-transition-guard.sh specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at` returns **0 BLOCKs**.
`artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at` returns **PASSED**.
`traceability-guard.sh specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at` returns **PASSED**.
Single commit with structured prefix `bubbles(029/bug-029-007)`.
