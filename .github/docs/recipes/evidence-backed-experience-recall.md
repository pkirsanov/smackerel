# Recipe: Evidence-Backed Experience Recall

**When to use:** Your repository already holds the record of a problem it is
about to repeat — a compacted result from the incident, the lesson somebody
wrote afterwards, the owner decision that settled it — and none of it is
findable at the moment it matters.

> **Consumer status:** the capability ships with `none` as the default adapter,
> so an untouched repository gets **zero behavioral change**. Opting in gives
> four top-level orchestrators (`bubbles.workflow`, `bubbles.goal`,
> `bubbles.sprint`, `bubbles.iterate`) a bounded advisory lookup, plus a CLI and
> read-only MCP surface for your own use. The one **gating** consumer is
> [`result-envelope-validate.sh`](../../bubbles/scripts/result-envelope-validate.sh),
> and it gates *against* recall: it refuses any envelope that cites a recall
> artifact as evidence. That refusal is active whether or not you enable a
> provider. Treat that as the honest state.

---

## Why this exists

The defect class: **retained experience that cannot be retrieved by relevance.**

Bubbles already preserves a great deal of structured history. Compacted
RESULT-ENVELOPE records carry source pointers. Lessons are written at result
close after a non-obvious fix. Owner decisions live in schema-backed artifacts.
Findings and outcomes are structured.

None of that answered the question an agent actually asks: *"has this repository
hit this before?"* `trajectory-inspector.sh` summarizes chronologically.
`lessons` appends. Neither accepts a relevance query. So the record of the
incident sits in the repository, correctly stored and effectively invisible,
while the same problem is solved again from scratch.

---

## The rule that makes it safe

Recalled experience is **authority tier 4**, permanently:

1. Current source and current-session executed evidence.
2. Active specs, scopes, scenarios, and state.
3. Reviewed lessons and approved skills.
4. **Recalled experience.**

A hit never leaves tier 4. A valid source anchor does **not** promote it — the
anchor tells you where to look, and you must read that source yourself before
relying on it. Every record carries `recallAuthority: advisory` and the schema
rejects any other value.

Recall can never:

- satisfy a DoD item or serve as execution evidence
- authorize a tool or weaken a tool-risk decision
- select, bind, or change a repository
- override current source, specs, scopes, state, or owner decisions
- create, update, or approve a Skill
- dispatch an agent or change ownership

### This is enforced, not requested

Two of those are structural rather than documentary, which is the part worth
understanding before you enable anything:

**Evidence.** `result-envelope-validate.sh` refuses an envelope citing a recall
record id, the recall index directory, or a recall export in `evidenceRefs`,
`toolCalls`, `evidence`, or `dodRef`. It refuses in **every** mode, including
`--advisory`, because citing advisory content as proof is an authority breach
rather than a schema defect. Exports are classified by **content**, not
filename, so renaming the file does not launder it. Narrative fields such as
`summary` are deliberately *not* scanned — an agent should be able to say out
loud that it consulted advisory recall.

**Repository.** The public twin derives its repository root and alias from its
own install location and **refuses** `--repo-root`, `--repository-alias`, and
`--adapter` (exit 2). Cross-repository recall is not a rule you can break; it is
not expressible.

---

## The corpus is closed

Only Bubbles-owned structured artifacts are admissible:

| Kind | Source |
|------|--------|
| `compacted-result` | bound `compactedHistory[]` records with resolvable anchors |
| `lesson` | structured lessons with stable id, repository scope, review state, valid anchor |
| `owner-decision` | schema-backed approvals and accepted improvement decisions |
| `finding` / `outcome` | structured RESULT-ENVELOPE findings and outcomes |

Explicitly **inadmissible**: raw host transcripts, editor conversations, chat
logs, terminal scrollback, screenshots, arbitrary Markdown bodies, source-code
text, and inferred preferences or personas. This is not a warehouse of
everything that happened; it is an index over records that already had
structure and provenance.

