# Bubbles Deprecations (v7.0)

> Maintained as the authoritative log of what changed shape, what changed default, and what is on a removal path. Covers the v5 → v6 reshape and the v7.0 removal of the v5 mode-name input surface.

This file is the operator-facing answer to "did Bubbles change X under me?" The v5 → v6 cycle was **monotonically stronger**: every v5 form kept working through the entire v6 cycle, with the v6 form canonical. **v7.0 completes that cycle by removing bare v5 mode NAMES as operator input.** This is the one intentional breaking change in v7 — see the Workflow Modes section for exactly what "removed" means and why existing artifacts are unaffected.

---

## How to Use This File

1. If you have an automation script, CI gate, prompt, or operator-facing doc that mentions any v5 name listed below, run `bash bubbles/scripts/migrate-modes-v5-to-v6.sh --check` (Bubbles framework-source) or `bash .github/bubbles/scripts/migrate-modes-v5-to-v6.sh --check` (downstream installs). It dry-runs the rewrite and exits 2 if anything needs migrating.
2. If exit code is 2, run with `--write` to apply.
3. The rewrite is idempotent and safe to re-run.

---

## Workflow Modes (B4): 55 v5 names → 15 v6 primitives + tags

Source of truth: `bubbles/workflows/aliases.yaml`. Resolver: `bubbles/scripts/mode-resolver.sh`. Selftest: `bubbles/scripts/mode-alias-selftest.sh`.

**v7.0 change — bare v5 mode names are REMOVED as operator input.** Through the v6 cycle, typing a v5 name (e.g. `bugfix-fastlane`) emitted a deprecation hint and resolved. In v7, `mode-resolver.sh` **rejects** a bare v5 name (exit 3) and prints the v6 primitive+tag form to use instead. Start new work with the v6 form (e.g. `fix target:bug action:fastlane`).

**What is NOT removed — existing artifacts are unaffected.** The v5 names remain the canonical **registry keys** inside `bubbles/workflows/modes.yaml`. `state.json.workflowMode` continues to store those keys, and the guards (`state-transition-guard.sh`, `artifact-lint.sh`, `is-terminal-for-mode.sh`) resolve status ceilings by direct registry lookup of the stored key. There is **no `state.json` schema change** and **no per-spec migration**: every already-complete spec, scope, bug, and ops artifact keeps validating exactly as before. Tools that resolve a persisted mode programmatically pass `--grandfather` / set `BUBBLES_MODE_GRANDFATHER=1`, which the guards do automatically. Only **new operator input** must use the v6 form.

### Canonical 15 v6 primitives

`analyze`, `plan`, `implement`, `test`, `validate`, `fix`, `ship`, `propagate`, `upkeep`, `review`, `improve`, `docs`, `iterate`, `resume`, `framework-health`.

### Tag grammar

- `action:<verb>` — what the run does (`analyze-design-plan`, `full-delivery`, `promote`, `fastlane`, …)
- `task:<task-name>` — named upkeep/maintenance task
- `target:<thing>` — the thing being acted on (`spec`, `bug`, `release-train`, `product`, …)
- `train:<name>` — release-train identifier
- `edge:<direction>` — propagation direction
- `lifecycle:<state>` — lifecycle phase

### Migration table (selected high-traffic modes)

| v5 mode name | v6 form |
|---|---|
| `value-first-e2e-batch` | `implement action:full-delivery target:highest-value-batch` |
| `full-delivery` | `implement action:full-delivery target:spec` |
| `product-to-delivery` | `implement action:full-delivery target:product prelude:analyze-design-plan` |
| `improve-existing` | `improve target:existing-feature action:analyze-and-harden` |
| `idea-to-release-completion` | `improve target:release-cycle action:full-loop` |
| `bugfix-fastlane` | `fix target:bug action:fastlane` |
| `incident-fastlane` | `fix target:production-incident action:triage-fix-validate-propagate-audit` |
| `release-train-cut` | `ship target:release-train action:cut` |
| `release-train-promote` | `ship target:release-train action:promote` |
| `release-train-rollback` | `ship target:release-train action:rollback` |
| `release-train-retire` | `ship target:release-train action:retire` |
| `release-planning` | `plan target:release-packet action:author` |
| `analyze-and-discover` | `analyze target:codebase` |
| `outcome-first-specs` | `plan target:spec action:outcome-first` |
| `propagate-feature` | `propagate edge:forward target:feature` |
| `propagate-fix` | `propagate edge:forward target:fix` |
| `propagate-backport` | `propagate edge:backward target:fix` |
| `upkeep-backup` | `upkeep task:backup` |
| `upkeep-restore-drill` | `upkeep task:restore-drill` |
| `upkeep-bcdr-drill` | `upkeep task:bcdr-drill` |
| `upkeep-patch-cycle` | `upkeep task:patch-cycle` |
| `upkeep-secret-rotation` | `upkeep task:secret-rotation` |
| `upkeep-flag-cleanup` | `upkeep task:flag-cleanup` |
| `upkeep-compliance-sweep` | `upkeep task:compliance-sweep` |
| `review-code-directly` | `review target:code action:direct` |
| `review-then-improve` | `review target:code action:then-improve` |
| `system-review` | `review target:system` |
| `retro` | `review target:session action:retro` |
| `retro-driven-harden` | `improve target:existing-feature action:retro-driven-harden` |
| `retro-driven-simplify` | `improve target:existing-feature action:retro-driven-simplify` |
| `retro-driven-review` | `review target:code action:retro-driven` |
| `retro-quality-sweep` | `improve target:existing-feature action:retro-quality-sweep` |
| `docs-only` | `docs action:update` |
| `docs-managed-only` | `docs action:managed-only` |
| `framework-health-cycle` | `framework-health` |
| `iterate` | `iterate action:pick-next` |
| `resume-paused-work` | `resume` |
| `chaos` | `test action:chaos` |
| `chaos-from-status` | `test action:chaos-from-status` |
| `spec-scope-hardening` | `plan target:spec action:harden` |
| `spec-review` | `review target:spec` |
| `spec-review-to-doc` | `review target:spec action:to-doc` |
| `audit-only` | `validate action:audit` |
| `validate-only` | `validate` |
| `validate-to-doc` | `validate action:to-doc` |
| `plan-only` | `plan` |
| `dark-launch-shipped` | `ship target:feature action:dark-launch` |
| `migration-shipped-pending-cutover` | `ship target:migration action:cutover-pending` |
| `release-planning-to-doc` | `plan target:release-packet action:to-doc` |
| `product-to-planning` | `plan target:product action:bootstrap` |
| `adapter-readiness-to-packet` | `plan target:adapter-readiness action:to-packet` |
| `retro-to-review` | `review target:session action:retro-to-review` |

