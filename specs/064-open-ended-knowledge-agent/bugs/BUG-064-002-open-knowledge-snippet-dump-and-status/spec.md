# BUG-064-002 — Specification (expected behavior)

- **Spec:** `specs/064-open-ended-knowledge-agent`
- **Bug:** BUG-064-002
- **Severity:** S1

## Problem statement

For an open-ended factual question routed to the `open_knowledge` agent, the
user must receive ONE concise synthesized answer that directly answers the
question, followed by a short, deduplicated, capped citation list of the
sources actually used — with no in-flight `thinking…` header, no verbatim
snippet dumping, no triplicate repetition, and no absurd source count.

## Product Principle Alignment

- **Principle 2 (Vague In, Precise Out):** an open-ended NL question must yield a
  precise, synthesized answer, not a raw search-result preview.
- **Principle 4 (Source-Qualified Processing):** the answer is grounded in cited
  sources; the citation list reflects the sources actually used, deduplicated
  and capped.

## Requirements

### FR-1 — Single synthesized answer (DEFECT 1)
When the open-knowledge agent terminates `final`/`success`, the user-visible
body MUST be a synthesized answer, not a verbatim passthrough of raw
`web_search` snippet text. The agent's system prompt MUST instruct the model to
EXTRACT the specific data the user asked for and present a synthesized answer,
and MUST forbid pasting raw search snippets verbatim.

### FR-2 — No duplicated blocks in the assembled body (DEFECT 2)
The assembled final body MUST NOT contain the same snippet/answer block 2+
times. The snippet-salvage assembly (the last-resort path used when the model
fails to synthesize) MUST de-duplicate snippet text so each distinct snippet
appears at most once.

### FR-3 — Terminal status on a delivered answer (DEFECT 3a)
A completed `open_knowledge` answer (agent `OutcomeOK`) MUST carry a terminal,
user-facing success status — NOT `StatusThinking`. The Telegram adapter MUST
NOT render a `thinking…` header on a delivered open-knowledge answer.

### FR-4 — Capped, deduplicated source set (DEFECT 3b)
The open-knowledge agent MUST cap the number of sources it attaches to a
salvaged answer to the SST-configured `assistant.sources_max`, deduplicated
(no duplicate `(kind, locator, content_hash)`). When the model produces a real
cited synthesis, the attached sources remain the verified cited set.

### FR-5 — NO-DEFAULTS / fail-loud (SST)
Any configuration value the fix consumes (the source cap) MUST be sourced from
`config/smackerel.yaml` and be fail-loud (rejected when missing/invalid). No
`${VAR:-default}`, no silent in-code fallback. The agent's `New()` MUST reject
a non-positive source cap.

## Gherkin scenarios

### SCN-064-002-A01 — synthesized answer, not a snippet dump
```gherkin
Given the open-knowledge agent ran web_search tool calls that returned snippets
  And the model produced a grounded synthesized answer with a valid <CITATIONS> block
When the agent assembles the final TurnResult
Then the body is the model's synthesized answer (the cited synthesis)
  And the body is NOT the verbatim concatenation of the raw tool snippets
```

### SCN-064-002-A02 — no triplicate duplication in salvage
```gherkin
Given three web_search tool calls each returned the same top snippet "S"
  And the model failed to synthesize (empty forced-final text, or an ungrounded excuse)
  And the agent falls back to snippet-salvage
When the agent assembles the final body from the tool snippets
Then the snippet "S" appears exactly once in the body
  And the body does not contain "S" two or more times
```

### SCN-064-002-A03 — terminal status, not "thinking…"
```gherkin
Given the open_knowledge agent returned OutcomeOK with a sourced body
When the facade derives the user-visible status
Then the status is the terminal "answered" status, not "thinking"
  And the Telegram renderer emits no "thinking…" header line for the answer
```

### SCN-064-002-A04 — source set capped and deduplicated
```gherkin
Given the open-knowledge tool trace recorded N distinct web sources where N > sources_max
  And the model failed to cite, so the agent salvages trace sources
When the agent attaches sources to the TurnResult
Then the number of attached sources is at most sources_max
  And there are no duplicate sources
```

### SCN-064-002-A05 — fail-loud source cap
```gherkin
Given the open-knowledge agent is constructed with a non-positive source cap
When New() validates the config
Then construction fails with an explicit "SourcesMax must be > 0" error
  And no silent default is substituted
```

## Out of scope

- Swapping the local model (`gemma4:26b`). Per the operator directive, the fix
  is in-repo: prompt-contract redesign (extract-then-synthesize), the
  synthesis/answer-assembly de-duplication, source cap+dedup, and the
  status-label fix. If real data extraction remains limited by model capability,
  that limitation is documented honestly in `design.md` with evidence.
- The live redeploy itself (owner `bubbles.devops`), tracked as a follow-up.
