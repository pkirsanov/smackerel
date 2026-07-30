# Operating Baseline

Use this file for shared operating behavior instead of duplicating the same session/loading/loop prose in prompts.

## Repository Authority Baseline

Prompt source, chat CWD, process CWD, active editor, tool CWD, recent file access, workspace declaration order, and host repository metadata are diagnostic-only. They cannot establish, switch, repair, break ties for, or override repository authority.

Repository authority is limited to valid explicit repository or concrete target intent, one valid durable same-session work boundary, or the sole eligible repository in a true single-repository workspace. Every repository-sensitive command requires repository-binding preflight before repository-local state, relative expansion, scans, selection, commands, or dispatch; unresolved multi-root authority refuses with zero repository-local side effects.

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

### Phase-Local Authoring Reference (opt-in bundle reduction) — SUPERSEDED 2026-07-29

**⛔ DO NOT ACT ON THIS SECTION. Its premise was disproven; the reduction it
describes is a no-op.** Retained for provenance, not as guidance. The corrected
account is in the R3 block below.

This section assumed the heavy authoring modules are "pinned into the
orchestrator's always-loaded reference closure", and that the reduction "only
changes WHEN the heavy text is loaded versus ALWAYS". **Neither is true.** A
markdown link inside an `*.agent.md` body is just text; nothing inlines it, so
the modules were ALREADY loaded on demand. A fresh `bubbles.workflow` session
confirmed it holds "only the governance references that *point* to" 
`scope-workflow.md` and never read its contents.

So the proposed change moves a module from on-demand to on-demand. It frees
nothing, while rewriting a `MANDATORY: Follow …` directive — the single edit R3
names as its own motivating hazard. **Cost: real. Benefit: zero.**

The heavy authoring modules — `project-config-contract.md`, `scope-workflow.md`, and `feature-templates.md` — are **phase-local / specialist-owned authoring reference**. They are load-bearing for the PLANNING and AUTHORING specialists (`bubbles.plan`, `bubbles.analyst`, `bubbles.design`, `bubbles.implement`), which need their full text to author specs, scopes, DoD templates, and artifact structure. An orchestrator that *purely routes* — selecting the next scope and dispatching to the owning specialist — does not need the full text of these modules pinned into its own always-loaded reference closure to make a routing decision.

