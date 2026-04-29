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
        h1 { font-size: 1.5rem; font-weight: 600; margin-bottom: 1rem; }
        .search-box { width: 100%; padding: 0.75rem; border: 1px solid var(--border); border-radius: var(--radius); font-size: 1rem; background: var(--card-bg); color: var(--fg); }
        .search-box:focus { outline: 2px solid var(--accent); border-color: transparent; }
        .card { background: var(--card-bg); border: 1px solid var(--border); border-radius: var(--radius); padding: 1rem; margin: 0.75rem 0; box-shadow: var(--shadow); }
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
    <nav>
        <a href="/">Search</a>
        <a href="/digest">Digest</a>
        <a href="/topics">Topics</a>
        <a href="/knowledge">Knowledge</a>
        <a href="/settings">Settings</a>
        <a href="/status">Status</a>
        <button class="theme-toggle" id="themeBtn" aria-label="Toggle dark mode">Light / Dark</button>
    </nav>
    <script>
        (function(){var t=localStorage.getItem('theme');if(t==='dark'){document.documentElement.setAttribute('data-theme','dark');}else if(t==='light'){document.documentElement.removeAttribute('data-theme');}})();
        function toggleTheme(){var h=document.documentElement;if(h.getAttribute('data-theme')==='dark'){h.removeAttribute('data-theme');localStorage.setItem('theme','light');}else{h.setAttribute('data-theme','dark');localStorage.setItem('theme','dark');}}
        document.getElementById('themeBtn').addEventListener('click',toggleTheme);
    </script>
{{end}}

{{define "foot"}}</body></html>{{end}}

{{define "search.html"}}
{{template "head" .}}
<h1>Search</h1>
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
</div>
{{end}}
{{end}}

{{define "detail.html"}}
{{template "head" .}}
<a href="/" class="back-link">< Back to search</a>
<h1>{{.Title}}</h1>
<div class="meta"><span class="type-badge">{{.Type}}</span> {{.Connections}} connections</div>
<div class="detail-section"><h2>Summary</h2><p>{{.Summary}}</p></div>
{{if .KeyIdeas}}<div class="detail-section"><h2>Key Ideas</h2><ul class="idea-list">{{range .KeyIdeas}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
{{if .Topics}}<div class="detail-section"><h2>Topics</h2>{{range .Topics}}<span class="tag">{{.}}</span>{{end}}</div>{{end}}
{{if .SourceURL}}<div class="detail-section"><a href="{{safeURL .SourceURL}}" target="_blank">View Source</a></div>{{end}}
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
