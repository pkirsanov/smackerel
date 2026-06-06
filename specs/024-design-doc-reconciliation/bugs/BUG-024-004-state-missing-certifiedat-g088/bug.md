# Bug: BUG-024-004 specs/024-design-doc-reconciliation/state.json missing top-level `certifiedAt` triggers Gate G088 post-certification spec edit guard BLOCK

## Summary

Sweep round 19 of `sweep-2026-06-06-r20` (`mode: gaps-to-doc`, parent-expanded) ran the gaps trigger probe on `specs/024-design-doc-reconciliation` and surfaced one BLOCKING governance gap that survived BUG-024-003's chaos-hardening close-out on 2026-05-27:

1. **F1 (MEDIUM, BLOCKING governance gate) — `specs/024-design-doc-reconciliation/state.json` is missing the top-level `certifiedAt` field that Gate G088 (`post_certification_spec_edit_gate`) requires for any spec with `status: done`.** Running `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 1 with `🔴 BLOCK: Post-certification spec edit guard failed — Gate G088`. The direct `post-cert-spec-edit-guard.sh` diagnostic is: `post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec specs/024-design-doc-reconciliation (status=done)`. The parent spec's `certification` block carries per-scope `certifiedAt: 2026-04-10T14:00:00Z` and `2026-04-10T14:30:00Z`, plus `certification.status: done`, but the TOP-LEVEL `certifiedAt` string field that G088 demands has never been backfilled. Two sibling bug packets in the same parent (BUG-024-002, BUG-024-003) already carry top-level `certifiedAt` + `certifiedBy` strings — the parent spec itself was overlooked when G088's contract was extended.

## Severity

- [ ] Critical — System unusable, data loss
- [ ] High
- [x] Medium — Blocks every future state-transition-guard run against spec 024 (every sweep round will block on G088 instead of being able to run its other 28 checks); no runtime impact; the planning-truth files (spec.md/design.md/scopes.md) have not regressed; the spec's substantive certification status is preserved
- [ ] Low

## Status

- [x] Reported
- [x] Confirmed by sweep round 19 gaps-to-doc probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD with spec 024 in its current `status: done` state, run:
   ```bash
   bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation > /tmp/stg-024-r19.log 2>&1
   echo "EXIT=$?"
   ```
2. Observe `EXIT=1` and:
   ```
   🔴 BLOCK: Post-certification spec edit guard failed — Gate G088. Run 'bash ~/smackerel/.github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation' for full diagnostic
     TRANSITION GUARD VERDICT
   🔴 TRANSITION BLOCKED: 1 failure(s), 2 warning(s)
   ```
3. Run the direct diagnostic:
   ```bash
   bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation
   ```
4. Observe:
   ```
   post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec specs/024-design-doc-reconciliation (status=done)
   ```
5. Inspect spec 024 `state.json` for the missing field:
   ```bash
   grep -nE '"certifiedAt"' specs/024-design-doc-reconciliation/state.json
   ```
6. Observe only per-scope `certifiedAt` lines under `certification.scopeProgress[].certifiedAt` — zero top-level `certifiedAt`.
7. Compare with sibling bug packets that DO satisfy G088:
   ```bash
   grep -nE '"certifiedAt"' specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/state.json
   grep -nE '"certifiedAt"' specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/state.json
   ```
8. Observe both sibling packets carry `"certifiedAt": "2026-05-27T00:00:00Z"` at line 8 (top level) plus the cert block + per-scope entries — 3 occurrences each, mirroring the G088 contract.

## Expected Behavior

- `specs/024-design-doc-reconciliation/state.json` has a top-level `"certifiedAt"` string field with an RFC3339 timestamp ≥ the latest commit timestamp that touched spec.md / design.md / scopes.md / scopes/_index.md / scopes/*/scope.md.
- `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation` exits 0 with `PASS Gate G088 (post_certification_spec_edit_gate)`.
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 0 with `🟢 TRANSITION ALLOWED` (carrying the 2 pre-existing non-blocking warnings only — those are out of scope for this bug).
- Spec 024 `status` remains `done`; `certification.status` remains `done`; per-scope `certifiedAt` entries preserved verbatim; `certification.completedScopes` and `certification.certifiedCompletedPhases` preserved verbatim; the substantive certification of scopes 1 and 2 is unchanged.

## Actual Behavior

- `state.json` has no top-level `certifiedAt`. Three `certifiedAt` lines exist but all are nested under `certification.scopeProgress[]`.
- `state-transition-guard.sh` cannot progress past Check 23B (post-cert spec edit guard) — it exits with a single failure and never reaches the remaining checks.
- This silently blocks every future sweep round, every CI gate run, and every recertification attempt against spec 024.

## Environment

- Branch: `main`
- Sweep: `sweep-2026-06-06-r20` round 19, mode `gaps-to-doc`, executionModel `parent-expanded-child-mode` (subagent runtime lacks nested `runSubagent`; mode is single-spec and not `requiresTopLevelRuntime`, so parent-expansion is policy-compliant per `bubbles-workflow-execution-loops`)
- Parent feature: `specs/024-design-doc-reconciliation` (`status: done` end-to-end; original cert 2026-04-10; BUG-024-001 closed; BUG-024-002 closed 2026-05-24; BUG-024-003 closed 2026-05-27; OPS-001 swept `spec.md` `**Status:**` banner on 2026-05-28T05:07:50Z)
- Source-of-truth for the missing-field contract: `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` lines 179-184 (`certified_type` check → exit 2 BLOCK if not a string)
- Latest planning-truth commit touching tracked paths: `2026-05-28T05:07:50+00:00` commit `19b31c0a` (`bubbles(ops/OPS-001): sweep spec.md status banners across 54 certified specs`) → backfill `certifiedAt` MUST be **strictly greater than** this timestamp because `post-cert-spec-edit-guard.sh` invokes `git log --since=$certifiedAt` which is INCLUSIVE of commits at the exact same instant. Backfill value chosen: `2026-05-28T05:07:51Z` (1 second after the OPS-001 commit, the smallest RFC3339 increment that excludes it from the post-cert window)
- Two sibling bug packets that already comply: `bugs/BUG-024-002-reconcile-artifact-drift/state.json` line 8 (`"certifiedAt": "2026-05-27T00:00:00Z"`); `bugs/BUG-024-003-dev-doc-connector-drift/state.json` line 8 (`"certifiedAt": "2026-05-27T00:00:00Z"`)

## Error Output

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -5
🔴 BLOCK: Post-certification spec edit guard failed — Gate G088. Run 'bash ~/smackerel/.github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation' for full diagnostic
  TRANSITION GUARD VERDICT
🔴 TRANSITION BLOCKED: 1 failure(s), 2 warning(s)

$ bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation
post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec specs/024-design-doc-reconciliation (status=done)

$ grep -nE '"certifiedAt"' specs/024-design-doc-reconciliation/state.json
62:        "certifiedAt": "2026-04-10T14:00:00Z"
73:        "certifiedAt": "2026-04-10T14:30:00Z"

$ grep -nE '"certifiedAt"' specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/state.json
8:  "certifiedAt": "2026-05-27T00:00:00Z",
130:    "certifiedAt": "2026-05-27T00:00:00Z",
143:        "certifiedAt": "2026-05-25T00:00:00Z"
```

