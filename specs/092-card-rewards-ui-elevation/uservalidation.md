# User Validation 092 — Card-Rewards Web UI Elevation (match-or-exceed CCManager)

**Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **Scopes:** [scopes.md](scopes.md)

> **How this gate works.** Items are **checked `[x]` by default** (baseline — validated by the spec/design/UX
> phases). The operator **unchecks `[ ]`** an item to report that the elevated `/cards` UI is broken or
> regressed for that capability; an unchecked item is a **BLOCKING user-reported regression** that
> `/bubbles.validate` must root-cause before further work proceeds.

## Checklist (baseline = validated)

- [x] **UC-1 / AC-3 — Dashboard is action-oriented.** `/cards` leads with a stat-grid (4 tiles) + an `.alert-warning` "Pending Actions" priority section, then this-period recommendations and active rotating categories.
- [x] **UC-2 / AC-4 — Bonuses show a visual progress bar + color-coded status.** `/cards/bonuses` renders each bonus's spend progress as a visual bar (success/in-progress), not plain text.
- [x] **UC-3 / AC-5 — Rotating shows confidence + verification urgency visually.** `/cards/rotating` renders confidence as a visual meter; needs-verification reads urgent, manual-override reads resolved.
- [x] **UC-4 / AC-6 — Wallet renders polished card surfaces.** `/cards/wallet` shows each card as a polished surface with a card-type badge and a clear active/inactive state; edit/activate/remove still work.
- [x] **UC-5 / AC-9 — Offers, selections, categories, recommendations, report, admin are elevated.** Each uses the shared component vocabulary (cards, badges, buttons, styled tables) consistently; every form still submits.
- [x] **UC-6 / AC-7 — Responsive nav + layout.** Mobile = single-row horizontally-scrollable pill strip + single-column; desktop = wrapping pill row + width-using multi-column layout.
- [x] **UC-7 / AC-1 — Dark-mode parity.** Every `/cards` page renders a correct, legible dark theme via `prefers-color-scheme` (no broken contrast), and the correct light theme when light is preferred.
- [x] **UC-8 / AC-9 — `data-*` regression contract preserved.** Every existing Playwright/Go locator still resolves; the redesign changed look, not test-addressable structure.
- [x] **UC-9 / AC-8 — CSP-clean, no new JS.** No `/cards` page emits a CSP violation; no inline `<script>` / event handler was introduced; every mutation stays a `<form>` PRG submit.
- [x] **AC-2 — Component vocabulary present.** Stat cards, badges (success/warning/danger/info/neutral/starred), button hierarchy, progress bars, and card surfaces are all token-driven and correct in both themes.
- [x] **AC-10 — Functional parity.** Every route, form action, and link on every `/cards` page works exactly as before — no route/handler/data-model change was required.
- [x] **AC-11 — Scope boundary respected.** Only `cardrewards_templates.go` + `cardrewards_dashboard_templates.go` changed; `cardrewards.go` and `templates.go` (the knowledge-base design system) are untouched.
