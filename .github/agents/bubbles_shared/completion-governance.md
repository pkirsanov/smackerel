# Completion Governance

Use this file for the authoritative completion chain and completion-state rules.

## Absolute Completion Hierarchy

Completion flows bottom-up only:

1. A DoD item is implemented.
2. The item is validated with a real command or tool run.
3. Raw evidence is recorded inline.
4. Only then may the item be marked `[x]`.
5. A scope is `Done` only when every DoD item is `[x]` with real evidence.
6. A spec is `done` only when every scope is `Done`.

If any link in that chain is false, completion state must be lowered immediately.

## Per-DoD Validation Rules

- Each DoD item requires its own validation and its own evidence.
- Batch-checking multiple items from one generic run is invalid.
- Evidence must be current-session raw output with recognizable terminal or tool signals.
- If execution fails, the item remains `[ ]` until fixed and re-run.

## Scope Completion Rules

A scope cannot be `Done` (or `Done with Concerns`) when any of these are true:

- any DoD item is unchecked
- any checked DoD item lacks inline evidence
- required test types have not run
- evidence shows failure
- the scope contains deferral language
- the scope claims behavior that the tests do not prove

### Done with Concerns

A scope may be marked `Done with Concerns` instead of `Done` when ALL DoD items pass with evidence AND ALL gates pass, but the agent identified concrete risks worth recording. This status:

- Does NOT bypass any gate — every gate must still pass identically to plain `Done`
- Does NOT count as incomplete — `Done with Concerns` is a `Done`-equivalent for all gate checks (G024, G027)
- MUST include a `concerns` list with severity, description, and originating agent
- Is subject to `bubbles.validate` review — validate may downgrade to `In Progress` if a concern is actually a gate failure in disguise

Concerns are observational, not deferral. Examples of valid concerns:
- "Stress test p99 is 48ms, just under 50ms SLA threshold — monitor in production"
- "Third-party API latency was high during testing — may affect real-world perf"
- "Code coverage is 100% but one edge case relies on external timing"

Examples of INVALID concerns (these are deferrals, not concerns):
- "Integration test skipped due to flaky infrastructure" → fix the test
- "Admin UI not implemented yet" → scope is In Progress
- "Will address error handling in follow-up" → blocked by deferral language gate (G040)

## Spec Completion Rules

A spec cannot be `done` (or `done_with_concerns`) when any scope is:

- `Not Started`
- `In Progress`
- `Blocked`

`state.json` must reflect the lower truth immediately when artifacts and status disagree.

Implementation-bearing specs also cannot be `done` when report artifacts lack code-diff evidence showing real non-artifact runtime, config, contract, or source files in the delivery delta.

## Deferral Is Incomplete Work

The following are blocking completion signals in scope artifacts:

- `deferred`
- `future work`
- `follow-up`
- `out of scope`
- `address later`
- `separate ticket`
- `placeholder`
- `temporary workaround`
- `Next Steps` (as heading or bullet leader — indicates unfinished routed work)
- `Recommended routing:` / `Recommended resolution:` (indicates unresolved findings)
- `Ready for /bubbles.` / `Re-run /bubbles.validate` (indicates uncompleted specialist phase)
- `Commit the fix` / `Record DoD evidence` / `Run full E2E suite` (indicates unfinished mechanical work)

If deferred work is still required, the scope stays in progress.

**Detection rule:** `artifact-lint.sh` and `state-transition-guard.sh` scan for these phrases. Any match in report.md or scope artifacts blocks the `done` transition.

## Partial Stack Fixes Do Not Close Full Findings

If a finding affects both backend and consumer surfaces, completion requires the full stack to participate.

- Backend-only fixes do not close findings while frontend/mobile/admin clients still use the broken path.
- Security/trust fixes stay open until risky client storage, stale routes, or old contracts are removed from real consumers.
- Integration fixes stay open until the delivered consumer path exercises the real integrated behavior.

## Documentation Claims Require Delivery Proof

README tables, ledgers, and status matrices cannot claim a feature is delivered based only on artifact edits.

