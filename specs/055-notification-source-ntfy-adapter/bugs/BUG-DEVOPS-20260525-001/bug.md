# BUG-DEVOPS-20260525-001: Spec 055 state.json Declares done_with_concerns But concerns Array Is Missing

## Status

Fixed - state.json `certification.concerns` populated with structured entries per completion-governance.md schema; child bug `concerns` normalized from string list to structured objects; deploy-side contract test added to lock down the schema.

## Severity

P3 - Low. Governance/artifact-integrity drift; zero runtime, deploy, security, or operator-facing impact. The schema violation makes parent spec 055 audit-illegible to any downstream reader (or contract test) that assumes `done_with_concerns ⇒ concerns: [...] non-empty`.

## Source

- Agent: bubbles.devops
- Discovered at: 2026-05-25T03:21:00Z
- Sweep: sweep-2026-05-24-r10, round 6 of 10
- Mapped child workflow mode: devops-to-doc
- Feature: specs/055-notification-source-ntfy-adapter

## Symptom

The parent spec 055 `state.json` carries `status: "done_with_concerns"` (top-level) and `certification.status: "done_with_concerns"` but `certification.concerns` is absent entirely — no key, no empty array, nothing. The notes field at the bottom of the same state.json explicitly enumerates the concerns that should have been recorded structurally (stale legacy report.md evidence fences not contradicted by latest executed evidence; child bug `BUG-CHAOS-20260524-001` literal child done blocked by the same evidence-fence cleanup).

The child bug `BUG-CHAOS-20260524-001` has a `certification.concerns` array but its entries are plain strings, not the structured objects required by `.github/agents/bubbles_shared/completion-governance.md` (each entry MUST have `id`, `severity ∈ {low, medium}`, `summary`, `followUpOwner`, `followUpAction ∈ {new-spec, issue-doc, next-sprint-todo, accept}`).

## Reproduction

1. From repo root, inspect parent state.json:

   ```bash
   python3 -c "import json; d=json.load(open('specs/055-notification-source-ntfy-adapter/state.json')); print('top-level status:', d.get('status')); print('certification.status:', d.get('certification',{}).get('status')); print('certification.concerns:', d.get('certification',{}).get('concerns','MISSING'))"
   ```

2. Observe: `top-level status: done_with_concerns`, `certification.status: done_with_concerns`, `certification.concerns: MISSING`.

3. Inspect child bug state.json:

   ```bash
   python3 -c "import json; d=json.load(open('specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json')); cn=d.get('certification',{}).get('concerns',[]); print('count:', len(cn)); print('type of first:', type(cn[0]).__name__)"
   ```

4. Observe: `count: 7`, `type of first: str` — strings instead of structured objects with `severity / followUpOwner / followUpAction`.

## Expected Behavior

Per [`completion-governance.md` → Outcome State: done_with_concerns](../../../../.github/agents/bubbles_shared/completion-governance.md), any artifact carrying `done_with_concerns` (either at top-level `status` or `certification.status`) MUST include a non-empty `certification.concerns` array whose every entry follows the structured shape:

```yaml
concerns:
  - id: CONCERN-1                        # short stable id, unique within envelope
    severity: low | medium               # ONLY low or medium permitted
    summary: >                            # 1-2 sentences, concrete and observable
      ...
    followUpOwner: <agent-name> | human  # concrete owner, never `tbd` or `everyone`
    followUpAction: new-spec | issue-doc | next-sprint-todo | accept
```

## Actual Behavior

- Parent state.json: `certification.concerns` is absent entirely.
- Child bug state.json: `certification.concerns` exists but its entries are flat strings, violating the structured-object requirement.

The state-transition-guard.sh does not (yet) catch this — its current set of 22 checks validates schema invariants other than the `done_with_concerns ⇒ structured concerns` contract — so the drift accumulated silently. The notes field in the parent state.json describes the concerns in prose, confirming this is honest under-recording rather than a fabricated promotion.

## Impact

- **Audit-legibility:** Any downstream reader or tooling that consumes `state.json` to derive a "what follow-ups remain?" view of spec 055 will see zero structured concerns despite the explicit `done_with_concerns` status.
- **Schema discipline:** Without the contract test added by this bug fix, any future spec promoted to `done_with_concerns` is free to repeat the same violation.
- **Runtime:** None. No code, deploy, config, or security path changes.

## Out of Scope

- Cleaning up the legacy `report.md` evidence fences referenced by the existing concerns. That work is captured as a single structured concern on the parent state.json itself, with `followUpOwner: bubbles.docs` and `followUpAction: next-sprint-todo`. A future docs-only sweep round owns the actual cleanup.
- Extending the same contract enforcement to all 100+ specs in the repo. This bug fixes the one parent + one child that the round-6 devops probe surfaced; broader enforcement is naturally inherited because the new `internal/deploy/state_concerns_contract_test.go` runs on every `./smackerel.sh test unit --go` invocation and any new violation on these two paths will fail it.
- Modifying any framework-managed file under `.github/bubbles/` (the state-transition-guard.sh check addition is upstream work for the Bubbles framework repo, not this project).
