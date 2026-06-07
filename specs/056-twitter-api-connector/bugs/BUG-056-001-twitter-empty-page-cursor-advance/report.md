# Report: BUG-056-001 — anchor the resume cursor to the last non-empty page

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07

## Summary

`fetchEndpointPaginated` advanced the persisted resume cursor
(`lastNonEmptyToken`) on every page carrying a `next_token`, including empty
non-terminal pages, so a sparse Twitter v2 result pushed the cursor past the
last page with data. The fix advances `lastNonEmptyToken` only when the page
actually carried data, honoring the variable's name and the existing
`TestTwitterAPI_ReplayPagination` contract. No data loss, no infinite loop, no
change to rate-limit/dedup/restart behavior.

## Root Cause

The loop ran `lastNonEmptyToken = body.Meta.NextToken` unconditionally after the
terminal check, so an empty page (`len(body.Data) == 0`) that still carried a
`next_token` advanced the persisted resume token. The existing tests only
covered the empty TERMINAL page (no `next_token`), so the case was uncaught.

## Fix

A guarded assignment in `internal/connector/twitter/api.go`:

<!-- bubbles:evidence-legitimacy-skip-begin -->
```
if len(body.Data) > 0 {
    lastNonEmptyToken = body.Meta.NextToken
}
cursor = body.Meta.NextToken
```
<!-- bubbles:evidence-legitimacy-skip-end -->

## Test Evidence

### RED — pre-fix loop advances the cursor to the empty page's token

```
$ go test -count=1 -run 'TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor' ./internal/connector/twitter/
--- FAIL: TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor (0.03s)
    api_test.go:459: final cursor must stay anchored to PAGE2_TOKEN (last non-empty page);
    an empty non-terminal page advanced it to "PAGE3_TOKEN"
FAIL    github.com/smackerel/smackerel/internal/connector/twitter       0.053s
```

### GREEN — new + existing pagination tests pass against the fix

```
$ go test -v -count=1 -run 'TestTwitterAPI_(EmptyNonTerminalPageDoesNotAdvanceCursor|ReplayPagination|BookmarksPaginatesAndPersistsCursor)' ./internal/connector/twitter/
=== RUN   TestTwitterAPI_BookmarksPaginatesAndPersistsCursor
=== RUN   TestTwitterAPI_ReplayPagination
=== RUN   TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor
--- PASS: TestTwitterAPI_BookmarksPaginatesAndPersistsCursor (0.03s)
--- PASS: TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor (0.04s)
--- PASS: TestTwitterAPI_ReplayPagination (0.05s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.076s
```

### Broader regression — full twitter package green

```
$ go test -count=1 ./internal/connector/twitter/
ok      github.com/smackerel/smackerel/internal/connector/twitter       4.052s
PASS (no FAIL lines across the package; pagination, rate-limit, dedup, restart suites included)
```

## Code Diff Evidence

```
$ go build ./internal/connector/twitter/...
# BUILD=0
$ go vet ./internal/connector/twitter/
# VET=0
$ git diff --stat internal/connector/twitter/api.go
 internal/connector/twitter/api.go | 11 ++++++++++-
 1 file changed, 10 insertions(+), 1 deletion(-)
```

Files changed: `internal/connector/twitter/api.go` (guarded cursor advance +
comment); `internal/connector/twitter/api_test.go` (new adversarial
regression). No schema, config, or consumer change.

### Validation Evidence

```
$ go build ./internal/connector/twitter/...
$ go vet ./internal/connector/twitter/
$ go test -count=1 ./internal/connector/twitter/
ok      github.com/smackerel/smackerel/internal/connector/twitter       4.052s
```

Build clean, vet clean, full package green with the new test.

### Audit Evidence

```
$ git status --short internal/connector/twitter/
 M internal/connector/twitter/api.go
 M internal/connector/twitter/api_test.go
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration; diff confined to the connector + its test)
```

Diff is confined to the twitter connector and its test. No migration, no
`.github/bubbles` framework files.

## Completion Statement

The persisted pagination cursor now stays anchored to the last page that
produced tweets; an empty non-terminal page no longer advances it past real
data. The regression test fails on a revert to the unguarded assignment. All
existing pagination contracts pass and the full `internal/connector/twitter`
package is green. Scope 1 DoD is complete (8/8). BUG-056-001 is Done.
