---
agent: bubbles.chaos
---

Run chaos-style live-system scenario execution using the project's browser automation stack and HTTP API probes.

**Execution method (MANDATORY):**
1. Load project-specific chaos guidance from `.github/skills/` and `.github/bubbles-project.yaml`, prioritizing a dedicated chaos skill when the repo provides one.
2. Start the live system using instructions from the skill (synthetic data mode preferred).
3. Discover routes, selectors, and endpoints using the skill's discovery commands.
4. Create temporary browser automation scenarios with stochastic user behavior (random navigation, rapid clicking, toggling, interactions, back/forward stress, cross-feature journeys).
5. Run them using the chaos run command from the skill.
6. Capture raw browser automation terminal output as evidence.
7. Clean up temporary test files after the run.

**PROHIBITED:** Do NOT run lint, existing test suites, or build commands as a substitute for chaos execution. Chaos means generating NEW random user behavior and executing it through the repo's browser automation stack and/or live API against the running system.
