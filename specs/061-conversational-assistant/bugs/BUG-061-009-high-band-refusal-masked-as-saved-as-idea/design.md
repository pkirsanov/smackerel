# BUG-061-009 — Design

## Mechanism (verified against source + live logs)

```
/ask …  ->  open_knowledge (requires_provenance=true)
  agent runs -> Outcome=OK, Final=<answer>, cited_artifact_ids=∅
  source-assembler -> resp.Sources = ∅
  provenance.Enforce(OK, no sources, non-empty body):
      resp.Body   = "I don't have a sourced answer for that."   # honest
      resp.Status = StatusSavedAsIdea                            # WRONG shape
      resp.CaptureRoute = true
      # ErrorCause intentionally left empty
  canonicalizeSuccessfulCaptureResponse (CaptureRoute && StatusSavedAsIdea):
      resp.Body = "saved as an idea — i'll surface it later."    # overwrites honesty
      resp.ErrorCause = ""
  telegram buildTelegramRendering:
      openKnowledgeRefusalCauseFromError("") -> false            # honest renderer skipped
      Status != StatusUnavailable -> default body send
      -> "saved as an idea — i'll surface it later."             # the screenshot
```

Two refusal paths currently both end at a "saved as an idea" flavour:

- **Gate refusal** (OK + no sources): generic `captureFallbackAcknowledgement`
  (worst — no reason at all).
- **Agent refusal** (typed spec-064 `RefusalCause` in `ErrorCause`):
  `RenderRefusalWithCapture` → `"<reason> (saved as idea)"` (better — leads with
  a reason — but still contains the confusing suffix).

## Target contract

Enforce **INV-HB-REFUSAL** (spec.md). Concretely:

### 1. New honest error cause — `contracts.ErrNoGroundedAnswer`

`internal/assistant/contracts/response.go`: add
`ErrNoGroundedAnswer ErrorCause = "no_grounded_answer"` (+ `AllErrorCauses`).
Meaning: a `requires_provenance` scenario could not produce a sourced answer.
Distinct from `ErrProviderUnavailable` (upstream failed) and `ErrNoMatch`
(owned graph empty for the query).

### 2. Provenance gate rewrite shape — honest refusal, not capture

`internal/assistant/provenance/gate.go` `Enforce`, when it refuses
(requires_provenance && no valid sources && non-empty body):

| field | before | after |
|---|---|---|
| `Status` | `StatusSavedAsIdea` | `StatusUnavailable` |
| `ErrorCause` | unset | `ErrNoGroundedAnswer` |
| `Body` | `CanonicalRefusalBody` | `CanonicalRefusalBody` (unchanged, honest) |
| `CaptureRoute` | `true` | `false` |
| `Sources` | `nil` | `nil` |

Uniform across every requires_provenance scenario (weather_query, retrieval_qa,
recipe_search, open_knowledge). The gate's anti-fabrication job (strip the
uncited body) is unchanged; only the *shape it refuses into* becomes honest.

### 3. Facade canonicalize scoped to band-low

`internal/assistant/facade.go`: pass `band` into
`canonicalizeSuccessfulCaptureResponse`. It applies the capture acknowledgement
ONLY for `BandLow`. For a band-high response that still carries
`StatusSavedAsIdea` (defense-in-depth), it converts to the honest refusal shape
(`StatusUnavailable` + `ErrNoGroundedAnswer` + honest body) rather than the
capture ack. Band-low behavior is byte-for-byte unchanged.

### 4. Refusal taxonomy reworded (drop "— saved as an idea")

`internal/assistant/contracts/refusal.go`: the five cause strings that end with
"— saved as an idea" are reworded to honest headlines (default is already
honest and unchanged):

| cause | after |
|---|---|
| `RefusalBudgetExhausted` | "I couldn't complete that within the answer budget." |
| `RefusalToolUnavailable` | "A tool I needed isn't available right now." |
| `RefusalFabricatedSourceBlocked` | "I couldn't verify the sources I would have cited." |
| `RefusalInternalOnlyRestricted` | "That requires looking outside your knowledge graph, which is disabled." |
| `RefusalAmbiguousNotClarified` | "I couldn't decide what to look up." |
| `RefusalDefault` | "I don't have a sourced answer for that." (unchanged) |

