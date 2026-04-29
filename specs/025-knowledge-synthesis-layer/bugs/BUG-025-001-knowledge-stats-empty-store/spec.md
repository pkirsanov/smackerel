# Feature: BUG-025-001 Knowledge stats empty-store 500

## Problem Statement
The knowledge layer should be observable even before any knowledge has been synthesized. Returning HTTP 500 for empty stats breaks live-stack E2E and gives operators an error for a valid initial state.

## Outcome Contract
**Intent:** Empty knowledge stores report valid zero-valued stats instead of errors.
**Success Signal:** Authenticated `GET /api/knowledge/stats` on a fresh disposable stack returns HTTP 200 with zero counts and no scan error.
**Hard Constraints:** Missing data must not be hidden by broad fallbacks; the query must explicitly model the empty-store case and still surface real database errors.
**Failure Condition:** Empty knowledge stats returns HTTP 500, or tests accept a stats response without asserting zero-valued content.

## Goals
- Capture targeted red-stage evidence for `/api/knowledge/stats` on empty store.
- Fix the empty-store stats query without masking genuine database failures.
- Add adversarial regression coverage for no concepts and no lint reports.

## Non-Goals
- Changing synthesis business logic.
- Seeding fake knowledge data into E2E just to avoid empty-store behavior.
- Weakening the stats endpoint to ignore database errors.

## Requirements
- Empty store stats must return HTTP 200.
- Counts must be zero when the corresponding tables are empty.
- Prompt contract version must serialize as an empty string or equivalent explicit empty value without scan failure.
- Regression tests must fail if the scalar subquery again produces unhandled NULL for an empty concepts table.

## User Scenarios (Gherkin)

```gherkin
Scenario: Knowledge stats returns zero values for an empty store
  Given the knowledge store contains no concepts, entities, or synthesized artifacts
  When an authenticated caller requests knowledge stats
  Then the response is successful
  And the stats counts are zero
  And prompt_contract_version is an explicit empty value

Scenario: Knowledge stats regression fails on unhandled empty prompt contract version
  Given knowledge_concepts has no rows
  When the stats query computes the latest prompt contract version
  Then the empty result is handled explicitly without scanning NULL into a string
```

## Acceptance Criteria
- Targeted pre-fix failure output captures the empty-store 500 or scan error.
- Post-fix unit or integration coverage proves empty-store stats returns zero values.
- Post-fix E2E coverage proves `/api/knowledge/stats` returns HTTP 200 on a fresh disposable stack.
