# Spec: Spec 055 state.json done_with_concerns Schema Compliance

## Expected Behavior

The parent `specs/055-notification-source-ntfy-adapter/state.json` carries `status: "done_with_concerns"` at both the top level and inside `certification`. Per [`completion-governance.md`](../../../../.github/agents/bubbles_shared/completion-governance.md), that outcome state MUST be backed by a non-empty `certification.concerns` array whose every entry follows the structured shape (`id`, `severity ∈ {low, medium}`, `summary`, `followUpOwner`, `followUpAction`).

The child bug `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json` also carries `done_with_concerns` and currently records a non-empty `certification.concerns` array, but each entry is a plain string instead of a structured object. That violates the same contract.

Both files must satisfy the schema, and a deploy-side contract test must lock the schema in so any future regression on these two files fails `./smackerel.sh test unit --go` immediately.

## Acceptance Criteria

- `specs/055-notification-source-ntfy-adapter/state.json` `certification.concerns` exists, is a non-empty JSON array, and every entry is a JSON object that includes `id`, `severity`, `summary`, `followUpOwner`, `followUpAction`.
- Every concern's `severity` is exactly `low` or `medium`. No other values are permitted.
- Every concern's `followUpOwner` is a concrete Bubbles agent name or the literal `human`. The strings `tbd` and `everyone` are rejected.
- Every concern's `followUpAction` is exactly one of `new-spec`, `issue-doc`, `next-sprint-todo`, or `accept`.
- Every concern's `id` is unique within its own state.json file.
- `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json` `certification.concerns` satisfies the same structured-shape rules (each entry is an object with the same required fields).
- The original string-form prose carried by the child bug's pre-fix `concerns` array is preserved verbatim inside the new structured entries' `summary` fields. No information is lost.
- A new test at `internal/deploy/state_concerns_contract_test.go` asserts the schema for both files and fails when status is `done_with_concerns` and either array is missing, empty, or contains a non-conforming entry.
- The test fails deterministically against the pre-fix tree (verified by deterministic-red evidence) and passes after the fix.
- `./smackerel.sh test unit --go` includes the new test via `./...` auto-discovery.
- `./smackerel.sh format --check` and `./smackerel.sh lint` both pass after the change.
- Artifact lint and traceability guard pass on both the BUG-DEVOPS-20260525-001 packet and the parent spec 055.
- State-transition-guard on the parent spec 055 remains PERMITTED with 0 BLOCKs (advisory warnings unchanged from baseline).
- No file outside the change boundary listed in `scopes.md` is modified.

## Gherkin Scenario

```gherkin
Scenario: Spec 055 state.json done_with_concerns parent envelope passes schema
  Given specs/055-notification-source-ntfy-adapter/state.json declares status "done_with_concerns"
  When the state_concerns contract test parses certification.concerns
  Then the array is non-empty
  And every entry is a JSON object
  And every entry has id, severity, summary, followUpOwner, followUpAction
  And severity is one of {low, medium}
  And followUpAction is one of {new-spec, issue-doc, next-sprint-todo, accept}

Scenario: BUG-CHAOS-20260524-001 state.json done_with_concerns child envelope passes schema
  Given specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json declares certification.status "done_with_concerns"
  When the state_concerns contract test parses certification.concerns
  Then every entry is a JSON object with the structured shape
  And no entry is a flat string
  And every entry's followUpOwner is a concrete agent name or the literal "human"
```

## Non-Goals

- Do not modify any framework-managed file under `.github/bubbles/scripts/`, `.github/agents/bubbles_shared/`, or `.github/bubbles/workflows.yaml`.
- Do not promote the child bug `BUG-CHAOS-20260524-001` from `done_with_concerns` to `done` (the legacy report.md evidence-fence cleanup that drove the `done_with_concerns` classification remains outstanding and tracked through the new structured concern).
- Do not perform the legacy report.md evidence-fence cleanup itself. That is a separate docs-only follow-up captured as a single structured concern on parent state.json.
- Do not extend the new contract test to all 100+ specs in the repo. The test stays narrowly scoped to the two files this bug fixes; if future devops rounds discover the same drift elsewhere, those rounds spawn their own bug packets.
- Do not change any runtime, deploy, security, or operator-facing surface. Zero `.go` runtime files, zero `docker-compose.*` files, zero `deploy/**` files, zero `config/smackerel.yaml` keys, zero `internal/api/**` routes.
