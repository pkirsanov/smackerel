# Design 092 — Card-Rewards Web UI Elevation (match-or-exceed CCManager)

**Spec:** [spec.md](spec.md) · **Status:** in_progress · **Workflow mode:** full-delivery · **Status ceiling:** done
**Owns:** technical design for a CSS-only re-skin of the ten server-rendered `/cards` pages.
**Does NOT own:** scopes/DoD (bubbles.plan), implementation (bubbles.implement), tests (bubbles.test).
**Scope boundary (binding):** only [internal/web/cardrewards_templates.go](../../internal/web/cardrewards_templates.go) and [internal/web/cardrewards_dashboard_templates.go](../../internal/web/cardrewards_dashboard_templates.go). `internal/web/cardrewards.go` is **not** edited (Decision (d) below resolves to no view-model change). `internal/web/templates.go` is untouched.

---

## Design Brief (review this first — ~40 lines)

**Current State.** The `/cards` surface is server-rendered Go `html/template` text living in two `const` string blobs: [cardrewards_templates.go](../../internal/web/cardrewards_templates.go) (the shared `{{define "head"}}` / `{{define "foot"}}` / `{{define "cardrewards-nav"}}` chrome + an 11-variable minimal monochrome `:root`, plus the Scope-10 pages and the two `cardrewards-card-select` partials) and [cardrewards_dashboard_templates.go](../../internal/web/cardrewards_dashboard_templates.go) (the Scope-11 dashboard / recommendations / rotating / report / admin pages). Today it is a single-column **820 px** body, a plain wrapping text-link nav, flat `.card`/`.tag`/`.empty` classes, and `progress X of Y (Z%)` rendered as **plain text**. Every interactive element is a `<form>` PRG submit or an `<a href>` — there is **no JavaScript** anywhere in the surface, by design, because the pages render under a strict CSP.

**Target State.** Re-skin all ten pages to match-or-exceed the legacy CCManager Apple-grade design system — a full light+dark design-token palette (via `prefers-color-scheme`), a responsive sticky glass nav, a stat-grid, badges, a button hierarchy, real **visual progress bars**, and card/table layouts — **without adding a single line of JavaScript** and **without moving a single `data-*` test hook**. The visual restructure changes *look*, not *test-addressable structure* or *behavior*.

**Patterns to Follow.**
- The existing `{{define "head"}}`/`{{define "foot"}}`/`{{define "cardrewards-nav"}}` indirection — the entire design system is one embedded `<style>` in `head`, reused by all ten pages. Put **all** new token + component CSS there (cardrewards_templates.go). (`internal/web/cardrewards_templates.go` lines 13–55.)
- The token-only color discipline already in the file header comment ("design-token palette only (`var(--…)`) — no hardcoded colors").
- PRG `<form>` submits + `<a href>` for every mutation/navigation (no JS), exactly as today.
- The existing Go `template.FuncMap` helpers `pct` (0–100 clamp), `confpct` (0–1→0–100), `cents`, `centsPtr`, `csv`, `deref`, `intPtr`, `date` ([cardrewards.go](../../internal/web/cardrewards.go) lines 82–135) — the progress-bar width and confidence-meter width are computed by these **already-present** helpers; no new helper is required.

**Patterns to Avoid.**
- **CCManager's inline-JS affordances** (`CCManager/web/templates/base.html`): inline `<script>` auto-dismiss flash, double-submit guard, CSRF/cookie helpers, and `onclick`/`onsubmit` handlers. These are exactly what smackerel's CSP forbids — drop them or achieve a CSS-only / server-side equivalent.
- **CCManager's fixed bottom tab-bar** + `env(safe-area-inset)` padding — rejected by UX Decision (b) (10 destinations exceed a ~5-tab bottom-bar budget and a bottom bar occludes the page-bottom add-forms). Use the responsive top-nav only.
- **CCManager's Inter Google-Fonts `<link>`** — rejected by UX/design Decision (a) (CSP has no `font-src`; the link + font fetch both fail the spec-077 CSP guard).
- Hard-coded hex in markup; per-page bespoke palettes; any route/handler/data-model change to make the UI nicer.

**Resolved Decisions (this doc, binding):** **(a)** system-font stack, zero CSP change (ratified §2.1). **(d)** **no** view-model change — the dashboard stat-grid is built purely from `{{len .X}}` of the four collections `DashboardPage` already passes (ratified §2.2). Palette = a **smackerel-tuned variant of CCManager's Apple token system** (warm-neutral surfaces + Apple accent/semantic/elevation; full hex in §3).

**Open Questions:** None blocking. UX already settled (a-font), (b-nav), (c-theme); this doc settles the two design-owned questions. Pixel-level breakpoint/spacing tuning is delegated to implement within the contract in §4.

---

## 1. Purpose & Scope

Elevate the *presentation* of the ten `/cards` pages to match-or-exceed CCManager while keeping the pages strictly CSP-clean (no new JS), preserving every `data-*` test hook, and changing nothing about routes, handlers, or the data the pages serve. The change is **100 % presentation**: two template files, plus the shared `head`/`nav` chrome they already reference.

**In scope:** the token system + component CSS in `{{define "head"}}`, the nav markup in `{{define "cardrewards-nav"}}`, and the per-page markup restructure of all ten pages + their sub-pages (wallet add / add-custom / edit; offer-edit; selection-edit) and the two `card-select` partials — all inside the two card-rewards template files.

**Out of scope:** `internal/web/templates.go` (the separate knowledge-base design system), any non-card-rewards page, routes/handlers/data-model, a JS framework or any JS, per-user theming beyond `prefers-color-scheme`, new runtime dependencies, and any CSP relaxation.

---

## 2. Settled Open Decisions (BINDING)

### 2.1 Decision (a) — FONT: system-font stack (ratified; zero CSP change)

**Decision: the system-font stack. No web font, no `<link>`, no `@font-face`, no `font-src` CSP amendment.** UX already chose this; design **ratifies** it and rejects the two heavier sub-options (a-ii self-host/inline Inter via same-origin `@font-face`; a-iii amend `router.go` to allow `fonts.googleapis.com` + `fonts.gstatic.com`).

