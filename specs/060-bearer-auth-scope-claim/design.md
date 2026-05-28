# Feature 060 — Design — Bearer Auth Scope Claim & `auth.RequireScope` Middleware

> **Author:** bubbles.design
> **Date:** May 28, 2026
> **Status:** Draft (design — ready for `bubbles.plan`)
> **Spec:** [spec.md](spec.md)
> **Amends:** [specs/044-per-user-bearer-auth/design.md](../044-per-user-bearer-auth/design.md) (Done; not edited in place)
> **Unblocks:** [specs/058-chrome-extension-bridge/design.md §12.1](../058-chrome-extension-bridge/design.md)

---

## Design Brief

**Current State.** Spec 044 is shipped and `Done`. `internal/auth.IssueToken` mints a PASETO v4.public token with `iss`/`sub`/`jti`/`iat`/`nbf`/`exp` claims and a JSON footer `{"kid":"<KeyID>"}`. `internal/auth.VerifyAndParse` returns a `ParsedToken{UserID, TokenID, KeyID, IssuedAt, ExpiresAt}`. `internal/api.bearerAuthMiddleware` (router.go:626) verifies the token, performs revocation lookup, and pushes an `auth.Session{Source, UserID, TokenID, KeyID, IssuedAt, ExpiresAt}` onto the request context. There is no per-token capability claim and no `auth.RequireScope` middleware. Spec 058 cannot wire its `/v1/connectors/extension/ingest` endpoint without this seam.

**Target State.** Extend the PASETO claim payload with an OPTIONAL `scope` claim (JSON array of strings). Extend `auth.ParsedToken` and `auth.Session` with `Scopes []string`. Add `auth.RequireScope(required ...string) func(http.Handler) http.Handler` middleware that asserts logical-AND containment over `Session.Scopes`. Add `--scope` flag to the operator `auth enroll` and `auth rotate` subcommands; rotation preserves prior scopes by default. Add `auth_scope_rejected_total{required_scope,user_id}` counter. No new SST config keys; the registered-surface allowlist lives in a single canonical Go file (`internal/auth/scopes.go`).

**Patterns to Follow.**
- PASETO claim shape extension — extend `paseto.NewToken().Set(...)` in `internal/auth/issue.go::IssueToken` and pull via `token.Get(...)` in `internal/auth/verify.go::VerifyAndParse`. Follow the existing `GetIssuer/GetSubject/GetJti` pattern.
- Session population — extend the struct in `internal/auth/session.go` and the `auth.Session{...}` literal in `internal/api/router.go::bearerAuthMiddleware`.
- Middleware constructor — match the `func(next http.Handler) http.Handler` signature used by `bearerAuthMiddleware` so it composes via `chi`'s `r.Use(...)` and `r.With(...)`.
- Metrics — register a new `prometheus.CounterVec` in `internal/metrics/auth.go` following the `AuthValidationOutcome` pattern (closed-set label cardinality, registered in the same package-level `Register(...)` list).
- CLI flag plumbing — `flag.NewFlagSet(...).String(...)` against `auth enroll` / `auth rotate` in `cmd/core/cmd_auth.go`; thread through `auth.IssueAndPersistOptions` (new `Scopes []string` field) into `auth.IssueToken`.
- Fail-loud SST — anywhere the registered-surface allowlist is referenced, missing/empty registration fails the build (compile-time `var` slice), not a runtime fallback.

**Patterns to Avoid.**
- Do NOT introduce a parallel token type, parallel cookie, or parallel session source for the extension. Spec 058 NC-1 forbids it; spec 044's session machinery is reused unchanged.
- Do NOT add a `smackerel.yaml` key for the registered-surface allowlist (OQ-DSN-AMD-3 resolved (b), see §5). Config-driven allowlists fragment the auth audit surface across YAML and Go.
- Do NOT use `fmt.Sprintf` to splice the `scope` claim into the token footer or as a delimited string field on top of `iss`/`sub`. PASETO `Set(key, value)` with a `[]string` value carries native JSON-array semantics; the wire format is the canonical contract.
- Do NOT treat a missing `scope` claim as "all scopes". Missing-claim is mismatch; this is the spec 060 hard constraint and the entire reason the spec exists.
- Do NOT short-circuit the dev/test `SessionSourceSharedToken` bypass by setting `Session.Scopes` to a wildcard; the middleware bypass on `Source == SessionSourceSharedToken` is explicit (OQ-DSN-AMD-2 resolved no-op, see §5).
- Do NOT add scope hierarchy / wildcards in this spec. Spec 060 Non-Goals are explicit; future work may revisit.

