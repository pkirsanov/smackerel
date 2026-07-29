# Spec 109 — MCP Knowledge Server

**Status:** draft (target ceiling `specs_hardened`)
**Workflow mode:** `product-to-planning` (planning only — no source code changes)
**Release train:** `next`
**Feature flag introduced:** `mcpKnowledgeServer`
**Owner surface:** integration capability mounted at `/mcp` on the existing `smackerel-core` listener

---

## 1. Problem Statement

Smackerel's knowledge graph is only reachable through Smackerel's own surfaces: the
PWA, the Chrome bridge extension, the Telegram delivery surface, and the internal
assistant. The operator's actual working day happens somewhere else — inside VS Code,
inside a coding agent, inside a general-purpose chat client. Every one of those
environments now speaks the Model Context Protocol (MCP), and none of them can see a
single artifact Smackerel has captured.

The consequence is that the operator re-explains context that Smackerel already holds.
The system observes, extracts, links, and lifecycles knowledge (Product Principles 1, 3,
5) and then strands it behind a wall the rest of the toolchain cannot cross. That is a
direct failure of Principle 5 — "One Graph, Many Views" — because there is currently
exactly one family of views, all of them Smackerel-owned.

No MCP server exists today. Spec 069 explicitly deferred it: *"A second backend ingress
mechanism (gRPC, MCP). Out of scope"* (`specs/069-.../spec.md:271`). That deferral was
correct at the time; this spec closes it.

The problem is not "add an API". Smackerel already has 517 route registrations across
`internal/api/`. The problem is that a foreign, tool-calling, possibly-remote model is a
fundamentally different consumer than a first-party client:

- It is **untrusted with raw text.** Smackerel passively ingests email from anyone, so
  the corpus contains attacker-controlled prose *by design*. Handing that prose to a
  foreign agent turns Smackerel into a prompt-injection delivery vector.
- It may run **off-machine.** The product's entire brand position is local-first
  (`docs/smackerel.md` §21.4; `docs/INVESTOR_OVERVIEW.md`). An MCP endpoint that
  silently ships the corpus to a hosted model destroys that position.
- It **enumerates capabilities.** A naive `tools/list` built from `agent.All()` would
  publish 19 non-capabilities: 7 `noop_*` bookkeeping tools plus 12 tools in
  `internal/recommendation/tools/register.go` whose handlers unconditionally return
  `{"ok":true}` against a bare `{"type":"object"}` schema. That is a fabricated
  capability surface — a direct Principle 8 (Trust Through Transparency) violation.

So the problem statement is: **expose the personal knowledge graph to external MCP
clients without leaking raw content, without breaking local-first, and without
advertising capabilities that do not exist.**

---

## 2. Outcome Contract

**Intent.** An operator can attach any conformant MCP client — starting with the coding
agent they already live in — to their own Smackerel instance, and get grounded,
provenance-carrying, *processed* answers from their personal corpus, without the raw
corpus ever leaving the machine unless they have explicitly and auditably decided
otherwise for that specific client.

**Success Signal.** From a cold MCP client with a fresh audience-bound credential:
`initialize` succeeds; `tools/list` returns a deterministically ordered toolset that
contains **only** tools the presented credential is actually authorized for and that are
**backed by real domain services**; `memory.search` returns projected artifacts carrying
`source_kind`, `retrieval_strategy`, `retrieval_fell_back`, and a `trace_token`; and a
byte-level inspection of every response shows **zero occurrences of `content_raw`**.

**Hard Constraints.**
- `content_raw` is never returned over `/mcp`, in any toolset, at any egress class (D2).
- The MCP endpoint accepts only credentials explicitly issued for it (D3). A legacy
  Smackerel bearer presented at `/mcp` is rejected, not honored.
- `remote-inference` egress is off unless an operator grant exists for that specific
  client, and every use is written to the MCP audit ledger (D1).
- Tools are backed by domain services. No MCP tool handler calls an internal HTTP
  handler, and no MCP tool is derived by passthrough from `agent.All()`.
- A non-OK `agent.Outcome` never renders as a successful tool result
  (BUG-061-008 / BUG-061-009 failure-honesty invariant).
- MCP is **pull-only**. No server-initiated notifications, no subscriptions (D4) —
  the ratified `docs/smackerel.md` §1.4 success metric is "system-initiated prompts
  < 3 per week", and a subscription firehose would obliterate it.

**Failure Condition.** This feature has failed — even with every test green — if any of
the following is true: raw corpus text reaches a foreign agent; the corpus reaches a
hosted model without an operator grant and an audit row; `tools/list` advertises a tool
that returns a canned `{"ok":true}`; an execution failure is rendered to the client as a
success; or the operator has to hand-edit a config file to attach a client.

---

## 3. Decisions Of Record

These are settled. They are recorded here so downstream design and scopes inherit them
rather than relitigating them.

### D1 — Egress posture: local-inference default, remote-inference opt-in per client

`local-inference` is the **default and only enabled egress class**. `remote-inference`
is fully coded but **default-OFF**, enabled only per-client by an explicit operator
grant, and **every** remote-class invocation is recorded in the MCP audit ledger.

Rationale: Constitution C1 (Local-First); `docs/smackerel.md` §21.4 UVP — *"your data
never leaves your machine"*; `docs/INVESTOR_OVERVIEW.md` — *"Defend Local-First as a
brand commitment… a moat, not a constraint"*.

The control variable is the **client's declared inference locality**, not the data's
sensitivity tier. A per-artifact "sensitivity ceiling" is **unimplementable today** (see
§14) and this spec does not pretend otherwise.

**Honest limitation, stated plainly:** Smackerel cannot verify a client's inference
locality. `inference_locality` is an **operator declaration** recorded in the MCP client
registry. The control is administrative and auditable, not cryptographic. Any documented
claim about it must say exactly that.

### D2 — Projection invariant: never return `content_raw`

`content_raw` is **never** returned over MCP — not in `memory-read`, not in `context`,
not under `local-inference`, not to a client the operator trusts. Four independent
justifications, any one of which is sufficient:

1. **Constitution C3 / Design Principle 2 — "Processed, not raw."** Returning raw text
   is a category error against the product's core stance.
2. **Injection containment.** Smackerel passively ingests email from arbitrary senders,
   so the corpus contains attacker-controlled text by design. `docs/smackerel.md` §17.2's
   "content is data, not instructions" defense lives inside *Smackerel's own* system
   prompts. A foreign tool-calling agent does not inherit that defense. Shipping raw
   corpus text to it makes Smackerel a prompt-injection **delivery vector**.
3. **Egress minimization.** Projections are smaller and enumerable; raw content is not.
4. **Provenance integrity.** A projection carries its retrieval strategy and source
   kind. A raw blob carries nothing, and the client cannot tell a verified fact from an
   attacker's sentence.

### D3 — MCP owns its credential and its authorizer

MCP uses its **own audience-bound credential** and its **own authorizer**, not the legacy
`bearerAuthMiddleware`.

Rationale: the published MCP specification states *"MCP servers MUST NOT accept any
tokens that were not explicitly issued for the MCP server."* Accepting an existing
Smackerel bearer at `/mcp` is literal token passthrough — a conformance violation.
`internal/auth/issue.go`'s `IssueToken` sets issuer, subject, jti, iat, nbf, exp, and
footer-kid, plus an optional `scope` — but sets **no audience**, and the `Audience` type
in `internal/auth/request_authenticator.go` is unwired to issuance. Closing that gap is
therefore an in-scope prerequisite, not a nice-to-have.

Consequence: spec 109 is **decoupled from spec 108**. Because MCP authorizes against its
own credential and its own grant model, it is secure-by-construction on day one and does
not wait on spec 108's migration of the legacy path.

### D4 — Scope deletions, and one trigger-conditioned exclusion

> **Amended 2026-07-29 by ratified §18 item 3.** The original D4 listed OAuth 2.1 + DCR
> alongside the permanent deletions. That was overclaiming, and it is corrected below.

**D4a — Permanently deleted (never re-opened).** Out of scope permanently for this product,
not "phase 2":

- MCP prompts
- MCP Apps
- Subscriptions, resource subscriptions, and `notifications/tools/list_changed`

Rationale: they serve a multi-tenant ecosystem that does not exist here. Smackerel is
single-user and self-hosted; `docs/smackerel.md` §1.5 lists multi-user as an explicit
non-goal. Subscriptions specifically would conflict with the ratified §1.4 success
metric "system-initiated prompts < 3 per week" and with Product Principle 6 (Invisible By
Default, Felt Not Heard). **MCP is pull-only.**

**D4b — Out of scope with a named re-open trigger (NOT deleted).**

- Full OAuth 2.1 authorization-server behavior
- Dynamic Client Registration (DCR)
- RFC 9728 protected-resource metadata

These are unnecessary for a tailnet-only, single-operator deployment in which every client is
first-party and the operator issues every credential by hand. They become **required** the
moment that stops being true.

> **Re-open trigger:** this exclusion re-opens **if and only if a client outside the
> operator's control must connect to `/mcp`.**

This is a **trigger condition, not a dated deferral**. Dated deferrals rot into stale TODOs
because a date carries no information about whether the work is needed; a trigger is
observable, and the answer stays correct until it fires.

### D5 — Toolset inventory and default posture

| Toolset | Default | Gate |
|---|---|---|
| `context` | **on** | audience-bound credential |
| `memory-read` | **on** | credential **+ corpus grant** |
| `person-context` | opt-in | explicit operator grant |
| `graph-read` | opt-in | explicit operator grant |
| `hospitality-read` | opt-in | explicit operator grant — **BLOCKED on BUG-019-003** |
| `memory-write` | off (later) | `confirm.Machine` + MRTR |
| `assistant` | off | — |
| `operations` | off | — |
| `external` | **off, permanently** | — |

**Tool Inventory — the names a client actually sees in `tools/list`.** A toolset is the
grantable unit; a **tool name is the wire contract**. Clients pin tool names in their MCP
configuration, so a name is a public, stable, externally-depended-on identifier in exactly
the way a toolset name is not. The table below is therefore normative: it is the complete
set of tool names `/mcp` may export, and no tool may be served under a name absent from it.

