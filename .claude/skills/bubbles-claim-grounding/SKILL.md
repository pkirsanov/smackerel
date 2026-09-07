---
name: bubbles-claim-grounding
description: Never assume or guess. Use before asserting what a file, function, API, config key, gate, spec, or test contains; before editing code you have not just read; before recommending a command, flag, script, or path; before citing a gate ID, script, module, skill, or spec directory; when reporting that something does not exist; when relaying a subagent's or the operator's claim as fact; or when you must proceed on a premise you could not verify.
---

# Bubbles Claim Grounding

## Goal
Every factual assertion about the system, and every action or recommendation derived from one, traces to a source the agent actually read, executed, or retrieved in the CURRENT session. Recall is not a source. Plausibility is not a source. A name is not a source.

## When to use
- Before writing "the system does X", "the file contains Y", "this already handles Z"
- Before editing a file you have not read in this session
- Before recommending a command, flag, script, subcommand, or path
- Before citing a gate ID, script path, shared module, agent, skill, or spec directory
- Before reporting an absence — no test, no handler, no coverage
- Before relaying a subagent result or operator-pasted context as your own finding
- When you cannot verify a premise the work depends on

## The rule
**If you cannot name where a fact came from in this session, you do not know the fact.**

Being right does not satisfy this. A correct guess and an incorrect guess are the same violation, because neither is repeatable and neither can be reviewed. The framework already rejects an accurate prediction of a command's output as fabrication. The same standard applies to facts.

## Admissible sources (closed list)
| Source | Must be able to name |
|---|---|
| Repository artifact opened this session | Path, and the region or symbol read |
| Output captured this session from a real run | Command, exit code, the line carrying the signal |
| External research retrieved this session | URL or document identity, and the passage relied on |
| Operator statement made this session | The stated requirement, kept distinct from your inference |

## Not sources
- Training recall about a library, tool, flag, or API surface
- Inference from a name — `retry_handler.go` does not prove retries are handled
- Inference from a convention — another repo doing it proves nothing about this one
- A prior session, a summary, or a compacted record used to support a NEW claim
- A subagent assertion that carries no source of its own
- Operator-pasted scrollback or screenshots restated as your own finding
- A search that found nothing, when you never establish what you searched and where

## No phantom references
Never cite a gate ID, script, module, agent, skill, spec directory, or path you did not confirm exists. A phantom reference reads as verified evidence and it propagates: the next agent builds on it. Label planned-but-undelivered work at the point of citation. Never write it in the present tense. Never invent an ID to fill a slot.

Mechanically checked for paths by Gate G132 (`bubbles/scripts/reference-existence-lint.sh`) and for gate IDs by `bubbles/scripts/gate-id-grep.sh`.

## When you must assume anyway
Record it. Do not hide it.

```markdown
> **Assumption**
> **Assumed:** <the premise, stated as a premise and not as a fact>
> **Why unverified:** <what blocked verification>
> **Blast radius:** <what breaks or must be redone if this is false>
> **Would confirm or refute:** <the concrete read, command, or question that settles it>
```

An Assumption MUST NOT satisfy a DoD item, close a finding, or support a completion claim. Those need evidence. An assumption later proven false is a discovered issue and takes a disposition under Gate G095.

## Self-check (before reporting, and before any edit)
1. Can I name the file, command, or source behind every factual claim I am about to write?
2. Did I open every artifact I am describing?
3. Does every path, gate ID, script, agent, skill, and spec directory I cited actually exist — checked, not recalled?
4. Did I read the region I edited before editing it?
5. Where I could not verify, did I record an Assumption instead of asserting a fact?
6. Am I relaying an inherited claim as established fact without its source?

Any "no" means the claim is ungrounded. Verify it, downgrade it to a labeled Assumption, or remove it.

## Authoritative governance modules
This skill is a discovery shim. The full enforceable policy lives in:
- `agents/bubbles_shared/claim-grounding.md` — the complete contract
- `agents/bubbles_shared/critical-requirements.md` — policy 25, Grounded Claims Only
- `agents/bubbles_shared/evidence-rules.md` — execution evidence and Uncertainty Declarations
- `agents/bubbles_shared/analytical-rigor.md` — depth and honesty of the resulting analysis

## Sibling skills
- `bubbles-anti-fabrication` — whether a claimed EXECUTION happened
- `bubbles-evidence-capture` — how to record captured output
- `bubbles-quality-gates-catalog` — Gate G132 and the rest of the gate set
