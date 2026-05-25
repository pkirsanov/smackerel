# Completion Governance

Use this file for the authoritative completion chain and completion-state rules.

## Absolute Completion Hierarchy

Completion flows bottom-up only:

1. A DoD item is implemented.
2. The item is validated with a real command or tool run.
3. Raw evidence is recorded inline with a `**Claim Source:**` provenance tag (see evidence-rules.md).
4. Only then may the item be marked `[x]`.
5. A scope is `Done` only when every DoD item is `[x]` with real evidence.
6. A spec is `done` only when every scope is `Done`.

If any link in that chain is false, completion state must be lowered immediately.

## Per-DoD Validation Rules

- Each DoD item requires its own validation and its own evidence.
- Batch-checking multiple items from one generic run is invalid.
- Evidence must be current-session raw output with recognizable terminal or tool signals.
- Evidence must include a `**Claim Source:**` tag (`executed`, `interpreted`, or `not-run`). See evidence-rules.md for the Evidence Provenance Taxonomy.
- Only `executed` evidence permits `[x]` without further review. `interpreted` evidence permits `[x]` but requires audit verification.
- `not-run` evidence MUST NOT be used to mark `[x]` — the item stays `[ ]` with an Uncertainty Declaration.
- If execution fails, the item remains `[ ]` until fixed and re-run.
- If an item cannot be verified, it MUST remain `[ ]` with an Uncertainty Declaration (see evidence-rules.md). An honest gap is preferred over fabricated completion (see Honesty Incentive in critical-requirements.md).

## Scope Completion Rules

A scope cannot be `Done` when any of these are true:

- any DoD item is unchecked (unless it has an approved Uncertainty Declaration that was resolved by audit)
- any checked DoD item lacks inline evidence
- required test types have not run
- evidence shows failure
- the scope contains deferral language
- the scope claims behavior that the tests do not prove

### Done With Observations

A scope may be marked `Done` with an `observations[]` collection when ALL DoD items pass with evidence AND ALL gates pass, but the agent identified concrete non-blocking notes worth recording. This shape:

- Does NOT bypass any gate — every gate must still pass identically to plain `Done`
- Does NOT introduce a third terminal status — the status remains `Done`
- MUST include observations following the schema below (id, severity, summary, followUpOwner, followUpAction)
- Is subject to `bubbles.validate` review — validate may downgrade to `In Progress` or `Blocked` if an observation is actually a gate failure in disguise

