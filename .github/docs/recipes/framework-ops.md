# Recipe: Framework Operations

> *"Ask the super first. We'll figure out the right move before we make a mess of this."*

Use `bubbles.super` when the problem is about the Bubbles framework itself: health, hooks, gates, metrics, framework validation, release hygiene, repo-readiness guidance, upgrades, or recovering from a framework-level problem. If you need broader prompt help first, use the dedicated [Ask the Super First](ask-the-super-first.md) recipe.

If the work is inside a target project's CI/CD, deployment, monitoring, or build surfaces, use [DevOps Work](devops-work.md) instead. If that work is cross-cutting and not feature-owned, use [Ops Packet Work](ops-packet-work.md). Framework ops is for Bubbles itself, not application delivery plumbing.

**Scope rule:** Bubbles-managed git hooks are for the Bubbles framework repo only. Consumer repos use installed Bubbles files, but they must not install Bubbles-managed `pre-commit` or `pre-push` hooks.

**Write rule:** Consumer repos must not directly edit `.github/bubbles/**`, `.github/agents/bubbles*`, `.github/prompts/bubbles*`, `.github/instructions/bubbles-*`, or other framework-managed Bubbles files. If a repo needs a framework change, it must record a proposal in `.github/bubbles-project/proposals/` or run `bubbles framework-proposal <slug>`, then make the real change in the Bubbles source repo.

**Interop rule:** Review-only interop intake is project-owned. `bubbles interop import --review-only` may snapshot and normalize Claude Code, Roo Code, Cursor, or Cline assets into `.github/bubbles-project/imports/**`, and it may emit project-owned proposals under `.github/bubbles-project/proposals/**` when imported workflow concepts would require framework-managed Bubbles changes. It must never write directly to `.github/bubbles/**`, `.github/agents/bubbles*`, `.github/prompts/bubbles*`, or `.github/skills/bubbles-*`.

## Check Project Health

Refresh the framework-owned setup first when you have just installed or upgraded Bubbles:

```
/bubbles.setup  mode: refresh
```

Then run the broader health checks:

```
/bubbles.super  check my project health and fix any issues
```

Or via CLI:
```bash
bash .github/bubbles/scripts/cli.sh doctor --heal
bash .github/bubbles/scripts/cli.sh agnosticity
bash .github/bubbles/scripts/cli.sh interop detect
bash .github/bubbles/scripts/cli.sh interop import --review-only
bash .github/bubbles/scripts/cli.sh interop status
bash .github/bubbles/scripts/cli.sh framework-write-guard
bash .github/bubbles/scripts/cli.sh framework-validate
bash .github/bubbles/scripts/cli.sh framework-events --tail 20
bash .github/bubbles/scripts/cli.sh run-state --all
bash .github/bubbles/scripts/cli.sh repo-readiness . --deep
bash .github/bubbles/scripts/cli.sh guard-selftest
```

`guard-selftest` exercises the framework's promotion guard with temporary fixtures so you can verify the concrete-result and child-workflow enforcement paths without mutating real specs.

`framework-validate` is the framework's self-check surface. Use it when you want the portable-surface, ownership, registry, and selftest surfaces verified as a bundle.

`framework-events` exposes the typed framework event stream, and `run-state` shows the active and recent workflow-run records that make resume and runtime attachment explicit.

## Skills-First Discovery (v4.0+)

The framework ships discovery skills that route agents and humans to the right governance module on demand instead of bulk-loading `agent-common.md`. When an agent (or you) is unsure which Bubbles rule applies, start at [`skills/bubbles-skills-first-discovery/SKILL.md`](../../skills/bubbles-skills-first-discovery/SKILL.md). It maps common situations to:

