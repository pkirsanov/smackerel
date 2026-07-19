# Spec: BUG-026-008 Bounded synthesis schema repair

Owning feature: `026-domain-extraction`.

## Outcome Contract

- **Intent:** Recover a synthesis extraction when the first LLM response is valid JSON but violates the committed prompt-contract schema, without fabricating missing semantic content or creating an unbounded retry loop.
- **Success Signal:** The real `handle_extract` path turns a first response missing required `concepts` followed by a valid corrected response into `success: true` with exactly two LLM dispatches, while every terminal repair failure remains an explicit `success: false` with the final error class.
- **Hard Constraints:** Retry budget is explicit, SST-owned, generated into the ML runtime contract, and fail-loud; exactly one repair call is permitted at the configured value; original artifact content/context and Ollama request profile are retained; token usage and processing duration cover both calls; logs and returned repair errors expose no artifact content, model output, credentials, or exception message; required concepts or claims are never synthesized as empty defaults; core status/acknowledgement semantics are unchanged; BUG-026-006 malformed-JSON capture preservation and BUG-026-007 thinking/token-budget behavior remain green.
- **Failure Condition:** The work fails if a schema-invalid first response remains permanently failed without its configured repair, more than one repair call can occur, missing required semantics are normalized to empty values, the repair loses request profile/context, accounting covers only one call, a terminal repair is reported as success, sensitive content appears in logs/errors, or either sibling regression returns.

## Product Principle Alignment

- **Principle 3 - Knowledge Breathes:** recoverable extraction shape drift receives one bounded correction instead of permanently freezing the artifact in failed synthesis.
- **Principle 5 - One Graph, Many Views:** repaired output must still pass the canonical extraction schema before entering the existing graph path.
- **Principle 8 - Trust Through Transparency:** terminal failures remain explicit and token/error accounting remains truthful.

## Expected Behavior

### First response is valid

When the initial response parses and validates, `handle_extract` performs one LLM call, applies existing validation rules, and returns the existing success shape unchanged.

### First response is parsed but schema-invalid

When the initial response parses as JSON but fails `extraction_schema` validation:

1. Resolve the schema-repair attempt budget from the generated ML environment contract with no fallback.
2. At the supported budget value of `1`, issue exactly one corrective call.
3. Preserve the original system prompt, artifact prompt, model/provider, temperature, output-token budget, response format, Ollama API base, keep-alive, context-window profile, and qwen3 thinking control.
4. Preserve conversational trace by including the first assistant response, then append a corrective user instruction containing the validator error and the original schema and requiring corrected JSON only.
5. Record one content-free repair-attempt metric/log event.
6. Sum token usage from the initial and repair responses and measure processing time across the entire operation.
7. Validate the corrected JSON through the same schema and validation-rule path as an initially valid response.

### Repair terminates truthfully

- A second schema-invalid response returns `success: false` with a schema-validation error class from the second response.
- Malformed repair JSON returns `success: false` with the repair JSON-decode error class.
- A repair LLM exception returns `success: false` with the exception type only; exception text and artifact/model content are not returned or logged.
- No third LLM call is permitted.

### Normalization boundary

Deterministic normalization may only initialize an optional array when the committed schema does not require it and doing so does not invent semantic content. Required `concepts`, required concept `claims`, required entities, and required relationships must never be fabricated as empty defaults. Missing required semantic content always uses the corrective model call.

## Configuration Contract

- `config/smackerel.yaml::services.ml.synthesis_schema_repair_attempts` is required and set to `1`.
- `scripts/commands/config.sh` emits `ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS` into generated dev/test/deploy environment contracts.
- ML startup and the handler resolver both reject missing, empty, non-integer, or non-`1` values. No code default is permitted.

## Acceptance Criteria

- [ ] A focused regression using the actual `handle_extract` and committed `ingest-synthesis-v1` schema proves the pre-fix path returns permanent failure after one call for missing `concepts`.
- [ ] First invalid then valid succeeds with exactly two calls and the corrected extraction result.
- [ ] Invalid twice fails after exactly two calls with the final schema error.
- [ ] Invalid then malformed repair JSON fails after exactly two calls with the repair decode class.
- [ ] Invalid then repair LLM exception fails after exactly two calls without exception-message or content leakage.
- [ ] Valid first response succeeds with exactly one call.
- [ ] Thinking control, keep-alive, context-window profile, response format, temperature, and output-token budget are retained on both calls.
- [ ] `tokens_used` is the sum of both response usage counts and `processing_time_ms` spans both calls.
- [ ] The request `trace_id` is returned unchanged on initial success, repaired success, and terminal failure, and is absent from model messages and logs.
- [ ] Repair observability records a content-free attempt class and no sensitive artifact/model/exception content.
- [ ] The malformed-JSON capture-preservation regression remains green.
- [ ] Missing required semantic fields are not normalized to empty arrays or objects.
- [ ] Core synthesis-failed and acknowledgement semantics remain explicit and unchanged for terminal failure.

## G094 Proportionality

### Single-Capability Justification

This is a narrow repair inside the existing synthesis-extraction capability and its existing prompt-contract, LLM-dispatch, schema-validation, and telemetry foundations. It adds no provider, adapter, screen, service, shared data model, or reusable plugin surface. A new capability foundation would duplicate the existing extraction abstraction and increase risk without reducing complexity.

## Deployment Boundary

This packet stops after source delivery, tests, security/audit checks, and validate-owned certification. It does not build, deploy, mutate a target, or claim live-runtime remediation. A later `bubbles.devops` operation consumes the pushed source through the normal signed build/deploy path.