**Exact stack (replaces the current `body { font-family … }`):**
```
font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, system-ui, sans-serif;
```
(This adds `Roboto` and `system-ui` to today's stack so Android/Linux render the CCManager-equivalent face.)

**WHY (the CSP argument, grounded in the real header).** The governing CSP ([router.go](../../internal/api/router.go) ≈L658) for the web surface is `default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' https://unpkg.com/htmx.org@1.9.12/ 'sha256-…'; img-src 'self' data:; connect-src 'self'` — with **no `font-src` directive**, so font loads fall back to `default-src 'self'`. A CCManager-style `<link href="https://fonts.googleapis.com/…">`:
1. is a cross-origin stylesheet link → **blocked at `style-src 'self' 'unsafe-inline'`** (the remote origin is not allow-listed; `'unsafe-inline'` covers inline `<style>`/`style=""`, **not** a remote `<link>`), and
2. its `@font-face` would fetch `fonts.gstatic.com` → **blocked at the defaulted `font-src` (`'self'`)**.

Either violation makes the browser fire `securitypolicyviolation`, which the spec-077 guard (`web/pwa/tests/_support/csp.ts`, `attachCSPGuard`/`assertNoCSPViolations`) captures → **the e2e suite fails**. The system stack renders the same Apple/Segoe/Roboto faces the CCManager Inter look approximates, is native + zero-network + zero-CSP-risk, and — paired with CCManager's **type scale** (below) — reads as polished as Inter. Self-hosting Inter (a-ii) or amending the CSP (a-iii) buys the *exact* Inter glyphs at the cost of a new same-origin asset or a reviewed security-surface change, for a benefit users will not perceive over the native faces. Not worth it.

**Type scale (the "polish" lever — applied to the system stack, ported from CCManager `base.html`):**

| Token | Size (mobile → desktop) | Weight | Letter-spacing | Line-height | Used by |
|-------|-------------------------|--------|----------------|-------------|---------|
| Page title | `2rem` → `2.5rem` (≥768px) | 700 | `-0.02em` | 1.15 | `.page-title` |
| Page subtitle | `1rem` | 400 | — | 1.4 | `.page-subtitle` (period) |
| Card / section title | `1.125rem` | 600 | — | 1.3 | `.card-title`, `<h2>` heads |
| Stat value | `2rem` → `2.5rem` (≥768px) | 700 | `-0.02em` | 1.1 | `.stat-value` |
| Stat label | `0.75rem` → `0.8125rem` | 500 | `0.05em`, UPPERCASE | 1.2 | `.stat-label` |
| Body | `1rem` (16px base) | 400 | — | 1.5 | body copy, `.meta` |
| Badge | `0.75rem` | 600 | — | 1 | all badges |

### 2.2 Decision (d) — STAT-GRID VIEW MODEL: no Go change (ratified)

**Decision: NO additive view-model field. `internal/web/cardrewards.go` is NOT edited.** The dashboard stat-grid is built **purely in-template** from the four collections `DashboardPage` already passes.

**Grounding.** `DashboardPage` ([cardrewards.go](../../internal/web/cardrewards.go) lines 770–777) renders exactly: `Title`, `Period`, `Recommendations` (`[]recommendationRow`), `ActiveRotating` (`[]rotatingRow`), `NeedsVerification` (`[]rotatingRow`), `PendingReEnroll` (`[]…`). The four stat tiles in the wireframe map 1:1 to these collections via the template `len` builtin:

| Tile | Count expression (template-only) | Links to |
|------|----------------------------------|----------|
| Recommendations | `{{len .Recommendations}}` | `/cards/recommendations` |
| Active Rotating | `{{len .ActiveRotating}}` | `/cards/rotating` |
| Needs Verify | `{{len .NeedsVerification}}` (urgent if `> 0`) | `/cards/rotating` |
| Pending Re-enroll | `{{len .PendingReEnroll}}` | `/cards/selections` |

No new query, no new field, no handler/route/data-model touch — so AC-11's scope boundary stays clean and the no-Go-change keeps the redesign a pure template diff.

**Rejected alternative:** adding owned-card-count / active-offer-count / open-bonus-count tiles. Those data are **not** on `DashboardPage` today; surfacing them would require new `Service` reads + new view-model fields in `cardrewards.go` (a handler change), expanding the diff and the failure surface for a *cosmetic* fourth-quadrant tile. The four existing-collection tiles already deliver the AC-3 "at-a-glance action overview." If a future spec wants a wallet/offer count, it adds them additively then — out of scope here.

> Net effect of (a)+(d): the entire feature is a **two-file template diff** with **zero Go change** and **zero CSP change**.

---

## 3. The CSS Design-Token System (exact hex, both themes)

This **replaces** the current minimal `:root{ --bg; --fg; --muted; --border; --accent; --card-bg; --success; --warning; --error; --radius; --shadow }` block in `{{define "head"}}` ([cardrewards_templates.go](../../internal/web/cardrewards_templates.go) lines 18–19). It is a **smackerel-tuned variant of CCManager's Apple token system**: CCManager's structural token *names*, accent/semantic *Apple system colors*, and three-step elevation/radius scale — but the **neutral surface ramp is warmed** to preserve smackerel's heritage warm-paper identity (today's `#fafaf8` / `#1a1a18`), and the semantic colors are tuned to **WCAG-AA as badge text on their own tint in both themes** (an a11y improvement over CCManager, per the NFR "maintain-or-improve" + AC-1).

**Token additions beyond the user-enumerated list, and why:** `--on-accent` / `--on-danger` (text color that sits on an accent/danger *fill* — needed because the accent flips luminance between themes, so white-on-accent is AA in light but near-black-on-accent is AA in dark); and the badge **tint pairs** `--*-soft` (the tinted badge background that pairs with the vivid `--success/-warning/-danger/-info` as AA text). These keep "colors come only from tokens" true while guaranteeing AA without `color-mix()` browser-support risk.

### 3.1 `:root` — LIGHT (default)

```css
:root {
  /* surfaces (warm-neutral, smackerel heritage) */
  --bg-primary:#ffffff; --bg-secondary:#f7f6f3; --bg-tertiary:#ecebe5; --bg-elevated:#ffffff;
  /* text */
  --text-primary:#1a1a18; --text-secondary:#5c5b57; --text-tertiary:#84837e;
  /* accent + on-accent (text on an accent fill) */
  --accent:#0066cc; --accent-hover:#0052a3; --on-accent:#ffffff; --on-danger:#ffffff;
  /* semantic (vivid — used as fills, glyphs, AA badge text on the matching -soft tint) */
  --success:#1e7d34; --warning:#8a5a00; --danger:#c4291c; --info:#4b48c4; --starred:#8a6d00;
  /* semantic soft tints (badge / alert backgrounds) */
  --success-soft:#e6f4ea; --warning-soft:#f8efd7; --danger-soft:#fbe7e4; --info-soft:#ececfb;
  --neutral-soft:#ecebe5; --starred-soft:#fbf1d4;
  /* borders + elevation */
  --border:#e2e0da; --border-strong:#ccc9c1;
  --shadow-sm:0 1px 2px rgba(0,0,0,0.05);
  --shadow-md:0 4px 12px rgba(0,0,0,0.09);
  --shadow-lg:0 8px 24px rgba(0,0,0,0.13);
  /* radii (theme-independent — declared once, NOT re-declared in dark) */
  --radius-sm:8px; --radius-md:12px; --radius-lg:16px; --radius-xl:24px;
  /* nav (glass) */
  --nav-bg:rgba(255,255,255,0.80); --nav-border:#e2e0da;
}
```

### 3.2 `@media (prefers-color-scheme: dark)` — DARK (re-declares colors + shadows + nav only; radii unchanged)

