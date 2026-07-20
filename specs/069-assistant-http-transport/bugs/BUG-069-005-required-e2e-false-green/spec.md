# Spec: BUG-069-005 Required assistant E2E must prove real compiler state transitions

**Status:** in_progress
**Workflow Mode:** `bugfix-fastlane`
**Release Train:** `mvp`
**Flags Introduced:** none

## Problem Statement

The Spec 069 manifest requires five live HTTP tests for compiler-driven
annotation, confirmation, disambiguation, clarification, and list-write
behavior. On the certified parent revision, all five execute and skip when the
required response control is absent. The package exits 0 while no required
behavior passes.

The repair must complete the real vertical path rather than weakening the
manifest, reclassifying the tests, or adding an anonymous/test-only production
endpoint.

## Authoritative Expected Behavior

- Spec 068 is authoritative for compiler-before-route, ambiguity handling,
  structured slots, and side-effect gating.
- Spec 069 is authoritative for bearer-authenticated HTTP transport and live
  cross-spec E2E proof through `POST /api/assistant/turn`.
- Spec 061's existing persistent conversation, disambiguation, and confirm
  machines remain the state authority.
- Spec 071 observes compiled turns and cannot replace compiler execution.
- Spec 076 preserves cross-transport disambiguation and confirm-card parity and
  does not list this path as a post-release exception.

## Domain Capability Model

### Capability

**Compiler-backed assistant turn execution** converts each authenticated
natural-language turn into one validated `CompiledIntent`, then composes the
existing routing, disambiguation, confirmation, persistence, and transport
rendering capabilities.

This bug adds no new capability foundation. It completes wiring inside the
existing `intent.Compiler`, `assistant.Facade`, `TransportAdapter`,
disambiguation state, and confirm state foundations.

### Domain Primitives

| Primitive | Purpose | Lifecycle |
|-----------|---------|-----------|
| `RawTurn` | Authenticated transport-normalized user input | received -> compiled or refused/captured |
| `CompiledIntent` | Schema-validated action, slots, side-effect class, and ambiguity metadata | compiled -> validated -> clarify, confirm, route, capture, or refuse |
| Pending disambiguation | Durable choices for one user and transport | proposed -> selected or expired |
| Pending confirm | Durable proposed write for one user and transport | proposed -> accepted, rejected, expired, or replay-rejected |
| `AssistantResponse` | Stable HTTP/transport response envelope | emitted once per accepted turn |

### Business Policies

- Natural-language input cannot reach the router with no compiled intent when
  any user-facing assistant transport is enabled.
- The compiler cannot execute tools or write business state.
- Clarification candidates are resolved after compilation through an existing
  read-only capability and persisted in the existing disambiguation state.
- Write and external-write intent never executes before the existing confirm
  machine accepts the persisted proposal.
- Required E2E tests fail on absent behavior; skip-family calls cannot satisfy a
  required scenario.

## Requirements

### R1 - Compiler availability is coherent and fail-loud

The test stack must explicitly set `assistant.intent_compiler.enabled=true`.
When any user-facing assistant transport is enabled, a disabled or unwireable
compiler must fail startup with a named configuration/wiring error. It must not
silently restore raw-text routing.

### R2 - The real provider transport exists

The core must call the real internal ML compiler route through an
`intent.Transport` implementation. The ML sidecar must expose the designed
`POST /assistant/intent/compile` request/response contract and return only
schema-bound compiler output; it must not execute tools.

### R3 - Core wiring attaches the compiler exactly once

`cmd/core` constructs `intent.NewLLMCompiler` from fail-loud SST, provider
transport, and bounded client settings, then calls
`Facade.WithIntentCompiler` exactly once during assistant startup. Constructor,
transport, or route failures abort startup when compiler-backed transports are
enabled.

### R4 - Clarification uses persistent disambiguation state

A compiled ambiguous Springfield weather turn must resolve candidate locations
through an existing read-only capability, persist the established pending
disambiguation shape keyed by `(user_id, transport)`, and return a non-empty
`DisambiguationPrompt`. Selecting a candidate clears the same pending record and
continues with the selected location. No weather lookup runs before selection.

### R5 - Writes use persistent confirmation state

A compiled list or annotation write must call the existing confirm proposal
path, persist a pending confirm payload containing the validated compiled
intent/action, and return a non-empty `ConfirmCard`. A valid accept executes the
action once, clears pending state before effect/audit completion, and rejects
replay. A valid confirm-required write is not mislabeled as capture fallback.

### R6 - Compiled slots drive annotation behavior

Annotation interaction/action values come from the validated compiled intent,
not from raw-text keywords. The adversarial E2E input must omit the legacy
keywords that would make the assertion tautological.

### R7 - Required tests are deterministic and strict

