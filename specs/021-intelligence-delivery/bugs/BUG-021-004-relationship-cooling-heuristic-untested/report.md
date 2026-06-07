# Report: BUG-021-004 — make the cooling heuristic explicit and test-guarded

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07

## Summary

The relationship-cooling alert heuristic was inline SQL magic numbers with no
test, and the code's threshold (`≥ 4 distinct interactions in the prior 90-day
window`) deviates from the spec's "≥ 1/week" shorthand. This fix extracts the
heuristic into named, documented constants + a testable
`relationshipCoolingAlertQuery()` builder (producing byte-for-byte identical SQL
— no behavior change), and adds a lock test so a threshold change cannot drift
silently. The spec/code threshold disagreement is surfaced as DI-021-004 for the
owner; the running alert behavior and the parent spec requirement text are NOT
changed.

## Root Cause

`ProduceRelationshipCoolingAlerts` embedded `> 30`, `INTERVAL '180 days'`,
`INTERVAL '90 days'`, `>= 4`, dedup `30 days`, `LIMIT 10` as inline literals with
no unit or integration coverage (the existing producer tests cover only pure-Go
helper math), so a threshold change would pass CI silently.

## Fix

Named constants + a `relationshipCoolingAlertQuery()` builder (integer constants
only — no injection), called by the producer. A lock test asserts the produced
query embeds the documented threshold fragments and that the constants back
them. No runtime behavior change.

## Test Evidence

### Lock test passes against the shipped heuristic

```
$ go test -v -count=1 -run 'TestRelationshipCoolingHeuristic' ./internal/intelligence/
=== RUN   TestRelationshipCoolingHeuristic_MatchesDocumentedContract
--- PASS: TestRelationshipCoolingHeuristic_MatchesDocumentedContract (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/intelligence    0.037s
```

### Adversarial drift — changing a threshold makes the lock test FAIL

```
$ sed -i 's/coolingMinPriorInteractions = 4/coolingMinPriorInteractions = 13/' internal/intelligence/alert_producers.go
$ go test -count=1 -run 'TestRelationshipCoolingHeuristic' ./internal/intelligence/
--- FAIL: TestRelationshipCoolingHeuristic_MatchesDocumentedContract (0.00s)
    alert_producers_test.go:311: cooling query missing the at least 4 distinct prior interactions contract — expected fragment ") >= 4" not found.
    alert_producers_test.go:320: cooling heuristic constants drifted from the documented BUG-021-004 contract: silence=30 priorStart=180 priorEnd=90 minInteractions=13 dedup=30 cap=10
FAIL
```

(the constant was then restored; the lock test returns to PASS — `ok ... internal/intelligence 0.022s`.)

## Code Diff Evidence

```
$ go build ./internal/intelligence/...
# BUILD=0
$ git diff --stat internal/intelligence/alert_producers.go
 internal/intelligence/alert_producers.go | 59 ++++++++++++++++++++++++++------
 1 file changed, 48 insertions(+), 11 deletions(-)
$ git status --short internal/intelligence/alert_producers_test.go
 M internal/intelligence/alert_producers_test.go
```

Files changed: `internal/intelligence/alert_producers.go` (constants + builder;
the producer calls the builder — identical SQL);
`internal/intelligence/alert_producers_test.go` (lock test + `strings` import).
No schema, no config, no behavior change.

### Validation Evidence

```
$ go build ./internal/intelligence/...
$ go test -count=1 ./internal/intelligence/
ok      github.com/smackerel/smackerel/internal/intelligence    0.047s
```

Build clean and the full `internal/intelligence` package passes with the lock
test included.

### Audit Evidence

```
$ git status --short internal/intelligence/
 M internal/intelligence/alert_producers.go
 M internal/intelligence/alert_producers_test.go
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration; the produced SQL is byte-for-byte the prior literal)
```

The diff is confined to the producer + its test. No migration, no
`.github/bubbles` framework files, and the parent spec 021 planning artifacts
are deliberately untouched (the threshold-direction decision is surfaced as
DI-021-004, not applied).

## Completion Statement

The cooling heuristic is now explicit (named constants + a query builder) and
guarded by a lock test that fails on any threshold drift (adversarial drift run
proven), with no change to the running alert behavior. The spec-vs-code
threshold disagreement is surfaced for the owner as DI-021-004 (Option A keep
the more-surfacing shipped value; Option B tighten toward "≥ 1/week"), with the
parent spec requirement text left for owner-directed refresh. Scope 1 DoD is
complete (9/9). BUG-021-004 is Done; DI-021-004 is open and routed to the owner.
