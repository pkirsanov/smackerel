# Spec: BUG-058-001 — extension-devices admin HTML surface on a shared admin scaffold

## Expected Behavior

The extension-devices admin view MUST be available as a rendered HTML page
(`GET /admin/extension/devices`) on a REUSABLE admin scaffold, gated by the same
admin predicate as the JSON view, reusing the same aggregation store (no
duplicated query), with non-admin callers scoped to their own `owner_user_id`.

## Actual Behavior

The devices view shipped JSON-only (`GET /v1/admin/extension/devices`); the repo
had no shared admin-UI scaffolding (`internal/web/admin/` did not exist). See
`bug.md`.

## Acceptance Criteria

1. **AC-1 (reusable scaffold):** `internal/web/admin` provides a shared base
   layout + navigation fragment + an `AuthGate` helper, matching the repo's
   `internal/web/agent_admin.go` Go `html/template` convention (no new engine, no
   static-embed shortcut).
2. **AC-2 (HTML devices page):** `GET /admin/extension/devices` renders the
   devices as an HTML table on the scaffold, behind `webAuthMiddleware`.
3. **AC-3 (no duplication):** the page reuses the certified
   `extensiondevices.Store.AggregateDevices` and the same admin predicate as the
   JSON handler; no parallel query or second auth primitive.
4. **AC-4 (auth + scoping):** unauthenticated → 401; non-admin → scoped to own
   `owner_user_id` (403 when absent); admin → all owners; deterministic
   `(owner, source_device_id)` order; GET-only (405 otherwise).
5. **AC-5 (XSS-safe):** user-influenced values (`source_device_id`,
   `owner_user_id`) are HTML-escaped — proven by test.

## Out of Scope

- Rewriting the certified `/admin/auth/tokens` static page onto the scaffold (a
  safe optional follow-up; not rewritten here to avoid a security regression).
- BLOCKER-1 (MV3 Playwright e2e) and BLOCKER-4 (CI MV3 sideload smoke) of the
  tracking bug — they require browser/CI infrastructure.
- Changing the JSON endpoint or its contract.

## Cross-References

- Bug detail + no-shortcut decisions: `bug.md`
- Tracking bug: `../BUG-058-EXTERNAL-INFRA-MISSING/`
- Precedent: `internal/web/agent_admin.go`