- Delivered status requires implementation evidence plus executed proof.
- Docs may describe planned or partial work only if the status is explicit and non-delivered.

## Red-Green Traceability

For new or changed behavior:

1. show the failing or missing state first when applicable
2. implement the fix or behavior
3. show the passing targeted proof
4. run the broader impacted regression coverage

For bug fixes, the failing targeted proof must come from an adversarial regression case that would fail if the bug returned. A tautological pre-fix test does not satisfy red/green traceability.

Tests and fixes must trace back to planned behavior in `spec.md`, `design.md`, `scopes.md`, and DoD.

## Consumer Trace Requirement

When work renames, removes, moves, replaces, or deprecates any public or internal interface, completion also requires:

- inventory of producers and consumers
- updates to all first-party consumers
- stale-reference search for old identifiers or paths
- regression coverage for the affected consumer flows

If consumer trace is incomplete, the work stays `in_progress`.

## Scope Size Discipline

Scopes are small by default:

- one primary outcome per scope
- one coherent validation story per scope
- DoD items small enough to validate individually

Split scopes when they mix unrelated journeys, unrelated validation paths, or more than one independent outcome.

## Micro-Fix Containment

When a failure is narrow, the repair loop must stay narrow:

1. start with the smallest failing unit
2. fix the exact failure
3. rerun the narrowest relevant check first
4. widen only after the local fix is proven

## Live-Stack Authenticity

Tests labeled `integration`, `e2e-api`, or `e2e-ui` must use the real stack.

If a test uses interception or canned backend behavior, it is mocked and must be reclassified. A mocked test cannot satisfy live-stack DoD items.

## Execution Depth Requirement

Handlers, routes, endpoints, or workflow steps are incomplete if they only return shaped placeholder success.

Real implementation requires real delegation, query, command, or upstream execution. Literal placeholder payloads are blocking implementation-reality failures.

## State Claim Integrity

`state.json` is derived state, never aspirational state.

- `status` must match artifact reality
- `certification.completedScopes` must match actual done scopes
- `execution.completedPhaseClaims` and `certification.certifiedCompletedPhases` must not get ahead of actual completed work
- `policySnapshot` must record the effective grill, TDD, lockdown, regression, and validation settings with source provenance
- `transitionRequests` and `reworkQueue` must be closed before validate certifies completion
- `scenario-manifest.json` must cover stable `SCN-*` IDs for changed user-visible behavior

If state is stale, lower it immediately.

## Validate-Owned Certification

Only `bubbles.validate` may certify completion state. Other agents submit execution claims and transition requests.

- Only `bubbles.validate` may write `certification.status`, `certification.completedScopes`, `certification.certifiedCompletedPhases`, `certification.scopeProgress`, or `certification.lockdownState`
- Valid `certification.status` values: `not_started`, `in_progress`, `done`, `done_with_concerns`, `blocked`
- `done_with_concerns` carries all done-equivalent privileges but includes a `certification.concerns` array
- Other agents may write `execution.*` fields (current phase, active agent, execution claims)
- The top-level `status` field mirrors `certification.status` and must not contradict it

### Concerns Schema

```json
"certification": {
  "status": "done_with_concerns",
  "concerns": [
    {
      "scope": "02-api-handlers",
      "description": "Stress test p99 is 48ms, just under 50ms SLA. Monitor in production.",
      "severity": "low",
      "agent": "bubbles.test"
    }
  ]
}
```

Severity levels: `low` (informational), `medium` (warrants monitoring), `high` (close to gate failure threshold).

## Related Modules

- [evidence-rules.md](evidence-rules.md) — evidence format, attribution, and anti-fabrication requirements
- [artifact-ownership.md](artifact-ownership.md) — which agents own which artifacts
- [state-gates.md](state-gates.md) — mechanical gate definitions and script enforcement
- [quality-gates.md](quality-gates.md) — test taxonomy, evidence standard, and anti-fabrication rules
- Agents MUST NOT self-promote to `done`; they route the promotion request through validate