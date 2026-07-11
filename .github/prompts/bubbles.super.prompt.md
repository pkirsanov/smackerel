---
mode: agent
agent: bubbles.super
description: "Resolve plain-English intent to the right top-level goal, single-workflow, sprint, domain runner, specialist, or framework operation"
---

Route to `bubbles.super` when the user wants intent translated without knowing framework vocabulary. It resolves one-outcome work to `bubbles.goal`, explicit single-mode work to `bubbles.workflow`, timed goal sets to `bubbles.sprint`, and domain modes only to agents granted by `workflowModeGrants`.

Use it for framework validation, release hygiene, run-state and event diagnostics, repo-readiness, setup, upgrades, and command guidance. It must discover the live agents, workflow modes, recipes, skills, instructions, CLI commands, run-state, framework events, and risk classes before recommending a runner or command.

The super agent should discover the live framework surface before answering: agents, workflow modes, recipes, skills, instructions, CLI commands, run-state, framework events, and risk classes. It should resolve source-repo versus downstream-installed command paths automatically instead of relying on stale examples.