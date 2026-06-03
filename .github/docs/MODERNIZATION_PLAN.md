# Bubbles Modernization Plan — v5.0.1 → v5.1 → v6.0

> **Working reference. Update as items land.**
>
> **Non-negotiable invariant:** anti-fabrication, evidence, validation, ownership, and release-provenance policies are NEVER softened across any release. Every change must preserve or strengthen them.

---

## Background

A two-pass review (post v5.0.0 ship + downstream upgrade) surfaced these structural risks:

- 14 undefined gate IDs referenced across [bubbles/workflows.yaml](../bubbles/workflows.yaml), scripts, and docs: `G017`, `G030`, `G039`, `G045`, `G046`, `G049`, `G050`, `G054`, `G062`, `G065`, `G071`, `G073`, `G099`, `G100`. `G073` alone is referenced in 28 `requiredGates:` lists.
- [bubbles/scripts/state-transition-guard.sh](../bubbles/scripts/state-transition-guard.sh) is 3,878 LOC with duplicate `CHECK 3B` and `CHECK 4` labels.
- No JSON Schema validation on critical YAML — caused strict-parser failures in downstream upgrade.
- Installer post-install behavior (adapters dir, root `.gitignore`) not asserted by selftests — shipped 2 latent bugs.
- Evidence model is prose-based markdown parsed by grep; modern agent platforms (Claude Code, Cursor, Cline, VS Code agent mode) standardize on MCP + structured tool-call provenance.
- Mode surface (54 real modes + many overlapping recipes/skills/guides) exceeds the maintenance capacity demonstrated in recent sessions.

The plan below ships in three releases. Each release is independently shippable. Order matters: hardening first restores confidence; modernization second adds structured evidence and machine-verifiable envelopes; subtractive third reduces surface area and adds client portability.

---

## Cross-Release Invariants

Apply to every change in every release:

1. Anti-fabrication is monotonically stronger or equal — never weaker.
2. No gate is removed. Gates are consolidated, generated, or moved, never softened.
3. Every release ships with a downstream upgrade recipe and is tested against all 5 downstream repos (smackerel, wanderaide, guestHost, quantitativeFinance, knb) before tagging.
4. Every release passes its own `framework-validate` + `release-check` on a clean checkout.
5. No release introduces a hand-edited surface that duplicates an existing source-of-truth — anything duplicated must be generated.

---

## v5.0.1 — Hardening (target: 1 week, no new features)

**Theme:** The framework eats its own dog food. Close every drift gap.

