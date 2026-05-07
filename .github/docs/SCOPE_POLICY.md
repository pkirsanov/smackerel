# Bubbles Framework Scope Policy

> **STATUS: NON-NEGOTIABLE. READ THIS BEFORE COMMITTING ANYTHING TO THIS REPOSITORY.**

---

## ⛔ The Absolute Rule

**The Bubbles repository is a GENERIC, REPO-AGNOSTIC FRAMEWORK.**

**NO repository-specific, project-specific, product-specific, machine-specific, deployment-specific, or operator-specific content of ANY kind may be committed to this repository. Ever. Under any circumstances. With no exceptions.**

This applies to every directory in the Bubbles repo: `agents/`, `bubbles/`, `docs/`, `examples/`, `icons/`, `instructions/`, `pictures/`, `prompts/`, `recipes/`, `skills/`, `templates/`, the repo root, and any future directory.

---

## 🚫 What Is Repo-Specific (FORBIDDEN Here)

If content references, names, configures, plans, or describes any of the following, it is repo-specific and **MUST NOT live in this repository**:

| Category | Forbidden Examples |
|----------|-------------------|
| **Specific product repos** | Any named downstream installation (real product names, codenames, repo names — including the names of the specific repos this framework is currently installed into) |
| **Specific products** | Any product brand, codename, or trademark belonging to a downstream installation |
| **Specific machines** | Any hostname, machine codename, IP, Tailscale node, or operator-owned device identifier |
| **Specific deployment targets** | "production", "staging", a specific cloud account, a specific VPS, a specific home lab — when named as the actual deployment, not as a generic illustration |
| **Specific people/operators** | Any operator name, username, or identity |
| **Specific business domains** | "trading", "hospitality", "travel planning", "personal knowledge" — when used to describe an actual product, not as an abstract example |
| **Specific tech stacks chosen by a product** | A specific named stack belonging to a downstream installation |
| **Specific port numbers** | Any literal host port assigned to a real product |
| **Specific subnets / domains** | Any literal subnet, real Tailscale tail name, real production hostname |
| **Specific business plans** | "MVP date", "1.0 release schedule", "investor pitch content" of an actual product |
| **Specific infrastructure choices** | Named host services / secrets managers / reverse proxies tied to one downstream's chosen setup |
| **Specific SLAs / SLOs** | Latency targets, throughput targets, uptime promises tied to a real product |
| **Cross-product synthesis** | Documents that compare or coordinate multiple specific products by name |
| **Operator runbooks** | BIOS settings, host LVM layout, machine-level setup steps for a real machine |

If a piece of content **only makes sense for one specific repo or one specific deployment**, it does not belong here.

---

## ✅ What Is Repo-Agnostic (Belongs Here)

Bubbles content MUST be reusable verbatim by any conforming downstream repo. Acceptable content describes the framework itself in fully generic terms:

| Category | Acceptable Examples |
|----------|---------------------|
| **Generic agents** | `bubbles.implement`, `bubbles.audit`, `bubbles.workflow` — described in terms of artifacts, gates, evidence |
| **Generic skills** | "Configuration single source of truth", "Deployment target adapter pattern", "Test environment isolation" — described as patterns, not as a specific implementation |
| **Generic templates** | `spec.md` template, `state.json` schema, `scopes.md` structure |
| **Generic instructions** | Policies that apply to any repo using Bubbles (e.g., "no fabrication", "raw evidence required") |
| **Generic gate definitions** | Gate IDs, what they check, why they exist — never tied to a specific tech stack |
| **Generic recipes** | "How to add a new endpoint", "How to debug a failing scope" — using placeholder repo names like `<repo>` |
| **Framework documentation** | This file. The README. Cheatsheets. Manuals describing how Bubbles works as a framework. |
| **Abstract example identifiers** | `example-app`, `home-lab`, `aws`, `fly`, `gcp`, `staging-vps`, `local-dev`, `<repo>`, `<service>`, `<target>`, `<NNN-feature-name>` |

When examples must be concrete enough to be useful, use abstract placeholders — never real names.

---

## ✅ Where Repo-Specific Content Belongs

Each downstream product repo owns its own concrete content under its OWN repo:

