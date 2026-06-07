# Report: BUG-073-002 — single-flight, time-bounded, busy-aware web assistant client

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07

## Summary

The web assistant chat client had a submit race (overlapping turns with
different `transport_message_id`s), no request timeout (a hung endpoint froze
the composer), and no busy affordance. The fix adds an `inFlight` single-flight
guard, an `AbortController` request timeout, and a disabled-Send busy state,
and locks them with a Go source-contract guard (the page is auth-gated, so a
browser interaction test cannot reach the composer).

## Root Cause

`dispatchTurn` had no re-entry guard and the submit handler dispatched without
awaiting, so rapid submits created overlapping turns. `postTurn` fetched with no
`signal`. The Send button was never disabled.

## Fix

`inFlight` guard in `dispatchTurn` + the submit handler; `AbortController` +
`TURN_TIMEOUT_MS` in `postTurn`; `setComposerBusy` toggling the Send button and
`aria-busy`.

## Test Evidence

### Source-contract guard + adversarial twin pass against the fixed source

```
$ go test -v -count=1 -run 'TestWebAssistantRobustnessGuard' ./web/pwa/tests/
=== RUN   TestWebAssistantRobustnessGuard_BUG_073_002
--- PASS: TestWebAssistantRobustnessGuard_BUG_073_002 (0.00s)
=== RUN   TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002
--- PASS: TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002 (0.00s)
PASS
ok      github.com/smackerel/smackerel/web/pwa/tests    0.008s
```

The guard asserts: `new AbortController(`, `setTimeout(... .abort()`,
`signal: controller.signal`, `if (inFlight)` (>= 2 sites), `.disabled = busy`,
and `TURN_TIMEOUT_MS` (>= 2). The adversarial twin proves a regressed source
(no AbortController, no signal, no inFlight, no busy toggle) fails every
contract.

### Adversarial re-RED — stripping the wiring makes the guard FAIL

```
$ # remove inFlight guard + signal + AbortController from assistant.js, then:
$ go test -count=1 -run 'TestWebAssistantRobustnessGuard_BUG_073_002$' ./web/pwa/tests/
--- FAIL: TestWebAssistantRobustnessGuard_BUG_073_002 (0.00s)
    assistant_robustness_guard_test.go:66: assistant.js is missing the "AbortController construction" robustness wiring: pattern new\s+AbortController\s*\( matched 0 time(s), want >= 1
    assistant_robustness_guard_test.go:66: assistant.js is missing the "single-flight inFlight guard" robustness wiring: pattern if\s*\(\s*inFlight\s*\) matched 0 time(s), want >= 2
FAIL    github.com/smackerel/smackerel/web/pwa/tests    0.012s
```

(assistant.js was then restored; `node --check` clean.)

## Code Diff Evidence

```
$ node --check web/pwa/assistant.js
# node --check: OK
$ git diff --stat web/pwa/assistant.js
 web/pwa/assistant.js | 92 ++++++++++++++++++++++++++++++++++++++++++----------
 1 file changed, 74 insertions(+), 18 deletions(-)
$ git status --short web/pwa/tests/assistant_robustness_guard_test.go
?? web/pwa/tests/assistant_robustness_guard_test.go
```

Files changed: `web/pwa/assistant.js` (inFlight guard, AbortController timeout,
setComposerBusy, submit-handler guard); `web/pwa/tests/assistant_robustness_guard_test.go`
(new source-contract guard + adversarial twin). No server, config, or schema
change; mobile client untouched.

### Validation Evidence

```
$ node --check web/pwa/assistant.js
# (no output — syntax OK)
$ go test -count=1 ./web/pwa/tests/
ok      github.com/smackerel/smackerel/web/pwa/tests    1.377s
```

`node --check` is clean and the full `web/pwa/tests` Go package (storage guard,
codegen-drift guard, robustness guard) is green — no regression.

### Audit Evidence

```
$ git status --short web/pwa/ | grep -vE 'node_modules|playwright-report|test-results'
 M web/pwa/assistant.js
?? web/pwa/tests/assistant_robustness_guard_test.go
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration)
```

The diff is confined to the web client and its new guard. No migration, no
`.github/bubbles` framework files, no edits to the parent spec 073 planning
artifacts, no mobile-client change.

## Completion Statement

The web assistant client now dispatches a single turn at a time, bounds each
request with an AbortController timeout, and shows a disabled-Send busy state.
The Go source-contract guard locks all three mechanisms and fails on their
removal (adversarial re-RED proven); `node --check` is clean and the
`web/pwa/tests` package passes. Scope 1 DoD is complete (8/8). BUG-073-002 is
Done.
