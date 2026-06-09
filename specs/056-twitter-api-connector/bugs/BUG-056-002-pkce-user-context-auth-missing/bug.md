# Bug: [BUG-056-002] User-Context OAuth 2.0 PKCE auth missing — App-Only bearer used for user-owned endpoints

## Summary
Spec 056 mandates **User-Context OAuth 2.0 with PKCE** for the user-owned Twitter v2 endpoints (`/2/users/me`, `/2/users/:id/bookmarks`, `/2/users/:id/liked_tweets`) and explicitly states "App-Only bearer tokens are insufficient for these user-owned endpoints" (spec.md:225, NC-1; design.md:131-133). The shipped connector implements **no PKCE flow at all** — it applies a single static `Authorization: Bearer <token>` (App-Only) uniformly to all four endpoints (`internal/connector/twitter/api.go:141`). Against the real Twitter/X API, App-Only bearer tokens return **403** for bookmarks, liked-tweets, and `/2/users/me`, so the connector cannot actually retrieve bookmarks or likes — a central capability. Compounding this, `specs/056-twitter-api-connector/report.md:7` **falsely claims** "App-Only bearer + User-Context PKCE" was delivered.

A secondary observability gap (R-016): the required `x-rate-limit-remaining` gauge was never implemented; only `x-rate-limit-reset` is parsed, and the single gauge updates only on a 429.

## Two Verified Gaps

| ID | Severity | Class | One-line |
|----|----------|-------|----------|
| GAP-056-G1 | **High** | divergent + false delivered-claim | No User-Context OAuth 2.0 PKCE flow; App-Only bearer applied to user-owned endpoints that require user-context; report.md:7 falsely claims PKCE delivery. |
| GAP-056-G2 | Medium | missing + untested | No `x-rate-limit-remaining` gauge (R-016); only `x-rate-limit-reset` parsed, gauge set only on 429 — no per-call rate-headroom visibility. |

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Major feature broken, no workaround
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

**Severity rationale (High, functional gap — not over-specification):** Twitter/X genuinely requires User-Context auth for `/2/users/me`, bookmarks, and liked-tweets; App-Only bearer is rejected with 403 on those endpoints. The connector therefore cannot retrieve bookmarks or likes at all — only the public `/2/users/:id/tweets` and `/2/users/:id/mentions` (which legitimately accept App-Only) would work. There is no workaround within the shipped code path: a different token type and a full authorization-code exchange are required. The accompanying false delivery claim in report.md is an artifact-integrity issue that masks the gap.

## Status
- [ ] Reported
- [x] Confirmed (reproduced via diagnostic evidence at HEAD `9638b065`)
- [x] In Progress (Path A DELIVERED + 7 phases + audit GREEN; terminal close pending the CI-gated migration live-apply)
- [ ] Fixed
- [ ] Verified
- [ ] Closed

**Triage state (2026-06-09, bubbles.audit PASS 2):** DELIVERED + AUDITED — pending CI migration-apply (state.json `status: in_progress`, NON-terminal). The maintainer resolved design.md Q1 → Path A; the real User-Context OAuth 2.0 PKCE flow shipped across Scopes A–D and is independently re-verified GREEN (PKCE S256, AES-256-GCM encrypted token store, authorize CLI, endpoint auth-tier routing, fail-loud `ErrUserContextTokenRequired` with no App-Only fallback, refresh-on-401 + pre-expiry refresh, R-016 `x-rate-limit-remaining` gauge), and is regression-free / appropriately-simple / stable / OWASP-clean, with the named adversarial regression `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` GREEN. The bug is **NOT yet marked Fixed**: the ONLY remaining step is the migration LIVE DB-apply under `./smackerel.sh test integration` (operator/CI-gated — the integration stack is unavailable in this sandbox; the migration auto-applies via `//go:embed`, is unit-verified to parse, and `TestTwitterOAuthMigration_AppliesCleanly` exists CI-ready). The state-transition guard mechanically blocks terminal `done` on that single honest `[ ]` row (G024); no `[x]` was fabricated. See report.md → "Independent Audit Evidence — PASS 2".

## Reproduction Steps (diagnostic — verified at HEAD `9638b065`)
1. Confirm no PKCE / OAuth2 user-context flow exists anywhere in the connector:
   `grep -rniE 'pkce|code_verifier|code_challenge|oauth2|refresh_token|/oauth2/token' internal/connector/twitter/` → exit 1 (no matches).
2. Confirm the implementation applies one static App-Only bearer to every request:
   `grep -n 'Authorization' internal/connector/twitter/api.go` → `141: req.Header.Set("Authorization", "Bearer "+c.bearerToken)` reached by `buildRequest` for `fetchUsersMe` and every paginated endpoint.
3. Confirm the requirement: `grep -n 'User-Context OAuth 2.0 with PKCE' specs/056-twitter-api-connector/spec.md` → line 225 (NC-1) plus design.md:131-133 endpoint matrix.
4. Confirm the false delivered-claim: `grep -n 'App-Only bearer + User-Context PKCE' specs/056-twitter-api-connector/report.md` → line 7 (re-quoted line 342).
5. (GAP-G2) Confirm the missing gauge: `grep -rniE 'x-rate-limit-remaining|RateLimitRemaining' internal/connector/twitter/ internal/metrics/` → exit 1; only `ConnectorTwitterAPIRateLimitReset` exists, written only inside the 429 branch (`api.go:530-534`).

