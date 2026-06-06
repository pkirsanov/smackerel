---
name: bubbles-status-transition
description: Transition a Bubbles spec/scope/bug status safely through the framework's gate checks. Use when changing state.json status (`in_progress` → `done`, `done_with_concerns`, `blocked`, `specs_hardened`, etc.), promoting an artifact, or running pre-push checks. Covers the mechanical transition guard and the grandfather clause for historical done specs.
---

# Bubbles Status Transition

## Goal
Move a spec or scope through its lifecycle without violating the framework's transition gates, and understand exactly when historical done specs are re-evaluated vs. grandfathered.

## When to use
- About to edit `state.json` status field
- About to set a scope's `Status:` line in scopes.md or `scopes/*/scope.md`
- Before staging a commit that includes status changes
- Investigating why `state-transition-guard.sh` rejected a transition

## Status ceiling per workflow mode
Each workflow mode in `bubbles/workflows/modes.yaml` declares a `statusCeiling`. The spec cannot transition to a status above this ceiling. Common ceilings:

**v4.1.0:** the new `delivered_pending_activation` ceiling sits between `validated` and `done` for work that ships implementation + tests + audit + docs but defers live-runtime evidence to an external actor (operator commit, third-party approval, cutover window, regulator review). Modes targeting it: `adapter-readiness-to-packet`, `dark-launch-shipped`, `migration-shipped-pending-cutover`. See [`docs/v4.1.0-delivered-pending-activation.md`](../../docs/v4.1.0-delivered-pending-activation.md) for the full opt-in surface (`deliverableFiles[]` manifest, `phaseStubs{}`, lockdown tags, evidence-by-reference).

| Mode family | Ceiling | Meaning |
|-------------|---------|---------|
| `brainstorm` | `specs_scoped` | Planning only; no implementation |
| `plan-only` | `specs_scoped` | Same as brainstorm |
| `spec-scope-hardening` | `specs_hardened` | Specs and design hardened; no code |
| `docs-only` | `docs_updated` | Managed docs may change; no source code |
| `validate-only` | `validated` | Certification check only |
| `audit-only` | `validated` | Audit-owned findings only |
| `full-delivery`, `feature`, `bugfix-fastlane`, etc. | `done` | Full implementation permitted |

If the ceiling is below `done`, the agent MUST refuse to mark the spec `done` even if the work feels complete. Set the ceiling status instead and emit a `route_required` packet for the next-stage workflow.

## Ceiling status is terminal-for-mode (not a backlog item)

When a workflow mode has `statusCeiling` other than `done` (e.g., `validate-to-doc` → `validated`, `docs-only` → `docs_updated`, `adapter-readiness-to-packet` → `delivered_pending_activation`), the ceiling status IS the completion signal for that mode. Each such mode also declares `terminalAliases: [<ceiling>]` so that portfolio tooling can treat it as terminal.

- **Terminal-for-mode** means: `status == "done"` OR `status == mode.statusCeiling` OR `status ∈ mode.terminalAliases`.
- Use the helper `bash bubbles/scripts/is-terminal-for-mode.sh <status> <mode>` (exit 0 = terminal) instead of comparing to the literal string `"done"`.
- Promotion past the ceiling is forbidden by `state-transition-guard.sh`. Re-orchestrating a ceiling-bound spec/bug through `bugfix-fastlane` to force `done` is fake make-work — the actual work already shipped.
- Portfolio sweeps (`spec-dashboard.sh`, `bubbles.status`, `bubbles.recap`, retro tooling) MUST use `is-terminal-for-mode.sh` so ceiling-bound packets are counted as completed work, not as open items.

## Done is sequential
- A spec cannot be `done` until ALL scopes show `Status: Done` (Gate G024).
- A scope cannot be `Done` until ALL DoD items are `[x]` with inline raw-output evidence (Gate G025).
- A DoD item cannot be `[x]` until executed in the current session (anti-fabrication).

## Mechanical guard (must pass before push)
```
bash bubbles/scripts/state-transition-guard.sh specs/<NNN-feature>
```
Returns exit 0 only when every relevant check passes. The guard runs in the pre-push hook and in CI; it cannot be bypassed.

## Grandfather clause for historical done specs (PRESERVED)
This is the most important rule for framework upgrades:

- **`done-spec-audit.sh` default profile is `advisory`** — read-only historical report, exits 0 even if older specs would fail current policy.
- **Pre-push uses `--profile changed`** — only re-evaluates specs whose `state.json` actually changed in the diff. Spec 042 that's been `done` since v2.x stays green when v4.0 ships, unless someone edits its `state.json` in the same commit.
- **Recertification is opt-in only** — requires `--profile recertification --all --reopen-failing`. There is no implicit recertification.
- **The deprecated `--fix` flag is blocked** without an explicit recertification command.

When a historical done spec is reopened (because the work needs to be revisited), the agent MUST apply current-version policy from that point forward. The grandfather only applies while the spec stays untouched.

## Pre-completion mechanical pipeline
Run all of these and capture raw output into report.md before flipping any status to `done`:

```bash
bash bubbles/scripts/state-transition-guard.sh specs/<NNN-feature>
bash bubbles/scripts/artifact-lint.sh specs/<NNN-feature>
bash bubbles/scripts/cli.sh framework-validate            # in source repo
bash .github/bubbles/scripts/cli.sh framework-validate    # in downstream repo
```

## Failure recovery
If any guard exits non-zero:
1. Read the failing check label and the suggested remediation.
2. Fix the underlying gap in the spec/artifact/code.
3. Re-run the guard.
4. Do NOT lower the status target. Do NOT add a bypass flag.

## Authoritative modules
- `agents/bubbles_shared/state-gates.md` — state-claim integrity
- `agents/bubbles_shared/completion-governance.md` — sequential completion, deferral blocking
- `agents/bubbles_shared/scope-workflow.md` — status ceiling enforcement
- `bubbles/workflows/modes.yaml` — per-mode `statusCeiling` definitions
- `bubbles/scripts/state-transition-guard.sh` — mechanical guard
- `bubbles/scripts/done-spec-audit.sh` — historical grandfather + recertification
