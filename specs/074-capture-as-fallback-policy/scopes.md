# Scopes: 074 Capture-as-Fallback Cross-Cutting Policy

## Execution Outline

### Phase Order

1. **Scope 1 — Policy Foundation, Config, And Inviolability:** create the transport-neutral fallback policy, required SST validation, normalization contract, no-disable invariant, and no-interpretation-at-capture rule.
2. **Scope 2 — Provenance And Explicit/Fallback Separation:** persist distinct provenance metadata so explicit captures and fallback captures never collapse into one analytic or dedup source.
3. **Scope 3 — Per-User Dedup Semantics:** enforce same-user same-text dedup within the configured window, outside-window new captures, and forbidden cross-user dedup.
4. **Scope 4 — Trigger Execution And Abandoned Clarification:** route unrouted/no-ground/abandoned clarification turns through the capture path exactly once and preserve the original prompt.
5. **Scope 5 — Telemetry, IntentTrace Link, And Cross-Transport Acknowledgement:** emit counters/trace links and prove the saved-as-idea acknowledgement shape is identical across transports.

### New Types & Signatures

- `capturefallback.Policy.Decide(ctx, Request) (Decision, error)`
- `capturefallback.Policy.Capture(ctx, Decision) (CaptureResult, error)`
- `type Cause string` values: `unrouted`, `open_knowledge_no_ground`, `clarify_abandoned`, `compiler_error`
- `type Provenance string` values: `capture-as-fallback`, `capture-explicit`
- `artifact_capture_policy(artifact_id, user_id, provenance, fallback_cause, normalized_text_hash, dedup_bucket_start, dedup_window_seconds, source_turn_id, intent_trace_id, abandoned_clarification, already_captured_source_id, schema_version, created_at)`
- Acknowledgement shape: `AssistantResponse{status:"saved_as_idea", capture_ack:{schema_version, provenance, idea_artifact_id, already_captured, trace_id}}`

### Validation Checkpoints

- After Scope 1, config/no-disable/no-interpretation tests must pass before any facade hook can call the policy.
- After Scope 2, provenance metadata tests must prove explicit and fallback captures remain distinct.
- After Scope 3, dedup tests must prove same-user window behavior and cross-user isolation before trigger integration.
- After Scope 4, integration tests must prove eligible turns and abandoned clarifications produce exactly one Idea.
- After Scope 5, telemetry/trace/renderer tests must prove observability and acknowledgement parity across Telegram, HTTP, WhatsApp, web, and Android.

### Planning Notes

- `.github/bubbles-project.yaml` has no `testImpact` or `traceContracts` entries.
- Scope 1 is `foundation:true` because design defines `CaptureFallbackPolicy` as a shared policy consumed by facade, open-knowledge, compiler timeout, transport renderers, and telemetry.
- All scopes are runtime-behavior scopes and include live-system validation rows.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1 | Policy Foundation, Config, And Inviolability | policy module, config validation, normalization | SCN-074-A08, SCN-074-A09, SCN-074-A10 | Not Started |
| 2 | Provenance And Explicit/Fallback Separation | artifact metadata, explicit capture amendment seam | SCN-074-A02 | Not Started |
| 3 | Per-User Dedup Semantics | dedup store, normalized hashes, time buckets | SCN-074-A03, SCN-074-A04, SCN-074-A05 | Not Started |
| 4 | Trigger Execution And Abandoned Clarification | facade, compiler/open-knowledge hooks, capture writer | SCN-074-A01, SCN-074-A06 | Not Started |
| 5 | Telemetry, IntentTrace Link, And Cross-Transport Acknowledgement | metrics, IntentTrace, transport renderers | SCN-074-A07, SCN-074-A11 | Not Started |

---

## Scope 1: Policy Foundation, Config, And Inviolability

**Status:** Not Started  
**Depends On:** —  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-074-A08 — Missing SST keys fail loud
  Given capture_as_fallback.dedup_window is unset
  When the core process starts
  Then startup fails with a NO-DEFAULTS error naming the missing key

Scenario: SCN-074-A09 — Capture is inviolable
  Given any SST configuration in any environment
  When a turn meets the trigger contract (unrouted, no ground, not in clarify, not in confirm)
  Then a fallback Idea is produced
  And no SST key exists that can suppress fallback capture for that turn