A repo therefore MAY reduce an orchestrator's effective bundle by ensuring these modules are referenced from the phase-specific `*-bootstrap.md` profiles above (loaded when the orchestrator dispatches into that phase) rather than from the orchestrator's own top-level closure. The dispatched specialist still loads the full authoring reference through its bootstrap profile, so no authoring capability is lost — the reduction only changes WHEN the heavy text is loaded (on dispatch into planning/authoring) versus ALWAYS (in the router's closure).

**⚠️ OPT-IN and eval-gated — NOT a completed default (R3).** These modules are load-bearing: an orchestrator body may carry `MANDATORY: Follow ... scope-workflow.md` and reference `project-config-contract.md` for indirection rules, so moving them out of the always-loaded closure is exactly the kind of change a held-out task evaluation exists to catch. A repo MUST validate, via a held-out evaluation showing **zero gate-detection regression**, that the orchestrator still detects and routes every gate correctly BEFORE relying on this reduction. `framework-validate` cannot substitute for that eval — it exercises scripts and selftests, not LLM routing behavior. Until such an eval passes for a given repo, keep the authoring modules referenced exactly as the agent files ship them; do NOT blind-rewire an `*.agent.md` reference to chase a smaller bundle.

**The shipped golden-task corpus does NOT satisfy R3.** `bubbles/eval/` scores output QUALITY — it grades static artifacts with deterministic check types (`contains`, `not-contains`, `file-exists`, `executable-oracle`) and never invokes a model. It therefore cannot observe whether an orchestrator still *detects and routes* a gate after the heavy modules move, which is precisely what R3 requires. Passing `bubbles eval run` is necessary evidence that the corpus baseline is intact; it is not sufficient evidence to perform this reduction. Satisfying R3 needs a routing evaluation that actually runs the orchestrator against held-out scenarios and compares the gates it raised — which this repo does not yet have. Reducing the closure on corpus-green alone would be exactly the substitution the paragraph above forbids.

### Effective-Bundle Budget (opt-in, measured, eval-gated)

The effective bundle is the static transitive closure of `agents/bubbles_shared/*.md` references reachable from an agent file. Two already-shipped scripts operate on it:

- **Measure:** `bash bubbles/scripts/effective-bundle-measure.sh <agent-file>` — reports the closure's `totalBytes` and per-file breakdown for one agent.
- **Budget check:** `bash bubbles/scripts/effective-bundle-budget.sh` — flags agents whose effective bundle exceeds a repo-declared cap.

The budget is **inert until a downstream repo configures it** in `.github/bubbles-project.yaml`:

- `effectiveBundleMaxBytes: <N>` — sets the cap. **Advisory by default**: over-budget agents are flagged and the check still exits `0`.
- `effectiveBundleBudget: block` — makes the cap **blocking** (the check exits `1` when an agent is over budget).

**Measured baseline (for reference):** the largest orchestrator bundle is `bubbles.workflow` at **≈ 491,622 bytes** (measured with `effective-bundle-measure.sh`; note this closure includes `operating-baseline.md` itself, so this figure drifts slightly as the shared modules evolve). Its three heaviest shared modules are `project-config-contract.md` (54,949 B), `scope-workflow.md` (47,621 B), and `feature-templates.md` (20,545 B) — together **≈ 123 KB / ~25%** of the closure, all of it planning/authoring reference (see the Phase-Local Authoring Reference note above).

**⚠️ A BLOCKING budget MUST only be set AFTER a held-out eval confirms no gate-detection regression (R3).** Setting `effectiveBundleBudget: block` before validating the reduction risks forcing an orchestrator below the point where it still loads a load-bearing contract. Start advisory, reduce via the phase-local seam, run the held-out eval, and only then consider making the budget blocking.

**⚠️ UNVERIFIED PREMISE — check this before acting on any bundle figure.** Every
number in this section, and `effective-bundle-measure.sh` itself, rests on the
assertion in that script's header: *"an agent's effective loaded prompt is NOT
just its agent.md — it is that file PLUS every shared contract it transitively
references."* That is an assumption, not a measurement of runtime behaviour, and
direct observation contradicts it.

In a VS Code Copilot session (2026-07-29) the agent received `*.instructions.md`
files inline — they carry `applyTo: "**"` — and skills as *descriptions plus
paths*, to be fetched with `read_file` on demand. It did NOT receive
`scope-workflow.md`, `project-config-contract.md`, or `feature-templates.md`
inline. Those were read on demand, when needed.

If that holds generally, the transitive closure measures **everything reachable
by link**, not everything loaded — a different quantity, and probably a much
larger one. A reduction chasing the closure figure would then be optimising a
number that is not the context cost, while still changing the orchestrator's
contract. It would also explain why the 160,000 B target never looked reachable.

**Scope of this evidence:** first-person and behavioural, for `bubbles.goal`.
That agent's file references `agent-common.md`, `operating-baseline.md`, and
`scenario-compile.md` (4-module closure). The running session received **none**
of their content — demonstrated, not asserted: when it needed R3's text it had to
`grep` for it, and its first pattern missed. An agent cannot search for text it
is already holding.

The mechanism explains it. `applyTo: "**"` instruction files are inlined by an
explicit VS Code feature. A markdown link inside an `*.agent.md` body is just
text; nothing resolves or inlines it. The agent must call `read_file`. So the
"transitive closure" is a documentation-linkage graph, not a prompt.

**CONFIRMED for `bubbles.workflow` — 2026-07-29.** A fresh `bubbles.workflow`
session was asked, without tools, to define the "Isolated Design-Experiment
Contract" and the `contextFit` field from `scope-workflow.md`. It answered:

> "That text is not in my context. I have not loaded `scope-workflow.md` from the
> bubbles repo in this session. What I have is only the governance references
> that *point* to it … the file's actual contents were never read."

That is the measured agent, not an inference from another one, and it draws the
exact distinction at issue: it holds the POINTERS, never the CONTENTS.

**Therefore the closure is NOT the loaded prompt, and every byte figure in this
section is a linkage measurement rather than a context cost.** Do NOT budget,
reduce, or set `effectiveBundleBudget` against these numbers believing they are
context. A reduction that moves an already-on-demand module frees nothing and
only risks the routing behaviour R3 protects.

**Measured 2026-07-29 — read this before attempting the reduction.** The three
modules named above were analysed with `bubbles/scripts/gate-attribution.sh`
(deterministic; resolves the closure and reports which modules are the SOLE
carrier of a gate reference). Findings:

| Question | Answer |
|---|---|
| Closure modules / gate ids referenced | 42 / 100 |
| Gates with exactly one carrying module | 54 |
| Moving all three to on-demand | frees 130,284 B, leaves **0 gates unreachable** |
| Gates that become pointer-only | **7** — G005 G047 G048 G051 G199 G900 (`project-config-contract.md`), G037 (`scope-workflow.md`) |
| Modules carrying no sole gate | 33, totalling 236,194 B |

Use `gate-attribution.sh <agent.md> --ondemand <mods>` before ANY reduction. It
decides reachability exactly and needs no model, so it is valid where a routing
eval is unavailable. Reachability is necessary, NOT sufficient: it proves the
agent CAN still reach a gate, never that it WILL load the module at the right
moment. The 7 pointer-only gates above are the routing eval's priority cases —
the other 93 keep an always-loaded carrier and are unaffected by construction.

**The 160,000 B target is not reachable by this reduction.** 505,847 - 130,284 =
375,563 B, still 215,563 B over. Closing that gap needs reducing the agent file
itself (72,472 B) plus further on-demand moves, each with its own reachability
and routing questions. Do not treat the three-module move as sufficient.

**Two measurement traps, both hit for real while producing the numbers above.**
A surrogate model reading the bundle over HTTP and listing gate ids does NOT
satisfy R3 — it measures that model's text recall, not the orchestrator's
routing, so its verdict is invalid at any context size. And an over-long prompt
is silently truncated from the FRONT by some servers, which produced a
reproducible-but-wrong result: determinism proved stability, never validity.
Verify the instrument measures the intended subject before trusting its
resolution.

**Deduplication is not available here.** The anti-fabrication doctrine across
`critical-requirements.md`, `agent-common.md`, `quality-gates.md` and
`evidence-rules.md` was measured for redundancy: 68 distinct normative clauses,
**0** appearing in more than one file; 55 gate ids, **2** shared. These files
PARTITION the doctrine rather than repeat it. Their combined size is not
duplicated content, and collapsing them would delete normative statements rather
than dedupe them.

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
| **status-adjusted** | The issue invalidates a completion claim on an EXISTING spec/scope/DoD item | The edited `state.json` (`status` moved off `done`, and/or `requiresRevalidation:true`) and/or the unchecked `- [ ]` DoD item — saved THIS turn, paired with a `bug-filed` entry for the underlying defect |

**File on discovery — not report-and-wait (NON-NEGOTIABLE).** A disposition is discharged by CREATING or EDITING the tracked artifact NOW, in the SAME turn you observed the issue — never by describing it in chat prose, a findings table, or a `report.md` appendix and then waiting. Concretely:

- `bug-filed` / `spec-filed` / `ops-filed` mean the artifact EXISTS ON DISK as of this turn (the bug folder with its required artifacts, the spec, the ops ticket) — not "will be filed", not "recommended for a future workflow", not "documented for later".
- `routed` means a transition packet naming a CONCRETE owner + target was emitted THIS turn — not "deferred to a future delivery-mode workflow" with no packet.
- When a finding shows a `done` spec/scope is not actually done (unimplemented requirement, broken behavior, missing coverage), you MUST immediately flip the artifact: move `status` off `done` OR set `requiresRevalidation:true`, uncheck the affected `- [ ]` DoD item(s), set the scope status — AND file the `bug-filed` entry for the defect. Do this in the same turn; do NOT report "this spec looks over-certified" and wait.
- **Filing is NOT user-gated.** Never end a turn by handing the user a list of unfiled findings and asking whether or what to file. The default, unconditional action on discovery is to FILE; THEN report what you filed. "Should I file these?" is not a valid stopping point — file them, then tell the user they are filed.

FORBIDDEN responses to a discovered issue:

- ❌ "The hang in Check 3G is pre-existing and unrelated" — without a filed BUG / TR
- ❌ "Out of scope, skipping" — without an `ops-filed` or `routed` entry
- ❌ "I'll fix this later" — `later` is not a disposition
- ❌ "Known issue" — known where? cite the BUG / TR ID
- ❌ Silently moving on after observing a failure
- ❌ "Here are the findings — let me know which to file" — filing is not user-authorized; file first, report second
- ❌ "Recommend filing these as bugs" / "should be tracked in a follow-up workflow" — a recommendation is not a filing; create the artifact now
- ❌ Documenting a finding in `report.md` prose or an appendix as the ONLY action — the tracked artifact (bug folder / status flip) MUST also exist this turn

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
  - For a repository-sensitive result, first validate the current actionable packet, then run `bash bubbles/scripts/context-compactor.sh --session-id <session-id> --session-control-file <control-file> --binding-packet-file <packet-file> <raw-result-file>` against the saved raw envelope. The compactor refuses a repository-sensitive result when any binding input is omitted, stale, substituted, redacted, or malformed.
  - For a legacy result with no repository binding fields, run `bash bubbles/scripts/context-compactor.sh <raw-result-file>`.
   - Append the resulting single-line JSON record to `compactedHistory[]` in `.specify/memory/bubbles.session.json`.
2. After appending, DELETE that raw envelope from in-context working memory. Keep only the latest 2 raw envelopes plus the full `compactedHistory` ledger in scope.
3. The compactor is idempotent — re-running it on the same input file produces a byte-identical record. Re-compacting is safe.
4. Before repository-local work resumes from a compacted record, reconstruct the packet from `repositoryRoot`, `repositoryAlias`, and the nested `repositoryResolution`, then run `bubbles/scripts/repository-binding.sh validate-packet` against the current control record. A failed validation is a refusal before reads or dispatch. Never reconstruct repository identity from CWD, prompt text, workspace order, or the flattened compatibility fields.

### What MUST Be Preserved (Non-Negotiable)

- All scope IDs encountered (`scopeIds`).
- All `nextRequiredOwner` chain entries — orchestrators rely on these for routing decisions.
- All `blockedReason` strings — never collapse a blocked finding into "all good".
- All artifact paths (`artifactsCreated`, `artifactsUpdated`).
- The exact current repository decision: `repositoryRoot`, `repositoryAlias`, and every nested `repositoryResolution` field (`sessionId`, `decisionId`, `controlRevision`, `controlPathDigest`, `authority`, `transition`, `scopeKind`, `scopeId`, `targetKind`, `pathVisibility`, `actionable`).
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

Operator-supplied context — pasted screenshots, terminal scrollback, another repository's logs, or another session's state — is DIAGNOSTIC INPUT ONLY. It MUST NOT be restated as the agent's own execution evidence, and MUST NOT be used to infer an active work mandate. Work is authorized only by the operator's explicit request in the current conversation (and, for repository selection, by IMP-103 repository-binding preflight).

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

- Each orchestrator agent calls `bash bubbles/scripts/state-snapshot.sh --mode start --phase <p> --session-id <session-id> --session-control-file <control-file> --binding-packet-file <packet-file>` at the beginning of every turn, and repeats the complete binding triplet with `--mode end` at the close, before yielding control back to the operator.
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