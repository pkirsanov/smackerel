# Feature 060 — Bearer Auth Scope Claim & RequireScope Middleware (Spec 044 Amendment)

> **Author:** bubbles.analyst
> **Date:** May 28, 2026
> **Status:** Draft (analyst bootstrap — ready for `bubbles.design`)
> **Workflow Mode:** full-delivery (proposed)
> **Amends:** [specs/044-per-user-bearer-auth/](../044-per-user-bearer-auth/) (Status: Done)
> **Triggered by:** [specs/058-chrome-extension-bridge/](../058-chrome-extension-bridge/) OQ-DSN-1 (blocking spec 058 advancement past `specs_hardened`)

---

## Related

- **Amends:** [specs/044-per-user-bearer-auth/spec.md](../044-per-user-bearer-auth/spec.md) — adds `scope` claim to the PASETO claim set; spec 044 is Done and not edited in place per Bubbles terminal-spec discipline.
- **Unblocks:** [specs/058-chrome-extension-bridge/design.md §12.1](../058-chrome-extension-bridge/design.md) — extension ingestion endpoint requires `auth.RequireScope("extension:bookmarks,history")` enforcement.
- **Closure path for:** OQ-DSN-1 (spec 058 design open question).

---

## Problem Statement

Spec 044 established the per-user PASETO bearer-token trust boundary with
claim-binding for `actor_id`, `owner_user_id`, and `actor_source`. The
claim set it shipped is **identity-only**: it answers *who* the caller is,
not *what surface* the token was minted for. Every per-user token issued
through `./smackerel.sh auth enroll` is therefore equivalent to every
other per-user token for the same user across all authenticated endpoints.

Spec 058 (Chrome Extension Bridge) is the first downstream feature that
**requires** a capability split inside one user's token set:

- The Chrome extension is sideloaded into the operator's browser process,
  which is a higher-exposure environment than a backend-issued token. A
  leaked extension token MUST NOT be usable against, e.g., the photo
  reveal mint endpoint, the cloud-drive Connect endpoint, the annotation
  endpoint, or any future per-user surface.
