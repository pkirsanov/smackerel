# Scopes: 001 -- Smackerel MVP (Cross-Cutting)

Links: [spec.md](spec.md) | [design.md](design.md)

This spec covers cross-cutting product concerns that span multiple phases. Phase-specific implementation scopes live in their own specs (002-006).

---

## Execution Outline

### Phase Order

1. **Scope 01: Monochrome Icon System** — Full SVG icon set (32 icons across 5 categories) and Telegram text markers. Visual foundation for all UI surfaces.
2. **Scope 02: Design System CSS** — CSS custom properties, typography, responsive layout, dark/light themes. Depends on Scope 01 (icons used in component styling).
3. **Scope 03: Generic Connector Framework** — Connector interface, OAuth2 abstraction, sync state contract, error recovery. Independent of Scopes 01-02; foundational for Phase 2+ connectors.
4. **Scope 04: Product-Level Testing** — Cross-phase E2E validation, search accuracy benchmarks, data persistence. Depends on all other scopes and phase-spec implementations.

### New Types & Signatures

- `Connector` interface: `ID() string`, `Connect(ctx, config) error`, `Sync(ctx, cursor) ([]RawArtifact, string, error)`, `Health(ctx) HealthStatus`, `Close() error`
- `ConnectorConfig` struct: auth, schedule, qualifiers, processing rules
- `ConnectorRegistry`: register/unregister/get connector lifecycle
- `OAuth2Provider` interface: `AuthURL(scopes)`, `ExchangeCode(code)`, `RefreshToken(refresh)`
- `HealthStatus` enum: `healthy | syncing | error | disconnected`
- CSS custom properties: `--fg`, `--bg`, `--subtle`, `--surface`, `--divider` (light + dark)
- SVG icon partials: 32 icons (8 source + 8 artifact + 4 status + 4 action + 8 navigation)
- Telegram text markers: `. ? ! > - ~ # @`

### Validation Checkpoints

- After Scope 01: All 32 icons render in test HTML; Telegram formatter produces zero emoji
- After Scope 02: Theme toggle works, responsive breakpoints verified, all component styles applied
- After Scope 03: Test connector passes full lifecycle; sync state round-trips; error recovery verified
- After Scope 04: Cross-phase E2E flows pass; search accuracy ≥75% on vague queries

### Scope Summary Table

| # | Scope | Surfaces | Key Tests | Status |
|---|-------|----------|-----------|--------|
| 01 | Monochrome Icon System | Web UI, Telegram | Icon catalog validation, theme adaptation, text marker enforcement | Not Started |
| 02 | Design System CSS | Web UI | Theme toggle, responsive layout, typography, palette enforcement | Not Started |
| 03 | Generic Connector Framework | Backend, Config | Interface contract, OAuth2 flow, sync state, error recovery | Not Started |
| 04 | Product-Level Testing | All | Cross-phase E2E, search accuracy benchmark, data persistence | Not Started |

---

## Scope: 01-monochrome-icon-system

**Status:** Not Started
**Priority:** P0
**Depends On:** None (ships with Phase 1)

### Gherkin Scenarios

```gherkin
Scenario: SCN-001-001 Icon set covers all categories
  Given the icon system defines source, artifact, status, and action categories
  When the full icon catalog is reviewed
  Then every source type has a unique monochrome icon
  And every artifact type has a unique monochrome icon
  And every status state has a unique monochrome icon
  And every action has a unique monochrome icon
  And no icon uses emoji, color fills, or external icon library glyphs

Scenario: SCN-001-002 Icons adapt to light and dark theme
  Given the web UI supports light and dark themes
  When the theme is toggled
  Then all icons inherit their color from CSS currentColor
  And remain visible with adequate contrast in both themes

Scenario: SCN-001-003 Icons render at multiple sizes
  Given icons are designed on a 24x24 grid
  When rendered at 16px, 24px, and 32px
  Then stroke weight remains visually consistent
  And icons remain recognizable at all sizes

Scenario: SCN-001-004 Telegram bot uses text markers only
  Given the Telegram bot cannot render SVG icons
  When the bot sends messages
  Then it uses the text marker system (. ? ! > - ~ # @)
  And never uses emoji characters

Scenario: SCN-001-018 Navigation and UI chrome icons exist
  Given the web UI requires navigation and utility controls
  When the navigation icon catalog is reviewed
  Then 8 navigation/chrome icons exist (menu, back, expand, collapse, filter, settings, close, refresh)
  And they follow the same 24x24 grid, 1.5px stroke, round cap rules as all other icons
```

