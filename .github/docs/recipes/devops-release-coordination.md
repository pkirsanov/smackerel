# Recipe: DevOps + Release Coordination

> *"Two hands on the same wheel, boys. Tommy steers the rack, Sonny steers the story, and the park rolls forward without runnin' over the curb."* — Mr. Lahey.

---

## The Situation

You just shipped a devops change — a new deploy target, a new cosign flow, a restructured config bundle, a new rollback strategy, a registry move — and the release packet for the current phase claims a deployment story that no longer matches reality. The `deployment.md` doc inside `docs/releases/<phase>/` is now stale.

You need both updates to land coherently:

- The **technical surface** (adapters, CI workflow, signing flow, bundle layout) is owned by `bubbles.devops` (Tommy Bean).
- The **release narrative** (`deployment.md` inside the phase packet) is owned by `bubbles.releases` (Sonny "Iron Lung" Smith).

Tommy never edits packet docs. Sonny never edits adapter code. The coordination happens in a fixed two-step pattern with a validation handshake at the end.

## Quick Start — Natural Language

```
/bubbles.devops    focus: deployment-target <change description>
/bubbles.releases  refresh <phase> deployment
```

If the framing is ambiguous (you are not sure which phase the change affects, or whether the change is a single-target tweak vs a cross-product shift):

```
/bubbles.super  I just shipped a devops change and I think the release packet is stale
```

The super resolves the framing into the right two-step chain and identifies which phase packet (or packets) need refreshing.

## The Two-Step Pattern

**Step A — Tommy executes the technical change.**

`bubbles.devops` ships the deployment-target work, the cosign change, the bundle restructuring, or whatever the technical delta is. In his result envelope he flags any phase packet whose `deployment.md` claims a story that the new technical surface invalidates. This flagging is required by his cross-handoff clause whenever he ships work that changes how the project is deployed.

Tommy does NOT touch `docs/releases/<phase>/deployment.md`. That is a packet doc and it belongs to Sonny.

**Step B — Sonny refreshes ONLY the `deployment.md` packet doc.**

`bubbles.releases` is invoked with `refresh <phase>` scoped to the deployment doc. He reads the new technical surface, drafts the narrative, and cites the canonical surfaces (`bubbles-deployment-target-adapter` skill, state-gate G081, the new adapter directory). He does NOT touch deploy adapter code. That is technical implementation and it belongs to Tommy.

**Validation handshake.**

Sonny calls `runSubagent(bubbles.devops): focus: deployment-target validate this packet doc against current reality` for technical-accuracy validation. Only AFTER Tommy returns `completed_owned` confirming the narrative matches the deployed reality does Sonny mark the packet doc done. The validation handshake is what keeps the packet honest — it is the difference between "Sonny wrote a plausible story" and "Sonny wrote a story Tommy will defend."

## The Steps (Manual Control)

### Step 1 — Run The DevOps Change

Pick the right invocation for the technical work. Common cases:

| Technical Change | Invocation |
|------------------|------------|
| New deployment target | `/bubbles.devops focus: deployment-target add a new target named <name>` |
| New cosign / signing flow | `/bubbles.devops focus: ci-cd update CI signing flow to <new approach>` |
| Restructured config bundle | `/bubbles.devops focus: config-sst restructure config bundle layout for <reason>` |
| New rollback strategy | `/bubbles.devops focus: deployment-target switch <target> rollout strategy to <strategy>` |
| Registry move | `/bubbles.devops focus: ci-cd move image publication to <new registry>` |

Capture the affected phase(s) from Tommy's result envelope. He flags them under his cross-handoff clause; do not skip reading them.

### Step 2 — Invoke Sonny Scoped To Deployment Doc Only

```
/bubbles.releases  refresh <phase> deployment docs: deployment
```

The `docs: deployment` scope is mandatory. If you omit it, Sonny may refresh the entire packet (vision, features, marketing, etc.) and that is more change than the situation requires.

For multi-phase impact, repeat the invocation per phase:

```
/bubbles.releases  refresh <phase-a> deployment docs: deployment
/bubbles.releases  refresh <phase-b> deployment docs: deployment
```

### Step 3 — Sonny Drafts The Narrative

Sonny reads the new technical surface (the changed adapter directory, the updated CI workflow, the new bundle layout), drafts the narrative inside `docs/releases/<phase>/deployment.md`, and cites the canonical surfaces:

- The `bubbles-deployment-target-adapter` skill (link to the section that documents the change)
- State-gate **G081 (Build-Once Deploy-Many Integrity)** (in `agents/bubbles_shared/state-gates.md`)
- The new or changed adapter directory (`deploy/<target>/`)
- The new or changed CI job (if applicable)

