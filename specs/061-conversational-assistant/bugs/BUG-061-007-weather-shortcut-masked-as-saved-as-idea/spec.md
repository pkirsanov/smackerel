# BUG-061-007 — Spec: deterministic `/weather` shortcut

## Problem statement

The explicit `/weather <location>` slash command depends on the LLM emitting a
`weather_lookup` tool call. When the self-hosted local model fails to emit it, the
provenance gate masks the resulting provider-error as the capture-as-fallback
acknowledgement `saved as an idea — i'll surface it later.` — a wrong, confusing answer
for an explicit command.

## Expected behavior (BDD)

```gherkin
Feature: explicit /weather command is deterministic

  Scenario: SCN-061-007-01 — a valid location returns the forecast
    Given the weather skill is configured
    When the user sends "/weather <us-zip>"
    Then the reply is the forecast line with provider + timestamp attribution
    And the reply is NEVER "saved as an idea — i'll surface it later."
    And the LLM tool-call loop is not consulted for the explicit command

  Scenario: SCN-061-007-02 — a provider failure is reported honestly
    Given the weather provider is unavailable
    When the user sends "/weather <us-zip>"
    Then the reply is an honest "couldn't get the weather … please try again" line
    And the reply is NEVER "saved as an idea — i'll surface it later."

  Scenario: SCN-061-007-03 — a bare command asks for a location
    When the user sends "/weather" with no location
    Then the reply asks which location to check
    And the weather provider is not called
    And the reply is NEVER "saved as an idea — i'll surface it later."
```

## Requirements

- **FR-1** — An explicit `/weather <location>` shortcut MUST dispatch the weather lookup
  deterministically (location = the stripped shortcut tail), independent of any LLM
  tool-call emission.
- **FR-2** — On success the response body MUST be the forecast line and MUST carry exactly
  one external-provider Source (provider name + original upstream `retrieved_at`).
- **FR-3** — On provider failure or unreadable output the response MUST be an honest
  `provider_unavailable` line and MUST NOT set `CaptureRoute` or emit the capture-as-fallback
  acknowledgement.
- **FR-4** — A bare `/weather` (empty tail) MUST return a `slot_missing` prompt without
  calling the provider.
- **FR-5** — The change MUST be backward-compatible: when the fast-path seam is not wired,
  `/weather` keeps its prior LLM-routed behavior (no regression to existing tests).

## Non-goals

- Natural-language weather ("what's the weather in Paris?") continues through the LLM path;
  this bug only makes the **explicit** `/weather` command deterministic.
- Multi-day/forecast-window parsing from the shortcut tail (window is fixed to `now`).
