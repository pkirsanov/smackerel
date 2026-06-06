# Bubbles v6.0 — MCP Migration + Subtractive Release

> Status: DESIGN. Not implemented. Depends on v5.2 shipping first.
> Theme: client-agnostic via MCP; surface materially smaller; evidence structurally enforced (not grep-parsed).
> Constraint: every operator capability reachable in v5.x stays reachable in v6.0 (via primitive + tag if a mode collapsed, via MCP tool if a script moved).

---

## Non-Negotiable Invariants

1. Anti-fabrication monotonically stronger or equal — never weaker.
2. No gate removed; gates may be consolidated/generated/moved.
3. v5 mode names accepted as aliases through the entire v6 cycle; **removed as operator input in v7.0** (they remain registry keys; existing artifacts grandfathered).
4. No new operator command required to keep working — every v5.x command keeps working until v7.
5. MCP server is **optional** for repos that don't use MCP-aware clients; bash scripts remain the supported fallback.
6. Pure mechanical upgrade via `install.sh --local-source` + one-time `bubbles/scripts/migrate-modes-v5-to-v6.sh`.

---

## What Ships

### Group A: MCP server (additive)

| # | Item | Files | Done when |
|---|---|---|---|
| A1 | MCP server skeleton | new `bubbles/mcp/server.sh` (bash, stdlib-only) OR `bubbles/mcp/server.py` (stdlib Python 3.10+, no pip) | Server boots, responds to `initialize`, lists tools/resources, handles `tools/call` for at least one tool. |
| A2 | Tool catalog (MCP `tools`) | `bubbles/mcp/tools/*.json` (declarative), dispatcher in server | Exposes: `validate_dod`, `record_evidence`, `check_gate`, `resolve_mode`, `route_finding`, `query_tool_log`, `verify_status_transition`, `search_code`, `read_spec`, `list_open_findings`. Each tool wraps an existing `bubbles/scripts/*.sh`. |
| A3 | Resource catalog (MCP `resources`) | `bubbles/mcp/resources/*.json` | Exposes: `bubbles://workflows.yaml`, `bubbles://gates/{id}`, `bubbles://schemas/{name}`, `bubbles://spec/{nnn}/state.json`, `bubbles://tool-log/{spec}/latest`. |
| A4 | Client config samples | `bubbles/mcp/clients/{vscode,claude,cursor,cline}.json` | Each contains a one-line server registration; install.sh asks operator (default yes) to install into the active client's config dir. |
| A5 | MCP selftest | new `bubbles/scripts/mcp-server-selftest.sh` | Boots server, issues `initialize` + each tool/resource call via stdio, validates JSON-RPC responses against schemas. |
| A6 | Documentation | new `docs/MCP.md` (operator), `docs/MCP-tools.md` (per-tool reference, generated from `tools/*.json`) | Operator can register the server in ≤2 minutes. |

### Group B: Subtractive (cuts)

| # | Item | Files | Done when |
|---|---|---|---|
| B1 | Flip M2 (evidence bridge) to MCP-primary | `bubbles/scripts/evidence-tool-log-bridge.sh`, `bubbles/scripts/guards/evidence-depth.sh` | When MCP server is registered, gate queries the bridge via MCP `query_tool_log`; bash path is fallback. Markdown path remains accepted (v6 still has it). |
| B2 | Flip M5 (diff-evidence-guard) to default-on for ALL specs | `bubbles/scripts/diff-evidence-guard.sh`, `bubbles/scripts/state-transition-guard.sh` | Guard always blocks; per-spec opt-out via `state.json.modernization.diffEvidence = "advisory"` documented for emergency. |
| B3 | Result-envelope JSON: validator blocking | `bubbles/scripts/result-envelope-validate.sh`, `bubbles/scripts/framework-validate.sh` | Agent file without a valid `result_envelope:` block fails framework-validate. |
| B4 | Mode collapse (55 v5 modes → 15 v6 primitives + tag grammar) | legacy alias table in `bubbles/workflows/aliases.yaml`; `mode-resolver.sh` resolves v5 mode names ↔ v6 primitive+tag form; v6.1 split the `modes:` registry out of `workflows.yaml` into `bubbles/workflows/modes.yaml` | 55 v5 modes resolve via primitive + tag. Through v6, old mode strings still resolved with a deprecation warning; **v7.0 rejects bare v5 names as input** (registry keys retained; existing artifacts grandfathered). Selftest covers every v5 mode → primitive+tag mapping. |
| B5 | Skill pruning | delete listed thin-pointer skills (<80 LOC each); keep substantive policy skills | Inventory file `bubbles/skills/INVENTORY.md` lists kept + removed (with reason). No unique policy content lost. |
| B6 | Doc consolidation | merge near-duplicate recipes/guides; tag each remaining doc with audience (`operator`/`agent`/`maintainer`) | Doc count drops ~15%; `docs/governance-index.md` regenerated. |
| B7 | Cheatsheet generator (replaces drift check) | `bubbles/scripts/generate-cheatsheet.sh` | Single source of truth → emits both `docs/CHEATSHEET.md` and `docs/its-not-rocket-appliances.html`. H7 drift check retired. |
| B8 | Remove v5.1 advisory paths superseded by v5.2 primaries | `bubbles/scripts/{evidence-tool-log-bridge,diff-evidence-guard,result-envelope-validate}.sh` advisory branches | Advisory code paths deleted in favor of MCP-or-block. Selftests updated. |
| B9 | Installer as generated artifact (S8) | new `bubbles/installer/installer.yaml` + `bubbles/scripts/generate-installer.sh` → produces `install.sh` | Each post-install action is a typed step with a fixture; adapter/gitignore bug class structurally impossible. |
| B10 | Parallel phase fan-out (S7) | `agents/bubbles_shared/workflow-execution-loops.md`, workflow agent dispatch | Independent phases (`analyze`+`ux`, `test`+`audit`, `code-review`+`security`) run in parallel where DAG permits. 2–3× wall-clock improvement on multi-phase modes. |

