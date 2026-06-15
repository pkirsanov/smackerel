# User Validation — Spec 089 (Runtime Model Hot-Swap & Persistent Selection)

> **Convention (matches spec 088).** This file is authored at the **UX /
> planning phase — before any code exists** — so every acceptance item
> below is UNCHECKED `[ ]` (pending). An item flips to `[x]` only when the
> later implement → test → validate phases prove it in-repo, and the
> live-behavior items are confirmed by the operator against the downstream
> home-lab deploy (see Verification note). Anti-fabrication: nothing here
> is claimed as already-passing.
>
> Each item is phrased as something the **operator can actually check** on
> the live `/ask` surfaces (Telegram + web/HTTP). Items are grouped by the
> analyst's behavioral scenarios `SCN-089-A01..A13` (spec.md §5). This
> spec EXTENDS spec 088; the per-request override wording, the fail-loud
> rejection register, and the attribution primitive are carried forward
> verbatim and widened to the sticky + gather + persistent-default axes.
>
> **Home-lab reference values used throughout** (from `config/smackerel.yaml`
> after this spec ships): gather model `gemma4:26b`
> (`assistant.open_knowledge.llm_model_id`); standing synthesis default
> `deepseek-r1:32b` (was `deepseek-r1:7b`); switchable set
> `[deepseek-r1:32b, deepseek-r1:7b, gemma4:26b]`; ollama envelope raised
> `28G → ≥48G`. Tool-capable gather models on home-lab: `gemma4:26b`
> (default), `llama3.1:8b`.

## Acceptance checklist

### SCN-089-A01 — The committed-SST persistent default is applied to every `/ask` with no selection

- [ ] With no sticky preference and no `--model=`, a bare `/ask <question>`
  on home-lab is answered by `deepseek-r1:32b` — confirmable because the
  API envelope for the same question reports `"model": "deepseek-r1:32b"`,
  `"model_source": "default"`, and `/model` shows
  `deepseek-r1:32b — system default, active now`.
- [ ] The gather turns still use `gemma4:26b`: the API envelope reports
  `"gather_model": "gemma4:26b"`, `"gather_model_source": "default"`.
- [ ] The bare-default Telegram answer carries **no** `— model:` footer
  (the standing default is invisible-by-default; Principle 6 / NFR-4) — it
  looks exactly like a spec-087/088 baseline answer, only now produced by
  the 32b default.
- [ ] The answer is produced under the full trust perimeter (citations
  present; a fabricated-citation or zero-source case still refuses) —
  unchanged from spec 087/088.

### SCN-089-A02 — A sticky `/model <id>` persists across subsequent turns until changed

- [ ] After `/model deepseek-r1:7b`, two **later** bare `/ask <question>`
  turns from the same user are both answered by `deepseek-r1:7b` (each
  footer reads `— model: deepseek-r1:7b (your default)`), with no flag
  repeated on either turn.
- [ ] A different user's bare `/ask` issued in between is still answered by
  the system default `deepseek-r1:32b` — the sticky set changed only the
  setting user's default, not the SST baseline and not anyone else's
  default.

### SCN-089-A03 — `/model` (no arg) shows current + allowed; `/model default` resets to the SST default

- [ ] `/model` with no argument shows the caller's current **effective**
  model and marks it active, lists the full switchable set, names the
  **system default**, and states whether the caller's choice is
  **sticky-set** (`your default`) or **inherited** (`system default`).
- [ ] `/model default` (or `/model reset`) clears the caller's sticky
  preference and replies confirming the revert to the system default; the
  next bare `/ask` from that user is answered by `deepseek-r1:32b` again.

### SCN-089-A04 — A sticky preference is claim-bound to the authenticated user and never leaks across users

- [ ] User A's `/model deepseek-r1:7b` and User B's `/model` are
  independent: B never sees A's preference, and B's `/ask` uses the system
  default until B sets their own — proving the preference is keyed on the
  authenticated `actor_user_id` (Telegram `Bot.resolveActorUserID`; HTTP
  PASETO bearer), not shared process state.
