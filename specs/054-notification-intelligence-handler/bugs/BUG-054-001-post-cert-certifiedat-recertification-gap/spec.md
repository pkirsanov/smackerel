# BUG-054-001 — Spec 054 `certifiedAt` predates post-cert scopes.md drift cleanup, blocking Gate G088

| Field | Value |
|-------|-------|
| Parent spec | `specs/054-notification-intelligence-handler/` |
| Discovered by | Sweep round 15 (`stochastic-quality-sweep` → `devops-to-doc` mapped from `devops` trigger, parent-expanded-child-mode) |
| Discovered at HEAD | `31e964957823eaff18d2ee4ae9bfe0a8b59a1cdb` (2026-06-06T00:32:40Z) |
| Severity | medium |
| Class | governance · state.json recertification gap · Gate G088 |
| Status | open |

## Problem Statement

`bash .github/bubbles/scripts/state-transition-guard.sh specs/054-notification-intelligence-handler`
fails Check 30 (Gate G088 — `post_certification_spec_edit_gate`) with:

```text
G088 post_certification_spec_edit_gate violation: certified planning truth changed after certifiedAt
  spec: specs/054-notification-intelligence-handler
  status: done
  certifiedAt: 2026-06-03T23:59:59Z
  trackedFiles: 3
  postCertEdits: 1
  commits/files:
    - commit=48ad42a79caa8b8a4fbe042f00507e696d457291 date=2026-06-05T16:15:45+00:00 file=specs/054-notification-intelligence-handler/scopes.md subject=fix(specs/039,041,054): tier-3 drift cleanup + ratchet 365 -> 356
```

Spec 054 carries top-level `certifiedAt: "2026-06-03T23:59:59Z"` and
`certification.status: "done"`. The state.json schema is already
G088-conformant (this is NOT a missing-field gap like BUG-049-002).
The gap is that a single later commit `48ad42a7` (2026-06-05T16:15:45Z)
edited `scopes.md` to fix tier-3 drift in Test Plan path annotations
without bumping `certifiedAt` or adding a `bubbles.spec-review`
`reviewStatus: CURRENT` entry.

Inspection of `git show 48ad42a79caa -- specs/054-notification-intelligence-handler/scopes.md`
(`--numstat` reports 4 added + 4 removed lines) confirms the edit is
strictly **planning-text pointer drift**: the Test Plan table cells
for SCN-054-004, SCN-054-005, SCN-054-006, and SCN-054-022 had their
"File/Location" column updated from a planned-but-not-created e2e test
path (e.g., `tests/e2e/notification_ingest_api_test.go`) to the actual
file where the equivalent live-DB coverage lives (e.g.,
`internal/api/notifications_pipeline.go`, with a parenthetical
explaining the consolidation). The test FUNCTION NAMES, the SCENARIO
IDs, the Gherkin BODIES, the DoD checkboxes, and the design.md /
spec.md / scenario-manifest.json artifacts are unchanged.

So the underlying contract is unchanged; the state.json `certifiedAt`
timestamp simply did not advance to reflect the harmless drift
cleanup.

## Why It Matters

1. **Promotion blocker.** `state-transition-guard.sh
   specs/054-notification-intelligence-handler` exits non-zero on the
   current HEAD. Any future workflow that needs to re-promote spec 054
   (e.g., a sweep that touches spec 054, a release planning
   recertification, or a portfolio drift fix) cannot complete until
   G088 passes.
2. **Sweep audit chain.** Round 15 of the active 20-round stochastic
   sweep landed on spec 054 with the `devops` trigger. The devops
   probe surfaced this guard failure as a first-class finding;
   ignoring it would silently bake the gap into the sweep history.
3. **Governance precedent.** BUG-049-002 already established the
   recertification pattern (`bubbles.spec-review`
   `reviewStatus: CURRENT` + `certifiedAt` advance). This bug applies
   the same pattern to spec 054 — only the trigger differs (post-cert
   scopes.md drift cleanup instead of post-cert spec.md/design.md
   successor notices).

## Scenarios (Gherkin)

### SCN-054-B001 — Spec 054 G088 PASSES after recertification

```gherkin
Given specs/054-notification-intelligence-handler/state.json with status "done"
And   a top-level certifiedAt timestamp on or after the latest post-cert
      edit on scopes.md (2026-06-05T16:15:45Z)
And   a bubbles.spec-review executionHistory entry with reviewStatus "CURRENT"
      whose runCompletedAt is on or before that certifiedAt
When  bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh
      specs/054-notification-intelligence-handler is invoked
Then  the guard exits 0
And   it reports "PASS Gate G088 (post_certification_spec_edit_gate)"
And   the message annotates certifiedAt and currentSpecReview with the new
      recertification timestamp.
```