Legacy read-only `done_with_concerns` is documented in [Legacy Status: done_with_concerns](#legacy-status-done_with_concerns). It is compatibility-only for old specs and is not a valid new outcome.

## Spec Completion Rules

A spec cannot be `done` when any scope is:

- `Not Started`
- `In Progress`
- `Blocked`

`state.json` must reflect the lower truth immediately when artifacts and status disagree.

Implementation-bearing specs also cannot be `done` when report artifacts lack code-diff evidence showing real non-artifact runtime, config, contract, or source files in the delivery delta. For done-ceiling delivery modes, Gate G093 adds the status-level path check: the certification window must include at least one implementation/runtime/config/contract/test/docs path outside `specs/` and `.specify/`. If the changed-path classification is planning-only, the result is `blocked` with finding `G093`, next owner `bubbles.implement` / `bubbles.test` / `bubbles.docs` as appropriate, or a below-done planning-only downgrade governed by G087.

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

## Finding-Set Closure Is Mandatory

When a workflow round, audit, harden pass, stabilize pass, security review, gap scan, or validation step discovers multiple findings, completion requires one-to-one closure accounting for the entire finding set.

- Every finding must end in exactly one of these states: fixed and revalidated, routed to the correct owner with the unresolved finding preserved verbatim, or blocked with a concrete blocker.
- Fixing only the easy subset while narrating the remaining findings as larger, later, separate, or follow-up work is invalid.
- The workflow and implement agents must reject responses that claim success without enumerating addressed versus unresolved findings.

Invalid example:

- "The timing attack is fixable now. The JWT migration is a larger change. Let me fix the timing attack."

That pattern is incomplete work. The correct outcomes are either: fix both findings now, or return `route_required` / `blocked` with the full unresolved list.

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
- Valid new `certification.status` values: `not_started`, `in_progress`, `done`, `blocked`
- Legacy read-only `done_with_concerns` may be read only when `legacyStatusCompatibility:true` is present; new writes and recertification MUST migrate to `done` plus observations or `blocked`
- Other agents may write `execution.*` fields (current phase, active agent, execution claims)
- The top-level `status` field mirrors `certification.status` and must not contradict it

### Observations Schema (state.json certification)

```json
"certification": {
  "status": "done",
  "observations": [
    {
      "id": "OBS-1",
      "scope": "02-api-handlers",
      "summary": "Stress test p99 is 48ms, just under 50ms SLA. Monitor in production.",
      "severity": "low",
      "followUpOwner": "human",
      "followUpAction": "next-sprint-todo",
      "agent": "bubbles.test"
    }
  ]
}
```

Severity levels: `low` (informational, accept-and-note), `medium` (warrants tracked follow-up). `high` is **NOT** a permitted severity for observations attached to `done` — anything that would warrant `high` is a real gate failure and MUST use `blocked` instead.

## Legacy Status: done_with_concerns

### What it is

`done_with_concerns` is a legacy read-only compatibility status. It may appear in old specs that were certified before Gate G092 and explicitly carry `legacyStatusCompatibility:true`. It is not a valid new RESULT-ENVELOPE outcome, top-level status, certification status, workflow outcome state, or dependency-stability state for newly certified or recertified work.

### New terminal status contract

| Situation | Outcome |
|----------|--------|
| All DoD items checked with real evidence, all gates pass, zero notes | `done` |
| All DoD items checked with real evidence, all gates pass, AND one or more non-blocking notes worth tracking | `done` with `observations[]` |
| Any required gate fails, any DoD item lacks evidence, any HIGH-severity risk discovered, scope contains deferral language | `blocked` |
| Required work was deferred to "later" or "a follow-up ticket" instead of done now | `blocked` (deferral, not an observation — see G040) |

### RESULT-ENVELOPE observations array

When an agent emits `outcome: completed_owned` or validate certifies `done` with non-blocking notes, the RESULT-ENVELOPE may include an `observations: []` array. Each entry MUST follow this shape:

```yaml
outcome: completed_owned
observations:
  - id: OBS-1
    severity: low | medium
    summary: >
      Stress test p99 is 48ms, just under the 50ms SLA threshold. Recommend
      production monitoring before adding more load.
    followUpOwner: <agent-name> | human
    followUpAction: new-spec | issue-doc | next-sprint-todo | accept
```

Field rules:

- **id** — short stable identifier (e.g., `OBS-1`, `OBS-2`); MUST be unique within the envelope.
- **severity** — only `low` or `medium` are permitted. See severity rules below.
- **summary** — 1-2 sentences. Concrete and observable. Not "might be slow" — "p99 was 48ms during stress run".
- **followUpOwner** — a concrete owner: a Bubbles agent name (e.g., `bubbles.test`, `bubbles.devops`, `bubbles.implement`) or `human`. NEVER `everyone` or `tbd`.
- **followUpAction** — exactly one of:
  - `new-spec` — a new feature spec MUST be opened to address this
  - `issue-doc` — a tracked issue document MUST be created in `docs/issues/`
  - `next-sprint-todo` — record in the next sprint backlog (no immediate action)
  - `accept` — acknowledged, no follow-up planned (use sparingly; HIGH-trust acceptance only)

### Severity rules (NON-NEGOTIABLE)

| Severity | Meaning | Permitted with `done` observations? |
|---------|--------|--------------------------------------|
| `low` | Informational; observed during this work; no production impact expected | yes |
| `medium` | Warrants tracked follow-up but does not block shipping the current scope | yes |
| `high` | Close to or actually a gate failure; ships unsafe behavior or violates a required gate | NO — use `blocked` instead |

If you find yourself wanting to write `severity: high`, the outcome is `blocked`, not `done` with observations. Observations make honest notes auditable; they do not launder gate failures.

### Anti-fabrication tie-in (NON-NEGOTIABLE)

Observations are **NOT** a deferral mechanism. Agents MUST NOT use them to dodge required work. Observations are for genuinely non-blocking notes discovered during the in-scope work — not for deferred-required-work that the agent simply did not finish.

The following patterns are FORBIDDEN and constitute fabrication (Gate G021 + G040):

| Forbidden pattern | Why it's wrong | Correct outcome |
|------------------|---------------|----------------|
| "Skipped one DoD item, recording as observation" | A DoD item without evidence = `[ ]`, scope stays In Progress | `blocked` or finish the item |
| "E2E test was flaky, accepting as low observation" | Flaky test is a real gate failure | `blocked` until test is stable |
| "Will address error handling in follow-up — observation severity medium" | Deferral language; G040 violation | `blocked` until handled now |
| "Found unrelated bug, marking as medium observation with followUpAction: accept" | Unrelated bug needs `new-spec` or `issue-doc`, not silent acceptance | `done` with the right observation `followUpAction` |
| "Code coverage is 87%, recording as low observation" | If 100% is required by the spec, this is a gate failure | `blocked` until coverage >= required |

The validate agent (`bubbles.validate`) MUST inspect every observation and downgrade to `blocked` if any observation is actually a gate failure in disguise.

### Legacy read-only compatibility

Old specs may retain top-level `status: done_with_concerns` and/or `certification.status: done_with_concerns` only when both conditions hold:

- `legacyStatusCompatibility:true` is present at top level or under `certification`
- the spec is not being touched, recertified, or revalidated in the current run

On recertification, migrate legacy concerns into `observations[]` and write one of the valid new terminal statuses:

- all gates pass: `done` plus observations
- required work remains: `blocked`

New `done_with_concerns` writes are FORBIDDEN by Gate G092.

### Worked examples

**Example 1 — discovered an unrelated flaky test outside scope**
```yaml
outcome: completed_owned
observations:
  - id: OBS-1
    severity: medium
    summary: >
      During regression run, observed a flaky test in
      `services/foo/test_unrelated_thing.rs::test_timeout_path` (unrelated to
      this scope). Failed once across 5 runs.
    followUpOwner: bubbles.bug
    followUpAction: new-spec
```

**Example 2 — lint warning unrelated to changed files**
```yaml
outcome: completed_owned
observations:
  - id: OBS-1
    severity: low
    summary: >
      `cargo clippy` flagged 1 warning in `services/legacy/old_module.rs`
      (untouched by this scope). Recommend cleanup in next hygiene sweep.
    followUpOwner: human
    followUpAction: issue-doc
```

**Example 3 — feature shipped, audit found observation worth tracking**
```yaml
outcome: completed_owned
observations:
  - id: OBS-1
    severity: medium
    summary: >
      Audit confirmed all DoD items met. Spotted that the new endpoint
      logs request body without redacting potential PII fields. Not in
      this scope's threat model but worth tightening.
    followUpOwner: bubbles.security
    followUpAction: new-spec
```

**Example 4 — third-party latency observed during stress test**
```yaml
outcome: completed_owned
observations:
  - id: OBS-1
    severity: low
    summary: >
      Stress run observed external upstream API p95 latency of 380ms
      (outside our SLA, our service met internal targets). May affect
      real-world end-to-end perception.
    followUpOwner: bubbles.devops
    followUpAction: next-sprint-todo
```

### Cross-reference

The schema, severity rules, and follow-up-action vocabulary above MUST stay in sync with [`bubbles/workflows.yaml#outcome-states`](../../bubbles/workflows.yaml). When updating one, update the other in the same change.

### Mechanical Enforcement Note (Gate G040 / G092)

`state-transition-guard.sh` Check 18 (Deferral Language Scan) understands observations and legacy read-only `done_with_concerns` compatibility. Specifically:

1. **Legacy status-conditional skip:** When `state.json.status == "done_with_concerns"` and `legacyStatusCompatibility:true` is present, Check 18 emits an INFO line and skips entirely for read-only compatibility. New `done_with_concerns` writes are blocked by G092.
2. **Schema field exclusion:** When `status == "done"`, Check 18 still runs but excludes lines containing the schema-canonical follow-up field names: `followUpOwner`, `followUpAction`, `followUpTarget`, `followUps` (case-insensitive). These are the structured tracking mechanism, not deferred-work prose.
3. **Section heading exclusion:** The canonical section heading `## Follow-Up Narrative` (and `## Follow-Up Section`) is excluded from the scan. The container heading itself is schema-allowed.
4. **Sentinel markers:** Authors may wrap quoted historical material in `<!-- bubbles:g040-skip-begin -->` ... `<!-- bubbles:g040-skip-end -->` to exempt it from the scan. Marker lines themselves are stripped before scanning. This complements the existing code-fence exemption.

These exemptions exist because structured observation schemas require authors to write tokens that the deferral pattern could otherwise catch. The exclusions are intentional and narrow — raw deferred-work prose under `status == "done"` is still blocked.

## Related Modules

- [evidence-rules.md](evidence-rules.md) — evidence format, attribution, and anti-fabrication requirements
- [artifact-ownership.md](artifact-ownership.md) — which agents own which artifacts
- [state-gates.md](state-gates.md) — mechanical gate definitions and script enforcement
- [quality-gates.md](quality-gates.md) — test taxonomy, evidence standard, and anti-fabrication rules
- Agents MUST NOT self-promote to `done`; they route the promotion request through validate