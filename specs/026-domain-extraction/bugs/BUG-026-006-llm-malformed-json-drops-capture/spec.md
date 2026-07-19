# Spec: BUG-026-006 — Malformed / empty LLM JSON must preserve the capture

Owning feature: `026-domain-extraction` (owns the "Invalid JSON from LLM" ML-processing contract).
This bug spec defines the EXPECTED behavior the fix must satisfy. It is a resilience change on the
ML sidecar's universal-processing path plus a NO-DEFAULTS / fail-loud SST posture (smackerel
`smackerel-no-defaults` policy) — an unparseable LLM response MUST NOT silently drop the user's
capture.

## Expected Behavior

1. **An unparseable LLM payload preserves the capture under the SST gate.** When the LLM returns a
   malformed / truncated JSON payload (the observed `"Invalid JSON from LLM: Unterminated string"`
   from an overrun output budget) AND `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=true`, the ML
   sidecar's `process_content` MUST degrade gracefully — return `success: True` with a low-signal
   fallback result (`topics:["degraded-fallback-malformed-json"]`, title derived from the raw
   content, `source_quality:"low"`) so the capture is preserved and searchable, NOT hard-dropped.

2. **Prose-wrapped JSON is salvaged.** A valid JSON object wrapped in a prose preamble/suffix
   (which litellm's `response_format={"type":"json_object"}` does not suppress for every
   Ollama-served model) MUST be salvaged — the widest `{…}` span is extracted and parsed with its
   real fields — instead of being fed whole to `json.loads` and hard-failing.

3. **An EMPTY / None LLM content preserves the capture on the SAME path.** Some Ollama-served models
   return `content=None` (or an all-whitespace string) on an overrun / aborted generation. That is
   as unrecoverable as a truncated payload and MUST route through the SAME degraded-fallback branch:
   under `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=true` it preserves the capture
   (`topics:["degraded-fallback-malformed-json"]`); it MUST NOT raise a `TypeError` that bypasses
   that branch and hard-drops the capture via the generic exception handler.

4. **The SST gate is fail-loud and gates both directions (NO silent success).** When
   `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=false`, an unparseable / empty / None payload MUST
   return a hard error classified as `Invalid JSON from LLM: …` (never `success: True`, never the
   opaque generic `LLM processing failed`). A missing / non-`true`/`false` value for the gate MUST
   be treated as disabled (hard error), never a silent degraded success.

5. **The output-token budget is SST-owned (no hardcoded magic number).** The
   universal-processing `max_tokens` MUST be read from the SST
   (`services.ml.domain_output_token_budget` → `ML_DOMAIN_OUTPUT_TOKEN_BUDGET`, fail-loud, no
   default), NOT the historical hardcoded `2000` that could truncate a rich-schema JSON object
   mid-object and manufacture the very malformed payload this bug preserves against.

## Acceptance Criteria

- [ ] `_parse_llm_json(text)` strict-parses valid JSON, salvages the widest `{…}` span from a
  prose-wrapped payload, and re-raises `json.JSONDecodeError` for a truncated payload (no closing
  brace to salvage).
- [ ] `_parse_llm_json(None)` and `_parse_llm_json("   ")` raise `json.JSONDecodeError` (NOT
  `TypeError`), so an empty/None payload routes through the same capture-preserving branch.
- [ ] With `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=true`, a truncated payload, a prose-wrapped
  payload, and a None payload each yield `success: True` with the capture preserved (the malformed
  and None cases tagged `topics:["degraded-fallback-malformed-json"]`).
- [ ] With `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=false`, a truncated payload AND a None payload
  each yield `success: False` with `Invalid JSON` in the error (adversarial: proves the switch
  actually gates and does not silently succeed).
- [ ] The universal-processing `max_tokens` is `resolve_domain_output_token_budget()` (SST-owned),
  and a distinct non-2000 SST value flows through to the litellm request (adversarial: a re-hardcoded
  `2000` fails the test).
- [ ] The full ml unit suite (`./smackerel.sh test unit --python`) passes with no regressions.

## Out of Scope

- Any build, deploy, host mutation, or push (the orchestrator drives the `<deploy-host>` rebuild +
  signed redeploy so the resilience fix reaches the running image).
- The live model-quality / latency root cause (`gemma4:26b` emitting truncated JSON and 71–95s
  synthesis / >30s domain-extraction under light host load). That truncation-and-latency root cause
  is a model-selection / output-budget ops call (routed to bubbles.devops, R-102-D); the in-repo
  deliverable is the capture-preservation resilience + the SST-owned output budget, not the live
  latency measurement.
- Changing the retry policy (the salvage + degraded-fallback approach is the declared fix; no
  JSON-retry loop is added).
