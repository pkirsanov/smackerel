# Analytical Rigor

Shared contract for the QUALITY of analysis, review, diagnosis, and design output. Load this whenever the agent's primary deliverable is judgment-bearing narrative — findings, requirements, a design, a review, a diagnosis, a root cause, or a proposal — rather than executed code.

This module is the analytical-deliverable counterpart to the E2E Test Substance gate: the presence of a section is not substance. It composes with — and never weakens — the Honesty Incentive in [critical-requirements.md](critical-requirements.md) and the evidence/uncertainty rules in [evidence-rules.md](evidence-rules.md). Anti-fabrication governs whether a *completion/test claim* is real; this module governs whether the *analysis itself* is deep, grounded, and honest.

## The Four Rigor Rules (non-negotiable)

1. **Deep, not surface.** Reason from the actual system and domain to a real conclusion. Trace the concrete path — files, routes, models, data flows, contracts, prior artifacts — before asserting a capability, gap, risk, or requirement. Restating the request, summarizing what the code obviously is, or listing generic best practices is not analysis.

2. **Grounded, not generic.** Every actor, capability, gap, risk, requirement, finding, and proposal MUST cite concrete evidence: a real file/route/model/symbol, an observed competitor behavior, a named domain constraint, a measured value, or a specific prior artifact. Output that could be pasted unchanged into a different feature — or a different project — is canned filler and fails this rule.

3. **Honest findings first.** Surface the uncomfortable results — contradictions, unproven assumptions, missing scenarios, weak or untestable requirements, silent design conflicts, hidden coupling — as the primary content, not an appendix. A tidy report that rubber-stamps a flawed input is a FAILED analysis, not a passing one. "Looks good" with nothing named at stake is a red flag: state explicitly what you checked and what would have made you reject it.

4. **No canned template-filling.** A section template is a checklist of what to consider, not a form to populate with plausible text. If a section has no real content for this target, write `None found — <specific reason>`, never invented boilerplate. Declare uncertainty (see [evidence-rules.md](evidence-rules.md) → Uncertainty Declaration Protocol) instead of confident filler.

## Depth Dial

Honor an optional `depth:` parameter when the caller supplies one:

- `depth: standard` (default) — full grounded pass at normal breadth; the Four Rigor Rules always apply.
- `depth: deep` — maximize scrutiny: chase second-order effects, edge cases, cross-artifact conflicts, and the assumptions the request itself is making. Equivalent to the caller hand-writing "go deeper, challenge the premise."

Callers should never need to type "do deep analysis / no canned responses / be genuine / give an honest report" — those behaviors are this module's DEFAULT, not an opt-in. The dial only raises the ceiling; it never lowers the floor.

## Rigor Self-Check (Tier 2 — run before reporting)

1. Did I trace at least one concrete, named piece of evidence for every material claim?
2. Would this output be false or awkward if pasted into a different feature/project? If not, it is too generic — reground it.
3. Did I surface the real weaknesses, or did I produce a clean bill of health by avoiding the hard questions?
4. Did I write `None found — <reason>` where a section had no real content, instead of filler?
5. If uncertain, did I declare the uncertainty rather than assert a confident guess?

If any answer is "no," strengthen the analysis before reporting. A shallow-but-complete-looking report is a failure mode, not a pass.