### Implementation Plan
- Design full SVG icon set on 24x24 grid, 1.5px stroke, round caps
- Source icons: mail, calendar, video, chat, bookmark, link, note, rss (8)
- Artifact icons: article, idea, person, place, book, recipe, bill, trip (8)
- Status icons: healthy, syncing, error, dormant (4)
- Action icons: capture, search, archive, resurface (4)
- Navigation/UI chrome icons: menu, back, expand, collapse, filter, settings, close, refresh (8)
- Embed as Go template partials for web UI rendering
- CSS currentColor inheritance for theme adaptation
- Telegram text marker constants in Go

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | All 32 icon SVGs exist, valid XML, valid viewBox | Unit | internal/web/icons_test.go | SCN-001-001 |
| 2 | No emoji chars, color fills, or external icon glyphs in SVGs | Unit | internal/web/icons_test.go | SCN-001-001 |
| 3 | Icons inherit currentColor — no hardcoded color values | Unit | internal/web/icons_test.go | SCN-001-002 |
| 4 | Icon viewBox is 24x24 and stroke is 1.5px for all icons | Unit | internal/web/icons_test.go | SCN-001-003 |
| 5 | Telegram formatter output contains no emoji characters | Unit | internal/telegram/format_test.go | SCN-001-004 |
| 6 | Telegram text markers match defined set (. ? ! > - ~ # @) | Unit | internal/telegram/format_test.go | SCN-001-004 |
| 7 | Navigation/chrome icons follow same grid and stroke rules | Unit | internal/web/icons_test.go | SCN-001-018 |
| 8 | Regression E2E: icons render in web UI across themes | E2E | tests/e2e/test_icons.sh | SCN-001-001, SCN-001-002 |
| 9 | Regression E2E: icons render at 16/24/32px sizes | E2E | tests/e2e/test_icons.sh | SCN-001-003 |
| 10 | Regression E2E: Telegram output uses text markers, no emoji | E2E | tests/e2e/test_telegram_format.sh | SCN-001-004 |

### Definition of Done
- [ ] 32 SVG icons designed (8 source + 8 artifact + 4 status + 4 action + 8 navigation)
- [ ] All icons follow 24x24 grid, 1.5px stroke, round cap, round join, no fills, no gradients
- [ ] Icons render correctly in light and dark themes via CSS currentColor
- [ ] Icons render cleanly at 16px, 24px, 32px with consistent visual weight
- [ ] Go template partials for all 32 icons
- [ ] Telegram text marker constants defined (. ? ! > - ~ # @) and used in all bot output
- [ ] No emoji in any system output (bot, web, digest, API)
- [ ] No external icon library imports (FontAwesome, Material Icons, etc.)
- [ ] Scenario-specific E2E regression tests for icon rendering, sizing, and text markers
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 02-design-system-css

**Status:** Not Started
**Priority:** P0
**Depends On:** 01-monochrome-icon-system

### Gherkin Scenarios

```gherkin
Scenario: SCN-001-005 Light theme renders correctly
  Given the user visits the web UI in light mode
  When the page loads
  Then background is warm white (#FAFAF8), text is warm black (#2C2C2C)
  And all elements use the monochrome palette with no accent colors

Scenario: SCN-001-006 Dark theme renders correctly
  Given the user prefers dark mode (OS setting or manual toggle)
  When the page loads
  Then background is warm near-black (#1A1A18), text is warm off-white (#E8E6E3)
  And all elements adapt via CSS custom properties

Scenario: SCN-001-007 Responsive layout at mobile width
  Given the viewport is under 480px
  When the web UI renders
  Then navigation becomes a hamburger menu
  And content fills full width with 12px padding
  And tap targets are at least 44px

Scenario: SCN-001-008 Typography uses system fonts
  Given no custom fonts are loaded
  When text renders
  Then it uses the system font stack (system-ui, -apple-system, etc.)
  And body text is 16px with 1.5 line-height

Scenario: SCN-001-009 No accent colors anywhere
  Given the monochrome design mandate
  When any UI element is inspected
  Then no blue links, no colored badges, no accent highlights exist
  And links use foreground color with underline
```

### Implementation Plan
- CSS custom properties for the monochrome palette (light + dark)
- `prefers-color-scheme` media query for auto dark mode
- Manual dark mode toggle stored in localStorage
- System font stack, no custom font loading
- Type scale: body 16px, small 14px, h1 24px, h2 20px, h3 16px/500
- Base layout: 720px max-width centered, single column
- Responsive breakpoints: >720px desktop, 480-720px tablet, <480px mobile
- Component styles: cards, buttons, inputs, nav, toast, modal

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Light theme palette: warm white bg (#FAFAF8), warm black fg (#2C2C2C) | Functional | tests/visual/test_theme.html | SCN-001-005 |
| 2 | Dark theme palette: warm near-black bg (#1A1A18), off-white fg (#E8E6E3) | Functional | tests/visual/test_theme.html | SCN-001-006 |
| 3 | Dark mode auto-detects via prefers-color-scheme media query | Functional | tests/visual/test_theme.html | SCN-001-006 |
| 4 | Manual dark mode toggle persists selection in localStorage | Functional | tests/visual/test_theme.html | SCN-001-006 |
| 5 | Mobile breakpoint (<480px): hamburger nav, full-width, 44px tap targets | Functional | tests/visual/test_responsive.html | SCN-001-007 |
| 6 | System font stack renders (no custom @font-face declarations) | Functional | tests/visual/test_typography.html | SCN-001-008 |
| 7 | No accent colors in any CSS rule (no blue, no colored badges) | Unit | tests/lint/test_no_accent.sh | SCN-001-009 |
| 8 | Links use foreground color with underline, not blue | Unit | tests/lint/test_no_accent.sh | SCN-001-009 |
| 9 | Regression E2E: light and dark theme render correctly | E2E | tests/e2e/test_design_system.sh | SCN-001-005, SCN-001-006 |
| 10 | Regression E2E: responsive layout at mobile width | E2E | tests/e2e/test_design_system.sh | SCN-001-007 |
| 11 | Regression E2E: monochrome palette enforcement (zero accent colors) | E2E | tests/e2e/test_design_system.sh | SCN-001-009 |

### Definition of Done
- [ ] CSS custom properties define full monochrome palette (light + dark) per design spec
- [ ] Dark mode auto-detects from OS prefers-color-scheme
- [ ] Manual dark mode toggle works and persists state in localStorage
- [ ] System font stack used (system-ui, -apple-system, etc.), zero custom font downloads
- [ ] Type scale: body 16px/1.5, small 14px/1.4, h1 24px/1.2, h2 20px/1.2, h3 16px/500
- [ ] Base layout: 720px max-width centered, single column
- [ ] Responsive breakpoints: desktop >720px, tablet 480-720px, mobile <480px
- [ ] Mobile: hamburger nav, full-width with 12px padding, 44px minimum tap targets
- [ ] No accent colors, no colored badges, no blue links anywhere
- [ ] Links use foreground color with underline; hover/focus at 50% opacity underline
- [ ] Cards, buttons, inputs, nav, toast, modal, capture overlay all styled
- [ ] Scenario-specific E2E regression tests for theme, responsive, palette
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 03-generic-connector-framework

**Status:** Not Started
**Priority:** P0
**Depends On:** None (ships early in Phase 1, used by Phase 2)

### Gherkin Scenarios

```gherkin
Scenario: SCN-001-010 Connector interface contract
  Given the Connector interface defines ID, Connect, Sync, Health, Close
  When a new connector is implemented (e.g., IMAP)
  Then it implements all interface methods
  And can be registered with the ConnectorRegistry

Scenario: SCN-001-011 OAuth2 provider abstraction
  Given OAuth2Provider interface defines AuthURL, ExchangeCode, RefreshToken
  When GoogleOAuth2 is configured
  Then one OAuth consent screen covers Gmail IMAP + Calendar + YouTube

Scenario: SCN-001-012 Protocol layer reuse
  Given the IMAPConnector handles IMAP SEARCH, FETCH, flags
  When Gmail and Outlook both use IMAPConnector
  Then only the auth adapter and qualifier mapping differ

Scenario: SCN-001-013 Sync state contract
  Given a connector completes a sync cycle
  When sync state is persisted
  Then source_id, cursor, last_sync, items_synced, error_count are recorded
  And the next sync resumes from the cursor

Scenario: SCN-001-019 Rate limit detection and exponential backoff
  Given a connector receives a rate limit response (HTTP 429)
  When the backoff policy is applied
  Then retries follow exponential backoff with jitter (1s, 2s, 4s, 8s, 16s)
  And after max retries the current sync cycle is skipped
  And the next scheduled sync cycle proceeds normally

Scenario: SCN-001-020 Connector health status reporting
  Given a connector can be in one of four states
  When the health endpoint is queried for a registered connector
  Then it reports one of: healthy, syncing, error, disconnected
  And status transitions follow the connector state machine

Scenario: SCN-001-021 Connector error recovery and dead-letter
  Given a connector encounters a non-recoverable processing failure
  When the item has failed 3 delivery attempts via NATS
  Then the item is moved to a dead-letter queue
  And the connector continues processing remaining items
  And the supervisor restarts the connector goroutine on crash
```

### Implementation Plan
- `Connector` interface in `internal/connector/connector.go`
- `ConnectorConfig` struct with auth, schedule, qualifiers, processing rules
- `ConnectorRegistry` for lifecycle management
- `OAuth2Provider` interface in `internal/auth/oauth.go`
- `GoogleOAuth2`, `MicrosoftOAuth2`, `GenericOAuth2` implementations
- Sync state CRUD on `sync_state` table
- Cron scheduler integration (robfig/cron)
- Rate limit detection and exponential backoff with jitter
- `HealthStatus` enum: healthy, syncing, error, disconnected
- Dead-letter queue for items failing 3 delivery attempts
- Connector supervisor goroutine with crash recovery

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Connector interface satisfied by test implementation | Unit | internal/connector/connector_test.go | SCN-001-010 |
| 2 | ConnectorRegistry registers, retrieves, and unregisters connectors | Unit | internal/connector/registry_test.go | SCN-001-010 |
| 3 | OAuth2 provider AuthURL generation with correct scopes | Unit | internal/auth/oauth_test.go | SCN-001-011 |
| 4 | OAuth2 token refresh flow returns valid access token | Unit | internal/auth/oauth_test.go | SCN-001-011 |
| 5 | IMAP connector works with different provider adapters | Integration | internal/connector/imap/imap_test.go | SCN-001-012 |
| 6 | Sync state persists cursor and resumes from it | Integration | internal/connector/state_test.go | SCN-001-013 |
| 7 | Sync state records items_synced and error_count | Integration | internal/connector/state_test.go | SCN-001-013 |
| 8 | Rate limit backoff follows exponential schedule with jitter | Unit | internal/connector/backoff_test.go | SCN-001-019 |
| 9 | Max retries reached skips current sync cycle | Unit | internal/connector/backoff_test.go | SCN-001-019 |
| 10 | Connector health reports correct status enum values | Unit | internal/connector/health_test.go | SCN-001-020 |
| 11 | Dead-letter queue receives items after 3 failed delivery attempts | Integration | internal/connector/deadletter_test.go | SCN-001-021 |
| 12 | Connector supervisor restarts goroutine on crash | Integration | internal/connector/supervisor_test.go | SCN-001-021 |
| 13 | Regression E2E: connector full lifecycle (register → connect → sync → health → close) | E2E | tests/e2e/test_connector_framework.sh | SCN-001-010 |
| 14 | Regression E2E: sync state round-trip persistence across restarts | E2E | tests/e2e/test_connector_framework.sh | SCN-001-013 |
| 15 | Regression E2E: error recovery with dead-letter and supervisor restart | E2E | tests/e2e/test_connector_framework.sh | SCN-001-021 |

### Definition of Done
- [ ] Connector interface defined with ID, Connect, Sync, Health, Close
- [ ] ConnectorConfig supports auth, schedule, qualifiers, processing rules
- [ ] ConnectorRegistry registers, retrieves, and manages connector lifecycle
- [ ] OAuth2Provider interface with Google, Microsoft, Generic implementations
- [ ] Single Google OAuth consent covers Gmail IMAP + Calendar + YouTube
- [ ] Sync state CRUD on sync_state table (source_id, cursor, last_sync, items_synced, error_count)
- [ ] Cursor-based incremental sync contract enforced
- [ ] Rate limit backoff with jitter (exponential: 1s → 2s → 4s → 8s → 16s)
- [ ] HealthStatus enum: healthy, syncing, error, disconnected
- [ ] Dead-letter queue after 3 failed delivery attempts
- [ ] Connector supervisor with crash recovery (goroutine restart)
- [ ] Cron scheduler integration (robfig/cron) verified
- [ ] Scenario-specific E2E regression tests for lifecycle, sync state, error recovery
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 04-product-level-testing

**Status:** Not Started
**Priority:** P0
**Depends On:** 01, 02, 03 (within this spec); phase-spec implementations (002-006)

### Gherkin Scenarios

```gherkin
Scenario: SCN-001-014 End-to-end capture-to-search
  Given the full system is running via docker compose
  When a user captures 10 articles via Telegram over 3 days
  And searches for one with a vague query
  Then the correct article appears as the top result

Scenario: SCN-001-015 Cross-phase integration
  Given Phase 1 (capture + search) and Phase 2 (passive ingestion) are deployed
  When Gmail passively ingests 50 emails
  And the user searches for one with a vague description
  Then the email artifact is found via semantic search

Scenario: SCN-001-016 Full digest pipeline
  Given artifacts from capture, email, and YouTube exist
  When the daily digest generates
  Then it includes action items from email, topic trends from all sources
  And is delivered via both web and Telegram

Scenario: SCN-001-017 Data persistence and portability
  Given the system has been running for 7 days with 200+ artifacts
  When docker compose is restarted
  Then all data is preserved
  And a full database export produces a portable backup
```

### Implementation Plan
- E2E test suite covering cross-phase flows
- Test data seeding scripts for realistic 7-day dataset
- Search accuracy benchmark: 20 vague queries with expected results
- Digest quality check: generated digests reviewed for content and length
- Data persistence verification across compose restarts
- Export/import verification

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Capture to search end-to-end | E2E | tests/e2e/test_capture_to_search.sh | SCN-001-014 |
| 2 | Gmail + search cross-phase | E2E | tests/e2e/test_cross_phase.sh | SCN-001-015 |
| 3 | Digest includes cross-source data | E2E | tests/e2e/test_digest_pipeline.sh | SCN-001-016 |
| 4 | Data survives compose restart | E2E | tests/e2e/test_persistence.sh | SCN-001-017 |
| 5 | Search accuracy benchmark | Stress | tests/benchmark/test_search_accuracy.sh | SCN-001-014 |
| 6 | Regression E2E: product flows | E2E | tests/e2e/test_product_flows.sh | SCN-001-014 |

### Definition of Done
- [ ] E2E capture-to-search flow passes with >75% accuracy on vague queries
- [ ] Cross-phase integration (capture + passive ingestion + search) verified
- [ ] Digest pipeline generates from multi-source data (email, YouTube, active capture)
- [ ] Data persists across docker compose down/up cycle (volumes intact)
- [ ] Full export produces portable JSONL backup with documented schema
- [ ] Search accuracy benchmark: 20 vague queries with >75% first-result accuracy
- [ ] Test data seeding scripts produce realistic 7-day dataset (200+ artifacts)
- [ ] Search accuracy methodology and benchmark results documented
- [ ] Scenario-specific E2E regression tests for each product flow (capture→search, cross-phase, digest, persistence, export)
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