- [ ] An HTTP `PUT /v1/agent/model` whose body attempts to name a
  different user id sets/reads only the PASETO-authenticated user's
  preference; the body-supplied user id is ignored — a spoofed actor id
  cannot set or read another user's preference (OWASP A01).

### SCN-089-A05 — Selection precedence is per-request override > per-user sticky > SST persistent default

- [ ] With sticky = `deepseek-r1:7b`, a one-off
  `/ask --model=gemma4:26b <question>` is answered by `gemma4:26b` (footer
  `— model: gemma4:26b (this question)`), and the **very next** bare
  `/ask` reverts to `deepseek-r1:7b` (footer
  `— model: deepseek-r1:7b (your default)`) — the per-request override did
  not mutate the sticky preference.
- [ ] With a sticky preference set and no per-request override, `/ask`
  uses the sticky model, not the system default — the operator can tell
  which level applied from the source tag in the footer / API
  `model_source`.

### SCN-089-A06 — The persistent-default footprint headroom is verified safe (not just the profile number)

- [ ] `./smackerel.sh config generate --env home-lab` succeeds **only**
  with `ollama_memory_limit` raised so `gemma4:26b` (gather) +
  `deepseek-r1:32b` (synthesis) are co-resident-consistent (owner figure:
  measured ~45824 MiB needs ≥48G); a config that leaves the envelope at
  `28G` fails loud, naming the offending model and the envelope
  (`validateModelEnvelopes`).
- [ ] The design's footprint-headroom decision is recorded — whether the
  profile-based envelope check suffices or a real-footprint-aware guard /
  an explicit synthesis `num_ctx` bound is required — so the standing 32b
  default cannot silently OOM the host alongside the ingestion pipeline at
  the pipeline's `per_query_token_budget` (128000).

### SCN-089-A07 — The gather (tool-calling) model is runtime-selectable and a non-tool-capable gather model is refused

- [ ] A gather override to a **tool-capable** model (e.g.
  `--gather-model=llama3.1:8b`, semantics per Fork C) is applied to the
  gather turns (the API envelope reports the selected `gather_model`), and
  the synthesis turn still resolves by the normal precedence.
- [ ] A gather override to a **non-tool-capable** model (e.g.
  `--gather-model=deepseek-r1:7b`, whose tool-calling is weak) is rejected
  fail-loud (`model_not_tool_capable`), names the tool-capable gather set,
  states it was **NOT** used, and never runs a gather turn with it.

### SCN-089-A08 — An off-allowlist / over-envelope selection is rejected fail-loud, identically on every surface, and never reaches the backend

- [ ] `/ask --model=gpt-4o <question>` (Telegram) and
  `POST /v1/agent/invoke {"model":"gpt-4o"}` (HTTP) both return the SAME
  rejection sentence — names `"gpt-4o"`, says it was **NOT** used and the
  request did **NOT** fall back to the default, lists the allowed set,
  names the default — with HTTP `400` `model_not_allowlisted`, and the
  operator can confirm (logs / Ollama) no request for `gpt-4o` was sent.
- [ ] The same is true for a **sticky** off-allowlist attempt
  (`/model gpt-4o`): rejected with the same register, **and** the reply
  states the caller's sticky preference was **NOT** changed.
- [ ] An over-envelope selection is rejected `model_over_memory_envelope`
  with the envelope-fitting allowed set + the raise-the-envelope opt-up,
  identically on both surfaces.

### SCN-089-A09 — No `<think>` / `<CITATIONS>` scaffolding leaks into the user body under any model

- [ ] Under `deepseek-r1:32b` (or any selected synthesis model), no
  `<think>` reasoning and no `<CITATIONS>` contract scaffolding appears in
  the user-visible body, and the stripped `<think>` content never becomes
  a citation — the spec-087 `<think>`-strip runs before citation parsing
  for every model.

