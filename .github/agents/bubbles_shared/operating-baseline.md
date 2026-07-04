# Operating Baseline

Use this file for shared operating behavior instead of duplicating the same session/loading/loop prose in prompts.

## Project-Agnostic Indirection

Agents MUST resolve project-specific commands, ports, paths, and policy details through `.specify/memory/agents.md`, `.specify/memory/constitution.md`, and `.github/copilot-instructions.md`. Do not hardcode project-specific values into portable prompts.

## Framework File Immutability — Upstream-First (NON-NEGOTIABLE)

**Agents MUST NEVER create, modify, or delete Bubbles framework-managed files inside downstream project repos.** These files are owned exclusively by the canonical Bubbles repository and propagated to downstream projects only through `install.sh` upgrades.

**Upstream-First Flow (ABSOLUTE):** ALL Bubbles framework changes — governance docs, agent definitions, shared modules, scripts, workflows, instructions, skills, prompts — MUST be authored in the **canonical Bubbles repository**. Downstream projects receive these updates via the upgrade command (`bash .github/bubbles/scripts/cli.sh upgrade`). Agents MUST NOT edit framework-managed files in downstream repos, and MUST NOT manually copy or sync framework files between repos.

**Multi-Root Workspace Rule:** When working in a multi-root workspace that contains both the canonical Bubbles repo and one or more downstream projects, all framework file edits go to the Bubbles repo. The `.github/` copies in downstream repos are read-only install artifacts — not authoring targets.

Downstream repos may request framework changes via `.github/bubbles-project/proposals/` or `bubbles framework-proposal <slug>`, but they MUST NOT directly edit framework-managed files.

### Framework-Managed Paths (READ-ONLY for agents)

| Path | Owner | Update Mechanism |
|------|-------|------------------|
| `.github/agents/bubbles.*.agent.md` | Bubbles framework | `install.sh` |
| `.github/agents/bubbles_shared/*.md` | Bubbles framework | `install.sh` |
| `.github/bubbles/scripts/*.sh` | Bubbles framework | `install.sh` |
| `.github/bubbles/workflows.yaml` | Bubbles framework | `install.sh` |
| `.github/bubbles/hooks.json` | Bubbles framework | `install.sh` |
| `.github/bubbles/agnosticity-allowlist.txt` | Bubbles framework | `install.sh` |
| `.github/bubbles/*.yaml` (except `bubbles-project.yaml`) | Bubbles framework | `install.sh` |
| `.github/prompts/bubbles.*.prompt.md` | Bubbles framework | `install.sh` |
| `.github/instructions/bubbles-*.instructions.md` | Bubbles framework | `install.sh` |
| `.github/skills/bubbles-*/SKILL.md` | Bubbles framework | `install.sh` |

### Project-Owned Paths (agents MAY modify)

| Path | Owner | Purpose |
|------|-------|---------|
| `.github/bubbles-project.yaml` | Project | Custom quality gates and scan patterns |
| `.github/bubbles-project/proposals/**` | Project | Proposed upstream Bubbles changes requested by this repo |
| `.github/copilot-instructions.md` | Project | Project-specific policies |
| `.specify/memory/agents.md` | Project | CLI entrypoint, commands, naming |
| `.specify/memory/constitution.md` | Project | Project governance principles |
| `specs/**` | Project | Classified work artifacts (feature, bug, ops) |

### What To Do Instead

| Need | Action |
|------|--------|
| Fix a framework script bug | Run `bubbles framework-proposal <slug>` or add a proposal under `.github/bubbles-project/proposals/`, then implement it upstream in the Bubbles repository |
| Add a project-specific quality check | Add to `scripts/` or `.github/bubbles-project.yaml` custom gates |
| Add project-specific scan patterns | Edit `.github/bubbles-project.yaml` `scans:` section |
| Need an agnosticity-lint exception or framework allowlist change | Propose the framework change upstream instead of editing `.github/bubbles/agnosticity-allowlist.txt` locally |

### Violation Detection

The `agnosticity-lint.sh --staged` pre-commit check detects project-specific content in framework files. The downstream `framework-write-guard` verifies that framework-managed files still match the last installed upstream checksum snapshot. Additionally, `install.sh` upgrades will overwrite local modifications, causing silent regression if agents modify framework files locally.

## Loop Guard

