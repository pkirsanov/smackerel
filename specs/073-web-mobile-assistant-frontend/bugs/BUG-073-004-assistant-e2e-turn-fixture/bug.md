# BUG-073-004 - Transport-hint parity E2E uses reset as a normal turn

**Status:** Confirmed - reproduction and fix in progress
**Severity:** High - assistant package broad E2E is red and blocks synthesis closeout
**Spec:** 073-web-mobile-assistant-frontend
**Discovered:** 2026-07-19 during serialized broad `./smackerel.sh test e2e`

## Summary

The transport-hint parity E2E sends `Text: "/reset"` while claiming to prove
ordinary assistant response parity. `/reset` is a stateful capability command
that deletes the current `(user_id, transport)` conversation and returns a
reset acknowledgement before an agent invocation is created. Sequential reset
responses therefore do not prove that `transport_hint` leaves a normal
scenario response unchanged. The HTTP adapter also canonically returns
`transport="web"`; `transport_hint="mobile"` is telemetry metadata and must
not rewrite that field.

The PWA retry finding was initially grouped here because it used the same stale
`/reset` fixture. A deterministic ordinary-weather RED proved a separate
production HTTP dedup defect. That finding is owned by
`specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup`.

## Reproduction

Run from the isolated worktree with no concurrent Smackerel test process or
`smackerel-test` Compose resources:

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantTransportHintParity_WebAndMobileShareResponseShape$'
```

The RED sentinel must reject the reset acknowledgement as an invalid parity
fixture even though the old shape-only comparison passed.

## Expected Behavior

- Transport-hint parity uses a real, ordinary text turn and compares only
  contract-relevant, non-volatile response fields.
- Both `web` and `mobile` hints are accepted, remain telemetry-only, and the
  response transport remains the canonical HTTP transport `web`.
- Any shared HTTP identity conversation state is snapshotted and restored by
  exact `(user_id, transport)` key. Tests never update or delete all rows.

## Actual Behavior

The parity test uses `/reset`, a state-mutating short circuit, as if it were a
scenario-neutral ordinary turn. Before the RED sentinel it passed while proving
only that two reset acknowledgements happened to match.

## Impact

The parity E2E can pass without exercising its intended contract and can
perturb neighboring tests that share the harness HTTP identity.

## Security And Privacy

The fix must preserve bearer/session privacy. Test isolation is limited to the
single SST-bound shared identity row and must restore the exact prior value on
success or failure. No global conversation-table mutation is permitted.
