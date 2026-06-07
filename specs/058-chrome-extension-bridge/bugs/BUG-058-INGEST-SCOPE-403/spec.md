# Spec: BUG-058-INGEST-SCOPE-403 — extension ingest must accept the canonical single scope

## Expected Behavior

A per-user PASETO token whose `scope` claim is the canonical extension scope
`"extension:bookmarks,history"` (one comma-joined capability string, per spec
060 spec.md L15/L70/L138 and spec 058 design.md L295/L330/L498/L684) MUST be
accepted at `POST /v1/connectors/extension/ingest` and reach the ingest
handler. A token that lacks that scope MUST still be rejected `403
scope_required`.

## Actual Behavior

The route was wired with `auth.RequireScope("extension:bookmarks",
"extension:history")` — two separate scopes under AND-semantics with exact
`slices.Contains` matching. Because the real token carries the scope as ONE
comma-joined element (`getScopeClaim` does not split on `","`), neither bare
substring is present and **every** legitimate per-user token is rejected
`403 scope_required`. See `bug.md` → "Mechanism" + "Reproduction".

## Acceptance Criteria

1. **AC-1 (canonical scope accepted):** A per-user token with
   `Scopes = ["extension:bookmarks,history"]` POSTing to
   `/v1/connectors/extension/ingest` through the real `NewRouter` reaches the
   ingest handler (not `403`).
2. **AC-2 (missing scope still rejected):** A per-user token without the
   canonical scope (e.g. a legacy spec-044 token with no `scope` claim) POSTing
   to the same route is rejected `403`.
3. **AC-3 (regression pinned):** `internal/api/router_extension_scope_test.go`
   exercises the real router wiring end-to-end (httptest) and FAILS if the gate
   is reverted to two separate scopes.
4. **AC-4 (contract restored):** `internal/api/router.go` wires
   `auth.RequireScope("extension:bookmarks,history")` and its comment matches
   the spec 060 / spec 058 canonical single-scope contract.
5. **AC-5 (no collateral change):** No token-format change, no schema change,
   no change to shared-token/bootstrap bypass behavior; the full `internal/api`
   package stays green.

## Out of Scope

- Re-architecting `RequireScope` AND-semantics — the middleware is correct; the
  defect was the wiring passing a split scope.
- The four external-infrastructure gaps tracked in
  `../BUG-058-EXTERNAL-INFRA-MISSING/`.
- The cross-tenant dedup isolation fixed in `../BUG-058-DEDUP-KEY-OWNER-ISOLATION/`.
- Reconciling `specs/058-chrome-extension-bridge/report.md` L19 prose (it
  documents the old two-scope form); noted for the spec owner to refresh when
  spec 058 unblocks — not edited here to keep the blocked parent's evidence
  stable.

## Cross-References

- Bug detail + mechanism + fix: `bug.md`
- Auth contract owner: `specs/060-bearer-auth-scope-claim/spec.md`
- Parent design (required-scope contract): `../../design.md`
