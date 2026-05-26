---
name: bubbles-anti-fabrication
description: Enforce the Bubbles Anti-Fabrication Policy. Use when about to claim work is done, mark a DoD item complete, write a report.md evidence section, set state.json status, or assert that tests/commands passed. Triggers include marking [x], writing "Exit Code: 0", claiming "all tests pass", recording evidence, or transitioning a scope or spec to a terminal status.
---

# Bubbles Anti-Fabrication

## Goal
Never claim work happened that did not actually execute in the current session. Evidence must come from real tool/terminal output, not from inference, expectation, or summary.

## When to use
- Before marking ANY DoD checkbox `[x]`
- Before writing evidence into `report.md`
- Before transitioning a scope to `Done` or a spec to `done`
- Before asserting a build, test, lint, deploy, query, or curl succeeded
- Before composing a result envelope claiming `completed_owned`

## Non-negotiable rules
1. **Execute first, record second.** Run the command in this session, observe the output, then paste the actual output. Never write evidence before the command ran.
2. **Raw terminal output, ≥10 lines.** Evidence sections shorter than ~10 lines are presumed to be summaries and are rejected. Copy/paste the literal terminal text, including the command line and the exit code.
3. **Session-bound proof.** Evidence from a prior session, a prior tool call earlier in the same task that was not re-executed for the current claim, or evidence reused verbatim across multiple DoD items is fabrication.
4. **No narrative substitutes.** Strings like "all tests pass", "verified", "endpoint works", "UI looks correct" are not evidence. Show the command and output.
5. **No template stubs.** Placeholder text like `[ACTUAL output, ≥10 lines]` left in place is fabrication.
6. **No batch checking.** Marking many DoD items `[x]` in one edit without per-item execution is fabrication regardless of how confident the agent is.
7. **No fabricated specialists.** Claiming `bubbles.audit`, `bubbles.chaos`, `bubbles.validate`, or any specialist agent ran without an actual sub-agent invocation in this session is fabrication.

## Self-check (run before every status transition)
1. Did I actually run a tool/terminal command for this DoD item in this session?
2. Did I see the output in a terminal or tool response?
3. Am I about to paste the actual output, not what I expected to see?
4. Is the evidence ≥10 lines of raw output?

If any answer is "no", do not mark the item `[x]`. Execute first.

## Authoritative governance modules
This skill is a discovery shim. The full enforceable policy lives in:
- `agents/bubbles_shared/critical-requirements.md` — Honesty Incentive, hard non-negotiables
- `agents/bubbles_shared/evidence-rules.md` — Evidence Provenance Taxonomy, ≥10-line rule, Uncertainty Declaration Protocol
- `agents/bubbles_shared/completion-governance.md` — sequential completion, deferral blocking, red/green traceability
- `bubbles/scripts/state-transition-guard.sh` — mechanical enforcement that runs in pre-push

Read the module that matches the specific question. Do not rewrite these rules inside agent prompts.

## Mechanical detection
The framework auto-detects fabrication via:
- `state-transition-guard.sh` Checks 2B, 3G, 5B–5C, 7A–7B
- `artifact-lint.sh` (template + section presence)
- `done-spec-audit.sh` (recertification-only; advisory by default — see `bubbles-status-transition` skill for grandfather rules)

Bypassing these scripts or asking for `--force` flags is itself a policy violation. They do not exist.
