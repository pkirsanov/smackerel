# User Validation: 075 Legacy-Surface Deprecation Telemetry & User Comms

## Checklist

- [x] Planning baseline reflects `spec.md` and `design.md` scenarios SCN-075-A01 through SCN-075-A11.
- [ ] Long-time user sees a one-time replacement notice and still gets the intended result when mapping is confident.
- [ ] Same user is not re-notified for the same retired command across transports or sessions.
- [ ] Operator can see window state, residual usage, threshold state, and alert state without raw user identifiers.
- [ ] Closed-window response is clear, includes `/help`, and does not invoke retired handlers.
- [ ] Observation report proves zero retired-handler invocations before final deletion work proceeds.

## Planning Note

This checklist is a user-acceptance scaffold. Items other than the planning baseline require implementation and validation evidence before a human reviewer marks them complete.