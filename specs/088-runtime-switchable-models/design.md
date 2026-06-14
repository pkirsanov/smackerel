# Design 088 — Runtime-Switchable Models

> Design authored by `bubbles.workflow (parent-expanded)` during the
> `analyze-design-plan` prelude (no specialist sub-agent dispatch
> available in this runtime; same precedent as specs 084 / 087). Reads
> the analyst spec.md (FR/NFR/SCN/forks), the UX uservalidation.md +
> spec.md `## UI Wireframes` / `## User Flows`, and the dependency
> design 087 (house style + the synthesis-model seam this builds on).
>
> Order: **Design Brief** (human steering checkpoint) → **Current
> Truth** (solution-blind, grounded in committed code) → **Decision
> Record** (Forks A/B/C, each decision + rationale + recorded
> alternative) → **Implementation Design** (data flow, SST allowlist,
> validator, parity seam, attribution, latency) → **Trust-Invariant
> Preservation** → **Change Manifest** → **Complexity Tracking** →
> **Findings**.
>
> **Extends, does NOT amend** spec 087. Every spec-064/084/087 trust and
> synthesis behaviour is preserved verbatim and made model-agnostic.

---

## Design Brief

A code owner can review this ~45-line brief and catch a wrong pattern or
direction BEFORE the full design + plan are generated.

