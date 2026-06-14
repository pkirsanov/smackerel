# Scopes 092 — Card-Rewards Web UI Elevation (match-or-exceed CCManager)

**Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **Evidence:** [report.md](report.md)
**Workflow mode:** full-delivery · **Status ceiling:** done · **Release train:** mvp · **Scope layout:** single-file

> **Owner:** bubbles.plan authored this plan. bubbles.implement executes it starting at **SCOPE-01**.
> This is a **CSS-only re-skin** of the ten server-rendered `/cards` pages — a **two-file template diff**
> (`internal/web/cardrewards_templates.go` + `internal/web/cardrewards_dashboard_templates.go`) with
> **zero Go change** (Decision (d), design §2.2) and **zero CSP change** (Decision (a), design §2.1).

---

## Execution Outline (read first — alignment checkpoint)

### Phase Order (sequential; DAG below)
1. **SCOPE-01 — Design-system foundation** (`foundation:true`). Rewrite the shared `{{define "head"}}` embedded `<style>` (design-token system + component CSS), the shell wrappers (`head`/`foot`), and the responsive glass `{{define "cardrewards-nav"}}` in `cardrewards_templates.go`. Every page depends on this; it ships the token system + nav + component vocabulary the other scopes consume.
2. **SCOPE-02 — Scope-10 pages restyle** (`cardrewards_templates.go` page bodies): wallet (+add/+add-custom/+edit), offers (+edit), selections (+edit), **bonuses** (progress bar), categories, and the two `card-select` partials.
3. **SCOPE-03 — Scope-11 pages restyle** (`cardrewards_dashboard_templates.go` page bodies): dashboard (stat-grid + action-priority), recommendations, rotating (confidence meter), report, admin.
4. **SCOPE-04 — Consolidated verification + live deploy proof**: full card-rewards suite (Go render + all e2e-ui specs + CSP guard) green across all ten pages, the `data-*` preservation diff proof (§7), and the live rebuild/render proof (combined with spec 091's deploy).

### New Types & Signatures (the "header file" of this change)
- **CSS design tokens** (design §3, both themes, exact hex): `--bg-primary/secondary/tertiary/elevated`, `--text-primary/secondary/tertiary`, `--accent`/`--accent-hover`/`--on-accent`/`--on-danger`, `--success/-warning/-danger/-info/-starred` + matching `--*-soft` badge tints + `--neutral-soft`, `--border`/`--border-strong`, `--shadow-sm/md/lg`, `--radius-sm/md/lg/xl`, `--nav-bg`/`--nav-border`.
- **CSS component classes** (design §4): `.app-container`, `.main-content` (820→1200px), `.cr-nav`/`.cr-nav__list`/`.nav-pill`(+`.active`/`[aria-current]`), `.page-header`/`.page-title`/`.page-subtitle`, `.card`/`.card-header`/`.card-title`/`.card-grid`/`.meta`, `.stats-grid`/`.stat-card`(+`--link`/`--urgent`)/`.stat-value`/`.stat-label`, `.badge`(+`-success/-warning/-danger/-info/-neutral/-starred`), `.btn`(+`-primary/-secondary/-ghost/-danger/-sm`), `.progress`/`.progress-fill`(+`--success/-warning/-danger`), `.tag`/`.chip`, `.empty`(alias)/`.empty-state`, `.alert`(+`-info/-warning/-danger`), `.table-wrap`/`.cr-table`, `.search-box`(alias `.form-control`)/`.form-section`/`.form-row`. Breakpoints **480 / 768 / 1024px**.
- **The ONLY inline style in the surface** (design §4.8/§6): `style="width:NN%"` on `.progress-fill`, value server-computed by the existing `pct`/`confpct` `FuncMap` helpers (no JS) — first consumed by **bonuses** (SCOPE-02) and **rotating** (SCOPE-03).
- **New test files (test code only — outside the AC-11 production-scope boundary):**
  - `internal/web/cardrewards_render_test.go` (Go unit; SCOPE-01) — constructs `CardRewardsWebHandler` and renders all ten pages + sub-pages + partials, asserting parse/render success + design-system markers + Go-level CSP-clean. Closes the gap that `handler_test.go` only scans the KB `allTemplates`, never `cardRewardsTemplates`.
  - `web/pwa/tests/cardrewards_chrome.spec.ts` (e2e-ui; SCOPE-01) — responsive nav + dark-mode token application + CSP-clean on a representative `/cards` page.
  - `web/pwa/tests/cardrewards_bonuses.spec.ts` (e2e-ui; SCOPE-02) — **closes the bonuses regression gap** (`/cards/bonuses` has no existing spec and no Go test): `data-bonus-*` regression locators + progress-bar elevated assertions.
- **Zero new Go production type / field / route** (Decision (d)). `internal/web/cardrewards.go` and `internal/web/templates.go` are untouched.

### Validation Checkpoints (between phases — catch breakage before the next scope)
- **After SCOPE-01:** `./smackerel.sh check` + `./smackerel.sh test unit --go --go-run 'CardRewards'` + `./smackerel.sh test e2e-ui` (existing 7 specs MUST stay green under the new chrome — the foundation must not break any `data-*` locator or CSP). This is the gate that proves the design system applies without regressing the page bodies (which still carry their old markup at this point, restyled by the shared classes).
- **After SCOPE-02 and SCOPE-03:** the same trio, scoped to the touched specs + the new bonuses spec — proves each page group's `data-*` survived the body restructure and the elevated affordances (badges/progress/stat-grid/tables) render.
- **After SCOPE-04:** the full trio (no `--go-run` filter) + the `data-*` diff proof + the live rebuild/render proof — the consolidated green bar across all ten pages.

---

## Scope Table

| # | Scope | Primary file(s) | Status | Depends On | Tests (rows) | DoD test items |
|---|-------|-----------------|--------|------------|--------------|----------------|
| 01 | Design-system foundation (tokens + component CSS + responsive nav chrome) `foundation:true` | `cardrewards_templates.go` (`head`/`foot`/`cardrewards-nav`) | Done | — | 4 | 4 |
| 02 | Scope-10 pages restyle (wallet, offers, selections, bonuses, categories + sub-pages + card-select) | `cardrewards_templates.go` (page bodies) | Done | SCOPE-01 | 5 | 5 |
| 03 | Scope-11 pages restyle (dashboard, recommendations, rotating, report, admin) | `cardrewards_dashboard_templates.go` | Done | SCOPE-01 | 5 | 5 |
| 04 | Consolidated verification + live deploy proof | — (verification only; no prod edit) | Done | SCOPE-01, SCOPE-02, SCOPE-03 | 4 | 4 |

### Dependency DAG

```
SCOPE-01 (foundation)
   ├──> SCOPE-02 (Scope-10 page bodies; same file as SCOPE-01)
   └──> SCOPE-03 (Scope-11 page bodies; cardrewards_dashboard_templates.go)
            └──> SCOPE-04 (depends on 01 + 02 + 03 — full-surface verification)
```

Sequential execution order: **01 → 02 → 03 → 04**. SCOPE-02 and SCOPE-03 both consume SCOPE-01's design system; SCOPE-03 edits a different file than SCOPE-01/02 but is gated behind SCOPE-01 (it needs the shared chrome/CSS). SCOPE-04 verifies the whole surface and therefore depends on all three.

---

## Planning Notes / Findings (binding context for implement + test)

1. **Bonuses regression gap (test-coverage finding).** `/cards/bonuses` has **no** existing Playwright spec and **no** `handler_test.go` case — its `data-bonus-id` / `data-met` / `data-bonus-progress` / `data-bonus-met` hooks (enumerated in AC-9 as "preserved (regression)") are currently guarded by **nothing**. Design §9.2 says "add to the corresponding existing spec file", but the corresponding file does not exist. The plan therefore creates a **new** `web/pwa/tests/cardrewards_bonuses.spec.ts` (a new FILE inside the existing `e2e-ui` category — not a new test category) in SCOPE-02 to give the bonuses `data-*` contract + the new progress bar a real live-stack guard.
2. **Cardrewards templates have no Go-level parse/render coverage today.** `cardRewardsTemplates` is parsed via `template.Must(...)` inside the `CardRewardsWebHandler` constructor (`cardrewards.go:136`), and **no `_test.go` exercises that constructor**. `handler_test.go`'s `TestAllTemplates_Present` / `TestTemplates_NoInlineEventHandlers` scan only the knowledge-base `allTemplates` const, never `cardRewardsTemplates`. So `./smackerel.sh test unit --go` does **not** currently validate the cardrewards templates parse or render. The plan adds `internal/web/cardrewards_render_test.go` in SCOPE-01 so `test unit --go` genuinely proves parse + render + Go-level CSP-cleanliness for the cardrewards set, mirroring the KB guard for the surface that this feature actually changes.
3. **Report is covered (not a gap).** `/cards/report` is exercised by `cardrewards_dashboard.spec.ts` (block K06: `data-report-row`/`data-report-card`/`data-report-reason`). The report elevated assertions (`.cr-table`) attach to that existing spec in SCOPE-03 — no new report spec.
4. **The `data-*` contract is the regression contract.** Design §7 is the authoritative 60+ row old→new element map. Every existing locator stays 1:1; a dropped/renamed hook fails an existing (or, for bonuses, the newly added) spec. SCOPE-04 additionally diffs the post-redesign `data-*` set against the pre-redesign set to prove zero removals/renames (AC-9).
5. **CSP-clean is enforced two ways:** the Go render test asserts the cardrewards markup contains no `<script` / `onclick=` / `onsubmit=` / `onload=` (SCOPE-01), and every e2e-ui spec calls `attachCSPGuard`/`assertNoCSPViolations` (`_support/csp.ts`, spec 077) so any inline script/handler or CSP-blocked resource fires `securitypolicyviolation` → fails the test. The only inline style is the server-computed `.progress-fill` width (design §6).

---

## DoD Evidence Standard (applies to EVERY DoD item — read before implement)

1. **No box is pre-checked.** Every `- [ ]` starts unchecked; bubbles.implement/test checks it **only** after recording the evidence.
2. **Each checked item needs ≥10 lines of raw terminal output** captured **in `report.md`** at the linked anchor (no agent-written summaries — raw command + exit code + output).
3. **Build Quality Gate is a single grouped item** per the Tiered DoD model (zero-warning `check`/`lint`/`format`, `regression-quality-guard.sh` on touched specs, `data-*` integrity, docs/evidence alignment) — recorded as one evidence block, not split into catch-all duplicates.
4. **PII-generic evidence:** redact absolute home paths to `~/...` before staging (gitleaks `linux-home-username-leak`); these pages render no secrets.
5. **Test Plan ↔ DoD parity** is asserted per scope below: `Test Plan row count == test-related DoD checkbox count`.

---

## SCOPE-01 — Design-system foundation (tokens + component CSS + responsive nav chrome)

**Status:** Done
**Depends On:** (none) · **Foundation:** `true` (design §8 — the single shared design-system capability all ten pages compose)
**Files:**
  - `internal/web/cardrewards_templates.go` — `{{define "head"}}` (replace token `:root` + dark `@media` + `<style>` body), `{{define "foot"}}` (close the shell), `{{define "cardrewards-nav"}}` (responsive glass nav).
  - NEW `internal/web/cardrewards_render_test.go` (Go unit).
  - NEW `web/pwa/tests/cardrewards_chrome.spec.ts` (e2e-ui).

### Use Cases (Gherkin — traced to spec)

```gherkin
Scenario: SCOPE-01-A Responsive nav (UC-6 / AC-7)
  Given the operator opens a /cards page on a narrow (<768px) viewport
  Then the .cr-nav__list is a single-row horizontally-scrollable pill strip
    (scrollWidth > clientWidth) and each .nav-pill is at least 44px tall
  When the operator opens the same page on a wide (>=768px) viewport
  Then the .cr-nav__list wraps into a full-width pill row
  And the .cr-nav is position:sticky at the top in both cases

Scenario: SCOPE-01-B Dark-mode token parity (UC-7 / AC-1)
  Given the operator's environment prefers a dark color scheme
  When any /cards page renders
  Then the dark @media token block applies and a token-driven computed property
    (e.g. body background-color) DIFFERS from the light render
  When the environment prefers light
  Then the light token values apply

Scenario: SCOPE-01-C CSP-clean foundation (UC-9 / AC-8)
  Given the rewritten head/foot/nav chrome
  Then it introduces no inline <script> and no inline event handler
  And a representative /cards page load emits zero CSP violations
    (attachCSPGuard buffer stays empty)

Scenario: SCOPE-01-D Chrome preserves structure (UC-8 / AC-9 / AC-11)
  Given the foundation rewrites only head/foot/nav (not page bodies)
  When all ten /cards pages render under the new shell + design system
  Then every existing data-* hook on every page is still present
  And internal/web/cardrewards.go and internal/web/templates.go are unchanged
```

### Implementation Plan (cite design.md)

- **Token system** — replace the minimal `:root{ --bg; --fg; … --shadow }` (design §3 says `cardrewards_templates.go` lines 18–19) with design **§3.1** (LIGHT `:root`, exact hex) + **§3.2** (`@media (prefers-color-scheme: dark)` re-declaring colors/shadows/nav; radii declared once). Use the **§3.3** token→role map so no literal hex appears in markup.
- **Component CSS** — replace the `<style>` body with the design **§4** rule-by-rule contract: §4.1 reset + app-shell (**remove** `body{max-width:820px;margin:0 auto;padding:1rem}`; add `.app-container` + `.main-content` max-width **1200px**, `padding:16px` → `24px 32px` ≥768px), §4.2 nav, §4.3 page-header, §4.4 card/`.card-grid`/`.meta`, §4.5 `.stats-grid`/`.stat-card`(+`--link`/`--urgent`), §4.6 badge system, §4.7 button hierarchy, §4.8 `.progress`/`.progress-fill` (+threshold modifiers — the class is defined here; first consumed in SCOPE-02/03), §4.9 tag/chip + `.empty`/`.empty-state` alias + alert, §4.10 `.table-wrap`/`.cr-table`, §4.11 `.search-box`(+`.form-control` alias)/`.form-section`/`.form-row`, §4.12 `:focus-visible` + `prefers-reduced-motion`. Breakpoints **480/768/1024**.
- **Shell wrappers** — design **§5.11**: `{{define "head"}}` opens `<body><div class="app-container"><main class="main-content">`; `{{define "foot"}}` closes `</main></div></body></html>`.
- **Responsive nav** — design **§4.2** + **§5**: `{{define "cardrewards-nav"}}` → `.cr-nav` sticky glass (`backdrop-filter` progressive-enhancement; rgba `--nav-bg` fallback), `.cr-nav__list` of `.nav-pill` `<a>`; preserve `<nav aria-label="Card rewards">`. Active pill via title-match → `aria-current="page"` (zero Go change); ship without active state if title-matching is brittle (still AC-7-compliant).
- **No Go change** (Decision (d), §2.2): `cardrewards.go` untouched. **No** remote CSS/font (Decision (a), §2.1). One embedded `<style>`; no new inline style (the `.progress-fill` width is added by consumers later).
- **Go render test** — NEW `cardrewards_render_test.go`: construct `CardRewardsWebHandler` via its constructor (exercises the `template.Must` parse of `cardRewardsTemplates`+`cardRewardsInsightsTemplates`+dashboard set), render each of the ten pages + sub-pages + the two `card-select` partials with minimal view models, assert (a) no render error, (b) HTML contains design-system markers (`--bg-primary`, `.cr-nav`, `.main-content`), (c) HTML contains **no** `<script`, `onclick=`, `onsubmit=`, `onload=` (Go-level CSP-clean guard).

### Test Plan (4 rows → 4 DoD test items; parity 4 == 4)

| # | Test Type | Category | File | Description | Command | Live |
|---|-----------|----------|------|-------------|---------|------|
| 1 | Go render + CSP-clean | unit | `internal/web/cardrewards_render_test.go` (NEW) | Construct the cardrewards handler; render all 10 pages + sub-pages + 2 partials; assert no error + design-token/chrome markers present + zero `<script`/`onclick=`/`onsubmit=`/`onload=` in the cardrewards markup | `./smackerel.sh test unit --go --go-run 'CardRewards'` | No |
| 2 | Responsive nav | e2e-ui | `web/pwa/tests/cardrewards_chrome.spec.ts` (NEW) | Live stack: `<768px` → `.cr-nav__list` overflow-x scrollable (scrollWidth>clientWidth); `>=768px` → wraps; `.nav-pill` min-height ≥44px; `.cr-nav` sticky. Direct `expect(...).toBeVisible()`, no early-return bailout | `./smackerel.sh test e2e-ui` | Yes |
| 3 | Dark-mode token application | e2e-ui | `web/pwa/tests/cardrewards_chrome.spec.ts` (NEW) | Adversarial: `colorScheme:'dark'` → computed `background-color` of `body` (and/or `.card`) DIFFERS from the light render (a light-only-literal regression fails it) | `./smackerel.sh test e2e-ui` | Yes |
| 4 | Chrome regression + CSP-clean | e2e-ui | the 7 existing specs (`web/pwa/tests/cardrewards_dashboard.spec.ts` … the `web/pwa/tests/cardrewards_*.spec.ts` glob, UNCHANGED) + `web/pwa/tests/cardrewards_chrome.spec.ts` | The 7 existing specs pass UNCHANGED under the new chrome (every `data-*` locator resolves; `attachCSPGuard`/`assertNoCSPViolations` empty on every page) — proves the foundation preserved structure + CSP | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done — Tiered

**Core Items**
- [x] **Implementation:** design §3 token system (both themes, exact hex) + §4 component CSS in `{{define "head"}}`; `{{define "foot"}}` closes the shell; `{{define "cardrewards-nav"}}` rewritten to the §4.2 responsive glass nav; `cardrewards.go` + `templates.go` untouched; no new inline style beyond the (consumer-added) `.progress-fill` width class. → Evidence: [report.md](report.md#scope-01-impl)
- [x] **Behavior:** all ten `/cards` pages render under the new `.app-container`/`.main-content` shell + design system; their existing page bodies (restyled by the shared `.card`/`.meta`/`.tag`/`.empty`/`.search-box` classes) keep every `data-*` hook. → Evidence: [report.md](report.md#scope-01-behavior)
- [x] **Test row 1** — Go render + CSP-clean unit test green (≥10-line raw output). → Evidence: [report.md](report.md#scope-01-go-render)
- [x] **Test row 2** — responsive-nav e2e-ui assertion green. → Evidence: [report.md](report.md#scope-01-nav)
- [x] **Test row 3** — dark-mode token e2e-ui assertion green. → Evidence: [report.md](report.md#scope-01-dark)
- [x] **Test row 4** — existing 7 e2e-ui specs pass UNCHANGED + CSP guard empty. → Evidence: [report.md](report.md#scope-01-regression)

**Build Quality Gate (grouped — single evidence block)**
- [x] Zero-warning `./smackerel.sh check` + `./smackerel.sh lint` + `./smackerel.sh format --check`; `regression-quality-guard.sh web/pwa/tests/cardrewards_chrome.spec.ts` (no silent-pass bailout); `data-*` integrity (foundation diff touches no page body → zero hooks removed/renamed); docs/evidence aligned. → Evidence: [report.md](report.md#scope-01-bqg)

---

## SCOPE-02 — Scope-10 pages restyle (wallet, offers, selections, bonuses, categories)

**Status:** Done
**Depends On:** SCOPE-01
**Files:**
  - `internal/web/cardrewards_templates.go` — page bodies: `cardrewards-wallet.html` (+`-wallet-add` / `-wallet-add-custom` / `-wallet-edit`), `cardrewards-offers.html` (+`-offer-edit`), `cardrewards-selections.html` (+`-selection-edit`), `cardrewards-bonuses.html`, `cardrewards-categories.html`, and the `cardrewards-card-select` / `-required` partials.
  - NEW `web/pwa/tests/cardrewards_bonuses.spec.ts` (e2e-ui — closes the bonuses gap, Finding #1).
  - Targeted new assertions added to existing `cardrewards_wallet.spec.ts`, `cardrewards_offers_selections.spec.ts`, `cardrewards_categories.spec.ts`.
  - Extends `internal/web/cardrewards_render_test.go` (page-body markers).

### Use Cases (Gherkin — traced to spec)

```gherkin
Scenario: SCOPE-02-A Wallet polished card surfaces (UC-4 / AC-6)
  Given the operator opens /cards/wallet with owned cards
  When the wallet renders
  Then each card is a .card carrying data-card-id and data-active, with a
    .card-title (data-card-name), a card-type .badge (data-card-type), and a
    clear active vs inactive status badge (data-card-status active/inactive)
  And edit/activate-deactivate/remove remain working .btn <form>/<a> controls
    (data-action edit/toggle/delete)
  And the empty state keeps data-empty="wallet"

Scenario: SCOPE-02-B Bonuses visual progress bar + status (UC-2 / AC-4)
  Given the operator opens /cards/bonuses with a tracked bonus that has spend progress
  When the page renders
  Then the bonus is a .card (data-bonus-id, data-met) with a visual .progress bar
    whose .progress-fill width equals the server-computed spend percentage
    (role="progressbar" aria-valuenow set)
  And the kept text label retains data-bonus-progress
  And a met bonus shows a success .badge (data-bonus-met="true"); an unmet bonus
    shows an in-progress state
  And the empty state keeps data-empty="bonuses"

Scenario: SCOPE-02-C Offers / selections / categories elevated (UC-5 / AC-9)
  Given the operator opens /cards/offers, /cards/selections, /cards/categories
  When each renders
  Then offers are .card (data-offer-id/data-activated) with .badge-info shared
    limit and an activated/not-activated status badge (data-offer-status);
    selections are .card (data-selection-id/data-tier) with a tier .badge;
    categories render in a .cr-table (data-category/data-starred) with a starred
    ★ .badge-starred
  And every add/edit/toggle/delete/star form still submits to the same action

Scenario: SCOPE-02-D Scope-10 regression + CSP (UC-8 / UC-9)
  Given the restyled Scope-10 pages
  Then every Scope-10 data-* hook in design §7 resolves on its new element
  And no inline <script> or event handler exists; the only inline style is the
    server-computed .progress-fill width
```

### Implementation Plan (cite design.md)

- **Wallet** — design **§5.2**: `.card-grid` of `.card` (data-card-id/data-active), `.card-title` (data-card-name), `.badge` (data-card-type), status `.badge-success` `● Active` / `.badge-neutral` `○ Inactive` (data-card-status), `.summary` (data-card-note), `.btn`-row edit/toggle/delete (data-action). Sub-pages: add (candidate `.card`s data-candidate-id/-name, data-action="confirm-add", data-empty="candidates"), add-custom (data-action="create-custom"), edit (`.meta` data-card-catalog, data-action="save-card").
- **Bonuses** — design **§5.3** + **§4.8**: `.card` (data-bonus-id/data-met); the **progress bar** `<div class="progress"><div class="progress-fill" style="width:{{pct .SpendProgressCents .SpendRequiredCents}}%">` with `role="progressbar" aria-valuenow aria-valuemin="0" aria-valuemax="100"`, threshold modifier `--success` when `.Met` else `--warning` near-deadline-unmet else `--accent`; the **kept** text label `.meta` (data-bonus-progress) `progress $X of $Y (Z%)` + `.badge-success` (data-bonus-met="true") ✓ when `.Met`; add-bonus `.form-section` (uses `cardrewards-card-select-required`) at page bottom; data-empty="bonuses".
- **Offers** — design **§5.5**; **Selections** — **§5.6**; **Categories** — **§5.7** (`.table-wrap > table.cr-table`); **card-select partials** — **§5.11** (unchanged markup; `<select class="search-box">` inherits §4.11).
- **`data-*` map** — design **§7** rows for wallet / wallet-add / wallet-edit / wallet-add-custom / offers / offer-edit / selections / selection-edit / bonuses / categories carried **1:1** onto the restructured elements; drop/rename none.
- **CSP** — design **§6**: the `.progress-fill` width is the ONLY inline style (server-computed via existing `pct` helper — no JS).
- **Bonuses spec (Finding #1)** — NEW `cardrewards_bonuses.spec.ts` reusing `_support/cardrewards.ts` (`login`, `createCustomCardAPI`, `isoDate`) + `_support/csp.ts` (`attachCSPGuard`/`assertNoCSPViolations`); seed a card + create a bonus with spend progress via the live `/cards/bonuses` PRG form (no interception), then assert the data-bonus-* regression locators + the progress-bar elevated assertions.

### Test Plan (5 rows → 5 DoD test items; parity 5 == 5)

| # | Test Type | Category | File | Description | Command | Live |
|---|-----------|----------|------|-------------|---------|------|
| 1 | Go render (Scope-10 markers) | unit | `internal/web/cardrewards_render_test.go` (extend) | Render wallet/offers/selections/bonuses/categories + sub-pages; assert new markers (`.card-grid`, `.progress` on bonuses, `.cr-table` on categories, badges) + no `<script`/handler | `./smackerel.sh test unit --go --go-run 'CardRewards'` | No |
| 2 | Wallet regression + elevated | e2e-ui | `cardrewards_wallet.spec.ts` (existing UNCHANGED locators + new assertions) | Existing `data-card-*` locators resolve unchanged; NEW: `.card-grid` present, card-type + status badges present | `./smackerel.sh test e2e-ui` | Yes |
| 3 | Offers + selections regression + elevated | e2e-ui | `cardrewards_offers_selections.spec.ts` (existing UNCHANGED + new) | `data-offer-*` / `data-selection-*` resolve unchanged; NEW: `.badge-info` shared-limit, offer status badge, selection tier `.badge` present | `./smackerel.sh test e2e-ui` | Yes |
| 4 | Categories regression + elevated | e2e-ui | `cardrewards_categories.spec.ts` (existing UNCHANGED + new) | `data-category`/`data-starred` resolve unchanged; NEW: `.cr-table` + starred `.badge-starred` ★ present | `./smackerel.sh test e2e-ui` | Yes |
| 5 | Bonuses (NEW spec — gap closure) | e2e-ui | `web/pwa/tests/cardrewards_bonuses.spec.ts` (NEW) | Live stack: seed card + create a bonus with progress; assert `data-bonus-id`/`data-met`/`data-bonus-progress`/`data-bonus-met` resolve (NEW regression guard); `.progress`+`.progress-fill` numeric width matches the `(Z%)` label; `role="progressbar"`+`aria-valuenow`; met → `.badge-success` ✓; `attachCSPGuard`/`assertNoCSPViolations` empty | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done — Tiered

**Core Items**
- [x] **Implementation:** wallet/offers/selections/bonuses/categories + sub-pages + card-select partials restructured per design §5.2/§5.3/§5.5/§5.6/§5.7/§5.11; bonuses progress bar per §4.8; `cardrewards.go` untouched. → Evidence: [report.md](report.md#scope-02-impl)
- [x] **Behavior:** every Scope-10 `data-*` hook in design §7 lands on its new element (zero removed/renamed); every add/edit/toggle/delete/star form submits to its unchanged action. → Evidence: [report.md](report.md#scope-02-behavior)
- [x] **Test row 1** — Go render (Scope-10 markers) green. → Evidence: [report.md](report.md#scope-02-go-render)
- [x] **Test row 2** — wallet regression + elevated e2e-ui green. → Evidence: [report.md](report.md#scope-02-wallet)
- [x] **Test row 3** — offers + selections regression + elevated e2e-ui green. → Evidence: [report.md](report.md#scope-02-offers-selections)
- [x] **Test row 4** — categories regression + elevated e2e-ui green. → Evidence: [report.md](report.md#scope-02-categories)
- [x] **Test row 5** — bonuses NEW spec green (data-bonus-* regression + progress bar). → Evidence: [report.md](report.md#scope-02-bonuses)

> Test Plan rows = 5; test-related DoD items = 5 (rows 1–5). Parity ✓.

**Build Quality Gate (grouped — single evidence block)**
- [x] Zero-warning `./smackerel.sh check` + `lint` + `format --check`; `regression-quality-guard.sh` on `cardrewards_bonuses.spec.ts` + the touched wallet/offers/categories specs (no silent-pass bailout; the bonus width-matches-label assertion is adversarial); `data-*` integrity (zero Scope-10 hooks removed/renamed vs §7); docs/evidence aligned. → Evidence: [report.md](report.md#scope-02-bqg)

---

## SCOPE-03 — Scope-11 pages restyle (dashboard, recommendations, rotating, report, admin)

**Status:** Done
**Depends On:** SCOPE-01
**Files:**
  - `internal/web/cardrewards_dashboard_templates.go` — `cardrewards-dashboard.html` (§5.1), `cardrewards-recommendations.html` (§5.8), `cardrewards-rotating.html` (§5.4), `cardrewards-report.html` (§5.9), `cardrewards-admin.html` (§5.10).
  - Targeted new assertions added to existing `cardrewards_dashboard.spec.ts` (dashboard **and** report — report is covered there, Finding #3), `cardrewards_rotating_verify.spec.ts`, `cardrewards_recommendations.spec.ts`, `cardrewards_admin.spec.ts`.
  - Extends `internal/web/cardrewards_render_test.go` (Scope-11 page markers).

### Use Cases (Gherkin — traced to spec)

```gherkin
Scenario: SCOPE-03-A Dashboard action-oriented + stat-grid (UC-1 / AC-3)
  Given the operator opens /cards
  When the dashboard renders
  Then it presents a .stats-grid of 4 .stat-cards built from {{len .Recommendations}},
    {{len .ActiveRotating}}, {{len .NeedsVerification}}, {{len .PendingReEnroll}}
  And the Needs-Verify tile gets .stat-card--urgent + a ⚠ glyph when its count > 0
  And an .alert-warning "Pending Actions" priority section precedes the
    recommendations grid
  And data-dashboard, data-rec-row, data-active-rotating, data-needs-verification,
    data-pending-reenroll, data-empty are unchanged

Scenario: SCOPE-03-B Rotating confidence meter + verify urgency (UC-3 / AC-5)
  Given the operator opens /cards/rotating with reconciled records
  When a record renders
  Then its confidence is a visual .progress meter (width = confpct), its label
    span keeps data-confidence-badge
  And a needs-verification record shows .badge-warning (data-badge), a
    manual-override record shows .badge-success
  And data-rotating-row/-id, data-needs-verification, data-manual-override,
    data-confidence, data-citation/-source/-empty are unchanged

Scenario: SCOPE-03-C Recommendations / report / admin elevated (UC-5 / AC-9)
  Given the operator opens /cards/recommendations, /cards/report, /cards/admin
  When each renders
  Then recommendations are a .card-grid (data-rec-*) with a starred ★ .badge and
    star/unstar/regenerate forms; report renders in a .cr-table (data-report-*)
    with an emphasized best-card cell; admin shows trigger buttons + a run-history
    .cr-table (data-run-*) with a status .badge colored by data-run-status
  And every form still submits to its unchanged action

Scenario: SCOPE-03-D Scope-11 regression + CSP (UC-8 / UC-9)
  Given the restyled Scope-11 pages
  Then every Scope-11 data-* hook in design §7 resolves on its new element
  And no inline <script>/handler exists; the only inline style is the
    server-computed rotating confidence .progress-fill width
```

### Implementation Plan (cite design.md)

- **Dashboard** — design **§5.1** + **§4.5** + **§4.9**: `.page-header` (`<h1 data-dashboard class="page-title">` + `.page-subtitle` `{{.Period}}`); `.stats-grid` of 4 link `.stat-card`s from `{{len .Recommendations}}` / `{{len .ActiveRotating}}` / `{{len .NeedsVerification}}` / `{{len .PendingReEnroll}}` (Decision (d), §2.2 — template-only, no Go change); Needs-Verify tile `.stat-card--urgent` + ⚠ when `{{if .NeedsVerification}}`; `.alert-warning` "Pending Actions" priority section FIRST (the data-needs-verification + data-pending-reenroll cards), then a `.card-grid` of recommendation `.card`s, then active-rotating `.card`s with category `.chip`s; empties keep data-empty="pending"/"recommendations"/"active-rotating".
- **Rotating** — design **§5.4** + **§4.8**: `.card` (data-rotating-row/-id/-needs-verification/-manual-override/-confidence); confidence meter `.progress`/`.progress-fill` width `{{confpct .Confidence}}%` (threshold success/warning/danger), label span keeps data-confidence-badge; status `.badge-warning` (data-badge="needs-verification") ⚠ vs `.badge-success` (data-badge="manual-override") ✓; citations `<li data-citation data-citation-source>` or `.meta` data-citation-empty; verify `.form-row` + `.btn` data-action="verify"; data-empty="rotating".
- **Recommendations** — design **§5.8**; **Report** — **§5.9** (`.table-wrap > table.cr-table`, emphasized best-card cell); **Admin** — **§5.10** (triggers `.btn-secondary` forms; run-history `.cr-table` with status `.badge` by data-run-status, data-events-written-cell).
- **`data-*` map** — design **§7** rows for dashboard / recommendations / rotating / report / admin carried **1:1**.
- **CSP** — design **§6**: the rotating confidence-fill width is the only inline style on these pages.

### Test Plan (5 rows → 5 DoD test items; parity 5 == 5)

| # | Test Type | Category | File | Description | Command | Live |
|---|-----------|----------|------|-------------|---------|------|
| 1 | Go render (Scope-11 markers) | unit | `internal/web/cardrewards_render_test.go` (extend) | Render dashboard/recommendations/rotating/report/admin; assert new markers (`.stats-grid`, `.alert-warning`, `.progress` on rotating, `.cr-table` on report/admin) + no `<script`/handler | `./smackerel.sh test unit --go --go-run 'CardRewards'` | No |
| 2 | Dashboard + report regression + elevated | e2e-ui | `cardrewards_dashboard.spec.ts` (existing UNCHANGED + new) | `data-dashboard`/`data-rec-row`/`data-active-rotating`/`data-needs-verification`/`data-pending-reenroll` + report `data-report-*` resolve unchanged; NEW: `.stats-grid` 4 `.stat-card`s, `.stat-card--urgent` when NeedsVerification>0, `.alert-warning` precedes the recommendations grid | `./smackerel.sh test e2e-ui` | Yes |
| 3 | Rotating regression + elevated | e2e-ui | `cardrewards_rotating_verify.spec.ts` (existing UNCHANGED + new) | `data-rotating-*`/`data-confidence`/`data-badge`/`data-citation*` resolve unchanged; NEW: confidence `.progress` meter width matches `confpct`; needs-verification → `.badge-warning`, manual-override → `.badge-success` | `./smackerel.sh test e2e-ui` | Yes |
| 4 | Recommendations regression + elevated | e2e-ui | `cardrewards_recommendations.spec.ts` (existing UNCHANGED + new) | `data-rec-*`/`data-rec-starred` resolve unchanged; NEW: `.card-grid` + `.badge-starred` ★ + star/unstar/regenerate buttons present | `./smackerel.sh test e2e-ui` | Yes |
| 5 | Admin regression + elevated | e2e-ui | `cardrewards_admin.spec.ts` (existing UNCHANGED + new) | `data-run-*`/`data-triggers`/`data-events-written-cell` resolve unchanged; NEW: run-history `.cr-table` + status `.badge` colored by `data-run-status` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done — Tiered

**Core Items**
- [x] **Implementation:** dashboard/recommendations/rotating/report/admin restructured per design §5.1/§5.4/§5.8/§5.9/§5.10; stat-grid from `{{len}}` (§2.2, no Go change); rotating confidence meter per §4.8; `cardrewards.go` untouched. → Evidence: [report.md](report.md#scope-03-impl)
- [x] **Behavior:** every Scope-11 `data-*` hook in design §7 lands on its new element (zero removed/renamed); every star/regenerate/verify/scrape/sync-calendar form submits to its unchanged action. → Evidence: [report.md](report.md#scope-03-behavior)
- [x] **Test row 1** — Go render (Scope-11 markers) green. → Evidence: [report.md](report.md#scope-03-go-render)
- [x] **Test row 2** — dashboard + report regression + elevated e2e-ui green. → Evidence: [report.md](report.md#scope-03-dashboard-report)
- [x] **Test row 3** — rotating regression + elevated e2e-ui green. → Evidence: [report.md](report.md#scope-03-rotating)
- [x] **Test row 4** — recommendations regression + elevated e2e-ui green. → Evidence: [report.md](report.md#scope-03-recommendations)
- [x] **Test row 5** — admin regression + elevated e2e-ui green. → Evidence: [report.md](report.md#scope-03-admin)

> Test Plan rows = 5; test-related DoD items = 5 (rows 1–5). Parity ✓.

**Build Quality Gate (grouped — single evidence block)**
- [x] Zero-warning `./smackerel.sh check` + `lint` + `format --check`; `regression-quality-guard.sh` on the touched dashboard/rotating/recommendations/admin specs (no silent-pass bailout; the dashboard stat-grid/urgent assertions fail if the element is missing); `data-*` integrity (zero Scope-11 hooks removed/renamed vs §7); docs/evidence aligned. → Evidence: [report.md](report.md#scope-03-bqg)

---

## SCOPE-04 — Consolidated verification + live deploy proof

**Status:** Done
**Depends On:** SCOPE-01, SCOPE-02, SCOPE-03
**Files:** none (verification only — **no production edit**). Runs the full card-rewards suite across all ten pages, the `data-*` preservation diff proof, and the live rebuild/render proof.

### Use Cases (Gherkin — traced to spec)

```gherkin
Scenario: SCOPE-04-A Full card-rewards suite green (UC-1..9 integrated)
  Given the re-skin of all ten /cards pages is complete
  When the full Go unit suite and the full card-rewards e2e-ui suite run against
    the live smackerel-test-e2e-ui stack
  Then the cardrewards render test + all 7 existing specs + cardrewards_bonuses.spec.ts
    + cardrewards_chrome.spec.ts pass
  And every attachCSPGuard/assertNoCSPViolations buffer is empty on every page

Scenario: SCOPE-04-B data-* preservation proof (AC-9 / §7)
  Given the pre-redesign and post-redesign template files
  When the data-* attribute set of both template files is diffed (base vs HEAD)
  Then the post-redesign set is a superset of the pre-redesign set
  And zero data-* hooks were removed or renamed

Scenario: SCOPE-04-C Live rebuild + render proof (light + dark) (UC-7 / AC-10)
  Given a freshly built smackerel core image
  When the e2e-ui suite runs against the freshly-built stack
  Then every /cards page renders the new design system in both light and dark
  And Docker bundle freshness is verified (the served markup contains the new
    design-system markers, not a stale bundle)
  And the home-lab apply is operator-gated and ships in the same rebuild as spec 091
```

### Implementation Plan

- **No production code change.** This scope is the consolidated green-bar gate over SCOPE-01..03.
- **Full Go unit** — `./smackerel.sh test unit --go` (whole `internal/web` package incl. the cardrewards render test) green.
- **Full card-rewards e2e-ui** — `./smackerel.sh test e2e-ui` (all 7 existing specs + `cardrewards_bonuses.spec.ts` + `cardrewards_chrome.spec.ts`) green; every CSP guard empty.
- **`data-*` diff proof** (design §7, AC-9) — extract the `data-[a-z-]+` set from `cardrewards_templates.go` + `cardrewards_dashboard_templates.go` at the pre-redesign base vs HEAD (`git show <base>:<file>` vs working tree) and prove the post set ⊇ pre set with zero removals/renames. Capture the diff as evidence.
- **Live rebuild + render proof** — `./smackerel.sh build` then `./smackerel.sh test e2e-ui` against the fresh stack is the deploy-grade proof that the **built artifact** renders the new design (light + dark via Playwright `colorScheme`); verify Docker bundle freshness (served markup contains the new markers). The home-lab `deploy-target apply` itself is **operator-gated** and ships in the **same rebuild as spec 091** (combined deploy) — the agent drives the rebuild + the e2e-ui proof; the operator authorizes the apply.

### Test Plan (4 rows → 4 DoD test items; parity 4 == 4)

| # | Test Type | Category | File / Target | Description | Command | Live |
|---|-----------|----------|---------------|-------------|---------|------|
| 1 | Full Go unit | unit | `internal/web` package | Whole web package incl. cardrewards render test green (no `--go-run` filter) | `./smackerel.sh test unit --go` | No |
| 2 | Full card-rewards e2e-ui | e2e-ui | all 9 cardrewards specs | 7 existing + `cardrewards_bonuses.spec.ts` + `cardrewards_chrome.spec.ts` pass; every CSP guard empty across all ten pages | `./smackerel.sh test e2e-ui` | Yes |
| 3 | `data-*` preservation diff | verification (regression contract) | both template files | Diff `data-*` set pre vs post (base vs HEAD); prove superset, zero removals/renames (AC-9 / §7) | `git show <base>:internal/web/cardrewards_templates.go` + working-tree grep diff | No |
| 4 | Live rebuild + render proof | e2e-ui | freshly-built `smackerel-test-e2e-ui` stack | Build core, run e2e-ui against the fresh stack; /cards renders the new design light+dark; Docker bundle freshness verified (markers present, not stale) | `./smackerel.sh build` + `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done — Tiered

**Core Items**
- [x] **Behavior:** all ten `/cards` pages render the elevated design (light + dark) with every `data-*` hook intact and zero CSP violations — the full suite is the integrated proof of AC-1..AC-11. → Evidence: [report.md](report.md#scope-04-behavior)
- [x] **Test row 1** — full Go unit green. → Evidence: [report.md](report.md#scope-04-go-unit)
- [x] **Test row 2** — full card-rewards e2e-ui suite green (9 specs, CSP guards empty). → Evidence: [report.md](report.md#scope-04-e2e-ui)
- [x] **Test row 3** — `data-*` preservation diff proof (zero removals/renames). → Evidence: [report.md](report.md#scope-04-data-diff)
- [x] **Test row 4** — live rebuild + render proof (light+dark; bundle freshness). → Evidence: [report.md](report.md#scope-04-deploy)

> Test Plan rows = 4; test-related DoD items = 4 (rows 1–4). Parity ✓.

**Build Quality Gate (grouped — single evidence block)**
- [x] Zero-warning `./smackerel.sh check` + `lint` + `format --check`; `regression-quality-guard.sh` across all touched/new spec files (no silent-pass bailout; bonuses + dark-mode + width-match assertions are adversarial); `artifact-lint.sh specs/092-card-rewards-ui-elevation` exit 0; `state-transition-guard.sh specs/092-card-rewards-ui-elevation` exit 0 readiness; docs/evidence aligned. → Evidence: [report.md](report.md#scope-04-bqg)

---

## Consumer Impact Sweep

This feature is a **CSS-only re-skin** (presentation-only). It restructures **HTML element shape** inside two Go template files; it does **not** rename or remove any route, handler, form action, URL, or `data-*` test hook. Per design §7 the `data-*` contract is carried **1:1** onto every restructured element, and SCOPE-04-B proves the post-redesign `data-*` set is **byte-identical** to the pre-redesign set (66 tokens, 0 dropped/renamed).

**Consumers of the changed surface (enumerated):**

- **The ten `/cards` page renders** (wallet, offers, selections, bonuses, categories, dashboard, recommendations, rotating, report, admin) — they consume the shared `{{define "head"}}` / `{{define "cardrewards-nav"}}` chrome and the component classes. Re-skinned in place; every route/link/form action is unchanged.
- **Navigation** — the responsive `cardrewards-nav` pill row links to the same ten route paths; no route path was renamed, so no nav link, breadcrumb, or in-app deep link goes stale.
- **Redirects** — no route was moved or removed, so there is no redirect to add and no stale redirect target.
- **API clients / generated clients** — none. These pages are server-rendered HTML behind `webAuthMiddleware`; there is no JSON API client, generated client, or external consumer of the `/cards` HTML shape.
- **Playwright regression specs** (`web/pwa/tests/cardrewards_*.spec.ts`) + the Go render test — they address elements by `data-*` locator; because every `data-*` hook is carried through, no locator needs a change.

**Stale-reference scan result:** zero. The `data-*` set is byte-identical (SCOPE-04-B), the route table is unchanged, and the only consumers (the ten page renders + the regression specs) resolve every locator unchanged.

**Consumer impact (feature-wide):**
- [x] Consumer impact sweep completed — every consumer is a `/cards` page render or a Playwright/Go regression test; the `data-*` set is byte-identical (66 tokens, 0 dropped) and no route/link/form action was renamed or removed, so zero stale first-party references remain. → Evidence: [report.md](report.md#scope-04-data-diff)

---

## Persistent Regression E2E Coverage (feature-wide)

The regression contract for this re-skin is the **seven existing card-rewards Playwright specs passing UNCHANGED** under the new design system — the glob `web/pwa/tests/cardrewards_*.spec.ts` (dashboard, wallet, offers_selections, recommendations, rotating_verify, categories, admin) plus the new `cardrewards_bonuses.spec.ts` and `cardrewards_chrome.spec.ts`. Every spec addresses its page by `data-*` locator and asserts no CSP violation (`attachCSPGuard`/`assertNoCSPViolations`); because the `data-*` set is byte-identical (SCOPE-04-B), a dropped/renamed hook or any inline-script/handler regression fails an existing spec. This is persistent scenario-specific E2E regression coverage for every changed page.

| # | Test Type | Category | File | Description | Command | Live |
|---|-----------|----------|------|-------------|---------|------|
| R1 | Regression E2E | e2e-ui | `web/pwa/tests/cardrewards_dashboard.spec.ts` | Regression: the 7 existing `web/pwa/tests/cardrewards_*.spec.ts` specs pass UNCHANGED under the re-skin (every `data-*` locator resolves; every CSP guard empty), proving the redesign preserved structure + behavior + CSP-cleanliness | `./smackerel.sh test e2e-ui` | Yes |

**Regression coverage (feature-wide — evidence is the consolidated SCOPE-04 run; R1's single e2e-ui run satisfies both items):**
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass — the 7 existing `web/pwa/tests/cardrewards_*.spec.ts` specs run UNCHANGED, plus the new `cardrewards_bonuses.spec.ts` and `cardrewards_chrome.spec.ts` cover the new affordances. → Evidence: [report.md](report.md#scope-04-e2e-ui)
- [x] Broader E2E regression suite passes — the full card-rewards `e2e-ui` suite (19/19) is green across all ten `/cards` pages with every CSP guard empty. → Evidence: [report.md](report.md#scope-04-e2e-ui)

---

## Superseded Scopes (Do Not Execute)

None — this is the initial plan for spec 092.
