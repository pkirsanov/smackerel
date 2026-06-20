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