The five named tests must run through the live bearer-authenticated HTTP route,
real core facade, real ML sidecar route, and ephemeral persistence. Their
external LLM dependency may use a deterministic test provider behind the same
provider interface and route. Missing expected controls or state transitions
must call `t.Fatal`/`t.Fatalf`, never `t.Skip`, `t.Skipf`, or `t.SkipNow`.

### R8 - No insecure test bypass

No anonymous route, shared-secret bypass, handler-only fixture endpoint,
runtime `if env == test` branch, or direct facade call may satisfy the live E2E
contract. The deterministic provider is selected by explicit test-stack
composition and SST while production uses the same core-to-sidecar protocol.

### R9 - Mechanical skip-family coverage is routed upstream

The upstream Bubbles regression-quality guard must reject Go `t.Skip`,
`t.Skipf`, and `t.SkipNow` in required test files, with hermetic selftests. The
Smackerel framework-managed copy is updated only by canonical Bubbles
propagation, never patched inline in this bug.

### R10 - Completion remains evidence-bound

This bug remains `in_progress` until the same exact five tests report five
passes and zero skips, the focused and broader required suites pass, artifact
lint and traceability pass, and `bubbles.validate` certifies the packet. Parent
Spec 069 state reconciliation is validate-owned and occurs only after real
behavior evidence exists.

## Acceptance Scenarios

```gherkin
Feature: Required assistant HTTP scenarios fail closed and execute real behavior

  Scenario: SCN-BUG069005-001 - Annotation slots come from the live compiled intent
    Given the disposable test stack has the compiler enabled through the real ML route
    And the annotation input omits legacy annotation keywords
    When an authenticated user sends the annotation turn over POST /api/assistant/turn
    Then a persistent ConfirmCard is returned from compiled state-mutation slots
    And accepting it applies the compiled annotation values exactly once

  Scenario: SCN-BUG069005-002 - Springfield ambiguity creates a persistent choice
    Given the compiler and location normalization capability are wired
    When an authenticated user asks for Springfield weather over HTTP
    Then the response contains a persistent DisambiguationPrompt with at least two choices
    And no weather lookup occurs before a choice is submitted

  Scenario: SCN-BUG069005-003 - Disambiguation choice resolves pending state
    Given a prior HTTP turn persisted a DisambiguationPrompt
    When the user submits one listed choice with the issued reference
    Then the same pending state is cleared exactly once
    And the selected candidate drives the resumed assistant turn

  Scenario: SCN-BUG069005-004 - List write is not persisted before confirmation
    Given the compiler returns a validated list-write intent
    When an authenticated user asks to add milk to a shopping list
    Then the response contains a persistent ConfirmCard
    And the list is unchanged until the issued confirm reference is accepted

  Scenario: SCN-BUG069005-005 - Confirm acceptance executes the gated action once
    Given a prior HTTP turn persisted a ConfirmCard
    When the user accepts the issued confirm reference
    Then the proposed action executes exactly once
    And replaying the same reference does not execute it again

  Scenario: SCN-BUG069005-006 - Required tests fail closed instead of skipping
    Given the five manifest-required assistant HTTP tests are selected through the repo CLI
    When any required compiler, disambiguation, or confirmation behavior is absent
    Then the responsible test fails rather than calling a Go skip-family method
    And a healthy run reports five passes and zero skips
```

## Release Train

This bug repairs behavior already claimed by the active `mvp` train. It
introduces no feature flag and does not modify release-train or train-bundle
files. Compiler enablement remains explicit SST, with cross-field validation
that prevents an enabled user-facing assistant transport from starting without
its required compiler.

## Security Implications

- Bearer auth and required assistant scope remain mandatory before facade
  invocation.
- The deterministic test provider is an external-provider substitute behind
  the real ML sidecar route; it does not bypass auth, schema validation,
  side-effect gates, or persistence.
- Compiler prompts, slots, bearer tokens, and user IDs must not enter logs or
  metric labels in raw form.
- Confirm references and disambiguation references remain user/transport
  scoped and replay protected.

## Offline Implications

No cloud dependency is introduced. The deterministic E2E provider runs inside
the disposable local test stack, and supported production remains compatible
with local inference through the configured provider. This bug changes no
mobile offline data contract and adds no client-side cache as a data source.

## Configuration Implications

- All existing `assistant.intent_compiler.*` keys stay required and explicit.
- Test configuration must set `enabled=true`; production/startup validation
  rejects the incoherent combination of an enabled user-facing assistant
  transport and a disabled compiler.
- Provider URL/route, timeout, model role, prompt contract, schema version,
  output cap, and retry budget have no fallback values.
- No secret values or environment-specific topology are added to committed
  product config.

## Non-Goals

- Changing the public assistant v1 HTTP schema.
- Replacing the existing compiler, confirm, or disambiguation foundations.
- Adding an anonymous or test-only production ingress.
- Modifying parent Spec 069 artifacts in this packet.
- Editing release trains, feature-flag bundles, secrets, deployment adapters,
  or framework-managed Bubbles files in the Smackerel repo.
