# Design — Spec 061 Conversational Assistant (Transport-Agnostic)

**Owner:** `bubbles.design`
**Status ceiling:** `specs_hardened` (no code changes; planning-only)
**Workflow mode:** `product-to-planning`
**Depends on:**
- spec 037 (LLM agent runtime — terminal `done`; this design CONSUMES
  it as substrate and MUST NOT re-implement any of its parts)
- spec 044 (per-user bearer auth, chat→user mapping)
- spec 054 (notification scheduler)
- spec 060 (PASETO scope claims)
- existing `ml/` Python sidecar
- existing `internal/telegram/bot.go`
- existing `/api/search` endpoint backed by pgvector
**Supersedes:** the prior design draft at this path (archived outside
the repo). The pre-revision draft proposed a parallel
`internal/assistant/{router,registry,executor,tracer}` stack that
re-implemented machinery already shipped by spec 037; spec.md §3.1
and Hard Constraint 6 made that approach a blocking failure. This
revision collapses the capability layer onto the spec 037 substrate.

---

## 0. Design Brief (REQUIRED alignment checkpoint)

**Current state.** Spec 037 (`specs/037-llm-agent-tools/`, terminal
`done`, full-delivery) already shipped a complete LLM-scenario agent
runtime: `agent.Router` (similarity + explicit-id + floor + fallback),
`agent.Executor` (loop, retry budget, allowlist, schema validation),
`agent.Bridge` (YAML scenario loader + hot-reload), `agent.NATSLLMDriver`,
`agent.PostgresTracer` (with `agent_traces` table + `smackerel agent
replay/traces/scenarios/tools` CLI), and `internal/telegram/agent_bridge.go::AgentBridge.Handle(ctx, chatID, text)`
which renders any `InvocationResult` via `internal/agent/userreply`
within a 4-line Telegram budget. The bridge is constructed in
`cmd/core/wiring_agent.go` but is wired ONLY to the REST handler
`POST /v1/agent/invoke`; `internal/telegram/bot.go::handleMessage`
(line 534) still routes every plain-text message straight to
`handleTextCapture`. 15 backend-only scenarios live under
`config/prompt_contracts/` (recommendation, digest assembly, drive
classification, etc.); none are user-facing. The `ml/` FastAPI sidecar
is online, NATS JetStream brokers the Go↔Python LLM channel, and
PostgreSQL + pgvector backs `/api/search`.

**Target state.** Introduce a **transport-agnostic assistant capability
layer** (`internal/assistant/`) that is a **thin facade over the spec
037 substrate** PLUS exactly five net-new packages enumerated in
spec.md §3.1.4 (contracts, provenance, context, confirm,
telegram_adapter). The capability layer:

- Reuses `agent.Router` for intent classification (no new router).
- Reuses `agent.Bridge` (loader + hot-reload) as the "skill registry"
  (no new registry).
- Reuses `agent.Executor` for skill execution (no new executor).
- Reuses `agent.PostgresTracer` for per-turn trace persistence (no
  new tracer; turns are agent traces tagged with `transport=...`).
- Reuses `agent.NATSLLMDriver` (no new LLM driver).
- Reuses `internal/telegram/agent_bridge.go::AgentBridge.Handle` as
  the inner core of the v1 Telegram adapter.

Three v1 "skills" ship as **YAML scenarios** under
`config/prompt_contracts/` (retrieval, weather, notifications) PLUS
their tool handlers added to the existing tool registry. They are
ordinary spec 037 scenarios distinguished only by user-facing
`intent_examples` and a user-facing tool surface.

Layer a `TransportAdapter` interface on top of that capability with
**one v1 reference implementation: Telegram** in
`internal/telegram/assistant_adapter/`. The Telegram adapter
intercepts the plain-text branch of `handleMessage` BEFORE
`handleTextCapture`, builds an `agent.IntentEnvelope`, delegates to
the existing `AgentBridge`, transforms the resulting
`agent.InvocationResult` + `agent.RoutingDecision` into an
`AssistantResponse` via the capability facade, and renders using
Telegram-native widgets. When the capability returns
`captureRoute=true`, the Telegram adapter delegates to the existing
`handleTextCapture` byte-for-byte.

**Patterns to follow.**
- **Substrate reuse (spec.md §3.1).** Every machinery layer of spec
  037 is consumed as-is. Net-new code is limited to thin facades and
  the five packages in spec.md §3.1.4. Any departure from this list
  is a Hard Constraint 6 violation and a P0 architectural rollback
  trigger.
- **Capability-foundation-first** per
  `.github/skills/bubbles-capability-foundation-design/SKILL.md`:
  proportionality is satisfied on the `TransportAdapter` axis
  (≥1 v1 plus ≥3 planned future transports — WhatsApp, web chat,
  mobile in-app). The agent-runtime axis is NOT a new foundation —
  spec 037 already owns it; this spec EXTENDS the existing
  foundation by adding three user-facing scenarios plus a
  user-facing facade layer.
- **NO-DEFAULTS / fail-loud SST** per
  `.github/instructions/smackerel-no-defaults.instructions.md`. Every
  net-new `assistant.*` and `assistant.transports.*` key uses
  `${VAR:?...}` substitution. Existing spec 037 `agent.*` keys are
  reused unchanged.
- **Capture-as-fallback is inviolable** (spec Hard Constraint 1,
  `policySnapshot.captureAsFallback="inviolable"`). The capability
  defaults to `CaptureRoute=true` on every uncertainty path
  (low-band router score, borderline-timeout, confirm-discarded,
  confirm-timeout, error-offered-capture).
- **Test environment isolation** per
  `.github/instructions/bubbles-test-environment-isolation.instructions.md`:
  integration/e2e/stress tests use ephemeral PostgreSQL via the
  existing test compose project; the spec 037 `NATSLLMDriver` and
  Python sidecar are real; only the Telegram `tgbotapi` boundary is
  in-process-faked (external dep).
- **Existing module conventions:** per-package `doc.go`, exported
  interfaces in `contracts.go`, table-driven tests in `*_test.go`.

**Patterns to avoid.**
- **FORBIDDEN — parallel agent runtime.** No `internal/assistant/router/`,
  no `internal/assistant/registry/`, no `internal/assistant/executor/`,
  no `internal/assistant/tracer/`, no second LLM driver, no second
  scenario loader, no second hot-reload mechanism. Reuse
  `internal/agent/*` types exclusively. Per spec.md §3.1.4, the
  presence of any such package is a P0 architectural rollback
  trigger and a Hard Constraint 6 violation. A build-time
  package-lint test (§11.3) fails the build if any of these
  package paths exist.
- **FORBIDDEN — wrapping `agent.Router` in a new `Router` interface
  inside `internal/assistant/`.** The capability layer USES
  `agent.Router` directly (or through `agent.Bridge` which already
  composes router+executor). Wrapping it would re-export the same
  machinery under a new name and is a covert duplication.
- **FORBIDDEN — `AssistantResponse` that mutates / re-encodes
  `agent.InvocationResult`.** The response is a thin facade that
  embeds (or references) the underlying `InvocationResult` and adds
  exactly six net-new fields. Anything more is duplication.
- **FORBIDDEN — direct Spec 037 mutation.** Spec 037 is in terminal
  `done` status; per `bubbles-artifact-ownership-routing`, this spec
  CANNOT edit Spec 037 files directly. The recommended
  borderline-band design (§3.2 below) is **additive in the
  capability layer**, not a `agent.Router` change. If any change to
  Spec 037 substrate becomes unavoidable, it MUST be routed via a
  Spec 037 bug folder (`specs/037-llm-agent-tools/bugs/BUG-NNN-.../`)
  and is OUT OF SCOPE for this spec until that bug ships.
- **FORBIDDEN — string-formatted source blocks from the capability.**
  The facade emits structured `Source` values; each adapter renders.
- **FORBIDDEN — typed `yes`/`no` interpreted as confirm callbacks**
  (UX §14.A.6). The capability accepts only
  `AssistantMessage{Kind: KindConfirm, ConfirmRef: ...}`.

**Resolved decisions.**
- **OQ #5 (substrate reconciliation, BLOCKING) — RESOLVED.**
  Eliminate parallel agent runtime. Capability layer = thin facade
  over spec 037 substrate + five net-new packages from spec.md
  §3.1.4. See §1, §3, §4, §10.
- **OQ #6 (borderline-band placement) — RESOLVED.** The
  capability-layer facade post-processes
  `agent.RoutingDecision.TopScore` against a NEW SST key
  `assistant.borderline_floor` (additive; zero Spec 037 mutation).
  When `agent.RoutingDecision.OK==true` AND
  `TopScore >= agent.confidence_floor` AND
  `TopScore < assistant.borderline_floor`, the facade SHORT-CIRCUITS
  before calling the executor and emits a
  `DisambiguationPrompt` whose `Choices` are derived from
  `agent.RoutingDecision.Considered[:3]`. See §3.2.
- **OQ #7 (confirm-card primitive ownership) — RESOLVED.** Net-new
  capability-layer state machine in `internal/assistant/confirm/`.
  Two-phase flow: (a) **propose phase** — the scenario's tool
  handler returns a structured proposal payload (not an executed
  side effect); the facade detects the propose marker in the
  scenario `output_schema` and surfaces a `ConfirmCard`. (b)
  **confirm phase** — on the user's confirm callback, the facade
  re-invokes the SAME scenario with a `confirm_ref` in the input
  envelope; the scenario's tool handler reads the pending payload
  from the state store and EXECUTES the side effect. State store
  is the PostgreSQL `assistant_conversations.pending_confirm`
  column (§6). See §5.3 (notifications skill) and §5.4 (confirm
  state machine).