**Resolved Decisions.**
- OQ-DSN-AMD-1 → AND semantics (see §5.1)
- OQ-DSN-AMD-2 → no-op when `Session.Source ∈ {SessionSourceSharedToken, SessionSourceBootstrap}` (see §5.2)
- OQ-DSN-AMD-3 → single canonical registry in `internal/auth/scopes.go` (see §5.3)
- Scope wire format → JSON array of strings under PASETO claim key `scope` (see §3.1)
- Storage → no DB schema change; scope lives only in the PASETO claim payload, not in `auth_tokens` rows (see §6)
- Rotation default → prior token's `scope` claim is parsed during rotation and replayed into the new token unless `--scope` is supplied (see §7.2)

**Open Questions.** None. All three OQ-DSN-AMD-* questions are resolved in §5.

---

## 1. Purpose & Scope

This design extends spec 044's per-user PASETO bearer-token surface with a capability split inside one user's token set. After this design ships:

- One PASETO token may carry one or more capability scopes (`<surface>:<capability,capability>`).
- One HTTP endpoint may require a fixed set of scopes via `auth.RequireScope(...)`.
- Tokens minted before this design lands (no `scope` claim) continue to authenticate against every endpoint NOT wired with `auth.RequireScope`; they are rejected by every endpoint that IS wired.
- Spec 058 can wire `/v1/connectors/extension/ingest` with `auth.RequireScope("extension:bookmarks,history")` and advance past `specs_hardened`.

Out of scope (inherited from spec.md Non-Goals): RBAC, scope hierarchy/wildcards, partial-scope revocation, scoped-claim shape on the dev/test bypass, batch migration of legacy tokens, default per-endpoint scope wiring.

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│  Operator CLI: `smackerel auth enroll --user alice --scope <csv>`    │
│  ─ flag parse ─► validate scope regex ─► validate surface allowlist  │
│                          │                                            │
│                          ▼                                            │
│  auth.IssueAndPersistToken(opts{..., Scopes:[...]} )                  │
│  ─ auth.IssueToken: token.Set("scope", scopes)                        │
│  ─ auth.HashToken; store.PersistToken (no schema change)              │
└──────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼  (PASETO wire token, claim payload
                                      now includes optional "scope":[...])
┌──────────────────────────────────────────────────────────────────────┐
│  Hot path (HTTP request):                                            │
│                                                                       │
│    chi route                                                          │
│      └─ bearerAuthMiddleware (spec 044, EXTENDED)                     │
│         └─ VerifyAndParse → ParsedToken{...,Scopes:[]string}          │
│         └─ Session{...,Scopes,Source}                                 │
│            └─ auth.WithSession(ctx, sess)                             │
│      └─ auth.RequireScope("extension:bookmarks,history") (NEW, §4)    │
│         └─ if Source ∈ {Shared,Bootstrap}: pass through (§5.2)        │
│         └─ else assertContainsAll(sess.Scopes, required)              │
│         └─ on mismatch: 403 scope_required + metric increment         │
│      └─ handler                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

Composition follows the existing `chi` pattern: `r.Group(...)` applies `bearerAuthMiddleware`; sub-routers that need scope enforcement apply `r.With(auth.RequireScope(...))`. `RequireScope` MUST run AFTER `bearerAuthMiddleware`; the constructor enforces this at runtime by reading `auth.SessionFromContext` and returning `500 missing_session` if absent (programming-error guard; never reached on correctly composed routes).

---

## 3. PASETO Claim Schema Extension

### 3.1 Wire Format

PASETO claim key: `scope`
Value type: JSON array of strings (`[]string` in Go).
Cardinality: optional. Missing = legacy spec-044 token.

Example claim payload (informative; the on-wire form is base64url-encoded PASETO payload bytes):

```json
{
  "iss": "smackerel",
  "sub": "alice",
  "jti": "9c1c5b...e7",
  "iat": "2026-05-28T12:00:00Z",
  "nbf": "2026-05-28T12:00:00Z",
  "exp": "2026-08-28T12:00:00Z",
  "scope": ["extension:bookmarks,history"]
}
```

Footer remains unchanged: `{"kid":"<KeyID>"}`. The `scope` claim lives in the **payload** (encrypted-and-authenticated for PASETO v4.local; signed-and-authenticated for v4.public, which is what spec 044 uses).

### 3.2 Scope Name Format

```
^[a-z][a-z0-9]*:[a-z0-9,_-]+$
```

- Surface segment: lowercase ASCII, starts with letter, no separators.
- Capability segment: lowercase ASCII alphanumerics plus `,` `_` `-`. Comma is the multi-capability separator within ONE surface (spec.md example: `extension:bookmarks,history`).
- Each element of the `scope` array is a single capability string; multi-element arrays carry capabilities across distinct surfaces (rare; the recommended pattern is one scope per token).
- Validation is dual-sided:
  - **Mint time** (CLI): regex check before calling `IssueToken`; reject with non-zero exit + `invalid scope name`.
  - **Parse time** (hot path): `VerifyAndParse` re-applies the regex to every element of the parsed `scope` claim. Malformed = treat the entire claim as missing (NOT as "all scopes"), increment a parse-error counter, log structured warning. This is defense-in-depth against a forged claim that somehow slipped past mint-time validation (e.g. a token issued by a downgraded binary).

