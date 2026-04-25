// Spec 037 Scope 8 — admin web templates for the operator UI.
//
// Inline templates parsed once in NewAgentAdminHandler. Each page is a
// self-contained template (no shared layout block) so html/template can
// parse the set without name collisions across pages.
package web

const agentAdminTemplates = `
{{define "agent_head.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Title}} — Smackerel Agent Admin</title>
<style>
body { font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 1rem; background: #fafafa; color: #222; }
nav { background: #1f2937; color: white; padding: 0.6rem 1rem; margin: -1rem -1rem 1rem -1rem; }
nav a { color: white; margin-right: 1rem; text-decoration: none; }
nav a:hover { text-decoration: underline; }
h1 { font-size: 1.4rem; margin-top: 0; }
table { border-collapse: collapse; width: 100%; background: white; }
th, td { border: 1px solid #e5e7eb; padding: 0.4rem 0.6rem; text-align: left; vertical-align: top; }
th { background: #f3f4f6; font-weight: 600; }
.badge { display: inline-block; padding: 0.1rem 0.5rem; border-radius: 0.3rem; font-size: 0.85em; font-weight: 600; }
.side-effect-read     { background: #d1fae5; color: #065f46; }
.side-effect-write    { background: #fef3c7; color: #92400e; }
.side-effect-external { background: #fee2e2; color: #991b1b; }
.outcome-banner { padding: 0.8rem 1rem; border-radius: 0.4rem; margin: 0.5rem 0; }
.outcome-info    { background: #dbeafe; color: #1e3a8a; border-left: 4px solid #1d4ed8; }
.outcome-warning { background: #fef3c7; color: #92400e; border-left: 4px solid #d97706; }
.outcome-error   { background: #fee2e2; color: #991b1b; border-left: 4px solid #b91c1c; }
.muted { color: #6b7280; font-size: 0.9em; }
pre { background: #f3f4f6; padding: 0.6rem; border-radius: 0.3rem; overflow: auto; max-height: 30rem; font-size: 0.85em; }
.filter-form { margin-bottom: 1rem; }
.filter-form select, .filter-form input { padding: 0.3rem 0.4rem; }
.pager a { margin-right: 0.6rem; }
</style>
</head>
<body>
<nav>
  <strong>Agent Admin</strong>
  <a href="/admin/agent/traces">Traces</a>
  <a href="/admin/agent/scenarios">Scenarios</a>
  <a href="/admin/agent/tools">Tools</a>
</nav>
<h1>{{.Title}}</h1>
{{end}}

{{define "agent_foot.html"}}
</body>
</html>
{{end}}

{{define "agent_traces_index.html"}}
{{template "agent_head.html" .}}
<form class="filter-form" method="get" action="/admin/agent/traces">
  <label>Outcome:
    <select name="outcome" onchange="this.form.submit()">
      <option value="">(all)</option>
      {{range .OutcomeClasses}}
        <option value="{{.}}" {{if eq . $.OutcomeFilter}}selected{{end}}>{{.}}</option>
      {{end}}
    </select>
  </label>
  <input type="hidden" name="limit" value="{{.Limit}}">
  <noscript><button type="submit">Apply</button></noscript>
</form>
<p class="muted">Total matching: {{.Total}} — showing {{len .Rows}} (offset {{.Offset}})</p>
{{if .Rows}}
<table>
<thead><tr><th>Started</th><th>Trace ID</th><th>Scenario</th><th>Version</th><th>Source</th><th>Outcome</th><th>Calls</th><th>Latency</th></tr></thead>
<tbody>
{{range .Rows}}
<tr>
  <td><span class="muted">{{.StartedAt.UTC.Format "2006-01-02 15:04:05Z"}}</span></td>
  <td><a href="/admin/agent/traces/{{.TraceID}}"><code>{{.TraceID}}</code></a></td>
  <td>{{.ScenarioID}}</td>
  <td><span class="muted">{{.ScenarioVersion}}</span></td>
  <td>{{.Source}}</td>
  <td><span class="badge {{sevClass .OutcomeSeverity}}">{{.Outcome}}</span></td>
  <td>{{.ToolCallCount}}</td>
  <td>{{.LatencyMs}}ms</td>
</tr>
{{end}}
</tbody>
</table>
<p class="pager">
  {{if gt .Offset 0}}<a href="?outcome={{.OutcomeFilter}}&offset={{.PrevOffset}}&limit={{.Limit}}">&laquo; prev</a>{{end}}
  {{if gt .Total .NextOffset}}<a href="?outcome={{.OutcomeFilter}}&offset={{.NextOffset}}&limit={{.Limit}}">next &raquo;</a>{{end}}
</p>
{{else}}
<p>No traces match the current filter.</p>
{{end}}
{{template "agent_foot.html" .}}
{{end}}

{{define "agent_trace_show.html"}}
{{template "agent_head.html" .}}
{{with .Detail}}
<p><a href="/admin/agent/traces">&laquo; All traces</a></p>
<div class="outcome-banner {{sevClass .Outcome.Severity}}">
  <strong>Outcome: {{.Outcome.Label}}</strong>
  ({{.Outcome.Class}})
  <div class="muted">{{.Outcome.Summary}}</div>
  <table style="margin-top: 0.5rem; max-width: 60rem;">
    <tbody>
    {{range .Outcome.Fields}}<tr><th>{{.Key}}</th><td>{{.Value}}</td></tr>{{end}}
    </tbody>
  </table>
</div>

<h2>Summary</h2>
<table style="max-width: 60rem;">
<tbody>
<tr><th>scenario_id</th><td>{{.Summary.ScenarioID}}</td></tr>
<tr><th>scenario_version</th><td>{{.Summary.ScenarioVersion}}</td></tr>
<tr><th>source</th><td>{{.Summary.Source}}</td></tr>
<tr><th>provider / model</th><td>{{.Provider}} / {{.Model}}</td></tr>
<tr><th>tokens prompt / completion</th><td>{{.TokensPrompt}} / {{.TokensComp}}</td></tr>
<tr><th>started / ended</th><td>{{.StartedAt.UTC.Format "2006-01-02 15:04:05Z"}} / {{.EndedAt.UTC.Format "2006-01-02 15:04:05Z"}}</td></tr>
<tr><th>latency</th><td>{{.Summary.LatencyMs}}ms</td></tr>
</tbody>
</table>

<h2>Routing</h2>
<p>reason: <strong>{{.Routing.Reason}}</strong> &middot; chosen: <strong>{{.Routing.Chosen}}</strong>
&middot; top_score: {{printf "%.3f" .Routing.TopScore}} &middot; threshold: {{printf "%.3f" .Routing.Threshold}}</p>
{{if .Routing.Considered}}
<table style="max-width: 30rem;">
<thead><tr><th>scenario</th><th>score</th></tr></thead>
<tbody>
{{range .Routing.Considered}}<tr><td>{{.ScenarioID}}</td><td>{{printf "%.3f" .Score}}</td></tr>{{end}}
</tbody>
</table>
{{end}}

<h2>Envelope</h2>
<p>source: <code>{{.Envelope.Source}}</code> &middot; raw_input: <code>{{.Envelope.RawInput}}</code></p>
{{if .Envelope.StructuredContext}}<pre>{{.Envelope.StructuredContext}}</pre>{{end}}

<h2>Tool Calls ({{len .ToolCalls}})</h2>
{{if .ToolCalls}}
<table>
<thead><tr><th>seq</th><th>tool</th><th>outcome</th><th>rejection / error</th><th>latency</th></tr></thead>
<tbody>
{{range .ToolCalls}}
<tr>
  <td>{{.Seq}}</td>
  <td><code>{{.Name}}</code></td>
  <td><span class="badge {{sevClass .OutcomeSeverity}}">{{.Outcome}}</span></td>
  <td>{{if .RejectionReason}}<strong>{{.RejectionReason}}</strong>{{end}} {{.Error}}</td>
  <td>{{.LatencyMs}}ms</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}

{{if .FinalOutput}}<h2>Final Output</h2><pre>{{.FinalOutput}}</pre>{{end}}
{{if .OutcomeDetail}}<h2>Outcome Detail</h2><pre>{{.OutcomeDetail}}</pre>{{end}}
{{if .TurnLog}}<h2>Turn Log</h2><pre>{{.TurnLog}}</pre>{{end}}
{{end}}
{{template "agent_foot.html" .}}
{{end}}

{{define "agent_scenarios_index.html"}}
{{template "agent_head.html" .}}
{{if .FatalErr}}<div class="outcome-banner outcome-error"><strong>Loader fatal:</strong> {{.FatalErr}}</div>{{end}}
<h2>Registered ({{len .Registered}})</h2>
<table>
<thead><tr><th>ID</th><th>Version</th><th>Side effect</th><th>Tools</th><th>Source</th></tr></thead>
<tbody>
{{range .Registered}}
<tr>
  <td><a href="/admin/agent/scenarios/{{.ID}}">{{.ID}}</a></td>
  <td><span class="muted">{{.Version}}</span></td>
  <td><span class="badge side-effect-{{.SideEffectClass}}">{{.SideEffectClass}}</span></td>
  <td>{{range .AllowedTools}}<code>{{.}}</code> {{end}}</td>
  <td><span class="muted">{{.SourcePath}}</span></td>
</tr>
{{end}}
</tbody>
</table>

<h2>Rejected at load time ({{len .Rejected}})</h2>
{{if .Rejected}}
<table>
<thead><tr><th>Path</th><th>Reason</th></tr></thead>
<tbody>
{{range .Rejected}}<tr><td><code>{{.Path}}</code></td><td>{{.Reason}}</td></tr>{{end}}
</tbody>
</table>
{{else}}<p class="muted">No rejected scenarios.</p>{{end}}
{{template "agent_foot.html" .}}
{{end}}

{{define "agent_scenario_show.html"}}
{{template "agent_head.html" .}}
{{with .Detail}}
<p><a href="/admin/agent/scenarios">&laquo; All scenarios</a></p>
<table style="max-width: 60rem;">
<tbody>
<tr><th>id</th><td>{{.Summary.ID}}</td></tr>
<tr><th>version</th><td>{{.Summary.Version}}</td></tr>
<tr><th>description</th><td>{{.Summary.Description}}</td></tr>
<tr><th>side_effect_class</th><td><span class="badge side-effect-{{.Summary.SideEffectClass}}">{{.Summary.SideEffectClass}}</span></td></tr>
<tr><th>source_path</th><td><code>{{.Summary.SourcePath}}</code></td></tr>
<tr><th>content_hash</th><td><code>{{.Summary.ContentHash}}</code></td></tr>
<tr><th>allowed_tools</th><td>{{range .Summary.AllowedTools}}<code>{{.}}</code> {{end}}</td></tr>
<tr><th>model_preference</th><td>{{.ModelPreference}}</td></tr>
<tr><th>token_budget</th><td>{{.TokenBudget}}</td></tr>
<tr><th>temperature</th><td>{{printf "%.2f" .Temperature}}</td></tr>
<tr><th>limits.max_loop_iterations</th><td>{{.Limits.MaxLoopIterations}}</td></tr>
<tr><th>limits.timeout_ms</th><td>{{.Limits.TimeoutMs}}</td></tr>
<tr><th>limits.schema_retry_budget</th><td>{{.Limits.SchemaRetryBudget}}</td></tr>
<tr><th>limits.per_tool_timeout_ms</th><td>{{.Limits.PerToolTimeoutMs}}</td></tr>
</tbody>
</table>

<h2>Intent examples</h2>
<ul>{{range .IntentExamples}}<li>{{.}}</li>{{end}}</ul>
<h2>System prompt</h2><pre>{{.SystemPrompt}}</pre>
<h2>Input schema</h2><pre>{{.InputSchema}}</pre>
<h2>Output schema</h2><pre>{{.OutputSchema}}</pre>
{{end}}
{{template "agent_foot.html" .}}
{{end}}

{{define "agent_tools_index.html"}}
{{template "agent_head.html" .}}
<table>
<thead><tr><th>Name</th><th>Side effect</th><th>Owning package</th><th>Allowlisted by</th><th>Description</th></tr></thead>
<tbody>
{{range .Rows}}
<tr>
  <td><a href="/admin/agent/tools/{{.Name}}"><code>{{.Name}}</code></a></td>
  <td><span class="badge side-effect-{{.SideEffectBadge}}">{{.SideEffectClass}}</span></td>
  <td>{{.OwningPackage}}</td>
  <td>{{range .AllowlistedByIDs}}{{.}} {{end}}</td>
  <td><span class="muted">{{.Description}}</span></td>
</tr>
{{end}}
</tbody>
</table>
{{template "agent_foot.html" .}}
{{end}}

{{define "agent_tool_show.html"}}
{{template "agent_head.html" .}}
{{with .Detail}}
<p><a href="/admin/agent/tools">&laquo; All tools</a></p>
<table style="max-width: 60rem;">
<tbody>
<tr><th>name</th><td><code>{{.Summary.Name}}</code></td></tr>
<tr><th>side_effect_class</th><td><span class="badge side-effect-{{.Summary.SideEffectBadge}}">{{.Summary.SideEffectClass}}</span></td></tr>
<tr><th>owning_package</th><td>{{.Summary.OwningPackage}}</td></tr>
<tr><th>per_call_timeout_ms</th><td>{{.Summary.PerCallTimeoutMs}}</td></tr>
<tr><th>description</th><td>{{.Summary.Description}}</td></tr>
</tbody>
</table>
<h2>Allowlisted by ({{len .Summary.AllowlistedByIDs}})</h2>
{{if .Summary.AllowlistedByIDs}}
<ul>{{range .Summary.AllowlistedByIDs}}<li><a href="/admin/agent/scenarios/{{.}}">{{.}}</a></li>{{end}}</ul>
{{else}}<p class="muted">No scenarios currently allowlist this tool.</p>{{end}}
<h2>Input schema</h2><pre>{{.InputSchema}}</pre>
<h2>Output schema</h2><pre>{{.OutputSchema}}</pre>
{{end}}
{{template "agent_foot.html" .}}
{{end}}
`
