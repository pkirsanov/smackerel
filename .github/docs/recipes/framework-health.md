# Recipe: Framework Self-Observation (framework-health)

**Persona:** Jim Lahey (`bubbles.retro`) — but pointed at the framework itself instead of at product specs.
**When to use:** You want Bubbles to tell you which gates fail most, which modes stall, and which capabilities have gone stale.

---

## Just tell super

```
> framework health
> anything broken in bubbles
> improve bubbles
> self observe
```

`bubbles.super` matches via `bubbles/intent-routes.yaml` and dispatches `bubbles.retro` with `target: framework` under workflow mode `framework-health`.

---

## Direct invocation

```
/bubbles.workflow framework-health action:proposal-first
/bubbles.retro target: framework
bash bubbles/scripts/retro-framework-health.sh
```

---

## What it reads

- `.specify/runtime/framework-events.jsonl` — per-line JSON events (agent starts, gate pass/fail, mode lifecycle)
- `.specify/runtime/workflow-runs.json` — per-run records with mode + outcome + duration
- `bubbles/capability-ledger.yaml` — entries with `lastValidated > 90 days` flagged. Consumer freshness on this same ledger is *enforced* by **G127** (`capability-consumer-freshness.sh`): every `state: shipped` capability MUST declare a non-empty `consumers:` list whose every path exists on disk, so a shipped-but-orphaned capability is a blocking finding, not just a retro nudge.

All read-only.

## What it writes

A single proposal at `improvements/IMP-NNN-<slug>.md` containing:

1. **Diagnosis** — top failing gates, stalled/non-completed modes, stale capabilities
2. **Recommended next step** — if signal warrants change, open a normal spec under `specs/`
3. **Provenance** — explicit list of input files; explicit declaration of ZERO framework mutation

In the bubbles source repo, `improvements/` is committed as the canonical proposal archive. In downstream repos, it is gitignored by default.

## What it never does

- Mutate `bubbles/*` files
- Mutate `agents/*` files
- Mutate `bubbles/workflows.yaml`
- Touch any file outside `improvements/`

The selftest enforces this with a sentinel-mtime check. G125 (framework-health-evidence) enforces the proposal output.

## Why proposal-first

Framework evolution is high-trust. Even when the data shows "G123 has failed 47 times this month", auto-mutating the gate would be a self-modifying system without a human-in-the-loop. Proposal-first preserves the human boundary.

If you read the proposal and agree:

```
/bubbles.workflow implement action:full-delivery target:spec
> implement IMP-NNN: <slug>
```

Then it becomes a normal spec with the usual gates.

---

## Promote a recurring lesson to a skill

Framework self-observation has a second, narrower loop that runs alongside the retro proposal pass. Where the retro pass turns runtime-event signal into an `improvements/` proposal, the **Skill Evolution Loop** turns a *repeated lesson* into a reusable skill — so agents stop relearning the same thing.

```
bash bubbles/scripts/cli.sh skill-proposals           # show pending proposals
bash bubbles/scripts/cli.sh skill-proposals --dismiss   # clear them
```

The flow:

1. **Repetition detected** — `skill-evolution.sh` counts exact-normalized repeated lines in `.specify/memory/lessons.md`. At `triggerThreshold` (default 3) it writes a proposal to `.specify/memory/skill-proposals.md`. *"Same greasy mistake three times, boys."*
2. **Quality bar** — each proposal carries the creation bar **Reusable · Non-trivial · Specific · Verified**. A one-off or unverified lesson does not qualify.
3. **Dedup before create** — search the existing `.github/skills/` set and `skills/INVENTORY.md` first; prefer UPDATING a near-match over standing up a duplicate. When the skill set is large, review the least-recently-modified skills for deprecation (anti-hoarding) — promotion and pruning are the two ends of one lifecycle.
4. **Decision rule** — *do it once → a prompt is fine; recurring + non-obvious + verified → promote to a skill.*
5. **Author** — when a proposal clears the bar, `bubbles.create-skill` scaffolds the new `SKILL.md` (including the **When NOT to use** and **Works well with** sections) and records it in `skills/INVENTORY.md`.

Like the retro proposal pass, this loop is proposal-first: it never auto-creates a skill. You review the proposal, then author with `bubbles.create-skill`. See the `bubbles-skill-authoring` skill for the full template + quality-bar contract.

---

## Quote

> *"The liquor helps me see the patterns, Randy."* — Jim Lahey