- **Conversational state key:** `(user_id, transport)` (UX Open Q
  #1, ratified — §6).
- **Intent classifier substrate:** reuse spec 037 `agent.Router`
  which is similarity-based via the existing embeddings model
  (`nomic-embed-text` per spec 037 SST). No new classifier path.
  Analyst Open Q #1 thereby resolves to "use spec 037 substrate";
  the `assistant.capability.intent.model` SST key from the prior
  design draft is REMOVED (model selection lives in spec 037
  `agent.*` config).
- **Notifications skill:** reuse spec 054 scheduler directly; small
  additive extension to scheduler payload (`Source`, `Originator`)
  for audit. Analyst Open Q #2 resolved.
- **Per-skill PASETO scopes** (analyst Open Q #3, ratified): three
  new scopes added to spec 060 catalog —
  `assistant.skill.retrieval` (read),
  `assistant.skill.weather` (read),
  `assistant.skill.notifications.write` (write). See §9 + §14.
- **Source attribution UX** (analyst Open Q #4 / UX decision 1):
  capability emits structured `sources[]`; Telegram adapter renders
  trailing numbered block per UX §14.B.1.

**Open items deferred to bubbles.plan.**
- Whether `/reset` ships in Telegram v1 adapter SCOPE-05 or is
  deferred. Design recommends shipping in SCOPE-05.
- Eval harness corpus size, refresh cadence, intent-ground-truth set
  shape (SCOPE-10 sizing; bubbles.plan owns).
- Scope ordering (see §13).

**Blockers requiring owner clarification.** None. The Principle 1
deviation is already flagged in spec.md §11 and requires owner
ratification at user-validation time (not at design time).

**Pre-existing portfolio findings (out of scope, passed through).**
10 hardening gaps and SCOPE-01 blockers identified in prior runs are
NOT addressed here per packet directive and are forwarded as
`preExisting: true` in the result envelope.

---

## 1. Architecture Overview

### 1.1 Layered architecture (Mermaid)

```mermaid
flowchart TD
    subgraph TRANSPORTS["Transport Adapter Layer (per-frontend, translation-only)"]
      direction LR
      TG["Telegram Adapter (v1 reference)<br/>internal/telegram/assistant_adapter/<br/>+ existing AgentBridge"]
      WA["WhatsApp Adapter (future)"]:::future
      WEB["Web Chat Adapter (future)"]:::future
      MOB["Mobile In-App Adapter (future)"]:::future
    end

    subgraph CAPABILITY["Assistant Capability Layer (thin facade — net-new)"]
      FACADE["Assistant Facade<br/>internal/assistant/facade.go"]
      CONTRACTS["Canonical Contracts<br/>internal/assistant/contracts/"]
      PROV["Provenance Enforcer<br/>internal/assistant/provenance/"]
      CTX["Conversational State Store<br/>internal/assistant/context/<br/>key: (user_id, transport)"]
      CONF["Confirm-Card State Machine<br/>internal/assistant/confirm/"]
    end

    subgraph SUBSTRATE["Spec 037 Substrate (REUSED AS-IS — DO NOT FORK)"]
      direction LR
      ROUTER["agent.Router<br/>internal/agent/router.go"]
      BRIDGE["agent.Bridge (loader + hot-reload)<br/>internal/agent/bridge.go"]
      EXEC["agent.Executor<br/>internal/agent/executor.go"]
      DRV["agent.NATSLLMDriver<br/>internal/agent/nats_driver.go"]
      TRC["agent.PostgresTracer<br/>internal/agent/tracer.go"]
      AB["telegram.AgentBridge<br/>internal/telegram/agent_bridge.go"]
      UR["agent/userreply"]
    end

    subgraph SCENARIOS["3 v1 Scenario YAMLs (net-new content; existing loader)"]
      SC_R["retrieval-qa-v1.yaml<br/>config/prompt_contracts/"]
      SC_W["weather-query-v1.yaml<br/>config/prompt_contracts/"]
      SC_N["notification-schedule-v1.yaml<br/>config/prompt_contracts/"]
    end

    subgraph TOOLS["Net-new tool handlers (in existing tool registry)"]
      T_R["retrieval_search<br/>internal/agent/tools/retrieval/"]
      T_W["weather_lookup<br/>internal/agent/tools/weather/"]
      T_N1["notification_propose<br/>internal/agent/tools/notification/"]
      T_N2["notification_execute"]
    end

    subgraph EXTERNAL["External / Existing dependencies (unchanged)"]
      ML["ml/ sidecar (FastAPI over NATS)<br/>intent embed + LLM synthesis"]
      SEARCH["/api/search → pgvector"]
      SCHED["spec 054 notification scheduler"]
      WX["External weather provider<br/>(open-meteo or compatible)"]
      DB["PostgreSQL<br/>artifacts + agent_traces + assistant_conversations"]
      INF["Infisical secret store"]
      AUTH["spec 044 chat→user_id mapping<br/>spec 060 PASETO scopes"]
    end

    TG <-->|AssistantMessage / AssistantResponse| FACADE
    WA <-.->|future| FACADE
    WEB <-.->|future| FACADE
    MOB <-.->|future| FACADE

    FACADE -->|build IntentEnvelope| AB
    AB --> ROUTER
    AB --> EXEC
    ROUTER --> BRIDGE
    BRIDGE --> SC_R
    BRIDGE --> SC_W
    BRIDGE --> SC_N
    EXEC --> DRV
    EXEC --> T_R
    EXEC --> T_W
    EXEC --> T_N1
    EXEC --> T_N2
    DRV --> ML
    T_R --> SEARCH
    T_W -->|HTTPS| WX
    T_W --> INF
    T_N2 --> SCHED
    EXEC --> TRC
    TRC --> DB

    FACADE -->|borderline post-processor| ROUTER
    FACADE --> PROV
    FACADE --> CTX
    FACADE --> CONF
    CTX --> DB
    CONF --> DB
    AB --> UR

    TG --> AUTH
    TG -->|captureRoute=true| CAPRT["existing handleTextCapture<br/>internal/telegram/bot.go"]
    CAPRT --> DB

    classDef future fill:#eee,stroke:#999,stroke-dasharray: 4 4,color:#666
```

### 1.2 Invariants

1. **Adapters never call skills, scenarios, the agent runtime, or the
   facade's internals directly.** They call
   `Assistant.Handle(ctx, AssistantMessage) (AssistantResponse, error)`
   on the capability facade.
2. **The capability layer has zero transport-specific imports.**
   No `tgbotapi` import, no Telegram payload shapes, no per-transport
   flags. Enforced by a build-time package-lint (§11.3).
3. **The capability layer does NOT re-implement spec 037 substrate.**
   Enforced by a build-time package-existence test (§11.3) that
   fails if any of `internal/assistant/{router,registry,executor,tracer,loader}/`
   exist.
4. **`handleMessage` becomes a thin shim.** Inside `bot.go`, the
   plain-text branch is rewritten to delegate to the Telegram
   `assistant_adapter` BEFORE `handleTextCapture`. Adapter calls
   `assistant.Handle(...)` → renders response OR, on
   `CaptureRoute=true`, delegates to existing `handleTextCapture`
   byte-for-byte.
5. **Capture-as-fallback is inviolable on every transport**
   (spec Hard Constraint 1).
6. **Synthesis without sources is rejected at the capability layer.**
   The `provenance` gate runs after every scenario whose manifest
   marks `requires_provenance: true` (a new YAML key — additive in
   net-new scenarios only; existing 15 scenarios omit it and behave
   exactly as today). Empty `sources[]` on a non-empty synthesized
   `body` is dropped and replaced with the canonical refusal +
   capture (BS-007).
7. **Per-turn trace persistence reuses spec 037 `PostgresTracer`.**
   No new trace store. The capability layer adds a `transport=<name>`
   label to existing trace rows via the existing `Routing` metadata
   slot on `agent.IntentEnvelope` (no schema change to the
   `agent_traces` table is required; the executor already passes
   the routing context through).

### 1.3 Process boundary

All capability + adapter code compiles into the same `cmd/core`
binary. There is **no new process boundary**. Out-of-process
dependencies are unchanged: `ml/` sidecar (NATS), PostgreSQL (libpq),
Infisical (HTTP), external weather provider (HTTPS), Telegram Bot API
(HTTPS via existing `tgbotapi`).

---

## 2. Canonical Contracts (net-new — `internal/assistant/contracts/`)

All canonical Go types live in `internal/assistant/contracts/`.
Imported by the facade, every adapter, and the test suites. They are
the ONLY net-new top-level type surface this spec introduces; all
runtime mechanics use the spec 037 types directly.

### 2.1 `AssistantMessage` (inbound: adapter → capability)

```go
package contracts

import "time"

// AssistantMessage is the canonical inbound message handed to the
// capability layer by any transport adapter. It is trivially
// convertible to an agent.IntentEnvelope (see facade.go).
type AssistantMessage struct {
    UserID               string             // resolved by adapter from transport identity
    Transport            string             // closed vocab: "telegram", "whatsapp", "web", "mobile"
    TransportMessageID   string             // opaque, adapter-side idempotency
    Text                 string             // plain text, transport markup stripped
    Kind                 MessageKind        // text | confirm | disambiguation | reset
    ConfirmRef           string             // echo of prior ConfirmCard.ConfirmRef
    ConfirmChoice        ConfirmChoice      // positive | negative (when Kind=confirm)
    DisambiguationRef    string             // echo of prior DisambiguationPrompt.DisambiguationRef
    DisambiguationChoice int                // 1-indexed (when Kind=disambiguation)
    Attachments          []Attachment       // v1 unused
    ReceivedAt           time.Time          // adapter-side observe time
    TransportMetadata    map[string]string  // opaque to capability
}

type MessageKind string

const (
    KindText           MessageKind = "text"
    KindConfirm        MessageKind = "confirm"
    KindDisambiguation MessageKind = "disambiguation"
    KindReset          MessageKind = "reset"
)

type ConfirmChoice string

const (
    ConfirmPositive ConfirmChoice = "positive"
    ConfirmNegative ConfirmChoice = "negative"
)

type Attachment struct {
    Kind        string
    MimeType    string
    URL         string
    SizeBytes   int64
    Description string
}
```

### 2.2 `AssistantResponse` (outbound: capability → adapter)

`AssistantResponse` is a **thin facade over `agent.InvocationResult`
+ `agent.RoutingDecision`**. It carries a reference (NOT a copy) to
the underlying invocation result so trace IDs and tool-call details
are reachable without duplication, plus exactly **six net-new
fields** added by this spec (status, sources[], confirmCard,
disambiguationPrompt, errorCause, captureRoute) per spec.md §3.1.3.

```go
package contracts

import (
    "time"

    "github.com/smackerel/smackerel/internal/agent"
)

type AssistantResponse struct {
    // --- Substrate references (REUSED, NOT COPIED) ---
    // Invocation may be nil for short-circuit paths that never reached
    // the executor (e.g. low-band capture, borderline disambiguation,
    // confirm-card propose phase shortcut).
    Invocation *agent.InvocationResult // spec 037 substrate
    Routing    *agent.RoutingDecision  // spec 037 substrate; nil iff no router call

    // --- Six net-new fields added by spec 061 ---
    Status               StatusToken           // closed vocab §14.A.2 / spec.md UX
    Sources              []Source              // structured; bounded by sources_max
    ConfirmCard          *ConfirmCard          // §14.A.6
    DisambiguationPrompt *DisambiguationPrompt // §14.A.3
    ErrorCause           ErrorCause            // when Status=unavailable
    CaptureRoute         bool                  // adapter MUST invoke local capture path

    // --- Convenience derivatives (computed from above; not new state) ---
    Body                 string                // derived from Invocation.Final OR refusal text
    SourcesOverflowCount int
    EmittedAt            time.Time
}

type StatusToken string

const (
    StatusThinking          StatusToken = "thinking"
    StatusCheckingWeather   StatusToken = "checking_weather"
    StatusCheckingEmail     StatusToken = "checking_email" // v2
    StatusReminderProposed  StatusToken = "reminder_proposed"
    StatusReminderConfirmed StatusToken = "reminder_confirmed"
    StatusReminderCancelled StatusToken = "reminder_cancelled"
    StatusSavedAsIdea       StatusToken = "saved_as_idea"
    StatusUnavailable       StatusToken = "unavailable"
)

type ErrorCause string

const (
    ErrProviderUnavailable ErrorCause = "provider_unavailable"
    ErrMissingScope        ErrorCause = "missing_scope"
    ErrSlotMissing         ErrorCause = "slot_missing"
    ErrInternalError       ErrorCause = "internal_error"
)

type SourceKind string

const (
    SourceArtifact         SourceKind = "artifact"
    SourceExternalProvider SourceKind = "external_provider"
)

type Source struct {
    ID    string
    Title string
    Kind  SourceKind
    Ref   SourceRef
}

type SourceRef interface{ isSourceRef() }

type ArtifactRef struct {
    ArtifactID string
    CapturedAt time.Time
}

func (ArtifactRef) isSourceRef() {}

type ExternalProviderRef struct {
    ProviderName string
    RetrievedAt  time.Time
}

func (ExternalProviderRef) isSourceRef() {}

type ConfirmCard struct {
    ProposedAction string
    Payload        []byte        // opaque; capability persists; adapter echoes
    Timeout        time.Duration
    ConfirmRef     string
    PositiveLabel  string
    NegativeLabel  string
}

type DisambiguationPrompt struct {
    Choices           []DisambiguationChoice // length 1..3; "save_as_note" always last
    Timeout           time.Duration
    DisambiguationRef string
}

type DisambiguationChoice struct {
    Number   int
    ID       string // matches a scenario id, or "save_as_note"
    Label    string
    Shortcut string
}
```

**Net-new field count check:** Status, Sources, ConfirmCard,
DisambiguationPrompt, ErrorCause, CaptureRoute = exactly 6 per
spec.md §3.1.3. Body / SourcesOverflowCount / EmittedAt are
derivatives (not net-new state) but are exposed as convenience
fields so adapters do not have to re-derive them.

### 2.3 `TransportAdapter` interface

```go
package contracts

import "context"

type TransportAdapter interface {
    Name() string                                                                   // closed vocab
    Translate(ctx context.Context, payload TransportPayload) (AssistantMessage, error)
    Render(ctx context.Context, identity TransportIdentity, resp AssistantResponse) error
    Identity(ctx context.Context, payload TransportPayload) (TransportIdentity, error)
    Start(ctx context.Context, a Assistant) error
    Stop(ctx context.Context) error
}

type TransportPayload interface{} // opaque (e.g. *tgbotapi.Update for Telegram)

type TransportIdentity struct {
    UserID    string
    Transport string
}
```

**Adapter MUST.** See spec.md §6.2 — unchanged. (Translate, call
`Assistant.Handle`, render, honor `CaptureRoute=true`, translate
confirm/disambig callbacks, emit per-transport telemetry, resolve
identity.)

**Adapter MUST NOT.** See spec.md §6.2 — unchanged.

### 2.4 `Assistant` facade interface

```go
package contracts

import "context"

type Assistant interface {
    Handle(ctx context.Context, msg AssistantMessage) (AssistantResponse, error)
}
```

---

## 3. Intent Routing via Spec 037 Substrate

There is **no `internal/assistant/router` package**. Intent routing IS
`agent.Router` (spec 037 substrate). The capability-layer facade
adds a **borderline-band post-processor** on top of
`agent.RoutingDecision` (resolves OQ #6).

### 3.1 Routing flow inside the facade

```text
AssistantMessage (KindText)
    │
    ▼
facade.Handle:
  1. Resolve "/reset" / reference phrases (capability concerns).
  2. Build agent.IntentEnvelope{
        Source:   msg.Transport,        // "telegram" | ...
        RawInput: msg.Text,
     }
  3. Call agent.Router.Route(env) → RoutingDecision
  4. Apply BORDERLINE-BAND POST-PROCESSOR (§3.2):
        - If decision.OK == false:
            → CaptureRoute=true, Status=saved_as_idea  (BS-001)
        - If decision.OK == true AND
             decision.TopScore < assistant.borderline_floor:
            → emit DisambiguationPrompt; do NOT call executor
        - Else (above borderline_floor):
            → proceed to executor (via existing AgentBridge.Handle
              composition or a direct executor call — see §3.3)
  5. After executor: apply provenance gate, source assembly,
     confirm-card detection, error mapping. Build AssistantResponse.
```

### 3.2 Borderline-Band Post-Processor (resolves OQ #6)

`agent.Router` is binary: a score is above or below `confidence_floor`
(SST `agent.routing.confidence_floor`). Spec 061 needs a three-band
decision (high / borderline / low) per UX §14.A.3. The recommended
resolution is **option (a) from spec.md §13 OQ #6**: a
capability-layer post-processor on `RoutingDecision.TopScore` against
a NEW SST key. This is purely additive — zero Spec 037 mutation, zero
new scenarios, zero new `Outcome` values.

| Band | Bound (uses spec 037 + new key) | Capability action |
|------|---------------------------------|-------------------|
| **High** | `decision.OK == true` AND `TopScore >= assistant.borderline_floor` | Proceed to executor; invoke top scenario. |
| **Borderline** | `decision.OK == true` AND `agent.routing.confidence_floor <= TopScore < assistant.borderline_floor` | Emit `DisambiguationPrompt` (≤3 choices from `decision.Considered`; `save_as_note` always last). DO NOT call executor. |
| **Low** | `decision.OK == false` (router fell through floor with no fallback OR ReasonUnknownIntent) | `CaptureRoute=true`, `Status=saved_as_idea`. |

**New SST key (only addition to the routing surface):**

- `assistant.borderline_floor` — float [0, 1]; MUST be **strictly
  greater than** `agent.routing.confidence_floor`. Validated at
  startup; abort on violation.

> **Note.** This places the borderline band ABOVE the existing
> confidence floor, NOT below it. Below the existing floor, the
> router already produces `OK=false` (capture). Between the existing
> floor and `assistant.borderline_floor`, the capability asks once.

**`DisambiguationPrompt.Choices` construction:**

```go
choices := make([]DisambiguationChoice, 0, 3)
for i, c := range decision.Considered {
    if i >= 2 { break } // leave room for save_as_note
    skill := lookupSkillManifest(c.ScenarioID)
    choices = append(choices, DisambiguationChoice{
        Number:   i + 1,
        ID:       c.ScenarioID,
        Label:    skill.UserFacingLabel,
        Shortcut: skill.SlashShortcut,
    })
}
choices = append(choices, DisambiguationChoice{
    Number:   len(choices) + 1,
    ID:       "save_as_note",
    Label:    "save as note",
    Shortcut: "/save",
})
```

On the user's disambiguation reply (`Kind=KindDisambiguation`), the
facade looks up the chosen scenario ID and re-routes via
`agent.IntentEnvelope{Source, RawInput, ScenarioID: chosen}` —
which causes `agent.Router` to take the **explicit-id fast path**
(`ReasonExplicitScenarioID`, no embedding call). On timeout or
`save_as_note`, the facade emits `CaptureRoute=true`.

### 3.3 Calling the executor

The facade has two equivalent options for invoking the executor:

**Option A — reuse `telegram.AgentBridge.AgentRunner`-shaped seam
(RECOMMENDED).** Extract the production wiring of
`agent.Router + agent.Executor` (built in `cmd/core/wiring_agent.go`)
behind a small `Runner` interface that the facade depends on. This
is the same interface `telegram.AgentBridge` already consumes
(`AgentRunner.Invoke(ctx, env) → (*InvocationResult, *RoutingDecision)`)
— zero new abstraction; both bridges share one runner.

```go
// Already exists in internal/telegram/agent_bridge.go as AgentRunner;
// move the type to internal/agent/runner.go (no behavior change) so
// the facade can import without depending on telegram.
type Runner interface {
    Invoke(ctx context.Context, env IntentEnvelope) (*InvocationResult, *RoutingDecision)
    KnownIntents() []string
}
```

**Option B — facade composes `Router` + `Executor` directly.** Also
acceptable; trade-off is the facade duplicates the small composition
glue in `wireAgentBridge`. Recommend A.

If Option A's `Runner` type move requires touching spec 037 files
(`internal/agent/`), it MUST go through a Spec 037 bug folder (§0
"FORBIDDEN — direct Spec 037 mutation"). Alternative: define the
interface in `internal/assistant/contracts/` and have the facade
adapt the existing `agent.Bridge`-composed runner at construction
time. Either way, no spec 037 logic moves.

### 3.4 Fast-path command shortcuts

Slash commands (`/ask`, `/weather`, `/save`, `/reset`) map to
explicit scenario IDs (or to capability-level actions for `/reset`).
The facade pre-checks a small `RouterShortcut` text-prefix map and,
on a hit, builds the envelope with `ScenarioID=<id>` so
`agent.Router` takes the explicit-id fast path. This is a
capability-layer concern (uniform across transports) — adapters pass
`Text` verbatim.

---

## 4. "Skill Registry" = Spec 037 Scenario Loader

There is **no `internal/assistant/registry` package**. The "skill
registry" IS `agent.Bridge` (the scenario loader + SIGHUP-triggered
hot-reload — `internal/agent/bridge.go` + `loader.go`). Three v1
"skills" ship as YAML scenarios under `config/prompt_contracts/`
exactly like the existing 15 backend-only scenarios; their tool
handlers are registered in the existing tool registry from each
tool's owning package `init()` (per the spec 037 loader rule —
see `loader.go:311`).

### 4.1 Scenario YAML conventions for user-facing scenarios

Net-new conventions added in this spec (all are additive YAML keys
the loader already tolerates per spec 037's `top` map — or, if a key
needs new loader recognition, a tiny additive change to `loader.go`
which MUST be routed via a Spec 037 bug folder):

| Key | Purpose | Required for v1 user-facing scenarios? |
|-----|---------|-----------------------------------------|
| `user_facing: true` | Distinguishes user-facing scenarios from backend-only. Capability facade considers only scenarios with this flag for router intent matching against user input. | Yes |
| `requires_provenance: true` | Capability provenance gate (§4.3) drops empty-`sources[]` synthesis. | Yes for retrieval + weather; No for notifications (scheduler record IS provenance) |
| `user_facing_label: "weather"` | Human label used in `DisambiguationPrompt.Choices`. | Yes |
| `slash_shortcut: "/weather"` | Slash-command fast path (§3.4). | Yes |
| `confirm_required: true` | Marks a side-effect scenario needing the confirm-card state machine (§5.4). | Yes for notifications |

These keys are additive metadata. If the loader rejects unknown
top-level keys (per `loader.go` `reject` paths), the loader needs a
small allowlist addition — that single-line addition is the ONE
unavoidable Spec 037 substrate change and MUST be routed via a
Spec 037 bug folder. **Alternative (preferred if loader change is
deemed risky):** stash the new keys under an existing tolerated
key such as `description` or carry them in a sibling
`config/assistant/scenarios.yaml` lookup file that the capability
facade reads at startup, keyed by scenario `id`. The lookup file
keeps spec 037 untouched. See §13 plan-time decision.

### 4.2 Enabled/disabled gating (BS-008 preserved)

For each v1 user-facing scenario, an SST key controls enablement:

| Scenario | SST enable key |
|----------|----------------|
| `retrieval-qa-v1` | `assistant.skills.retrieval.enabled` |
| `weather-query-v1` | `assistant.skills.weather.enabled` |
| `notification-schedule-v1` | `assistant.skills.notifications.enabled` |

The capability layer filters disabled scenarios out of the router's
candidate set at facade construction time (or, equivalently, after
`agent.Router.Route` returns, if the executor would otherwise run
them). The disabled scenarios stay registered in `agent.Bridge`
(spec 037 cares about the registry contents for tracer integrity and
for the REST `POST /v1/agent/invoke` surface) — the capability layer
just refuses to dispatch them and emits `ErrMissingScope` +
capture per BS-008.

### 4.3 Provenance Enforcement Gate (net-new — `internal/assistant/provenance/`)

```go
package provenance

import "github.com/smackerel/smackerel/internal/assistant/contracts"

// Enforce wraps a completed agent.InvocationResult plus the candidate
// sources extracted from the executor turn messages. Returns a
// possibly-rewritten AssistantResponse where empty-sources synthesis
// has been replaced with the canonical refusal + capture.
func Enforce(scenarioRequiresProvenance bool, resp contracts.AssistantResponse) contracts.AssistantResponse {
    if !scenarioRequiresProvenance { return resp }
    if len(resp.Body) > 0 && len(resp.Sources) == 0 {
        return contracts.AssistantResponse{
            Status:       contracts.StatusSavedAsIdea,
            Body:         "I don't have a sourced answer for that.",
            CaptureRoute: true,
        }
    }
    return resp
}
```

Guarantees BS-007 mechanically. Counter
`smackerel_assistant_provenance_violations_total{scenario}` (§8)
increments on every trigger so drift is observable.

---

## 5. v1 Skill Designs (as Spec 037 Scenarios)

Each "skill" is a YAML scenario consumed by the spec 037 loader +
executor, plus its tool handlers registered in the existing tool
registry. Spec 061 contributes ZERO new runtime mechanics — only
content (YAML + tool Go code).

### 5.1 Retrieval Q&A — `retrieval-qa-v1.yaml`

```yaml
id: retrieval_qa
version: "retrieval-qa-v1"
type: "scenario"
description: "Answer a user question over the user's knowledge graph with artifact-ID citations."

user_facing: true
requires_provenance: true
user_facing_label: "search my notes"
slash_shortcut: "/ask"

intent_examples:
- "what did I save about Tailscale last month?"
- "did I capture anything on sourdough?"
- "find my notes on ACL tags"
- "do I have anything on CGNAT?"
- "remind me what I wrote about the home lab"

system_prompt: |
  You are Smackerel's retrieval assistant. Answer ONLY from artifacts
  returned by the retrieval_search tool. Cite every artifact you used
  by its artifact_id. Never invent citations. If no artifacts match,
  return an empty answer and let the capability layer handle the
  refusal.

allowed_tools:
- name: retrieval_search
  side_effect_class: read

input_schema:
  type: object
  required: [ query, user_id ]
  properties:
    query:    { type: string, minLength: 1 }
    user_id:  { type: string, minLength: 1 }

output_schema:
  type: object
  required: [ answer, cited_artifact_ids ]
  properties:
    answer:
      type: string
    cited_artifact_ids:
      type: array
      items: { type: string }

limits:
  max_loop_iterations: 4
  timeout_ms: 5000
  schema_retry_budget: 1
  per_tool_timeout_ms: 2500

token_budget: 1200
temperature: 0.2
model_preference: "default"
side_effect_class: read
```

**Tool handler — `retrieval_search`** (package
`internal/agent/tools/retrieval/`): wraps existing `/api/search`
backed by pgvector. Input `{query, user_id, top_k}`; output
`{hits: [{artifact_id, title, snippet, captured_at}]}`. `top_k` is
capped at `assistant.skills.retrieval.top_k` (SST). Capability
facade assembles `[]contracts.Source` from `cited_artifact_ids`
(NOT from the raw hits — only what the LLM actually cited).
Drops `cited_artifact_ids` for missing artifacts (graph drift) and
increments
`smackerel_assistant_source_assembly_drops_total{cause="missing_artifact"}`.
If ALL citations are missing, `Sources` is empty and the
provenance gate fires (refusal + capture).

### 5.2 Weather — `weather-query-v1.yaml`

```yaml
id: weather_query
version: "weather-query-v1"
type: "scenario"
description: "Answer a current/forecast weather question from an external provider with provider+timestamp attribution."

user_facing: true
requires_provenance: true
user_facing_label: "check weather"
slash_shortcut: "/weather"

intent_examples:
- "weather in Seattle today"
- "is it going to rain in Reykjavík tomorrow?"
- "what's the forecast for Portland this weekend?"
- "temperature in London right now"

system_prompt: |
  You are Smackerel's weather assistant. Call the weather_lookup tool
  with the parsed location and time window. Render a single, terse
  forecast line. Always emit the provider name and retrieved_at in
  your output so the capability layer can attach external_provider
  attribution.

allowed_tools:
- name: weather_lookup
  side_effect_class: external

input_schema:
  type: object
  required: [ raw_query, user_id ]
  properties:
    raw_query: { type: string, minLength: 1 }
    user_id:   { type: string, minLength: 1 }

output_schema:
  type: object
  required: [ forecast_line, provider_name, retrieved_at ]
  properties:
    forecast_line: { type: string }
    provider_name: { type: string }
    retrieved_at:  { type: string, format: "date-time" }
    slot_missing:
      type: string
      enum: [ "location" ]

limits:
  max_loop_iterations: 3
  timeout_ms: 3000
  schema_retry_budget: 1
  per_tool_timeout_ms: 2000

token_budget: 600
temperature: 0.1
model_preference: "default"
side_effect_class: external
```

**Tool handler — `weather_lookup`** (package
`internal/agent/tools/weather/`): wraps an external provider via
HTTPS. v1 ships one concrete provider (open-meteo or compatible —
owner picks at SCOPE-07 time). Selection via SST
`assistant.skills.weather.provider`; API key via
`assistant.skills.weather.api_key_ref` → Infisical secret name.
In-process LRU keyed `(provider, location, forecast_window)` with
TTL = `assistant.skills.weather.cache_ttl` (SST, fail-loud). Cache
hits emit the ORIGINAL `retrieved_at`, not cache-hit time.

Failure mapping (tool returns error → executor records
`OutcomeToolError` → capability facade translates):
- HTTP 5xx / timeout / DNS → `ErrorCause=ErrProviderUnavailable`,
  `Status=StatusUnavailable`, body `"weather: unavailable"`.
- `slot_missing="location"` in output → `ErrorCause=ErrSlotMissing`
  + one-choice disambiguation prompt asking for the city.

### 5.3 Notifications — `notification-schedule-v1.yaml`

```yaml
id: notification_schedule
version: "notification-schedule-v1"
type: "scenario"
description: "Propose a scheduled reminder; on user confirm, register it with the spec 054 scheduler."

user_facing: true
requires_provenance: false        # scheduler record IS the provenance
confirm_required: true            # triggers capability confirm-card state machine
user_facing_label: "remind me"
slash_shortcut: "/remind"

intent_examples:
- "remind me to take out the trash at 7pm"
- "remind me to call mom tomorrow at 9am"
- "set a reminder for the meeting at 14:30"
- "ping me about the laundry in 2 hours"

system_prompt: |
  You are Smackerel's reminder assistant. PROPOSE the reminder by calling
  notification_propose with the parsed {what, when}. Do NOT call
  notification_execute on the first turn; the capability layer will
  re-invoke this scenario with a confirm_ref after the user confirms.
  When confirm_ref is present in the input, call notification_execute
  with the same confirm_ref to commit.

allowed_tools:
- name: notification_propose
  side_effect_class: read
- name: notification_execute
  side_effect_class: write

input_schema:
  type: object
  required: [ user_id, raw_query ]
  properties:
    user_id:     { type: string, minLength: 1 }
    raw_query:   { type: string, minLength: 1 }
    transport:   { type: string, minLength: 1 }
    confirm_ref: { type: string }     # present iff this is the confirm-phase re-invocation

output_schema:
  type: object
  required: [ phase ]
  properties:
    phase:
      type: string
      enum: [ "proposed", "confirmed", "slot_missing" ]
    proposed_action: { type: string }
    payload:         { type: string }   # opaque base64 — capability stores in pending_confirm
    confirm_ref:     { type: string }
    scheduled_job_id:{ type: string }
    slot_missing_options:
      type: array
      items: { type: string }

limits:
  max_loop_iterations: 4
  timeout_ms: 3000
  schema_retry_budget: 1
  per_tool_timeout_ms: 2000

token_budget: 800
temperature: 0.1
model_preference: "default"
side_effect_class: write
```

**Tool handlers — `notification_propose` and `notification_execute`**
(package `internal/agent/tools/notification/`):

- `notification_propose` extracts `{what, when}` slots. On
  ambiguity, returns `phase="slot_missing"` + up-to-3 candidate
  times. Otherwise returns
  `phase="proposed", proposed_action, payload (opaque-encoded
  {what, when_utc, user_id, transport}), confirm_ref (ULID)`. NO
  side effect.
- `notification_execute` reads the pending payload from the
  capability state store via the supplied `confirm_ref`, calls
  spec 054's `scheduler.Schedule(...)` with `Source` and
  `Originator` set, and returns `phase="confirmed",
  scheduled_job_id`.

**Spec 054 extension.** Additive, backward-compatible — two optional
`Job` fields: `Source string` (e.g. `"assistant.skill.notifications"`)
and `Originator struct{Transport, ConfirmRef string}`. Zero-valued
fields preserve current behavior. Spec 054 file edits routed via a
packet per `bubbles-artifact-ownership-routing` (spec 054 is the
owner). SCOPE-08 owns the coordination.

### 5.4 Confirm-Card State Machine (net-new — `internal/assistant/confirm/`) — resolves OQ #7

Spec 037's executor invokes tools directly with no "propose then
execute" primitive. The notifications skill (BS-004) requires
**explicit user confirmation between proposal and execution**.

**State machine (capability-layer, scenario-agnostic):**

```text
PHASE 1 — propose
─────────────────
facade.Handle(KindText) → router → executor invokes scenario with
  no confirm_ref. Scenario's first tool (e.g. notification_propose)
  returns:
    {phase: "proposed", proposed_action, payload (opaque), confirm_ref}
facade detects (a) scenario manifest has confirm_required: true AND
  (b) output.phase == "proposed", then:
    1. Persist {payload, scenario_id, confirm_ref, expires_at} into
       assistant_conversations.pending_confirm (§6.1).
    2. Build ConfirmCard{ProposedAction, Timeout, ConfirmRef, ...}.
    3. Build AssistantResponse{
         Status: StatusReminderProposed,
         Body:   proposed_action,
         ConfirmCard: ...,
       }
    4. Write assistant_turn audit row (§6.3) with outcome="proposed".

PHASE 2 — confirm callback
──────────────────────────
adapter delivers AssistantMessage{Kind: KindConfirm, ConfirmRef,
  ConfirmChoice} → facade:
  1. Lookup pending_confirm by (user_id, transport, confirm_ref).
     Not-found OR expired → emit StatusUnavailable + ErrInternalError
     OR CaptureRoute=true; clear pending row.
  2. If ConfirmChoice == ConfirmNegative → emit
     Status=StatusReminderCancelled then a follow-up
     AssistantResponse{Status=StatusSavedAsIdea, CaptureRoute=true}
     for the ORIGINAL message text (carried in pending payload).
     Clear pending row. Write assistant_proposal audit
     outcome="discarded_user".
  3. If ConfirmChoice == ConfirmPositive → build
     agent.IntentEnvelope{
       ScenarioID: pending.scenario_id,    // explicit-id fast path
       RawInput:   pending.original_text,
       StructuredContext: jsonEncode({confirm_ref: pending.confirm_ref}),
     }
     and re-invoke the runner. The scenario re-runs with confirm_ref
     in input → calls notification_execute → returns
     phase="confirmed". Facade emits
     Status=StatusReminderConfirmed. Clear pending row. Write
     assistant_proposal audit outcome="confirmed",
     scheduled_job_id=<from output>.

PHASE 3 — timeout (no callback within ConfirmCard.Timeout)
─────────────────────────────────────────────────────────
idle-sweep ticker (§6.2) deletes the pending_confirm row. NO
  follow-up message is pushed to the user proactively (Principle 6 —
  bot never initiates). On the user's NEXT message that references
  the stale confirm_ref (e.g. tapping a stale Telegram button), the
  facade returns a brief "that proposal expired" body + captures
  the new message. Audit row written with outcome="discarded_timeout"
  at sweep time.
```

**Idempotency.** `confirm_ref` is a ULID. Repeated positive callbacks
with the same ref are no-ops after the first execute (single-flight
guarded by row deletion in step 3). The notification scheduler also
deduplicates via spec 054's own idempotency.

**Audit invariant.** Per UX §14.A.6, the facade writes an
`assistant_proposal` artifact on EVERY proposal regardless of
outcome (`confirmed | discarded_user | discarded_timeout`). Schema
extension is additive on the existing `artifacts` table (one
nullable JSONB column).

---

## 6. Conversational State

Package: `internal/assistant/context/` (net-new — per spec.md §3.1.4).

### 6.1 Schema

**Storage:** PostgreSQL table `assistant_conversations`. Survives
capability-layer restart for the rolling window; idle sweep is a
single SQL query. (UX §14.A.5 was provisional in-memory; design
ratifies PostgreSQL.)

```sql
CREATE TABLE assistant_conversations (
    user_id          TEXT        NOT NULL,
    transport        TEXT        NOT NULL,
    last_activity_at TIMESTAMPTZ NOT NULL,
    working_context  JSONB       NOT NULL,   -- last N turns
    pending_confirm  JSONB,                  -- nullable; in-flight ConfirmCard payload
    pending_disambig JSONB,                  -- nullable; in-flight DisambiguationPrompt
    schema_version   INT         NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, transport)
);

CREATE INDEX idx_assistant_conversations_idle
    ON assistant_conversations (last_activity_at);
```

`pending_confirm` JSONB shape:
```json
{
  "confirm_ref": "01H...",
  "scenario_id": "notification_schedule",
  "original_text": "remind me to call mom at 6pm",
  "payload_b64": "...",
  "expires_at": "2026-05-28T18:01:00Z"
}
```

`working_context` JSONB shape: list of `ContextTurn`:
```json
[{"user_text":"...","scenario_id":"retrieval_qa","summary":"...","source_ids":["a1b2..."],"ts":"..."}]
```

### 6.2 Idle timeout sweep

In-process ticker (NOT scheduler-based — scheduler is for user-visible
jobs; idle sweep is internal maintenance). Ticker runs every
`assistant.context.idle_sweep_interval` (SST, fail-loud; recommend
`60s`):

```sql
DELETE FROM assistant_conversations
WHERE last_activity_at < NOW() - INTERVAL '<idle_timeout>';
```

`idle_timeout` = `assistant.context.idle_timeout` (SST, fail-loud;
recommend `10m`). Row deletion drops any pending confirm/disambig
and writes a final `assistant_proposal` audit row with
`outcome="discarded_timeout"` for each cleared confirm.

### 6.3 Audit boundary

`working_context` is ephemeral. The permanent record reuses TWO
write paths:

1. **Per-turn trace** — every executor invocation already writes a
   row to `agent_traces` via spec 037 `PostgresTracer`. Spec 061
   adds NO new trace table; instead the capability facade ensures
   `transport` is recorded in the trace's existing routing/context
   payload (via `agent.IntentEnvelope.StructuredContext` JSONB or
   the existing `Routing` field — both already pass through to the
   tracer). No `agent_traces` schema change.
2. **`assistant_turn` artifact** — one row per FACADE turn (even
   for short-circuit paths like low-band capture that never reach
   the executor). Written via the existing artifacts table; payload
   shape:
   ```json
   {
     "user_id": "...",
     "transport": "telegram",
     "user_text": "...",
     "router_decision": {"scenario_id":"retrieval_qa","top_score":0.91,"band":"high","reason":"similarity_match"},
     "agent_trace_id": "...",
     "response_status": "thinking",
     "outcome": "answered | captured | proposed | confirmed | discarded | error",
     "error_cause": null,
     "timestamp": "2026-05-28T14:03:00Z"
   }
   ```
3. **`assistant_proposal` artifact** — one row per confirm proposal
   regardless of outcome (§5.4). Stored in the existing `artifacts`
   table with a new nullable JSONB column (additive migration in
   SCOPE-08).

Honors Principle 3 + Principle 8.

### 6.4 Reference resolution

On `KindText` whose text contains a reference phrase ("that one",
"the second one", "open 2"), the facade pre-processes against the
most recent `ContextTurn.SourceIDs` (1-indexed for numeric;
most-relevant-first for "that"). Resolution is a capability-layer
concern; scenarios receive the resolved artifact id in their input
slot. Unresolvable refs short-circuit BEFORE the router and emit
`Status=StatusUnavailable, ErrorCause=ErrSlotMissing,
Body="cannot resolve reference. last result has <N> sources."`
(UX §14.A.5).

---

## 7. SST Configuration Surface (NO-DEFAULTS / fail-loud)

All net-new keys NO-DEFAULTS, fail-loud per
`.github/instructions/smackerel-no-defaults.instructions.md`. Every
substitution uses `${VAR:?...}`. Secrets reference Infisical secret
names (spec 150). **Existing spec 037 `agent.*` keys are reused
unchanged** — there is NO `assistant.capability.intent.*` block;
classifier/model selection lives entirely in `agent.*`.

### 7.1 Full YAML schema fragment

```yaml
assistant:
  enabled:             ${ASSISTANT_ENABLED:?ASSISTANT_ENABLED required}

  # Three-band confidence rule (§3.2). agent.routing.confidence_floor
  # is the spec 037 SST key; this borderline_floor is the only new
  # routing knob spec 061 introduces.
  borderline_floor:    ${ASSISTANT_BORDERLINE_FLOOR:?required, float 0..1, MUST be > agent.routing.confidence_floor}

  context:
    window_turns:        ${ASSISTANT_CONTEXT_WINDOW_TURNS:?required, int 4..6}
    idle_timeout:        ${ASSISTANT_CONTEXT_IDLE_TIMEOUT:?required, duration recommend 10m}
    idle_sweep_interval: ${ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL:?required, duration recommend 60s}
    state_key:           ${ASSISTANT_CONTEXT_STATE_KEY:?required, enum "user_transport"|"user"}

  sources_max:           ${ASSISTANT_SOURCES_MAX:?required, int recommend 5}
  body_max_chars:        ${ASSISTANT_BODY_MAX_CHARS:?required, int}
  status_max_duration:   ${ASSISTANT_STATUS_MAX_DURATION:?required, duration recommend 10s}

  disambiguate_timeout:  ${ASSISTANT_DISAMBIGUATE_TIMEOUT:?required, duration recommend 30s}

  error:
    capture_timeout:     ${ASSISTANT_ERROR_CAPTURE_TIMEOUT:?required, duration recommend 30s}

  rate_limit:
    retrieval:     {requests_per_minute: ${ASSISTANT_RL_RETRIEVAL_RPM:?required, int}}
    weather:       {requests_per_minute: ${ASSISTANT_RL_WEATHER_RPM:?required, int}}
    notifications: {requests_per_minute: ${ASSISTANT_RL_NOTIFICATIONS_RPM:?required, int}}

  skills:
    retrieval:
      enabled:           ${ASSISTANT_SKILL_RETRIEVAL_ENABLED:?required, bool}
      top_k:             ${ASSISTANT_SKILL_RETRIEVAL_TOP_K:?required, int}
    weather:
      enabled:           ${ASSISTANT_SKILL_WEATHER_ENABLED:?required, bool}
      provider:          ${ASSISTANT_SKILL_WEATHER_PROVIDER:?required, e.g. "open-meteo"}
      api_key_ref:       ${ASSISTANT_SKILL_WEATHER_API_KEY_REF:?required, Infisical secret name}
      cache_ttl:         ${ASSISTANT_SKILL_WEATHER_CACHE_TTL:?required, duration recommend 300s}
    notifications:
      enabled:           ${ASSISTANT_SKILL_NOTIFICATIONS_ENABLED:?required, bool}
      confirm_timeout:   ${ASSISTANT_SKILL_NOTIFICATIONS_CONFIRM_TIMEOUT:?required, duration recommend 60s}

  transports:
    telegram:
      enabled:              ${ASSISTANT_TRANSPORT_TELEGRAM_ENABLED:?required, bool}
      markdown_mode:        ${ASSISTANT_TRANSPORT_TELEGRAM_MARKDOWN_MODE:?required, enum "plain"|"markdown_v2"}
      max_message_chars:    ${ASSISTANT_TRANSPORT_TELEGRAM_MAX_MESSAGE_CHARS:?required, int recommend 3500}
    # whatsapp: {...}   # future adapter
    # web:      {...}   # future adapter
    # mobile:   {...}   # future adapter
```

### 7.2 Validation rules (startup; abort on violation)

1. Every key resolves to a non-empty value (NO-DEFAULTS / fail-loud).
2. `assistant.borderline_floor > agent.routing.confidence_floor`.
   Abort if equal or less (would erase the borderline band).
3. `assistant.enabled=true` requires at least one
   `assistant.transports.*.enabled=true`.
4. Every `skills.*.enabled=true` has its dependencies present:
   - weather `api_key_ref` resolves in Infisical;
   - notifications spec 054 scheduler reachable on startup ping;
   - retrieval `/api/search` responds 200 on startup ping.
5. `state_key="user"` (non-recommended) emits a startup WARN.
6. The three v1 scenario YAMLs are present in `AGENT_SCENARIO_DIR`
   and pass spec 037 loader validation (per `loader.go`). Abort if
   missing or invalid (so the user-facing capability cannot silently
   ship without its content).

### 7.3 Fail-loud error envelopes

On startup, any failed validation emits a single-line error to
stderr in the form:

```
FATAL assistant config invalid: <key>: <reason>
```

and aborts before any HTTP listener binds, any Telegram polling loop
starts, or any NATS subscription is created. No partial-startup
state is reachable.

### 7.4 Secrets

| Secret | SST key (ref) | Infisical secret name |
|--------|---------------|------------------------|
| Weather provider API key | `assistant.skills.weather.api_key_ref` | `WEATHER_PROVIDER_API_KEY` |

LLM API keys (if any non-local model is ever configured) reuse the
existing spec 037 secret indirection — NOT a new path.

### 7.5 No legacy aliases

The prior draft kept `assistant.intent.*` aliases. This revision
DROPS them — the prior draft never shipped, so there is nothing to
migrate from. Net is a single clean key namespace.

---

## 8. Observability

### 8.1 Prometheus metrics (cardinality bounded)

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_assistant_facade_turns_total` | counter | `transport, outcome` | Facade-level turn count (`answered`/`captured`/`proposed`/`confirmed`/`discarded`/`error`) |
| `smackerel_assistant_facade_latency_seconds` | histogram | `transport, outcome` | Facade enter → response emit |
| `smackerel_assistant_router_band_total` | counter | `band, transport` | High / borderline / low decision counts (post-processor §3.2) |
| `smackerel_assistant_skill_invocations_total` | counter | `scenario_id, outcome, transport` | Per-scenario success/failure (maps to spec 037 `Outcome`) |
| `smackerel_assistant_capture_fallback_total` | counter | `cause, transport` | `low_confidence`/`borderline_timeout`/`confirm_discarded`/`confirm_timeout`/`error_offered_capture`/`unresolvable_reference` |
| `smackerel_assistant_confirm_card_outcomes_total` | counter | `scenario_id, outcome, transport` | `confirmed`/`discarded_user`/`discarded_timeout` |
| `smackerel_assistant_disambiguation_outcomes_total` | counter | `outcome, transport` | `resolved_user`/`resolved_timeout_capture`/`resolved_non_matching_reply_capture` |
| `smackerel_assistant_active_threads` | gauge | `transport` | Rows in `assistant_conversations` per transport |
| `smackerel_assistant_provenance_violations_total` | counter | `scenario_id` | Provenance gate trips |
| `smackerel_assistant_source_assembly_drops_total` | counter | `scenario_id, cause` | Graph drift |

Spec 037 metrics (router latency, executor outcomes, tracer counts)
are reused unchanged.

### 8.2 Structured log fields

Every assistant log line includes: `user_id` (hashed if policy
requires), `transport`, `assistant_turn_id` (ULID), `scenario_id`,
`top_score`, `band`, `status`, `error_cause`, `latency_ms`,
`agent_trace_id` (spec 037 trace cross-ref).

### 8.3 OpenTelemetry trace spans

```
assistant.adapter.translate           (adapter: transport.translate)
  └─ assistant.facade.handle          (facade entry)
     ├─ assistant.context.load
     ├─ assistant.router.classify     (calls agent.Router.Route)
     ├─ assistant.router.band         (borderline post-processor §3.2)
     ├─ agent.executor.run            (spec 037 span; reused)
     │  └─ agent.tool.<name>.invoke   (spec 037 span per tool)
     ├─ assistant.provenance.check
     ├─ assistant.confirm.persist     (when confirm_required + propose phase)
     ├─ assistant.context.persist
     └─ assistant.audit.write
  └─ assistant.adapter.render         (adapter: transport.render)
```

Spans attribute `transport`, `user_id`, `assistant_turn_id`, and the
cross-referenced `agent_trace_id`.

### 8.4 Operator dashboards

SCOPE-09 ships a Grafana panel fragment under
`deploy/observability/grafana/dashboards/` covering: per-transport
turn volume + band mix; per-scenario success/failure; capture-as-
fallback rate with `cause` breakdown; provenance violation counter
(target 0); active threads per transport.

---

## 9. Security & Policy

### 9.1 Per-skill PASETO scopes (spec 060 catalog additions)

| Scope | Type | Default grant |
|-------|------|---------------|
| `assistant.skill.retrieval` | read | Granted by default to existing bot-shared tokens |
| `assistant.skill.weather` | read | Granted by default to existing bot-shared tokens |
| `assistant.skill.notifications.write` | write | NOT granted by default; explicit owner grant per user |

Coordinated via a packet to spec 060 (owner edits its own catalog
per `bubbles-artifact-ownership-routing`). See §14.

`Assistant.Handle` checks the caller's PASETO scope claim against
each enabled scenario's required scope BEFORE invoking the executor.
On scope mismatch: `Status=StatusUnavailable,
ErrorCause=ErrMissingScope, Body="<scenario>: not permitted"` +
offer-to-capture.

### 9.2 Tool API key handling

- Weather API key: SST `assistant.skills.weather.api_key_ref` →
  Infisical `WEATHER_PROVIDER_API_KEY`. Resolved at deploy time. No
  plaintext in YAML. No `.env.secrets` entry.
- No secret in logs, traces, or metrics labels.

### 9.3 Rate limiting

Per `(user_id, transport, scenario_id)` counter-based limiter via
existing `internal/middleware/ratelimit/`. SST keys per §7.1
`assistant.rate_limit.*`. Exceed → `Status=StatusUnavailable,
ErrorCause=ErrInternalError, Body="<scenario>: rate limited"`.

### 9.4 Audit artifacts

Every assistant turn writes one `kind='assistant_turn'` artifact
(§6.3). Every confirm-card proposal writes one
`kind='assistant_proposal'` artifact (§5.4). Both honor Principle 3
+ Principle 8.

---

## 10. Module Layout (net-new only; spec 037 substrate untouched)

```
internal/
├── assistant/                                 # NEW — capability facade (NO router/registry/executor/tracer)
│   ├── doc.go
│   ├── facade.go                              # Assistant interface impl; thin orchestration over agent.Runner
│   ├── facade_test.go
│   ├── borderline.go                          # §3.2 post-processor on agent.RoutingDecision
│   ├── shortcuts.go                           # §3.4 slash-command → ScenarioID map
│   ├── contracts/                             # spec.md §3.1.4 — package 1
│   │   ├── doc.go
│   │   ├── message.go                         # AssistantMessage + kinds
│   │   ├── response.go                        # AssistantResponse (facade over agent.InvocationResult) + tokens
│   │   ├── source.go                          # Source, SourceRef, refs
│   │   ├── adapter.go                         # TransportAdapter interface
│   │   ├── assistant.go                       # Assistant interface
│   │   └── contracts_test.go
│   ├── provenance/                            # spec.md §3.1.4 — package 2
│   │   ├── doc.go
│   │   ├── gate.go                            # Enforce()
│   │   └── gate_test.go
│   ├── context/                               # spec.md §3.1.4 — package 3
│   │   ├── doc.go
│   │   ├── store.go                           # Store interface
│   │   ├── pg_store.go                        # PostgreSQL impl
│   │   ├── ticker.go                          # idle sweep
│   │   ├── reference_resolver.go              # "that one" / numeric
│   │   └── store_test.go                      # uses test PostgreSQL (ephemeral)
│   ├── confirm/                               # spec.md §3.1.4 — package 4 — OQ #7 state machine
│   │   ├── doc.go
│   │   ├── machine.go                         # propose / confirm / cancel / timeout
│   │   ├── machine_test.go
│   │   └── audit.go                           # assistant_proposal writes
│   └── skills_manifest.go                     # capability-side view of user-facing scenarios
│                                              # (user_facing_label, slash_shortcut, requires_provenance,
│                                              #  enable SST mapping). Sourced either from additive YAML
│                                              #  keys in the scenario files OR from config/assistant/
│                                              #  scenarios.yaml — see §4.1.
│
├── agent/                                     # SPEC 037 SUBSTRATE — UNTOUCHED
│   ├── (router.go, executor.go, bridge.go,
│   │    nats_driver.go, tracer.go, loader.go,
│   │    schema.go, registry.go, replay.go,
│   │    userreply/, render/, ...)
│   └── tools/                                 # NEW subpackages registering tool handlers in existing registry
│       ├── retrieval/
│       │   ├── doc.go
│       │   ├── tool.go                        # retrieval_search — wraps /api/search
│       │   └── tool_test.go
│       ├── weather/
│       │   ├── doc.go
│       │   ├── tool.go                        # weather_lookup
│       │   ├── provider.go                    # Provider interface
│       │   ├── open_meteo.go                  # concrete provider
│       │   ├── cache.go                       # LRU
│       │   └── tool_test.go
│       └── notification/
│           ├── doc.go
│           ├── propose.go                     # notification_propose tool
│           ├── execute.go                     # notification_execute tool — calls spec 054 scheduler
│           └── tool_test.go
│
├── telegram/
│   ├── bot.go                                 # EDITED — handleMessage plain-text branch delegates to assistant_adapter
│   ├── agent_bridge.go                        # SPEC 037 — REUSED AS THE INNER CORE OF THE TELEGRAM ADAPTER
│   ├── handleTextCapture.go                   # unchanged (adapter delegates on CaptureRoute=true)
│   └── assistant_adapter/                     # spec.md §3.1.4 — package 5 — NEW v1 reference adapter
│       ├── doc.go
│       ├── adapter.go                         # TransportAdapter impl; wraps existing AgentBridge
│       ├── translate_inbound.go               # *tgbotapi.Update → AssistantMessage
│       ├── render_outbound.go                 # AssistantResponse → *tgbotapi.MessageConfig (uses agent/userreply for base body)
│       ├── render_sources.go                  # trailing numbered block (UX §14.B.1)
│       ├── render_confirm.go                  # inline keyboard pair
│       ├── render_disambig.go                 # numbered list + optional inline keyboard
│       ├── callbacks.go                       # callback_data → AssistantMessage{Kind: Confirm|Disambig}
│       ├── identity.go                        # chat_id → user_id via spec 044
│       ├── reset.go                           # /reset command surface
│       └── adapter_test.go                    # golden tests vs UX §14.B.1
│
└── config/
    └── prompt_contracts/                      # NEW user-facing scenarios (alongside the existing 15)
        ├── retrieval-qa-v1.yaml               # §5.1
        ├── weather-query-v1.yaml              # §5.2
        └── notification-schedule-v1.yaml      # §5.3
```

**Forbidden paths (build-time package-existence test fails if any of
these exist):** `internal/assistant/router/`,
`internal/assistant/registry/`, `internal/assistant/executor/`,
`internal/assistant/tracer/`, `internal/assistant/loader/`,
`internal/assistant/llm/`, `internal/assistant/nats/`.

**Future adapter slots (NOT implemented in v1; documented as
future):** `internal/whatsapp/assistant_adapter/`,
`internal/webchat/assistant_adapter/`,
`internal/mobile/assistant_adapter/`.

---

## 10.1 Wiring Plan (`cmd/core/`)

Two edits in v1, both small:

1. **`cmd/core/wiring_agent.go`** — already constructs the
   production `agent.Router + agent.Executor` and the existing
   `telegram.AgentBridge`. Add: also construct
   `assistant.Facade{Runner, ContextStore, ConfirmMachine,
   SkillManifest, Borderline}` and return it on `coreServices` /
   `api.Dependencies` so downstream wiring can pass it to the
   Telegram bot constructor. Spec 037 substrate construction does
   NOT move.
2. **`cmd/core/wiring.go::startTelegramBotIfConfigured`** — extend
   `telegram.Config` (and the `NewBot` signature) with an
   `AssistantBridge contracts.Assistant` field; pass the
   capability facade through. Inside `internal/telegram/bot.go`,
   `handleMessage` plain-text branch delegates to the new
   `assistant_adapter.Adapter` (constructed once at `NewBot` time
   with the supplied `AssistantBridge`).

The existing `AgentBridge.Handle(ctx, chatID, text)` REST flow
(`POST /v1/agent/invoke` via `cmd/core/wiring_agent.go`) continues
to work unchanged — it's a separate caller of the same runner.

---

## 11. Testing Strategy (design-level; bubbles.plan owns per-scope Test Plan)

Per `bubbles-test-environment-isolation` and the Canonical Test
Taxonomy in `agent-common.md`.

### 11.1 Per-category coverage

| Category | Coverage | Live system | Notes |
|----------|----------|-------------|-------|
| **unit** | Canonical contract round-trip; borderline post-processor (golden); provenance gate; confirm state machine transitions (table-driven); reference resolver; Telegram adapter rendering (golden tests vs UX §14.B.1); package-import lint (capability MUST NOT import any `internal/<transport>/...`); **forbidden-package-existence lint** (no `internal/assistant/{router,registry,executor,tracer,loader,llm}/`) | No | Pure Go; no external deps |
| **functional** | Three v1 scenarios round-trip through real `agent.Bridge` loader → `agent.Router` → `agent.Executor` against real `ml/` sidecar + test PostgreSQL; confirm-card state persistence; context store CRUD; idle sweep | Yes (test PG + real `ml/` sidecar via NATS) | Ephemeral `docker-compose.test.yml` per `bubbles-test-environment-isolation` |
| **integration** | Capability + Telegram adapter end-to-end against real local stack via existing `AgentBridge`; confirm/disambig callbacks; spec 044 chat→user resolution; spec 054 scheduler reuse; provenance refusal path; borderline disambig→re-route via explicit scenario id | Yes | Telegram `tgbotapi` boundary mocked in-process (external dep); capability + spec 037 substrate REAL |
| **e2e-api** | Full Telegram update → adapter → facade → router → executor → response → render. Real `ml/` sidecar, real PostgreSQL, real spec 054 scheduler. Exercises BS-001..BS-010. | Yes | Driven by Telegram webhook fixture POSTs |
| **stress** | Router p95 latency under burst load (SLO from spec 037 + per-scenario `limits.timeout_ms`); concurrent confirm callbacks against the same `confirmRef` (single-flight) | Yes | Required per Gate G026; owned by SCOPE-04 + SCOPE-09 |

### 11.2 Adapter-substitution test (capability/adapter split is real)

`internal/assistant/facade_test.go` drives the facade via a
`fakeTransportAdapter` and asserts:
- Every UX §14.A primitive is observable from `AssistantResponse`
  alone (no transport-specific fields needed).
- The facade NEVER reaches into the adapter (the fake panics if
  any method other than `Identity()` is called from inside
  `Handle`).

### 11.3 Build-time architecture tests

Two tests in `internal/assistant/contracts/architecture_test.go`:

1. **Forbidden-package existence.** Fails if any of
   `internal/assistant/{router,registry,executor,tracer,loader,llm,nats}/`
   directories exist. Catches re-introduction of parallel substrate.
2. **Import direction.** Walks the AST of `internal/assistant/...`
   and fails on any import path beginning with
   `internal/telegram/`, `internal/whatsapp/`, `internal/webchat/`,
   `internal/mobile/`. Catches capability→transport leaks.

A third optional test asserts the facade imports
`internal/agent` (proving substrate reuse).

### 11.4 Anti-fabrication / live-stack authenticity

- Integration/e2e/stress MUST NOT use `httptest.Server` in place
  of the `ml/` sidecar or spec 054 scheduler.
- Telegram `tgbotapi` boundary MAY be a thin in-process fake.
- Every retrieval/weather test asserting non-empty `Body` MUST also
  assert non-empty `Sources` — otherwise the provenance gate would
  drop it.

---

## 12. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **Re-introduction of parallel agent runtime** (Hard Constraint 6 regression) | Medium (it was the prior design) | Critical | Forbidden-package-existence test (§11.3); explicit module-layout §10; FORBIDDEN list in §0 patterns-to-avoid. |
| **LLM hallucination on retrieval** (unsourced answers) | High | High | `requires_provenance: true` on retrieval scenario + `provenance.Enforce` gate (§4.3). `assistant_provenance_violations_total` exposes drift. |
| **Intent misclassification causing data loss** (capture goes to a skill) | Medium | Critical | Default-to-capture on low band; borderline disambiguation; `assistant_capture_fallback_total{cause}` exposes drift. |
| **Provider outages** (weather, `ml/` sidecar) | Medium | Medium | Closed `ErrorCause` vocabulary; offer-to-capture confirm card; router unavailable → fall through to capture. |
| **Confirm-card race** (two callbacks for same `confirm_ref`) | Low | Medium | ULID `confirm_ref`; PostgreSQL `pending_confirm` row deletion is the single-flight gate (UPDATE…RETURNING semantics or DELETE…RETURNING). |
| **Stale confirm callback after timeout** | Low | Low | Pending row already deleted by sweep; second callback resolves to "expired" body + capture. |
| **Spec 060 PASETO scope migration** | Low | High | Three new scopes; two read scopes granted to existing bot-shared tokens; write scope explicit-grant only; coordinated via packet (§14). |
| **Adapter business-logic leak** | Low | High | TransportAdapter MUST/MUST NOT (§2.3); adapter-substitution test (§11.2); package-import lint (§11.3). |
| **Spec 054 scheduler payload extension breaks spec 054 tests** | Medium | Medium | Additive optional fields; routed via packet; SCOPE-08 ships spec 054 owner-reviewed change. |
| **Loader rejects additive scenario YAML keys** | Medium | Low | Two fallback options (§4.1): either route a one-line allowlist addition via a Spec 037 bug folder, OR keep new keys in a sibling `config/assistant/scenarios.yaml` lookup. bubbles.plan chooses. |
| **`assistant_conversations` unbounded growth** | Low | Medium | Idle sweep (§6.2); active-threads gauge. |
| **Eval harness flakiness** masking regressions | Medium | Low | SCOPE-10 deterministic corpus + seeded RNG; CI gate ≥85% routing accuracy. |

---

## 13. Open Items for bubbles.plan

1. **YAML metadata channel for user-facing scenarios (§4.1).**
   Choose between (a) additive top-level keys + one-line Spec 037
   loader allowlist (routed via Spec 037 bug folder), OR (b)
   sibling `config/assistant/scenarios.yaml` lookup keyed by
   scenario id (zero Spec 037 change). Recommend (b) — strictly
   additive, no cross-spec coordination cost. bubbles.plan ratifies.
2. **`/reset` Telegram command** in SCOPE-05. Recommend shipping
   in SCOPE-05.
3. **Borderline-band runtime placement.** Design recommends the
   capability-layer post-processor (§3.2). Already resolves OQ #6
   per analyst packet directive; bubbles.plan needs only to
   schedule SCOPE-04 around it.
4. **`Runner` interface location (§3.3).** Recommend Option A
   (small `Runner` extraction in `internal/agent/runner.go`,
   routed via Spec 037 bug folder if any movement is needed),
   else Option B (facade composes directly).
5. **SCOPE ordering.** SCOPE-02 (contracts) before SCOPE-05
   (adapter). SCOPE-01 (SST) before SCOPE-04 (borderline). SCOPE-03
   (scenarios+tools) parallelizable with SCOPE-04. SCOPE-06/07/08
   depend on SCOPE-03. SCOPE-09 ships incrementally; final
   dashboard in SCOPE-09. SCOPE-10 last.
6. **Eval harness sizing (SCOPE-10).** Recommend ≥30 messages per
   intent label × 5 labels = ≥150 total; quarterly refresh; seeded
   RNG; CI gate ≥85%.

---

## 14. Spec 060 PASETO Scope Migration Plan

Routed via packet to spec 060 owner per
`bubbles-artifact-ownership-routing`. Three new constants:
`assistant.skill.retrieval` (read), `assistant.skill.weather`
(read), `assistant.skill.notifications.write` (write). Default-grant
migration adds the two read scopes to existing bot-shared tokens;
write scope requires explicit per-user grant via existing spec 060
grant API. SCOPE-05/08 each declare the matching catalog version as
blocking prerequisite. Full step-by-step migration shape is in the
prior draft (preserved in git history) and is unchanged by this
revision.

---

## 15. Product Principle Alignment

Per `.github/instructions/product-principles.instructions.md`.

### Principle 2 — Vague In, Precise Out
The retrieval scenario takes imprecise input ("what did I save
about Tailscale?") and returns precise sourced answers via semantic
search + LLM synthesis. The borderline-band disambiguation prompt
(§3.2) preserves precision in the face of ambiguity by ASKING
ONCE rather than guessing. Default-to-capture on uncertainty
preserves the input value rather than degrading it.

### Principle 6 — Invisible By Default, Felt Not Heard
- The capability NEVER initiates a conversation; every turn is
  user-initiated (UX §14.A.1).
- Borderline asks at most ONE clarifying question per turn
  (§3.2); a non-matching reply is captured.
- Confirm cards have a single ask + cancel + timeout (§5.4); the
  capability does NOT escalate or re-prompt.
- Capture-as-fallback emits no extra chatter — uses the existing
  `handleTextCapture` reply byte-for-byte.

### Principle 8 — Trust Through Transparency
- Every synthesized answer carries `sources[]` with either
  `artifact` (Smackerel knowledge graph ID + capture timestamp) or
  `external_provider` (provider name + retrieval timestamp). The
  `provenance.Enforce` gate (§4.3) mechanically drops un-sourced
  synthesis (BS-007).
- Every facade turn writes an `assistant_turn` artifact with
  `agent_trace_id` cross-reference (§6.3). Spec 037's
  `PostgresTracer` records every tool call. Trace lineage is
  reachable end-to-end: user message → router decision →
  scenario invocation → tool calls → cited artifacts.
- Every confirm proposal writes an `assistant_proposal` artifact
  regardless of outcome (§5.4). The user can audit what was
  proposed AND what was confirmed/discarded.

### Principle 1 — Observe First, Ask Second (justified deviation)
Already flagged in spec.md §11. The capability defaults to capture
on every uncertainty path; the active-ask path is reserved for
operations that cannot be inferred from capture alone (weather is
inherently action-time; retrieval is the read-side of the graph;
notifications are explicit time-bound user requests). Owner
ratification required at user-validation time.

---

## 16. References

- `spec.md` §3.1 — Substrate reuse contract (binding).
- `spec.md` §6 — Transport Adapter Contract.
- `spec.md` §14 — UX & Interaction Model.
- `specs/037-llm-agent-tools/` — agent runtime substrate; terminal
  `done`; consumed as-is.
- `internal/agent/router.go`, `executor.go`, `bridge.go`,
  `nats_driver.go`, `tracer.go`, `loader.go` — substrate types.
- `internal/telegram/agent_bridge.go` — existing reusable bridge.
- `internal/agent/userreply/` — existing 4-line Telegram budget renderer.
- `config/prompt_contracts/recommendation-why-v1.yaml` (and 14 others)
  — scenario YAML shape exemplar.
- `.github/skills/bubbles-capability-foundation-design/SKILL.md` —
  proportionality satisfied on the `TransportAdapter` axis.
- `.github/skills/bubbles-artifact-ownership-routing/SKILL.md` —
  spec 037 / 054 / 060 edits routed via packets.
- `.github/instructions/smackerel-no-defaults.instructions.md` —
  fail-loud SST policy.
- `.github/instructions/bubbles-test-environment-isolation.instructions.md`
  — ephemeral test backing stores.
- spec 044 — per-user bearer auth, chat→user_id mapping.
- spec 054 — notification scheduler (reused; additive payload
  extension via SCOPE-08 packet).
- spec 060 — PASETO scope claim catalog (three new scopes via
  packet — §14).
- spec 150 — Infisical-only secrets policy.
