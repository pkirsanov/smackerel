package web

// allTemplates contains all HTML templates as a single embedded string.
// Uses HTMX for interactivity, no JavaScript framework.
const allTemplates = `
{{define "head"}}<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Title}} - Smackerel</title>
    <script src="https://unpkg.com/htmx.org@1.9.12" integrity="sha384-ujb1lZYygJmzSR5/VeIZlJGW6b2fUe6w0rKWFIIbP7cM+QaJaTtRv8bXqpKW7MSR" crossorigin="anonymous"></script>
    <style>
        :root {
            --bg: #fafaf8; --fg: #1a1a18; --muted: #6b6b68;
            --border: #d4d4d0; --accent: #2d2d2b; --card-bg: #ffffff;
            --success: #2d5a2d; --warning: #8b6914; --error: #8b1414;
            --radius: 6px; --shadow: 0 1px 3px rgba(0,0,0,0.08);
        }
        @media (prefers-color-scheme: dark) {
            :root {
                --bg: #1a1a18; --fg: #e8e8e4; --muted: #8b8b88;
                --border: #3a3a38; --accent: #d4d4d0; --card-bg: #242422;
                --success: #5a8b5a; --warning: #c49b1f; --error: #c44848;
                --shadow: 0 1px 3px rgba(0,0,0,0.3);
            }
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
            background: var(--bg); color: var(--fg);
            line-height: 1.6; max-width: 800px; margin: 0 auto; padding: 1rem;
        }
        nav { display: flex; gap: 1.5rem; padding: 1rem 0; border-bottom: 1px solid var(--border); margin-bottom: 1.5rem; }
        nav a { color: var(--muted); text-decoration: none; font-size: 0.9rem; }
        nav a:hover { color: var(--fg); }
        .app-shell-nav { flex-wrap: wrap; align-items: center; gap: 1rem; }
        .app-shell-link[aria-current="page"], .app-shell-link.active { color: var(--fg); font-weight: 600; }
        h1 { font-size: 1.5rem; font-weight: 600; margin-bottom: 1rem; }
        .search-box { width: 100%; padding: 0.75rem; border: 1px solid var(--border); border-radius: var(--radius); font-size: 1rem; background: var(--card-bg); color: var(--fg); }
        .search-box:focus { outline: 2px solid var(--accent); border-color: transparent; }
        .card { background: var(--card-bg); border: 1px solid var(--border); border-radius: var(--radius); padding: 1rem; margin: 0.75rem 0; box-shadow: var(--shadow); }
        .intent-hero { border-left: 3px solid var(--accent); }
        .intent-hero-cta { display: inline-block; margin-top: 0.5rem; font-weight: 600; color: var(--fg); text-decoration: none; }
        .intent-hero-cta:hover { text-decoration: underline; }
        .card h3 { font-size: 1rem; margin-bottom: 0.25rem; }
        .card h3 a { color: var(--fg); text-decoration: none; }
        .card h3 a:hover { text-decoration: underline; }
        .card .meta { font-size: 0.8rem; color: var(--muted); }
        .card .summary { font-size: 0.9rem; margin-top: 0.5rem; }
        .type-badge { display: inline-block; font-size: 0.75rem; padding: 0.1rem 0.4rem; border: 1px solid var(--border); border-radius: 3px; color: var(--muted); }
        .tag { display: inline-block; font-size: 0.75rem; padding: 0.1rem 0.4rem; background: var(--bg); border-radius: 3px; margin: 0.1rem; }
        .empty { text-align: center; padding: 2rem; color: var(--muted); }
        .error { text-align: center; padding: 1rem; color: var(--error); }
        .status-card { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; }
        .stat { text-align: center; padding: 1rem; }
        .stat .value { font-size: 2rem; font-weight: 700; }
        .stat .label { font-size: 0.8rem; color: var(--muted); }
        .health { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 0.25rem; }
        .health.up { background: var(--success); }
        .health.down { background: var(--error); }
        .topic-list { list-style: none; }
        .topic-list li { padding: 0.5rem 0; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; }
        .detail-section { margin: 1rem 0; }
        .detail-section h2 { font-size: 1.1rem; margin-bottom: 0.5rem; }
        .idea-list { list-style: disc; padding-left: 1.5rem; }
        .idea-list li { margin: 0.25rem 0; font-size: 0.9rem; }
        .back-link { font-size: 0.9rem; color: var(--muted); text-decoration: none; }
        .back-link:hover { color: var(--fg); }
        .digest-text { white-space: pre-wrap; line-height: 1.8; }
        .htmx-indicator { display: none; text-align: center; padding: 1rem; color: var(--muted); }
        @media (max-width: 600px) { body { padding: 0.5rem; } .status-card { grid-template-columns: 1fr; } }
        .theme-toggle { background: none; border: 1px solid var(--border); border-radius: var(--radius); color: var(--muted); cursor: pointer; padding: 0.25rem 0.5rem; font-size: 0.8rem; }
        .theme-toggle:hover { color: var(--fg); }
        html[data-theme="dark"] { --bg: #1a1a18; --fg: #e8e8e4; --muted: #8b8b88; --border: #3a3a38; --accent: #d4d4d0; --card-bg: #242422; --success: #5a8b5a; --warning: #c49b1f; --error: #c44848; --shadow: 0 1px 3px rgba(0,0,0,0.3); }
    </style>
</head>
<body>
    <nav class="app-shell-nav" aria-label="Primary">
        {{template "app-shell-nav" .}}
        <a class="app-shell-link" href="/digest" data-nav="digest">Digest</a>
        <a class="app-shell-link" href="/topics" data-nav="topics">Topics</a>
        <a class="app-shell-link" href="/status" data-nav="status">Status</a>
        <button class="theme-toggle" id="themeBtn" aria-label="Toggle dark mode">Light / Dark</button>
    </nav>
    <script>
        (function(){var t=localStorage.getItem('theme');if(t==='dark'){document.documentElement.setAttribute('data-theme','dark');}else if(t==='light'){document.documentElement.removeAttribute('data-theme');}})();
        function toggleTheme(){var h=document.documentElement;if(h.getAttribute('data-theme')==='dark'){h.removeAttribute('data-theme');localStorage.setItem('theme','light');}else{h.setAttribute('data-theme','dark');localStorage.setItem('theme','dark');}}
        document.getElementById('themeBtn').addEventListener('click',toggleTheme);
    </script>
{{end}}

{{define "foot"}}</body></html>{{end}}

{{define "notification-nav"}}
<p class="meta"><a href="/notifications">Status</a> · <a href="/notifications/sources">Sources</a> · <a href="/notifications/events">Events</a> · <a href="/notifications/incidents">Incidents</a> · <a href="/notifications/approvals">Approvals</a> · <a href="/notifications/suppressions">Suppressions</a> · <a href="/notifications/summary">Summary</a> · <a href="/notifications/outputs">Outputs</a></p>
{{end}}

{{define "notifications-status.html"}}
{{template "head" .}}
<h1>Notifications</h1>
{{template "notification-nav" .}}
<div class="status-card">
    <div class="card stat"><div class="value">{{.Summary.SourceCount}}</div><div class="label">Sources</div></div>
    <div class="card stat"><div class="value">{{.Summary.OpenIncidentCount}}</div><div class="label">Open Incidents</div></div>
    <div class="card stat"><div class="value">{{.Summary.PendingApprovals}}</div><div class="label">Pending Approvals</div></div>
    <div class="card stat"><div class="value">{{.Summary.QueuedDeliveries}}</div><div class="label">Queued Outputs</div></div>
</div>
{{template "foot"}}
{{end}}

{{define "notifications-sources.html"}}
{{template "head" .}}
<h1>Notification Sources</h1>
{{template "notification-nav" .}}
{{range .Sources}}
<div class="card"><h3>{{if eq .Config.SourceType "ntfy"}}<a href="/notifications/sources/{{.Config.SourceInstanceID}}">{{.Config.SourceInstanceID}}</a>{{else}}{{.Config.SourceInstanceID}}{{end}}</h3><p class="meta">{{.Config.SourceType}} · {{.Config.SourceForm}} · {{.Health.State}} · retry {{.Health.RetryCount}}</p><p class="meta">config {{.Config.ConfigHash}} · auth {{index .Config.RedactedMetadata "auth_mode"}} · topics {{index .Config.RedactedMetadata "topic_count"}} · endpoint ref {{index .Config.RedactedMetadata "endpoint_ref_name"}}</p>{{if .Health.LastErrorRedacted}}<p class="summary">{{.Health.LastErrorRedacted}}</p>{{end}}{{if eq .Config.SourceType "ntfy"}}<p class="meta"><a href="/notifications/sources/{{.Config.SourceInstanceID}}/dead-letters">Dead letters</a> · source/output boundary: source ingest only</p>{{end}}</div>
{{else}}<div class="empty">No notification sources registered</div>{{end}}
{{template "foot"}}
{{end}}

{{define "notifications-ntfy-source.html"}}
{{template "head" .}}
<a href="/notifications/sources" class="back-link">< Back to sources</a>
<h1>ntfy Source {{.Source.Config.SourceInstanceID}}</h1>
{{template "notification-nav" .}}
<div class="card"><h3>Source Health</h3><p class="meta">{{.Source.Config.SourceType}} · {{.Source.Config.SourceForm}} · {{.Source.Health.State}} · retry {{.Source.Health.RetryCount}}</p><p class="summary">{{.Source.Health.LastErrorRedacted}}</p><p class="meta">config {{.Source.Config.ConfigHash}} · auth {{index .Source.Config.RedactedMetadata "auth_mode"}} · endpoint ref {{index .Source.Config.RedactedMetadata "endpoint_ref_name"}}</p></div>
<div class="card"><h3>Topic Health And Troubleshooting</h3><p class="meta"><a href="/api/notifications/sources/{{.Source.Config.SourceInstanceID}}/ntfy">Refresh Health</a> · <form style="display:inline" method="post" action="/api/notifications/sources/{{.Source.Config.SourceInstanceID}}/ntfy/reconnect"><button type="submit">Reconnect Source</button></form></p>{{range .Topics}}<p class="meta">{{.Topic}} · {{.SubscriptionState}} · lag {{.LagSeconds}}s · possible gap {{.PossibleGap}} · retry {{.RetryCount}}/{{.RetryBudget}}</p>{{if .LastErrorRedacted}}<p class="summary">{{.LastErrorRedacted}}</p>{{end}}{{else}}<p class="meta">No ntfy topic state recorded yet. A real source check, accepted event, reconnect action, or dead-letter pressure records topic state.</p>{{end}}</div>
<div class="card"><h3>Last Accepted Event</h3>{{if .LastEvent}}<p class="meta">source event {{.LastEvent.SourceEventID}} · raw {{.LastEvent.RawEventID}} · topic {{index .LastEvent.DeliveryMetadata "topic"}}</p><p class="summary">{{.LastEvent.Title}}</p>{{else}}<p class="meta">No accepted ntfy event recorded yet.</p>{{end}}</div>
<div class="card"><h3>Dead-Letter Queue</h3><p class="meta"><a href="/notifications/sources/{{.Source.Config.SourceInstanceID}}/dead-letters">Open ntfy dead letters</a></p>{{range .DeadLetters}}<p class="meta">{{.CauseKind}} · {{.ReplayStatus}} · {{.PayloadHash}}</p><p class="summary">{{.CauseRedacted}}</p>{{else}}<p class="meta">No ntfy dead letters recorded.</p>{{end}}</div>
<div class="card"><h3>Source/Output Boundary</h3><p class="summary">ntfy source events enter through SourceEventSink. Replay submits through the same source sink. Output dispatch is handled only by the notification core.</p></div>
{{template "foot"}}
{{end}}

{{define "notifications-ntfy-dead-letters.html"}}
{{template "head" .}}
<a href="/notifications/sources/{{.SourceInstanceID}}" class="back-link">< Back to ntfy source</a>
<h1>ntfy Dead Letters</h1>
{{template "notification-nav" .}}
{{range .DeadLetters}}<div class="card"><h3><a href="/notifications/sources/{{$.SourceInstanceID}}/dead-letters/{{.ID}}">{{.CauseKind}}</a></h3><p class="meta">{{.ID}} · topic {{.Topic}} · replay {{.ReplayStatus}} · eligible {{.ReplayEligible}}</p><p class="summary">{{.CauseRedacted}}</p><p class="meta">payload {{.PayloadHash}} · preview {{.SafePayloadPreview}}</p><p class="meta">Replay confirmation must use replay_through_source_sink; no output dispatch is performed here.</p></div>{{else}}<div class="empty">No ntfy dead letters recorded</div>{{end}}
{{template "foot"}}
{{end}}

{{define "notifications-ntfy-dead-letter-detail.html"}}
{{template "head" .}}
<a href="/notifications/sources/{{.SourceInstanceID}}/dead-letters" class="back-link">< Back to ntfy dead letters</a>
<h1>ntfy Dead Letter</h1>
{{template "notification-nav" .}}
<div class="card"><h3>{{.Record.CauseKind}}</h3><p class="meta">{{.Record.ID}} · topic {{.Record.Topic}} · replay {{.Record.ReplayStatus}} · eligible {{.Record.ReplayEligible}}</p><p class="summary">{{.Record.CauseRedacted}}</p><p class="meta">payload {{.Record.PayloadHash}} · preview {{.Record.SafePayloadPreview}}</p></div>
<div class="card"><h3>Replay Confirmation</h3><p class="summary">Replay confirmation value: replay_through_source_sink</p><p class="meta">Replay submits through SourceEventSink and does not perform output dispatch.</p><p class="meta">API route: /api/notifications/sources/{{.SourceInstanceID}}/ntfy/dead-letters/{{.Record.ID}}/replay</p></div>
{{template "foot"}}
{{end}}

{{define "notifications-events.html"}}
{{template "head" .}}
<h1>Notification Events</h1>
{{template "notification-nav" .}}
{{range .Events}}
<div class="card"><h3>{{.Title}}</h3><p class="meta">{{.SourceType}}/{{.SourceInstanceID}} · {{.Severity}} · {{.Domain}} · {{.Intent}}</p><p class="summary">{{truncate .Body 180}}</p></div>
{{else}}<div class="empty">No notification events recorded</div>{{end}}
{{template "foot"}}
{{end}}

{{define "notifications-incidents.html"}}
{{template "head" .}}
<h1>Notification Incidents</h1>
{{template "notification-nav" .}}
{{range .Incidents}}
<div class="card"><h3><a href="/notifications/incidents/{{.ID}}">{{.Title}}</a></h3><p class="meta">{{.State}} · {{.Severity}} · {{.Domain}} · {{.Intent}} · persistence {{.PersistenceCount}}</p><p class="summary">{{.StateReason}}</p></div>
{{else}}<div class="empty">No notification incidents recorded</div>{{end}}
{{template "foot"}}
{{end}}

{{define "notifications-incident-detail.html"}}
{{template "head" .}}
<a href="/notifications/incidents" class="back-link">< Back to incidents</a>
<h1>{{.Incident.Title}}</h1>
{{template "notification-nav" .}}
<div class="card"><h3>Incident Timeline</h3><p class="meta">{{.Incident.ID}} · {{.Incident.State}} · {{.Incident.Severity}} · {{.Incident.RiskLevel}}</p><p class="summary">{{.Incident.StateReason}}</p><p class="meta">Subject {{.Incident.Subject}} · Service {{.Incident.Service}} · Sources {{range .Incident.SourceInstanceIDs}}{{.}} {{end}}</p></div>
{{template "foot"}}
{{end}}

{{define "notifications-approvals.html"}}
{{template "head" .}}
<h1>Notification Approvals</h1>
{{template "notification-nav" .}}
{{range .Approvals}}
<div class="card"><h3><a href="/notifications/approvals/{{.ID}}">{{.ActionKey}}</a></h3><p class="meta">{{.Status}} · incident {{.IncidentID}} · expires {{timeAgo .ExpiresAt}}</p><p class="summary">{{.RiskExplanation}}</p></div>
{{else}}<div class="empty">No notification approvals recorded</div>{{end}}
{{template "foot"}}
{{end}}

{{define "notifications-approval-detail.html"}}
{{template "head" .}}
<a href="/notifications/approvals" class="back-link">< Back to approvals</a>
<h1>Approval {{.Approval.ID}}</h1>
{{template "notification-nav" .}}
<div class="card"><h3>{{.Approval.ActionKey}}</h3><p class="meta">{{.Approval.Status}} · {{.Approval.TargetRef}}</p><p class="summary">{{.Approval.RiskExplanation}}</p><p class="summary">{{.Approval.ExpectedEffect}}</p></div>
{{range .Decisions}}<div class="card"><h3>{{.Decision}}</h3><p class="meta">{{.ActorKind}} · {{.Channel}} · {{timeAgo .CreatedAt}}</p><p class="summary">{{.Reason}}</p></div>{{end}}
{{template "foot"}}
{{end}}

{{define "notifications-suppressions.html"}}
{{template "head" .}}
<h1>Notification Suppressions</h1>
{{template "notification-nav" .}}
<div class="card"><h3>Quiet Windows</h3>{{range .QuietWindows}}<p class="meta">{{.Reason}} · {{.StartsAt}} {{if .ExpiresAt}}to {{.ExpiresAt}}{{end}}</p>{{else}}<p class="meta">No quiet windows recorded</p>{{end}}</div>
{{range .Suppressions}}<div class="card"><h3>{{.Kind}}</h3><p class="meta">incident {{.IncidentID}} · source {{.SourceInstanceID}}</p><p class="summary">{{.Reason}}</p></div>{{else}}<div class="empty">No suppressions recorded</div>{{end}}
{{template "foot"}}
{{end}}

{{define "notifications-summary.html"}}
{{template "head" .}}
<h1>Notification Summary</h1>
{{template "notification-nav" .}}
<div class="card"><h3>Handled Noise And Open Work</h3><p class="summary">{{.Summary.OpenIncidentCount}} open incidents, {{.Summary.PendingApprovals}} pending approvals, {{.Summary.QueuedDeliveries}} queued outputs, and {{.Summary.SourceCount}} source instances.</p></div>
{{template "foot"}}
{{end}}

{{define "notifications-outputs.html"}}
{{template "head" .}}
<h1>Notification Outputs</h1>
{{template "notification-nav" .}}
{{range .Outputs}}<div class="card"><h3>{{.Channel}}</h3><p class="meta">{{.Status}} · decision {{.DecisionID}} · incident {{.IncidentID}}</p>{{if .ErrorRedacted}}<p class="summary">{{.ErrorRedacted}}</p>{{end}}</div>{{else}}<div class="empty">No notification outputs recorded</div>{{end}}
{{template "foot"}}
{{end}}

{{define "notification-error.html"}}
{{template "head" .}}
<h1>{{.Title}}</h1>
{{template "notification-nav" .}}
<div class="error">{{.Error}}</div>
{{template "foot"}}
{{end}}

{{define "search.html"}}
{{template "head" .}}
<section class="intent-hero card" aria-label="Assistant front door">
    <h1>Ask Smackerel</h1>
    <p class="summary">Describe what you're after in plain words — the assistant is the front door to everything you've captured, and capture-as-fallback always works.</p>
    <p><a class="intent-hero-cta" href="/assistant">Open the assistant &rarr;</a></p>
</section>
<h2>Or search by keyword</h2>
<input class="search-box" type="search" name="query" placeholder="Search your knowledge..."
       hx-post="/search" hx-trigger="input changed delay:300ms, keyup[key=='Enter']"
       hx-target="#results" hx-indicator="#search-spinner">
<div id="search-spinner" class="htmx-indicator">Searching...</div>
<div id="results"><div class="empty">Type a query to search your knowledge</div></div>
{{template "foot"}}
{{end}}

{{define "results-partial.html"}}
{{if .KnowledgeMatch}}
<div class="card" style="border-left:3px solid var(--warning)">
    <h3><span aria-label="Result from pre-synthesized knowledge layer">★</span> From Knowledge Layer</h3>
    <div class="meta">Concept: <strong>{{.KnowledgeMatch.Title}}</strong></div>
    <div class="summary">{{truncate .KnowledgeMatch.Summary 300}}</div>
    <div class="meta">{{.KnowledgeMatch.CitationCount}} citations · Updated {{timeAgo .KnowledgeMatch.UpdatedAt}}</div>
    <div style="margin-top:0.5rem"><a href="/knowledge/concepts/{{.KnowledgeMatch.ConceptID}}">View Full Concept Page</a></div>
</div>
{{end}}
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
{{if .Empty}}<div class="empty">{{.Empty}}</div>{{end}}
{{range .Results}}
<div class="card">
    <h3><a href="/artifact/{{.ID}}">{{.Title}}</a></h3>
    <div class="meta"><span class="type-badge">{{.Type}}</span></div>
    {{if .Summary}}<div class="summary">{{truncate .Summary 200}}</div>{{end}}
    {{if .QFCard}}{{template "qf-card" .QFCard}}{{end}}
</div>
{{end}}
{{end}}

{{define "qf-card"}}
<div class="qf-card" data-card-kind="{{.CardKind}}">
    <div class="meta"><strong>{{.DisplayLabel}}</strong> · {{.ApprovalState}} · read-only</div>
    <div class="meta">Packet {{.PacketID}} · Trace {{.TraceID}}</div>
    {{if .UnknownDecisionType}}<div class="meta">Generic QF packet</div>{{end}}
    {{if .TrustObjects}}<ul class="idea-list">{{range .TrustObjects}}<li><strong>{{.Label}}</strong> ({{.Severity}}): {{.Summary}}</li>{{end}}</ul>{{end}}
    {{if .DeepLink.URL}}<div class="meta"><a href="{{safeURL .DeepLink.URL}}" target="_blank">Open in QF</a> · {{.DeepLink.Status}}</div>{{end}}
</div>
{{end}}

{{define "detail.html"}}
{{template "head" .}}
<a href="/" class="back-link">< Back to search</a>
<h1>{{.Title}}</h1>
<div class="meta"><span class="type-badge">{{.Type}}</span> {{.Connections}} connections</div>
{{if .QFCard}}<div class="detail-section">{{template "qf-card" .QFCard}}</div>{{end}}
<div class="detail-section"><h2>Summary</h2><p>{{.Summary}}</p></div>
{{if .KeyIdeas}}<div class="detail-section"><h2>Key Ideas</h2><ul class="idea-list">{{range .KeyIdeas}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
{{if .Topics}}<div class="detail-section"><h2>Topics</h2>{{range .Topics}}<span class="tag">{{.}}</span>{{end}}</div>{{end}}
{{if .SourceURL}}<div class="detail-section"><a href="{{safeURL .SourceURL}}" target="_blank">View Source</a></div>{{end}}
{{template "foot"}}
{{end}}

{{define "evidence-builder.html"}}
{{template "head" .}}
<a href="/" class="back-link">< Back to search</a>
<h1>Personal Evidence Bundle</h1>
<div class="card" id="qf-evidence-builder" data-qf-artifact-id="{{.QFArtifactID}}" data-packet-id="{{.PacketID}}">
    <form id="qf-evidence-export-form">
        <label>QF artifact ID <input name="packet_artifact_id" value="{{.QFArtifactID}}" required></label>
        <label>Source artifact IDs <input name="source_artifact_ids" required></label>
        <label>Extracted claims <textarea name="extracted_claims" required></textarea></label>
        <label>Consent scope <input name="consent_scope" required></label>
        <label>Sensitivity tier <input name="sensitivity_tier" required></label>
        <label>Source provenance class <input name="source_provenance_class" required></label>
        <label>Confidence <input name="confidence" type="number" min="0.01" max="1" step="0.01" required></label>
        <button type="submit">Export Evidence Bundle</button>
    </form>
    <div id="qf-evidence-export-status" aria-live="polite"></div>
    <button id="qf-evidence-revoke" type="button" disabled>Revoke Evidence Sharing</button>
</div>
<script>
(function () {
    const form = document.getElementById("qf-evidence-export-form");
    const status = document.getElementById("qf-evidence-export-status");
    const revoke = document.getElementById("qf-evidence-revoke");
    let exportID = "";
    function splitList(value) { return value.split(",").map(function (x) { return x.trim(); }).filter(Boolean); }
    form.addEventListener("submit", async function (event) {
        event.preventDefault();
        const data = new FormData(form);
        const sourceIDs = splitList(data.get("source_artifact_ids") || "");
        const payload = {
            packet_artifact_id: String(data.get("packet_artifact_id") || ""),
            source_artifact_ids: sourceIDs,
            source_refs: [],
            source_provenance_classes: sourceIDs.map(function (id) { return { source_artifact_id: id, source_provenance_class: String(data.get("source_provenance_class") || "") }; }),
            extracted_claims: splitList(data.get("extracted_claims") || ""),
            confidence: Number(data.get("confidence")),
            consent_scope: String(data.get("consent_scope") || ""),
            sensitivity_tier: String(data.get("sensitivity_tier") || ""),
            provenance: { surface: "web_evidence_builder" },
            redaction_summary: { raw_personal_content: "omitted" },
            related_symbols: [],
            related_entities: []
        };
        status.textContent = "Exporting";
        const response = await fetch("/api/qf/evidence-bundles/", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
        const body = await response.json();
        if (!response.ok) { status.textContent = body.error ? body.error.message : "Export failed"; return; }
        exportID = body.record.export_id;
        revoke.disabled = false;
        status.textContent = "Export " + body.record.status + " · " + exportID;
    });
    revoke.addEventListener("click", async function () {
        if (!exportID) { return; }
        const response = await fetch("/api/qf/evidence-bundles/" + encodeURIComponent(exportID), { method: "DELETE", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ reason: "consent_revoked" }) });
        const body = await response.json();
        status.textContent = response.ok ? "Revocation " + body.record.status : (body.error ? body.error.message : "Revocation failed");
    });
}());
</script>
{{template "foot"}}
{{end}}

{{define "digest.html"}}
{{template "head" .}}
<h1>Daily Digest - {{.DigestDate}}</h1>
<div class="card"><div class="digest-text">{{.DigestText}}</div></div>
{{template "foot"}}
{{end}}

{{define "topics.html"}}
{{template "head" .}}
<h1>Topics</h1>
{{if .Topics}}<ul class="topic-list">
{{range .Topics}}<li><span>{{.Name}}</span><span><span class="type-badge">{{.State}}</span> {{.Count}} items</span></li>{{end}}
</ul>{{else}}<div class="empty">No topics yet. Capture some content to start building your knowledge graph.</div>{{end}}
{{template "foot"}}
{{end}}

{{define "settings.html"}}
{{template "head" .}}
<h1>Settings</h1>
<div class="card"><h3>LLM Configuration</h3><p class="meta">Provider: {{.LLMProvider}} / Model: {{.LLMModel}}</p></div>
<div class="card"><h3>Digest Schedule</h3><p class="meta">Cron: {{.DigestCron}}</p></div>
{{if .Connectors}}<div class="card"><h3>Connectors</h3>{{range .Connectors}}<div style="display:flex;justify-content:space-between;align-items:center;padding:0.5rem 0;border-bottom:1px solid var(--border)"><div><span class="health {{if .Enabled}}up{{else}}down{{end}}"></span> {{.Name}}<span class="meta"> — {{.ItemsSynced}} items{{if .LastSync}}, last sync: {{.LastSync}}{{end}}{{if .LastErr}} ({{.LastErr}}){{end}}</span></div><form method="POST" action="/settings/connectors/{{.Name}}/sync" style="margin:0"><button type="submit" style="font-size:0.8rem;padding:0.2rem 0.6rem;cursor:pointer;border:1px solid var(--border);border-radius:var(--radius);background:var(--card-bg);color:var(--fg)">Sync Now</button></form></div>{{end}}</div>{{else}}<div class="card"><h3>Connectors</h3><p class="meta">No connectors registered</p></div>{{end}}
{{if .OAuth}}<div class="card"><h3>OAuth Connections</h3>{{range .OAuth}}<p><span class="health {{if .Connected}}up{{else}}down{{end}}"></span> {{.Provider}}</p>{{end}}</div>{{end}}
<div class="card"><h3>Import Bookmarks</h3><form method="POST" action="/settings/bookmarks/import" enctype="multipart/form-data"><p class="meta">Upload a Chrome JSON or Netscape HTML bookmark file</p><input type="file" name="file" accept=".json,.html,.htm" style="margin:0.5rem 0;display:block"><button type="submit" style="padding:0.4rem 1rem;cursor:pointer;border:1px solid var(--border);border-radius:var(--radius);background:var(--card-bg);color:var(--fg)">Import</button></form></div>
{{template "foot"}}
{{end}}

{{define "status.html"}}
{{template "head" .}}
<h1>System Status</h1>
<div class="status-card">
    <div class="card stat"><div class="value">{{.ArtifactCount}}</div><div class="label">Artifacts</div></div>
    <div class="card stat"><div class="value">{{.TopicCount}}</div><div class="label">Topics</div></div>
    <div class="card stat"><div class="value">{{.EdgeCount}}</div><div class="label">Connections</div></div>
    <div class="card stat"><div class="value">{{.Uptime}}</div><div class="label">Uptime</div></div>
</div>
<div class="card">
    <h3>Services</h3>
    <p><span class="health {{if .DBHealthy}}up{{else}}down{{end}}"></span> PostgreSQL</p>
    <p><span class="health {{if .NATSHealthy}}up{{else}}down{{end}}"></span> NATS</p>
</div>
{{if .RecommendationsEnabled}}
<section aria-label="Recommendation provider status">
<div class="card">
    <h3>Recommendation Providers</h3>
    {{range .RecommendationProviderStatuses}}
    <p><span class="health {{if .Healthy}}up{{else}}down{{end}}"></span> {{.DisplayName}} <span class="meta">({{.ProviderID}}) - {{.Status}}{{if .Reason}}: {{.Reason}}{{end}}{{if .CategoryLabel}} · {{.CategoryLabel}}{{end}}</span></p>
    {{else}}
    <p class="meta">0 recommendation providers configured</p>
    {{end}}
</div>
</section>
{{end}}
{{if .KnowledgeStats}}
<section aria-label="Knowledge layer status">
<div class="card">
    <h3>Knowledge Layer</h3>
    <p class="meta">Concept Pages: {{.KnowledgeStats.ConceptCount}} · Entity Profiles: {{.KnowledgeStats.EntityCount}}</p>
    <p class="meta">Knowledge Edges: {{.KnowledgeStats.EdgeCount}} · Synthesis Pending: {{.KnowledgeStats.SynthesisPending}}</p>
    {{if .KnowledgeStats.LastSynthesisAt}}<p class="meta">Last Synthesis: {{timeAgo .KnowledgeStats.LastSynthesisAt}}</p>{{end}}
    <p class="meta">Lint Findings: {{.KnowledgeStats.LintFindingsTotal}} ({{.KnowledgeStats.LintFindingsHigh}} high)</p>
    {{if .KnowledgeStats.PromptContractVersion}}<p class="meta">Prompt Contract: {{.KnowledgeStats.PromptContractVersion}}</p>{{end}}
    <p style="margin-top:0.5rem"><a href="/knowledge">View Knowledge Dashboard</a></p>
</div>
</section>
{{end}}
{{template "foot"}}
{{end}}

{{define "bookmark-import-result.html"}}
{{template "head" .}}
<h1>Bookmark Import</h1>
<div class="card"><p>Imported {{.Imported}} bookmarks.</p><a href="/settings" class="back-link">Back to Settings</a></div>
{{template "foot"}}
{{end}}

{{define "knowledge-dashboard.html"}}
{{template "head" .}}
<h1>Knowledge Layer</h1>
{{if .Empty}}<div class="empty">{{.Empty}}</div>{{else}}
<div class="status-card">
    <div class="card stat"><a href="/knowledge/concepts" style="text-decoration:none;color:inherit"><div class="value">{{.Stats.ConceptCount}}</div><div class="label">Concepts</div></a></div>
    <div class="card stat"><a href="/knowledge/entities" style="text-decoration:none;color:inherit"><div class="value">{{.Stats.EntityCount}}</div><div class="label">Entities</div></a></div>
    <div class="card stat"><div class="value">{{.Stats.EdgeCount}}</div><div class="label">Connections</div></div>
    <div class="card stat"><a href="/knowledge/lint" style="text-decoration:none;color:inherit"><div class="value">{{.Stats.LintFindingsTotal}}</div><div class="label">Lint Findings</div></a></div>
</div>
<div class="card">
    <h3>Synthesis Status</h3>
    <p class="meta">Completed: {{.Stats.SynthesisCompleted}} · Pending: {{.Stats.SynthesisPending}} · Failed: {{.Stats.SynthesisFailed}}</p>
    {{if .Stats.LastSynthesisAt}}<p class="meta">Last synthesis: {{timeAgo .Stats.LastSynthesisAt}}</p>{{end}}
    {{if .Stats.PromptContractVersion}}<p class="meta">Prompt contract: {{.Stats.PromptContractVersion}}</p>{{end}}
</div>
{{if .RecentConcepts}}
<div class="card">
    <h3>Recent Knowledge Activity</h3>
    {{range .RecentConcepts}}<p class="meta">{{.Title}} — updated {{timeAgo .UpdatedAt}}</p>{{end}}
</div>
{{end}}
<p style="margin-top:1rem">
    <a href="/knowledge/concepts">Browse Concepts</a> ·
    <a href="/knowledge/entities">View Entities</a> ·
    <a href="/knowledge/lint">Lint Report</a>
</p>
{{end}}
{{template "foot"}}
{{end}}

{{define "concepts-list.html"}}
{{template "head" .}}
<a href="/knowledge" class="back-link">← Knowledge</a>
<h1>Concept Pages ({{.Total}})</h1>
<input class="search-box" type="search" name="q" placeholder="Search concepts..."
       hx-get="/knowledge/concepts" hx-trigger="input changed delay:300ms"
       hx-target="#concepts-list" hx-push-url="true" hx-include="[name='sort']">
<div style="margin:0.75rem 0">
    <label class="meta">Sort: </label>
    <select name="sort" style="font-size:0.85rem;padding:0.2rem 0.4rem;border:1px solid var(--border);border-radius:var(--radius);background:var(--card-bg);color:var(--fg)"
            hx-get="/knowledge/concepts" hx-trigger="change" hx-target="#concepts-list" hx-push-url="true" hx-include="[name='q']">
        <option value="updated"{{if eq .Sort "updated"}} selected{{end}}>Updated Recently</option>
        <option value="citations"{{if eq .Sort "citations"}} selected{{end}}>Most Referenced</option>
        <option value="title"{{if eq .Sort "title"}} selected{{end}}>Alphabetical</option>
    </select>
</div>
<div id="concepts-list">
{{if .Concepts}}{{range .Concepts}}
<div class="card">
    <h3><a href="/knowledge/concepts/{{.ID}}">{{.Title}}</a></h3>
    <div class="meta">{{len .SourceArtifactIDs}} citations · Updated {{timeAgo .UpdatedAt}}</div>
    {{if .Summary}}<div class="summary">{{truncate .Summary 200}}</div>{{end}}
    {{if .SourceTypeDiversity}}<div class="meta">Sources: {{range $i, $s := .SourceTypeDiversity}}{{if $i}}, {{end}}{{$s}}{{end}}</div>{{end}}
</div>
{{end}}{{else}}<div class="empty">No concept pages yet. The knowledge layer will build concept pages automatically as content is ingested.</div>{{end}}
</div>
{{template "foot"}}
{{end}}

{{define "concept-detail.html"}}
{{template "head" .}}
<a href="/knowledge/concepts" class="back-link">← Concept Pages</a>
<h1>{{.Concept.Title}}</h1>
<div class="meta">Last updated: {{timeAgo .Concept.UpdatedAt}} · Contract: {{.Concept.PromptContractVersion}}</div>
<div class="card"><h3>Summary</h3><p>{{.Concept.Summary}}</p></div>
{{if .Claims}}
<div class="card">
    <h3>Claims</h3>
    <dl>
    {{range .Claims}}
    <dt style="margin-top:0.5rem;font-style:italic">"{{.Text}}"</dt>
    <dd class="meta" style="margin-left:1rem">📎 <a href="/artifact/{{.ArtifactID}}">{{.ArtifactTitle}}</a> ({{.SourceType}})</dd>
    {{end}}
    </dl>
</div>
{{end}}
{{if .RelatedConcepts}}
<div class="card"><h3>Related Concepts</h3>{{range .RelatedConcepts}}<a href="/knowledge/concepts/{{.ID}}" class="tag">{{.Title}}</a> {{end}}</div>
{{end}}
{{if .Entities}}
<div class="card"><h3>Connected Entities</h3>{{range .Entities}}<a href="/knowledge/entities/{{.ID}}" class="tag">{{.Name}}</a> {{end}}</div>
{{end}}
{{template "foot"}}
{{end}}

{{define "entities-list.html"}}
{{template "head" .}}
<a href="/knowledge" class="back-link">← Knowledge</a>
<h1>Entity Profiles ({{.Total}})</h1>
<input class="search-box" type="search" name="q" placeholder="Search entities..."
       hx-get="/knowledge/entities" hx-trigger="input changed delay:300ms"
       hx-target="#entities-list" hx-push-url="true" hx-include="[name='sort']">
<div style="margin:0.75rem 0">
    <label class="meta">Sort: </label>
    <select name="sort" style="font-size:0.85rem;padding:0.2rem 0.4rem;border:1px solid var(--border);border-radius:var(--radius);background:var(--card-bg);color:var(--fg)"
            hx-get="/knowledge/entities" hx-trigger="change" hx-target="#entities-list" hx-push-url="true" hx-include="[name='q']">
        <option value="updated"{{if eq .Sort "updated"}} selected{{end}}>Updated Recently</option>
        <option value="interactions"{{if eq .Sort "interactions"}} selected{{end}}>Most Interactions</option>
        <option value="name"{{if eq .Sort "name"}} selected{{end}}>Alphabetical</option>
    </select>
</div>
<div id="entities-list">
{{if .Entities}}{{range .Entities}}
<div class="card">
    <h3><a href="/knowledge/entities/{{.ID}}">{{.Name}}</a></h3>
    <div class="meta"><span class="type-badge">{{.EntityType}}</span> {{.InteractionCount}} mentions</div>
    {{if .Summary}}<div class="summary">{{truncate .Summary 200}}</div>{{end}}
    {{if .SourceTypes}}<div class="meta">Sources: {{range $i, $s := .SourceTypes}}{{if $i}}, {{end}}{{$s}}{{end}}</div>{{end}}
</div>
{{end}}{{else}}<div class="empty">No entity profiles yet. Entities are detected automatically during content synthesis.</div>{{end}}
</div>
{{template "foot"}}
{{end}}

{{define "entity-detail.html"}}
{{template "head" .}}
<a href="/knowledge/entities" class="back-link">← Entities</a>
<h1>{{.Entity.Name}}</h1>
<div class="meta"><span class="type-badge">{{.Entity.EntityType}}</span> · {{.Entity.InteractionCount}} mentions across {{len .Entity.SourceTypes}} source types</div>
<div class="card"><h3>Profile Summary</h3><p>{{.Entity.Summary}}</p></div>
{{if .Entity.SourceTypes}}
<div class="card"><h3>Source Types</h3>{{range .Entity.SourceTypes}}<span class="type-badge" style="margin:0.1rem">{{.}}</span> {{end}}</div>
{{end}}
{{if .RelatedConcepts}}
<div class="card"><h3>Related Concepts</h3>{{range .RelatedConcepts}}<a href="/knowledge/concepts/{{.ID}}" class="tag">{{.Title}}</a> {{end}}</div>
{{end}}
{{if .Mentions}}
<div class="card">
    <h3>Interaction Timeline</h3>
    <ol style="list-style:none;padding:0">
    {{range .Mentions}}
    <li style="padding:0.3rem 0;border-bottom:1px solid var(--border)">
        <span class="meta">{{.MentionedAt}}</span> <span class="type-badge">{{.SourceType}}</span>
        <a href="/artifact/{{.ArtifactID}}">{{.ArtifactTitle}}</a>
        {{if .Context}}<div class="meta">{{truncate .Context 150}}</div>{{end}}
    </li>
    {{end}}
    </ol>
</div>
{{end}}
{{template "foot"}}
{{end}}

{{define "lint-report.html"}}
{{template "head" .}}
<a href="/knowledge" class="back-link">← Knowledge</a>
<h1>Knowledge Lint Report</h1>
{{if .Report}}
<div class="meta">Last run: {{timeAgo .Report.RunAt}} · Duration: {{.Report.DurationMs}}ms</div>
<div class="status-card" style="margin-top:0.75rem">
    <div class="card stat"><div class="value" style="color:var(--error)">{{.Summary.High}}</div><div class="label">High</div></div>
    <div class="card stat"><div class="value" style="color:var(--warning)">{{.Summary.Medium}}</div><div class="label">Medium</div></div>
    <div class="card stat"><div class="value" style="color:var(--muted)">{{.Summary.Low}}</div><div class="label">Low</div></div>
    <div class="card stat"><div class="value">{{.Summary.Total}}</div><div class="label">Total</div></div>
</div>
{{if .Findings}}
{{range .Findings}}
<div class="card">
    <h3>{{if eq .Severity "high"}}🔴 HIGH{{else if eq .Severity "medium"}}🟡 MEDIUM{{else}}🔵 LOW{{end}} — {{.Type}}</h3>
    <p>{{.TargetTitle}}: {{.Description}}</p>
    {{if .SuggestedAction}}<p class="meta">Suggested: {{.SuggestedAction}}</p>{{end}}
    {{if eq .Type "contradiction"}}<p><a href="/knowledge/lint/{{.TargetID}}">View Contradiction</a></p>
    {{else if eq .TargetType "concept"}}<p><a href="/knowledge/concepts/{{.TargetID}}">View Concept</a></p>
    {{else if eq .TargetType "entity"}}<p><a href="/knowledge/entities/{{.TargetID}}">View Entity</a></p>{{end}}
</div>
{{end}}
{{else}}<div class="empty">No lint findings. Your knowledge layer is healthy. ✓</div>{{end}}
{{else}}<div class="empty">No lint report available yet. The lint job runs on a scheduled basis.</div>{{end}}
{{template "foot"}}
{{end}}

{{define "lint-finding-detail.html"}}
{{template "head" .}}
<a href="/knowledge/lint" class="back-link">← Lint Report</a>
<h1>⚠ {{.Finding.Type}}: {{.Finding.TargetTitle}}</h1>
<div class="meta">Severity: {{.Finding.Severity}} · {{.Finding.Description}}</div>
{{if .Finding.SuggestedAction}}<div class="card"><h3>Suggested Action</h3><p>{{.Finding.SuggestedAction}}</p></div>{{end}}
{{if .Concept}}
<div class="card">
    <h3>Concept Page</h3>
    <p>{{.Concept.Title}}</p>
    <p><a href="/knowledge/concepts/{{.Concept.ID}}">View Concept Page</a></p>
</div>
{{end}}
{{template "foot"}}
{{end}}
`
