# Spec: BUG-025-005 Cross-Source Response Validation Routing

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Problem Statement

The ML NATS dispatcher applies an artifact-centric response validator to the concept-centric `synthesis.crosssource` result. This creates false error telemetry for valid responses and, because validation errors are swallowed, permits invalid responses to publish and acknowledge. Operators cannot trust validation logs or assume that a published cross-source result satisfied its documented contract.

## Outcome Contract

**Intent:** Route every validated ML NATS response through the validator for its actual subject contract, with cross-source concept responses validated before publication and invalid responses handed to the existing poison/retry mechanism.

**Success Signal:** Dispatching a valid documented `CrossSourceResponse` publishes exactly once and acknowledges without an artifact-schema error; dispatching each malformed cross-source permutation publishes zero responses, acknowledges zero times, and invokes poison handling once. Artifact responses still reject missing `artifact_id`, while digest, photo, and unknown-subject semantics remain unchanged.

**Hard Constraints:** Validation occurs before publish; cross-source validation requires a non-empty string `concept_id`, a real boolean `has_genuine_connection`, finite numeric `confidence` in `[0.0, 1.0]`, a list containing at least two non-empty string `artifact_ids`, a non-empty string `prompt_contract_version`, a non-negative finite numeric `processing_time_ms`, and a string `model_used`. Artifact validation is not weakened. Digest remains exempt from the artifact validator. Photo validation remains delegated to the photo contract. Unknown subjects remain acknowledged without publication. No deploy or config change is included.

**Failure Condition:** The change is a failure if a valid concept response still emits `artifact_id is required`, if any malformed validated response can publish or acknowledge, if artifact responses no longer require `artifact_id`, or if digest/photo/unknown subject behavior changes.

## Capability Framing

- Existing capability: ML NATS response validation and JetStream poison handling.
- Repair: explicit response-validator routing for the existing cross-source response contract plus fail-loud propagation from validation to poison handling.
- Foundation decision: use a small closed routing helper/table only if it keeps all current semantics explicit; do not introduce a framework or generic schema engine.
- Consumers: the existing `synthesis.crosssource.result` publisher and core synthesis subscriber.

## Requirements

- **FR-01:** A valid `synthesis.crosssource` response MUST be validated as `CrossSourceResponse`, published once to `synthesis.crosssource.result`, and acknowledged once.
- **FR-02:** Cross-source validation MUST reject a missing/empty/non-string `concept_id`.
- **FR-03:** Cross-source validation MUST reject a non-boolean `has_genuine_connection`; Python truthy integers MUST NOT count as booleans.
- **FR-04:** Cross-source validation MUST reject non-numeric, boolean, NaN, infinite, or out-of-range `confidence` values.
- **FR-05:** Cross-source validation MUST reject non-list `artifact_ids`, fewer than two IDs, empty IDs, and non-string IDs.
- **FR-06:** Cross-source validation MUST require a non-empty string `prompt_contract_version`, non-negative finite numeric `processing_time_ms`, and string `model_used`; `insight_text` MUST be a string.
- **FR-07:** Any outgoing validation failure MUST reach existing poison/retry handling before publish and before acknowledgement.
- **FR-08:** Artifact-centric outgoing validation MUST continue requiring non-empty `artifact_id`.
- **FR-09:** Digest validation behavior MUST remain unchanged.
- **FR-10:** Photo validation behavior MUST remain unchanged.
- **FR-11:** Unknown subjects MUST retain their current acknowledge-without-publish behavior.

## Gherkin Scenarios

```gherkin
Scenario: Valid concept response follows the cross-source validator
  Given a synthesis.crosssource message whose handler returns the documented CrossSourceResponse shape
  When the actual NATS consumer dispatches and validates the result
  Then the result is published once to synthesis.crosssource.result
  And the input message is acknowledged once
  And poison handling is not invoked
  And artifact_id is not required

Scenario: Malformed concept response enters poison handling before publish
  Given a synthesis.crosssource result with a missing concept_id, wrong field type, invalid confidence, or malformed artifact_ids
  When the actual NATS consumer dispatches and validates the result
  Then no result is published
  And the input message is not acknowledged
  And poison handling is invoked once with a PayloadValidationError

Scenario: Neighboring subject semantics remain intact
  Given artifact, digest, photo, and unknown subjects
  When each follows the outgoing dispatch boundary
  Then artifact results still require artifact_id
  And digest remains exempt from artifact validation
  And photo remains governed by validate_photo_result
  And an unknown subject is acknowledged without publication
```

## Acceptance Criteria

- **AC-01:** A pre-fix dispatch regression fails because the current generic validator is selected for a valid cross-source response.
- **AC-02:** The same dispatch regression passes after the fix and proves publish/ack/no-poison behavior.
- **AC-03:** Adversarial permutations for missing `concept_id`, wrong types, NaN/infinite/out-of-range confidence, and malformed `artifact_ids` prove zero publish and poison invocation.
- **AC-04:** Artifact, digest, photo, and unknown-subject regression cases pass without semantic changes.
- **AC-05:** `./smackerel.sh test unit --python`, relevant integration/live synthetic checks, lint, and format checks pass with zero warnings.
- **AC-06:** Artifact lint, traceability, regression quality, implementation reality, and state-transition checks execute with truthful evidence.

## Product Principle Alignment

This bug supports Principle 8, Trust Through Transparency: validation telemetry must describe real contract failures, and invalid synthesized output must never masquerade as validated output. It also preserves Principle 4, Source-Qualified Processing, because artifact provenance identifiers remain structurally required on cross-source connections.

## Non-Goals

- Changing the CrossSourceResponse wire shape.
- Changing confidence thresholds used by the core subscriber to create edges.
- Rewriting all NATS response schemas.
- Changing digest, photo, or unknown-subject behavior.
- Deploying the fix.