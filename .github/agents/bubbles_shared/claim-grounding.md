<!-- governance-version: 1.0.0 -->
# Claim Grounding

Canonical contract for the GROUNDING of every factual claim an agent makes, and of every action, edit, recommendation, routing decision, or refusal derived from one.

Load this in any agent that reads a repository, asserts what the system contains, edits a file, or recommends a next step. That is every `bubbles.*` agent.

## Division Of Labor (read this first)

Three modules govern three different questions. They do not overlap and none of them substitutes for another.

| Module | Question it answers |
|--------|---------------------|
| [evidence-rules.md](evidence-rules.md) | Did the command actually RUN? Is the pass/fail claim backed by real captured output? |
| [analytical-rigor.md](analytical-rigor.md) | Is the resulting ANALYSIS deep, honest, and non-generic? |
| **claim-grounding.md** (this file) | Is the underlying FACT true of this repository, and did the agent actually look? |

An agent can execute a command perfectly, report it honestly, and still fail this module. It fails when the claim that motivated the command was never checked. Example: running a test against `parseConfig()` proves the test ran. It does not prove `parseConfig()` was the function the spec named, if the agent never opened the spec.

## The Grounding Rule (ABSOLUTE)

**Every factual assertion about the system, and every action or recommendation derived from one, MUST trace to a source the agent actually read, executed, or retrieved in the CURRENT session.**

Recall is not a source. Plausibility is not a source. Naming convention is not a source. An agent that cannot name where a fact came from does not know the fact.

This rule is not satisfied by being right. A correct guess and an incorrect guess are the same policy violation, because neither is repeatable and neither can be reviewed. The framework already applies this logic to execution in [evidence-rules.md](evidence-rules.md) → Analysis-As-Execution Is Fabrication, which rejects an accurate prediction of a command's output. This module applies the identical standard to facts.

## Admissible Sources (closed list)

A claim is grounded when it traces to one of these four, and to nothing else.

| Source class | What qualifies | What the agent must be able to name |
|---|---|---|
| **Repository artifact** | A file the agent opened this session — source, spec, design, scope, config, schema, registry, doc, lockfile, test | Path, and the region or symbol read |
| **Executed output** | Output the agent captured this session from a real command or tool run | Command, exit code, and the line that carries the signal |
| **Retrieved research** | External documentation, standard, changelog, or upstream source fetched this session | URL or document identity, and the passage relied on |
| **Operator statement** | An instruction, constraint, or decision the operator stated in this session | The stated requirement, kept distinct from any inference drawn from it |

Anything else is ungrounded. In particular, these are NOT sources:

- Training recall about a library, framework, tool, flag, or API surface.
- Inference from a name. A file called `retry_handler.go` does not prove retries are handled.
- Inference from a convention. Another repo doing it that way proves nothing about this repo.
- A prior session, a summary of a prior session, or a compacted history record used as the basis for a NEW claim.
- A subagent's or specialist's assertion that carries no source of its own.
- Operator-pasted context — scrollback, screenshots, another repo's logs — restated as the agent's own finding. This is already fabrication under Gate G021 (AF-BORROWED-CONTEXT). It is repeated here because it is the most common ungrounded input.
- The absence of a search hit, used as proof of absence, when the search surface was never established. "I grepped and found nothing" grounds a claim only when the agent also names WHAT it searched and WHERE.

## Read Before You Assert

Before writing any statement of the form "the system does X", "the file contains Y", "the API accepts Z", or "this already handles W", the agent MUST have opened the artifact that decides it.

Applies to, without exception:

- Function, method, type, field, route, event, and CLI-flag existence and signature.
- Config key names, default values, and whether a default exists at all.
- Gate IDs, script paths, module paths, agent names, spec directories, and skill names.
- What a spec, design, scope, DoD item, or requirement actually says.
- Whether a test exists, what it asserts, and whether it covers the named behavior.
- Whether a dependency, version, or capability is present.

## Read Before You Edit

Before editing a file, read the region being changed and its surrounding context in the CURRENT session. An edit built on a remembered shape of the file is an ungrounded action even when the resulting diff applies cleanly.

Before deleting or replacing anything, establish why it exists. Code that looks redundant is frequently load-bearing. If the reason cannot be established, that is an assumption, and the Assumption Ledger below applies.

## Verify Before You Recommend

A recommendation inherits the grounding obligation of the facts it rests on.

- Do not recommend a command, flag, script, or subcommand without confirming it exists in this repository.
- Do not recommend a pattern as "already used here" without naming where.
- Do not assert that a change is safe, backward compatible, or non-breaking without naming the consumers checked. [consumer-trace.md](consumer-trace.md) owns the inventory rules.
- Do not report an absence — no tests, no handler, no coverage — without naming the search performed.

## No Phantom References (Gate G132)

**An agent MUST NOT cite an identifier or path that does not exist.**

