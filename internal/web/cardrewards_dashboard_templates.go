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
<h1 data-dashboard>Card Rewards &mdash; {{.Period}}</h1>

<h2>This month&rsquo;s recommendations</h2>
{{if .Recommendations}}
  {{range .Recommendations}}
  <article class="card" data-rec-row data-rec-category="{{.Category}}">
    <h3>{{.Category}}</h3>
    <p class="summary">Best card: <strong data-rec-card>{{.CardName}}</strong>{{if .StarredOverride}} <span class="tag" data-rec-starred="true">&starf; starred</span>{{end}}</p>
    {{if .Reason}}<p class="meta" data-rec-reason>{{.Reason}}</p>{{end}}
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="recommendations">No recommendations yet. <a href="/cards/recommendations">Open recommendations</a>.</p>
{{end}}

<h2>Active rotating categories</h2>
{{if .ActiveRotating}}
  {{range .ActiveRotating}}
  <article class="card" data-active-rotating data-catalog="{{.CardCatalogID}}">
    <h3>{{.CatalogName}} &mdash; {{.PeriodLabel}}</h3>
    <p class="summary">{{csv .Categories}}</p>
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="active-rotating">No active rotating categories.</p>
{{end}}

<h2>Pending actions</h2>
{{if .NeedsVerification}}
  {{range .NeedsVerification}}
  <article class="card" data-needs-verification data-catalog="{{.CardCatalogID}}">
    <h3>{{.CatalogName}} &mdash; {{.PeriodLabel}}</h3>
    <p><span class="tag" data-badge="needs-verification">needs verification</span> &middot; <a href="/cards/rotating">verify</a></p>
  </article>
  {{end}}
{{end}}
{{if .PendingReEnroll}}
  {{range .PendingReEnroll}}
  <article class="card" data-pending-reenroll>
    <h3>{{.CatalogName}} &mdash; re-enroll</h3>
    <p class="summary">{{.Category}} &middot; {{.PeriodLabel}}</p>
  </article>
  {{end}}
{{end}}
{{if and (not .NeedsVerification) (not .PendingReEnroll)}}
  <p class="empty" data-empty="pending">No pending actions.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-recommendations.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Recommendations &mdash; {{.Period}}</h1>

<form method="post" action="/cards/recommendations/regenerate" style="display:inline">
  <input type="hidden" name="period_label" value="{{.Period}}">
  <button type="submit" data-action="regenerate">Regenerate from optimizer</button>
</form>

<h2>Add / edit a recommendation</h2>
<form method="post" action="/cards/recommendations">
  <input type="hidden" name="period_label" value="{{.Period}}">
  <p><label>Category<br><input class="search-box" type="text" name="category" required aria-label="Category"></label></p>
  <p><label>Recommended card<br>
    <select name="recommended_user_card_id" class="search-box" aria-label="Recommended card">
      <option value="">&mdash; none &mdash;</option>
      {{range .Cards}}<option value="{{.ID}}">{{.Name}}</option>{{end}}
    </select></label></p>
  <p><label>Rate<br><input class="search-box" type="text" name="rate" aria-label="Rate"></label></p>
  <p><label>Reason<br><input class="search-box" type="text" name="reason" aria-label="Reason"></label></p>
  <button type="submit" data-action="save-recommendation">Save</button>
</form>

