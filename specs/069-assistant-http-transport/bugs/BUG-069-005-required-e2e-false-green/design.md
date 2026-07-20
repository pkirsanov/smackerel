# Design: BUG-069-005 Complete the compiler-backed HTTP state-machine path

## Root Cause Analysis

### Investigation Summary

The authoritative requirements and current source were compared in this order:

1. Spec 068 `spec.md`, `design.md`, `scopes.md`, and `state.json`.
2. Spec 069 `spec.md`, `design.md`, `scopes.md`, `state.json`, and
   `scenario-manifest.json`.
3. Spec 071 and Spec 076 specification/design/scope/state surfaces.
4. Current SST, generated test env, core wiring, compiler client, facade gates,
   five required tests, and generic regression-quality guard.

The stale-test hypothesis is falsified. Spec 068 explicitly transfers its live
HTTP compiler proofs to Spec 069; Spec 069 lists the five tests as required;
Spec 071 depends on compiled turns; and Spec 076 preserves disambiguation and
confirm parity without superseding the compiler path.

### Root Cause

The transport-neutral compiler was implemented as a nullable facade extension,
but the vertical delivery stopped before provider and production wiring. The
nullable path allowed core startup with no compiler, and explicit test config
selected that incomplete state. Separately, the clarification and side-effect
branches returned prose/capture envelopes instead of entering the existing
persistent interaction machines. Required E2E tests then treated those absent
controls as nondeterminism and skipped.

The defect chain is:

```text
ASSISTANT_INTENT_COMPILER_ENABLED=false
  -> no production NewLLMCompiler construction
  -> no Facade.WithIntentCompiler call
  -> no live ML /assistant/intent/compile handler
  -> raw-text/pre-068 route remains possible
  -> required DisambiguationPrompt/ConfirmCard is absent
  -> five required tests call t.Skipf
  -> package exit 0 is misread as scenario completion
```

### Root Cause Classes

| Class | Concrete failure |
|-------|------------------|
| Configuration coherence | Required HTTP compiler E2E runs with compiler explicitly disabled. |
| Runtime integration | Compiler client/provider and core facade attachment are absent. |
| State-machine composition | Compiler clarify/write outcomes do not enter persistent disambiguation/confirm machines. |
| Test integrity | Required behavior absence becomes skip rather than failure. |
| Framework enforcement | Generic guard does not mechanically reject Go skip-family calls. |

### Impact Analysis

- Affected components: config generation/loading, ML sidecar compiler route,
  core assistant wiring, facade clarification/write branches, conversation
  pending state, HTTP response controls, required assistant E2E, and upstream
  regression-quality guard.
- Affected data: ephemeral and production assistant conversation pending state;
  list/annotation effects are at risk of lacking required proof but no
  unauthorized write is asserted by this packet.
- Affected users: every user-facing natural-language transport sharing the
  facade, not only web HTTP.

## Capability Foundation

This is a narrow repair inside existing foundations; no new plugin/provider
framework is introduced.

| Existing contract | Responsibility reused by the repair | Consumers |
|-------------------|-------------------------------------|-----------|
| `intent.Compiler` / `LLMCompiler` | Typed compiler request, response validation, and failure contract | assistant facade |
| `intent.Transport` | Core-to-ML provider boundary | production HTTP transport and deterministic external-provider test fixture |
| `assistant.Facade` | One transport-neutral turn orchestration path | HTTP, Telegram, WhatsApp, future mobile |
| Existing conversation store | Durable per-user/per-transport pending state | disambiguation and confirm machines |
| Existing confirm machine | Proposal persistence, single-flight accept/reject, audit | compiler write intents |
| Existing disambiguation response/state | Choice persistence and callback resolution | compiler ambiguity intents |
| `httpadapter.TurnResponse` | Stable v1 wire envelope | required E2E and future clients |

### Variation Axes

| Axis | Options | Foundation owner |
|------|---------|------------------|
| Compiler provider | production local inference, deterministic test external-provider fixture | ML provider adapter |
| Transport | web, Telegram, WhatsApp, future mobile | `TransportAdapter` |
| Interactive control | disambiguation, confirm | assistant state machines |
| Outcome | route, clarify, confirm, capture, refuse | facade policy |

