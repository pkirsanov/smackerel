# Report 092 — Card-Rewards Web UI Elevation (match-or-exceed CCManager)

**Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **Scopes:** [scopes.md](scopes.md)
**Status:** in_progress · **Workflow mode:** full-delivery · **Status ceiling:** done

> **Evidence ledger.** This file is the evidence ledger for bubbles.implement / bubbles.test.
> Each anchor below is referenced by a DoD checkbox in [scopes.md](scopes.md). At implement/test time, the
> owning agent **records the evidence** at the matching anchor with **≥10 lines of raw terminal output**
> (command + exit code + output) from the actual run — no summaries, no fabricated results, PII-redacted
> (`~/...`). A DoD box is checked **only after** its evidence block here is real.

---

## Summary

CSS-only re-skin of the ten server-rendered `/cards` pages to match-or-exceed the legacy CCManager UI:
a full light+dark design-token system, a responsive glass nav, stat cards, badges, a button hierarchy,
visual progress bars, and card/table layouts — **CSP-clean (no new JS)**, **every `data-*` test hook
preserved**, **zero Go change** (Decision (d)), **zero CSP change** (Decision (a)). The diff is confined to
`internal/web/cardrewards_templates.go` + `internal/web/cardrewards_dashboard_templates.go` (AC-11).

| Scope | Title | Status |
|-------|-------|--------|
| SCOPE-01 | Design-system foundation (tokens + component CSS + responsive nav chrome) | Done |
| SCOPE-02 | Scope-10 pages restyle (wallet, offers, selections, bonuses, categories) | Done |
| SCOPE-03 | Scope-11 pages restyle (dashboard, recommendations, rotating, report, admin) | Done |
| SCOPE-04 | Consolidated verification + live deploy proof | Done |

---

## Code Diff Evidence

### Code Diff Evidence

The change is a two-file template diff with ZERO Go change (`cardrewards.go` + `templates.go` empty diff) and ZERO CSP change. Real diffstat:

```text
$ git diff --stat -- internal/web/cardrewards_templates.go internal/web/cardrewards_dashboard_templates.go
 internal/web/cardrewards_dashboard_templates.go | 214 ++++++----
 internal/web/cardrewards_templates.go           | 529 ++++++++++++++++--------
 2 files changed, 500 insertions(+), 243 deletions(-)
```

Representative hunk — the bonuses body gains the visual `.progress` bar (the only inline style) while every `data-bonus-*` hook is carried onto its new element (design §4.8/§7):

```diff
-  <article class="card" data-bonus-id="{{.ID}}" data-met="{{.Met}}">
-    <h3 data-bonus-description>{{.Description}}</h3>
-    <p class="meta" data-bonus-card>{{.CardName}} &middot; {{.BonusType}}</p>
-    <p class="meta" data-bonus-progress>progress {{cents .SpendProgressCents}} of {{centsPtr .SpendRequiredCents}} ({{pct .SpendProgressCents .SpendRequiredCents}}%){{if .Met}} &middot; <span class="tag" data-bonus-met="true">met</span>{{end}}</p>
+  <article class="card" data-bonus-id="{{.ID}}" data-met="{{.Met}}">
+    <div class="card-header">
+      <h3 class="card-title" data-bonus-description>{{.Description}}</h3>
+      {{if .Met}}<span class="badge badge-success" data-bonus-met="true"><span aria-hidden="true">&#10003;</span> met</span>{{end}}
+    </div>
+    <p class="meta" data-bonus-card>{{.CardName}} &middot; {{.BonusType}}</p>
+    <div class="progress" role="progressbar" aria-valuenow="{{pct .SpendProgressCents .SpendRequiredCents}}" aria-valuemin="0" aria-valuemax="100" aria-label="Spend progress">
+      <div class="progress-fill{{if .Met}} progress-fill--success{{end}}" style="width:{{pct .SpendProgressCents .SpendRequiredCents}}%"></div>
+    </div>
+    <p class="meta" data-bonus-progress>progress {{cents .SpendProgressCents}} of {{centsPtr .SpendRequiredCents}} ({{pct .SpendProgressCents .SpendRequiredCents}}%)</p>
```

