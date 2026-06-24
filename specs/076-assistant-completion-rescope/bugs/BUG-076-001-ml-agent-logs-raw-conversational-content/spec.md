# Spec: BUG-076-001 — ML agent dispatcher MUST NOT log raw conversational content

## Expected behavior (what the code SHOULD do)

The ML sidecar per-turn dispatcher (`ml/app/agent.py` `handle_invoke`)
MAY emit diagnostic `INFO` logs to help operators debug schema
rejections and provider routing, but those logs MUST NOT contain raw
conversational content.

### Privacy contract (binds spec 076 Hard Constraint 6 + Principle 8)

```gherkin
Scenario: BUG-076-001-A01 — request diagnostic log carries no raw user turn text
  Given an agent.invoke request whose first user message contains arbitrary user text
  When handle_invoke runs and emits the "agent.invoke.request" INFO log
  Then the emitted log record MUST NOT contain the raw user message content
  And the log record MAY carry non-reversible metadata (trace_id, model, counts, message length, type)

Scenario: BUG-076-001-A02 — envelope diagnostic log carries no raw final answer
  Given the LLM returns a final answer containing arbitrary synthesized text
  When handle_invoke emits the "agent.invoke.envelope" INFO log
  Then the emitted log record MUST NOT contain the raw final-answer content
  And the log record MAY carry non-reversible metadata (trace_id, tool_calls_count, final length, final type)

Scenario: BUG-076-001-A03 — diagnostic logs still fire (redaction, not removal)
  Given a successful agent.invoke turn at INFO level
  When handle_invoke completes
  Then both the "agent.invoke.request" and "agent.invoke.envelope" diagnostic logs MUST still be emitted
  And each MUST carry the trace_id so operators retain correlation ability
```

### Consistency requirement

The Python redaction discipline MUST match the established Go agent
turn-log contract: the Go side logs `prompt_sha256` (a hash) and is
guarded by `TestAgentTurnLog_RedactsSecrets`. The Python diagnostic
logs MUST likewise carry only non-reversible metadata, never raw
content.

### Single-Capability Justification

This bug introduces no reusable capability, provider, adapter, strategy,
plugin, channel, driver, connector, or variant. It is a single,
self-contained privacy redaction of two diagnostic `INFO` log statements
in one function (`ml/app/agent.py` `handle_invoke`), whose sole mechanism
is non-reversible length+type log metadata. The word "provider" appears
only to record that the dispatcher's provider routing is UNCHANGED by the
fix — no new provider/adapter surface is created — so there is exactly one
implementation and no capability-foundation / concrete-implementation
split is warranted.

## Requirement-Mechanism Justifications

**Mechanism-Justification: HMAC — out-of-scope parent-spec telemetry axis; this bug uses a distinct length+type redaction mechanism.**

`design.md` and `bug.md` reference **HMAC** only to describe the project's
*existing* privacy discipline — the spec 076 telemetry "HMAC user buckets"
axis, where user identifiers are keyed-hash (HMAC) bucketed so raw user
ids never reach metric labels or dashboards. That telemetry HMAC axis is a
parent-spec (076) concern; `bubbles.security` verified it CLEAN during the
R25 review (see `state.json` completedPhaseClaims → security).

This bug neither implements, changes, nor depends on HMAC. Its change
boundary is the two diagnostic `logger.info` statements in
`ml/app/agent.py` `handle_invoke`, and its mechanism is deliberately
different: non-reversible **length + type** log metadata
(`first_user_msg_len`, `final_len`, `final_type`) replacing the raw `%r`
content previews. Length+type is the correct minimal mechanism for
log-content privacy here — the original diagnostic goal
(presence / emptiness / type for schema-rejection triage) needs only
shape, never raw content — so HMAC is intentionally not part of this fix.