1. Start with the smallest role bootstrap that fits the job.
2. Take one real action after the minimum initial context set is loaded.
3. No redundant rereads without a new reason.
4. One feature-resolution attempt before failing fast on an ambiguous or missing target.
5. Read only the files needed for the current phase, gate, or claim.

## Context Loading Profiles

- `planner`: `plan-bootstrap.md`
- `implementer`: `implement-bootstrap.md`
- `tester`: `test-bootstrap.md`
- `analyst`: `analysis-bootstrap.md`
- `designer`: `design-bootstrap.md`
- `docs`: `docs-bootstrap.md`
- `clarifier`: `clarify-bootstrap.md`
- `ux`: `ux-bootstrap.md`
- `validator`: `audit-bootstrap.md` plus project command sources as needed
- `auditor`: `audit-bootstrap.md`
- `orchestrator`: `bubbles/workflows.yaml`, `state.json`, the scope entrypoint, and only the dispatch metadata required for the active step
- `simplifier`: `implement-bootstrap.md`
- `chaos`: `test-bootstrap.md`

## Autonomous Operation

- Non-interactive by default unless the prompt explicitly opts into bounded questioning.
- Fix the smallest blocked unit first, then re-run the narrowest relevant verification.
- Route foreign-artifact changes to the owning specialist instead of editing them inline.
- **Honesty over completion:** When evidence is ambiguous, prefer leaving a DoD item `[ ]` with an Uncertainty Declaration over marking `[x]` with uncertain evidence. A wrong answer is 3x worse than an honest gap. See `critical-requirements.md` → Honesty Incentive.
- **Evidence provenance:** Every evidence block must include a `**Claim Source:**` tag (`executed`, `interpreted`, `not-run`). See `evidence-rules.md` → Evidence Provenance Taxonomy.

## Run-Level Rollback (gitIsolation)

When `gitIsolation=true`, the whole run lives on an isolated branch/worktree, so a failed or abandoned run is cleanly rolled back by dropping that branch/worktree — no partial mutations survive on the working branch. When `gitIsolation=false` (the default), rollback granularity is per-scope instead: `autoCommit=scope|dod` commits land on the working branch and are undone individually. Choose `gitIsolation=true` when a run needs atomic whole-run undo.

## Discovered-Issue Disposition (NON-NEGOTIABLE — Gate G095)

**Every issue an agent observes during work MUST have an explicit disposition. Saying "pre-existing", "unrelated", "out of scope", or "not my session" without filing is forbidden and counts as fabrication.**

When an agent encounters any of the following — hang, crash, regression, broken test, broken script, broken doc, broken link, missing artifact, fragile pattern, security smell, policy violation, suspected bug, performance cliff, undocumented behavior — it MUST choose ONE of these dispositions BEFORE returning control:

| Disposition | When to use | Required evidence |
|---|---|---|
| **fixed-in-session** | The fix is small, in-scope or trivially safe, and you applied it now | Diff / commit SHA, plus targeted re-verification output |
| **bug-filed** | The issue is a defect that needs structured remediation | Path to `specs/<feature>/bugs/BUG-NNN-*/bug.md` you just created |
| **spec-filed** | The issue requires new design / behavior change | Path to `specs/NNN-*/spec.md` you just created |
| **ops-filed** | The issue is operational (infra, deploy, monitoring, governance hygiene) | Path to ops artifact / ticket URL / issue link you just created |
| **routed** | The issue belongs to another owner and you emitted a transition packet | Path to `transition-requests.json` entry with `routedTo` + `routedToCommit\|Spec\|Ticket` |

FORBIDDEN responses to a discovered issue:

- ❌ "The hang in Check 3G is pre-existing and unrelated" — without a filed BUG / TR
- ❌ "Out of scope, skipping" — without an `ops-filed` or `routed` entry
- ❌ "I'll fix this later" — `later` is not a disposition
- ❌ "Known issue" — known where? cite the BUG / TR ID
- ❌ Silently moving on after observing a failure

**Disposition record.** Every discovered-issue disposition MUST be recorded in the active spec's `report.md` under a `## Discovered Issues` section using this shape:

```markdown
## Discovered Issues

| Observed | Description | Disposition | Reference |
|---|---|---|---|
| 2026-05-27 | state-transition-guard.sh Check 3G hangs >60s on real spec dir | bug-filed | specs/<feature>/bugs/BUG-NNN-check-3g-hang/bug.md |
```

