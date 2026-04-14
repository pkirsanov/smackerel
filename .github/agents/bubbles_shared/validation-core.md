# Validation Core

All agent completion checks follow a shared two-tier model.

## Tier 1 — Universal Completion Checks

Every agent MUST pass ALL of the following before reporting a completion or diagnostic result:

1. **Artifact lint** — run `artifact-lint.sh` against the target spec/bug folder; must exit 0.
2. **State-transition guard** — run `state-transition-guard.sh` against the target spec/bug folder when transitioning toward `done`; must exit 0.
3. **No unresolved continuation language** — report.md, scopes.md, and scope directories must not contain unresolved pseudo-completion text: `Next Steps` (as heading or bullet leader), `Recommended routing:`, `Recommended resolution:`, `Ready for /bubbles.`, `Re-run /bubbles.validate`, `Commit the fix`, `Record DoD evidence`, `Run full E2E suite`, `[PENDING`, `header only initially`.
4. **No deferral language** — scope artifacts must not contain any phrases from the deferral-language list in `completion-governance.md`.
5. **RESULT-ENVELOPE emitted** — state-modifying and diagnostic agents must end their response with a `## RESULT-ENVELOPE` containing a concrete outcome (`completed_owned`, `completed_diagnostic`, `route_required`, or `blocked`). Read-only agents (status, recap, handoff) are exempt, but if they emit continuation guidance they should use a `## CONTINUATION-ENVELOPE` and recommend workflow commands rather than raw specialist commands.
6. **Evidence provenance tags present** — every evidence block in scope/report artifacts must include a `**Claim Source:**` tag. All `interpreted` blocks must include an `**Interpretation:**` line. Missing tags are treated as `interpreted` and flagged as lint failures.
7. **Uncertainty Declarations present for unchecked items** — any DoD item left `[ ]` after agent work must have an Uncertainty Declaration explaining what was attempted and what would resolve it. Unchecked items without explanation are incomplete handoffs.

If ANY Tier 1 check fails, the agent must fix the issue or report failure — not claim completion.

## Tier 2 — Role-Specific Checks

Each agent must also satisfy its role-specific validation profile from `validation-profiles.md` before claiming completion.

## Rules

1. Validation claims require actual executed evidence.
2. Agent-specific checks are additive, not optional.
3. If any Tier 1 or Tier 2 check fails, the agent must report failure and must not claim completion.
4. Prompts should reference the matching profile instead of embedding duplicate tables.
5. **Execution means terminal execution (Gate G071).** Running `artifact-lint.sh`, `traceability-guard.sh`, test commands, or any validation script means invoking it via `run_in_terminal` and recording the real output. Reading the files those scripts would check and predicting findings is analysis-as-execution fabrication — see `evidence-rules.md`. If a command cannot be executed, report it as NOT RUN.