# BUG-056-001: empty non-terminal page advances the persisted pagination cursor past the last real data

**Status:** Resolved (cursor anchored to last non-empty page via bugfix-fastlane — see report.md)
**Severity:** Low
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Stochastic Quality Sweep Round 18 (parent: stochastic-quality-sweep) — `regression`, parent-expanded
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/056-twitter-api-connector/`
**Affected surface:** `internal/connector/twitter/api.go` (`fetchEndpointPaginated`)

## Summary

`fetchEndpointPaginated` returns a `lastNonEmptyToken` that the connector
persists as the per-endpoint resume cursor (`cursor.PerEndpoint[ep]` in
`internal/connector/twitter/twitter.go`). The loop updated
`lastNonEmptyToken` on EVERY page that carried a `next_token`, including a page
whose `data` array was empty. Twitter API v2 results can be sparse — an empty
page may still carry a `next_token` — so an empty non-terminal page pushed the
persisted resume cursor PAST the last page that actually produced tweets,
contradicting the variable's own name and the documented contract in
`TestTwitterAPI_ReplayPagination` ("persist the last non-empty next_token").

## Mechanism (verified by a red regression test at repo HEAD)

In the pagination loop:

```
tweets = append(tweets, body.Data...)
if body.Meta.NextToken == "" { return tweets, lastNonEmptyToken, nil }
lastNonEmptyToken = body.Meta.NextToken   // ← updated even when body.Data was empty
cursor = body.Meta.NextToken
```

For the fixture sequence page1(3 tweets, next=PAGE2) → page2(EMPTY, next=PAGE3)
→ page3(EMPTY, terminal), the loop set `lastNonEmptyToken = PAGE3_TOKEN` on the
empty page2, so the connector persisted `PAGE3_TOKEN` — a cursor past the last
real data — instead of `PAGE2_TOKEN`.

## Impact / Severity rationale (Low)

- **No data loss / no infinite loop:** the loop is bounded by
  `maxPagesPerEndpoint` and terminates on `next_token == ""`. The dedup layer
  (`seenPrimary`) also guards against re-import.
- **Resume-point drift only:** the next sync tick resumes from a cursor past the
  last data, so a late-arriving item in the skipped empty region could be
  missed until a later full pass.
- **Rare trigger:** Twitter v2 seldom emits an empty page with a `next_token`;
  the existing tests only covered the empty TERMINAL page (no `next_token`).

## Fix (delivered)

Only advance `lastNonEmptyToken` when the page actually carried data:

```
if len(body.Data) > 0 {
    lastNonEmptyToken = body.Meta.NextToken
}
cursor = body.Meta.NextToken
```

This honors the variable's name and the `TestTwitterAPI_ReplayPagination`
contract. The single-owner forward-sync, rate-limit, and dedup behaviors are
unchanged; the existing pagination tests still pass.

## Cross-References

- Keyer/loop: `internal/connector/twitter/api.go` (`fetchEndpointPaginated`)
- Consumer: `internal/connector/twitter/twitter.go` (`cursor.PerEndpoint[ep]`)
- Regression test: `internal/connector/twitter/api_test.go`
  (`TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor`)
- Sibling coverage preserved: `TestTwitterAPI_ReplayPagination`,
  `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor`