### 3.3 Registered-Surface Allowlist

See §5.3 for OQ-DSN-AMD-3 resolution. The allowlist is checked at mint time in the CLI; bypassable with `--allow-unknown-surface`. It is NOT checked at parse time (a registered-surface check at parse time would break operator forward-compatibility on rolling upgrades where the issuer binary knows surface `mobile` but the verifier binary doesn't yet).

---

## 4. `auth.Session` Wiring & `auth.RequireScope` Middleware

### 4.1 `auth.Session.Scopes`

Extend `internal/auth/session.go`:

```go
type Session struct {
    UserID     string
    TokenID    string
    KeyID      string
    IssuedAt   time.Time
    ExpiresAt  time.Time
    Source     SessionSource
    Scopes     []string // NEW; nil for legacy tokens and shared/bootstrap sources
}
```

`Scopes` is a `[]string` (NOT a `map[string]struct{}`). Cardinality in production is expected to be small (1-3 scopes per token); a slice with a length-3 linear `slices.Contains` is faster than a map allocation on the hot path. Containment check inside `RequireScope` uses `slices.Contains` per required scope.

### 4.2 `auth.ParsedToken.Scopes`

Extend `internal/auth/verify.go::ParsedToken` symmetrically:

```go
type ParsedToken struct {
    UserID     string
    TokenID    string
    KeyID      string
    IssuedAt   time.Time
    ExpiresAt  time.Time
    Scopes     []string // NEW
}
```

`VerifyAndParse` populates `Scopes` via:

```go
var scopes []string
if raw, err := token.GetString("scope"); err == nil && raw != "" {
    // claim is JSON-encoded by go-paseto's Set("scope", []string); parse back
}
```

Note: `go-paseto`'s `token.Set(key, value)` JSON-marshals `value`; `token.GetString(key)` returns the JSON string. The implementation will use a small typed helper `getScopeClaim(token *paseto.Token) ([]string, error)` that:

1. Calls `token.Get("scope")` (returns `string`, error). If the underlying `paseto.Token` API does not expose typed array access, fall back to `token.GetString("scope")` and `json.Unmarshal([]byte(raw), &scopes)`.
2. Returns `(nil, nil)` when the claim is absent (the error is sentinel-checked; spec 044's `VerifyAndParse` already uses this pattern for `nbf`).
3. Validates each element against the §3.2 regex; on mismatch, returns `(nil, ErrScopeClaimMalformed)`. The caller (bearerAuthMiddleware) logs and proceeds with `Scopes = nil`; the request is treated as a legacy token.

### 4.3 `auth.RequireScope` Signature

```go
// RequireScope returns middleware that asserts the authenticated session
// in the request context carries every scope in `required`. The session
// is read from auth.SessionFromContext; bearerAuthMiddleware MUST run
// first. Logical AND across required scopes (spec 060 §5.1 / OQ-DSN-AMD-1).
//
// Special-case: when the session's Source is SessionSourceSharedToken
// or SessionSourceBootstrap, RequireScope passes through unchanged
// (spec 060 §5.2 / OQ-DSN-AMD-2). This preserves the dev/test ergonomic
// and keeps the one-shot bootstrap flow usable for enrolling the first
// scoped operator.
//
// On mismatch the response is 403 + body `{"error":"scope_required",
// "required":[...]}`, the `auth_scope_rejected_total` counter increments
// once per request (one label set per required scope that was missing
// from the session is too cardinality-hostile; the metric labels on the
// FIRST missing scope), and a structured log line is emitted.
//
// On anonymous calls (no session in context) RequireScope responds 500
// with `{"error":"middleware_misconfigured"}` — this is a programming
// error; bearerAuthMiddleware should have already rejected the request
// with 401 before RequireScope ever ran.
func RequireScope(required ...string) func(http.Handler) http.Handler
```

Signature rationale:

- Variadic `...string` mirrors the spec 058 call site (`auth.RequireScope("extension:bookmarks,history")`) and Go-idiomatic constructor pattern.
- Returns `func(http.Handler) http.Handler` (not a method on `*Dependencies`) so it can be composed by any router and unit-tested without the full `Dependencies` graph.
- `required` is captured by closure; the slice is treated as immutable. A zero-length `required` is a programming error (the constructor panics at registration time — never at request time).

### 4.4 Containment Check

```go
for _, want := range required {
    if !slices.Contains(sess.Scopes, want) {
        // 403 + metric + log on the FIRST missing scope.
        return
    }
}
```

Exact string match; no case folding, no prefix match, no wildcard. The metric label is the FIRST missing scope (avoids per-request label-set explosion under the common case where a legacy token is missing ALL required scopes).

### 4.5 Rejection Response

Status: `403 Forbidden`.

Body:
```json
{
  "error": "scope_required",
  "required": ["extension:bookmarks,history"]
}
```

The `required` field is included ONLY when the request successfully authenticated (i.e., `auth.SessionFromContext` returned `ok=true`). For unauthenticated requests `RequireScope` is unreachable in correct composition (bearer middleware already returned 401); if reached anyway, the body is `{"error":"middleware_misconfigured"}` and the status is 500 (see §4.3).

This satisfies spec.md NFR "Rejection responses MUST NOT leak the required scope value to anonymous callers" — the `required` field is only emitted post-authentication.

### 4.6 Structured Log Line

```
event=scope_rejected required_scope=<first-missing> user_id=<sess.UserID>
token_scopes=<comma-joined sess.Scopes> endpoint=<r.URL.Path>
request_id=<chi.RequestID>
```

`token_scopes` is joined with `,`; empty `Scopes` → empty string. The log severity is `WARN` (mirrors `bearerAuthMiddleware`'s rejection logging).

### 4.7 Composition Example (Spec 058 Site)

```go
r.Route("/v1/connectors/extension", func(r chi.Router) {
    r.Use(deps.bearerAuthMiddleware)
    r.With(auth.RequireScope("extension:bookmarks,history")).
        Post("/ingest", deps.handleExtensionIngest)
})
```

---

## 5. Resolved Open Questions

### 5.1 OQ-DSN-AMD-1 — AND vs OR semantics

**Decision: AND.** `RequireScope("a", "b")` requires the session to carry BOTH `a` and `b`.

Rationale:
- Matches OAuth 2.0 RFC 6749 §3.3 semantics (`scope` is a set; the resource server demands a subset is present).
- Principle of least privilege: an OR seam ("token has *any* of these capabilities") is footgunny — adding a new required scope SHOULDN'T weaken the gate.
- Real spec 058 call site only requires one scope (`extension:bookmarks,history`); AND is degenerate-correct here and forward-safe.
- If a future site genuinely needs OR-of-scopes, it can call `RequireScope` twice in alternative sub-routes, or a new helper `RequireAnyScope(...)` can be added. AND is the durable default.

### 5.2 OQ-DSN-AMD-2 — Dev/test bypass interaction

**Decision: No-op when `Source ∈ {SessionSourceSharedToken, SessionSourceBootstrap}`.**

`RequireScope` checks `sess.Source` first and passes through without any scope check when the source is the shared-token dev/test bypass OR the one-shot bootstrap session.

Rationale:
- "Implicit all-scopes" requires injecting a synthetic scope set into `Session.Scopes` somewhere, which couples the bypass to the scope vocabulary and forces every future surface to remember to extend the synthetic set.
- No-op is the simpler invariant: dev/test bypass means "scope enforcement is off for this session". The implication is consistent with the rest of spec 044's dev/test posture (relaxed claim binding, no revocation lookup against the shared token).
- Bootstrap session is one-shot and operator-driven; subjecting the very first enrollment to scope enforcement creates a chicken-and-egg problem when the first scoped token must be minted.
- The dev/test bypass is logged at runtime (existing spec 044 telemetry); `RequireScope`'s no-op path increments `auth_scope_check_bypassed_total{source}` so dashboards can detect drift (e.g., production accidentally taking the bypass path).

### 5.3 OQ-DSN-AMD-3 — Registered-surface allowlist location

**Decision: Single canonical registry in `internal/auth/scopes.go`.**

```go
// internal/auth/scopes.go
package auth

// RegisteredScopeSurfaces is the closed set of <surface> segments
// the CLI accepts at mint time without --allow-unknown-surface.
// New specs add their surface here as part of their implementation;
// the allowlist exists as a typo-catch, not as a security boundary
// (the hot-path verifier does NOT consult it — see §3.3).
var RegisteredScopeSurfaces = []string{
    "extension", // spec 058 — Chrome extension bridge
}
```

Rationale:
- One file to grep; one PR diff per new surface; auditable in a single read.
- Option (a) per-package `init()` registration spreads the registry across the codebase and creates a hidden import-order dependency.
- Option (c) `smackerel.yaml` key violates "No new SST keys without justification" hard constraint AND fragments the auth audit surface across YAML and Go.
- Adding a surface is a one-line code change reviewed alongside the spec that introduces it — exactly the audit trail this spec wants.
- The CLI imports `internal/auth` already (for `IssueAndPersistToken`); zero new import dependencies.

---

## 6. Storage & Persistence

**No schema change.** `auth_tokens` columns remain unchanged. Scope lives entirely in the PASETO claim payload, which is on the wire and inside the operator's possession; the database row stores `hashed_token` (HMAC of the wire token), not the claim payload.

Implications:
- `auth.BearerStore.PersistToken` signature is unchanged.
- Listing tokens via `smackerel auth list-users` does NOT show scopes (the database doesn't know them). Operators inspect a token's scopes by holding the token and running a `smackerel auth inspect <token>` introspection subcommand (NEW; trivial wrapper over `VerifyAndParse` that prints parsed claims). Spec 058 does not require this surface; it's added because operators will ask "what scopes does this token carry?" within the first week of shipping this spec.
- Rotation reads the prior token's scopes from the prior PASETO token, NOT from the database. The CLI rotation flow already requires `--prior-token-id`; for §7.2's "preserve scopes by default" behavior, the rotation flow MUST also accept the prior wire token (or the operator MUST pass `--scope` explicitly). See §7.2 for the resolved rotation contract.

---

## 7. CLI Surface

### 7.1 `auth enroll --scope`

```
smackerel auth enroll [--notes "..."] [--scope <csv>] [--allow-unknown-surface] <user-id>
```

Flag plumbing (`cmd/core/cmd_auth.go::runAuthEnroll`):

```go
scope := fs.String("scope", "", "comma-separated scope claims (e.g. extension:bookmarks,history)")
allowUnknown := fs.Bool("allow-unknown-surface", false, "permit a scope whose surface is not in RegisteredScopeSurfaces")
```

Parse semantics:
- Empty `--scope` (or omitted): no `scope` claim minted; legacy spec-044 token shape.
- Non-empty `--scope`: split on `,` AT THE TOP LEVEL is ambiguous because individual capability strings already use `,` internally (e.g. `extension:bookmarks,history`). Resolution: the `--scope` flag value is a **single** scope string per invocation. To mint a token with multiple scopes across distinct surfaces (rare), the operator passes `--scope` multiple times. Implementation: use `flag.Func("scope", ...)` to accumulate into `[]string`.

Validation:
1. For each accumulated scope string, apply the §3.2 regex. On mismatch, exit 2 with `invalid scope name: <bad>`.
2. Extract the surface segment (everything before `:`). If not in `auth.RegisteredScopeSurfaces` AND `--allow-unknown-surface` not supplied, exit 2 with `unknown scope surface: <surface>`. With `--allow-unknown-surface`, log a `WARN` and proceed.

Threading: pass the validated `[]string` into the new `auth.IssueAndPersistOptions.Scopes` field. `auth.IssueToken` calls `token.Set("scope", opts.Scopes)` ONLY when `len(opts.Scopes) > 0`.

### 7.2 `auth rotate --scope`

```
smackerel auth rotate --prior-token-id <id> [--scope <csv>] [--prior-token <wire>] [--allow-unknown-surface] <user-id>
```

Resolved rotation contract (refining spec.md UC-003):

- **No `--scope` argument:** preserve the prior token's scopes. The rotation flow REQUIRES `--prior-token <wire>` to read prior scopes from the prior token's claim payload. If `--prior-token` is not supplied AND the operator has not passed `--scope`, the CLI exits 2 with `rotation requires --prior-token <wire> to preserve scopes, or --scope to set them explicitly`. This is the trade-off for keeping scopes out of the database: rotation needs the prior wire token to preserve them. (Spec 044 operators already hold their tokens; this is not a new burden.)
- **`--scope` supplied (one or more times):** the supplied scopes are the new token's scopes. The prior wire token is NOT required. This is the operator's explicit replace path.
- **`--scope ""` supplied exactly once:** the new token has NO `scope` claim. This is BS-009's explicit "demote to legacy" path. Implementation: `flag.Func` accepts the empty string as a sentinel meaning "demote"; if any non-empty `--scope` appears alongside the empty sentinel, exit 2 with `--scope "" cannot be combined with non-empty --scope values`.

Threading: same as enrollment — validated `[]string` flows into `IssueAndPersistOptions.Scopes`.

### 7.3 `auth inspect` (Operator Affordance)

```
smackerel auth inspect <wire-token>
```

Prints parsed claims (issuer, subject, jti, iat, exp, kid, scopes) to stdout. Wraps `auth.VerifyAndParse` against the live SST signing key. Exit 1 on verify failure. Added because operators will need it within the first week to confirm what a freshly minted token carries.

### 7.4 `./smackerel.sh` Wrapper

The `./smackerel.sh` script does NOT currently wrap `auth` subcommands (spec 044 ran `auth` directly via the `smackerel` binary on the deploy host). Spec 060 does NOT add wrapping; the operator continues to invoke the binary directly inside the deploy host. The spec.md user-facing examples (`./smackerel.sh auth enroll --scope ...`) are documentation shorthand for "operator-driven mint flow"; the actual command on a deploy host is `smackerel auth enroll --scope ...`. The plan/implementation scope MUST decide whether to update spec.md UC text to match the binary's actual invocation OR to add a `smackerel.sh auth` passthrough wrapper. Recommendation: passthrough wrapper for operator ergonomic parity with the rest of the CLI surface; cost is ~10 lines in `smackerel.sh`. Defer to `bubbles.plan`.

---

## 8. Backwards Compatibility With Legacy Tokens

**Hard invariant:** A token minted before spec 060 ships (no `scope` claim) authenticates against every endpoint NOT wired with `auth.RequireScope` identically to spec 044's behavior today.

Mechanism:
- `VerifyAndParse` returns `Scopes: nil` for legacy tokens (the §4.2 `getScopeClaim` helper returns `(nil, nil)` on absent claim).
- `bearerAuthMiddleware` populates `Session.Scopes = nil`; no change to its rejection logic.
- Endpoints without `RequireScope` never read `Session.Scopes`; the legacy token flows through unchanged.
- Endpoints with `RequireScope`: `slices.Contains(nil, "extension:bookmarks,history") == false` → 403, metric increment, structured log.

**Explicitly rejected alternatives:**
- "Silent upgrade" where legacy tokens are treated as carrying a default scope set — violates spec.md hard constraint "no silent upgrade path that grants implicit scopes to legacy tokens".
- "Database-side backfill" where the runtime stamps a default scope set into the database for every existing token — violates the no-schema-change decision in §6 and creates a one-way ratchet.

The migration path is operator-initiated: re-enroll or rotate users with `--scope` when a wired endpoint needs them.

---

## 9. Security & Compliance

- **Constant-time check.** The `slices.Contains` comparison is NOT constant-time. This is acceptable: the strings being compared are the operator's own scope vocabulary (`extension:bookmarks,history`) and the authenticated session's scopes. Timing leakage reveals which scopes a session carries; this is information the session OWNER already possesses. Spec 044's constant-time discipline is preserved for the signature verification and shared-token compare; scope check is post-authentication and timing-safe-by-context.
- **Missing-claim semantics.** Codified in §8; missing = mismatch, never = all-scopes. Defense-in-depth applies the regex re-check at parse time (§3.2).
- **Forged-claim resistance.** PASETO v4.public signs the payload; a tampered `scope` claim invalidates the signature. The `getScopeClaim` parse-time regex catches a `scope` array that contains malformed elements (impossible without operator-side mint, but defended anyway).
- **Cross-scope replay.** BS-003 in spec.md — a stolen extension token cannot reach `/v1/admin/users` if that route is wired with `auth.RequireScope("admin:users")`. This is the entire point of the spec.
- **Authorization matrix (initial wiring; spec 060 ships ZERO endpoint wiring of pre-existing endpoints):**

| Endpoint | bearerAuthMiddleware | RequireScope | Wired by |
|----------|---------------------|--------------|----------|
| `/v1/connectors/extension/ingest` (spec 058) | YES | `extension:bookmarks,history` | spec 058 implementation |
| All other existing per-user endpoints | YES | (none — backward compatible) | n/a |
| `/v1/auth/*` admin surface | YES | (none — admin gating is the existing SST allowlist) | spec 044 |
| `/healthz`, `/metrics` | per spec 044 router config | (none) | spec 044 |

Spec 060 explicitly does NOT wire `RequireScope` on any pre-existing endpoint. Spec 058 is the first caller.

---

## 10. Configuration & SST

**Zero new SST keys.** Per spec.md hard constraint and §5.3 OQ-DSN-AMD-3 resolution, the registered-surface allowlist is a Go file (`internal/auth/scopes.go`), not a `smackerel.yaml` key. No `config/smackerel.yaml` change. No `config/generated/dev.env` change. No `internal/config/config.go` change.

This means:
- `./smackerel.sh config generate` is a no-op for this spec.
- The NO-DEFAULTS / fail-loud SST policy is satisfied trivially (no new keys = no new fallback risk).
- Future specs that add a surface modify `internal/auth/scopes.go` only.

---

## 11. Observability

### 11.1 Metrics

Two new counter vectors in `internal/metrics/auth.go`:

```go
// AuthScopeRejected counts requests rejected by RequireScope.
// Labels:
//   required_scope — the FIRST scope from `required` not present in
//                    the session (avoids per-request label cardinality
//                    explosion when a legacy token is missing all).
//   user_id        — the session UserID (bounded by enrolled-user count;
//                    safe per existing AuthFailure label conventions).
var AuthScopeRejected = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "smackerel",
        Subsystem: "auth",
        Name:      "scope_rejected_total",
        Help:      "...",
    },
    []string{"required_scope", "user_id"},
)

// AuthScopeCheckBypassed counts RequireScope passes attributable to
// the dev/test or bootstrap session source bypass (spec 060 §5.2).
// Labels:
//   source — "shared_token" | "bootstrap"
var AuthScopeCheckBypassed = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "smackerel",
        Subsystem: "auth",
        Name:      "scope_check_bypassed_total",
        Help:      "...",
    },
    []string{"source"},
)
```

Both registered in the package-level `Register(...)` list alongside `AuthValidationOutcome`. Closed-set label cardinality unit tests follow the `TestAuthValidationOutcome_AcceptsClosedSetLabels` pattern.

### 11.2 Structured Logs

One `slog.Warn` per rejection (§4.6). No log on successful pass-through (hot-path silence). One `slog.Warn` per parse-time regex failure inside `getScopeClaim` (defense-in-depth event).

### 11.3 Failure Modes

| Mode | Symptom | Detection | Operator Action |
|------|---------|-----------|-----------------|
| Operator forgot `--scope` at enrollment | Extension POST returns 403 | `auth_scope_rejected_total{required_scope="extension:bookmarks,history"}` spike on extension endpoint | Re-enroll or rotate with `--scope` |
| Rotation accidentally stripped scope (no `--prior-token` passed) | Existing extension stops working after rotation | Same as above, immediately after a rotation event | CLI now refuses this case (§7.2) — exit 2 with diagnostic. Symptom blocked at source. |
| Production accidentally taking shared-token bypass path | `auth_scope_check_bypassed_total{source="shared_token"}` increments in prod | Prometheus alert on `> 0` in `production` environment | Investigate SST config; production should never take this path |
| Forged or downgraded-issuer scope claim | Parse-time regex fails | `auth_scope_check_bypassed_total{source="malformed_claim"}` (NEW sub-bucket if needed) + WARN log | Investigate token provenance; rotate signing keys if signature verification is suspect |

---

## 12. Testing & Validation Strategy

| Scenario | Test type | Location | Assertion |
|----------|-----------|----------|-----------|
| BS-001 — Scoped token accepted on wired endpoint | integration | `internal/api/scope_middleware_test.go` (new) | 202 from a chi route composed with `bearerAuthMiddleware` + `RequireScope`; live PASETO mint |
| BS-002 — Legacy token rejected on wired endpoint (ADVERSARIAL) | integration | same file | 403 `scope_required`; counter increment by exactly 1; structured log present |
| BS-003 — Cross-scope replay rejected | integration | same file | 403 on hypothetical `RequireScope("admin:users")` route when session carries only `extension:bookmarks,history` |
| BS-004 — Unwired endpoint remains backward compatible | integration | `internal/api/router_test.go` (extend) | Existing endpoints continue to accept legacy tokens; assertion is a regression check (no new behavior) |
| BS-005 — Invalid scope name rejected at enrollment | unit (CLI) | `cmd/core/cmd_auth_test.go` (extend) | Exit 2; no token minted; no DB write |
| BS-006 — Unknown surface rejected unless escape hatch | unit (CLI) | same file | Exit 2 without `--allow-unknown-surface`; exit 0 with warning when escape hatch supplied |
| BS-007 — Dev/test bypass satisfies scope requirements | integration | `internal/api/scope_middleware_test.go` | 202 with shared-token session; `auth_scope_check_bypassed_total{source="shared_token"}` increments |
| BS-008 — Rotation preserves scope by default | unit (CLI) + integration | `cmd/core/cmd_auth_test.go`, `internal/auth/issue_test.go` | New token's parsed claims include the prior token's scope; CLI flow accepts `--prior-token` |
| BS-009 — Rotation can explicitly demote to no scope | unit (CLI) | `cmd/core/cmd_auth_test.go` | New token has no `scope` claim; subsequent verify returns `Scopes: nil` |
| Scope claim wire-format roundtrip | unit | `internal/auth/issue_test.go` + `verify_test.go` | Mint → parse → equality on `[]string` |
| Parse-time regex defense (malformed claim) | unit | `internal/auth/verify_test.go` | Hand-craft a token with malformed scope element; `VerifyAndParse` returns `Scopes: nil` + sentinel error |
| `RequireScope` panics on zero-length required at construction | unit | `internal/auth/scope_middleware_test.go` (new file inside `internal/auth`) | `require.Panics` |
| `RequireScope` returns 500 when session is absent | unit | same file | Composed without `bearerAuthMiddleware`; assert 500 `middleware_misconfigured` |
| Closed-set label cardinality on `AuthScopeRejected` / `AuthScopeCheckBypassed` | unit | `internal/metrics/auth_test.go` (extend) | Follow `TestAuthValidationOutcome_AcceptsClosedSetLabels` pattern |
| BS-002 adversarial regression — bug-fix discipline | regression test (lives next to BS-002) | same file | Comment block documents: "If `getScopeClaim` ever falls back to treating missing claim as `[]string{"*"}` or any wildcard, this test MUST fail." Implementation: assert exact 403 status AND counter delta of exactly 1 AND log capture contains `event=scope_rejected`. The test MUST NOT use bailout `if err != nil { return }` patterns. |

Tooling alignment: all tests run via `./smackerel.sh test unit` and `./smackerel.sh test integration`. No new test infrastructure. No new docker-compose surface (integration tests reuse the existing live-stack contract from spec 044).

---

## 13. Alternatives & Tradeoffs

| Alternative | Why rejected |
|-------------|--------------|
| Parallel per-extension token type (spec 058 NC-1 forbids) | Doubles the auth audit surface; spec 044 deliberately built ONE per-user token primitive |
| `scope` as space-separated string (OAuth 2.0 wire form) | Spec 058 example `extension:bookmarks,history` already contains a comma; space-separation collides poorly with `chi` URL conventions and is harder to introspect via `auth inspect` |
| `scope` as DB-side column on `auth_tokens` | Forces a schema migration; couples scope to row identity; breaks the "scopes live in the token claim" invariant; means rotation can preserve scopes without `--prior-token` BUT at the cost of every other invariant |
| Scope hierarchy / wildcards (`extension:*`) | Explicit Non-Goal; can be added later without breaking existing tokens |
| OR-of-scopes semantics for `RequireScope` | §5.1; principle-of-least-privilege footgun |
| Implicit all-scopes for dev/test bypass | §5.2; couples bypass to vocabulary; brittle on future surface adds |
| Per-package `init()` scope registration | §5.3; fragments audit surface; introduces import-order coupling |
| `smackerel.yaml` allowlist | §5.3; violates SST justification bar; fragments audit surface |
| Pass `--scope csv` as a single comma-delimited flag | Collides with capability-internal commas (`extension:bookmarks,history`); §7.1 resolved via repeatable `--scope` |

---

## 14. Rollout

Single-binary upgrade. Backwards compatible. No SST migration, no DB migration, no docker-compose change.

Operator runbook update (spec.md G6): `docs/Operations.md` gains a "Scoped Token Enrollment" subsection covering:
- When to use `--scope` (currently: only when minting a token for the Chrome extension).
- Mint command: `smackerel auth enroll --scope extension:bookmarks,history --user alice`.
- Rotation: `smackerel auth rotate --prior-token-id <id> --prior-token <wire> --user alice` (preserve) OR `--scope <new>` (replace) OR `--scope ""` (demote).
- Inspect: `smackerel auth inspect <wire>`.
- Migration: re-enroll/rotate users only when they need a scoped token; no batch operation.

`docs/API.md` update: document the `403 scope_required` response shape and the initial wiring matrix (§9).

No staged rollout; the change ships in one release. The first spec to wire `RequireScope` (spec 058) ships SEPARATELY; spec 060 lands the primitive first and stays inert until spec 058 wires its endpoint.

---

## 15. Capability Foundation

Spec.md AN5 (Domain Capability Model) is satisfied by §3 + §4 + §5.3:

- **Capability foundation:** `auth.Session.Scopes` + `auth.RequireScope(...)` middleware + scope-name regex + registered-surface allowlist.
- **Concrete implementations (variation axes):**
  - **Surface axis:** `extension:*` (spec 058), `admin:*` (hypothetical future), `mobile:*` (spec 033 future), `automation:*` (future).
  - **Capability granularity axis:** single-capability scopes (`extension:bookmarks,history`) vs multi-surface tokens (multi-element `scope` array — supported but not the recommended pattern).
- **Foundation-owned policies:** AND semantics (§5.1), dev/test bypass behavior (§5.2), missing-claim semantics (§8), scope-name format regex (§3.2), parse-time defense-in-depth (§3.2).
- **Single-implementation justification:** N/A — the foundation already has one concrete consumer (spec 058) and a second hypothetical surface (`admin:users` in BS-003) demonstrating the variation axis.

---

## 16. Out of Scope (Reaffirmed)

- General-purpose RBAC.
- Scope hierarchies / wildcards.
- Per-scope partial revocation.
- Scope claims on the dev/test bypass path.
- Batch migration of legacy tokens.
- Default per-endpoint scope wiring (spec 058 wires its own endpoint; this spec wires none).
- Storing scopes in the database (§6).
- Adding `smackerel.yaml` keys (§10).

---

## Summary

Design extends spec 044's PASETO claim set with an optional `scope: []string` claim, threads it through `ParsedToken` → `Session.Scopes`, and ships `auth.RequireScope(required ...string)` middleware with AND semantics (OQ-DSN-AMD-1), dev/test pass-through (OQ-DSN-AMD-2), and a single Go-file registered-surface allowlist at `internal/auth/scopes.go` (OQ-DSN-AMD-3). CLI gets `--scope` (repeatable) and `--allow-unknown-surface` flags on `auth enroll` and `auth rotate`; rotation preserves scopes when `--prior-token` is passed, otherwise refuses with a diagnostic. Backwards compatibility is the spec's hard invariant: legacy tokens carry `Scopes: nil` and authenticate against every unwired endpoint identically to today, while wired endpoints reject them with `403 scope_required`. Zero SST changes, zero DB schema changes, zero new docker-compose surface. Spec 058 unblocked once this design's primitives ship.