## Minimal Correct Repair

### 1. SST And Startup Coherence

Keep every existing `assistant.intent_compiler.*` key required. Add fail-loud
cross-field validation:

- if any user-facing assistant transport is enabled, compiler enablement must
  be true;
- if compiler enablement is true, provider transport construction and route
  readiness are mandatory startup dependencies;
- the disposable test stack explicitly compiles with `enabled=true`;
- no runtime fallback restores a nil compiler or raw-text route.

An operator can disable the entire user-facing assistant surface, but cannot
claim a working assistant transport while disabling the compiler required by
Spec 068.

### 2. Real Compiler Provider Transport

Implement the designed internal ML sidecar route:

```text
core intent.Transport
  -> POST /assistant/intent/compile
  -> ML-side provider adapter
  -> schema-bound CompileResponse
  -> Go LLMCompiler schema validation
```

The ML route accepts only the existing versioned request, calls no product tool,
and returns only compiler JSON plus bounded provider metadata. It uses the same
service authentication/network policy as other core-to-ML calls.

For deterministic E2E, the disposable test stack supplies a deterministic
external LLM-provider fixture behind the ML provider interface. Core still
calls the real sidecar route; the sidecar still validates and maps the provider
response; facade, HTTP, auth, persistence, and callbacks remain real. The
fixture is selected by explicit test-stack composition/SST and is not compiled
as a production-selectable bypass.

### 3. Core Facade Wiring

In the assistant wiring owner:

1. Construct the bounded production `intent.Transport`.
2. Map validated `config.IntentCompilerConfig` into
   `intent.CompilerConfig` without defaults.
3. Construct `intent.NewLLMCompiler`.
4. Call `facade.WithIntentCompiler(compiler)` exactly once before adapters are
   bound.
5. Return a startup error for every missing/invalid dependency.

The existing nullable builder remains useful for isolated unit construction,
but production wiring may not leave it nil when a user-facing assistant
transport is enabled.

### 4. Persistent Clarification Path

The current `PendingClarify` record is an abandoned-turn capture timer, not a
replacement for user-selectable disambiguation state. The repaired path is:

```text
CompiledIntent(action=clarify, structured location ambiguity)
  -> read-only location normalization/resolution capability
  -> canonical candidate list
  -> existing pending disambiguation state persisted by (user, transport)
  -> AssistantResponse.DisambiguationPrompt with stable ref + choices
  -> callback validates user/transport/ref/choice
  -> pending state cleared once
  -> selected location resumes compiled weather action
```

No weather provider call occurs before selection. Candidate derivation uses
compiled structured context and the read-only resolver, not a new raw-text
regex or a transport branch.

### 5. Persistent Confirmation Path

Replace the valid-write capture envelope with the existing confirm proposal
path:

```text
CompiledIntent(side_effect=write|external_write)
  -> build schema-validated proposal payload
  -> confirm.Machine.Propose
  -> persist PendingConfirm by (user, transport)
  -> AssistantResponse.ConfirmCard
  -> accept callback loads and clears pending state
  -> execute gated action exactly once
  -> replay returns stale/pending-not-found outcome without execution
```

The proposal payload carries the compiled action and slots required for the
post-confirm executor. It never trusts callback-supplied business arguments.
Capture-as-fallback remains available for compiler/provider failure,
abandonment, refusal, or explicit capture policy; it is not the response for a
valid write awaiting confirmation.

### 6. HTTP Response Contract

No v1 schema change is required: `TurnResponse` already has
`disambiguation_prompt` and `confirm_card`. The repair populates those existing
fields and preserves `schema_version`, transport/message-id echo, trace fields,
and bearer-auth behavior.

### 7. Deterministic Required E2E

The five existing test functions remain the protected public test identities.
They are strengthened rather than renamed or replaced:

- remove behavior-dependent `t.Skipf` branches;
- fail immediately when the expected control is absent;
- use deterministic provider fixtures for compiler output;
- assert persistent pending state through the live ephemeral store;
- assert no mutation before accept and exactly one after accept;
- use adversarial annotation text that cannot pass through a legacy keyword
  path;