```css
@media (prefers-color-scheme: dark) {
  :root {
    --bg-primary:#161614; --bg-secondary:#1f1f1c; --bg-tertiary:#2b2b27; --bg-elevated:#242421;
    --text-primary:#e8e8e4; --text-secondary:#a8a8a2; --text-tertiary:#7a7a74;
    --accent:#4d9fff; --accent-hover:#6fb1ff; --on-accent:#0a1626; --on-danger:#1a0a08;
    --success:#5fd97e; --warning:#f0b429; --danger:#ff6b5e; --info:#9b99f5; --starred:#e8c75a;
    --success-soft:#18331f; --warning-soft:#36290f; --danger-soft:#3a1c19; --info-soft:#23224f;
    --neutral-soft:#2b2b27; --starred-soft:#33290f;
    --border:#34332f; --border-strong:#45443f;
    --shadow-sm:0 1px 2px rgba(0,0,0,0.30);
    --shadow-md:0 4px 12px rgba(0,0,0,0.40);
    --shadow-lg:0 8px 24px rgba(0,0,0,0.50);
    --nav-bg:rgba(22,22,20,0.80); --nav-border:#34332f;
  }
}
```

### 3.3 Token → role map (so implement uses the right token, never a literal)

| Role | Token(s) |
|------|----------|
| Page background | `--bg-primary` |
| Inset / muted surface, table zebra, progress track, chip bg | `--bg-secondary` / `--bg-tertiary` |
| Card / stat-card / elevated surface | `--bg-elevated` + `--shadow-sm` |
| Primary / secondary / tertiary text | `--text-primary` / `--text-secondary` / `--text-tertiary` |
| Link, primary button fill, nav-active fill, progress "in-progress" fill, focus ring | `--accent` (+ `--accent-hover`, `--on-accent`) |
| Badge `success/warning/danger/info/neutral/starred` | text `--success/…/--starred` on bg `--success-soft/…/--starred-soft` |
| Danger button fill | `--danger` bg + `--on-danger` text |
| Progress fill by threshold | `--success` (met) / `--warning` (near-deadline unmet) / `--danger` (low confidence) / `--accent` (neutral in-progress) |
| Border / divider, strong border (hover lift) | `--border` / `--border-strong` |
| Radius sm/md/lg/xl | `--radius-sm` … `--radius-xl` |
| Sticky glass nav | `--nav-bg` (with `backdrop-filter`) + `--nav-border` |

> **Contrast intent (verified by the test phase, AC-1 + a11y NFR):** every badge `*-soft`↔`*` pair, every text-on-surface pair, and `--on-accent`/`--on-danger` on their fills target **WCAG-AA (≥4.5:1 body, ≥3:1 large)** in **both** themes. The dark palette is a full parallel set, not a tint of light.

---

## 4. Component CSS (the single embedded `<style>` contract — CSP-clean)

All component CSS lives in the **one** `{{define "head"}}` `<style>` block (no second stylesheet, no remote CSS). Below is the rule-by-rule contract: selector → purpose → key declarations → responsive/state behavior. Implement writes the final CSS to this contract; exact pixel spacing is implement's tuning latitude **within** these rules. **Breakpoints: 480 / 768 / 1024 px.**

### 4.1 Reset, base, app shell
- `*,*::before,*::after { box-sizing:border-box; margin:0; padding:0 }` (already present, keep).
- `html { font-size:16px }`; `body { font-family:<§2.1 stack>; background:var(--bg-primary); color:var(--text-primary); line-height:1.5; -webkit-font-smoothing:antialiased }`. **Remove** today's `body { max-width:820px; margin:0 auto; padding:1rem }`.
- `.app-container { min-height:100vh; min-height:100dvh; display:flex; flex-direction:column }`.
- `.main-content { width:100%; max-width:1200px; margin:0 auto; padding:16px }` → at `≥768px` `padding:24px 32px`. **No** bottom-bar safe-area padding (no bottom bar). The 820 px→1200 px max-width bump is the desktop "use the viewport" win (AC-7).

### 4.2 Responsive nav (`{{define "cardrewards-nav"}}` rewrite — Decision (b))
- `.cr-nav { position:sticky; top:0; z-index:10; background:var(--nav-bg); backdrop-filter:blur(18px) saturate(1.4); -webkit-backdrop-filter:blur(18px) saturate(1.4); border-bottom:1px solid var(--nav-border) }`. **Progressive enhancement:** where `backdrop-filter` is unsupported the `--nav-bg` rgba still gives a legible translucent bar (NFR compat).
- `.cr-nav__brand { font-weight:700; … }` ("💳 Card Rewards" — emoji is decorative).
- `.cr-nav__list { display:flex; gap:8px; padding:10px 16px; list-style:none }`.
  - **Desktop (≥768px):** `flex-wrap:wrap` → the full-width wrapping pill row.
  - **Mobile (<768px):** `flex-wrap:nowrap; overflow-x:auto; scroll-snap-type:x proximity; -webkit-overflow-scrolling:touch` → single-row horizontal-scroll strip; right-edge **scroll-shadow fade** via a `.cr-nav::after` gradient (CSS only, no JS).
- `.nav-pill { display:inline-flex; align-items:center; min-height:44px; padding:8px 14px; border-radius:var(--radius-md); color:var(--text-secondary); text-decoration:none; white-space:nowrap; scroll-snap-align:start }`.
  - hover → `background:var(--bg-tertiary); color:var(--text-primary)`.
  - `:focus-visible` → `outline:2px solid var(--accent); outline-offset:2px`.
  - `.nav-pill.active` → `background:var(--accent); color:var(--on-accent)`. (Active = current route; see §5 "active-state mechanism".)
- `<nav aria-label="Card rewards">` landmark preserved; pills are `<a href>` (no JS). Tap targets ≥44px (a11y).

### 4.3 Page header
- `.page-header { margin:8px 0 20px }`; `.page-title { font:700 2rem/1.15; letter-spacing:-0.02em }` → `≥768px` `2.5rem`; `.page-subtitle { color:var(--text-secondary); font-size:1rem }`. The dashboard `<h1>` keeps `data-dashboard`.

### 4.4 Card / surface
- `.card { background:var(--bg-elevated); border:1px solid var(--border); border-radius:var(--radius-lg); padding:16px 18px; box-shadow:var(--shadow-sm); margin:12px 0 }`.
- `.card-header { display:flex; align-items:flex-start; justify-content:space-between; gap:12px }`; `.card-title { font:600 1.125rem/1.3 }`.
- `.card-grid { display:grid; gap:14px; grid-template-columns:1fr }` → `≥768px` `repeat(2,1fr)` (wallet/offers/recommendation card grids; AC-6/UC-4).
- `.meta { font-size:0.875rem; color:var(--text-secondary) }` (keep the class name — used widely).

### 4.5 Stat-grid + stat-card (net-new; AC-2/AC-3)
- `.stats-grid { display:grid; gap:12px; grid-template-columns:repeat(2,1fr) }` → `≥768px` `repeat(4,1fr)`.
- `.stat-card { display:flex; flex-direction:column; gap:4px; padding:16px; background:var(--bg-elevated); border:1px solid var(--border); border-radius:var(--radius-lg); box-shadow:var(--shadow-sm); text-decoration:none; color:inherit }`.
  - `.stat-card--link:hover { border-color:var(--border-strong); box-shadow:var(--shadow-md); transform:translateY(-2px) }` (hover-lift, progressive enhancement only).
  - `.stat-card--link:focus-visible { outline:2px solid var(--accent); outline-offset:2px }`.
  - `.stat-card--urgent { border-color:var(--warning); background:var(--warning-soft) }` (applied when needs-verification count `> 0`; pairs with a ⚠ glyph).
