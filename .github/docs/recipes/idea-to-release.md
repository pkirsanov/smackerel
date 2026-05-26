# <img src="../../icons/bubbles-glasses.svg" width="28"> Recipe: Idea → Release Completion

> *"From the napkin to the next park release. We don't ship liquor without inventory, boys."* — Sonny "Iron Lung" Smith

## The Situation

You have a raw idea — a new capability, a phase theme, an investor commitment, or a competitive response — and you want it to flow end-to-end through the framework: brainstorm → release packet bootstrap → spec/design/scopes → implementation → validation → audit → release packet refresh ("delivered" status update with evidence trace) → finalize.

The standard `product-to-delivery` and `new-feature` recipes drop you off at "code shipped, audit clean." They do not loop back to the release packet, so `features.md` and the `INVESTOR_OVERVIEW.md` Phase Overview table go stale every time you ship. This recipe closes that loop.

## Quick Start

```
/bubbles.workflow  mode: idea-to-release-completion phase: <phase-id> idea: <your idea in plain English>
```

That single command walks the full lifecycle: explore → bootstrap packet → decompose into specs → deliver → certify → refresh packet → update Phase Overview → finalize.

## Manual / Step-By-Step Path

If you want to drive each step yourself instead of running the chained mode, follow this sequence.

### Step 1 — Make sure the Product Direction Surfaces trio exists

`bubbles.releases` refuses to run without the trio. Check for:
- `docs/INVESTOR_OVERVIEW.md` (capability ledger + Phase Overview table)
- `docs/Product-Principles.md` (ratified principles)
- `.github/instructions/product-principles.instructions.md` (agent-facing enforcement)

If any are missing:

```
/bubbles.setup  bootstrap product direction trio
```

### Step 2 — Explore or grill the idea (optional but recommended)

```
/bubbles.workflow  mode: brainstorm for <your idea>
```

or

```
/bubbles.grill  pressure-test <your idea> before we commit to a phase
```

These steps stay diagnostic. They produce findings, scenarios, and direction — they do not change the codebase or the release packet.

### Step 3 — Bootstrap (or refresh) the release packet for the target phase

```
/bubbles.releases  <phase-id> mode: bootstrap
```

or, if the packet already exists:

```
/bubbles.releases  <phase-id> mode: refresh
```

This produces or reconciles the canonical 8-doc release packet:
`vision.md`, `features.md`, `actions.md`, `business-plan.md`, `deployment.md`, `marketing.md`, `monetization.md`, `ops-scalability.md`.

The new capability lands in `features.md` with status `planned` (or `in-progress` if specs already exist). The carry-forward table is enforced. Open questions flow into `actions.md` instead of being asked of you.

### Step 4 — Decompose the planned capability into specs

```
/bubbles.workflow  mode: product-to-delivery for <feature-name>
```

`bubbles.analyst` reads the new release packet's `vision.md` + `features.md` row for this capability, then produces `spec.md`. `bubbles.ux` adds the wireframes when UI is in scope. `bubbles.design` drafts `design.md`. `bubbles.plan` decomposes into scopes with Gherkin scenarios and DoD.

Capability-foundation checkpoint: if the idea introduces a reusable capability, a second provider/adapter/variant, or shared UI/data/contract surfaces, the planning chain must produce the Domain Capability Model, Capability Foundation, Concrete Implementations, Variation Axes, UI Primitives where applicable, and foundation-before-overlay scope dependencies before implementation starts.

### Step 5 — Implement the scopes through full delivery

The `product-to-delivery` mode invoked above continues automatically: `implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → chaos → finalize`.

If you want a tighter loop without the heavy hardening tail:

```
/bubbles.workflow  mode: full-delivery for <feature-name>
```

### Step 6 — Refresh the release packet to mark the capability "delivered"

This is the step the framework historically forgot. After audit certifies the spec as done:

```
/bubbles.releases  <phase-id> mode: refresh
```

