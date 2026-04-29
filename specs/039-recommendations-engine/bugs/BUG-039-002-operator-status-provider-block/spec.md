# Feature: BUG-039-002 Operator status provider block

## Problem Statement
Feature 039 requires the operator status page to expose recommendation provider health even when no providers are configured. Broad E2E now reports that the `Recommendation Providers` block is missing, so `SCN-039-002` lacks a valid live UI proof.

## Outcome Contract
**Intent:** The operator `/status` page always renders the recommendation provider health block when the recommendations feature is enabled, including the empty-provider state.
**Success Signal:** `TestOperatorStatus_RecommendationProvidersEmptyByDefault` observes the `Recommendation Providers` block and zero-provider messaging with no fabricated rows.
**Hard Constraints:** The regression must exercise the real status route/template and recommendation registry state in the live stack. It must not bypass the page with a unit-only assertion or weaken the test to accept a missing block.
**Failure Condition:** `/status` omits the provider health block, fabricates provider rows when none are configured, or the E2E passes without checking the user-visible block.

## Goals
- Restore the `SCN-039-002` operator status live UI proof.
- Capture red-stage evidence showing the `/status` response without the provider block.
- Preserve the empty-provider semantics required by feature 039.

## Non-Goals
- Completing unrelated recommendation provider behavior from later 039 scopes.
- Marking feature 039 Scope 1 done without full validation evidence.
- Changing Phase 1 search, digest, or Phase 2 topic lifecycle behavior.

## Requirements
- `/status` must render a clear `Recommendation Providers` block when recommendations are enabled.
- With no providers configured, the block must show zero providers without fabricated provider rows.
- The E2E must assert user-visible content on the real status page.
- The API path for empty provider requests must continue to return `no_providers` without fabricated candidates.

## User Scenarios (Gherkin)

```gherkin
Scenario: Operator status shows empty recommendation providers block
  Given recommendations are enabled and no providers are configured
  When the operator opens the status page
  Then the page shows the Recommendation Providers block
  And the block indicates zero configured providers without fabricated rows

Scenario: Operator status regression fails when the provider block is absent
  Given the status page response does not include Recommendation Providers
  When the E2E validates the empty-provider state
  Then the test fails with diagnostics instead of accepting the missing section
```

## Acceptance Criteria
- Targeted pre-fix evidence includes the status page response or relevant excerpt missing the provider block.
- The fixed `TestOperatorStatus_RecommendationProvidersEmptyByDefault` passes against the live stack.
- Broad `./smackerel.sh test e2e` no longer reports this operator status failure once all routed blockers are fixed.
