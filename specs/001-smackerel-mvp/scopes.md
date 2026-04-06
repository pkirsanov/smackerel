# Scopes: 001 -- Smackerel MVP (Cross-Cutting)

Links: [spec.md](spec.md) | [design.md](design.md)

This spec covers cross-cutting product concerns that span multiple phases. Phase-specific implementation scopes live in their own specs (002-006).

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
```

### Implementation Plan
- Design full SVG icon set on 24x24 grid, 1.5px stroke, round caps
- Source icons: mail, calendar, video, chat, bookmark, link, note, rss (8)
- Artifact icons: article, idea, person, place, book, recipe, bill, trip (8)
- Status icons: healthy, syncing, error, dormant (4)
- Action icons: capture, search, archive, resurface (4)
- Embed as Go template partials for web UI rendering
- CSS currentColor inheritance for theme adaptation
- Telegram text marker constants in Go

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | All icon SVGs exist and are valid | Unit | internal/web/icons_test.go | SCN-001-001 |
| 2 | Icons inherit currentColor | Unit | internal/web/icons_test.go | SCN-001-002 |
| 3 | Icon viewBox is 24x24 for all icons | Unit | internal/web/icons_test.go | SCN-001-003 |
| 4 | Telegram messages contain no emoji | Unit | internal/telegram/format_test.go | SCN-001-004 |
| 5 | Regression E2E: icons render in web UI | E2E | tests/e2e/test_icons.sh | SCN-001-001 |

### Definition of Done
- [ ] 24 SVG icons designed (8 source + 8 artifact + 4 status + 4 action)
- [ ] All icons follow 24x24 grid, 1.5px stroke, round cap, no fills
- [ ] Icons render correctly in light and dark themes via currentColor
- [ ] Icons render cleanly at 16px, 24px, 32px
- [ ] Go template partials for all icons
- [ ] Telegram text marker constants defined and used
- [ ] No emoji in any system output (bot, web, digest, API)
- [ ] Scenario-specific E2E regression tests for icon rendering
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
| 1 | Light theme palette applied | Unit | tests/visual/test_theme.html | SCN-001-005 |
| 2 | Dark theme palette applied | Unit | tests/visual/test_theme.html | SCN-001-006 |
| 3 | Mobile breakpoint works | Unit | tests/visual/test_responsive.html | SCN-001-007 |
| 4 | System fonts render | Unit | tests/visual/test_typography.html | SCN-001-008 |
| 5 | No accent colors in CSS | Unit | tests/lint/test_no_accent.sh | SCN-001-009 |
| 6 | Regression E2E: design system | E2E | tests/e2e/test_design_system.sh | SCN-001-005 |

### Definition of Done
- [ ] CSS custom properties define full monochrome palette (light + dark)
- [ ] Dark mode auto-detects from OS preference and supports manual toggle
- [ ] System font stack used, zero custom font downloads
- [ ] Responsive breakpoints: desktop 720px max, tablet, mobile 480px
- [ ] No accent colors, no colored badges, no blue links
- [ ] Cards, buttons, inputs, nav, toast, modal all styled
- [ ] Scenario-specific E2E regression tests
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
```

### Implementation Plan
- `Connector` interface in `internal/connector/connector.go`
- `ConnectorConfig` struct with auth, schedule, qualifiers, processing rules
- `ConnectorRegistry` for lifecycle management
- `OAuth2Provider` interface in `internal/auth/oauth.go`
- `GoogleOAuth2`, `MicrosoftOAuth2`, `GenericOAuth2` implementations
- Sync state CRUD on `sync_state` table
- Cron scheduler integration (robfig/cron)
- Rate limit detection and exponential backoff

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Connector interface satisfied by test impl | Unit | internal/connector/connector_test.go | SCN-001-010 |
| 2 | OAuth2 provider AuthURL generation | Unit | internal/auth/oauth_test.go | SCN-001-011 |
| 3 | IMAP connector works with different adapters | Integration | internal/connector/imap/imap_test.go | SCN-001-012 |
| 4 | Sync state persists and resumes | Integration | internal/connector/state_test.go | SCN-001-013 |
| 5 | Regression E2E: connector lifecycle | E2E | tests/e2e/test_connector_framework.sh | SCN-001-010 |

### Definition of Done
- [ ] Connector interface defined with ID, Connect, Sync, Health, Close
- [ ] ConnectorConfig supports auth, schedule, qualifiers, processing rules
- [ ] ConnectorRegistry registers and manages connector lifecycle
- [ ] OAuth2Provider interface with Google, Microsoft, Generic implementations
- [ ] Sync state CRUD on sync_state table
- [ ] Cursor-based incremental sync contract enforced
- [ ] Rate limit backoff with jitter
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 04-product-level-testing

**Status:** Not Started
**Priority:** P0
**Depends On:** Phase 1 complete

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
- [ ] Cross-phase integration (capture + passive ingestion + search) works
- [ ] Digest pipeline generates from multi-source data
- [ ] Data persists across compose down/up
- [ ] Full export produces portable database backup
- [ ] Search accuracy benchmark: 20 queries with >75% first-result accuracy
- [ ] Scenario-specific E2E regression tests for product flows
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
