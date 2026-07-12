# Design 089 — Runtime Model Hot-Swap & Persistent Selection

> Design authored by `bubbles.workflow (parent-expanded)` during the
> `analyze-design-plan` prelude of a `full-delivery` run (no specialist
> sub-agent dispatch available in this runtime; same precedent as specs
> 084 / 087 / 088). Reads the analyst `spec.md` (FR-1..FR-16 / NFR-1..NFR-7
> / C1..C9 / SCN-089-A01..A13 / forks A-D), the UX `uservalidation.md` +
> `spec.md` `## UI Wireframes` / `## User Flows`, the A/B evidence
> ([`docs/experiments/open-knowledge-synthesis-model-ab.md`](../../docs/experiments/open-knowledge-synthesis-model-ab.md)),
> and the dependency design 088 (house style + the per-request model seam
> this builds on).
>
> Order: **Design Brief** (human steering checkpoint) → **Current Truth**
> (solution-blind, grounded in committed code) → **Decision Record**
> (Forks A/B/C/D, each decision + rationale + recorded alternative, incl.
> the §2 footprint resolution) → **Implementation Design** (precedence
> resolver, pref store + migration + claim-binding, gather carrier +
> tool-capability guard, attribution + source wiring, persistent-default
> SST, output-hygiene/reliability, multi-surface parity seam, latency,
> footprint headroom) → **Trust-Invariant Preservation** → **Change
> Manifest** → **Contracts / Data Model / Authz** → **Testing Strategy** →
> **Complexity Tracking** → **Findings**.
>
> **Extends, does NOT amend** spec 088. Every spec-064/084/087/088 trust
> and synthesis behaviour is preserved verbatim and made
> model-/selection-agnostic. The persistent default is committed SST
> (build-once / deploy-many); the sticky preference is per-user runtime
> STATE (DB) — **never** a runtime SST mutation (C6).

---

## Design Brief

A code owner can review this ~50-line brief and catch a wrong pattern or
direction BEFORE the full design + plan are generated.

