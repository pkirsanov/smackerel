# BUG-061-008 — Spec: honest execution errors + systemic prevention (P1–P5)

## The invariant (P1)

**A non-OK executor outcome MUST surface honestly and MUST NEVER be masked as
capture-as-fallback / "saved as an idea".**

- OK outcome + body + no valid sources → provenance capture-refusal (fabrication guard) —
  UNCHANGED.
- Non-OK outcome (`OutcomeProviderError`, `OutcomeTimeout`, `OutcomeSchemaFailure`,
  `OutcomeToolReturnInvalid`, `OutcomeInputSchemaViolation`, `OutcomeLoopLimit`,
  `OutcomeUnknownIntent`) → `Status=StatusUnavailable` + a set `ErrorCause` + a truthful
  body; `CaptureRoute=false`; the response is NOT re-canonicalised into the "saved as an
  idea" acknowledgement.

## Scenarios (BDD)

```gherkin
Feature: execution errors surface honestly across every requires_provenance scenario

  Scenario: SCN-061-008-01 — a provider error is honest, never "saved as an idea"
    Given a requires_provenance scenario (weather_query / retrieval_qa / recipe_search)
    When the executor returns OutcomeProviderError
    Then the response Status is unavailable with a provider-unavailable ErrorCause
    And the body is a truthful "couldn't do that right now" line
    And the response is NEVER "saved as an idea — i'll surface it later."
    And CaptureRoute is false

  Scenario: SCN-061-008-02 — a timeout is honest, never "saved as an idea"
    Given a requires_provenance scenario
    When the executor returns OutcomeTimeout
    Then the response is an honest unavailable error, never the capture acknowledgement

  Scenario: SCN-061-008-03 — genuine fabrication still refuses (guard preserved)
    Given an OK outcome that produced a body but no valid sources
    When the provenance gate runs
    Then it still refuses (capture-as-fallback) — the anti-fabrication guard is unchanged

  Scenario: SCN-061-008-04 — execution failures are observable
    When any scenario surfaces a non-OK outcome to the user
    Then a scenario+outcome-labelled metric is incremented so a dashboard/alert can see it
```

## Requirements

- **FR-1 (P1)** — The provenance gate runs ONLY on `OutcomeOK`. Non-OK outcomes keep the
  honest `StatusUnavailable`/`ErrorCause` the facade already computed and are not
  re-canonicalised into the capture acknowledgement.
- **FR-2 (P1)** — The truthful body for a provider/timeout failure is user-friendly
  ("the service is unavailable right now — please try again"), not a bare token.
- **FR-3 (P2)** — A cross-scenario invariant test covers every `requires_provenance`
  scenario × each error outcome and asserts honest surfacing (never `StatusSavedAsIdea`,
  never the capture body), plus the complementary OK+no-sources fabrication case still
  refuses. This is the mechanical regression gate.
- **FR-4 (P3)** — A scenario+outcome-labelled metric is emitted when a non-OK outcome is
  surfaced, so execution failures are visible on a dashboard/alert (not only via a user
  screenshot).
- **FR-5 (P4)** — The deterministic-dispatch seam (an explicit slash command dispatches its
  tool directly, never depending on LLM tool-call reliability — the BUG-061-007
  `WithWeatherLookup` pattern) is documented as the recommended approach for explicit
  commands with an unambiguous argument.
- **FR-6 (P5)** — The invariant ("execution errors are never rendered as capture/soft-refusal;
  explicit commands are deterministic") is encoded in the assistant design + the review
  checklist, with the FR-3 test as its mechanical enforcement.

## Non-goals

- Changing the anti-fabrication behaviour for OK-but-no-sources (that guard is correct).
- Mass-converting every scenario to deterministic dispatch (P4 is a documented pattern, not
  a blanket refactor).
