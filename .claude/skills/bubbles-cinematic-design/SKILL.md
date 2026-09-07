---
name: bubbles-cinematic-design
description: Premium "cinematic" UI design language for Bubbles — aesthetic presets, a fixed premium design system, and a cinematic interaction-pattern library, applied THROUGH the host repo's own UI skill and design tokens. Opt-in per repo. Use when a feature's spec.md `### Design Language` selects cinematic (or the repo default is cinematic) and an agent is wireframing or implementing a premium / flagship UI surface.
---

# Bubbles Cinematic Design Language

## What this is
The premium UI design *vocabulary* that `bubbles.ux` can select and `bubbles.implement` applies: four aesthetic presets, a fixed premium design system, and a library of cinematic interaction patterns. It is a **design-language skill, not an agent** — it carries the knowledge; the gated `bubbles.ux` agent selects it and the `bubbles.implement` agent builds with it. (It replaces the retired `bubbles.cinematic-designer` agent, whose execution flow was identical to `bubbles.implement`.)

## Portability
Portable governance/knowledge skill. It contains NO project-specific commands, hosts, ports, tokens, frameworks, or component names. All concrete materials come from the host repo's own UI skill (see Composition).

## Opt-in (per repo) — NOT loaded by default
This skill is **not vendored or active by default**. A repo enables it by listing `bubbles-cinematic-design` (or `cinematic`) under `designLanguages` in `.github/bubbles-project.yaml`. A repo that has not opted in never receives this skill on disk and uses only its local UI skills.

## Selection + stickiness (resolution precedence)
The active design language for any `bubbles.ux` / `bubbles.implement` run resolves in this order:

1. explicit `design-language:` option on the **current** invocation (override),
2. the feature's recorded `### Design Language` in `spec.md` (**sticky** — set once at planning, detected on every later run),
3. the repo default in `.github/bubbles-project.yaml` `designLanguages.default`,
4. none → local UI skills only.

`bubbles.ux` **writes** the resolved choice into `spec.md` under `## UI Wireframes` → `### Design Language`. `bubbles.implement` **reads** that field and auto-loads this skill. You only re-specify to *change* the language; otherwise every later `ux`/`implement` invocation picks it up implicitly.

## Composition — the project UI skill WINS (NON-NEGOTIABLE)
Apply these patterns **through** the host repo's own UI skill (e.g. `web-ui`, `wa-admin-portal`) and its `ui-design.instructions.md`:

- concrete colors / typography / spacing / radii / shadows / motion come from the repo's design **tokens**, never hardcoded here;
- concrete components come from the repo's **shared component library**;
- if the repo forbids `var(--token, fallback)`, bare hex, or feature-scoped components, honor that — the preset is expressed in the repo's tokens.

This skill supplies the *pattern vocabulary*; the project skill supplies the *materials*. **On any conflict, the project UI skill / instruction wins.**

## When to use
- `bubbles.ux` is selecting a premium / cinematic design language for a feature with UI.
- `bubbles.implement` is building a UI surface whose `spec.md ### Design Language` is cinematic.

## When NOT to use
- Standard / non-premium UI work → use only the repo's local UI skill.
- A repo that has not opted in via `designLanguages` → this skill is absent by design; do not reintroduce it.
- Pure backend / non-UI work.

## Presets + patterns (knowledge)
- Aesthetic presets (A Organic Tech, B Midnight Luxe, C Brutalist Signal, D Vapor Clinic) + the fixed premium design system → see [references/presets.md](references/presets.md).
- Cinematic interaction-pattern library (floating-island navbar, opening-shot hero, functional artifacts, the manifesto, sticky stacking archive, pricing, terminal footer) → see [references/patterns.md](references/patterns.md).

## Rigor tier (orthogonal, optional)
Design language (this skill) and build rigor are **independent axes**. A premium build MAY declare `rigor: premium`, which layers extra verification (visual-regression, motion-perf, screenshot evidence). Those gates are **advisory unless the project wires them as blocking** — the framework defines the tier contract; each repo wires the actual gate (like the Build-Once Deploy-Many invariant). Selecting cinematic does NOT imply premium rigor, and premium rigor does NOT imply cinematic.

## Works well with
- `bubbles-skills-first-discovery` — routes a UI task to the right skill.
- the repo's `web-ui` / `wa-admin-portal` UI skill — supplies the concrete tokens + components.
- `bubbles.ux` — selects the design language and writes `### Design Language`.
- `bubbles.implement` — reads `### Design Language` and builds with this vocabulary.