The honest-capability rule applies: any "delivered" claim in the narrative MUST trace to a spec marked done in `state.json`, a row in `Capability_Ledger.md` if the project uses one, or a runbook section explicitly marked delivered. Marketing copy that fabricates capability is REFUSED.

### Step 4 — Sonny Calls Tommy For Technical-Accuracy Validation

```
runSubagent(bubbles.devops): focus: deployment-target validate this packet doc against current reality
```

Tommy reads the drafted narrative against the actual technical surface and returns one of:

- `completed_owned` — narrative matches reality; Sonny marks the packet doc done
- `route_required` — narrative claims something the technical surface does not actually do; Sonny revises and re-validates
- `blocked` — there is a real gap between the narrative's intent and what is shippable; Tommy explains the blocker, Sonny escalates

Only after Tommy returns `completed_owned` does Sonny mark the packet doc done.

## Anti-patterns (FORBIDDEN)

| Anti-pattern | Why It Breaks Coordination |
|--------------|----------------------------|
| `bubbles.devops` editing `docs/releases/<phase>/deployment.md` directly | ROLE VIOLATION — Tommy is the technical owner; packet docs are Sonny's surface. Tommy editing the packet doc means there is no independent narrative review. |
| `bubbles.releases` editing deploy adapter code (anything under `deploy/<target>/`, `.github/workflows/build.yml`, `scripts/deploy/`) | ROLE VIOLATION — Sonny is the narrative owner; technical surfaces are Tommy's. Sonny editing adapter code means there is no independent technical review. |
| Refreshing the packet without devops technical-accuracy validation | BLOCKING — the narrative may describe a deployment story that no longer matches reality. The validation handshake is what catches this. |
| Refreshing the packet doc with marketing language about "secure deploys" without citing G081 | Fails the honest-capability rule. "Secure deploys" is unverifiable copy; "G081-compliant pipeline with cosign keyless signing, SBOM + SLSA attestations, and per-target adapter pointer-swap rollback" is verifiable copy. |
| Skipping Step A and refreshing the packet doc first to "get ahead" | The narrative will describe the OLD technical surface; you will rewrite it once Tommy ships the change. Always Step A first. |
| Running both invocations in the same agent session without the validation handshake | The handshake is the gate. Without it, the packet ships unreviewed. |

## Common Modes

| Intent | Invocation Pattern |
|--------|-------------------|
| Single-target change → single-phase packet refresh | `bubbles.devops focus: deployment-target <change>` then `bubbles.releases refresh <phase> deployment docs: deployment` |
| Multi-target change → multi-phase packet refresh | One devops invocation; repeat `bubbles.releases refresh <phase-N> deployment docs: deployment` for each affected phase |
| Cross-product devops change (project A and a paired companion both change) | One devops invocation per project; one `bubbles.releases coordinate <phase> across this repo and a paired repo paired_repo: <path>` to refresh both packets coherently |
| Devops change that introduces a new principle (e.g., new cross-product boundary surfaced by the deploy change) | Add `/bubbles.analyst surface a new product principle for <description>` BETWEEN Step 1 and Step 2 — packet narrative cites the new principle |

## Related Recipes

- [`devops-work.md`](devops-work.md) — Focused devops execution lane and the full set of `bubbles.devops` focus values
- [`build-once-deploy-many.md`](build-once-deploy-many.md) — The pipeline shape the deployment narrative must reflect
- [`release-planning.md`](release-planning.md) — Sonny's full release packet authoring workflow (this recipe is the deployment-doc-only subset)
- [`add-deployment-target.md`](add-deployment-target.md) — The most common Step-A trigger for this coordination recipe

## Related Skills

- [`bubbles-deployment-target-adapter`](../../skills/bubbles-deployment-target-adapter/SKILL.md) — The canonical surface Sonny cites when drafting the narrative
- [`bubbles-product-principle-discovery`](../../skills/bubbles-product-principle-discovery/SKILL.md) — When the devops change surfaces a new product principle that needs to land before the packet refresh

## Related Convention

- [`docs/guides/PRODUCT_DIRECTION_SURFACES.md`](../guides/PRODUCT_DIRECTION_SURFACES.md) — The convention `bubbles.releases` enforces; the `deployment.md` packet doc lives inside this surface

## Related Gate

- **G081 (Build-Once Deploy-Many Integrity)** — `agents/bubbles_shared/state-gates.md`
