# BUG-061-008 — Execution errors are masked as "saved as an idea" (systemic)

- **Spec:** specs/061-conversational-assistant
- **Severity:** S2 (a whole class of real failures is silently hidden from users; each new
  scenario is a latent recurrence)
- **Discovered by:** operator (recurring pattern: "it comes up constantly after fixes")
- **Discovered at:** 2026-07-22
- **Related:** BUG-061-006 (duplicate ack — fixed), BUG-061-007 (`/weather` masked — fixed
  the weather *scenario*). This bug fixes the **mechanism** those two were instances of.

## The recurring pattern

Every prior fix patched **one scenario**. But "saved as an idea" for a failed request is
produced by a **shared masking mechanism** under all `requires_provenance` assistant
scenarios. So each scenario (`weather_query`, `retrieval_qa`, `recipe_search`) is a latent
copy of the same defect, surfacing one at a time.

## Root cause (grounded)

For a `requires_provenance` scenario, the facade high-band path runs the provenance gate
([internal/assistant/provenance/gate.go](../../../../../internal/assistant/provenance/gate.go))
**unconditionally** whenever the response has no sources — regardless of *why* there are no
sources:

- **OK outcome, body, no sources** = the LLM produced an answer with no citations →
  fabrication → gate refusal is CORRECT.
- **Non-OK outcome** (provider-error / timeout / no-tool-call / schema-failure) = an
  **execution failure**. The gate STILL rewrites it to `Status=StatusSavedAsIdea` +
  `CaptureRoute=true`, and `canonicalizeSuccessfulCaptureResponse`
  ([facade.go](../../../../../internal/assistant/facade.go)) then **clears `ErrorCause`**
  and forces `Body="saved as an idea — i'll surface it later."`.

So a genuine failure is laundered into a benign-looking "saved as an idea" with the error
cause discarded — invisible to the user AND to alerting. The `BS006` test literally ratifies
this masking today.

## Impact

- Users are told their failed request was "saved as an idea" — confusing and untruthful.
- Operators cannot distinguish a genuine capture from a masked failure.
- Every future `requires_provenance` scenario inherits the defect until individually patched.

## Expected

An execution **error** (non-OK outcome) MUST surface **honestly** (`StatusUnavailable` +
`ErrorCause` + a truthful "couldn't do that right now" message) and MUST NEVER be rendered
as "saved as an idea". The provenance capture-refusal is reserved for its real purpose:
an **OK** outcome that produced a body without valid sources (fabrication). This invariant
is enforced mechanically across all scenarios so it cannot silently recur.
