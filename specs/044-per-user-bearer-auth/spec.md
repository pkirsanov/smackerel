# Feature 044 — Per-User Bearer Auth Foundation

### Status

In Progress

## Problem Statement

Smackerel's API trust boundary today is a **single, shared bearer token**
(`SMACKEREL_AUTH_TOKEN`) configured per deployment. This is sufficient for the
single-tenant MVP posture documented in
[`specs/040-cloud-photo-libraries/state.json`](../040-cloud-photo-libraries/state.json),
but it forces every multi-user identity question to be answered the wrong way:

- **Trust boundary collapse.** Every authenticated request is "the deployment
  operator." There is no way to distinguish caller A from caller B at the API
  layer. `internal/api/router.go` `bearerAuthMiddleware` does a
  constant-time compare against one token and stops there
  (lines 444–471 at HEAD `f7001ab`).
- **Header-trust workarounds.** Handlers that need a per-user identity have
  resorted to client-controlled `X-Actor-Id` headers (post-MIT-040-S-003 partial
  closure at commit `4e399a4`,
  [`internal/api/photos_upload.go`](../../internal/api/photos_upload.go) lines
  246–321) or body-sourced `owner_user_id` fields
  ([`internal/drive/google/google.go`](../../internal/drive/google/google.go)
  per spec 038's MIT-038-S-003 routing). Both are documented
  [carry-forward](../040-cloud-photo-libraries/state.json) trust deferrals
  whose closure trigger is "once per-user bearer tokens / claim-binding land."
- **No real attribution surface.** Spec 027 user annotations record an
  `actor_source` segment (per MIT-027-TRACE-001) but the only thing that can
  be recorded in production today is the literal string `system` or whatever
  the client claims via header. Annotation provenance is therefore advisory,
  not authenticated.
- **Migration cliff.** Any future feature that needs per-user state (saved
  searches, personal lists, per-user lockdown approvals, per-user QF packets)
  inherits the same constraint and pushes the closure further out.

The MIT-040-S-008 routing entry at commit `4e399a4` is explicit:

> **NEW FEATURE SPEC required (foundation-level per-user authentication).**
> Carry-forward from MIT-040-S-003 partial-close — no current trigger; the
> per-user-auth feature spec must be filed by the user when ready.

This spec is that file. It scopes the **foundation-level authentication
contract**: per-user bearer-token issuance, stateless validation on the hot
path, claim-binding for `actor_id` derivation, rotation with a grace window,
revocation, and a non-disruptive migration path that keeps the existing
single-tenant `SMACKEREL_AUTH_TOKEN` valid for `development` and `test`
environments. It is deliberately scoped to the **trust boundary**, not the
enrollment UX (admin UI, self-service signup) or third-party-connector OAuth
(those remain separate concerns).

Closing this spec closes three documented mitigations in one foundation:

- **MIT-040-S-008** — Replace `X-Actor-Id` header source with
  `bearerAuthMiddleware`-extracted authenticated-session identity for photo
  reveal mints (carry-forward from MIT-040-S-003 partial closure).
- **MIT-038-S-003** — Replace cloud-drive `Connect` body-sourced
  `OwnerUserID` with session-derived owner identity.
- **MIT-027-TRACE-001** *actor-source segment portion* — Replace
  client-supplied or system-default `actor_source` on user annotations with
  authenticated-session identity. (MIT-027-TRACE-001 also covered DoD-trace
  coverage gaps that were already closed at iter 11; this spec only closes
  the residual actor-attribution segment.)

## Outcome Contract

**Intent:** Establish a per-user bearer-token trust boundary so that every
authenticated request to Smackerel in the `production` environment carries a
verifiable, claim-binding identity that handlers MUST derive `actor_id`,
`owner_user_id`, and `actor_source` from — eliminating the header-trust and
body-trust workarounds catalogued in MIT-040-S-008, MIT-038-S-003, and
MIT-027-TRACE-001's actor-source segment, while keeping the existing
single-tenant `SMACKEREL_AUTH_TOKEN` fully valid for `development` and
`test` environments.

**Success Signal:** A deployment whose
[`config/smackerel.yaml`](../../config/smackerel.yaml) sets
`runtime.environment: production` AND has at least one enrolled user can:

1. Issue a per-user bearer token through the documented enrollment flow.
2. Call any authenticated API endpoint with that token in `Authorization:
   Bearer <token>` and have the request authenticated stateless on the hot
   path (no DB roundtrip per request).
3. Observe that handlers downstream of `bearerAuthMiddleware` derive
   `actor_id` (photo reveal mint, user annotations, future per-user
   surfaces) and `owner_user_id` (cloud-drive Connect, future per-user
   surfaces) **exclusively** from the authenticated session context — with
   request-body and request-header sources for those identities rejected at
   the boundary.
4. Rotate that user's token without breaking active sessions for the
   configured grace window, and revoke a token that takes effect on the
   next authenticated request.
5. Run the existing `development` and `test` deployments unchanged with
   `SMACKEREL_AUTH_TOKEN` providing the same dev/test ergonomic that exists
   today.

**Hard Constraints:**

- **No DB roundtrip on the hot validation path.** Per-request token
  validation in `production` MUST resolve the caller identity from token
  material alone (signature verification + claim parsing). Revocation
  checks MAY consult an in-memory cache that is replicated/refreshed
  asynchronously, but per-request validation MUST NOT issue a DB query
  for the common case.
- **Claim-binding is the only identity source for the hot path.**
  Handlers under `bearerAuthMiddleware` MUST NOT trust request-header or
  request-body fields for `actor_id`, `owner_user_id`, or `actor_source`
  in `production`. The `X-Actor-Id` header workaround introduced for
  MIT-040-S-003 partial closure MUST be removed (or downgraded to a
  dev/test-only fallback explicitly gated on
  `cfg.Environment != "production"`) when the new path replaces it.
- **Backward compatibility with `SMACKEREL_AUTH_TOKEN` is preserved for
  `development` and `test` environments.** The dev ergonomic of
  "one shared token, no enrollment, no claim-binding" MUST survive intact
  for those environments, including the empty-token bypass already
  documented in
  [`internal/api/router.go`](../../internal/api/router.go) lines 444–451.
- **Fail-loud on missing config in `production`.** If
  `SMACKEREL_ENV=production` AND the per-user bearer-auth subsystem is
  enabled but its required SST configuration (signing key material,
  algorithm choice, rotation grace window) is absent or invalid, the
  service MUST refuse to start (mirroring the
  `SMACKEREL_AUTH_TOKEN must be set when SMACKEREL_ENV=production`
  precedent at
  [`cmd/core/wiring.go`](../../cmd/core/wiring.go) lines 48–55).
- **All token-related runtime values flow through SST.** Signing keys,
  algorithm selection, token TTL, rotation grace window, revocation cache
  refresh interval, and clock-skew tolerance MUST originate from
  [`config/smackerel.yaml`](../../config/smackerel.yaml) and propagate via
  `./smackerel.sh config generate` — zero hardcoded secrets, zero
  hardcoded TTLs in business logic.
- **MIT-040-S-008, MIT-038-S-003, and MIT-027-TRACE-001 actor-source
  closure is in-scope.** Closing this spec MUST mark all three resolved
  in their owning state.json files with cross-references back to spec
  044, and the documented trigger ("once per-user bearer tokens /
  claim-binding land") MUST be honored — not deferred again.

**Failure Condition:** If a `production` deployment's authenticated
handlers can be made to act on behalf of a user identity supplied via the
request body, a request header, or a single shared token, the spec has
failed. If hot-path validation requires a DB roundtrip per request, the
spec has failed. If `development`/`test` deployments are forced into the
new enrollment flow and break their existing dev ergonomic, the spec has
failed. If MIT-040-S-008, MIT-038-S-003, or the actor-source segment of
MIT-027-TRACE-001 remain open after this spec is marked done, the spec
has failed.

## Goals

- **G1** — Define the per-user bearer-token issuance contract that the
  enrollment surface (admin-issued in scope; self-service signup
  out-of-scope) MUST satisfy: token format, signing material, claims
  payload, expiry, and storage discipline.
- **G2** — Define the stateless validation contract that
  `bearerAuthMiddleware` (and any equivalent web-UI or NATS-bridged
  bearer surface) MUST satisfy in `production`, including a per-request
  validation latency budget that excludes DB roundtrips.
- **G3** — Define the claim-binding contract: how handlers downstream of
  the middleware MUST derive `actor_id`, `owner_user_id`, and
  `actor_source` from the authenticated session, and what handlers MUST
  do when callers supply those values via body/header (reject in
  `production`; tolerate as dev/test fallback otherwise).
- **G4** — Define the rotation contract: how a user's token can be
  rotated without breaking active sessions for a configured grace window,
  and what happens to the prior token after the grace window elapses.
- **G5** — Define the revocation contract: how a user's token can be
  invalidated immediately, what data structure backs revocation lookup,
  and what the propagation-latency contract is across runtime instances.
- **G6** — Define the migration contract that keeps `SMACKEREL_AUTH_TOKEN`
  fully valid for `development` and `test` environments while making
  per-user bearer auth the *only* authenticated-identity source in
  `production`.
- **G7** — Define the fail-loud configuration contract: which SST values
  the per-user bearer-auth subsystem requires in `production` and how
  startup MUST refuse to proceed when any are missing or invalid.
- **G8** — Close MIT-040-S-008 (photos reveal mint actor_id source),
  MIT-038-S-003 (drive Connect owner_user_id source), and the
  actor-source segment of MIT-027-TRACE-001 (annotation actor_source) by
  routing each of those handlers through the new claim-binding contract.

## Non-Goals

- **Third-party connector OAuth tokens.** Cloud-drive provider OAuth
  (Google Drive, OneDrive, Dropbox) and any future provider OAuth flows
  remain separate concerns owned by their respective specs (e.g., spec
  038 cloud-drives-integration, spec 040 cloud-photo-libraries). This
  spec governs the **Smackerel-issued** bearer token only.
- **End-user enrollment UX (admin UI, self-service signup, password
  reset, email verification).** This spec defines the **issuance
  contract** that any enrollment surface MUST satisfy, but the actual
  admin UI / signup flow / password mechanism is a separate spec. The
  bootstrap path for the first deployment is documented but not a
  user-facing UX feature.
- **Multi-factor authentication.** TOTP, WebAuthn, hardware-key second
  factors, and step-up auth are out of scope. The MVP per-user bearer
  contract is single-factor (token-bearer).
- **Federated identity / SSO.** OIDC, SAML, social login (Google, GitHub,
  Apple) are out of scope. Smackerel issues its own tokens.
- **Session management UI.** "View / revoke active sessions per user"
  surfaces are out of scope. The revocation contract is server-side; the
  end-user surface to drive it is a separate spec.
- **RBAC / ACL beyond identity.** This spec establishes *who* the caller
  is. *What* the caller is allowed to do (per-resource permissions,
  roles, ACLs) is out of scope. Existing authorization checks remain
  unchanged.
- **Rate limiting and abuse mitigation.** Per-user request quotas, IP
  throttling, and abuse-detection signals are not part of this spec.
- **Replacing `SMACKEREL_AUTH_TOKEN` for `development` and `test`
  environments.** The single-tenant token survives intact for those
  profiles. Production-only tightening is the contract.
- **Migration of historical `actor_id="system"` annotations / photo
  reveals.** Records written before this spec lands keep their
  documented values. Forward-looking writes use the new claim-binding
  contract.

## User Scenarios (Gherkin)

### SCN-AUTH-001 — User enrollment issues a per-user bearer token

```gherkin
Given a Smackerel deployment with `runtime.environment: production`
And the per-user bearer-auth subsystem is configured with valid signing
   material, algorithm, TTL, and rotation grace window per SST
And an enrollment surface (admin-issued for the MVP) is available to the
   operator
When the operator enrolls a new user identified by a stable user
   identifier
Then the enrollment surface returns a per-user bearer token whose
   token material is bound to that user identifier
And the token's claims include the user identifier, an issued-at
   timestamp, an expiry timestamp consistent with the configured TTL,
   and an issuer claim identifying the deployment
And the persisted user record includes the data required to drive
   future rotation and revocation lookups (without requiring the raw
   token to be stored alongside)
```

### SCN-AUTH-002 — Bearer token survives stateless validation in production mode without DB roundtrip

```gherkin
Given a `production` deployment whose per-user bearer-auth subsystem is
   live
And a previously enrolled user holds a non-expired, non-revoked
   per-user bearer token
When that user calls any authenticated API endpoint with
   `Authorization: Bearer <token>`
Then `bearerAuthMiddleware` validates the token statelessly using the
   SST-derived signing material and algorithm
And the validation path consumes no per-request database query for the
   common (non-revoked) case
And per-request validation latency at the middleware boundary stays
   inside the latency budget declared in NFR-AUTH-001
And the request proceeds to the downstream handler with an
   authenticated session context that exposes the caller's user
   identifier
```

### SCN-AUTH-003 — actor_id is derived from token claims, not request header trust

```gherkin
Given a `production` deployment with the per-user bearer-auth subsystem
   live
And an authenticated user calling `POST /v1/photos/{id}/reveal`
When the request body or `X-Actor-Id` header attempts to supply a
   different `actor_id`
Then the handler ignores the body/header value and derives `actor_id`
   exclusively from the authenticated session context
And the audit-log entry written for the reveal records the
   session-derived `actor_id`
And the handler rejects the request when the body or header attempts
   to claim an identity different from the session identity (per the
   MIT-040-S-008 closure contract)
And the same claim-binding rule applies to every handler that
   previously read `actor_id` or `actor_source` from a body or header
```

### SCN-AUTH-004 — Token rotation revokes prior token without breaking active sessions for grace window

```gherkin
Given a `production` deployment with a configured rotation grace window
   (per SST)
And a user holds a current per-user bearer token T1
When the user (or operator on the user's behalf) rotates the token,
   producing a new token T2
Then T2 is immediately valid for authenticated requests
And T1 remains valid for the configured grace window after rotation,
   allowing in-flight clients to refresh without seeing 401 errors
And T1 is rejected with HTTP 401 once the grace window elapses
And after the grace window elapses, only T2 (or further-rotated
   successors) authenticate successfully for that user
```

### SCN-AUTH-005 — Single-tenant SMACKEREL_AUTH_TOKEN remains valid for dev/test profiles

```gherkin
Given a deployment whose `runtime.environment` is `development` or
   `test`
And `SMACKEREL_AUTH_TOKEN` is set (or empty for dev-mode bypass per
   today's contract)
When clients call authenticated API endpoints exactly as they did at
   HEAD `f7001ab` (using the shared bearer or relying on the empty-
   token dev bypass)
Then the requests authenticate (or bypass authentication, in the
   empty-token dev case) exactly as they did before this spec
And no enrollment of per-user tokens is required for that deployment
   to function
And handlers that have a `production`-only strict claim-binding rule
   continue to honor the existing dev/test fallback (e.g., `X-Actor-Id`
   header in dev/test mirrors the MIT-040-S-003 partial-closure pattern)
```

### SCN-AUTH-006 — Token-issuance flow is fail-loud on missing config

```gherkin
Given an operator boots a deployment with `SMACKEREL_ENV=production`
   AND the per-user bearer-auth subsystem is enabled
And one or more required SST configuration values for the subsystem
   (signing key material, algorithm choice, TTL, rotation grace window)
   is absent or invalid in `config/smackerel.yaml` and the resolved
   `production.env`
When the service starts
Then the service refuses to start with a clear error message naming
   each missing or invalid configuration key
And the service does NOT silently fall back to permissive behavior
   (no auto-generated secret, no default key, no warn-and-continue)
And the same SST-validation discipline applies on `./smackerel.sh
   config generate --env production`: the generator surfaces the same
   missing-config errors before producing the env file
```

### SCN-AUTH-007 — Cloud-drive Connect derives owner_user_id from session (closes MIT-038-S-003)

```gherkin
Given a `production` deployment with the per-user bearer-auth subsystem
   live
And an authenticated user calling the cloud-drive `Connect` flow
When the request body or any request header attempts to supply
   `owner_user_id` directly
Then the Connect handler ignores the body/header value and derives
   `owner_user_id` exclusively from the authenticated session context
And the persisted `drive_oauth_states` row records the session-derived
   `owner_user_id`
And the persisted `drive_connections` row, after FinalizeConnect,
   records the same session-derived `owner_user_id`
And MIT-038-S-003 is marked resolved in
   `specs/038-cloud-drives-integration/state.json` with a
   cross-reference to spec 044
```

### SCN-AUTH-008 — User annotation actor_source is session-derived (closes MIT-027-TRACE-001 actor source)

```gherkin
Given a `production` deployment with the per-user bearer-auth subsystem
   live
And an authenticated user creating a user annotation through any of
   the documented annotation entry points (Telegram bridge, NATS
   payload, API)
When the annotation entry-point payload attempts to supply
   `actor_source` or an actor identity directly
Then the annotation pipeline ignores the supplied identity and derives
   `actor_source` from the authenticated session context that mints the
   pipeline event
And the persisted annotation record carries the session-derived
   `actor_source` value
And the actor-source segment of MIT-027-TRACE-001 is marked resolved
   in `specs/027-user-annotations/state.json` with a cross-reference to
   spec 044
```

### SCN-AUTH-009 — Revoked token is refused on the next authenticated request

```gherkin
Given a `production` deployment with the per-user bearer-auth subsystem
   live and a configured revocation propagation budget per NFR-AUTH-006
And a user holds a non-expired per-user bearer token T1
When an operator (or the user) revokes T1 through the documented
   revocation surface
Then within the configured propagation budget, the next authenticated
   request bearing T1 is rejected with HTTP 401
And subsequent revocation lookups for T1 remain rejecting until T1's
   natural expiry
And revocation does not require restarting the service
```

### SCN-AUTH-010 — Stale or tampered token is refused with constant-time discipline

```gherkin
Given a `production` deployment with the per-user bearer-auth subsystem
   live
When an authenticated request bears a token that is expired, signed
   with the wrong key, structurally malformed, or whose signature does
   not verify
Then `bearerAuthMiddleware` rejects the request with HTTP 401
And the rejection path uses signature verification primitives that do
   not leak timing information about which validation step failed
And the response body does not disclose which validation step failed
   beyond a generic `UNAUTHORIZED` error
And the failure is logged with the request path and remote address
   (mirroring today's `bearer auth failure` slog warning at
   `internal/api/router.go` line 467)
```

### SCN-AUTH-011 — Migration path: existing dev / test deployments need zero changes

```gherkin
Given a developer's existing `development` deployment at HEAD
   `f7001ab` with the current `SMACKEREL_AUTH_TOKEN` model
When the developer pulls the spec-044 implementation and runs
   `./smackerel.sh config generate && ./smackerel.sh up`
Then the deployment continues to authenticate with
   `SMACKEREL_AUTH_TOKEN` exactly as before
And no new required configuration values are introduced for
   `development` deployments (per FR-AUTH-013)
And no enrollment step is required for the deployment to be usable
   for local development
```

### SCN-AUTH-012 — Telegram bridge per-user PASETO wiring + operator-visible auth metrics surface (Scope 04 F02 closure)

```gherkin
Given a `production` deployment at HEAD `99be90d8` with
   `auth.enabled: true`, signing key material configured, and the
   Telegram connector enabled with a populated
   `TELEGRAM_USER_MAPPING` (chat_id:user_id pairs)
When the Telegram bot makes any internal API call on behalf of a
   mapped chat (capture, reply-annotation, annotation-command,
   share-flow, photo-upload, recipe-flow)
Then `cmd/core/wiring.go::startTelegramBotIfConfigured` has
   constructed a `PerUserTokenMinter` (TTL=5m) and called
   `tgBot.SetPerUserTokenMinter` at startup (because production AND
   `auth.enabled` AND signing key material are all present)
And `internal/telegram/bot.go::Bot.bearerForChat` mints a per-user
   PASETO via `tokenMinter.MintForChat(chatID)` whose claims bind
   the token to the resolved `user_id` from the chat-id mapping
And `Bot.setBearerHeader` attaches the per-user PASETO to the
   outbound `Authorization` header (replacing any shared bearer)
And the production unmapped-chat case propagates
   `auth.ErrNoUserMappingForChat` through `setBearerHeader` and
   forces the caller to refuse the outbound request (no shared-bearer
   leak; counter delta=0 on refused mint)
And the seven-series `smackerel_auth_*` Prometheus surface
   (`AuthIssuance`, `AuthRotation`, `AuthRevocation`,
   `AuthValidationLatency`, `AuthValidationOutcome`,
   `AuthLegacyFallbackUsed`, `AuthFailure`) registered via
   `internal/metrics/auth.go` `init()` ticks under closed-set labels
   (no actor IDs, no chat IDs, no token contents in label values)
And `auth.production_shared_token_fallback_enabled: false` (the
   SST default at `config/smackerel.yaml` line 514) ensures
   `internal/api/router.go` `bearerAuthMiddleware` Branch 2
   refuses the legacy `SMACKEREL_AUTH_TOKEN` in production while
   recording `AuthLegacyFallbackUsed{environment="production"}`
   only when the operator has explicitly opted in to the
   transition fallback
```

## Functional Requirements

### Issuance (G1)

- **FR-AUTH-001** — The per-user bearer-auth subsystem MUST expose an
  issuance entry point (admin-issued for the MVP) that accepts a stable
  user identifier and returns a per-user bearer token whose claims bind
  the token to that user identifier.
- **FR-AUTH-002** — Each issued token MUST carry, at minimum: the user
  identifier (subject claim), an issued-at timestamp, an expiry
  timestamp consistent with the SST-configured TTL, and an issuer claim
  identifying the deployment.
- **FR-AUTH-003** — The persisted per-user record MUST contain the
  metadata required to drive rotation and revocation (issuance
  identifier, last-rotated-at, revocation flags, etc.) without storing
  the raw token. If a hashed-token storage strategy is selected by
  `bubbles.design`, only the hash MUST be persisted (mirroring the
  MIT-040-S-001 hash-on-mint pattern).

### Validation (G2)

- **FR-AUTH-004** — `bearerAuthMiddleware` (and any equivalent web-UI
  or NATS-bridged bearer surface) MUST validate per-user bearer tokens
  in `production` using SST-derived signing material and algorithm,
  with no per-request DB query for the common (non-revoked) case.
- **FR-AUTH-005** — Successful validation MUST attach an authenticated
  session context (user identifier, optional metadata) to the request
  for downstream handlers. The mechanism for surfacing the session
  context to handlers (request context value, helper accessor) is a
  design decision; the contract is that handlers MUST be able to read
  the authenticated user identifier without re-validating the token.
- **FR-AUTH-006** — Failed validation MUST produce HTTP 401 with the
  generic `UNAUTHORIZED` error body that today's middleware emits, and
  MUST log the failure with path + remote-address metadata (parity
  with `internal/api/router.go` line 467).

### Claim-binding (G3, MIT-040-S-008, MIT-038-S-003, MIT-027-TRACE-001)

- **FR-AUTH-007** — Every handler downstream of `bearerAuthMiddleware`
  in `production` MUST derive caller identity (`actor_id`,
  `owner_user_id`, `actor_source`, equivalent fields) exclusively from
  the authenticated session context. Body- and header-sourced identity
  for those fields MUST be rejected at the handler boundary in
  `production`.
- **FR-AUTH-008** — The photo-reveal mint flow
  (`POST /v1/photos/{id}/reveal`,
  [`internal/api/photos_upload.go`](../../internal/api/photos_upload.go)
  `MintReveal`) MUST stop honoring `X-Actor-Id` as the identity source
  in `production` and MUST instead derive `actor_id` from the
  authenticated session context. The dev/test fallback documented in
  the MIT-040-S-003 partial closure MAY remain for non-production
  environments.
- **FR-AUTH-009** — The cloud-drive Connect flow
  ([`internal/drive/google/google.go`](../../internal/drive/google/google.go)
  `Connect` and the OAuth state pipeline,
  [`internal/drive/context.go`](../../internal/drive/context.go)) MUST
  derive `owner_user_id` from the authenticated session context for
  every `production` request, closing MIT-038-S-003. Body-sourced
  `OwnerUserID` MUST be rejected in `production`.
- **FR-AUTH-010** — User annotation entry points (Telegram bridge,
  NATS payload, API) MUST derive `actor_source` from the authenticated
  session context for every `production` request, closing the
  actor-source segment of MIT-027-TRACE-001. The literal
  `actor_source: "system"` fallback MUST NOT be used for
  user-originated annotations in `production`.

### Rotation (G4)

- **FR-AUTH-011** — The subsystem MUST support per-user token rotation
  that issues a new token AND keeps the prior token valid for the
  SST-configured grace window. After the grace window, the prior token
  MUST be rejected.
- **FR-AUTH-012** — Rotation MUST not require service restart. Rotation
  state changes MUST be visible to the validation hot path within the
  same request after the rotation surface confirms success.

### Revocation (G5)

- **FR-AUTH-013** — The subsystem MUST support immediate revocation of
  a per-user token. Revocation lookup on the validation hot path MUST
  use an in-memory cache (refreshed asynchronously) so that the common
  case adds no DB roundtrip per request.
- **FR-AUTH-014** — Revocation propagation across multiple runtime
  instances MUST occur within the SST-configured propagation budget
  (NFR-AUTH-006). The propagation mechanism (NATS broadcast, periodic
  refresh, log replay) is a design decision.

### Backward compatibility (G6)

- **FR-AUTH-015** — `SMACKEREL_AUTH_TOKEN` MUST remain a fully valid
  authentication source for `runtime.environment in {development,
  test}`. Today's empty-token dev bypass at
  `internal/api/router.go` lines 444–451 MUST survive intact.
- **FR-AUTH-016** — The per-user bearer-auth subsystem MUST be
  enable-by-default-in-production but MAY be disabled in
  `development` and `test` (so existing dev/test workflows that do not
  enroll users continue to function without modification).
- **FR-AUTH-017** — When a `production` deployment carries BOTH a
  configured `SMACKEREL_AUTH_TOKEN` AND the per-user bearer-auth
  subsystem enabled, per-user tokens MUST take precedence; the shared
  token MUST be rejected (deprecation path) OR explicitly opt-in
  preserved as a documented operator-mode escape hatch (design
  decision via `bubbles.design`).

### Configuration (G7)

- **FR-AUTH-018** — All token-related runtime values (signing key
  material, algorithm choice, TTL, rotation grace window, revocation
  cache refresh interval, clock-skew tolerance, propagation budget)
  MUST originate from `config/smackerel.yaml` and propagate via
  `./smackerel.sh config generate`. Zero hardcoded secrets, TTLs, or
  algorithm strings in business logic.
- **FR-AUTH-019** — When `SMACKEREL_ENV=production` AND the per-user
  bearer-auth subsystem is enabled, a missing or invalid required
  configuration value MUST cause both `./smackerel.sh config generate`
  and the runtime startup path to fail loudly with an error message
  naming the missing/invalid keys (no auto-generated defaults, no
  warn-and-continue).

### Closure routing (G8)

- **FR-AUTH-020** — Closing this spec MUST mark the following resolved
  with cross-references back to spec 044 in their owning state.json
  files: `MIT-040-S-008` in
  `specs/040-cloud-photo-libraries/state.json`; `MIT-038-S-003` in
  `specs/038-cloud-drives-integration/state.json`; the actor-source
  segment of `MIT-027-TRACE-001` in
  `specs/027-user-annotations/state.json`.
- **FR-AUTH-021** — Closing this spec MUST update any handler comment
  blocks that today reference the outstanding trust deferral
  (notably the comment at
  [`internal/api/photos_upload.go`](../../internal/api/photos_upload.go)
  lines 246–321) to reflect the resolved state.

## Non-Functional Requirements

- **NFR-AUTH-001** — **Hot-path validation latency.** Per-request
  middleware validation latency for the common (non-revoked) case MUST
  be ≤ 5 ms p99 on a developer-class host, measured from middleware
  entry to handler entry, excluding network and DB.
- **NFR-AUTH-002** — **No DB roundtrip on hot path.** The common-case
  validation path MUST NOT issue a database query per request.
  Revocation lookups MUST consult an in-memory cache; cache refresh
  MAY be async and MAY use a database query, but not on the request
  hot path.
- **NFR-AUTH-003** — **Rotation grace window default.** The default
  rotation grace window in
  [`config/smackerel.yaml`](../../config/smackerel.yaml) MUST be ≥ 24
  hours and configurable. The actual default is a design decision but
  cannot be lower than 24 h to allow long-lived clients to rotate
  safely.
- **NFR-AUTH-004** — **Token storage at-rest hashing.** When a hashed
  storage strategy is selected by `bubbles.design`, the hash function
  MUST be a one-way construction suitable for token authentication
  (HMAC-SHA-256, Argon2id, or equivalent — algorithm choice deferred
  to design). Constant-time comparison MUST be used wherever a stored
  hash is compared against an incoming token's hash.
- **NFR-AUTH-005** — **Clock-skew tolerance.** The validation path
  MUST tolerate a configurable clock-skew window (default ≤ 60
  seconds) so that small clock drift between issuer and verifier does
  not produce false 401s on otherwise-valid tokens.
- **NFR-AUTH-006** — **Revocation propagation budget.** Revocation
  state changes MUST become visible on the validation hot path within
  ≤ 60 seconds (configurable; default ≤ 60 s) across all runtime
  instances of a deployment.
- **NFR-AUTH-007** — **Failure-mode logging hygiene.** Authentication
  failure logs MUST NOT include the raw token, the token's signature,
  or any data that would allow an attacker reading logs to
  reconstruct or replay the token. Path, remote address, and a
  reason category are the only fields permitted (parity with today's
  `slog.Warn("bearer auth failure", ...)` site).
- **NFR-AUTH-008** — **Constant-time comparison for token match
  paths.** Any path that compares a stored token (or token hash)
  against an incoming candidate MUST use a constant-time comparison
  primitive (e.g., `crypto/subtle.ConstantTimeCompare`), preserving
  today's `subtle.ConstantTimeCompare` discipline at
  `internal/api/router.go` line 467.

## Acceptance Criteria

- **AC-1** — In a `production` deployment with the per-user bearer-auth
  subsystem live, a previously enrolled user can call any authenticated
  endpoint with their per-user bearer token and receive a non-401
  response. The per-request middleware path issues zero database
  queries, verifiable from a query-counting harness.
- **AC-2** — Calling `POST /v1/photos/{id}/reveal` with `actor_id` in
  the request body OR `X-Actor-Id` in the request header is rejected
  in `production`. The handler instead derives `actor_id` from the
  authenticated session, verifiable from the audit-log entry.
- **AC-3** — Calling the cloud-drive Connect flow with `owner_user_id`
  in the request body is rejected in `production`. The persisted
  `drive_oauth_states` and `drive_connections` rows record the
  session-derived `owner_user_id`.
- **AC-4** — Creating a user annotation in `production` records an
  `actor_source` value derived from the authenticated session — never
  the literal string `system` and never a body/header-supplied value.
- **AC-5** — Rotating a user's token in `production` produces a new
  token; both the old and new tokens authenticate inside the
  configured grace window; only the new token authenticates after the
  grace window elapses.
- **AC-6** — Revoking a user's token in `production` causes the next
  authenticated request bearing that token (issued no later than the
  configured propagation budget after revocation) to be rejected with
  HTTP 401. Service restart is not required.
- **AC-7** — Booting a `production` deployment with the per-user
  bearer-auth subsystem enabled and missing required SST configuration
  causes both `./smackerel.sh config generate --env production` and
  the runtime startup to fail loudly with an error naming each missing
  configuration key.
- **AC-8** — Booting `./smackerel.sh up` in the dev profile at HEAD
  with no per-user bearer-auth configuration changes succeeds; a
  client calling authenticated endpoints with the existing
  `SMACKEREL_AUTH_TOKEN` (or relying on the empty-token dev bypass)
  authenticates exactly as it did at HEAD `f7001ab`.
- **AC-9** — `bash .github/bubbles/scripts/artifact-lint.sh
  specs/044-per-user-bearer-auth` exits 0 once design.md, scopes.md,
  uservalidation.md, and report.md are authored by their respective
  owners (`bubbles.design`, `bubbles.plan`, etc.). The spec.md
  authoring step (this artifact) MUST pass spec-template BDD checks
  per `.github/skills/bubbles-spec-template-bdd/SKILL.md`.
- **AC-10** — After spec close, the following carry the resolution:
  - `specs/040-cloud-photo-libraries/state.json` marks
    `MIT-040-S-008` resolved with a cross-reference to spec 044.
  - `specs/038-cloud-drives-integration/state.json` marks
    `MIT-038-S-003` resolved with a cross-reference to spec 044.
  - `specs/027-user-annotations/state.json` marks the actor-source
    segment of `MIT-027-TRACE-001` resolved with a cross-reference to
    spec 044.
- **AC-11** — `grep -rEn 'X-Actor-Id|actor_id_in_body_forbidden|"actor_id"'
  internal/` after spec close shows that the only remaining matches
  in `production`-applicable code paths are dev/test fallbacks
  explicitly gated by `cfg.Environment != "production"`, OR are
  removed entirely. No production-applicable header-trust or
  body-trust paths remain for `actor_id`.

## Backward Compatibility

This spec is **non-breaking** for `development` and `test` deployments by
design. Specifically:

- `runtime.environment: development` and `runtime.environment: test`
  deployments at HEAD `f7001ab` continue to function without any
  configuration changes after spec 044 ships. `SMACKEREL_AUTH_TOKEN`
  remains the authentication mechanism; the empty-token dev bypass
  remains intact; no enrollment step is required.
- The per-user bearer-auth subsystem is **enabled-by-default in
  `production`** and **disabled-by-default in `development`/`test`**.
  Operators of `production` deployments MUST enroll at least one user
  before the deployment serves authenticated traffic.
- Migration of existing `production` operators (today running
  `SMACKEREL_AUTH_TOKEN` only) requires:
  1. Adding the per-user bearer-auth SST configuration to
     `config/smackerel.yaml` (signing material, algorithm, TTL,
     rotation grace window).
  2. Enrolling the operators / users who will hold per-user tokens.
  3. Distributing per-user tokens to the appropriate clients.
  4. Optionally retiring `SMACKEREL_AUTH_TOKEN` for `production` (per
     FR-AUTH-017's design decision).
- Historical writes (audit-log entries, photo reveal records, drive
  connections, annotations) created before spec 044 lands keep their
  documented `actor_id` / `owner_user_id` / `actor_source` values
  intact. Only forward-looking writes use the new claim-binding
  contract.

## Security Considerations

The following security topics are surfaced for `bubbles.design` to
resolve. Each is in-scope for the spec's outcome contract; the
*implementation choice* is design-owned.

- **Token format.** Three candidates are surfaced for `bubbles.design`:
  (a) JWT with a HS256 / EdDSA signing key; (b) PASETO v4.local /
  v4.public; (c) opaque random tokens with stored hashes (HMAC-SHA-256
  on the hash side). Each has a different stateless-validation
  posture, key-management posture, and revocation posture. The choice
  belongs in design.md per [`docs/Product-Principles.md`](../Product-Principles.md)
  and the constitution's Local-First posture (avoid cloud-hosted JWKS
  unless strictly necessary).
- **Signing key storage and rotation.** Signing keys for stateless
  tokens are themselves SST values. Their storage discipline (env-only,
  sealed-secrets, KMS-backed, file-mode-restricted on disk) is a
  design decision, but they MUST NOT be checked into the repo and MUST
  NOT be auto-generated on first boot in `production` (per FR-AUTH-019
  fail-loud).
- **Token transport.** Tokens MUST NOT be exposed in URLs or
  query-string parameters. The `Authorization: Bearer <token>` header
  is the only sanctioned transport. Cookie-based delivery for the
  web-UI surface MAY be retained from today's `auth_token` cookie
  pattern at `internal/api/router.go` lines 425–433, but the cookie
  MUST be marked HTTP-only and Secure in `production`.
- **Token storage on the client.** This spec does not constrain client
  storage (browser local storage, secure file on disk, OS keychain).
  Clients are operator-controlled.
- **Audit logging.** Authentication events (issuance, rotation,
  revocation, validation failure) MUST be logged at structured-log
  level with the user identifier, event type, and request metadata.
  The raw token MUST NOT appear in logs (NFR-AUTH-007).
- **Replay.** Stateless tokens with bounded TTL plus the revocation
  cache provide the replay window. Reducing TTL reduces replay
  exposure; the SST default TTL is a design decision but MUST be
  bounded (no infinite tokens in `production`).
- **Cross-spec impact: QF Companion (spec 041).** Per the
  [`product-principles.instructions.md`](../../.github/instructions/product-principles.instructions.md)
  Principle 10 (QF Companion Boundary), Smackerel-issued tokens MUST
  NOT be reused as QF authentication tokens. The QF
  `PersonalEvidenceBundle` and `QFDecisionPacket` metadata are
  separate trust boundaries; this spec MUST NOT alter their existing
  attribution surface.

## Product Principle Alignment

> Per [`.github/instructions/product-principles.instructions.md`](../../.github/instructions/product-principles.instructions.md),
> every new feature spec touching a principle area MUST declare its
> alignment. Principles 1–10 in
> [`docs/Product-Principles.md`](../../docs/Product-Principles.md) are
> currently *Surfaced for owner approval — not yet ratified*; this
> alignment is therefore advisory until ratification, recorded for the
> future binding pass.

- **Principle 1 — Observe First, Ask Second.** This spec eliminates a
  family of "ask the client to claim its own identity" patterns
  (`X-Actor-Id` header, body `owner_user_id`, body `actor_source`) in
  favor of "infer caller identity from the authenticated session the
  middleware already established." The user is asked nothing extra at
  the boundary; the system already has the answer from the token
  presented at the start of the request.
- **Principle 6 — Invisible By Default, Felt Not Heard.** This spec is
  pure trust-boundary infrastructure. It adds no notifications, no
  status prompts, and no end-user UX. The end-user surface area
  changes by zero. Operators feel a one-time enrollment step in
  `production`; users see nothing different unless their token
  rotates.
- **Principle 8 — Trust Through Transparency.** Audit logs gain real
  per-user attribution where today they record `system` or
  client-asserted strings. Annotations, photo reveals, and drive
  connections become attributable to a verified user identity rather
  than a header the client supplied.
- **Principle 10 — QF Companion Boundary (NON-NEGOTIABLE
  cross-product).** This spec explicitly DOES NOT change the QF
  packet metadata surface. Smackerel-issued tokens are scoped to
  Smackerel; QF retains its own trust boundary. Cross-product impact
  is "none required."
- **Constitution C1 (Local-First Knowledge Ownership).** The default
  token format, signing material, and storage discipline MUST favor a
  local-first posture (no cloud KMS dependency on the hot path). The
  design pass MAY consider cloud-KMS-backed signing as an opt-in for
  operators who want it, but the default MUST work fully offline.
- **Constitution C5 (Passive by Default, Explicit on Action).** The
  per-user trust boundary is the *passive* surface that lets every
  feature downstream of authentication remain passive (annotations,
  recommendations, digests). Without this, every feature that needs
  per-user state becomes an "explicit on action" surface that asks
  the client to declare its own identity. Closing this spec turns
  per-user features back into passive surfaces.
- **Constitution C7 (Single CLI Operations) and C8 (Single Source Of
  Truth Configuration).** Token issuance, key rotation, and operator
  workflows for the per-user surface MUST flow through `./smackerel.sh`
  (per the CLI surface contract in
  [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md))
  and MUST consume their values from `config/smackerel.yaml`. Zero
  ad-hoc binaries; zero hardcoded values.

## References

### Routing origin

- **Primary trigger:** Backlog item `MIT-040-S-008` recorded in
  [`specs/040-cloud-photo-libraries/state.json`](../040-cloud-photo-libraries/state.json)
  (entry under `executionHistory` for the MIT-040-S-003 partial closure
  pass), commit `4e399a4`. The closing trigger documented there is
  "Production deployment requires per-user authentication beyond the
  single-tenant SMACKEREL_AUTH_TOKEN. NEW FEATURE SPEC required."
- **Carry-forward source:** `MIT-040-S-003` (partial closure at
  commit `4e399a4`, full closure deferred to spec 044).

### Cross-spec closures

- **MIT-038-S-003** —
  [`specs/038-cloud-drives-integration/state.json`](../038-cloud-drives-integration/state.json)
  routes "Connect handler auth-context owner_user_id" to "bubbles.plan
  / implement, future scope." Spec 044 is that future scope.
- **MIT-027-TRACE-001 actor-source segment** —
  [`specs/027-user-annotations/state.json`](../027-user-annotations/state.json)
  records actor-source provenance gaps. The DoD-trace coverage portion
  was closed at iter 11; the actor-source provenance portion is
  closed by spec 044.

### Code surfaces touched (informative, not exhaustive — design owns)

- [`internal/api/router.go`](../../internal/api/router.go) — `bearerAuthMiddleware`,
  `webAuthMiddleware`, `extractBearerToken`, `matchBearerToken`.
- [`internal/api/photos_upload.go`](../../internal/api/photos_upload.go) —
  `MintReveal` X-Actor-Id source replacement.
- [`internal/drive/google/google.go`](../../internal/drive/google/google.go) +
  [`internal/drive/context.go`](../../internal/drive/context.go) —
  `Connect` / `BeginConnect` / `FinalizeConnect` owner_user_id source
  replacement.
- [`internal/api/drive_handlers.go`](../../internal/api/drive_handlers.go) —
  Drive Connect handler entry point.
- [`internal/annotation/`](../../internal/annotation/) — Annotation
  pipeline `actor_source` derivation.
- [`internal/config/config.go`](../../internal/config/config.go) — SST
  surface for new per-user bearer-auth configuration values
  (extends the existing `SMACKEREL_AUTH_TOKEN` validation block at
  lines 880–960).
- [`cmd/core/wiring.go`](../../cmd/core/wiring.go) — Production
  fail-fast extension for new required configuration values.
- [`config/smackerel.yaml`](../../config/smackerel.yaml) — SST
  declaration for signing material, algorithm, TTL, rotation grace
  window, revocation cache refresh interval, clock-skew tolerance,
  propagation budget.

### Architectural anchors

- [`docs/smackerel.md`](../../docs/smackerel.md) — overall product and
  architecture posture.
- [`docs/Operations.md`](../../docs/Operations.md) — operator surfaces
  for runtime configuration.
- [`docs/Deployment.md`](../../docs/Deployment.md) — Build-Once
  Deploy-Many configuration bundle contract that the new SST values
  flow through.

### Governance

- [`.github/instructions/bubbles-config-sst.instructions.md`](../../.github/instructions/bubbles-config-sst.instructions.md)
  and the `bubbles-config-sst` skill — applies to FR-AUTH-018 and
  FR-AUTH-019.
- [`.github/instructions/product-principles.instructions.md`](../../.github/instructions/product-principles.instructions.md) —
  Product Principle Alignment section above.
- [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md) —
  Core Principles cited in the alignment section.
- [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md) —
  SST Zero-Defaults Enforcement, Secrets Management, Live-stack test
  authenticity rules that downstream test plans MUST honor.

### Adjacent ratified specs

- [`specs/020-security-hardening/`](../020-security-hardening/) — sets
  the authentication-and-trust-boundary precedent this spec extends.
- [`specs/040-cloud-photo-libraries/`](../040-cloud-photo-libraries/) —
  parent of MIT-040-S-008 routing.
- [`specs/038-cloud-drives-integration/`](../038-cloud-drives-integration/) —
  parent of MIT-038-S-003 routing.
- [`specs/027-user-annotations/`](../027-user-annotations/) — parent
  of MIT-027-TRACE-001 actor-source segment.

## Open Questions (to be resolved by `bubbles.design`)

These do not block spec acceptance — they are the design-owned
decisions the next phase will make.

- **OQ-1 — Token format.** JWT (HS256 / EdDSA), PASETO (v4.local /
  v4.public), or opaque-token-with-stored-hash. Each has different
  stateless-validation posture, key-management complexity, and
  revocation posture. Design decision driven by the local-first
  Constitution C1 posture and the no-DB-roundtrip-on-hot-path Hard
  Constraint.
- **OQ-2 — Signing key management.** Where are signing keys stored?
  How are they rotated? Single key with overlap, key-id-in-claims
  with multi-key validation, KMS-backed signing? Design decision.
- **OQ-3 — Authenticated session context shape.** What exactly does
  `bearerAuthMiddleware` attach to the request context? A typed
  `Session` value object with helper accessors? A user-id string
  only? A claims map? Design decision; affects every downstream
  handler refactor.
- **OQ-4 — Revocation propagation mechanism.** NATS broadcast,
  periodic DB refresh, log-replay on startup, or a hybrid? Design
  decision driven by NFR-AUTH-006 (≤ 60 s propagation) and the
  no-DB-roundtrip-on-hot-path Hard Constraint.
- **OQ-5 — Coexistence policy with `SMACKEREL_AUTH_TOKEN` in
  `production`.** Per FR-AUTH-017: when both are configured, is the
  shared token rejected (forced deprecation), accepted as an
  operator-mode escape hatch (documented opt-in), or warned-and-
  rejected (transitional)? Design decision driven by operator
  migration ergonomics.
- **OQ-6 — Enrollment surface for the MVP.** This spec scopes
  *issuance contract*, not *enrollment UX*. The MVP enrollment
  surface (admin CLI command, admin API endpoint, manual SQL +
  scripted token generation) is a design decision. Self-service
  signup is explicitly out of scope.
- **OQ-7 — Web UI session model.** Today the web UI uses an
  `auth_token` cookie at `internal/api/router.go` lines 425–433.
  Does the per-user model preserve cookie-based sessions for the web
  UI (with HTTP-only + Secure cookies) or migrate the web UI to
  bearer-only? Design decision driven by browser ergonomics.
- **OQ-8 — Token storage at-rest hashing strategy.** Hashed-token
  storage (FR-AUTH-003, NFR-AUTH-004) requires choosing a hash
  function and storage column shape. HMAC-SHA-256 with a stored
  random salt? Argon2id? Design decision driven by attacker model
  for stolen-DB scenarios.
- **OQ-9 — Telemetry / metrics surface.** Should authentication
  events emit Prometheus metrics (issuance count, validation
  latency histogram, revocation count, failure count by reason)?
  Design decision; ties into spec 030 observability.
- **OQ-10 — Operator bootstrapping.** How does the very-first
  `production` deployment get its first user enrolled? Bootstrap
  user from SST? Initial-admin token issued at first config
  generate? Design decision driven by the chicken-and-egg of
  needing-an-enrolled-user-to-enroll-a-user.