### Group C: Operator-facing migration

| # | Item | Files | Done when |
|---|---|---|---|
| C1 | Mode-alias migration script | new `bubbles/scripts/migrate-modes-v5-to-v6.sh` | One-shot rewrite of any operator-side mode invocations from v5 names to v6 primitive+tag. Idempotent. |
| C2 | DEPRECATIONS doc | new `docs/DEPRECATIONS.md` | Lists every v5 mode name + its v6 primitive+tag equivalent + the v7 removal date. |
| C3 | Upgrade recipe | new `docs/recipes/upgrade-to-v6.md` | Step-by-step downstream upgrade: `install.sh --local-source`; opt: register MCP server; opt: run migrate-modes script; push. |
| C4 | CHANGELOG + release | `CHANGELOG.md`, `VERSION` → `6.0.0`, tag, `release-check` exit 0 | Tagged on `main`. Downstream upgrade mechanical. |

---

## MCP Implementation Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language | bash dispatcher OR stdlib Python 3.10+ (no pip) | Matches Bubbles' no-install philosophy. Survives downstream repo isolation. Zero dependency drift. Pick **Python** if JSON-RPC framing in bash proves too fragile during A1; fall back to bash if Python adds drift. |
| Transport | stdio only in v6.0; HTTP/SSE deferred to v6.1 | All target clients (VS Code agent, Claude Desktop, Cursor, Cline) support stdio. HTTP adds auth + lifetime headaches not worth solving in v6.0. |
| Install model | local script, registered in client config | No `pip install`, no `npm install`, no daemon. Operator copies a one-line config snippet from `bubbles/mcp/clients/<client>.json` into their client config. `install.sh` offers to do it automatically. |
| Tool surface | thin wrappers around existing scripts | MCP tool = JSON-RPC over `bubbles/scripts/<existing>.sh`. No business logic duplicated. |
| Resource surface | read-only handles to canonical files | `bubbles://` URIs resolve to repo-local file reads. Versioned via git SHA in `provenance` field. |
| Fallback when MCP server not registered | bash scripts work exactly as v5.x | MCP is additive UX, not a replacement runtime. |

---

## Backward Compatibility Matrix

| v5.x artifact / behavior | v6.0 behavior |
|---|---|
| Mode name `release-train-promote` | Resolves via alias to `ship train:<id> action:promote`. Deprecation warning. |
| Mode name `upkeep-restore-drill` | Alias to `upkeep task:restore-drill`. Warning. |
| `report.md` with markdown-only evidence | Still validates as long as `diffEvidence != enforce` OR a matching tool-log entry exists. |
| `tool-call.jsonl` v1 entries | Still read. |
| Repo without MCP server registered | All bash gates and scripts work identically. |
| Agent file without `result_envelope:` block | **Fails** framework-validate (flipped from v5.2 warn). |
| Spec without `state.json.modernization.diffEvidence` | Diff-evidence-guard defaults to `enforce`; opt-out is explicit. |