- `.stat-value { font:700 2rem/1.1; letter-spacing:-0.02em }` → `≥768px` `2.5rem`; `.stat-label { font:500 0.75rem/1.2; letter-spacing:0.05em; text-transform:uppercase; color:var(--text-secondary) }` → `≥768px` `0.8125rem`.

### 4.6 Badge system (AC-2; text+glyph+color, never color-only — a11y)
- `.badge { display:inline-flex; align-items:center; gap:4px; font:600 0.75rem/1; padding:3px 9px; border-radius:999px; background:var(--neutral-soft); color:var(--text-secondary) }`.
- `.badge-success { background:var(--success-soft); color:var(--success) }`; `.badge-warning { … --warning-soft / --warning }`; `.badge-danger { … --danger-soft / --danger }`; `.badge-info { … --info-soft / --info }`; `.badge-neutral` = base; `.badge-starred { background:var(--starred-soft); color:var(--starred) }`.
- Glyphs inside badges (✓ / ⚠ / ★ / ● / ○) are decorative → `aria-hidden="true"`; the **text** carries meaning.

### 4.7 Button hierarchy (AC-2)
- `.btn { display:inline-flex; align-items:center; gap:6px; font:600 0.9375rem/1; padding:9px 16px; border-radius:var(--radius-md); border:1px solid transparent; cursor:pointer; text-decoration:none; transition:background .12s, box-shadow .12s, transform .06s }`.
- `.btn-primary { background:var(--accent); color:var(--on-accent) }` → hover `background:var(--accent-hover); box-shadow:var(--shadow-md); transform:translateY(-1px)`.
- `.btn-secondary { background:var(--bg-tertiary); color:var(--text-primary); border-color:var(--border) }` → hover `border-color:var(--border-strong)`.
- `.btn-ghost { background:transparent; color:var(--text-secondary) }` → hover `background:var(--bg-tertiary); color:var(--text-primary)` (cancel / low-emphasis).
- `.btn-danger { background:var(--danger); color:var(--on-danger) }` (remove/delete).
- `.btn-sm { padding:6px 12px; font-size:0.8125rem }` (inline row actions).
- `.btn:focus-visible { outline:2px solid var(--accent); outline-offset:2px }`; `.btn:disabled { opacity:.5; cursor:not-allowed; transform:none }`.
- `@media (max-width:768px){ .btn-sm{ min-height:44px } }` (touch targets).

### 4.8 Progress bar (AC-4/AC-5 — smackerel EXCEEDS CCManager; the ONLY inline style)
- `.progress { height:8px; background:var(--bg-tertiary); border-radius:999px; overflow:hidden }`.
- `.progress-fill { height:100%; border-radius:999px; background:var(--accent) }`; threshold modifiers `.progress-fill--success { background:var(--success) }`, `--warning`, `--danger`.
- **Width is the ONLY inline style in the surface:** `<div class="progress-fill" style="width:{{pct .SpendProgressCents .SpendRequiredCents}}%">` (bonuses) / `style="width:{{confpct .Confidence}}%"` (rotating). The CSP allows `style=""` (`style-src … 'unsafe-inline'`); the value is **server-computed** by the existing `pct`/`confpct` helpers (no JS).
- **A11y:** wrap with `role="progressbar" aria-valuenow="{{…}}" aria-valuemin="0" aria-valuemax="100"` **and** keep the visible text label (`progress $X of $Y (Z%)` / `confidence N%`). The bar is an *enhancement over* the text, not a replacement — and the text label keeps `data-bonus-progress` / `data-confidence-badge`.

### 4.9 Tag / chip, empty state, alert / priority section
- `.tag` / `.chip { display:inline-block; font-size:0.75rem; padding:2px 8px; background:var(--bg-tertiary); color:var(--text-secondary); border-radius:999px; margin:2px }` (category / equivalents chips; keep `.tag` name where current markup uses it).
- `.empty-state { text-align:center; padding:2rem; color:var(--text-secondary) }` — keep the existing `.empty` class **as an alias** so no current `data-empty` element loses styling (`.empty,.empty-state{…}`).
- `.alert { padding:14px 16px; border-radius:var(--radius-lg); border:1px solid var(--border) }`; `.alert-info { background:var(--info-soft) }`, `.alert-warning { background:var(--warning-soft); border-color:var(--warning) }`, `.alert-danger { background:var(--danger-soft); border-color:var(--danger) }`. Used for the dashboard "⚠ Pending Actions" priority section (tinted card + leading glyph; **not** a JS-dismissible — there is no JS dismiss).

