---
name: bubbles-test-integrity
description: Enforce real, specification-driven test quality during any test work. Use when writing tests, implementing scope test plans, reviewing test coverage, or validating DoD test items. Triggers include writing tests, adding test coverage, implementing scope tests, reviewing test plans, marking test DoD items, and verifying Gherkin scenario coverage.
---

# Bubbles Test Integrity

## Goal
Ensure every test is real, validates planned behavior from specifications, covers all Gherkin scenarios, and contains no fakes, shortcuts, or silent-pass patterns.

## When to Use
- Writing or modifying tests for any scope or bug
- Implementing test plan items from `scopes.md`
- Reviewing test coverage before marking DoD items
- Validating that tests match Gherkin scenarios in scope artifacts
- Checking test quality during audit or review phases

## Non-Negotiable Principles

1. **Tests validate specifications, not implementation** — derive tests from `spec.md`, `design.md`, and scope Gherkin scenarios. Never weaken a test to match broken code.
2. **Tests must be real** — every test must execute real code paths against real systems (for live categories) or real logic (for unit categories). No stubs, no fakes, no canned responses.
3. **No shortcuts** — no partial coverage presented as complete, no reduced-scope tests presented as full validation, no proxy assertions substituting for real behavior checks.
4. **Every Gherkin scenario must have a test** — each Given/When/Then scenario in scope artifacts maps to at least one executable test. No scenario left untested.
5. **Scenario contracts are durable** — when `scenario-manifest.json` exists, tests must preserve the linked `SCN-*` contracts unless the scenario has been explicitly invalidated and replaced.

---

## Pre-Test-Writing Checklist

Before writing any test:

- [ ] Read the Gherkin scenarios in the scope artifact (`scopes.md` or `scopes/*/scope.md`)
- [ ] Read `spec.md` and `design.md` for expected behavior definitions
- [ ] Identify ALL scenarios: happy path, error paths, boundary conditions, parameter permutations
- [ ] Determine required test categories per the Canonical Test Taxonomy (`unit`, `functional`, `integration`, `ui-unit`, `e2e-api`, `e2e-ui`, `stress`, `load`)
- [ ] Verify the Test Plan table in the scope artifact has a row for each required test

---

## Test Quality Gates

### Gate 1: Gherkin Coverage

Every Gherkin scenario in scope artifacts MUST map to at least one test.

When `scenario-manifest.json` exists, every `SCN-*` entry marked `regressionRequired: true` MUST map to at least one live-system test and evidence reference.

**Verification:**
```
Count Gherkin scenarios → Count matching tests → They must be equal or tests > scenarios
```

**Prohibited:**
- Gherkin scenario exists but no corresponding test
- Test claims to cover a scenario but asserts different behavior
- Multiple scenarios collapsed into one test that only checks one path

### Gate 2: No Internal Mocks (Live Categories)

Tests classified as `integration`, `e2e-api`, or `e2e-ui` MUST hit the real running stack.

**Prohibited interception patterns (scan before marking done):**
```bash
grep -rn 'page\.route\|context\.route\|intercept(\|cy\.intercept\|msw\|nock\|wiremock' [test-files]
```

If any match is found in a file classified as live-system, either:
- Reclassify the test to `ui-unit` or `unit` (mocked category), OR
- Remove the interception and test against the real stack

**Rule:** A mocked test MUST NOT satisfy a live-stack DoD item.

### Gate 3: No Silent-Pass Patterns

Required tests MUST fail when the feature is missing or broken.

**Prohibited patterns (scan before marking done):**
```bash
bash .github/bubbles/scripts/regression-quality-guard.sh [test-files]
bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix [test-files]  # bug-fix scopes only
```

**Rules:**
- Missing required selector → test MUST fail via direct assertion (e.g., `expect(locator).toBeVisible()`)
- No early-return paths after entering a required scenario
- No redirect/auth bailout branches such as `if (page.url().includes('/login')) { return; }`
- Persistence tests MUST assert concrete field values, not just null/defined checks
- UI tests MUST assert user-visible outcomes (computed styles, DOM state, round-trip reload)

### Gate 4: Real Assertions

Every test MUST contain assertions that prove the specified behavior.

**Prohibited:**
- Tests without assertions
- Tests that only check HTTP status codes without validating response content
- Tests that assert "returns 200 or 404" (proxy assertions)
- Tests that log output without asserting on it
- `// TODO: add assertions` or equivalent

**Required:**
- Assert the EXACT behavior described in the Gherkin scenario
- Assert error messages, field values, state transitions — not just success/failure signals
- For data mutations: assert round-trip (write → read → verify persisted state)

