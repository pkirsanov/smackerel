# Scopes: BUG-058-001

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Resolves BLOCKER-3 of `../BUG-058-EXTERNAL-INFRA-MISSING/`.

## Scope 1 — Shared admin scaffold + extension-devices HTML page

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `internal/web/admin/scaffold.go`: reusable shared base layout + `navLinks` nav fragment + `AuthGate` (= `extensiondevices.AdminPredicate`) + `renderContent`/`renderPage`, in Go `html/template` (matching `internal/web/agent_admin.go`; no new engine, no static-embed shortcut)
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0); `## Test Evidence` (TestNavLinks_MarksActive PASS)
- [x] `internal/web/admin/devices.go`: `DevicesHandler` renders `GET /admin/extension/devices` as an HTML table on the scaffold, reusing `extensiondevices.Store.AggregateDevices` (no duplicated query)
      → Evidence: report.md `## Test Evidence` (TestDevicesHandler_RendersTableForAdmin PASS)
- [x] Auth + scoping: unauthenticated → 401; non-admin scoped to own `owner_user_id` (403 when absent); admin → all owners; GET-only (405); store error → 500
      → Evidence: report.md `## Test Evidence` (Unauthenticated/NonAdminScoped/NonAdminMissingOwner/StoreError/MethodNotAllowed PASS)
- [x] XSS-safe: user-influenced values are HTML-escaped (proven by an adversarial `<script>` test)
      → Evidence: report.md `## Test Evidence` (TestDevicesHandler_EscapesUserInfluencedValues PASS)
- [x] Same admin predicate as the JSON handler reused in wiring (no second auth primitive); fail-loud nil-arg constructor
      → Evidence: report.md `### Code Diff Evidence` (wiring.go reuses extStore + adminPredicate); `## Test Evidence` (TestNewDevicesHandler_NilArgsPanic PASS)
- [x] Route mounted at `GET /admin/extension/devices` behind `webAuthMiddleware` (same auth as `/admin/agent/*`)
      → Evidence: report.md `### Code Diff Evidence` (router.go + health.go)
- [x] `go build ./...`, `go vet`, the `internal/web/...` + `internal/api` + `internal/api/admin/extensiondevices` + `cmd/core` packages green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-058-001-01..02` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Tokens page deliberately NOT rewritten (certified XSS-audited surface; linked from nav); recorded as a no-regression decision
      → Evidence: report.md `## Completion Statement` + `bug.md` (deliberate no-shortcut decisions)
- [x] No regression — the existing `internal/web` + `internal/api` router suites stay green with the new mount
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-058-001-01 | TestDevicesHandler_RendersTableForAdmin / EmptyState / EscapesUserInfluencedValues | internal/web/admin/devices_test.go | unit (render) | SCN-058-001-01 |
| T-058-001-02 | TestDevicesHandler_{Unauthenticated,NonAdminScoped,NonAdminMissingOwner,StoreError,MethodNotAllowed} + NilArgsPanic + NavLinks_MarksActive | internal/web/admin/devices_test.go | unit (auth/scaffold) | SCN-058-001-02 |

### Non-Goals

- BLOCKER-1 (MV3 Playwright e2e) and BLOCKER-4 (CI sideload smoke).
- Rewriting the tokens page or changing the JSON endpoint.
