# Specification: BUG-025-006 Knowledge lint scale integrity

## Purpose

Make the 1000-artifact lint stress claim executable. A passing test must prove its workload, production lint result, timing, and cleanup.

## Parent Contract

- Parent feature: `specs/025-knowledge-synthesis-layer`
- Parent requirement: `R-2506: Knowledge Lint System`
- Parent business scenario: `BS-010: Knowledge Layer Scales with Volume`
- Parent performance contract: daily lint completes within five minutes for a 1000-artifact knowledge base

## Problem Statement

`TestKnowledge_LintAt1000ArtifactScale` currently starts production lint without seeding artifacts. It also lacks a pre-run cardinality assertion.

The test can therefore pass while proving only empty-store lint behavior. Its name and comments overstate the workload that executed.

## Outcome Contract

### Inputs

- The repository-managed disposable stress PostgreSQL database.
- The repository-managed disposable stress NATS service.
- Generated test configuration loaded through the existing production configuration path.
- One unique run ownership token.

### Outputs

- Exactly 1000 synthetic artifact rows owned by the run before lint starts.
- One fresh lint report produced by the production `knowledge.Linter`.
- Wall-clock and persisted report durations within five minutes.
- Zero test-owned artifact and lint-report rows after cleanup.

### Failure Semantics

The test must fail when any precondition, seed, count, lint call, result assertion, duration assertion, deletion, or residue check fails.

The test must not skip because configuration, PostgreSQL, NATS, or a knowledge endpoint is unavailable.

## Functional Requirements

### FR-01 Unique Ownership

Generate one fresh run token. Store it in every seeded artifact's indexed `source_id` field.

Embed the same token in every artifact ID and content hash. Each ID and content hash must also include a unique ordinal.

### FR-02 Exact Synthetic Seed

Insert exactly 1000 artifacts in one bounded, atomic seed operation. Use valid test-only values for required artifact fields.

Set `synthesis_status` to `completed`. This isolates lint-scale measurement from retry publication and prevents synthetic backlog traffic.

### FR-03 Pre-Run Cardinality Proof

Before calling production lint, query PostgreSQL by the run token.

Assert all of these values equal 1000:

- total owned rows
- distinct artifact IDs
- distinct content hashes

Any value other than 1000 must fail before `RunLint` starts.

### FR-04 Production Lint Execution

Construct the existing production `KnowledgeStore` and `Linter`. Invoke `RunLint` with the generated stress configuration and real disposable dependencies.

Do not add a fake linter, cache result, canned report, or test-only production path.

### FR-05 Result Assertions

Assert that `RunLint` returns no error. Read the resulting report through the existing knowledge lint endpoint.

Assert that the report has a non-empty ID. Assert `run_at` is not earlier than the captured run start.

Assert the report duration is non-negative. Assert the report duration and measured wall duration are each at most five minutes.

Capture the report ID so cleanup removes only the report created by this test.

### FR-06 Cleanup Integrity

Register cleanup before the seed begins. Delete artifact rows only where `source_id` equals the run token.

Delete the captured lint report only by its exact report ID. Do not truncate shared tables.

After deletion, query both owned row sets. Any remaining owned artifact or report row must fail the test.

Cleanup checks supplement disposable-stack destruction. They must never justify running against a persistent environment.

### FR-07 Disposable Environment Only

Run only through `./smackerel.sh test stress`. Use the isolated stress stack and generated test credentials.

Never read or write the persistent development, staging, or production database. Never introduce a persistent test volume.

### FR-08 Honest Synthetic Data

Synthetic rows exist only in the stress test. Their IDs, hashes, titles, content, and source marker must identify them as test data.

Do not add synthetic data, fixtures, defaults, fallbacks, or caches to production code.

### FR-09 Preserve Fail-Loud Stress Behavior

Preserve the dirty changes that replace `t.Skip`, `t.Skipf`, and unavailable-endpoint skips with direct failures.

Do not weaken endpoint status, transport, report freshness, or duration assertions while adding scale setup.

### FR-10 Focused Change

The implementation may change `tests/stress/knowledge_stress_test.go` only. Runtime code, migrations, config, Compose, and generated files are out of scope.

## Gherkin Scenarios

```gherkin
Feature: Knowledge lint stress scale integrity

  Scenario: SCN-B025006-001 Production lint starts with exactly 1000 owned artifacts
    Given a disposable stress PostgreSQL database
    And a unique run ownership token
    When the stress test seeds its synthetic artifact workload
    Then the owned row count is exactly 1000
    And the distinct artifact ID count is exactly 1000
    And the distinct content hash count is exactly 1000
    And production lint has not started before these assertions pass

  Scenario: SCN-B025006-002 Production lint returns a fresh result within budget
    Given exactly 1000 owned synthetic artifacts exist
    When the test invokes the production knowledge linter
    Then production lint returns without error
    And the knowledge lint endpoint returns the newly created report
    And the report identity and run time prove freshness
    And wall and report durations are each at most five minutes

  Scenario: SCN-B025006-003 Zero or drifted scale fails before lint
    Given the ownership marker identifies zero, 999, or 1001 artifact rows
    When the cardinality guard expects 1000 rows
    Then the guard returns an exact cardinality error
    And production lint is not invoked

  Scenario: SCN-B025006-004 Cleanup proves zero owned residue
    Given the test created owned artifacts and a lint report
    When cleanup runs
    Then only rows owned by the test are deleted
    And the owned artifact count is zero
    And the owned lint report count is zero
    And any remaining owned row fails the test

  Scenario: SCN-B025006-005 Stress dependency failures remain fail loud
    Given a required generated setting, disposable dependency, or knowledge endpoint is unavailable
    When the knowledge stress lane reaches that dependency
    Then the test fails with the observed reason
    And no required path returns through t.Skip or t.Skipf
```

## Acceptance Criteria

- AC-01: A pass cannot occur unless exactly 1000 rows match the run token before lint starts.
- AC-02: All owned artifact IDs and content hashes are distinct.
- AC-03: The real production linter and disposable PostgreSQL and NATS dependencies execute.
- AC-04: The fresh lint report identity, freshness, shape, and durations are asserted.
- AC-05: Cleanup fails on deletion errors, count errors, or owned residue.
- AC-06: No production cache, fake data path, persistent environment, or fallback is added.
- AC-07: Existing dirty fail-loud endpoint and configuration assertions remain intact.
- AC-08: The adversarial cardinality test rejects zero and off-by-one workloads.

## Non-Goals

- Changing production lint behavior.
- Changing the five-minute budget.
- Changing artifact schema or adding an ownership column.
- Testing synthesis retry throughput.
- Changing stress stack lifecycle or configuration.
- Reverting unrelated dirty edits in the target file.

## Release Train

Target train: `mvp`.

No feature flag is introduced. This packet repairs validation for the parent train and adds no runtime behavior to other trains.

## Product Principle Alignment

- Principle 3, **Knowledge Breathes (Lifecycle, Not Static)**: the lint lifecycle receives a measured scale contract instead of a name-only claim.
- Principle 8, **Trust Through Transparency**: the test must expose the actual workload cardinality and fresh report evidence behind its performance claim.

This packet makes no claim that new product capability shipped. It defines a test-integrity repair for an existing capability.