---

## Selftest Plan

| Selftest | Asserts |
|---|---|
| `mcp-server-selftest.sh` (new) | Server boots; `initialize` returns capability list; each declared tool returns valid JSON-RPC; each declared resource serves a file; malformed request returns proper error code. |
| `mcp-tool-catalog-selftest.sh` (new) | Every `bubbles/mcp/tools/*.json` references an existing `bubbles/scripts/*.sh` AND has a valid input/output schema. |
| `mode-alias-selftest.sh` (new) | Every v5 mode name in `aliases.yaml` resolves to a primitive+tag combination that produces an equivalent workflow plan. |
| `migrate-modes-selftest.sh` (new) | Fixture with v5 mode invocations gets rewritten to v6 form; second run is no-op. |
| `installer-generator-selftest.sh` (new) | Generated `install.sh` is byte-identical across two consecutive generations from the same `installer.yaml`. |
| `parallel-phase-selftest.sh` (new) | DAG with two `dependsOn`-independent phases completes in ≤ max(phase A, phase B) + overhead. |
| `result-envelope-validate-selftest.sh` (extend) | Agent without envelope → exit 1 (was warn in v5.2). |

---

## Risk Register

| Risk | Mitigation |
|---|---|
| MCP transport differs subtly between clients | Stick to stdio; selftest each declared client config; document known-good versions of each client in `docs/MCP.md`. |
| Mode collapse breaks an operator's muscle memory | Aliases active full v6 cycle; deprecation warning + DEPRECATIONS.md; `migrate-modes-v5-to-v6.sh` is one command. |
| Subtractive cuts in B5/B6 remove something a downstream relies on | Inventory + git history checked before each delete; B5/B6 PRs require explicit "no unique content lost" review note. |
| Parallel phase fan-out introduces nondeterminism | DAG enforced; output ordering deterministic (sort by phase name); selftest validates same DAG → same envelope sequence across 100 runs. |
| MCP server crashes leave gates broken | Bash fallback is unconditional — every MCP tool has a bash twin invoked when MCP not registered or returns error. |
| Schema-version churn (`tool-call.jsonl` v2 → v3) breaks downstreams | v6 keeps reading v2; new writes are v2; v3 deferred to v6.1. |

---

## Working Order

1. **A1 + B4** in parallel: MCP server skeleton AND mode-collapse design + alias table. Both are large; ship A1 first as a no-tools scaffold to unblock A2/A5.
2. **A2 → A3 → A4 → A5**: tools, resources, client configs, selftest.
3. **B1 + B2 + B3**: flip evidence/diff/envelope to MCP-primary or blocking. Each gated by its selftest passing.
4. **B5 + B6 + B7**: cuts. Continuous trickle while A2–A5 land.
5. **B8 + B9 + B10**: cleanup, installer generator, parallel fan-out.
6. **C1 + C2 + C3**: migration tooling and docs.
7. **C4**: tag `v6.0.0`; downstream rollout.

## What v6.0 Does Not Do

- Does not remove the bash script surface (MCP wraps it, doesn't replace it).
- Does not remove markdown evidence (it remains valid when diff-evidence-guard is not `enforce`).
- Does not remove any operator capability — only renames (with aliases), generates (instead of hand-editing), or prunes redundant docs/skills.
- Does not introduce HTTP transport (deferred to v6.1).
- Does not change `state.json` schema (deferred to v7).
- Does not delete v5 mode names (deferred to v7).

---

## Open Questions

1. **Python vs bash for MCP server.** Bash is dependency-zero but JSON-RPC framing is tedious. Python 3.10+ is on every supported OS by default. Recommendation: start with Python `server.py`, fall back to bash if Python introduces any drift in selftests.
2. **MCP tool granularity.** Should `validate_dod` be one tool or one tool per DoD-class (evidence/diff/scope-status/…)? Recommendation: one tool, with a `class` parameter — matches existing `state-transition-guard.sh` `--check` flag.
3. **Resource versioning.** Should resources include git SHA in URI (`bubbles://gates/G024@<sha>`) or in payload only? Recommendation: payload only; URI is stable name.
4. **Mode collapse: how many primitives?** Plan says ~15. Recommendation: lock at 15 in v6.0; revisit only with operator data after 60 days.
5. **Parallel fan-out concurrency cap.** Should `bubbles/workflows/*.yaml` declare a `maxConcurrentPhases:` field? Recommendation: yes, default 2, override per mode.