### Gate 5: Test Plan ↔ DoD Parity

Every row in the Test Plan table MUST have a corresponding DoD checkbox item, and vice versa.

**Verification:**
```
Count Test Plan rows == Count DoD test-related items
```

Mismatch is a BLOCKING issue — scope cannot be marked Done.

### Gate 6: Adversarial Regression Coverage (Bug Fixes)

Bug-fix regression tests MUST include at least one adversarial test case — a test using input that would FAIL if the bug were reintroduced.

**What makes a test adversarial:**
- Filter/gate bugs: test data does **not** match the buggy filter, and still must pass
- Auth/redirect bugs: direct assertion that the failure condition does **not** occur, with no bailout-return
- Persistence/data-shape bugs: round-trip with the edge-case input that triggered the original bug

**Prohibited (tautological tests):**
- All test data already satisfies the broken code path
- Test setup mirrors the exact condition the buggy filter checks for
- No negative or adversarial scenario exists in any regression case

**Prohibited (false-negative bailouts):**
- `if (page.url().includes('/login')) { return; }`
- `if (!hasControl) { return; }`
- Any conditional early return in a required test body that exits without asserting on the failure path

This gate applies to bug fixes and regression scopes. Feature-only scopes are exempt unless they claim to close an existing bug.

---

## Per-Category Requirements

| Category | What "Real" Means |
|----------|-------------------|
| `unit` | Tests real code paths, no mocking of internal modules. Only external third-party deps may be mocked. |
| `functional` | May use real dependencies (DB, filesystem). Tests actual function behavior with real inputs. |
| `integration` | Multi-component, real dependencies, zero mocks. Tests actual service interactions. |
| `ui-unit` | Component-level tests with mocked backend are acceptable. Must assert visual/behavioral output. |
| `e2e-api` | Full API workflow against live system. No request interception. Real HTTP calls, real responses. |
| `e2e-ui` | Full UI workflow against live system. No `page.route()` or mock interceptors. Real browser, real backend. |
| `stress` | Burst load against live system. Must verify behavior under pressure, not just "no crash". |
| `load` | Sustained load against live system. Must verify throughput and latency SLAs. |

---

## Verification Workflow

When completing test work, execute this sequence:

### Step 1: Scenario Traceability
For each Gherkin scenario in scope artifacts, confirm a test exists that:
- Sets up the Given precondition
- Performs the When action
- Asserts the Then outcome

### Step 2: Anti-Mock Scan (live categories)
Run interception pattern scan against all files classified as `integration`, `e2e-api`, or `e2e-ui`.

### Step 3: Anti-False-Positive Scan
Run silent-pass pattern scan against all required test files.

### Step 4: Assertion Audit
Verify every test body contains at least one behavior-proving assertion matching the corresponding Gherkin scenario.

### Step 5: Adversarial Regression Audit
For bug-fix scopes, verify at least one regression case would fail if the bug returned and that no bailout patterns convert the bug back into a pass.

### Step 6: Test Plan ↔ DoD Cross-Check
Count Test Plan rows. Count DoD test items. Confirm parity.

### Step 7: Execute and Record Evidence
Run all tests. Capture raw terminal output (≥10 lines per test category). Record in `report.md`.

---

## Decision Tree: Is This Test Real?

```
Does the test execute real production code?
├─ NO → ❌ FAKE — rewrite to use real code paths
└─ YES
   Does it assert the exact behavior from a Gherkin scenario or spec requirement?
   ├─ NO → ❌ PROXY — rewrite assertions to match spec
   └─ YES
      For live categories: does it hit the real stack without interception?
      ├─ NO → ❌ MOCKED — reclassify or remove interception
      └─ YES
         Can the test fail if the feature is broken or missing?
         ├─ NO → ❌ SILENT-PASS — add direct failure assertions
         └─ YES
            For bug fixes: is at least one regression case adversarial rather than tautological?
            ├─ NO → ❌ TAUTOLOGICAL — rewrite the regression case with adversarial input
            └─ YES → ✅ REAL TEST
```

---

## References
- `agents/bubbles_shared/test-core.md`
- `agents/bubbles_shared/test-fidelity.md`
- `agents/bubbles_shared/quality-gates.md`
- `agents/bubbles_shared/critical-requirements.md`
- `agents/bubbles_shared/evidence-rules.md`
- `agents/bubbles_shared/scope-workflow.md`
- `agents/bubbles_shared/feature-templates.md`
- `docs/guides/CONTROL_PLANE_SCHEMAS.md`
