# <img src="../../icons/bubbles-glasses.svg" width="28"> Effective Prompting

> **Audience:** operator (humans driving Bubbles through prompts and the CLI).
>
> Bubbles ships rich *agent* prompts, but the quality of what comes back still
> depends on how *you* frame the request. This guide is a short, durable
> reference for writing requests that Bubbles can act on confidently — without
> turning your ask into a brittle step-by-step script.

This guide is **guidance, not a gate.** Nothing here blocks work or changes a
status transition; it just helps you get a better first result. It is built to
**complement** Bubbles' autonomous orchestration, not to replace it with manual
runbooks (see [Intent over runbook](#bubbles-specific-intent-over-runbook)).

---

## The good-request checklist

A strong request usually has most of these. You do **not** need all five every
time — reach for the ones that remove ambiguity for *this* ask.

1. **State the goal / outcome first.** Lead with the result you want, not the
   mechanics. "Returning users can sign in without re-entering their workspace"
   tells Bubbles what done looks like; "add a function to the auth module"
   only tells it where to start typing.
2. **Name the constraints.** Call out the boundaries that matter: what must not
   change, performance or compatibility limits, deadlines, dependencies to
   avoid. Constraints are how you keep an autonomous run inside the lines.
3. **Name the surface / area when it matters.** If the work clearly belongs to a
   specific feature, screen, service, or module, say so. If you genuinely don't
   know where it lives, say *that* instead of guessing — Bubbles can locate it.
4. **Request verification.** Ask for the proof you'd want to see: a test that
   covers the new behavior, a before/after measurement, a reproduction that now
   passes. This anchors the work to evidence instead of a claim.
5. **Specify the output shape when it matters.** If you need a particular form —
   a plan only, a single focused change, a short summary, a specific file or
   format — say so. If the shape doesn't matter, leave it open and let the right
   workflow decide.

---

## Anti-patterns to avoid

| Anti-pattern | Why it hurts | Do this instead |
|--------------|--------------|-----------------|
| **Too vague** ("make it better", "clean this up") | No outcome to aim at and no way to tell when it's done. | Lead with a concrete result and how you'd verify it. |
| **Over-prescriptive step-dump** (the "47-step runbook") | Hand-writing every step out-races the orchestrator, bakes in your assumptions, and breaks the moment one step is wrong. | Describe the *outcome* and the *constraints*; let the workflow plan the steps. |
| **Missing context** (referring to "the thing we discussed" with no anchor) | Forces guessing, which produces confident-but-wrong work. | Add the missing anchor: the feature, the error, the file, or the prior decision. |
| **Multiple unrelated tasks in one turn** ("fix search, add a theme, update docs, and look at the slow build") | Splits focus, tangles evidence, and makes partial results hard to judge. | Send one focused request, or use a sprint that names and prioritizes each goal. |

---

## Bubbles-specific: intent over runbook

Bubbles' core value is **autonomous orchestration** — you describe what you
want and the framework resolves the right workflow mode, picks the agents, and
drives them to a verified finish. For the autonomous entry points, a crisp
**outcome** beats a step list almost every time:

- `/bubbles.goal  "<intent>"` — the universal goal endpoint: it plans,
  implements, tests, validates, and remediates in a loop until it converges on
  your stated outcome.
- `/bubbles.workflow  <target> mode: <mode>` — deterministic execution of one
  explicit or super-resolved workflow mode.
- `/bubbles.sprint minutes: <N>` — several named goals under one time budget.

Because these modes do the planning for you, a long hand-written runbook
actively gets in the way: it constrains the planner to *your* sequence, which
is usually less complete than the workflow's own plan and goes stale the moment
the codebase differs from your mental model. Give it the **destination and the
guardrails**; let it choose the route.

Two reliable patterns:

- **You know the outcome, not the steps** → state the outcome + constraints +
  how to verify, and hand it to `/bubbles.goal`.
- **You know the exact process** → give one `mode:` to `/bubbles.workflow`.
- **You're not sure what to even ask for** → start a level up and let the
  front-door assistant translate your situation into the right command.

**Where to go next:**

- [`bubbles-workflow-mode-resolution`](../../skills/bubbles-workflow-mode-resolution/SKILL.md)
  — how plain-English intent maps to a workflow mode (the machinery behind
  "just describe it").
- [Just Tell Bubbles](../recipes/just-tell-bubbles.md) — describe what you want
  in plain English and let the workflow route it.
- [Autonomous Goal](../recipes/autonomous-goal.md) — give one goal and let the
  agent run everything to convergence.
- [Ask the Super First](../recipes/ask-the-super-first.md) — when you're unsure
  how to phrase the ask, get a recommendation before acting.

---

## Worked examples: vague → crisp

These are deliberately generic. The point is the *shape* of the request, not the
domain.

### Example 1 — feature work

> **Vague:** "Make the login better."

> **Crisp:** "Outcome: returning users can sign in without re-selecting their
> workspace each time. Constraint: don't change the existing session-token
> format. Verify with a test that signs in twice and asserts the workspace is
> remembered the second time."

*Why it's better:* it leads with the outcome, fences off the one thing that must
not change, and names the proof.

### Example 2 — outcome instead of a runbook

> **Over-prescriptive:** "Open the export module, add a `buildCsv` helper, then
> wire it into the report controller, then add a button, then add a route, then
> add a unit test for `buildCsv`, then …"

> **Crisp:** "Outcome: from the reporting area, a user can download all their
> records as a single file. Verify the downloaded file opens and contains every
> row that's visible on screen."

*Why it's better:* it states the destination and the check, and lets the
workflow choose the implementation steps — which may be simpler or more complete
than the hand-written sequence.

### Example 3 — one focused ask instead of a pile

> **Multiple unrelated tasks:** "Fix the search bug, also add a dark theme, and
> update the docs, and figure out why the build is slow."

> **Crisp (single outcome):** "Outcome: searching for an exact title returns
> that record on the first page. Reproduce the current miss first, then make it
> pass."

> **Crisp (when you really do have several):** hand the whole list to a sprint
> and let it prioritize — "Spend the session on: (1) the exact-title search miss,
> (2) a dark theme, (3) refreshing the docs — in that priority order."

*Why it's better:* a single outcome keeps the evidence clean; an explicit,
prioritized list is the right tool when you genuinely have a batch.

---

## See also

- [Recipe Catalog](../CATALOG.md) — the full menu of workflows, by problem.
- [Agent Manual](AGENT_MANUAL.md) — which agent or mode fits which situation.
- [Workflow Modes](WORKFLOW_MODES.md) — detailed mode descriptions with
  use-when guidance.