**Current State.** Spec 088 shipped the **per-request** synthesis-model
override: a typed `AssistantMessage.ModelOverride` (Telegram `--model=`)
/ `AgentInvokeRequest.Model` (HTTP) is validated by ONE shared
`modelswitch.Allowlist.Resolve` ([allowlist.go](../../internal/assistant/openknowledge/modelswitch/allowlist.go)),
applied via a per-invocation `Agent.WithModelOverride` clone
([agent.go:266](../../internal/assistant/openknowledge/agent/agent.go#L266)),
and attributed back through `TurnResult.Model`. The override re-points
the spec-087 **synthesis turn only**; the gather turns keep
`cfg.Model`. self-hosted synthesis default is `deepseek-r1:7b`; switchable =
`[gemma4:26b, deepseek-r1:7b]`; `ollama_memory_limit: 28G`. There is **no
persistent-default change path**, **no per-user store** (only the PASETO
minter [per_user_token.go](../../internal/telegram/per_user_token.go)),
**no gather override**, and **no documented prod hot-swap**.

**Target State.** Promote `deepseek-r1:32b` to the **standing SST
default** synthesis model on self-hosted (owner-decided; the A/B fixed the Q1
false-balance + Q4 hallucination), with the envelope raised + the real
footprint verified; add a **per-user sticky** `/model <id>` selector
(claim-bound), a **per-request gather** override (`--gather-model=` /
`gather_model`), a single **precedence resolver** (per-request > sticky >
SST default), **model + source attribution** on both surfaces, and a
documented **~15s core-recreate** hot-swap. No selection ⇒ byte-for-byte
the new SST baseline.

**Patterns to Follow.**
- The spec-088 `modelswitch` shared validator + `agenttool` singleton
  (`SwitchableModels()`/`Resolve`) — the new precedence resolver +
  gather/tool-capability check + source classification land in the SAME
  package, reached by both fast-paths (the one parity seam).
- The spec-088 `Agent.WithModelOverride` per-request clone — extend it to
  also re-point `cfg.Model` (gather) when a gather override is present;
  the SST singleton is still never mutated (C6).
- The actor-keyed per-user table pattern (`recommendation_*` /
  `annotation` stores: `actor_user_id TEXT`, PK/own index, no DB-side
  defaults — [022_recommendations.sql](../../internal/db/migrations/022_recommendations.sql))
  for the new `user_model_preferences` table.
- The claim-binding seam: Telegram `Bot.resolveActorUserID(chatID)` →
  `AssistantMessage.UserID`; HTTP `auth.UserIDFromContext(r.Context())`
  (the PASETO subject; `/v1/agent/invoke` is already behind
  `bearerAuthMiddleware`, [router.go:598](../../internal/api/router.go#L598)) —
  the exact `subject := auth.UserIDFromContext(...)` pattern
  [annotation_list.go:34](../../internal/api/annotation_list.go#L34) uses.
- The fail-loud envelope guard
  ([config.go::validateModelEnvelopes](../../internal/config/config.go#L2137))
  — the standing-default co-residence check lands here, beside the
  spec-088 switchable pass.

**Patterns to Avoid.**
- Do NOT mutate the singleton `okagent.Config` at runtime (C6 /
  build-once) — sticky is DB state read per-invocation, applied as a
  clone, never an SST write.
- Do NOT write the sticky preference from a request-body user id (FR-5 /
  OWASP A01) — only the claim-bound `actor_user_id`.
- Do NOT overload `--model=` to re-point BOTH turns (the rejected Fork C
  combined-flag): a single combined flag would silently push a
  non-tool-capable synthesis model (`deepseek-r1*`) onto the gather turn
  and degrade evidence (FR-8 hazard) — keep `--model=` = synthesis-only
  (byte-for-byte 088) and add a SEPARATE `--gather-model=`.
- Do NOT trust the **profile number** (22528 MiB) as the 32b footprint —
  it understates the real KV-cache-dominated ~64 GB at full ctx; the
  Docker `OLLAMA_MEMORY_LIMIT` cgroup cap is the real bound (see Fork A).
- Do NOT bump `model_memory_profiles[deepseek-r1:32b]` — it is a shared
  q4 ceiling used across the co-residence matrix; a context-specific bump
  corrupts every other sum.

**Resolved Decisions.** Fork A → 32b standing default + raise envelope
28G→48G + ADD an explicit standing-default co-residence guard + rely on
the cgroup cap as the real-KV bound (do NOT bump the profile). Fork B →
NEW `user_model_preferences` table (actor-keyed PK), synthesis sticky
only. Fork C → SEPARATE `--gather-model=` / `gather_model`, per-request
only, gated by a new `tool_capable_gather_models` SST set. Fork D → the
~15s core-recreate is THE hot-swap; zero-downtime reload deferred
(F-HOTRELOAD). FR-13/FR-14 → in-scope (cheap): strip residual
`<CITATIONS>` scaffolding on the salvage arms + a regression proving the
empty-forced-final retry fires.

**Open Questions.** None blocking. Four deferred follow-ups recorded as
findings (F-HOTRELOAD, F-STICKY-GATHER, F-FOOTPRINT, F-RETRYBUDGET); each
is a named, non-blocking next lever, not an unresolved fork.

---

## Current Truth (gathered 2026-06-14, solution-blind)

Verified directly against committed code (confirmed the spec, not
re-derived from it). CT-1..CT-7 extend design 088's CT-1..CT-8.

### CT-1 — Both surfaces already converge on ONE singleton agent + ONE allowlist
- **Telegram:** `/ask` → `Facade.Handle` open_knowledge fast-path
  ([facade.go:1064](../../internal/assistant/facade.go#L1064)):
  `allow := okagenttool.SwitchableModels(); ov, rej := allow.Resolve(msg.ModelOverride)`
  → `runOpenKnowledgeDirect(ctx, sc, env, emittedAt, ov)`
  ([facade.go:1235](../../internal/assistant/facade.go#L1235)) →
  `okagenttool.CurrentAgent().WithModelOverride(ov).Run(ctx, prompt)`.
- **Web/HTTP:** `POST /v1/agent/invoke` open_knowledge fast-path
  ([agent_invoke.go:148](../../internal/api/agent_invoke.go#L148)):
  `allow := agenttool.SwitchableModels(); resolved, rej := allow.Resolve(req.Model)`
  → `CurrentAgent().WithModelOverride(ov).Run(r.Context(), prompt)` →
  `MapTurnResult(turn)` → `writeOpenKnowledgeResponse`.
- **Consequence:** the precedence resolver + gather check + source
  classification MUST extend the SAME `modelswitch` package + `agenttool`
  singleton both fast-paths already read (the parity seam is built; 089
  widens it, does not re-plumb it).

### CT-2 — `/v1/agent/invoke` is already claim-authenticated (bearer)
- Mounted under `bearerAuthMiddleware`
  ([router.go:598-600](../../internal/api/router.go#L598)): "Behind bearer
  auth (same policy as /api/*) so callers must authenticate." So
  `auth.UserIDFromContext(r.Context())` resolves the PASETO subject INSIDE
  the open_knowledge fast-path — the HTTP sticky read is claim-bound for
  free, with no new auth wiring. The handler does NOT currently read it
  (the agent runs on the prompt alone); 089 adds the read.
- The canonical claim-bound read pattern is
  [annotation_list.go:34](../../internal/api/annotation_list.go#L34):
  `subject := auth.UserIDFromContext(r.Context())` then
  `Store.ListByActor(ctx, subject, …)` with an `actor=="me"||==subject`
  guard. The same shape governs `GET/PUT/DELETE /v1/agent/model`.

### CT-3 — Telegram already resolves the claim-bound actor for `/ask`
- `translateInbound` ([translate_inbound.go:35](../../internal/telegram/assistant_adapter/translate_inbound.go#L35))
  calls `resolve(chatID)` (= `Bot.resolveActorUserID`, the production
  chat→user claim guard) and stamps `AssistantMessage.UserID`. The
  facade fast-path therefore already HAS the claim-bound user id
  (`msg.UserID`) at the point it resolves the override — the sticky read
  needs no new identity plumbing.
- `parseModelFlag` ([translate_inbound.go:144](../../internal/telegram/assistant_adapter/translate_inbound.go#L144))
  consumes a leading `--model=<id>` token (slash preserved). The gather
  flag extends this exact function.

### CT-4 — The agent splits gather (`cfg.Model`) vs synthesis (`cfg.SynthesisModel`)
- [agent.go::Config](../../internal/assistant/openknowledge/agent/agent.go#L108):
  `Model` (every gather/tool turn, `reqModel := a.cfg.Model`,
  [agent.go:330](../../internal/assistant/openknowledge/agent/agent.go#L330))
  and `SynthesisModel` (forced-final turn + retries,
  [agent.go:342](../../internal/assistant/openknowledge/agent/agent.go#L342)),
  both REQUIRED non-empty in `New` (G028).
- `WithModelOverride(o)` ([agent.go:266](../../internal/assistant/openknowledge/agent/agent.go#L266))
  clones and sets `clone.cfg.SynthesisModel = o.SynthesisModel` ONLY. To
  add a gather override (Fork C) the clone must ALSO set
  `clone.cfg.Model` when a gather model is present — a thin extension of
  the existing clone, the SST singleton still untouched.
- `TurnResult.Model` ([agent.go:88](../../internal/assistant/openknowledge/agent/agent.go#L88))
  is stamped once in `finalize` from `answeringModel` (the model of the
  turn that produced the final text — honest across success / salvage /
  refuse / early-StopEndTurn). There is NO `GatherModel` field yet.

### CT-5 — `modelswitch` is the pure validator (extensible, no cycle)
- [allowlist.go](../../internal/assistant/openknowledge/modelswitch/allowlist.go):
  `Override{SynthesisModel}`, `Resolve(raw) (Override, *Rejection)`,
  `Rejection{RejectedModel, AllowedModels, DefaultModel, ReasonCode,
  Message}`, reason codes `model_not_allowlisted` /
  `model_over_memory_envelope`, `NewAllowlist(switchable, profiles,
  envelopeMiB, gatherModel, defaultModel)`. Pure (stdlib only) — a leaf,
  so adding a gather/tool-capability check + a precedence/source resolver
  here forms no import cycle.
- The rejection `message(...)` builds the exact UX golden sentence keyed
  on `ReasonCode` (the capitalised **NOT**, the allowed set with
  `(default)`, a retry hint). The `model_not_tool_capable` template is
  the F-TOOLMODEL addition.

### CT-6 — The SST envelope guard is profile-based; the standing default is NOT checked
- [validateModelEnvelopes](../../internal/config/config.go#L2137) checks
  per-service buckets (a fixed list of ollama model env vars) + the
  spec-082 interactive concurrent-sum + the **spec-088 switchable_models
  co-residence pass** ([config.go ~2300](../../internal/config/config.go#L2300)):
  for each switchable entry, `profiles[gather] (+ profiles[entry] if
  entry≠gather) ≤ OllamaMemoryLimitMiB`.
- **Gap (089-relevant):** the standing `synthesis_model_id` is NOT in any
  bucket and is NOT co-residence-checked — today it is only validated
  structurally (non-empty, [openknowledge.go:200](../../internal/config/openknowledge.go#L200)).
  Spec 088 got away with this because the self-hosted default (7b, 4864) is
  tiny. Promoting the default to 32b (22528) makes the EVERY-QUERY model
  large and un-envelope-checked — the design must close this.
- Profiles ([smackerel.yaml:1551](../../config/smackerel.yaml#L1551)):
  `gemma4:26b`=18432, `deepseek-r1:7b`=4864, `deepseek-r1:32b`=22528,
  `llama3.1:8b`=6144, `gemma3:4b`=4096. self-hosted `ollama_memory_limit:
  "28G"` (=28672); gather+32b = 18432+22528 = 40960 > 28672 ⇒ 32b is the
  `model_over_memory_envelope` opt-up TODAY (structurally rejected until
  the envelope is raised).

### CT-7 — The footprint reality (A/B-measured) — KV-cache, not weights, dominates
- The profile (22528 MiB ≈ 22 GiB) is a **q4 weights + small-ctx
  ceiling** ([smackerel.yaml:1560 comment](../../config/smackerel.yaml#L1560)).
  The A/B `ollama ps` measured `deepseek-r1:32b` at its default 131072
  ctx as a **~64 GB** resident footprint (19 GB weights + a very large
  KV-cache) vs **~20 GB at 4096 ctx** (A/B §4). The open-knowledge
  pipeline runs `per_query_token_budget: 128000`
  ([smackerel.yaml:1044](../../config/smackerel.yaml#L1044)).
- **Crucial A/B finding:** the live 32b arm fit at a raised
  `OLLAMA_MEMORY_LIMIT: 48G` — 82 GiB used / 26 GiB free on the 109 GiB
  box, "no pressure" — **because the Docker memory cap CONSTRAINS
  ollama's context** (ollama sizes its KV-cache to fit the cgroup limit).
  i.e. the deploy `OLLAMA_MEMORY_LIMIT` is itself the real-footprint
  bound, not the profile. This is the hinge of the Fork A resolution.

### CT-8 — The output-hygiene + retry seams (FR-13 / FR-14)
- `stripThinkBlocks` ([agent.go:811](../../internal/assistant/openknowledge/agent/agent.go#L811))
  runs on the forced-final text BEFORE `parseCitations`
  ([agent.go:484](../../internal/assistant/openknowledge/agent/agent.go#L484)),
  so `<think>` cannot reach the body (FR-13 for `<think>` already holds —
  the A/B confirmed no `<think>` leak on 32b).
- The residual `<CITATIONS>` / `<one synthesized answer…>` **scaffolding
  leak** the A/B saw comes from the salvage arms surfacing
  `result.FinalText` / `trimmedText` (the missing-CITATIONS salvage at
  [agent.go:497](../../internal/assistant/openknowledge/agent/agent.go#L497)
  and the empty-citations salvage at
  [agent.go:526](../../internal/assistant/openknowledge/agent/agent.go#L526))
  which can still contain a malformed/partial `<CITATIONS>` fragment.
  That is the cheap FR-13 fix target (model-independent).
- `synthesisNeedsRetry(text)` ([agent.go:825](../../internal/assistant/openknowledge/agent/agent.go#L825))
  returns true on empty OR ungrounded-excuse; the forced-final retry loop
  ([agent.go:391](../../internal/assistant/openknowledge/agent/agent.go#L391))
  is `for retry := 0; retry < SynthesisRetryBudget && synthesisNeedsRetry(...)`.
  self-hosted `synthesis_retry_budget=1` ⇒ a blank forced-final DOES fire
  exactly one escalated retry before the salvage. CT-confirmed (FR-14).

### CT-9 — The latency invariant (carried from 084/087/088, unchanged)
- `WriteTimeout = (max_iterations + synthesis_retry_budget) ×
  llm_timeout_ms = (6 + 1) × 600s = 4200s` (smackerel.yaml: `max_iterations:
  6`, `synthesis_retry_budget: 1`, `llm_timeout_ms: 600000`). Each LLM
  round is independently bounded by `llm_timeout_ms` (600s). A slower
  standing default (32b) swaps WHICH model runs the existing forced-final
  turn — it adds no turns, so the envelope is unchanged.

### CT-10 — No general per-user preference store exists
- The only per-user persistence is the PASETO minter
  ([per_user_token.go](../../internal/telegram/per_user_token.go)). The
  actor-keyed CRUD pattern to mirror is `recommendation_preference_corrections`
  / `recommendation_seen_state` (migration 022:
  `actor_user_id TEXT NOT NULL`, own PK/UNIQUE, IDs+timestamps written by
  app code, no DB-side defaults). Latest migration on disk is **058** ⇒
  the new one is **059**.

---

## Decision Record — the four forks

Each fork is resolved to the smallest buildable decision that fully meets
the owner directive, with the rejected alternative recorded.

### Fork A — Standing default + footprint guard → **DECISION: `deepseek-r1:32b` default, envelope 28G→48G, ADD a standing-default co-residence guard, rely on the cgroup cap as the real-KV bound (do NOT bump the profile)**

**Decision.** Promote the self-hosted persistent default synthesis model
`deepseek-r1:7b → deepseek-r1:32b` (committed
`environments.self-hosted.assistant_open_knowledge_synthesis_model_id`);
raise `environments.self-hosted.ollama_memory_limit` `28G → 48G`; add
`deepseek-r1:32b` to the self-hosted `switchable_models` (keep `7b` +
`gemma4:26b`). Resolve the §2 footprint-correctness problem with a
**four-part** mechanism rather than a single knob:

1. **Raise the envelope to 48G.** Satisfies the profile co-residence
   check (18432 + 22528 = 40960 ≤ 49152) AND the owner's measured
   ~45824 MiB; matches the A/B-validated live arm.
2. **Add an explicit STANDING-DEFAULT co-residence guard** to
   `validateModelEnvelopes` (close CT-6's gap): the resolved
   `synthesis_model_id` co-resident with the gather `llm_model_id` MUST
   fit `OllamaMemoryLimitMiB`, the same arithmetic the switchable pass
   uses. This is the real fix — today the every-query default is the ONE
   large-model selection that is NOT envelope-checked. Makes
   SCN-089-A06's "over-envelope standing default refused at config
   generation" true and fail-loud.
3. **Rely on the Docker `OLLAMA_MEMORY_LIMIT` cgroup cap as the
   real-KV-cache bound.** The profile understates the 64 GB full-ctx
   footprint, but the A/B proved the cgroup cap CONSTRAINS ollama's
   KV-cache: at 48G the live 32b arm measured 82 GiB used / 26 GiB free,
   "no pressure", co-resident with the ingestion pipeline (CT-7). The cap
   — not the profile — is the real-footprint guard; raising it to fit the
   co-resident profile sum is exactly the right lever.
4. **Document the verified headroom** (A/B numbers) in the SST comment +
   `docs/Operations.md`, so the standing-default footprint is an
   explicit, evidence-cited decision, not an accident.

**Do NOT bump `model_memory_profiles[deepseek-r1:32b]`** to 64 GB: it is a
shared q4 ceiling consumed by every co-residence sum in
`validateModelEnvelopes`; a context-specific bump would corrupt the
interactive-set and switchable arithmetic for unrelated models. The
context-dependent real footprint belongs to the cgroup cap, not the
static profile.

**Rationale.** The owner decided 32b for quality — it fixed the Q1
false-balance ("depends on priorities" → "Phoenix is better; Minneapolis
unsuitable") and the Q4 hallucination ("Blade Runner, author unknown,
1982" → "Do Androids Dream of Electric Sheep?, Philip K. Dick, 1968"),
the exact failure class that motivated the whole effort (Principle 2). The
footprint risk is real but is correctly bounded by the cgroup cap (4)
plus the new standing-default guard (2), both evidence-grounded. NFR-2
latency is acceptable for a research/recall assistant: the A/B worst
single `/ask` was 830s (Q4) — well within `WriteTimeout` 4200s — and each
synthesis ROUNDTRIP (long `<think>` + answer at ~8.6 tok/s) stayed within
`llm_timeout_ms` 600s on every completed A/B run; a runaway `<think>`
would hit the 600s per-round cap → the graceful retry/salvage path, never
a hang.

**Recorded alternative (the directive's "safer" flag).** Keep `7b` as the
standing default + a prominent sticky/per-request `32b`. Safer footprint
(~45 GB vs ~89 GB co-resident at full ctx) and latency (~5 min vs ~9-10
min). **Rejected** because (a) the owner chose 32b for quality, (b) the
48G envelope + cgroup cap make 32b footprint-safe (A/B-verified), and (c)
`7b` stays switchable, so the speed escape hatch is one
`/model deepseek-r1:7b` away — the alternative's only advantage is
preserved as a first-class sticky.

**Recorded finding F-FOOTPRINT.** An explicit synthesis-turn `num_ctx`
bound (a belt-and-suspenders real-footprint-aware guard that caps the
KV-cache independent of the cgroup) — deferred, because the cgroup cap
already bounds the real KV footprint and the A/B verified the headroom.
Revisit only if concurrent ingestion + the 32b default shows pressure in
practice.

### Fork B — Per-user preference store → **DECISION: a NEW `user_model_preferences` table (actor-keyed PK), synthesis sticky only**

**Decision.** A NEW migration `059_user_model_preferences.sql`:

```sql
CREATE TABLE IF NOT EXISTS user_model_preferences (
    actor_user_id   TEXT PRIMARY KEY,        -- claim-bound principal (spec 044); one row per user
    synthesis_model TEXT NOT NULL,           -- the sticky /ask synthesis model id
    gather_model    TEXT,                    -- RESERVED (nullable) for F-STICKY-GATHER; unread today
    updated_at      TIMESTAMPTZ NOT NULL     -- written by app code (no DB-side default)
);
```

A new leaf store package `internal/assistant/openknowledge/modelpref`:

```go
type Preference struct { SynthesisModel string; UpdatedAt time.Time }
type Store interface {
    Get(ctx context.Context, userID string) (Preference, bool, error) // PK lookup; ok=false ⇒ inherit default
    Set(ctx context.Context, userID, synthesisModel string) error     // upsert (ON CONFLICT (actor_user_id) DO UPDATE)
    Clear(ctx context.Context, userID string) error                   // DELETE; idempotent
}
```

`Get` is a single primary-key lookup — O(1) on the hot path, negligible
against a multi-minute `/ask`. No cache (don't over-engineer; the read is
one indexed row).

**Rationale.** No general per-user store exists (CT-10); the sticky
preference is genuinely new per-user state. A dedicated table (vs
overloading `recommendation_preference_corrections`, which is semantically
a recommendation artifact, or a generic KV that would invite scope creep)
is the smallest honest fit and mirrors the established actor-keyed pattern
exactly. PK on `actor_user_id` gives one row per user, a cheap indexed
read, and reset == `DELETE` (no tombstone). `gather_model` is included
nullable per the owner directive's 4-column shape but is **reserved** —
sticky gather is deferred (Fork C / F-STICKY-GATHER); the resolver does
not read it until that finding is picked up, so it adds a forward-compat
column, not behaviour.

**Claim-binding (FR-5 / C3 / SCN-089-A04).** The store is keyed ONLY on
the authenticated principal:
- **Telegram:** `msg.UserID` (already `Bot.resolveActorUserID(chatID)`,
  CT-3) — never a body field.
- **HTTP:** `auth.UserIDFromContext(r.Context())` (the PASETO subject,
  CT-2) — the `PUT /v1/agent/model` body carries ONLY `{model}`; any
  user-id-shaped body field is ignored. A spoofed actor id structurally
  cannot reach the key.

**Recorded alternative.** Reuse the actor-keyed
`recommendation_preference_corrections` row shape with a
`preference_key='open_knowledge_synthesis_model'`. **Rejected** — it
couples an unrelated capability's table to open-knowledge, muddies that
table's `correction_kind` CHECK vocabulary, and offers no schema saving
over a 4-column purpose-built table.

### Fork C — Gather-override semantics → **DECISION: a SEPARATE `--gather-model=` / `gather_model`, per-request only, gated by a new `tool_capable_gather_models` SST set**

**Decision.** Add a SEPARATE gather override carrier
(`AssistantMessage.GatherModelOverride` / `AgentInvokeRequest.GatherModel`)
that re-points the gather/tool turns independently of `--model=`
(synthesis). `--model=` keeps byte-for-byte spec-088 synthesis-only
semantics. A gather selection is validated against a NEW operator-curated
SST list `assistant.open_knowledge.tool_capable_gather_models`
(self-hosted = `[gemma4:26b, llama3.1:8b]`; dev = `[gemma3:4b]` == baseline,
testable no-op) and rejected `model_not_tool_capable` if absent. Gather
sticky is **deferred** (F-STICKY-GATHER) — gather override is
per-request only.

**Rationale.** A single combined flag would silently push a
non-tool-capable synthesis model onto the gather turn — `gemma3:4b` errors
`does not support tools`, `deepseek-r1*` tool-calling is weak
([smackerel.yaml:1556 comment](../../config/smackerel.yaml#L1556)) — which
degrades evidence and confounds the very A/B the operator wants (FR-8
hazard). Separating the flags keeps the spec-087 architecture (strong
tool-caller gathers, reasoning model synthesizes) and makes the gather a
DELIBERATE, independently-attributed variable. A dedicated tool-capable
SST set (vs deriving capability from a hard-coded model list in Go) keeps
the no-defaults / SST contract: capability is operator-declared data,
fail-loud, not a code constant. Per-request-only avoids a second sticky
column read on the hot path for a rarer experiment; the synthesis sticky
is the owner's stated need.

**Tool-capability constraint enforcement (FR-8).** `ResolveGather(raw)`
checks membership in `tool_capable_gather_models` BEFORE any gather turn
runs; a miss is `model_not_tool_capable` with the tool-capable set named.
The baseline gather (`llm_model_id`) MUST be in the set (validated at
config load) so the no-override path always passes.

**Recorded alternative (F-STICKY-GATHER).** A `/gather-model <id>` sticky
knob + a second sticky column read. **Deferred** — the
`user_model_preferences.gather_model` column is reserved for it, the
resolver and carrier are designed to accept a sticky gather with a
one-line precedence extension, so picking it up later is additive, not a
rework.

### Fork D — Hot-swap mechanism → **DECISION: the documented ~15s core-recreate IS the hot-swap; zero-downtime reload deferred (F-HOTRELOAD)**

**Decision.** The prod hot-swap is: edit the committed SST default →
`./smackerel.sh config generate --env self-hosted` (fail-loud) → the operator
overlay recreates the core service only (`--no-deps`, image digests from
the running container, ~15s) → verify via the boot log
(`open-knowledge subsystem wired … synthesis_model=<new>`,
[wiring_assistant_openknowledge.go:275](../../cmd/core/wiring_assistant_openknowledge.go#L275))
+ a live `/ask` envelope (`model_source: default`). True zero-downtime
config-hot-reload is **deferred** (F-HOTRELOAD).

**Rationale.** The ~15s recreate was proven during the A/B ("core healthy
in 15s"). For a self-hosted single-operator research assistant, ~15s of
`/ask` unavailability on a deliberate model swap is immaterial; the
ingestion pipeline is a separate service and is untouched. True
zero-downtime reload (config-watch + in-place model-registry swap of the
`agenttool` singleton) is a large lift (a new watch loop, atomic
swap-while-serving, and a re-validation path) disproportionate to the
need. The repo owns only the committed-SST edit + the `config generate`
step; the host-specific recreate lives in the operator overlay
(deployment-ownership boundary / C5).

**Recorded alternative (F-HOTRELOAD).** A config-watch that atomically
re-installs `agenttool.SetAgent` + `SetSwitchableModels` on file change
(the singleton is already an `atomic.Pointer`, so the swap primitive
exists). **Deferred** unless an operator demonstrates the 15s recreate is
a real problem.

---

## Implementation Design

### Precedence resolver (the single resolution point — FR-6 / FR-9)

ONE pure function in `modelswitch` computes the effective selection +
source, validates every winning model, and returns a fail-loud
`*Rejection` — consumed identically by both surfaces. The sticky value is
passed in as a plain string (the claim-bound store read happens at the
surface, NOT inside the pure validator — keeps `modelswitch` storeless +
leaf):

```go
// modelswitch — source classification (closed set).
const ( SourceDefault = "default"; SourceSticky = "sticky"; SourcePerRequest = "per_request" )

// Override extended (Fork C): zero value ⇒ baseline (byte-for-byte NFR-4).
type Override struct { SynthesisModel string; GatherModel string }
func (o Override) IsZero() bool { return o.SynthesisModel == "" && o.GatherModel == "" }

// Effective is the resolved selection for ONE invocation: model ids ALWAYS
// populated (default ⇒ the baseline ids) for attribution; sources classified.
type Effective struct {
    SynthesisModel  string; SynthesisSource string // SourceDefault|Sticky|PerRequest
    GatherModel     string; GatherSource    string // SourceDefault|PerRequest (sticky gather deferred)
}
// Override() builds the per-invocation clone input: zero for a pure-default
// invocation (so WithModelOverride returns the receiver unchanged, NFR-4).
func (e Effective) Override() Override {
    var o Override
    if e.SynthesisSource != SourceDefault { o.SynthesisModel = e.SynthesisModel }
    if e.GatherSource    != SourceDefault { o.GatherModel    = e.GatherModel }
    return o
}

// ResolveEffective applies precedence (per-request > sticky > SST default),
// validates each WINNING model, and classifies the source. Pure; no store, no
// backend. perReqSynth/perReqGather are UNTRUSTED request values; stickySynth
// is the claim-bound stored pref (already-validated at set time, re-validated
// here defensively).
func (a *Allowlist) ResolveEffective(perReqSynth, perReqGather, stickySynth string) (Effective, *Rejection)
```

Resolution rules (deterministic, SCN-089-A05):

| Turn | If per-request supplied | Else if sticky supplied | Else |
|------|-------------------------|--------------------------|------|
| **Synthesis** | validate via `isSwitchable`; reject(`synthesis`) on fail ⇒ source `per_request` | re-validate; if switchable ⇒ source `sticky`; **if orphaned** (operator retired it) ⇒ fall to default, source `default` (+ structured log) | source `default`, model = `defaultModel` |
| **Gather** | validate via `tool_capable_gather_models`; reject(`gather`) on fail ⇒ source `per_request` | *(sticky gather deferred)* | source `default`, model = baseline gather (`gatherModel`) |

- A **per-request** bad value is the user's input THIS turn ⇒ fail-loud
  reject (no agent call). A **sticky** value that has gone off-allowlist
  (operator removed the model from `switchable_models`) is NOT the asker's
  fault this turn ⇒ resolve to the system default + log, never break every
  `/ask` for that user. (Documented honesty rule; matches FR-16 "validate
  before backend" — the orphaned sticky never reaches the backend; the
  default does.)
- A synthesis reject does NOT fall through to sticky/default — the user
  asked for a specific model and got an explicit refusal (Principle 8).

### Per-user preference store + claim-binding (FR-4 / FR-5)

- Migration `059_user_model_preferences.sql` (Fork B shape above; ROLLBACK
  comment; no DB-side defaults).
- `internal/assistant/openknowledge/modelpref`: `Store` interface +
  `PostgresStore` (`Get`/`Set` upsert via `ON CONFLICT (actor_user_id) DO
  UPDATE`/`Clear` delete). Wired into the facade (Telegram `/ask` read +
  `/model` CRUD) and the api Dependencies (HTTP `/ask` read +
  `/v1/agent/model` CRUD).
- **Hot-path read:** facade open_knowledge fast-path resolves
  `sticky, ok, _ := f.modelPref.Get(ctx, msg.UserID)` (claim-bound by
  CT-3) BEFORE the precedence resolver; HTTP fast-path resolves
  `subject := auth.UserIDFromContext(r.Context()); sticky, ok, _ :=
  h.ModelPref.Get(r.Context(), subject)` (claim-bound by CT-2). A nil
  store (capability not wired) ⇒ `stickySynth=""` ⇒ default path (never a
  panic; mirrors the spec-088 nil-allowlist passthrough).
- **`/model` set/show/reset** is a per-user CRUD + discovery affordance,
  NOT an agent run — so it does NOT flow through the agent/facade
  invocation path. Telegram: a new bot-command handler (`case "model"` in
  the bot dispatch, beside `case "ask"`, [bot.go:596](../../internal/telegram/bot.go#L596))
  resolves `resolveActorUserID`, calls the shared store + a shared
  discovery/confirm/reject renderer. HTTP: the new `GET/PUT/DELETE
  /v1/agent/model` handlers. BOTH use the SAME store + the SAME
  `modelswitch` validator + the SAME rendered strings (parity, FR-10).

### Gather carrier + tool-capability guard (FR-7 / FR-8)

- Carriers: `AssistantMessage.GatherModelOverride` (typed field, beside
  the spec-088 `ModelOverride`); `AgentInvokeRequest.GatherModel`
  (`json:"gather_model,omitempty"`). Both UNTRUSTED, validated before use.
- Telegram: `parseModelFlag` extended to ALSO consume a leading
  `--gather-model=<id>` token (order-independent with `--model=`); set
  `msg.GatherModelOverride`. ollama tags have no whitespace ⇒ a single
  token is the whole value (same discipline as `--model=`).
- Validator: `tool_capable_gather_models` SST set → `Allowlist.toolCapableGather`;
  `ResolveGather(raw)` returns a `model_not_tool_capable` `Rejection` when
  the model is profiled+switchable but not tool-capable, reusing the
  spec-088 `model_not_allowlisted` when it is unknown.
- Apply: `WithModelOverride` extended to also set `clone.cfg.Model =
  o.GatherModel` when non-empty (Fork C). The clone is still a per-request
  copy; the SST singleton `Model`/`SynthesisModel` are never mutated (C6).

### Attribution + source wiring (FR-11 / Principle 8)

- `agent.TurnResult`: add `GatherModel string` (the gather model that ran,
  `= a.cfg.Model` at finalize), beside the existing `Model` (the answering
  model — honest "which turn produced the text", CT-4). Stamped once in
  `finalize`.
- `agenttool.outputEnvelope`: add `ModelSource`, `GatherModel`,
  `GatherModelSource` (all `json:",omitempty"` except `model`/`model_source`
  which the HTTP envelope ALWAYS carries). `MapTurnResult` sets
  `Model`/`GatherModel` from the turn; the SOURCE fields are set by the
  caller (the api/facade KNOWS the `Effective` it resolved — source is a
  resolver concept, not a turn concept), so a thin
  `WithSelection(env, eff)` helper (or direct field set after
  `MapTurnResult`) stamps `model_source`/`gather_model_source`.
- `contracts.ModelAttribution`: extend from `{ModelID, OverrideApplied}`
  to also carry `SynthesisSource string`, `GatherModel string`,
  `GatherSource string`, `GatherOverridden bool`. `render_outbound.go::appendModelFooter`
  renders:
  - single form `— model: <id> (<source>)` when only the synthesis
    selection is non-default (source = `your default` / `this question`);
  - dual form `— gather: <g> (<gsrc>) · synth: <s> (<ssrc>)` whenever a
    gather override is active (the only time the gather turn is an
    operator-chosen variable);
  - **no footer** on a pure system-default answer (Principle 6 / NFR-4 —
    byte-for-byte the spec-087/088 baseline render).
- **Honest answering-model nuance (carried from 088, CT-4).** The
  envelope `model` = `turn.Model` (the model that ACTUALLY produced the
  text — may be the gather model on an early `StopEndTurn`); `model_source`
  describes how THAT model's turn was selected (a small picker maps
  `turn.Model == eff.SynthesisModel ⇒ eff.SynthesisSource`,
  `== eff.GatherModel ⇒ eff.GatherSource`). On the honest-salvage path the
  footer still reads `— model: <id> (<source>)` (neutral metadata), never
  "answered by", so it never contradicts the "I searched but couldn't
  directly answer" framing (spec 087/088 preserved).

### Persistent-default SST edits (FR-1 / FR-2 / NFR-5 / C2)

Committed `config/smackerel.yaml`, self-hosted `environments.<env>` block
(deployment-ownership-allowed generic per-env config, C5):

```yaml
    ollama_memory_limit: "48G"                                   # was "28G" (FR-2; gather 18432 + 32b 22528 = 40960 <= 49152; A/B-verified 82/26 GiB no pressure)
    assistant_open_knowledge_synthesis_model_id: "deepseek-r1:32b"  # was "deepseek-r1:7b" (FR-1; quality-first standing default; owner-decided)
    assistant_open_knowledge_switchable_models: [ "deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b" ]  # add 32b (now envelope-fits at 48G); 7b stays the speed escape hatch
    assistant_open_knowledge_tool_capable_gather_models: [ "gemma4:26b", "llama3.1:8b" ]  # NEW (Fork C / FR-8); baseline gather gemma4:26b MUST be present
```

Base `assistant.open_knowledge` block gains the new key (dev = baseline,
testable no-op):

```yaml
    tool_capable_gather_models: [ "gemma3:4b" ]   # REQUIRED non-empty when enabled. Spec 089 — the allowlist of models the GATHER turn may be runtime-switched TO (--gather-model= / API gather_model). Each MUST be tool-calling-capable on Ollama. Dev = [gemma3:4b] (== baseline gather; testable no-op; dev has no daemon). self-hosted override adds the real set. Fail-loud (G028); the baseline llm_model_id MUST be a member.
```

Fail-loud, no `${VAR:-default}`. `internal/config/openknowledge.go` loads
`ToolCapableGatherModels` via `lookupJSONStringList(
"ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS")` (mirrors
`SwitchableModels`) + `Validate` non-empty + `llm_model_id ∈ set`.
`scripts/commands/config.sh` resolves+emits it via the per-env override
pattern (mirror `switchable_models`).

### Output hygiene + forced-final reliability (FR-13 / FR-14) — IN SCOPE (cheap)

- **FR-13 (scaffolding strip).** Add `stripContractScaffolding(text)`
  (agent.go) that removes any residual `<CITATIONS>…</CITATIONS>` block,
  a stray unterminated `<CITATIONS>`, and the `<one synthesized answer…>`
  contract marker, then trims. Apply it to the **salvage body arms** (the
  missing-CITATIONS salvage at agent.go:497 and the empty-citations /
  honest-salvage bodies) BEFORE `finalize`, so a malformed/partial
  `<CITATIONS>` fragment can never reach the user body under ANY model.
  Model-independent; the happy cited-synthesis path already strips
  cleanly via `parseCitations`. (`<think>` is already stripped pre-parse,
  CT-8 — no change there.)
- **FR-14 (forced-final retry).** **Verified:** `synthesisNeedsRetry("")`
  returns `true` (CT-8), so the self-hosted `synthesis_retry_budget=1` DID
  fire exactly one escalated retry on the 32b Q6 blank; it still blanked
  because the retry attempt ALSO returned empty/`<think>`-only, after
  which the salvage fired (the A/B `len=39` is the salvage/refusal, not a
  silent blank). The mechanism is correct and sufficient — no logic
  change. Add a fake-LLM **regression** proving
  empty-forced-final → escalated-retry → still-empty →
  honest-salvage-with-sources (never a silently-empty body), the exact
  32b-Q6 shape. `WriteTimeout` stays 4200s (budget unchanged).
- **Recorded finding F-RETRYBUDGET.** Raising self-hosted
  `synthesis_retry_budget` 1→2 would give the 32b default two escalated
  retries before salvage — but it widens `WriteTimeout` to (6+2)×600 =
  4800s (NFR-2). Deferred as an operator SST knob, NOT a forced change;
  recorded so the operator can trade reliability↔latency consciously.

### Multi-surface parity seam (FR-9 / FR-10 / SCN-089-A08 / A11)

ONE store, ONE validator, ONE precedence resolver, ONE attribution shape:

| Concern | Telegram | Web/HTTP | Shared seam |
|---|---|---|---|
| Sticky read (claim-bound) | `modelPref.Get(msg.UserID)` | `modelPref.Get(auth.UserIDFromContext(ctx))` | **same `modelpref.Store`** |
| Per-request carriers | `--model=` / `--gather-model=` → `AssistantMessage.{ModelOverride,GatherModelOverride}` | `AgentInvokeRequest.{Model,GatherModel}` | both → two raw strings |
| Precedence + validate + source | `allow.ResolveEffective(...)` | `allow.ResolveEffective(...)` | **same function** |
| Clone + run | `CurrentAgent().WithModelOverride(eff.Override()).Run` | same | same method |
| Rejection | reply = `rej.Message` (+ `rejected_turn`) | 400 + `error_code`/`rejected_turn` + same `Message` | **same `Rejection`** |
| Sticky CRUD | `/model` bot command | `GET/PUT/DELETE /v1/agent/model` | same store + same renderer |
| Attribution | footer (source tags, dual gather form, only-on-override) | envelope `model`+`model_source`(+`gather_model`+`gather_model_source`), always | `turn.Model`/`turn.GatherModel` + `Effective` sources |

The surfaces differ ONLY in (a) parsing the raw strings and (b) rendering
the result — the read→resolve→clone→run→attribute spine is identical.
Parity proof: drive the SAME off-allowlist string + the SAME sticky
through both and assert the SAME `Rejection` / `Effective` (SCN-089-A08/A11).

### Latency analysis (NFR-2 / C4)

```
WriteTimeout = (max_iterations + synthesis_retry_budget) × llm_timeout_ms
             = (6 + 1) × 600s = 4200s   (UNCHANGED)
```
- **Standing 32b default:** swaps WHICH model runs the existing
  forced-final turn (+ retries); turn COUNT is identical; each round is
  still bounded by `llm_timeout_ms` (600s). A/B worst single `/ask` =
  830s (Q4) ≤ 4200s; every completed A/B synthesis roundtrip stayed
  within 600s. A runaway `<think>` would hit the 600s per-round cap → the
  graceful retry/salvage path, never a hang. **No `WriteTimeout` change.**
- **Gather override:** re-points the gather model on the EXISTING up-to-6
  gather turns — adds no turns; each still bounded by `llm_timeout_ms`. A
  slower tool model multiplies across `max_iterations` but stays inside
  the same 4200s envelope. The design keeps the bound honest (does not
  hide it).
- **F-RETRYBUDGET** (budget 1→2) would re-derive `WriteTimeout` to 4800s —
  the explicit reason it is a recorded operator knob, not a silent change.

---

## Capability Foundation (DE4)

**Proportionality trigger:** a shared contract surface across two services
(Telegram + web/HTTP `/ask`), now extended along three new axes
(persistent default × per-user sticky × gather+synthesis turn). The
selection capability — **claim-bound sticky persistence + allowlist/tool-
capability validation + precedence resolution + per-invocation config
construction + model+source attribution** — is ONE capability consumed by
thin per-surface carriers, NOT re-implemented per surface (FR-9/FR-10;
SCN-089-A08/A11 prove parity).

### Capability Foundation (shared, surface-agnostic)
- **`modelswitch.Allowlist`** (extended) — `ResolveEffective` (precedence
  + source), `ResolveGather` (tool-capability), the `model_not_tool_capable`
  reason code + message. Pure, leaf.
- **`modelswitch.Override{SynthesisModel, GatherModel}`** + **`Effective`**
  — the validated per-invocation result + its source classification.
- **`modelpref.Store`** — the ONE claim-bound per-user sticky store,
  consumed by both surfaces, keyed on the authenticated principal.
- **`Agent.WithModelOverride(Override)`** — the per-request clone (synthesis
  + gather), singleton never mutated (C6).
- **`agenttool` singleton accessors** (`SwitchableModels`, plus the wired
  pref store + tool-capable set) — installed once, reached by both
  fast-paths.

### Concrete Implementations (per-surface carriers — thin)
- **Telegram:** parse `--model=`/`--gather-model=`; `/model` bot command;
  footer render (source tags, dual gather form, only-on-override).
- **Web/HTTP:** `model`/`gather_model` request fields; `GET/PUT/DELETE
  /v1/agent/model` route; JSON envelope (`model`/`model_source`/
  `gather_model`/`gather_model_source`, always present).

### Variation Axes (≥2)
1. **Surface composition** — Telegram chat (human footer, only-on-override)
   vs HTTP JSON (structured fields, always present).
2. **Selection persistence** — per-request (one invocation, stateless) vs
   per-user sticky (DB row, claim-bound, persists across turns).
3. **Turn target** — synthesis turn (reasoning, switchable set) vs gather
   turn (tool-calling, tool-capable set) — different validators, different
   constraints, one resolver.
4. **Rejection transport** — Telegram reply text vs HTTP 400 +
   `error_code`/`rejected_turn`; SAME `Message` sentence (parity).

The single concrete *capability* is justified (spec.md §10): exactly one
open-knowledge loop; a selection is a SELECTION over operational config
(an Ollama model id), not a new code-level provider/strategy. The
capability-first binding is the **cross-surface selection contract**,
satisfied by the foundation above.

---

## Trust-Invariant Preservation Matrix ("trust contracts unchanged")

The selection changes WHICH model runs (and WHICH turn), never the trust
perimeter. Every invariant runs on the turn OUTPUT and is inherently
model-/selection-agnostic (design 088 CT-8).

| Invariant | How preserved under any selection | Proof |
|---|---|---|
| Cite-back verifier (hash-match, enforce) | Unchanged; runs on the post-`<think>`-strip text from whichever synthesis model ran. | Fake-LLM trace: switched/sticky/default model emits a fabricated citation ⇒ still refused (SCN-089-A09 sibling). |
| Provenance gate (no zero-source) | Untouched (downstream in the Facade/assembler); no selection reaches it. | Switched/sticky-model zero-source ⇒ refuse-with-capture. |
| Capture-as-fallback (Facade, inviolable) | Untouched on every path where the agent RUNS. A selection **rejection** is pre-agent request validation (no agent run, no capture) — parallel to the spec-088 "Rejection ≠ capture-skip" and the `raw_input` 4xx path. | No-ground under any selection ⇒ capture still fires. |
| `<think>`-strip + retry-before-salvage (spec 087) | `stripThinkBlocks` / `synthesisNeedsRetry` run on the selected synthesis model's output before any parse/salvage; the retry fires identically under default/sticky/per-request (FR-14). | Empty forced-final under the 32b default ⇒ escalated retry ⇒ salvage (regression test). |
| Output hygiene (FR-13) | `stripContractScaffolding` on the salvage arms removes residual `<CITATIONS>` under ANY model. | `<CITATIONS>` fragment never reaches the body (SCN-089-A09). |
| No runtime SST mutation (C6) | `WithModelOverride` clones (synthesis + gather); the singleton `cfg` is never written; the sticky pref is DB row state, NOT an SST write. | Clone test: singleton `Model`/`SynthesisModel` unchanged after override; SST baseline file unchanged by `/model`. |
| Claim-binding (FR-5 / spec 044) | Sticky keyed ONLY on `msg.UserID` / `auth.UserIDFromContext`; body user ids ignored. | Two-user test: B never reads/sets A's pref; spoofed body id ignored (SCN-089-A04). |
| Baseline byte-for-byte (NFR-4) | No sticky + no per-request ⇒ `Effective.Override()` is zero ⇒ `WithModelOverride` returns the receiver; no footer; same render (now pointing at the new 32b default). | Baseline render golden unchanged but for the default model id (SCN-089-A01). |

**Rejection ≠ capture-skip violation** (carried from design 088): a
selection rejection (off-allowlist / over-envelope / non-tool-capable, on
synthesis OR gather, per-request OR sticky-set) is **pre-agent request
validation** — the agent never runs, the selection is a malformed control
parameter. Capture-as-fallback remains inviolable on every path where the
agent actually executes.

---

## Change Manifest (file-by-file, for `/bubbles.plan` → implement)

Data flow (no selection ⇒ identical to the new SST baseline, NFR-4):

```
Telegram:  /ask [--model=S] [--gather-model=G] <q>     |  HTTP: POST /v1/agent/invoke {model:S, gather_model:G}
  resolve actor (msg.UserID | auth.UserIDFromContext)  ── claim-bound (CT-2/CT-3)
  sticky := modelPref.Get(actor)                       ── one PK read
  eff, rej := allow.ResolveEffective(S, G, sticky)     ── precedence + validate + source (FR-6/FR-9/FR-16)
  rej != nil ─▶ fail-loud (Telegram reply | HTTP 400 + rejected_turn)   NO agent call, NO capture
  ok        ─▶ CurrentAgent().WithModelOverride(eff.Override()).Run(...) ── per-request clone (C6)
               attribute: turn.Model/turn.GatherModel + eff sources      (FR-11)
  /model set|show|reset  ─▶ shared modelPref CRUD + shared renderer       (claim-bound, no agent run)
```

| # | File | Change |
|---|------|--------|
| 1 | `config/smackerel.yaml` | self-hosted: `ollama_memory_limit` 28G→48G; `assistant_open_knowledge_synthesis_model_id` 7b→32b; `switchable_models` += 32b; NEW `assistant_open_knowledge_tool_capable_gather_models`. Base `assistant.open_knowledge`: NEW `tool_capable_gather_models` (dev = baseline). |
| 2 | `internal/config/openknowledge.go` | `OpenKnowledgeConfig.ToolCapableGatherModels []string`; load via `lookupJSONStringList("ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS")`; `Validate`: non-empty + `llm_model_id ∈ set`. |
| 3 | `internal/config/config.go::validateModelEnvelopes` | (a) NEW standing-default co-residence guard (resolve `synthesis_model_id` + gather ≤ envelope) — closes CT-6 gap (Fork A.2); (b) each `tool_capable_gather_models` entry profiled (sanity). Fail-loud. |
| 4 | `scripts/commands/config.sh` | resolve+emit `ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS` (per-env override pattern, mirror `switchable_models`). |
| 5 | `internal/assistant/openknowledge/modelswitch/allowlist.go` | `Override{+GatherModel}`; `Effective` + `Source*` consts; `ResolveEffective`; `ResolveGather`; `toolCapableGather` field + `NewAllowlist` param; `ReasonNotToolCapable="model_not_tool_capable"` + its message template; orphaned-sticky→default rule. |
| 6 | `internal/assistant/openknowledge/agent/agent.go` | `WithModelOverride` also sets `clone.cfg.Model` (gather, Fork C); `TurnResult.GatherModel` + finalize stamp; NEW `stripContractScaffolding` applied on the salvage arms (FR-13). |
| 7 | `internal/assistant/openknowledge/agenttool/substrate_tool.go` | `outputEnvelope` += `ModelSource`/`GatherModel`/`GatherModelSource`; `MapTurnResult` sets `Model`/`GatherModel`; a `WithSelection(env, eff)` helper stamps the source fields. |
| 8 | `internal/db/migrations/059_user_model_preferences.sql` | NEW table (Fork B). |
| 9 | `internal/assistant/openknowledge/modelpref/` | NEW leaf store: `Store` interface + `PostgresStore` (`Get`/`Set` upsert/`Clear`). |
| 10 | `internal/assistant/contracts/message.go` | `AssistantMessage.GatherModelOverride string` (typed, untrusted). |
| 11 | `internal/assistant/contracts/response.go` | `ModelAttribution` += `SynthesisSource`/`GatherModel`/`GatherSource`/`GatherOverridden`; update `response_test.go` field inventory. |
| 12 | `internal/telegram/assistant_adapter/translate_inbound.go` | `parseModelFlag` also consumes `--gather-model=`; set `GatherModelOverride`. |
| 13 | `internal/telegram/assistant_adapter/render_outbound.go` | `appendModelFooter`: source tags + dual gather form; footer when synthesis OR gather non-default; baseline still footer-free. |
| 14 | `internal/telegram/` (`bot.go` + NEW `model_command.go`) | `case "model"` dispatch → claim-bound `/model` set/show/reset via the shared store + shared renderer. |
| 15 | `internal/assistant/facade.go` | open_knowledge fast-path: read sticky (`msg.UserID`), call `ResolveEffective`, clone+run, stamp extended `ModelAttribution`; thread `msg.GatherModelOverride`; `runOpenKnowledgeDirect` takes the full `Override` (synthesis+gather). |
| 16 | `internal/api/agent_invoke.go` | `AgentInvokeRequest.GatherModel`; fast-path reads `auth.UserIDFromContext` + sticky, calls `ResolveEffective`, clone+run; `openKnowledgeRejectionEnvelope` += `rejected_turn`; success envelope += `model_source`/`gather_model`/`gather_model_source`. |
| 17 | `internal/api/agent_model.go` | NEW `GET/PUT/DELETE /v1/agent/model` claim-bound handlers (mirror `annotation_list.go`). |
| 18 | `internal/api/router.go` | mount `/agent/model` GET/PUT/DELETE in the SAME bearer-auth `/v1` group as `/agent/invoke`. |
| 19 | `internal/api/health.go` (`Dependencies`) | add the `modelpref.Store` + `AgentModelHandler` deps. |
| 20 | `cmd/core/wiring_assistant_openknowledge.go` + api wiring | pass `ToolCapableGatherModels` to `NewAllowlist`; construct + inject the `modelpref` store (facade + api); add to the boot log. |
| 21 | `deploy/contract.yaml` | add `assistant.open_knowledge.tool_capable_gather_models` (string[]). |
| 22 | `cmd/core/main.go` | comment-only: `WriteTimeout` stays 4200s; the 32b default + gather override add no turns (NFR-2 honest). |
| 23 | `docs/Operations.md` | hot-swap runbook (Fork D) + `/model` sticky + `--gather-model=` + the standing-default footprint note (A/B headroom). |
| 24 | tests | §Testing Strategy below (modelswitch, agent, modelpref, telegram, api, config). |

---

## API Contracts (from-analysis)

**`POST /v1/agent/invoke`** (open_knowledge fast-path; both override fields
optional; absent ⇒ resolve by precedence):
```json
{ "scenario_id": "open_knowledge", "raw_input": "<q>", "model": "deepseek-r1:7b", "gather_model": "gemma4:26b" }
```
**200** (envelope ALWAYS carries `model`+`model_source`; gather fields
always present since gather always runs):
```json
{ "status": "success", "body": "…", "termination": "final",
  "model": "deepseek-r1:7b", "model_source": "per_request",
  "gather_model": "gemma4:26b", "gather_model_source": "default", "sources": [ … ] }
```
A bare baseline call ⇒ `"model":"deepseek-r1:32b","model_source":"default"`.
`model_source`/`gather_model_source` ∈ `{default, sticky, per_request}`.

**400** (selection rejected; pre-agent request validation; `rejected_turn`
discriminates the turn):
```json
{ "status": "rejected", "error_code": "model_not_tool_capable",
  "rejected_model": "deepseek-r1:7b", "rejected_turn": "gather",
  "allowed_models": ["gemma4:26b","llama3.1:8b"], "default_model": "deepseek-r1:32b",
  "message": "\"deepseek-r1:7b\" can't be used as the gather model … nothing was sent to the model. Tool-capable gather models: gemma4:26b (default), llama3.1:8b." }
```
`error_code` ∈ `{model_not_allowlisted, model_over_memory_envelope,
model_not_tool_capable}`; `rejected_turn` ∈ `{synthesis, gather}`. The
first two reuse the spec-088 `Rejection` verbatim.

**`GET/PUT/DELETE /v1/agent/model`** (claim-bound; keyed on the PASETO
subject; body never carries a user id):
```
GET    → 200 { "effective_model":"deepseek-r1:32b","source":"default","sticky_model":null,
               "system_default":"deepseek-r1:32b","allowed_models":[…] }
PUT  {"model":"deepseek-r1:7b"}
       → 200 { "status":"set","sticky_model":"deepseek-r1:7b","source":"sticky","system_default":"deepseek-r1:32b" }
       → 400 { …same Rejection shape; sticky_model UNCHANGED… }
DELETE → 200 { "status":"reset","sticky_model":null,"effective_model":"deepseek-r1:32b","source":"default" }
```

## Data Model (from-analysis)

The ONE new persistent entity (Fork B); everything else is config +
per-invocation in-memory carriers (C6).

| Datum | Type | Lifetime | Source |
|---|---|---|---|
| `user_model_preferences(actor_user_id PK, synthesis_model, gather_model⊘, updated_at)` | SQL row | until changed/reset | `/model` set / `PUT /v1/agent/model` |
| `synthesis_model_id` (standing default) | string (SST) | process / per-deploy | `config/smackerel.yaml` (env-resolved) |
| `switchable_models`, `tool_capable_gather_models` | `[]string` (SST) | process | `config/smackerel.yaml` (env-resolved) |
| `Allowlist` (incl. tool-capable set) | immutable struct | process | built at wiring from SST |
| `ModelOverride`, `GatherModelOverride` / `model`,`gather_model` | raw strings | one request | message/JSON carriers |
| `Effective` / `Override` | structs | one invocation | `ResolveEffective` output |
| `Rejection` | struct | one request | resolver output |
| `TurnResult.Model` / `.GatherModel` | strings | one turn | `finalize` stamp |

⊘ `gather_model` reserved for F-STICKY-GATHER; unread today.

## Authorization Matrix

Single self-hosted operator; the selection adds NO role surface — the
allowlist/tool-capable sets are the **integrity** boundary (an untrusted
model string can never reach Ollama un-validated, NFR-1 / OWASP A03/A08),
and the sticky store is a **per-user access** boundary (a user can only
read/write THEIR OWN preference, OWASP A01).

| Surface | Who | Allowed | Boundary |
|---|---|---|---|
| `/ask --model=`/`--gather-model=`, API `model`/`gather_model` | authenticated caller | allowlisted synthesis / tool-capable gather only | `ResolveEffective`/`ResolveGather` |
| `/model` set/show/reset, `GET/PUT/DELETE /v1/agent/model` | authenticated caller | own preference only | `actor_user_id` = claim-bound subject; body id ignored |

---

## Testing Strategy (scenario → test)

Dev has no Ollama daemon (mechanism-level proof, mirroring spec 087/088).

| Scenario | Test type | Location | Assertion |
|---|---|---|---|
| SCN-089-A01 default applied | unit | `agent_invoke_test.go`, `facade_*_test.go` | bare call ⇒ envelope `model=<default>`,`model_source=default`; no footer |
| A02 sticky persists | unit | `modelpref/*_test.go`, `facade_*_test.go` | `Get` after `Set` returns the model across two invocations |
| A03 show/reset | unit | `agent_model_test.go`, telegram `model_command_test.go` | GET shows effective+allowed+default; DELETE clears |
| A04 claim-bound | unit (adversarial) | `agent_model_test.go`, `modelpref/*_test.go` | B never reads A; body user-id ignored (`auth.UserIDFromContext` only) |
| A05 precedence | unit | `modelswitch/allowlist_test.go` | `ResolveEffective`: per-request > sticky > default; source tags correct |
| A06 footprint guard | unit | `internal/config/*_test.go` | over-envelope `synthesis_model_id` ⇒ `validateModelEnvelopes` fails loud naming model+envelope |
| A07 gather tool-capability | unit | `modelswitch/allowlist_test.go` | tool-capable applied; non-tool-capable ⇒ `model_not_tool_capable` |
| A08 off-allowlist parity | unit (adversarial) | `modelswitch/`, `facade_*`, `agent_invoke_test.go` | same `Rejection` both surfaces; no backend call |
| A09 scaffolding strip | unit | `agent/*_test.go` | `<think>`/`<CITATIONS>` never in body (incl. salvage arms) |
| A10 forced-final retry | unit (regression) | `agent/*_test.go` | empty→escalated-retry→empty→salvage-with-sources; never blank |
| A11 multi-surface parity | unit (adversarial) | cross-surface table | same `Effective`/`Rejection` both surfaces |
| A12 attribution+source | unit | `substrate_tool_test.go`, `render_outbound_test.go` | envelope+footer carry model+source; dual gather form |
| A13 hot-swap | doc + boot-log assertion | `docs/Operations.md`, wiring test | boot log names new `synthesis_model`; runbook complete |

Regression guard: MUST NOT regress the 9 spec-084 + 7 spec-087 agent
tests or the spec-088 suite (C8).

---

## Complexity Tracking

| Deviation from simplest viable | Simpler alternative considered | Why rejected |
|---|---|---|
| NEW `user_model_preferences` table + store package | Reuse `recommendation_preference_corrections` (actor-keyed) | Couples an unrelated capability's table + CHECK vocabulary to open-knowledge; no schema saving over a purpose-built 4-column table (Fork B). |
| SEPARATE `--gather-model=` + a `tool_capable_gather_models` SST set | One `--model=` re-points both turns | A combined flag silently pushes a non-tool-capable synthesis model onto the gather turn, degrading evidence + confounding the A/B (FR-8 hazard, Fork C). |
| NEW standing-default co-residence guard in `validateModelEnvelopes` | Rely on the spec-088 switchable pass (32b is also switchable) | The standing default is the EVERY-QUERY model; coupling its safety to "it happens to also be switchable" is fragile — an explicit guard is correct + cheap (Fork A.2). |
| `Effective` + `ResolveEffective` (precedence + source in one pure fn) | Inline precedence at each surface | Two surfaces would drift; FR-9/FR-10 mandate ONE resolver; source classification must be identical (SCN-089-A11). |
| `stripContractScaffolding` on salvage arms (FR-13) | Leave the residual `<CITATIONS>` leak | The A/B observed it reaching user bodies; cheap, model-independent, default-path hygiene now that 32b is standing. |
| `gather_model` reserved column (nullable, unread) | Omit it; add a column when sticky gather ships | Directive-listed 4-column shape; a nullable reserved column is cheaper than a later migration and the resolver simply ignores it (F-STICKY-GATHER). |

Everything else is the simplest viable approach (the spec-088 validator /
clone / attribution / parity seam are EXTENDED, not re-built).

---

## Findings (deferred, non-blocking)

- **F-HOTRELOAD** — true zero-downtime config-hot-reload (config-watch +
  atomic `agenttool.SetAgent`/`SetSwitchableModels` swap). Deferred (Fork
  D); the ~15s core-recreate is sufficient for a single-operator product.
  The singleton is already an `atomic.Pointer`, so the swap primitive
  exists if picked up.
- **F-STICKY-GATHER** — a per-user sticky gather selection (a
  `/gather-model <id>` knob + the reserved `user_model_preferences.gather_model`
  column + a one-line precedence extension). Deferred (Fork C lean =
  gather per-request only); the carrier/resolver/store are designed to
  accept it additively.
- **F-FOOTPRINT** — an explicit synthesis-turn `num_ctx` bound (a
  real-footprint-aware guard that caps the KV-cache independent of the
  cgroup cap). Deferred (Fork A.5); the Docker `OLLAMA_MEMORY_LIMIT`
  cgroup cap already bounds the real KV footprint (A/B-verified). Revisit
  only on observed pressure.
- **F-RETRYBUDGET** — raise self-hosted `synthesis_retry_budget` 1→2 for the
  32b standing default (two escalated retries before salvage), at the cost
  of `WriteTimeout` 4200s→4800s. Deferred as an operator SST knob (NFR-2
  reliability↔latency tradeoff), recorded so the choice is conscious.

---

## Open Questions

None blocking. The four findings above are named, non-blocking next
levers, not unresolved forks. Forks A-D are decided; the §2 footprint
concern is resolved (Fork A's four-part mechanism). Ready for
`/bubbles.plan`.
