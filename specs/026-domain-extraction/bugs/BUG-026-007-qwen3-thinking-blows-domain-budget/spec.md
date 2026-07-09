# Spec: BUG-026-007 — Thinking-disabled structured-JSON extraction

Owning feature: `026-domain-extraction`. This bug spec defines the EXPECTED behavior the fix must
satisfy. It is a NO-DEFAULTS / fail-loud SST change (smackerel `smackerel-no-defaults` policy).

## Expected Behavior

1. **The structured-JSON extraction path MUST NOT emit a qwen3 hidden-reasoning block when SST
   disables it.** When `ML_STRUCTURED_EXTRACTION_THINKING=false`, every in-scope structured-JSON
   extraction call on the `LLM_MODEL` (qwen3) text path MUST send the native Ollama `think=false`
   request field, so qwen3 skips its `<think>…</think>` block and returns the JSON directly (~1s
   compute, inside the 30s domain budget). (The `/no_think` prompt token is INEFFECTIVE — qwen3's
   template ignores it; see bug.md → Mechanism Correction.)

2. **The switch is SST-owned and fail-loud (NO default).** The value comes from
   `services.ml.structured_extraction_thinking`, is emitted as `ML_STRUCTURED_EXTRACTION_THINKING`
   by `scripts/commands/config.sh`, and is validated at sidecar startup. A missing / empty /
   non-`true`/`false` value MUST fail loud (resolver raises `RuntimeError`; startup validation
   `sys.exit(1)`), never silently substitute a default.

3. **Value semantics.**
   - `false` → thinking DISABLED (native `think=false`) — the fix's default posture.
   - `true` → thinking ENABLED (request unchanged).
   - anything else / empty / unset → fail loud.

4. **The agent reasoning path is UNCHANGED.** `ml/app/agent.py::handle_invoke` MUST NOT receive a
   `think=false` even when `ML_STRUCTURED_EXTRACTION_THINKING=false`; the agent path keeps qwen3
   thinking (quality > latency). This is a hard scope boundary.

5. **Non-qwen safety.** The `think=false` mechanism MUST be a no-op on non-ollama providers (no
   `think` field set) and on non-thinking Ollama models (e.g. `gemma3:4b` ignore the field), so
   dev/test deployments are unaffected.

## Acceptance Criteria

- [ ] `resolve_structured_extraction_thinking()` returns `True` for `"true"`, `False` for
  `"false"`, and raises `RuntimeError` for unset / empty / whitespace / any other value.
- [ ] `apply_structured_extraction_thinking(completion_kwargs, "ollama")` with SST=`false` sets
  `completion_kwargs["think"] = False`; with SST=`true` leaves kwargs unchanged; with a non-ollama
  provider leaves kwargs unchanged regardless of SST.
- [ ] Each in-scope call site (domain, synthesis extract + crosssource, processor, search-rerank,
  card-categories, drive-classify), when driven with `ML_STRUCTURED_EXTRACTION_THINKING=false`,
  composes its `litellm.acompletion` request with `think=False`; the two formerly-legacy routes
  (search-rerank, drive-classify) resolve to the `ollama_chat/…` prefix.
- [ ] With `ML_STRUCTURED_EXTRACTION_THINKING=true`, the same call sites compose requests WITHOUT
  `think=False` (adversarial: proves the switch actually gates and is not hard-wired on).
- [ ] The agent path composes requests WITHOUT `think=False` even when SST=`false` (scope boundary).
- [ ] `_check_required_config` fails (`SystemExit`) when provider=ollama and
  `ML_STRUCTURED_EXTRACTION_THINKING` is missing, and rejects a non-`true`/`false` value.
- [ ] The full ml unit suite (`./smackerel.sh test unit --python`) passes with no regressions.

## Out of Scope

- Any build, deploy, host mutation, or push (the orchestrator drives the evo-x2 rebuild + signed
  redeploy + live verify).
- Live latency verification (only provable post-redeploy).
- Changing the agent reasoning path, the warmup, the digest, or the chat surface.