| Tool | Toolset | Required grants | `readOnly` | `destructive` | `idempotent` | `openWorld` | Description |
|---|---|---|---|---|---|---|---|
| `smackerel_active_topics` | `context` | audience-bound MCP credential | ✅ | ✖ | ✅ | ✖ | The currently-active topics from the lifecycle service (§8.2 `context.get_active_topics`). |
| `smackerel_daily_brief` | `context` | audience-bound MCP credential | ✅ | ✖ | ✅ | ✖ | The daily brief from the synthesis service (§8.2 `context.get_daily_brief`). |
| `smackerel_server_context` | `context` | audience-bound MCP credential | ✅ | ✖ | ✅ | ✖ | Server identity, negotiated protocol version, the toolsets **this credential** is granted, and per-tool readiness — the same facts `initialize` and `tools/list` already carry, exposed as a tool so a composing client (J3) can read them without re-negotiating. Backed by the MCP capability registry and authorizer, and a pure function of `(manifest, credential, readiness)`; it returns real per-credential state, never a constant, so it is not a fabricated capability under §2. (§8.2 `context.get_server_context`.) |
| `smackerel_get_artifact` | `memory-read` | credential **+ `memory-read` grant + `corpus` data scope** | ✅ | ✖ | ✖ | ✖ | One artifact's **processed Projection** (§9). Never `content_raw` (D2). `idempotent=false` across time because `lifecycle_state` moves (P3). (§8.2 `memory.get_artifact_projection`.) |
| `smackerel_recall` | `memory-read` | credential **+ `memory-read` grant + `corpus` data scope** | ✅ | ✖ | ✖ | ✖ | Vague natural-language recall (P2) compiled to an `intent.CompiledIntent` and served by the spec-095 `routing.Executor`. Returns compact Projections carrying all six provenance fields. (§8.2 `memory.search`.) |
| `smackerel_person_context` | `person-context` | credential **+ explicit operator grant** | ✅ | ✖ | ✖ | ✖ | Person / meeting context brief from the person-entity service. Projection fields only; no `content_raw`. (§8.2 `person.get_context`.) |
| `smackerel_graph_neighbors` | `graph-read` | credential **+ explicit operator grant** | ✅ | ✖ | ✖ | ✖ | Typed traversal to a node's neighbours in the knowledge graph, carrying `lifecycle_state`. (§8.2 `graph.get_neighbors`.) |
| `smackerel_graph_topic` | `graph-read` | credential **+ explicit operator grant** | ✅ | ✖ | ✖ | ✖ | One topic node with its lifecycle state and linked artifacts. (§8.2 `graph.get_topic`.) |
| `smackerel_guesthost_context` | `hospitality-read` | credential **+ explicit operator grant** — **NOT SERVED** | ✅ | ✖ | ✖ | ✖ | GuestHost guest / property / booking context. Registered with `Readiness.Available=false` under BUG-019-003 (F-109-001), so it is omitted from `tools/list` and returns method-not-found. (§8.2 `hospitality.get_guest_context`.) |

**T1 — Name rule.** A tool name is 1–128 characters, **case-sensitive**, unique within the
server, and drawn only from `A-Za-z0-9_-.`. Manifest registration refuses a duplicate name,
an out-of-charset name, and any name absent from this table. Because the charset admits `.`,
§8.2's logical identifiers (`memory.search`, `graph.get_topic`, …) are themselves legal MCP
names; the table's `smackerel_`-prefixed form is the one actually exported, and the §8.2
identifier in each Description cell is the mapping between the two. A name here is **never
renamed** — renaming breaks every client config that pinned it, so a rename is a removal
plus an addition, not an edit.

**T2 — Row order is the ordering contract.** This table's row order is the **normative
source** for the deterministic `tools/list` ordering §8.1 P-DETERMINISTIC already requires.
The rows are ordered by `(toolset ordinal from the D5 table, tool name ascending)`, which is
exactly the total order `design.md` §5 specifies — one fact stated once, not two competing
sources. `tools/list` **MAY** vary by the credential presented on that request (an ungranted
tool is omitted — R-109-UX11) and **MUST NOT** vary per-connection or with connection state
(SCN-109-003).

**T3 — Annotations are advisory, never a control.** The MCP specification states that clients
**MUST** treat tool annotations as **untrusted** hints. The four columns above are therefore a
courtesy to the client, and nothing more. What actually enforces is server-side authorization:
P-AUTHZ (audience + grant intersection), P-EGRESS (egress class), and P-PROJ (the closed
projection type). A tool marked `readOnly` is not read-only *because* of the annotation; it is
read-only because its handler has no write path and the foundation gives it none.

**T4 — Deferred and permanently-off toolsets export no names.** `memory-write` is off by
decision D5, so §8.2's `memory.propose_capture` has **no** wire name in this table and no
entry in `tools/list`; a name is assigned when that toolset is delivered, under the same T1
rule. `assistant`, `operations`, and `external` export nothing, permanently for `external`.

`external` is permanently off because enabling any third-party-API toolset would make
Smackerel an MCP **proxy server**, which triggers the entire confused-deputy mitigation
set: a per-client consent registry, an MCP-owned consent UI, exact `redirect_uri`
matching, and single-use `state`. That is a different product.

---

## Competitive Context

`docs/smackerel.md` §21.1 already lists MCP integration as a **Fabric.so strength**. In
the AI-memory category, an MCP server is no longer a differentiator — it is table
stakes, and its absence is a visible gap in the comparison table.

The row this spec actually **strengthens** for Smackerel is **"Local-first / own your
data"**. Shipping MCP without D1 and D2 would neutralize that row: an MCP endpoint that
hands raw corpus text to a hosted model is functionally indistinguishable from a cloud
memory product, and the §21.4 UVP claim would become false. So the competitive framing
is precise:

- **Parity move:** ship MCP at all (closes the Fabric gap).
- **Differentiating move:** ship MCP that is *provably* local-first by default and
  *never* emits raw content (D1 + D2). That combination is the thing competitors
  structurally cannot copy without abandoning their hosted model.

`docs/smackerel.md` §21 MUST be updated with an MCP row and an honest annotation of the
D1 limitation (operator declaration, not verified).

---

## 4. Actors & Personas

| Actor | Description | Key Goals | Boundary |
|---|---|---|---|
| **Operator** | The single human owner of this self-hosted instance. Lives in VS Code and a coding agent. | Reach personal memory from the tools they already use; keep data local; know exactly what left the machine. | Sole authority to register MCP clients, issue MCP credentials, and grant toolsets/egress classes. |
| **MCP Client (local-inference)** | A conformant MCP client whose model runs on the operator's machine (local coding agent, local chat client). | Enumerate authorized tools; retrieve grounded, provenance-carrying context. | Untrusted with raw content (D2). Sees only tools its credential authorizes. |
| **MCP Client (remote-inference)** | A conformant MCP client backed by a hosted model. | Reason over the corpus with a stronger model. | Disabled by default. Requires an explicit per-client operator grant; every invocation is audited (D1). |
| **Smackerel Domain Services** | `internal/retrieval/routing`, `internal/assistant/confirm`, graph and person services. | Serve the MCP capability layer directly, with no HTTP hop. | Never reached via internal HTTP handlers. |
| **MCP Authorizer** | The MCP-owned authorization component (D3). | Verify every inbound request; intersect credential scope × client grants × toolset egress class. | Does not reuse `bearerAuthMiddleware`. Never authenticates via session. |
| **QF Companion** | The QuantitativeFinance sibling product. | Unchanged. | Product Principle 10 boundary is untouched by this spec; MCP MUST NOT expose trade approval, mandate change, execution, or financial advice. |

---

## 5. Jobs-To-Be-Done

**J1 — "Bring my memory to where I already work."**
The operator spends the day in VS Code and a coding agent. They want the agent to answer
"what did we decide about X?" from *their* corpus, not from a general model's guess.
This is the primary job and the one `context` + `memory-read` exist to serve. It is a
**local-inference** job: the agent is on the machine.

**J2 — "Let a stronger model reason over my corpus."**
Sometimes the local model is not good enough and the operator deliberately wants a
frontier hosted model to synthesize across their knowledge. This is the **high-egress**
job and it is exactly why D1 exists as a per-client grant rather than a global switch:
the operator must be able to say *yes, for this one client*, and later be able to read
back exactly what that decision cost them.

**J3 — "Compose Smackerel with other tools."**
The operator wants Smackerel to be one server among several in a client's MCP
configuration — sitting next to a filesystem server, a git server, a browser server —
and to contribute the memory layer of a composed workflow. This job is why
deterministic tool ordering, honest annotations, and stable tool names matter more than
tool count.

**J4 — "Guest context in the STR host's ops agent."**
The short-term-rental host runs an operations agent. They want prior guest context
surfaced there rather than re-derived. This job is real but **BLOCKED**: it depends on a
stable external reference per artifact, and `artifacts.source_ref` is never persisted
(BUG-019-003). `hospitality-read` is specified here and delivered later.

---

## 6. Use Cases

### UC-109-001 — Attach an MCP client
- **Actor:** Operator
- **Preconditions:** `mcpKnowledgeServer` enabled on the running train; core listener up.
- **Main flow:** Operator registers a client (name, `inference_locality`, requested
  toolsets) → Smackerel issues an audience-bound MCP credential → operator pastes it into
  the client's MCP config → client `initialize`s successfully.
- **Alternative:** Operator requests `hospitality-read` → registration succeeds but the
  toolset is reported unavailable with the BUG-019-003 reason.
- **Postconditions:** Client registry row exists; credential is bound to the MCP audience.

### UC-109-002 — Enumerate authorized capabilities
- **Actor:** MCP Client
- **Main flow:** Client calls `tools/list` with its credential → authorizer resolves
  credential scope × client grants × egress class → server returns a deterministically
  ordered list containing only real, service-backed tools.
- **Postconditions:** No `noop_*` tool and no `internal/recommendation/tools/register.go`
  stub appears. List content is a function of the presented credential, never of
  connection state.

### UC-109-003 — Retrieve grounded memory
- **Actor:** MCP Client (local-inference)
- **Preconditions:** `memory-read` granted; corpus grant present.
- **Main flow:** Client calls `memory.search` → capability layer compiles the query into
  an `intent.CompiledIntent` and calls `routing.Executor.Retrieve` → results are mapped
  through the Projection Contract (§9) → returned with `source_kind`,
  `retrieval_strategy`, `retrieval_fell_back`, `retrieval_contract_known`, `trace_token`.
- **Postconditions:** Zero `content_raw` in the response. One audit row written.

### UC-109-004 — Degrade honestly
- **Actor:** MCP Client
- **Main flow:** The routing strategy falls back → `StrategySelection.FellBack` is true →
  the response carries `retrieval_fell_back: true` and the fallback `Reason`, and the
  result is still `isError: false` because retrieval *succeeded*, just not on the
  preferred path.
- **Postconditions:** Client can distinguish "best path" from "degraded path" without
  guessing.

### UC-109-005 — Refuse an unauthorized toolset with a targeted challenge
- **Actor:** MCP Client
- **Main flow:** Client calls a tool outside its grants → authorizer denies → server
  responds with a targeted `WWW-Authenticate` challenge naming **only** the missing
  scope.