| # | Work item | Files touched | Done when |
|---|---|---|---|
| H1 | Registry-consistency selftest | new `bubbles/scripts/registry-consistency-selftest.sh`; wired into `framework-validate.sh` | Every `Gxxx` referenced in [workflows.yaml](../bubbles/workflows.yaml) / scripts / agents / docs resolves to a defined gate (or is intentionally listed under `legacyGateAliases:` with a mapping). Exit 1 otherwise. |
| H2 | Purge 14 dead gate refs | [bubbles/workflows.yaml](../bubbles/workflows.yaml) (G017, G030, G039, G045, G046, G049, G050, G054, G062, G065, G071, G073, G099, G100); update `requiredGates:` lists to current IDs | H1 selftest passes. Gate badge in [README.md](../README.md) reflects only defined gates. |
| H3 | Duplicate-CHECK-label lint | [bubbles/scripts/state-transition-guard.sh](../bubbles/scripts/state-transition-guard.sh) (rename Check 3B/4 duplicates); new lint in `framework-validate.sh` | No duplicate `# CHECK <id>:` labels anywhere. Lint fails if reintroduced. |
| H4 | JSON Schemas for critical YAML | new `bubbles/schemas/{workflows,capability-ledger,adoption-profiles,intent-routes,release-manifest,propagation-policy}.schema.json`; validator in `framework-validate.sh` (Python `jsonschema`) | Each YAML validates against its schema in CI and pre-push. Strict-parser failures (yesterday's colon-quoting bug class) caught at commit. |
| H5 | Installer post-install fixture assertions | extend `bubbles/scripts/install-provenance-selftest.sh` | Fixture asserts: adapters dir installed; `*.sh` executable bit set; repo-root `.gitignore` contains `improvements/`; manifest count matches; no stray `.github/.gitignore` created. |
| H6 | Manifest enumeration purity selftest | new `bubbles/scripts/release-manifest-purity-selftest.sh` | Selftest with intentionally-placed untracked file proves it does NOT appear in the manifest. Locks in the manifest-enumeration fix. |
| H7 | Cheatsheet drift check | new `bubbles/scripts/cheatsheet-drift-check.sh` (diff-only — full generator deferred to v6) | Fails if the v5 workflow modes / TPB vocabulary present in [docs/CHEATSHEET.md](CHEATSHEET.md) and [docs/its-not-rocket-appliances.html](its-not-rocket-appliances.html) diverge beyond an allowed delta. |
| H8 | Pre-push hook for bubbles repo itself | new `.git/hooks/pre-push` template + installer for maintainers (`bubbles/scripts/install-bubbles-hooks.sh`) | Bubbles maintainers cannot push if `registry-consistency-selftest`, `framework-validate`, or `release-check` fail. |
| H9 | Honest badge counts | `bubbles/scripts/generate-framework-stats.sh` | Gate count = `len(gates:)` block in workflows.yaml. README badge regenerated; no dead-ref inflation. |
| H10 | Changelog + release | [CHANGELOG.md](../CHANGELOG.md), [VERSION](../VERSION) → `5.0.1` | Tagged; downstream upgrade is purely mechanical (`install.sh --local-source`). |

**Exit criteria:** every framework bug found in the latest review session is now caught by a selftest. No mode collapse, no MCP, no architecture change.

---

## v5.1 — Modernization Foundation (target: 2-3 weeks)

**Theme:** Replace prose-based evidence with structured tool-call provenance and structured envelopes. Make validation provably stronger.

| # | Work item | Files touched | Done when |
|---|---|---|---|
| M1 | Structured tool-call evidence log | new `bubbles/scripts/tool-log.sh` + `bubbles/schemas/tool-call.schema.json`; integration in `cli.sh` and example wrappers in downstream `wanderaide.sh` / `smackerel.sh` | Every gate-relevant command (`test`, `build`, `curl`, `psql`, etc.) wrapped to append `{ts, cmd, exit_code, stdout_hash, stderr_hash, agent, spec, scope}` to `.specify/runtime/tool-calls.jsonl`. Existing logs preserved. |
| M2 | Evidence gate reads tool log first | [bubbles/scripts/state-transition-guard.sh](../bubbles/scripts/state-transition-guard.sh) Check 9 (Evidence depth) | DoD evidence satisfied when EITHER (a) tool-log entry exists for the DoD's recorded command in current session OR (b) traditional ≥10-line raw evidence is present. Markdown stays valid; tool-log becomes the stronger path. Anti-fabrication strictly stronger. |
| M3 | JSON Schema result envelopes | new `bubbles/schemas/result-envelope.schema.json`; `bubbles/scripts/result-envelope-validate.sh`; every agent emits both markdown AND JSON envelope blocks | Workflow agent parses JSON; markdown remains human-readable. Validator runs in framework-validate. |
| M4 | Split state-transition-guard | `bubbles/scripts/guards/{state-integrity,dod-completion,evidence-depth,scope-status,phase-coherence,certification,framework-ownership,deferral,scenario-tdd,reality-scan}.sh`; thin dispatcher [bubbles/scripts/state-transition-guard.sh](../bubbles/scripts/state-transition-guard.sh) | All current checks preserved exactly. Each family independently runnable and selftested. No behavioral change for callers. |
| M5 | Diff-aware DoD evidence | new `bubbles/scripts/diff-evidence-guard.sh` | DoD items claiming "X file added", "Y test created", "Z handler wired" verified against `git diff <baseSha>..HEAD` for the spec range. Catches "claimed done, didn't change code" at gate time. |
| M6 | Gate registry as authoritative source | new `bubbles/registry/gates.yaml` (canonical); `gates:` block in workflows.yaml becomes generated; scripts reference IDs via `bubbles/scripts/gate-meta.sh` helper | One file owns gate IDs, names, descriptions, severity, owner agent. Every other surface cross-checks against it. Removes ~40% of drift surface. |
| M7 | Model-tier policy field (advisory) | `bubbles/workflows.yaml` per-mode/per-phase `modelFloor:` | Workflow agent surfaces a warning when host client reports a model below the floor for `audit`, `security`, `design`, `validate`. Enforcement deferred to v6 S9. |
| M8 | Code-index facade | new `bubbles/scripts/code-search.sh` (delegates to host's `rg` / index where available; uniform output) | Agents stop reinventing search per repo. Token cost reduced; behavior unchanged. |
| M9 | Schema-validated control-plane manifests | extend H4 to `scenario-manifest.json`, `lockdown-approvals.json`, `propagation-ledger.yaml` | Control-plane drift caught at commit time. |
| M10 | Changelog + release | [VERSION](../VERSION) → `5.1.0` | Downstream upgrade mechanical; tool-log and JSON envelope are opt-in adoption initially. |

**Exit criteria:** anti-fabrication provably stronger — tool-log makes "claim without execution" detectable as missing log entries. No mode change, no MCP yet.

---

## v6.0 — Subtractive + Portable (target: 4-6 weeks)

**Theme:** Reduce surface, become client-agnostic via MCP, ship the simplification release. Functionality preserved entirely; legacy mode names accepted via aliases for one minor cycle.

| # | Work item | Files touched | Done when |
|---|---|---|---|
| S1 | Mode collapse: 54 → ~15 primitives + tags | [bubbles/workflows.yaml](../bubbles/workflows.yaml) → `bubbles/workflows/{analyze,plan,implement,test,validate,fix,ship,propagate,upkeep,review,improve,docs,iterate,resume,framework-health}.yaml`; tag grammar; legacy mode aliases preserved | `release-train-promote` → `ship train:<id> action:promote`; `upkeep-restore-drill` → `upkeep task:restore-drill`; etc. Old mode strings resolve via alias table; deprecation warning only. |
| S2 | Workflow split + per-family schema | `bubbles/workflows/*.yaml`; merge resolver in `mode-resolver.sh`; CI validates each | No single 2600-line hand-edited file. Faster lint; cleaner merges. |
| S3 | Bubbles MCP server | new `mcp/bubbles-gates/` (TypeScript or Python); tools: `validate_dod`, `record_evidence`, `check_gate`, `resolve_mode`, `route_finding`, `query_tool_log`, `verify_status_transition` | Bubbles usable from VS Code Copilot agent mode, Claude Code, Cursor, Cline. Bash scripts remain as fallback. |
| S4 | MCP-native evidence model | MCP tool calls auto-log to `.specify/runtime/tool-calls.jsonl` | "Did this DoD's test actually run?" becomes a deterministic MCP query, not a grep. Strongest anti-fabrication state to date. |
| S5 | Full cheatsheet generator | `bubbles/scripts/generate-cheatsheet.sh` emits both HTML and MD from gate / mode / agent / skill registries | Single source of truth for operator reference. H7 evolves from drift check to generator. |
| S6 | Skill pruning + repositioning | delete 14 thin (<80 LOC) skill pointers; keep 20 substantive policy skills; reserve `skills/` for project domain skills | Aligns with Claude Skills semantics. ~3,500 LOC removed. Substantive policy preserved. |
| S7 | Parallel phase fan-out | [agents/bubbles_shared/workflow-execution-loops.md](../agents/bubbles_shared/workflow-execution-loops.md); workflow agent dispatch logic | Independent phases (`analyze` + `ux`, `test` + `audit`, `code-review` + `security`) run in parallel where `dependsOn` permits. 2-3× wall-clock improvement on multi-phase modes. |
| S8 | Installer as generated artifact | new `bubbles/installer/installer.yaml` + `bubbles/scripts/generate-installer.sh` → produces [install.sh](../install.sh) | Each post-install action is a declared typed step with a selftest fixture. Adapter/gitignore class of bug structurally impossible. |
| S9 | Model-tier enforcement | extends M7 to blocking | Phases refuse to run below `modelFloor:`; operator gets explicit message instead of silent quality loss. |
| S10 | Doc consolidation | reduce 63 recipes + 8 guides + 34 skills + 45 shared modules to curated set with audience labels (operator / agent / maintainer) | ~15% doc surface removed; no unique content lost. Drift opportunities reduced. |
| S11 | Honest deprecation cycle | new `docs/DEPRECATIONS.md`; v5 mode names still accepted with warning; removed in v7 | Downstream operators have clean migration path. |
| S12 | Release + downstream uplift recipe | new `docs/recipes/upgrade-to-v6.md`; [VERSION](../VERSION) → `6.0.0` | One command for downstreams: install v6, run mode-alias migration script, push. |

**Exit criteria:** Bubbles is client-agnostic via MCP, surface is materially smaller, evidence is structurally enforced (not grep-parsed), validation strictly stronger than v5.0. All v5 functionality reachable via primitive + tag. No operator capability removed.

---

## Working Order

1. **Now → v5.0.1:** start immediately. H1, H3, H4 in parallel (independent commits). H2 depends on H1. H5/H6 independent. H7/H8/H9 cleanup. H10 ships.
2. **v5.0.1 ships → begin v5.1 design.** Implement M4 (split guard) first to make M2/M5 changes safer. M1+M3 in parallel. M6 last in the foundation (it touches everything else).
3. **v5.1 ships → begin v6 design.** S2 unblocks S1. S3 (MCP) is the longest single thread; S6/S10 (cuts) can ship continuously. S1 is the user-visible centerpiece; ship behind aliases.

## How To Use This Document

- Treat each row as a unit of work with its own scope/spec/state.json if it's non-trivial.
- Mark progress inline: change "Done when" cell to ✅ when shipped; reference the commit SHA.
- Don't reorder rows without updating dependencies noted above.
- If a row spawns sub-items, add them as a nested table under the parent row, not as new top-level rows.
- Every release closes with: badge regenerated, downstream upgrade tested on all 5 repos, CHANGELOG appended, VERSION bumped.

## What This Plan Does Not Do

- Does not soften any anti-fabrication, evidence, validation, ownership, or release-provenance policy.
- Does not remove operator capabilities — only renames (with aliases), generates (instead of hand-editing), and prunes redundant docs/skills.
- Does not introduce optional bypass flags. Every gate stays mandatory unless its `gateClassification:` already declares otherwise.
- Does not commit Bubbles to a particular MCP transport (stdio vs HTTP) — that's a v6 design decision inside S3.

## Open Questions (decide during v5.0.1)

1. Where does `bubbles/registry/gates.yaml` live relative to `bubbles/workflows.yaml`? (Recommendation: same folder, generator wires them.)
2. Should the tool-log be per-repo (`.specify/runtime/tool-calls.jsonl`) or per-session? (Recommendation: per-repo with `sessionId` field.)
3. Should the MCP server ship as part of the framework install or as a separate optional install? (Recommendation: separate install, declared in `bubbles/release-manifest.json` so downstream knows it exists.)
4. v6 mode aliases — how long do we keep them? (Recommendation: through entire v6 cycle; remove in v7.0.0.)
