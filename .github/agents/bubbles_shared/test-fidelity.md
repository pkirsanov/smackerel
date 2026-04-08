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
- **Tests must not assert on their own setup data (No Self-Validating Tests).** Every assertion must verify a value that was *produced* by the code under test, not a value the test itself hardcoded or injected. The input-to-assertion path must pass through real code that meaningfully transforms, validates, computes, queries, or routes the data. If the code under test could be replaced with `return input` and the test would still pass, the test is circular and must be rewritten.
- When the data source is dynamic (connectors, simulators, external feeds), prefer structural assertions (correct shape, type, range, format, cardinality) over hardcoded magic-value comparisons. Asserting `score == 0.912` is only valid if `0.912` is the deterministic output of a known computation with known inputs — not if it is a value the test fixture invented.
- For unit tests: mocking external dependencies is allowed, but the assertion must verify that the code under test *responded correctly* to the mock's behavior — not merely that the mock returned what it was configured to return.
- For live-system tests (integration, e2e, stress, functional): assertions must verify behavior produced by the real running system. Injecting canned response data and asserting on that canned data is self-validation, not system validation.