**Current State.** The open-knowledge `/ask` model is a process-wide
singleton: `okagent.New(..., okagent.Config{Model, SynthesisModel})` is
built once at wiring ([cmd/core/wiring_assistant_openknowledge.go](../../cmd/core/wiring_assistant_openknowledge.go#L229))
from SST, installed via `agenttool.SetAgent`, and read by BOTH surfaces
through `agenttool.CurrentAgent().Run(ctx, prompt)`. There is no
per-request override and no answer-level model attribution on the
fast-path. Spec 087 split the synthesis turn onto a reasoning model
(`deepseek-r1:7b` home-lab) but could not prove the swap live (dev has no
GPU/Ollama).

**Target State.** An OPTIONAL, allowlist-validated, per-invocation model
override that re-points the spec-087 **synthesis turn** for one `/ask`,
on BOTH surfaces, with the answering model attributed back — enabling a
live, no-redeploy `gemma4:26b`-vs-`deepseek-r1:7b` A/B. No override ⇒
byte-for-byte today's baseline.

**Patterns to Follow.**
- The `agenttool` package singleton (`SetAgent`/`CurrentAgent`,
  [substrate_tool.go](../../internal/assistant/openknowledge/agenttool/substrate_tool.go#L97-L113))
  — the override allowlist installs the SAME way (one instance, reached
  by both structurally-separate surfaces).
- The spec-087 SST split (`synthesis_model_id`) load+validate+resolve
  chain ([openknowledge.go](../../internal/config/openknowledge.go),
  [config.sh](../../scripts/commands/config.sh#L1472), the home-lab
  `environments.<env>` override) — the new `switchable_models` SST list
  mirrors it exactly.
- The fail-loud envelope guard ([config.go::validateModelEnvelopes](../../internal/config/config.go#L2125))
  — the switchable-list envelope-consistency check lands here.
- The spec-087 fake-LLM agent tests (dev has no Ollama; in-repo proof is
  mechanism-level).

**Patterns to Avoid.**
- Do NOT mutate the singleton `okagent.Config` at runtime (C6 / SST
  build-once) — the override is a per-request **clone**, never a write.
- Do NOT re-point the tool/gather turns with `--model=` (running a weak
  tool-caller like `deepseek-r1` on the gather turns degrades evidence —
  the spec-087 split exists precisely to avoid that).
- Do NOT stuff the override into `TransportMetadata` — use a typed field
  (the owner directive's explicit preference).
- Do NOT validate per-surface — ONE validator, ONE allowlist (SCN-088-A06).

**Resolved Decisions.** Fork A → per-request only (sticky deferred).
Fork B → `--model=` re-points the synthesis turn only (tool-model knob
deferred). Fork C → defer compare-both (ask-twice already enables A/B).
Allowlist → new explicit `assistant.open_knowledge.switchable_models` SST
list. Carrier → typed `contracts.AssistantMessage.ModelOverride` (Telegram)
+ `AgentInvokeRequest.Model` (HTTP). Per-request clone → new
`Agent.WithModelOverride`. Attribution → new `TurnResult.Model`.

**Open Questions.** None blocking. Three deferred follow-ups recorded as
findings (F-STICKY, F-TOOLMODEL, F-COMPARE-LATENCY); each is a named,
non-blocking next lever, not an unresolved fork.

---

## Current Truth (gathered 2026-06-13, solution-blind)

Verified directly against committed code (not re-derived from the spec —
confirmed it).

### CT-1 — Both surfaces converge on ONE singleton agent, via TWO different fast-paths
- **Telegram:** `/ask` → bot dispatch (`case "ask"`,
  [bot.go](../../internal/telegram/bot.go#L596)) → assistant adapter
  `HandleUpdate` → `translateInbound`
  ([translate_inbound.go](../../internal/telegram/assistant_adapter/translate_inbound.go))
  builds `contracts.AssistantMessage` (slash preserved so
  `assistant.LookupShortcut` stamps `ScenarioID=open_knowledge`) →
  `Facade.Handle` ([facade.go](../../internal/assistant/facade.go#L391))
  → open_knowledge fast-path (facade.go:1062) →
  `runOpenKnowledgeDirect` (facade.go:1190) →
  `okagenttool.CurrentAgent().Run(ctx, prompt)` (facade.go:1204).
- **Web/HTTP:** `POST /v1/agent/invoke` with `scenario_id=open_knowledge`
  → `AgentInvokeHandlerFunc`
  ([agent_invoke.go](../../internal/api/agent_invoke.go#L87)) → the
  explicit-scenario fast-path → `agenttool.CurrentAgent().Run(r.Context(),
  prompt)` **directly, bypassing the Facade**, then
  `agenttool.MapTurnResult(turn)` → `writeOpenKnowledgeResponse`.
- **Consequence (architecture-shaping):** the override cannot be threaded
  through the Facade alone — the HTTP fast-path never touches the Facade.
  The shared validator + allowlist + per-request clone MUST live where
  BOTH paths reach them: the `agenttool` singleton boundary.

### CT-2 — The singleton model fields are static after wiring
- [agent.go::Config](../../internal/assistant/openknowledge/agent/agent.go#L108)
  carries `Model` (gather/tool turns) and `SynthesisModel` (spec-087
  forced-final turn + retries), both REQUIRED non-empty in `New`
  (agent.go:167, G028 fail-loud).
- `okagent.New(...)` is called once (wiring_assistant_openknowledge.go:229)
  with `Model: okCfg.LLMModelID`, `SynthesisModel: okCfg.SynthesisModelID`;
  `agenttool.SetAgent` installs it into `atomic.Pointer[okagent.Agent]`
  (substrate_tool.go:97-109). No per-request, per-user, or runtime
  override exists anywhere in the path.

### CT-3 — Per-turn model selection inside `Run` (the override target)
- [agent.go::Run](../../internal/assistant/openknowledge/agent/agent.go#L256):
  `reqModel := a.cfg.Model` (agent.go:332) for every gather turn; on the
  forced-final turn `iter == MaxIterations-1` it sets `reqModel =
  a.cfg.SynthesisModel` (agent.go:342) and builds
  `llm.ChatRequest{Model: reqModel,...}` (agent.go:361); the synthesis
  retries also use `a.cfg.SynthesisModel` (agent.go:394).
- **The answer is NOT always produced by the synthesis model.** If the
  model emits `StopEndTurn` on an EARLY iteration (≤ MaxIterations-2), the
  final text comes from the **tool model**. The synthesis model only
  produces the answer when the loop reaches the forced-final turn (the
  norm for the comparison-class questions this feature targets). Honest
  attribution must report the model of the turn that actually produced
  the text, not "the synthesis model" unconditionally.

### CT-4 — The attribution gap
- [agent.go::TurnResult](../../internal/assistant/openknowledge/agent/agent.go#L70)
  has NO model field. Every terminal `TurnResult` is stamped through the
  single `finalize` closure (agent.go:278) — including `refuse(...)` and
  the empty-text / missing-CITATIONS / empty-citations salvage paths,
  which all `return finalize(TurnResult{...})` (agent.go:432, 449, 502,
  516). `finalize` is therefore the one chokepoint to stamp the answering
  model.
- [agenttool.outputEnvelope](../../internal/assistant/openknowledge/agenttool/substrate_tool.go#L137)
  (`status/body/refusal_cause/termination/sources`) has NO model field;
  `MapTurnResult` (substrate_tool.go:176) does not set one.
- `runOpenKnowledgeDirect` builds an `agent.InvocationResult`
  (facade.go:1191) and never sets `.Model` — though
  [InvocationResult.Model](../../internal/agent/executor.go#L251) exists
  and the facade already logs it.

### CT-5 — The transport carriers
- [contracts.AssistantMessage](../../internal/assistant/contracts/message.go#L10)
  carries `Text`, `UserID`, `Transport`, and `TransportMetadata
  map[string]string` (message.go:58). The Telegram adapter is the only
  thing that parses Telegram syntax; it builds the AssistantMessage.
- [AgentInvokeRequest](../../internal/api/agent_invoke.go#L76)
  (`raw_input` / `structured_context` / `scenario_id` / `source` /
  `confidence_floor`) is the HTTP carrier. Body capped at 64 KiB; JSON
  decoded; `raw_input` required.
- [AssistantResponse](../../internal/assistant/contracts/response.go#L23)
  is the Telegram-facing response; `Invocation *agent.InvocationResult`
  reaches the renderer. The open-knowledge Telegram render
  ([render_openknowledge.go](../../internal/telegram/assistant_adapter/render_openknowledge.go),
  dispatched from `buildTelegramRendering`,
  [render_outbound.go](../../internal/telegram/assistant_adapter/render_outbound.go#L34))
  emits `<body>\n\n[1] … (your graph)` / `from the web:` citation blocks.

### CT-6 — The SST model surface + envelope guard
- [config/smackerel.yaml](../../config/smackerel.yaml#L1025)
  `assistant.open_knowledge.{llm_model_id, synthesis_model_id,
  max_iterations, synthesis_retry_budget, llm_timeout_ms, …}` — every
  field REQUIRED + fail-loud. Home-lab `environments.<env>` overrides
  `assistant_open_knowledge_llm_model_id: "gemma4:26b"` (line 2046) and
  `assistant_open_knowledge_synthesis_model_id: "deepseek-r1:7b"` (line
  2057).
- `model_memory_profiles` (smackerel.yaml:1541): `gemma4:26b` 18432,
  `gemma3:4b` 4096, `deepseek-r1:7b` 4864, `deepseek-r1:32b` 22528, … —
  reachable in Go as
  [Config.MLModelMemoryProfiles](../../internal/config/config.go#L363)
  (`map[string]int`). Home-lab `ollama_memory_limit: "28G"`
  (smackerel.yaml:2006) → `Config.OllamaMemoryLimitMiB = 28672`
  (config.go:357).
- [validateModelEnvelopes](../../internal/config/config.go#L2125) already
  fails loud when a referenced model has no profile or busts the
  envelope (per-model + spec-082 concurrent-set sum). The open-knowledge
  models are on-demand specialists, deliberately NOT in its summed
  buckets — so adding the switchable check is additive, not a conflict.

### CT-7 — The latency invariant (spec 084 F-LAT / 087)
- [cmd/core/main.go](../../cmd/core/main.go) `WriteTimeout = (max_iterations
  + synthesis_retry_budget) × llm_timeout_ms = (6 + 1) × 600s = 4200s`
  is the real `/ask` fast-path ceiling (the fast-path bypasses the
  substrate per-tool timeout). Each LLM round is independently bounded by
  `llm_timeout_ms` (600s).

### CT-8 — The trust perimeter (spec 064/084/087, preserved)
- Cite-back verifier (`citeback.Verify` + `Decide`, enforce mode) runs on
  the post-`<think>`-strip text (agent.go ~470+); provenance gate (refuse
  zero-source) lives downstream in the Facade/assembler;
  capture-as-fallback is the Facade's unconditional job
  (facade.go open-knowledge no-ground hook ~1145); `stripThinkBlocks` +
  `synthesisNeedsRetry` (agent.go:387-411) are model-agnostic. None of
  these reads which model ran — they operate on the turn output — so they
  are inherently model-agnostic and need no change.

---

## Decision Record — the three forks

The analyst handed three forks (spec.md §8). Each is resolved to the
smallest buildable decision that fully enables the owner's live A/B, with
the rejected alternative recorded.

### Fork A — Override scope / stickiness → **DECISION: per-request only**

**Decision.** Ship the **per-request** override
(`/ask --model=<id> <q>` ; API `model` field). One invocation, no stored
state, no SST write. Per-user-sticky `/model <id>` is **deferred** as a
named follow-up (finding **F-STICKY**). Global runtime default is a
**non-goal** (conflicts with C6 / build-once-deploy-many).

**Rationale.** Per-request fully enables the owner journey: arm A
(`--model=gemma4:26b`) then arm B (`--model=deepseek-r1:7b`) back-to-back,
each attributed (uservalidation A/B journey steps 2-4). It needs **zero
new persistence**: the Telegram path carries the override on the message;
the HTTP path is already stateless per-request. Sticky would require a
brand-new per-user preference store — there is NO general per-user prefs
store today (only the PASETO minter
[per_user_token.go](../../internal/telegram/per_user_token.go)) — and the
HTTP surface has no user-session store at all, so sticky would need a DB
table + migration + a read/write path bolted onto BOTH surfaces. That is
disproportionate to an A/B need that per-request already satisfies.

**Recorded alternative (F-STICKY).** Per-user-sticky via a new
`assistant_user_model_prefs` table keyed `(user_id, transport)`, written
by `/model <id>`, cleared by `/model default`, read at the fast-path
before per-request resolution. Deferred — revisit only if the operator
asks for a persistent default. The per-request rejection + attribution
primitives are identical either way, so adding sticky later is additive,
not a redesign. The UX `/model` discovery screens (spec.md UI Wireframes)
collapse to a `/ask` help line under this decision.

### Fork B — Which turn(s) the override targets → **DECISION: synthesis turn only**

**Decision.** A single `--model=X` (and API `model: X`) re-points the
spec-087 **forced-final SYNTHESIS turn** (and its retries) for that one
invocation. It does **NOT** re-point the tool/gather turns. Re-pointing
the tool model is **deferred** as a named follow-up (finding
**F-TOOLMODEL**).

**Rationale.** The owner's motivating A/B is "which model *reasons*
better" on the synthesis turn (`gemma4:26b` gather vs `deepseek-r1:7b`
synthesize). Re-pointing only the synthesis seam:
- Exactly matches the A/B: `/ask --model=gemma4:26b` = "what if gemma
  synthesizes?" (arm A); `/ask --model=deepseek-r1:7b` = baseline
  reasoning (arm B). The gather always uses the strong tool-caller.
- Preserves the spec-087 architecture (strong tool-caller gathers,
  reasoning model synthesizes) — the override just swaps **which**
  reasoning model occupies the synthesis seat.
- Avoids the active harm of re-pointing BOTH turns: a reasoning model
  (`deepseek-r1`) has weak tool-calling, so running it on the gather
  turns would degrade evidence and **muddy the very A/B** the owner wants
  ("is it a better synthesizer?" gets confounded by "it gathered worse").

**Recorded alternative (F-TOOLMODEL).** A separate, optional
`--tool-model=Y` / API `tool_model` knob (same allowlist, same
validator) for the rarer "A/B the gather model" experiment. Not shipped:
every uservalidation item and the motivating journey target the reasoning
turn, and the carrier/validator/clone are designed general enough that
adding a second target field later is a thin extension, not a rework.

**Attribution consequence (honest, documented).** Because the answer is
produced by whichever turn emits the final text (CT-3), attribution
reports the **model of that turn**: for a comparison-class question that
reaches the forced-final turn (the A/B norm) it is the overridden
synthesis model; for a trivial question answered early it is the baseline
tool model even under a synthesis override (the synthesis seat was never
reached). This is truthful (Principle 8) and tells the operator whether
the synthesis turn actually ran — a feature, not a bug.

### Fork C — Compare-both affordance → **DECISION: defer (ship switchable primitive)**

**Decision.** Ship the per-request switchable primitive only. The owner
runs the A/B by **asking twice** (arm A, then arm B — the documented
journey). A first-class `/compare` (one command, two models, two labeled
answers) is **deferred** as a named follow-up (finding
**F-COMPARE-LATENCY**).

**Rationale.** The switchable primitive *fully* enables A/B (two attributed
answers via two asks). Compare-both runs **two full agent passes in one
request**, which doubles the worst case to `(6+1) × 600s × 2 = 8400s`,
**busting the current 4200s `WriteTimeout`** (C3). Shipping it honestly
would force either (a) a `WriteTimeout` bump to 8400s — a 2.3-hour
single-connection hold, undesirable — or (b) a streaming / partial-result
redesign. Both are disproportionate to a need ask-twice already meets.
Deferring keeps `WriteTimeout` **unchanged at 4200s** and the latency
contract honest for the shipped scope.

**Recorded alternative (F-COMPARE-LATENCY).** `/compare <q>` (Telegram) /
`compare_models: [a,b]` (API) running two passes under the full trust
perimeter, returning two `[ <model> ]`-labeled answers, with the UX's
"takes about 2× a normal ask" caveat surfaced up front and a re-derived
`WriteTimeout`. The capability-foundation split (below) keeps the validator
+ attribution reusable so a future `/compare` consumes them without
duplication. The UX compare-both wireframe (spec.md) is the binding
contract if/when this is picked up.

---

## Capability Foundation (DE4)

**Proportionality trigger:** a shared contract surface across two
services (Telegram + web/HTTP `/ask`). The override **validation +
allowlist gating + per-invocation config construction + answer
attribution** is ONE capability consumed by two thin per-surface carriers
— NOT re-implemented per surface (FR-6/FR-8; SCN-088-A06 proves parity).

### Capability Foundation (shared, surface-agnostic)
- **`modelswitch.Allowlist`** (new leaf package
  `internal/assistant/openknowledge/modelswitch`) — the closed-set
  validator: `Resolve(raw string) (Override, *Rejection)`. Owns the
  allowlist, the envelope-consistency check, the two reason-codes, and
  the canonical rejection sentence. Pure, no side effects (FR-8/FR-11).
- **`modelswitch.Override`** — the validated per-invocation result
  (`{SynthesisModel string}`; zero value = baseline, FR-1).
- **`modelswitch.Rejection`** — the typed fail-loud value
  (`{RejectedModel, DefaultModel string; AllowedModels []string;
  ReasonCode, Message string}`); ONE struct rendered by both surfaces.
- **`Agent.WithModelOverride(Override) *Agent`** — the per-request Config
  clone (the singleton is never mutated, C6).
- **`agenttool` singleton accessor** (`SetSwitchableModels` /
  `SwitchableModels`) — installs ONE `*modelswitch.Allowlist`, reached by
  both structurally-separate fast-paths (parallels `SetAgent`).

### Concrete Implementations (per-surface carriers — thin)
- **Telegram carrier:** parse `--model=<id>` in `translateInbound` →
  `AssistantMessage.ModelOverride`; the facade calls the shared
  `Resolve`; render the `Rejection.Message` as the reply and the
  `— model: <id>` footer on the answer.
- **HTTP carrier:** read `AgentInvokeRequest.Model`; the handler calls
  the shared `Resolve`; render the `Rejection` as a 400 JSON envelope and
  the `model` field on the success envelope.

### Variation Axes (≥2)
1. **Surface composition** — Telegram chat render (human footer, only on
   override) vs HTTP JSON envelope (structured `model`, always present).
2. **Rejection transport** — Telegram reply text vs HTTP 400 +
   `error_code` + structured fields; SAME `Message` sentence (parity).
3. **Override syntax** — `--model=` flag token (Telegram) vs JSON `model`
   field (HTTP), both normalising to one raw model string for one
   `Resolve`.

The single concrete *capability* (one validator, one allowlist, one
clone, one attribution primitive) is justified: there is exactly one
open-knowledge loop; the override is a SELECTION over operational config
(an Ollama model id), not a new code-level provider/strategy. A
provider-lifecycle Domain Capability Model is not warranted at this
granularity (spec.md §9). The capability-first binding is the
**cross-surface override contract**, satisfied by the foundation above.

---

## Implementation Design (file-by-file)

Data flow (per-request, no override ⇒ identical to today, NFR-4):

```
Telegram:  /ask --model=X <q>
  translateInbound  ── parse --model, strip flag ──▶ AssistantMessage{Text:<q>, ModelOverride:"X"}
  Facade.Handle (open_knowledge fast-path)
     allow := agenttool.SwitchableModels()
     ov, rej := allow.Resolve(msg.ModelOverride)     // UNTRUSTED → validated (FR-8)
     rej != nil ─▶ rejection AssistantResponse (NO agent call, NO capture)   [Flow b]
     ov  ok    ─▶ runOpenKnowledgeDirect(..., ov)
                    agent := CurrentAgent().WithModelOverride(ov)   // per-request CLONE (C6)
                    turn  := agent.Run(ctx, prompt)
                    result.Model = turn.Model                        // attribution (FR-7)
  buildTelegramRendering ── footer "— model: <turn.Model>" iff override applied

HTTP:      POST /v1/agent/invoke {scenario_id:open_knowledge, raw_input:<q>, model:"X"}
  AgentInvokeHandlerFunc (open_knowledge fast-path)
     allow := agenttool.SwitchableModels()
     ov, rej := allow.Resolve(req.Model)              // SAME validator (SCN-088-A06)
     rej != nil ─▶ HTTP 400 rejection envelope         [Flow b]
     ov  ok    ─▶ CurrentAgent().WithModelOverride(ov).Run(...)
                  env := MapTurnResult(turn)           // env.Model = turn.Model (always)
                  writeOpenKnowledgeResponse(env)
```

### CHANGE 1 — new SST list (`config/smackerel.yaml`)
Add under `assistant.open_knowledge` (after `synthesis_model_id`):
```yaml
    switchable_models: [ "gemma3:4b" ] # REQUIRED non-empty when enabled. Spec 088 — the allowlist of models /ask may be runtime-switched TO on the SYNTHESIS turn (per-request --model= / API model). Dev = [gemma3:4b] (== baseline synthesis; switching to it is a no-op but keeps the path testable; dev has no Ollama daemon). Each entry MUST have a model_memory_profiles entry AND fit the env ollama envelope when co-resident with llm_model_id (gather) — validated fail-loud in internal/config/config.go::validateModelEnvelopes (G028 / FR-10). Home-lab override below adds the real A/B set. NEVER a silent default.
```
Add to the home-lab `environments.<env>` block (after
`assistant_open_knowledge_synthesis_model_id`):
```yaml
    # Spec 088 — home-lab switchable set: the gemma4:26b-vs-deepseek-r1:7b
    # synthesis A/B. Both envelope-consistent: gather gemma4:26b (18432) +
    # synthesis candidate ≤ 28672 ollama_memory_limit (gemma4:26b loads
    # once when candidate==gather; +deepseek-r1:7b 4864 = 23296). deepseek-r1:32b
    # is NOT switchable here (40960 > 28672) — operator opt-up (raise the
    # envelope first), surfaced as the over-envelope rejection.
    assistant_open_knowledge_switchable_models: [ "gemma4:26b", "deepseek-r1:7b" ]
```

### CHANGE 2 — config load + structural validate (`internal/config/openknowledge.go`)
- `OpenKnowledgeConfig`: add `SwitchableModels []string`.
- `LoadOpenKnowledge`: `cfg.SwitchableModels, errs =
  lookupJSONStringList("ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS", errs)`
  (mirrors `ToolAllowlist` / `AllowedEgressHosts`).
- `Validate()` (enabled-gated): non-empty list; each entry non-empty
  (trim). Per-model envelope consistency is checked in CHANGE 3 (where
  the profiles + envelope live) — keep this struct-level only.

### CHANGE 3 — envelope-consistency validate (`internal/config/config.go::validateModelEnvelopes`)
- After the existing buckets, add a switchable-models pass (only when
  `c.Assistant.OpenKnowledge.Enabled` and `c.OllamaMemoryLimitMiB != 0`):
  for each `m` in `c.Assistant.OpenKnowledge.SwitchableModels`:
  - `profileMiB, ok := c.MLModelMemoryProfiles[m]` — `!ok` ⇒ append to
    `missing` (`switchable_models entry %q has no model_memory_profiles
    entry`).
  - `baseMiB := c.MLModelMemoryProfiles[llm_model_id]`;
    `coresident := baseMiB; if m != llm_model_id { coresident += profileMiB }`;
    `coresident > c.OllamaMemoryLimitMiB` ⇒ append to `oversized`
    (`switchable_models entry %q + gather model needs %d MiB but
    OLLAMA_MEMORY_LIMIT resolves to %d MiB`).
  - This makes FR-10 fail-loud at config-generation: an operator cannot
    ship a switchable list that busts the envelope. The same co-residence
    arithmetic the runtime validator uses (CHANGE 6).

### CHANGE 4 — config generation (`scripts/commands/config.sh`)
- Resolve via the per-environment override pattern (mirror
  `synthesis_model_id`, but JSON-list-shaped like `tool_allowlist`):
  ```sh
  ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS="$(yaml_get_json "environments.$TARGET_ENV.assistant_open_knowledge_switchable_models" 2>/dev/null || true)"
  if [[ -z "$ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS" ]]; then
    ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS="$(yaml_get_json assistant.open_knowledge.switchable_models)"
  fi
  ```
- Emit in the generated-env heredoc beside the other
  `ASSISTANT_OPEN_KNOWLEDGE_*` keys:
  `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS=${ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS}`.

### CHANGE 5 — new shared validator package (`internal/assistant/openknowledge/modelswitch/`)
New leaf package (stdlib + `contracts` only; no agenttool/facade/api dep
⇒ no cycle):
```go
type Entry struct { Model string; MemoryMiB int }
type Allowlist struct {            // immutable after build
    switchable   []string          // env-resolved switchable set (order preserved for messages)
    profiles     map[string]int    // model_memory_profiles
    envelopeMiB  int               // OllamaMemoryLimitMiB (0 ⇒ envelope check skipped, dev)
    gatherModel  string            // baseline llm_model_id (co-resident on synthesis turn)
    defaultModel string            // baseline synthesis_model_id (the no-override synthesis model)
}
type Override struct { SynthesisModel string } // zero value ⇒ baseline (FR-1)
func (o Override) IsZero() bool { return o.SynthesisModel == "" }

const (
    ReasonNotAllowlisted   = "model_not_allowlisted"
    ReasonOverMemEnvelope  = "model_over_memory_envelope"
)
type Rejection struct {
    RejectedModel string; AllowedModels []string; DefaultModel string
    ReasonCode string; Message string
}

func NewAllowlist(switchable []string, profiles map[string]int, envelopeMiB int, gatherModel, defaultModel string) (*Allowlist, error)
// fail-loud build: switchable non-empty; each entry profiled; if envelopeMiB!=0, each entry co-resident-fits; defaultModel non-empty.

func (a *Allowlist) Resolve(raw string) (Override, *Rejection) {
    raw = strings.TrimSpace(raw)
    if raw == "" { return Override{}, nil }            // baseline (FR-1, NFR-4)
    if a.isSwitchable(raw) { return Override{SynthesisModel: raw}, nil }
    return Override{}, a.reject(raw)                   // FR-3 (no silent default, no backend)
}
func (a *Allowlist) reject(raw string) *Rejection {
    code := ReasonNotAllowlisted
    if p, ok := a.profiles[raw]; ok {                  // profiled but not offered…
        cores := a.profiles[a.gatherModel]
        if raw != a.gatherModel { cores += p }
        if a.envelopeMiB != 0 && cores > a.envelopeMiB { code = ReasonOverMemEnvelope }
    }
    return &Rejection{RejectedModel: raw, AllowedModels: a.switchable,
        DefaultModel: a.defaultModel, ReasonCode: code, Message: a.message(raw, code)}
}
```
- `message(...)` builds the exact UX sentence (sentence-case, capital
  "I", em-dash, the deliberate capitalised **NOT**, lists the allowed set
  + default + retry hint) — ONE string used verbatim by BOTH surfaces.
  Two templates keyed on `ReasonCode` (off-allowlist vs over-envelope),
  matching the UX wireframe wording. The concrete model ids in the UX
  examples are env-resolved data; the binding contract is the sentence
  shape + the marked default.

### CHANGE 6 — per-request clone (`internal/assistant/openknowledge/agent/agent.go`)
- Add `Model string` to `TurnResult` (CT-4 chokepoint).
- `finalize` stamps it once: `if out.Model == "" { out.Model = answeringModel }`.
- Track `answeringModel` in `Run`: init `answeringModel := a.cfg.Model`
  before the loop; set `answeringModel = reqModel` each iteration after
  the switch computes `reqModel`; set `answeringModel = a.cfg.SynthesisModel`
  inside the synthesis-retry loop. (All terminal paths already route
  through `finalize`, so this is the only stamp needed.)
- Add the per-request clone:
  ```go
  // WithModelOverride returns a shallow per-invocation copy whose
  // SynthesisModel is replaced. The receiver (the SST singleton) is
  // NEVER mutated (C6). A zero override returns the receiver unchanged
  // so the no-override path is byte-for-byte identical (NFR-4). All
  // deps (llm, registry, verify, rec, log, traces, enforcement) are
  // concurrency-safe and shared.
  func (a *Agent) WithModelOverride(o modelswitch.Override) *Agent {
      if o.IsZero() { return a }
      clone := *a
      clone.cfg.SynthesisModel = o.SynthesisModel   // Fork B: synthesis turn only
      return &clone
  }
  ```
  (Importing `modelswitch` here is acyclic: `modelswitch` depends only on
  stdlib+contracts. If a leaf-purity concern arises, `WithModelOverride`
  may instead take a plain `synthesisModel string`; the agenttool seam
  passes `ov.SynthesisModel`. Either is fine — decide at implement.)

### CHANGE 7 — singleton accessor + envelope model (`agenttool/substrate_tool.go`)
- Add `outputEnvelope.Model string json:"model,omitempty"`;
  `MapTurnResult` sets `Model: turn.Model` on success AND refusal arms.
- Add the allowlist singleton (parallels `SetAgent`/`CurrentAgent`):
  ```go
  var allowlistRef atomic.Pointer[modelswitch.Allowlist]
  func SetSwitchableModels(a *modelswitch.Allowlist) { allowlistRef.Store(a) }
  func SwitchableModels() *modelswitch.Allowlist     { return allowlistRef.Load() }
  ```
  Installed alongside `SetAgent` (CHANGE 9). Both surfaces read
  `SwitchableModels()` — the ONE shared allowlist (SCN-088-A06).

### CHANGE 8 — carriers + surface wiring
- **`internal/assistant/contracts/message.go`** — add typed field
  `ModelOverride string` to `AssistantMessage` (owner directive: a typed
  field, NOT `TransportMetadata`). Untrusted; validated before use.
- **`internal/assistant/contracts/response.go`** — add
  `ModelAttribution *ModelAttribution` (new type
  `{ModelID string; OverrideApplied bool}`); update the field-inventory
  assertion in `response_test.go`.
- **`internal/telegram/assistant_adapter/translate_inbound.go`** — on the
  `/ask` shortcut branch, parse a leading `--model=<id>` token from the
  post-`/ask` arguments: set `msg.ModelOverride = <id>` and remove the
  token from `Text` (slash prefix preserved so `LookupShortcut` + the
  facade `StripShortcutPrefix` still work; the model the agent sees gets
  a clean question — same discipline as the BUG-064-001 prefix strip).
  Only `--model=` is consumed; everything else stays in `Text`.
- **`internal/assistant/facade.go`** — in the open_knowledge fast-path
  (facade.go:1062): `ov, rej := okagenttool.SwitchableModels().Resolve(
  msg.ModelOverride)`; on `rej != nil` build a rejection
  `AssistantResponse` (Status `rejected`/`StatusUnavailable`-class, Body =
  `rej.Message`) and SKIP the agent + assembler + provenance + capture
  (the rejection is pre-agent request validation — see Trust matrix);
  else `result = f.runOpenKnowledgeDirect(ctx, sc, env, emittedAt, ov)`
  and set `resp.ModelAttribution = &ModelAttribution{ModelID:
  result.Model, OverrideApplied: !ov.IsZero()}`.
- **`runOpenKnowledgeDirect`** — new `ov modelswitch.Override` param:
  `turn, runErr := okagenttool.CurrentAgent().WithModelOverride(ov).Run(
  ctx, prompt)`; set `result.Model = turn.Model`.
- **`internal/telegram/assistant_adapter/render_outbound.go`** — in
  `buildTelegramRendering`, after the open-knowledge `okOut` (and on the
  refusal/salvage arms), append `\n— model: <resp.ModelAttribution.ModelID>`
  iff `resp.ModelAttribution != nil && resp.ModelAttribution.OverrideApplied`.
  Baseline (no override) ⇒ no footer (SCN-088-A03 / NFR-4).
- **`internal/api/agent_invoke.go`** — add `Model string json:"model,omitempty"`
  to `AgentInvokeRequest`; in the open_knowledge fast-path: `ov, rej :=
  agenttool.SwitchableModels().Resolve(req.Model)`; on `rej != nil` write
  HTTP 400 with the rejection envelope (`status:"rejected"`, `error_code`,
  `rejected_model`, `allowed_models`, `default_model`, `message`); else
  `CurrentAgent().WithModelOverride(ov).Run(...)` → `MapTurnResult` (env
  carries `model`) → `writeOpenKnowledgeResponse`.

### CHANGE 9 — wiring (`cmd/core/wiring_assistant_openknowledge.go`)
- After `agenttool.SetAgent(agent)`, build + install the allowlist from
  the SAME SST already loaded:
  ```go
  allow, err := modelswitch.NewAllowlist(
      okCfg.SwitchableModels,
      cfg.MLModelMemoryProfiles,      // model_memory_profiles
      cfg.OllamaMemoryLimitMiB,       // env envelope (0 on dev ⇒ check skipped)
      okCfg.LLMModelID,               // gather model (co-resident)
      okCfg.SynthesisModelID)         // baseline synthesis = the "default"
  if err != nil { return fmt.Errorf("wireOpenKnowledge: switchable allowlist: %w", err) }
  agenttool.SetSwitchableModels(allow)
  ```
  Installed only when `open_knowledge.enabled` (same gate as `SetAgent`),
  so `SwitchableModels()` is non-nil exactly when `CurrentAgent()` is.
  Add `switchable_models` to the startup log line.

### CHANGE 10 — deploy contract (`deploy/contract.yaml`)
Add beside `tool_allowlist`:
```yaml
  - path: assistant.open_knowledge.switchable_models
    type: string[]
    secret: false
    notes: "Spec 088 — runtime-switchable synthesis models for /ask; non-empty when enabled; each entry profiled + co-resident-fits the ollama envelope; per-environment override via environments.<env>.assistant_open_knowledge_switchable_models"
```

### CHANGE 11 — latency invariant (`cmd/core/main.go` comment) — NO value change
`WriteTimeout` stays `4200s`. The synthesis-only per-request switch does
NOT add turns (it swaps the synthesis model on the existing forced-final
turn + retries, each still bounded by `llm_timeout_ms`). Update the
comment to record that a switched synthesis model is still bounded by the
same envelope, and that compare-both (deferred, F-COMPARE-LATENCY) would
require re-deriving this. (Mechanical: no code change beyond the comment.)

### CHANGE 12 — docs (`docs/Operations.md`)
Amend the open-knowledge section: the per-request `--model=` / API `model`
switch, the `switchable_models` allowlist + envelope-consistency, the
fail-loud rejection (two reason-codes), the `— model:` attribution, and
the unchanged `WriteTimeout`.

### CHANGE 13 — tests (mechanism-level; dev has no Ollama, mirror spec 087)
- `modelswitch/allowlist_test.go` — table-driven `Resolve`: baseline
  (empty ⇒ zero override), in-list ⇒ applied, off-list ⇒
  `model_not_allowlisted`, profiled-over-envelope ⇒
  `model_over_memory_envelope`, same-model-as-gather ⇒ single-load fits,
  message wording golden. `NewAllowlist` fail-loud build cases.
- `agent/*_test.go` — `WithModelOverride` clones (singleton cfg
  unchanged after clone; zero override returns receiver); `TurnResult.Model`
  stamped on success / salvage / refuse / early-StopEndTurn (fake-LLM
  traces). Update `baseCfg` only if needed (no new REQUIRED agent field).
- `agenttool/substrate_tool_test.go` — `outputEnvelope.Model` carried;
  `MapTurnResult` sets it on both arms.
- `internal/config/openknowledge_test.go`, `validate_test.go`,
  `spec_076_foundation_test.go`, `validate_ml_envelope_test.go` — add
  `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS` to the full-env maps; new
  fail-loud + over-envelope coverage.
- `internal/telegram/assistant_adapter/translate_inbound_test.go` —
  `--model=` parsed into `ModelOverride`, stripped from `Text`, slash
  preserved; `render_outbound_test.go` — footer on override, none on
  baseline.
- `internal/assistant/facade_*_test.go` — rejection short-circuits (no
  agent call, no capture); applied override threads to attribution.
- `internal/api/agent_invoke_test.go` — `model` field applied;
  off-allowlist ⇒ 400 envelope; baseline ⇒ envelope `model` present.
- `internal/assistant/contracts/response_test.go` — field inventory +
  `ModelAttribution`.

---

## SST Allowlist Mechanism (decision + semantics)

**Decision: a NEW explicit `assistant.open_knowledge.switchable_models`
list**, NOT reuse of `model_memory_profiles` as the allowlist.

**Why not reuse `model_memory_profiles`.** That map is a memory
*registry* of every profiled model (incl. vision/embedding/OCR models
irrelevant to `/ask`). Deriving the switchable set from it would offer
models that make no sense to synthesize with. The switchable set is an
operator **curation** ("you may switch `/ask` to these") and must be
explicit.

**Why the validator still consults `model_memory_profiles` +
`ollama_memory_limit`.** To produce the precise two-reason rejection
(SCN-088-A07): a profiled-but-too-big model (`deepseek-r1:32b`) gets the
helpful `model_over_memory_envelope` ("raise the envelope" — the spec-087
F-OPTUP opt-up), while an unknown/un-profiled string (`gpt-4o`,
`totally-made-up`) gets `model_not_allowlisted`. The allowlist is the
*offer*; the profiles+envelope distinguish *why* an off-offer model is
refused.

**Semantics (closed set):** a model is switchable **iff** it is in
`switchable_models` AND (by config-load CHANGE 3) co-resident-fits the
env envelope. The envelope arithmetic — `profiles[gather] (+
profiles[candidate] if candidate≠gather) ≤ OllamaMemoryLimitMiB` — is the
project's established spec-087 co-residence model (gather model resident
during synthesis), and correctly treats candidate==gather as a single
load. New config is REQUIRED + fail-loud (G028); no `${VAR:-default}`.

---

## Two-Surface Parity Seam (SCN-088-A06)

ONE allowlist, ONE validator, ONE rejection shape:

| Concern | Telegram | Web/HTTP | Shared seam |
|---|---|---|---|
| Override carrier | `AssistantMessage.ModelOverride` (parsed from `--model=`) | `AgentInvokeRequest.Model` (JSON) | both → one raw string |
| Allowlist | `okagenttool.SwitchableModels()` | `agenttool.SwitchableModels()` | **same singleton** |
| Validation | `allow.Resolve(raw)` | `allow.Resolve(raw)` | **same function** |
| Clone+run | `CurrentAgent().WithModelOverride(ov).Run` | `CurrentAgent().WithModelOverride(ov).Run` | same method |
| Rejection | reply = `rej.Message` | 400 + `error_code`/structured + same `Message` | **same `Rejection`** |
| Attribution | footer `— model:` iff override | envelope `model` always | `turn.Model` |

The two surfaces differ ONLY in (a) how the raw string is parsed and (b)
how the result is rendered — the validate→clone→run→attribute spine is
identical. Proof of parity is a test that drives the SAME off-allowlist
string through both and asserts the SAME `Rejection`.

---

## Model Attribution Wiring (FR-7, Principle 8)

- **Source of truth:** `turn.Model` (new), stamped by `finalize` to the
  model of the turn that produced the final text (CT-3/CT-4) — honest
  across success, salvage, refuse, and early-StopEndTurn.
- **HTTP (always):** `MapTurnResult` → `outputEnvelope.Model` →
  `writeOpenKnowledgeResponse`. Structured metadata, every success/
  baseline path. (Rejections carry `rejected_model`, not `model` — no
  model ran.)
- **Telegram (only on override/compare):** `runOpenKnowledgeDirect` sets
  `InvocationResult.Model`; the facade sets `ModelAttribution{ModelID,
  OverrideApplied}`; `buildTelegramRendering` appends `— model: <id>`
  iff `OverrideApplied`. Baseline ⇒ no footer (byte-for-byte spec-087,
  NFR-4 / Principle 6).
- **Salvage honesty:** the footer reads `— model: <id>` (neutral
  metadata), NOT `— answered by <id>`, so it never contradicts the
  honest-salvage "I searched but couldn't directly answer…" framing (C1).
  The salvage TurnResult is `StatusSuccess` + snippet sources, so it
  renders via the sourced-answer path and gets the footer.

---

## Latency Analysis (C3 / FR-12)

```
WriteTimeout = (max_iterations + synthesis_retry_budget) × llm_timeout_ms
             = (6 + 1) × 600s = 4200s   (UNCHANGED)
```
- **Per-request switch:** swaps WHICH synthesis model runs on the
  forced-final turn (+ retries). Turn COUNT is identical; each LLM round
  is still bounded by `llm_timeout_ms` (600s). A slower switched model
  (e.g. `deepseek-r1` on CPU) is bounded by the same per-round cap →
  worst-case envelope unchanged. **No `WriteTimeout` change.** (SCN-088-A08
  arm 1.)
- **Compare-both (DEFERRED, F-COMPARE-LATENCY):** two full passes in one
  request ⇒ `8400s` worst case, busting `4200s`. This is the explicit
  reason Fork C is deferred (it would force a `WriteTimeout` bump or a
  streaming redesign). If picked up, the doubled bound MUST be re-derived
  and the "~2× a normal ask" caveat surfaced up front (SCN-088-A08 arm 2 /
  spec.md compare wireframe). `cmd/core/main.go` stays honest at 4200s.

---

## Trust-Invariant Preservation Matrix ("trust contracts unchanged")

The override changes WHICH model runs, never the trust perimeter. Every
invariant runs on the turn OUTPUT and is inherently model-agnostic (CT-8).

| Invariant | How preserved under override | Proof |
|---|---|---|
| Cite-back verifier (hash-match, enforce) | Unchanged; runs on post-`<think>`-strip text from whichever model synthesized. | Fake-LLM trace: switched model emits fabricated citation ⇒ still refused (SCN-088-A05). |
| Provenance gate (no zero-source) | Untouched (downstream in Facade/assembler); the override never reaches it. | Switched-model zero-source ⇒ refuse-with-capture (SCN-088-A05). |
| Capture-as-fallback (Facade, inviolable) | Untouched on every path where the **agent runs**. | No-ground under override ⇒ capture still fires (SCN-088-A05). |
| `<think>`-strip + retry-before-salvage (spec 087) | `stripThinkBlocks` / `synthesisNeedsRetry` run on the switched synthesis model's output before any parse/salvage. | Reasoning-model `<think>` never leaks under override (SCN-088-A05). |
| No runtime SST mutation (C6) | `WithModelOverride` clones; the singleton `cfg` is never written. | Clone test: singleton `Model`/`SynthesisModel` unchanged after override (SCN-088-A01 "baseline untouched"). |
| Baseline byte-for-byte (NFR-4) | Zero override ⇒ `WithModelOverride` returns the receiver; no footer; same render. | Baseline render golden unchanged (SCN-088-A03). |

**Rejection ≠ capture-skip violation.** A model-override rejection is
**pre-agent request validation** (the agent never runs; the override is a
malformed control parameter) — parallel to the existing malformed-request
4xx path ([agent_invoke.go](../../internal/api/agent_invoke.go#L113)
`raw_input` check) and the UX's explicit "NO backend call, NO answer, NO
capture-as-answer for the rejected request". Capture-as-fallback remains
inviolable on every path where the agent actually executes; a pre-flight
validation refusal is not an agent run, so skipping capture there honours
C1's intent rather than violating it.

---

## API Contracts (from-analysis)

**Request** (`POST /v1/agent/invoke`, fast-path):
```json
{ "scenario_id": "open_knowledge", "raw_input": "<question>", "model": "deepseek-r1:7b" }
```
`model` OPTIONAL string; absent ⇒ baseline (FR-1).

**200 success** (extends the open-knowledge envelope with `model`,
always present):
```json
{ "status": "success", "body": "…", "termination": "final",
  "model": "deepseek-r1:7b", "sources": [ … ] }
```

**400 off-allowlist / over-envelope** (request-value validation failure,
caught before the agent runs):
```json
{ "status": "rejected", "error_code": "model_not_allowlisted",
  "rejected_model": "gpt-4o",
  "allowed_models": ["gemma4:26b","deepseek-r1:7b"],
  "default_model": "gemma4:26b",
  "message": "\"gpt-4o\" is not a switchable model. It was NOT used and the request did NOT fall back to the default — nothing was sent to the model. Switchable models: gemma4:26b (default), deepseek-r1:7b." }
```
(`error_code: "model_over_memory_envelope"` uses the same shape with the
over-envelope `message`.) The concrete ids are env-resolved;
`default_model` = the resolved baseline **synthesis** model.

**Telegram command surface (shipped):**
| Affordance | Shape |
|---|---|
| Per-request override | `/ask --model=<id> <question>` |
| Baseline (unchanged) | `/ask <question>` |
| Discovery (help line) | surfaced in `/ask` help (no `/model` store — Fork A deferred) |

## Data Model (from-analysis)

**No new persistent entities** (Fork A per-request ⇒ no store; C6 ⇒ no
SST write at runtime). The feature's "data" is config + per-request
in-memory carriers:

| Datum | Type | Lifetime | Source |
|---|---|---|---|
| `switchable_models` | `[]string` (SST) | process | `config/smackerel.yaml` (env-resolved) |
| `Allowlist` | immutable struct | process | built at wiring from SST |
| `ModelOverride` | raw string | one request | message/JSON carrier |
| `Override` | `{SynthesisModel}` | one invocation | `Resolve` output |
| `Rejection` | struct | one request | `Resolve` output |
| `TurnResult.Model` | string | one turn | `finalize` stamp |

## Authorization Matrix

Single self-hosted operator; the override adds NO new authz surface. The
existing transport auth gates access (Telegram chat-id mapping; HTTP
`auth_token`). The allowlist is the **integrity** boundary (an untrusted
model string can never reach Ollama un-validated — NFR-1 / OWASP A03/A08),
not a role boundary.

| Surface | Who | Override allowed? | Boundary |
|---|---|---|---|
| Telegram `/ask --model=` | mapped chat owner | yes, allowlisted only | allowlist `Resolve` |
| HTTP `/v1/agent/invoke` `model` | authenticated caller | yes, allowlisted only | allowlist `Resolve` |

---

## BDD Scenario Enrichment (technical)

Representative enrichment (the manifest entries SCN-088-A01..A08 are
authored at plan):

```gherkin
# SCN-088-A01 — applied (technical)
Given switchable_models = ["gemma4:26b","deepseek-r1:7b"], baseline synthesis = deepseek-r1:7b
When Resolve("gemma4:26b") runs
Then it returns Override{SynthesisModel:"gemma4:26b"} and no Rejection
And CurrentAgent().WithModelOverride(ov) clones with cfg.SynthesisModel="gemma4:26b"
And the singleton's cfg.SynthesisModel is still "deepseek-r1:7b"
And turn.Model surfaces the model that produced the final text

# SCN-088-A02 — off-allowlist (technical)
When Resolve("gpt-4o") runs
Then it returns Rejection{ReasonCode:"model_not_allowlisted", AllowedModels:[…], DefaultModel:"deepseek-r1:7b"}
And no Agent.Run is invoked (no Ollama call)
And the facade emits the rejection body / the API returns HTTP 400 with the same Message

# SCN-088-A07 — over-envelope (technical)
Given profiles[deepseek-r1:32b]=22528, gather gemma4:26b=18432, envelope=28672
When Resolve("deepseek-r1:32b") runs
Then 18432+22528=40960 > 28672 ⇒ Rejection{ReasonCode:"model_over_memory_envelope"}
```

## Scenario → Test Mapping

| Scenario | Test type (in-repo) | Location | Asserts |
|---|---|---|---|
| A01 applied + baseline-untouched | unit (fake-LLM) | `modelswitch`, `agent` | Resolve→Override; clone; singleton unchanged; `turn.Model` |
| A02 off-allowlist fail-loud | unit | `modelswitch`, `facade`, `api` | Rejection; no Run; reply / 400 |
| A03 no-override baseline | unit (golden) | `agent`, `render_outbound` | byte-for-byte; no footer |
| A04 attribution | unit | `agent`, `agenttool`, `render_outbound`, `api` | `turn.Model` → footer / `model` |
| A05 trust under override | unit (fake-LLM adversarial) | `agent`, `facade` | cite-back/provenance/`<think>`/capture hold |
| A06 parity | unit | `modelswitch`, cross-surface | same `Rejection` both surfaces |
| A07 untrusted / over-envelope | unit | `modelswitch`, `config` | two reason-codes; config-load fail-loud |
| A08 latency honesty | doc + unit | `main.go` comment, budget | turn count unchanged; 4200s bound |
| **live A/B (decisive)** | **home-lab (downstream)** | `bubbles.devops` (C7) | the gemma-vs-deepseek synthesis verdict |

Dev has no GPU/Ollama (C4/C7); in-repo proof is mechanism-level via
fake-LLM traces (mirrors spec 087). The decisive live A/B is the separate
downstream devops dispatch — no live result is fabricated here.

---

## Testing Strategy

- **Unit (Go, fake-LLM):** the `modelswitch` validator (all branches +
  message goldens + fail-loud build), `WithModelOverride` clone isolation
  (C6), `TurnResult.Model` stamping across every terminal path,
  `MapTurnResult` model carry, `translateInbound` `--model=` parse/strip,
  the Telegram footer (on override / not on baseline), the facade
  rejection short-circuit (no Run / no capture), the API 400 envelope +
  applied `model`, and the config full-env + envelope-consistency maps.
- **Adversarial (RED→GREEN):** an off-allowlist string that, if the
  validator were bypassed, would reach Ollama — asserts no Run; a
  reintroduced silent-default would flip the test (no tautology). A
  switched-model fabricated citation still refused. A singleton-mutation
  attempt detected by the post-clone singleton-cfg assertion.
- **Out of scope in-repo (C7):** the live latency + the decisive synthesis
  verdict — home-lab only.

---

## Complexity Tracking

| Deviation from simplest viable | Simpler alternative considered | Why rejected |
|---|---|---|
| New `modelswitch` package + `agenttool` allowlist singleton | Validate inside the Facade | The HTTP fast-path bypasses the Facade (CT-1); a Facade-only validator cannot serve both surfaces (breaks SCN-088-A06). The singleton is the ONLY shared seam. |
| New `TurnResult.Model` + `finalize` stamp | Have the caller set `result.Model` = resolved synthesis model | Dishonest on early-StopEndTurn (the tool model produced the answer, CT-3). Threading the actual answering model is the only truthful attribution (Principle 8). |
| New explicit `switchable_models` SST list | Derive the allowlist from `model_memory_profiles` | The profiles map includes vision/embed/OCR models irrelevant to `/ask`; an explicit curated offer is required, and the two-reason rejection still needs the profiles for the over-envelope case. |
| `WithModelOverride` shallow clone | A new `Run(ctx, prompt, override)` param | A clone leaves `Run` (and all its terminal paths) untouched and makes the "per-invocation Config copy" explicit and immutable-singleton-safe (C6). |
| `contracts.ModelAttribution` field on the response | Reuse `Invocation.Model` only | The renderer also needs the "override applied" bool to honour "footer only on override" (NFR-4 / Principle 6); a typed primitive matches the UX9 `ModelAttribution`. |
| Defer compare-both (Fork C) | Ship `/compare` now | Doubles worst-case latency past `WriteTimeout` (C3); ask-twice already enables A/B. Recorded F-COMPARE-LATENCY. |

Everything else uses the simplest viable approach (mirror the spec-087
SST split, the `agenttool` singleton pattern, and the fake-LLM test
style verbatim).

---

## Findings

- **F-STICKY** — per-user-sticky `/model <id>` is the deferred Fork-A
  follow-up (new `assistant_user_model_prefs` store keyed
  `(user_id, transport)`). Not shipped; per-request enables the A/B with
  zero new persistence. Additive later.
- **F-TOOLMODEL** — a separate `--tool-model=` / API `tool_model` knob is
  the deferred Fork-B follow-up (A/B the gather model). Not shipped; the
  motivating A/B is the synthesis turn, and the carrier/validator/clone
  are general enough to extend.
- **F-COMPARE-LATENCY** — first-class compare-both (Fork C) ~doubles the
  worst case to 8400s, busting the 4200s `WriteTimeout`. Deferred; needs a
  `WriteTimeout` re-derivation or a streaming/partial-result redesign.
  Ask-twice satisfies the A/B today.
- **F-ATTR-EARLY** — attribution names the model of the turn that
  produced the answer; on an early-StopEndTurn answer the footer is the
  baseline tool model even under a synthesis override (the synthesis seat
  was not reached). Honest by design; documented so it is not read as a
  bug. The comparison-class A/B questions reach the forced-final turn.
- **F-PROOF (carried, spec 087/088)** — the decisive
  `gemma4:26b`-vs-`deepseek-r1:7b` synthesis verdict is GPU/home-lab
  dependent and runs via a separate `bubbles.devops` dispatch (C7). This
  spec ships + validates the switchable primitive in-repo only; no live
  result is fabricated.
- **F-ENV-083 (carried)** — whole-working-tree guards may fail on the
  operator's uncommitted spec-083 card-rewards WIP; this spec touches
  none of the do-not-touch paths (C5).

---

## Open Questions

None blocking. The three forks are resolved; the deferred items are named
findings (F-STICKY / F-TOOLMODEL / F-COMPARE-LATENCY), each a non-blocking
next lever with a recorded design, not an unresolved decision. Plan may
proceed to scope decomposition.
```