- Spec 058 NC-1 ("reuse spec 044, do NOT mint a parallel per-extension
  token type") binds the extension to spec 044's enrollment flow but
  explicitly defers the scope-segregation mechanism back to spec 044 as
  OQ-DSN-1.
- Spec 058 `design.md` §12.1 captures the routing packet: extend the
  PASETO claim set with `scope` (CSV / array of strings), parse it into
  `auth.Session.Scopes`, and ship `auth.RequireScope(scope...)`
  middleware. Without that contract, spec 058 cannot advance past
  `specs_hardened`.

Spec 044 itself anticipated this. Its Non-Goals carve out RBAC ("This
spec establishes *who* the caller is. *What* the caller is allowed to
do … is out of scope. Existing authorization checks remain unchanged.")
A capability claim is **not** RBAC across resources — it is a
token-issuance scoping primitive that prevents a token minted for one
surface from being replayed against another. That distinction is the
seam this spec lives in.

---

## Outcome Contract

**Intent:** Extend spec 044's PASETO claim set with a `scope` claim
(array of string capability names) and ship an `auth.RequireScope(...)`
middleware constructor, so a per-user token minted with
`scope: ["extension:bookmarks,history"]` is accepted by spec 058's
extension ingestion endpoint and rejected by every other authenticated
endpoint — and vice versa, a token minted without that scope is rejected
by the extension endpoint with `403 scope_required`.

**Success Signal:** A `production` deployment with at least one enrolled
user can:

1. Mint a scoped token via `./smackerel.sh auth enroll --user <id> --scope extension:bookmarks,history` and observe the issued PASETO token's claim payload includes `scope: ["extension:bookmarks,history"]`.
2. POST to spec 058's `/v1/connectors/extension/ingest` with that token and receive `202 Accepted`.
3. POST to the same endpoint with a spec-044-vintage token that has no `scope` claim (or a `scope` claim that does not include `extension:bookmarks,history`) and receive `403 scope_required` with no side effects.
4. POST to any pre-existing authenticated endpoint (photo reveal mint, cloud-drive Connect, annotations) with the scoped extension token and have the request rejected at the middleware seam if the endpoint is wired with `auth.RequireScope` for a non-`extension:*` capability; endpoints **not** wired with `RequireScope` continue to accept the token unchanged (backward-compatible default).
5. Rotate the scoped token through the spec 044 grace-window contract and have the new token carry the same `scope` claim by default unless `--scope` is supplied again on rotation.
6. Revoke the scoped token via the spec 044 revocation contract and observe the next request fails authentication identically to a non-scoped token revocation.
7. Inspect logs/metrics and see `auth_scope_rejected_total{required_scope, user_id}` increment on every scope mismatch.

**Hard Constraints:**

- **Backward compatibility with spec 044 tokens already in circulation.** Tokens minted before this spec lands have no `scope` claim. They MUST continue to authenticate against every endpoint that is NOT wired with `auth.RequireScope`. Endpoints wired with `auth.RequireScope` MUST reject them with `403 scope_required` (missing-claim is treated as scope mismatch, never as "all-scopes"). No silent upgrade path that grants implicit scopes to legacy tokens.
- **`auth.RequireScope` is additive, not retroactive.** Existing endpoints are NOT auto-wired with scope requirements by this spec. Wiring is per-endpoint and explicit; spec 058 is the first caller. Future specs MAY wire their own endpoints with their own scope names.
- **Scope namespace convention is enforced at enrollment time.** Scope names MUST match `^[a-z][a-z0-9]*:[a-z0-9,_-]+$` (surface:capability,capability). The enrollment CLI MUST reject unknown surfaces unless `--allow-unknown-surface` is supplied (operator escape hatch for new specs that haven't registered yet).
- **Hot-path validation budget is unchanged.** Adding scope parsing MUST NOT introduce a DB roundtrip or any per-request I/O beyond what spec 044 already does. Scope match is a string-set containment check against claims parsed from the PASETO payload.
- **Dev/test ergonomic survives.** The single-tenant `SMACKEREL_AUTH_TOKEN` bypass for `development` and `test` environments documented in spec 044 MUST continue to satisfy `auth.RequireScope` checks (or `auth.RequireScope` MUST be a no-op when the dev/test bypass is active). The choice between those two is a design decision; the constraint is that dev/test workflows do NOT need to mint scoped tokens to run integration tests.
- **No new SST keys without justification.** This spec SHOULD avoid adding new `config/smackerel.yaml` keys; scope is a per-token enrollment property, not a deployment-level config. If a config key is genuinely required (e.g., a registered-scope allowlist), it MUST be justified explicitly in `design.md` and follow NO-DEFAULTS / fail-loud SST policy.
- **Revocation / rotation contracts inherited unchanged.** This spec does NOT modify spec 044's revocation broadcaster, rotation grace window, signing-material discipline, or claim-binding rules. It only extends the claim shape and adds a middleware constructor.

**Failure Condition:** If a leaked extension token can be used to call
any endpoint wired with `auth.RequireScope` for a non-`extension:*`
capability, the spec has failed. If a legacy spec-044 token (no `scope`
claim) silently satisfies `auth.RequireScope` against any wired
endpoint, the spec has failed. If hot-path validation latency regresses
beyond spec 044's documented budget, the spec has failed. If
`development`/`test` integration tests are broken by this change, the
spec has failed.

---

## Goals

- **G1** — Extend the PASETO claim payload defined by spec 044 with a `scope` claim (array of strings), wire-level optional for backward compatibility but enforced at the `auth.RequireScope` middleware seam.
- **G2** — Add a `--scope <csv>` flag to the enrollment / rotation CLI (`./smackerel.sh auth enroll`, `./smackerel.sh auth rotate`) and persist scope into the issued token's claim payload at mint time.
- **G3** — Export `auth.RequireScope(scope ...string) func(http.Handler) http.Handler` (signature TBD by `bubbles.design`) that, when chained after spec 044's bearer middleware, asserts the authenticated session carries ALL of the required scopes; mismatch returns `403 scope_required`.
- **G4** — Define the scope namespace convention (`<surface>:<capability,capability>`) and the registered-surface allowlist mechanism (registered at code-load time per package init, or via a single canonical registry — design choice).
- **G5** — Add `auth_scope_rejected_total{required_scope, user_id}` counter and a structured log line on every rejection for operator forensics.
- **G6** — Provide a migration note in `docs/Operations.md` describing how operators issue scoped tokens for the spec 058 extension without disturbing existing tokens.
- **G7** — Unblock spec 058 OQ-DSN-1: ship `auth.Session.Scopes` and `auth.RequireScope` exported and documented before spec 058's `/v1/connectors/extension/ingest` handler lands.

---

## Non-Goals

- **General-purpose RBAC.** This spec does NOT introduce per-resource permissions, roles, ACLs, or per-action authorization beyond the token-minting-time capability split. Endpoints continue to do their own resource-level authorization checks.
- **Scope hierarchies / wildcards.** `extension:*` does NOT automatically satisfy `extension:bookmarks,history`. Scope match is exact string-set containment. (A future spec MAY add hierarchy if a real need surfaces.)
- **Scope revocation as a separate mechanism.** Scopes are baked into the token at mint time. To change a token's scopes, rotate and re-mint with new `--scope`. No partial-scope revocation surface.
- **Scope claims on the `SMACKEREL_AUTH_TOKEN` dev/test path.** Dev/test bypass either satisfies all scope requirements or skips them entirely (design choice); it does NOT carry a scope claim because it is not a PASETO token.
- **Migration of legacy tokens to scoped tokens.** Operators re-enroll users with `--scope` when they need scoping. No batch migration tool.
- **Per-endpoint default scope.** Wiring `auth.RequireScope` to an endpoint is an explicit per-spec decision. Spec 058 wires the extension ingest endpoint; this spec does NOT wire any pre-existing endpoint.

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Operator** | Self-hosted Smackerel deployment owner; enrolls users and rotates tokens via the CLI on the deploy host | Mint extension-scoped tokens for the human user without re-issuing or replacing existing identity tokens | Runs `./smackerel.sh auth enroll --scope ...` on the deploy host; reads structured logs |
| **Human user** | Holds one or more per-user PASETO tokens; pastes the extension-scoped token into the Chrome extension options page | Have the extension authenticate against the spec 058 endpoint; have the same token rejected if accidentally pasted into another smackerel client | Pastes token into extension storage; never sees the claim payload |
| **`bearerAuthMiddleware`** | Existing spec 044 middleware that verifies signature, parses claims, populates `auth.Session` on the request context | Unchanged validation behavior; additionally surface `Scopes []string` on `auth.Session` | Reads PASETO claims; writes session into request context |
| **`auth.RequireScope` middleware** | NEW middleware constructor that asserts the request's `auth.Session.Scopes` contains all required scopes | Reject with `403 scope_required` on mismatch; pass through on match | Reads `auth.Session` from context; writes response on rejection |
| **Spec 058 extension ingest handler** | Downstream consumer; wires `auth.RequireScope("extension:bookmarks,history")` between `bearerAuthMiddleware` and its own handler | Be guaranteed every request reaching its handler carries the required scope | Receives requests only after both middleware layers pass |

---

## Use Cases

### UC-001 — Operator mints an extension-scoped token

- **Actor:** Operator
- **Preconditions:** Spec 044 is deployed; user is already enrolled OR being enrolled fresh
- **Main Flow:**
  1. Operator runs `./smackerel.sh auth enroll --user alice --scope extension:bookmarks,history` on the deploy host
  2. CLI validates scope name against `^[a-z][a-z0-9]*:[a-z0-9,_-]+$`
  3. CLI mints a PASETO token with the spec 044 claim set plus `scope: ["extension:bookmarks,history"]`
  4. CLI prints the token to stdout once (per spec 044 contract)
  5. Operator copies the token to the human user via an out-of-band secure channel
- **Alternative Flows:**
  - 2a. Scope name fails regex → CLI exits non-zero with `invalid scope name` and does NOT mint
  - 2b. Surface segment (`extension`) not in registered-surface allowlist AND `--allow-unknown-surface` not supplied → CLI exits non-zero with `unknown scope surface` and does NOT mint
- **Postconditions:** Token is valid for any endpoint NOT wired with `auth.RequireScope` AND for endpoints wired with `auth.RequireScope("extension:bookmarks,history")`; token is rejected by endpoints wired with any other scope

### UC-002 — Human user authenticates extension against spec 058 endpoint

- **Actor:** Human user
- **Preconditions:** UC-001 completed; user has pasted the token into the Chrome extension options page; extension is ready to POST a diff
- **Main Flow:**
  1. Extension POSTs to `/v1/connectors/extension/ingest` with `Authorization: Bearer <scoped-token>`
  2. `bearerAuthMiddleware` verifies signature, parses claims, populates `auth.Session{Scopes: ["extension:bookmarks,history"]}` on the request context
  3. `auth.RequireScope("extension:bookmarks,history")` reads `auth.Session.Scopes`, confirms containment, passes through
  4. Spec 058 handler accepts the payload and returns `202 Accepted`
- **Alternative Flows:**
  - 3a. Token has no `scope` claim (legacy spec 044 token) → `auth.RequireScope` rejects with `403 scope_required`, increments `auth_scope_rejected_total{required_scope="extension:bookmarks,history", user_id=alice}`
  - 3b. Token has `scope: ["other:capability"]` → same as 3a
- **Postconditions:** Valid extension token ingests one diff; legacy or wrong-scope token is rejected with no side effects

### UC-003 — Operator rotates a scoped token, scope is preserved

- **Actor:** Operator
- **Preconditions:** UC-001 completed; user holds an extension-scoped token
- **Main Flow:**
  1. Operator runs `./smackerel.sh auth rotate --user alice` on the deploy host
  2. Rotation reads the prior token's claim payload, preserves the `scope` claim, issues a new token with the same scopes
  3. Spec 044's grace window applies as before
- **Alternative Flows:**
  - 1a. Operator runs `./smackerel.sh auth rotate --user alice --scope ""` → new token has no `scope` claim (operator explicitly demotes to legacy)
  - 1b. Operator runs `./smackerel.sh auth rotate --user alice --scope api:full,extension:bookmarks,history` → new token has the new scope set; prior token continues to honor its own scopes during the grace window
- **Postconditions:** Rotation does not silently strip scopes; explicit `--scope` argument is the only way to change them

### UC-004 — Legacy unscoped token continues to authenticate against unwired endpoints

- **Actor:** Human user holding a pre-amendment spec 044 token
- **Preconditions:** Token was issued before this spec landed; has no `scope` claim
- **Main Flow:**
  1. User calls `GET /v1/photos/...` (an endpoint NOT wired with `auth.RequireScope`)
  2. `bearerAuthMiddleware` populates `auth.Session{Scopes: []}` (empty slice for missing claim)
  3. Handler runs as today — no scope check applied
- **Postconditions:** Backward compatibility is preserved; operator is not forced to re-enroll every user

---

## Business Scenarios

### BS-001 — Scoped token accepted on wired endpoint

```gherkin
Given a `production` deployment running spec 044 + this amendment
And user alice has been enrolled with `--scope extension:bookmarks,history`
And spec 058's `/v1/connectors/extension/ingest` is wired with `auth.RequireScope("extension:bookmarks,history")`
When alice's Chrome extension POSTs a valid bookmarks diff to that endpoint with her scoped token
Then the response is `202 Accepted`
And the request reaches the spec 058 handler
```

### BS-002 — Legacy token rejected on wired endpoint

```gherkin
Given a `production` deployment running spec 044 + this amendment
And user bob holds a pre-amendment spec 044 token (no `scope` claim)
And spec 058's `/v1/connectors/extension/ingest` is wired with `auth.RequireScope("extension:bookmarks,history")`
When bob's client POSTs to that endpoint with his legacy token
Then the response is `403 scope_required`
And `auth_scope_rejected_total{required_scope="extension:bookmarks,history", user_id="bob"}` increments by 1
And a structured log line is emitted with the rejection reason
And the spec 058 handler is NOT invoked
```

### BS-003 — Cross-scope replay rejected

```gherkin
Given a `production` deployment running spec 044 + this amendment
And user alice holds a token minted with `--scope extension:bookmarks,history`
And a hypothetical future endpoint `/v1/admin/users` is wired with `auth.RequireScope("admin:users")`
When an attacker who stole alice's extension token POSTs to `/v1/admin/users`
Then the response is `403 scope_required`
And `auth_scope_rejected_total{required_scope="admin:users", user_id="alice"}` increments by 1
```

### BS-004 — Unwired endpoint remains backward compatible

```gherkin
Given a `production` deployment running spec 044 + this amendment
And user alice holds a token minted with `--scope extension:bookmarks,history`
And the photo reveal mint endpoint is NOT wired with `auth.RequireScope`
When alice's scoped token is used to call the photo reveal mint endpoint
Then the request is authenticated by spec 044's middleware
And no scope check is applied
And the handler runs as it does today
```

### BS-005 — Invalid scope name rejected at enrollment

```gherkin
Given a `production` deployment running spec 044 + this amendment
When the operator runs `./smackerel.sh auth enroll --user carol --scope "ExtensionBookmarks"`
Then the CLI exits non-zero with `invalid scope name: must match ^[a-z][a-z0-9]*:[a-z0-9,_-]+$`
And no token is minted
And no audit entry is created
```

### BS-006 — Unknown surface rejected unless escape hatch supplied

```gherkin
Given a `production` deployment running spec 044 + this amendment
And the registered-surface allowlist contains only `extension`
When the operator runs `./smackerel.sh auth enroll --user dave --scope "future-surface:capability"`
Then the CLI exits non-zero with `unknown scope surface: future-surface`
And no token is minted

When the operator re-runs with `--allow-unknown-surface`
Then the CLI mints the token with the unknown-surface scope
And a warning is logged
```

### BS-007 — Dev/test bypass satisfies scope requirements

```gherkin
Given a `development` deployment with the `SMACKEREL_AUTH_TOKEN` dev/test bypass active
And spec 058's `/v1/connectors/extension/ingest` is wired with `auth.RequireScope("extension:bookmarks,history")`
When an integration test POSTs to that endpoint with the dev/test bypass token
Then the request is accepted as today
And no scope rejection metric is incremented
```

### BS-008 — Rotation preserves scope by default

```gherkin
Given user alice holds a token minted with `--scope extension:bookmarks,history`
When the operator runs `./smackerel.sh auth rotate --user alice` with no `--scope` argument
Then the new token carries `scope: ["extension:bookmarks,history"]`
And the prior token remains valid until the spec 044 grace window elapses
```

### BS-009 — Rotation can explicitly demote to no scope

```gherkin
Given user alice holds a token minted with `--scope extension:bookmarks,history`
When the operator runs `./smackerel.sh auth rotate --user alice --scope ""`
Then the new token has no `scope` claim
And POSTs with the new token to wired endpoints fail with `403 scope_required`
And POSTs with the new token to unwired endpoints succeed
```

---

## Competitive Analysis

Token-scope claims are an industry-standard primitive; this section
documents the prior art that informed the design choice and confirms
this amendment stays inside well-trodden ground.

| Capability | Smackerel (after this spec) | OAuth 2.0 (`scope`) | GitHub Personal Access Tokens (fine-grained) | AWS IAM session tags |
|------------|-----------------------------|---------------------|-----------------------------------------------|----------------------|
| Per-token capability claim | `scope: [...]` array in PASETO | `scope` space-separated string in access token | Repository + permission scope at mint time | Session-tag map signed into STS token |
| Enforcement seam | `auth.RequireScope(...)` middleware | Resource server scope check | API endpoint scope check | IAM policy condition |
| Hierarchy / wildcards | NO (exact match) | Provider-defined (often hierarchical) | NO (explicit per-permission) | NO (tag match) |
| Default for legacy tokens | Reject on wired endpoints | Reject (no `scope` claim → no access) | Tokens always have explicit scopes | Tokens always have explicit tags |
| Rotation preserves scope | YES (unless `--scope` supplied) | Refresh tokens carry original scope | Re-issue requires re-specifying | Re-issue requires re-specifying |

**Differentiation rationale:** Smackerel's choice is the conservative
subset of OAuth 2.0's `scope` semantics — no hierarchy, no wildcards,
exact string-set containment. This matches the single-tenant,
operator-issued posture (no third-party app authorization layer) and
keeps the validation path trivially auditable.

---

## Platform Direction & Market Trends

### Industry Trends

| Trend | Status | Relevance | Impact on Product |
|-------|--------|-----------|-------------------|
| Token-scoped capabilities for browser extensions / personal automation tools | Established | High | Validates the `extension:*` namespace as a long-lived design choice; future automation specs (mobile capture, desktop watcher) will mint their own `<surface>:<capability>` tokens through the same enrollment path |
| Move away from monolithic PATs toward fine-grained capability tokens (GitHub fine-grained PATs, AWS session tags) | Growing | High | Confirms the per-token scope claim (vs a separate token type per surface) is the durable choice |
| PASETO / branca over JWT for self-hosted single-tenant deployments | Growing | Medium | Spec 044's PASETO choice already aligns; this amendment inherits that posture |

### Strategic Opportunities

| Opportunity | Type | Priority | Rationale |
|-------------|------|----------|-----------|
| Scope-claim primitive ready before spec 058 ships | Table Stakes | High | Without it, spec 058 is blocked at `specs_hardened` and the extension cannot ship |
| Reusable middleware for future per-surface specs (mobile capture, desktop watcher, automation API) | Differentiator | Medium | One enrollment flow, many scoped tokens, single rejection surface — reduces per-spec auth boilerplate |

### Recommendations

1. **Immediate (this work):** Ship the `scope` claim + `auth.RequireScope` middleware so spec 058 can advance past `specs_hardened`.
2. **Near-term:** When spec 033 (mobile capture) or any future per-surface spec is picked up, wire its endpoints with `auth.RequireScope("<surface>:<capability>")` and re-use the same enrollment flow.
3. **Strategic:** Resist the temptation to add scope hierarchies or wildcards until at least three independent surfaces have shipped scoped tokens and the patterns are clear.

---

## UI Scenario Matrix

No end-user UI ships with this spec. Operator surface is the existing
`./smackerel.sh auth` CLI extended with `--scope` and
`--allow-unknown-surface` flags.

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Surface |
|----------|-------|-------------|-------|-------------------|---------|
| Mint scoped token | Operator | `./smackerel.sh auth enroll` | `--user <id> --scope extension:bookmarks,history` | Token printed once with scope claim | CLI |
| Rotate scoped token | Operator | `./smackerel.sh auth rotate` | `--user <id>` (preserves scope) or `--user <id> --scope <csv>` (replaces) | New token issued; grace window starts | CLI |
| Inspect scope rejection forensics | Operator | structured logs + Prometheus | `auth_scope_rejected_total{required_scope, user_id}` | Counter increments per rejection; one structured log line per rejection | Logs + metrics |

---

## Non-Functional Requirements

- **Performance:** Scope-match check MUST add ≤ 10 µs p99 per request relative to spec 044's bearer middleware baseline. No DB roundtrip, no allocation beyond the parsed claims slice already present.
- **Security:** Missing-claim semantics are "reject on wired endpoints", never "all-scopes". Scope names are validated against the namespace regex at enrollment time AND at parse time on the hot path (defense in depth). Rejection responses MUST NOT leak the required scope value to anonymous callers (the `403 scope_required` body MAY include the required scope only when the bearer token successfully authenticated but failed scope; for anonymous calls, the response is plain `401`).
- **Observability:** `auth_scope_rejected_total{required_scope, user_id}` counter; one structured log line per rejection with fields `event=scope_rejected`, `required_scope`, `user_id`, `token_scopes`, `endpoint`, `request_id`.
- **Backward compatibility:** Legacy spec 044 tokens (no `scope` claim) MUST authenticate against every unwired endpoint identically to today. Wiring is opt-in per endpoint.
- **Migration:** No database migrations; no data backfill; scope is per-token at mint time and the token store already accommodates arbitrary claim payloads.
- **Documentation:** `docs/Operations.md` MUST gain a "Scoped Token Enrollment" subsection; `docs/API.md` MUST document the `403 scope_required` response and which endpoints are wired (initially: only the spec 058 extension ingest endpoint).
- **Test coverage:** Go unit tests for scope-name validation, claim parsing, and middleware behavior; integration test asserting BS-001 through BS-009; adversarial regression test asserting BS-002 (legacy token rejected on wired endpoint) cannot be silently weakened.

---

## Domain Capability Model

Capability-first proportionality triggers apply: this spec introduces
the **scoped-capability token** primitive that future per-surface specs
will re-use as a foundation. AN5 requires a capability model.

### Domain Primitives

- **Token capability claim** — a string from the namespace
  `<surface>:<capability,capability>` carried in the PASETO `scope`
  claim. Examples: `extension:bookmarks,history`, `mobile:capture`,
  `admin:users`.
- **Scope-enforcement seam** — the boundary at which a request handler
  asserts the authenticated session carries a required capability.
  Realized as `auth.RequireScope(...)` middleware.
- **Registered-surface allowlist** — the set of `<surface>` prefixes
  known to the binary at start time. Prevents typos at enrollment time;
  bypassable with `--allow-unknown-surface` for forward compatibility.

### Lifecycle States

| Token state | Description | Transition |
|-------------|-------------|------------|
| `unscoped` | Legacy spec 044 token; claim payload has no `scope` field | Re-mint with `--scope` to migrate; rotation does NOT auto-add scope |
| `scoped` | PASETO claim payload includes `scope: [...]` | Rotation preserves by default; `--scope ""` demotes back to unscoped |
| `revoked` | Spec 044 revocation broadcaster has invalidated the token | Inherited from spec 044 unchanged |

### Relationships

- One **user** holds zero or more **tokens**.
- One **token** carries zero or more **capability claims**.
- One **endpoint** is wired with zero or more **required scopes** (logical AND across required scopes).
- One **specs/058 ingest endpoint** is wired with exactly one required scope: `extension:bookmarks,history` (cardinality matches spec 058 NC-1).

### Business Policies (Every Implementation Must Obey)

1. **Missing-claim is scope mismatch.** Never treat a missing `scope` claim as "all scopes".
2. **Exact string-set containment.** No hierarchy, no wildcards, no case folding.
3. **Wiring is explicit per endpoint.** This spec does NOT auto-wire any pre-existing endpoint.
4. **Rotation preserves scope by default.** Operators must explicitly pass `--scope` to change it.
5. **Validation is dual-sided.** Scope names are validated at enrollment time AND at hot-path parse time.

### Provider-Neutral Behavior Vocabulary

- **MintScopedToken(user, scopes []string) → Token**
- **ParseScopes(claims) → []string**
- **RequireScope(required ...string) → middleware**
- **RejectWithScopeRequired(w, required) → 403 + metric increment**

---

## Open Questions for `bubbles.design`

- **OQ-DSN-AMD-1** — Should `auth.RequireScope` accept multiple required scopes as logical AND (every required scope must be present) or logical OR (any one required scope satisfies)? **Recommendation:** AND (matches OAuth 2.0 RFC 6749 §3.3 semantics and avoids the principle-of-least-privilege footgun of OR). Confirm during `bubbles.design`.
- **OQ-DSN-AMD-2** — Should `auth.RequireScope` no-op for the dev/test `SMACKEREL_AUTH_TOKEN` bypass, or should the bypass be treated as implicitly carrying all scopes? **Recommendation:** no-op when the bypass is active (simpler reasoning and matches "dev/test ergonomic survives" hard constraint). Confirm during `bubbles.design`.
- **OQ-DSN-AMD-3** — Where does the registered-surface allowlist live? Options: (a) compile-time `init()` registration per package that owns a surface; (b) single canonical registry in `internal/auth/scopes.go`; (c) config-driven via `smackerel.yaml`. **Recommendation:** (b) — single canonical registry to keep the auth surface auditable; new specs add their surface to the registry as part of their own implementation. Confirm during `bubbles.design`.

---

## Closure Mapping

This spec closes spec 058's OQ-DSN-1 by delivering:

- `auth.Session.Scopes []string` populated from the PASETO `scope` claim
- `auth.RequireScope(scope ...string)` middleware constructor
- `./smackerel.sh auth enroll --scope <csv>` and `./smackerel.sh auth rotate --scope <csv>` CLI flags
- Documented scope namespace convention and registered-surface allowlist mechanism
- Operator runbook entry in `docs/Operations.md`

Spec 058 MUST update its design doc to reference this spec (rather than
the spec 044 routing packet) once this spec advances to `specs_hardened`,
and MAY then advance past `specs_hardened` itself.