If no `## Discovered Issues` section exists and the agent observed at least one issue, the agent MUST add it. If no issues were observed, omit the section entirely (do NOT write "None" — silence is the truthful default).

**Enforcement.** Gate G095 (`discovered_issue_disposition_gate`) scans the agent's RESULT-ENVELOPE narrative for forbidden phrases (`pre-existing.*unrelated`, `out of scope`, `known issue`, `skipping`, `will (?:fix|file) later`, `not my session`) and requires either:

1. The matching phrase to be paired with a concrete artifact reference on the same paragraph, OR
2. The active spec's `report.md` to contain a `## Discovered Issues` row whose `Observed` date matches the current session.

Failure to satisfy either condition emits a `blocked` RESULT-ENVELOPE with finding `G095` whose only remediation is filing the missing disposition.

**This policy applies to ALL agents — orchestrators, specialists, advisory/read-only agents. Read-only agents that cannot file artifacts directly MUST emit a routed transition packet naming the disposition owner.**

## Auto-Approval And Timeouts

- Avoid shell wrapper patterns that trigger approval prompts unless explicitly required.
- Every long-running operation must have an explicit timeout or bounded polling rule.

## Context Compaction Discipline (Orchestrator Agents)

Long-running orchestrator agents (`bubbles.workflow`, `bubbles.sprint`, `bubbles.goal`, `bubbles.iterate`) accumulate `runSubagent` RESULT-ENVELOPEs across many specialist invocations. Without compaction, this leads to context-window pressure, premature self-summarization (lossy), mid-loop truncation, or fabricated continuation. The Bubbles framework requires explicit in-loop compaction.

### When To Compact (BOTH signals — compact when EITHER fires)

- **Count signal:** After every 3 subagent RESULT-ENVELOPEs collected in the active loop.
- **Size signal:** When the accumulated raw RESULT-ENVELOPE text held in working memory exceeds 8 KB.

Compact eagerly, before the next dispatch. Do not wait for the model to start truncating its own output.

### How To Compact

1. For each raw RESULT-ENVELOPE older than the latest 2 (which stay in working memory verbatim):
   - Run `bash bubbles/scripts/context-compactor.sh <raw-result-file>` against the saved raw envelope.
   - Append the resulting single-line JSON record to `compactedHistory[]` in `.specify/memory/bubbles.session.json`.
2. After appending, DELETE that raw envelope from in-context working memory. Keep only the latest 2 raw envelopes plus the full `compactedHistory` ledger in scope.
3. The compactor is idempotent — re-running it on the same input file produces a byte-identical record. Re-compacting is safe.

### What MUST Be Preserved (Non-Negotiable)

- All scope IDs encountered (`scopeIds`).
- All `nextRequiredOwner` chain entries — orchestrators rely on these for routing decisions.
- All `blockedReason` strings — never collapse a blocked finding into "all good".
- All artifact paths (`artifactsCreated`, `artifactsUpdated`).
- The `rawPointer` field — every compact record MUST point back to the original raw envelope file so an operator (or audit) can drill in.

Truncation may only affect verbose narrative or evidence prose, never the structural routing fields above.

### What MUST NOT Be Done

- ⛔ **Never drop blocked findings.** A `blocked` outcome MUST survive every compaction round verbatim.
- ⛔ **Never summarize "all good — proceeding"** without preserving the underlying RESULT-ENVELOPE pointers. The ledger entry IS the proof.
- ⛔ **Never fabricate continuity** by inferring outcomes from earlier compacted records. If a routing decision needs a field that was already compacted, re-read the raw envelope via `rawPointer`.
- ⛔ **Never compact the latest 2 raw envelopes** — they remain in working memory until the next compaction round.

### Anti-Fabrication Tie-In

Compacted records still satisfy the framework's anti-fabrication contract:

