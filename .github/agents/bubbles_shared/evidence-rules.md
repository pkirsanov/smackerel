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
- **Empty-output evidence:** When a tool prints nothing on success, capture `$?` explicitly (`bash <tool>; echo "exit=$?"`). Never record "no output" as the only evidence — always pair with exit code.
- **Windowed reads for evidence:** When a script's output is >100 lines, capture only the relevant 10-30 line window for evidence (with line-number annotations: "lines 42-58 of full output"). Full output is preserved by the script's own logging if needed for retro analysis.
- **Bounded capture is the DEFAULT evidence shape (>40 lines).** For any command whose output exceeds 40 lines, the default evidence is the block produced by `bubbles/scripts/evidence-capture.sh`, not a pasted transcript. The block records the command, exit code, line count, a sha256 over every line produced, the failure-shaped lines, and the first and last 20 lines. It satisfies the ≥10-line raw-output requirement and is STRONGER against fabrication than a paste, because `--verify` re-derives the hash against a re-run and a paste can never be checked. Below 40 lines, paste the output as it came back. The rule that evidence must come from real execution in the current session is unchanged and is not relaxed by bounding the retained bytes.

## Evidence by Reference (avoid double-paste)

The same ≥10-line raw terminal output MUST NOT be carried twice — once inlined under a `[x]` DoD item in `scopes.md` and again in `report.md`. When the raw output already lives in `report.md` (or in the structured tool-call log), a DoD item MAY **reference** that evidence instead of re-pasting it. The transition guard (`state-transition-guard.sh` Check 9) resolves the reference and treats a resolved reference as **equivalent to an inline ≥10-line evidence block**.

Reference is **opt-in and default-preserving**: an inline ≥10-line evidence block placed directly under a `[x]` item (the format in "Evidence Attribution" above) remains fully valid. The `**Claim Source:**` tag (`executed | interpreted | not-run`) is still required on the referenced evidence — a reference changes *where* the raw output lives, never *whether* it must be real executed output.

There are exactly TWO supported reference forms. Do NOT invent an `evidence://` URI or any other scheme — these two shapes are the only references the guard resolves.

### 1. Markdown `report.md#anchor` reference

Shape the DoD item so the `Evidence:` marker links to a `report.md` anchor:

```
- [x] <item description> → Evidence: [<anchor>](report.md#<anchor>)
```

The guard follows the link to the `report.md` anchor — an HTML `<a name="…">`, an explicit `{#anchor}` attribute, or a GitHub-slugified heading — and counts the non-blank lines from that anchor heading to the next heading (or end of file). The item is satisfied only when that block has **≥10 non-blank lines**. A plain `report.md` link with no `#anchor` (e.g. `[report.md](report.md)`) is accepted only when `report.md` exists with ≥10 non-blank lines total.

**Fail-closed:** an unresolvable anchor, or a resolved block with fewer than 10 non-blank lines, does NOT satisfy the item — the guard FAILS that DoD line (it is never a silent pass). Keep the referenced `report.md` block as raw executed output with its `**Claim Source:**` tag.

### 2. Structured tool-log reference (`record_evidence`)

Wrap the gate-relevant command with the `record_evidence` MCP tool. It runs the command and appends a structured entry (argv, `exitCode`, stdout/stderr hashes, spec slug) to `.specify/runtime/tool-calls.jsonl`. The guard then covers the matching DoD item automatically: it accepts the item when a log entry (a) names this spec, (b) has `exitCode == 0`, and (c) whose recorded command shares ≥2 distinct alpha-tokens with the DoD item body (common words like `the`/`test`/`docs` do not count). No inline paste is needed for that item.

**Fail-closed:** a missing log, a schema-invalid or spec-less log line, a non-zero exit, or a command that does not token-match the DoD body does NOT cover the item — the guard falls through and FAILS that DoD line. The structured log entry is the evidence of record; the `**Claim Source:**` convention still applies to any human-readable evidence kept alongside it.

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

- [claim-grounding.md](claim-grounding.md) — whether the underlying FACT was verified at all (this module governs whether the COMMAND ran)
- [artifact-ownership.md](artifact-ownership.md) — who may write to which artifacts (evidence blocks follow the same ownership)
- [completion-governance.md](completion-governance.md) — what "complete" means and what deferral language blocks it
- [state-gates.md](state-gates.md) — mechanical gate definitions including G040 (incomplete work language) and G066 (phase-claim provenance)
