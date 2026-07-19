# Bug: BUG-026-008 Synthesis schema failures are permanent after one LLM call

## Summary

`ml/app/synthesis.py::handle_extract` treats a parsed but schema-invalid LLM response as a permanent extraction failure after one model call. Live card-rewards captures repeatedly returned JSON without the required `concepts` property, so core marked synthesis failed and acknowledged the artifact without one bounded corrective attempt.

## Severity

- [ ] Critical - System unusable or data loss
- [x] High - A required knowledge-extraction path permanently fails without bounded recovery
- [ ] Medium - Feature broken with a reliable workaround
- [ ] Low - Minor issue

## Status

- [ ] Reported
- [x] Confirmed
- [ ] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Load the committed `ingest-synthesis-v1` prompt contract.
2. Invoke the real `handle_extract` function with a card-rewards artifact.
3. Make the external LLM boundary return parsed JSON containing `entities` and `relationships` but omitting required `concepts`.
4. Observe that `handle_extract` returns `success: false` after one LLM dispatch.
5. Observe that no bounded corrective request is made even though the original output is parseable and the validator identifies the exact missing required property.

## Expected Behavior

The first parsed-but-schema-invalid response triggers exactly one corrective LLM request under an explicit fail-loud SST retry budget. The repair request preserves the original artifact context and request profile, includes the validation error and original extraction schema, and asks for corrected JSON only. A valid repair continues through normal validation-rule enforcement. A failed repair returns truthful `success: false` with the final error class.

## Actual Behavior

`handle_extract` dispatches once, parses once, validates once, logs the schema error, and immediately returns `success: false`. Core therefore persists the synthesis failure and acknowledges the artifact without a repair opportunity.

## Environment

- Service: `smackerel-ml`
- Source baseline: `4b28bb9f0c2cc3a48ab78aa04395ebe817c50864`
- Runtime symptom: accepted runtime, card-rewards artifacts
- Platform: Linux container runtime with Ollama-backed structured extraction

## Error Output

```text
synthesis extraction failed ... Schema validation failed: 'concepts' is a required property
```

## Root Cause

The extraction handler has a terminal branch immediately after `validate_extraction`. It has no schema-repair state, no explicit retry budget, and no second dispatch path. The malformed-JSON and qwen3 request-profile fixes operate earlier in the response/request lifecycle and do not address parsed JSON that violates the extraction schema.

## Related

- Owning feature: `specs/026-domain-extraction/`
- Preserved malformed-JSON fix: `BUG-026-006-llm-malformed-json-drops-capture`
- Preserved qwen3 thinking fix: `BUG-026-007-qwen3-thinking-blows-domain-budget`
- Runtime owner after certification: `bubbles.devops` for build/deploy only
