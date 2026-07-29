# User Validation: 109 MCP Knowledge Server

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened` · **Planning only — no source edited.**

Items are **checked `[x]` by default**: each records a planning decision this packet made and
believes to be correct. **Uncheck `[ ]` any item that is wrong, unacceptable, or not what you asked
for** — an unchecked item is a blocking regression report and must be resolved before implementation
proceeds.

Nothing below claims a capability is built. No MCP server exists yet (`spec.md` §1).

---

## Checklist

### Scope Plan

- [x] Seven scopes is the right decomposition, and each is independently testable.
- [x] Scope 01 (foundation) correctly blocks everything else — no toolset can be authorized, projected, refused, or audited before the credential, authorizer, projection type, refusal vocabulary, ledger, and `/mcp` mount exist.
- [x] Scope 02 delivering `context` **before** `memory-read` is right: determinism and the no-existence-oracle rule are proven before any corpus tool exists.
- [x] Scope 05 (`graph-read`) depending on Scope 04 (`person-context`) is right: with only one opt-in toolset, "the challenge names exactly one scope" is trivially satisfied and cannot detect a catalog leak.
- [x] Scope 07 (docs/release/config) **not** depending on Scope 06 is right: Scope 06's documentation obligation is the honest *unavailability* statement, which is writable today.
- [x] `memory-write` (`SCN-109-013`, `SCN-109-014`) is correctly **not planned** here — D5 sets it off, and `design.md` §8 already settles the mapping so the deferral is not a future redesign.

### Blocked Work

- [x] Scope 06 (`hospitality-read`) is correctly marked **Blocked** on BUG-019-003 rather than planned as deliverable.
- [x] Planning the *honestly unavailable* behavior now (registered with `Readiness.Available=false`, omitted from `tools/list`, method-not-found on invocation, `capability-unavailable` on the operator surface) is right, and is better than omitting the toolset entirely — it tells you why a grant you made has no effect.
- [x] Not implementing a guest-context handler body against a blocked dependency is right; a handler that cannot be served would be a stub.
- [x] Emitting **no** `external_ref` anywhere, rather than synthesizing one, is right (`spec.md` §9 honesty rule; §18 item 5).

### Security & Egress Posture

- [x] MCP owning its own audience-bound credential and its own authorizer (D3) — never reusing `bearerAuthMiddleware` — is right.
- [x] Verifying the audience with a **positive equality** rather than a negative filter is right: a token from today's `IssueToken` carries no `aud` at all and would pass every negative filter.
- [x] The corpus grant being **MCP-owned** (`design.md` §4, resolving UX-F-001) rather than consuming spec 108's `corpus:read` is the right call, including its cost: OF-2 now needs a data-scope grant/revoke step.
- [x] A corpus denial rendering as `unauthorized-toolset` / Case A — byte-identical to a nonexistent tool — is right, even though it is less informative to the client, because a distinct token would be an existence oracle.
- [x] Egress running as gate 4 (after toolset and readiness, before dispatch) is right, and testing it with a **spy executor that must record zero `Retrieve` calls** is the right proof.
- [x] `remote-inference` being a per-client operator grant and deliberately **not** a feature flag is right — no build can enable it globally.
- [x] Stating honestly in every document that Smackerel **cannot verify** a client's declared inference locality (administrative and auditable, not cryptographic) is right, even though it weakens the marketing claim.

### Content & Provenance

- [x] `content_raw` never leaving `/mcp` under any configuration — including a fully local client you trust — is right, with no "raw mode" escape hatch.
- [x] Enforcing it **structurally** (a projection type that cannot express raw content) rather than by review or a struct tag is right.
- [x] Treating the marshal-time guard as a **detector**, not the control, is right.
- [x] All six provenance fields being mandatory and non-omittable — including `retrieval_reason` and `retrieval_contract_known`, which §2's Success Signal omits (UX-F-004) — is right.
- [x] A retrieval fallback being `isError=false` / `degraded-fallback` rather than an error is right; and "no results found" over a provider error being forbidden is right.

### Documentation, Release, And Configuration (Scope 07)

- [x] Every `spec.md` §15 documentation row is planned with a concrete DoD item: `docs/smackerel.md` §17.2, §21 (MCP row — Fabric.so already ships MCP), §22 (sibling integration capability, not a connector), `docs/API.md`, `docs/Operations.md`, `docs/Architecture.md`, `docs/INVESTOR_OVERVIEW.md`.
- [x] Adding the `docs/smackerel.md` §17.1 row (UX-F-002) as a recorded planning decision — rather than silently amending `spec.md` §15 — is right.
- [x] The capability-ledger surface recording the honest delivery state (`hospitality-read` and `memory-write` as **specified, not delivered**, with reasons) is right.
- [x] The `next` release packet under `docs/releases/` and its `features.md` carrying the MCP entry, flag name, owning train, honest delivery state, and the `remote-inference` opt-in is right.
- [x] `mcpKnowledgeServer` default-ON in **exactly one** train (`next`) and default-OFF in `mvp` is right, and the parity test enforcing it is right.
- [x] `mcp.knowledge_server` resolving fail-loud with **no** fallback default — absent or malformed aborts startup — is right.
- [x] The `docs/Product-Principles.md` alignment note citing **Principle 8** (UX-F-003) and carrying the §16 honest discrepancy verbatim, without inventing a local-first principle, is right.

### Findings Handling

- [x] UX-F-001 is genuinely **resolved** by `design.md` §4 — not merely reworded.
- [x] UX-F-002, UX-F-004, and UX-F-005 remain **OPEN** with named owners, and were not silently resolved by this packet. UX-F-003 is **partially resolved** — by the operator's own ratified §18 item 7, not by this packet.
- [x] F-109-OF2-AMEND (new, surfaced by `design.md` §4) is recorded as open and routed rather than applied to `spec.md`.
- [x] F-109-002 is handled honestly: MCP owns its own wiring, funds live coverage, and discharges F-095-E2E-LIVE **for the MCP path only** — no artifact claims spec 095 is generally proven.

### Evidence Honesty

- [x] `report.md` records **no** test output, because no test has been run and none could be.
- [x] `state.json` sits at `specs_hardened` — the mode ceiling — and never claims `done`.
- [x] `spec.md` and `design.md` were read only; no source file was created or modified under the G073 lockout; no other spec folder was touched.

### Operator Decision Record — RATIFIED (`spec.md` §18)

These seven decisions were **ratified by operator delegation on 2026-07-29** under the instruction
*"pick the best option for long term, no shortcuts."* They are recorded here as **ratified**, not
pre-decided by this packet. `spec.md` §18 carries the full rationale for each.

- [x] **1. D1 acceptance** — ACCEPTED. `remote-inference` is a per-client grant, administratively controlled and audited but **not technically verifiable**. Binding honesty constraint added: the only permitted claim is *"Smackerel never sends your knowledge anywhere; a client you explicitly authorize may."* Any text implying Smackerel **enforces** client-side locality is a defect.
- [x] **2. D2 absoluteness** — ACCEPTED, permanent. `content_raw` is never returned over MCP under any configuration; no "raw mode" escape hatch, and no future spec may add one without superseding D2.
- [x] **3. D4 permanence** — ACCEPTED **with a carve-out**. MCP prompts, MCP Apps, subscriptions, and `notifications/tools/list_changed` are **permanently deleted**. OAuth 2.1 + DCR + RFC 9728 are **NOT** deleted — they are out of scope with a named re-open trigger: *re-opens if and only if a client outside the operator's control must connect to `/mcp`*. D4 and §13 were amended accordingly.
- [x] **4. `external` toolset** — ACCEPTED, permanently off; Smackerel will never be an MCP proxy server.
- [x] **5. J4 sequencing** — ACCEPTED. `hospitality-read` ships only after BUG-019-003; the STR host job waits rather than shipping a fabricated external reference.
- [x] **6. Spec 095 exposure** — ACCEPTED. MCP is the first live consumer of `internal/retrieval/routing`, with live integration and e2e coverage funded here. Side benefit (retires part of 095's deferred debt for the MCP path) and risk (first-consumer defects are expected) are both recorded.
- [x] **7. Principle gap** — RESOLVED by **addition**: `docs/Product-Principles.md` gains `Principle 11 — Your Data Stays Yours`; Constitution C1 is not the sole carrier. The edit itself is a delivery-time Scope 07 obligation **requiring owner sign-off**, together with a matching block in its BLOCKING companion enforcement file.

**Not ratified by this gate.** UX-F-002, UX-F-004, and UX-F-005 were bundled into "the §18 pass" for
convenience but are `spec.md` wording corrections, not product decisions. They remain **OPEN** and
are re-routed to `bubbles.analyst`.