Sonny re-reads the capability ledger and the spec's `state.json`, finds the new `done` evidence, then updates:
- `features.md` — capability row moves from `planned` / `in-progress` to `delivered` with the spec ID and evidence link.
- `actions.md` — items that resolved with this delivery move to "Closed."
- `INVESTOR_OVERVIEW.md` Phase Overview table — counters update, capability state lifts.
- `business-plan.md` / `marketing.md` / `monetization.md` — only refreshed if claims explicitly cited this capability.

This is the moment a release packet stops lying about reality.

### Step 7 — Finalize

```
/bubbles.workflow  finalize <feature-name>
```

The finalize phase records the cross-spec audit trail and certifies that the release packet now matches delivered reality.

### Step 8 — Optional: cross-product coordination

If the capability touches sibling product repos (shared schemas, shared deployment surface, shared evidence packets):

```
/bubbles.releases  <phase-id> mode: cross-product
```

Sonny produces or refreshes the cross-product traceability table inside the packet so the other repos can see what shipped here and what they need to consume.

## What `idea-to-release-completion` Does Automatically

When you use the chained mode in Quick Start, the orchestrator runs:

| Phase | Owner | Output |
|-------|-------|--------|
| `analyze` (optional) | `bubbles.analyst` | Capability framing, competitive context, actor/use-case model |
| `releases` (bootstrap) | `bubbles.releases` | Phase release packet drafted; new capability declared `planned` |
| `select` | `bubbles.workflow` | Resolves the spec target the new capability decomposes into |
| `bootstrap` | `bubbles.design` + `bubbles.plan` | `spec.md`, `design.md`, `scopes.md` for the new capability |
| `implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → chaos` | Specialist agents | Standard delivery + hardening chain |
| `releases` (refresh) | `bubbles.releases` | `features.md` capability flips to `delivered`; Phase Overview updated |
| `finalize` | `bubbles.workflow` | Cross-spec audit trail + certification |

## Tips

- The `releases` phase appears TWICE in this mode: once at the start to declare intent, once at the end to record reality. Both are owned exclusively by `bubbles.releases` (Sonny). No other agent is allowed to mutate `features.md`.
- If the trio is missing, the chained mode aborts at the start and routes to `/bubbles.setup`. Fix the trio first, then retry — there is no opt-out.
- `bubbles.releases` will NEVER fabricate a delivered claim. If audit did not certify the spec as done, the refresh step leaves the capability at `in-progress` — the packet stays honest.
- For a phase with multiple capabilities shipping in parallel, run this recipe per capability, OR use `/bubbles.sprint  minutes: N` with one goal per capability and let the sprint controller iterate. Sonny's refresh step deduplicates updates across capabilities.
- When devops changes the deployment surface during step 5, `bubbles.devops` flags the affected packet phase and recommends `runSubagent(bubbles.releases): refresh <phase> deployment`. This handoff already lives in the framework — it's covered by the [DevOps + Release Coordination](devops-release-coordination.md) recipe.

## When To Use This Recipe Instead Of New Feature

- **Use [New Feature](new-feature.md)** when the work is a single feature inside an existing already-planned phase — the release packet already lists it as planned, you just need to ship it.
- **Use this recipe (Idea → Release Completion)** when the work is a NEW capability that does not yet appear in any release packet, OR when the packet exists but its capability ledger is out of date with what's actually shipped.
- **Use [Release Planning](release-planning.md)** when you only want the planning side (refresh the packet, no delivery work).

## Related Recipes

- [New Feature](new-feature.md) — Idea → shipped code (no release packet step)
- [Brainstorm an Idea](brainstorm-idea.md) — Pre-commitment exploration
- [Explore an Idea](explore-idea.md) — Flesh out a vague idea
- [Release Planning](release-planning.md) — Packet authoring only (no implementation)
- [DevOps + Release Coordination](devops-release-coordination.md) — Devops handoff to release packet
- [Set Up a New Project](setup-project.md) — Bootstrap the trio if missing
- [Autonomous Goal](autonomous-goal.md) — Autonomous variant for a single capability
- [Autonomous Sprint](autonomous-sprint.md) — Autonomous variant for multiple capabilities under a time budget

---

*"Now the books match the bottles. That's how you run a park."* — Sonny "Iron Lung" Smith
