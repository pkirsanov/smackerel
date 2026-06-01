# User Validation — Spec 069 Assistant HTTP Transport

Links: [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] Baseline checklist initialized for the planning packet.
- [x] Planned contract covers text turns, errors, disambiguation, confirmation, reset, capture, hints, and live-stack testing.
- [x] Scope ordering keeps adapter/schema foundation before auth, callbacks, reset/capture, and parity overlays.
- [x] Scope 1 owns the cross-spec HTTP-route e2e proof for spec 068 scenarios SCN-068-A01, SCN-068-A02, SCN-068-A03, SCN-068-A04, SCN-068-A05, SCN-068-A06, SCN-068-A07, and SCN-068-A09 (scenario IDs preserved verbatim); spec 069's `specDependsOn` already correctly includes spec 068, and no inverse dependency was added.
- [x] No owner-reported regression is recorded in this planning pass.
- [x] Rework scope SCOPE-1d added for finding F-069-ADAPTER-NOT-BOUND (wireAssistantFacade never binds *HTTPAdapter; live /api/assistant/turn returns 503); planned bind block + integration coverage resolves F074-04B-ASSISTANT-HTTP-LATE-BIND.

## Owner Sign-Off

Owner sign-off occurs after delivery evidence exists in [report.md](report.md).
