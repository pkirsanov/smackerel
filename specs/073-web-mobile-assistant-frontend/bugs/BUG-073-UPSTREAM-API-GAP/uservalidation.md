# User Validation — BUG-073-UPSTREAM-API-GAP (Tracking Bug)

**Status:** N/A — tracking bug; user-validation surface lives in upstream specs.

## Checklist

- [x] Tracking bug acknowledged (no own UX surface exists in this folder)
- [x] Upstream resolution shipped via spec 080 (knowledge-graph-public-api) commit `98c16290`, certifiedAt `2026-06-04T02:30:00Z`
- [x] Upstream resolution shipped via spec 027 (user-annotations) Scope 9 commit `e6ccdb2a`, certifiedAt `2026-06-03T21:33:19Z`
- [x] User-facing acceptance is owned by the parent feature spec [specs/073-web-mobile-assistant-frontend/uservalidation.md](../../uservalidation.md) (wiki/graph-browse surface), not by this bug folder

## Why no own user validation

This bug filed a *backend* dependency — JSON endpoints that the spec
073 frontend needed. End-user validation of the frontend surface
itself lives in spec 073's own `uservalidation.md`. End-user
validation of the JSON endpoints themselves lives in spec 080 and
spec 027 respective `uservalidation.md` files.
