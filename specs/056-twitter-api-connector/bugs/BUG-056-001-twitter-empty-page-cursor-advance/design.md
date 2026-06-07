# Design: BUG-056-001

## Problem

`fetchEndpointPaginated` advanced `lastNonEmptyToken` on every page carrying a
`next_token`, including empty pages, so the persisted resume cursor could point
past the last page with data. See `bug.md` for the verified mechanism.

## Change

Guard the cursor advance on the page actually having data:

```
if len(body.Data) > 0 {
    lastNonEmptyToken = body.Meta.NextToken
}
cursor = body.Meta.NextToken
```

`cursor` (the in-loop walk pointer) still advances every page so pagination
proceeds; only the PERSISTED resume token (`lastNonEmptyToken`) is held back on
empty pages.

### Why this shape

- Honors the variable's documented intent and the existing
  `TestTwitterAPI_ReplayPagination` contract ("last non-empty next_token").
- Forward sync is unaffected: the next tick resumes by re-checking the region
  after the last data (conservative — catches late-arriving items) rather than
  skipping it.
- No data loss, no infinite loop (loop remains bounded; dedup unchanged).

## Schema / Blast Radius

- `internal/connector/twitter/api.go` — one guarded assignment + comment.
- `internal/connector/twitter/api_test.go` — new adversarial regression test.
- No schema, no config, no consumer change.

## Alternatives Considered

- **Persist the in-loop `cursor` instead of `lastNonEmptyToken`.** Rejected:
  that is the current (buggy) behavior on empty pages; it advances the resume
  point past real data.