> The live-API 403 against the real Twitter/X endpoints was **NOT** reproduced here — that requires real user-context credentials and is deferred with the fix. The gap is proven structurally by the code/spec divergence above (the connector has no mechanism to obtain or send a user-context token), which is conclusive.

## Expected Behavior
`/2/users/me`, `/2/users/:id/bookmarks`, and `/2/users/:id/liked_tweets` are fetched using a **User-Context OAuth 2.0 access token** obtained via the PKCE authorization-code flow (`code_verifier`/`code_challenge` → `POST /2/oauth2/token`), with encrypted refresh-token persistence and refresh-on-401/expiry. App-Only bearer remains for the public `/2/users/:id/tweets` and `/2/users/:id/mentions`. Spec 056's reports describe only what actually shipped.

## Actual Behavior
A single static App-Only bearer token (`apiClient.bearerToken`, `api.go:62`) is attached to **every** request by `buildRequest` (`api.go:117,141`). There is no authorization-code flow, no `code_verifier`/`code_challenge`, no token exchange (`POST /2/oauth2/token`), and no refresh-on-expiry. Against the real API the three user-owned endpoints would return 403, so bookmarks and likes cannot be ingested. report.md:7 nonetheless claims PKCE was delivered.

## Environment
- Service: smackerel-core (Go connector) — `internal/connector/twitter`
- Version: HEAD `9638b065`
- Affected surface: `internal/connector/twitter/api.go` (`apiClient`, `buildRequest`, `fetchUsersMe`, `fetchEndpointPaginated`, `doWithRetry`)
- Observability surface (G2): `internal/metrics/metrics.go` (`ConnectorTwitterAPIRateLimitReset`)

## Error Output
```
(real Twitter/X API, App-Only bearer against a user-owned endpoint — NOT reproduced here; documented expected response)
GET /2/users/:id/bookmarks  ->  HTTP 403 Forbidden
{"title":"Unsupported Authentication","detail":"Authenticating with OAuth 2.0 Application-Only is forbidden for this endpoint. Supported authentication types are [OAuth 1.0a User Context, OAuth 2.0 User Context].","status":403}
```
The structural root cause (no user-context flow exists) is proven by the grep evidence in report.md → Diagnostic Evidence; the live 403 is the documented consequence, not a fabricated execution result.

## Root Cause
Spec/design correctly resolved NC-1 to mandate User-Context OAuth 2.0 PKCE, but the implementation phase shipped only the App-Only bearer path (scopes 01-03 built `apiClient` around a single `bearerToken` field). Certification did not catch the divergence because: (a) the unit/integration tests exercise App-Only fixtures via `httptest.Server` and never hit a real user-context endpoint, so an App-Only request "passes" against the fake server; and (b) the live PKCE arms are env-gated SKIPs (`api_live_test.go`), so the missing flow is never executed. The result is a connector that builds and tests green while being unable to authenticate to its headline endpoints.

## Artifact-Integrity Issue (false delivered-claim — MUST be corrected at closure)
`specs/056-twitter-api-connector/report.md:7` states: *"Spec 056 delivered the Twitter API v2 connector path (App-Only bearer + User-Context PKCE) covering 4 endpoints"* (re-quoted at report.md:342). PKCE was never implemented (GAP-G1). This claim is false and masks the gap. The bug closure (delivery pass) MUST correct spec 056's claims to match the shipped reality — either by delivering PKCE (making the claim true) or by de-scoping the user-owned endpoints and restating the capability as App-Only-public-endpoints-only. **This packet does NOT edit parent report.md** (create-only; parent artifacts untouched); the correction is recorded as a delivery-pass DoD item.

## Related
- Feature: `specs/056-twitter-api-connector/` (parent — status `done`, untouched by this bug)
- Requirement anchors: `specs/056-twitter-api-connector/spec.md:225` (NC-1), `:111` (R-016), `:52`
- Design anchors: `specs/056-twitter-api-connector/design.md:131-133` (endpoint auth matrix), `:340` (NC-1 resolution row)
- Implementation anchors: `internal/connector/twitter/api.go:62,117,141` (static bearer), `:530-534,636-637` (reset-only gauge)
- Metrics anchor: `internal/metrics/metrics.go:105` (`ConnectorTwitterAPIRateLimitReset`)
- False-claim anchor: `specs/056-twitter-api-connector/report.md:7,342`
- Sibling bug (different defect, done): `specs/056-twitter-api-connector/bugs/BUG-056-001-twitter-empty-page-cursor-advance/`
- Diagnostic origin: reconcile-to-doc gaps phase (orchestrator-independently-verified)

## Deferred Reason
The remediation is substantial and gated on a maintainer **product decision** (design.md Open Question Q1): either (a) build the full User-Context OAuth 2.0 PKCE authorization-code flow now — a sizeable scope that NC-1 itself anticipated ("scopes.md MAY split the PKCE flow into its own scope") — or (b) formally de-scope bookmarks/likes/users-me to a future spec and correct spec 056's claims to App-Only-public-endpoints-only. Choosing between shipping new auth machinery vs. reducing advertised scope is a deliberate product call the maintainer must own; it is not a proportionate sweep-round drive-by. Priority: High; deploy-blocking for any operator who intends to ingest Twitter bookmarks or likes. Default users syncing only public tweets/mentions are unaffected.
