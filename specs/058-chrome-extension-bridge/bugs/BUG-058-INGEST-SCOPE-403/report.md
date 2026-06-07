# Report: BUG-058-INGEST-SCOPE-403 — restore single-scope extension ingest gate

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07

## Summary

`POST /v1/connectors/extension/ingest` was gated with
`auth.RequireScope("extension:bookmarks", "extension:history")` — two separate
scope strings under AND-semantics with exact `slices.Contains` matching. The
canonical extension token carries the scope as ONE comma-joined element
`["extension:bookmarks,history"]` (spec 060 spec.md L15/L70/L138; spec 058
design.md L295/L330/L498/L684; `getScopeClaim` does not split on `","`), so
every real per-user token was rejected `403 scope_required`. The fix restores
the single canonical scope `auth.RequireScope("extension:bookmarks,history")`
and pins it with an end-to-end router regression test. Dev/test shared-token +
bootstrap sessions bypass the scope gate, which is why the defect shipped
undetected.

## Root Cause

`auth.RequireScope` (`internal/auth/scope_middleware.go`) loops the required
scopes and checks exact `slices.Contains(sess.Scopes, scope)`. `getScopeClaim`
(`internal/auth/verify.go`) reads the PASETO `scope` claim as a `[]string`
without splitting on `","`. A real token therefore has
`Scopes = ["extension:bookmarks,history"]` (one element), and
`slices.Contains(["extension:bookmarks,history"], "extension:bookmarks")` is
`false` → `403`. The wiring mis-read spec 060's "AND-semantics" (which is for
requiring multiple DISTINCT scopes) and split one comma-joined capability scope
into two.

## Fix

One wiring line in `internal/api/router.go`:

<!-- bubbles:evidence-legitimacy-skip-begin -->
```
- r.With(auth.RequireScope("extension:bookmarks", "extension:history")).
+ r.With(auth.RequireScope("extension:bookmarks,history")).
      Post("/v1/connectors/extension/ingest", deps.ExtensionIngestHandler.ServeHTTP)
```
<!-- bubbles:evidence-legitimacy-skip-end -->

The adjacent comment was corrected to state the single-scope contract and
reference the regression test. A new end-to-end (httptest) regression file
`internal/api/router_extension_scope_test.go` drives the real `NewRouter`.

## Test Evidence

### RED — canonical-scope token is 403'd by the pre-fix two-scope wiring

```
$ go test -count=1 -run 'TestExtensionIngest_' ./internal/api/
INFO request method=POST path=/v1/connectors/extension/ingest status=403 duration_ms=0
--- FAIL: TestExtensionIngest_CanonicalScopeReachesHandler (0.00s)
    router_extension_scope_test.go:56: canonical-scope token was 403'd — gate mis-wired;
    body={"error":"scope_required","required":["extension:bookmarks"]}
FAIL    github.com/smackerel/smackerel/internal/api     0.129s
```

### GREEN — both tests pass against the fixed single-scope wiring

```
$ go test -v -count=1 -run 'TestExtensionIngest_' ./internal/api/
=== RUN   TestExtensionIngest_CanonicalScopeReachesHandler
--- PASS: TestExtensionIngest_CanonicalScopeReachesHandler (0.00s)
=== RUN   TestExtensionIngest_MissingScopeRejected
--- PASS: TestExtensionIngest_MissingScopeRejected (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     0.248s
```

### Adversarial Re-RED — reverting to two scopes makes the regression fail again

```
$ sed -i 's/RequireScope("extension:bookmarks,history")/RequireScope("extension:bookmarks", "extension:history")/' internal/api/router.go
$ go test -count=1 -run 'TestExtensionIngest_CanonicalScopeReachesHandler' ./internal/api/
--- FAIL: TestExtensionIngest_CanonicalScopeReachesHandler (0.00s)
    router_extension_scope_test.go:56: canonical-scope token was 403'd — required:["extension:bookmarks"]
FAIL    github.com/smackerel/smackerel/internal/api     0.110s
```

(`REVERT_RC=1` as expected; `internal/api/router.go` was then restored to the single-scope fix.)

### Broader Regression — full internal/api package green with the new tests

```
$ go build ./internal/api/...
$ go vet ./internal/api/
$ go test -count=1 ./internal/api/
ok      github.com/smackerel/smackerel/internal/api     9.435s
```

### Code Diff Evidence

```
$ git diff --stat internal/api/router.go
 internal/api/router.go | 19 ++++++++++++-------
 1 file changed, 12 insertions(+), 7 deletions(-)
$ git status --short internal/api/router_extension_scope_test.go
?? internal/api/router_extension_scope_test.go
```

Files changed: `internal/api/router.go` (gate `RequireScope("extension:bookmarks", "extension:history")` → `RequireScope("extension:bookmarks,history")` + corrected comment); `internal/api/router_extension_scope_test.go` (new red→green regression + adversarial twin through the real `NewRouter`). No schema migration. No token-format change. No change to `internal/auth/*` or the extension handler.

### Validation Evidence

```
$ go build ./internal/api/...
$ go vet ./internal/api/
$ go test -count=1 ./internal/api/
ok      github.com/smackerel/smackerel/internal/api     9.435s
```

Build clean, vet clean, the full `internal/api` package (router + auth
middleware + handlers) green with the two new regression tests included.

### Audit Evidence

```
$ git diff --stat internal/api/router.go
 internal/api/router.go | 19 ++++++++++++-------
 1 file changed, 12 insertions(+), 7 deletions(-)
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration added; diff confined to router.go + the new test + this bug packet)
```

The change is confined to `internal/api/router.go` (one wiring line + its
comment) and the new `internal/api/router_extension_scope_test.go`. No
migration, no `.github/bubbles` framework files, no edits to the blocked parent
spec 058 planning artifacts.

## Completion Statement

The extension ingest endpoint now accepts the canonical
`"extension:bookmarks,history"` per-user token and reaches the handler, while
tokens lacking the scope still receive `403` (adversarial twin proves the gate
keeps enforcing). The regression test pins the real router wiring and fails on
any revert to the two-scope form. The fix restores the contract documented in
spec 060 and spec 058 with zero collateral change; the full `internal/api`
package is green. Scope 1 DoD is complete (9/9). BUG-058-INGEST-SCOPE-403 is
Done.
