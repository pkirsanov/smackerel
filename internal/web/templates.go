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
        <a href="/settings">Settings</a>
        <a href="/status">Status</a>
        <button class="theme-toggle" onclick="toggleTheme()" aria-label="Toggle dark mode">Light / Dark</button>
    </nav>
    <script>
        (function(){var t=localStorage.getItem('theme');if(t==='dark'){document.documentElement.setAttribute('data-theme','dark');}else if(t==='light'){document.documentElement.removeAttribute('data-theme');}})();
        function toggleTheme(){var h=document.documentElement;if(h.getAttribute('data-theme')==='dark'){h.removeAttribute('data-theme');localStorage.setItem('theme','light');}else{h.setAttribute('data-theme','dark');localStorage.setItem('theme','dark');}}
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
{{if .Connectors}}<div class="card"><h3>Connectors</h3>{{range .Connectors}}<p><span class="health {{if .Enabled}}up{{else}}down{{end}}"></span> {{.Name}}{{if .LastErr}} <span class="meta">({{.LastErr}})</span>{{end}}</p>{{end}}</div>{{else}}<div class="card"><h3>Connectors</h3><p class="meta">No connectors registered</p></div>{{end}}
{{if .OAuth}}<div class="card"><h3>OAuth Connections</h3>{{range .OAuth}}<p><span class="health {{if .Connected}}up{{else}}down{{end}}"></span> {{.Provider}}</p>{{end}}</div>{{end}}
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
{{template "foot"}}
{{end}}
`