- **Postconditions:** The full scope catalog is never disclosed (Scope Minimization).

### UC-109-006 — Propose a memory write (deferred toolset, specified now)
- **Actor:** MCP Client with `memory-write`
- **Main flow:** Client calls a write tool → capability layer calls
  `confirm.Machine.Propose` with `transport="mcp"` → server returns an
  `InputRequiredResult` / issues `elicitation/create` carrying the `requestState` → client
  returns the operator's decision → capability layer calls `confirm.Machine.Confirm` or
  `Discard`.
- **Alternative:** No decision before `ExpiresAt` → `SweepTimeouts` discards the proposal
  → a later `Confirm` for that `confirm_ref` fails honestly.
- **Postconditions:** Single-flight semantics hold on the `(user_id, transport,
  confirm_ref)` key. No write occurs without an explicit confirm.

### UC-109-007 — Grant remote-inference egress
- **Actor:** Operator
- **Main flow:** Operator explicitly grants `remote-inference` to one registered client →
  that client's subsequent invocations are permitted **and** each writes a
  remote-egress audit row.
- **Postconditions:** The operator can enumerate every remote-egress event from the
  ledger. Revocation takes effect on the next request (credentials are per-request input).

### UC-109-008 — Surface an execution failure as a failure
- **Actor:** MCP Client
- **Main flow:** A domain call returns a non-OK `agent.Outcome` → the capability layer
  emits a tool result with `isError: true` and an outcome-appropriate message, preserving
  the error cause.
- **Postconditions:** No success-shaped response for a failed turn. No "saved as an idea".

---

## 7. Business Scenarios (Gherkin)

```gherkin
Scenario: SCN-109-001 — Raw content never crosses the MCP boundary
  Given an MCP client with the "memory-read" toolset granted
  And the corpus contains an artifact whose content_raw is a phishing email body
  When the client calls memory.search and receives results
  Then no field in any response contains content_raw
  And each result carries only Projection Contract fields
  And the phishing body text does not appear anywhere in the response

Scenario: SCN-109-002 — A legacy Smackerel bearer is rejected at /mcp
  Given a valid Smackerel bearer token issued for the first-party API
  When that token is presented to the /mcp endpoint
  Then the request is rejected as not issued for the MCP audience
  And the response carries a WWW-Authenticate challenge
  And no tool is executed

Scenario: SCN-109-003 — tools/list reflects the presented credential
  Given client A holds a credential granting only "context"
  And client B holds a credential granting "context" and "memory-read"
  When each calls tools/list against the same running server
  Then client A sees only context tools
  And client B additionally sees memory-read tools
  And each list is byte-identical across repeated calls with the same credential

Scenario: SCN-109-004 — No fabricated capability is advertised
  Given the general agent registry contains 7 noop_* tools
  And internal/recommendation/tools/register.go registers 12 tools returning {"ok":true}
  When any MCP client calls tools/list
  Then none of those 19 registrations appear in the result
  And every advertised tool is backed by a domain service

Scenario: SCN-109-005 — Remote-inference egress is denied without a grant
  Given a registered MCP client whose declared inference_locality is "remote"
  And the operator has not granted remote-inference to that client
  When the client calls any memory-read tool
  Then the call is denied
  And a denial is recorded with reason "remote-inference not granted"
  And no corpus projection is returned

Scenario: SCN-109-006 — Granted remote-inference egress is always audited
  Given the operator has explicitly granted remote-inference to client R
  When client R successfully calls memory.search
  Then the projection is returned
  And exactly one remote-egress audit row is written naming client R, the tool, and the timestamp

Scenario: SCN-109-007 — Retrieval fallback is reported, not hidden
  Given the routing executor selects a fallback strategy for a query
  When the MCP client receives the result
  Then retrieval_fell_back is true
  And the fallback reason from StrategySelection is present
  And the result is not marked isError

Scenario: SCN-109-008 — Provider failure is surfaced as a failure
  Given a domain call returns a non-OK agent.Outcome
  When the MCP client receives the tool result
  Then the result has isError set to true
  And the outcome-appropriate cause is preserved in the message
  And the result is not shaped like a successful retrieval

Scenario: SCN-109-009 — Protocol errors and tool errors are distinguished
  Given a client calls a tool that does not exist
  Then the server returns a JSON-RPC error
  Given a client calls an existing tool whose execution fails
  Then the server returns a result with isError true, not a JSON-RPC error

Scenario: SCN-109-010 — Scope challenges disclose only what is missing
  Given a client authorized for "context" calls a graph-read tool
  When the authorizer denies the call
  Then the WWW-Authenticate challenge names only the missing graph-read scope
  And no other scope name appears in the response

Scenario: SCN-109-011 — Sessions are never used for authentication
  Given a client completes initialize successfully
  When a subsequent request omits its credential
  Then the request is rejected
  And no prior session state is used to authorize it

Scenario: SCN-109-012 — hospitality-read is honestly unavailable
  Given BUG-019-003 is unresolved and artifacts.source_ref is never persisted
  When the operator grants hospitality-read to a client
  Then the toolset is reported unavailable with the BUG-019-003 reason
  And no guest-context tool appears in tools/list
  And no fabricated external reference is emitted

Scenario: SCN-109-013 — A write proposal requires an explicit confirmation
  Given an MCP client holds the memory-write toolset
  When it calls a write tool
  Then confirm.Machine.Propose is invoked with transport "mcp"
  And the client receives an input-required round trip carrying the requestState
  And no write is committed until confirm.Machine.Confirm succeeds

Scenario: SCN-109-014 — An expired proposal cannot be confirmed
  Given a write proposal whose ExpiresAt has passed and which SweepTimeouts has discarded
  When the client returns a confirmation for that confirm_ref
  Then the confirmation fails honestly
  And no write is committed
  And the failure is surfaced with isError true

Scenario: SCN-109-015 — MCP never pushes
  Given an MCP client is connected
  When the corpus changes, a digest is generated, or a topic is promoted
  Then the server sends no notification, no subscription event, and no tools/list_changed
  And the client observes changes only by calling a tool
```

---

## 8. Capability & Toolset Model

This section states the domain capability model MCP is built on: the primitives, their
lifecycles, the policies every capability must obey, and the toolset inventory that
instantiates them. `design.md` maps this model onto a `## Capability Foundation`, a set of
`## Concrete Implementations`, and the `### Variation Axes` that separate them.

## Domain Capability Model

*(Referenced elsewhere in this document and in `design.md` as §8.1.)*

**Primitives**

| Primitive | Meaning |
|---|---|
| `MCPClient` | A registered external consumer: id, display name, declared `inference_locality`, granted toolsets, credential binding. |
| `Toolset` | A named, independently grantable bundle of tools sharing one egress class and one authorization gate. |
| `Tool` | A single invocable capability, backed by exactly one domain service call. |
| `Projection` | The processed, egress-safe representation of an artifact (§9). Never raw. |
| `Grant` | An operator-issued authorization binding an `MCPClient` to a `Toolset` and, separately, to an egress class. |
| `EgressClass` | `local-inference` (default) or `remote-inference` (operator-granted, audited). |
| `AuditRow` | An append-only record of one invocation: client, tool, outcome, egress class, timestamp. |

**Lifecycle**

`MCPClient`: `registered → credentialed → granted → active → revoked`.
`Grant`: `absent → granted → revoked` (revocation is effective on the next request, since
credentials are per-request input, not connection state).
A write proposal follows `confirm.Machine`'s existing lifecycle: `proposed → confirmed |
discarded | expired`.

**Business policies every tool must obey**

1. **P-PROJ** — Emit Projection fields only. `content_raw` is never emitted (D2).
2. **P-SVC** — Resolve through a domain service. Never call an internal HTTP handler,
   never derive the toolset from `agent.All()`.
3. **P-AUTHZ** — Authorize per request against the MCP audience-bound credential (D3).
   Never authorize from session state.
4. **P-EGRESS** — Refuse if the client's effective egress class is not granted (D1),
   and audit every permitted `remote-inference` invocation.
5. **P-HONEST** — Map a non-OK `agent.Outcome` to `isError: true`. Never render failure
   as success.
6. **P-DETERMINISTIC** — `tools/list` is deterministically ordered for a given credential.
7. **P-PULL** — No server-initiated messages (D4).

### 8.2 Toolset Inventory

Tools are listed here by their **logical identifier**. The **wire name** each one is
exported under — the string a client pins in its config and sees in `tools/list` — is the
normative Tool Inventory in **D5**, and the two tables are two-way consistent: every logical
identifier below has exactly one wire name there, and no wire name exists without a logical
identifier here.

| Toolset | Tools | Backing domain service | Default | Egress |
|---|---|---|---|---|
| `context` | `context.get_daily_brief`, `context.get_active_topics`, `context.get_server_context` | synthesis / lifecycle services; capability registry + authorizer for `get_server_context` | on | local |
| `memory-read` | `memory.search`, `memory.get_artifact_projection` | `internal/retrieval/routing.Executor` | on (corpus grant required) | local |
| `person-context` | `person.get_context` | person/entity service | opt-in | local |
| `graph-read` | `graph.get_neighbors`, `graph.get_topic` | knowledge-graph service | opt-in | local |
| `hospitality-read` | `hospitality.get_guest_context` | hospitality projection | opt-in, **blocked** | local |
| `memory-write` | `memory.propose_capture` | `internal/assistant/confirm.Machine` | off (later) | local |
| `assistant` | — | — | off | — |
| `operations` | — | — | off | — |
| `external` | — | — | **permanently off** | — |

**Annotations.** Each tool declares `readOnlyHint`, `destructiveHint`, `idempotentHint`,
and `openWorldHint` **explicitly in the MCP capability layer**; the normative per-tool values
are the four annotation columns of the D5 Tool Inventory. These MUST NOT be derived
from `internal/agent/registry.go`'s `SideEffectClass`, whose only values are
`read | write | external` — a three-value enum cannot express four orthogonal MCP hints.
Per the MCP specification, clients treat annotations as untrusted hints; server-side
enforcement (P-AUTHZ, P-EGRESS, P-PROJ) is therefore mandatory regardless of what any
annotation says (D5 T3).

---

## 9. Projection Contract

Every artifact returned over MCP is a **Projection**. The field set is closed.

**Returned fields**

| Field | Source |
|---|---|
| `artifact_id` | `artifacts` row id |
| `kind` | artifact kind |
| `title` | processed title |
| `summary` | processed summary (extraction output, never the raw body) |
| `entities[]` | extracted entities |
| `topics[]` | linked topics |
| `lifecycle_state` | topic/artifact lifecycle state |
| `created_at`, `observed_at` | timestamps |
| `source_kind` | `routing.RetrievalResult.RetrievedSource{Kind}` |
| `retrieval_strategy` | `routing.StrategySelection.Strategy` |
| `retrieval_reason` | `routing.StrategySelection.Reason` |
| `retrieval_fell_back` | `routing.StrategySelection.FellBack` |
| `retrieval_contract_known` | `routing.StrategySelection.ContractKnown` |
| `trace_token` | `routing.StrategySelection.TraceToken()` |
| `deep_link` | Smackerel-local deep link |

**Explicitly excluded — never emitted**

- `content_raw` (D2, absolute)
- attachment bytes, photo binaries, and any blob payload
- embedding vectors
- raw email headers/bodies and any verbatim third-party prose
- `external_ref` / stable external source reference — **blocked on BUG-019-003**

**External reference honesty rule.** Because `artifacts.source_ref` is never persisted by
the connector front door (BUG-019-003), the Projection MUST omit `external_ref` entirely
rather than synthesize one. Emitting a reconstructed or guessed external reference is a
fabrication and is forbidden. `deep_link` (Smackerel-local) is the only navigational
field until BUG-019-003 is fixed.

**Resource links do not widen this field set.** A `resource_link` (§9B) is a **transport
content entry**, not a Projection field: it sits beside the `[]Projection` payload in the
tool result, carries only a `smackerel://` URI built from opaque identifiers, and adds
nothing to the closed table above. The closed field set therefore stands exactly as
written, and `external_ref` remains absent from both the Projection **and** every URI.

