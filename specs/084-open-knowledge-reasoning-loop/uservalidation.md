# User Validation — Spec 084 (Open-Knowledge Reasoning Loop)

> Items are CHECKED `[x]` by default when created (already validated via the
> implementation + test evidence in `report.md`). The owner UNCHECKS `[ ]` an
> item to report that the behavior is broken in live use. An unchecked item is
> a user-reported regression and is BLOCKING for further work.

## Live-behavior expectations (self-hosted `/ask`, gemma4:26b)

- [x] A comparison question ("what is a better place to grow pomegranate, X or Y?")
  produces an answer that addresses BOTH places and reaches a comparison
  verdict — not a per-source recap ("here is what each link said").
- [x] When sources disagree (e.g. "thrives to 0F" vs "cannot stand freezing"),
  the answer reconciles or caveats the conflict — it never pastes two
  contradictory snippets side by side as if both are the answer.
- [x] A "why" / multi-hop question is allowed to drill in with more than one
  distinct web search before the agent answers (it is no longer pushed to
  answer after the first search).
- [x] When the agent genuinely cannot answer, the reply is honest — it says it
  searched and could not directly answer, then shows the raw findings — rather
  than presenting a stitched snippet wall as a confident answer.
- [x] A normal, answerable factual / unit / recipe question still returns a
  grounded, cited answer (no regression of the happy path) within the existing
  latency envelope.
- [x] The cite-back / provenance trust contract is intact: no fabricated URLs,
  no zero-source "answers", citations still hash-match the tool trace.
- [x] The deployed model is unchanged (gemma4:26b on self-hosted, gemma3:4b on dev).

## Verification note

This spec terminates at **validated-in-repo**. The live self-hosted re-verification
of the pomegranate query (turn `73900d2089b6a557` motivating case) is performed
by the downstream `bubbles.devops` dispatch AFTER the isolated push + CI + apply.
The owner re-checks the boxes above against that live run.