### SCN-089-A10 — A blank forced-final synthesis is rescued by retry-before-salvage before the honest salvage

- [ ] A forced-final empty synthesis (the 32b Q6-blank class) triggers the
  escalated "write the verdict now, no `<think>`, no preamble" retry up to
  `synthesis_retry_budget` times; only if every retry is still
  empty/ungrounded does the honest snippet salvage fire — the user never
  receives a silently-empty answer from the standing default.

### SCN-089-A11 — Telegram + web/HTTP expose sticky + per-request + gather selection identically

- [ ] The same allowlisted sticky set, per-request override, and gather
  override behave identically on Telegram and HTTP — same applied
  model(s), same rejection shape/sentence, same attribution — proving one
  shared `modelswitch` validator, not per-surface re-implementations.

### SCN-089-A12 — Each answer is attributed to the model(s) AND the selection source

- [ ] Each answer surfaces which model produced each overridable turn AND
  the selection source (`default` / `sticky` / `per_request`): Telegram
  footer `— model: <id> (<source>)` (or the dual gather form when a gather
  override is active); the API envelope always carries `model` +
  `model_source` (+ `gather_model` + `gather_model_source`).
- [ ] Two answers to the SAME question from two different models or two
  different sources are unambiguously distinguishable by that attribution —
  the operator can never mix up which arm produced which answer.

### SCN-089-A13 — The persistent default is hot-swappable in prod via a documented config procedure

- [ ] Following the documented hot-swap procedure (below) changes the
  standing default for subsequent `/ask` invocations; the core boot log
  line `open-knowledge subsystem wired … synthesis_model=<new> …` names
  the new model; a live `/ask` API envelope shows `model: <new>`,
  `model_source: default`.
- [ ] After the swap, the trust perimeter and every other behavior are
  unchanged, and users with a sticky preference are unaffected (the swap
  moves only un-sticky users to the new default).

---

## Concrete affordance wording (operator-verifiable strings)

These are the exact user-facing strings the UX phase proposes. The owner
checks the live surfaces render this register (sentence-case, capital "I",
em-dash, honest, no emoji; the capitalized **NOT** is a deliberate
fail-loud emphasis carried forward from spec 088). Design resolves the §9
Forks but MUST NOT change the rejection shape, the attribution shape, the
claim-binding, or the cross-surface parity defined here.

### Telegram command shapes

| Affordance | Shape | Fork |
|------------|-------|------|
| Bare ask (unchanged) | `/ask <question>` | — |
| Per-request synthesis override (this turn only) | `/ask --model=<id> <question>` | 088 (carried forward) |
| Per-request gather override (this turn only) | `/ask --gather-model=<id> <question>` | C |
| Sticky set (my default) | `/model <id>` | B (F-STICKY) |
| Sticky show (mine + allowed + system default) | `/model` (no argument) | B |
| Sticky reset (back to system default) | `/model default` (or `/model reset`) | B |

> **Fork C note for design.** Whether `--model=` re-points BOTH turns or
> only synthesis, and whether the gather knob is a separate
> `--gather-model=` / `/gather-model` sticky, is design's call. The
> RECOMMENDED shape (above) keeps `--model=` = synthesis-only (byte-for-byte
> spec-088 semantics, no regression) and adds a SEPARATE `--gather-model=`
> for the gather turn, because a single combined flag would silently push a
> non-tool-capable synthesis model onto the gather turn (FR-8 hazard).

### `/model` with no argument — discovery + current state (home-lab, un-set user)

```
Your /ask model: deepseek-r1:32b (system default) — you have not set a personal model.

Models you can switch /ask to:
- deepseek-r1:32b — system default, active now
- deepseek-r1:7b — faster, lighter (about half the wait)
- gemma4:26b — gather model (also selectable for synthesis)

Set yours (sticky):   /model <id>
One-off (this turn):  /ask --model=<id> <question>
Restore the default:  /model default
```

After `/model deepseek-r1:7b`, the same command marks the sticky choice:

