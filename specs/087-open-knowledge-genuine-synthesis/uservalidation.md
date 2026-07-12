# User Validation — Spec 087 (Open-Knowledge Genuine Synthesis)

> Items are CHECKED `[x]` by default when created (already validated via the
> implementation + test evidence in `report.md`). The owner UNCHECKS `[ ]` an
> item to report that the behavior is broken in live use. An unchecked item is
> a user-reported regression and is BLOCKING for further work.

## Checklist

_Live-behavior expectations (self-hosted `/ask`, gemma4:26b gather + deepseek-r1:7b synthesis):_

- [x] The motivating comparison question ("what is a better place to grow
  pomegranate, wa-town-A or wa-town-B, wa?") produces an actual
  synthesized comparison VERDICT — it states which location is better for
  pomegranates and why — instead of "I searched but couldn't directly answer …"
  followed by a per-source snippet wall.
- [x] When sources disagree ("thrives down to 0 degrees" vs "cannot stand
  freezing"), the answer RECONCILES or caveats the conflict in the verdict — it
  never pastes two contradictory snippets side by side.
- [x] The synthesis turn runs a reasoning model (deepseek-r1:7b on self-hosted);
  its `<think>` chain-of-thought never appears in the reply.
- [x] When the first synthesis attempt comes back empty/ungrounded, the agent
  retries once with a stronger prompt before any fallback — the user does not
  see a salvage wall just because the model paused.
- [x] When the agent genuinely cannot synthesize (retries exhausted), the reply
  is still honest — it says it searched and could not directly answer, then
  shows the raw findings — never a stitched snippet wall dressed as a verdict.
- [x] A normal, answerable factual / unit / recipe question still returns a
  grounded, cited answer (no regression of the happy path) within the latency
  envelope.
- [x] The cite-back / provenance trust contract is intact: no fabricated URLs
  (including any that appeared inside a `<think>` block), no zero-source
  "answers", citations still hash-match the tool trace.
- [x] The tool-calling model is unchanged (gemma4:26b self-hosted / gemma3:4b dev);
  only the forced-final synthesis turn uses the reasoning model.

## Verification note

This spec terminates at **validated-in-repo**. The decisive self-hosted
re-verification of the pomegranate query is performed by the downstream
`bubbles.devops` dispatch AFTER the isolated push + CI + apply (build new signed
images carrying the synthesis-turn split + `deepseek-r1:7b` pull + bundle that
sets `assistant_open_knowledge_synthesis_model_id`). The owner re-checks the
boxes above against that live run. No live-stack result is fabricated here.
