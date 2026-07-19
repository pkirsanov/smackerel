# Spec: BUG-073-005 - Comment-aware PWA storage policy scanning

## Problem Statement

The served-route E2E scans raw JavaScript and treats forbidden API names inside
policy comments as executable browser-storage access. The dedicated source
guard already has comment-aware behavior, so the two checks disagree.

## Requirements

- **R1 - Shared scanner.** One reusable helper must expose executable
  JavaScript source for policy scans. Both the unit storage guard and live E2E
  use it.
- **R2 - Comment-safe.** Line and block comments naming forbidden APIs do not
  trigger findings.
- **R3 - String-safe.** URL/string content containing comment markers is not
  accidentally truncated into misleading source.
- **R4 - Fail on executable access.** Real `localStorage`, `sessionStorage`,
  `indexedDB`, cookie, or cache access remains detectable.
- **R5 - Live fidelity.** The E2E still fetches `/pwa/assistant.js` from the
  live disposable stack and scans the served production asset.
- **R6 - No production behavior change.** Do not remove security comments or
  weaken the no-browser-storage policy to make the test pass.

## Acceptance Scenarios

```gherkin
Feature: Comment-aware PWA storage policy scanning

  Scenario: Policy comments do not fail the live route test
    Given assistant.js comments name localStorage as forbidden
    When the served executable source is inspected
    Then the comment token is ignored and the test passes

  Scenario: Executable browser storage access is rejected
    Given JavaScript calls localStorage.getItem outside a comment
    When the shared scanner and forbidden-pattern check run
    Then the executable reference is reported as a violation

  Scenario: Comment markers inside strings remain source text
    Given JavaScript contains an https URL or string with slash characters
    When comments are removed
    Then the string remains intact and following executable code is preserved
```

## Out Of Scope

- Changing the PWA auth/session model.
- Allowing any browser persistence for bearer or session material.
- Replacing live E2E with a mocked asset response.
