# Evidence Rules

Purpose: canonical source for execution evidence and anti-fabrication requirements.

## Rules
- Pass/fail claims require actual command execution.
- Evidence must be raw terminal output, not narrative summaries.
- Required test or validation evidence must contain enough raw output to show real execution signals.
- Evidence blocks must map to actual tool executions from the current session.
- Fabricated, copied, or template evidence blocks invalidate completion claims.
- Evidence sections must not contain unresolved continuation or follow-up language (`Next Steps`, `Recommended routing`, `Re-run /bubbles.*`, `Commit the fix`, `Record DoD evidence`, `Run full E2E suite`). If any of these phrases appear outside quoted historical evidence, the evidence section is incomplete.
- All state-modifying and diagnostic agents must conclude with a structured `## RESULT-ENVELOPE` outcome (`completed_owned`, `completed_diagnostic`, `route_required`, or `blocked`). Narrative-only conclusions without a structured envelope are equivalent to fabrication for completion-tracking purposes.

## Analysis-As-Execution Is Fabrication (NON-NEGOTIABLE — Gate G071)

Reading source files, artifact files, or code that a command would inspect and predicting what the command would output is **fabrication**, regardless of whether the prediction is accurate. This applies to all agents, and especially to validation and audit agents invoking lint, guard, or test scripts.

The distinction:
- **Execution:** Agent runs `bash artifact-lint.sh specs/042-feature` in a terminal. The script applies its canonical logic. The terminal output is the evidence.
- **Analysis-as-execution (FABRICATED):** Agent reads `spec.md`, `scopes.md`, `state.json` manually, pattern-matches against known lint rules, and reports predicted findings as if the script ran. No terminal command was executed.

Why accurate predictions are still fabrication:
- The canonical script may contain logic the agent cannot replicate (version checks, cross-file correlations, stateful path resolution).
- The agent's pattern matching may miss or hallucinate issues the real script wouldn't.
- The real script IS the source of truth — any other method is a proxy with unknown fidelity.

If a command cannot be executed (tool unavailable, timeout, environment issue), the correct response is to report it as NOT RUN — never to substitute manual file analysis as a fallback.

## Evidence Attribution (NON-NEGOTIABLE)

Each evidence block recorded under a DoD item in `scopes.md` MUST include a `**Phase:**` tag identifying which specialist phase produced the evidence. This enables mechanical cross-referencing between evidence provenance and `completedPhaseClaims`.

**Required format inside evidence blocks:**
```
**Phase:** <phase-name>
**Command:** <exact command executed>
**Exit Code:** <actual exit code>
**Claim Source:** <executed | interpreted | not-run>
<raw output, ≥10 lines>
```

**Ownership rule:** An agent may only write evidence under DoD items that belong to its phase ownership. For example:
- `bubbles.implement` may write evidence tagged `**Phase:** implement`
- `bubbles.test` may write evidence tagged `**Phase:** test`
- `bubbles.validate` may write evidence tagged `**Phase:** validate`

An agent MUST NOT write evidence tagged with another agent's phase name. Cross-phase evidence writing is fabrication.

## Evidence Provenance Taxonomy (NON-NEGOTIABLE)

Every evidence block MUST include a `**Claim Source:**` tag that classifies how the DoD claim is supported by the evidence. This enables fast review — auditors and users can skim for `interpreted` and `not-run` blocks instead of reviewing everything.

| Claim Source | Meaning | Gate Treatment | Review Priority |
|-------------|---------|----------------|-----------------|
| `executed` | Command output **directly and unambiguously** proves the DoD claim. No interpretation needed — the raw output contains the exact verification signal (e.g., test name + PASS, exit code 0, expected string present). | Accepted | Low (spot-check only) |
| `interpreted` | Command executed and output captured, but the DoD conclusion **requires interpretation** of the output. The raw output does not contain a single unambiguous verification signal — the agent had to reason about what the output means. | Flagged for review — `bubbles.audit` MUST verify the interpretation is correct | Medium |
| `not-run` | No command was executed that proves this claim. The agent was unable to run the verification (tool unavailable, timeout, environment issue, or no known command that directly proves the claim). | BLOCKED — item MUST stay `[ ]` with an Uncertainty Declaration | High |

