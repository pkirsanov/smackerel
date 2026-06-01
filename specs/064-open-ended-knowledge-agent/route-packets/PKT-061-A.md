# Route Packet PKT-061-A — Provenance Gate Source Taxonomy + Canonical Refusal Taxonomy Amendment

| Field            | Value |
|------------------|-------|
| **Packet ID**    | PKT-061-A |
| **Routed from**  | `bubbles.implement` on `specs/064-open-ended-knowledge-agent` SCOPE-10 |
| **Routed to**    | `specs/061-conversational-assistant` owner (next dispatch via `bubbles.workflow`) |
| **Status**       | `pending` |
| **Date**         | 2026-05-31 |
| **Kind**         | `cross_spec_amendment` |
| **Blocks**       | spec 064 SCOPE-13 (Telegram surface) |
| **Does NOT block** | spec 064 SCOPE-11, SCOPE-12, SCOPE-14, SCOPE-15, SCOPE-16, SCOPE-17, SCOPE-18 (independent — may proceed in parallel) |

---

## 1. Rationale

The Open-Ended Knowledge Agent (spec 064) returns cited answers grounded in
**web search results** and **tool computations** (`unit_convert`,
`calculator`), not only internal knowledge-graph artifacts. The current
provenance gate in `internal/assistant/provenance/gate.go` enforces an
implicit *artifact-only* Source taxonomy: any AssistantResponse whose
`Sources` are not internal artifacts is rewritten to the single canonical
refusal body `"I don't have a sourced answer for that."`.

Under that taxonomy, every successful spec 064 answer with web citations
would be silently refused — defeating the entire purpose of the open-ended
knowledge agent and blocking SCOPE-13 (Telegram surface) from shipping.

Extending the Source taxonomy + canonical refusal taxonomy is the
**minimum** change required to unblock spec 064's user-visible surface
while keeping spec 061's hardened invariants intact (gate still refuses
when `Sources` is empty or contains an unrecognised Kind; canonical
refusal language remains the default for uncategorised refusals).

This amendment is additive. No existing spec 061 scenario changes shape;
no existing gate test should require modification (adversarial: this is
itself an acceptance criterion below).

---

## 2. Diff Proposal (concrete)

### A. `internal/assistant/provenance/gate.go` — extend Source taxonomy

Today the gate treats every Source as opaque-but-internal. Extend it to
accept the explicit taxonomy mirrored from
`internal/assistant/openknowledge/tool.go`:

- `SourceArtifact`        — existing behaviour (internal knowledge-graph artifact reference)
- `SourceWeb`             — web search result (URL + title + snippet hash + retrievedAt)
- `SourceToolComputation` — deterministic tool output (tool name + canonical input hash + output hash)

The `Enforce` function MUST pass when **all** of the following hold:

1. `len(Sources) > 0`
2. Every `Source.Kind` is in `{SourceArtifact, SourceWeb, SourceToolComputation}`
3. (Optional new parameter) the cite-back `verifier.OK` is true when supplied

When (1) or (2) fails, `Enforce` MUST rewrite the response body to the
appropriate `CanonicalRefusalBody` entry (see B). The default canonical
body is preserved for any uncategorised refusal.

**Backward compatibility (NON-NEGOTIABLE):** existing scenarios
(retrieval, weather, notifications, every other spec 061 scenario) produce
`SourceArtifact` sources and MUST continue to pass without modification.
The amendment is additive; the taxonomy now names what was previously
implicit.

### B. `CanonicalRefusalBody` — extend with spec 064 refusal causes

Add the following causes (mirroring `TerminationReason` values from
`internal/assistant/openknowledge/agent/agent.go`). Each refusal line is
kept short per Product Principle P7 (Small, Frequent, Actionable Output)
and ends with the *capture-as-fallback* tail per P9 (Design For Restart,
Not Perfection):