```
Your /ask model: deepseek-r1:7b (your default) — sticky, persists until you change it.

Models you can switch /ask to:
- deepseek-r1:32b — system default
- deepseek-r1:7b — your default, active now
- gemma4:26b — gather model (also selectable for synthesis)

…
Restore the default:  /model default
```

### `/model <id>` — set sticky preference (success)

```
OK — your /ask will use deepseek-r1:7b from now on. This is your personal default; it persists across questions until you change it.
/model default restores the system default (deepseek-r1:32b).
```

### `/model default` (or `/model reset`) — clear sticky preference (success)

```
OK — cleared your personal model. Your /ask now uses the system default (deepseek-r1:32b) again.
```

### Fail-loud rejection — off-allowlist sticky `/model <bad>` (pref UNCHANGED)

```
"badmodel" is not a switchable model. I did NOT set it as your default, and I did NOT change your current preference — nothing was sent to the model.
Switchable models: deepseek-r1:32b (system default), deepseek-r1:7b, gemma4:26b.
Your /ask model is unchanged: deepseek-r1:7b (your default).
Retry e.g. /model deepseek-r1:7b
```

> The last "unchanged" line names the caller's CURRENT effective model and
> source, so a failed sticky set can never be mistaken for a silent change
> (Principle 8 / SCN-089-A08). If the caller had no sticky preference, the
> line reads `Your /ask model is unchanged: deepseek-r1:32b (system default).`

### Fail-loud rejection — off-allowlist per-request `/ask --model=<bad>` (carried forward from 088, default updated)

```
"gpt-4o" is not a switchable model. I did NOT use it, and I did NOT fall back to the default — nothing was sent to the model.
Switchable models: deepseek-r1:32b (system default), deepseek-r1:7b, gemma4:26b.
Retry e.g. /ask --model=deepseek-r1:7b <your question>
```

### Fail-loud rejection — over-envelope (profiled but too large, carried forward from 088)

```
"<model>" needs more memory than this environment's model budget allows, so it isn't switchable here. I did NOT use it and did NOT fall back to the default — nothing was sent to the model.
Switchable models that fit: deepseek-r1:32b (system default), deepseek-r1:7b, gemma4:26b.
To use a larger model, raise the environment's Ollama memory envelope first (operator opt-up).
```

### Fail-loud rejection — gather override to a non-tool-capable model (F-TOOLMODEL, new)

```
"deepseek-r1:7b" can't be used as the gather model — it doesn't reliably make the tool calls /ask needs to search your graph and the web. I did NOT use it for gathering, and I did NOT fall back silently — nothing was sent to the model.
Tool-capable gather models: gemma4:26b (default), llama3.1:8b.
Your synthesis model is unaffected — set that with /ask --model=<id> or /model <id>.
```

### Attribution footer (Telegram) — single-turn form

| Selection source | Footer | When shown |
|------------------|--------|------------|
| System default | *(no footer)* | bare `/ask`, no sticky, no override (Principle 6 / NFR-4); source visible via `/model` + API |
| Per-user sticky | `— model: deepseek-r1:7b (your default)` | a sticky preference is in effect |
| Per-request override | `— model: deepseek-r1:7b (this question)` | a `--model=` was supplied for this turn |

- The footer is shown on Telegram **only when a non-default selection was
  used** (carried forward from spec 088 / spec.md §11 Principle 6). The
  pure system-default path stays footer-free; the operator confirms the
  effective model any time with `/model`.
- On the honest-salvage path the footer still reads `— model: <id>
  (<source>)` (NOT "answered by"), so it never contradicts the "I searched
  but couldn't directly answer" framing (spec 088 carried forward).

### Attribution footer (Telegram) — dual form when a gather override is active

```
— gather: llama3.1:8b (this question) · synth: deepseek-r1:7b (your default)
```

- The dual `gather: … · synth: …` line is used WHENEVER a gather override
  is in effect — that is the only time the gather turn becomes an operator-
  chosen, A/B-relevant variable. Each half carries its own source tag.
