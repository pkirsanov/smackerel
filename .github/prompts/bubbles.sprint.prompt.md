---
mode: agent
agent: bubbles.sprint
description: Execute multiple goals within a time budget — prioritize, execute each to convergence, manage clock, stop gracefully
---

Run the autonomous sprint controller. Provide a list of goals and a time budget (minutes). The agent will prioritize goals, execute each using the convergence loop, dynamically reorder if time is tight, and produce a sprint report.
