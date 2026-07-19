# Bug Fix Design: BUG-025-005

## Root Cause Analysis

### Investigation Summary

The subject handler branch in `NATSClient._consume_loop` correctly calls `handle_crosssource`, whose result matches the parent feature's `CrossSourceResponse`. After handler completion, a second branch selects outgoing validation. It delegates photo responses to `validate_photo_result`, skips artifact validation for digest, and sends every other subject through `validate_processed_result`. That generic validator checks only `artifact_id`. Its exception is caught and logged locally, after which the consumer publishes and acknowledges.

### Root Cause

Handler routing and response-validator routing evolved independently. The dispatcher encodes three implicit schema families but has no explicit contract mapping for `synthesis.crosssource`. The generic fallback therefore mistakes a concept identifier for a missing artifact identifier. Local exception swallowing further converts contract enforcement into telemetry-only advice.

### Impact Analysis

- Affected components: Python ML NATS consumer and validation module.
- Affected contract: `synthesis.crosssource.result` / `CrossSourceResponse`.
- Affected data: valid responses are still published, but error logs and error-derived metrics are polluted; invalid responses can also be published.
- Affected operators: stabilization and observability consumers see false validation errors and cannot trust publish-time contract enforcement.
- Unaffected surfaces: core response schema, NATS subjects, database schema, HTTP/UI, config, deployment adapters.

## Fix Design

### Solution Approach

1. Add `validate_crosssource_result` beside the existing NATS payload validators.
2. Validate the documented concept response fields using strict Python type checks, explicitly excluding booleans from numeric fields and using `math.isfinite` for confidence/timing.
3. Add one small outgoing-validator routing boundary in `nats_client.py`: photo delegates to the existing photo validator; digest remains intentionally unvalidated here; cross-source delegates to `validate_crosssource_result`; all other currently validated subjects retain `validate_processed_result`.
4. Remove the local catch-and-log around outgoing validation. A `PayloadValidationError` then reaches the existing outer exception handler, which calls `_handle_poison` before any publish or acknowledgement.
5. Exercise the actual `_consume_loop` dispatch boundary with a one-message subscription harness, not only the validator in isolation.

### Contract Rules

| Field | Rule |
|-------|------|
| `concept_id` | non-empty string |
| `has_genuine_connection` | exact boolean |
| `insight_text` | string; empty is permitted for a non-genuine connection |
| `confidence` | numeric but not boolean; finite; inclusive range 0.0 through 1.0 |
| `artifact_ids` | list of at least two non-empty strings, matching the parent requirement that cross-source overlap spans 2+ source artifacts |
| `prompt_contract_version` | non-empty string |
| `processing_time_ms` | numeric but not boolean; finite; non-negative |
| `model_used` | string; empty remains permitted for the handler's documented contract-load/LLM-failure response |

### Failure Flow

```text
handler result
  -> subject-specific outgoing validator
     -> valid: publish response -> ack input
     -> invalid: raise PayloadValidationError
        -> outer consumer exception boundary
           -> _handle_poison(subject, msg, error)
              -> nak for retry or dead-letter+term at delivery exhaustion
```

### Alternative Approaches Considered

1. Add only `if subject == "synthesis.crosssource"` around the generic validator - rejected because it would still leave validation failure as log-and-publish and keep contract selection embedded in publish control flow.
2. Skip validation for cross-source like digest - rejected because it would remove enforcement from a business contract and violate the requested fail-loud behavior.
3. Introduce JSON Schema validation for every NATS response - rejected as disproportionate to a two-contract routing defect and likely to broaden blast radius.

## Design Proportionality

The repair adds one validator and one routing helper at the existing validation boundary. It does not add a new framework, dependency, schema language, transport, or runtime configuration. The change boundary is limited to two Python runtime files, focused Python tests, and this bug packet.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|----------|------------------------|--------------|
| Subject-specific routing helper | Inline cross-source `if` only | A helper makes contract selection directly testable and prevents the existing catch-and-publish control flow from hiding validation failure. |

## Testing Strategy

- Unit validator permutations in `ml/tests/test_validation.py` prove exact field semantics.
- Dispatch regression cases in `ml/tests/test_nats_client.py` invoke `_consume_loop`, prove red before implementation, and assert publish/ack/poison ordering outcomes.
- Neighbor regressions cover artifact, digest, photo, and unknown subjects.
- Full Python unit, applicable NATS integration/live synthetic, lint, format, and Bubbles gates provide broader evidence.

## Risks

- Removing swallowed validation errors may expose another subject whose valid response does not match its existing validator. The scope preserves current validator assignment and full Python tests must reveal such drift; any discovered contract mismatch must be dispositioned, not suppressed.
- `float("nan")` passes ordinary range comparisons; explicit finite checks are mandatory.