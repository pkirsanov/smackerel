# Spec 044: Per-User Bearer Auth Foundation — User Validation

**Status:** in_progress (validation phase pending)

This file is a placeholder for user validation evidence. It will be populated after Scope 4 closure when the spec is ready for user acceptance.

## Acceptance Criteria

The user will accept this spec as complete when all 11 acceptance criteria from spec.md AC-1..AC-11 are met:

1. **AC-1** — In a `production` deployment with the per-user bearer-auth subsystem live, a previously enrolled user can call any authenticated endpoint with their per-user bearer token and receive a non-401 response. Per-request middleware path issues zero database queries.
2. **AC-2** — Calling `POST /v1/photos/{id}/reveal` with `actor_id` in the request body OR `X-Actor-Id` in the request header is rejected in `production`. Handler instead derives `actor_id` from the authenticated session.
3. **AC-3** — Calling the cloud-drive Connect flow with `owner_user_id` in the request body is rejected in `production`. Persisted `drive_oauth_states` and `drive_connections` rows record session-derived `owner_user_id`.
4. **AC-4** — Creating a user annotation in `production` records an `actor_source` value derived from the authenticated session — never the literal string `system` and never a body/header-supplied value.
5. **AC-5** — Rotating a user's token in `production` produces a new token; both old and new tokens authenticate inside the configured grace window; only the new token authenticates after the grace window elapses.
6. **AC-6** — Revoking a user's token in `production` causes the next authenticated request bearing that token (issued no later than the configured propagation budget after revocation) to be rejected with HTTP 401. Service restart is not required.
7. **AC-7** — Booting a `production` deployment with the per-user bearer-auth subsystem enabled and missing required SST configuration causes both `./smackerel.sh config generate --env production` and the runtime startup to fail loudly with an error naming each missing configuration key.
8. **AC-8** — Booting `./smackerel.sh up` in the dev profile at HEAD with no per-user bearer-auth configuration changes succeeds; a client calling authenticated endpoints with the existing `SMACKEREL_AUTH_TOKEN` (or relying on the empty-token dev bypass) authenticates exactly as it did at HEAD `f7001ab`.
9. **AC-9** — `bash .github/bubbles/scripts/artifact-lint.sh specs/044-per-user-bearer-auth` exits 0 once design.md, scopes.md, uservalidation.md, and report.md are authored.
10. **AC-10** — After spec close, the following carry the resolution:
    - `specs/040-cloud-photo-libraries/state.json` marks `MIT-040-S-008` resolved with a cross-reference to spec 044.
    - `specs/038-cloud-drives-integration/state.json` marks `MIT-038-S-003` resolved with a cross-reference to spec 044.
    - `specs/027-user-annotations/state.json` marks the actor-source segment of `MIT-027-TRACE-001` resolved with a cross-reference to spec 044.
11. **AC-11** — `grep -rEn 'X-Actor-Id|actor_id_in_body_forbidden|"actor_id"' internal/` after spec close shows that the only remaining matches in `production`-applicable code paths are dev/test fallbacks explicitly gated by `cfg.Environment != "production"`, OR are removed entirely. No production-applicable header-trust or body-trust paths remain for `actor_id`.

## Checklist

- [x] Spec 044 artifacts exist (spec.md, design.md, scopes.md, scenario-manifest.json, report.md, uservalidation.md, state.json)
- [ ] All 11 acceptance criteria above are met and demonstrated
- [ ] Spec 040/038/027 trace-guards return PASSED after MIT closure routing
- [ ] All test suites (check, format, lint, unit, integration, e2e, stress) report EXIT 0
- [ ] Adversarial regression test `TestMintReveal_BodyActorIDInProduction_Returns400_FailsLoudly` passes
- [ ] AC-11 grep guard `TestNoBodyHeaderActorIDInProductionHandlers` returns ZERO production-applicable header-trust paths
- [ ] User has manually run a fresh production deployment through the bootstrap → enrollment → rotation → revocation workflow and confirmed all five workflows succeed
- [ ] User has confirmed dev/test deployments at HEAD continue to work without configuration changes (FR-AUTH-015 backward compat)

## Sign-Off

Pending Scope 4 completion.
