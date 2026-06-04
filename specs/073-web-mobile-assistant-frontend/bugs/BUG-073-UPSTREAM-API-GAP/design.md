# Design — BUG-073-UPSTREAM-API-GAP (Tracking Bug)

**Status:** N/A — tracking bug; design work shipped upstream.

## Why this file is a stub

This is a tracking bug whose resolution lives in two separate upstream
specs. No design work belongs in this folder; the design rationale
lives where the implementation lives.

## Upstream Design References

| Acceptance Criterion | Upstream spec | Design surface |
|----------------------|---------------|----------------|
| AC-1 .. AC-8 (8 JSON endpoints) | [specs/080-knowledge-graph-public-api](../../../080-knowledge-graph-public-api/design.md) | Knowledge graph public API design + Section 3 (endpoint contract table) |
| SCN-073-B06 (annotation editing) | [specs/027-user-annotations](../../../027-user-annotations/design.md) | Annotation editing API design + Scope 9 |

## Why no own design

Resolution shipped in spec 080 (commit `98c16290`, status=done,
certifiedAt `2026-06-04T02:30:00Z`) and spec 027 Scope 9 (commit
`e6ccdb2a`, status=done, certifiedAt `2026-06-03T21:33:19Z`). This
folder exists only to track that the spec 073 wiki/graph-browse
surface had a known backend dependency until those upstream
deliveries landed; it never owned any code.

## Owned changes in this folder

None. All code, tests, scopes, and design rationale live in the
upstream specs above.