- **Gate G021 (Anti-Fabrication):** The `evidenceRefs` array in each compact record IS the cited evidence. Each `rawPointer` MUST resolve to a real file on disk; orchestrators MUST NOT invent compact records.
- **Gate G023 (State Transition Guard):** When a compact record claims an `outcome` of `completed_owned` for a scope's specialist, the underlying raw envelope at `rawPointer` MUST itself satisfy G023 (real DoD evidence, real scope status). Compaction never bypasses this — it only relocates the proof.
- **Gate G083 (Context Compaction Discipline):** The compaction thresholds above (`count > 3` OR `cumulative rawSizeBytes > 8192` for the eligible slice, keeping the latest 2 raw) are enforced mechanically by `bubbles/scripts/compaction-discipline-guard.sh` against `.specify/memory/bubbles.session.json` `envelopesReceived[]`. Eligible envelopes that breach either threshold without a `compactedAt` timestamp fail Gate G083 (exit 1). Orchestrators receiving a Gate G083 violation MUST emit a `blocked` RESULT-ENVELOPE with finding `G083` and remediate by running `bubbles/scripts/context-compactor.sh` on the over-budget envelopes — the compactor additively stamps `compactedAt` so the guard reads the next run as clean. `state-transition-guard.sh` invokes the guard as Check 24; `framework-validate.sh` runs the hermetic selftest on every framework validation pass.

If `rawPointer` ever points to a file that does not exist, the compact record is invalid and MUST be discarded; the orchestrator MUST re-dispatch the specialist to obtain a fresh envelope.

## Trajectory Inspector Health Mode

Orchestrators SHOULD include the single-line trajectory health summary in periodic status updates for long-running framework work:

```bash
bash bubbles/scripts/trajectory-inspector.sh --health --spec specs/<feature>
```

When a retrospective has already produced convergence-health JSON with `bubbles/scripts/retro-convergence-health.sh`, pass it directly:

```bash
bash bubbles/scripts/trajectory-inspector.sh --health --input /tmp/convergence-health.json
```

The output is intentionally one line so it can be pasted into status updates without burying the active finding set:

```text
Convergence Health: turnCount=12 compactionInvocations=2 recapInvocations=1 handoffInvocations=0 blockedFindings=0 status=HEALTHY
```

Use the status as a quick operator signal: `HEALTHY` means no blocked finding or recap/handoff breach was observed, `DEGRADED` means convergence-support activity occurred but did not breach the health threshold, and `FAILED` means blocked findings or recap/handoff overuse need immediate routing attention. This complements Gate G090 retro convergence health: G090 remains the retrospective evidence gate, while `trajectory-inspector.sh --health` is the live status surface.

## Per-Turn State Snapshot

Long-running orchestrator agents (`bubbles.workflow`, `bubbles.sprint`, `bubbles.goal`, `bubbles.iterate`) and any agent doing multi-turn work emit a tiny structured record at the START and END of every turn into `.specify/memory/bubbles.session.json` under a `turnSnapshots[]` array. The records make crash-resume deterministic and give the operator a clear per-turn audit trail of agent decisions.

Hard dependency: `jq` is required (already used elsewhere in the framework). If `jq` is missing, the snapshot script fails loudly and the orchestrator MUST surface that in its RESULT-ENVELOPE — see "When MUST you skip" below.

### What

- Each orchestrator agent calls `bash bubbles/scripts/state-snapshot.sh --mode start --phase <p>` at the beginning of every turn, and `--mode end` at the close, before yielding control back to the operator.
- Each invocation appends a single record to `.specify/memory/bubbles.session.json` `turnSnapshots[]` carrying: `turnNumber` (auto-incremented), `timestamp` (UTC ISO8601), `phase`, `scopeId` (or null), `mode` (`start` | `end`), `note` (or null), and `agent` (from `$BUBBLES_AGENT_NAME`, defaulting to `unknown`).

### Why

- Crash-resume determinism — the next agent (or a re-invoked agent after operator interruption) can read `turnSnapshots[]` and know exactly which phase / scope was active and whether the prior turn completed (had a matching `end`) or crashed mid-turn (only had a `start`).
- Per-turn audit trail — operators and auditors can reconstruct the agent's per-turn decisions without re-deriving them from compacted RESULT-ENVELOPEs.

### When MUST you skip

Never. If the snapshot script fails (e.g., `jq` missing, filesystem read-only), the orchestrator MUST log the failure and continue, but the orchestrator's RESULT-ENVELOPE MUST include `state_snapshot_drift: true` so downstream surfaces can flag the gap.

### What MUST be preserved

