# BUG-058-INGEST-SCOPE-403: extension ingest scope gate split into two scopes → 403s every real per-user token

**Status:** Resolved (single-scope gate restored via bugfix-fastlane — see report.md)
**Severity:** High
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Stochastic Quality Sweep Round 16 (parent: stochastic-quality-sweep) — `harden`, parent-expanded
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/058-chrome-extension-bridge/` (auth contract owned by `specs/060-bearer-auth-scope-claim/`)
**Affected surface:** `internal/api/router.go` (extension ingest route wiring)

## Summary

The extension ingest endpoint `POST /v1/connectors/extension/ingest` was wired
with `auth.RequireScope("extension:bookmarks", "extension:history")` — **two
separate scope strings**. `auth.RequireScope` enforces AND-semantics with an
exact `slices.Contains(sess.Scopes, scope)` membership test, so the gate
requires the session's `Scopes` slice to contain BOTH the bare element
`"extension:bookmarks"` AND the bare element `"extension:history"`.

But the canonical extension scope — per spec 060 spec.md (L15/L70/L138) and
spec 058 design.md (L295/L330/L498/L684) — is the SINGLE comma-joined
capability string `"extension:bookmarks,history"` (one surface, two
capabilities). `getScopeClaim` (`internal/auth/verify.go`) reads the PASETO
`scope` claim as a `[]string` and does **not** split on `","`, so a real
per-user token carries `Scopes = ["extension:bookmarks,history"]` — ONE
element. Neither bare substring is an element of that slice, so the gate
returns `403 scope_required` (`required:["extension:bookmarks"]`) for **every**
legitimate per-user extension token.

## Mechanism (verified by code reading + a red regression test at repo HEAD)

1. `internal/api/router.go` (pre-fix) →
   `r.With(auth.RequireScope("extension:bookmarks", "extension:history"))` —
   `want = ["extension:bookmarks", "extension:history"]`.
2. `internal/auth/scope_middleware.go` → for each required scope,
   `if !slices.Contains(sess.Scopes, scope) { 403 }` — exact-string membership.
3. `internal/auth/verify.go` `getScopeClaim` → returns the `scope` claim as a
   `[]string`; NO comma split. Confirmed by `scope_claim_test.go` round-trip:
   `["extension:bookmarks,history"]` stays one element.
4. A real token therefore has `Scopes = ["extension:bookmarks,history"]`.
   `slices.Contains(["extension:bookmarks,history"], "extension:bookmarks")` is
   `false` → first required scope missing → `403 scope_required`.

## Why it shipped undetected

`auth.RequireScope` short-circuits (bypasses the scope check) for
`SessionSourceSharedToken` and `SessionSourceBootstrap`. Dev/test and the
existing handler-level tests use the shared deployment token or inject a
session directly — both bypass the gate — so the route worked in every
non-production path. Only a **per-user PASETO** session (the real production
extension-enrollment flow) reaches the broken exact-match check. No
integration/e2e test minted a real per-user token and exercised the actual
`POST /v1/connectors/extension/ingest` route through `NewRouter`, so the
defect was invisible to the suite. `specs/058-chrome-extension-bridge/report.md`
L19 even documents the wrong two-scope form, having mis-read spec 060's
"AND-semantics" (which is for requiring multiple DISTINCT scopes, not for
splitting one comma-joined capability scope).

## Reproduction (in-process, deterministic — `router_extension_scope_test.go`)

1. Mint a per-user PASETO with `Scopes = ["extension:bookmarks,history"]`
   (the canonical enrollment scope, `./smackerel.sh auth enroll --scope
   extension:bookmarks,history`).
2. `POST /v1/connectors/extension/ingest` through the real `NewRouter(deps)`
   with that bearer token.
3. **Pre-fix outcome:** `403 scope_required`, body
   `{"error":"scope_required","required":["extension:bookmarks"]}` — the
   ingest handler is never reached. Every legitimate extension client is
   locked out of the core ingest path.

## Impact / Severity rationale (High)

- **Total functional outage of the core ingest path for real users:** every
  production per-user extension token is rejected `403`; the chrome extension
  cannot capture anything. This is the headline capability of spec 058.
- **Fails CLOSED, not open:** the bug is over-restrictive (denial), NOT a
  security bypass — no unauthorized access is granted. Severity is High for
  availability/correctness, not a confidentiality/integrity breach.
- **Silent in all non-production paths:** shared-token/bootstrap bypass masks
  it in dev/test, so it would surface only on a real deployment.

## Fix (delivered)

`internal/api/router.go` — change the gate to the single canonical scope:

```
auth.RequireScope("extension:bookmarks", "extension:history")   // ❌ before
auth.RequireScope("extension:bookmarks,history")                // ✅ after
```

This restores the contract documented in spec 060 spec.md (L15/L138) and spec
058 design.md (L295/L684). No token-format change, no schema change, no
behavior change for shared-token/bootstrap sessions. Tokens lacking the scope
still `403` (adversarial twin proves the gate keeps enforcing).

## Cross-References

- Parent spec: [`../../spec.md`](../../spec.md), design: [`../../design.md`](../../design.md), report: [`../../report.md`](../../report.md)
- Auth contract owner: `specs/060-bearer-auth-scope-claim/spec.md` (L15/L70/L138), design.md (§3/§4)
- Gate: `internal/auth/scope_middleware.go` (`RequireScope`, AND-semantics, exact `slices.Contains`)
- Claim parse: `internal/auth/verify.go` (`getScopeClaim`, no comma split)
- Regression test: `internal/api/router_extension_scope_test.go`