- In the dual form the synth half names its source even when it is the
  system default (e.g. `synth: deepseek-r1:32b (system default)`), because
  the footer is already present and honesty requires naming both turns.

### API request + envelopes (parity with Telegram)

Request — `POST /v1/agent/invoke` (both override fields optional; absent ⇒
resolve by precedence):

```
POST /v1/agent/invoke
Content-Type: application/json

{
  "scenario_id": "open_knowledge",
  "raw_input": "what is a better place to grow pomegranate, wa-town-A or wa-town-B, wa?",
  "model": "deepseek-r1:7b",
  "gather_model": "gemma4:26b"
}
```

Success — `200 OK` (extends the spec-088 envelope; `model` + `model_source`
+ `gather_model` + `gather_model_source` are ALWAYS present, even on the
pure baseline):

```
HTTP/1.1 200 OK
{
  "status": "success",
  "body": "wa-town-B is the better choice …",
  "termination": "final",
  "model": "deepseek-r1:7b",
  "model_source": "per_request",
  "gather_model": "gemma4:26b",
  "gather_model_source": "default",
  "sources": [ { "kind": "web", "url": "https://…", "title": "WSU Extension" } ]
}
```

(`model_source` / `gather_model_source` ∈ `{ "default", "sticky",
"per_request" }`. A bare baseline call returns `model: "deepseek-r1:32b"`,
`model_source: "default"`.)

Rejection — `400 Bad Request` (same envelope as spec 088, plus a
`rejected_turn` discriminator so a client knows which turn the selection
targeted; `message` is the SAME sentence the Telegram surface renders —
parity):

```
HTTP/1.1 400 Bad Request
{
  "status": "rejected",
  "error_code": "model_not_tool_capable",
  "rejected_model": "deepseek-r1:7b",
  "rejected_turn": "gather",
  "allowed_models": ["gemma4:26b", "llama3.1:8b"],
  "default_model": "deepseek-r1:32b",
  "message": "\"deepseek-r1:7b\" can't be used as the gather model — it doesn't reliably make the tool calls /ask needs to search your graph and the web. It was NOT used for gathering and the request did NOT fall back silently — nothing was sent to the model. Tool-capable gather models: gemma4:26b (default), llama3.1:8b."
}
```

(`error_code` ∈ `{ "model_not_allowlisted", "model_over_memory_envelope",
"model_not_tool_capable" }`; `rejected_turn` ∈ `{ "synthesis", "gather" }`.
The first two reason codes reuse the spec-088 `modelswitch.Rejection`
verbatim; `model_not_tool_capable` is the F-TOOLMODEL addition.)

### API sticky-preference affordance (mirrors Telegram `/model`; claim-bound)

The preference is keyed on the PASETO-authenticated user (`actor_user_id`),
NEVER a request-body user id (SCN-089-A04). A body that names another user
is ignored.

Show — `GET /v1/agent/model`:

```
GET /v1/agent/model
Authorization: Bearer <PASETO>

→ 200 OK
{
  "effective_model": "deepseek-r1:32b",
  "source": "default",
  "sticky_model": null,
  "system_default": "deepseek-r1:32b",
  "allowed_models": ["deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"]
}
```

Set sticky — `PUT /v1/agent/model`:

```
PUT /v1/agent/model
Authorization: Bearer <PASETO>
{ "model": "deepseek-r1:7b" }

→ 200 OK
{ "status": "set", "sticky_model": "deepseek-r1:7b", "source": "sticky", "system_default": "deepseek-r1:32b" }

→ 400 Bad Request  (off-allowlist)
{ … same rejection envelope as above; the user's sticky_model is UNCHANGED … }
```

Reset sticky — `DELETE /v1/agent/model`:

```
DELETE /v1/agent/model
Authorization: Bearer <PASETO>

→ 200 OK
{ "status": "reset", "sticky_model": null, "effective_model": "deepseek-r1:32b", "source": "default" }
```

---

