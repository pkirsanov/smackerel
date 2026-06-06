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
- `bubbles/capability-ledger.yaml` — entries with `lastValidated > 90 days` flagged

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

## Quote

> *"The liquor helps me see the patterns, Randy."* — Jim Lahey
