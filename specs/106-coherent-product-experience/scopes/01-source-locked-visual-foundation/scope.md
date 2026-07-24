# SCOPE-106-01: Source-Locked Visual Assets And Appearance Foundation

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Tags:** foundation:true
**Depends On:** -

## Outcome

One same-origin, source-locked visual foundation supplies semantic tokens, typography, icons, component geometry, appearance preference, CSP metadata, and service-worker cache identity to server and PWA renderers without changing active navigation or domain behavior.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-009 Theme follows the user across renderers
  Given the shared asset package and appearance codec are loaded before first paint
  When System Light or Dark and Comfortable or Compact is selected
  Then server and PWA renderers consume the same token names typography state semantics and preference
  And forced colors and reduced motion remain platform-controlled
  And no credential or business value is stored with appearance
```

## Implementation Plan

1. Define one `ExperienceAssetManifest` with immutable path, source, license, SHA-256, size, media type, CSP class, and service-worker policy for every shared CSS, JavaScript, IBM Plex Sans, Source Serif 4, IBM Plex Mono, and icon byte.
2. Resolve every dependency through the repository's trusted source allowlist and lockfile; reuse BUG-002-006's exact HTMX asset rather than adding another copy or runtime CDN.
3. Implement one semantic token source covering the UX color roles, 4px spacing scale, 2-8px radii, stable shell/control dimensions, type tokens, focus, motion, and density. Renderer templates and page CSS may reference tokens but may not copy hardcoded colors or spacing.
4. Implement the closed benign appearance cookie `system|light|dark` plus `comfortable|compact`, explicit positive SST retention, pre-paint resolution, invalid-value diagnostics, production cookie attributes, and no localStorage authority.
5. Package familiar source-locked icons for icon-only controls; every icon-only control contract requires an accessible name and hover/focus tooltip, while consequential or destructive commands retain text.
6. Add mechanical rules for no nested cards, no card-styled page sections, stable dimensions, no viewport-width font scaling, no negative letter spacing, contrast, and no overlap or clipping.
7. Advance the service-worker asset identity atomically while keeping authenticated HTML, `/api/*`, `/v1/*`, non-GET requests, and business responses network-only.

## Shared Infrastructure Impact Sweep

Protected surfaces are the shared server head, every PWA head, Card head, CSP, HTMX source, service worker, web manifest, first-paint timing, appearance cookie, source allowlist, lockfile, and browser test bootstrap. Independent canaries cover one native server page, one HTMX read, one HTMX mutation, one PWA authenticated read, one Card PRG page, invalid asset bytes, and stale service-worker cache identity before any renderer migration.

## Rollback

Assets, manifest, CSP references, pre-paint code, and service-worker cache identity roll back as one immutable release pointer. Rollback does not fetch remote assets, weaken CSP, restore duplicated tokens, rewrite the appearance cookie, rebuild on the target, or touch domain data.

## Change Boundary

**Allowed:** shared experience asset sources, manifest/compiler, token/component CSS, same-origin font/icon bytes and licenses, appearance codec/config, server/PWA head adapters, service-worker static inventory, and focused tests.

**Excluded:** navigation cutover, product-data APIs, auth issuance, domain templates beyond asset consumption, Cards/Graph business behavior, foreign spec packets, spec 079, deployment adapters, knb, CCManager, release-train configuration, and managed readiness claims.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Test Title / Behavior | Command | Live System |
|---|---|---|---|---|---|---|---|
| XP106-01-U | Unit | `unit` | `internal/web/experience_assets_test.go` | SCN-106-009 | `TestExperienceAssetManifestLocksSourcesLicensesBytesTokensAndAppearanceEnums` | `./smackerel.sh test unit --go` | No |
| XP106-01-I | Integration | `integration` | `tests/integration/web/experience_assets_test.go` | SCN-106-009 | `TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP` | `./smackerel.sh test integration` | Yes |
| XP106-01-A | E2E API regression | `e2e-api` | `tests/e2e/experience_assets_e2e_test.go` | SCN-106-009 | `Experience assets expose immutable headers exact digests and network-only protected routes` | `./smackerel.sh test e2e` | Yes |
| XP106-01-W | E2E UI regression | `e2e-ui` | `web/pwa/tests/coherent_appearance.spec.ts` | SCN-106-009 | `source-locked appearance applies before first paint across server PWA and Card canaries` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-01-C | Shared-infrastructure canary | `e2e-ui` | `web/pwa/tests/coherent_foundation_canary.spec.ts` | SCN-106-009 | `asset cutover preserves native Search HTMX read HTMX mutation PWA auth Card PRG and service-worker isolation` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-009 Theme follows the user across renderers`: System, Light, Dark, Comfortable, and Compact resolve before first paint and remain coherent across server and PWA renderers while forced colors and reduced motion stay platform-controlled and no credential or business value enters appearance storage.
- [ ] One source-locked same-origin manifest and token source serves all renderers with explicit source, license, digest, CSP, and cache policy.
- [ ] Typography, icons, controls, stable dimensions, no-overlap, no-nested-card, contrast, forced-colors, and reduced-motion contracts are mechanically enforceable.
- [ ] Independent canaries and the immutable rollback unit protect every high-fan-out consumer before renderer migration.

#### Test Evidence - 5 Rows / 5 Items

- [ ] XP106-01-U passes with current-session evidence in `report.md#xp106-01-u`.
- [ ] XP106-01-I passes with current-session evidence in `report.md#xp106-01-i`.
- [ ] XP106-01-A passes with current-session evidence in `report.md#xp106-01-a`.
- [ ] XP106-01-W passes without interception or auth injection in `report.md#xp106-01-w`.
- [ ] XP106-01-C passes every independent shared-infrastructure canary in `report.md#xp106-01-c`.

#### Build Quality Gate

- [ ] Source locking, trusted-source allowlist, license inventory, CSP, service-worker safety, no-hardcoded-token, no-nested-card, contrast, check, lint, format, artifact lint, traceability, rollback, and directly affected security/design documentation checks pass with zero warnings.
