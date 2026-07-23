# Report — Spec 104 Universal `/ask` + Self-Knowledge

## Summary

Planning artifacts authored (spec + design + scopes), grounded in smackerel's real
seams (the `openknowledge.Tool` contract, the `RawArtifactPublisher` ingestion
pipeline, the `artifacts`/pgvector store with `source_id` namespace + `embedding
vector(384)`, the cite-back verifier, and the `/help` command surface). Motivated
by the BUG-061-010 live diagnosis (the bot cannot answer about itself because the
public web knows "smackerel" only as a Super Mario enemy, the personal graph has
no product docs, and the local LLM has no training data on a private product).

Design decision (operator: best for long term, no shortcuts): self-knowledge is a
dedicated `smackerel_self` pgvector namespace, ingested via the existing pipeline
(real embeddings), searched by a new **general** embedding-backed namespace
searcher (resolving the 064 SCOPE-06 deferral) — NOT an in-memory keyword bolt-on.

## Completion Statement

Planning complete (analyze + design + plan). Implementation pending: scopes 1–8 in
dependency order (searcher → corpus → ingest → tool → doc source → /help twin →
trust integration → e2e + deploy).

## Test Evidence

Pending implementation. Each scope's DoD records unit/integration/e2e evidence
against the ephemeral test stack; scope 8 records build + on-host deploy + verify.
