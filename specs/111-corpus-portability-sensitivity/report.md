# Report: 111 Corpus Portability & Artifact Sensitivity

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Owner of this artifact:** the delivery agents
**Created by:** `bubbles.analyst` as an honest initial artifact so the packet is structurally complete

---

## Summary

This packet was **authored**, not executed. `bubbles.analyst` created the requirements in
[`spec.md`](spec.md) from diagnostic findings D12, D23 and D11, governed by
[`docs/Product-Principles.md`](../../docs/Product-Principles.md) **Principle 11 — Local-First
Data Ownership** (ratified 2026-07-29, BLOCKING), and re-verified every current-state claim
against the working tree on 2026-08-04 before writing a single requirement.

**No source file was changed. No feature work was performed. No test was run.**

What this run produced:

- `spec.md` — problem statement, outcome contract, a twelve-row verified evidence base, a
  domain capability model with eight binding policies, five actors, five use cases, 25
  Gherkin scenarios with stable `SCN-111-*` ids, 29 requirements, six non-functional
  requirements, six non-goals, eight routed open findings, product principle alignment,
  the release-train declaration, and an exposure contract.
- `design.md`, `scopes.md`, `uservalidation.md`, `state.json`, and this file — honest
  initial artifacts. Each states in its own opening section that it carries no decision,
  no ratified plan, and no evidence.

**Artifact ownership deviation, recorded rather than hidden.** `bubbles.analyst` owns
`spec.md`. `design.md` is owned by `bubbles.design`, `scopes.md` by `bubbles.plan`, and
`report.md` and `uservalidation.md` by the delivery agents. They were created in this run
under an explicit operator instruction to produce the full initial artifact set so the
packet is structurally complete and lintable. Each carries a header naming its real owner
and stating that its contents are provisional. The same record appears in
`state.json.executionHistory`.

## Source Verification

Every defect claim in the authoring brief was re-read against the working tree before it
was written into `spec.md`. All four claims hold. **Two line references in the brief were
wrong and were corrected rather than copied:**

| Brief claim | Outcome |
|---|---|
| Export emits only processed rows — `internal/db/postgres.go:92` | Claim **holds**; the filter is at **`:122`**, not `:92`. Reference corrected in `spec.md` E1. |
| Export pagination is `created_at`-only, orders on `created_at`, serialises RFC3339 seconds while the store keeps sub-second precision | **Holds in full.** Filter and ordering at `postgres.go:122-123`; serialisation at `internal/api/capture.go:388`; parsing at `:356`; column type at `001_initial_schema.sql:39`. Recorded as E3, E4, E5. |
| No user-facing delete surface at any granularity | **Holds.** `internal/api/router.go` exposes export at `:105`; every delete route present covers annotation tags, lists, list items, preference corrections, watches, evidence exports, drive rules, and agent model config. No artifact, source, topic, or whole-corpus delete exists. Recorded as E6. |
| `artifacts` has no canonical sensitivity column — `001_initial_schema.sql:24` | Claim **holds**; the accurate statement is that **no line in the whole `artifacts` declaration (`:16-63`) defines one**, not that line 24 does something wrong. Reference corrected in `spec.md` E7. |

Two facts were found that the brief did not state and that strengthen it. Canonical
sensitivity columns **already exist** for other record classes (`drive_files.sensitivity`
with a CHECK constraint; the `photo_sensitivity` enum), which makes the `artifacts` gap an
inconsistency with the repository's own established pattern rather than an open design
question (E9). And a repository-wide search for a remote-egress decision or audit symbol
returns nothing outside test files, confirming there is no location today where such a
decision could even be recorded (E10).

## Unverifiable Premise

The authoring brief instructed that this spec follow `.specify/templates/spec-template.md`.
**That file does not exist in this repository** — `.specify/templates/` is absent, and the
template exists only in sibling repositories in the workspace. The spec therefore follows
the structure established by the adjacent spec in this repository together with the Bubbles
BDD scenario contract, and the discrepancy is recorded as finding `F-111-TEMPLATE-01`
rather than being silently ignored or falsely claimed as satisfied.

## Findings

Eight findings are recorded in [`spec.md`](spec.md) §11 with severity and owner. Four are
carried into [`scopes.md`](scopes.md) as explicit per-scope blockers rather than left for
discovery during execution.

| ID | Severity | Owner |
|---|---|---|
| `F-111-FLAG-01` | BLOCKING | `bubbles.train` |
| `F-111-MANIFEST-01` | BLOCKING | `bubbles.design` |
| `F-111-110-01` | HIGH | `bubbles.design` (with spec 110) |
| `F-111-108-01` | HIGH | `bubbles.plan` |
| `F-111-BACKFILL-01` | HIGH | `bubbles.design` |
| `F-111-TEMPLATE-01` | MEDIUM | operator via `bubbles.plan` |
| `F-111-DELETE-01` | MEDIUM | `bubbles.design` |
| `F-111-CENSUS-01` | MEDIUM | `bubbles.design` |

`F-111-FLAG-01` is the finding that most constrains delivery. A capability of this size
warrants a feature flag, but G111 requires an introduced flag to be default-OFF in every
non-owning train's bundle — which means editing `config/feature-flags.mvp.yaml`, an artifact
owned by `bubbles.train` and outside this run's permitted surface. `flagsIntroduced` is
therefore an empty array: honest, and deliberately incomplete rather than falsely populated.

## Completion Statement

**This feature is `not_started` and nothing in this packet claims otherwise.**

- Every scope in [`scopes.md`](scopes.md) is `Not Started`.
- Every Definition-of-Done checkbox is unchecked.
- No test was executed, so no test result is reported.
- No source file was modified.
- `state.json` carries `status: "not_started"`, matching `certification.status`, with
  `certification.evidenceComplete: false` and no certified phases.
- The only work performed was requirements authoring plus the source verification recorded
  above, and the artifact set that makes the packet lintable.

Terminal status for workflow mode `product-to-planning` is `specs_hardened`. This packet has
**not** reached it: `design.md` and `scopes.md` are unowned placeholders, and three findings
block the design pass. Promotion to any status above `not_started` requires the design and
planning passes to run under their own owners.

## Test Evidence

**No test has been executed for this feature, because no feature code exists.**

Nothing in this packet asserts that any test passed, any command succeeded, any metric was
measured, or any behaviour was verified. The evidence rows E1 through E12 in
[`spec.md`](spec.md) §3 are **current-state source observations** — statements about what
the code says today, each with a file and line reference a reader can check — and they are
not test results. They prove the defects exist; they prove nothing about a fix, because no
fix exists.

The coverage each scope must carry is recorded in the Test Plan in
[`scopes.md`](scopes.md). No row in that table names a test file path, because no such file
has been created and naming one would be a fabricated claim.

Two artifact-governance commands were executed against this packet during authoring —
`artifact-lint.sh` and `release-train-guard.sh`. Their results belong to the authoring run
and are reported to the caller in the run's result envelope. They validate artifact
structure and release-train declarations. They are **not** feature test evidence and are
not presented as such.
