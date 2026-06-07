# Design: BUG-058-001

## Problem

Spec 058's devices view shipped JSON-only, and the repo had no shared admin-UI
scaffolding to render an operator HTML surface on. See `bug.md`.

## Architecture

```
GET /admin/extension/devices  (router: behind webAuthMiddleware)
  → internal/web/admin.DevicesHandler.ServeHTTP
      → AuthGate(r) → ownerUserID, isAdmin, ok      (same closure as JSON handler)
          ok == false                → 401
          !isAdmin && ownerUserID==""→ 403
          !isAdmin                   → filter = ownerUserID
      → extensiondevices.Store.AggregateDevices(ctx, filter)   (REUSED, no dup)
      → sort (owner_user_id, source_device_id)
      → renderContent(devicesContent, devices) → escaped inner HTML
      → renderPage(base, "Extension Devices", activeHref, inner)  (shared chrome)
```

### The reusable scaffold (`internal/web/admin/scaffold.go`)

The foundation the page (and future admin pages) build on:

- **`baseLayout` + `newBaseTemplate()`** — one parsed shared layout: head +
  nav + a `{{.Content}}` slot. Unlike `agent_admin.go` (self-contained
  per-page templates so a shared layout block does not collide), this factors
  the chrome into a single base, which is what makes it genuinely shared.
- **`navLinks(activeHref)`** — the canonical admin nav (Agent Traces, Auth
  Tokens, Extension Devices) with active highlighting. Static UI chrome (a fixed
  operator surface), so a hardcoded list is correct — not a runtime business
  value, no SST concern.
- **`AuthGate = extensiondevices.AdminPredicate`** — a type alias so the
  production wiring passes the exact `callerIsAdmin` closure it already builds.
  No second auth primitive, no drift.
- **`renderContent` / `renderPage`** — compose an inner page template into the
  base. The inner template is itself an `html/template`, so interpolated fields
  are auto-escaped BEFORE becoming trusted HTML — wrapping in `template.HTML`
  does not bypass escaping.

### The devices page (`internal/web/admin/devices.go`)

`DevicesHandler{ store extensiondevices.Store; gate AuthGate; base, content *template.Template }`,
constructed via `NewDevicesHandler(store, gate)` with fail-loud nil guards
(matching `extensiondevices.NewHandler`). `ServeHTTP` enforces the auth/scoping
rules and renders the table (with an empty state) on the scaffold.

### Wiring

`cmd/core/wiring.go` reuses the already-constructed `extStore` and
`adminPredicate` (built for the JSON handler) to construct the UI handler —
maximal reuse. `internal/api/health.go` gains `ExtensionDevicesUIHandler`;
`internal/api/router.go` mounts the route behind `webAuthMiddleware` (same auth
as `/admin/agent/*`).

## Why this package layout

- `internal/web/admin` is a NEW leaf package importing
  `internal/api/admin/extensiondevices` (a leaf with no `internal/web`/`internal/api`
  import) — no cycle: `internal/api → internal/web/admin → internal/api/admin/extensiondevices`.
- Mirrors the existing `internal/web` admin convention while giving the shared
  chrome its own home, so future admin pages adopt it without touching the
  bespoke `agent_admin.go`.

## Test Strategy

`internal/web/admin/devices_test.go` (no DB — fake `extensiondevices.Store` + a
scripted `AuthGate`):

- renders the table for an admin (scaffold chrome present, nav active link,
  rows, no owner filter);
- empty state;
- non-admin scoped to own owner (filter == owner);
- non-admin missing owner → 403 (store NOT queried);
- unauthenticated → 401 (store NOT queried);
- store error → 500;
- non-GET → 405;
- **XSS:** a `<script>` device id is escaped, not rendered raw;
- nil-arg constructor panics;
- `navLinks` marks exactly one active link.

The live HTTP route mount is covered by the existing `internal/api` router tests
(green) + the live-stack integration tier.

## Blast Radius

- New: `internal/web/admin/{scaffold.go, devices.go, devices_test.go}`.
- Modified: `internal/api/health.go` (+ field), `internal/api/router.go` (+ mount),
  `cmd/core/wiring.go` (+ construction + import).
- No schema migration, no SST change, no change to the JSON endpoint or the
  certified tokens page.

## Alternatives Considered

- **Another bespoke static HTML blob** (like tokens.html). Rejected: re-creates
  the duplication BLOCKER-3 calls out; the scaffold is the generalization.
- **Introduce `templ`/HTMX as a dependency.** Rejected: the repo already
  standardizes on Go `html/template` (`agent_admin.go`); adding an engine is
  unjustified churn.
- **Rewrite the tokens page onto the scaffold now.** Rejected: it is certified +
  XSS-audited; rewriting risks a security regression for cosmetic parity. Safe
  optional follow-up.
- **A parallel aggregation query for the HTML view.** Rejected: reusing
  `extensiondevices.Store` guarantees the JSON and HTML views never diverge.
