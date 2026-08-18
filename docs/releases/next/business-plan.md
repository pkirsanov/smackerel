# Business Plan — Smackerel `next`

## Position in the product arc

`mvp` proved capture and delivery. `next` proves reasoning. That distinction is the
whole business argument for this phase: a system that stores what you read is a
filing cabinet, and filing cabinets are a solved, unpriceable problem. A system that
answers a vague question about what you read, precisely, with sources, from a corpus
you own, is not.

## Target audience for this phase

The **self-hosting knowledge worker** who has already stood Smackerel up and passed
the `mvp` gate. They have hardware, they have a corpus accumulating, and they now
want the corpus to be useful for thinking rather than only for recall.

A secondary, non-paying but strategically important audience arrives in this phase:
**authorized external clients**. The knowledge-graph public API (delivered) and the
planned MCP knowledge server make Smackerel readable by other software the user
already trusts. That converts Smackerel from an application into a substrate.

## Value proposition

Consistent with [`vision.md`](vision.md):

> Ask a vague question, get a precise answer with its sources — from a corpus that
> never leaves hardware you control, using a model you chose at runtime.

Three properties carry the weight, and each maps to a delivered capability rather
than an aspiration:

| Claimed property | Delivered by | Why a competitor cannot trivially match it |
|---|---|---|
| Intent-aware retrieval over one graph | `095` retrieval-strategy routing | Most retrieval-augmented products have exactly one retrieval path. Routing per query intent requires a per-artifact-type contract registry, which requires an artifact taxonomy, which requires passive ingestion — the `mvp` gate's work. |
| Synthesis with attribution, honest refusal on failure | `087` genuine synthesis, `084` reasoning loop | Refusing honestly is a product decision that costs demo quality. Systems optimised for demos fabricate instead. |
| Model is a runtime choice, local by default | `088` runtime-switchable models | A hosted competitor cannot offer local-default inference at all; their unit economics depend on the opposite. |

## Competition and the gap this phase closes

| Competitor class | Example shape | What `next` does that they do not |
|---|---|---|
| Hosted note apps with AI search | Cloud notebook products with a semantic-search add-on | The corpus is theirs, not yours. There is no local-inference default and no unconditional export of the derived graph. |
| Local-first note apps with plugin AI | Markdown-vault editors with community RAG plugins | Retrieval is single-path and the graph is derived per-plugin. No per-artifact-type retrieval contract, no evergreen/ephemeral distinction, no shared graph across sources. |
| Personal RAG scaffolds | Self-assembled vector-store + LLM chains | No passive ingestion, no lifecycle, no honest-refusal contract. They answer confidently when they should decline. |
| Enterprise knowledge platforms | Org-wide search and Q&A suites | Aimed at organisations, priced per seat, and structurally cloud. They cannot serve a single self-hosting individual. |

The honest gap assessment: no competitor class above is *unable* to build intent-aware
retrieval. The defensibility is not the router; it is the router sitting on top of a
passively-ingested, source-qualified, lifecycle-managed graph that took the `mvp`
gate to build.

## Pricing model assumption for this phase

**None. `next` commits to no commercialization.** Smackerel is self-hosted; the
operator supplies the hardware and the inference. There is no per-seat surface, no
metered inference, and no hosted tier in this phase.

The pricing conversation is deliberately deferred — see
[`monetization.md`](monetization.md) for what this phase does and does not unlock.
Marking this "TBD" is a decision, not an omission: introducing a paid surface before
corpus portability lands would contradict Product Principle 11's unconditional-exit
requirement.

## Risk assessment

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | **Blocked promotion cohort.** `096` is blocked behind an owner-directed handoff for the `088` → `087` → `084` cohort. If that handoff stalls, multi-provider model connections never promote. | High | Recorded as OPS-N1 in [`actions.md`](actions.md) with the exact discharge path quoted from the spec's `blockedReason`. The other seven delivered members promote independently. |
| R2 | **Reasoning quality is unmeasured.** `next` ships a router and a synthesiser but no retrieval-evaluation gate. Quality regressions would be invisible. | High | The reserved `next-retrieval-quality-pipeline` slot exists precisely for this and includes an eval gate. Until it lands, quality claims must stay qualitative. |
| R3 | **MCP surface before grant enforcement.** Delivering `109` before `108` would expose the corpus to an external client with no audited per-client grant, contradicting Product Principle 11. | High | Sequencing constraint recorded in [`features.md`](features.md) and enforced by ENG-N2 gating ENG-N3 in [`actions.md`](actions.md). |
| R4 | **Local-default erosion.** Runtime-switchable models make cloud inference one config change away. A convenience default could silently become cloud. | Medium | Constitution C1 and Product Principle 11 make local the shipped default; the principles instruction file greps `config/*.yaml` for a cloud/remote provider default and blocks on it. |
| R5 | **No exit path yet.** Corpus portability (export / import / delete) is a reserved slot, not a delivered capability. Accumulated value without an exit is the switching barrier Principle 11 forbids. | Medium | Reserved slot `next-corpus-portability` with a stated flip condition. This risk is the strongest argument for keeping monetization deferred. |
| R6 | **CORRECTED 2026-08-18 — the original claim is superseded, not amended.** This row previously read *"Gate G101 not wired to CI for this phase … enforcement depends on someone remembering to run it."* That was already false when published: OPS-N3 wired it in commit `088cdef8`. `release reconcile` is now blocking in two places — [`scripts/git-hooks/pre-push`](../../../scripts/git-hooks/pre-push) line 137, and the CI `release-schema` job ([`.github/workflows/ci.yml`](../../../.github/workflows/ci.yml), block comment at line 168, job key at line 181, G101 step running `./smackerel.sh release reconcile`). The same block also wires the G110/G111 release-train guard, so neither release axis is manual. **Genuine residual, narrower than the original:** G101 reconciles the machine-bound `delivery=required` feature set against spec certification truth. It does not check packet *prose* — this row's own staleness survived every green reconcile run, which is the proof. | Low | Enforcement is delivered (OPS-N3, `088cdef8`); no further action is open. Prose accuracy remains a review obligation, not a gate. One seam to know: the pre-push hook is an opt-in per-clone install and CI triggers only on push to `main`/`v*` and PRs to `main` ([`ci.yml`](../../../.github/workflows/ci.yml) lines 5–10), so a branch push from a hookless clone is covered by neither until the PR opens. |
| R7 | **Delivery record shape defect.** Three specs record certified phases in a mixed array that the guard parses only partially. One (`039`) is invisible to the gate despite being certified. | Low | Routed as RTE-M1 / RTE-M2 in [`../mvp/actions.md`](../mvp/actions.md). No `next`-train spec has this shape — verified at HEAD. |
| R8 | **Phase-close ambiguity.** Whether `next` closes on the delivered seven or waits for `108`/`109` is undecided. An open-ended promotion train stops being a promotion train. | Low | Surfaced as OQ-N1 in [`actions.md`](actions.md) with a recommended default. |

## Capital requirements

Not applicable in the conventional sense. Smackerel is self-hosted and this phase
adds no hosted infrastructure, no third-party service dependency that must be paid
for, and no headcount assumption. The operator's existing hardware and their own
model choice are the entire cost base.

The one genuine cost this phase introduces is **operator time on the blocked
handoff** (OPS-N1), which is human attention rather than capital.
