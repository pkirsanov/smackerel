# User Validation: [BUG-039-005]

## Checklist

- [x] Planning fidelity baseline: the eight-scope plan preserves provider-backed readiness, explicit required/optional behavior, independent Google and Yelp production adapters, typed outcomes, watch refusal, privacy, accessibility, migration/rollback, and real provider-compatible validation; this is not runtime acceptance.
- [x] Fixture providers remain test-only and cannot satisfy production readiness or live validation.
- [x] Unavailable, degraded, no-match, filtered-empty, authentication, quota, timeout, malformed-response, and provider-error states remain distinct across API, request, watches, scheduler, and UI.
- [x] Every planned live browser journey uses the real validate stack without first-party request interception, canned internal responses, or bailout assertions.
- [x] Test Plan rows and test-evidence DoD items are paired one-for-one in every scope.

## Goal

- Goal: request recommendations or watches only when a real healthy provider can perform them.
- Success signal: healthy provider yields sourced output; otherwise the product is explicitly unavailable/degraded and refuses inert actions.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Open enabled capability | Reported mounted UI/routes | Interpreted input in `report.md` | unclear |
| 2 | Get result/watch | Reported no provider/result/watch | Interpreted input in `report.md` | broken |
| 3 | Receive truthful availability | Planned in Scopes 01-08; not observed | No runtime evidence | not yet executed |

## Human Acceptance After Implementation

- Confirm at least one real configured healthy production provider yields sourced results and operable watches.
- Confirm zero, fixture-only, wrong-category, stale, and all-unhealthy providers never mount ready actions or inert watches.
- Confirm no-match, filtered-empty, partial coverage, and typed failures remain visibly distinct and value-safe at desktop/mobile/assistive modes.
