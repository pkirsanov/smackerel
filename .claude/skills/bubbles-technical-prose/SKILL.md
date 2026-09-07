---
name: bubbles-technical-prose
description: Write generated technical prose that a reader can act on without re-reading. Use when authoring or reviewing spec.md, design.md, report.md, scopes.md, managed docs, agent modules, refusal messages, or result-envelope narrative. Covers sentence length, banned punctuation, active voice, one instruction per sentence, and note-versus-warning form. Never applies to verbatim evidence, human-owned uservalidation.md, or release-packet narrative.
---

# Bubbles Technical Prose

## Goal

Make generated prose unambiguous on first read. Every artifact this framework
produces is technical prose that another agent or a human must act on. Prose
that has to be re-read is a defect, and it is the one defect no test catches.

## Portability

Portable governance skill. It contains no project paths, commands, hosts, or
ports.

---

## Where this ranks

Read [`analytical-rigor.md`](../../agents/bubbles_shared/analytical-rigor.md)
first. The Four Rigor Rules are Tier 1. Form is Tier 2. Form never outranks
Rule 3.

A clean, confident, well-punctuated, hollow paragraph is still a failed
analysis. Never reword a finding to satisfy a rule on this page. If a rule here
and an honest finding conflict, the finding wins and the rule yields.

---

## The rules

### Sentence length

Use 20 words maximum for a procedural or instructional sentence. Use 25 words
maximum for a descriptive sentence.

Count words, not clauses. A long sentence usually holds two instructions. Split
it.

### Punctuation

Do not use a semicolon in prose. Split the sentence, or use a list.

**The em dash is permitted.** The source standard says so directly: *"You can
use all standard English punctuation marks except the semicolon (;)."* No other
punctuation mark is restricted.

This matters because a popular retelling of the standard claims it bans the em
dash. That claim is wrong. Adopting it would invent a constraint the standard
does not contain and would flag thousands of conforming sentences as defects.
Do not re-import the folk version.

### Voice

Write in the active voice. Write a procedure in the imperative.

- Write: `Run the guard before the transition.`
- Do not write: `The guard should be run before the transition.`

### Verbs

A verb describes an action. Do not turn a verb into a noun and then attach a
weak verb to it.

- Write: `Validate the manifest.`
- Do not write: `Perform validation of the manifest.`

Do not stack auxiliaries. `will have been able to be run` names no action a
reader can take.

Do not use a phrasal verb where a single verb exists.

- Write `start`, not `kick off`. Write `investigate`, not `look into`. Write
  `cancel`, not `call off`.

### One instruction per sentence

A sentence carries one instruction. A paragraph carries six sentences at most.

If a step has a condition, put the condition first and the action second, in
one sentence. If it has two conditions, use two sentences.

### Notes, warnings, cautions

A note informs. A warning and a caution instruct.

- A note gives information the reader may want. It has no imperative.
- A warning states what will injure a person. It is imperative.
- A caution states what will damage the system or the data. It is imperative.

Never bury an instruction in a note. A reader who skips notes must not lose a
required step.

### Do not compress by deleting words

Do not drop articles, subjects, or verbs to shorten a sentence. `Guard rejects
transition missing evidence` is shorter and worse than `The guard rejects a
transition that has no evidence.`

Shorten by removing an idea that does not belong, never by removing the grammar
that carries it.

### One name per concept

Use one term for one concept everywhere in an artifact. Resolve the term against
the framework glossary before you invent a synonym.

The framework already distinguishes several near-synonyms deliberately. A
**gate** is a policy. A **guard** is the script that enforces it. A **check** is
one assertion inside a run. Using them interchangeably destroys a distinction
other agents rely on.

Prefer `subagent` over `sub-agent`.

---

## Two corrections you must not undo

These are recorded because both errors are widely repeated, and both would
corrupt an implementation that copied them uncritically.

1. **The standard does not ban the em dash.** Rule 8.1 excludes the semicolon
   and nothing else. The word "dash" appears three times in 382 pages, never as
   a prohibition.

2. **A marketing-adjective blacklist is the wrong mechanism.** Words such as
   `seamless`, `robust`, `powerful`, and `leverage` appear zero times in the
   standard. They are excluded by a positive allowlist of approved words, not by
   a list of banned ones. Any blacklist built inside this framework would be the
   hand-maintained list the original argument objects to. Do not build one.

---

## When NOT to use

This skill does not apply to:

- **`uservalidation.md`** — human-owned. Never reword what a person wrote.
- **Verbatim evidence.** Terminal output, logs, diffs, and captured tool results
  are quoted exactly. Rewording evidence to improve its prose is evidence
  corruption, and both `bubbles-evidence-capture` and `bubbles-anti-fabrication`
  forbid it.
- **Release-packet narrative.** Vision, marketing, and business-plan sections
  need voice. Flattening them removes their purpose.
- **Anything quoted from an external source.** Quote it as written.
- **Code, tables, and inline code spans.** These are not prose.

---

## Works well with

- [`bubbles-skill-authoring`](../bubbles-skill-authoring/SKILL.md) — the shape
  of the artifact you are writing.
- [`bubbles-result-envelope`](../bubbles-result-envelope/SKILL.md) — the
  narrative fields an envelope carries.
- [`analytical-rigor.md`](../../agents/bubbles_shared/analytical-rigor.md) — the
  Tier 1 substance floor that this skill sits under.

---

## The standard's own warning

The source standard states that it cannot be used alone: *"It is intended to be
used with other applicable specifications for technical publications, style
guides, and official directives."* Its maintenance group adds that *"if authors
rely blindly on what checkers tell them, they are likely to write rubbish."*

Treat this page the same way. It is additive to substance, never a substitute
for it.