- All snapshots from prior turns. The `turnSnapshots[]` array grows monotonically and is NEVER truncated by the snapshot script.
- All non-`turnSnapshots` session fields (e.g., `sessionId`, `compactedHistory`) — the snapshot script only appends to `turnSnapshots[]` and leaves the rest of the session JSON intact.

### What MUST NOT be done

- ⛔ **Never edit a prior turn's snapshot.** Each record is append-only and immutable once written.
- ⛔ **Never call `--mode end` without a matching prior `--mode start` of the same `phase + scopeId`.** A spurious `end` without a prior `start` corrupts the crash-resume signal.
- ⛔ **Never wrap the snapshot call in code that swallows non-zero exits silently.** The orchestrator must observe the failure to set `state_snapshot_drift`.

### Idempotency Note

Two consecutive `--mode start` calls for the same `phase + scopeId` are intentionally allowed — they support resume-after-crash flows where the orchestrator restarts a turn it had already begun. Each `start` still gets its own monotonic `turnNumber` and a fresh timestamp.

## Linter-On-Edit Gate (Project-Pluggable)

Specialist agents (`bubbles.implement`, `bubbles.devops`, `bubbles.simplify`, `bubbles.harden`) MAY invoke `bash bubbles/scripts/edit-lint-gate.sh <changed-file>...` after editing source files. The framework supplies the gate dispatcher; downstream projects supply language-specific linters via `.specify/memory/bubbles.config.json` under `editLintGate.linters`.

Hard dependency: `jq` is required to parse the config (already used elsewhere in the framework).

### What

- Configuration shape (in `.specify/memory/bubbles.config.json`):
  ```json
  {
    "editLintGate": {
      "enabled": true,
      "linters": [
        {"name": "rust-clippy", "match": "*.rs", "command": ["cargo", "clippy", "--no-deps", "--", "-D", "warnings"]},
        {"name": "ts-eslint",   "match": "*.ts", "command": ["npx", "eslint", "--max-warnings=0"]}
      ]
    }
  }
  ```
- Invocation: `bash bubbles/scripts/edit-lint-gate.sh <changed-file-path> [<changed-file-path>...]`.
- Dispatch: For each changed file, the gate matches every configured linter against the file's basename (and full path as fallback) by glob. Each matched linter is invoked with the changed file path appended as the final command argument.
- Exit code: 0 if all matched linters pass; non-zero if any fail. Output (stdout/stderr) from each linter is streamed verbatim.

### Why

- Catches stale-bundle / lint-warnings-from-edit issues before the agent claims completion — a much tighter feedback loop than waiting for the full repo lint at the end of a phase.
- Pluggability avoids hardcoding language-specific tooling in the framework. Rust shops can register `cargo clippy`, TypeScript shops can register `eslint`, Python shops can register `ruff`, etc., without the framework having to know about any of them.

### When OPTIONAL vs REQUIRED

- **Today:** Optional. The gate is opt-in via downstream config; specialist agents MAY call it.
- **Future (v3.9+):** May become required for specialist agents that touch source files, gated on whether the downstream has registered any linters.

### Default Behavior — No-Op (Opt-In Only)

To preserve framework agnosticity, the gate is a no-op when:

1. The config file is missing, OR
2. `editLintGate.enabled` is false (or absent), OR
3. No configured linter matches the changed file's basename or path.

In all three cases the gate exits 0 silently. The framework MUST NOT bundle default linters.

### Anti-Fabrication Tie-In

If downstream's `editLintGate.enabled: true`, agents that invoked the gate MUST include the gate's exit code in their RESULT-ENVELOPE evidence (e.g., as part of the `evidenceRefs` array). A claimed "lint clean" outcome without a recorded gate exit code is treated as fabrication under Gate G021.

## Windowed File Reads

For files >500 lines, read in windows (sections of 200-500 lines) rather than loading the entire file. This:
- Keeps each read operation predictable in size
- Allows targeted edits without retaining unnecessary context
- Reduces token consumption when only a section is needed

Workflow:
1. First pass: read header (lines 1-50) and table of contents
2. Identify the relevant section by line range
3. Read that range with explicit start/end
4. Edit using `replace_string_in_file` against the precise context

Exception: short files (<300 lines) may be read whole. State files (state.json, session.json) are usually small — read whole.

## Classified Work Resolution

- Work only inside classified `specs/...` feature, bug, or ops targets.
- If the target is not found after one resolution attempt, fail fast and report the valid alternatives.