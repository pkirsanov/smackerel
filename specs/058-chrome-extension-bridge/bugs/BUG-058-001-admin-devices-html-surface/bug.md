# BUG-058-001: extension-devices admin HTML surface on a shared admin scaffold (BLOCKER-3)

**Status:** Resolved (admin scaffolding foundation + devices HTML page via bugfix-fastlane — see report.md)
**Severity:** Medium (discharges BUG-058-EXTERNAL-INFRA-MISSING BLOCKER-3, one of the spec 058 `blocked` causes)
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "need proper long term solution, no shortcuts/simplifications" (resolving the spec 058 blockers)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/058-chrome-extension-bridge/`
**Tracking bug:** `../BUG-058-EXTERNAL-INFRA-MISSING/` (BLOCKER-3)

## Summary

Spec 058 was `blocked` partly because its admin **devices view existed only as
JSON** (`GET /v1/admin/extension/devices`) with no rendered HTML surface, and
the repo had **no shared admin-UI scaffolding** to build one on — the
"HTMX-admin generalization missing" blocker (BLOCKER-3 of
BUG-058-EXTERNAL-INFRA-MISSING).

This delivers the proper long-term foundation, not a one-off page:

1. **`internal/web/admin` — a reusable server-rendered admin scaffold** (shared
   base layout + navigation fragment + an `AuthGate` helper), matching the
   repo's established `internal/web/agent_admin.go` Go `html/template` convention
   (no new template engine, no static-HTML-embed shortcut).
2. **The extension-devices admin page** (`GET /admin/extension/devices`)
   rendered on that scaffold, **reusing the certified `extensiondevices.Store`
   aggregation and the same admin predicate** as the JSON handler — zero
   duplicated query logic, zero second auth primitive.

## Mechanism (what was missing)

- `internal/web/admin/` did not exist — no shared layout, nav, or auth-gating
  helper. Each admin surface (`/admin/agent/*`, `/admin/auth/tokens`) was bespoke.
- The devices view shipped JSON-only; spec 058 design §3.2 wants the rendered
  operator surface. Without scaffolding, the only paths were another bespoke HTML
  blob (a shortcut that re-creates the duplication) or leaving it JSON-only
  (incomplete).

## Fix (delivered — the proper foundation)

1. **`internal/web/admin/scaffold.go`** — the reusable foundation:
   - `baseLayout` + `newBaseTemplate()`: one parsed shared layout (head + nav +
     content slot) so every admin page renders consistent chrome.
   - `navLinks(activeHref)`: the canonical admin navigation fragment (Agent
     Traces, Auth Tokens, Extension Devices) with active-link highlighting.
   - `AuthGate` (= `extensiondevices.AdminPredicate`): the production wiring
     passes the SAME `callerIsAdmin` closure it already builds for the JSON
     handler — no auth drift.
   - `renderContent` / `renderPage`: compose an inner page template into the base
     layout; because the inner template is itself an `html/template`, every
     interpolated field is auto-escaped (XSS-safe).
2. **`internal/web/admin/devices.go`** — `DevicesHandler`: GET-only;
   runs the gate (401 unauth); non-admin callers are scoped to their own
   `owner_user_id` (403 if absent); reuses `extensiondevices.Store.AggregateDevices`;
   renders an HTML table (with an empty state); 500 on store error. The order
   matches the JSON handler `(owner_user_id, source_device_id)` so the two
   surfaces present identically.
3. **Wiring**: `cmd/core/wiring.go` constructs
   `webadmin.NewDevicesHandler(extStore, adminPredicate)` reusing the EXISTING
   store + predicate; `internal/api/health.go` gains `ExtensionDevicesUIHandler`;
   `internal/api/router.go` mounts `GET /admin/extension/devices` behind
   `webAuthMiddleware` (same auth as the agent operator UI).

## Deliberate no-shortcut decisions (recorded)

- **Tokens page left as-is.** `/admin/auth/tokens` is a certified, XSS-audited
  spec-044 static surface. Rewriting it onto the new scaffold would be
  change-for-change's-sake and risk a security regression for cosmetic
  consistency. The scaffold's nav LINKS to it; the "generalization exists and is
  reusable" requirement is satisfied by the scaffold + the new devices page using
  it. Migrating tokens later is a safe, optional follow-up.
- **Reused the certified `extensiondevices.Store`** instead of a parallel query —
  no duplicated aggregation, no divergence risk between the JSON and HTML views.
- **Matched the repo convention** (Go `html/template`) rather than introducing
  `templ`/HTMX-static — the proper long-term fit for this codebase.

## Scope boundary

This packet resolves **BLOCKER-3 only**. Spec 058 remains `blocked` pending
BLOCKER-1 (MV3 Playwright e2e harness — needs Chromium + the live stack) and
BLOCKER-4 (CI MV3 sideload smoke — runs on CI runners). BLOCKER-2 (live-Postgres
integration) was resolved 2026-06-05; the parent spec's stale
`blockingDependencies` entry is reconciled in this change.

## Cross-References

- Scaffold: `internal/web/admin/scaffold.go`
- Page: `internal/web/admin/devices.go`
- Reused store: `internal/api/admin/extensiondevices/devices.go`
- Wiring: `cmd/core/wiring.go`, `internal/api/router.go`, `internal/api/health.go`
- Tracking bug: `../BUG-058-EXTERNAL-INFRA-MISSING/` (BLOCKER-3)
- Precedent mirrored: `internal/web/agent_admin.go`