**Source-level egress policy.** `internal/api/search.go`'s `SearchFilters` has no
`source_id` filter and `SearchResult` does not carry `source_id`. Source-level egress
policy therefore **cannot** be implemented as a post-filter over the existing handler
shape; it MUST be enforced in SQL inside the domain service that backs `memory.search`.
Design must treat this as a required capability of the service, not of the MCP layer.

---

## 9A. Non-UI UX Contract

Spec 109 ships **no screens**. Its two consumers are an operator working through the
`docs/Operations.md` runbook (§15) and a foreign MCP client that experiences Smackerel
only as a tool list, a tool result, and an HTTP challenge. The UX surface is therefore
**workflow behavior, status language, refusal shape, and exception handling** — nothing
in this section describes a wireframe, a screen, or an end-user layout.

This section is additive. It constrains how §10 (authorization), §11 (audit), and §12
(failure semantics) are *rendered*. It introduces no capability, no toolset, and no
change to D1–D5.

---

### 9A.1 Closed Status & Refusal Vocabulary (NON-NEGOTIABLE)

Exactly **seven** tokens may ever describe the disposition of an MCP request. They are
lowercase and hyphen-separated. They are the closed value set for the §11 audit ledger's
`outcome` and `denial_reason` columns and for the `outcome` / `reason` label values on
the §11 metrics.

| Token | Meaning |
|---|---|
| `authorized` | Every gate passed (P-AUTHZ + P-EGRESS) and the tool returned an OK `agent.Outcome`. The only success token. |
| `unauthorized-toolset` | The presented credential's scope intersected with the client's toolset grants does not include the requested toolset (D3, D5). |
| `unauthorized-egress-class` | The client's effective egress class is not granted — in practice `remote-inference` with no per-client operator grant (D1, P-EGRESS, SCN-109-005). |
| `capability-unavailable` | The toolset exists in the §8.2 inventory but is blocked by a recorded finding and is therefore not served (`hospitality-read` under BUG-019-003, F-109-001, SCN-109-012). |
| `degraded-fallback` | Retrieval succeeded on a non-preferred strategy; `StrategySelection.FellBack` is true. **Not an error** (UC-109-004, SCN-109-007). |
| `refused` | The server structurally will not proceed for a non-authorization reason: a permanently-off surface (D4 / `external`), or an expired or discarded write proposal (SCN-109-014). |
| `execution-failed` | A domain call returned a non-OK `agent.Outcome`; rendered per §12 (P-HONEST, SCN-109-008). |

**R-109-UX1 — No other status word may ever appear.** Banned in every operator-facing
readout, every audit row, every metric label value, and every client-visible message:
`ok`, `success`, `succeeded`, `passed`, `done`, `complete`, `error`, `failed`, `failure`,
`denied`, `forbidden`, `rejected`, `blocked`, `warning`, `warn`, `partial`, `soft-fail`,
`skipped`, `retrying`, `unknown`, and any uppercase, camelCase, or space-separated
variant of any of the seven tokens. A new disposition is added by amending this table,
never by coining a synonym at a call site.

**Explicit carve-outs (these are not status words and are unaffected).**

- The `agent.Outcome` identifiers in §12 (`OutcomeOK`, `OutcomeProviderError`, …) are Go
  enum values and the **input** to the mapping, not the emitted token.
- `isError` is an MCP protocol boolean, not a status word.
- `local-inference` / `remote-inference` are egress-class names (D1) recorded in the
  separate `egress_class` column, not in `outcome`.

---

### 9A.2 Refusal Envelope (fixed fields, fixed order)

Every refusal an operator or a client can read renders **exactly four fields, in exactly
this order**:

```
observed:    <one line — what the server actually saw>
required:    <one line — what the server would have accepted>
remediation: <one line — a concrete operator action>
reference:   <one line — a governance-doc pointer>
```

- **R-109-UX2 — Fixed shape.** Exactly four fields. No additional field, no reordering,
  no omission, no nesting. Verbosity settings never alter the shape.
- **R-109-UX3 — One line per field, machine-extractable.** Each field is a single line
  keyed by its `<field>:` prefix, so a script can extract it without parsing prose.
- **R-109-UX4 — Remediation is an action, not a diagnosis.** It names a concrete thing
  the operator does next (`grant graph-read to client <client_id> — see docs/Operations.md`).
  "Check your configuration", "contact support", or a restatement of the problem is a
  violation. Alternatives are a bounded list, never paragraph prose.
- **R-109-UX5 — Reference points to governance, never to this spec.** Permitted targets:
  `docs/Operations.md`, `docs/API.md`, `docs/Architecture.md`, `docs/smackerel.md`. A
  reference to `specs/109-mcp-knowledge-server/` — or to any `specs/` path — is a
  violation. Operators read governance docs, not engineering specs. §15 already funds
  every permitted target.
- **R-109-UX6 — Banned envelope content.** Stack traces; `file:line` markers; Go type,
  package, or identifier names; timestamps (the ledger owns `ts`, §11); emoji; ANSI
  color; bold/italic markup; and any prose outside the four lines.

**R-109-UX7 — The envelope never widens disclosure.** The envelope composes with, and
never replaces, the transport-level `WWW-Authenticate` challenge of §10 and §12, and it
is bound by §10 Scope Minimization: a client-facing `required` line names **only** the
single missing scope, never the catalog (SCN-109-010).

Where the envelope is emitted:

| Situation | Client sees | Operator sees |
|---|---|---|
| Tool correctly omitted from `tools/list` (`unauthorized-toolset`, `capability-unavailable`) | JSON-RPC method-not-found only — **no envelope** (§12). An envelope here would be an existence oracle. | Full envelope on the operator surface. |
| Tool legitimately visible to this credential, denied on a later gate (`unauthorized-egress-class`, `refused`) | Envelope, scope-minimized. | Full envelope. |
| `execution-failed` | Tool result with `isError: true` carrying the envelope in place of a success body. | Full envelope + audit row. |

Worked example — client-facing, `unauthorized-egress-class`:

```
observed:    client cli-7 invoked memory.search at egress class remote-inference
required:    an operator grant of remote-inference for client cli-7
remediation: grant remote-inference to cli-7, or point the client at a local model
reference:   docs/Operations.md — "Grant or revoke remote-inference"
```

Worked example — operator-facing, `capability-unavailable`:

```
observed:    toolset hospitality-read is granted to client cli-3 but is not served
required:    a persisted stable external reference per artifact
remediation: track BUG-019-003; no operator action can enable this toolset today
reference:   docs/API.md — "MCP toolset availability"
```

---

### 9A.3 Operator Flows

Each step states the operator's **observable outcome**. Every flow is executed through
the `docs/Operations.md` runbook required by §15; none requires hand-editing a config
file (that is an explicit §2 Failure Condition).

**OF-1 — Register an MCP client and issue an audience-bound credential** (UC-109-001, D3, F-109-004)

1. Operator registers the client with a display name, a declared `inference_locality`,
   and the requested toolsets. → **Observable:** a client registry row exists carrying an
   `MCPClient` id in state `registered`.
2. Smackerel issues a credential whose `aud` names the MCP server. → **Observable:** the
   credential is displayed exactly once; the client moves to `credentialed`.
3. Operator pastes the credential into the client's MCP config and the client calls
   `initialize`. → **Observable:** `initialize` succeeds; a first audit row appears with
   `outcome = authorized`.
4. Operator presents a legacy first-party Smackerel bearer instead. → **Observable:**
   rejected as not issued for the MCP audience, with a `WWW-Authenticate` challenge and
   no tool executed (SCN-109-002).

**OF-2 — Grant / revoke a toolset** (D5, §8.1 `Grant` lifecycle, UC-109-002)

1. Operator grants toolset `T` to client `C`. → **Observable:** the grant moves
   `absent → granted`; `C`'s next `tools/list` includes `T`'s tools.
2. Operator grants a toolset blocked by a finding (`hospitality-read`). → **Observable:**
   the grant is recorded, and the operator surface reports `capability-unavailable` with
   the BUG-019-003 reason; `tools/list` still omits the tools (SCN-109-012).
3. Operator revokes `T` from `C`. → **Observable:** the grant moves `granted → revoked`;
   because credentials are per-request input and never connection state, `C`'s **next**
   request no longer sees `T`'s tools — no reconnect required.
4. Operator lists `C`'s effective toolsets. → **Observable:** the list is identical to
   what `C`'s own `tools/list` returns for that credential (UC-109-002 postcondition).

**OF-3 — Grant / revoke `remote-inference` egress class** (UC-109-007, D1 — HIGHEST CONSEQUENCE)

This is the only flow in the spec that moves corpus projections off the operator's
machine. It is treated accordingly.

1. Operator initiates a `remote-inference` grant for one named client. → **Observable:**
   the runbook presents the consequence in plain language: projections for this client
   will reach a hosted model, and Smackerel **cannot verify** the client's declared
   locality (D1 is administrative and auditable, not cryptographic).
2. **R-109-UX8 — Explicit confirmation is mandatory.** The operator must affirmatively
   confirm the specific `client_id`; the grant is never a default, never inherited from
   another client, and never a train flag (§15: `remote-inference` is deliberately not a
   feature flag, so no build can enable it globally). → **Observable:** no grant exists
   until the confirmation is given; an abandoned confirmation leaves state unchanged.
