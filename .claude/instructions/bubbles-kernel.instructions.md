---
applyTo: "**"
---

# Bubbles Universal Kernel (NON-NEGOTIABLE)

> **Why this file is small.** It is the ONLY Bubbles instruction that is always
> on, so every byte here is re-sent on every request. Everything that can be
> conditioned on a file surface lives in a narrowed instruction instead — see
> the pointer table at the end. What remains are invariants that must hold on a
> request that touches no agent file, no test, and no config: truthfulness,
> evidence integrity, and repository authority.
>
> Introduced by IMP-039 SCOPE-6 when `bubbles-agents.instructions.md` (15,754 B)
> and `bubbles-env-pollution-isolation.instructions.md` (4,933 B) were narrowed
> off `applyTo: "**"`. Both are authoring/testing governance and were being paid
> on every unrelated request.

## 1. Repository Authority (refuse before acting)

Before any repository-sensitive Bubbles command, resolve host context with the
installed `repository-binding-host-context.sh` (`bubbles/scripts/` in the source
repo, `.github/bubbles/scripts/` downstream). In VS Code pass the host-provided
per-chat value exactly as:

```text
--session-log "{{VSCODE_TARGET_SESSION_LOG}}"
```

Pass every host-declared workspace folder as an explicit `--workspace-root`.
Consume the adapter's `sessionId`, `sessionControlFile`,
`expectedControlRevision`, and canonical `workspaceRoots` in
`repository-binding.sh preflight`, forwarding `expectedControlRevision` verbatim
as `--expected-control-revision`. On a stale revision, rerun the host adapter and
retry with its new observation rather than guessing.

NEVER derive session identity or repository authority from CWD, prompt source,
active editor, workspace order, process ID, generic host repository metadata, or
a repo-local session file. If the host does not resolve the session-log token,
REFUSE before any repository-local read or write. There is no fallback.

## 2. Anti-Fabrication (no claim without execution)

- Every pass/fail claim maps to a command that was actually executed, with its
  real output and real exit code.
- Status stays `in_progress` or `blocked` when evidence is missing or
  contradictory. A terminal status is a claim like any other.
- Apply Fabrication Detection Heuristics (G021), Sequential Spec Completion
  (G019), Specialist Completion Chain (G022), and the Mandatory Completion
  Checkpoint before reporting anything complete.
- Operator-supplied context — pasted screenshots, scrollback, another
  repository's logs, another session's state — is DIAGNOSTIC INPUT ONLY. It must
  never be restated as the agent's own execution evidence.

## 3. Evidence Integrity

- Evidence comes from real execution in the CURRENT session.
- Above 40 lines, the default shape is the bounded block from
  `bubbles/scripts/evidence-capture.sh`: command, exit code, line count, a
  sha256 over every line produced, the failure-shaped lines, and the first and
  last 20 lines. It is stronger than a paste because `--verify` re-derives the
  hash. Below 40 lines, show the output as it came back.
- Never filter a command through a discarding pipe. Bounding what RE-ENTERS
  context is not the same as discarding what the command PRODUCED.

## 4. Never Wait Forever

Every operation carries an explicit time limit. Wrap commands with
`timeout <duration>`; no unbounded loops; health-check polling is capped (30 × 2s);
background processes are checked at most 10 times. On timeout: log it, kill the
hung process, report the failure, and do NOT auto-retry without approval.

## 5. Where Everything Else Lives

| Surface | Instruction | `applyTo` |
|---|---|---|
| Agent / prompt / instruction / skill authoring | `bubbles-agents.instructions.md` | agent + prompt + instruction + skill files |
| Test, compose, monitoring, backup isolation | `bubbles-env-pollution-isolation.instructions.md` | test / compose / monitoring / backup surfaces |
| Cross-platform shell (WSL + macOS) | `bubbles-wsl-macos-compatibility.instructions.md` | shell / hook / workflow / make surfaces |
| Full policy set | [agent-common.md](../agents/bubbles_shared/agent-common.md), [critical-requirements.md](../agents/bubbles_shared/critical-requirements.md), [scope-workflow.md](../agents/bubbles_shared/scope-workflow.md) | loaded on demand |
| Gates, modes, phases | [workflows.yaml](../bubbles/workflows.yaml) | loaded on demand |

A rule that can be conditioned on a file surface belongs in a narrowed
instruction, not here. Adding to this file costs every request in every
downstream repository.
