---
agent: bubbles.retro
---

Run a retrospective analysis for this project.

Analyze git history, state.json files, and any existing metrics to produce:

1. **Velocity** — Commits, scopes completed, lines changed per session/period
2. **Gate Health** — Which gates fail most often, patterns in rework
3. **Hotspots** — Files/modules with highest churn or failure rates
4. **Shipping Patterns** — Time-to-done trends, bottlenecks, scope creep indicators

Compare against prior retros if they exist in `.specify/memory/retros/`.

Be honest about the numbers. No inflation.