Unanchored legacy lessons stay valid input for skill evolution but are excluded
from recall, and `status` reports the exclusion count rather than hiding it.

---

## The shipped adapters

| Adapter | Behavior | Requires |
|---------|----------|----------|
| `none` (default) | returns empty, changes nothing | nothing |
| `local-lexical` | deterministic lexical retrieval over the closed corpus | `python3` (standard library only) |

`local-lexical` adds **no** package, model, daemon, database, network call, or
hosted service. Nothing leaves the repository.

---

## Enable it in your repo

```yaml
# .github/bubbles-project.yaml
experienceRecall:
  adapter: local-lexical
```

Absent block or `adapter: none` both resolve to the neutral provider.

Then build the index:

```bash
bash .github/bubbles/scripts/experience-recall.sh sync
```

---

## The CLI surface

```bash
experience-recall.sh search <query> [--limit N] [--kind KIND] [--trust TRUST]
                                    [--spec-ref REF] [--scope-ref REF]
                                    [--format json|text]
experience-recall.sh read <record-id>
experience-recall.sh status
experience-recall.sh freshness
experience-recall.sh sync
experience-recall.sh delete <record-id> [--reason TEXT]
experience-recall.sh admit <record-id> [--reason TEXT]
experience-recall.sh lifecycle list
experience-recall.sh export --limit N [--output REPO-RELATIVE-PATH]
```

`--limit` is bounded to 1..20 and defaults to 5.

### The exit codes are the contract

| Code | Meaning |
|------|---------|
| 0 | success (freshness: fresh) |
| 1 | provider, engine, or malformed-response failure |
| 2 | usage error |
| 3 | freshness: stale |
| 4 | freshness: unknown |
| 5 | adapter disabled |
| 6 | lifecycle or export refusal |

Note that **3, 4, and 5 are not 0**. "The index is stale", "I cannot tell", and
"recall is off" are distinct from "I looked and found nothing". A consumer that
collapses them into "no prior incidents" has fabricated a clean history.

---

## Lifecycle

`admitted -> superseded | expired | deleted`. Search returns `admitted` records
only. Deletion changes **derived recall state only** — it never deletes or
rewrites the source artifact, and an explicit `admit` can re-admit a previously
deleted anchor.

---

## Retrieval quality is measured, not asserted

A labeled corpus lives at
[`bubbles/eval/fixtures/experience-recall/corpus.json`](../../bubbles/eval/fixtures/experience-recall/corpus.json)
and is scored by
[`experience-recall-eval-selftest.sh`](../../bubbles/scripts/experience-recall-eval-selftest.sh)
on every framework validation.

Measured on the shipped `local-lexical` provider: **macro precision 1.00, macro
recall 1.00** at result bound 5 over 13 queries, with per-query full recall
required.

That number was not free. The first measurement was **precision 0.54**, and a
filler query of pure stopwords returned five confident hits. Two controls fixed
it: a document-frequency guard that zeroes query tokens saturated across the
corpus (the summary template starts with "Problem:", so `problem` carried no
signal at all), and a relevance floor that keeps only hits within half of the
top score. Recall never moved.

---

## Rules for consuming recall

1. **Query after, never before.** Load current source, specs, and state first,
   then recall. Querying first lets a stale record frame how you read current
   truth — the ordering *is* the safety property.
2. **Budget it.** At most 5 hit summaries and 2 drill-downs per phase, at one
   context boundary.
3. **Label it.** Present the block as `advisory recalled experience`.
4. **Discard it** before any repository, tool, DoD, status, Skill, or dispatch
   decision.
5. **Re-read before relying.** Cite the source anchor you re-read yourself,
   never the recall result that pointed you at it.
6. **Never convert absence into innocence.** Empty, disabled, stale, or failed
   recall means the index did not answer — not that nothing went wrong before.

---

## Graceful degradation

Unavailable, disabled, stale, or empty recall must never block a workflow.
Record the observed state and continue without recalled context.

---

## Quote

> Recalled experience is a witness, not a judge. It can tell you where to look.
> It cannot tell you what is true.
