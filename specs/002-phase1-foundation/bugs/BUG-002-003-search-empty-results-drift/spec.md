# Feature: BUG-002-003 Search empty results drift

## Problem Statement
Spec 002 protects the Phase 1 search contract that unknown queries must not fabricate matches. Broad E2E now reports the empty-results scenario returning five results, which means the live search proof no longer satisfies `SCN-002-023`.

## Outcome Contract
**Intent:** Unknown or unmatched search queries return an honest zero-result response without unrelated artifacts.
**Success Signal:** The live E2E search scenario observes zero results and the configured nothing-found message for the unknown-query fixture.
**Hard Constraints:** The regression must exercise the real API, persistence, embedding/search path, and broad-suite fixture state with no request interception, silent skip, or weakened assertions.
**Failure Condition:** The scenario returns unrelated artifacts, accepts any nonzero result count, or passes without checking the honest zero-result response.

## Goals
- Preserve `SCN-002-023` as a protected empty-results search contract.
- Capture red-stage evidence showing the five unexpected results and their source.
- Restore strict zero-result behavior for unknown-query fixtures in the live stack.

## Non-Goals
- Changing the expected behavior to accept fuzzy matches for intentionally unknown queries.
- Weakening broad E2E search assertions.
- Modifying unrelated recommendation or domain extraction behavior.

## Requirements
- Unknown-query E2E fixtures must be isolated from artifacts created by earlier broad-suite scenarios.
- The regression must assert result count, response message, and absence of unrelated artifacts.
- The adversarial case must include an unknown query with tempting but irrelevant broad-suite artifacts present.
- Any fix must preserve successful search behavior for known queries.

## User Scenarios (Gherkin)

```gherkin
Scenario: Unknown query returns honest empty result
  Given the disposable live stack contains artifacts unrelated to a deliberately unknown query
  When the user searches for that unknown query
  Then the search response contains zero results
  And the response includes the honest nothing-found message

Scenario: Empty-results regression rejects leaked broad-suite artifacts
  Given prior E2E scenarios have created searchable artifacts
  When the empty-results scenario runs in the same broad suite
  Then unrelated artifacts are not returned as matches for the unknown query
```

## Acceptance Criteria
- Targeted red-stage evidence records the unexpected five result identifiers or titles.
- The fixed targeted search E2E passes with zero results for `SCN-002-023`.
- Broad `./smackerel.sh test e2e` no longer reports this search empty-results failure once all routed blockers are fixed.