**Rules:**
- `executed` is the only claim source that permits marking `[x]` without further review.
- `interpreted` permits marking `[x]` but the evidence block MUST include an `**Interpretation:**` line explaining what the agent concluded and why. `bubbles.audit` MUST verify every `interpreted` block.
- `not-run` MUST NOT be used to mark `[x]`. The item stays `[ ]` with an Uncertainty Declaration explaining what was attempted.
- If an agent is unsure whether its evidence is `executed` or `interpreted`, it MUST use `interpreted`. When in doubt, label conservatively — a wrong `executed` label is a provenance fabrication.

**Examples:**

```markdown
# executed — output directly proves the claim
**Phase:** test
**Command:** [test-all]
**Exit Code:** 0
**Claim Source:** executed
ok  	myproject/internal/api/handlers	12.450s
ok  	myproject/internal/storage/postgres	8.230s
PASS
...
```

```markdown
# interpreted — output requires reasoning to connect to DoD claim
**Phase:** test
**Command:** [test-all]
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** DoD claims "webhook retries exactly 3 times." Test `TestWebhookRetry` passed,
but the assertion only checks HTTP 200 response — it does not explicitly assert retry count.
Marking [x] based on test design intent, but retry count assertion should be added for confidence.
ok  	myproject/internal/api/handlers	12.450s
...
```

```markdown
# not-run — could not execute verification
**Phase:** test
**Claim Source:** not-run
**Reason:** Integration test requires running Docker stack which timed out during this session.
Attempted [test-integration] but received timeout after 300s.
```

## Uncertainty Declaration Protocol (For Unchecked DoD Items)

When a DoD item cannot be verified — either because evidence is ambiguous, execution failed, or no suitable verification command exists — the agent MUST leave the item `[ ]` and attach an **Uncertainty Declaration** instead of guessing or fabricating.

An Uncertainty Declaration is a **positive signal**, not a failure. It gives the next agent (or the user) an actionable path forward. Per the Honesty Incentive in `critical-requirements.md`: a wrong answer is 3x worse than a blank answer.

**Required format for unchecked items with uncertainty:**

```markdown
- [ ] [DoD item description]
  > **Uncertainty Declaration**
  > **What was attempted:** [exact command(s) run, or "no suitable command identified"]
  > **What was observed:** [actual output or "command not executed — reason"]
  > **Why this is uncertain:** [specific explanation of what is ambiguous or unverifiable]
  > **What would resolve this:** [concrete next step — a specific command, test, or manual check]
```

**Rules:**
- Uncertainty Declarations are REQUIRED when an item stays `[ ]` after an agent has worked on it. Leaving an item unchecked with no explanation is an incomplete handoff.
- Uncertainty Declarations MUST be specific and actionable. "Could not verify" without explanation is not acceptable.
- Uncertainty Declarations do not block scope progress — other DoD items can still be completed. But the scope cannot be `Done` until all items are either `[x]` with evidence or resolved.
- `bubbles.audit` MUST review all Uncertainty Declarations and either resolve them (by executing the suggested verification) or confirm they are genuine blockers.

**Examples:**

```markdown
- [ ] Integration tests pass for webhook retry logic
  > **Uncertainty Declaration**
  > **What was attempted:** [test-integration] (exit 0, see evidence block #4)
  > **What was observed:** Test `TestWebhookRetry` passed but assertion only checks HTTP status, not retry count
  > **Why this is uncertain:** DoD requires "retries exactly 3 times" but no test assertion explicitly counts retries
  > **What would resolve this:** Add assertion in `TestWebhookRetry` that checks retry counter equals 3, or add a dedicated `TestWebhookRetryCount` test

- [ ] Stress test proves p99 < 50ms under 10x load
  > **Uncertainty Declaration**
  > **What was attempted:** [test-stress] — command timed out after 600s
  > **What was observed:** Partial output showed 42 of 100 test iterations completed before timeout
  > **Why this is uncertain:** Incomplete run — cannot claim p99 from partial data
  > **What would resolve this:** Re-run with increased timeout or reduce iteration count for a complete run
```

## Related Modules

- [artifact-ownership.md](artifact-ownership.md) — who may write to which artifacts (evidence blocks follow the same ownership)
- [completion-governance.md](completion-governance.md) — what "complete" means and what deferral language blocks it
- [state-gates.md](state-gates.md) — mechanical gate definitions including G040 (incomplete work language) and G066 (phase-claim provenance)