| Type | Belongs In |
|------|------------|
| Per-product overview, principles, capability ledger | `<product-repo>/docs/` |
| Per-product specs and implementation | `<product-repo>/specs/` |
| Per-product architecture | `<product-repo>/docs/Architecture.md` (or equivalent) |
| Per-product deployment adapters | `<product-repo>/deploy/<target>/` |
| Per-product deployment plans | `<product-repo>/docs/` (e.g., `Maturity_Plan.md`, `<Target>_Deployment_Plan.md`) |
| Cross-product synthesis (when needed) | A non-Bubbles workspace location chosen by the operator (e.g., a separate `homelab/` repo, a personal notes vault, the operator's `~/notes/`) — **never Bubbles** |
| Operator/host runbooks | The operator's own ops repo or notes — **never Bubbles** |

If you find yourself wanting to write a document that lists or coordinates multiple specific products, that document does not belong in Bubbles — it belongs in a non-framework location.

---

## 🔍 Self-Audit Before Every Commit

Before committing any change to the Bubbles repository, answer these questions. If you answer "yes" to any one of them, the change is **forbidden** and must be reworked or moved out of this repo.

1. Does this content name a specific repo? (real product names, codenames, downstream installation names)
2. Does this content name a specific product or brand?
3. Does this content reference a specific machine, host, IP, or Tailscale node?
4. Does this content list specific port numbers, subnets, or domains tied to a real product?
5. Does this content describe one specific person's setup, preferences, or schedule?
6. Does this content coordinate or compare multiple named products?
7. Does this content describe a specific business model, monetization, or release plan of a real product?
8. Does this content embed an operator runbook for a specific machine or deployment?
9. Does this content describe a specific tech stack choice that other repos shouldn't be forced to copy?
10. If a brand-new repo adopted Bubbles tomorrow with a totally different stack, would they have to delete or ignore this content?

**Any "yes" answer = this content does not belong in Bubbles.**

### Verification Grep (run from the Bubbles repo root before commit)

```bash
# Replace the placeholder regex with the names of any downstream installations
# the framework is currently installed into. The list is intentionally not
# committed here so this file stays generic.
grep -rln '<comma-separated regex of real product / repo / machine names>' . \
  | grep -v '\.git/\|SCOPE_POLICY.md\|generated/'
# Expectation: zero matches.
```

The agnosticity lint (`bash bubbles/scripts/agnosticity-lint.sh`) automates a portion of this scan. Run it locally before pushing.

---

## 🛑 Recovery When This Rule Is Violated

If repo-specific content is committed to Bubbles:

1. **Stop**. Do not add more.
2. **Identify** every file that violates this policy (run the grep above).
3. **Move** the content to its rightful per-repo location (or to a non-framework location if it is truly cross-product operator material).
4. **Delete** the file(s) from Bubbles, OR scrub the offending names in place if the file is otherwise generic.
5. **Update** any cross-references in downstream repos to point at the new location.
6. **Audit** the rest of Bubbles for similar violations introduced in the same change set.
7. **Record** the incident in this file's history below so the same mistake is not repeated.

---

## 📜 Violation History

| Date | What Was Committed | Where It Was Moved | Why It Happened |
|------|--------------------|--------------------|-----------------|
| 2026-05-07 | Cross-product gap matrix doc referencing four specific downstream installations by name with their port blocks, subnets, and infisical/postgres/caddy choices | Deleted from Bubbles. Content folded into each downstream installation's `docs/Maturity_Plan.md`. | Agent treated Bubbles as a workspace-level scratchpad for cross-product synthesis instead of a framework repo. |
| 2026-05-07 | Machine-specific BIOS/LVM/Caddy/ufw/Tailscale/Uptime Kuma host runbook for one specific home server | Deleted from Bubbles. Each downstream's `deploy/<target>/README.md` documents what it needs from the host; the cross-cutting host setup is operator-owned and lives outside any framework or product repo. | Same root cause — agent placed an operator runbook in the framework repo. |
| 2026-05-07 | Workspace-specific Docker image cleanup notes naming three specific downstream installations | Deleted from Bubbles. Operator-owned cleanup record; lives outside any framework or product repo. | Pre-existing baggage from before the policy was codified. Discovered during the same purity sweep that produced this policy. |
| 2026-05-07 | Real names embedded as illustrative examples in `docs/guides/CONTROL_PLANE_SCHEMAS.md`, `docs/guides/PRODUCT_DIRECTION_SURFACES.md`, `docs/issues/session-aware-runtime-coordination.md`, `docs/recipes/release-planning.md`, `docs/recipes/autonomous-goal.md`, `CHANGELOG.md`, `skills/bubbles-product-principle-discovery/SKILL.md`, `skills/bubbles-deployment-target-adapter/SKILL.md`, `instructions/bubbles-deployment-target.instructions.md`, `agents/bubbles.releases.agent.md` | Scrubbed in place. Real names (specific downstream repos, a specific machine codename, a real Tailscale IP) replaced with generic placeholders (`example-app`, `home-lab`, `<a real Tailscale or LAN IP>`, "downstream installation"). | Pre-existing baggage from when those files were first written using real downstreams as illustrative examples. The policy now treats real names anywhere in framework code as a violation regardless of context. |

When adding a new row, summarize the violation, the destination, and the lesson so the rule cannot be silently rediscovered later.

---

## Cross-Reference

This policy is **enforcing** the framework-vs-product separation that is also encoded in:

- `agents/bubbles_shared/project-config-contract.md` (downstream config contract)
- `instructions/*.md` (downstream-applied policies — written generically)
- `skills/*/SKILL.md` (downstream-applied skills — written generically)

Any future architectural change to Bubbles MUST preserve this scope rule.
