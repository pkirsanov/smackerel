// Spec 058 BUG-058-EXTERNAL-INFRA-MISSING (BLOCKER-3) — shared admin web
// scaffolding.
//
// This package is the reusable server-rendered admin-UI foundation the spec 058
// devices view (and future admin pages) build on. It mirrors the established
// internal/web/agent_admin.go convention (Go html/template, no external
// template engine, no static-HTML-embed shortcut) but factors the shared chrome
// — a single base layout, a navigation fragment, and an auth-gating helper —
// into one place so every admin page renders consistently and gates access the
// same way.
//
// Why a shared scaffold and not per-page static HTML: spec 058 design §3.2
// requires the extension-devices admin view as a rendered surface (the JSON
// handler at /v1/admin/extension/devices already exists). Rather than embed
// another bespoke HTML blob, the scaffold gives admin pages one composable base
// + nav + gate, which is the "generalization" the BUG-058 HTMX-admin blocker
// called for.
package admin

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/smackerel/smackerel/internal/api/admin/extensiondevices"
)

// AuthGate decides whether the caller may view an admin page and, for non-admin
// callers, which owner_user_id their view is scoped to. It is the exact shape
// of extensiondevices.AdminPredicate so the production wiring can pass the same
// callerIsAdmin closure it already builds for the JSON handler (no second auth
// primitive, no drift).
type AuthGate = extensiondevices.AdminPredicate

// NavLink is one entry in the shared admin navigation fragment.
type NavLink struct {
	Label  string
	Href   string
	Active bool
}

// navLinks returns the canonical admin navigation, marking activeHref active.
// The set is intentionally static: the admin surface is a small, fixed set of
// operator pages, so a hardcoded nav is correct (this is UI chrome, not a
// runtime business value — no SST concern).
func navLinks(activeHref string) []NavLink {
	links := []NavLink{
		{Label: "Agent Traces", Href: "/admin/agent/traces"},
		{Label: "Auth Tokens", Href: "/admin/auth/tokens"},
		{Label: "Extension Devices", Href: "/admin/extension/devices"},
	}
	for i := range links {
		if links[i].Href == activeHref {
			links[i].Active = true
		}
	}
	return links
}

// pageModel is the data passed to the base layout. Content is the
// already-rendered inner HTML for the active page; ActiveHref drives the nav
// highlight.
type pageModel struct {
	Title      string
	ActiveHref string
	Nav        []NavLink
	Content    template.HTML
}

// baseLayout is the shared chrome: head + nav + a content slot. Individual
// pages render their own inner template to HTML and hand it to renderPage,
// which wraps it in this layout. Keeping the layout as a single parsed template
// (rather than one self-contained template per page, as agent_admin.go does)
// is what makes the chrome genuinely shared.
const baseLayout = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — Smackerel Admin</title>
<style>
body { font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 1rem; background: #fafafa; color: #222; }
nav { background: #1f2937; color: white; padding: 0.6rem 1rem; margin: -1rem -1rem 1rem -1rem; }
nav strong { margin-right: 1rem; }
nav a { color: #cbd5e1; margin-right: 1rem; text-decoration: none; }
nav a:hover { text-decoration: underline; color: white; }
nav a.active { color: white; font-weight: 600; }
h1 { font-size: 1.4rem; margin-top: 0; }
table { border-collapse: collapse; width: 100%; background: white; }
th, td { border: 1px solid #e5e7eb; padding: 0.4rem 0.6rem; text-align: left; vertical-align: top; }
th { background: #f3f4f6; font-weight: 600; }
.muted { color: #6b7280; font-size: 0.9em; }
.empty { padding: 1rem; background: white; border: 1px solid #e5e7eb; color: #6b7280; }
code { background: #f3f4f6; padding: 0.05rem 0.3rem; border-radius: 0.2rem; }
</style>
</head>
<body>
<nav>
  <strong>Smackerel Admin</strong>
  {{range .Nav}}<a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a>{{end}}
</nav>
<h1>{{.Title}}</h1>
{{.Content}}
</body>
</html>
`

// newBaseTemplate parses the shared base layout once. Pages embed their own
// inner template separately and pass the rendered HTML via pageModel.Content.
func newBaseTemplate() *template.Template {
	return template.Must(template.New("admin_base").Parse(baseLayout))
}

// renderContent executes an inner page template to an html/template.HTML value.
// Because the inner template is itself an html/template, every interpolated
// field is auto-escaped before it becomes trusted HTML — so wrapping the result
// in template.HTML for the base layout does NOT bypass escaping.
func renderContent(inner *template.Template, data any) (template.HTML, error) {
	var sb strings.Builder
	if err := inner.Execute(&sb, data); err != nil {
		return "", err
	}
	return template.HTML(sb.String()), nil
}

// renderPage writes the base layout wrapping the supplied inner content.
func renderPage(w http.ResponseWriter, base *template.Template, title, activeHref string, content template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = base.Execute(w, pageModel{
		Title:      title,
		ActiveHref: activeHref,
		Nav:        navLinks(activeHref),
		Content:    content,
	})
}