Scenario: SCN-074-A10 — No content interpretation at capture time
  Given a fallback-eligible turn
  When the Idea artifact is created
  Then the artifact contains the normalized text and provenance only
  And no inferred tags, topics, or categories are attached at capture time
```

### Implementation Plan

- Add `internal/assistant/capturefallback` with `Policy.Decide`, `Policy.Capture`, closed cause vocabulary, and closed provenance vocabulary.
- Add fail-loud config validation for `capture_as_fallback.dedup_window`, `clarify_abandon_timeout`, `normalization_policy`, `dedup_hash_key`, and `retention_audit_days`.
- Encode the invariant that no `disable_capture_as_fallback` key exists and no suppression branch can be configured for eligible turns.
- Add strict `nfkc_casefold_ws_v1` normalization contract and hash-key handling without defaults.
- Ensure capture payload creation excludes inferred tags, topics, categories, and lifecycle guesses.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Facade fallback policy | Eligible turns must not be dropped | TP-074-02 inviolability guard |
| Config loader | Missing dedup/abandonment/hash settings fail loud | TP-074-01 config row |
| Idea artifact shape | Capture writes normalized text/provenance only | TP-074-03 no-interpretation row |

### Change Boundary

- **Allowed file families:** `internal/assistant/capturefallback/**`, `internal/config/**`, config validation tests, policy unit tests.
- **Excluded surfaces:** explicit capture flow implementation, transport renderers, open-knowledge routing, IntentTrace implementation, ML sidecar.
- **Containment rule:** this scope defines policy contracts only; it must not wire facade trigger execution until Scope 4.

### Impact-Aware Validation

No project impact map is configured. Because this scope defines a shared runtime policy, unit guard rows must run before integration work begins.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-074-01 | SCN-074-A08 | unit | `internal/config/capture_fallback_test.go` | Planned: missing capture_as_fallback.dedup_window fails loud | `./smackerel.sh test unit` | No |
| TP-074-02 | SCN-074-A09 | unit/guard | `tests/integration/policy/capture_fallback_inviolable_test.go` | Planned: no config key or branch can suppress eligible fallback capture | `./smackerel.sh test integration` | Yes |
| TP-074-03 | SCN-074-A10 | unit | `internal/assistant/capturefallback/payload_test.go` | Planned: fallback payload contains normalized text and provenance only | `./smackerel.sh test unit` | No |
| TP-074-04 | SCN-074-A09 | e2e-api | `tests/e2e/assistant/capture_fallback_inviolable_e2e_test.go` | Planned regression: eligible live turn always produces fallback Idea | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Policy foundation, required config, no-disable invariant, normalization contract, and no-interpretation rule satisfy SCN-074-A08, SCN-074-A09, and SCN-074-A10.
- [ ] TP-074-01 through TP-074-04 pass with evidence.
- [ ] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, and artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not run implementation, build, lint, or test commands. Each unchecked item requires current-session execution evidence before completion.

---

## Scope 2: Provenance And Explicit/Fallback Separation

**Status:** Not Started  
**Depends On:** Scope 1  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-074-A02 — Explicit capture is provenance-distinct
  Given the user invokes the spec 008 explicit capture path with text T
  When the Idea artifact is created
  Then provenance = "capture-explicit"
  And a later fallback capture of the same normalized text T (within the dedup window or outside it) does NOT dedup against this artifact
```

### Implementation Plan

- Add `artifact_capture_policy` metadata persistence with closed provenance values.
- Amend explicit capture flow through its owning implementation path so explicit captures write `capture-explicit` metadata without entering fallback dedup.
- Ensure fallback `content_hash` or dedup key includes user, provenance, normalized hash, and dedup bucket so explicit/fallback captures remain separate.
- Add analytics query coverage proving provenance can distinguish explicit and fallback captures without heuristics.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Existing spec 008 capture path | Explicit captures keep their own provenance and do not enter fallback dedup | TP-074-05 integration row |
| Artifact graph | No new artifact type is introduced | TP-074-06 metadata query row |
| Dedup metadata | Provenance is part of dedup separation | TP-074-07 e2e-api row |

### Change Boundary

- **Allowed file families:** `internal/assistant/capturefallback/**`, artifact metadata migration/store, explicit capture metadata write seam, targeted integration tests.
- **Excluded surfaces:** explicit capture user UX copy, transport renderers, topic/tag extraction, lifecycle state additions.
- **Containment rule:** do not dedup explicit captures against fallback captures under any time window.

### Consumer Impact Sweep

| Consumer | Search Surface | Validation |
|---|---|---|
| Explicit capture analytics | provenance field and capture-explicit value | TP-074-05 |
| Fallback dedup store | provenance value in unique/dedup key | TP-074-07 |
| Artifact readers | Idea artifact remains existing type | TP-074-06 |

### Impact-Aware Validation

No configured impact map exists. Provenance changes require integration evidence because both explicit and fallback paths are runtime capture paths.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-074-05 | SCN-074-A02 | integration | `tests/integration/assistant/capture_fallback_policy_test.go` | Planned: explicit capture writes capture-explicit provenance | `./smackerel.sh test integration` | Yes |
| TP-074-06 | SCN-074-A02 | integration | `tests/integration/assistant/capture_provenance_query_test.go` | Planned: explicit and fallback captures are distinguishable by provenance query | `./smackerel.sh test integration` | Yes |
| TP-074-07 | SCN-074-A02 | e2e-api | `tests/e2e/assistant/capture_provenance_e2e_test.go` | Planned regression: explicit and fallback same text create separate Ideas | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Metadata persistence and explicit/fallback provenance separation satisfy SCN-074-A02.
- [ ] TP-074-05 through TP-074-07 pass with evidence.
- [ ] Consumer Impact Sweep confirms no query or store path treats explicit and fallback provenance as the same source.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime or test commands.

---

## Scope 3: Per-User Dedup Semantics

**Status:** Not Started  
**Depends On:** Scope 2  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-074-A03 — Same-user same-text within dedup window dedupes
  Given a user sends a fallback-eligible turn with normalized text T
  And the same user sends another fallback-eligible turn with normalized text T within capture_as_fallback.dedup_window
  When the facade processes the second turn
  Then exactly one Idea artifact exists for (user, T)
  And the second turn's acknowledgement indicates "already captured"

Scenario: SCN-074-A04 — Same-user same-text outside dedup window does not dedup
  Given a user sends a fallback-eligible turn with normalized text T
  And the same user sends another fallback-eligible turn with normalized text T after capture_as_fallback.dedup_window has elapsed
  When the facade processes the second turn
  Then two distinct Idea artifacts exist with provenance = "capture-as-fallback"

Scenario: SCN-074-A05 — Cross-user dedup is forbidden
  Given user U1 captures text T as a fallback Idea
  When user U2 sends a fallback-eligible turn with the same normalized text T
  Then a separate Idea artifact is created for U2
  And no cross-user dedup occurs
```

### Implementation Plan

- Implement strict normalized-text equality dedup per `(user_id, provenance, normalized_text_hash, dedup_bucket_start)`.
- Add bucket calculation from explicit SST dedup window and required hash key.
- Ensure dedup hit returns canonical acknowledgement with `already_captured=true` and existing artifact id linkage.
- Add tests for same-window hit, outside-window new artifact, and cross-user isolation.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Dedup store/index | Per-user scoped uniqueness must not collapse users | TP-074-10 cross-user row |
| Acknowledgement metadata | Dedup hit exposes `already_captured=true` without changing copy | TP-074-08 row |
| Hash/key handling | Missing hash key fails in Scope 1 and no static literal is used | TP-074-08 and TP-074-10 inspect metadata |

### Change Boundary

- **Allowed file families:** `internal/assistant/capturefallback/**`, metadata store/index tests, integration/e2e capture tests.
- **Excluded surfaces:** explicit capture path beyond provenance metadata already introduced, cross-user analytics aggregation, transport renderer copy.
- **Containment rule:** dedup cannot use similarity/embedding thresholds in this v1 scope.

### Impact-Aware Validation

No project impact map is configured. Dedup touches mutable state, so integration/e2e rows must use isolated test users and disposable test stores.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-074-08 | SCN-074-A03 | unit | `internal/assistant/capturefallback/dedup_test.go` | Planned: same user same normalized text within window returns dedup hit | `./smackerel.sh test unit` | No |
| TP-074-09 | SCN-074-A04 | unit | `internal/assistant/capturefallback/dedup_test.go` | Planned: same user same normalized text outside window creates a new bucket | `./smackerel.sh test unit` | No |
| TP-074-10 | SCN-074-A05 | integration | `tests/integration/assistant/capture_fallback_policy_test.go` | Planned: cross-user same normalized text creates separate Ideas | `./smackerel.sh test integration` | Yes |
| TP-074-11 | SCN-074-A03 | e2e-api | `tests/e2e/assistant/capture_fallback_dedup_e2e_test.go` | Planned regression: live second same-window fallback returns already-captured acknowledgement | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Dedup store, bucket calculation, per-user scope, and already-captured acknowledgement satisfy SCN-074-A03, SCN-074-A04, and SCN-074-A05.
- [ ] TP-074-08 through TP-074-11 pass with evidence.
- [ ] Tests use isolated users/fixtures and do not mutate persistent dev state.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime or test commands.

---

## Scope 4: Trigger Execution And Abandoned Clarification

**Status:** Not Started  
**Depends On:** Scope 3  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-074-A01 — Unrouted turn produces exactly one fallback Idea
  Given a user turn that no scenario claims and open-knowledge cannot ground
  When the facade processes the turn
  Then exactly one Idea artifact is created with provenance = "capture-as-fallback"
  And the acknowledgement returned to the user is the canonical "saved-as-idea" shape

Scenario: SCN-074-A06 — Abandoned clarification captures the original prompt
  Given the spec 068 compiler issues a clarify prompt
  And the user does not respond within capture_as_fallback.clarify_abandon_timeout
  When the facade times out the clarification
  Then exactly one Idea artifact is created from the ORIGINAL prompt with provenance = "capture-as-fallback" and abandoned_clarification = true
  And the cause label on the capture_as_fallback_total counter is "clarify_abandoned"
```

### Implementation Plan

- Wire facade/open-knowledge/compiler failure points to `Policy.Decide` and `Policy.Capture` for eligible turns.
- Preserve original prompt text for clarify-abandon timeout decisions and write `abandoned_clarification=true` metadata.
- Ensure exactly one capture write or dedup result per fallback decision, with observable capture failure handling.
- Ensure confirm/in-flight clarification states that are not eligible do not route into fallback capture.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Assistant facade | Eligible unrouted/no-ground turn must capture exactly once | TP-074-12 integration row |
| Compiler clarification state | Abandoned clarify captures original prompt, not clarify answer text | TP-074-13 row |
| Capture writer | Capture failure must be observable and not reported as success | TP-074-14 e2e row |

### Change Boundary

- **Allowed file families:** assistant facade fallback hook, open-knowledge no-ground integration, compiler clarification timeout integration, capturefallback policy/store tests.
- **Excluded surfaces:** scenario selection logic unrelated to fallback eligibility, transport renderers, explicit capture UX, topic/tag extraction.
- **Containment rule:** no code path may interpret or classify the content during capture.

### Impact-Aware Validation

No project impact map is configured. Trigger execution requires integration and live e2e-api validation because it mutates artifact state.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-074-12 | SCN-074-A01 | integration | `tests/integration/assistant/capture_fallback_policy_test.go` | Planned: unrouted/no-ground turn creates exactly one fallback Idea | `./smackerel.sh test integration` | Yes |
| TP-074-13 | SCN-074-A06 | integration | `tests/integration/assistant/clarify_abandon_capture_test.go` | Planned: abandoned clarification captures original prompt with flag and cause | `./smackerel.sh test integration` | Yes |
| TP-074-14 | SCN-074-A01 | e2e-api | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` | Planned regression: live fallback-eligible turn returns saved-as-idea acknowledgement and one artifact | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Facade/open-knowledge/compiler hooks satisfy SCN-074-A01 and SCN-074-A06.
- [ ] TP-074-12 through TP-074-14 pass with evidence.
- [ ] Shared Infrastructure Impact Sweep confirms exactly one capture write/dedup result per fallback decision.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime or test commands.

---

## Scope 5: Telemetry, IntentTrace Link, And Cross-Transport Acknowledgement

**Status:** Not Started  
**Depends On:** Scope 4  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-074-A07 — Counter and IntentTrace carry the capture link
  Given a fallback capture occurs with cause = "open_knowledge_no_ground"
  When telemetry is inspected
  Then capture_as_fallback_total{cause="open_knowledge_no_ground"} increments by 1
  And the IntentTrace (spec 071) for that turn carries idea_artifact_id pointing to the produced artifact

Scenario: SCN-074-A11 — Acknowledgement shape is identical across transports
  Given the facade returns AssistantResponse with CaptureRoute = true
  When Telegram, HTTP-test, WhatsApp, web, iPhone/iOS, and Android render the response
  Then the "saved-as-idea" acknowledgement carries the same shape and copy on every transport
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User/Operator Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-074-A07 | Capture-as-Fallback Telemetry | fallback capture occurred | operator filters by cause | counter increments and trace links to Idea artifact id | TP-074-15 |
| SCN-074-A11 | Telegram/HTTP/WhatsApp/Web/iPhone+iOS/Android | `CaptureRoute=true` response | transport renderer displays acknowledgement | same saved-as-idea shape/copy appears across transports | TP-074-17 |

### Implementation Plan

- Emit `smackerel_capture_as_fallback_total{cause,outcome}`, dedup, latency, and provenance metrics with closed labels.
- Populate IntentTrace `capture_cause`, `idea_artifact_id`, and `final_response_status` when fallback capture occurs.
- Add dashboard/query tests for cause breakdown and recent capture trace links.
- Add cross-transport renderer fixture tests for canonical saved-as-idea acknowledgement, including `already_captured` metadata.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| IntentTrace integration | Spec 071 trace carries capture link without owning capture policy | TP-074-15 integration row |
| Transport renderers | Saved-as-idea shape/copy stays canonical | TP-074-17 cross-transport row |
| Monitoring | Operator can distinguish fallback from explicit capture | TP-074-16 dashboard/metrics row |

### Change Boundary

- **Allowed file families:** capturefallback metrics, IntentTrace capture fields integration, renderer fixture tests, monitoring integration tests.
- **Excluded surfaces:** dashboard layout owned by other specs unless routed, transport-specific custom copy, new artifact lifecycle states.
- **Containment rule:** telemetry labels cannot include raw user text or raw user identifiers.

### Consumer Impact Sweep

| Consumer | Reference Surface | Validation |
|---|---|---|
| Spec 071 IntentTrace | `capture_cause`, `idea_artifact_id`, `final_response_status` | TP-074-15 |
| Spec 072/073 renderers | canonical `capture_ack` response shape | TP-074-17 |
| Monitoring stack | `capture_as_fallback_total{cause,outcome}` | TP-074-16 |

### Impact-Aware Validation

No configured impact/trace map exists. Because this scope touches trace export, transport renderers, and runtime telemetry, scenario-specific integration/e2e rows are mandatory.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-074-15 | SCN-074-A07 | integration | `tests/integration/assistant/capture_trace_join_test.go` | Planned: fallback counter increments and IntentTrace links produced Idea artifact id | `./smackerel.sh test integration` | Yes |
| TP-074-16 | SCN-074-A07 | integration | `tests/integration/monitoring/capture_fallback_dashboard_test.go` | Planned: telemetry query exposes cause breakdown without raw text | `./smackerel.sh test integration` | Yes |
| TP-074-17 | SCN-074-A11 | e2e-ui/e2e-api | `tests/e2e/assistant/capture_ack_cross_transport_test.go` | Planned regression: Telegram, HTTP, WhatsApp, web, iPhone/iOS, and Android render same saved-as-idea shape | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Telemetry, IntentTrace capture links, dashboard/query rows, and cross-transport acknowledgement parity satisfy SCN-074-A07 and SCN-074-A11.
- [ ] TP-074-15 through TP-074-17 pass with evidence.
- [ ] Consumer Impact Sweep confirms capture response and trace field references are updated across first-party consumers.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime, UI, or test commands.