- report five passes and zero skips under one exact repo-CLI selector.

### 8. Framework Guard Routing

`regression-quality-guard.sh` is framework-managed in Smackerel, so this packet
does not edit it. Route a companion upstream Bubbles change:

1. Add language-aware required-test bailout patterns for Go `t.Skip`,
   `t.Skipf`, and `t.SkipNow`.
2. Emit a stable violation code such as `REQUIRED_TEST_SKIP`.
3. Add hermetic positive/negative selftests, including a required Go test with
   `t.Skipf` that must exit non-zero.
4. Run Bubbles framework validation.
5. Propagate the framework release through the canonical installer; never
   patch `.github/bubbles/**` directly in this product bug.

## Persistence And State Integrity

- Reuse existing assistant conversation rows and pending-state columns.
- No parallel pending-state table or client cache is introduced.
- Test state uses unique user/message/ref IDs in disposable Postgres.
- Each E2E test either owns isolated rows or snapshots/restores only its exact
  key; table-wide cleanup is forbidden.

## Security And Privacy

- Bearer auth/scope middleware runs before any compiler or facade invocation.
- Test provider selection does not add an anonymous route or weaken service
  authentication.
- Compiler raw text/slots are redacted according to the existing IntentTrace
  policy before export.
- Confirm/disambiguation callbacks are bound to authenticated user and
  canonical transport.
- The server executes the persisted proposal, not client-submitted action
  arguments.

## Offline And Deployment Posture

- No cloud service is required; production may use the configured local
  provider.
- The deterministic provider is local to the disposable test stack and leaves
  no resources after teardown.
- No deploy adapter, target topology, secret, or release-train file changes are
  part of this packet.

## Mechanical Change Boundary

### Product Repair Allowed

- `internal/config/**` and `config/smackerel.yaml` compiler coherence only
- generated test env through the sanctioned config generator
- `internal/assistant/intent/**` provider transport additions
- `ml/app/**` real compiler route and provider adapter
- `cmd/core/wiring_assistant_facade.go` compiler construction/attachment
- existing assistant facade clarification/confirmation composition
- exact focused unit/integration/E2E tests and test-stack external-provider
  fixture composition
- bug packet evidence updates

### Product Repair Excluded

- public HTTP v1 field changes
- anonymous/test-only product routes
- raw-text regex/keyword replacement logic
- parallel conversation or pending-state stores
- client-side business-state caches
- parent Spec 069 artifact edits before validate-owned reconciliation
- release-train, feature-flag bundle, deploy, secret, or target files

### Foreign Framework Route

- Upstream only: `bubbles/scripts/regression-quality-guard.sh` and its
  selftest/docs under a separate Bubbles-owned change.
- Forbidden here: direct edits to Smackerel `.github/bubbles/**`,
  `.github/agents/bubbles*`, `.github/skills/bubbles-*`, or
  `.github/instructions/bubbles-*`.

## Alternatives Considered

1. **Remove the five tests from the manifest.** Rejected because Specs 068 and
   069 still require the behavior and no superseding scenario exists.
2. **Keep skips for model nondeterminism.** Rejected because required behavior
   must be deterministic at the provider boundary; a skip proves nothing.
3. **Call the facade directly from E2E.** Rejected because it bypasses HTTP,
   auth, core wiring, ML transport, and persistence.
4. **Add a test-only assistant endpoint.** Rejected as an insecure production
   bypass and a false live-stack test.
5. **Use raw-text heuristics to force confirm/disambiguation.** Rejected because
   it recreates the legacy path Spec 068 replaced.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|----------|------------------------|--------------|
| Real ML route plus provider seam | Turn compiler on and inject hardcoded output in core | Would bypass the designed service boundary and leave production unwired. |
| Persistent interaction-machine composition | Return a synthetic `ConfirmCard`/`DisambiguationPrompt` only in the response | Would not support the second turn, replay protection, or durable state. |
| Upstream guard route | Add a Smackerel-only grep override | Would fork framework policy and be overwritten by propagation. |
