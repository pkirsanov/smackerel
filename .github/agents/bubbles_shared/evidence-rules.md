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

## Evidence Attribution (NON-NEGOTIABLE)

Each evidence block recorded under a DoD item in `scopes.md` MUST include a `**Phase:**` tag identifying which specialist phase produced the evidence. This enables mechanical cross-referencing between evidence provenance and `completedPhaseClaims`.

**Required format inside evidence blocks:**
```
**Phase:** <phase-name>
**Command:** <exact command executed>
**Exit Code:** <actual exit code>
<raw output, ≥10 lines>
```

**Ownership rule:** An agent may only write evidence under DoD items that belong to its phase ownership. For example:
- `bubbles.implement` may write evidence tagged `**Phase:** implement`
- `bubbles.test` may write evidence tagged `**Phase:** test`
- `bubbles.validate` may write evidence tagged `**Phase:** validate`

An agent MUST NOT write evidence tagged with another agent's phase name. Cross-phase evidence writing is fabrication.

## Related Modules

- [artifact-ownership.md](artifact-ownership.md) — who may write to which artifacts (evidence blocks follow the same ownership)
- [completion-governance.md](completion-governance.md) — what "complete" means and what deferral language blocks it
- [state-gates.md](state-gates.md) — mechanical gate definitions including G040 (incomplete work language) and G066 (phase-claim provenance)