- [`bubbles-anti-fabrication`](../../skills/bubbles-anti-fabrication/SKILL.md) — before marking any DoD `[x]` or claiming a command passed
- [`bubbles-evidence-capture`](../../skills/bubbles-evidence-capture/SKILL.md) — recording terminal output ≥10 lines
- [`bubbles-dod-validation`](../../skills/bubbles-dod-validation/SKILL.md) — Tier 1/Tier 2 pre-completion audit
- [`bubbles-status-transition`](../../skills/bubbles-status-transition/SKILL.md) — `state.json` changes and grandfather clause
- [`bubbles-result-envelope`](../../skills/bubbles-result-envelope/SKILL.md) — end-of-run packet shape
- [`bubbles-artifact-ownership-routing`](../../skills/bubbles-artifact-ownership-routing/SKILL.md) — own it or route it
- [`bubbles-quality-gates-catalog`](../../skills/bubbles-quality-gates-catalog/SKILL.md) — gate ID lookup
- [`bubbles-scope-workflow-runtime`](../../skills/bubbles-scope-workflow-runtime/SKILL.md) — scope structure + DoD shape

Skills are discovery shims. The authoritative policy still lives in `agents/bubbles_shared/*.md` and the mechanical guards in `bubbles/scripts/`. Skills do not change the grandfather clause for historical `done` specs.

When you are in a downstream repo, `doctor` and `framework-write-guard` now consume `.github/bubbles/release-manifest.json` plus `.github/bubbles/.install-source.json` so the trust story stays explicit:
- installed version and upstream git SHA
- install mode (`remote-ref` vs `local-source`)
- symbolic source ref
- dirty local-source risk when applicable
- managed-file integrity against `.github/bubbles/.checksums`

## Run Release Hygiene Checks

Use this in the Bubbles source repo before packaging or publishing framework changes:

```bash
bash bubbles/scripts/cli.sh release-check
```

`release-check` runs framework validation first, confirms the expected release docs are present, and blocks shipment when stray temp or backup files are still in the tree.

This is a framework-source operation, not a downstream repo command.

## Install Git Hooks

```
/bubbles.super  install all git hooks in the Bubbles framework repo
```

This installs:
- **pre-commit:** Portable-surface agnosticity lint plus fast artifact lint on staged spec files
- **pre-push:** Full portable-surface agnosticity lint plus changed-spec artifact checks and current-policy guard on changed or newly promoted done specs

Manual historical recertification remains available through `bash bubbles/scripts/cli.sh audit-done --recertify-all`; routine hooks do not recertify every historical done spec.

## Add a Custom Quality Gate

```
/bubbles.super  add a pre-push gate that checks for license compliance using scripts/license-check.sh
```

This creates the entry in `.github/bubbles-project.yaml` and registers the hook.

## Propose A Framework Change From A Consumer Repo

```
/bubbles.super  create a proposal for a Bubbles framework change called tighter-framework-write-guard
```

Or via CLI:

```bash
bash .github/bubbles/scripts/cli.sh framework-proposal tighter-framework-write-guard
```

This creates a project-owned proposal under `.github/bubbles-project/proposals/`. The actual framework edit still belongs in the Bubbles source repo.

## Inspect The Effective Managed-Doc Registry

```bash
bash .github/bubbles/scripts/cli.sh docs-registry effective
```

Use this when a project's managed-doc layout differs from the framework default and you need to confirm how framework defaults plus project-owned overrides resolve in practice.

## Upgrade Bubbles

```
/bubbles.super  upgrade bubbles to latest
```

Or preview first:
```
/bubbles.super  upgrade --dry-run
```

The dry-run path is now a trust preview, not just a file-overwrite preview. It compares the current installed trust metadata with the target release manifest and distinguishes:
- framework-managed files that will be replaced
- project-owned files that will not be touched
- profile or interop support changes between current and target manifests
- trust warnings such as dirty local-source provenance or existing managed-file drift

If you are validating a local source checkout before refreshing a downstream repo, preview that exact checkout explicitly:

```bash
bash .github/bubbles/scripts/cli.sh upgrade --dry-run --local-source /path/to/bubbles
```

That dry-run will warn if the local checkout is dirty so maintainers do not mistake a working-tree snapshot for a clean published release.

## Scope Dependency Visualization

```
/bubbles.super  show the dependency graph for spec 042
```

Outputs a Mermaid diagram showing scope dependencies with completion status.

## Enable Metrics

```
/bubbles.super  enable metrics tracking
```

After enabling, governance scripts log events to `.specify/metrics/events.jsonl`. View with:
```
/bubbles.super  show metrics summary
```

## Coordinate Runtime Resources Across Sessions

Use the runtime lease surface when multiple sessions may start or reuse Docker/Compose stacks:

