# Test Fidelity

Purpose: canonical source for planned-behavior fidelity and use-case-centered testing.

## Rules
- Tests validate `spec.md`, `design.md`, scope artifacts, and DoD.
- Do not weaken tests to match broken implementation.
- If the plan is wrong, correct the owning planning artifact first, then align test and implementation together.
- Required tests must prove real user or API-consumer behavior, not proxy signals.
- Changed behavior needs red then green proof.
- Bug-fix regressions need at least one adversarial case that would fail if the bug returned; tautological setups that already satisfy the broken code path do not count.
- Required test bodies must not early-return on the failure condition. Assert the unwanted behavior directly instead of bailing out when it appears.
- Live-state tests that create or mutate data must use agent-owned fixtures, not borrowed shared fixtures.
- Write paths must not target "first existing" resources from list endpoints unless the scenario is explicitly read-only.
- Shared defaults, host-level settings, and other cross-scenario baseline state require snapshot-and-restore proof before a mutation test can claim completion.
- When shared fixtures, harnesses, or bootstrap/auth/session infrastructure change, tests must include an independent downstream canary that validates the consumer contract before the broad suite reruns; using the modified fixture to validate only itself is insufficient.
- Canary tests for shared infrastructure must assert the contract surfaces that tend to cascade silently, such as ordering, timing, bootstrap state injection, session/context hydration, or equivalent downstream assumptions.
