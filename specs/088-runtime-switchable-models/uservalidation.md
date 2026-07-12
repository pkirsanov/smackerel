# User Validation — Spec 088 (Runtime-Switchable Models)

> **Convention (note the difference from spec 087).** Spec 087's items were
> created CHECKED `[x]` because they were authored after implementation +
> test evidence existed. Spec 088 is authored here at the **UX / planning
> phase — before any code exists** — so every item below is UNCHECKED
> `[ ]` (pending). An item flips to `[x]` only when the later
> implement → test → validate phases prove it in-repo, and the
> live-behavior items are confirmed by the operator against the downstream
> self-hosted A/B run (see Verification note). Anti-fabrication: nothing here
> is claimed as already-passing.
>
> Each item is phrased as something the **operator can actually check** on
> the live `/ask` surfaces. Items are grouped by the analyst's behavioral
> scenarios `SCN-088-A01..A08` (spec.md §4).

## Acceptance checklist

### SCN-088-A01 — A valid allowlisted override is applied, and the chosen model answers

- [ ] Sending `/ask --model=deepseek-r1:7b <question>` on Telegram returns
  an answer whose attribution footer reads `— model: deepseek-r1:7b` — i.e.
  the override actually changed the model that produced the answer.
- [ ] Calling `POST /v1/agent/invoke` with `"model": "deepseek-r1:7b"`
  returns an envelope with `"model": "deepseek-r1:7b"`.
- [ ] Immediately afterward, a **bare** `/ask <question>` (no flag) is
  answered by the baseline model — the override did not "stick" to the
  process or change anyone else's answer (SST baseline untouched).

### SCN-088-A02 — An off-allowlist override is rejected fail-loud and never reaches the backend

- [ ] `/ask --model=gpt-4o <question>` returns the rejection text, NOT an
  answer: it names `"gpt-4o"`, says it was **NOT** used and there was
  **NO** fall-back to the default, lists the allowed models, names the
  default, and gives a copy-paste retry.
- [ ] No answer body and no citations are shown for the rejected request —
  the operator cannot mistake the rejection for a (wrong-model) answer.
- [ ] The API form returns `HTTP 400` with
  `"error_code": "model_not_allowlisted"`, the structured
  `rejected_model` / `allowed_models` / `default_model`, and a `message`
  that is the SAME sentence as the Telegram rejection.

### SCN-088-A03 — No override leaves baseline behavior byte-for-byte unchanged

- [ ] A bare `/ask <question>` on Telegram looks EXACTLY as it does today
  (spec 087): same answer/citation shape, and **no** `— model:` footer is
  added.
- [ ] An API `/ask` call with no `model` field behaves exactly as today;
  the envelope's `model` field reports the resolved baseline (structured
  metadata only — it is not surfaced as human chat noise on Telegram).

### SCN-088-A04 — The answer is attributed to the model that produced it

- [ ] When an override was used, the operator can always tell which model
  produced the answer (Telegram footer `— model: <id>`; API `model` field).
- [ ] Asking the SAME question twice with two different overrides yields
  two answers whose footers carry two different model ids — the two A/B
  arms are unambiguously distinguishable.