## Workaround

None. G088 is a BLOCKING gate inside the state-transition-guard, and the guard is a pre-promotion / pre-push prerequisite for any subsequent state.json transition. Until the top-level `certifiedAt` field is present and ≥ the latest planning-truth file commit timestamp, every state-transition-guard invocation on spec 024 will exit 1 and prevent further governance changes.

## Root Cause Analysis (Five Whys)

- **Why does Gate G088 fail today?** Because spec 024's `state.json` has no top-level `certifiedAt` field, and `post-cert-spec-edit-guard.sh` lines 179-184 require the field to be a string for any spec with `status: done`.
- **Why was the field never added to spec 024?** The original certification on 2026-04-10 predates the G088 contract extension that requires top-level `certifiedAt`. The two sibling BUG packets (BUG-024-002, BUG-024-003) created in 2026-05 were authored AFTER the contract was tightened and correctly include the field; the parent spec was overlooked because it was already at `status: done` and nobody re-touched its top-level state.json keys after the gate tightened.
- **Why didn't BUG-024-002 or BUG-024-003 backfill the parent?** BUG-024-002's scope was reconciling the `docs/smackerel.md` §22.7 + §24-A connector inventory + augmenting `certification.certifiedCompletedPhases` with retroactive provenance entries. BUG-024-003's scope was reconciling `docs/Development.md` + spec.md R-006 + adding a forward-detection contract test. Neither bug's design.md nor scopes.md enumerated "verify parent state.json carries top-level `certifiedAt`" as a DoD item, so the gap survived both closures.
- **Why didn't a framework guard catch this earlier?** Because G088 itself catches it — but the catch only fires when state-transition-guard runs (typically only when a new transition is attempted). Between sweep rounds the gap is dormant; once a sweep round invokes STG, the gap surfaces as a BLOCK. This is exactly the gaps-trigger probe class: a documented invariant that the framework already enforces, but a single artifact in the corpus violates it.
- **Why is this categorized MEDIUM not HIGH?** The gap is governance-only — no runtime behavior is broken, no test fails (TestConnectorCountContract still PASSES), no user-facing artifact regresses, and the substantive certification of the 2 scopes is preserved. The only impact is that STG cannot progress past Check 23B on this spec until the field is added. Once the 1-line backfill lands, all downstream gates can resume.

## Related

- Parent: `specs/024-design-doc-reconciliation/`
- Prior sibling bugs:
  - `bugs/BUG-024-001-dod-scenario-fidelity-gap/` (G068 DoD fidelity, closed)
  - `bugs/BUG-024-002-reconcile-artifact-drift/` (32 BLOCKS + 19 freshness + §22.7/§24-A docs drift, closed 2026-05-24)
  - `bugs/BUG-024-003-dev-doc-connector-drift/` (3 chaos findings: docs/Development.md L31 + spec R-006 parity + forward-detection contract test, closed 2026-05-27)
- Gate source: `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` lines 179-220 (top-level `certifiedAt` requirement + RFC3339 parsing + post-cert-edit invariant)
- Gate registry: `.github/bubbles/workflows.yaml` `G088 post_certification_spec_edit_gate`
- Reference compliant peer state.jsons: `bugs/BUG-024-002-reconcile-artifact-drift/state.json` line 8; `bugs/BUG-024-003-dev-doc-connector-drift/state.json` line 8 — both carry `"certifiedAt": "2026-05-27T00:00:00Z"` at top level
- Sweep ledger: round 19 of 20, gaps trigger, mapped child mode `gaps-to-doc`, executionModel `parent-expanded-child-mode`
