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
:root { --bg:#fafaf8; --fg:#1a1a18; --muted:#6b6b68; --border:#d4d4d0; --accent:#2d2d2b; --card-bg:#ffffff; --success:#2d5a2d; --warning:#8b6914; --error:#8b1414; --radius:6px; --shadow:0 1px 3px rgba(0,0,0,0.08); }
@media (prefers-color-scheme: dark) { :root { --bg:#1a1a18; --fg:#e8e8e4; --muted:#8b8b88; --border:#3a3a38; --accent:#d4d4d0; --card-bg:#242422; --success:#5a8b5a; --warning:#c49b1f; --error:#c44848; --shadow:0 1px 3px rgba(0,0,0,0.3); } }
* { margin:0; padding:0; box-sizing:border-box; }
body { font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif; background:var(--bg); color:var(--fg); line-height:1.6; max-width:820px; margin:0 auto; padding:1rem; }
nav { display:flex; gap:1.25rem; padding:1rem 0; border-bottom:1px solid var(--border); margin-bottom:1.5rem; flex-wrap:wrap; }
nav a { color:var(--muted); text-decoration:none; font-size:0.9rem; }
nav a:hover { color:var(--fg); }
h1 { font-size:1.5rem; font-weight:600; margin-bottom:1rem; }
h2 { font-size:1.15rem; font-weight:600; margin:1.5rem 0 0.75rem; }
h3 { font-size:1rem; margin-bottom:0.25rem; }
a { color:var(--accent); }
.search-box { width:100%; padding:0.6rem; border:1px solid var(--border); border-radius:var(--radius); font-size:1rem; background:var(--card-bg); color:var(--fg); }
.search-box:focus { outline:2px solid var(--accent); border-color:transparent; }
.card { background:var(--card-bg); border:1px solid var(--border); border-radius:var(--radius); padding:1rem; margin:0.75rem 0; box-shadow:var(--shadow); }
.card .summary { font-size:0.9rem; margin-top:0.5rem; }
.meta { font-size:0.8rem; color:var(--muted); }
.type-badge { display:inline-block; font-size:0.75rem; padding:0.1rem 0.4rem; border:1px solid var(--border); border-radius:3px; color:var(--muted); }
.tag { display:inline-block; font-size:0.75rem; padding:0.1rem 0.4rem; background:var(--bg); border:1px solid var(--border); border-radius:3px; margin:0.1rem; }
.empty { text-align:center; padding:2rem; color:var(--muted); }
button { font:inherit; padding:0.35rem 0.7rem; border:1px solid var(--border); border-radius:var(--radius); background:var(--card-bg); color:var(--fg); cursor:pointer; }
button:hover { border-color:var(--accent); }
label { font-size:0.9rem; }
input, select, textarea { color:var(--fg); }
table { border-collapse:collapse; width:100%; background:var(--card-bg); }
th, td { border:1px solid var(--border); padding:0.4rem 0.6rem; text-align:left; }
th { color:var(--muted); font-weight:600; }
form p { margin:0.5rem 0; }
</style>
</head>
<body>
{{end}}

{{define "foot"}}</body></html>{{end}}

{{define "cardrewards-nav"}}
<nav aria-label="Card rewards" style="border-top:1px solid var(--border);margin-top:-1rem">
  <a href="/cards">Dashboard</a>
  <a href="/cards/wallet">My Cards</a>
  <a href="/cards/offers">Offers</a>
  <a href="/cards/selections">Selections</a>
  <a href="/cards/bonuses">Sign-up Bonuses</a>
  <a href="/cards/categories">Categories</a>
  <a href="/cards/recommendations">Recommendations</a>
  <a href="/cards/rotating">Rotating</a>
  <a href="/cards/report">Report</a>
  <a href="/cards/admin">Admin</a>
</nav>
{{end}}

{{define "cardrewards-wallet.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>My Cards</h1>
<p class="meta"><a href="/cards/wallet/add">+ Add via discovery</a> &middot; <a href="/cards/wallet/add-custom">+ Add custom card</a></p>
{{if .Cards}}
  {{range .Cards}}
  <article class="card" data-card-id="{{.ID}}" data-active="{{.Active}}">
    <h3 data-card-name>{{if deref .Nickname}}{{deref .Nickname}}{{else}}{{.CatalogName}}{{end}}</h3>
    <p class="meta"><span class="type-badge" data-card-type="{{.CardType}}">{{.CardType}}</span> &middot; {{.CatalogName}}</p>
    {{if deref .Note}}<p class="summary" data-card-note>{{deref .Note}}</p>{{end}}
    <p>{{if .Active}}<span class="tag" data-card-status="active">Active</span>{{else}}<span class="tag" data-card-status="inactive">Inactive</span>{{end}}</p>
    <p class="meta">
      <a href="/cards/wallet/{{.ID}}/edit" data-action="edit">Edit</a>
      <form method="post" action="/cards/wallet/{{.ID}}/toggle" style="display:inline">
        <button type="submit" data-action="toggle">{{if .Active}}Deactivate{{else}}Activate{{end}}</button>
      </form>
      <form method="post" action="/cards/wallet/{{.ID}}/delete" style="display:inline">
        <button type="submit" data-action="delete">Remove</button>
      </form>
    </p>
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="wallet">No cards yet. Add one to get started.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-wallet-add.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Add Card</h1>
<form method="get" action="/cards/wallet/add">
  <input class="search-box" type="text" name="q" value="{{.Query}}" placeholder="Search the card catalog (e.g. custom cash)" aria-label="Search catalog">
  <button type="submit">Search</button>
</form>
{{if .Query}}
  <h2>Candidates</h2>
  {{if .Candidates}}
    {{range .Candidates}}
    <article class="card" data-candidate-id="{{.CardID}}">
      <h3 data-candidate-name>{{.Name}}</h3>
      <p class="meta">match: {{.MatchType}} &middot; score {{printf "%.2f" .Score}}</p>
      <form method="post" action="/cards/wallet">
        <input type="hidden" name="catalog_id" value="{{.CardID}}">
        <input type="text" name="nickname" placeholder="Nickname (optional)" aria-label="Nickname">
        <button type="submit" data-action="confirm-add">Add to wallet</button>
      </form>
    </article>
    {{end}}
  {{else}}
    <p class="empty" data-empty="candidates">No catalog matches for &ldquo;{{.Query}}&rdquo;.</p>
  {{end}}
{{end}}
<p class="meta"><a href="/cards/wallet">&larr; Back to wallet</a></p>
{{template "foot"}}
{{end}}

{{define "cardrewards-wallet-add-custom.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Add Custom Card</h1>
<form method="post" action="/cards/wallet/custom">
  <p><label>Name<br><input class="search-box" type="text" name="name" required aria-label="Name"></label></p>
  <p><label>Issuer<br><input class="search-box" type="text" name="issuer" required aria-label="Issuer"></label></p>
  <p><label>Card type<br>
    <select name="card_type" class="search-box" aria-label="Card type">
      <option value="fixed">fixed</option>
      <option value="rotating">rotating</option>
      <option value="user-selected">user-selected</option>
    </select>
  </label></p>
  <p><label>Annual fee (cents)<br><input class="search-box" type="number" name="annual_fee_cents" value="0" min="0" aria-label="Annual fee cents"></label></p>
  <p><label>Nickname (optional)<br><input class="search-box" type="text" name="nickname" aria-label="Nickname"></label></p>
  <p><label>Note (optional)<br><textarea class="search-box" name="note" aria-label="Note"></textarea></label></p>
  <button type="submit" data-action="create-custom">Create card</button>
  <a href="/cards/wallet">Cancel</a>
</form>
{{template "foot"}}
{{end}}

{{define "cardrewards-wallet-edit.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Edit Card</h1>
<form method="post" action="/cards/wallet/{{.Card.ID}}">
  <p class="meta" data-card-catalog>{{.Card.CatalogName}}</p>
  <p><label>Nickname<br><input class="search-box" type="text" name="nickname" value="{{deref .Card.Nickname}}" aria-label="Nickname"></label></p>
  <p><label>Note<br><textarea class="search-box" name="note" aria-label="Note">{{deref .Card.Note}}</textarea></label></p>
  <button type="submit" data-action="save-card">Save</button>
  <a href="/cards/wallet">Cancel</a>
</form>
{{template "foot"}}
{{end}}

{{define "cardrewards-offers.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Offers</h1>
{{if .Offers}}
  {{range .Offers}}
  <article class="card" data-offer-id="{{.ID}}" data-activated="{{.Activated}}">
    <h3 data-offer-title>{{.Title}}</h3>
    <p class="meta" data-offer-card>{{.CardName}} &middot; {{.Category}} &middot; {{printf "%.2f" .Rate}} {{.RateType}}</p>
    {{if deref .SharedLimitGroup}}<p><span class="tag" data-shared-limit-group="{{deref .SharedLimitGroup}}">shared limit: {{deref .SharedLimitGroup}}</span></p>{{end}}
    <p class="meta">limit {{centsPtr .LimitCents}}{{if .ActivationRequired}} &middot; activation required{{end}} &middot; {{if .Activated}}<span class="tag" data-offer-status="activated">activated</span>{{else}}<span class="tag" data-offer-status="not-activated">not activated</span>{{end}}</p>
    <p class="meta">
      <a href="/cards/offers/{{.ID}}/edit" data-action="edit">Edit</a>
      <form method="post" action="/cards/offers/{{.ID}}/toggle" style="display:inline"><button type="submit" data-action="toggle">{{if .Activated}}Deactivate{{else}}Activate{{end}}</button></form>
      <form method="post" action="/cards/offers/{{.ID}}/delete" style="display:inline"><button type="submit" data-action="delete">Remove</button></form>
    </p>
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="offers">No offers yet.</p>
{{end}}
<h2>Add offer</h2>
<form method="post" action="/cards/offers">
  {{template "cardrewards-card-select" .}}
  <p><label>Title<br><input class="search-box" type="text" name="title" required aria-label="Title"></label></p>
  <p><label>Category<br><input class="search-box" type="text" name="category" required aria-label="Category"></label></p>
  <p><label>Rate<br><input class="search-box" type="number" step="0.01" name="rate" value="0" aria-label="Rate"></label></p>
  <p><label>Rate type<br>
    <select name="rate_type" class="search-box" aria-label="Rate type">
      <option value="percent">percent</option>
      <option value="multiplier">multiplier</option>
      <option value="points">points</option>
    </select>
  </label></p>
  <p><label>Shared limit group (optional)<br><input class="search-box" type="text" name="shared_limit_group" aria-label="Shared limit group"></label></p>
  <p><label>Limit (cents, optional)<br><input class="search-box" type="number" name="limit_cents" min="0" aria-label="Limit cents"></label></p>
  <p><label><input type="checkbox" name="activation_required"> Activation required</label></p>
  <button type="submit" data-action="create-offer">Add offer</button>
</form>
{{template "foot"}}
{{end}}

{{define "cardrewards-offer-edit.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Edit Offer</h1>
<form method="post" action="/cards/offers/{{.Offer.ID}}">
  <p><label>Card<br>
    <select name="user_card_id" class="search-box" aria-label="Card">
      <option value="">&mdash; General &mdash;</option>
      {{$sel := deref .Offer.UserCardID}}{{range .Cards}}<option value="{{.ID}}" {{if eq .ID $sel}}selected{{end}}>{{.Name}}</option>{{end}}
    </select>
  </label></p>
  <p><label>Title<br><input class="search-box" type="text" name="title" value="{{.Offer.Title}}" required aria-label="Title"></label></p>
  <p><label>Category<br><input class="search-box" type="text" name="category" value="{{.Offer.Category}}" required aria-label="Category"></label></p>
  <p><label>Rate<br><input class="search-box" type="number" step="0.01" name="rate" value="{{printf "%.2f" .Offer.Rate}}" aria-label="Rate"></label></p>
  <p><label>Rate type<br>
    <select name="rate_type" class="search-box" aria-label="Rate type">
      <option value="percent" {{if eq .Offer.RateType "percent"}}selected{{end}}>percent</option>
      <option value="multiplier" {{if eq .Offer.RateType "multiplier"}}selected{{end}}>multiplier</option>
      <option value="points" {{if eq .Offer.RateType "points"}}selected{{end}}>points</option>
    </select>
  </label></p>
  <p><label>Shared limit group (optional)<br><input class="search-box" type="text" name="shared_limit_group" value="{{deref .Offer.SharedLimitGroup}}" aria-label="Shared limit group"></label></p>
  <p><label>Limit (cents, optional)<br><input class="search-box" type="number" name="limit_cents" value="{{intPtr .Offer.LimitCents}}" min="0" aria-label="Limit cents"></label></p>
  <p><label><input type="checkbox" name="activation_required" {{if .Offer.ActivationRequired}}checked{{end}}> Activation required</label></p>
  <p><label><input type="checkbox" name="activated" {{if .Offer.Activated}}checked{{end}}> Activated</label></p>
  <button type="submit" data-action="save-offer">Save</button>
  <a href="/cards/offers">Cancel</a>
</form>
{{template "foot"}}
{{end}}

{{define "cardrewards-selections.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Selections</h1>
{{if .Selections}}
  {{range .Selections}}
  <article class="card" data-selection-id="{{.ID}}" {{if .Tier}}data-tier="{{intPtr .Tier}}"{{end}}>
    <h3 data-selection-category>{{.Category}}</h3>
    <p class="meta" data-selection-card>{{.CardName}} &middot; period {{.PeriodLabel}}{{if .Tier}} &middot; <span class="tag" data-selection-tier="{{intPtr .Tier}}">tier {{intPtr .Tier}}</span>{{end}}{{if .Enrolled}} &middot; enrolled{{end}}</p>
    <p class="meta"><a href="/cards/selections/{{.ID}}/edit" data-action="edit">Edit</a></p>
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="selections">No selections yet.</p>
{{end}}
<h2>Save selection</h2>
<p class="meta">Fill the single category for a non-tiered card, or tier-1 and tier-2 for a tiered card.</p>
<form method="post" action="/cards/selections">
  {{template "cardrewards-card-select" .}}
  <p><label>Period label<br><input class="search-box" type="text" name="period_label" placeholder="2026-Q1" required aria-label="Period label"></label></p>
  <p><label>Category (non-tiered)<br><input class="search-box" type="text" name="category" aria-label="Category"></label></p>
  <p><label>Tier 1 category<br><input class="search-box" type="text" name="category_tier1" aria-label="Tier 1 category"></label></p>
  <p><label>Tier 2 category<br><input class="search-box" type="text" name="category_tier2" aria-label="Tier 2 category"></label></p>
  <button type="submit" data-action="save-selection">Save selection</button>
</form>
{{template "foot"}}
{{end}}

{{define "cardrewards-selection-edit.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Edit Selection</h1>
<form method="post" action="/cards/selections/{{.Selection.ID}}">
  <input type="hidden" name="user_card_id" value="{{.Selection.UserCardID}}">
  <p><label>Category<br><input class="search-box" type="text" name="category" value="{{.Selection.Category}}" required aria-label="Category"></label></p>
  <p><label>Tier (optional)<br><input class="search-box" type="number" name="tier" value="{{intPtr .Selection.Tier}}" min="1" aria-label="Tier"></label></p>
  <p><label>Period label<br><input class="search-box" type="text" name="period_label" value="{{.Selection.PeriodLabel}}" required aria-label="Period label"></label></p>
  <p><label><input type="checkbox" name="enrolled" {{if .Selection.Enrolled}}checked{{end}}> Enrolled</label></p>
  <button type="submit" data-action="save-selection">Save</button>
  <a href="/cards/selections">Cancel</a>
</form>
{{template "foot"}}
{{end}}

{{define "cardrewards-bonuses.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Sign-up Bonuses</h1>
{{if .Bonuses}}
  {{range .Bonuses}}
  <article class="card" data-bonus-id="{{.ID}}" data-met="{{.Met}}">
    <h3 data-bonus-description>{{.Description}}</h3>
    <p class="meta" data-bonus-card>{{.CardName}} &middot; {{.BonusType}}</p>
    <p class="meta" data-bonus-progress>progress {{cents .SpendProgressCents}} of {{centsPtr .SpendRequiredCents}} ({{pct .SpendProgressCents .SpendRequiredCents}}%){{if .Met}} &middot; <span class="tag" data-bonus-met="true">met</span>{{end}}</p>
    {{if .Deadline}}<p class="meta">deadline {{date .Deadline}}</p>{{end}}
    <form method="post" action="/cards/bonuses/{{.ID}}/progress">
      <label>Update progress (cents) <input type="number" name="spend_progress_cents" value="{{.SpendProgressCents}}" min="0" aria-label="Spend progress cents"></label>
      <button type="submit" data-action="update-progress">Save progress</button>
    </form>
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="bonuses">No sign-up bonuses yet.</p>
{{end}}
<h2>Add bonus</h2>
<form method="post" action="/cards/bonuses">
  {{template "cardrewards-card-select-required" .}}
  <p><label>Bonus type<br>
    <select name="bonus_type" class="search-box" aria-label="Bonus type">
      <option value="spend">spend</option>
      <option value="first_year_rate">first_year_rate</option>
    </select>
  </label></p>
  <p><label>Description<br><input class="search-box" type="text" name="description" required aria-label="Description"></label></p>
  <p><label>Spend required (cents, optional)<br><input class="search-box" type="number" name="spend_required_cents" min="0" aria-label="Spend required cents"></label></p>
  <p><label>Spend progress (cents)<br><input class="search-box" type="number" name="spend_progress_cents" value="0" min="0" aria-label="Spend progress cents"></label></p>
  <p><label>Reward (optional)<br><input class="search-box" type="text" name="reward_description" aria-label="Reward description"></label></p>
  <p><label>Deadline (optional, YYYY-MM-DD)<br><input class="search-box" type="date" name="deadline" aria-label="Deadline"></label></p>
  <button type="submit" data-action="create-bonus">Add bonus</button>
</form>
{{template "foot"}}
{{end}}

{{define "cardrewards-categories.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Categories</h1>
{{if .Aliases}}
  <table>
    <thead><tr><th>Canonical</th><th>Equivalents</th><th>Starred</th><th>Priority</th></tr></thead>
    <tbody>
    {{range .Aliases}}
      <tr data-category="{{.CanonicalCategory}}" data-starred="{{.Starred}}">
        <td data-category-name>{{.CanonicalCategory}}</td>
        <td data-category-equivalents>{{csv .Equivalents}}</td>
        <td>{{if .Starred}}<span class="tag" data-starred="true">&#9733; starred</span>{{else}}&mdash;{{end}}</td>
        <td>{{intPtr .Priority}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
{{else}}
  <p class="empty" data-empty="categories">No categories yet.</p>
{{end}}
<h2>Add / update category</h2>
<p class="meta">Re-submitting an existing canonical name updates its equivalents, star, and priority.</p>
<form method="post" action="/cards/categories">
  <p><label>Canonical category<br><input class="search-box" type="text" name="canonical_category" required aria-label="Canonical category"></label></p>
  <p><label>Equivalents (comma-separated)<br><input class="search-box" type="text" name="equivalents" aria-label="Equivalents"></label></p>
  <p><label><input type="checkbox" name="starred"> Starred</label></p>
  <p><label>Priority (optional)<br><input class="search-box" type="number" name="priority" min="0" aria-label="Priority"></label></p>
  <button type="submit" data-action="save-category">Save category</button>
</form>
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