### 5. Adapter render — honest headline, structural distinguishability

Telegram `internal/telegram/assistant_adapter`:

- `render_outbound.go`: add `ErrNoGroundedAnswer` to the "render `resp.Body`
  verbatim" fast-path (like `ErrModelNotSwitchable`) so a gate refusal shows the
  friendly body ("I don't have a sourced answer for that."), NOT the machine
  `"<skill>: <cause>"` form.
- `render_openknowledge.go`: `RenderRefusalWithCapture` drops the
  `OpenKnowledgeCaptureSuffix` — it returns `CanonicalRefusalBodyFor(cause)`
  only (rename to `RenderRefusal`). The `(saved as idea)` suffix is removed from
  the user-visible surface.
- G021 refusal-vs-answer distinguishability becomes **structural**: a refusal is
  `StatusUnavailable`/typed cause with no citations; a sourced answer is
  `StatusAnswered` with `[N]` citations. Tests assert structure, not the string.

WhatsApp `internal/whatsapp/assistant_adapter`: mirror the honest-refusal
rendering for `ErrNoGroundedAnswer`; the band-low capture ack
("saved as an idea — i'll surface it later.") is unchanged.

### 6. Silent capture preserved

The spec-074 no-ground capture (`runCaptureFallback` on `openKnowledgeNoGround`,
i.e. agent `status="refused"`) still persists the question as an idea silently.
It never sets the user-visible body to the capture acknowledgement on a
band-high turn. An OK-but-uncited gate refusal is NOT captured (an answerable-
looking question the model simply couldn't ground); the honest refusal is the
value.

## Cross-path invariant test (the mechanical class-killer)

`internal/assistant/facade_execution_error_honesty_test.go`:

- **Flip** `TestExecutionErrorHonesty_OKNoSourcesStillRefuses`: OK + no sources
  on every requires_provenance scenario now asserts `StatusUnavailable` +
  non-empty `ErrorCause` + body ≠ `captureFallbackAcknowledgement` (honest
  refusal), NOT `StatusSavedAsIdea`.
- **Extend** the invariant table so a single test asserts: across every
  requires_provenance scenario × {non-OK error, OK-uncited}, the response is
  never `StatusSavedAsIdea` and never `captureFallbackAcknowledgement`. Reverting
  any layer of the fix fails this test.
- A band-LOW unrouted case still asserts the legitimate `StatusSavedAsIdea` +
  capture ack (guards against over-correction).

## Files touched

- `internal/assistant/contracts/response.go` (new `ErrNoGroundedAnswer`)
- `internal/assistant/contracts/refusal.go` (reword 5 cause strings)
- `internal/assistant/provenance/gate.go` (honest refusal shape) + `gate_test.go`
- `internal/assistant/facade.go` (canonicalize band-scoping)
- `internal/assistant/facade_execution_error_honesty_test.go` (flip + extend invariant)
- `internal/assistant/facade_open_knowledge_no_ground_test.go` (canonicalize band param)
- `internal/telegram/assistant_adapter/render_outbound.go` (ErrNoGroundedAnswer body) + tests
- `internal/telegram/assistant_adapter/render_openknowledge.go` (drop suffix) + tests
- `internal/whatsapp/assistant_adapter/*` (mirror honest refusal) + goldens
- `docs/smackerel.md` §3.8.6 (extend the honesty invariant to OK-uncited + band-low-only capture)
- `.github/copilot-instructions.md` (extend the Assistant Response Honesty rule)

## Grounding-gap follow-up

Separately diagnose why open_knowledge grounded nothing for a question about the
user's own product (retrieval wiring vs. un-ingested smackerel docs vs. agent
search behavior) and route as its own bug/spec. Not fixed here.
