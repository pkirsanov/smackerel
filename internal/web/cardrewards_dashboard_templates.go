// Spec 083 Scope 11 — card-rewards web templates for the Dashboard,
// Recommendations, Rotating-Verify, Report, and Admin pages.
//
// Parsed onto the same self-contained template set as the Scope 10 pages (see
// NewCardRewardsWebHandler), so they reuse the script-free "head"/"foot" chrome
// and the shared "cardrewards-nav" — no new inline <script>, no inline event
// handlers, design-token palette only (var(--…)). Every mutation is a plain
// Post/Redirect/Get <form>, so the pages stay strictly CSP-clean.
package web

const cardRewardsInsightsTemplates = `
{{define "cardrewards-dashboard.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title" data-dashboard>Card Rewards</h1>
  <p class="page-subtitle">{{.Period}}</p>
</div>

<div class="stats-grid">
  <a class="stat-card stat-card--link" href="/cards/recommendations">
    <span class="stat-value">{{len .Recommendations}}</span>
    <span class="stat-label">Recommendations</span>
  </a>
  <a class="stat-card stat-card--link" href="/cards/rotating">
    <span class="stat-value">{{len .ActiveRotating}}</span>
    <span class="stat-label">Active Rotating</span>
  </a>
  <a class="stat-card stat-card--link{{if .NeedsVerification}} stat-card--urgent{{end}}" href="/cards/rotating">
    <span class="stat-value">{{len .NeedsVerification}}{{if .NeedsVerification}} <span aria-hidden="true">&#9888;</span>{{end}}</span>
    <span class="stat-label">Needs Verify</span>
  </a>
  <a class="stat-card stat-card--link" href="/cards/selections">
    <span class="stat-value">{{len .PendingReEnroll}}</span>
    <span class="stat-label">Pending Re-enroll</span>
  </a>
</div>

{{if or .NeedsVerification .PendingReEnroll}}
<section class="alert alert-warning">
  <h2 class="card-title"><span aria-hidden="true">&#9888;</span> Pending Actions</h2>
  {{range .NeedsVerification}}
  <article class="card" data-needs-verification data-catalog="{{.CardCatalogID}}">
    <div class="card-header">
      <h3 class="card-title">{{.CatalogName}} &mdash; {{.PeriodLabel}}</h3>
      <span class="badge badge-warning" data-badge="needs-verification"><span aria-hidden="true">&#9888;</span> needs verification</span>
    </div>
    <div class="btn-row"><a class="btn btn-secondary btn-sm" href="/cards/rotating">Verify</a></div>
  </article>
  {{end}}
  {{range .PendingReEnroll}}
  <article class="card" data-pending-reenroll>
    <h3 class="card-title">{{.CatalogName}} &mdash; re-enroll</h3>
    <p class="summary">{{.Category}} &middot; {{.PeriodLabel}}</p>
  </article>
  {{end}}
</section>
{{else}}
  <p class="empty-state" data-empty="pending">No pending actions.</p>
{{end}}

<h2 class="card-title">This month&rsquo;s recommendations</h2>
{{if .Recommendations}}
  <div class="card-grid">
  {{range .Recommendations}}
  <article class="card" data-rec-row data-rec-category="{{.Category}}">
    <div class="card-header">
      <h3 class="card-title">{{.Category}}</h3>
      {{if .StarredOverride}}<span class="badge badge-starred" data-rec-starred="true"><span aria-hidden="true">&starf;</span> starred</span>{{end}}
    </div>
    <p class="summary">Best card: <strong data-rec-card>{{.CardName}}</strong></p>
    {{if .Reason}}<p class="meta" data-rec-reason>{{.Reason}}</p>{{end}}
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="recommendations">No recommendations yet. <a href="/cards/recommendations">Open recommendations</a>.</p>
{{end}}

<h2 class="card-title">Active rotating categories</h2>
{{if .ActiveRotating}}
  <div class="card-grid">
  {{range .ActiveRotating}}
  <article class="card" data-active-rotating data-catalog="{{.CardCatalogID}}">
    <h3 class="card-title">{{.CatalogName}} &mdash; {{.PeriodLabel}}</h3>
    <p class="summary">{{range .Categories}}<span class="chip">{{.}}</span>{{end}}</p>
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="active-rotating">No active rotating categories.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-recommendations.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Recommendations</h1>
  <p class="page-subtitle">{{.Period}}</p>
</div>

<form class="form-section" method="post" action="/cards/recommendations/regenerate">
  <input type="hidden" name="period_label" value="{{.Period}}">
  <button class="btn btn-primary" type="submit" data-action="regenerate">Regenerate from optimizer</button>
</form>

<div class="card form-section">
<h2 class="card-title">Add / edit a recommendation</h2>
<form method="post" action="/cards/recommendations">
  <input type="hidden" name="period_label" value="{{.Period}}">
  <div class="form-row"><label>Category<br><input class="search-box" type="text" name="category" required aria-label="Category"></label></div>
  <div class="form-row"><label>Recommended card<br>
    <select name="recommended_user_card_id" class="search-box" aria-label="Recommended card">
      <option value="">&mdash; none &mdash;</option>
      {{range .Cards}}<option value="{{.ID}}">{{.Name}}</option>{{end}}
    </select></label></div>
  <div class="form-row"><label>Rate<br><input class="search-box" type="text" name="rate" aria-label="Rate"></label></div>
  <div class="form-row"><label>Reason<br><input class="search-box" type="text" name="reason" aria-label="Reason"></label></div>
  <button class="btn btn-primary" type="submit" data-action="save-recommendation">Save</button>
</form>
</div>

<h2 class="card-title">This period</h2>
{{if .Recommendations}}
  <div class="card-grid">
  {{range .Recommendations}}
  <article class="card" data-rec-row data-rec-category="{{.Category}}" data-rec-starred="{{.StarredOverride}}">
    <div class="card-header">
      <h3 class="card-title">{{.Category}}</h3>
      {{if .StarredOverride}}<span class="badge badge-starred" data-rec-starred-badge="true"><span aria-hidden="true">&starf;</span> starred</span>{{end}}
    </div>
    <p class="summary">Card: <strong data-rec-card data-rec-card-id="{{deref .RecommendedUserCardID}}">{{.CardName}}</strong> &middot; rate {{printf "%.1f" .Rate}}</p>
    {{if .Reason}}<p class="meta" data-rec-reason>{{.Reason}}</p>{{end}}
    <form class="btn-row" method="post" action="/cards/recommendations/star">
      <input type="hidden" name="period_label" value="{{$.Period}}">
      <input type="hidden" name="category" value="{{.Category}}">
      {{if .StarredOverride}}<button class="btn btn-ghost btn-sm" type="submit" data-action="unstar">Unstar</button>{{else}}<input type="hidden" name="starred" value="on"><button class="btn btn-secondary btn-sm" type="submit" data-action="star">Star</button>{{end}}
    </form>
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="recommendations">No recommendations for {{.Period}}.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-rotating.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Rotating Categories</h1>
  <p class="page-subtitle">Reconciled rotating categories, confidence, and verification status.</p>
</div>
{{if .Rows}}
  <div class="card-grid">
  {{range .Rows}}
  <article class="card" data-rotating-row data-rotating-id="{{.ID}}" data-needs-verification="{{.NeedsVerification}}" data-manual-override="{{.ManualOverride}}" data-confidence="{{printf "%.2f" .Confidence}}">
    <div class="card-header">
      <h3 class="card-title">{{.CatalogName}} &mdash; {{.PeriodLabel}}</h3>
      {{if .NeedsVerification}}<span class="badge badge-warning" data-badge="needs-verification"><span aria-hidden="true">&#9888;</span> needs verification</span>{{end}}
      {{if .ManualOverride}}<span class="badge badge-success" data-badge="manual-override"><span aria-hidden="true">&#10003;</span> manually verified</span>{{end}}
    </div>
    <p class="summary" data-rotating-categories>{{range .Categories}}<span class="chip">{{.}}</span>{{end}}</p>
    <div class="progress" role="progressbar" aria-valuenow="{{confpct .Confidence}}" aria-valuemin="0" aria-valuemax="100" aria-label="Confidence">
      <div class="progress-fill{{if ge (confpct .Confidence) 70}} progress-fill--success{{else if ge (confpct .Confidence) 40}} progress-fill--warning{{else}} progress-fill--danger{{end}}" style="width:{{confpct .Confidence}}%"></div>
    </div>
    <p class="meta"><span data-confidence-badge>confidence {{confpct .Confidence}}%</span></p>
    {{if .Citations}}
    <p class="meta">Sources:</p>
    <ul>
      {{range .Citations}}
      <li data-citation data-citation-source="{{.SourceName}}">{{.SourceName}} &middot; {{.SourceURL}} &middot; {{csv .Categories}} &middot; conf {{confpct .Confidence}}%</li>
      {{end}}
    </ul>
    {{else}}
    <p class="meta" data-citation-empty>No source citations.</p>
    {{end}}
    <form class="form-section" method="post" action="/cards/rotating/{{.ID}}/verify">
      <div class="form-row"><input class="search-box" type="text" name="categories" value="{{csv .Categories}}" aria-label="Verified categories (comma separated)"></div>
      <button class="btn btn-primary btn-sm" type="submit" data-action="verify">Verify / override</button>
    </form>
  </article>
  {{end}}
  </div>
{{else}}
  <p class="empty-state" data-empty="rotating">No rotating categories yet.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-report.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Optimization Report</h1>
  <p class="page-subtitle">{{.Report.Period}}</p>
</div>
{{if .Report.Categories}}
<div class="table-wrap">
<table class="cr-table">
  <thead><tr><th>Category</th><th>Best card</th><th>Rate</th><th>Reason</th></tr></thead>
  <tbody>
  {{range .Report.Categories}}
    <tr data-report-row data-report-category="{{.Category}}">
      <td>{{.Category}}</td>
      <td data-report-card>{{if .CardName}}<strong>{{.CardName}}</strong>{{else}}&mdash;{{end}}</td>
      <td>{{printf "%.1f" .Rate}} {{.RateType}}</td>
      <td data-report-reason>{{.Reason}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
  <p class="empty-state" data-empty="report">No optimization data for {{.Report.Period}}. Add cards and tracked categories first.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-admin.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<div class="page-header">
  <h1 class="page-title">Admin</h1>
  <p class="page-subtitle">Manual pipeline triggers and run history.</p>
</div>
<h2 class="card-title">Manual triggers</h2>
{{if .TriggersEnabled}}
<div class="btn-row">
  <form method="post" action="/cards/admin/scrape">
    <button class="btn btn-secondary" type="submit" data-action="scrape-now">Scrape now</button>
  </form>
  <form method="post" action="/cards/admin/sync-calendar">
    <button class="btn btn-secondary" type="submit" data-action="sync-calendar-now">Sync calendar now</button>
  </form>
</div>
{{else}}
<p class="meta" data-triggers="disabled">Manual triggers are not configured on this instance.</p>
{{end}}

<h2 class="card-title">Run history</h2>
{{if .Runs}}
<div class="table-wrap">
<table class="cr-table">
  <thead><tr><th>Type</th><th>Trigger</th><th>Status</th><th>Sources</th><th>Categories</th><th>Events written</th><th>When</th></tr></thead>
  <tbody>
  {{range .Runs}}
    <tr data-run-row data-run-id="{{.ID}}" data-run-type="{{.RunType}}" data-run-trigger="{{.Trigger}}" data-run-status="{{.Status}}" data-events-written="{{.EventsWritten}}">
      <td>{{.RunType}}</td>
      <td>{{.Trigger}}</td>
      <td>{{if eq .Status "success"}}<span class="badge badge-success">{{.Status}}</span>{{else if eq .Status "failed"}}<span class="badge badge-danger">{{.Status}}</span>{{else if eq .Status "partial"}}<span class="badge badge-warning">{{.Status}}</span>{{else}}<span class="badge badge-neutral">{{.Status}}</span>{{end}}</td>
      <td>{{.SourcesSucceeded}}/{{.SourcesAttempted}}</td>
      <td>{{.CategoriesExtracted}}</td>
      <td data-events-written-cell>{{.EventsWritten}}</td>
      <td class="meta">{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
  <p class="empty-state" data-empty="runs">No runs recorded yet.</p>
{{end}}
{{template "foot"}}
{{end}}
`
