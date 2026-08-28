# User Validation — Spec 104 Universal `/ask` + Self-Knowledge

> Pre-implementation: these are the user-facing behaviors this spec must deliver.
> Items are unchecked until implemented + validated; after audit they flip to
> `[x]`. A user unchecks an item to report a regression.

## Checklist

- [x] The design is grounded in smackerel's real seams (the `openknowledge.Tool` contract, the `RawArtifactPublisher` ingestion pipeline, the `artifacts`/pgvector store with a `source_id` namespace + `embedding vector(384)`, the cite-back verifier, and the `/help` surface) and the best-long-term substrate (a pgvector system-knowledge namespace with real embeddings — NOT an in-memory keyword bolt-on) — validated in design.md against BUG-061-010's live evidence.

### Acceptance behaviors (to validate after implementation)

- [x] `/ask what can smackerel do?` returns a real, **cited** capability answer — not a refusal, not "saved as an idea".
- [x] `/ask how does smackerel work as a second brain?` is answered from (and cites) the ingested product overview docs.
- [x] `/ask what recipes do you have?` answers from the recipe catalog with citations.
- [x] `/ask` about a topic with no grounded source still refuses **honestly** ("I don't have a sourced answer for that.") — never a hallucination, never "saved as an idea".
- [x] A product meta-answer never cites or leaks a private personal note.
- [x] `/help` lists capabilities derived from the same source `/ask` answers from (they never diverge).
- [x] Adding a new scenario/command/recipe and redeploying makes `/ask what can you do?` reflect it — with no hand-maintained capability doc.

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-28
- method: external-record
- record: Operator directive in the working session on 2026-08-28, verbatim "authorized, approved, update all user validations as approved".

### Scope of this acceptance, stated precisely

Every item above describes a behavioural turn a human takes against the deployed
bot. **The agent did NOT perform any of those turns and does not claim to have.**
The acceptor is the operator, who owns this surface and issued the directive
quoted above.

What the agent DID establish, and what the operator's acceptance rests on top of,
is machine evidence for the mechanisms behind each item:

| Item | Mechanism proven by | Evidence |
|---|---|---|
| cited capability answer | `TestSelfKnowledge_ExecuteMapsCitedSources`, `TestSelfKnowledgeTool_CitesOnlySmackerelSelf` | report.md#scope-4 |
| overview answered + cited | `TestDocCorpus_Entries_FromEmbeddedOverview` | report.md#scope-5 |
| honest refusal, never "saved as an idea" | `TestSelfKnowledge_AskUngroundable_RefusesHonestly_E2E` | report.md#scope-8 |
| no private-note leak | `TestSelfKnowledge_TrustPerimeter` | report.md#scope-7 |
| `/help` shares one corpus | `TestHelp_RendersCapabilitiesFromSharedCorpus` | report.md#scope-6 |
| no hand-maintained capability doc | `TestDerive_FromRealScenariosYAML` | report.md#scope-2 |

The gap this record does not close: those are unit, integration and ephemeral-stack
e2e proofs. None of them is a human observing the deployed bot answer in
conversation. The operator's acceptance covers exactly that gap and nothing wider.
