// Spec 083 Scope 10 — card-rewards web templates.
//
// Parsed in NewCardRewardsWebHandler on top of the shared allTemplates set, so
// these pages reuse the shared "head"/"foot" chrome and the design-token CSS
// palette (var(--…)) — no hardcoded colors. Interactivity is plain <form>
// submits (Post/Redirect/Get); there are NO inline <script> blocks and NO
// inline event handlers, so the pages stay clean under the global CSP
// (script-src 'self' + pinned htmx + one hashed inline theme script only).
package web

const cardRewardsTemplates = `
{{define "head"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} - Smackerel</title>
<style>
/* Spec 092 — card-rewards design-token system (light default, §3.1). All page
   colors derive from these tokens (no hex literals in markup). */
:root {
  --bg-primary:#ffffff; --bg-secondary:#f7f6f3; --bg-tertiary:#ecebe5; --bg-elevated:#ffffff;
  --text-primary:#1a1a18; --text-secondary:#5c5b57; --text-tertiary:#84837e;
  --accent:#0066cc; --accent-hover:#0052a3; --on-accent:#ffffff; --on-danger:#ffffff;
  --success:#1e7d34; --warning:#8a5a00; --danger:#c4291c; --info:#4b48c4; --starred:#8a6d00;
  --success-soft:#e6f4ea; --warning-soft:#f8efd7; --danger-soft:#fbe7e4; --info-soft:#ececfb;
  --neutral-soft:#ecebe5; --starred-soft:#fbf1d4;
  --border:#e2e0da; --border-strong:#ccc9c1;
  --shadow-sm:0 1px 2px rgba(0,0,0,0.05);
  --shadow-md:0 4px 12px rgba(0,0,0,0.09);
  --shadow-lg:0 8px 24px rgba(0,0,0,0.13);
  --radius-sm:8px; --radius-md:12px; --radius-lg:16px; --radius-xl:24px;
  --nav-bg:rgba(255,255,255,0.80); --nav-border:#e2e0da;
}
/* §3.2 — dark theme re-declares colors + shadows + nav only; radii unchanged. */
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
/* §4.1 reset + base + app shell */
*,*::before,*::after { box-sizing:border-box; margin:0; padding:0; }
html { font-size:16px; }
body { font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,system-ui,sans-serif; background:var(--bg-primary); color:var(--text-primary); line-height:1.5; -webkit-font-smoothing:antialiased; }
.app-container { min-height:100vh; min-height:100dvh; display:flex; flex-direction:column; }
.main-content { width:100%; max-width:1200px; margin:0 auto; padding:16px; }
@media (min-width:768px) { .main-content { padding:24px 32px; } }
a { color:var(--accent); }
/* Spec 100 SCOPE-01 — shared cross-surface app-shell nav (renders above the card sub-nav) */
.app-shell-nav { display:flex; flex-wrap:wrap; align-items:center; gap:8px; padding:10px 16px; border-bottom:1px solid var(--nav-border); background:var(--nav-bg); }
@media (min-width:768px) { .app-shell-nav { padding:10px 32px; } }
.app-shell-link { color:var(--text-secondary); text-decoration:none; font-size:0.9rem; padding:6px 10px; border-radius:var(--radius-md); min-height:44px; display:inline-flex; align-items:center; }
.app-shell-link:hover { background:var(--bg-tertiary); color:var(--text-primary); }
.app-shell-link[aria-current="page"], .app-shell-link.active { background:var(--accent); color:var(--on-accent); }
/* §4.2 responsive glass nav (breaks out to the main-content edges, sticky top) */
.cr-nav { position:sticky; top:0; z-index:10; margin:-16px -16px 16px; background:var(--nav-bg); backdrop-filter:blur(18px) saturate(1.4); -webkit-backdrop-filter:blur(18px) saturate(1.4); border-bottom:1px solid var(--nav-border); }
@media (min-width:768px) { .cr-nav { margin:-24px -32px 24px; } }
.cr-nav__brand { display:block; font-weight:700; font-size:0.95rem; padding:10px 16px 0; color:var(--text-primary); }
.cr-nav__scroll { position:relative; }
.cr-nav__list { display:flex; gap:8px; padding:10px 16px; margin:0; list-style:none; flex-wrap:nowrap; overflow-x:auto; scroll-snap-type:x proximity; -webkit-overflow-scrolling:touch; }
@media (min-width:768px) { .cr-nav__list { flex-wrap:wrap; overflow-x:visible; } }
.cr-nav__scroll::after { content:""; position:absolute; top:0; right:0; bottom:0; width:36px; pointer-events:none; background:linear-gradient(to right, transparent, var(--nav-bg)); }
@media (min-width:768px) { .cr-nav__scroll::after { display:none; } }
.nav-pill { display:inline-flex; align-items:center; min-height:44px; padding:8px 14px; border-radius:var(--radius-md); color:var(--text-secondary); text-decoration:none; white-space:nowrap; scroll-snap-align:start; font-size:0.9rem; }
.nav-pill:hover { background:var(--bg-tertiary); color:var(--text-primary); }
.nav-pill[aria-current="page"], .nav-pill.active { background:var(--accent); color:var(--on-accent); }
/* §4.3 page header */
.page-header { margin:8px 0 20px; }
.page-title { font-weight:700; font-size:2rem; line-height:1.15; letter-spacing:-0.02em; }
@media (min-width:768px) { .page-title { font-size:2.5rem; } }
.page-subtitle { color:var(--text-secondary); font-size:1rem; margin-top:4px; }
/* heading defaults (transitional: pre-restructure page bodies use bare h1/h2/h3) */
h1 { font-weight:700; font-size:2rem; line-height:1.15; letter-spacing:-0.02em; margin:8px 0 16px; }
@media (min-width:768px) { h1 { font-size:2.5rem; } }
h2 { font-weight:600; font-size:1.125rem; line-height:1.3; margin:20px 0 10px; }
h3 { font-weight:600; font-size:1.125rem; line-height:1.3; margin-bottom:4px; }
/* §4.4 card / surface */
.card { background:var(--bg-elevated); border:1px solid var(--border); border-radius:var(--radius-lg); padding:16px 18px; box-shadow:var(--shadow-sm); margin:12px 0; }
.card-header { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }
.card-title { font-weight:600; font-size:1.125rem; line-height:1.3; }
.card-grid { display:grid; gap:14px; grid-template-columns:1fr; }
@media (min-width:768px) { .card-grid { grid-template-columns:repeat(2,1fr); } }
.meta { font-size:0.875rem; color:var(--text-secondary); }
.card .summary, .summary { font-size:0.9rem; margin-top:6px; color:var(--text-primary); }
.btn-row { display:flex; flex-wrap:wrap; align-items:center; gap:8px; margin-top:10px; }
/* §4.5 stat-grid + stat-card */
.stats-grid { display:grid; gap:12px; grid-template-columns:repeat(2,1fr); margin:12px 0 20px; }
@media (min-width:768px) { .stats-grid { grid-template-columns:repeat(4,1fr); } }
.stat-card { display:flex; flex-direction:column; gap:4px; padding:16px; background:var(--bg-elevated); border:1px solid var(--border); border-radius:var(--radius-lg); box-shadow:var(--shadow-sm); text-decoration:none; color:inherit; }
.stat-card--link:hover { border-color:var(--border-strong); box-shadow:var(--shadow-md); transform:translateY(-2px); }
.stat-card--link:focus-visible { outline:2px solid var(--accent); outline-offset:2px; }
.stat-card--urgent { border-color:var(--warning); background:var(--warning-soft); }
.stat-value { font-weight:700; font-size:2rem; line-height:1.1; letter-spacing:-0.02em; }
@media (min-width:768px) { .stat-value { font-size:2.5rem; } }
.stat-label { font-weight:500; font-size:0.75rem; line-height:1.2; letter-spacing:0.05em; text-transform:uppercase; color:var(--text-secondary); }
@media (min-width:768px) { .stat-label { font-size:0.8125rem; } }
/* §4.6 badge system (text+glyph carry meaning; color is reinforcement) */
.badge { display:inline-flex; align-items:center; gap:4px; font-weight:600; font-size:0.75rem; line-height:1; padding:3px 9px; border-radius:999px; background:var(--neutral-soft); color:var(--text-secondary); }
.badge-success { background:var(--success-soft); color:var(--success); }
.badge-warning { background:var(--warning-soft); color:var(--warning); }
.badge-danger { background:var(--danger-soft); color:var(--danger); }
.badge-info { background:var(--info-soft); color:var(--info); }
.badge-neutral { background:var(--neutral-soft); color:var(--text-secondary); }
.badge-starred { background:var(--starred-soft); color:var(--starred); }
/* transitional: pre-restructure bodies (and the kept .type-badge hook) reuse the neutral badge look */
.tag, .type-badge { display:inline-flex; align-items:center; gap:4px; font-weight:600; font-size:0.75rem; line-height:1; padding:3px 9px; border-radius:999px; background:var(--neutral-soft); color:var(--text-secondary); margin:2px 0; }
/* §4.7 button hierarchy */
.btn { display:inline-flex; align-items:center; gap:6px; font-weight:600; font-size:0.9375rem; line-height:1; padding:9px 16px; border-radius:var(--radius-md); border:1px solid transparent; cursor:pointer; text-decoration:none; transition:background .12s, box-shadow .12s, transform .06s; }
.btn-primary { background:var(--accent); color:var(--on-accent); }
.btn-primary:hover { background:var(--accent-hover); box-shadow:var(--shadow-md); transform:translateY(-1px); }
.btn-secondary { background:var(--bg-tertiary); color:var(--text-primary); border-color:var(--border); }
.btn-secondary:hover { border-color:var(--border-strong); }
.btn-ghost { background:transparent; color:var(--text-secondary); }
.btn-ghost:hover { background:var(--bg-tertiary); color:var(--text-primary); }
.btn-danger { background:var(--danger); color:var(--on-danger); }
.btn-sm { padding:6px 12px; font-size:0.8125rem; }
.btn:disabled { opacity:.5; cursor:not-allowed; transform:none; }
@media (max-width:767px) { .btn-sm { min-height:44px; } }
/* transitional bare button (pre-restructure form submits) */
button { font:inherit; font-weight:600; font-size:0.9375rem; padding:9px 16px; border-radius:var(--radius-md); border:1px solid var(--border); background:var(--bg-tertiary); color:var(--text-primary); cursor:pointer; }
button:hover { border-color:var(--border-strong); }
/* §4.8 progress bar (width is server-computed; the only inline style in the surface) */
.progress { height:8px; background:var(--bg-tertiary); border-radius:999px; overflow:hidden; margin:8px 0; }
.progress-fill { height:100%; border-radius:999px; background:var(--accent); }
.progress-fill--success { background:var(--success); }
.progress-fill--warning { background:var(--warning); }
.progress-fill--danger { background:var(--danger); }
/* §4.9 tag/chip, empty, alert */
.chip { display:inline-block; font-size:0.75rem; padding:2px 8px; background:var(--bg-tertiary); color:var(--text-secondary); border-radius:999px; margin:2px; }
.empty, .empty-state { text-align:center; padding:2rem; color:var(--text-secondary); }
.alert { padding:14px 16px; border-radius:var(--radius-lg); border:1px solid var(--border); margin:12px 0; }
.alert-info { background:var(--info-soft); }
.alert-warning { background:var(--warning-soft); border-color:var(--warning); }
.alert-danger { background:var(--danger-soft); border-color:var(--danger); }
/* §4.10 design-system table */
.table-wrap { overflow-x:auto; -webkit-overflow-scrolling:touch; margin:12px 0; }
.cr-table { width:100%; border-collapse:collapse; background:var(--bg-elevated); border-radius:var(--radius-lg); overflow:hidden; }
.cr-table th { background:var(--bg-secondary); color:var(--text-secondary); font-weight:600; font-size:0.8125rem; line-height:1.2; text-align:left; padding:10px 12px; }
.cr-table td { padding:10px 12px; border-top:1px solid var(--border); }
.cr-table tbody tr:nth-child(even) { background:var(--bg-secondary); }
.cr-table tbody tr:hover { background:var(--bg-tertiary); }
/* transitional bare table (pre-restructure categories/report/admin) */
table { width:100%; border-collapse:collapse; background:var(--bg-elevated); border-radius:var(--radius-lg); overflow:hidden; margin:12px 0; }
th { background:var(--bg-secondary); color:var(--text-secondary); font-weight:600; font-size:0.8125rem; text-align:left; padding:10px 12px; }
td { padding:10px 12px; border-top:1px solid var(--border); }
/* §4.11 form controls (keep .search-box — every form references it) */
.search-box, .form-control { width:100%; padding:10px 12px; border:1px solid var(--border); border-radius:var(--radius-md); font:inherit; background:var(--bg-secondary); color:var(--text-primary); }
.form-section { margin:18px 0; }
.form-row { margin:10px 0; }
label { font-size:0.9rem; color:var(--text-secondary); }
input, select, textarea { color:var(--text-primary); }
form p { margin:10px 0; }
/* §4.12 focus / motion */
a:focus-visible, button:focus-visible, .btn:focus-visible, .nav-pill:focus-visible, .stat-card--link:focus-visible, .search-box:focus-visible, .form-control:focus-visible { outline:2px solid var(--accent); outline-offset:2px; }
@media (prefers-reduced-motion: reduce) { * { transition:none !important; } .stat-card--link:hover, .btn-primary:hover { transform:none; } }
</style>
</head>
<body>
<div class="app-container">
<nav class="app-shell-nav" aria-label="Primary">{{template "app-shell-nav" .}}</nav>
<main class="main-content">
{{end}}

{{define "foot"}}</main></div></body></html>{{end}}

{{define "cardrewards-nav"}}
<nav class="cr-nav" aria-label="Card rewards">
  <span class="cr-nav__brand" aria-hidden="true">&#128179; Card Rewards</span>
  <div class="cr-nav__scroll">
    <ul class="cr-nav__list">
      <li><a class="nav-pill{{if eq .Title "Card Rewards"}} active{{end}}" href="/cards"{{if eq .Title "Card Rewards"}} aria-current="page"{{end}}>Dashboard</a></li>
      <li><a class="nav-pill{{if eq .Title "My Cards"}} active{{end}}" href="/cards/wallet"{{if eq .Title "My Cards"}} aria-current="page"{{end}}>My Cards</a></li>
      <li><a class="nav-pill{{if eq .Title "Offers"}} active{{end}}" href="/cards/offers"{{if eq .Title "Offers"}} aria-current="page"{{end}}>Offers</a></li>
      <li><a class="nav-pill{{if eq .Title "Selections"}} active{{end}}" href="/cards/selections"{{if eq .Title "Selections"}} aria-current="page"{{end}}>Selections</a></li>
      <li><a class="nav-pill{{if eq .Title "Sign-up Bonuses"}} active{{end}}" href="/cards/bonuses"{{if eq .Title "Sign-up Bonuses"}} aria-current="page"{{end}}>Sign-up Bonuses</a></li>
      <li><a class="nav-pill{{if eq .Title "Categories"}} active{{end}}" href="/cards/categories"{{if eq .Title "Categories"}} aria-current="page"{{end}}>Categories</a></li>
      <li><a class="nav-pill{{if eq .Title "Recommendations"}} active{{end}}" href="/cards/recommendations"{{if eq .Title "Recommendations"}} aria-current="page"{{end}}>Recommendations</a></li>
      <li><a class="nav-pill{{if eq .Title "Rotating Categories"}} active{{end}}" href="/cards/rotating"{{if eq .Title "Rotating Categories"}} aria-current="page"{{end}}>Rotating</a></li>
      <li><a class="nav-pill{{if eq .Title "Optimization Report"}} active{{end}}" href="/cards/report"{{if eq .Title "Optimization Report"}} aria-current="page"{{end}}>Report</a></li>
      <li><a class="nav-pill{{if eq .Title "Admin"}} active{{end}}" href="/cards/admin"{{if eq .Title "Admin"}} aria-current="page"{{end}}>Admin</a></li>
    </ul>
  </div>
</nav>
{{end}}

{{define "cardrewards-wallet.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">My Cards</h1>
  <p class="page-subtitle">Your wallet of cards and their reward profiles.</p>
</div>
<div class="btn-row">
  <a class="btn btn-secondary btn-sm" href="/cards/wallet/add">&plus; Add via discovery</a>
  <a class="btn btn-secondary btn-sm" href="/cards/wallet/add-custom">&plus; Add custom card</a>
</div>
{{if .Cards}}
  <div class="card-grid">
  {{range .Cards}}
  <article class="card" data-card-id="{{.ID}}" data-active="{{.Active}}">
    <div class="card-header">
      <h3 class="card-title" data-card-name>{{if deref .Nickname}}{{deref .Nickname}}{{else}}{{.CatalogName}}{{end}}</h3>
      {{if .Active}}<span class="badge badge-success" data-card-status="active"><span aria-hidden="true">&#9679;</span> Active</span>{{else}}<span class="badge badge-neutral" data-card-status="inactive"><span aria-hidden="true">&#9675;</span> Inactive</span>{{end}}
    </div>
    <p class="meta"><span class="badge type-badge" data-card-type="{{.CardType}}">{{.CardType}}</span> &middot; {{.CatalogName}}</p>
    {{if deref .Note}}<p class="summary" data-card-note>{{deref .Note}}</p>{{end}}
    <div class="btn-row">
      <a class="btn btn-secondary btn-sm" href="/cards/wallet/{{.ID}}/edit" data-action="edit">Edit</a>
      <form method="post" action="/cards/wallet/{{.ID}}/toggle">
        <button class="btn btn-secondary btn-sm" type="submit" data-action="toggle">{{if .Active}}Deactivate{{else}}Activate{{end}}</button>
      </form>
      <form method="post" action="/cards/wallet/{{.ID}}/delete">
        <button class="btn btn-danger btn-sm" type="submit" data-action="delete">Remove</button>
      </form>
    </div>
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="wallet">No cards yet. Add one to get started.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-wallet-add.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Add Card</h1>
  <p class="page-subtitle">Search the catalog and add a card to your wallet.</p>
</div>
<form class="form-section" method="get" action="/cards/wallet/add">
  <div class="form-row"><input class="search-box" type="text" name="q" value="{{.Query}}" placeholder="Search the card catalog (e.g. custom cash)" aria-label="Search catalog"></div>
  <button class="btn btn-primary" type="submit">Search</button>
</form>
{{if .Query}}
  <h2 class="card-title">Candidates</h2>
  {{if .Candidates}}
    <div class="card-grid">
    {{range .Candidates}}
    <article class="card" data-candidate-id="{{.CardID}}">
      <h3 class="card-title" data-candidate-name>{{.Name}}</h3>
      <p class="meta">match: {{.MatchType}} &middot; score {{printf "%.2f" .Score}}</p>
      <form method="post" action="/cards/wallet">
        <input type="hidden" name="catalog_id" value="{{.CardID}}">
        <div class="form-row"><input class="search-box" type="text" name="nickname" placeholder="Nickname (optional)" aria-label="Nickname"></div>
        <button class="btn btn-primary btn-sm" type="submit" data-action="confirm-add">Add to wallet</button>
      </form>
    </article>
    {{end}}
    </div>
  {{else}}
    <p class="empty-state" data-empty="candidates">No catalog matches for &ldquo;{{.Query}}&rdquo;.</p>
  {{end}}
{{end}}
<p class="meta"><a href="/cards/wallet">&larr; Back to wallet</a></p>
{{template "foot"}}
{{end}}

{{define "cardrewards-wallet-add-custom.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Add Custom Card</h1>
  <p class="page-subtitle">Create a card that isn&rsquo;t in the catalog.</p>
</div>
<div class="card">
<form method="post" action="/cards/wallet/custom">
  <div class="form-row"><label>Name<br><input class="search-box" type="text" name="name" required aria-label="Name"></label></div>
  <div class="form-row"><label>Issuer<br><input class="search-box" type="text" name="issuer" required aria-label="Issuer"></label></div>
  <div class="form-row"><label>Card type<br>
    <select name="card_type" class="search-box" aria-label="Card type">
      <option value="fixed">fixed</option>
      <option value="rotating">rotating</option>
      <option value="user-selected">user-selected</option>
    </select>
  </label></div>
  <div class="form-row"><label>Annual fee (cents)<br><input class="search-box" type="number" name="annual_fee_cents" value="0" min="0" aria-label="Annual fee cents"></label></div>
  <div class="form-row"><label>Nickname (optional)<br><input class="search-box" type="text" name="nickname" aria-label="Nickname"></label></div>
  <div class="form-row"><label>Note (optional)<br><textarea class="search-box" name="note" aria-label="Note"></textarea></label></div>
  <div class="btn-row">
    <button class="btn btn-primary" type="submit" data-action="create-custom">Create card</button>
    <a class="btn btn-ghost" href="/cards/wallet">Cancel</a>
  </div>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-wallet-edit.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Edit Card</h1>
</div>
<div class="card">
<form method="post" action="/cards/wallet/{{.Card.ID}}">
  <p class="meta" data-card-catalog>{{.Card.CatalogName}}</p>
  <div class="form-row"><label>Nickname<br><input class="search-box" type="text" name="nickname" value="{{deref .Card.Nickname}}" aria-label="Nickname"></label></div>
  <div class="form-row"><label>Note<br><textarea class="search-box" name="note" aria-label="Note">{{deref .Card.Note}}</textarea></label></div>
  <div class="btn-row">
    <button class="btn btn-primary" type="submit" data-action="save-card">Save</button>
    <a class="btn btn-ghost" href="/cards/wallet">Cancel</a>
  </div>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-offers.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Offers</h1>
  <p class="page-subtitle">Card-linked offers and activation status.</p>
</div>
{{if .Offers}}
  <div class="card-grid">
  {{range .Offers}}
  <article class="card" data-offer-id="{{.ID}}" data-activated="{{.Activated}}">
    <div class="card-header">
      <h3 class="card-title" data-offer-title>{{.Title}}</h3>
      {{if .Activated}}<span class="badge badge-success" data-offer-status="activated"><span aria-hidden="true">&#9679;</span> activated</span>{{else}}<span class="badge badge-neutral" data-offer-status="not-activated"><span aria-hidden="true">&#9675;</span> not activated</span>{{end}}
    </div>
    <p class="meta" data-offer-card>{{.CardName}} &middot; {{.Category}} &middot; {{printf "%.2f" .Rate}} {{.RateType}}</p>
    <p class="meta">limit {{centsPtr .LimitCents}}{{if .ActivationRequired}} &middot; activation required{{end}}</p>
    {{if deref .SharedLimitGroup}}<p><span class="badge badge-info" data-shared-limit-group="{{deref .SharedLimitGroup}}">shared limit: {{deref .SharedLimitGroup}}</span></p>{{end}}
    <div class="btn-row">
      <a class="btn btn-secondary btn-sm" href="/cards/offers/{{.ID}}/edit" data-action="edit">Edit</a>
      <form method="post" action="/cards/offers/{{.ID}}/toggle"><button class="btn btn-secondary btn-sm" type="submit" data-action="toggle">{{if .Activated}}Deactivate{{else}}Activate{{end}}</button></form>
      <form method="post" action="/cards/offers/{{.ID}}/delete"><button class="btn btn-danger btn-sm" type="submit" data-action="delete">Remove</button></form>
    </div>
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="offers">No offers yet.</p>
{{end}}
<div class="card form-section">
<h2 class="card-title">Add offer</h2>
<form method="post" action="/cards/offers">
  {{template "cardrewards-card-select" .}}
  <div class="form-row"><label>Title<br><input class="search-box" type="text" name="title" required aria-label="Title"></label></div>
  <div class="form-row"><label>Category<br><input class="search-box" type="text" name="category" required aria-label="Category"></label></div>
  <div class="form-row"><label>Rate<br><input class="search-box" type="number" step="0.01" name="rate" value="0" aria-label="Rate"></label></div>
  <div class="form-row"><label>Rate type<br>
    <select name="rate_type" class="search-box" aria-label="Rate type">
      <option value="percent">percent</option>
      <option value="multiplier">multiplier</option>
      <option value="points">points</option>
    </select>
  </label></div>
  <div class="form-row"><label>Shared limit group (optional)<br><input class="search-box" type="text" name="shared_limit_group" aria-label="Shared limit group"></label></div>
  <div class="form-row"><label>Limit (cents, optional)<br><input class="search-box" type="number" name="limit_cents" min="0" aria-label="Limit cents"></label></div>
  <div class="form-row"><label><input type="checkbox" name="activation_required"> Activation required</label></div>
  <button class="btn btn-primary" type="submit" data-action="create-offer">Add offer</button>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-offer-edit.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Edit Offer</h1>
</div>
<div class="card">
<form method="post" action="/cards/offers/{{.Offer.ID}}">
  <div class="form-row"><label>Card<br>
    <select name="user_card_id" class="search-box" aria-label="Card">
      <option value="">&mdash; General &mdash;</option>
      {{$sel := deref .Offer.UserCardID}}{{range .Cards}}<option value="{{.ID}}" {{if eq .ID $sel}}selected{{end}}>{{.Name}}</option>{{end}}
    </select>
  </label></div>
  <div class="form-row"><label>Title<br><input class="search-box" type="text" name="title" value="{{.Offer.Title}}" required aria-label="Title"></label></div>
  <div class="form-row"><label>Category<br><input class="search-box" type="text" name="category" value="{{.Offer.Category}}" required aria-label="Category"></label></div>
  <div class="form-row"><label>Rate<br><input class="search-box" type="number" step="0.01" name="rate" value="{{printf "%.2f" .Offer.Rate}}" aria-label="Rate"></label></div>
  <div class="form-row"><label>Rate type<br>
    <select name="rate_type" class="search-box" aria-label="Rate type">
      <option value="percent" {{if eq .Offer.RateType "percent"}}selected{{end}}>percent</option>
      <option value="multiplier" {{if eq .Offer.RateType "multiplier"}}selected{{end}}>multiplier</option>
      <option value="points" {{if eq .Offer.RateType "points"}}selected{{end}}>points</option>
    </select>
  </label></div>
  <div class="form-row"><label>Shared limit group (optional)<br><input class="search-box" type="text" name="shared_limit_group" value="{{deref .Offer.SharedLimitGroup}}" aria-label="Shared limit group"></label></div>
  <div class="form-row"><label>Limit (cents, optional)<br><input class="search-box" type="number" name="limit_cents" value="{{intPtr .Offer.LimitCents}}" min="0" aria-label="Limit cents"></label></div>
  <div class="form-row"><label><input type="checkbox" name="activation_required" {{if .Offer.ActivationRequired}}checked{{end}}> Activation required</label></div>
  <div class="form-row"><label><input type="checkbox" name="activated" {{if .Offer.Activated}}checked{{end}}> Activated</label></div>
  <div class="btn-row">
    <button class="btn btn-primary" type="submit" data-action="save-offer">Save</button>
    <a class="btn btn-ghost" href="/cards/offers">Cancel</a>
  </div>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-selections.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Selections</h1>
  <p class="page-subtitle">Your chosen categories per user-selected card.</p>
</div>
{{if .Selections}}
  <div class="card-grid">
  {{range .Selections}}
  <article class="card" data-selection-id="{{.ID}}" {{if .Tier}}data-tier="{{intPtr .Tier}}"{{end}}>
    <div class="card-header">
      <h3 class="card-title" data-selection-category>{{.Category}}</h3>
      {{if .Tier}}<span class="badge badge-neutral" data-selection-tier="{{intPtr .Tier}}">tier {{intPtr .Tier}}</span>{{end}}
    </div>
    <p class="meta" data-selection-card>{{.CardName}} &middot; period {{.PeriodLabel}}{{if .Enrolled}} &middot; enrolled{{end}}</p>
    <div class="btn-row"><a class="btn btn-secondary btn-sm" href="/cards/selections/{{.ID}}/edit" data-action="edit">Edit</a></div>
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="selections">No selections yet.</p>
{{end}}
<div class="card form-section">
<h2 class="card-title">Save selection</h2>
<p class="meta">Fill the single category for a non-tiered card, or tier-1 and tier-2 for a tiered card.</p>
<form method="post" action="/cards/selections">
  {{template "cardrewards-card-select" .}}
  <div class="form-row"><label>Period label<br><input class="search-box" type="text" name="period_label" placeholder="2026-Q1" required aria-label="Period label"></label></div>
  <div class="form-row"><label>Category (non-tiered)<br><input class="search-box" type="text" name="category" aria-label="Category"></label></div>
  <div class="form-row"><label>Tier 1 category<br><input class="search-box" type="text" name="category_tier1" aria-label="Tier 1 category"></label></div>
  <div class="form-row"><label>Tier 2 category<br><input class="search-box" type="text" name="category_tier2" aria-label="Tier 2 category"></label></div>
  <button class="btn btn-primary" type="submit" data-action="save-selection">Save selection</button>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-selection-edit.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Edit Selection</h1>
</div>
<div class="card">
<form method="post" action="/cards/selections/{{.Selection.ID}}">
  <input type="hidden" name="user_card_id" value="{{.Selection.UserCardID}}">
  <div class="form-row"><label>Category<br><input class="search-box" type="text" name="category" value="{{.Selection.Category}}" required aria-label="Category"></label></div>
  <div class="form-row"><label>Tier (optional)<br><input class="search-box" type="number" name="tier" value="{{intPtr .Selection.Tier}}" min="1" aria-label="Tier"></label></div>
  <div class="form-row"><label>Period label<br><input class="search-box" type="text" name="period_label" value="{{.Selection.PeriodLabel}}" required aria-label="Period label"></label></div>
  <div class="form-row"><label><input type="checkbox" name="enrolled" {{if .Selection.Enrolled}}checked{{end}}> Enrolled</label></div>
  <div class="btn-row">
    <button class="btn btn-primary" type="submit" data-action="save-selection">Save</button>
    <a class="btn btn-ghost" href="/cards/selections">Cancel</a>
  </div>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-bonuses.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Sign-up Bonuses</h1>
  <p class="page-subtitle">Track spend progress toward each sign-up bonus.</p>
</div>
{{if .Bonuses}}
  <div class="card-grid">
  {{range .Bonuses}}
  <article class="card" data-bonus-id="{{.ID}}" data-met="{{.Met}}">
    <div class="card-header">
      <h3 class="card-title" data-bonus-description>{{.Description}}</h3>
      {{if .Met}}<span class="badge badge-success" data-bonus-met="true"><span aria-hidden="true">&#10003;</span> met</span>{{end}}
    </div>
    <p class="meta" data-bonus-card>{{.CardName}} &middot; {{.BonusType}}</p>
    <div class="progress" role="progressbar" aria-valuenow="{{pct .SpendProgressCents .SpendRequiredCents}}" aria-valuemin="0" aria-valuemax="100" aria-label="Spend progress">
      <div class="progress-fill{{if .Met}} progress-fill--success{{end}}" style="width:{{pct .SpendProgressCents .SpendRequiredCents}}%"></div>
    </div>
    <p class="meta" data-bonus-progress>progress {{cents .SpendProgressCents}} of {{centsPtr .SpendRequiredCents}} ({{pct .SpendProgressCents .SpendRequiredCents}}%)</p>
    {{if .Deadline}}<p class="meta">deadline {{date .Deadline}}</p>{{end}}
    <form class="form-row" method="post" action="/cards/bonuses/{{.ID}}/progress">
      <label>Update progress (cents) <input class="search-box" type="number" name="spend_progress_cents" value="{{.SpendProgressCents}}" min="0" aria-label="Spend progress cents"></label>
      <button class="btn btn-primary btn-sm" type="submit" data-action="update-progress">Save progress</button>
    </form>
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="bonuses">No sign-up bonuses yet.</p>
{{end}}
<div class="card form-section">
<h2 class="card-title">Add bonus</h2>
<form method="post" action="/cards/bonuses">
  {{template "cardrewards-card-select-required" .}}
  <div class="form-row"><label>Bonus type<br>
    <select name="bonus_type" class="search-box" aria-label="Bonus type">
      <option value="spend">spend</option>
      <option value="first_year_rate">first_year_rate</option>
    </select>
  </label></div>
  <div class="form-row"><label>Description<br><input class="search-box" type="text" name="description" required aria-label="Description"></label></div>
  <div class="form-row"><label>Spend required (cents, optional)<br><input class="search-box" type="number" name="spend_required_cents" min="0" aria-label="Spend required cents"></label></div>
  <div class="form-row"><label>Spend progress (cents)<br><input class="search-box" type="number" name="spend_progress_cents" value="0" min="0" aria-label="Spend progress cents"></label></div>
  <div class="form-row"><label>Reward (optional)<br><input class="search-box" type="text" name="reward_description" aria-label="Reward description"></label></div>
  <div class="form-row"><label>Deadline (optional, YYYY-MM-DD)<br><input class="search-box" type="date" name="deadline" aria-label="Deadline"></label></div>
  <button class="btn btn-primary" type="submit" data-action="create-bonus">Add bonus</button>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-categories.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Categories</h1>
  <p class="page-subtitle">Canonical categories, their equivalents, and starred priority.</p>
</div>
{{if .Aliases}}
  <div class="table-wrap">
    <table class="cr-table">
      <thead><tr><th>Canonical</th><th>Equivalents</th><th>Starred</th><th>Priority</th></tr></thead>
      <tbody>
      {{range .Aliases}}
        <tr data-category="{{.CanonicalCategory}}" data-starred="{{.Starred}}">
          <td data-category-name>{{.CanonicalCategory}}</td>
          <td data-category-equivalents>{{range .Equivalents}}<span class="chip">{{.}}</span>{{end}}</td>
          <td>{{if .Starred}}<span class="badge badge-starred" data-starred="true"><span aria-hidden="true">&#9733;</span> starred</span>{{else}}&mdash;{{end}}</td>
          <td>{{intPtr .Priority}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
  </div>
{{else}}
  <p class="empty-state" data-empty="categories">No categories yet.</p>
{{end}}
<div class="card form-section">
<h2 class="card-title">Add / update category</h2>
<p class="meta">Re-submitting an existing canonical name updates its equivalents, star, and priority.</p>
<form method="post" action="/cards/categories">
  <div class="form-row"><label>Canonical category<br><input class="search-box" type="text" name="canonical_category" required aria-label="Canonical category"></label></div>
  <div class="form-row"><label>Equivalents (comma-separated)<br><input class="search-box" type="text" name="equivalents" aria-label="Equivalents"></label></div>
  <div class="form-row"><label><input type="checkbox" name="starred"> Starred</label></div>
  <div class="form-row"><label>Priority (optional)<br><input class="search-box" type="number" name="priority" min="0" aria-label="Priority"></label></div>
  <button class="btn btn-primary" type="submit" data-action="save-category">Save category</button>
</form>
</div>
{{template "foot"}}
{{end}}

{{define "cardrewards-card-select"}}
<p><label>Card<br>
  <select name="user_card_id" class="search-box" aria-label="Card">
    <option value="">&mdash; General &mdash;</option>
    {{range .Cards}}<option value="{{.ID}}">{{.Name}}</option>{{end}}
  </select>
</label></p>
{{end}}

{{define "cardrewards-card-select-required"}}
<p><label>Card<br>
  <select name="user_card_id" class="search-box" required aria-label="Card">
    <option value="">&mdash; choose a card &mdash;</option>
    {{range .Cards}}<option value="{{.ID}}">{{.Name}}</option>{{end}}
  </select>
</label></p>
{{end}}
`