```bash
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose validation --share-mode shared-compatible --fingerprint-file docker-compose.yml --resource container:backend
bash .github/bubbles/scripts/cli.sh runtime leases
bash .github/bubbles/scripts/cli.sh runtime doctor
bash .github/bubbles/scripts/cli.sh runtime release <lease-id>
```

Typical flow:
- acquire before starting a shared stack
- heartbeat or attach if another session reuses the same compatible runtime
- doctor before cleanup if sessions appear to be colliding
- release when the owning session is done

To stop two heavy builds from OOM-killing one shared host, give the host a weight
budget (`runtime.capacityWeight` under `runtime` in `.specify/memory/bubbles.config.json`,
disabled by default at `0`) and weight each heavy acquire so the registry holds or
refuses the second build instead of letting both run at once:

```bash
# Refuse immediately if the host is already at budget:
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose build --weight heavy
# Or wait up to 600s for a slot to free, then refuse:
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose build --weight heavy --wait 600
```

`summary` and `doctor` print `Runtime capacity: <active>/<capacityWeight> weight units`
(or `disabled` when `capacityWeight=0`). Stale or released leases free their slot
automatically. See [Coordinate Runtime Leases](runtime-coordination.md) for the full
`--weight` / `--weight-units` / `--wait` / `capacityWeight` surface.

For downstream repos using the installed framework layout, the same surface is available through `.github/bubbles/scripts/cli.sh`:

```bash
bash .github/bubbles/scripts/cli.sh runtime summary
bash .github/bubbles/scripts/cli.sh runtime doctor
```

If a consumer repo is missing the runtime commands entirely, refresh the framework layer from the downstream repo root by using a trusted upstream Bubbles checkout instead of patching `.github/bubbles` by hand:

```bash
bash /path/to/bubbles/install.sh --local-source /path/to/bubbles --bootstrap
```

Do not run that installer command inside the Bubbles source repository itself. Source-repo maintainers should update the framework directly and validate with `bash bubbles/scripts/cli.sh framework-validate` or `bash bubbles/scripts/cli.sh release-check`.

## View Lessons Learned

```
/bubbles.super  show recent lessons
```

Lessons are auto-compacted when the file exceeds 150 lines.

---

## Solve Framework Problems

Use `bubbles.super` when something in the framework itself is confused, blocked, or behaving in a way you do not understand.

### Diagnose Why Something Stopped

```
/bubbles.super  why did my workflow stop after validate?
→ Responds with: a short explanation of the likely gate or status ceiling issue, plus the next recovery command

/bubbles.super  why didn't my resume command pick up where I expected?
→ Responds with: the likely state issue, what file to check, and the next command to run
```

### Turn a Framework Problem Into Commands

```
/bubbles.super  fix my hooks setup and tell me how to verify it
→ Responds with: the framework action, then the follow-up verification command

/bubbles.super  explain why a result envelope or rework packet failed guard checks
→ Responds with: the likely failing gate, the exact file surface, and the next verification command

/bubbles.super  I think my custom gate is blocking the workflow, what do I do?
→ Responds with: the diagnostic step, the project-gate command, and the recommended repair sequence
```

### Recovery Sequence Examples

```
/bubbles.super  recover me from a failed upgrade
→ Responds with:
  1. /bubbles.super  upgrade --dry-run
  2. /bubbles.super  doctor
  3. /bubbles.super  install hooks

/bubbles.super  help me check whether this repo is Bubbles-ready
→ Responds with:
  1. /bubbles.setup  mode: refresh
  2. /bubbles.super  doctor --heal
  3. /bubbles.commands
  4. /bubbles.super  install hooks
```

If the question is specifically whether a repo is well-prepared for agentic work, the super should frame that as **repo-readiness guidance**, not certification. Use the repo-readiness skill or an equivalent framework-ops checklist, and do not treat the result as a substitute for `bubbles.validate`.

If you want the guidance as a concrete CLI check instead of a skill-only conversation, run:

```bash
bash .github/bubbles/scripts/cli.sh repo-readiness .
```

### Still Not Sure?

If you are not sure whether you have a framework problem, a feature problem, or just need the right prompts, go to [Ask the Super First](ask-the-super-first.md).