## Precedence + attribution wording (SCN-089-A05 / A12 — must be unmistakable)

The effective synthesis model for any invocation is resolved
deterministically:

```
per-request --model=  (this turn only)   >   my sticky /model pref   >   system SST default
        (this question)                            (your default)             (system default)
```

The source label THREADS the chosen level all the way to the user so they
can always tell WHICH level applied and WHY:

| Resolved level | Telegram source tag | API `model_source` | Confirmed by |
|----------------|---------------------|--------------------|--------------|
| per-request `--model=` | `(this question)` | `per_request` | footer + envelope |
| per-user sticky `/model` | `(your default)` | `sticky` | footer + envelope + `/model` |
| system SST default | *(no footer; shown in `/model`)* | `default` | `/model` + envelope |

Honesty rule on the spec-087 salvage path: the footer/source names the
model that **produced the salvage**, framed as `— model: <id> (<source>)` —
never "answered by", because the body explicitly says it could not directly
answer. The source still tells the operator which model/level failed to
synthesize, which is exactly the A/B signal they need.

---

## Operator hot-swap runbook snippet (Fork D — recommended ~15s core-recreate)

> **Mechanism (recommended).** Edit the committed SST default → regenerate
> the env bundle (fail-loud) → recreate the core service only (~15s),
> proven during the A/B ("core healthy in 15s", `--no-deps`, image digests
> taken from the running container). Downtime ≈ 15s for the `/ask` core;
> the ingestion pipeline is a separate service and is untouched. True
> zero-downtime config-hot-reload is deferred unless it proves cheap (Fork
> D recommends right-sizing for a single-operator product). The host-
> specific recreate is owned by the operator deploy overlay; this repo owns
> only the committed-SST edit + the `config generate` step.

```
# Hot-swap the standing /ask synthesis model in prod (home-lab)

# 1. Edit the committed SST default for the environment:
#    config/smackerel.yaml →
#      environments.home-lab.assistant_open_knowledge_synthesis_model_id: "deepseek-r1:32b"
#    If the new model needs more memory, RAISE the envelope FIRST:
#      environments.home-lab.ollama_memory_limit: "48G"
#    (config generation fails loud if the co-resident envelope is busted — SCN-089-A06)

# 2. Regenerate the env bundle (fail-loud SST; no hidden defaults):
./smackerel.sh config generate --env home-lab

# 3. Ensure the new model is pulled on the host, then the deploy overlay
#    recreates ONLY the core service (no rebuild; digests from the running
#    images; ~15s). The recreate command itself lives in the operator
#    overlay, not in this repo (deployment-ownership boundary / C5).

# 4. Verify the swap took effect — two independent checks:
#    a. core boot log names the new synthesis model:
#       open-knowledge subsystem wired … synthesis_model=deepseek-r1:32b switchable_models=[…]
#    b. a live /ask API envelope shows the new default:
#       "model": "deepseek-r1:32b", "model_source": "default"
#    (users with a sticky /model preference are unaffected — the swap moves
#     only un-sticky users to the new standing default)
```

---

## Latency expectation-setting (the A/B reality in the UX)

Making `deepseek-r1:32b` the standing default makes every bare `/ask`
~1.9× slower (mean ~565s ≈ 9–10 min vs ~298s ≈ 5 min for 7b). Both are
already multi-minute — this is a research/recall assistant, not a
chat-latency product.