<h2>This period</h2>
{{if .Recommendations}}
  {{range .Recommendations}}
  <article class="card" data-rec-row data-rec-category="{{.Category}}" data-rec-starred="{{.StarredOverride}}">
    <h3>{{.Category}}</h3>
    <p class="summary">Card: <strong data-rec-card data-rec-card-id="{{deref .RecommendedUserCardID}}">{{.CardName}}</strong> &middot; rate {{printf "%.1f" .Rate}}</p>
    {{if .Reason}}<p class="meta" data-rec-reason>{{.Reason}}</p>{{end}}
    <p>
      {{if .StarredOverride}}<span class="tag" data-rec-starred-badge="true">&starf; starred</span>{{end}}
      <form method="post" action="/cards/recommendations/star" style="display:inline">
        <input type="hidden" name="period_label" value="{{$.Period}}">
        <input type="hidden" name="category" value="{{.Category}}">
        {{if .StarredOverride}}<button type="submit" data-action="unstar">Unstar</button>{{else}}<input type="hidden" name="starred" value="on"><button type="submit" data-action="star">Star</button>{{end}}
      </form>
    </p>
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="recommendations">No recommendations for {{.Period}}.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-rotating.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Rotating Categories</h1>
{{if .Rows}}
  {{range .Rows}}
  <article class="card" data-rotating-row data-rotating-id="{{.ID}}" data-needs-verification="{{.NeedsVerification}}" data-manual-override="{{.ManualOverride}}" data-confidence="{{printf "%.2f" .Confidence}}">
    <h3>{{.CatalogName}} &mdash; {{.PeriodLabel}}</h3>
    <p class="summary" data-rotating-categories>{{csv .Categories}}</p>
    <p class="meta">
      <span class="tag" data-confidence-badge>confidence {{confpct .Confidence}}%</span>
      {{if .NeedsVerification}}<span class="tag" data-badge="needs-verification">needs verification</span>{{end}}
      {{if .ManualOverride}}<span class="tag" data-badge="manual-override">manually verified</span>{{end}}
    </p>
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
    <form method="post" action="/cards/rotating/{{.ID}}/verify">
      <input class="search-box" type="text" name="categories" value="{{csv .Categories}}" aria-label="Verified categories (comma separated)">
      <button type="submit" data-action="verify">Verify / override</button>
    </form>
  </article>
  {{end}}
{{else}}
  <p class="empty" data-empty="rotating">No rotating categories yet.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-report.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Optimization Report &mdash; {{.Report.Period}}</h1>
{{if .Report.Categories}}
<table>
  <thead><tr><th>Category</th><th>Best card</th><th>Rate</th><th>Reason</th></tr></thead>
  <tbody>
  {{range .Report.Categories}}
    <tr data-report-row data-report-category="{{.Category}}">
      <td>{{.Category}}</td>
      <td data-report-card>{{if .CardName}}{{.CardName}}{{else}}&mdash;{{end}}</td>
      <td>{{printf "%.1f" .Rate}} {{.RateType}}</td>
      <td data-report-reason>{{.Reason}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
{{else}}
  <p class="empty" data-empty="report">No optimization data for {{.Report.Period}}. Add cards and tracked categories first.</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "cardrewards-admin.html"}}
{{template "head" .}}
{{template "cardrewards-nav" .}}
<h1>Admin</h1>
<h2>Manual triggers</h2>
{{if .TriggersEnabled}}
<p>
  <form method="post" action="/cards/admin/scrape" style="display:inline">
    <button type="submit" data-action="scrape-now">Scrape now</button>
  </form>
  <form method="post" action="/cards/admin/sync-calendar" style="display:inline">
    <button type="submit" data-action="sync-calendar-now">Sync calendar now</button>
  </form>
</p>
{{else}}
<p class="meta" data-triggers="disabled">Manual triggers are not configured on this instance.</p>
{{end}}

<h2>Run history</h2>
{{if .Runs}}
<table>
  <thead><tr><th>Type</th><th>Trigger</th><th>Status</th><th>Sources</th><th>Categories</th><th>Events written</th><th>When</th></tr></thead>
  <tbody>
  {{range .Runs}}
    <tr data-run-row data-run-id="{{.ID}}" data-run-type="{{.RunType}}" data-run-trigger="{{.Trigger}}" data-run-status="{{.Status}}" data-events-written="{{.EventsWritten}}">
      <td>{{.RunType}}</td>
      <td>{{.Trigger}}</td>
      <td>{{.Status}}</td>
      <td>{{.SourcesSucceeded}}/{{.SourcesAttempted}}</td>
      <td>{{.CategoriesExtracted}}</td>
      <td data-events-written-cell>{{.EventsWritten}}</td>
      <td class="meta">{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
{{else}}
  <p class="empty" data-empty="runs">No runs recorded yet.</p>
{{end}}
{{template "foot"}}
{{end}}
`