3. **R-109-UX9 — The grant decision itself is written to the audit ledger**, not only its
   later uses. → **Observable:** a ledger row records the grant with the client id,
   `egress_class = remote-inference`, and `outcome = authorized`.
4. Client invokes a tool under the grant. → **Observable:** the projection returns **and**
   exactly one remote-egress audit row is written naming the client, the tool, and the
   timestamp (SCN-109-006); `smackerel_mcp_remote_egress_total` increments.
5. Client invokes with no grant. → **Observable:** `unauthorized-egress-class`, the §9A.2
   envelope, no projection returned (SCN-109-005).
6. Operator revokes the grant. → **Observable:** a revocation row is written; the next
   request from that client is denied. Revocation is forward-only — it does not and
   cannot recall projections already sent.

**OF-4 — Read the MCP audit ledger** (§11) — answering *"did my corpus ever go to a third-party model?"*

1. Operator opens the append-only ledger. → **Observable:** rows carrying `ts`,
   `client_id`, `toolset`, `tool`, `outcome`, `egress_class`, `denial_reason`,
   `trace_token`.
2. Operator filters `egress_class = remote-inference`. → **Observable:** the complete,
   enumerable set of every invocation that left the machine. An empty result is a
   **positive answer**: nothing ever left.
3. Operator cross-checks against `smackerel_mcp_remote_egress_total`. → **Observable:**
   the counter and the ledger row count agree; disagreement is an incident.
4. **R-109-UX10 — The ledger answers the question without interpretation.** The answer is
   a row set, not a computed verdict, not a summary, and not a reassurance string. The
   operator reads the evidence.
5. Operator traces one suspect answer. → **Observable:** the row's `trace_token` matches
   the `trace_token` the client received in that projection (§9, §11), so a
   client-reported bad answer resolves to an exact strategy selection.

**OF-5 — Revoke a compromised client credential** (§8.1 lifecycle `→ revoked`)

1. Operator revokes the credential for client `C`. → **Observable:** `C` moves to
   `revoked`; a ledger row records the revocation.
2. `C` issues its next request. → **Observable:** rejected. Because MCP never
   authenticates from session state (SCN-109-011), an already-`initialize`d connection
   confers nothing — revocation is effective on the next request, with no reconnect and
   no server push needed (D4, P-PULL).
3. Operator inspects what the compromised credential reached. → **Observable:** OF-4's
   ledger filtered by `client_id = C` — including any `egress_class = remote-inference`
   rows.
4. Operator re-issues. → **Observable:** OF-1 from step 1 with a **new** `MCPClient` id;
   grants are not silently carried over from the revoked identity.

---

### 9A.4 Agent-Facing Discovery UX

How a connecting MCP client *experiences* the boundary. All three cases are governed by
P-AUTHZ, §10, and §12; this section pins the observable behavior.

**Case A — Ungranted toolset.**

- **R-109-UX11 — Omit, never show-then-deny.** An ungranted toolset's tools are **absent
  from `tools/list`** (UC-109-002, SCN-109-003). Advertising a tool the credential cannot
  invoke is a fabricated capability surface and violates P8 the same way a `noop_*` tool
  would (SCN-109-004).
- **R-109-UX12 — Absence must not leak existence.** Invoking an omitted tool by name
  returns JSON-RPC **method-not-found** — byte-identical to invoking a name that has
  never existed in any toolset. No envelope, no `required` scope, no "this tool exists but
  you lack X". This is the MCP analogue of spec 108's no-existence-oracle rule
  (R-108-D2): the response for *withheld* and for *nonexistent* is indistinguishable.
- The list is a pure function of the presented credential, never of connection state, and
  is byte-identical across repeated calls with the same credential (SCN-109-003).

**Case B — Granted but unavailable capability.**

- The operator has granted it; a recorded finding blocks it (`hospitality-read` /
  BUG-019-003). The **client** sees Case A exactly — omitted, method-not-found on
  invocation — because a client is never told about a capability it cannot use.
- **R-109-UX13 — The honesty is owed to the operator, not to the client.** The operator
  surface reports `capability-unavailable` with the blocking reason (UC-109-001
  alternative flow), so the operator is never left guessing why a grant they made has no
  effect. The client is never present-but-fake (§12) and no fabricated `external_ref` is
  emitted (§9).

**Case C — Scope step-up challenge.**

- **R-109-UX14 — Targeted challenge only.** A denial that is legitimately visible to the
  client returns `WWW-Authenticate` naming **only** the missing scope —
  `scope="graph-read"`, never `scope="context memory-read person-context graph-read …"`.
  The full scope catalog is never published in any response (§10 Scope Minimization,
  SCN-109-005, SCN-109-010).
- The challenge is the client's step-up affordance: it tells the client exactly one thing
  to ask the operator for. Any additional scope name in the response is a violation.
- Case C never applies to Case A or Case B, where the correct response is
  method-not-found with no challenge at all.

---

### 9A.5 Honest-Degradation Surface

`internal/retrieval/routing`'s `StrategySelection` (spec 095; F-109-002 makes MCP its
first live consumer) is what lets a calling agent tell a best-path answer from a
degraded one. Rendering rules:

- **R-109-UX15 — The three degradation fields are mandatory and non-omittable** on every
  `memory-read` result, present whether or not a fallback occurred:
  `retrieval_strategy` ← `StrategySelection.Strategy`, `retrieval_reason` ←
  `StrategySelection.Reason`, `retrieval_fell_back` ← `StrategySelection.FellBack` (§9).
  Field *absence* must never be the way a client infers "no fallback" — that is an
  ambiguity a silent downgrade can hide inside.
- **R-109-UX16 — A fallback is visibly a fallback.** When `retrieval_fell_back` is true,
  `retrieval_reason` carries the selection's own reason, in the same payload as the
  answer, at the same level as the results. It is not an optional annotation, not a
  trailing note, and not something the client has to request.
- **R-109-UX17 — A fallback is not an error.** `isError` stays false and the audit
  `outcome` is `degraded-fallback`, because retrieval *succeeded* — just not on the
  preferred path (UC-109-004, SCN-109-007). Rendering a successful fallback as
  `execution-failed` is as dishonest as hiding it.
- **R-109-UX18 — `retrieval_contract_known: false` is never rendered as confidence.**
  Alongside `source_kind` and `trace_token` (§9), it is the client's signal that the
  retrieval contract was not known for that path.