A phantom reference is a citation to a gate ID, script, module, agent, skill, spec directory, doc, or file path that the agent did not confirm exists. Phantom references are the most damaging ungrounded claim, because they read as verified evidence and they propagate. A later agent treats the citation as established and builds on it.

This failure mode is observed, not hypothetical. This repository carries a section in [operating-baseline.md](operating-baseline.md) marked `⛔ DO NOT ACT ON THIS SECTION. Its premise was disproven`, and an entry in `improvements/INDEX.md` recording a withdrawn scope whose premise was `falsified against source`. Both were built on references nobody checked.

Requirements:

1. Confirm the target exists before citing it. Use a read or a search, not memory.
2. A reference to work that is planned but not yet delivered MUST be labeled as such at the point of citation. Never cite an intended gate, script, or path in the present tense.
3. Never invent an ID to fill a slot. If the correct ID is unknown, say it is unknown.

Gate **G132** (`reference_existence_gate`) enforces the mechanical half of this rule. `bubbles/scripts/reference-existence-lint.sh` resolves relative markdown link targets and inline framework paths in the supplied surface, and reports every one that does not exist on disk. It is advisory until a repository sets `referenceExistenceGuard: block` in `.github/bubbles-project.yaml`. Gate ID references are covered separately by `bubbles/scripts/gate-id-grep.sh`.

The lint catches dead paths. It cannot catch a reference that resolves to a real file whose CONTENT does not support the claim. That half stays the agent's obligation.

## Assumption Ledger

Some work cannot proceed without an assumption. That is permitted. Hiding it is not.

When an agent must proceed on an unverified premise, it MUST record the assumption where the work lives — the active `report.md`, the design or spec section that depends on it, or the RESULT-ENVELOPE narrative when no artifact is in scope.

Required shape:

```markdown
> **Assumption**
> **Assumed:** <the unverified premise, stated as a premise and not as a fact>
> **Why unverified:** <what blocked verification — no access, tool unavailable, artifact absent>
> **Blast radius:** <what breaks or must be redone if this is false>
> **Would confirm or refute:** <the concrete read, command, or question that settles it>
```

Rules:

- An Assumption is a positive signal. It carries the same standing as an Uncertainty Declaration in [evidence-rules.md](evidence-rules.md), and the same Honesty Incentive applies: a labeled assumption beats a confident wrong fact.
- An assumption MUST NOT be used to satisfy a DoD item, close a finding, or support a completion claim. Those need evidence. The two records answer different questions — an Uncertainty Declaration explains why a CLAIM could not be verified, an Assumption declares a PREMISE the work stands on.
- An assumption that turns out false is a discovered issue. It takes a disposition under Gate G095 in [operating-baseline.md](operating-baseline.md).
- Never launder an assumption into a fact by restating it in a later artifact, a summary, or a handoff. Carry the label forward.

## Inherited And Delegated Claims

A claim does not become grounded by passing through another agent.

**A subagent summary is a LEAD, not evidence.** It stays a lead until this agent
has read the cited artifact or executed the cited command itself. A summary is a
report that proof exists; it is not the proof. Adopting one as evidence is how a
single unverified assertion propagates through a chain of agents and arrives at
certification looking established.

- When a subagent, specialist, or tool returns a factual assertion the caller intends to ACT on, the caller MUST either receive the source with it, or re-verify it before use.
- The obligation scales with consequence. A subagent's descriptive summary can be relayed as a summary. A subagent's claim that a file, gate, or capability exists MUST be confirmed before it is written into an artifact or used to justify an edit.
- A returned claim that is ungrounded and load-bearing is a finding. Route it back under [workflow-fix-cycle-protocol.md](workflow-fix-cycle-protocol.md) rather than adopting it.

## Grounding Self-Check (run before reporting, and before any edit)

1. For every factual claim I am about to write, can I name the file, command, or source it came from in THIS session?
2. Did I open every artifact I am describing, or am I describing what I expect it to contain?
3. Does every path, gate ID, script, agent, skill, and spec directory I cited actually exist? Did I check, or did I recall?
4. Did I read the region I edited before editing it?
5. Where I could not verify, did I record an Assumption instead of asserting a fact?
6. Am I relaying an inherited claim as established fact without its source?

Any "no" means the claim is ungrounded. Verify it, downgrade it to a labeled Assumption, or remove it. Do not ship it as a fact.

## Related Modules

- [evidence-rules.md](evidence-rules.md) — execution evidence, Claim Source taxonomy, Uncertainty Declaration Protocol
- [analytical-rigor.md](analytical-rigor.md) — depth and honesty of the resulting analysis
- [critical-requirements.md](critical-requirements.md) — Honesty Incentive and the absolute policy set
- [operating-baseline.md](operating-baseline.md) — Discovered-Issue Disposition (G095)
- [untrusted-content.md](untrusted-content.md) — tool-trust boundary for retrieved and pasted content
- [consumer-trace.md](consumer-trace.md) — consumer inventory before renames and removals
