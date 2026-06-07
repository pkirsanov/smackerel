# Design: BUG-058-INGEST-SCOPE-403

## Problem

`internal/api/router.go` wired the extension ingest route with
`auth.RequireScope("extension:bookmarks", "extension:history")` — two separate
scope strings. `auth.RequireScope` (spec 060) enforces AND-semantics with an
exact `slices.Contains(sess.Scopes, scope)` test, and `getScopeClaim`
(`internal/auth/verify.go`) reads the PASETO `scope` claim as a `[]string`
WITHOUT splitting on `","`. The canonical extension token therefore carries
`Scopes = ["extension:bookmarks,history"]` (one element), and the two-scope
gate rejects it `403`. See `bug.md` for the verified mechanism.

The fix is a wiring correction, not a contract change: the canonical scope is
already defined by spec 060 spec.md (L15/L70/L138) and consumed by spec 058
design.md (L295/L330/L498/L684) as the single string
`"extension:bookmarks,history"`. The router simply called `RequireScope` with
the wrong argument shape.

## Change

`internal/api/router.go` (extension ingest group):

```
- r.With(auth.RequireScope("extension:bookmarks", "extension:history")).
+ r.With(auth.RequireScope("extension:bookmarks,history")).
      Post("/v1/connectors/extension/ingest", deps.ExtensionIngestHandler.ServeHTTP)
```

The adjacent comment is corrected to state the single-scope contract and to
reference the regression test.

### Why this shape

- `"extension:bookmarks,history"` is ONE atomic scope string representing one
  surface (`extension`) with two comma-joined capabilities. Spec 060 spec.md
  L122 makes scope matching "exact string-set containment" — `extension:*` does
  NOT satisfy it, and neither do the bare `extension:bookmarks` /
  `extension:history` substrings. The token and the gate must use the same
  atomic string.
- AND-semantics with a single required scope reduces to "the session must carry
  exactly that scope" — the intended check.
- Shared-token/bootstrap bypass is unchanged (the `RequireScope` source switch
  is untouched).

## Schema Impact

None. No token-format change, no migration, no new dependency.

## Blast Radius

- `internal/api/router.go` — one wiring line + its comment.
- `internal/api/router_extension_scope_test.go` (new) — end-to-end (httptest)
  regression through the real `NewRouter`: canonical-scope token reaches the
  handler; no-scope token is `403`.
- No change to `internal/auth/*` (the middleware was already correct), no change
  to the extension handler, no change to token minting.

## Test Tier Rationale

The defect is a router/middleware wiring error, so the faithful regression tier
is an in-process HTTP test that builds the REAL `NewRouter(deps)`, mints a real
per-user PASETO via `auth.IssueToken`, and drives the actual route. This pins
`router.go` itself (a revert to two scopes fails the test, proven by an
adversarial re-run). A live-stack curl would add nothing the in-process router
test does not already cover for a pure wiring fix, and the shared-token bypass
means a live-stack run under the dev token would NOT even exercise the gate.

## Alternatives Considered

- **Teach `RequireScope` / `getScopeClaim` to comma-split** so two-scope wiring
  would work. Rejected: spec 060 L122 defines scope matching as exact
  string-set containment with the comma INSIDE a single capability string;
  splitting would change the documented contract and the meaning of every other
  scope. The wiring was wrong, not the contract.
- **Add a composite `extension:bookmarks` + `extension:history` scope model.**
  Rejected: contradicts spec 060/058's single-string canonical scope and the
  enrollment CLI (`--scope extension:bookmarks,history`).