**Principle tie.** This is the retrieval-path expression of **Product Principle 8 — Trust
Through Transparency**, already claimed in §16 ("every projection carries retrieval
provenance and a `trace_token`"). It composes with the confidence-signal surface at
`docs/smackerel.md` §17.1 — see UX-F-002 for that reference's status.

---

### 9A.6 Failure-Honesty Mapping

The ratified BUG-061-008 / BUG-061-009 invariant applies verbatim (§12). Restated as a
UX rule:

- **R-109-UX19 — A non-OK `agent.Outcome` maps to `isError: true`.** It is rendered as a
  tool result carrying the §9A.2 envelope and the preserved cause. The audit `outcome` is
  `execution-failed`, and `smackerel_mcp_execution_error_surfaced_total` increments —
  that metric is the proof the failure surfaced (§11).
- **R-109-UX20 — A failed turn MUST NEVER render as a success content block.** Forbidden:
  a success-shaped result for a non-OK outcome; an empty result set presented as an
  answer when retrieval errored; a "no results found" phrasing over a provider error or a
  timeout. If the server does not know, it says so with `execution-failed`.
- **R-109-UX21 — "Saved as an idea" is forbidden on every MCP path.** That
  acknowledgement — and the whole capture-acknowledgement class of phrasing — is
  **band-LOW assistant-only** per BUG-061-008 / BUG-061-009. MCP has no band-LOW capture
  path: `memory-write` is off (D5) and, when it later lands, it goes through
  `confirm.Machine.Propose` with an explicit confirmation round trip (UC-109-006,
  SCN-109-013), never through a silent capture. The string must not appear on any MCP
  surface, in any toolset, at any egress class.
- **R-109-UX22 — Protocol errors and tool errors stay distinct** (SCN-109-009). An
  unknown tool or a malformed request is a JSON-RPC error; a real tool whose execution
  failed is a result with `isError: true`. Collapsing the two hides which one happened.
- **R-109-UX23 — Refusal is never silent.** Every disposition other than `authorized`
  writes an audit row carrying its token in `outcome` or `denial_reason`. A request that
  produced no row and no response is an incident, not a quiet success.

---

### 9A.7 UX Findings Routed Onward (not resolved here)

- **UX-F-001 — `memory-read`'s "corpus grant" is undefined in the capability model.**
  D5 gates `memory-read` on "credential **+ corpus grant**" and §8.2 repeats "corpus grant
  required" (UC-109-003 preconditions likewise), but the §8.1 `Grant` primitive binds an
  `MCPClient` only to a `Toolset` and to an egress class — there is no corpus grant in the
  model. Meanwhile D3 and §14 state spec 109 is decoupled from spec 108, which is the
  spec that owns `corpus:read`. It is therefore unresolved whether the corpus grant is an
  MCP-owned grant or spec 108's scope. This changes OF-2's observable outcomes and the
  `unauthorized-toolset` vs a distinct corpus-denial rendering. Routed to `bubbles.plan` /
  `bubbles.design`; **not** invented here.
- **UX-F-002 — `docs/smackerel.md` §17.1 is not in the §15 documentation table.** §15
  funds updates to §17.2, §21, and §22 only. If §17.1's confidence signals are the
  intended tie for R-109-UX15–UX18, §15 needs the row. **Still OPEN** — the §18 gate
  closed on 2026-07-29 without deciding it; it is a `spec.md` amendment, not a product
  decision. Routed to `bubbles.analyst` (Scope 07 already plans the §17.1 update).
- **UX-F-003 — Principle numbering.** The trust-through-transparency principle is
  **Principle 8** in `docs/Product-Principles.md`, which is what §16 cites and what
  §9A.5 uses. A commissioning reference to "Principle 11" does not resolve — that
  document has ten principles. **Partially resolved 2026-07-29:** the *principle-gap* half
  is settled by ratified §18 item 7, which adds **Principle 11 — Your Data Stays Yours**
  at delivery under owner sign-off. The *numbering* half was always a statement of fact.
  This spec still invents no principle and still cites only the ten ratified ones.
- **UX-F-004 — §2's Success Signal enumerates a subset of §9's provenance fields.** It
  names `source_kind`, `retrieval_strategy`, `retrieval_fell_back`, and `trace_token` but
  omits `retrieval_reason` and `retrieval_contract_known`, both of which §9 returns and
  R-109-UX15 / R-109-UX18 depend on. Editorial. **Still OPEN** — the §18 gate closed on
  2026-07-29 without deciding it, because it is a wording correction to `spec.md` rather
  than a product decision. Re-routed to `bubbles.analyst`.
- **UX-F-005 — D1 describes `remote-inference` as "fully coded but default-OFF"** while
  §1 states no MCP server exists today. Read literally these conflict. The intended
  meaning is presumably that the class is fully *specified* and ships default-OFF.
  **Still OPEN** — ratified §18 item 1 accepted D1's *posture*; it did not correct D1's
  *prose*. Re-routed to `bubbles.analyst`.

---

## 9B. MCP Resources — Reference In, Enumeration Out

MCP's Resources primitive was previously neither specified nor excluded here: D4a deletes
resource **subscriptions**, which says nothing about Resources as a *read primitive*. That
silence was the defect. This section records the ratified split. It is additive: it changes
no decision of record and weakens none of D1, D2, D3, D4a, D4b, or D5.

### 9B.1 In scope

**R-109-RES1 — `resource_link` entries in tool results.** A tool result MAY carry
`resource_link` content entries alongside its structured data. This is the
protocol-correct way to return a **reference** to a thing instead of inlining the thing,
and it therefore reinforces **D2** directly rather than tugging against it: a link is
categorically not `content_raw`, and it is the mechanism that lets a result stay compact
without the layer being tempted to inline a body. `smackerel_recall` returns compact
structured Projections **plus** `resource_link` entries pointing at the artifacts,
topics, people, and places behind them. Following a `resource_link` re-enters the same
authorized, projected path — it is a pointer, never a back door around P-AUTHZ, P-EGRESS,
or P-PROJ, and it never dereferences to raw content.

**R-109-RES2 — Resource URI templates.** Exactly five templates are defined. A
`resource_link` URI MUST match one of them; a URI outside this set is a defect.

| Template | Refers to |
|---|---|
| `smackerel://artifact/{id}` | one artifact's processed Projection (§9) |
| `smackerel://topic/{id}` | one topic node and its lifecycle state |
| `smackerel://person/{id}` | one person node in the entity graph |
| `smackerel://place/{id}` | one place node in the entity graph |
| `smackerel://guesthost/{kind}/{opaque-id}` | one GuestHost guest / property / booking record; `{kind}` is a closed vocabulary and `{opaque-id}` is an internal surrogate, never an external identifier |

**R-109-RES3 — BINDING: no PII in a resource URI.** No guest email address, no person
name, no meeting title, no street address, and no raw external identifier may appear in
any `smackerel://` URI. Only opaque internal identifiers are permitted, and the GuestHost
form is opaque by construction.

The reason is that **a URI is an egress surface in its own right.** It is logged by
intermediaries, echoed in client transcripts, rendered verbatim in client UIs, and
retained in places the response body is not. A URI that carries a guest's email address
has leaked that address to every hop on the path even when the body it points at is
perfectly projected — which would make D2 true of the payload and false of the envelope.
This constraint composes with §9's external-reference honesty rule: `artifacts.source_ref`
is not persisted (BUG-019-003), so there is no external identifier to embed even if one
were permitted, and synthesizing one would be the same fabrication §9 already forbids.

### 9B.2 Explicitly excluded — see §13

**No enumerable `resources/list` over the corpus, and no whole-graph resource.** An
enumerable resource list is a **corpus-enumeration oracle**: it discloses the existence
and the volume of what Smackerel holds *even when every individual read is denied*. That
directly contradicts this spec's own denial doctrine — the bare-403 / method-not-found,
no-existence-oracle rule that R-109-UX12 makes byte-identical between "withheld" and
"never existed", and that F-109-006 already forces into SQL so a denied source cannot leak
through result counts. Shipping an enumerable list would hand back, in one call, precisely
the information the rest of the authorization model spends five gates withholding.

Therefore: **Resources are addressable by template, never browsable.** A client that knows
an id can resolve it, subject to the same five gates; a client that does not know an id
cannot obtain one by listing. `resources/list` is not implemented and no resource-listing
capability is advertised during `initialize`.

---

## 10. Authorization & Egress Model

**Credential.** MCP issues its own credential with an explicit **audience** naming the MCP
server. `internal/auth/issue.go`'s `IssueToken` currently sets issuer, subject, jti, iat,
nbf, exp, footer-kid, and optional `scope` but sets **no audience**; the `Audience` type
in `internal/auth/request_authenticator.go` is unwired to issuance. Wiring audience into
issuance and verifying it on every `/mcp` request is a **conformance blocker** and is in
scope for this spec's delivery.

**Authorizer.** MCP-owned (D3). It does **not** call `bearerAuthMiddleware`. It verifies
every inbound request; it never authenticates from session state; and it resolves:

```
effective_tools = tools(granted_toolsets)
                ∩ tools(credential_scope)
                ∩ tools(allowed_egress_classes(client))
```

**Transport.** Stateless Streamable HTTP mounted at `/mcp` on the existing
`smackerel-core` listener. Sessions are never used for authentication.

**Scope minimization.** The initial scope set is minimal (`context` + `memory-read`).
Additional scopes are obtained by targeted step-up: a denial returns a
`WWW-Authenticate` challenge naming **only** the missing scope. The full scope catalog is
never published in a response.

**Egress enforcement.** Each `MCPClient` carries an operator-declared
`inference_locality`. `remote-inference` is denied unless an explicit per-client grant
exists; when granted, every invocation writes a remote-egress audit row. Smackerel cannot
verify the declaration (D1) — the model is administrative and auditable by design, and
all documentation MUST say so rather than implying technical enforcement.

---

## 11. Audit & Observability

**Audit ledger (append-only).** One row per MCP invocation: `ts`, `client_id`, `toolset`,
`tool`, `outcome`, `egress_class`, `denial_reason` (nullable), `trace_token`. Write-path
invocations additionally reuse `confirm.Machine`'s existing audit writer so proposal,
confirmation, discard, and timeout are recorded on the same trail as every other
transport.

**Metrics.**

| Metric | Labels | Purpose |
|---|---|---|
| `smackerel_mcp_tool_invocations_total` | `toolset`, `tool`, `outcome` | volume + outcome mix |
| `smackerel_mcp_tool_denied_total` | `toolset`, `tool`, `reason` | authorization pressure |
| `smackerel_mcp_remote_egress_total` | `client_id`, `toolset` | D1 accountability |
| `smackerel_mcp_execution_error_surfaced_total` | `outcome` | proves failures surfaced as failures |
| `smackerel_mcp_projection_raw_content_blocked_total` | — | **MUST always be 0**; non-zero is a P0 |

**Retrieval observability.** Every `memory-read` response carries the routing
`trace_token`, so a client-reported bad answer is traceable to the exact strategy
selection that produced it.

---

## 12. Failure Semantics

The ratified BUG-061-008 / BUG-061-009 failure-honesty invariant applies verbatim to MCP:
**a failed turn is never rendered as a successful one.**

| `agent.Outcome` | MCP rendering |
|---|---|
| `OutcomeOK` | tool result, `isError: false` |
| `OutcomeProviderError` | tool result, `isError: true`, cause preserved |
| `OutcomeTimeout` | tool result, `isError: true`, cause preserved |
| `OutcomeSchemaFailure` | tool result, `isError: true` |
| `OutcomeToolReturnInvalid` | tool result, `isError: true` |
| `OutcomeLoopLimit` | tool result, `isError: true` |
| `OutcomeInputSchemaViolation` | **JSON-RPC error** (invalid params) — protocol-level |

Additional mappings:

- Unknown tool → JSON-RPC method-not-found (protocol error).
- Malformed JSON-RPC → JSON-RPC parse/invalid-request error.
- Authorization denial → HTTP-level rejection with a targeted `WWW-Authenticate`
  challenge; no tool executes.
- Toolset unavailable due to a blocking finding (e.g. `hospitality-read` under
  BUG-019-003) → the tool is absent from `tools/list`; if invoked by name it returns
  method-not-found. It is never present-but-fake.

**Forbidden renderings:** a success-shaped result for a non-OK outcome; an empty result
set presented as an answer when retrieval errored; any capture-acknowledgement phrasing
(the "saved as an idea" class of response) on an MCP path.

---

## 13. Non-Goals

Permanent (D4a, §18 item 4, and §9B.2) — these are **deleted from the roadmap**, not deferred:

- MCP prompts
- MCP Apps
- Subscriptions, resource subscriptions, and `notifications/tools/list_changed`
- The `external` toolset and any MCP proxy-server behavior
- **An enumerable `resources/list` over the corpus, and any whole-graph resource** (§9B.2).
  An enumerable list is a **corpus-enumeration oracle**: it leaks the existence and the
  volume of what Smackerel holds even when every individual read is denied, contradicting
  the bare-403 / no-existence-oracle denial doctrine this spec enforces everywhere else
  (R-109-UX12, F-109-006). Resources are **addressable by template, never browsable**;
  `resource_link` and the five R-109-RES2 URI templates remain **in** scope.

Out of scope with a **named re-open trigger** (D4b — not deleted, per ratified §18 item 3):

- Full OAuth 2.1 authorization-server behavior, Dynamic Client Registration, and RFC 9728
  protected-resource metadata — **re-opens if and only if a client outside the operator's
  control must connect to `/mcp`**

Also out of scope for this spec:

- Multi-user or multi-tenant MCP access (`docs/smackerel.md` §1.5 non-goal)
- A fourth assistant `TransportAdapter` — MCP is a **sibling integration capability**,
  not an assistant transport
- Any change to the legacy `bearerAuthMiddleware` path (that is spec 108's surface)
- Per-artifact sensitivity ceilings (see §14 — unimplementable today)
- Exposing QF financial actions (Product Principle 10 boundary)

---

## 14. Dependencies & Blocking Findings

### F-109-001 — `hospitality-read` is blocked on BUG-019-003 (BLOCKING for J4)
`artifacts.source_ref` is never persisted by the connector front door
(`specs/019-connector-wiring/bugs/BUG-019-003-source-ref-never-persisted/`, status
`specs_hardened`). Any MCP projection citing a stable external reference is blocked on
that fix. **Resolution:** `hospitality-read` is specified but not delivered; the
Projection omits `external_ref` entirely (§9).

### F-109-002 — Spec 109 would be spec 095's first live consumer (RISK)
`internal/retrieval/routing` is the best substrate — handler-free, injectable,
`Executor.Retrieve(ctx, intent.CompiledIntent, RetrievalRequest) (RetrievalResult,
StrategySelection, error)` — but its request-path integration is deferred (PKT-095-A,
PKT-095-B, PKT-095-C) and live end-to-end coverage is deferred (F-095-E2E-LIVE). MCP
would be its **first live consumer**. **Resolution:** design must carry explicit live
integration and e2e coverage for the routing path rather than assuming spec 095 already
proved it in production. This risk is stated, not hidden.

### F-109-003 — Sensitivity ceilings are unimplementable (SCOPE CONSTRAINT)
The `artifacts` table has **no sensitivity column**. Sensitivity exists only in siloed
satellites with four incompatible vocabularies: `drive_files.sensitivity`
(`none|financial|medical|identity`), `photos.sensitivity` (`none|sensitive|hidden`),
`qf_*` (`low|medium|high`), and `docs/smackerel.md` §18.1's `Sensitive|Normal|Public`
which is implemented **nowhere**. A per-artifact sensitivity ceiling therefore cannot be
built. **Resolution:** D1 controls on client inference locality instead, and this spec
does not claim sensitivity-based filtering.

### F-109-004 — Missing `aud` claim is a conformance blocker (BLOCKING, in scope)
See §10. Wiring `Audience` into `IssueToken` and verifying it at `/mcp` is required
before any tool can be served conformantly.

### F-109-005 — The general agent registry is not an MCP source of truth (DESIGN CONSTRAINT)
`internal/agent/registry.go`'s `SideEffectClass` (`read|write|external`) cannot derive
MCP's four annotation hints, and `agent.All()` contains 19 non-capabilities. **Resolution:**
the MCP capability layer maintains its own explicit tool manifest (§8.2). No passthrough.

### F-109-006 — Source-level egress needs SQL, not a post-filter (DESIGN CONSTRAINT)
See §9. `SearchFilters` has no `source_id` and `SearchResult` does not carry it.

### Decoupling note
Spec 109 is **not blocked by spec 108** (`specs/108-corpus-grant-enforcement/`, status
`specs_hardened`). Per D3, MCP uses its own authorizer and its own audience-bound
credential, so its authorization correctness does not depend on 108's migration of the
legacy bearer path.

---

## 15. Documentation, Release, And Configuration Requirements

Every item below is required before this spec may reach a terminal status.

**Documentation**

| File | Required change |
|---|---|
| `docs/smackerel.md` §17.2 (Trust & Security) | Add the MCP boundary: D2 injection-containment rationale, why "content is data, not instructions" does not transfer to foreign agents, and the D3 no-token-passthrough rule. |
| `docs/smackerel.md` §21 (Competitive Landscape) | Add an MCP row. Fabric.so already ships MCP (§21.1 strength); record Smackerel's parity + the D1/D2 differentiation, with the honest "operator declaration, not verified" annotation. |
| `docs/smackerel.md` §22 (Connector Ecosystem & Reuse) | Add MCP to the integration inventory as a **sibling integration capability**, explicitly not a connector and not an assistant transport. |
| `docs/API.md` | Document `/mcp`: transport (stateless Streamable HTTP), credential + audience requirement, toolset inventory, Projection Contract, failure-semantics table, scope-challenge behavior. |
| `docs/Operations.md` | Operator runbook: register a client, issue an MCP credential, grant/revoke toolsets, grant/revoke `remote-inference`, read the MCP audit ledger, interpret the metrics in §11. |
| `docs/Architecture.md` | Place the MCP capability layer in the architecture: mounted at `/mcp` on `smackerel-core`, consuming domain services directly, with no internal HTTP hop and no assistant `TransportAdapter`. |
| `docs/INVESTOR_OVERVIEW.md` | Update the local-first moat narrative: MCP is shipped **and** local-first is preserved by D1 + D2. State the limitation honestly. |

**Configuration**

| File | Required change |
|---|---|
| `config/release-trains.yaml` | Confirm `next` is a declared train; spec 109 targets it. |
| `config/feature-flags.next.yaml` | `mcpKnowledgeServer: true` (owning train, default-ON). |
| `config/feature-flags.mvp.yaml` | `mcpKnowledgeServer: false` (default-OFF). |

**Flag contract.** Exactly one flag is introduced: `mcpKnowledgeServer`. It is default-ON
in exactly one train (`next`) and default-OFF in every other train. It is read from an
env var with **no fallback default** — a missing value is a startup failure, per the
NO-DEFAULTS SST policy. `remote-inference` is deliberately **not** a train flag: it is a
per-client operator grant recorded in the MCP client registry (D1), so it can never be
switched on globally by a build.

**State.** The feature's `state.json` must declare `releaseTrain: "next"` and
`flagsIntroduced: ["mcpKnowledgeServer"]`.

---

## 16. Product Principle Alignment

Principle numbers and names are cited from `docs/Product-Principles.md`.

| Principle | Alignment |
|---|---|
| **P4 — Source-Qualified Processing** | The Projection Contract preserves `source_kind` from `routing.RetrievedSource`. MCP never strips source qualification for "simplicity"; it is a required field (§9). |
| **P5 — One Graph, Many Views** | MCP is the canonical expression of this principle: a new *view* over the existing graph, not a parallel store, not a parallel index. No new artifact type, no second search backend. |
| **P6 — Invisible By Default, Felt Not Heard** | MCP is pull-only (D4). No subscriptions, no `tools/list_changed`, no server-initiated messages — the ratified §1.4 budget of "< 3 system-initiated prompts per week" is untouched. |
| **P8 — Trust Through Transparency** | Three ways: every projection carries retrieval provenance and a `trace_token`; no fabricated capability is advertised (SCN-109-004); and failures render as failures (§12). |
| **P2 — Vague In, Precise Out** | `memory.search` accepts a natural-language query compiled into an `intent.CompiledIntent`; it does not demand exact tags or dates. Semantic retrieval stays the primary path. |
| **P3 — Knowledge Breathes** | `lifecycle_state` is a first-class Projection field, so external clients see lifecycle rather than a flat, static snapshot. |
| **P10 — QF Companion Boundary** | Untouched. No MCP tool initiates trade approval, mandate change, execution, or financial advice. |

**Principle-gap note — RESOLVED by ratified §18 item 7 (2026-07-29).** The commissioning
brief cited *"Product Principle 9 — Own your data"*. `docs/Product-Principles.md` Principle 9
is **"Design For Restart, Not Perfection"**, and no numbered product principle states data
ownership — the local-first commitment is carried today by **Constitution C1 (Local-First)**,
`docs/smackerel.md` §21.4 (UVP), and `docs/INVESTOR_OVERVIEW.md`. Those numbering facts stand.
The open question they raised — whether `docs/Product-Principles.md` should gain an explicit
local-first principle, or whether C1 remains the sole carrier — is **decided**: the document
**gains** a new principle, proposed as **Principle 11 — Your Data Stays Yours** (§18 item 7
carries the proposed text). Constitution C1 is **not** the sole carrier, because
`docs/Product-Principles.md` states the constitution is a separate enforcement track, and a
product claim this load-bearing must live on the product track.

The edit to `docs/Product-Principles.md` is **not** made by this spec and **not** made by this
packet: that document is owner-ratified, so the amendment is a delivery-time obligation in
`scopes.md` Scope 07, flagged as **requiring explicit owner sign-off**. Until it lands, this
spec continues to cite only the ten existing ratified principles and invents none.

---

## 17. Release Train

**Train:** `next`.

Rationale: MCP introduces a new externally-reachable ingress with a new credential type
and a new egress class. It must ride a train where it can be default-ON and exercised,
while `mvp` stays default-OFF. Per the trunk + release-train model there are no long-lived
feature branches: the capability lands on trunk behind `mcpKnowledgeServer`, default-ON in
`next` only.

Promotion of `next` remains subject to the standard promote gates (backup freshness,
restore-drill currency). This spec adds no exception to them.

---

## 18. Operator Decision Record — RATIFIED (review gate CLOSED)

**Status: RATIFIED by operator delegation on 2026-07-29.** The operator delegated all seven
decisions below under the standing instruction *"pick the best option for long term, no
shortcuts."* Each item records the **decision**, its **rationale**, and whether it is
**permanent** or **trigger-conditioned**. Item numbering 1–7 is preserved verbatim so every
existing cross-reference in `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, and
`state.json` stays valid.

**Ratification boundary — what this gate did and did not settle.** It ratifies exactly the
seven decisions below. It does **not** ratify the editorial findings **UX-F-002**,
**UX-F-004**, and **UX-F-005**, which §9A.7 had bundled into "the operator's §18 pass" for
convenience. Those are wording corrections to `spec.md`, not product decisions; they remain
**OPEN**, owned by `bubbles.analyst`, and are re-routed accordingly in §9A.7. Recording them
as ratified would be overclaiming.

**Status effect.** Ratification closes the review gate. It does **not** change this packet's
status: the workflow mode remains `product-to-planning` with ceiling `specs_hardened`, no
implementation is claimed, and no test result is asserted.

---

### 1. D1 acceptance — **ACCEPTED, with a binding honesty constraint** (permanent)

**RATIFIED 2026-07-29 by operator delegation.**

**Decision.** `remote-inference` stands as a per-client operator grant that is
administratively controlled and fully audited, but **not technically verifiable**.

**Rationale.** No MCP server can technically verify where a client's model executes. The
server sees a client; it never sees the client's inference topology. That is a property of
the protocol, universal to every MCP implementation — **not** a Smackerel weakness, and not
something a future release closes. The long-term-correct posture is therefore exactly the
one D1 already takes: (a) `local-inference` is the default and only enabled egress class;
(b) remote is an explicit, per-client, individually audited grant; (c) no user-facing or
marketing text ever claims technical verification.

**BINDING CONSTRAINT (normative, applies beyond this feature).** The **only** permitted
claim shape about MCP egress is:

> *"Smackerel never sends your knowledge anywhere; a client you explicitly authorize may."*

That sentence is literally true and independently verifiable, because it asserts only what
Smackerel's own egress does. Any phrasing that implies Smackerel **enforces**, **verifies**,
**guarantees**, or **attests** client-side inference locality is a **defect** — in
`docs/smackerel.md` §21.4, `docs/INVESTOR_OVERVIEW.md`, `docs/API.md`, `docs/Operations.md`,
the MCP operator surface, release packets, and marketing copy alike. Scope 07 carries this
as SCN-109-P03 and a blocking DoD item.

**Permanence.** Permanent. The honesty constraint binds every future surface, not just this
feature's documentation pass.

**Not resolved by this item.** UX-F-005 (D1's "fully coded but default-OFF" wording, which
read literally conflicts with §1's "no MCP server exists") is an editorial defect in D1's
prose, not a question about D1's posture. It remains OPEN — see the ratification boundary
above.

### 2. D2 absoluteness — **ACCEPTED, permanent, no escape hatch** (permanent)

**RATIFIED 2026-07-29 by operator delegation.**

**Decision.** `content_raw` is **never** returned over MCP under **any** configuration —
including a fully local client the operator personally trusts. No "raw mode", no debug flag,
no per-client override, ever.

**Rationale.**

1. **Client trust is irrelevant to the threat.** D2 is a control against prompt injection,
   and the injected text lives in the **corpus**, not in the client. Smackerel passively
   ingests email from arbitrary senders, so attacker-controlled sentences are in the store by
   design. A fully trusted local client is just as effectively weaponized by an attacker's
   sentence as an untrusted one. "Trusted client" therefore does not weaken the reason the
   control exists — which is precisely why a trust-scoped exception would be unsound.
2. **It is constitutional.** Constitution C3 / Design Principle 2 — *"processed, not raw"* —
   is not a preference this feature may trade away.
3. **Escape hatches metastasize.** A "raw mode for trusted local clients" becomes the default
   path within two releases: first as a debugging convenience, then as the fast path, then as
   the documented one. The only reliable way to not have that happen is to not build the door.

**Cost of the decision — low, and stated.** Legitimate full-document needs are already served:
the existing `whole_document` retrieval strategy returns a synthesized full-document view
through the Projection Contract. D2 removes a raw pipe, not a capability.

**Binding forward constraint.** **No future spec may add a raw-content toolset, a raw-content
Projection field, or a raw-content mode without explicitly superseding this decision.** A spec
that adds one without superseding D2 is a defect, not a feature.

**Permanence.** Permanent.

### 3. D4 permanence — **ACCEPTED WITH ONE CARVE-OUT** (split: permanent + trigger-conditioned)

**RATIFIED 2026-07-29 by operator delegation.**

This is the only one of the seven that **amends** an existing decision of record. D4 (§3) and
§13 are updated accordingly.

**3a — PERMANENTLY DELETED (never re-opened).**

- MCP prompts
- MCP Apps
- Subscriptions, resource subscriptions, and `notifications/tools/list_changed`

**Rationale.** These serve a multi-tenant client ecosystem that does not exist here;
`docs/smackerel.md` §1.5 lists multi-user as an explicit non-goal. Subscriptions in particular
conflict with the **ratified §1.4 success metric "system-initiated prompts < 3 per week"** and
with Product Principle 6 (Invisible By Default, Felt Not Heard). **MCP is pull-only by design**,
and that is a product stance, not a capacity constraint.

**3b — NOT permanently deleted. OUT OF SCOPE WITH A NAMED RE-OPEN TRIGGER.**

- Full OAuth 2.1 authorization-server behavior
- Dynamic Client Registration (DCR)
- RFC 9728 protected-resource metadata

**Rationale.** Calling these "permanently deleted" would be **overclaiming**. They are
genuinely unnecessary for a tailnet-only, single-operator deployment where every client is
first-party and the operator issues every credential by hand. But they become **REQUIRED** —
not optional, not nice-to-have — the moment a client outside the operator's control must
connect.

**Re-open trigger (exact, normative):**

> **This decision re-opens if and only if a client outside the operator's control must connect
> to `/mcp`.**

**Why a trigger, not a date.** Dated deferrals rot: "revisit in Q3" becomes a stale TODO that
nobody reads and nobody can act on, because the date carries no information about whether the
work is needed. A trigger condition does not rot, because the trigger is **observable** and the
answer stays correct until it fires. Until it fires, the correct engineering answer is
genuinely "do not build it."

### 4. `external` toolset — **ACCEPTED permanent-off** (permanent)

**RATIFIED 2026-07-29 by operator delegation.**

**Decision.** The `external` toolset is permanently off. Smackerel will never be an MCP proxy
server and will therefore never need the confused-deputy mitigation set.

**Rationale.** Enabling drive/photos/provider calls converts Smackerel into an MCP **proxy
server** in the specification's own terminology. That immediately triggers the full
confused-deputy mitigation set — a per-client consent registry, an MCP-owned consent UI, exact
`redirect_uri` matching, and single-use `state` — which is an entire spec of security surface
for a job the client's own MCP servers already do **better**. A client that wants Google Drive
should use the Google Drive MCP server; it should not proxy through Smackerel, which would add
a hop, add a consent surface, and add a place for authority to be confused, while subtracting
nothing.

**Permanence.** Permanent, and permanently *correct* — this is a clean architectural boundary
(Smackerel is a knowledge server, not an integration broker), not a resourcing decision that a
larger team would reverse.

### 5. J4 sequencing — **ACCEPTED** (permanent while its cause holds)

**RATIFIED 2026-07-29 by operator delegation.**

**Decision.** `hospitality-read` ships **only after** BUG-019-003. The STR host job (J4) waits.

**Rationale.** `artifacts.source_ref` is NULL for every connector until BUG-019-003 is fixed
(`specs/019-connector-wiring/bugs/BUG-019-003-source-ref-never-persisted/`). Shipping
`hospitality-read` earlier therefore means emitting a **fabricated external reference** — the
exact dishonesty §9's external-reference honesty rule forbids and the exact failure mode this
whole feature exists to prevent. Waiting is the only honest option. **No shortcut.**

**Permanence.** The constraint is discharged by the **fix**, not by a schedule or a deadline.
Scope 06 remains **Blocked** and is planned as honestly-unavailable (registered with
`Readiness.Available=false`, omitted from `tools/list`, method-not-found on invocation) rather
than as deliverable.

### 6. Spec 095 exposure — **ACCEPTED** (permanent)

**RATIFIED 2026-07-29 by operator delegation.**

**Decision.** MCP becomes the **first live consumer** of `internal/retrieval/routing`, and
live integration + `e2e-api` coverage for that path is **funded inside this feature's scopes**
(Scope 03, TP-03-07).

**Rationale.** Building a parallel retrieval path for MCP would violate **Product Principle 5
(One Graph, Many Views)** and create exactly the parallel-store problem the constitution
forbids. Consuming the existing executor is the architecturally correct answer even though it
means absorbing first-consumer risk.

**Side benefit — recorded honestly, and bounded.** This retires part of spec 095's
deferred-integration debt: the MCP path ships its own concrete `cmd/core` wiring rather than
waiting on PKT-095-A/B/C, and TP-03-07 discharges **F-095-E2E-LIVE for the MCP path
specifically** — converting a deferral into shipped, exercised code. It is **not** a claim that
spec 095 is generally proven in production, and **no document may state otherwise**.

**Risk — recorded honestly, and owned.** The executor is unit-tested but **has never run in a
live request path**. First-consumer defects are therefore *expected*, and absorbing them is
this feature's cost, not a surprise to be re-litigated later. No scope may be planned or
estimated as if the substrate were already proven.

### 7. Principle gap — **RESOLVED: ADD a new product principle** (permanent)

**RATIFIED 2026-07-29 by operator delegation.**

**Decision.** `docs/Product-Principles.md` **gains** an explicit local-first / data-ownership
product principle. **Constitution C1 does NOT remain the sole carrier.**

**Proposed number.** **Principle 11** — the next available number.
`docs/Product-Principles.md` currently carries exactly ten principles (1–10), all stamped
"Ratified 2026-06-03".

**Proposed name.** **Principle 11 — Your Data Stays Yours.**

**Proposed text (one paragraph, for owner sign-off):**

> **Principle 11 — Your Data Stays Yours.** Smackerel runs on hardware the user controls, and
> the user's knowledge never leaves it by Smackerel's own action. Capture, storage, embedding,
> retrieval, and synthesis execute locally by default; local inference is the default and only
> enabled egress class, and any remote-inference path is an explicit, per-client, individually
> audited grant the user makes — never a default, never a build-time switch, never silent.
> Where Smackerel cannot technically verify a downstream consumer's behavior — as with a
> connected MCP client, whose inference locality **no** server can verify — it says so plainly
> rather than implying an enforcement it does not have; the permitted claim is *"Smackerel
> never sends your knowledge anywhere; a client you explicitly authorize may."* Data ownership
> also means the user can export, relocate, or delete the entire corpus without asking
> permission, and that accumulated value must never become a switching barrier.

**Rationale.** `docs/Product-Principles.md` states explicitly that the constitution is a
**separate enforcement track**. Local-first is the product's single biggest differentiator —
`docs/smackerel.md` §21.4 states it as the UVP, and `docs/INVESTOR_OVERVIEW.md` calls it *"a
moat, not a constraint"* — yet it exists only as an **engineering** constitution principle
(C1). That is a real gap: a **product claim with no product principle**. D1's entire posture
rests on it, which means D1 currently borrows an engineering principle to justify a product
decision. Closing the gap puts the load-bearing commitment on the track that actually governs
product review.

**GOVERNANCE CONSTRAINT — the edit is NOT made here.** `docs/Product-Principles.md` is an
owner-ratified document ("Ratified 2026-06-03"; *"Edits go through the normal product-principles
change process"*). This section records the **decision only**. The edit itself is a
**delivery-time obligation** carried by `scopes.md` **Scope 07**, where it is flagged as
**requiring explicit owner sign-off** because it amends a ratified product document. No agent
and no planning packet may apply it unilaterally.

**Enforcement companion — same change set, no shortcut.** `docs/Product-Principles.md` declares
its companion file `.github/instructions/product-principles.instructions.md` **BLOCKING**, and
that file carries a per-principle enforcement block for each of P1–P10. Principle 11 therefore
ships with a matching enforcement block **in the same change set**. Adding the principle without
it would leave the product's biggest differentiator unenforced — which is the very "claim with no
enforcement track" shape this item exists to eliminate. Scope 07 carries this as a second
owner-sign-off DoD item, and TP-07-03 fails if the principle lands without the enforcement block.

**Consequence.** §16's honest-discrepancy note is updated from "open owner decision" to
"resolved — Principle 11 to be added at delivery, owner sign-off required". **UX-F-003**'s
*principle-gap* half is resolved by this item; its *numbering* half (Trust Through Transparency
is Principle 8, and the commissioning brief's "Principle 11" pointed at a principle that did
not yet exist) was always a statement of fact, not an open question.

---

### Ratification provenance

| Field | Value |
|---|---|
| Ratified on | 2026-07-29 |
| Ratified by | Operator, by explicit delegation |
| Delegation instruction | *"pick the best option for long term, no shortcuts."* |
| Items ratified | 1–7 (all) |
| Items **not** ratified by this gate | UX-F-002, UX-F-004, UX-F-005 (editorial; owner `bubbles.analyst`) |
| Decisions amended as a result | D4 (§3) and §13 — item 3's OAuth/DCR/RFC-9728 carve-out |
| Status effect | None. Mode `product-to-planning`, ceiling and status remain `specs_hardened`. |