- [ ] On the honest-salvage path ("I searched but couldn't directly
  answer…"), the footer still names the model that produced the salvage —
  and the footer never contradicts the salvage framing (it reads
  `— model: <id>`, not "answered by").

### SCN-088-A05 — Every trust contract holds under an overridden model

- [ ] Under an allowlisted override, a fabricated citation is still
  rejected (cite-back verifier) — the operator never sees an un-grounded
  URL just because a different model was selected.
- [ ] Under an allowlisted override, a zero-source response is still
  refused-with-capture (provenance gate), and capture-as-fallback still
  fires.
- [ ] When the override targets a reasoning model on the synthesis turn,
  its `<think>` chain-of-thought never appears in the reply and never
  becomes a citation.

### SCN-088-A06 — Telegram and web/HTTP `/ask` validate and apply the override identically

- [ ] The same allowlisted override produces the same applied behavior on
  both surfaces (same model answers).
- [ ] The same off-allowlist override produces the same rejection on both
  surfaces — same allowed set, same default, same human sentence.
- [ ] The allowlist is the SAME on both surfaces (one shared validator) —
  a model switchable on Telegram is switchable on the API, and vice versa.

### SCN-088-A07 — An untrusted / un-profiled / over-envelope model never passes through

- [ ] An arbitrary string (e.g. `/ask --model=totally-made-up <q>`) is
  rejected before any backend call — the operator can confirm (via logs or
  Ollama) that no request for that model was ever sent.
- [ ] A profiled but oversize model (e.g. `deepseek-r1:32b` on an
  environment whose Ollama envelope cannot hold it) is rejected as
  over-envelope, and the operator is told the allowed, envelope-fitting
  set plus that raising the envelope is the opt-up path.

### SCN-088-A08 — The latency envelope stays honest under a slower switched model

- [ ] Selecting a slower allowlisted model still returns within the
  documented `/ask` timeout envelope — it does not hang past the bound or
  fail silently.
- [ ] If the compare-both affordance (Fork C) ships, the operator is told
  up front that it "takes about 2× a normal ask," and the two-pass run
  stays within (or per an explicitly updated) documented timeout bound.

---

## Concrete affordance wording (operator-verifiable strings)

These are the exact user-facing strings the UX phase proposes. The owner
checks the live surfaces render this register (sentence-case, capital "I",
em-dash, honest, no emoji; the capitalized **NOT** is a deliberate
fail-loud emphasis).

### Telegram command shapes

| Affordance | Shape | Fork |
|------------|-------|------|
| Per-request override | `/ask --model=<id> <question>` | A (per-request) |
| Sticky set | `/model <id>` | A (sticky) |
| Sticky reset | `/model default` (or `/model reset`) | A (sticky) |
| Discover allowed set | `/model` (no argument) | — |
| Compare both | `/compare <question>` or `/ask --compare=<a>,<b> <question>` | C (optional) |
| Baseline (unchanged) | `/ask <question>` | — |

### Model-list / discovery (`/model` with no argument)

```
Models you can switch /ask to:
- gemma4:26b — baseline default, active now
- deepseek-r1:7b

Switch sticky: /model <id>
One-off:       /ask --model=<id> <question>
Restore baseline: /model default
```

(The "active now" marker moves to whichever model is currently in
effect — the sticky choice, or the baseline default when no sticky is set.)

### Fail-loud rejection — off-allowlist

```
"gpt-4o" is not a switchable model. I did NOT use it, and I did NOT fall back to the default — nothing was sent to the model.
Switchable models: gemma4:26b (default), deepseek-r1:7b.
Retry e.g. /ask --model=deepseek-r1:7b <your question>
```

### Fail-loud rejection — over-envelope (profiled but too large)

```
"deepseek-r1:32b" needs more memory than this environment's model budget allows, so it isn't switchable here. I did NOT use it and did NOT fall back to the default — nothing was sent to the model.
Switchable models that fit: gemma4:26b (default), deepseek-r1:7b.
To use a larger model, raise the environment's Ollama memory envelope first (operator opt-up).
```

### Attribution footer (recommended canonical form)

```
— model: deepseek-r1:7b
```

- Shown on Telegram **only when an override was used** (or in compare
  mode); a bare baseline `/ask` shows no footer.
- The API envelope **always** carries a `model` field (structured
  metadata, not chat noise).
- `— answered by <id>` is a friendlier **success-path-only** alternative
  design may adopt, but `— model: <id>` is recommended as the single
  primitive because it is honest on the salvage path too.

### API request + envelopes (parity with Telegram)

Request:

```
POST /v1/agent/invoke
{ "scenario_id": "open_knowledge", "raw_input": "<question>", "model": "deepseek-r1:7b" }
```

Success (`200`):

```
{ "status": "success", "body": "…", "termination": "final", "model": "deepseek-r1:7b", "sources": [ … ] }
```

Off-allowlist (`400`):

```
{
  "status": "rejected",
  "error_code": "model_not_allowlisted",
  "rejected_model": "gpt-4o",
  "allowed_models": ["gemma4:26b", "deepseek-r1:7b"],
  "default_model": "gemma4:26b",
  "message": "\"gpt-4o\" is not a switchable model. It was NOT used and the request did NOT fall back to the default — nothing was sent to the model. Switchable models: gemma4:26b (default), deepseek-r1:7b."
}
```

(Over-envelope uses the same shape with `error_code: "model_over_memory_envelope"`.)

---

## A/B operator journey (the thing this feature exists to enable)

The owner can run this end-to-end on self-hosted once the feature ships and
the downstream devops dispatch has deployed it:

1. Pick the motivating question (spec 087): *"what is a better place to
   grow pomegranate, wa-town-A or wa-town-B, wa?"*
2. **Arm A** — `/ask --model=gemma4:26b <question>`. Read the answer and
   its footer `— model: gemma4:26b`.
3. **Arm B** — send the SAME question with `/ask --model=deepseek-r1:7b
   <question>`. Read the answer and its footer `— model: deepseek-r1:7b`.
4. Compare the two attributed verdicts side by side — the footers make it
   impossible to mix up which model produced which answer.
5. (Optional, if Fork C ships) run `/compare <question>` once and read the
   two `[ <model> ]`-labeled answers in a single reply, accepting the
   "~2× a normal ask" latency caveat.
6. Confirm: at no point did redeploying happen between arms; the SST
   baseline never changed; a bare `/ask` in between still used the
   baseline model.

The interaction flows backing this journey are diagrammed in
[spec.md → User Flows](spec.md#user-flows).

---

## Verification note

This spec terminates at **validated-in-repo** (state.json
`planningOnlyJustification` + constraint C7). The decisive, live
`gemma4:26b`-vs-`deepseek-r1:7b` A/B on self-hosted hardware — the proof
spec 087 could not run on dev (no GPU/Ollama daemon) — is performed by a
separate downstream `bubbles.devops` dispatch AFTER the isolated push +
CI + apply. The owner re-checks the live-behavior boxes above against that
run. No live-stack result is fabricated here; the UX phase authors the
contract, not the proof.
