# Recipe: UX Single-File Sweep

> *"One job, Bubbles. One job."*

---

## The Problem

UX cleanup has a magnetic field. You open one form to fix the placeholder text and walk out three days later having re-architected the whole settings module, broken two unrelated tests, and shipped half a redesign nobody asked for. Reviewers find adjacent issues; you fix them; reviewers find more. Scope balloons, the PR grows past review-ability, and the original UX issue is now buried under "while I'm here" changes.

The fix is a discipline: **one pass, one file, one outcome surface.**

## The Pattern

A UX single-file sweep has exactly three constraints:

1. **ONE pass** — read the file once, list everything that needs to change, then make the changes in a single coherent edit. No second-pass discoveries.
2. **ONE file** — one user-facing surface (a CLI subcommand handler, a single page component, a single CRUD form, a single error template). Touching imports of that file's dependencies is allowed; modifying the dependencies is not.
3. **ONE outcome surface** — what the end user sees changes. Internal refactors that are invisible to the user belong in a [`bubbles.simplify`](../../agents/bubbles.simplify.agent.md) pass, not a UX sweep.

If you find yourself wanting to break any of these three, the answer is to file a follow-up ticket — not to expand the sweep.

## Sweep Checklist

Walk the target file and answer each question. Anything that gets a "no" is in scope:

- **Error messages** — Are they user-readable (not stack traces, not internal codes)? Do they say what to do next?
- **Defaults** — Is the most common case the default? Is the default safe (won't delete data, won't bill, won't email everyone)?
- **Help text** — Does it match the actual current behavior? Is anything stale or aspirational?
- **Naming** — Are flag names, button labels, and headings consistent with the rest of the surface? Plural vs. singular consistent? Verb tense consistent?
- **Loading state** — Is there one? Does it appear within ~100ms?
- **Empty state** — Does the surface explain itself when there's no data, or just show a blank?
- **Disabled state** — When a control is disabled, is the reason visible (tooltip, hint text)?
- **Confirmation** — Are destructive actions confirmed once and only once? No double-confirms, no missing confirms.
- **Keyboard / a11y** — Tab order sane? Focus visible? Labels associated with inputs?

If the file is a CLI subcommand: substitute "exit codes are documented", "`--help` matches the implementation", "errors go to stderr, data goes to stdout" for the GUI-specific items.

## Workflow

```text
/bubbles.ux  sweep dashboard/src/features/billing/CancelSubscription.tsx
```

Or, if the file already has identified UX issues filed as scopes:

```text
/bubbles.workflow  ux-sweep for 047-billing-cancel
```

The [`bubbles.ux`](../../agents/bubbles.ux.agent.md) agent reads the file, applies the checklist, produces a wireframe diff (or text diff for CLI), and routes follow-ups to `bubbles.implement` or `bubbles.simplify` as appropriate.

## Out-of-Scope Policy

Adjacent issues become **followup tickets**, not patches inside the sweep. The discipline is enforced by the change boundary:

- Issue is in a different file → file a follow-up.
- Issue requires a refactor across multiple components → file a follow-up; flag it as a `simplify` candidate.
- Issue is the root cause of the UX problem and lives elsewhere → STOP the sweep, fix the root cause first, then resume.

The follow-up ticket MUST cite the sweep PR. That backlinks the discovery and prevents the same issue from being "discovered" again next sweep.

## Anti-Patterns

- **"While I'm here" creep.** The single most common failure mode. The instant you notice an unrelated issue, write the follow-up ticket and keep walking.
- **UX sweep + refactor in one PR.** UX changes are user-visible; refactors are not. Bundling them makes review harder and ties a behavior change to an internal change for no reason.
- **UX sweep + new feature in one PR.** Sweeps polish what exists. New features are `bubbles.plan` work. Mixing them turns a low-risk polish PR into a high-risk feature PR.
- **Sweep without a checklist.** Without the checklist, "UX sweep" degenerates into "I changed some words around." The checklist is what makes the pass repeatable and reviewable.
- **Sweeping a file that's about to be deleted.** Check the roadmap first. Polishing a doomed surface is wasted craft.

## References

- [`bubbles.ux` agent](../../agents/bubbles.ux.agent.md) — ASCII wireframes, interaction flows, accessibility patterns, design system compliance.
- [`bubbles.simplify` agent](../../agents/bubbles.simplify.agent.md) — for invisible refactors that emerge from a sweep.
- [Quality Sweep recipe](quality-sweep.md) — broader cousin for non-UX surfaces.
- [`bubbles/workflows/modes.yaml`](../../bubbles/workflows/modes.yaml) — the canonical workflow-mode registry (v6.1 split this out of `workflows.yaml`).
- [`bubbles/workflows.yaml`](../../bubbles/workflows.yaml) — gates + phases, including the Change Boundary discipline (state-transition-guard Check 8D).
