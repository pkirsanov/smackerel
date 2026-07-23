# User Validation — Spec 104 Universal `/ask` + Self-Knowledge

> Pre-implementation: these are the user-facing behaviors this spec must deliver.
> Items are unchecked until implemented + validated; after audit they flip to
> `[x]`. A user unchecks an item to report a regression.

## Checklist

- [x] The design is grounded in smackerel's real seams (the `openknowledge.Tool` contract, the `RawArtifactPublisher` ingestion pipeline, the `artifacts`/pgvector store with a `source_id` namespace + `embedding vector(384)`, the cite-back verifier, and the `/help` surface) and the best-long-term substrate (a pgvector system-knowledge namespace with real embeddings — NOT an in-memory keyword bolt-on) — validated in design.md against BUG-061-010's live evidence.

### Acceptance behaviors (to validate after implementation)

- [ ] `/ask what can smackerel do?` returns a real, **cited** capability answer — not a refusal, not "saved as an idea".
- [ ] `/ask how does smackerel work as a second brain?` is answered from (and cites) the ingested product overview docs.
- [ ] `/ask what recipes do you have?` answers from the recipe catalog with citations.
- [ ] `/ask` about a topic with no grounded source still refuses **honestly** ("I don't have a sourced answer for that.") — never a hallucination, never "saved as an idea".
- [ ] A product meta-answer never cites or leaks a private personal note.
- [ ] `/help` lists capabilities derived from the same source `/ask` answers from (they never diverge).
- [ ] Adding a new scenario/command/recipe and redeploying makes `/ask what can you do?` reflect it — with no hand-maintained capability doc.
