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
- [x] All six provenance fields being mandatory and non-omittable — including `retrieval_reason` and `retrieval_contract_known`, which §2's Success Signal originally omitted (UX-F-004, corrected in §2 on 2026-07-29 so §2 now agrees with §9) — is right.
- [x] A retrieval fallback being `isError=false` / `degraded-fallback` rather than an error is right; and "no results found" over a provider error being forbidden is right.

### Documentation, Release, And Configuration (Scope 07)

- [x] Every `spec.md` §15 documentation row is planned with a concrete DoD item: `docs/smackerel.md` §17.2, §21 (MCP row — Fabric.so already ships MCP), §22 (sibling integration capability, not a connector), `docs/API.md`, `docs/Operations.md`, `docs/Architecture.md`, `docs/INVESTOR_OVERVIEW.md`.
- [x] Adding the `docs/smackerel.md` §17.1 row (UX-F-002) is right — originally recorded as a planning decision rather than a silent spec amendment by a non-owner, and since **applied to `spec.md` §15 by its owner** on 2026-07-29.
- [x] Declaring `docs/Product-Principles.md` as a `spec.md` §15 documentation target (added 2026-07-29) is right — TP-07-03 already asserted against it, so the spec now declares what the test checks.
- [x] The capability-ledger surface recording the honest delivery state (`hospitality-read` and `memory-write` as **specified, not delivered**, with reasons) is right.
- [x] The `next` release packet under `docs/releases/` and its `features.md` carrying the MCP entry, flag name, owning train, honest delivery state, and the `remote-inference` opt-in is right.
- [x] `mcpKnowledgeServer` default-ON in **exactly one** train (`next`) and default-OFF in `mvp` is right, and the parity test enforcing it is right.
- [x] `mcp.knowledge_server` resolving fail-loud with **no** fallback default — absent or malformed aborts startup — is right.
- [x] The `docs/Product-Principles.md` alignment note citing **Principle 8** (UX-F-003) and carrying the §16 honest discrepancy verbatim, without inventing a local-first principle, is right.

### Findings Handling

- [x] UX-F-001 is genuinely **resolved** by `design.md` §4 — not merely reworded.
- [x] UX-F-002, UX-F-004, and UX-F-005 were correctly recorded **OPEN** with named owners at
  planning close and were **not** silently resolved by the planning packet. **All three were
  subsequently RESOLVED on 2026-07-29** by their recorded owner `bubbles.analyst`: §15 gained the
  `docs/smackerel.md` §17.1 row (UX-F-002); §2's Success Signal now enumerates the complete
  six-field §9 provenance set (UX-F-004); and D1 now reads "fully **specified** but
  **default-OFF**" (UX-F-005). All three are editorial corrections — no ratified decision was
  reopened or weakened. UX-F-003 is **resolved** — its *numbering* half was always a statement of
  fact, and its *principle-gap* half was settled by the operator's own ratified §18 item 7 and
  delivered by the owner's 2026-07-29 amendment, not invented by this packet.
- [x] The report's original justification for UX-F-002 — *"amending `spec.md` §15 is not permitted
  under G073 in this mode"* — was **factually wrong** and has been corrected. G073 forbids changes
  *outside* `spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json, docs/**,
  .github/**`; `spec.md` is on the permitted list. The real reason all of these stayed open was
  **artifact ownership** — `bubbles.plan` and `bubbles.ux` correctly declined to rewrite
  analyst-owned `spec.md` sections. **That routing was right; the gate citation was not**, and
  correcting it matters because a fabricated gate constraint left in the record becomes precedent.
- [x] F-109-OF2-AMEND (new, surfaced by `design.md` §4) was recorded as open and routed rather than
  applied to `spec.md` by a non-owner. **RESOLVED 2026-07-29** by `bubbles.analyst`: §9A.3's OF-2
  now carries an explicit `corpus` data-scope grant/revoke step, an effective-result operator
  surface step, data-scope revocation, and the four-term intersection requirement for the
  operator-side effective-list computation. Scopes 03 and 07 were **verified** to already plan to
  that behavior, so no planned work changed.
- [x] A fifth defect found in re-review — `scopes.md` TP-07-03 asserts *"every `spec.md` §15 target
  file exists"* and names `docs/Product-Principles.md`, but §15 declared no such row — was
  **RESOLVED 2026-07-29** by adding the row, carrying both the §16 alignment note and the
  delivered-Principle-11 verification (principles file **and** its BLOCKING companion). The row is
  a **verification** obligation only; no agent may amend the owner-ratified principles document.
- [x] F-109-002 is handled honestly: MCP owns its own wiring, funds live coverage, and discharges F-095-E2E-LIVE **for the MCP path only** — no artifact claims spec 095 is generally proven.
- [x] The six inherited spec findings F-109-001…006 are **untouched** by the 2026-07-29 editorial pass — they are delivery constraints, not wording defects, and remain as recorded.

### Evidence Honesty

- [x] `report.md` records **no** test output, because no test has been run and none could be.
- [x] `state.json` sits at `specs_hardened` — the mode ceiling — and never claims `done`.
- [x] `design.md` was read only; **no source file** was created or modified under the G073 lockout; no other spec folder was touched. `spec.md` was edited only under explicit operator instruction — to convert §18 into a ratified decision record, to reconcile §9A.7 / §16 / §18 to the shipped `Principle 11 — Local-First Data Ownership` title, and on 2026-07-29 to apply the five editorial corrections (UX-F-002, UX-F-004, UX-F-005, F-109-OF2-AMEND, and the §15 `docs/Product-Principles.md` row) — and every such edit is recorded as an ownership deviation in `state.json` `executionHistory` rather than hidden. Those `spec.md` edits are **inside** G073's permitted set: G073 constrains changes *outside* `spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json, docs/**, .github/**`.

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
- [x] **7. Principle gap** — RESOLVED by **addition**, and **DELIVERED 2026-07-29**. Under the owner sign-off this item required, `docs/Product-Principles.md` gained `Principle 11 — Local-First Data Ownership` together with a matching block in its BLOCKING companion enforcement file. Constitution C1 is **not** the sole carrier: C1 governs *how the system is built*, Principle 11 governs *what the product promises*. `spec.md` §16 cites P11 as the product-track carrier with C1 as the engineering-track cross-reference, and Scope 07 now carries the shipped surfaces as **verification** DoD items rather than as edits to make.

**Not ratified by this gate.** UX-F-002, UX-F-004, and UX-F-005 were bundled into "the §18 pass" for
convenience but are `spec.md` wording corrections, not product decisions. **The gate did not decide
them** — that statement was true when written and remains the historical record. They were routed to
`bubbles.analyst`, and **that owner resolved all three on 2026-07-29**, together with
F-109-OF2-AMEND and the §15 `docs/Product-Principles.md` gap found in re-review. Recording their
later resolution does **not** retroactively make them ratified: they were corrected by their owner,
not decided by this gate.
