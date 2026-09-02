# Scopes: BUG-025-006 Knowledge lint zero-scale pass

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Scope 1: Prove knowledge lint at exactly 1000 artifacts

**Status:** In Progress
**Priority:** P1
**Depends On:** None
**Scope-Kind:** runtime-behavior

The authorized stress-test implementation is present. Product and Docker validation remains serialized for a later invocation, so no Definition of Done item is complete yet.

### Gherkin Scenarios

```gherkin
Feature: Knowledge lint stress scale integrity

  Scenario: SCN-B025006-001 Production lint starts with exactly 1000 owned artifacts
    Given a disposable stress PostgreSQL database and a unique run token
    When the test seeds its synthetic artifact workload
    Then owned rows, distinct IDs, and distinct content hashes each equal 1000
    And production lint has not started before those assertions pass

  Scenario: SCN-B025006-002 Production lint returns a fresh result within budget
    Given exactly 1000 owned synthetic artifacts exist
    When the test invokes production knowledge lint
    Then a fresh persisted report is returned
    And wall and report durations are each at most five minutes

  Scenario: SCN-B025006-003 Zero or drifted scale fails before lint
    Given the observed owned count is zero, 999, or 1001
    When the cardinality guard expects exactly 1000
    Then the guard fails with expected and actual counts
    And production lint is not invoked

  Scenario: SCN-B025006-004 Cleanup proves zero owned residue
    Given the test created artifacts and one lint report
    When cleanup runs
    Then only test-owned rows are deleted
    And any remaining owned row fails the test

  Scenario: SCN-B025006-005 Stress dependency failures remain fail loud
    Given required stress configuration, a dependency, or an endpoint is unavailable
    When the stress lane reaches that path
    Then the test fails with the observed reason
    And no required path skips
```

### Implementation Plan

1. Add the owned-row cardinality query and exact assertion before any seed.
2. Run the red-stage stress command and capture the zero-owned failure.
3. Add an atomic 1000-row synthetic seed keyed by a unique `source_id` token.
4. Preserve production linter construction, `RunLint`, and fail-loud endpoint assertions.
5. Capture and validate the fresh lint report and both five-minute timing measurements.
6. Add fail-loud cleanup for the owned artifacts and captured report.
7. Add the zero and off-by-one cardinality adversarial regression.
8. Run focused and broader validation through the repository command surface.

### Change Boundary

Allowed implementation file:

- `tests/stress/knowledge_stress_test.go`

Allowed artifact files:

- `specs/025-knowledge-synthesis-layer/bugs/BUG-025-006-knowledge-lint-zero-scale/**`

Excluded surfaces:

- production runtime code
- migrations and database schema
- configuration and generated files
- Docker and Compose lifecycle files
- production caches or fixture paths
- every other test file
- unrelated dirty edits in the target file

### Test Plan

| ID | Test Type | Category | File/Location | Scenario | Description | Command | Live System |
|----|-----------|----------|---------------|----------|-------------|---------|-------------|
| B025006-TP01 | Red-stage Regression E2E / stress | `stress` | `tests/stress/knowledge_stress_test.go::TestKnowledge_LintAt1000ArtifactScale` | SCN-B025006-001, SCN-B025006-003 | Add the exact pre-run count assertion before seeding and capture failure at zero owned rows | `./smackerel.sh test stress` | Yes, disposable stress stack |
| B025006-TP02 | Scenario-specific Regression E2E / stress | `stress` | `tests/stress/knowledge_stress_test.go::TestKnowledge_LintAt1000ArtifactScale` | SCN-B025006-001, SCN-B025006-002, SCN-B025006-004 | Seed exactly 1000 owned artifacts, run production lint, validate the fresh report and timing, and prove zero residue | `./smackerel.sh test stress` | Yes, disposable stress stack |
| B025006-TP03 | Adversarial Regression E2E / stress | `stress` | `tests/stress/knowledge_stress_test.go::TestKnowledge_LintScaleCardinalityGuardRejectsZeroAndDrift` | SCN-B025006-003 | Reject observed counts zero, 999, and 1001 while accepting exactly 1000 | `./smackerel.sh test stress` | Yes, disposable stress stack |
| B025006-TP04 | Broader Regression E2E / stress | `stress` | `tests/stress/knowledge_stress_test.go` | SCN-B025006-002, SCN-B025006-004, SCN-B025006-005 | Run the complete stress lane and preserve all neighboring fail-loud knowledge checks | `./smackerel.sh test stress` | Yes, disposable stress stack |
| B025006-TP05 | Regression quality | `functional` | `tests/stress/knowledge_stress_test.go` | SCN-B025006-003, SCN-B025006-005 | Reject silent-pass and weak adversarial patterns in the changed stress test | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/knowledge_stress_test.go` | No |
| B025006-TP06 | Build quality | `functional` | Repository Go and policy surfaces | All | Check compilation, lint, and formatting without bypassing the repository CLI | `./smackerel.sh check && ./smackerel.sh lint && ./smackerel.sh format --check` | No |

### Definition of Done

- [ ] B025006-TP01 red-stage regression fails before production lint because the owned count is zero.
- [ ] B025006-TP02 scenario-specific regression proves exactly 1000 owned rows, a fresh lint result, both timing budgets, and zero residue.
- [ ] B025006-TP03 adversarial regression rejects zero, 999, and 1001 counts and accepts exactly 1000.
- [ ] B025006-TP04 broader stress regression passes with neighboring knowledge paths still fail loud.
- [ ] B025006-TP05 regression-quality guard passes with adversarial signals and no silent-pass pattern.
- [ ] B025006-TP06 repository check, lint, and format checks pass with zero warnings.
- [ ] Root cause is confirmed with executed red-stage evidence.
- [ ] Exactly 1000 uniquely identified synthetic artifacts exist before production lint starts.
- [ ] Production `knowledge.Linter.RunLint` executes against disposable PostgreSQL and NATS.
- [ ] The returned report is fresh, valid, and inside both five-minute budgets.
- [ ] Cleanup deletes only owned artifacts and the captured report, then asserts both owned counts are zero.
- [ ] Cleanup errors and owned residue fail the test instead of logging and continuing.
- [ ] `SCN-B025006-005`: unavailable required stress configuration, dependencies, or knowledge endpoints fail with the observed reason, and no required stress path skips.
- [ ] No production cache, fake data path, persistent environment, default, fallback, migration, or runtime change is introduced.
- [ ] The exact change boundary is respected.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior.
- [ ] Broader E2E regression suite passes.
- [ ] Artifact lint and traceability guard pass for this packet.
- [ ] Human acceptance remains unchecked until a human exercises the delivered fix.

Test Plan rows: 6. Matching `B025006-TP` DoD items: 6.

### Ownership Routing

- Implementation owner: `bubbles.implement`
- Test execution owner: `bubbles.test`
- Certification owner: `bubbles.validate`
- Documentation owner: `bubbles.docs` only if implementation changes user-facing or operator documentation