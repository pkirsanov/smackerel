# Bug Fix Design: BUG-026-008

## Root Cause Analysis

### Investigation Summary

`handle_extract` was traced from prompt loading through request composition, `dispatch_litellm`, JSON decoding, JSON Schema validation, and result enforcement. The only schema-invalid branch logs and returns immediately. The handler has no bounded correction state and no configuration value governing a retry.

### Root Cause

The implementation models extraction as a single-attempt transaction. It distinguishes LLM exceptions, malformed JSON, and schema-invalid JSON, but the schema-invalid branch is terminal even though it has all data needed for one targeted correction: the original context, invalid assistant response, validator error, and canonical schema.

### Impact Analysis

- Affected component: `smackerel-ml` synthesis extraction consumer.
- Affected data: artifacts whose first model response is parseable but omits or mis-shapes required synthesis semantics.
- Affected users: capture flows relying on synthesized concepts, entities, claims, and relationships.
- Core behavior: terminal failures remain failed and acknowledged; this design changes only whether one bounded repair is attempted first.

## Fix Design

### Solution Approach

1. Add the required SST key `services.ml.synthesis_schema_repair_attempts: 1`, generate `ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS`, and validate it at ML startup and use time.
2. Isolate LLM response extraction into a small internal result path that returns output text, model identity, and token count while preserving the existing `completion_kwargs` construction.
3. On the first parsed-but-schema-invalid response, clone the original messages and append the invalid assistant response plus a repair instruction containing only the validator error and canonical schema.
4. Dispatch once more using the same request profile. Reapply qwen3 thinking control and the resolved Ollama profile through the same dispatch path rather than rebuilding a reduced request.
5. Sum usage across both calls. Return the final parse/schema/LLM failure class if repair does not validate.
6. Emit a content-free repair-attempt log/metric class with artifact identity represented only by the existing non-content identifier policy.
7. Copy the request `trace_id` to every synthesis result without including it in model messages or logs.

### Failure Classification

| Stage | Returned class | Calls | Sensitive payload handling |
|---|---|---:|---|
| Initial LLM exception | `LLM call failed: <ExceptionType>` | 1 | Type only for new/changed paths |
| Initial malformed JSON | Existing invalid-JSON failure | 1 | Parser metadata only |
| Repair LLM exception | `LLM schema repair failed: <ExceptionType>` | 2 | Type only |
| Repair malformed JSON | `LLM schema repair returned invalid JSON` | 2 | Parser metadata only |
| Repair schema-invalid | Final schema-validation error | 2 | Validator and JSON path only; invalid model values excluded |
| Repair valid | Existing success result | 2 | No model text logged |

### Alternative Approaches Considered

1. Fill missing required arrays with empty values - rejected because required concepts and claims are semantic output; empty defaults would fabricate success and hide model failure.
2. Retry the original prompt unchanged - rejected because it gives the model no validator feedback and repeats the same failure mode.
3. Retry indefinitely - rejected because it creates unbounded latency/cost and obscures terminal status.
4. Move retry into core - rejected because ML owns the prompt/schema/model interaction and core should continue receiving one explicit synthesis outcome.

## Capability Foundation

### Single-Implementation Justification

The existing synthesis handler is already the capability foundation. This change adds one internal transition to its current extraction state machine; there is no second provider-specific implementation or shared cross-service contract to abstract.

## Concrete Implementations

### Synthesis schema repair transition

- Foundation contract used: existing prompt contract, `dispatch_litellm`, JSON Schema validation, Ollama profile, and structured-thinking controls.
- Implementation-specific behavior: one correction after parsed schema failure.

### Variation Axes

| Axis | Options | Ownership |
|---|---|---|
| Initial response state | valid, malformed JSON, schema-invalid, LLM exception | Existing synthesis handler |
| Repair terminal state | valid, malformed JSON, schema-invalid, LLM exception | New bounded transition |
| Provider profile | Ollama with profile, hosted provider without Ollama profile | Existing dispatch abstraction |

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|---|---|---|
| Explicit two-attempt state with accumulated accounting | Repeat the existing call inline | Inline duplication would lose or drift request-profile fields and token accounting |
| Fail-loud SST attempt count | Literal one-call retry | A hardcoded retry hides operational policy and violates Smackerel configuration SST |

## Change Boundary

Allowed runtime/config/test families:

- `ml/app/synthesis.py`
- `ml/app/main.py`
- `ml/app/metrics.py`
- `ml/tests/test_synthesis.py`
- `ml/tests/test_main.py`
- `ml/tests/test_ollama_keepalive.py`
- `ml/tests/conftest.py`
- `ml/tests/fixtures/card_rewards_missing_concepts.json`
- `internal/pipeline/synthesis_types.go`
- `internal/pipeline/synthesis_subscriber_test.go`
- `config/smackerel.yaml`
- `scripts/commands/config.sh`
- `docs/Development.md`
- generated-env contract tests already owned by those surfaces
- this bug packet

Excluded surfaces:

- core synthesis status and acknowledgement logic
- prompt schema required-field definitions
- malformed-JSON degraded-capture implementation
- qwen3 thinking helper and model-memory profile implementation
- deploy adapters, manifests, secrets, host configuration, and release-train bundles

## Shared Infrastructure Impact Sweep

`scripts/commands/config.sh` is high-fan-out generated configuration. The change is additive: one required scalar, one generated env line, and startup validation. Existing config-sync/check tests are the canary before broader suites. Rollback removes the one scalar projection and resolver together; no generated file is hand-edited.

## Consumer Impact Sweep

The new environment key is consumed only by ML startup and synthesis extraction. Generated dev/test/deploy env surfaces must all contain it. The existing NATS synthesis request `trace_id` is now copied additively to the ML response and accepted by the Go response DTO; no API, database schema, frontend contract, or core status/acknowledgement logic changes.
