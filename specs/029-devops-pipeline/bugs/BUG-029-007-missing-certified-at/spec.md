# Spec: BUG-029-007 — Missing Top-Level certifiedAt After Post-Cert OPS-001 Spec.md Banner Sweep

**Parent Spec:** 029-devops-pipeline
**Discovered:** 2026-06-05 (stochastic sweep round 7 of 20, trigger=regression, mapped child workflow=regression-to-doc)
**Mode:** bugfix-fastlane (artifact reconciliation; zero runtime change)

## Use Cases

- **UC-01 — Sweep round closes 1 governance regression finding cleanly.** When the stochastic sweep parent dispatches `regression-to-doc` against `specs/029-devops-pipeline`, the orchestrator (a) re-runs the persistent regression cover (Go contract tests + Python pytest suite) and finds zero functional drift, (b) re-runs `state-transition-guard.sh` and finds 1 G088 BLOCK for missing `certifiedAt`, (c) creates this BUG packet to reconcile the legacy artifact drift via recertification, and (d) brings the parent spec guard count from 1 BLOCK back to 0 BLOCKs without touching production code, tests, or runtime configuration.
- **UC-02 — Auditors can verify spec 029 was recertified CURRENT after OPS-001 banner edit.** When a reviewer cross-checks `specs/029-devops-pipeline/state.json::certifiedAt` against `state.json::executionHistory`, there MUST be exactly one `bubbles.spec-review` entry with `reviewStatus: CURRENT` whose `runCompletedAt` is >= the latest commit touching `spec.md`/`design.md`/`scopes.md` (currently `19b31c0a` @ 2026-05-28T05:07:50Z).
- **UC-03 — Future post-cert edits trigger a fresh G088 cycle.** With `certifiedAt: 2026-06-05T22:00:00Z` in place, any subsequent commit touching planning truth files for spec 029 will trip G088 again and require either (a) fresh `bubbles.spec-review` CURRENT + `certifiedAt` advance, (b) `requiresRevalidation: true` flag, or (c) status demotion — the canonical reconcile loop.

## Functional Requirements

- **FR-01 — G088 closure via top-level certifiedAt.** `specs/029-devops-pipeline/state.json` MUST gain a top-level `"certifiedAt": "2026-06-05T22:00:00Z"` field (RFC3339 UTC). This timestamp MUST be >= 2026-05-28T05:07:50Z (the OPS-001 banner sweep commit time) so the OPS-001 commit becomes a PRE-certification edit, not a post-cert edit.
- **FR-02 — bubbles.spec-review CURRENT executionHistory entry.** `state.json::executionHistory` MUST gain a new entry with `agent: "bubbles.spec-review"`, `phasesExecuted: ["spec-review"]`, `reviewStatus: "CURRENT"`, `runCompletedAt: "2026-06-05T22:00:00Z"`, `executionModel: "parent-expanded-specialist"`, and a summary documenting that all 7 scopes' planning truth (spec.md/design.md/scopes.md) was cross-checked against the live runtime surfaces (`.github/workflows/ci.yml`, `.github/workflows/build.yml`, `Dockerfile`, `ml/Dockerfile`, `docker-compose.yml`, `deploy/compose.deploy.yml`, `scripts/commands/config.sh`, `docs/Branch_Protection.md`, `docs/Deployment.md`, `docs/Operations.md`, `internal/api/health.go`, `internal/deploy/*`) at HEAD `e05aef1b` and found CURRENT.
- **FR-03 — Parent spec 029 resolvedBugs entry.** `state.json::resolvedBugs[]` MUST gain a new entry for `BUG-029-007-missing-certified-at` with `sweepId`, `sweepRound: 7`, `trigger: "regression"`, `mappedChildMode: "regression-to-doc"`, `executionModel: "parent-expanded-child-mode"`, and a summary documenting the recertification.
- **FR-04 — Parent spec 029 report.md recertification evidence.** `specs/029-devops-pipeline/report.md` MUST gain a `### BUG-029-007 Recertification Evidence (Sweep Round 7 of 20)` subsection containing the pre→post `state-transition-guard.sh` re-run blocks (red=1 BLOCK pre-mutation, green=0 BLOCKs post-mutation), the persistent regression cover GREEN-by-construction statement, and the bubbles.spec-review CURRENT verification summary.
- **FR-05 — Single closure commit with structured prefix.** Closure commit MUST be a single commit on `main` with the structured prefix `bubbles(029/bug-029-007)` and MUST touch ONLY paths under `specs/029-devops-pipeline/`.

## Acceptance Criteria

- **AC-01** — `bash .github/bubbles/scripts/state-transition-guard.sh specs/029-devops-pipeline` exits 0 with `🔴 BLOCK` count == 0 (the 2 pre-existing non-blocking `⚠️ WARN` lines are out of scope).
- **AC-02** — `bash .github/bubbles/scripts/state-transition-guard.sh specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at` exits 0 with 0 BLOCKs.
- **AC-03** — `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline` returns `Artifact lint PASSED.`.
- **AC-04** — `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at` returns `Artifact lint PASSED.`.
- **AC-05** — `bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline` returns `RESULT: PASSED (0 warnings)`.
- **AC-06** — `bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at` returns `RESULT: PASSED`.
- **AC-07** — `specs/029-devops-pipeline/state.json::certifiedAt` is `"2026-06-05T22:00:00Z"` (string, RFC3339, >= 2026-05-28T05:07:50Z OPS-001 commit time).
- **AC-08** — `specs/029-devops-pipeline/state.json::executionHistory[] | select(.agent == "bubbles.spec-review" and .reviewStatus == "CURRENT")` returns exactly one entry whose `runCompletedAt` equals `"2026-06-05T22:00:00Z"`.
- **AC-09** — `specs/029-devops-pipeline/state.json::resolvedBugs[] | select(.bugId == "BUG-029-007-missing-certified-at")` returns exactly one entry with `status: "resolved"`, `sweepRound: 7`, `trigger: "regression"`, `mappedChildMode: "regression-to-doc"`.
- **AC-10** — Closure commit is single, prefix `bubbles(029/bug-029-007)`, and `git diff --cached --name-status` (captured pre-commit) lists ONLY paths under `specs/029-devops-pipeline/` (no stray edits to spec 003, 009, 016, 037, 067, internal/connector/bookmarks/, internal/connector/weather/, tests/integration/policy/, or any other workspace dirty paths).
