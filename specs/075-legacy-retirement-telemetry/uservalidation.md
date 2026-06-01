# User Validation: 075 Legacy-Surface Deprecation Telemetry & User Comms

## Checklist

- [x] Planning baseline reflects `spec.md` and `design.md` scenarios SCN-075-A01 through SCN-075-A11.
- [ ] Long-time user sees a one-time replacement notice and still gets the intended result when mapping is confident.
- [ ] Same user is not re-notified for the same retired command across transports or sessions.
- [ ] Operator can see window state, residual usage, threshold state, and alert state without raw user identifiers.
- [ ] Closed-window response is clear, includes `/help`, and does not invoke retired handlers.
- [ ] Observation report proves zero retired-handler invocations before final deletion work proceeds.
- [x] Rework planning adds SCOPE-075-06.2b 'Wire-Schema Notice Propagation' before SCOPE-075-06.3 covering schema/types/golden + PWA + Flutter shared-core codegen for an OPTIONAL additive `notice` field (v1-compatible; no schema_version bump). TP-075-09 re-targeted from Playwright (.spec.ts) to a Go e2e at tests/e2e/assistant/legacy_retirement_notice_test.go (pattern: photos_capability_banner Go counterpart) executed via ./smackerel.sh test e2e.

## Planning Note

This checklist is a user-acceptance scaffold. Items other than the planning baseline require implementation and validation evidence before a human reviewer marks them complete.