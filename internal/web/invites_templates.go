// Spec 093 SCOPE-03 — admin invites UI templates.
//
// Parsed in NewCardRewardsWebHandler on top of the card-rewards template set,
// so these pages reuse the spec-092 "head"/"cardrewards-nav"/"foot" chrome +
// the design-token CSS palette (var(--…)) + the card-rewards FuncMap (deref)
// verbatim. CSP-clean: interactivity is plain <form> submits + a CSS-only
// <details> revoke-confirm; there are NO inline <script> blocks and NO inline
// event handlers (onclick/onsubmit), so the pages stay clean under the spec-077
// CSP guard. The one-time plaintext token appears ONLY in
// cardrewards-invite-reveal.html — never in cardrewards-invites.html.
package web

const cardRewardsInviteTemplates = `
{{define "cardrewards-invite-table"}}
{{if .}}
<div class="table-wrap">
<table class="cr-table">
  <thead><tr><th>Label</th><th>Created by</th><th>Created</th><th>Status</th><th>Actions</th></tr></thead>
  <tbody>
  {{range .}}
    <tr data-invite-row data-invite-id="{{.ID}}" data-invite-status="{{.Status}}">
      <td>{{if .Label}}{{deref .Label}}{{else}}<span class="meta">&mdash;</span>{{end}}</td>
      <td>{{.CreatedBy}}</td>
      <td class="meta">{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
      <td><span class="badge {{.BadgeClass}}" data-badge="{{.Status}}"><span aria-hidden="true">{{.BadgeGlyph}}</span> {{.BadgeLabel}}{{if and (eq .Status "used") .UsedBy}} by {{deref .UsedBy}}{{end}}</span></td>
      <td>
        {{if .CanRevoke}}
        <details data-revoke-confirm>
          <summary class="btn btn-secondary btn-sm" data-action="revoke-open">Revoke</summary>
          <form method="POST" action="/admin/invites/{{.ID}}/revoke" data-action="revoke">
            <p class="meta">This invite can no longer be used to register.</p>
            <button class="btn btn-danger btn-sm" type="submit" data-action="revoke-confirm">Confirm revoke</button>
          </form>
        </details>
        {{else}}<span class="meta">&mdash;</span>{{end}}
      </td>
    </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<p class="empty-state" data-empty="invites">No invites yet. Generate one above.</p>
{{end}}
{{end}}

{{define "cardrewards-invites.html"}}
{{template "head" .}}
<div class="page-header">
  <h1 class="page-title">Account Invites</h1>
  <p class="page-subtitle">Generate single-use registration invites for new operators.</p>
</div>
<div class="btn-row">
  <a class="btn btn-ghost btn-sm" href="/cards/admin" data-action="back-to-admin">&lsaquo; Back to Admin</a>
</div>
{{if .Notice}}
<div class="alert alert-warning" role="status" data-notice="race">{{.Notice}}</div>
{{end}}
<div class="card">
  <h2 class="card-title">Generate an invite</h2>
  <form method="POST" action="/admin/invites" data-action="generate">
    <div class="form-row">
      <label for="invite-label">Label (optional)</label>
      <input class="form-control" type="text" id="invite-label" name="label" maxlength="120" placeholder="e.g. for the new analyst" data-field="label" autocomplete="off">
    </div>
    <div class="btn-row">
      <button class="btn btn-primary" type="submit" data-action="generate-submit">Generate invite</button>
    </div>
  </form>
</div>
<h2 class="card-title">Invites</h2>
{{template "cardrewards-invite-table" .Invites}}
{{template "foot"}}
{{end}}

{{define "cardrewards-invite-reveal.html"}}
{{template "head" .}}
<div class="page-header">
  <h1 class="page-title">Account Invites</h1>
  <p class="page-subtitle">Generate single-use registration invites for new operators.</p>
</div>
<div class="card alert-info" role="status" aria-live="polite" data-onetime-token-reveal>
  <h2 class="card-title">Invite created</h2>
  <p><strong>Copy this token now &mdash; it will not be shown again.</strong></p>
  <div class="form-row">
    <label for="onetime-token">One-time invite token</label>
    <input class="form-control" id="onetime-token" type="text" readonly value="{{.Token}}" aria-label="One-time invite token" data-onetime-token autocomplete="off">
  </div>
  <p class="meta">Select the field and press Ctrl/Cmd-A then Ctrl/Cmd-C to copy.{{if .Label}} Label: {{.Label}}.{{end}}</p>
  <div class="btn-row">
    <a class="btn btn-secondary" href="/admin/invites" data-action="done">Done &mdash; back to invites</a>
  </div>
</div>
<h2 class="card-title">Invites</h2>
{{template "cardrewards-invite-table" .Invites}}
{{template "foot"}}
{{end}}
`
