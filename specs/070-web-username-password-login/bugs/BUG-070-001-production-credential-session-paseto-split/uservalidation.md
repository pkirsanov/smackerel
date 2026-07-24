# User Validation: [BUG-070-001]

## Checklist

- [x] The final planning baseline preserves the operator-reported production login split and the expected one-login product-wide outcome; this validates packet fidelity only, not runtime repair.
- [x] The plan requires the unified production browser session before Assistant API/PWA acceptance or broader product-journey certification.
- [x] The plan requires real disposable-stack login through the visible form and browser cookie jar, with no interception, bearer injection, auth-state injection, or bailout.
- [x] The plan preserves distinct invalid, session-ended, access-denied, empty, degraded, logout-failed, and ordinary-error states.
- [x] The plan includes keyboard, screen-reader, 320px, 200% zoom, privacy, logout replay, and hostile safe-return acceptance.
- [x] The plan requires product-wide CSRF/Origin protection on every cookie-authenticated mutation family, returning 403 before mutation on forged or cross-origin requests (SCOPE-05).
- [x] The plan requires the daily-user and operator roles to share one login while enforcing the 2xx/403 route matrix, with no daily-user admin success (SCOPE-06).
- [x] The plan requires the single operator-owned global corpus to be grant-gated, with a leak-free 403 for ungranted reads and no tenant or per-user row-isolation claim (SCOPE-06).

Unchecked behavior items are not added by agents as fabricated acceptance. Runtime acceptance remains owned by later validation and the human operator.

## Goal

- Goal: sign in once with username and password and use every authorized Smackerel browser surface without another credential.
- Success signal: legacy and modern surfaces accept the same per-user HttpOnly session while shared-token fallback remains disabled.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Sign in with valid credentials | Operator reports accepted login and working legacy pages | Interpreted historical input in `report.md` | works |
| 2 | Open modern API-backed surfaces | Operator reports malformed-cookie rejection and broken Assistant/Connectors/Photos/model-admin | Interpreted historical input in `report.md` | broken |
| 3 | Re-use one repaired session everywhere | Planned in Scope 06; not observed in this invocation | No runtime evidence | not yet executed |

## Human Acceptance After Implementation

- Confirm one real username/password login reaches every permitted named surface without a second credential.
- Confirm expired/revoked sessions and logout remove protected content and return to the accessible recovery flow.
- Confirm no password, token, cookie value, claims body, or shared secret is visible in the browser UI, URLs, storage, responses, or console.