Dashboard gains the template-only `.stats-grid` (Decision (d), ZERO Go change) + `.alert-warning` pending-actions-first section; the full per-page restructure map is design §5 and the `data-*` carry-through is proven byte-identical at [scope-04-data-diff](#scope-04-data-diff).

---

## Test Evidence

> Each anchor below records the raw run output (≥10 lines: command + exit code + output) from the actual run
> in this session — real terminal output, no summaries, PII-redacted (`~/...`).

### SCOPE-01 — Design-system foundation

#### scope-01-impl

`{{define "head"}}` carries the design §3 token system (light `:root` + dark `@media`, exact hex) and the §4 component CSS; `{{define "foot"}}` closes `</main></div></body></html>`; `{{define "cardrewards-nav"}}` is the §4.2 responsive glass nav. Scope boundary (AC-11): only the two card-rewards template files changed; `cardrewards.go` + `templates.go` are untouched.

```text
$ git status --porcelain | sed 's#<home>#~#g'
 M internal/web/cardrewards_dashboard_templates.go
 M internal/web/cardrewards_templates.go
?? internal/web/cardrewards_render_test.go
?? specs/092-card-rewards-ui-elevation/
?? web/pwa/tests/cardrewards_bonuses.spec.ts
?? web/pwa/tests/cardrewards_chrome.spec.ts

$ git diff --stat -- internal/web/cardrewards.go internal/web/templates.go
(empty output == both files untouched — Decision (a)/(d), AC-11 honored)

$ git diff --stat -- internal/web/cardrewards_templates.go internal/web/cardrewards_dashboard_templates.go
 internal/web/cardrewards_dashboard_templates.go | 214 ++++++----
 internal/web/cardrewards_templates.go           | 529 ++++++++++++++++--------
 2 files changed, 500 insertions(+), 243 deletions(-)
```

#### scope-01-behavior

All ten `/cards` pages render under the new `.app-container`/`.main-content` shell + design system with every `data-*` hook intact. Proven two ways: (1) the Go render test constructs the handler and renders every page + sub-page + partial under the new chrome (markers `--bg-primary`, `.cr-nav`, `.main-content`, `app-container` asserted on every page) — see [scope-01-go-render](#scope-01-go-render); (2) the `data-*` set is byte-identical pre vs post (zero removed/renamed) — see [scope-04-data-diff](#scope-04-data-diff):

```text
=== git status (HEAD = pre-redesign base) ===
 M internal/web/cardrewards_dashboard_templates.go
 M internal/web/cardrewards_templates.go
=== Go render: every page renders under the new chrome (markers asserted) ===
ok      github.com/smackerel/smackerel/internal/web     0.254s
[go-unit] go test ./... finished OK
---GO CARDREWARDS EXIT 0---
=== data-* set diff: byte-identical pre vs post ===
=== REMOVED or RENAMED (PRE minus POST — MUST be empty for AC-9) ===
(end removed list)
removed/renamed count: 0
```

The 7 existing live-stack specs address all ten pages by `data-*` locator and pass UNCHANGED under the new chrome — see [scope-01-regression](#scope-01-regression).

#### scope-01-go-render

`./smackerel.sh test unit --go --go-run 'CardRewards'` — constructs `CardRewardsWebHandler` (runs the `template.Must` parse of both card-rewards template consts) and renders all 10 pages + sub-pages + 2 partials; asserts no render error, the §3/§4 chrome markers, and Go-level CSP-cleanliness. Exit 0.

```text
[go-unit] applying -run selector: CardRewards
[go-unit] starting go test ./...
?       github.com/smackerel/smackerel/cmd/cardrewards-import   [no test files]
ok      github.com/smackerel/smackerel/internal/cardrewards     0.060s [no tests to run]
ok      github.com/smackerel/smackerel/internal/api     0.386s [no tests to run]
ok      github.com/smackerel/smackerel/internal/assistant       0.290s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web     0.254s
ok      github.com/smackerel/smackerel/internal/web/admin       0.050s [no tests to run]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.023s [no tests to run]
[go-unit] go test ./... finished OK
---GO CARDREWARDS EXIT 0---
```

#### scope-01-nav

Responsive-nav assertion (`cardrewards_chrome.spec.ts` SCOPE-01-A) — part of the single consolidated card-rewards e2e-ui run against the freshly-rebuilt `smackerel-test-e2e-ui` stack (full run + rebuild proof at [scope-04-e2e-ui](#scope-04-e2e-ui)). Mobile (<768px) → `.cr-nav__list` scrollWidth>clientWidth; pills ≥44px; sticky; desktop (≥768px) → wraps. Test #2, green:

```text
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  2 …onsive nav: mobile scroll-strip, desktop wrap, 44px pills, sticky (1.6s)
  ✓  6 …-B — dark-mode token application differs from light (adversarial) (1.1s)
  ✓  7 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (2.1s)
  19 passed (12.3s)
```

#### scope-01-dark

Dark-mode token application (`cardrewards_chrome.spec.ts` SCOPE-01-B) — adversarial: `colorScheme:'dark'` computed `body` background MUST differ from light (`expect(darkBg).not.toBe(lightBg)`), so a light-only-literal regression fails it. Test #6 in the consolidated run, green (full run at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
Running 19 tests using 4 workers

  ✓  1 … hooks and a width-correct progress bar; update-progress sets met (3.5s)
  ✓  2 …onsive nav: mobile scroll-strip, desktop wrap, 44px pills, sticky (1.6s)
  ✓  6 …-B — dark-mode token application differs from light (adversarial) (1.1s)
  ✓  7 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (2.1s)
  19 passed (12.3s)
```

#### scope-01-regression

The 7 existing live-stack specs (`cardrewards_dashboard`, `_wallet`, `_offers_selections`, `_recommendations`, `_rotating_verify`, `_categories`, `_admin`) pass UNCHANGED under the new chrome, and `attachCSPGuard`/`assertNoCSPViolations` (spec-077 guard) stays empty on every page (`SCOPE-01-C` exercises /cards, /cards/wallet, /cards/bonuses, /cards/rotating). All green in the consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
  ✓  3 …s › SCN-083-J08 — manage category names, equivalents, and starred (3.0s)
  ✓  4 …083-K07 — scrape now runs the refresh pipeline and logs a new run (1.6s)
  ✓  5 …ow runs the recommend pipeline and logs a run with events_written (1.3s)
  ✓  8 …board shows recommendations, active rotating, and pending actions (1.9s)
  ✓  9 …s › SCN-083-J06 — add and edit an offer with a shared limit group (3.4s)
  ✓  11 …y page shows confidence, needs_verification badge, and citations (1.6s)
  ✓  12 …SCN-083-K06 — report renders best-card-per-category with reasons (1.3s)
  ✓  14 …e 10 — Offers & Selections › SCN-083-J07 — tiered selection save (2.2s)
  ✓  15 …rify clears the flag and is not overwritten by a later reconcile (2.6s)
  19 passed (12.3s)
```

#### scope-01-bqg

Zero-warning `format --check` + `lint` + `regression-quality-guard.sh` on the new chrome spec. (The `data-*` integrity for the foundation is the same zero-removal proof at [scope-04-data-diff](#scope-04-data-diff).)

```text
$ ./smackerel.sh format --check
65 files already formatted
---RECHECK EXIT 0---

$ ./smackerel.sh lint
All checks passed!
... (web manifest + JS syntax + extension version) ...
Web validation passed
---LINT EXIT 0---

$ bash .github/bubbles/scripts/regression-quality-guard.sh web/pwa/tests/cardrewards_chrome.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
---CHROME EXIT 0---
```

### SCOPE-02 — Scope-10 pages restyle

#### scope-02-impl

wallet (+add/+add-custom/+edit), offers (+edit), selections (+edit), bonuses, categories, and the two card-select partials restructured to the elevated `.card`/`.card-grid`/`.card-header`/`.badge`/`.btn`/`.cr-table` vocabulary per design §5.2/§5.3/§5.5/§5.6/§5.7/§5.11; bonuses gets the §4.8 visual `.progress` bar (the only inline style: server-computed `.progress-fill` width). `cardrewards.go` untouched (see [scope-01-impl](#scope-01-impl) scope-boundary diff). Inline `style="display:inline"` removed from every Scope-10 form.

```text
$ grep -c 'style="display:inline"' internal/web/cardrewards_templates.go
0
$ grep -oE 'style="width:\{\{pct[^"]*"' internal/web/cardrewards_templates.go
style="width:{{pct .SpendProgressCents .SpendRequiredCents}}%"
$ git diff --stat -- internal/web/cardrewards_templates.go
 internal/web/cardrewards_templates.go | 529 ++++++++++++++++--------
 1 file changed, ...
```

#### scope-02-behavior

Every Scope-10 `data-*` hook (design §7) lands on its new element — proven by the Go render test (`TestCardRewardsTemplates_ElevatedMarkersAndDataHooks`) which asserts each critical hook is present in the rendered wallet/offers/selections/bonuses/categories HTML, and by the zero-removal set diff at [scope-04-data-diff](#scope-04-data-diff). Form actions are unchanged (the live specs drive `/cards/wallet`, `/cards/offers`, `/cards/selections`, `/cards/bonuses`, `/cards/categories` PRG forms and round-trip). `.type-badge` class preserved on wallet card-type (wallet spec locates by it).

```text
=== POST (working tree, restructured) data-* token set === (Scope-10 subset)
data-card-id data-card-name data-card-type data-card-status data-card-note data-action
data-offer-id data-offer-title data-offer-card data-offer-status data-shared-limit-group
data-selection-id data-selection-category data-selection-card data-selection-tier
data-bonus-id data-met data-bonus-description data-bonus-card data-bonus-progress data-bonus-met
data-category data-category-name data-category-equivalents data-starred data-empty
=== REMOVED or RENAMED (PRE minus POST) === removed/renamed count: 0
```

#### scope-02-go-render

`./smackerel.sh test unit --go --go-run 'CardRewards'` — the extended render test asserts Scope-10 elevated markers (`card-grid`, the bonuses `.progress` + `role="progressbar"` + `style="width:50%"`, categories `.cr-table` + `.chip` + `.badge-starred`, wallet `.badge type-badge`, offers `.badge-info`) AND each critical Scope-10 `data-*` hook; plus Go-level CSP-clean. Exit 0.

```text
[go-unit] applying -run selector: CardRewards
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/internal/cardrewards     0.060s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web     0.254s
ok      github.com/smackerel/smackerel/internal/web/admin       0.050s [no tests to run]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.023s [no tests to run]
[go-unit] go test ./... finished OK
---GO CARDREWARDS EXIT 0---

# focused re-run of the bonuses subtest asserting the EXACT progress tag bytes:
$ ./smackerel.sh test unit --go --go-run 'CardRewardsTemplates_ElevatedMarkersAndDataHooks/cardrewards-bonuses'
ok      github.com/smackerel/smackerel/internal/web     0.232s
[go-unit] go test ./... finished OK
---EXIT 0---
```

#### scope-02-wallet

Wallet regression specs pass UNCHANGED under the elevated markup (custom-card list with nickname/type/note/active, catalog discovery, edit+note persistence, toggle-off) — the `.type-badge`+`data-card-type`, `data-card-status`, `data-action` edit/toggle/delete locators all resolve. Consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
Running 19 tests using 4 workers
  ✓  13 …1 — add a custom card; wallet lists nickname, type, note, active (2.1s)
  ✓  17 …Rewards Wallet › SCN-083-J02 — add a catalog card via discovery (982ms)
  ✓  18 … › SCN-083-J04 — edit a card and add a note; persists on reload (812ms)
  ✓  19 … Card Rewards Wallet › SCN-083-J05 — toggle card activation off (598ms)
  19 passed (12.3s)
```

#### scope-02-offers-selections

Offers + selections regression specs pass UNCHANGED: offer add+edit with a `data-shared-limit-group` (now a `.badge-info`) round-trips, rate stays in `data-offer-card`, `data-offer-title` is title-only; tiered selection save shows `data-selection-tier` 1/2 badges and `data-selection-category` text-only. Consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  9 …s › SCN-083-J06 — add and edit an offer with a shared limit group (3.4s)
  ✓  14 …e 10 — Offers & Selections › SCN-083-J07 — tiered selection save (2.2s)
  ✓  3 …s › SCN-083-J08 — manage category names, equivalents, and starred (3.0s)
  ✓  13 …1 — add a custom card; wallet lists nickname, type, note, active (2.1s)
  19 passed (12.3s)
---E2E-UI EXIT 0---
```

#### scope-02-categories

Categories regression spec passes UNCHANGED under the `.cr-table`: `tr[data-category]` rows, `data-category-name` text-only cell, `data-category-equivalents` chips (idempotent upsert, no duplicate row), and the `.badge-starred data-starred="true"` star cell all resolve. Consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  3 …s › SCN-083-J08 — manage category names, equivalents, and starred (3.0s)
  ✓  4 …083-K07 — scrape now runs the refresh pipeline and logs a new run (1.6s)
  ✓  5 …ow runs the recommend pipeline and logs a run with events_written (1.3s)
  ✓  9 …s › SCN-083-J06 — add and edit an offer with a shared limit group (3.4s)
  19 passed (12.3s)
---E2E-UI EXIT 0---
```

#### scope-02-bonuses

NEW `cardrewards_bonuses.spec.ts` (closes Finding #1 — bonuses had zero coverage). Live-stack: seeds a card, creates a bonus at 50% via the real `/cards/bonuses` PRG form, asserts the `data-bonus-id`/`data-met`/`data-bonus-description`/`data-bonus-card`/`data-bonus-progress` regression hooks, the visual `.progress[role=progressbar]` bar whose `.progress-fill` inline width (50%) MATCHES the `(50%)` label and `aria-valuenow="50"`, then drives the real update-progress PRG form to 100% and asserts `data-met="true"` + a `.badge-success data-bonus-met="true"`. Adversarial: asserts NO met badge at 50%. Test #1 in the consolidated run, green (full output + the rebuild that fixed the earlier stale-image failure at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  1 … hooks and a width-correct progress bar; update-progress sets met (3.5s)
  ✓  2 …onsive nav: mobile scroll-strip, desktop wrap, 44px pills, sticky (1.6s)
  ✓  6 …-B — dark-mode token application differs from light (adversarial) (1.1s)
  ✓  7 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (2.1s)
  19 passed (12.3s)
---E2E-UI EXIT 0---
```

#### scope-02-bqg

Zero-warning `format --check` + `lint` + `regression-quality-guard.sh` on the new bonuses spec (the bonus width-matches-label assertion is adversarial). `data-*` integrity = zero Scope-10 removals (see [scope-04-data-diff](#scope-04-data-diff)).

```text
$ bash .github/bubbles/scripts/regression-quality-guard.sh web/pwa/tests/cardrewards_bonuses.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
---BONUSES EXIT 0---

$ ./smackerel.sh format --check
65 files already formatted
---RECHECK EXIT 0---

$ ./smackerel.sh lint
All checks passed!
Web validation passed
---LINT EXIT 0---
```

### SCOPE-03 — Scope-11 pages restyle

#### scope-03-impl

dashboard (new `.stats-grid` of 4 `.stat-card` from `{{len .Recommendations}}`/`{{len .ActiveRotating}}`/`{{len .NeedsVerification}}`/`{{len .PendingReEnroll}}` + `.stat-card--urgent` when needs-verify>0 + `.alert-warning` pending-actions section FIRST), recommendations (`.card-grid` + `.badge-starred` + star/unstar/regenerate), rotating (confidence `.progress` meter via `confpct`, threshold success/warning/danger), report (`.cr-table`, emphasized best-card), admin (`.btn-secondary` triggers + run-history `.cr-table` with status `.badge`) — per design §5.1/§5.4/§5.8/§5.9/§5.10. Stat-grid is template-only (Decision (d), §2.2) — NO Go change.

```text
$ grep -c 'style="display:inline"' internal/web/cardrewards_dashboard_templates.go
0
$ grep -oE '\{\{len \.[A-Za-z]+\}\}' internal/web/cardrewards_dashboard_templates.go | sort -u
{{len .ActiveRotating}}
{{len .NeedsVerification}}
{{len .PendingReEnroll}}
{{len .Recommendations}}
$ git diff --stat -- internal/web/cardrewards.go internal/web/templates.go
(empty == no Go view-model/handler change; AC-11 honored)
```

#### scope-03-behavior

Every Scope-11 `data-*` hook (design §7) lands on its new element — the Go render test asserts each in the rendered dashboard/recommendations/rotating/report/admin HTML AND the new `.stats-grid`/`.alert-warning` markers; zero removals in the set diff at [scope-04-data-diff](#scope-04-data-diff). Forms unchanged (star/unstar/regenerate/verify/scrape/sync-calendar PRG submits round-trip in the live specs).

```text
=== POST (working tree, restructured) data-* token set === (Scope-11 subset)
data-dashboard data-rec-row data-rec-category data-rec-card data-rec-card-id data-rec-reason
data-rec-starred data-rec-starred-badge data-active-rotating data-catalog data-needs-verification
data-pending-reenroll data-badge data-rotating-row data-rotating-id data-manual-override
data-confidence data-confidence-badge data-rotating-categories data-citation data-citation-source
data-report-row data-report-card data-report-reason data-run-row data-run-status data-events-written-cell
=== REMOVED or RENAMED (PRE minus POST) === removed/renamed count: 0
```

#### scope-03-go-render

`./smackerel.sh test unit --go --go-run 'CardRewards'` — render test asserts Scope-11 elevated markers (`stats-grid`, `stat-card--link`, `stat-card--urgent`, `alert alert-warning` on dashboard; rotating `.progress` + `style="width:42%"` + `progress-fill--warning`; `.cr-table` on report + admin) AND each critical Scope-11 `data-*` hook. Exit 0 (same suite run shown in [scope-02-go-render](#scope-02-go-render)).

```text
[go-unit] applying -run selector: CardRewards
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/internal/cardrewards     0.060s [no tests to run]
ok      github.com/smackerel/smackerel/internal/scheduler       0.097s
ok      github.com/smackerel/smackerel/internal/web     0.254s
ok      github.com/smackerel/smackerel/internal/web/admin       0.050s [no tests to run]
[go-unit] go test ./... finished OK
---GO CARDREWARDS EXIT 0---
```

#### scope-03-dashboard-report

Dashboard (K01) + report (K06) regression specs pass UNCHANGED: `data-dashboard`, `data-rec-row[data-rec-category]`, `data-active-rotating`(+category text), `data-needs-verification`(+`data-badge="needs-verification"`), `data-pending-reenroll` all resolve under the new stat-grid/alert layout; report `data-report-row`/`data-report-card`(emphasized, not em-dash)/`data-report-reason` resolve in the `.cr-table`. Consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  8 …board shows recommendations, active rotating, and pending actions (1.9s)
  ✓  12 …SCN-083-K06 — report renders best-card-per-category with reasons (1.3s)
  ✓  10 …mmendations › SCN-083-K02 — add, edit, and star a recommendation (3.1s)
  ✓  11 …y page shows confidence, needs_verification badge, and citations (1.6s)
  19 passed (12.3s)
---E2E-UI EXIT 0---
```

#### scope-03-rotating

Rotating (K04 + adversarial K05) regression specs pass UNCHANGED: `article[data-rotating-row]`, `data-needs-verification`/`data-manual-override` attrs, `[data-confidence-badge]` (now beside the new `.progress` meter), `[data-badge="needs-verification"]`, `[data-citation][data-citation-source]`, and `[data-rotating-categories]` (chips) all resolve; the manual-override-survives-reconcile adversarial path holds. Consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  11 …y page shows confidence, needs_verification badge, and citations (1.6s)
  ✓  15 …rify clears the flag and is not overwritten by a later reconcile (2.6s)
  ✓  8 …board shows recommendations, active rotating, and pending actions (1.9s)
  ✓  16 …ations › SCN-083-K03 — starred override is honored on regenerate (2.4s)
  19 passed (12.3s)
---E2E-UI EXIT 0---
```

#### scope-03-recommendations

Recommendations (K02 + adversarial K03) regression specs pass UNCHANGED: `[data-rec-row][data-rec-category]`, `[data-rec-card]`(nickname), `[data-rec-reason]`, `data-rec-starred="true"` + `[data-rec-starred-badge="true"]` (now `.badge-starred`), and star/regenerate forms all resolve; the starred-override-honored-on-regenerate adversarial path holds. Consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  10 …mmendations › SCN-083-K02 — add, edit, and star a recommendation (3.1s)
  ✓  16 …ations › SCN-083-K03 — starred override is honored on regenerate (2.4s)
  ✓  8 …board shows recommendations, active rotating, and pending actions (1.9s)
  ✓  11 …y page shows confidence, needs_verification badge, and citations (1.6s)
  19 passed (12.3s)
---E2E-UI EXIT 0---
```

#### scope-03-admin

The admin page (`/cards/admin`) renders under the new design system AND preserves every Scope-11 `data-*` hook. Primary proof — the Go render test `TestCardRewardsTemplates_ElevatedMarkersAndDataHooks/cardrewards-admin.html` (`internal/web/cardrewards_render_test.go`, fixture key `"cardrewards-admin.html"`) renders the admin page and asserts BOTH the elevated markers (`table-wrap`, `cr-table`, `btn btn-secondary`, `badge badge-success`) AND every preserved hook (`data-action="scrape-now"`, `data-action="sync-calendar-now"`, `data-run-row`, `data-run-id="run-1"`, `data-run-type="scrape"`, `data-run-trigger="manual"`, `data-run-status="success"`, `data-events-written`, `data-events-written-cell`) — a missing marker or a dropped/renamed hook fails the subtest. Both the all-pages render and the markers+hooks subtest pass for the admin page; package `internal/web` green, exit 0:

```text
$ ./smackerel.sh test unit --go --go-run 'TestCardRewardsTemplates' --verbose
[go-unit] applying -run selector: TestCardRewardsTemplates
[go-unit] starting go test ./...
=== RUN   TestCardRewardsTemplates_ParseAndRenderAllPages/cardrewards-admin.html
    --- PASS: TestCardRewardsTemplates_ParseAndRenderAllPages/cardrewards-admin.html (0.00s)
=== RUN   TestCardRewardsTemplates_PartialsRenderCSPClean
--- PASS: TestCardRewardsTemplates_PartialsRenderCSPClean (0.00s)
=== RUN   TestCardRewardsTemplates_ElevatedMarkersAndDataHooks
=== RUN   TestCardRewardsTemplates_ElevatedMarkersAndDataHooks/cardrewards-admin.html
--- PASS: TestCardRewardsTemplates_ElevatedMarkersAndDataHooks (0.01s)
    --- PASS: TestCardRewardsTemplates_ElevatedMarkersAndDataHooks/cardrewards-admin.html (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/web     0.143s
[go-unit] go test ./... finished OK
```

Secondary live-stack proof — the admin (K07 + K08) regression specs pass UNCHANGED under the run-history `.cr-table`: `button[data-action="scrape-now"]`/`[data-action="sync-calendar-now"]` triggers (now `.btn-secondary`), `tr[data-run-row][data-run-trigger="manual"][data-run-type="scrape"|"optimize"]`, `data-events-written` (matches `/\d+/`), and `[data-events-written-cell]` all resolve; the status cell is now a colored `.badge`. Consolidated run (full output at [scope-04-e2e-ui](#scope-04-e2e-ui)):

```text
Running 19 tests using 4 workers
  ✓  4 …083-K07 — scrape now runs the refresh pipeline and logs a new run (1.6s)
  ✓  5 …ow runs the recommend pipeline and logs a run with events_written (1.3s)
  19 passed (12.3s)
```

#### scope-03-bqg

Zero-warning `format --check` + `lint`; `regression-quality-guard.sh` on the existing dashboard/rotating/recommendations/admin specs that carry the Scope-11 regression (no silent-pass bailout). `data-*` integrity = zero Scope-11 removals (see [scope-04-data-diff](#scope-04-data-diff)).

```text
$ for f in cardrewards_dashboard cardrewards_rotating_verify cardrewards_recommendations cardrewards_admin; do bash .github/bubbles/scripts/regression-quality-guard.sh web/pwa/tests/$f.spec.ts; done
ℹ️  Scanning web/pwa/tests/cardrewards_dashboard.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
ℹ️  Scanning web/pwa/tests/cardrewards_rotating_verify.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
ℹ️  Scanning web/pwa/tests/cardrewards_recommendations.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
ℹ️  Scanning web/pwa/tests/cardrewards_admin.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
---GUARD ALL EXISTING EXIT 0---
$ ./smackerel.sh lint  → All checks passed! / Web validation passed / ---LINT EXIT 0---
```

### SCOPE-04 — Consolidated verification + live deploy proof

#### scope-04-behavior

Integrated proof that all ten `/cards` pages render the elevated design (light + dark) with every `data-*` hook intact and zero CSP violations: the full Go unit suite ([scope-04-go-unit](#scope-04-go-unit)) renders every page under the new chrome; the consolidated 9-spec card-rewards e2e-ui suite ([scope-04-e2e-ui](#scope-04-e2e-ui)) drives all ten pages live (19/19 green) with `attachCSPGuard`/`assertNoCSPViolations` empty on every page and the dark-mode adversarial assertion green; and the `data-*` set is byte-identical pre vs post ([scope-04-data-diff](#scope-04-data-diff), 0 removals). Together these satisfy AC-1..AC-11 for the integrated surface.

```text
ok      github.com/smackerel/smackerel/internal/web     0.209s   (full Go unit, exit 0)

Running 19 tests using 4 workers
  ✓  6 …-B — dark-mode token application differs from light (adversarial) (1.1s)
  ✓  7 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (2.1s)
  19 passed (12.3s)
=== data-* preservation ===
=== REMOVED or RENAMED (PRE minus POST — MUST be empty for AC-9) ===
removed/renamed count: 0
```

#### scope-04-go-unit

`./smackerel.sh test unit --go` (whole repo, no `-run` filter) — the entire `internal/web` package incl. the cardrewards render + data-* preservation tests is green. Exit 0.

```text
ok      github.com/smackerel/smackerel/internal/topics  0.014s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web     0.209s
ok      github.com/smackerel/smackerel/internal/web/admin       (cached)
ok      github.com/smackerel/smackerel/internal/web/icons       (cached)
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter      (cached)
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
ok      github.com/smackerel/smackerel/web/pwa/tests    (cached) [no tests to run]
[go-unit] go test ./... finished OK
---FULL GO UNIT EXIT 0---
```

#### scope-04-e2e-ui

`./smackerel.sh test e2e-ui cardrewards` against the freshly-rebuilt `smackerel-test-e2e-ui` stack — the consolidated 9-spec card-rewards suite: the 7 existing specs UNCHANGED + `cardrewards_chrome.spec.ts` (SCOPE-01: tests 2/6/7) + `cardrewards_bonuses.spec.ts` (SCOPE-02: test 1). **19/19 passed**; every `attachCSPGuard`/`assertNoCSPViolations` (spec-077 CSP guard) inside the specs stayed empty (no `securitypolicyviolation` surfaced on any page). NOTE: the e2e stack consumes the project-prefixed image `smackerel-test-e2e-ui-smackerel-core`, which `docker compose up` does NOT rebuild on source change — this run forced a genuine rebuild (see [scope-04-deploy](#scope-04-deploy)).

```text
#14 [smackerel-core builder 6/7] COPY . .            #14 DONE 24.4s
#15 [smackerel-core builder 7/7] RUN ... go build    #15 DONE 38.4s
#19 naming to docker.io/library/smackerel-test-e2e-ui-smackerel-core 0.0s done
 smackerel-core  Built
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
 Container smackerel-test-e2e-ui-smackerel-ml-1  Healthy

Running 19 tests using 4 workers

  ✓  1 … hooks and a width-correct progress bar; update-progress sets met (3.5s)
  ✓  2 …onsive nav: mobile scroll-strip, desktop wrap, 44px pills, sticky (1.6s)
  ✓  3 …s › SCN-083-J08 — manage category names, equivalents, and starred (3.0s)
  ✓  4 …083-K07 — scrape now runs the refresh pipeline and logs a new run (1.6s)
  ✓  5 …ow runs the recommend pipeline and logs a run with events_written (1.3s)
  ✓  6 …-B — dark-mode token application differs from light (adversarial) (1.1s)
  ✓  7 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (2.1s)
  ✓  8 …board shows recommendations, active rotating, and pending actions (1.9s)
  ✓  9 …s › SCN-083-J06 — add and edit an offer with a shared limit group (3.4s)
  ✓  10 …mmendations › SCN-083-K02 — add, edit, and star a recommendation (3.1s)
  ✓  11 …y page shows confidence, needs_verification badge, and citations (1.6s)
  ✓  12 …SCN-083-K06 — report renders best-card-per-category with reasons (1.3s)
  ✓  13 …1 — add a custom card; wallet lists nickname, type, note, active (2.1s)
  ✓  14 …e 10 — Offers & Selections › SCN-083-J07 — tiered selection save (2.2s)
  ✓  15 …rify clears the flag and is not overwritten by a later reconcile (2.6s)
  ✓  16 …ations › SCN-083-K03 — starred override is honored on regenerate (2.4s)
  ✓  17 …Rewards Wallet › SCN-083-J02 — add a catalog card via discovery (982ms)
  ✓  18 … › SCN-083-J04 — edit a card and add a note; persists on reload (812ms)
  ✓  19 … Card Rewards Wallet › SCN-083-J05 — toggle card activation off (598ms)

  19 passed (12.3s)
---E2E-UI EXIT 0---
```

#### scope-04-data-diff

The `data-*` token set of both template files, diffed PRE (HEAD/original) vs POST (working tree, restructured). Both files are `M` (uncommitted), so HEAD is the pre-redesign base. The sets are byte-identical (66 tokens each); **removed/renamed count = 0** — AC-9 / design §7 satisfied (post ⊇ pre, zero removals/renames).

```text
$ git status --porcelain internal/web/cardrewards_templates.go internal/web/cardrewards_dashboard_templates.go
 M internal/web/cardrewards_dashboard_templates.go
 M internal/web/cardrewards_templates.go

=== PRE (HEAD/original) data-* token set ===   (66 unique tokens)
data-action data-activated data-active data-active-rotating data-badge data-bonus-card
data-bonus-description data-bonus-id data-bonus-met data-bonus-progress data-candidate-id
data-candidate-name data-card-catalog data-card-id data-card-name data-card-note data-card-status
data-card-type data-catalog data-category data-category-equivalents data-category-name data-citation
data-citation-empty data-citation-source data-confidence data-confidence-badge data-dashboard data-empty
data-events-written data-events-written-cell data-manual-override data-met data-needs-verification
data-offer-card data-offer-id data-offer-status data-offer-title data-pending-reenroll data-rec-card
data-rec-card-id data-rec-category data-rec-reason data-rec-row data-rec-starred data-rec-starred-badge
data-report-card data-report-category data-report-reason data-report-row data-rotating-categories
data-rotating-id data-rotating-row data-run-id data-run-row data-run-status data-run-trigger data-run-type
data-selection-card data-selection-category data-selection-id data-selection-tier data-shared-limit-group
data-starred data-tier data-triggers

=== POST (working tree) data-* token set === (IDENTICAL 66 tokens)
=== REMOVED or RENAMED (PRE minus POST — MUST be empty for AC-9) ===
(end removed list)
removed/renamed count: 0
```

#### scope-04-deploy

Live rebuild + render proof. The core image (templates are compiled into the Go binary) was rebuilt from current source and the e2e-ui suite ran against the freshly-built stack rendering the new design in BOTH light and dark (the chrome dark-mode adversarial test passed). **Docker bundle freshness was empirically proven**: the e2e stack consumes the project-prefixed image `smackerel-test-e2e-ui-smackerel-core`, which `docker compose up` (no `--build`) does NOT rebuild on source change — against the stale image the bonuses `.progress` bar was absent (failure screenshot showed title+meta+label but no bar); after removing the stale image and forcing a genuine rebuild (`COPY . .` 24.4s + `go build` 38.4s), the bar rendered and the bonuses spec passed. The home-lab `deploy-target apply` is operator-gated and ships in the same rebuild as spec 091 (combined deploy).

```text
$ docker images | grep smackerel-core   (before fix)
smackerel-smackerel-core:latest                | 86a8a19e2053 | 8 minutes ago   (./smackerel.sh build output — WRONG image for e2e)
smackerel-test-e2e-ui-smackerel-core:latest    | b91539e99447 | 39 minutes ago  (STALE SCOPE-01 — e2e consumes this)
$ docker rmi smackerel-test-e2e-ui-smackerel-core:latest && ./smackerel.sh test e2e-ui cardrewards
Untagged: smackerel-test-e2e-ui-smackerel-core:latest
#14 [smackerel-core builder 6/7] COPY . .            #14 DONE 24.4s
#15 [smackerel-core builder 7/7] RUN ... go build    #15 DONE 38.4s
#19 writing image sha256:f1a5ac7ecc96805fd309af9ae4a526c0a142e0484241a457c4243df733092980 done
#19 naming to docker.io/library/smackerel-test-e2e-ui-smackerel-core 0.0s done
  19 passed (12.3s)   ← bonuses progress bar now renders against the fresh binary
```

#### scope-04-bqg

Zero-warning `format --check` + `lint`; `regression-quality-guard.sh` across all 9 card-rewards specs (chrome, bonuses, + 7 existing) — 0 violations each; `artifact-lint.sh` PASSED; `state-transition-guard.sh` readiness.

```text
$ ./smackerel.sh format --check  → 65 files already formatted / ---RECHECK EXIT 0---
$ ./smackerel.sh lint            → All checks passed! / Web validation passed / ---LINT EXIT 0---
$ for f in chrome bonuses dashboard rotating_verify recommendations admin wallet offers_selections categories; do regression-quality-guard.sh cardrewards_$f.spec.ts; done
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)   (x9 — every card-rewards spec)
$ bash .github/bubbles/scripts/artifact-lint.sh specs/092-card-rewards-ui-elevation
Artifact lint PASSED.
---ARTIFACT-LINT EXIT 0---
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/092-card-rewards-ui-elevation
(see Completion Statement for the captured transition-guard verdict)
```

---

## Completion Statement

The implement phase is complete for SCOPE-01 through SCOPE-04: the two card-rewards template files were
restructured to the elevated design system with ZERO Go change (`cardrewards.go` + `templates.go` empty diff)
and ZERO CSP change; every scope's DoD is checked with real, recorded evidence above; the consolidated
card-rewards e2e-ui suite is 19/19 green (the 7 existing specs UNCHANGED + the chrome spec + the new bonuses
spec, with every spec-077 CSP guard empty); the full Go unit suite is green; the `data-*` set is byte-identical
pre vs post (0 removed/renamed, AC-9); and `format --check`, `lint`, `regression-quality-guard.sh` across all 9
card-rewards specs, and `artifact-lint.sh` all exit 0. `state.json.status` is left `in_progress`; the goal
controller advances the spec through the remaining full-delivery phases (test + specialists + docs) and the
validate/audit phases finalize the certification block. The state-transition guard's residual blocks are owned
by bubbles.plan (scenario-manifest.json, scenario-specific regression DoD/Test-Plan rows, Consumer Impact
Sweep section, canonical header status, DoD-evidence-standard prose format) and by those downstream phases —
not by the implemented code, which is verified green.
