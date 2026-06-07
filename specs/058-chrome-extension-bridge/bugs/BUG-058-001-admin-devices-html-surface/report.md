# Report: BUG-058-001 — extension-devices admin HTML surface on a shared admin scaffold

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Resolves:** `../BUG-058-EXTERNAL-INFRA-MISSING/` BLOCKER-3

## Summary

Spec 058's devices view shipped JSON-only and the repo had no shared admin-UI
scaffolding. This delivers a reusable `internal/web/admin` scaffold (shared base
layout + nav fragment + `AuthGate`) and the extension-devices HTML page
(`GET /admin/extension/devices`) on it, reusing the certified
`extensiondevices.Store` aggregation and the same admin predicate as the JSON
handler — no duplicated query, no second auth primitive.

## Root Cause

`internal/web/admin/` did not exist; the devices view was JSON-only.

## Fix

New `internal/web/admin` scaffold + `DevicesHandler`, mounted behind
`webAuthMiddleware`, wired by reusing the existing store + admin predicate.

## Test Evidence

### Scaffold + devices page (no DB — fake store + scripted gate)

```
$ go test -v -count=1 ./internal/web/admin/
--- PASS: TestDevicesHandler_RendersTableForAdmin (0.00s)
--- PASS: TestDevicesHandler_EmptyState (0.00s)
--- PASS: TestDevicesHandler_NonAdminScopedToOwnOwner (0.00s)
--- PASS: TestDevicesHandler_NonAdminMissingOwnerForbidden (0.00s)
--- PASS: TestDevicesHandler_UnauthenticatedRejected (0.00s)
--- PASS: TestDevicesHandler_StoreErrorIs500 (0.00s)
--- PASS: TestDevicesHandler_MethodNotAllowed (0.00s)
--- PASS: TestDevicesHandler_EscapesUserInfluencedValues (0.00s)
--- PASS: TestNewDevicesHandler_NilArgsPanic (0.00s)
--- PASS: TestNavLinks_MarksActive (0.00s)
ok      github.com/smackerel/smackerel/internal/web/admin       0.016s
```

`RendersTableForAdmin` asserts the shared scaffold chrome (DOCTYPE, nav links to
all three admin pages, the active highlight on the devices link) AND the device
rows; `NonAdminScopedToOwnOwner` proves the store is queried with the caller's
own owner filter; `EscapesUserInfluencedValues` proves a `<script>` device id is
HTML-escaped (XSS-safe by `html/template`); the auth tests prove 401/403/500/405.

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/web/admin/ ./internal/api/ ./cmd/core/
# VET=0
$ git diff --stat (modified) ; git status --short (new)
 cmd/core/wiring.go     |  5 +++++
 internal/api/health.go |  7 +++++++
 internal/api/router.go | 11 +++++++++++
?? internal/web/admin/scaffold.go
?? internal/web/admin/devices.go
?? internal/web/admin/devices_test.go
```

The wiring reuses the existing `extStore` + `adminPredicate` (built for the JSON
handler), so the diff is +23 lines of wiring/mount + the new package. No schema
migration.

### Validation Evidence

```
$ go test -count=1 ./internal/web/... ./internal/api/ ./internal/api/admin/extensiondevices/ ./cmd/core/
ok      github.com/smackerel/smackerel/internal/web     0.313s
ok      github.com/smackerel/smackerel/internal/web/admin       0.015s
ok      github.com/smackerel/smackerel/internal/web/icons       0.008s
ok      github.com/smackerel/smackerel/internal/api     15.229s
ok      github.com/smackerel/smackerel/internal/api/admin/extensiondevices      0.028s
ok      github.com/smackerel/smackerel/cmd/core 1.150s
```

Every affected package returns `ok` — the new route mount did not regress the
existing `internal/web` or `internal/api` router suites.

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/' || echo "(empty — no migration)"
(empty — no migration)
$ go vet ./internal/web/admin/
# (clean — no output)
```

The diff is confined to the new `internal/web/admin` package and the three
wiring/mount touch-points. No schema migration, no SST change, no change to the
JSON endpoint or the certified tokens page, no `.github/bubbles` framework files.

## Completion Statement

The extension-devices admin view is now available as a rendered HTML page at
`GET /admin/extension/devices` on a new reusable `internal/web/admin` scaffold
(shared base layout + nav fragment + `AuthGate`), built in Go `html/template`
matching the repo convention. It reuses the certified
`extensiondevices.Store.AggregateDevices` and the same admin predicate as the
JSON handler — no duplicated query, no second auth primitive — with
unauthenticated→401, non-admin→own-owner-scoped (403 when absent), admin→all,
GET-only, and XSS-safe escaping all proven by unit tests (10/10 PASS).
build/vet/the affected packages are green. The certified `/admin/auth/tokens`
static page was deliberately left intact (linked from the shared nav) to avoid a
security regression — a recorded no-shortcut decision. Scope 1 DoD is complete
(10/10). BUG-058-001 is Done and discharges BUG-058-EXTERNAL-INFRA-MISSING
BLOCKER-3; spec 058 remains `blocked` pending BLOCKER-1 and BLOCKER-4.