The complete table is in `bubbles/workflows/aliases.yaml`. Use the migration script for accurate, idempotent rewrites instead of consulting this table by hand.

---

## Evidence Pipeline Defaults (B1, B2, B3)

The three v5 advisory paths flipped to v6 defaults. Each retains an `--advisory` opt-out for bisecting upstream changes.

| Tool | v5 default | v6 default | Opt-out |
|---|---|---|---|
| `evidence-tool-log-bridge.sh` | text-only output, advisory matching | `--format=json` emits structured envelope; MCP-primary | `--format=text` |
| `diff-evidence-guard.sh` | advisory unless date-based or `state.json.modernization.diffEvidence == enforce` | strict for all specs unless `state.json.modernization.diffEvidence == "advisory"` | set `state.json.modernization.diffEvidence: "advisory"` |
| `result-envelope-validate.sh` | fully advisory (`--advisory` implied) | block on malformed envelopes; warn on missing | `--advisory` |

Schema compatibility for `result-envelope-validate`:

- `additionalProperties: true` — agents may carry richer fields (`roleClass`, `featureDir`, `scopeIds`, `dodItems`, `packetRef`, `artifactsCreated`, `artifactsUpdated`, …)
- `nextRequiredOwner` accepted as alias for `nextOwner` (route_required outcomes)
- `blockedReason` accepted as alias for `blocker.reason` (blocked outcomes)
- Both `nextOwner` and `nextRequiredOwner` accept `["string", "null"]`

---

## Drift / Cheatsheet Surface (B7)

- **REMOVED:** `bubbles/scripts/cheatsheet-drift-selftest.sh` — the v5.0.1 H7 diff-only check. Drift is now structurally impossible because both cheatsheets are generated from `bubbles/cheatsheet/{modes,aliases,vocabulary}.json`.
- **ADDED:** `bubbles/scripts/generate-cheatsheet.sh` (with `--check` for CI). Registry edits are the only edit point.
- **Migration:** If you previously edited `docs/CHEATSHEET.md` or `docs/its-not-rocket-appliances.html` directly, port your changes into `bubbles/cheatsheet/*.json` and re-run the generator.

---

## Installer (B9)

`install.sh` continues to be the runtime entrypoint. It is now structurally verified against `bubbles/installer/installer.yaml` on every `framework-validate` run.

- If you fork `install.sh`, mirror the changes into the manifest OR your fork will fail the new `Installer manifest check` (I4 — every step has a marker).
- Five invariants close historical bug classes (gitignore root, missing chmod, silent step deletion, missing provenance field).

---

## Skills (B5)

Zero deletions in v6.0. `skills/INVENTORY.md` is the new audit point; any future POINTER-DELETE candidate must appear there with rationale before being removed in a subsequent minor release.

---

## Docs (B6)

Zero deletions in v6.0. `docs/governance-index.md` gains an Audience Matrix and per-section `**Audience:**` tags. No recipe content was merged.

---

## Removal Schedule

| Removed in | What |
|---|---|
| **v7.0 (DONE)** | **Bare v5 mode NAMES as operator input.** `mode-resolver.sh` rejects them (exit 3) with the v6 form to use. v5 names remain registry keys; existing artifacts are grandfathered (no schema change, no per-spec migration). Operators MUST start new work with the v6 primitive+tag form. |
| v6.1 | `--advisory` flag on `result-envelope-validate.sh` (the schema already accepts richer envelopes; advisory should not be needed). |
| v6.x | Anything documented here that has been quiet for ≥1 release without operator pushback. |

---

## How to Verify Your Operator-Side Surface Is Clean

```bash
# Bubbles source repo
bash bubbles/scripts/migrate-modes-v5-to-v6.sh --check

# Downstream install
bash .github/bubbles/scripts/migrate-modes-v5-to-v6.sh --check

# Apply rewrites
bash bubbles/scripts/migrate-modes-v5-to-v6.sh --write
```

Both exit 0 when your operator surface is on the v6 form.
