# BUG-061-009 — A high-band request that can't be answered is masked as "saved as an idea"

## Summary

`/ask how smackerel works as second brain or llm wiki?` replied
**"saved as an idea — i'll surface it later."** even though the open_knowledge
agent **succeeded** (`openknowledge.turn status=success termination=final`) — it
produced an answer that was then discarded, and the discard was rendered as a
benign capture instead of an honest "I couldn't produce a sourced answer."

This is the SAME masking family as BUG-061-006 / -007 / -008. Those fixes each
closed one path; this closes the remaining one and converts the ad-hoc
"saved as an idea" catch-all into a single enforced invariant so the class
cannot recur.

## Reproduction (live, home-lab bot)

1. On Telegram, send `/ask how smackerel works as second brain or llm wiki?`
2. Observed reply: `saved as an idea — i'll surface it later.`
3. Home-lab core logs for the same turn:
   - `openknowledge.turn status=success termination=final` (the agent answered)
   - `assistant_turn status=saved_as_idea` (the facade discarded the answer)

## Severity

S2 — a matched, executed request is answered with a misleading non-answer. It
looks like the user's *question* was filed away as an idea, hiding that the
system could not ground an answer. Recurring user-visible trust defect.

## Root cause

open_knowledge is a `requires_provenance` scenario. The agent produced an answer
with **no citations**, so the provenance gate correctly refuses to surface an
uncited body — but it rewrites the response to
`Status=StatusSavedAsIdea + CaptureRoute=true` and leaves `ErrorCause` unset.
`canonicalizeSuccessfulCaptureResponse` then overwrites the gate's already-honest
body (`"I don't have a sourced answer for that."`) with the generic capture
acknowledgement `"saved as an idea — i'll surface it later."`.

Two aggravating facts:

- **BUG-061-008 P1 only guarded NON-OK outcomes.** This turn was an OK success,
  so the P1 guard never applied. The OK-but-uncited path was deliberately left
  intact — and its P2 test `TestExecutionErrorHonesty_OKNoSourcesStillRefuses`
  asserts the OK+no-sources case *should* return `saved_as_idea`, test-locking
  the exact behavior reported here.
- **An honest renderer already exists but is bypassed.** The Telegram adapter's
  `RenderRefusalWithCapture` renders `"I don't have a sourced answer for that.
  (saved as idea)"` — but only when `ErrorCause` matches a spec-064 refusal
  cause. The gate leaves `ErrorCause` empty, so the response falls through to
  the generic capture-ack renderer, and even that honest renderer still appends
  the confusing `(saved as idea)` suffix.

## Scope of the fix

Convert "saved as an idea" from an ad-hoc catch-all into a single enforced
invariant: it is a **band-LOW-only** outcome (the input was not a request). A
**band-HIGH** turn (router matched a scenario and executed it) must render the
answer, an honest refusal, or an honest error — never the capture
acknowledgement, and never the `(saved as idea)` suffix. Refusal-vs-answer
distinguishability becomes **structural** (`Status`/`ErrorCause`), not a
user-visible string.

## Supersedes

BUG-061-008 `SCN-061-008-03` ("an OK outcome with no valid sources still refuses
→ `saved_as_idea`") is replaced by "an OK outcome with no valid sources refuses
**honestly** (`StatusUnavailable` + `ErrNoGroundedAnswer` + friendly body),
never `saved_as_idea`."

## Follow-up (NOT fixed here)

The deeper "second brain" question — why `/ask` about the user's own product
grounded nothing and cited no sources — is a retrieval/ingestion gap, diagnosed
and routed separately. Making the refusal honest does not make the bot able to
answer; that is a distinct investigation.