### 4.10 Design-system table (report / categories / admin)
- `.table-wrap { overflow-x:auto; -webkit-overflow-scrolling:touch }` (mobile horizontal-scroll wrapper so columns aren't crushed).
- `.cr-table { width:100%; border-collapse:collapse; background:var(--bg-elevated); border-radius:var(--radius-lg); overflow:hidden }`.
- `.cr-table th { position:sticky; top:0; background:var(--bg-secondary); color:var(--text-secondary); font:600 0.8125rem/1.2; text-align:left; padding:10px 12px }`.
- `.cr-table td { padding:10px 12px; border-top:1px solid var(--border) }`; `.cr-table tbody tr:nth-child(even){ background:var(--bg-secondary) }` (zebra); `tr:hover{ background:var(--bg-tertiary) }`.

### 4.11 Form controls (keep class names existing forms already use)
- Keep `.search-box` as the input/select/textarea class (every form uses it) and add an alias `.form-control`: `.search-box,.form-control { width:100%; padding:10px 12px; border:1px solid var(--border); border-radius:var(--radius-md); font:inherit; background:var(--bg-secondary); color:var(--text-primary) }`.
- `.search-box:focus-visible,.form-control:focus-visible { outline:2px solid var(--accent); outline-offset:1px }` (token-only focus affordance, consistent with §4.12; no extra glow token).
- `.form-section { margin:18px 0 } .form-row { margin:10px 0 } label { font-size:0.9rem; color:var(--text-secondary) }`. The two `cardrewards-card-select` partials' `<select class="search-box">` inherit this automatically.

### 4.12 Focus / motion / reduced-motion
- Global `a:focus-visible, button:focus-visible, .btn:focus-visible, .nav-pill:focus-visible, .stat-card--link:focus-visible { outline:2px solid var(--accent); outline-offset:2px }` (visible keyboard focus in both themes — a11y).
- `@media (prefers-reduced-motion: reduce){ * { transition:none !important; } .stat-card--link:hover,.btn-primary:hover{ transform:none } }`.

---

## 5. Per-Page Template Restructure Plan (the 10 pages + chrome + sub-pages)

**The binding rule for every page:** the visual restructure (flat `<article>`/`<p>`/`<table>` → token-driven card / badge / progress / grid / styled-table) **moves every existing `data-*` attribute onto the semantically-corresponding new element**, dropping/renaming **none** (AC-9; the full map is §7). Glyphs added are decorative (`aria-hidden`). Every `<form>`/`<a>` action/href is unchanged.

**Active-nav mechanism (chrome).** The current `cardrewards-nav` has **no** active state. The redesign marks the current pill **CSS-only, with no Go change**: each page template already invokes `{{template "cardrewards-nav" .}}` and every render map already carries `{{.Title}}`, so `cardrewards-nav` matches a pill's label against `{{.Title}}` and sets `aria-current="page"` on the match; `.nav-pill[aria-current="page"]` then receives the `.active` styling (§4.2). No new Go field, no `dict` helper, no per-page nav duplication. The active-pill is a presentation nice-to-have, **not** an AC (AC-7 requires a *responsive* nav, not an active-state nav); if title-matching proves brittle, ship the nav without an active state (parity with today) — either way **zero Go change**.

### 5.1 Dashboard `cardrewards-dashboard.html` (the flagship — UC-1/AC-3; biggest lift)
- Wrap body in `.app-container > .main-content`. `<h1 data-dashboard class="page-title">Card Rewards</h1>` + `<p class="page-subtitle">{{.Period}}</p>` in a `.page-header`.
- **Stat-grid** (`.stats-grid`, 4 link stat-cards) from `{{len .Recommendations}}` / `{{len .ActiveRotating}}` / `{{len .NeedsVerification}}` / `{{len .PendingReEnroll}}` (Decision (d)). The Needs-Verify tile gets `.stat-card--urgent` + ⚠ when `{{len .NeedsVerification}}` `> 0` (`{{if .NeedsVerification}}`).
- **⚠ Pending Actions** `.alert-warning` priority section FIRST (before recommendations): the `data-needs-verification` cards (each → a `.card` carrying `data-needs-verification`+`data-catalog`, a `.badge-warning data-badge="needs-verification"` with ⚠, and a `.btn-sm.btn-secondary` "Verify" link to `/cards/rotating`) + the `data-pending-reenroll` cards. Empty → `.empty data-empty="pending"`.
- **This Month's Recommendations** in a `.card-grid` (2-up desktop): each `<article class="card" data-rec-row data-rec-category>` → `.card-title` category, `<strong data-rec-card>` best card + optional `.badge-starred data-rec-starred` (★), `.meta data-rec-reason`. Empty → `data-empty="recommendations"`.
- **Active Rotating Categories** cards (`data-active-rotating`+`data-catalog`) with category `.chip`s. Empty → `data-empty="active-rotating"`.

### 5.2 Wallet `cardrewards-wallet.html` (UC-4/AC-6)
- `.page-header` "My Cards" + add links as `.btn-sm.btn-ghost` ("+ Add via discovery", "+ Add custom card").
- `.card-grid` of `<article class="card" data-card-id data-active>`: `.card-title data-card-name`; `.badge data-card-type` (type); status `.badge-success` `● Active` (`data-card-status="active"`) vs `.badge-neutral` `○ Inactive` (`data-card-status="inactive"`); `.summary data-card-note`; a `.btn`-row — Edit `.btn-secondary.btn-sm data-action="edit"` (`<a>`), Activate/Deactivate `.btn-secondary.btn-sm data-action="toggle"` (form), Remove `.btn-danger.btn-sm data-action="delete"` (form). Empty → `data-empty="wallet"`.
- **Sub-pages:** add (`data-candidate-id`/`data-candidate-name` candidate `.card`s + `data-action="confirm-add"`; `data-empty="candidates"`), add-custom (`.form-section` + `data-action="create-custom"`), edit (`.meta data-card-catalog` + `data-action="save-card"`).

### 5.3 Bonuses `cardrewards-bonuses.html` (UC-2/AC-4 — progress bar EXCEEDS CCManager)
- Each `<article class="card" data-bonus-id data-met>`: `.card-title data-bonus-description`; `.meta data-bonus-card`; the **progress bar** (`.progress > .progress-fill` width `{{pct .SpendProgressCents .SpendRequiredCents}}%`, `role="progressbar"`), threshold modifier — `--success` when `.Met`, else `--warning` if near-deadline-unmet, else `--accent`; the **kept text label** `.meta data-bonus-progress` (`progress $X of $Y (Z%)`) + `.badge-success data-bonus-met="true"` ✓ met when `.Met`; deadline `.meta`; the inline "Update progress" `.form-row` + `.btn.btn-sm data-action="update-progress"`.
- "Add bonus" `.form-section` (uses `cardrewards-card-select-required`) at page bottom (top-nav, never occluded). Empty → `data-empty="bonuses"`.

### 5.4 Rotating `cardrewards-rotating.html` (UC-3/AC-5 — confidence meter + verify urgency)
- Each `<article class="card" data-rotating-row data-rotating-id data-needs-verification data-manual-override data-confidence>`: `.card-title` name+period; `.summary data-rotating-categories` chips; **confidence meter** (the same `.progress` primitive, width `{{confpct .Confidence}}%`, threshold success/warning/danger) whose label span keeps `data-confidence-badge`; status badges — `.badge-warning data-badge="needs-verification"` ⚠ vs `.badge-success data-badge="manual-override"` ✓; citations list (`<li data-citation data-citation-source>`) or `.meta data-citation-empty`; the verify `.form-row` + `.btn data-action="verify"`. Empty → `data-empty="rotating"`.

### 5.5 Offers `cardrewards-offers.html` (UC-5/AC-9)
- Each `<article class="card" data-offer-id data-activated>`: `.card-title data-offer-title`; `.meta data-offer-card`; `.badge-info data-shared-limit-group` when present; status `.badge-success` `● activated` vs `.badge-neutral` `○ not activated` (`data-offer-status`); `.btn`-row Edit/Deactivate-Activate/Remove (`data-action` edit/toggle/delete, Remove=`.btn-danger`). "Add offer" `.form-section` (uses `cardrewards-card-select` + `.form-row`s, `data-action="create-offer"`). Empty → `data-empty="offers"`. Edit sub-page keeps `data-action="save-offer"`.

### 5.6 Selections `cardrewards-selections.html` (UC-5/AC-9)
- Each `<article class="card" data-selection-id data-tier>`: `.card-title data-selection-category`; `.meta data-selection-card` + `.badge-neutral data-selection-tier` ("tier N"); Edit `.btn-sm data-action="edit"`. "Save selection" `.form-section` (`cardrewards-card-select` + tier rows, `data-action="save-selection"`). Empty → `data-empty="selections"`. Edit sub-page keeps `data-action="save-selection"`.

### 5.7 Categories `cardrewards-categories.html` (UC-5/AC-9 — design-system table)
- `.table-wrap > table.cr-table`: `<tr data-category data-starred>` with `<td data-category-name>`, `<td data-category-equivalents>` (chips), starred cell `.badge-starred data-starred="true"` ★ vs `—`, priority cell. "Add / update category" `.form-section` (`data-action="save-category"`). Empty → `data-empty="categories"`.

### 5.8 Recommendations `cardrewards-recommendations.html` (UC-5/AC-9)
- `.page-header` "Recommendations — {{.Period}}"; Regenerate `.btn-primary data-action="regenerate"`; "Add / edit" `.form-section data-action="save-recommendation"`. "This period" `.card-grid` of `<article class="card" data-rec-row data-rec-category data-rec-starred>`: `.card-title`; `<strong data-rec-card data-rec-card-id>` + rate; `.meta data-rec-reason`; `.badge-starred data-rec-starred-badge="true"` ★ + Star/Unstar `.btn-sm` (`data-action="star"/"unstar"`, Unstar=`.btn-ghost`). Empty → `data-empty="recommendations"`.

### 5.9 Report `cardrewards-report.html` (UC-5/AC-9 — design-system table)
- `.table-wrap > table.cr-table`: `<tr data-report-row data-report-category>` with category, **best-card** `<td data-report-card>` (emphasized), rate, `<td data-report-reason>`. Empty → `data-empty="report"`.

### 5.10 Admin `cardrewards-admin.html` (UC-5/AC-9 — triggers + run-history table)
- "Manual triggers": Scrape-now / Sync-calendar-now as `.btn-secondary` (forms, `data-action`); disabled → `.meta data-triggers="disabled"`. "Run history" `.table-wrap > table.cr-table`: `<tr data-run-row data-run-id data-run-type data-run-trigger data-run-status data-events-written>` with a status `.badge` colored by `data-run-status` (success/warning/danger), `<td data-events-written-cell>`. Empty → `data-empty="runs"`.

### 5.11 Chrome (`head`/`foot`/`nav`) + `card-select` partials
- `{{define "head"}}`: swap the `:root`/dark block for §3, the `<style>` body for §4; open `<body><div class="app-container"><main class="main-content">`. `{{define "foot"}}`: close `</main></div></body></html>`. (Today `foot` is just `</body></html>` — the wrappers are added head-side and closed foot-side so all ten pages get the shell with no per-page change beyond using the new classes.)
- `{{define "cardrewards-nav"}}`: rewrite to §4.2 markup (`.cr-nav` sticky glass, `.cr-nav__list` of `.nav-pill` `<a>`), `<nav aria-label="Card rewards">` preserved.
- `cardrewards-card-select` / `-required`: unchanged markup; the `<select class="search-box">` picks up §4.11 styling automatically.

---

## 6. CSP Compliance

The surface stays strictly CSP-clean under [router.go](../../internal/api/router.go) ≈L658 (`default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' https://unpkg.com/htmx.org@1.9.12/ 'sha256-…'; img-src 'self' data:; connect-src 'self'`):

| Requirement | How this design satisfies it |
|-------------|------------------------------|
| **Zero inline `<script>`** | No script is added anywhere; the redesign is CSS + semantic HTML only. CCManager's auto-dismiss-flash / double-submit / CSRF inline scripts are **dropped** (not ported). |
| **Zero inline event handlers** | No `onclick`/`onsubmit`/`onchange`/…; every mutation stays a `<form>` PRG submit, every navigation an `<a href>` (as today). |
| **Only one inline style: progress width** | `style="width:NN%"` on `.progress-fill` (bonuses spend %, rotating confidence %) — value server-computed by `pct`/`confpct`. The CSP permits the `style=""` attribute (`style-src 'self' 'unsafe-inline'`). Today's `style="display:inline"` (forms) and `style="border-top…"` (nav) are **removed** in favor of classes; the progress width is the single remaining inline style. |
| **No remote CSS/font** | One embedded `<style>` (inline-style allowed); **no** `<link>`/`@import`/font fetch to any origin → no `style-src`/`font-src` violation (this is why Decision (a) rejects the Inter CDN). |
| **`img-src 'self' data:`** | No new images; emoji/glyphs are text characters, not images. |
| **Guard** | Every card-rewards Playwright spec calls `attachCSPGuard(page)` + `assertNoCSPViolations(page)` ([web/pwa/tests/_support/csp.ts](../../web/pwa/tests/_support/csp.ts)); any inline script/handler or blocked resource fires `securitypolicyviolation` → captured → test fails. The design introduces none, so the guard buffer stays empty (AC-8/UC-9). |

---

## 7. `data-*` Preservation Contract (old → new element map)

Authoritative source: the **current** templates ([cardrewards_templates.go](../../internal/web/cardrewards_templates.go), [cardrewards_dashboard_templates.go](../../internal/web/cardrewards_dashboard_templates.go)) cross-checked against spec AC-9. Every hook below MUST land on the listed new element. Implement/test diff the post-redesign set against the pre-redesign set and prove **zero removals/renames** (AC-9).

| Page | `data-*` hook | Current element | New element it lives on |
|------|---------------|-----------------|--------------------------|
| Dashboard | `data-dashboard` | `<h1>` | `<h1 class="page-title">` |
| Dashboard | `data-rec-row`, `data-rec-category` | rec `<article>` | rec `.card` in `.card-grid` |
| Dashboard | `data-rec-card` | `<strong>` | `<strong>` in `.card-title`/summary |
| Dashboard | `data-rec-starred` | `<span class="tag">` | `.badge-starred` |
| Dashboard | `data-rec-reason` | `<p class="meta">` | `.meta` |
| Dashboard | `data-active-rotating`, `data-catalog` | `<article>` | active-rotating `.card` |
| Dashboard | `data-needs-verification`, `data-catalog` | `<article>` | `.card` inside `.alert-warning` priority section |
| Dashboard | `data-pending-reenroll` | `<article>` | `.card` inside priority section |
| Dashboard | `data-badge` (`needs-verification`) | `<span class="tag">` | `.badge-warning` |
| Dashboard | `data-empty` (`recommendations`/`active-rotating`/`pending`) | `<p class="empty">` | `.empty`/`.empty-state` |
| Wallet | `data-card-id`, `data-active` | `<article>` | wallet `.card` |
| Wallet | `data-card-name` | `<h3>` | `.card-title` |
| Wallet | `data-card-type` | `<span class="type-badge">` | `.badge` |
| Wallet | `data-card-status` (`active`/`inactive`) | `<span class="tag">` | `.badge-success` / `.badge-neutral` |
| Wallet | `data-card-note` | `<p class="summary">` | `.summary` |
| Wallet | `data-action` (`edit`/`toggle`/`delete`) | `<a>`/`<button>` | `.btn`-row `<a>`/`<button>` |
| Wallet | `data-empty="wallet"` | `<p class="empty">` | `.empty-state` |
| Wallet add | `data-candidate-id`, `data-candidate-name`, `data-action="confirm-add"`, `data-empty="candidates"` | candidate `<article>`/`<h3>`/`<button>` | candidate `.card`/`.card-title`/`.btn` |
| Wallet edit | `data-card-catalog`, `data-action="save-card"` | `<p class="meta">`/`<button>` | `.meta`/`.btn-primary` |
| Wallet add-custom | `data-action="create-custom"` | `<button>` | `.btn-primary` |
| Offers | `data-offer-id`, `data-activated` | `<article>` | offer `.card` |
| Offers | `data-offer-title` | `<h3>` | `.card-title` |
| Offers | `data-offer-card` | `<p class="meta">` | `.meta` |
| Offers | `data-shared-limit-group` | `<span class="tag">` | `.badge-info` |
| Offers | `data-offer-status` (`activated`/`not-activated`) | `<span class="tag">` | `.badge-success`/`.badge-neutral` |
| Offers | `data-action` (`edit`/`toggle`/`delete`/`create-offer`) | `<a>`/`<button>` | `.btn`-row / `.btn-primary` |
| Offers | `data-empty="offers"` | `<p class="empty">` | `.empty-state` |
| Offer edit | `data-action="save-offer"` | `<button>` | `.btn-primary` |
| Selections | `data-selection-id`, `data-tier` | `<article>` | selection `.card` |
| Selections | `data-selection-category` | `<h3>` | `.card-title` |
| Selections | `data-selection-card` | `<p class="meta">` | `.meta` |
| Selections | `data-selection-tier` | `<span class="tag">` | `.badge-neutral` |
| Selections | `data-action` (`edit`/`save-selection`) | `<a>`/`<button>` | `.btn-sm`/`.btn-primary` |
| Selections | `data-empty="selections"` | `<p class="empty">` | `.empty-state` |
| Selection edit | `data-action="save-selection"` | `<button>` | `.btn-primary` |
| Bonuses | `data-bonus-id`, `data-met` | `<article>` | bonus `.card` |
| Bonuses | `data-bonus-description` | `<h3>` | `.card-title` |
| Bonuses | `data-bonus-card` | `<p class="meta">` | `.meta` |
| Bonuses | `data-bonus-progress` | `<p class="meta">` (text label) | `.meta` text label **beside** the new `.progress` bar |
| Bonuses | `data-bonus-met="true"` | `<span class="tag">` | `.badge-success` (✓ met) |
| Bonuses | `data-action` (`update-progress`/`create-bonus`) | `<button>` | `.btn`/`.btn-primary` |
| Bonuses | `data-empty="bonuses"` | `<p class="empty">` | `.empty-state` |
| Categories | `data-category`, `data-starred` | `<tr>` | `<tr>` in `.cr-table` |
| Categories | `data-category-name`, `data-category-equivalents` | `<td>` | `<td>` (equivalents as chips) |
| Categories | `data-starred="true"` | `<span class="tag">` | `.badge-starred` (★) |
| Categories | `data-action="save-category"`, `data-empty="categories"` | `<button>`/`<p class="empty">` | `.btn-primary`/`.empty-state` |
| Recommendations | `data-rec-row`, `data-rec-category`, `data-rec-starred` | `<article>` | rec `.card` |
| Recommendations | `data-rec-card`, `data-rec-card-id` | `<strong>` | `<strong>` |
| Recommendations | `data-rec-reason` | `<p class="meta">` | `.meta` |
| Recommendations | `data-rec-starred-badge="true"` | `<span class="tag">` | `.badge-starred` |
| Recommendations | `data-action` (`regenerate`/`save-recommendation`/`star`/`unstar`) | `<button>` | `.btn`/`.btn-primary`/`.btn-sm` |
| Recommendations | `data-empty="recommendations"` | `<p class="empty">` | `.empty-state` |
| Rotating | `data-rotating-row`, `data-rotating-id`, `data-needs-verification`, `data-manual-override`, `data-confidence` | `<article>` | rotating `.card` |
| Rotating | `data-rotating-categories` | `<p class="summary">` | `.summary` (chips) |
| Rotating | `data-confidence-badge` | `<span class="tag">` | confidence label span beside the `.progress` meter |
| Rotating | `data-badge` (`needs-verification`/`manual-override`) | `<span class="tag">` | `.badge-warning`/`.badge-success` |
| Rotating | `data-citation`, `data-citation-source` | `<li>` | `<li>` in citations list |
| Rotating | `data-citation-empty` | `<p class="meta">` | `.meta` |
| Rotating | `data-action="verify"`, `data-empty="rotating"` | `<button>`/`<p class="empty">` | `.btn`/`.empty-state` |
| Report | `data-report-row`, `data-report-category` | `<tr>` | `<tr>` in `.cr-table` |
| Report | `data-report-card`, `data-report-reason` | `<td>` | `<td>` (best-card emphasized) |
| Report | `data-empty="report"` | `<p class="empty">` | `.empty-state` |
| Admin | `data-triggers="disabled"` | `<p class="meta">` | `.meta` |
| Admin | `data-action` (`scrape-now`/`sync-calendar-now`) | `<button>` | `.btn-secondary` |
| Admin | `data-run-row`,`data-run-id`,`data-run-type`,`data-run-trigger`,`data-run-status`,`data-events-written` | `<tr>` | `<tr>` in `.cr-table` |
| Admin | `data-events-written-cell` | `<td>` | `<td>` |
| Admin | `data-empty="runs"` | `<p class="empty">` | `.empty-state` |

> Every hook above is **carried**, never removed/renamed. New badges add a `data-*`-free decorative glyph; the `data-*` stays on the element the locator already targets.

---

## 8. Capability Foundation (DE4 — single-implementation design-system capability)

Per `capability-foundation.md`, the proportionality trigger applies: this is a **shared UI surface across ten pages**. The redesign is therefore specified as **one** capability foundation — a single embedded design-token system + component vocabulary — that all ten pages compose, **not** ten bespoke per-page styles.

**Capability Foundation (the shared contract, all in `{{define "head"}}` + `{{define "cardrewards-nav"}}`):** the §3 token system + the §4 component classes (nav, page-header, card, stat-grid/stat-card, badge, button, progress, tag/chip, empty-state, alert, table, form-control) + the binding policies (tokens-only colors; both `prefers-color-scheme` themes; no JS/no inline handler; carry every `data-*` through; AA contrast; ≥44px touch; `:focus-visible` ring).

**Concrete consumers (the ten pages) — Variation Axes (≥2):**
1. **Page archetype** — *stat-overview + priority-alert* (dashboard) vs *card-grid collection* (wallet / offers / selections / bonuses / rotating / recommendations) vs *design-system table* (categories / report / admin) vs *form sub-page* (wallet-add/-custom/-edit, offer-edit, selection-edit). Each archetype composes the **same** primitives differently.
2. **Status-signal primitive** — badge-only (wallet type/status, offers, selections, categories, admin run-status) vs **progress-bar** (bonuses spend %, rotating confidence %) vs alert/priority-section (dashboard pending-actions). The progress-bar axis is where smackerel **exceeds** CCManager (which has no literal bar).

### Single-Implementation Justification

There is exactly **one** implementation of the design-system capability (no second theme provider, no per-page palette). That is intentional and correct: the spec's Non-Goals forbid per-user theming beyond `prefers-color-scheme` and forbid per-page bespoke palettes; the NFR mandates "one shared design-token system + component vocabulary reused across all ten pages." A second concrete provider would violate the spec. The two **Variation Axes** above (page archetype × status-signal primitive) demonstrate the foundation is genuinely reusable, not a one-off — ten pages and three status-signal strategies compose it.

---

## 9. Test Strategy (hand to bubbles.plan)

**This is presentation-only: tests prove the re-skin preserved structure + behavior + CSP-cleanliness, and added the elevated affordances. No data/handler test changes.**

### 9.1 Regression harness — the 7 existing Playwright specs MUST pass UNCHANGED
The 7 live-stack e2e-ui specs ([web/pwa/tests/cardrewards_dashboard.spec.ts](../../web/pwa/tests/cardrewards_dashboard.spec.ts), `_wallet`, `_offers_selections`, `_recommendations`, `_rotating_verify`, `_categories`, `_admin`) + helper `_support/cardrewards.ts` address every page by `data-*` locators against the real `smackerel-test-e2e-ui` stack (no interception). Because §7 preserves every hook 1:1, these specs prove **data-* + functionality intact under the new skin** with **zero locator change**. The CSP guard (`attachCSPGuard`/`assertNoCSPViolations`, `_support/csp.ts`) in every spec proves **CSP-clean** stays green.
- **Run:** `./smackerel.sh test e2e-ui` (the card-rewards specs are part of this suite).

### 9.2 New assertions for the elevated elements (where the existing specs don't already cover)
Add to the corresponding existing spec files (no new live category needed):
- **Dashboard:** `.stats-grid` present with 4 `.stat-card`s; needs-verify tile `.stat-card--urgent` when `len(NeedsVerification) > 0`; `.alert-warning` pending-actions section precedes the recommendations grid.
- **Bonuses:** a `.progress` bar present with a `.progress-fill` whose inline `width:` is numeric+`%` and matches `data-bonus-progress`'s `(Z%)`; `role="progressbar"` + `aria-valuenow` present; met bonus → `.badge-success` ✓.
- **Rotating:** confidence `.progress` meter present + width matches `data-confidence`/`confpct`; needs-verification → `.badge-warning`, manual-override → `.badge-success`.
- **Badges/buttons:** `.badge-*` classes present where the wireframe specifies; button hierarchy classes present.
- **Responsive nav:** `.cr-nav` sticky; mobile (`<768px` viewport) `.cr-nav__list` is horizontally scrollable (overflow-x), desktop wraps; pills ≥44px.
- **Dark-mode token application:** emulate `prefers-color-scheme: dark` (Playwright `colorScheme: 'dark'`), assert a token-driven property differs from light (e.g. computed `background-color` of `body`/`.card`) — proving the dark `@media` block applies.
- **CSP:** the guard already asserts no violation on every page load — keep it; add no exception.

### 9.3 Go unit (template parse + render) — MUST stay green
[internal/web/handler_test.go](../../internal/web/handler_test.go) parses + renders the template set; the restructured templates must still parse (valid `html/template`) and render every page without error.
- **Run:** `./smackerel.sh test unit` (Go).

### 9.4 Live e2e proof at deploy
On the deployed core, each `/cards` page renders the new design (light + dark) — the e2e-ui suite executed against the live stack is the proof; no manual step is an AC.

### 9.5 Anti-false-positive / regression-quality
Per repo policy, the new dashboard/bonuses/rotating assertions MUST fail if the elevated element is **missing** (direct `expect(locator).toBeVisible()`, no early-return bailout). The dark-mode assertion is adversarial (asserts the computed value **differs** from light — a light-only literal regression fails it). Run `regression-quality-guard.sh` on the touched spec files before plan marks the test scope done.

**Test-type coverage (for the plan's Test Plan table):** unit (Go template render) + e2e-ui (7 regression specs + new elevated assertions, live stack) + the CSP guard (inside each e2e-ui spec). No new integration/stress/load — this is a stateless presentation change with no new query, hot path, or SLA.

---

## 10. Security & Compliance
- **CSP:** unchanged and unweakened (§6); no new script origin, no `font-src` amendment (Decision (a)), no remote resource. Value-safe — these pages render no secrets.
- **AuthZ:** every `/cards` route stays behind the unchanged `webAuthMiddleware` (spec 070); the redesign adds no route and no authorization surface.
- **PII:** generic — no user-identifying content in templates or this doc.
- **Storage:** PostgreSQL-only, unchanged; the redesign performs **no** data read/write change (Decision (d) confirms zero handler edit).

## 11. Configuration & Migrations
None. No config key, no env var, no DB migration, no new dependency, no bundler/asset pipeline. The template `const` strings compile into the existing Go binary exactly as today.

## 12. Observability & Failure Modes
- No new metric/log/trace — presentation-only, no new request path.
- **Failure modes & mitigations:** (1) a dropped/renamed `data-*` → a Playwright locator fails → caught by §9.1 (the binding mitigation is §7's 1:1 map). (2) an accidental inline `<script>`/handler or remote resource → CSP guard fails → caught by §6/§9.2. (3) broken dark contrast → §9.2 dark-mode assertion + the AC-1 a11y review. (4) a stale Docker bundle masking the new skin → rebuild + bundle-freshness verification at the implement/test gate (Docker Bundle Freshness). (5) `backdrop-filter` unsupported → `--nav-bg` rgba still legible (progressive enhancement, §4.2).

## 13. Alternatives & Tradeoffs
| Alternative | Rejected because |
|-------------|------------------|
| Inter via Google-Fonts CDN (CCManager parity) | CSP has no `font-src`; `<link>`+font fetch both fail the spec-077 guard → e2e suite fails (Decision (a)). |
| Self-host / inline Inter via same-origin `@font-face` | Adds a same-origin font asset for a benefit users won't perceive over native faces; system stack is zero-asset, zero-risk (Decision (a)). |
| Amend `router.go` CSP for fonts | A reviewed security-surface change for a cosmetic glyph swap; not worth the cost (Decision (a)). |
| Fixed bottom tab-bar (CCManager) | 10 destinations exceed a ~5-tab budget and a bottom bar occludes the page-bottom add-forms; responsive top-nav scales + never occludes (UX Decision (b)). |
| Additive stat-count view-model fields | Owned-card/active-offer counts aren't on `DashboardPage`; adding them is a handler change + larger failure surface for a cosmetic tile; the four existing-collection `{{len}}` tiles suffice (Decision (d)). |
| A JS-driven theme toggle / live switch | Forbidden by the no-JS Hard Constraint; `prefers-color-scheme` matches CCManager's default with zero script (UX Decision (c)). |
| `color-mix()` for badge tints | Browser-support risk + harder AA guarantee; explicit `--*-soft` tint tokens give deterministic AA pairs (§3). |

## 14. Complexity Tracking
| Deviation from simplest approach | Simpler alternative considered | Why rejected |
|----------------------------------|-------------------------------|--------------|
| Explicit `--*-soft` badge-tint tokens + `--on-accent`/`--on-danger` (beyond CCManager's flat token list) | Reuse the vivid semantic token as both fill and badge text (CCManager's approach) | The vivid value as text on a faint tint is marginal/below AA; explicit AA-checked tint pairs satisfy the AC-1 + a11y NFR "maintain-or-improve" in both themes without `color-mix()` support risk. Minimal, deterministic. |
| Active-nav pill resolved template-only (no Go change) | Add a `NavKey` to each render map in `cardrewards.go` | (d)'s scope boundary forbids editing `cardrewards.go`; and the active-pill is a nice-to-have, not an AC (AC-7 requires a *responsive* nav, not an active-state nav). Ship template-only or omit the active state — either way zero Go change. |

Otherwise: simplest viable approach (two-file template diff, zero Go change, zero CSP change, one embedded stylesheet).

## 15. Open Questions
None blocking. (a) and (d) are settled here; (b) and (c) were settled by UX. The only delegated latitude is implement-phase pixel tuning (spacing, exact breakpoint pill behavior) **within** the §3/§4 contract, and the optional active-nav pill (presentation nice-to-have, not an AC).

---

### Handoff
- **Next required owner:** `bubbles.plan` (author `scopes.md` + DoD from this design; this doc does not create scopes).
- **Then:** `bubbles.implement` (apply the two-file template diff to the §3/§4/§5 contract), `bubbles.test` (run §9 — 7 regression specs unchanged + new elevated/dark/CSP assertions + Go render + live e2e proof).