### SCN-054-B002 — Adversarial: a future post-cert planning-truth edit still trips G088

```gherkin
Given specs/054-notification-intelligence-handler/state.json with status "done"
And   the new certifiedAt set at the moment of this recertification
When  a hypothetical post-cert edit lands on spec.md, design.md, or scopes.md
      after that certifiedAt
And   neither requiresRevalidation:true nor a fresh bubbles.spec-review
      CURRENT entry is recorded
Then  post-cert-spec-edit-guard.sh exits non-zero
And   it names the offending commit, file, and subject in its diagnostic
      output.
```

> SCN-054-B002 is enforced by the existing `post-cert-spec-edit-guard.sh`
> logic (the `if [[ "${#post_cert_entries[@]}" -gt 0 ]]; then ... exit 1`
> branch). The bug's regression evidence demonstrates this branch by
> citing the conditional in the guard's source, avoiding any real
> planning-truth churn that would itself trigger the gate.

## Out Of Scope

- Re-running every spec 054 contract test in CI. The full test surface
  was already executed at validate-owned certification on 2026-05-24
  and again at the 2026-06-03 reopen-for-Scope-9-deferral closeout.
  The contract suite remains green and is re-run on every
  `./smackerel.sh test unit` invocation.
- Re-running the post-cert commit `48ad42a7` itself — it is the
  upstream operator's drift-cleanup work and stays as-is. This bug only
  ratifies that the existing edit is non-invalidating.
- Tightening G088's logic. The gate is framework-managed and immutable
  from this repo (`.github/bubbles/scripts/post-cert-spec-edit-guard.sh`).
- Editing parent spec planning truth (`spec.md`, `design.md`,
  `scopes.md`). Doing so would re-trigger the gate and defeat the
  purpose of the recertification.
- Wiring the notification-handler Prometheus metrics that Scope 8's
  implementation plan mentioned. That observability follow-up is a
  separate MEDIUM observation tracked in round 15's RESULT-ENVELOPE
  and report.md, and would require its own design + delivery cycle on
  a future round targeting `harden`, `gaps`, or `devops` triggers.

## Acceptance Criteria

1. `specs/054-notification-intelligence-handler/state.json` top-level
   `certifiedAt` is advanced from `"2026-06-03T23:59:59Z"` to an
   RFC3339 timestamp on or after `2026-06-05T16:15:45Z` (the post-cert
   commit timestamp).
2. `specs/054-notification-intelligence-handler/state.json`
   `executionHistory` records a `bubbles.spec-review` entry with
   `reviewStatus: "CURRENT"` whose `runCompletedAt` is on or before
   the new top-level `certifiedAt`. The entry's `summary` cites the
   post-cert commit (`48ad42a7`) and confirms it is planning-text
   pointer drift only.
3. `specs/054-notification-intelligence-handler/state.json`
   `executionHistory` also records a `bubbles.workflow` sweep-round
   entry that names round 15 / trigger `devops` / mapped mode
   `devops-to-doc` / executionModel `parent-expanded-child-mode`.
4. `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh
   specs/054-notification-intelligence-handler` exits 0 with a PASS
   message that annotates the new `certifiedAt` and the
   `currentSpecReview` timestamp.
5. `bash .github/bubbles/scripts/state-transition-guard.sh
   specs/054-notification-intelligence-handler` exits 0 (no BLOCK at
   Check 30 — Gate G088).
6. `bash .github/bubbles/scripts/artifact-lint.sh
   specs/054-notification-intelligence-handler` still PASSES.
7. `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh
   specs/054-notification-intelligence-handler` still PASSES with 0
   warnings.
8. `bash .github/bubbles/scripts/artifact-lint.sh
   specs/054-notification-intelligence-handler/bugs/BUG-054-001-post-cert-certifiedat-recertification-gap`
   passes.

## Product Principle Alignment

This bug supports Principle 8 (*"Trust Through Transparency"*) — a
certified spec MUST carry a `certifiedAt` that genuinely post-dates
every change to its planning truth, so any observer can verify the
contract has been ratified as of a known moment. It also supports
Principle 9 (*"Design For Restart, Not Perfection"*): the fix is
data-only, reversible, and uses an existing framework guard (G088) as
the regression mechanism.

### Single-Capability Justification

This bug is a single data-only schema repair on one artifact
(`specs/054-notification-intelligence-handler/state.json`). It does
NOT introduce a new capability, a second provider/component/variant,
or any adapter/strategy/plugin pattern. The framework already owns
the singular capability "state.json schema + Gate G088 recertification
ledger" in `.github/bubbles/scripts/post-cert-spec-edit-guard.sh`. No
multi-implementation foundation is warranted.