| Cause key                    | Canonical body |
|------------------------------|----------------|
| `budget-exhausted`           | `I couldn't complete that within the answer budget — saved as an idea.` |
| `tool-unavailable`           | `A tool I needed isn't available right now — saved as an idea.` |
| `fabricated-source-blocked`  | `I couldn't verify the sources I would have cited — saved as an idea.` |
| `internal-only-restricted`   | `That requires looking outside your knowledge graph, which is disabled — saved as an idea.` |
| `ambiguous-not-clarified`    | `I couldn't decide what to look up — saved as an idea.` |

The existing default `"I don't have a sourced answer for that."` remains
the canonical body for **any uncategorised refusal**.

### C. `specs/061-conversational-assistant/scenario-manifest.json` — cross-reference only

An `open_knowledge` scenario entry (last-before-capture-as-fallback) is
**owned by spec 064 SCOPE-12**, not by this packet. Spec 061's manifest
SHOULD include a cross-reference noting that SCOPE-12 will register the
entry; the actual entry lands in SCOPE-12. This packet's acceptance
criteria do not require spec 061 to author the entry itself.

### D. `specs/061-conversational-assistant/spec.md` — wording update

Minor: the `AssistantResponse.Sources` section MUST note the extended
taxonomy comes from spec 064 (`SourceWeb`, `SourceToolComputation`),
referencing spec 064 as the amend source. No structural changes.

---

## 3. Acceptance Criteria (what spec 061 owner must demonstrate to close this packet)

1. `internal/assistant/provenance/gate_test.go` has new cases proving
   `SourceWeb` and `SourceToolComputation` pass through the gate when
   present (alone and mixed with `SourceArtifact`).
2. `CanonicalRefusalBody` taxonomy is extended with the five new causes
   above; each cause has a dedicated test asserting the exact body
   string.
3. Existing gate behaviour for `SourceArtifact` is unchanged. Adversarial:
   the pre-amendment `provenance` package test set runs green with **zero
   modifications**.
4. `specs/061-conversational-assistant/report.md` gains an entry
   documenting the amendment via `PKT-061-A`.
5. Spec 061 `state.json` status remains `specs_hardened`. This is a small
   additive amendment within the hardened ceiling per the framework's
   amend-in-place rules. If spec 061 owner determines the ceiling must
   be bumped, the packet returns to spec 064 with that finding.

---

## 4. Spec 064 Dependency

- **Blocks:** SCOPE-13 (Telegram surface) — cannot land until this packet
  is closed because the surface depends on the extended taxonomy +
  refusal language at runtime.
- **Independent (may proceed in parallel):** SCOPE-11 (persistence),
  SCOPE-12 (scenario manifest entry + routing rule), SCOPE-14
  (observability), SCOPE-15 (security), SCOPE-16 (resilience), SCOPE-17
  (e2e), SCOPE-18 (docs).

---

## 5. References

- `internal/assistant/openknowledge/tool.go` — `Source` struct + `Kind` sentinels (`SourceArtifact`, `SourceWeb`, `SourceToolComputation`).
- `internal/assistant/openknowledge/citeback/verifier.go` — `VerifyResult` + `Rejection` reasons.
- `internal/assistant/openknowledge/agent/agent.go` — `TerminationReason` enum (`TerminationCapIterations`, `TerminationCapTokens`, `TerminationCapUSD`, `TerminationFabricatedSource`, etc.).
- `internal/assistant/provenance/gate.go` — current `CanonicalRefusalBody` + `Enforce` signature.
- `specs/061-conversational-assistant/design.md` — current provenance gate section.
- `specs/061-conversational-assistant/scenario-manifest.json` — current scenario taxonomy.
- `specs/064-open-ended-knowledge-agent/design.md` — Source taxonomy extension + refusal taxonomy sections.
- `specs/064-open-ended-knowledge-agent/scopes.md` — SCOPE-10 (this scope), SCOPE-12 (manifest entry), SCOPE-13 (Telegram surface, blocked by this packet).
