# Bug Specification: BUG-003-002 Topic momentum star aggregation

## Problem Statement

Topic lifecycle updates cannot execute against the canonical PostgreSQL schema because the production query references a nonexistent denormalized topic column. This silently disables the R-208 lifecycle outcome behind an error log while unrelated health checks remain green.

## Outcome Contract

**Intent:** Recalculate topic momentum from canonical persisted signals, including explicit user stars, without introducing schema drift or masking database failures.

**Success Signal:** Against a disposable PostgreSQL database created by the production migration entry point, the actual lifecycle query updates a zero-star topic and a topic linked to multiple starred artifacts. The resulting momentum values reflect only the starred artifacts linked by canonical `BELONGS_TO` relationships.

**Hard Constraints:**

- PostgreSQL remains the only persistence backend.
- Explicit stars come from `artifacts.user_starred`.
- Topic membership comes from `artifact -> topic` `BELONGS_TO` edges.
- No `topics.star_count` column or denormalized replacement is added.
- Query failures remain explicit and scheduler logs remain honest.
- All mutable test state uses the disposable integration stack.

**Failure Condition:** The repair is invalid if it makes the query syntactically pass without deriving stars from persisted relationships, counts unlinked starred artifacts, hides SQL errors, mutates a persistent environment, or changes the health contract.

## Requirements

### BUG-R1 Canonical schema compatibility

`UpdateAllMomentum` must execute against the schema produced by the canonical migrations when `topics.star_count` is absent.

### BUG-R2 Canonical explicit-star source

For each topic, explicit star count must equal the number of distinct starred artifacts linked to that topic by an edge satisfying all of:

- `edges.src_type = 'artifact'`
- `edges.src_id = artifacts.id`
- `edges.dst_type = 'topic'`
- `edges.dst_id = topics.id`
- `edges.edge_type = 'BELONGS_TO'`
- `artifacts.user_starred IS TRUE`

### BUG-R3 Zero-star behavior

A topic with linked artifacts but no linked starred artifacts must receive zero star contribution while retaining the existing connection contribution and recency decay semantics.

### BUG-R4 Multiple-star behavior

A topic linked to multiple distinct starred artifacts must receive one R-208 star contribution per starred artifact. Unstarred linked artifacts and starred artifacts not linked to that topic must not increase its explicit star count.

### BUG-R5 Honest failure behavior

Database query failures must continue to return from `UpdateAllMomentum` with `query topics` context. `doTopicMomentumJob` must continue to log `topic momentum update failed` and must not emit the success message for a failed run.

### BUG-R6 Change containment

The repair may modify only the lifecycle query, focused regression tests, scheduler log regression coverage, and this bug packet. It must not change migrations, deployment, release trains, secrets, manifests, or health semantics.

## User Scenarios

```gherkin
Feature: BUG-003-002 derive topic stars from canonical relationships

  Scenario: BUG-003-002-SCN-001 Canonical star aggregation updates momentum
    Given canonical migrations created topics without a star_count column
    And one topic has no linked starred artifacts
    And another topic has two linked starred artifacts and one linked unstarred artifact
    When the actual lifecycle momentum update runs
    Then both topics are updated without a missing-column error
    And the zero-star topic receives no star contribution
    And the multi-star topic receives exactly two star contributions
    And an unlinked starred artifact contributes nothing

  Scenario: BUG-003-002-SCN-002 Lifecycle query failures remain observable
    Given the lifecycle database pool cannot execute the topic query
    When the scheduler runs the topic momentum job
    Then the lifecycle returns a query-topics error
    And the scheduler logs topic momentum update failed
    And the scheduler does not log topic momentum updated for that run

  Scenario: BUG-003-002-SCN-003 Existing topic lifecycle surface remains available
    Given the disposable full test stack contains topic lifecycle fixtures
    When the targeted topic lifecycle shell flow runs
    Then the topics surface renders the lifecycle topics and momentum values
```

## Acceptance Criteria

| ID | Criterion | Scenario | Test Type |
|---|---|---|---|
| BUG-AC1 | Production lifecycle query executes against canonical migrations with no `topics.star_count` | BUG-003-002-SCN-001 | Integration |
| BUG-AC2 | Zero linked stars produce zero star contribution | BUG-003-002-SCN-001 | Integration |
| BUG-AC3 | Two linked starred artifacts produce exactly two star contributions | BUG-003-002-SCN-001 | Integration |
| BUG-AC4 | Unstarred and unlinked artifacts do not inflate explicit star count | BUG-003-002-SCN-001 | Integration |
| BUG-AC5 | Query errors remain returned and scheduler failure logging remains distinct from success | BUG-003-002-SCN-002 | Unit |
| BUG-AC6 | Existing topic lifecycle surface regression remains green | BUG-003-002-SCN-003 | E2E API |
