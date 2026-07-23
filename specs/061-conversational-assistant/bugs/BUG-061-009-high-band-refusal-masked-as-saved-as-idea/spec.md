# BUG-061-009 — Specification

## Problem statement

A band-high (matched + executed) turn that cannot produce a grounded answer is
rendered as the band-low capture acknowledgement "saved as an idea — i'll
surface it later.", masking a refusal as a benign capture.

## Invariant (the contract this bug enforces)

**INV-HB-REFUSAL:** For a band-HIGH turn, the user-visible response is exactly
one of:

1. **Answer** — `Status=StatusAnswered` (+ body, + citations when present).
2. **Honest refusal** — `Status=StatusUnavailable` + a non-empty `ErrorCause`
   (e.g. `ErrNoGroundedAnswer`) + a friendly, honest body (e.g. "I don't have a
   sourced answer for that.").
3. **Honest error** — `Status=StatusUnavailable` + `ErrorCause` (provider /
   slot / internal) + a friendly truthful body (BUG-061-008 P1).

A band-HIGH turn MUST NEVER render `captureFallbackAcknowledgement`
("saved as an idea — i'll surface it later.") NOR the `(saved as idea)` suffix.

**"saved as an idea" is band-LOW-only.** `Status=StatusSavedAsIdea` +
`captureFallbackAcknowledgement` is legitimate only when the router did not match
a request (unrouted / low-confidence input) — i.e. the user dropped a thought,
not a question.

**Distinguishability is structural.** A refusal is distinguished from a sourced
answer by `Status`/`ErrorCause` (and absence of citations), NOT by a
user-visible "(saved as idea)" string.

**Silent capture is preserved but never the headline.** The spec-074 no-ground
capture (persisting the question as an idea for later) still runs silently where
it applies; it never sets the user-visible body to the capture acknowledgement
on a band-high turn.

## Behavioral scenarios (Gherkin)

```gherkin
Scenario: SCN-061-009-01 — open_knowledge OK-but-uncited answer refuses honestly
  Given an open_knowledge (requires_provenance) turn
  And the agent produced an answer with no valid sources (Outcome=OK)
  When the facade assembles the response
  Then Status is StatusUnavailable
  And ErrorCause is ErrNoGroundedAnswer
  And the body is the honest refusal "I don't have a sourced answer for that."
  And the response is NOT StatusSavedAsIdea
  And the body is NOT "saved as an idea — i'll surface it later."

Scenario: SCN-061-009-02 — no band-high requires_provenance path renders the capture ack
  Given any requires_provenance scenario
  And any high-band outcome that produces no valid sources (OK-uncited, provider error, timeout)
  When the facade assembles the response
  Then the body is never the capture acknowledgement
  And Status is never StatusSavedAsIdea
  And ErrorCause is non-empty

Scenario: SCN-061-009-03 — band-low unrouted input still captures as an idea
  Given an unrouted / low-confidence input (no scenario matched)
  When the facade assembles the response
  Then Status is StatusSavedAsIdea
  And CaptureRoute is true
  And the body is "saved as an idea — i'll surface it later."

Scenario: SCN-061-009-04 — a typed refusal cause renders an honest headline, no "(saved as idea)"
  Given an open_knowledge refusal carrying a typed spec-064 RefusalCause
  When the Telegram/WhatsApp adapter renders it
  Then the rendered text leads with the honest cause-specific reason
  And the rendered text does NOT contain "(saved as idea)"
  And a sourced answer remains structurally distinguishable (StatusAnswered + citations)

Scenario: SCN-061-009-05 — grounding gap diagnosed and routed (no code fix here)
  Given the open_knowledge agent cited nothing for a question about the user's own product
  When the gap is investigated
  Then the finding is documented and routed as a separate follow-up
  And this bug's honesty fix stands independently
```

## Out of scope

- Making `/ask` actually answer questions about the user's own product
  (retrieval/ingestion grounding). Diagnosed + routed as a follow-up.
- Changing the band-LOW capture behavior or its acknowledgement string.
