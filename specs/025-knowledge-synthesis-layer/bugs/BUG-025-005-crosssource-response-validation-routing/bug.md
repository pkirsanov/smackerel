# Bug: BUG-025-005 Cross-Source Response Validation Routing

## Summary

The ML NATS consumer routes valid `synthesis.crosssource` responses through the artifact-centric outgoing validator, logs a false `artifact_id is required` error, and publishes anyway; genuinely invalid cross-source responses are also logged and published instead of entering poison/retry handling.

## Severity

- [ ] Critical - System unusable, data loss
- [x] High - Contract-invalid messages can be published and valid messages pollute error telemetry
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status

- [ ] Reported
- [x] Confirmed (source path and accepted live reproduction facts)
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Start from Smackerel commit `4b28bb9f0c2cc3a48ab78aa04395ebe817c50864`.
2. Dispatch a `synthesis.crosssource` message through `NATSClient._consume_loop` whose handler returns the documented `CrossSourceResponse` shape with `concept_id`, `has_genuine_connection`, `insight_text`, `confidence`, `artifact_ids`, `prompt_contract_version`, `processing_time_ms`, and `model_used`.
3. Observe the outgoing validation branch call `validate_processed_result`, which requires `artifact_id`.
4. Observe the validation exception being caught locally and logged while the result is still published and the input message acknowledged.

## Expected Behavior

- `synthesis.crosssource` output is validated against the concept-centric `CrossSourceResponse` contract.
- Valid concept responses publish once and acknowledge once without poison handling or artifact-schema errors.
- Invalid concept responses do not publish or acknowledge; they raise into the existing JetStream poison/retry path.
- Artifact-centric subjects retain their `artifact_id` requirement.
- Digest, photo, and unknown-subject behavior remains unchanged.

## Actual Behavior

- Valid concept responses are passed to `validate_processed_result` and produce a false `artifact_id is required` error.
- The exception is swallowed by the outgoing-validation block, so malformed validated output can still publish and acknowledge.

## Environment

- Service: `smackerel-ml`
- Source commit: `4b28bb9f0c2cc3a48ab78aa04395ebe817c50864`
- Platform: Linux, Docker-only repository CLI
- Observed date: 2026-07-19

## Error Output

Accepted stabilization observation:

```text
subject=synthesis.crosssource
response_subject=synthesis.crosssource.result
response_shape=concept_id,has_genuine_connection,insight_text,confidence,artifact_ids,prompt_contract_version,processing_time_ms,model_used
selected_validator=validate_processed_result
validator_contract=artifact_id required
observed_log=Invalid outgoing result on synthesis.crosssource: artifact_id is required
observed_publish=continued
observed_ack=continued
impact=false validation error and invalid-output publication risk
```

Current-session command-backed red evidence belongs in `report.md` and must exist before implementation.

## Root Cause

`NATSClient._consume_loop` has subject-specific handlers but not subject-specific outgoing validators. Its post-handler branch special-cases photo and digest results, then sends every other response through `validate_processed_result`. The `synthesis.crosssource` handler correctly returns a concept-centric response with no `artifact_id`, so the generic artifact validator is the wrong contract. The same branch catches `PayloadValidationError` and only logs it, violating fail-loud validation by publishing and acknowledging afterward.

## Fix

`ml/app/validation.py` now validates the documented cross-source response fields strictly. `ml/app/nats_client.py` declares a closed outgoing-validation mode for every subscribed subject and routes cross-source results to the concept validator. Validated failures propagate to the existing poison handler before publication; the real retry branch calls `nak()`. Artifact, digest, photo, and unknown-subject behavior is covered by regression tests.

## Related

- Parent feature: `specs/025-knowledge-synthesis-layer/`
- Parent requirements: R-2505, R-2507; AC-12, AC-13, AC-15 through AC-17
- Runtime anchors: `ml/app/nats_client.py`, `ml/app/validation.py`, `ml/app/synthesis.py`
- Regression surface: `ml/tests/test_nats_client.py`, `ml/tests/test_validation.py`
- Related closed bugs: BUG-025-001 through BUG-025-004 address different defects

## Capability Framing

This repairs the existing ML NATS response-validation capability. It does not add a provider, strategy, plugin, transport, or second reusable implementation. A small subject-to-validator routing boundary is proportional because two distinct response contracts already exist on the same dispatcher and the current implicit fallback caused the defect.

## Deployment Boundary

No deployment occurs in this invocation. A pushed, certified-as-far-as-owned commit is routed to build, security/audit, and deployment owners after repository gates complete.