- **The existing "thinking…" affordance suffices** — no new progress UI is
  warranted (don't over-engineer). The `/ask` worst-case envelope is
  already the documented `WriteTimeout = (max_iterations +
  synthesis_retry_budget) × llm_timeout_ms = (6 + 1) × 600s = 4200s`, which
  bounds a slow 32b synthesis turn; a single 32b synthesis roundtrip must
  be verified (NFR-2) to stay within `llm_timeout_ms` (600000 ms).
- **The speed escape hatch is in-product, not a redeploy:** a user who
  wants the faster arm sets `/model deepseek-r1:7b` once (sticky) or uses
  `/ask --model=deepseek-r1:7b <q>` for one turn — that is the whole point
  of keeping 7b switchable.
- A **gather** override to a slower tool model multiplies across up to
  `max_iterations` turns; design MUST keep the envelope honest or update
  the documented bound (NFR-2 / C4), not hide it.

---

## UX input to the design forks (§9)

- **Fork A (default choice — owner DECIDED 32b).** UX confirms the
  standing default surfaces honestly as `deepseek-r1:32b (system default)`
  in `/model` + the API envelope, and stays invisible on the bare Telegram
  path (Principle 6). UX defers to design on the §2 footprint-headroom
  guard; the only UX requirement is that an over-envelope config fails loud
  with the named model + envelope (SCN-089-A06 wording above).
- **Fork B (per-user store).** UX needs exactly: set / get-one / delete for
  the authenticated user, on the `/ask` hot path. Claim-binding
  (`actor_user_id`) is a UX-visible contract (SCN-089-A04) — the store must
  be keyed on the authenticated identity, never a body field. The
  recommended REST surface (`GET`/`PUT`/`DELETE /v1/agent/model`) mirrors
  the Telegram `/model` semantics 1:1.
- **Fork C (gather-override semantics).** UX RECOMMENDS a SEPARATE
  `--gather-model=` flag / API `gather_model` field (and an optional sticky
  gather knob) rather than overloading `--model=` to move both turns,
  because the gather turn has a hard tool-capability constraint (FR-8) a
  combined flag would silently violate. The dual-attribution footer
  (`gather: … · synth: …`) and the `model_not_tool_capable` rejection are
  the UX outputs design must wire.
- **Fork D (hot-swap mechanism).** UX RECOMMENDS documenting the ~15s
  core-recreate as the accepted hot-swap (the runbook above) and deferring
  true zero-downtime reload. The only UX requirements are the two verify
  signals: the boot-log `synthesis_model=` line and the live-envelope
  `model` / `model_source`.

---

## A/B operator journey (the thing this feature exists to enable)

The owner can run this end-to-end on home-lab once the feature ships and
the downstream devops dispatch has deployed it:

1. Confirm the standing default: `/model` shows
   `deepseek-r1:32b — system default, active now`; a bare `/ask <Q>` is
   answered by 32b (API envelope `model: deepseek-r1:32b, model_source:
   default`).
2. **Speed arm, one-off** — `/ask --model=deepseek-r1:7b <Q>`. Footer reads
   `— model: deepseek-r1:7b (this question)`. The next bare `/ask` reverts
   to the 32b default (precedence proven).
3. **Speed arm, sticky** — `/model deepseek-r1:7b`, then ask without flags;
   every answer footers `— model: deepseek-r1:7b (your default)` until
   `/model default`.
4. **Gather arm (Fork C)** — `/ask --gather-model=llama3.1:8b <Q>`; the dual
   footer `— gather: llama3.1:8b (this question) · synth: deepseek-r1:32b
   (system default)` makes both turns attributable.
5. **Hot-swap arm** — follow the runbook to move the standing default; the
   boot log + a fresh bare-ask envelope confirm the swap; a user who had
   set a sticky preference is unaffected.
6. Confirm throughout: no redeploy happened between the per-request/sticky
   arms; the SST baseline never changed at runtime; off-allowlist attempts
   on either surface were rejected fail-loud with nothing sent to the
   backend.

The interaction flows backing this journey are diagrammed in
[spec.md → User Flows](spec.md#user-flows).

---

## Verification note

This spec terminates at **validated-in-repo** (state.json
`planningOnlyJustification` + constraint C9). The live home-lab deploy
(persist the raised envelope + the `deepseek-r1:32b` standing default +
pull-on-deploy + the live re-verify) is a separate downstream
`bubbles.devops` dispatch AFTER the isolated push + CI + apply. The owner
re-checks the live-behavior boxes above against that run. No live-stack
result is fabricated here; the UX phase authors the contract, not the
proof.
